/*
 * Copyright © 2025 Kaleido, Inc.
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
	"sync"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/publictxmgr/metrics"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/ethclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

type inFlightTransactionStateGeneration struct {
	current             bool
	testOnlyNoEventMode bool

	metrics.PublicTransactionManagerMetrics
	InFlightStageActionTriggers
	InMemoryTxStateManager

	// input that should be set once the stage is running
	*TransientPreviousStageOutputs

	// the current in-flight stage
	// this is the core of in-flight transaction processing.
	// only 1 stage context can exist at any given time for a specific transaction generation.
	// in the case there are multiple generations, only the current generation is allowed to progress,
	// previous generations may persist what they have done so far but no more
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

	cancel chan bool

	submissionWriter *submissionWriter
	statusUpdater    StatusUpdater
}

func NewInFlightTransactionStateGeneration(
	thm metrics.PublicTransactionManagerMetrics,
	bm BalanceManager,
	ifsat InFlightStageActionTriggers,
	imtxs InMemoryTxStateManager,
	statusUpdater StatusUpdater,
	submissionWriter *submissionWriter,
	noEventMode bool) InFlightTransactionStateGeneration {
	return &inFlightTransactionStateGeneration{
		current:                         true,
		bufferedStageOutputs:            make([]*StageOutput, 0),
		cancel:                          make(chan bool, 1),
		testOnlyNoEventMode:             noEventMode,
		txLevelStageStartTime:           time.Now(),
		statusUpdater:                   statusUpdater,
		submissionWriter:                submissionWriter,
		PublicTransactionManagerMetrics: thm,
		InFlightStageActionTriggers:     ifsat,
		InMemoryTxStateManager:          imtxs,
	}
}

func (v *inFlightTransactionStateGeneration) Cancel(ctx context.Context) {
	select {
	case v.cancel <- true:
	default:
	}
}

// IsCancelled is intended to be used by async actions to check if there is a now a new generation and they should stop work.
// There should only ever be 1 async action running at a time for a given generation
func (v *inFlightTransactionStateGeneration) IsCancelled(ctx context.Context) bool {
	select {
	case <-v.cancel:
		return true
	default:
		return false
	}
}

func (v *inFlightTransactionStateGeneration) SetCurrent(ctx context.Context, current bool) {
	v.current = current
}

func (v *inFlightTransactionStateGeneration) IsCurrent(ctx context.Context) bool {
	return v.current
}

func (v *inFlightTransactionStateGeneration) GetStage(ctx context.Context) InFlightTxStage {
	return v.stage
}

func (v *inFlightTransactionStateGeneration) GetStageStartTime(ctx context.Context) time.Time {
	return v.txLevelStageStartTime
}

func (v *inFlightTransactionStateGeneration) SetValidatedTransactionHashMatchState(ctx context.Context, validatedTransactionHashMatchState bool) {
	v.validatedTransactionHashMatchState = validatedTransactionHashMatchState
}

func (v *inFlightTransactionStateGeneration) ValidatedTransactionHashMatchState(ctx context.Context) bool {
	return v.validatedTransactionHashMatchState
}

func (v *inFlightTransactionStateGeneration) SetTransientPreviousStageOutputs(tpso *TransientPreviousStageOutputs) {
	v.TransientPreviousStageOutputs = tpso
}

func (v *inFlightTransactionStateGeneration) GetRunningStageContext(ctx context.Context) *RunningStageContext {
	return v.runningStageContext
}

func (v *inFlightTransactionStateGeneration) GetStageTriggerError(ctx context.Context) error {
	return v.stageTriggerError
}

func (v *inFlightTransactionStateGeneration) StartNewStageContext(ctx context.Context, stage InFlightTxStage, substatus BaseTxSubStatus) {
	nowTime := time.Now() // pin the now time
	rsc := NewRunningStageContext(ctx, stage, substatus, v.InMemoryTxStateManager)
	if rsc.Stage != v.stage {
		if string(v.stage) != "" {
			// record metrics for the previous stage
			v.RecordStageChangeMetrics(ctx, string(v.stage), float64(nowTime.Sub(v.txLevelStageStartTime).Seconds()))
		}
		log.L(ctx).Tracef("Transaction with ID %s, switching from %s to %s after %s", rsc.InMemoryTx.GetSignerNonce(), v.stage, rsc.Stage, time.Since(v.txLevelStageStartTime))
		// set to the new stage
		v.stage = rsc.Stage
		v.txLevelStageStartTime = nowTime
	} else {
		log.L(ctx).Tracef("Transaction with ID %s, already on stage %s for %s", rsc.InMemoryTx.GetSignerNonce(), stage, time.Since(v.txLevelStageStartTime))
	}
	v.stageTriggerError = nil
	v.runningStageContext = rsc
	switch stage {
	case InFlightTxStageRetrieveGasPrice:
		log.L(ctx).Tracef("Transaction with ID %s, triggering retrieve gas price", rsc.InMemoryTx.GetSignerNonce())
		v.stageTriggerError = v.TriggerRetrieveGasPrice(ctx)
	case InFlightTxStageSigning:
		log.L(ctx).Tracef("Transaction with ID %s, triggering sign tx", rsc.InMemoryTx.GetSignerNonce())
		v.stageTriggerError = v.TriggerSignTx(ctx)
	case InFlightTxStageSubmitting:
		log.L(ctx).Tracef("Transaction with ID %s, triggering submission, signed message not nil: %t", rsc.InMemoryTx.GetSignerNonce(), v.TransientPreviousStageOutputs != nil && v.SignedMessage != nil)
		var signedMessage []byte
		var calculatedTxHash *pldtypes.Bytes32
		if v.TransientPreviousStageOutputs != nil {
			signedMessage = v.SignedMessage
			calculatedTxHash = v.TransactionHash
		}
		v.stageTriggerError = v.TriggerSubmitTx(ctx, signedMessage, calculatedTxHash)
	case InFlightTxStageStatusUpdate:
		log.L(ctx).Tracef("Transaction with ID %s, triggering status update", rsc.InMemoryTx.GetSignerNonce())
		v.stageTriggerError = v.TriggerStatusUpdate(ctx)
	default:
		log.L(ctx).Tracef("Transaction with ID %s, didn't trigger any action for new stage: %s", rsc.InMemoryTx.GetSignerNonce(), stage)
	}
}

func (v *inFlightTransactionStateGeneration) ClearRunningStageContext(ctx context.Context) {
	if v.runningStageContext != nil {
		rsc := v.runningStageContext
		log.L(ctx).Debugf("Transaction with ID %s clearing stage context for stage: %s after %s, total time spent on this stage so far: %s, txHash: %s", rsc.InMemoryTx.GetSignerNonce(), rsc.Stage, time.Since(rsc.StageStartTime), time.Since(v.txLevelStageStartTime), rsc.InMemoryTx.GetTransactionHash())
	} else {
		log.L(ctx).Warnf("Transaction with ID %s  has no running stage context to clear", v.GetSignerNonce())
	}
	v.runningStageContext = nil
	v.stageTriggerError = nil
}

func (v *inFlightTransactionStateGeneration) AddPersistenceOutput(ctx context.Context, stage InFlightTxStage, persistenceTime time.Time, err error) {
	start := time.Now()
	v.AddStageOutputs(ctx, &StageOutput{
		Stage: stage,
		PersistenceOutput: &PersistenceOutput{
			PersistenceError: err,
			Time:             persistenceTime,
		},
	})
	log.L(ctx).Debugf("%s AddPersistenceOutput took %s to write the result", v.GetSignerNonce(), time.Since(start))
}

func (v *inFlightTransactionStateGeneration) AddSubmitOutput(ctx context.Context, txHash *pldtypes.Bytes32, submissionTime *pldtypes.Timestamp, submissionOutcome SubmissionOutcome, errorReason ethclient.ErrorReason, err error) {
	start := time.Now()
	log.L(ctx).Debugf("%s Setting submit output, submissionOutcome: %s, errReason: %s, err %+v", v.GetSignerNonce(), submissionOutcome, errorReason, err)
	v.AddStageOutputs(ctx, &StageOutput{
		Stage: InFlightTxStageSubmitting,
		SubmitOutput: &SubmitOutputs{
			SubmissionTime:    submissionTime,
			SubmissionOutcome: submissionOutcome,
			TxHash:            txHash,
			ErrorReason:       string(errorReason),
			Err:               err,
		},
	})
	log.L(ctx).Debugf("%s AddSubmitOutput took %s to write the result", v.GetSignerNonce(), time.Since(start))
}

func (v *inFlightTransactionStateGeneration) ProcessStageOutputs(ctx context.Context, processFunction func(stageOutputs []*StageOutput) (unprocessedStageOutputs []*StageOutput)) {
	v.bufferedStageOutputsMux.Lock()
	defer v.bufferedStageOutputsMux.Unlock()
	v.bufferedStageOutputs = processFunction(v.bufferedStageOutputs)
}

func (v *inFlightTransactionStateGeneration) AddStageOutputs(ctx context.Context, stageOutput *StageOutput) {
	if v.testOnlyNoEventMode {
		return
	}
	v.bufferedStageOutputsMux.Lock()
	defer v.bufferedStageOutputsMux.Unlock()
	v.bufferedStageOutputs = append(v.bufferedStageOutputs, stageOutput)
}

func (v *inFlightTransactionStateGeneration) AddSignOutput(ctx context.Context, signedMessage []byte, txHash *pldtypes.Bytes32, err error) {
	start := time.Now()
	log.L(ctx).Debugf("%s Setting signed message, hash %s, signed message not nil %t, err %+v", v.GetSignerNonce(), txHash, signedMessage != nil, err)
	v.AddStageOutputs(ctx, &StageOutput{
		Stage: InFlightTxStageSigning,
		SignOutput: &SignOutputs{
			SignedMessage: signedMessage,
			TxHash:        txHash,
			Err:           err,
		},
	})
	log.L(ctx).Debugf("%s AddSignOutput took %s to write the result", v.GetSignerNonce(), time.Since(start))
}

func (v *inFlightTransactionStateGeneration) AddGasPriceOutput(ctx context.Context, gasPriceObject *pldapi.PublicTxGasPricing, err error) {
	start := time.Now()
	v.AddStageOutputs(ctx, &StageOutput{
		Stage: InFlightTxStageRetrieveGasPrice,
		GasPriceOutput: &GasPriceOutput{
			GasPriceObject: gasPriceObject,
			Err:            err,
		},
	})
	log.L(ctx).Debugf("%s AddGasPriceOutput took %s to write the result", v.GetSignerNonce(), time.Since(start))
}

func (v *inFlightTransactionStateGeneration) AddConfirmationsOutput(ctx context.Context, confirmedTx *pldapi.IndexedTransaction) {
	panic("unused")
	// start := time.Now()
	// v.AddStageOutputs(ctx, &StageOutput{
	// 	Stage:              InFlightTxStageConfirming,
	// 	ConfirmationOutput: &ConfirmationOutputs{},
	// })
	// log.L(ctx).Debugf("%s AddConfirmationsOutput took %s to write the result", v.InMemoryTxStateManager.GetSignerNonce(), time.Since(start))
}

func (v *inFlightTransactionStateGeneration) AddPanicOutput(ctx context.Context, stage InFlightTxStage) {
	start := time.Now()
	// unexpected error, set an empty input for the stage
	// so that the stage handler will handle this as unexpected error
	v.AddStageOutputs(ctx, &StageOutput{
		Stage: stage,
	})
	log.L(ctx).Debugf("%s AddPanicOutput took %s to write the result", v.GetSignerNonce(), time.Since(start))
}

func (v *inFlightTransactionStateGeneration) PersistTxState(ctx context.Context) (stage InFlightTxStage, persistenceTime time.Time, err error) {
	rsc := v.runningStageContext
	if rsc == nil || rsc.StageOutputsToBePersisted == nil {
		log.L(ctx).Error("Cannot persist transaction state, no running context or stageOutputsToBePersisted")
		return v.stage, time.Now(), i18n.NewError(ctx, msgs.MsgPersistError)
	}

	// flush any sub-status changes
	for _, subStatusUpdate := range rsc.StageOutputsToBePersisted.StatusUpdates {
		if err := subStatusUpdate(v.statusUpdater); err != nil {
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
			op := v.submissionWriter.Queue(ctx, newSubmission)
			_, err := op.WaitFlushed(ctx)
			if err != nil {
				return rsc.Stage, time.Now(), err
			}
		}

		if rsc.StageOutputsToBePersisted.TxUpdates.InFlightStatus != nil &&
			*rsc.StageOutputsToBePersisted.TxUpdates.InFlightStatus == InFlightStatusConfirmReceived {
			v.RecordCompletedTransactionCountMetrics(ctx, string(GenericStatusSuccess))
		}

		// update the in memory state
		v.ApplyInMemoryUpdates(ctx, rsc.StageOutputsToBePersisted.TxUpdates)
	}
	return rsc.Stage, time.Now(), nil
}
