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
	"time"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/ffapi"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	baseTypes "github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

// configurations
const (
	TransactionControllerSection = "controller"

	TransactionControllerMaxInFlightEngineInt   = "maxInFlightEngines"
	TransactionControllerIntervalDurationString = "interval"

	// after how long if the transaction engine stayed in stale state, we stop the transaction engine
	TransactionControllerMaxStaleDurationString = "maxStale"
	// after how long if the transaction engine stayed in idle state, we stop the transaction engine
	TransactionControllerMaxIdleDurationString = "maxIdle"

	// when the in-flight engine pool is full, after how long a transaction engine will be stopped since it started to ensure fairness
	TransactionControllerMaxOverloadProcessTimeDurationString = "maxOverloadProcessTime"

	TransactionControllerRetryInitDelayDurationString = "retry.initialDelay"
	TransactionControllerRetryMaxDelayDurationString  = "retry.maxDelay"
	TransactionControllerRetryFactorFloat             = "retry.factor"
)

const (
	defaultTransactionControllerMaxInFlightEngine = 50
	defaultTransactionControllerInterval          = "5s"
	defaultTransactionControllerMaxStale          = "1m"
	defaultTransactionControllerMaxIdle           = "10s"
	defaultMaxOverloadProcessTime                 = "10m"
	defaultTransactionControllerRetryInitDelay    = "250ms"
	defaultTransactionControllerRetryMaxDelay     = "30s"
	defaultTransactionControllerRetryFactor       = 2.0
)

func InitTransactionControllerConfig(conf config.Section) {

	controllerConfig := conf.SubSection(TransactionControllerSection)

	controllerConfig.AddKnownKey(TransactionControllerMaxInFlightEngineInt, defaultTransactionControllerMaxInFlightEngine)
	controllerConfig.AddKnownKey(TransactionControllerIntervalDurationString, defaultTransactionControllerInterval)
	controllerConfig.AddKnownKey(TransactionControllerMaxStaleDurationString, defaultTransactionControllerMaxStale)
	controllerConfig.AddKnownKey(TransactionControllerMaxIdleDurationString, defaultTransactionControllerMaxIdle)
	controllerConfig.AddKnownKey(TransactionControllerMaxOverloadProcessTimeDurationString, defaultMaxOverloadProcessTime)

	controllerConfig.AddKnownKey(TransactionControllerRetryInitDelayDurationString, defaultTransactionControllerRetryInitDelay)
	controllerConfig.AddKnownKey(TransactionControllerRetryMaxDelayDurationString, defaultTransactionControllerRetryMaxDelay)
	controllerConfig.AddKnownKey(TransactionControllerRetryFactorFloat, defaultTransactionControllerRetryFactor)
}

// metrics
const metricsLabelInFlightEngineState = "eng_state"

const metricsGaugeEnginesInFlightUsedWithState = "eng_in_flight_used_total"
const metricsGaugeEnginesInFlightUsedWithStateDescription = "Number of transaction engines currently in flight grouped by state"

const metricsGaugeEnginesInFlightFree = "eng_in_flight_free_total"
const metricsGaugeEnginesInFlightFreeDescription = "Number of space left in the in flight engines pool"

const metricsHistogramTransactionStageDurationNSWithStageName = "tx_process_stages_seconds"
const metricsHistogramTransactionStageDurationNSWithStageNameDescription = "Duration of in-flight transaction process stayed in a stage grouped by stage"

// role of transaction controller:
// 1. owner and the only manipulator of the transaction engines pool
//    - decides how many transaction engines can there be in total at a given time
//    - decides the size of each transaction engine queue
//    - decides signing address of each transaction engine
//    - controls the life cycle for transaction engines based on its latest status
//       - time box transaction engine lifespan
//       - creates transaction engine when necessary
//       - deletes transaction engine when there is no pending transaction for a signing address
//       - pauses (delete) transaction engine when it's in a stuck state
//       - resumes (re-create) transaction engine in a resuming mode (1 transaction to test the water before add more) based on user configuration
// 2. handle external requests of transaction status change
//       - update/delete of a transaction when its signing address not in-flight
//       - signal update/delete of a transaction in-flight to corresponding transaction engine
// 3. auto fueling management - the autofueling transactions
//    - creating auto-fueling transactions when asked by transaction engines
// 4. provides shared functionalities for optimization
//    - handles gas price information which is not signer specific

func (enterpriseHandler *enterpriseTransactionHandler) controllerLoop() {
	defer close(enterpriseHandler.controllerLoopDone)
	ctx := log.WithLogField(enterpriseHandler.ctx, "role", "controller-loop")
	log.L(ctx).Infof("Controller started polling on interval %s", enterpriseHandler.controllerPollingInterval)

	ticker := time.NewTicker(enterpriseHandler.controllerPollingInterval)
	// enterpriseHandler.MarkInFlightEnginesStale()
	for {
		// Wait to be notified, or timeout to run
		select {
		case <-ticker.C:
		case <-enterpriseHandler.InFlightEngineStale:
		case <-ctx.Done():
			ticker.Stop()
			log.L(ctx).Infof("Controller poller exiting")
			return
		}

		polled, total := enterpriseHandler.poll(ctx)
		log.L(ctx).Debugf("Controller polling complete: %d transaction engines were created, there are %d transaction engines in flight", polled, total)
	}
}

func (enterpriseHandler *enterpriseTransactionHandler) poll(ctx context.Context) (polled int, total int) {
	pollStart := time.Now()
	enterpriseHandler.InFlightEngineMux.Lock()
	defer enterpriseHandler.InFlightEngineMux.Unlock()

	oldInFlight := enterpriseHandler.InFlightEngines
	enterpriseHandler.InFlightEngines = make(map[string]*transactionEngine)

	InFlightSigningAddresses := make([]driver.Value, 0, len(oldInFlight))

	stateCounts := make(map[string]int)
	for _, sName := range AllTransactionEngineStates {
		// map for saving number of in flight transaction per stage
		stateCounts[sName] = 0
	}

	// Run through copying across from the old InFlight list to the new one, those that aren't ready to be deleted
	for signingAddress, te := range oldInFlight {
		log.L(ctx).Debugf("Controller checking engine for %s: state: %s, state duration: %s, number of transactions: %d", te.signingAddress, te.state, time.Since(te.stateEntryTime), len(te.InFlightTxs))
		if (te.state == TransactionEngineStateStale && time.Since(te.stateEntryTime) > enterpriseHandler.maxEngineStale) ||
			(te.state == TransactionEngineStateIdle && time.Since(te.stateEntryTime) > enterpriseHandler.maxEngineIdle) {
			// tell transaction engine to stop, there is a chance we later found new transaction for this address, but we got to make a call at some point
			// so it's here. The transaction engine won't be removed immediately as the state update is async
			te.Stop()
		}
		if te.state != TransactionEngineStateStopped {
			enterpriseHandler.InFlightEngines[signingAddress] = te
			te.MarkInFlightTxStale()
			stateCounts[string(te.state)] = stateCounts[string(te.state)] + 1
			InFlightSigningAddresses = append(InFlightSigningAddresses, signingAddress)
		} else {
			log.L(ctx).Infof("Controller removed engine for signing address %s", signingAddress)
		}
	}

	totalBeforePoll := len(enterpriseHandler.InFlightEngines)
	// check and poll new signers from the persistence if there are more transaction engines slots
	spaces := enterpriseHandler.maxInFlightEngines - totalBeforePoll
	if spaces > 0 {

		// Run through the paused engines for fairness control
		for signingAddress, pausedUntil := range enterpriseHandler.SigningAddressesPausedUntil {
			if time.Now().Before(pausedUntil) {
				log.L(ctx).Debugf("Controller excluded engine for signing address %s from polling as it's paused util %s", signingAddress, pausedUntil.String())
				stateCounts[string(TransactionEngineStatePaused)] = stateCounts[string(TransactionEngineStatePaused)] + 1
				InFlightSigningAddresses = append(InFlightSigningAddresses, signingAddress)
			}
		}

		var additionalTxFromNonInFlightSigners []*baseTypes.ManagedTX
		// We retry the get from persistence indefinitely (until the context cancels)
		err := enterpriseHandler.retry.Do(ctx, "get pending transactions with non InFlight signing addresses", func(attempt int) (retry bool, err error) {
			fb := enterpriseHandler.txStore.NewTransactionFilter(ctx)
			conditions := []ffapi.Filter{
				fb.Eq("status", baseTypes.BaseTxStatusPending),
			}
			if len(InFlightSigningAddresses) > 0 {
				conditions = append(conditions, fb.NotIn("from", InFlightSigningAddresses))
			}
			filter := fb.And(conditions...)
			_ = filter.Limit(uint64(spaces)).Sort("sequence") // TODO: use group by to be more efficient

			additionalTxFromNonInFlightSigners, _, err = enterpriseHandler.txStore.ListTransactions(ctx, filter)
			return true, err
		})
		if err != nil {
			log.L(ctx).Infof("Controller polling context cancelled while retrying")
			return -1, len(enterpriseHandler.InFlightEngines)
		}

		log.L(ctx).Debugf("Controller polled %d items to fill in %d empty slots.", len(additionalTxFromNonInFlightSigners), spaces)

		for _, mtx := range additionalTxFromNonInFlightSigners {
			if _, exist := enterpriseHandler.InFlightEngines[string(mtx.From)]; !exist {
				te := NewTransactionEngine(enterpriseHandler, mtx, enterpriseHandler.engineConfig)
				enterpriseHandler.InFlightEngines[string(mtx.From)] = te
				stateCounts[string(te.state)] = stateCounts[string(te.state)] + 1
				_, _ = te.Start(enterpriseHandler.ctx)
				log.L(ctx).Infof("Controller added engine for signing address %s", mtx.From)
			} else {
				log.L(ctx).Warnf("Controller fetched extra transactions from signing address %s", mtx.From)
			}
		}
		total = len(enterpriseHandler.InFlightEngines)
		if total > 0 {
			polled = total - totalBeforePoll
		}
	} else {
		// the in-flight engine pool is full, do the fairness control

		// TODO: don't stop more than required number of slots

		// Run through the existing running engines and stop the ones that exceeded the max process timeout
		for signingAddress, te := range enterpriseHandler.InFlightEngines {
			if time.Since(te.engineBirthTime) > enterpriseHandler.maxOverloadProcessTime {
				log.L(ctx).Infof("Controller pause, attempt to stop engine for signing address %s", signingAddress)
				te.Stop()
				enterpriseHandler.SigningAddressesPausedUntil[signingAddress] = time.Now().Add(enterpriseHandler.maxOverloadProcessTime)
			}
		}
	}
	enterpriseHandler.thMetrics.RecordInFlightEnginePoolMetrics(ctx, stateCounts, enterpriseHandler.maxInFlightEngines-len(enterpriseHandler.InFlightEngines))
	log.L(ctx).Debugf("Controller poll loop took %s", time.Since(pollStart))
	return polled, total
}

func (enterpriseHandler *enterpriseTransactionHandler) MarkInFlightEnginesStale() {
	// try to send an item in `InFlightStale` channel, which has a buffer of 1
	// to trigger a polling event to update the in flight transaction engines
	// if it already has an item in the channel, this function does nothing
	select {
	case enterpriseHandler.InFlightEngineStale <- true:
	default:
	}
}

func (enterpriseHandler *enterpriseTransactionHandler) GetPendingFuelingTransaction(ctx context.Context, sourceAddress string, destinationAddress string) (tx *baseTypes.ManagedTX, err error) {
	fb := enterpriseHandler.txStore.NewTransactionFilter(ctx)
	filter := fb.And(fb.Eq("from", sourceAddress),
		fb.Eq("to", destinationAddress),
		fb.Eq("status", baseTypes.BaseTxStatusPending),
		fb.Neq("value", nil), // NB: we assume if a transaction has value then it's a fueling transaction
	)
	_ = filter.Limit(1).Sort("-nonce")

	txs, _, err := enterpriseHandler.txStore.ListTransactions(ctx, filter)
	if err != nil {
		return nil, err
	}
	if len(txs) > 0 {
		tx = txs[0]
	}
	return tx, nil
}

func (enterpriseHandler *enterpriseTransactionHandler) CheckTransactionCompleted(ctx context.Context, tx *baseTypes.ManagedTX) (completed bool) {
	// no need for locking here as outdated information is OK given we do frequent retires
	log.L(ctx).Debugf("CheckTransactionCompleted checking state for transaction %s.", tx.ID)
	completedTxNonce, exists := enterpriseHandler.completedTxNoncePerAddress[string(tx.From)]
	if !exists {
		// need to query the database to check the status of managed transaction
		fb := enterpriseHandler.txStore.NewTransactionFilter(ctx)
		filter := fb.And(fb.Eq("from", tx.From), fb.Or(fb.Eq("status", baseTypes.BaseTxStatusSucceeded), fb.Eq("status", baseTypes.BaseTxStatusFailed)))
		_ = filter.Limit(1).Sort("-nonce")

		txs, _, err := enterpriseHandler.txStore.ListTransactions(ctx, filter)
		if err != nil {
			// can not read from the database, treat transaction as incomplete
			return false
		}
		if len(txs) > 0 {
			enterpriseHandler.updateCompletedTxNonce(ctx, txs[0])
			completedTxNonce = *txs[0].Nonce.BigInt()
			// found completed fueling transaction, do the comparison
			completed = completedTxNonce.Cmp(tx.Nonce.BigInt()) >= 0
		}
		// if no completed fueling transaction is found, the value of "completed" stays false (still pending)
	} else {
		// in memory tracked highest nonce available, do the comparison
		completed = completedTxNonce.Cmp(tx.Nonce.BigInt()) >= 0
	}
	log.L(ctx).Debugf("CheckTransactionCompleted checking against completed nonce of %s, from: %s, to: %s, value: %s. Completed nonce: %d, current tx nonce: %d.", tx.ID, tx.From, tx.To, tx.Value.String(), completedTxNonce.Uint64(), tx.Nonce.Uint64())

	return completed

}

func (enterpriseHandler *enterpriseTransactionHandler) updateCompletedTxNonce(ctx context.Context, tx *baseTypes.ManagedTX) (updated bool) {
	updated = false
	// no need for locking here as outdated information is OK given we do frequent retires
	enterpriseHandler.completedTxNoncePerAddressMutex.Lock()
	defer enterpriseHandler.completedTxNoncePerAddressMutex.Unlock()
	if tx.Status != baseTypes.BaseTxStatusSucceeded && tx.Status != baseTypes.BaseTxStatusFailed {
		// not a completed tx, no op
		return updated
	}
	currentNonce, exists := enterpriseHandler.completedTxNoncePerAddress[string(tx.From)]
	if !exists || currentNonce.Cmp(tx.Nonce.BigInt()) == -1 {
		enterpriseHandler.completedTxNoncePerAddress[string(tx.From)] = *tx.Nonce.BigInt()
		updated = true
	}
	return updated
}

func (enterpriseHandler *enterpriseTransactionHandler) HandleTransactionConfirmations(ctx context.Context, txID string, notification *baseTypes.ConfirmationsNotification) (err error) {
	// enterprise handler doesn't implement the default call back
	// we pass in transaction specific callback to use the extra info in the disclosure
	return i18n.NewError(ctx, msgs.MsgConfirmationHandlerNotFound)
}
func (enterpriseHandler *enterpriseTransactionHandler) HandleTransactionReceiptReceived(ctx context.Context, txID string, receipt *ethclient.TransactionReceiptResponse) (err error) {
	// enterprise handler doesn't implement the default call back
	// we pass in transaction specific callback to use the extra info in the disclosure
	return i18n.NewError(ctx, msgs.MsgReceiptHandlerNotFound)
}
