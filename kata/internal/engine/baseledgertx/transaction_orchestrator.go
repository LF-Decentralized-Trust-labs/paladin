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

package baseledgertx

import (
	"context"
	"database/sql/driver"
	"math/big"
	"sync"
	"time"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/dbsql"
	"github.com/hyperledger/firefly-common/pkg/ffapi"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/retry"
	baseTypes "github.com/kaleido-io/paladin/kata/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

// configurations
const (
	OrchestratorSection = "orchestrator"

	OrchestratorMaxInFlightTransactionsInt = "maxInFlight"
	OrchestratorTurnOffHistory             = "turnOffHistory"
	OrchestratorIntervalDurationString     = "interval"

	// after how long if the queue hasn't change, we mark the transaction orchestrator stale
	OrchestratorStaleTimeoutDurationString = "staleTimeout"

	OrchestratorResubmitIntervalDurationString = "resubmitInterval" // warnings will be written to the log at this interval if mining has not occurred, and the TX will be resubmitted

	OrchestratorStageRetryDurationString       = "stageRetry"
	OrchestratorPersistenceRetryDurationString = "persistenceRetry"

	OrchestratorSubmissionRetryInitDelayDurationString = "submissionRetry.initialDelay"
	OrchestratorSubmissionRetryMaxDelayDurationString  = "submissionRetry.maxDelay"
	OrchestratorSubmissionRetryFactorFloat             = "submissionRetry.factor"
	OrchestratorSubmissionRetryCountInt                = "submissionRetry.count"

	// balance check configuration
	OrchestratorGasPriceIncreaseMaxBigIntString = "gasPriceIncreaseMax"
	OrchestratorGasPriceIncreasePercentageInt   = "gasPriceIncreasePercentage"
	// what happens to the transaction orchestrator processing loop if the balance of the current account cannot be retrieved
	OrchestratorUnavailableBalanceHandlerString = "unavailableBalanceHandler"
)

const (
	defaultOrchestratorTurnOffHistory             = false
	defaultOrchestratorMaxInFlight                = 500
	defaultOrchestratorGasPriceIncreaseMax        = "" // empty
	defaultOrchestratorGasPriceIncreasePercentage = 0
	defaultOrchestratorInterval                   = "5s"
	defaultOrchestratorResubmitInterval           = "5m"
	defaultOrchestratorStaleTimeout               = "5m"
	defaultOrchestratorStageRetry                 = "10s"
	defaultOrchestratorPersistenceRetry           = "5s"

	defaultOrchestratorSubmissionRetryInitDelay = "250ms"
	defaultOrchestratorSubmissionRetryMaxDelay  = "10s"
	defaultOrchestratorSubmissionRetryFactor    = 4.0
	defaultOrchestratorSubmissionRetryCountInt  = 3
)

func InitOrchestratorConfig(conf config.Section) {
	orchestratorConfig := conf.SubSection(OrchestratorSection)

	orchestratorConfig.AddKnownKey(OrchestratorTurnOffHistory, defaultOrchestratorTurnOffHistory)
	orchestratorConfig.AddKnownKey(OrchestratorIntervalDurationString, defaultOrchestratorInterval)
	orchestratorConfig.AddKnownKey(OrchestratorMaxInFlightTransactionsInt, defaultOrchestratorMaxInFlight)
	orchestratorConfig.AddKnownKey(OrchestratorStaleTimeoutDurationString, defaultOrchestratorStaleTimeout)

	// Transaction Processing configs
	orchestratorConfig.AddKnownKey(OrchestratorResubmitIntervalDurationString, defaultOrchestratorResubmitInterval)
	orchestratorConfig.AddKnownKey(OrchestratorStageRetryDurationString, defaultOrchestratorStageRetry)
	orchestratorConfig.AddKnownKey(OrchestratorPersistenceRetryDurationString, defaultOrchestratorPersistenceRetry)
	orchestratorConfig.AddKnownKey(OrchestratorGasPriceIncreaseMaxBigIntString, defaultOrchestratorGasPriceIncreaseMax)
	orchestratorConfig.AddKnownKey(OrchestratorGasPriceIncreasePercentageInt, defaultOrchestratorGasPriceIncreasePercentage)
	orchestratorConfig.AddKnownKey(OrchestratorUnavailableBalanceHandlerString, string(OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyWait))

	orchestratorConfig.AddKnownKey(OrchestratorSubmissionRetryInitDelayDurationString, defaultOrchestratorSubmissionRetryInitDelay)
	orchestratorConfig.AddKnownKey(OrchestratorSubmissionRetryMaxDelayDurationString, defaultOrchestratorSubmissionRetryMaxDelay)
	orchestratorConfig.AddKnownKey(OrchestratorSubmissionRetryFactorFloat, defaultOrchestratorSubmissionRetryFactor)
	orchestratorConfig.AddKnownKey(OrchestratorSubmissionRetryCountInt, defaultOrchestratorSubmissionRetryCountInt)
}

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
	// transaction orchestrator is blocked and waiting for precondition to be fulfilled, e.g. waiting for fueling
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
//    - auto-fueling
//     - retrieve the balance of the signing account
//     - tally up total funds required by the in-flight transactions
//     - ask its transaction engine to create auto-fueling transactions
//    - action none event driven transaction stage
//      - stale transaction check and handling
//      - action signing request and retries
//      - action gas price calculation and retries
//      - action gas limit estimation and retries
//      - action transaction submission and retries
//    - record self-deletion request (based on settings) for its transaction engine to action
//      - the same stale transaction ID has been staying at the front of the queue for a period of time (based on settings)
//      - the auto-fueling request is stale
//    - decide when to stop iterating the queue
//      - when ran out of fund
//      - when self-deletion request has been raised after processing the previous transaction
// 3. persist updates of the managed transactions
// 4. Nonce management
//    - need more thinking on nonce too low and we lost the information for tracking the submitted transactions

type orchestrator struct {
	*baseLedgerTxEngine

	// in-flight transaction config
	resubmitInterval        time.Duration
	stageRetryTimeout       time.Duration
	persistenceRetryTimeout time.Duration
	txStore                 baseTypes.TransactionStore
	ethClient               ethclient.EthClient
	managedTXEventNotifier  baseTypes.ManagedTxEventNotifier
	txConfirmationListener  baseTypes.TransactionConfirmationListener
	turnOffHistory          bool

	transactionSubmissionRetry      *retry.Retry
	transactionSubmissionRetryCount int

	// each transaction orchestrator has its own go routine
	orchestratorBirthTime       time.Time     // when transaction orchestrator is created
	orchestratorPollingInterval time.Duration // between how long the transaction orchestrator will do a poll and trigger none-event driven transaction process actions
	signingAddress              string        // the signing address of the transaction managed by the current transaction orchestrator

	// balance check settings
	hasZeroGasPrice                    bool
	unavailableBalanceHandlingStrategy OrchestratorBalanceCheckUnavailableBalanceHandlingStrategy

	// in flight txs array
	maxInFlightTxs               int
	InFlightTxs                  []*InFlightTransactionStageController // a queue of all the in flight transactions
	processedTxIDs               map[string]bool                       // an internal record of completed transactions to handle persistence delays that causes reprocessing
	InFlightTxsMux               sync.Mutex
	orchestratorLoopDone         chan struct{}
	InFlightTxsStale             chan bool
	transactionIDsInStatusUpdate []string // a list of transaction IDs of which status are being updated

	// input channels
	stopProcess chan bool // a channel to tell the current transaction orchestrator to stop processing all events and mark itself as to be deleted

	// Metrics provided for fairness control in the controler
	totalCompleted int64 // total number of transaction completed since birth time
	state          OrchestratorState
	stateEntryTime time.Time // when it's run last time

	staleTimeout    time.Duration
	lastQueueUpdate time.Time
}

func NewOrchestrator(
	ble *baseLedgerTxEngine,
	signingAddress string,
	conf config.Section,
) *orchestrator {
	ctx := ble.ctx

	newOrchestrator := &orchestrator{
		baseLedgerTxEngine:                 ble,
		orchestratorBirthTime:              time.Now(),
		orchestratorPollingInterval:        conf.GetDuration(OrchestratorIntervalDurationString),
		maxInFlightTxs:                     conf.GetInt(OrchestratorMaxInFlightTransactionsInt),
		turnOffHistory:                     conf.GetBool(OrchestratorTurnOffHistory),
		signingAddress:                     signingAddress,
		state:                              OrchestratorStateNew,
		stateEntryTime:                     time.Now(),
		unavailableBalanceHandlingStrategy: OrchestratorBalanceCheckUnavailableBalanceHandlingStrategy(conf.GetString(OrchestratorUnavailableBalanceHandlerString)),

		// in-flight transaction configs
		resubmitInterval:        conf.GetDuration(OrchestratorResubmitIntervalDurationString),
		stageRetryTimeout:       conf.GetDuration(OrchestratorStageRetryDurationString),
		persistenceRetryTimeout: conf.GetDuration(OrchestratorPersistenceRetryDurationString),

		// submission retry
		transactionSubmissionRetry: &retry.Retry{
			InitialDelay: conf.GetDuration(OrchestratorSubmissionRetryInitDelayDurationString),
			MaximumDelay: conf.GetDuration(OrchestratorSubmissionRetryMaxDelayDurationString),
			Factor:       conf.GetFloat64(OrchestratorSubmissionRetryFactorFloat),
		},
		transactionSubmissionRetryCount: conf.GetInt(OrchestratorSubmissionRetryCountInt),
		staleTimeout:                    conf.GetDuration(OrchestratorStaleTimeoutDurationString),
		hasZeroGasPrice:                 ble.gasPriceClient.HasZeroGasPrice(ctx),
		processedTxIDs:                  make(map[string]bool),
		transactionIDsInStatusUpdate:    make([]string, 0),
		InFlightTxsStale:                make(chan bool, 1),
		stopProcess:                     make(chan bool, 1),
		txStore:                         ble.txStore,
		ethClient:                       ble.ethClient,
		managedTXEventNotifier:          ble.managedTXEventNotifier,
		txConfirmationListener:          ble.txConfirmationListener,
	}

	log.L(ctx).Debugf("NewOrchestrator for signing address %s created: %+v", newOrchestrator.signingAddress, newOrchestrator)

	return newOrchestrator
}

func (te *orchestrator) orchestratorLoop() {
	ctx := log.WithLogField(te.ctx, "role", "orchestrator-loop")
	log.L(ctx).Infof("Orchestrator for signing address %s started pooling based on interval %s", te.signingAddress, te.orchestratorPollingInterval)

	defer close(te.orchestratorLoopDone)

	ticker := time.NewTicker(te.orchestratorPollingInterval)
	for {
		// an InFlight
		select {
		case <-te.InFlightTxsStale:
		case <-ticker.C:
		case <-ctx.Done():
			log.L(ctx).Infof("Orchestrator loop exit due to canceled context, it processed %d transaction during its lifetime.", te.totalCompleted)
			return
		case <-te.stopProcess:
			log.L(ctx).Infof("Orchestrator loop process stopped, it processed %d transaction during its lifetime.", te.totalCompleted)
			te.state = OrchestratorStateStopped
			te.stateEntryTime = time.Now()
			te.MarkInFlightOrchestratorsStale() // trigger engine loop for removal
			return
		}
		polled, total := te.pollAndProcess(ctx)
		log.L(ctx).Debugf("Orchestrator loop polled %d txs, there are %d txs in total", polled, total)
	}

}

func (te *orchestrator) pollAndProcess(ctx context.Context) (polled int, total int) {
	pollStart := time.Now()
	te.InFlightTxsMux.Lock()
	defer te.InFlightTxsMux.Unlock()
	queueUpdated := false

	oldInFlight := te.InFlightTxs
	te.InFlightTxs = make([]*InFlightTransactionStageController, 0, len(oldInFlight))

	stageCounts := make(map[string]int)
	for _, stageName := range baseTypes.AllInFlightStages {
		// map for saving number of in flight transaction per stage
		stageCounts[stageName] = 0
	}

	var latestCompleted *baseTypes.ManagedTX
	startFromNonce, hasCompletedNonce := te.completedTxNoncePerAddress[te.signingAddress]
	// Run through copying across from the old InFlight list to the new one, those that aren't ready to be deleted
	for _, p := range oldInFlight {
		if !hasCompletedNonce || p.stateManager.GetNonce().Uint64()-startFromNonce.Uint64() <= 1 {
			startFromNonce = *p.stateManager.GetNonce()
			hasCompletedNonce = true
		}
		te.processedTxIDs[p.stateManager.GetTxID()] = true
		if p.stateManager.CanBeRemoved(ctx) {
			te.totalCompleted = te.totalCompleted + 1
			latestCompleted = p.stateManager.GetTx()
			queueUpdated = true
			log.L(ctx).Debugf("Orchestrator poll and process, marking %s as complete after: %s", p.stateManager.GetTxID(), time.Since(*p.stateManager.GetCreatedTime().Time()))
		} else if p.stateManager.IsSuspended() {
			log.L(ctx).Debugf("Orchestrator poll and process, removed suspended tx %s after: %s", p.stateManager.GetTxID(), time.Since(*p.stateManager.GetCreatedTime().Time()))
		} else {
			log.L(ctx).Debugf("Orchestrator poll and process, continuing tx %s after: %s", p.stateManager.GetTxID(), time.Since(*p.stateManager.GetCreatedTime().Time()))
			te.InFlightTxs = append(te.InFlightTxs, p)
			txStage := p.stateManager.GetStage(ctx)
			if string(txStage) == "" {
				txStage = baseTypes.InFlightTxStageQueued
			}
			stageCounts[string(txStage)] = stageCounts[string(txStage)] + 1
		}
	}

	log.L(ctx).Debugf("Orchestrator poll and process, stage counts: %+v", stageCounts)

	if latestCompleted != nil {
		te.updateCompletedTxNonce(latestCompleted)
	}
	oldLen := len(te.InFlightTxs)
	total = oldLen
	// check and poll new transactions from the persistence if we can handle more
	// If we are not at maximum, then query if there are more candidates now
	spaces := te.maxInFlightTxs - oldLen
	if spaces > 0 {
		completedTxIDsStillBeingPersisted := make(map[string]bool)
		fb := te.txStore.NewTransactionFilter(ctx)
		conds := []ffapi.Filter{
			fb.Eq("from", te.signingAddress),
			fb.Eq("status", baseTypes.BaseTxStatusPending),
		}

		if len(te.transactionIDsInStatusUpdate) > 0 {
			transactionIDInStatusUpdate := make([]driver.Value, 0, len(te.transactionIDsInStatusUpdate))
			for _, txID := range te.transactionIDsInStatusUpdate {
				transactionIDInStatusUpdate = append(transactionIDInStatusUpdate, txID)

			}
			conds = append(conds, fb.NotIn(dbsql.ColumnID, transactionIDInStatusUpdate))
		}
		var after string
		if len(te.InFlightTxs) > 0 {
			conds = append(conds, fb.Gt("nonce", startFromNonce.String()))
		}

		var additional []*baseTypes.ManagedTX
		// We retry the get from persistence indefinitely (until the context cancels)
		err := te.retry.Do(ctx, "get pending transactions", func(attempt int) (retry bool, err error) {
			filter := fb.And(conds...)
			_ = filter.Limit(uint64(spaces)).Sort("nonce")

			additional, _, err = te.txStore.ListTransactions(ctx, filter)
			return true, err
		})
		if err != nil {
			log.L(ctx).Infof("Orchestrator poll and process: context cancelled while retrying")
			return -1, len(te.InFlightTxs)
		}

		log.L(ctx).Debugf("Orchestrator poll and process: polled %d items, space: %d", len(additional), spaces)
		for _, mtx := range additional {
			if te.processedTxIDs[mtx.ID] {
				// already processed, still being persisted
				completedTxIDsStillBeingPersisted[mtx.ID] = true
				log.L(ctx).Debugf("Orchestrator polled transaction with ID: %s but it's already being processed before, ignoring it", mtx.ID)
			} else if mtx.Status == baseTypes.BaseTxStatusPending {
				queueUpdated = true
				it := NewInFlightTransactionStageController(te.baseLedgerTxEngine, te, mtx)
				te.InFlightTxs = append(te.InFlightTxs, it)
				txStage := it.stateManager.GetStage(ctx)
				if string(txStage) == "" {
					txStage = baseTypes.InFlightTxStageQueued
				}
				stageCounts[string(txStage)] = stageCounts[string(txStage)] + 1
				log.L(ctx).Debugf("Orchestrator added transaction with ID: %s", mtx.ID)
			}
		}
		te.processedTxIDs = completedTxIDsStillBeingPersisted
		total = len(te.InFlightTxs)
		polled = total - oldLen
		if polled > 0 {
			log.L(ctx).Debugf("InFlight set updated len=%d head-seq=%s tail-seq=%s old-tail=%s", len(te.InFlightTxs), te.InFlightTxs[0].stateManager.GetTx().SequenceID, te.InFlightTxs[total-1].stateManager.GetTx().SequenceID, after)
		}
		te.thMetrics.RecordInFlightTxQueueMetrics(ctx, stageCounts, te.maxInFlightTxs-len(te.InFlightTxs))
	}
	log.L(ctx).Debugf("Orchestrator polling from DB took %s", time.Since(pollStart))
	// now check and process each transaction

	if total > 0 {
		waitingForBalance, _ := te.ProcessInFlightTransaction(ctx, te.InFlightTxs)
		if queueUpdated {
			te.lastQueueUpdate = time.Now()
		}
		if time.Since(te.lastQueueUpdate) > te.staleTimeout && te.state != OrchestratorStateStale {
			te.state = OrchestratorStateStale
			te.stateEntryTime = time.Now()
		} else if waitingForBalance && te.state != OrchestratorStateWaiting {
			te.state = OrchestratorStateWaiting
			te.stateEntryTime = time.Now()
		} else if te.state != OrchestratorStateRunning {
			te.state = OrchestratorStateRunning
			te.stateEntryTime = time.Now()
		}
	} else if te.state != OrchestratorStateIdle {
		te.state = OrchestratorStateIdle
		te.stateEntryTime = time.Now()
	}
	log.L(ctx).Debugf("Orchestrator process loop took %s", time.Since(pollStart))

	return polled, total
}

// this function should only have one running instance at any given time
func (te *orchestrator) ProcessInFlightTransaction(ctx context.Context, its []*InFlightTransactionStageController) (waitingForBalance bool, err error) {
	processStart := time.Now()
	waitingForBalance = false
	var addressAccount *baseTypes.AddressAccount
	skipBalanceCheck := te.hasZeroGasPrice
	now := time.Now()
	log.L(ctx).Debugf("%s ProcessInFlightTransaction entry for signing address %s", now.String(), te.signingAddress)

	if !skipBalanceCheck {
		log.L(ctx).Debugf("%s: ProcessInFlightTransaction checking balance for %s", now.String(), te.signingAddress)

		addressAccount, err = te.balanceManager.GetAddressBalance(te.ctx, te.signingAddress)
		if err != nil {
			log.L(ctx).Errorf("Failed to retrieve balance for address %s due to %+v", te.signingAddress, err)
			if te.unavailableBalanceHandlingStrategy == OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyWait {
				// wait till next retry
				return true, nil
			} else if te.unavailableBalanceHandlingStrategy == OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyStop {
				te.Stop()
				return true, nil
			} else {
				// just continue without any balance check
				skipBalanceCheck = true
			}
		}

	}

	previousNonceCostUnknown := false
	for i, it := range its {
		log.L(ctx).Debugf("%s ProcessInFlightTransaction for signing address %s processing transaction with ID: %s, index: %d", now.String(), te.signingAddress, it.stateManager.GetTxID(), i)
		var availableToSpend *big.Int
		if !skipBalanceCheck {
			availableToSpend = addressAccount.GetAvailableToSpend(ctx)
		}
		triggerNextStageOutput := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
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

	if !skipBalanceCheck && addressAccount.GetAvailableToSpend(ctx).Sign() == -1 && te.balanceManager.IsAutoFuelingEnabled(ctx) {
		log.L(ctx).Debugf("%s Address %s requires top up, credit after estimated cost: %s", now.String(), te.signingAddress, addressAccount.GetAvailableToSpend(ctx).String())
		_, _ = te.balanceManager.TopUpAccount(ctx, addressAccount)
	}

	log.L(ctx).Debugf("%s ProcessInFlightTransaction exit for signing address: %s", now.String(), te.signingAddress)
	log.L(ctx).Debugf("Orchestrator process loop took %s", time.Since(processStart))
	return waitingForBalance, nil
}

func (te *orchestrator) Start(c context.Context) (done <-chan struct{}, err error) {
	te.orchestratorLoopDone = make(chan struct{})
	go te.orchestratorLoop()
	te.MarkInFlightTxStale()
	return te.orchestratorLoopDone, nil
}

// Stop the InFlight transaction process.
func (te *orchestrator) Stop() {
	// try to send an item in `stopProcess` channel, which has a buffer of 1
	// if it already has an item in the channel, this function does nothing
	select {
	case te.stopProcess <- true:
	default:
	}
}

func (te *orchestrator) MarkInFlightTxStale() {
	// try to send an item in `processNow` channel, which has a buffer of 1
	// if it already has an item in the channel, this function does nothing
	select {
	case te.InFlightTxsStale <- true:
	default:
	}
}

func (te *orchestrator) CreateTransactionConfirmationsHandler(txID string) func(ctx context.Context, txID string, notification *baseTypes.ConfirmationsNotification) (err error) {
	return func(ctx context.Context, txID string, notification *baseTypes.ConfirmationsNotification) (err error) {
		recordStart := time.Now()
		te.InFlightTxsMux.Lock()
		defer te.InFlightTxsMux.Unlock()
		var pending *InFlightTransactionStageController
		for _, p := range te.InFlightTxs {
			if p != nil && p.stateManager.GetTxID() == txID {
				pending = p
				break
			}
		}
		if pending == nil {
			err = i18n.NewError(ctx, msgs.MsgTransactionNotFound, txID)
			return
		}
		pending.MarkHistoricalTime("confirm_event_wait_to_be_recorded", recordStart)
		pending.MarkTime("confirm_event_wait_to_be_processed")
		pending.stateManager.AddConfirmationsOutput(ctx, notification)
		pending.MarkInFlightTxStale()
		return
	}
}
func (te *orchestrator) CreateTransactionReceiptReceivedHandler(txID string) func(ctx context.Context, txID string, receipt *ethclient.TransactionReceiptResponse) (err error) {
	return func(ctx context.Context, txID string, receipt *ethclient.TransactionReceiptResponse) (err error) {
		recordStart := time.Now()
		te.InFlightTxsMux.Lock()
		defer te.InFlightTxsMux.Unlock()
		var pending *InFlightTransactionStageController
		for _, p := range te.InFlightTxs {
			if p != nil && p.stateManager.GetTxID() == txID {
				pending = p
				break
			}
		}
		if pending == nil {
			err = i18n.NewError(ctx, msgs.MsgTransactionNotFound, txID)
			return
		}
		pending.MarkHistoricalTime("receipt_event_wait_to_be_recorded", recordStart)
		pending.MarkTime("receipt_event_wait_to_be_processed")
		// Will be picked up on the next orchestrator loop - guaranteed to occur before Confirmed
		pending.stateManager.AddReceiptOutput(ctx, receipt, nil)
		pending.MarkInFlightTxStale()
		return
	}
}
