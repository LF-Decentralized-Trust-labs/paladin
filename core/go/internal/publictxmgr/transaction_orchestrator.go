/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package publictxmgr

import (
	"context"
	"math/big"
	"sync"
	"time"

	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/persistence"

	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/retry"
)

const (
	// what happens to the transaction orchestrator processing loop if the balance of the current account cannot be retrieved
	OrchestratorUnavailableBalanceHandlerString = "unavailableBalanceHandler"
)

// what happens to the transaction orchestrator processing loop if the balance of the current account cannot be retrieved
type OrchestratorBalanceCheckUnavailableBalanceHandlingStrategy string

const (
	// stop the transaction orchestrator
	OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyStop OrchestratorBalanceCheckUnavailableBalanceHandlingStrategy = "stop"
	// put the transaction orchestrator into a wait state until balance becomes available
	OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyWait OrchestratorBalanceCheckUnavailableBalanceHandlingStrategy = "wait"
	// act as if balance check is turned off
	OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyContinue OrchestratorBalanceCheckUnavailableBalanceHandlingStrategy = "continue"
)

type OrchestratorState string

const (
	// created transaction orchestrator, but not started
	OrchestratorStateNew OrchestratorState = "new"
	// transaction orchestrator running normally
	OrchestratorStateRunning OrchestratorState = "running"
	// transaction orchestrator is blocked and waiting for precondition to be fulfilled, e.g. waiting for sufficient balance
	OrchestratorStateWaiting OrchestratorState = "waiting"
	// the head of the in-flight transaction queue hasn't changed after staleTimeout
	OrchestratorStateStale OrchestratorState = "stale"
	// the queue is empty
	OrchestratorStateIdle OrchestratorState = "idle"
	// transaction orchestrator is paused
	OrchestratorStatePaused OrchestratorState = "paused"
	// transaction orchestrator is stopped
	OrchestratorStateStopped OrchestratorState = "stopped"
)

var AllOrchestratorStates = []string{
	string(OrchestratorStateNew),
	string(OrchestratorStateRunning),
	string(OrchestratorStateWaiting),
	string(OrchestratorStateStale),
	string(OrchestratorStateIdle),
	string(OrchestratorStatePaused),
	string(OrchestratorStateStopped),
}

// a transaction is considered in-flight only if its in the queue of any transaction orchestrator
// role of transaction orchestrator:
// 1. polling transaction persistence to fetch new transactions of a given signing address, all the way to the limit if possible, regardless of whether there are stale transaction / lack of fund.
// 2. process transactions based on an interval
//    - action none event driven transaction stage
//      - stale transaction check and handling
//      - action signing request and retries
//      - action gas price calculation and retries
//      - action gas limit estimation and retries
//      - action transaction submission and retries
//    - record self-deletion request (based on settings) for its transaction engine to action
//      - the same stale transaction ID has been staying at the front of the queue for a period of time (based on settings)
//    - decide when to stop iterating the queue
//      - when ran out of fund
//      - when self-deletion request has been raised after processing the previous transaction
// 3. persist updates of the managed transactions
// 4. Nonce management
//    - need more thinking on nonce too low and we lost the information for tracking the submitted transactions

type orchestrator struct {
	*pubTxManager

	// in-flight transaction config
	resubmitInterval        time.Duration
	stageRetryTimeout       time.Duration
	persistenceRetryTimeout time.Duration
	ethClient               ethclient.EthClient
	bIndexer                blockindexer.BlockIndexer

	transactionSubmissionRetry *retry.Retry

	// each transaction orchestrator has its own go routine
	orchestratorBirthTime       time.Time           // when transaction orchestrator is created
	orchestratorPollingInterval time.Duration       // between how long the transaction orchestrator will do a poll and trigger none-event driven transaction process actions
	signingAddress              pldtypes.EthAddress // the signing address of the transaction managed by the current transaction orchestrator

	// balance check settings
	hasZeroGasPrice                    bool
	unavailableBalanceHandlingStrategy OrchestratorBalanceCheckUnavailableBalanceHandlingStrategy

	// in flight txs array
	maxInFlightTxs       int
	inFlightTxs          []*inFlightTransactionStageController // a queue of all the in flight transactions
	inFlightTxsMux       sync.Mutex
	orchestratorLoopDone chan struct{}
	InFlightTxsStale     chan bool

	// input channels
	stopProcess chan bool // a channel to tell the current transaction orchestrator to stop processing all events and mark itself as to be deleted

	// Metrics provided for fairness control in the controler
	totalCompleted int64 // total number of transaction completed since birth time
	state          OrchestratorState
	stateEntryTime time.Time // when it's run last time

	staleTimeout    time.Duration
	lastQueueUpdate time.Time

	lastNonceAlloc time.Time
	nextNonce      *uint64

	// updates
	updates   []*transactionUpdate
	updateMux sync.Mutex

	timeLineLoggingMaxEntries int
}

const veryShortMinimum = 50 * time.Millisecond

func NewOrchestrator(
	ptm *pubTxManager,
	signingAddress pldtypes.EthAddress,
	conf *pldconf.PublicTxManagerConfig,
) *orchestrator {
	ctx := ptm.ctx

	newOrchestrator := &orchestrator{
		pubTxManager:                ptm,
		orchestratorBirthTime:       time.Now(),
		orchestratorPollingInterval: confutil.DurationMin(conf.Orchestrator.Interval, veryShortMinimum, *pldconf.PublicTxManagerDefaults.Orchestrator.Interval),
		maxInFlightTxs:              confutil.IntMin(conf.Orchestrator.MaxInFlight, 1, *pldconf.PublicTxManagerDefaults.Orchestrator.MaxInFlight),
		signingAddress:              signingAddress,
		state:                       OrchestratorStateNew,
		stateEntryTime:              time.Now(),
		unavailableBalanceHandlingStrategy: OrchestratorBalanceCheckUnavailableBalanceHandlingStrategy(
			confutil.StringNotEmpty(conf.Orchestrator.UnavailableBalanceHandler, string(OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyWait))),

		// in-flight transaction configs
		resubmitInterval:        confutil.DurationMin(conf.Orchestrator.ResubmitInterval, veryShortMinimum, *pldconf.PublicTxManagerDefaults.Orchestrator.ResubmitInterval),
		stageRetryTimeout:       confutil.DurationMin(conf.Orchestrator.StageRetryTime, veryShortMinimum, *pldconf.PublicTxManagerDefaults.Orchestrator.StageRetryTime),
		persistenceRetryTimeout: confutil.DurationMin(conf.Orchestrator.PersistenceRetryTime, veryShortMinimum, *pldconf.PublicTxManagerDefaults.Orchestrator.PersistenceRetryTime),

		// submission retry
		transactionSubmissionRetry: retry.NewRetryLimited(&conf.Orchestrator.SubmissionRetry),
		staleTimeout:               confutil.DurationMin(conf.Orchestrator.StaleTimeout, 0, *pldconf.PublicTxManagerDefaults.Orchestrator.StaleTimeout),
		hasZeroGasPrice:            ptm.gasPriceClient.HasZeroGasPrice(ctx),
		InFlightTxsStale:           make(chan bool, 1),
		stopProcess:                make(chan bool, 1),
		ethClient:                  ptm.ethClient,
		bIndexer:                   ptm.bIndexer,
		timeLineLoggingMaxEntries:  conf.Orchestrator.TimeLineLoggingMaxEntries,
	}

	log.L(ctx).Debugf("NewOrchestrator for signing address %s created: %+v", newOrchestrator.signingAddress, newOrchestrator)

	return newOrchestrator
}

func (oc *orchestrator) orchestratorLoop() {
	ctx := log.WithLogField(oc.ctx, "role", "orchestrator-loop")
	log.L(ctx).Infof("Orchestrator for signing address %s started polling based on interval %s", oc.signingAddress, oc.orchestratorPollingInterval)

	defer close(oc.orchestratorLoopDone)

	if err := oc.initNextNonceFromDBRetry(ctx); err != nil {
		log.L(ctx).Warnf("Context cancelled while obtaining highest nonce for %s: %s", oc.signingAddress, err)
		return
	}

	ticker := time.NewTicker(oc.orchestratorPollingInterval)
	defer ticker.Stop()
	for {
		// an InFlight
		select {
		case <-oc.InFlightTxsStale:
		case <-ticker.C:
		case <-ctx.Done():
			log.L(ctx).Infof("Orchestrator loop exit due to canceled context, it processed %d transaction during its lifetime.", oc.totalCompleted)
			return
		case <-oc.stopProcess:
			log.L(ctx).Infof("Orchestrator loop process stopped, it processed %d transaction during its lifetime.", oc.totalCompleted)
			oc.state = OrchestratorStateStopped
			oc.stateEntryTime = time.Now()
			oc.MarkInFlightOrchestratorsStale() // trigger engine loop for removal
			return
		}
		oc.handleUpdates(ctx)
		polled, total := oc.pollAndProcess(ctx)
		log.L(ctx).Debugf("Orchestrator loop polled %d txs, there are %d txs in total", polled, total)
	}

}

func (oc *orchestrator) handleUpdates(ctx context.Context) {
	oc.updateMux.Lock()
	updates := oc.updates
	oc.updates = nil
	oc.updateMux.Unlock()

	oc.inFlightTxsMux.Lock()
	defer oc.inFlightTxsMux.Unlock()

	for _, update := range updates {
		for _, inflight := range oc.inFlightTxs {
			if inflight.stateManager.GetPubTxnID() == update.pubTXID {
				inflight.UpdateTransaction(ctx, update.newPtx)
				oc.MarkInFlightTxStale()
				break
			}
		}
	}
}

// Used in unit tests
func (oc *orchestrator) getFirstInFlight() (ift *inFlightTransactionStageController) {
	oc.inFlightTxsMux.Lock()
	defer oc.inFlightTxsMux.Unlock()
	if len(oc.inFlightTxs) > 0 {
		ift = oc.inFlightTxs[0]
	}
	return
}

func (oc *orchestrator) initNextNonceFromDBRetry(ctx context.Context) error {
	return oc.retry.Do(ctx, func(attempt int) (retryable bool, err error) {
		return true, oc.initNextNonceFromDB(ctx)
	})
}

func (oc *orchestrator) initNextNonceFromDB(ctx context.Context) error {
	var txns []*DBPublicTxn
	err := oc.p.DB().
		WithContext(ctx).
		Where(`"from" = ?`, oc.signingAddress).
		Where("nonce IS NOT NULL").
		Order("nonce DESC").
		Limit(1).
		Find(&txns).
		Error
	if err != nil || len(txns) == 0 {
		return err
	}
	nextNonce := *txns[0].Nonce + 1
	oc.nextNonce = &nextNonce
	log.L(ctx).Infof("Next nonce initialized from DB from %s: %d", oc.signingAddress, nextNonce)
	return nil
}

func (oc *orchestrator) allocateNonces(ctx context.Context, txns []*DBPublicTxn) error {

	// Some of the the transactions might have nonces already
	toAlloc := make([]*DBPublicTxn, 0, len(txns))
	for _, tx := range txns {
		if tx.Nonce == nil {
			toAlloc = append(toAlloc, tx)
		}
	}
	if len(toAlloc) == 0 {
		// Nothing to do
		return nil
	}

	// We need to ensure we have the next nonce to allocate
	if oc.nextNonce == nil || time.Since(oc.lastNonceAlloc) > oc.nonceCacheTimeout {
		log.L(ctx).Debugf("no cached nonce, or nonce expired for %s (cached=%v)", oc.signingAddress, oc.lastNonceAlloc)
		txCount, err := oc.ethClient.GetTransactionCount(ctx, oc.signingAddress)
		if err != nil {
			return err
		}
		// See if we have nonces in our DB that are ahead of the mempool.
		if oc.nextNonce != nil && *oc.nextNonce >= txCount.Uint64() {
			log.L(ctx).Infof("Next nonce for %s is %d (at or ahead of mempool %d)", oc.signingAddress, *oc.nextNonce, txCount.Uint64())
		} else {
			// Otherwise take the node's answer
			oc.nextNonce = (*uint64)(txCount)
			log.L(ctx).Infof("Next nonce for %s set to %d (from eth_getTransactionCount)", oc.signingAddress, *oc.nextNonce)
		}
	}

	// Set up the list of nonces we'll allocated, but until it's in the DB we do NOT update the oc.nextNonce beyond the first in the list
	newNextNonce := *oc.nextNonce
	newNonces := make([]uint64, len(toAlloc))
	for i := range newNonces {
		newNonces[i] = newNextNonce
		newNextNonce++
	}

	// Run the DB TXN using a VALUES temp table to update multiple rows in a single operation
	err := oc.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		sqlQuery := `WITH nonce_updates ("pub_txn_id", "nonce") AS ( VALUES `
		values := make([]any, 0, len(toAlloc)*2)
		for i, tx := range toAlloc {
			if i > 0 {
				sqlQuery += `, `
			}
			sqlQuery += `( CAST (? AS BIGINT), CAST (? AS BIGINT) ) `
			values = append(values, tx.PublicTxnID)
			values = append(values, newNonces[i])
			log.L(ctx).Debugf("assigning %s:%d (pubTxnId=%d)", oc.signingAddress, newNonces[i], tx.PublicTxnID)
		}
		sqlQuery += ` ) UPDATE "public_txns" SET "nonce" = nu."nonce" FROM ( SELECT "pub_txn_id", "nonce" FROM nonce_updates ) AS nu ` +
			`WHERE "public_txns"."pub_txn_id" = nu."pub_txn_id";`
		return dbTX.DB().WithContext(ctx).Exec(sqlQuery, values...).Error
	})
	if err != nil {
		return err
	}

	// Update the txns themselves, and our nextNonce
	for i, tx := range toAlloc {
		nonce := newNonces[i]
		tx.Nonce = &nonce
	}
	oc.lastNonceAlloc = time.Now()
	oc.nextNonce = &newNextNonce

	return nil
}

func (oc *orchestrator) pollAndProcess(ctx context.Context) (polled int, total int) {
	pollStart := time.Now()
	oc.inFlightTxsMux.Lock()
	defer oc.inFlightTxsMux.Unlock()
	queueUpdated := false

	oldInFlight := oc.inFlightTxs
	oc.inFlightTxs = make([]*inFlightTransactionStageController, 0, len(oldInFlight))

	stageCounts := make(map[string]int)
	for _, stageName := range AllInFlightStages {
		// map for saving number of in flight transaction per stage
		stageCounts[stageName] = 0
	}

	var highestInFlightNonce *uint64
	// Run through copying across from the old InFlight list to the new one, those that aren't ready to be deleted
	for _, p := range oldInFlight {
		if highestInFlightNonce == nil || p.stateManager.GetNonce() > *highestInFlightNonce {
			newHighest := p.stateManager.GetNonce()
			highestInFlightNonce = &newHighest
		}
		if p.stateManager.CanBeRemoved(ctx) {
			oc.totalCompleted = oc.totalCompleted + 1
			queueUpdated = true
			log.L(ctx).Debugf("Orchestrator poll and process, marking %s as complete after: %s", p.stateManager.GetSignerNonce(), time.Since(p.stateManager.GetCreatedTime().Time()))
			p.PrintTimeline()
		} else {
			log.L(ctx).Debugf("Orchestrator poll and process, continuing tx %s after: %s", p.stateManager.GetSignerNonce(), time.Since(p.stateManager.GetCreatedTime().Time()))
			oc.inFlightTxs = append(oc.inFlightTxs, p)
			txStage := p.stateManager.GetStage(ctx)
			if string(txStage) == "" {
				txStage = InFlightTxStageQueued
			}
			stageCounts[string(txStage)] = stageCounts[string(txStage)] + 1
		}
	}

	log.L(ctx).Debugf("Orchestrator poll and process, stage counts: %+v", stageCounts)
	oldLen := len(oc.inFlightTxs)
	total = oldLen
	// check and poll new transactions from the persistence if we can handle more
	// If we are not at maximum, then query if there are more candidates now
	spaces := oc.maxInFlightTxs - oldLen
	if spaces > 0 {
		// We retry the get from persistence indefinitely (until the context cancels)
		var additional []*DBPublicTxn
		err := oc.retry.Do(ctx, func(attempt int) (retry bool, err error) {
			q := oc.p.DB().
				WithContext(ctx).
				Table("public_txns").
				Joins("Completed").
				Where(`"Completed"."tx_hash" IS NULL`).
				Where("suspended IS FALSE").
				Where(`"from" = ?`, oc.signingAddress).
				Order(`"public_txns"."pub_txn_id"`).
				Limit(spaces)
			if len(oc.inFlightTxs) > 0 {
				// We don't want to see any of the ones we already have in flight.
				// The only way something leaves our in-flight list, is if we get a notification from the block indexer
				// that it committed a DB transaction that removed it from our list.
				q = q.Where("(nonce IS NULL OR nonce > ?)", highestInFlightNonce)
			}
			// Note we do not use an explicit DB transaction to coordinate the read of the
			// transactions table with the read of the submissions table,
			// as we are the only thread that writes to the submissions table, for
			// inflight transactions we have in memory that would not be overwritten
			// by this query.
			additional, err = oc.runTransactionQuery(ctx, oc.p.NOTX(), false /* just the individual transactions - no duplication for bindings */, nil, q)
			return true, err
		})
		if err != nil {
			log.L(ctx).Infof("Orchestrator poll and process: context cancelled while retrying")
			return -1, len(oc.inFlightTxs)
		}

		// Synchronously we ensure that we have a nonce for all of these.
		// This is an indefinite retry, as we MUST not proceed until a nonce has been allocated+stored for every one
		// of these transactions. Otherwise we might re-order transactions compared to their DB commit order
		// (which is unacceptable for strict TX ordering).
		if err := oc.retry.Do(ctx, func(attempt int) (retryable bool, err error) {
			return true, oc.allocateNonces(ctx, additional)
		}); err != nil {
			log.L(ctx).Warnf("Orchestrator context cancelled while allocating nonce: %s", err)
			return
		}

		log.L(ctx).Debugf("Orchestrator poll and process: polled %d items, space: %d", len(additional), spaces)
		for _, ptx := range additional {
			queueUpdated = true
			it := NewInFlightTransactionStageController(oc.pubTxManager, oc, ptx)
			oc.inFlightTxs = append(oc.inFlightTxs, it)
			txStage := it.stateManager.GetStage(ctx)
			if string(txStage) == "" {
				txStage = InFlightTxStageQueued
			}
			stageCounts[string(txStage)] = stageCounts[string(txStage)] + 1
			log.L(ctx).Debugf("Orchestrator added transaction with PublicTxnID=%d From=%s", ptx.PublicTxnID, ptx.From)
		}
		total = len(oc.inFlightTxs)
		polled = total - oldLen
		if polled > 0 {
			log.L(ctx).Debugf("InFlight set updated len=%d head-nonce=%d tail-nonce=%d old-tail=%d", len(oc.inFlightTxs), oc.inFlightTxs[0].stateManager.GetNonce(), oc.inFlightTxs[total-1].stateManager.GetNonce(), highestInFlightNonce)
		}
		oc.thMetrics.RecordInFlightTxQueueMetrics(ctx, stageCounts, oc.maxInFlightTxs-len(oc.inFlightTxs))
	}
	log.L(ctx).Debugf("Orchestrator polling from DB took %s", time.Since(pollStart))
	// now check and process each transaction

	if total > 0 {
		waitingForBalance, _ := oc.ProcessInFlightTransactions(ctx, oc.inFlightTxs)
		if queueUpdated {
			oc.lastQueueUpdate = time.Now()
		}
		if time.Since(oc.lastQueueUpdate) > oc.staleTimeout && oc.state != OrchestratorStateStale {
			oc.state = OrchestratorStateStale
			oc.stateEntryTime = time.Now()
		} else if waitingForBalance && oc.state != OrchestratorStateWaiting {
			oc.state = OrchestratorStateWaiting
			oc.stateEntryTime = time.Now()
		} else if oc.state != OrchestratorStateRunning {
			oc.state = OrchestratorStateRunning
			oc.stateEntryTime = time.Now()
		}
	} else if oc.state != OrchestratorStateIdle {
		oc.state = OrchestratorStateIdle
		oc.stateEntryTime = time.Now()
	}
	log.L(ctx).Debugf("Orchestrator process loop took %s", time.Since(pollStart))

	return polled, total
}

// this function should only have one running instance at any given time
func (oc *orchestrator) ProcessInFlightTransactions(ctx context.Context, its []*inFlightTransactionStageController) (waitingForBalance bool, err error) {
	processStart := time.Now()
	waitingForBalance = false
	var addressAccount *AddressAccount
	skipBalanceCheck := oc.hasZeroGasPrice
	now := time.Now()
	log.L(ctx).Debugf("%s ProcessInFlightTransaction entry for signing address %s", now.String(), oc.signingAddress)

	if !skipBalanceCheck {
		log.L(ctx).Debugf("%s: ProcessInFlightTransaction checking balance for %s", now.String(), oc.signingAddress)

		addressAccount, err = oc.balanceManager.GetAddressBalance(oc.ctx, oc.signingAddress)
		if err != nil {
			log.L(ctx).Errorf("Failed to retrieve balance for address %s due to %+v", oc.signingAddress, err)
			if oc.unavailableBalanceHandlingStrategy == OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyWait {
				// wait till next retry
				return true, nil
			} else if oc.unavailableBalanceHandlingStrategy == OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyStop {
				oc.Stop()
				return true, nil
			} else {
				// just continue without any balance check
				skipBalanceCheck = true
			}
		}

	}

	previousNonceCostUnknown := false
	for i, it := range its {
		log.L(ctx).Debugf("%s ProcessInFlightTransaction for signing address %s processing transaction with ID: %s, index: %d", now.String(), oc.signingAddress, it.stateManager.GetSignerNonce(), i)
		var availableToSpend *big.Int
		if !skipBalanceCheck {
			availableToSpend = addressAccount.GetAvailableToSpend(ctx)
		}
		triggerNextStageOutput := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
			AvailableToSpend:         availableToSpend,
			PreviousNonceCostUnknown: previousNonceCostUnknown,
		})
		if !skipBalanceCheck {
			if triggerNextStageOutput.Cost != nil {
				_ = addressAccount.Spend(ctx, triggerNextStageOutput.Cost)
				if i == 0 && addressAccount.GetAvailableToSpend(ctx).Sign() == -1 {
					waitingForBalance = true
				}
			} else {
				// current transaction doesn't know the cost yet, it risky to let any other transactions to do submission
				// but we should let them complete all the stages before submission
				previousNonceCostUnknown = true
			}
			// only modify spent when the cost is available for the current transaction
		}
	}

	log.L(ctx).Debugf("%s ProcessInFlightTransaction exit for signing address: %s", now.String(), oc.signingAddress)
	log.L(ctx).Debugf("Orchestrator process loop took %s", time.Since(processStart))
	return waitingForBalance, nil
}

func (oc *orchestrator) Start(ctx context.Context) (done <-chan struct{}, err error) {
	oc.orchestratorLoopDone = make(chan struct{})
	go oc.orchestratorLoop()
	oc.MarkInFlightTxStale()
	return oc.orchestratorLoopDone, nil
}

// Stop the InFlight transaction process.
func (oc *orchestrator) Stop() {
	// try to send an item in `stopProcess` channel, which has a buffer of 1
	// if it already has an item in the channel, this function does nothing
	select {
	case oc.stopProcess <- true:
	default:
	}
}

func (oc *orchestrator) MarkInFlightTxStale() {
	// try to send an item in `processNow` channel, which has a buffer of 1
	// if it already has an item in the channel, this function does nothing
	select {
	case oc.InFlightTxsStale <- true:
	default:
	}
}
