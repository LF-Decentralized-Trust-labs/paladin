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

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/retry"
	"github.com/kaleido-io/paladin/core/internal/components"
	baseTypes "github.com/kaleido-io/paladin/core/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type inFlightTransactionState struct {
	testOnlyNoEventMode bool // Note: this flag can never be set in normal code path, exposed for testing only
	retry               *retry.Retry

	PublicTxEngineMetricsManager
	baseTypes.BalanceManager

	txStore  components.TransactionStore
	bIndexer blockindexer.BlockIndexer
	// input that should be set once the stage is running
	*baseTypes.TransientPreviousStageOutputs
	orchestratorContext *baseTypes.OrchestratorContext
	baseTypes.InFlightStageActionTriggers
	baseTypes.InMemoryTxStateManager

	// the current in-flight stage
	// this is the core of in-flight transaction processing.
	// only 1 stage context can exist at any given time for a specific transaction.
	// in flight transaction contains the logic to process each stage to its completion,
	// any stage will have at least 1 asynchronous action, in-flight transaction relies on transaction orchestrator
	// to give it signal to collect result of those async actions.
	// Therefore, any coordination required cross in-flight transaction can be taken into consideration for next stage.
	//    e.g. even if the transaction is ready for submission, we might not want to submit it if the other transactions
	//     ahead of the current transaction used up all the funds
	runningStageContext                *baseTypes.RunningStageContext
	validatedTransactionHashMatchState bool

	// the current stage of this inflight transaction
	turnOffHistory        bool
	stage                 baseTypes.InFlightTxStage
	txLevelStageStartTime time.Time
	stageTriggerError     error

	bufferedStageOutputsMux sync.Mutex
	bufferedStageOutputs    []*baseTypes.StageOutput
}

func (iftxs *inFlightTransactionState) CanSubmit(ctx context.Context, cost *big.Int) bool {
	log.L(ctx).Tracef("ProcessInFlightTransaction transaction entry, transaction orchestrator context: %+v, cost: %s", iftxs.orchestratorContext, cost.String())
	if iftxs.orchestratorContext.AvailableToSpend == nil {
		log.L(ctx).Tracef("ProcessInFlightTransaction transaction can be submitted for zero gas price chain, orchestrator context: %+v", iftxs.orchestratorContext)
		return true
	}
	if cost != nil {
		return iftxs.orchestratorContext.AvailableToSpend.Cmp(cost) != -1 && !iftxs.orchestratorContext.PreviousNonceCostUnknown
	}
	log.L(ctx).Debugf("ProcessInFlightTransaction cannot submit transaction, transaction orchestrator context: %+v, cost: %s", iftxs.orchestratorContext, cost.String())
	return false
}

func (iftxs *inFlightTransactionState) StartNewStageContext(ctx context.Context, stage baseTypes.InFlightTxStage, substatus components.BaseTxSubStatus) {
	nowTime := time.Now() // pin the now time
	rsc := NewRunningStageContext(ctx, stage, substatus, iftxs.InMemoryTxStateManager)
	if rsc.Stage != iftxs.stage {
		if string(iftxs.stage) != "" {
			// record metrics for the previous stage
			iftxs.RecordStageChangeMetrics(ctx, string(iftxs.stage), float64(nowTime.Sub(iftxs.txLevelStageStartTime).Seconds()))
		}
		log.L(ctx).Tracef("Transaction with ID %s, switching from %s to %s after %s", rsc.InMemoryTx.GetTxID(), iftxs.stage, rsc.Stage, time.Since(iftxs.txLevelStageStartTime))
		// set to the new stage
		iftxs.stage = rsc.Stage
		iftxs.txLevelStageStartTime = nowTime
	} else {
		log.L(ctx).Tracef("Transaction with ID %s, already on stage %s for %s", rsc.InMemoryTx.GetTxID(), stage, time.Since(iftxs.txLevelStageStartTime))
	}
	iftxs.stageTriggerError = nil
	iftxs.runningStageContext = rsc
	switch stage {
	case baseTypes.InFlightTxStageRetrieveGasPrice:
		log.L(ctx).Tracef("Transaction with ID %s, triggering retrieve gas price", rsc.InMemoryTx.GetTxID())
		iftxs.stageTriggerError = iftxs.TriggerRetrieveGasPrice(ctx)
	case baseTypes.InFlightTxStageSigning:
		log.L(ctx).Tracef("Transaction with ID %s, triggering sign tx", rsc.InMemoryTx.GetTxID())
		iftxs.stageTriggerError = iftxs.TriggerSignTx(ctx)
	case baseTypes.InFlightTxStageSubmitting:
		log.L(ctx).Tracef("Transaction with ID %s, triggering submission, signed message not nil: %t", rsc.InMemoryTx.GetTxID(), iftxs.TransientPreviousStageOutputs != nil && iftxs.TransientPreviousStageOutputs.SignedMessage != nil)
		var signedMessage []byte
		if iftxs.TransientPreviousStageOutputs != nil {
			signedMessage = iftxs.TransientPreviousStageOutputs.SignedMessage
		}
		iftxs.stageTriggerError = iftxs.TriggerSubmitTx(ctx, signedMessage)
	case baseTypes.InFlightTxStageStatusUpdate:
		log.L(ctx).Tracef("Transaction with ID %s, triggering status update", rsc.InMemoryTx.GetTxID())
		iftxs.stageTriggerError = iftxs.TriggerStatusUpdate(ctx)
	default:
		log.L(ctx).Tracef("Transaction with ID %s, didn't trigger any action for new stage: %s", rsc.InMemoryTx.GetTxID(), stage)
	}
}

func (iftxs *inFlightTransactionState) GetStage(ctx context.Context) baseTypes.InFlightTxStage {
	return iftxs.stage
}

func (iftxs *inFlightTransactionState) GetStageStartTime(ctx context.Context) time.Time {
	return iftxs.txLevelStageStartTime
}

func (iftxs *inFlightTransactionState) SetValidatedTransactionHashMatchState(ctx context.Context, validatedTransactionHashMatchState bool) {
	iftxs.validatedTransactionHashMatchState = validatedTransactionHashMatchState
}

func (iftxs *inFlightTransactionState) ValidatedTransactionHashMatchState(ctx context.Context) bool {
	return iftxs.validatedTransactionHashMatchState
}

func (iftxs *inFlightTransactionState) SetOrchestratorContext(ctx context.Context, tec *baseTypes.OrchestratorContext) {
	iftxs.orchestratorContext = tec
}

func (iftxs *inFlightTransactionState) SetTransientPreviousStageOutputs(tpso *baseTypes.TransientPreviousStageOutputs) {
	iftxs.TransientPreviousStageOutputs = tpso
}

func (iftxs *inFlightTransactionState) GetRunningStageContext(ctx context.Context) *baseTypes.RunningStageContext {
	return iftxs.runningStageContext
}

func (iftxs *inFlightTransactionState) GetStageTriggerError(ctx context.Context) error {
	return iftxs.stageTriggerError
}

func (iftxs *inFlightTransactionState) ClearRunningStageContext(ctx context.Context) {
	if iftxs.runningStageContext != nil {
		rsc := iftxs.runningStageContext
		log.L(ctx).Debugf("Transaction with ID %s clearing stage context for stage: %s after %s, total time spent on this stage so far: %s, txHash: %s", rsc.InMemoryTx.GetTxID(), rsc.Stage, time.Since(rsc.StageStartTime), time.Since(iftxs.txLevelStageStartTime), rsc.InMemoryTx.GetTransactionHash())
	} else {
		log.L(ctx).Warnf("Transaction with ID %s  has no running stage context to clear", iftxs.InMemoryTxStateManager.GetTxID())
	}
	iftxs.runningStageContext = nil
	iftxs.stageTriggerError = nil
}

func (iftxs *inFlightTransactionState) ProcessStageOutputs(ctx context.Context, processFunction func(stageOutputs []*baseTypes.StageOutput) (unprocessedStageOutputs []*baseTypes.StageOutput)) {
	iftxs.bufferedStageOutputsMux.Lock()
	defer iftxs.bufferedStageOutputsMux.Unlock()
	iftxs.bufferedStageOutputs = processFunction(iftxs.bufferedStageOutputs)
}

func (iftxs *inFlightTransactionState) AddStageOutputs(ctx context.Context, stageOutput *baseTypes.StageOutput) {
	if iftxs.testOnlyNoEventMode {
		return
	}
	iftxs.bufferedStageOutputsMux.Lock()
	defer iftxs.bufferedStageOutputsMux.Unlock()
	iftxs.bufferedStageOutputs = append(iftxs.bufferedStageOutputs, stageOutput)
}

func NewInFlightTransactionStateManager(thm PublicTxEngineMetricsManager,
	bm baseTypes.BalanceManager,
	txStore components.TransactionStore,
	bIndexer blockindexer.BlockIndexer,
	ifsat baseTypes.InFlightStageActionTriggers,
	imtxs baseTypes.InMemoryTxStateManager,
	retry *retry.Retry,
	turnOffHistory bool,
	noEventMode bool,
) baseTypes.InFlightTransactionStateManager {
	return &inFlightTransactionState{
		testOnlyNoEventMode:          noEventMode,
		retry:                        retry,
		PublicTxEngineMetricsManager: thm,
		BalanceManager:               bm,
		txStore:                      txStore,
		bIndexer:                     bIndexer,
		InFlightStageActionTriggers:  ifsat,
		bufferedStageOutputs:         make([]*baseTypes.StageOutput, 0),
		txLevelStageStartTime:        time.Now(),
		InMemoryTxStateManager:       imtxs,
		turnOffHistory:               turnOffHistory,
	}
}

func (iftxs *inFlightTransactionState) AddPersistenceOutput(ctx context.Context, stage baseTypes.InFlightTxStage, persistenceTime time.Time, err error) {
	start := time.Now()
	iftxs.AddStageOutputs(ctx, &baseTypes.StageOutput{
		Stage: stage,
		PersistenceOutput: &baseTypes.PersistenceOutput{
			PersistenceError: err,
			Time:             persistenceTime,
		},
	})
	log.L(ctx).Debugf("%s AddPersistenceOutput took %s to write the result", iftxs.InMemoryTxStateManager.GetTxID(), time.Since(start))
}

func (iftxs *inFlightTransactionState) CanBeRemoved(ctx context.Context) bool {
	return iftxs.IsComplete() && iftxs.runningStageContext == nil
}

func (iftxs *inFlightTransactionState) AddSubmitOutput(ctx context.Context, txHash string, submissionTime *fftypes.FFTime, submissionOutcome baseTypes.SubmissionOutcome, errorReason ethclient.ErrorReason, err error) {
	start := time.Now()
	log.L(ctx).Debugf("%s Setting submit output, hash %s, submissionOutcome: %s, errReason: %s, err %+v", iftxs.InMemoryTxStateManager.GetTxID(), txHash, submissionOutcome, errorReason, err)
	iftxs.AddStageOutputs(ctx, &baseTypes.StageOutput{
		Stage: baseTypes.InFlightTxStageSubmitting,
		SubmitOutput: &baseTypes.SubmitOutputs{
			TxHash:            txHash,
			SubmissionTime:    submissionTime,
			SubmissionOutcome: submissionOutcome,
			ErrorReason:       string(errorReason),
			Err:               err,
		},
	})
	log.L(ctx).Debugf("%s AddSubmitOutput took %s to write the result", iftxs.InMemoryTxStateManager.GetTxID(), time.Since(start))
}

func (iftxs *inFlightTransactionState) AddSignOutput(ctx context.Context, signedMessage []byte, txHash string, err error) {
	start := time.Now()
	log.L(ctx).Debugf("%s Setting signed message, hash %s, signed message not nil %t, err %+v", iftxs.InMemoryTxStateManager.GetTxID(), txHash, signedMessage != nil, err)
	iftxs.AddStageOutputs(ctx, &baseTypes.StageOutput{
		Stage: baseTypes.InFlightTxStageSigning,
		SignOutput: &baseTypes.SignOutputs{
			SignedMessage: signedMessage,
			TxHash:        txHash,
			Err:           err,
		},
	})
	log.L(ctx).Debugf("%s AddSignOutput took %s to write the result", iftxs.InMemoryTxStateManager.GetTxID(), time.Since(start))
}
func (iftxs *inFlightTransactionState) AddGasPriceOutput(ctx context.Context, gasPriceObject *baseTypes.GasPriceObject, err error) {
	start := time.Now()
	iftxs.AddStageOutputs(ctx, &baseTypes.StageOutput{
		Stage: baseTypes.InFlightTxStageRetrieveGasPrice,
		GasPriceOutput: &baseTypes.GasPriceOutput{
			GasPriceObject: gasPriceObject,
			Err:            err,
		},
	})
	log.L(ctx).Debugf("%s AddGasPriceOutput took %s to write the result", iftxs.InMemoryTxStateManager.GetTxID(), time.Since(start))
}

func (iftxs *inFlightTransactionState) AddConfirmationsOutput(ctx context.Context, confirmedTx *blockindexer.IndexedTransaction) {
	start := time.Now()
	iftxs.AddStageOutputs(ctx, &baseTypes.StageOutput{
		Stage: baseTypes.InFlightTxStageConfirming,
		ConfirmationOutput: &baseTypes.ConfirmationOutputs{
			ConfirmedTransaction: confirmedTx,
		},
	})
	log.L(ctx).Debugf("%s AddConfirmationsOutput took %s to write the result", iftxs.InMemoryTxStateManager.GetTxID(), time.Since(start))
}

func (iftxs *inFlightTransactionState) AddPanicOutput(ctx context.Context, stage baseTypes.InFlightTxStage) {
	start := time.Now()
	// unexpected error, set an empty input for the stage
	// so that the stage handler will handle this as unexpected error
	iftxs.AddStageOutputs(ctx, &baseTypes.StageOutput{
		Stage: stage,
	})
	log.L(ctx).Debugf("%s AddPanicOutput took %s to write the result", iftxs.InMemoryTxStateManager.GetTxID(), time.Since(start))
}

func (iftxs *inFlightTransactionState) PersistTxState(ctx context.Context) (stage baseTypes.InFlightTxStage, persistenceTime time.Time, err error) {
	rsc := iftxs.runningStageContext
	mtx := iftxs.GetTx()
	if rsc == nil || rsc.StageOutputsToBePersisted == nil {
		log.L(ctx).Error("Cannot persist transaction state, no running context or stageOutputsToBePersisted")
		return iftxs.stage, time.Now(), i18n.NewError(ctx, msgs.MsgPersistError)
	}
	switch rsc.StageOutputsToBePersisted.UpdateType {
	case baseTypes.PersistenceUpdateUpdate:
		it := rsc.StageOutputsToBePersisted.ConfirmedTransaction
		trackedHashes := iftxs.GetSubmittedHashes()
		hashAlreadyTracked := false
		matchFound := false
		if rsc.StageOutputsToBePersisted.TxUpdates != nil && rsc.StageOutputsToBePersisted.TxUpdates.TransactionHash != nil && *rsc.StageOutputsToBePersisted.TxUpdates.TransactionHash != "" {
			newTxHash := *rsc.StageOutputsToBePersisted.TxUpdates.TransactionHash
			for _, h := range trackedHashes {
				if h == newTxHash {
					hashAlreadyTracked = true
				}
				if it != nil && h == it.Hash.String() {
					matchFound = true
				}
			}
			if !hashAlreadyTracked {
				matchFound = matchFound || newTxHash == it.Hash.String()
				rsc.StageOutputsToBePersisted.TxUpdates.SubmittedHashes = append(trackedHashes[:], newTxHash)
			}
		}
		if it != nil || rsc.StageOutputsToBePersisted.MissedConfirmationEvent {
			if it == nil {
				err = iftxs.retry.Do(ctx, "get confirmed transaction for "+mtx.ID, func(attempt int) (retry bool, err error) {
					retrievedTx, retryErr := iftxs.bIndexer.GetIndexedTransactionByNonce(ctx, *tktypes.MustEthAddress(string(mtx.From)), mtx.Nonce.Uint64())
					if retryErr == nil && retrievedTx == nil {
						// panic("block indexer missed a nonce")
						// the logic is in a confirmation loop until block indexer indexed the missing transaction
						return false, i18n.NewError(ctx, msgs.MsgMissingConfirmedTransaction, mtx.ID)
					}
					it = retrievedTx
					return true, retryErr
				})
				if err != nil {
					return rsc.Stage, time.Now(), err
				}
			}
			iftxs.NotifyAddressBalanceChanged(ctx, string(mtx.From))
			if mtx.Value != nil && mtx.To != nil {
				iftxs.NotifyAddressBalanceChanged(ctx, mtx.To.String())
			}
			if err = iftxs.txStore.SetConfirmedTransaction(ctx, mtx.ID, it); err != nil {
				log.L(ctx).Errorf("Failed to persist confirmed transaction for transaction %s due to error: %+v, confirmed tx: %+v", mtx.ID, err, it)
				return rsc.Stage, time.Now(), err
			}
			// update the in memory state
			iftxs.SetConfirmedTransaction(ctx, it)

			if matchFound {
				if it.Result == blockindexer.TXResult_SUCCESS.Enum() {
					mtx.Status = components.BaseTxStatusSucceeded
					rsc.StageOutputsToBePersisted.TxUpdates.Status = &mtx.Status
					iftxs.RecordCompletedTransactionCountMetrics(ctx, string(GenericStatusSuccess))
				} else {
					mtx.Status = components.BaseTxStatusFailed
					rsc.StageOutputsToBePersisted.TxUpdates.Status = &mtx.Status
					iftxs.RecordCompletedTransactionCountMetrics(ctx, string(GenericStatusFail))
				}
			} else {
				mtx.Status = components.BaseTxStatusConflict
				rsc.StageOutputsToBePersisted.TxUpdates.Status = &mtx.Status
				iftxs.RecordCompletedTransactionCountMetrics(ctx, string(GenericStatusConflict))
			}
			if rsc.SubStatus != components.BaseTxSubStatusConfirmed {
				rsc.StageOutputsToBePersisted.AddSubStatusAction(components.BaseTxActionConfirmTransaction, nil, nil)
				rsc.SetSubStatus(components.BaseTxSubStatusConfirmed)
			}
		}
		if !iftxs.turnOffHistory {
			// flush any sub-status changes
			for _, historyUpdate := range rsc.StageOutputsToBePersisted.HistoryUpdates {
				if err := historyUpdate(iftxs.txStore); err != nil {
					return rsc.Stage, time.Now(), err
				}
			}
		}
		if rsc.StageOutputsToBePersisted.TxUpdates != nil {
			err := iftxs.txStore.UpdateTransaction(ctx, mtx.ID, rsc.StageOutputsToBePersisted.TxUpdates)
			if err != nil {
				log.L(ctx).Errorf("Failed to update transaction %s (status=%s): %+v", mtx.ID, mtx.Status, err)
				return rsc.Stage, time.Now(), err
			}
			// update the in memory state
			iftxs.ApplyTxUpdates(ctx, rsc.StageOutputsToBePersisted.TxUpdates)
		}
	}
	return rsc.Stage, time.Now(), nil
}