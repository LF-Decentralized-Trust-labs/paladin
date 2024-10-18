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

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type inFlightTransactionState struct {
	testOnlyNoEventMode bool // Note: this flag can never be set in normal code path, exposed for testing only
	retry               *retry.Retry

	PublicTxManagerMetricsManager
	BalanceManager

	bIndexer blockindexer.BlockIndexer
	// input that should be set once the stage is running
	*TransientPreviousStageOutputs
	orchestratorContext *OrchestratorContext
	InFlightStageActionTriggers
	InMemoryTxStateManager

	// the current in-flight stage
	// this is the core of in-flight transaction processing.
	// only 1 stage context can exist at any given time for a specific transaction.
	// in flight transaction contains the logic to process each stage to its completion,
	// any stage will have at least 1 asynchronous action, in-flight transaction relies on transaction orchestrator
	// to give it signal to collect result of those async actions.
	// Therefore, any coordination required cross in-flight transaction can be taken into consideration for next stage.
	//    e.g. even if the transaction is ready for submission, we might not want to submit it if the other transactions
	//     ahead of the current transaction used up all the funds
	runningStageContext                *RunningStageContext
	validatedTransactionHashMatchState bool

	// the current stage of this inflight transaction
	stage                 InFlightTxStage
	txLevelStageStartTime time.Time
	stageTriggerError     error

	bufferedStageOutputsMux sync.Mutex
	bufferedStageOutputs    []*StageOutput

	submissionWriter *submissionWriter
	statusUpdater    StatusUpdater
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

func (iftxs *inFlightTransactionState) StartNewStageContext(ctx context.Context, stage InFlightTxStage, substatus BaseTxSubStatus) {
	nowTime := time.Now() // pin the now time
	rsc := NewRunningStageContext(ctx, stage, substatus, iftxs.InMemoryTxStateManager)
	if rsc.Stage != iftxs.stage {
		if string(iftxs.stage) != "" {
			// record metrics for the previous stage
			iftxs.RecordStageChangeMetrics(ctx, string(iftxs.stage), float64(nowTime.Sub(iftxs.txLevelStageStartTime).Seconds()))
		}
		log.L(ctx).Tracef("Transaction with ID %s, switching from %s to %s after %s", rsc.InMemoryTx.GetSignerNonce(), iftxs.stage, rsc.Stage, time.Since(iftxs.txLevelStageStartTime))
		// set to the new stage
		iftxs.stage = rsc.Stage
		iftxs.txLevelStageStartTime = nowTime
	} else {
		log.L(ctx).Tracef("Transaction with ID %s, already on stage %s for %s", rsc.InMemoryTx.GetSignerNonce(), stage, time.Since(iftxs.txLevelStageStartTime))
	}
	iftxs.stageTriggerError = nil
	iftxs.runningStageContext = rsc
	switch stage {
	case InFlightTxStageRetrieveGasPrice:
		log.L(ctx).Tracef("Transaction with ID %s, triggering retrieve gas price", rsc.InMemoryTx.GetSignerNonce())
		iftxs.stageTriggerError = iftxs.TriggerRetrieveGasPrice(ctx)
	case InFlightTxStageSigning:
		log.L(ctx).Tracef("Transaction with ID %s, triggering sign tx", rsc.InMemoryTx.GetSignerNonce())
		iftxs.stageTriggerError = iftxs.TriggerSignTx(ctx)
	case InFlightTxStageSubmitting:
		log.L(ctx).Tracef("Transaction with ID %s, triggering submission, signed message not nil: %t", rsc.InMemoryTx.GetSignerNonce(), iftxs.TransientPreviousStageOutputs != nil && iftxs.TransientPreviousStageOutputs.SignedMessage != nil)
		var signedMessage []byte
		if iftxs.TransientPreviousStageOutputs != nil {
			signedMessage = iftxs.TransientPreviousStageOutputs.SignedMessage
		}
		iftxs.stageTriggerError = iftxs.TriggerSubmitTx(ctx, signedMessage)
	case InFlightTxStageStatusUpdate:
		log.L(ctx).Tracef("Transaction with ID %s, triggering status update", rsc.InMemoryTx.GetSignerNonce())
		iftxs.stageTriggerError = iftxs.TriggerStatusUpdate(ctx)
	default:
		log.L(ctx).Tracef("Transaction with ID %s, didn't trigger any action for new stage: %s", rsc.InMemoryTx.GetSignerNonce(), stage)
	}
}

func (iftxs *inFlightTransactionState) GetStage(ctx context.Context) InFlightTxStage {
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

func (iftxs *inFlightTransactionState) SetOrchestratorContext(ctx context.Context, tec *OrchestratorContext) {
	iftxs.orchestratorContext = tec
}

func (iftxs *inFlightTransactionState) SetTransientPreviousStageOutputs(tpso *TransientPreviousStageOutputs) {
	iftxs.TransientPreviousStageOutputs = tpso
}

func (iftxs *inFlightTransactionState) GetRunningStageContext(ctx context.Context) *RunningStageContext {
	return iftxs.runningStageContext
}

func (iftxs *inFlightTransactionState) GetStageTriggerError(ctx context.Context) error {
	return iftxs.stageTriggerError
}

func (iftxs *inFlightTransactionState) ClearRunningStageContext(ctx context.Context) {
	if iftxs.runningStageContext != nil {
		rsc := iftxs.runningStageContext
		log.L(ctx).Debugf("Transaction with ID %s clearing stage context for stage: %s after %s, total time spent on this stage so far: %s, txHash: %s", rsc.InMemoryTx.GetSignerNonce(), rsc.Stage, time.Since(rsc.StageStartTime), time.Since(iftxs.txLevelStageStartTime), rsc.InMemoryTx.GetTransactionHash())
	} else {
		log.L(ctx).Warnf("Transaction with ID %s  has no running stage context to clear", iftxs.InMemoryTxStateManager.GetSignerNonce())
	}
	iftxs.runningStageContext = nil
	iftxs.stageTriggerError = nil
}

func (iftxs *inFlightTransactionState) ProcessStageOutputs(ctx context.Context, processFunction func(stageOutputs []*StageOutput) (unprocessedStageOutputs []*StageOutput)) {
	iftxs.bufferedStageOutputsMux.Lock()
	defer iftxs.bufferedStageOutputsMux.Unlock()
	iftxs.bufferedStageOutputs = processFunction(iftxs.bufferedStageOutputs)
}

func (iftxs *inFlightTransactionState) AddStageOutputs(ctx context.Context, stageOutput *StageOutput) {
	if iftxs.testOnlyNoEventMode {
		return
	}
	iftxs.bufferedStageOutputsMux.Lock()
	defer iftxs.bufferedStageOutputsMux.Unlock()
	iftxs.bufferedStageOutputs = append(iftxs.bufferedStageOutputs, stageOutput)
}

func NewInFlightTransactionStateManager(thm PublicTxManagerMetricsManager,
	bm BalanceManager,
	bIndexer blockindexer.BlockIndexer,
	ifsat InFlightStageActionTriggers,
	imtxs InMemoryTxStateManager,
	retry *retry.Retry,
	statusUpdater StatusUpdater,
	submissionWriter *submissionWriter,
	noEventMode bool,
) InFlightTransactionStateManager {
	return &inFlightTransactionState{
		testOnlyNoEventMode:           noEventMode,
		retry:                         retry,
		PublicTxManagerMetricsManager: thm,
		BalanceManager:                bm,
		bIndexer:                      bIndexer,
		InFlightStageActionTriggers:   ifsat,
		bufferedStageOutputs:          make([]*StageOutput, 0),
		txLevelStageStartTime:         time.Now(),
		InMemoryTxStateManager:        imtxs,
		statusUpdater:                 statusUpdater,
		submissionWriter:              submissionWriter,
	}
}

func (iftxs *inFlightTransactionState) AddPersistenceOutput(ctx context.Context, stage InFlightTxStage, persistenceTime time.Time, err error) {
	start := time.Now()
	iftxs.AddStageOutputs(ctx, &StageOutput{
		Stage: stage,
		PersistenceOutput: &PersistenceOutput{
			PersistenceError: err,
			Time:             persistenceTime,
		},
	})
	log.L(ctx).Debugf("%s AddPersistenceOutput took %s to write the result", iftxs.InMemoryTxStateManager.GetSignerNonce(), time.Since(start))
}

func (iftxs *inFlightTransactionState) CanBeRemoved(ctx context.Context) bool {
	return iftxs.IsReadyToExit() && iftxs.runningStageContext == nil
}

func (iftxs *inFlightTransactionState) AddSubmitOutput(ctx context.Context, txHash *tktypes.Bytes32, submissionTime *tktypes.Timestamp, submissionOutcome SubmissionOutcome, errorReason ethclient.ErrorReason, err error) {
	start := time.Now()
	log.L(ctx).Debugf("%s Setting submit output, submissionOutcome: %s, errReason: %s, err %+v", iftxs.InMemoryTxStateManager.GetSignerNonce(), submissionOutcome, errorReason, err)
	iftxs.AddStageOutputs(ctx, &StageOutput{
		Stage: InFlightTxStageSubmitting,
		SubmitOutput: &SubmitOutputs{
			SubmissionTime:    submissionTime,
			SubmissionOutcome: submissionOutcome,
			TxHash:            txHash,
			ErrorReason:       string(errorReason),
			Err:               err,
		},
	})
	log.L(ctx).Debugf("%s AddSubmitOutput took %s to write the result", iftxs.InMemoryTxStateManager.GetSignerNonce(), time.Since(start))
}

func (iftxs *inFlightTransactionState) AddSignOutput(ctx context.Context, signedMessage []byte, txHash *tktypes.Bytes32, err error) {
	start := time.Now()
	log.L(ctx).Debugf("%s Setting signed message, hash %s, signed message not nil %t, err %+v", iftxs.InMemoryTxStateManager.GetSignerNonce(), txHash, signedMessage != nil, err)
	iftxs.AddStageOutputs(ctx, &StageOutput{
		Stage: InFlightTxStageSigning,
		SignOutput: &SignOutputs{
			SignedMessage: signedMessage,
			TxHash:        txHash,
			Err:           err,
		},
	})
	log.L(ctx).Debugf("%s AddSignOutput took %s to write the result", iftxs.InMemoryTxStateManager.GetSignerNonce(), time.Since(start))
}

func (iftxs *inFlightTransactionState) AddGasPriceOutput(ctx context.Context, gasPriceObject *pldapi.PublicTxGasPricing, err error) {
	start := time.Now()
	iftxs.AddStageOutputs(ctx, &StageOutput{
		Stage: InFlightTxStageRetrieveGasPrice,
		GasPriceOutput: &GasPriceOutput{
			GasPriceObject: gasPriceObject,
			Err:            err,
		},
	})
	log.L(ctx).Debugf("%s AddGasPriceOutput took %s to write the result", iftxs.InMemoryTxStateManager.GetSignerNonce(), time.Since(start))
}

func (iftxs *inFlightTransactionState) AddConfirmationsOutput(ctx context.Context, confirmedTx *pldapi.IndexedTransaction) {
	panic("unused")
	// start := time.Now()
	// iftxs.AddStageOutputs(ctx, &StageOutput{
	// 	Stage:              InFlightTxStageConfirming,
	// 	ConfirmationOutput: &ConfirmationOutputs{},
	// })
	// log.L(ctx).Debugf("%s AddConfirmationsOutput took %s to write the result", iftxs.InMemoryTxStateManager.GetSignerNonce(), time.Since(start))
}

func (iftxs *inFlightTransactionState) AddPanicOutput(ctx context.Context, stage InFlightTxStage) {
	start := time.Now()
	// unexpected error, set an empty input for the stage
	// so that the stage handler will handle this as unexpected error
	iftxs.AddStageOutputs(ctx, &StageOutput{
		Stage: stage,
	})
	log.L(ctx).Debugf("%s AddPanicOutput took %s to write the result", iftxs.InMemoryTxStateManager.GetSignerNonce(), time.Since(start))
}

func (iftxs *inFlightTransactionState) PersistTxState(ctx context.Context) (stage InFlightTxStage, persistenceTime time.Time, err error) {
	rsc := iftxs.runningStageContext
	if rsc == nil || rsc.StageOutputsToBePersisted == nil {
		log.L(ctx).Error("Cannot persist transaction state, no running context or stageOutputsToBePersisted")
		return iftxs.stage, time.Now(), i18n.NewError(ctx, msgs.MsgPersistError)
	}

	// flush any sub-status changes
	for _, subStatusUpdate := range rsc.StageOutputsToBePersisted.StatusUpdates {
		if err := subStatusUpdate(iftxs.statusUpdater); err != nil {
			return rsc.Stage, time.Now(), err
		}
	}

	if rsc.StageOutputsToBePersisted.TxUpdates != nil {

		newSubmission := rsc.StageOutputsToBePersisted.TxUpdates.NewSubmission
		if newSubmission != nil {
			// This is the critical point where we must flush to persistence before we go any further - we have a new
			// transaction record we've signed, and we want to move on to submit it to the blockchain.
			// But if we do that without first recording the transaction hash, we cannot be sure we will be able
			// to correlate back and complete the transaction when requested by the blockchain indexer.
			//
			// This can be happening on lots of threads at the same time for different transactions,
			// so we don't want to create an excessive number of DB transactions.
			// Instead we use a pool of flush-writers that do the insertion in batches.
			op := iftxs.submissionWriter.Queue(ctx, newSubmission)
			_, err := op.WaitFlushed(ctx)
			if err != nil {
				return rsc.Stage, time.Now(), err
			}
		}

		if rsc.StageOutputsToBePersisted.TxUpdates.InFlightStatus != nil &&
			*rsc.StageOutputsToBePersisted.TxUpdates.InFlightStatus == InFlightStatusConfirmReceived {
			iftxs.RecordCompletedTransactionCountMetrics(ctx, string(GenericStatusSuccess))
		}

		// update the in memory state
		iftxs.ApplyInMemoryUpdates(ctx, rsc.StageOutputsToBePersisted.TxUpdates)
	}
	return rsc.Stage, time.Now(), nil
}
