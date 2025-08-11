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
	"encoding/json"
	"fmt"
	"math/big"
	"runtime/debug"
	"sync"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/ethclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

type InFlightStatus int

const (
	InFlightStatusPending         InFlightStatus = iota
	InFlightStatusSuspending      InFlightStatus = iota
	InFlightStatusConfirmReceived InFlightStatus = iota
)

func (ifs InFlightStatus) String() string {
	switch ifs {
	case InFlightStatusSuspending:
		return "suspending"
	case InFlightStatusConfirmReceived:
		return "confirm_received"
	default:
		return "pending"
	}
}

type inFlightTransactionStageController struct {
	testOnlyNoActionMode bool // Note: this flag can never be set in normal code path, exposed for testing only
	testOnlyNoEventMode  bool // Note: this flag can never be set in normal code path, exposed for testing only

	// a reference to the transaction orchestrator
	*orchestrator
	txInflightTime time.Time
	txInDBTime     time.Time
	txTimeline     []PointOfTime

	// this transaction mutex is used for transaction inflight stage context control
	transactionMux sync.Mutex

	stateManager InFlightTransactionStateManager

	newStatus *InFlightStatus

	updates   []*DBPublicTxn
	updateMux sync.Mutex

	// deleteRequested bool // figure out what's the reliable approach for deletion
}

type PointOfTime struct {
	name          string
	timestamp     time.Time
	tillNextEvent time.Duration
}

type GenericStatus string

const (
	GenericStatusSuccess  GenericStatus = "success"
	GenericStatusFail     GenericStatus = "fail"
	GenericStatusConflict GenericStatus = "conflict"
	GenericStatusTimeOut  GenericStatus = "timeout"
)

type InFlightTxOperation string

const (
	InFlightTxOperationSign                InFlightTxOperation = "sign"
	InFlightTxOperationTransferPreparation InFlightTxOperation = "transfer_prep"
	InFlightTxOperationInvokePreparation   InFlightTxOperation = "invoke_prep"
	InFlightTxOperationDeployPreparation   InFlightTxOperation = "deploy_prep"
	InFlightTxOperationTransactionSend     InFlightTxOperation = "send"
)

type BasicActionInfo struct {
	// we rely on a successful submit action to link the correlation id and transaction hash together
	// CorrelationID string `json:"correlationId,omitempty"` // Talked with Peter, based on current user requirement, this is not necessary. An ID that is used to group actions into different instances of sub status when a transaction hash is not available
	TxHash string `json:"txHash,omitempty"` // transaction hash that is used to group actions into different instances of sub status
	Output string `json:"output"`
}

type BasicActionError struct {
	// we rely on a successful submit action to link the correlation id and transaction hash together
	// CorrelationID string `json:"correlationId,omitempty"` // Talked with Peter, based on current user requirement, this is not necessary. An ID that is used to group actions into different instances of sub status when a transaction hash is not available
	TxHash       string `json:"txHash,omitempty"` // transaction hash that is used to group actions into different instances of sub status
	ErrorMessage string `json:"errorMsg"`
}

func NewInFlightTransactionStageController(
	enth *pubTxManager,
	oc *orchestrator,
	ptx *DBPublicTxn,
) *inFlightTransactionStageController {
	var txTimeline []PointOfTime
	if oc.timeLineLoggingMaxEntries > 0 {
		txTimeline = make([]PointOfTime, 0, oc.timeLineLoggingMaxEntries)
		txTimeline = append(txTimeline, PointOfTime{
			name:      "wait_in_db",
			timestamp: ptx.Created.Time(),
		})
	}

	ift := &inFlightTransactionStageController{
		orchestrator:   oc,
		txInflightTime: time.Now(),
		txInDBTime:     ptx.Created.Time(),
		txTimeline:     txTimeline,
	}

	ift.MarkTime("wait_in_inflight_queue")
	imtxs := NewInMemoryTxStateManager(enth.ctx, ptx)
	ift.stateManager = NewInFlightTransactionStateManager(enth.thMetrics, enth.balanceManager, ift, imtxs, oc, oc.submissionWriter, ift.testOnlyNoEventMode)
	return ift
}

func (it *inFlightTransactionStageController) UpdateTransaction(ctx context.Context, newPtx *DBPublicTxn) {
	it.updateMux.Lock()
	defer it.updateMux.Unlock()
	it.updates = append(it.updates, newPtx)
}

func (it *inFlightTransactionStageController) MarkTime(eventName string) {
	if it.timeLineLoggingMaxEntries > 0 {
		it.txTimeline[len(it.txTimeline)-1].tillNextEvent = time.Since(it.txTimeline[len(it.txTimeline)-1].timestamp)
		if len(it.txTimeline) == it.timeLineLoggingMaxEntries {
			// the array is full, we need to print the timeline and reset it
			it.PrintTimeline()
			it.txTimeline = make([]PointOfTime, 0, it.timeLineLoggingMaxEntries)
		}
		it.txTimeline = append(it.txTimeline, PointOfTime{
			name:      eventName,
			timestamp: time.Now(),
		})
	}
}

func (it *inFlightTransactionStageController) PrintTimeline() string {
	ptString := ""
	if it.timeLineLoggingMaxEntries > 0 {
		for index, tl := range it.txTimeline {
			if index == len(it.txTimeline)-1 {
				tl.tillNextEvent = time.Since(tl.timestamp)
			}
			ptString = fmt.Sprintf("%s -> %s", ptString, tl.String())
		}
		ptString = fmt.Sprintf("%s -> Event: printed_timeline, at: %s", ptString, time.Now().Format(time.RFC3339Nano))
	}
	return ptString
}

func (pot *PointOfTime) String() string {
	return fmt.Sprintf("Event: %s, start: %s, duration: %s", pot.name, pot.timestamp.Format(time.RFC3339Nano), pot.tillNextEvent.String())
}
func (it *inFlightTransactionStageController) TriggerNewStageRun(ctx context.Context, stage InFlightTxStage, substatus BaseTxSubStatus) {
	it.MarkTime(fmt.Sprintf("stage_%s_wait_to_trigger_async_execution", string(stage)))
	it.stateManager.GetCurrentGeneration(ctx).StartNewStageContext(ctx, stage, substatus)
}

// ProduceLatestInFlightStageContext produce a in-flight stage context that is passed over to the stage process logic, it provides the following logic:
//   - a locking mechanism to ensure each in-flight transaction only have 1 in-flight stage context at a given time
//   - check and complete existing stage context when criteria is met
//   - produce new stage context when the criteria is met
func (it *inFlightTransactionStageController) ProduceLatestInFlightStageContext(ctx context.Context, tIn *OrchestratorContext) (tOut *TriggerNextStageOutput) {
	tOut = &TriggerNextStageOutput{}
	log.L(ctx).Debugf("ProduceLatestInFlightStageContext entry for tx %s", it.stateManager.GetSignerNonce())
	// Take a snapshot of the pending state under the lock
	it.transactionMux.Lock()
	defer it.transactionMux.Unlock()

	it.updateMux.Lock()
	updates := it.updates
	it.updates = nil
	it.updateMux.Unlock()

	madeUpdate := false
	if len(updates) > 0 {
		// Process each update in order. If there are multiple updates they will all be recorded in the database, but only the
		// last one will be acted on
		for _, update := range updates {
			it.stateManager.UpdateTransaction(update)
			madeUpdate = true
		}
	}

	if madeUpdate {
		// If we have made an update we don't wait to collect the output of whatever stages might be already running before starting
		// the process of submitting the transaction with its new values.
		it.stateManager.NewGeneration(ctx)
	}

	// update the transaction orchestrator context
	it.stateManager.SetOrchestratorContext(ctx, tIn)

	tOut.Error = it.processCurrentGenerationStageOutputs(ctx)

	if it.stateManager.GetGasPriceObject() != nil {
		if it.stateManager.IsReadyToExit() {
			// already has confirmed transaction so the cost to submit this transaction is zero
			tOut.Cost = big.NewInt(0)
		} else {
			gpo := it.stateManager.GetGasPriceObject()
			c, err := calculateGasRequiredForTransaction(ctx, gpo, it.stateManager.GetGasLimit())
			if err == nil {
				tOut.Cost = c
			}
		}
	}

	// Only the current generation is progressed by starting a new stage
	if it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx) == nil {
		// no running context in flight
		// The action for each stage can be started asynchronously; however, any transation values from the in memory transaction must
		// be read within this goroutine so that we know that they haven't been changed by an update part way through.
		it.startNewStage(ctx, tOut.Cost)
	}
	tOut.TransactionSubmitted = it.stateManager.GetTransactionHash() != nil

	return tOut
}

func (it *inFlightTransactionStageController) processCurrentGenerationStageOutputs(ctx context.Context) (err error) {
	currentGeneration := it.stateManager.GetCurrentGeneration(ctx)
	if currentGeneration.GetRunningStageContext(ctx) != nil {
		rsc := currentGeneration.GetRunningStageContext(ctx)
		log.L(ctx).Debugf("ProduceLatestInFlightStageContext for tx %s, on stage: %s , current stage context lived: %s , stage lived: %s, last stage error: %+v", it.stateManager.GetSignerNonce(), currentGeneration.GetStage(ctx), time.Since(rsc.StageStartTime), time.Since(currentGeneration.GetStageStartTime(ctx)), currentGeneration.GetStageTriggerError(ctx))
		if currentGeneration.GetStageTriggerError(ctx) != nil {
			log.L(ctx).Errorf("Failed to trigger stage due to %+v, cleaning up the context and retry", currentGeneration.GetStageTriggerError(ctx))
			currentGeneration.ClearRunningStageContext(ctx)
		} else {
			currentGeneration.ProcessStageOutputs(ctx, func(stageOutputs []*StageOutput) (unprocessedStageOutputs []*StageOutput) {
				unprocessedStageOutputs = make([]*StageOutput, 0)
				log.L(ctx).Debugf("ProduceLatestInFlightStageContext for tx %s, has %d inputs", it.stateManager.GetSignerNonce(), len(stageOutputs))
				for _, stageOutput := range stageOutputs {
					if stageOutput.Stage == rsc.Stage {
						// First check whether there are errors persisting. In this case we just want to try again after the timeout and
						// don't need to look any closer at what the output state is
						if stageOutput.PersistenceOutput != nil && stageOutput.PersistenceOutput.PersistenceError != nil {
							if time.Since(stageOutput.PersistenceOutput.Time) > it.persistenceRetryTimeout {
								// retry persistence
								_ = it.TriggerPersistTxState(ctx)
							} else {
								// wait for retry timeout
								unprocessedStageOutputs = append(unprocessedStageOutputs, stageOutput)
							}
						} else {
							switch stageOutput.Stage {
							case InFlightTxStageRetrieveGasPrice:
								err = it.processRetrieveGasPriceStageOutput(ctx, currentGeneration, rsc, stageOutput)
							case InFlightTxStageSigning:
								err = it.processSigningStageOutput(ctx, currentGeneration, rsc, stageOutput)
							case InFlightTxStageSubmitting:
								err = it.processSubmittingStageOutput(ctx, currentGeneration, rsc, stageOutput)
							case InFlightTxStageStatusUpdate:
								err = it.processStatusUpdateStageOutput(ctx, currentGeneration, rsc, stageOutput)
							}
						}
					} else {
						log.L(ctx).Tracef("Current stage: %s, received inputs for future stage %s for transaction with ID: %s", rsc.Stage, stageOutput.Stage, rsc.InMemoryTx.GetSignerNonce())
						unprocessedStageOutputs = append(unprocessedStageOutputs, stageOutput)
					}
				}
				log.L(ctx).Debugf("ProduceLatestInFlightStageContext for tx %s, %d inputs is carrying on", rsc.InMemoryTx.GetSignerNonce(), len(unprocessedStageOutputs))
				return unprocessedStageOutputs
			})

			if rsc.StageErrored && time.Since(rsc.StageStartTime) > it.stageRetryTimeout {
				// if the stage didn't succeed, we retry the stage after the stage timeout
				log.L(ctx).Debugf("Retrying stage: %s, for transaction with ID: %s after %s", rsc.Stage, rsc.InMemoryTx.GetSignerNonce(), time.Since(rsc.StageStartTime))
				currentGeneration.ClearRunningStageContext(ctx)
			}
		}
	}
	return
}

func (it *inFlightTransactionStageController) processRetrieveGasPriceStageOutput(ctx context.Context, generation InFlightTransactionStateGeneration, rsc *RunningStageContext, stageOutput *StageOutput) (err error) {
	// first check whether we've already completed the action and just waiting for required persistence to go to the next stage
	if stageOutput.PersistenceOutput != nil {
		if rsc.StageOutput.GasPriceOutput.Err != nil {
			rsc.StageErrored = true
		}
		if stageOutput.PersistenceOutput.PersistenceError == nil && !rsc.StageErrored {
			// new gas price retrieved, the state no longer matches the transaction hash
			generation.SetValidatedTransactionHashMatchState(ctx, false)
			// we've persisted successfully, it's safe to move to the next stage based on the latest state of the managed transaction
			generation.ClearRunningStageContext(ctx)
		}
	} else if stageOutput.GasPriceOutput == nil {
		log.L(ctx).Errorf("gasPriceOutput should not be nil for transaction with ID: %s, in the stage output object: %+v.", rsc.InMemoryTx.GetSignerNonce(), stageOutput)
		err = i18n.NewError(ctx, msgs.MsgInvalidStageOutput, "gasPriceOutput", stageOutput)
		// unexpected error, reset the running stage context so that it gets retried if the current generation
		generation.ClearRunningStageContext(ctx)
	} else {
		rsc.StageOutput.GasPriceOutput = stageOutput.GasPriceOutput
		// gas price received, trigger persistence
		rsc.SetNewPersistenceUpdateOutput()
		if stageOutput.GasPriceOutput.Err != nil {
			// if failed to get gas price, persist the error
			rsc.StageOutputsToBePersisted.UpdateSubStatus(BaseTxActionRetrieveGasPrice, nil, pldtypes.RawJSON(`{"error":"`+stageOutput.GasPriceOutput.Err.Error()+`"}`))
		} else {
			gpo := it.calculateNewGasPrice(ctx, rsc.InMemoryTx.GetGasPriceObject(), stageOutput.GasPriceOutput.GasPriceObject)
			gpoJSON, _ := json.Marshal(gpo)
			rsc.StageOutputsToBePersisted.TxUpdates = &BaseTXUpdates{GasPricing: gpo}
			rsc.StageOutputsToBePersisted.UpdateSubStatus(BaseTxActionRetrieveGasPrice, pldtypes.RawJSON(gpoJSON), nil)
		}
		_ = it.TriggerPersistTxState(ctx)
	}
	return
}

func (it *inFlightTransactionStageController) processSigningStageOutput(ctx context.Context, generation InFlightTransactionStateGeneration, rsc *RunningStageContext, rsIn *StageOutput) (err error) {
	// first check whether we've already completed the action and just waiting for required persistence to go to the next stage
	if rsIn.PersistenceOutput != nil {
		if rsc.StageOutput.SignOutput.Err != nil {
			// wait for the stale transaction timeout to re-trigger the signing provided this is the current generation
			rsc.StageErrored = true
		}
		if rsIn.PersistenceOutput.PersistenceError == nil && !rsc.StageErrored {
			// we've persisted successfully, move to the next stage inline as signed message is not persisted
			log.L(ctx).Debugf("Signed message is not nil: %t", rsc.StageOutput.SignOutput.SignedMessage != nil)
			generation.SetTransientPreviousStageOutputs(&TransientPreviousStageOutputs{
				SignedMessage:   rsc.StageOutput.SignOutput.SignedMessage,
				TransactionHash: rsc.StageOutput.SignOutput.TxHash,
			})
			it.TriggerNewStageRun(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived)
		}
	} else if rsIn.SignOutput == nil {
		log.L(ctx).Errorf("signOutput should not be nil for transaction with ID: %s, in the stage output object: %+v.", rsc.InMemoryTx.GetSignerNonce(), rsIn)
		err = i18n.NewError(ctx, msgs.MsgInvalidStageOutput, "signOutput", rsIn)
		// unexpected error, reset the running stage context so that it can be retried if this is the current generation
		generation.ClearRunningStageContext(ctx)
	} else {
		rsc.StageOutput.SignOutput = rsIn.SignOutput

		rsc.SetNewPersistenceUpdateOutput()
		if rsIn.SignOutput.Err != nil {
			// persist the error
			log.L(ctx).Errorf("Transaction signing failed for transaction with ID: %s, due to error: %+v", rsc.InMemoryTx.GetSignerNonce(), rsIn.SignOutput.Err)
			rsc.StageOutputsToBePersisted.UpdateSubStatus(BaseTxActionSign, nil, pldtypes.RawJSON(`{"error":"`+rsIn.SignOutput.Err.Error()+`"}`))
		} else {
			log.L(ctx).Tracef("SignOutput %+v", rsIn.SignOutput)
			// signed data received
			if rsIn.SignOutput.SignedMessage != nil {
				// signed message can be nil when no signer is configured
				rsc.StageOutputsToBePersisted.UpdateSubStatus(BaseTxActionSign, pldtypes.RawJSON(fmt.Sprintf(`{"hash":"%s"}`, rsIn.SignOutput.TxHash)), nil)
			}
		}

		// Very important that we persist the transaction after SIGNING (and before SUBMISSION)
		// as once submitted we will only be able to match back up if we can recover our TX by
		// hash from our submission records.
		if rsc.StageOutput.SignOutput.TxHash != nil {
			// we add the tx hash in to the submitted transaction array
			// the persistence logic will add it to the submitted hashes tracking array if it's new
			if rsc.StageOutputsToBePersisted.TxUpdates == nil {
				rsc.StageOutputsToBePersisted.TxUpdates = &BaseTXUpdates{}
			}
			rsc.InMemoryTx.GetGasPriceObject()
			gasPriceJSON, _ := json.Marshal(rsc.InMemoryTx.GetGasPriceObject())
			rsc.StageOutputsToBePersisted.TxUpdates.NewSubmission = &DBPubTxnSubmission{
				from:            rsc.InMemoryTx.GetFrom().String(),
				PublicTxnID:     rsc.InMemoryTx.GetPubTxnID(),
				Created:         pldtypes.TimestampNow(),
				TransactionHash: *rsc.StageOutput.SignOutput.TxHash,
				GasPricing:      gasPriceJSON,
			}
			rsc.StageOutputsToBePersisted.TxUpdates.TransactionHash = rsc.StageOutput.SignOutput.TxHash
		}

		_ = it.TriggerPersistTxState(ctx)
	}
	return
}

func (it *inFlightTransactionStageController) processSubmittingStageOutput(ctx context.Context, generation InFlightTransactionStateGeneration, rsc *RunningStageContext, stageOutput *StageOutput) (err error) {
	// first check whether we've already completed the action and just waiting for required persistence to go to the next stage
	if stageOutput.PersistenceOutput != nil {
		if rsc.StageOutput.SubmitOutput.Err != nil {
			if rsc.StageOutput.SubmitOutput.ErrorReason == string(ethclient.ErrorReasonInsufficientFunds) {
				it.balanceManager.NotifyAddressBalanceChanged(ctx, it.signingAddress)
			}
			// wait for the stale transaction timeout to re-trigger the submission provided this is the current generation
			rsc.StageErrored = true
		} else if stageOutput.PersistenceOutput.PersistenceError == nil {
			// we've persisted successfully, it's safe to move to the next stage based on the latest state of the managed transaction
			generation.SetValidatedTransactionHashMatchState(ctx, true)
			generation.ClearRunningStageContext(ctx)
		}
	} else if stageOutput.SubmitOutput == nil {
		log.L(ctx).Errorf("submitOutput should not be nil for transaction with ID: %s, in the stage output object: %+v.", rsc.InMemoryTx.GetSignerNonce(), stageOutput)
		err = i18n.NewError(ctx, msgs.MsgInvalidStageOutput, "submitOutput", stageOutput)
		// unexpected error, reset the running stage context so that it gets retried if the current generation
		generation.ClearRunningStageContext(ctx)
	} else {
		rsc.StageOutput.SubmitOutput = stageOutput.SubmitOutput
		// transaction submitted
		rsc.SetNewPersistenceUpdateOutput()
		if stageOutput.SubmitOutput.Err != nil {
			log.L(ctx).Errorf("Submitting transaction error for transaction %s: %+v", rsc.InMemoryTx.GetSignerNonce(), stageOutput.SubmitOutput.Err)
			errMsg := stageOutput.SubmitOutput.Err.Error()
			rsc.StageOutputsToBePersisted.TxUpdates = &BaseTXUpdates{
				ErrorMessage: &errMsg,
			}
			rsc.StageOutputsToBePersisted.UpdateSubStatus(BaseTxActionSubmitTransaction, pldtypes.RawJSON(`{"reason":"`+string(stageOutput.SubmitOutput.ErrorReason)+`"}`), pldtypes.RawJSON(`{"error":"`+stageOutput.SubmitOutput.Err.Error()+`"}`))
			// TODO: this should be set from the signing stage- it doesn't tell us anything about whether this is a resubmission or not
			if rsc.InMemoryTx.GetTransactionHash() != nil {
				// did a re-submission, no matter the result, update the last warn time to avoid another retry
				rsc.StageOutputsToBePersisted.TxUpdates.LastSubmit = confutil.P(pldtypes.TimestampNow())
			}
		} else {
			if rsc.StageOutputsToBePersisted.TxUpdates == nil {
				rsc.StageOutputsToBePersisted.TxUpdates = &BaseTXUpdates{}
			}
			rsc.StageOutputsToBePersisted.TxUpdates.LastSubmit = stageOutput.SubmitOutput.SubmissionTime

			switch stageOutput.SubmitOutput.SubmissionOutcome {
			case SubmissionOutcomeSubmittedNew:
				// new transaction submitted successfully
				rsc.StageOutputsToBePersisted.UpdateSubStatus(BaseTxActionSubmitTransaction, pldtypes.RawJSON(fmt.Sprintf(`{"hash":"%s"}`, stageOutput.SubmitOutput.TxHash)), nil)
				log.L(ctx).Debugf("Transaction submitted for tx %s (hash=%s)", rsc.InMemoryTx.GetSignerNonce(), rsc.InMemoryTx.GetTransactionHash())
			case SubmissionOutcomeNonceTooLow:
				log.L(ctx).Debugf("Nonce too low for tx %s (hash=%s)", rsc.InMemoryTx.GetSignerNonce(), rsc.InMemoryTx.GetTransactionHash())
				rsc.StageOutputsToBePersisted.UpdateSubStatus(BaseTxActionSubmitTransaction, pldtypes.RawJSON(`{"txHash":"`+stageOutput.SubmitOutput.TxHash.String()+`"}`), nil)
			case SubmissionOutcomeAlreadyKnown:
				// nothing to add for persistence, go to the tracking stage
				log.L(ctx).Debugf("Transaction already known for tx %s (hash=%s)", rsc.InMemoryTx.GetSignerNonce(), rsc.InMemoryTx.GetTransactionHash())
			}

			// did the first submit
			if rsc.InMemoryTx.GetFirstSubmit() == nil {
				log.L(ctx).Debugf("Recorded the first submission for transaction %s", rsc.InMemoryTx.GetSignerNonce())
				rsc.StageOutputsToBePersisted.TxUpdates.FirstSubmit = stageOutput.SubmitOutput.SubmissionTime
			}

			if rsc.InMemoryTx.GetTransactionHash() == nil {
				log.L(ctx).Debugf("Recorded the tx hash %s for transaction %s", rsc.StageOutput.SubmitOutput.TxHash, rsc.InMemoryTx.GetSignerNonce())
				rsc.StageOutputsToBePersisted.TxUpdates.TransactionHash = rsc.StageOutput.SubmitOutput.TxHash
			}
		}

		_ = it.TriggerPersistTxState(ctx)
	}
	return
}

func (it *inFlightTransactionStageController) processStatusUpdateStageOutput(ctx context.Context, generation InFlightTransactionStateGeneration, rsc *RunningStageContext, stageOutput *StageOutput) (err error) {
	// only requires persistence output for this stage
	if stageOutput.PersistenceOutput != nil {
		// we've persisted successfully, check the status and clean up the newStatus if we successfully switched to the latest desired status
		if *it.newStatus == it.stateManager.GetInFlightStatus() {
			log.L(ctx).Debugf("Transaction with ID %s reached desired new status: %s", it.stateManager.GetSignerNonce(), it.stateManager.GetInFlightStatus())
			// already has the lock
			it.newStatus = nil
		}
		generation.ClearRunningStageContext(ctx)
		// Need to go back round again to clear this inflight out completely
		it.MarkInFlightTxStale()
	} else {
		log.L(ctx).Errorf("persistenceOutput should not be nil for transaction with ID: %s, in the stage output object: %+v.", rsc.InMemoryTx.GetSignerNonce(), stageOutput)
		err = i18n.NewError(ctx, msgs.MsgInvalidStageOutput, "persistenceOutput", stageOutput)
		// unexpected error, reset the running stage context so that it gets retried
		generation.ClearRunningStageContext(ctx)
	}
	return
}

func (it *inFlightTransactionStageController) startNewStage(ctx context.Context, cost *big.Int) {
	// first check whether the current transaction is before the confirmed nonce
	if it.newStatus != nil && !it.stateManager.IsReadyToExit() && *it.newStatus != it.stateManager.GetInFlightStatus() { // first apply any status update that's required
		log.L(ctx).Debugf("Transaction with ID %s entering status update, current status: %s, target status: %s", it.stateManager.GetSignerNonce(), it.stateManager.GetInFlightStatus(), *it.newStatus)
		it.TriggerNewStageRun(ctx, InFlightTxStageStatusUpdate, BaseTxSubStatusReceived)
	} else if it.stateManager.IsReadyToExit() {
		// then calculate the latest stage based on the managed transaction to kick off the next stage
		// if there isn't any running context and the transaction status is no longer in pending
		// we can wait for the transaction orchestrator to remove it from the in-flight transaction queue. It's either paused or completed
		log.L(ctx).Debugf("Transaction with ID %s is waiting for removal in status: %s.", it.stateManager.GetSignerNonce(), it.stateManager.GetInFlightStatus())
	} else if it.stateManager.GetGasPriceObject() == nil {
		// no gas price fetched, go and fetch gas price
		log.L(ctx).Debugf("Transaction with ID %s entering retrieve gas price as no gas price available.", it.stateManager.GetSignerNonce())
		it.TriggerNewStageRun(ctx, InFlightTxStageRetrieveGasPrice, BaseTxSubStatusReceived)
	} else if it.stateManager.GetTransactionHash() == nil {
		if it.stateManager.CanSubmit(ctx, cost) {
			// no transaction hash, do signing and submission
			log.L(ctx).Debugf("Transaction with ID %s entering signing stage as no transaction hash recorded.", it.stateManager.GetSignerNonce())
			it.TriggerNewStageRun(ctx, InFlightTxStageSigning, BaseTxSubStatusReceived)
		} else {
			log.L(ctx).Debugf("Transaction with ID %s no op, as cannot submit.", it.stateManager.GetSignerNonce())
		}
	} else {
		// we have a transaction hash recorded, we must ensure we check the hash matches
		// the state we persisted by triggering a submission
		if !it.stateManager.GetCurrentGeneration(ctx).ValidatedTransactionHashMatchState(ctx) {
			if it.stateManager.CanSubmit(ctx, cost) {
				log.L(ctx).Debugf("Transaction with ID %s entering signing stage as current state hasn't been validated.", it.stateManager.GetSignerNonce())
				it.TriggerNewStageRun(ctx, InFlightTxStageSigning, BaseTxSubStatusReceived)
			} else {
				log.L(ctx).Debugf("Transaction with ID %s no op, as cannot submit, state not validated.", it.stateManager.GetSignerNonce())
			}
		} else {
			// once we validated the transaction hash matched the transaction state
			lastSubmitTime := it.stateManager.GetLastSubmitTime()
			if lastSubmitTime != nil && time.Since(lastSubmitTime.Time()) > it.resubmitInterval {
				// do a resubmission when exceeded the resubmit interval
				log.L(ctx).Debugf("Transaction with ID %s entering retrieve gas price as exceeded resubmit interval of %s.", it.stateManager.GetSignerNonce(), it.resubmitInterval.String())
				it.TriggerNewStageRun(ctx, InFlightTxStageRetrieveGasPrice, BaseTxSubStatusStale)
			} else {
				// check and track the existing transaction hash
				// ... this is the "nil" stage
				log.L(ctx).Debugf("Transaction with ID %s entering tracking stage", it.stateManager.GetSignerNonce())
				it.stateManager.GetCurrentGeneration(ctx).ClearRunningStageContext(ctx)
			}
		}

	}
}

func (it *inFlightTransactionStageController) calculateNewGasPrice(ctx context.Context, existingGpo *pldapi.PublicTxGasPricing, newGpo *pldapi.PublicTxGasPricing) *pldapi.PublicTxGasPricing {
	if existingGpo == nil {
		log.L(ctx).Debugf("First time assigning gas price to transaction with ID: %s, gas price object: %+v.", it.stateManager.GetSignerNonce(), newGpo)
		return newGpo
	}

	// The change is not made here to InMemoryTx, but rather pushed to TxUpdates for persisting.
	// So we need to make sure we don't edit the in-memory existing object by passing it to calculateNewGasPrice

	if newGpo.GasPrice != nil && existingGpo.GasPrice != nil && existingGpo.GasPrice.Int().Cmp(newGpo.GasPrice.Int()) == 1 {
		// existing gas price already above the new gas price, increase using percentage
		newPercentage := big.NewInt(100)
		newPercentage = newPercentage.Add(newPercentage, big.NewInt(int64(it.gasPriceIncreasePercent)))
		newGasPrice := new(big.Int).Mul(existingGpo.GasPrice.Int(), newPercentage)
		newGasPrice = newGasPrice.Div(newGasPrice, big.NewInt(100))
		if it.gasPriceIncreaseMax != nil && newGasPrice.Cmp(it.gasPriceIncreaseMax) == 1 {
			newGasPrice.Set(it.gasPriceIncreaseMax)
		}
		newGpo = &pldapi.PublicTxGasPricing{
			GasPrice:             (*pldtypes.HexUint256)(newGasPrice),
			MaxFeePerGas:         existingGpo.MaxFeePerGas,         // copy over unchanged (although expected to be unset)
			MaxPriorityFeePerGas: existingGpo.MaxPriorityFeePerGas, //   "
		}
	} else if newGpo.MaxFeePerGas != nil && existingGpo.MaxFeePerGas != nil && existingGpo.MaxFeePerGas.Int().Cmp(newGpo.MaxFeePerGas.Int()) == 1 {
		// existing MaxFeePerGas already above the new MaxFeePerGas, increase using percentage
		newPercentage := big.NewInt(100)

		newPercentage = newPercentage.Add(newPercentage, big.NewInt(int64(it.gasPriceIncreasePercent)))
		newMaxFeePerGas := new(big.Int).Mul(existingGpo.MaxFeePerGas.Int(), newPercentage)
		newMaxFeePerGas = newMaxFeePerGas.Div(newMaxFeePerGas, big.NewInt(100))
		if it.gasPriceIncreaseMax != nil && newMaxFeePerGas.Cmp(it.gasPriceIncreaseMax) == 1 {
			newMaxFeePerGas.Set(it.gasPriceIncreaseMax)
		}
		newGpo = &pldapi.PublicTxGasPricing{
			GasPrice:             existingGpo.GasPrice, // copy over unchanged (although expected to be unset)
			MaxFeePerGas:         (*pldtypes.HexUint256)(newMaxFeePerGas),
			MaxPriorityFeePerGas: existingGpo.MaxPriorityFeePerGas,
		}
	}

	return newGpo
}

func calculateGasRequiredForTransaction(ctx context.Context, gpo *pldapi.PublicTxGasPricing, gasLimit uint64) (gasRequired *big.Int, err error) {
	if gpo.GasPrice != nil {
		log.L(ctx).Debugf("gas calculation using GasPrice (%+v)", gpo.GasPrice)
		gasRequired = new(big.Int).Mul(gpo.GasPrice.Int(), new(big.Int).SetUint64(gasLimit))
	} else if gpo.MaxFeePerGas != nil && gpo.MaxPriorityFeePerGas != nil {
		// max-fee and max-priority-fee have been provided. We can only use
		// max-fee to calculate how much this TX could cost, but we ultimately
		// require both to be set
		log.L(ctx).Debugf("gas calculation using MaxFeePerGas (%v)", gpo.MaxFeePerGas)
		maxFeePerGasCopy := new(big.Int).Set(gpo.MaxFeePerGas.Int())
		gasRequired = maxFeePerGasCopy.Mul(maxFeePerGasCopy, new(big.Int).SetUint64(gasLimit))
	}
	return gasRequired, nil

}

func (it *inFlightTransactionStageController) NotifyStatusUpdate(ctx context.Context, status InFlightStatus) (updateRequired bool, err error) {
	if it.stateManager.IsReadyToExit() {
		if it.stateManager.GetInFlightStatus() == InFlightStatusSuspending && status == InFlightStatusPending {
			log.L(ctx).Debugf("Resume of transaction %s before suspend completed", it.stateManager.GetSignerNonce())
		} else {
			// cannot update status of a completed transaction, return error
			return false, i18n.NewError(ctx, msgs.MsgStatusUpdateForbidden)
		}
	}
	// queue the status to be updated in future evaluation loops
	it.transactionMux.Lock() // acquire a lock here to prevent overrides from existing status update
	defer it.transactionMux.Unlock()
	it.newStatus = &status
	return true, nil
}

// For each of these "trigger" functions, if the asynchronous part requires values from the managed transaction or the generation,
// they should be read and passed in to the function in the goroutine as arguments. This is to ensure that these values are only
// ever read/set from the main orchestrator polling thread which has the benefit that we don't then have to worry about mutexes
// and synchronisation

func (it *inFlightTransactionStageController) TriggerRetrieveGasPrice(ctx context.Context) error {
	generation := it.stateManager.GetCurrentGeneration(ctx)
	it.executeAsync(func() {
		gasPrice, err := it.gasPriceClient.GetGasPriceObject(ctx)
		generation.AddGasPriceOutput(ctx, gasPrice, err)
	}, ctx, generation, false)
	return nil
}

func (it *inFlightTransactionStageController) TriggerStatusUpdate(ctx context.Context) error {
	generation := it.stateManager.GetCurrentGeneration(ctx)
	it.executeAsync(func() {
		rsc := generation.GetRunningStageContext(ctx)
		rsc.SetNewPersistenceUpdateOutput()
		rsc.StageOutputsToBePersisted.UpdateSubStatus(BaseTxActionStateTransition, pldtypes.RawJSON(fmt.Sprintf(`{"status":"%s"}`, *it.newStatus)), nil)
		rsc.StageOutputsToBePersisted.TxUpdates = &BaseTXUpdates{
			InFlightStatus: it.newStatus,
		}
		stage, persistenceTime, err := generation.PersistTxState(ctx)
		generation.AddPersistenceOutput(ctx, stage, persistenceTime, err)
	}, ctx, generation, false)
	return nil
}

func (it *inFlightTransactionStageController) TriggerSignTx(ctx context.Context) error {
	generation := it.stateManager.GetCurrentGeneration(ctx)
	from := it.stateManager.GetFrom()
	ethTX := it.stateManager.BuildEthTX()
	it.executeAsync(func() {
		signedMessage, txHash, err := it.signTx(ctx, from, ethTX)
		log.L(ctx).Debugf("Adding signed message to output, hash %s, signedMessage not nil %t, err %+v", txHash, signedMessage != nil, err)
		generation.AddSignOutput(ctx, signedMessage, txHash, err)
	}, ctx, generation, false)
	return nil
}

func (it *inFlightTransactionStageController) TriggerSubmitTx(ctx context.Context, signedMessage []byte, calculatedHash *pldtypes.Bytes32) error {
	generation := it.stateManager.GetCurrentGeneration(ctx)
	signerNonce := it.stateManager.GetSignerNonce()
	lastSubmitTime := it.stateManager.GetLastSubmitTime()

	it.executeAsync(func() {
		txHash, submissionTime, errReason, submissionOutcome, err := it.submitTX(ctx, signedMessage, calculatedHash, signerNonce, lastSubmitTime, generation.IsCancelled)
		generation.AddSubmitOutput(ctx, txHash, submissionTime, submissionOutcome, errReason, err)
	}, ctx, generation, false)
	return nil
}

func (it *inFlightTransactionStageController) TriggerPersistTxState(ctx context.Context) error {
	generation := it.stateManager.GetCurrentGeneration(ctx)
	it.executeAsync(func() {
		stage, persistenceTime, err := generation.PersistTxState(ctx)
		generation.AddPersistenceOutput(ctx, stage, persistenceTime, err)
	}, ctx, generation, true)
	return nil
}

type TriggerNextStageOutput struct {
	Cost                 *big.Int
	TransactionSubmitted bool
	Error                error
}

func (it *inFlightTransactionStageController) executeAsync(funcToExecute func(), ctx context.Context, generation InFlightTransactionStateGeneration, isPersistence bool) {
	if it.testOnlyNoActionMode {
		return
	}
	go func() {
		stage := generation.GetStage(ctx)
		defer func() {
			if err := recover(); err != nil {
				// if the function panicked, catch it and write a panic error to the output queue
				log.L(ctx).Errorf("Panic error detected for transaction %s, when executing: %s, error: %+v", it.stateManager.GetSignerNonce(), stage, err)
				debug.PrintStack()
				generation.AddPanicOutput(ctx, stage)
			}
			// trigger another loop of in-flight orchestrator
			it.MarkInFlightTxStale()
		}()
		if isPersistence {
			it.MarkTime(fmt.Sprintf("stage_%s_async_persistence_execution", string(stage)))
		} else {
			it.MarkTime(fmt.Sprintf("stage_%s_async_action_execution", string(stage)))
		}
		funcToExecute() // in non-panic scenarios, this function will add output to the output queue
		if isPersistence {
			it.MarkTime(fmt.Sprintf("stage_%s_persistence_result_wait_to_be_processed", string(stage)))
		} else {
			it.MarkTime(fmt.Sprintf("stage_%s_action_result_wait_to_be_processed", string(stage)))
		}
	}()
}
