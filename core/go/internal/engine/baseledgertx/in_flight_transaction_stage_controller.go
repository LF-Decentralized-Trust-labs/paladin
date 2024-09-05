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
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	baseTypes "github.com/kaleido-io/paladin/core/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/sirupsen/logrus"
)

type InFlightTransactionStageController struct {
	testOnlyNoActionMode bool // Note: this flag can never be set in normal code path, exposed for testing only
	testOnlyNoEventMode  bool // Note: this flag can never be set in normal code path, exposed for testing only

	// a reference to the transaction orchestrator
	*orchestrator
	txInflightTime         time.Time
	txInDBTime             time.Time
	txTimeline             []PointOfTime
	timeLineLoggingEnabled bool

	// this transaction mutex is used for transaction inflight stage context control
	transactionMux sync.Mutex

	newStatus *baseTypes.BaseTxStatus

	stateManager baseTypes.InFlightTransactionStateManager

	// pauseRequested bool
	// deleteRequested bool // figure out what's the reliable approach for deletion
}

type PointOfTime struct {
	name          string
	timestamp     time.Time
	tillNextEvent time.Duration
}

type GenericStatus string

const (
	GenericStatusSuccess GenericStatus = "success"
	GenericStatusFail    GenericStatus = "fail"
	GenericStatusTimeOut GenericStatus = "timeout"
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
	enth *baseLedgerTxEngine,
	te *orchestrator,
	mtx *baseTypes.ManagedTX,
) *InFlightTransactionStageController {

	ift := &InFlightTransactionStageController{
		orchestrator:   te,
		txInflightTime: time.Now(),
		txInDBTime:     *mtx.Created.Time(),
		txTimeline: []PointOfTime{
			{
				name:      "wait_in_db",
				timestamp: *mtx.Created.Time(),
			},
		},
		timeLineLoggingEnabled: logrus.IsLevelEnabled(logrus.DebugLevel),
	}

	ift.MarkTime("wait_in_inflight_queue")
	ift.stateManager = NewInFlightTransactionStateManager(enth.thMetrics, enth.balanceManager, enth.txStore, enth.txConfirmationListener, ift, NewInMemoryTxStateMananger(enth.ctx, mtx), te.turnOffHistory, ift.testOnlyNoEventMode)
	return ift
}

func (it *InFlightTransactionStageController) MarkTime(eventName string) {
	if it.timeLineLoggingEnabled {
		it.txTimeline[len(it.txTimeline)-1].tillNextEvent = time.Since(it.txTimeline[len(it.txTimeline)-1].timestamp)
		it.txTimeline = append(it.txTimeline, PointOfTime{
			name:      eventName,
			timestamp: time.Now(),
		})
	}
}
func (it *InFlightTransactionStageController) MarkHistoricalTime(eventName string, t time.Time) {
	if it.timeLineLoggingEnabled {
		it.txTimeline[len(it.txTimeline)-1].tillNextEvent = t.Sub(it.txTimeline[len(it.txTimeline)-1].timestamp)
		it.txTimeline = append(it.txTimeline, PointOfTime{
			name:      eventName,
			timestamp: t,
		})
	}
}

func (it *InFlightTransactionStageController) PrintTimeline() string {
	ptString := ""
	if it.timeLineLoggingEnabled {
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
func (it *InFlightTransactionStageController) TriggerNewStageRun(ctx context.Context, stage baseTypes.InFlightTxStage, substatus baseTypes.BaseTxSubStatus, signedMessage []byte) {
	it.MarkTime(fmt.Sprintf("stage_%s_wait_to_trigger_async_execution", string(stage)))
	if signedMessage != nil {
		it.stateManager.SetTransientPreviousStageOutputs(&baseTypes.TransientPreviousStageOutputs{
			SignedMessage: signedMessage,
		})
	}
	it.stateManager.StartNewStageContext(ctx, stage, substatus)
}

// ProduceLatestInFlightStageContext produce a in-flight stage context that is passed over to the stage process logic, it provides the following logic:
//   - a locking mechanism to ensure each in-flight transaction only have 1 in-flight stage context at a given time
//   - check and complete existing stage context when criteria is met
//   - produce new stage context when the criteria is met
func (it *InFlightTransactionStageController) ProduceLatestInFlightStageContext(ctx context.Context, tIn *baseTypes.OrchestratorContext) (tOut *TriggerNextStageOutput) {
	tOut = &TriggerNextStageOutput{}
	log.L(ctx).Debugf("ProduceLatestInFlightStageContext entry for tx %s", it.stateManager.GetTxID())
	// Take a snapshot of the pending state under the lock
	it.transactionMux.Lock()
	defer it.transactionMux.Unlock()
	// update the transaction orchestrator context
	it.stateManager.SetOrchestratorContext(ctx, tIn)
	if it.stateManager.GetRunningStageContext(ctx) != nil {
		rsc := it.stateManager.GetRunningStageContext(ctx)
		log.L(ctx).Debugf("ProduceLatestInFlightStageContext for tx %s, on stage: %s , current stage context lived: %s , stage lived: %s, last stage error: %+v", it.stateManager.GetTxID(), it.stateManager.GetStage(ctx), time.Since(rsc.StageStartTime), time.Since(it.stateManager.GetStageStartTime(ctx)), it.stateManager.GetStageTriggerError(ctx))
		// once we have a running context, all the metadata should already be loaded
		if it.stateManager.GetStageTriggerError(ctx) != nil {
			log.L(ctx).Errorf("Failed to trigger sate due to %+v, cleaning up the context and retry", it.stateManager.GetStageTriggerError(ctx))
			it.stateManager.ClearRunningStageContext(ctx)
		} else {
			// there is a running stage waiting for inputs
			// first of checking the inputs to see whether we have new items to process
			it.stateManager.ProcessStageOutputs(ctx, func(stageOutputs []*baseTypes.StageOutput) (unprocessedStageOutputs []*baseTypes.StageOutput) {
				unprocessedStageOutputs = make([]*baseTypes.StageOutput, 0)
				log.L(ctx).Debugf("ProduceLatestInFlightStageContext for tx %s, has %d inputs", it.stateManager.GetTxID(), len(stageOutputs))
				for _, rsIn := range stageOutputs {
					if rsIn.Stage == rsc.Stage {
						switch rsIn.Stage {
						case baseTypes.InFlightTxStageRetrieveGasPrice:
							// first check whether we've already completed the action and just waiting for required persistence to go to the next stage
							if rsIn.PersistenceOutput != nil {
								// record persistence information so that if the current stage is not complete
								// the logic outside the output check loop can use the information to determine the next step
								if rsIn.PersistenceOutput.PersistenceError != nil {
									if time.Since(rsIn.PersistenceOutput.Time) > it.persistenceRetryTimeout {
										// retry persistence
										_ = it.TriggerPersistTxState(ctx)
									} else {
										// wait for retry timeout
										unprocessedStageOutputs = append(unprocessedStageOutputs, rsIn)
									}
									continue
								}
								if rsc.StageOutput.GasPriceOutput.Err != nil {
									rsc.StageErrored = true
								}
								if rsIn.PersistenceOutput.PersistenceError == nil && !rsc.StageErrored {
									// new gas price retrieved, the state no longer matches the transaction hash
									it.stateManager.SetValidatedTransactionHashMatchState(ctx, false)
									// we've persisted successfully, it's safe to move to the next stage based on the latest state of the managed transaction
									it.stateManager.ClearRunningStageContext(ctx)
								}
							} else if rsIn.GasPriceOutput == nil {
								log.L(ctx).Errorf("gasPriceOutput should not be nil for transaction with ID: %s, in the stage output object: %+v.", rsc.InMemoryTx.GetTxID(), rsIn)
								tOut.Error = i18n.NewError(ctx, msgs.MsgInvalidStageOutput, "gasPriceOutput", rsIn)
								// unexpected error, reset the running stage context so that it gets retried
								it.stateManager.ClearRunningStageContext(ctx)
							} else {
								rsc.StageOutput.GasPriceOutput = rsIn.GasPriceOutput
								// gas price received, trigger persistence
								rsc.SetNewPersistenceUpdateOutput()
								if rsIn.GasPriceOutput.Err != nil {
									// if failed to get gas price, persist the error
									rsc.StageOutputsToBePersisted.AddSubStatusAction(baseTypes.BaseTxActionRetrieveGasPrice, nil, fftypes.JSONAnyPtr(`{"error":"`+rsIn.GasPriceOutput.Err.Error()+`"}`))
								} else {
									gpo := it.calculateNewGasPrice(ctx, rsc.InMemoryTx.GetGasPriceObject(), rsIn.GasPriceOutput.GasPriceObject)
									gpoJSON, _ := json.Marshal(gpo)
									rsc.StageOutputsToBePersisted.TxUpdates = &baseTypes.BaseTXUpdates{
										GasPrice:             (*ethtypes.HexInteger)(gpo.GasPrice),
										MaxFeePerGas:         (*ethtypes.HexInteger)(gpo.MaxFeePerGas),
										MaxPriorityFeePerGas: (*ethtypes.HexInteger)(gpo.MaxPriorityFeePerGas),
									}
									rsc.StageOutputsToBePersisted.PolicyInfo = &baseTypes.EnterprisePolicyInfo{
										LastWarnTime:      rsc.InMemoryTx.GetPolicyInfo().LastWarnTime,
										SubmittedTxHashes: rsc.InMemoryTx.GetPolicyInfo().SubmittedTxHashes,
									}
									rsc.StageOutputsToBePersisted.AddSubStatusAction(baseTypes.BaseTxActionRetrieveGasPrice, fftypes.JSONAnyPtr(string(gpoJSON)), nil)
								}
								_ = it.TriggerPersistTxState(ctx)
							}
						case baseTypes.InFlightTxStageSigning:
							// first check whether we've already completed the action and just waiting for required persistence to go to the next stage
							if rsIn.PersistenceOutput != nil {
								// record persistence information so that if the current stage is not complete
								// the logic outside the output check loop can use the information to determine the next step
								if rsIn.PersistenceOutput.PersistenceError != nil {
									if time.Since(rsIn.PersistenceOutput.Time) > it.persistenceRetryTimeout {
										// retry persistence
										_ = it.TriggerPersistTxState(ctx)
									} else {
										// wait for retry timeout
										unprocessedStageOutputs = append(unprocessedStageOutputs, rsIn)
									}
									continue
								}
								if rsc.StageOutput.SignOutput.Err != nil {
									// wait for the stale transaction timeout to re-trigger the signing
									rsc.StageErrored = true
								}
								if rsIn.PersistenceOutput.PersistenceError == nil && !rsc.StageErrored {
									// we've persisted successfully, move to the next stage inline as signed message is not persisted
									log.L(ctx).Debugf("Signed message is not nil: %t", rsc.StageOutput.SignOutput.SignedMessage != nil)
									it.TriggerNewStageRun(ctx, baseTypes.InFlightTxStageSubmitting, baseTypes.BaseTxSubStatusReceived, rsc.StageOutput.SignOutput.SignedMessage)
								}
							} else if rsIn.SignOutput == nil {
								log.L(ctx).Errorf("signOutput should not be nil for transaction with ID: %s, in the stage output object: %+v.", rsc.InMemoryTx.GetTxID(), rsIn)
								tOut.Error = i18n.NewError(ctx, msgs.MsgInvalidStageOutput, "signOutput", rsIn)
								// unexpected error, reset the running stage context so that it gets retried
								it.stateManager.ClearRunningStageContext(ctx)
							} else {
								rsc.StageOutput.SignOutput = rsIn.SignOutput

								rsc.SetNewPersistenceUpdateOutput()
								if rsIn.SignOutput.Err != nil {
									// persist the error
									log.L(ctx).Errorf("Transaction signing failed for transaction with ID: %s, due to error: %+v", rsc.InMemoryTx.GetTxID(), rsIn.SignOutput.Err)
									rsc.StageOutputsToBePersisted.AddSubStatusAction(baseTypes.BaseTxActionSign, nil, fftypes.JSONAnyPtr(`{"error":"`+rsIn.SignOutput.Err.Error()+`"}`))
								} else {
									log.L(ctx).Tracef("SignOutput %+v", rsIn.SignOutput)
									// signed data received
									if rsIn.SignOutput.SignedMessage != nil {
										// signed message can be nil when no signer is configured
										rsc.StageOutputsToBePersisted.AddSubStatusAction(baseTypes.BaseTxActionSign, fftypes.JSONAnyPtr(`{"hash":"`+rsIn.SignOutput.TxHash+`"}`), nil)
									}
								}
								_ = it.TriggerPersistTxState(ctx)
							}
						case baseTypes.InFlightTxStageSubmitting:
							// first check whether we've already completed the action and just waiting for required persistence to go to the next stage
							if rsIn.PersistenceOutput != nil {
								// record persistence information so that if the current stage is not complete
								// the logic outside the output check loop can use the information to determine the next step
								if rsIn.PersistenceOutput.PersistenceError != nil {
									if time.Since(rsIn.PersistenceOutput.Time) > it.persistenceRetryTimeout {
										// retry persistence
										_ = it.TriggerPersistTxState(ctx)
									} else {
										// wait for retry timeout
										unprocessedStageOutputs = append(unprocessedStageOutputs, rsIn)
									}
									continue
								}
								if rsc.StageOutput.SubmitOutput.Err != nil {
									rsc.StageErrored = true
									if rsc.StageOutput.SubmitOutput.ErrorReason == string(ethclient.ErrorReasonInsufficientFunds) {
										it.balanceManager.NotifyAddressBalanceChanged(ctx, it.signingAddress)
									}
								}
								if rsIn.PersistenceOutput.PersistenceError == nil && !rsc.StageErrored {
									// we've persisted successfully, it's safe to move to the next stage based on the latest state of the managed transaction
									it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
									it.stateManager.ClearRunningStageContext(ctx)
								}
							} else if rsIn.SubmitOutput == nil {
								log.L(ctx).Errorf("submitOutput should not be nil for transaction with ID: %s, in the stage output object: %+v.", rsc.InMemoryTx.GetTxID(), rsIn)
								tOut.Error = i18n.NewError(ctx, msgs.MsgInvalidStageOutput, "submitOutput", rsIn)
								// unexpected error, reset the running stage context so that it gets retried
								it.stateManager.ClearRunningStageContext(ctx)
							} else {
								rsc.StageOutput.SubmitOutput = rsIn.SubmitOutput
								// transaction submitted
								rsc.SetNewPersistenceUpdateOutput()
								if rsIn.SubmitOutput.Err != nil {
									log.L(ctx).Errorf("Submitting transaction error for transaction %s: %+v", rsc.InMemoryTx.GetTxID(), rsIn.SubmitOutput.Err)
									errMsg := rsIn.SubmitOutput.Err.Error()
									rsc.StageOutputsToBePersisted.TxUpdates = &baseTypes.BaseTXUpdates{
										ErrorMessage: &errMsg,
									}
									rsc.StageOutputsToBePersisted.AddSubStatusAction(baseTypes.BaseTxActionSubmitTransaction, fftypes.JSONAnyPtr(`{"reason":"`+string(rsIn.SubmitOutput.ErrorReason)+`"}`), fftypes.JSONAnyPtr(`{"error":"`+rsIn.SubmitOutput.Err.Error()+`"}`))
									if rsc.InMemoryTx.GetTransactionHash() != "" {
										// did a re-submission, no matter the result, update the last warn time to avoid another retry
										rsc.StageOutputsToBePersisted.PolicyInfo = &baseTypes.EnterprisePolicyInfo{
											LastWarnTime:      fftypes.Now(),
											SubmittedTxHashes: rsc.InMemoryTx.GetPolicyInfo().SubmittedTxHashes,
										}
									}
								} else {
									if rsIn.SubmitOutput.SubmissionOutcome == baseTypes.SubmissionOutcomeSubmittedNew {
										// new transaction submitted successfully
										rsc.StageOutputsToBePersisted.AddSubStatusAction(baseTypes.BaseTxActionSubmitTransaction, fftypes.JSONAnyPtr(`{"txHash":"`+rsIn.SubmitOutput.TxHash+`"}`), nil)
										rsc.StageOutputsToBePersisted.TxUpdates = &baseTypes.BaseTXUpdates{
											LastSubmit: rsIn.SubmitOutput.SubmissionTime,
										}
										log.L(ctx).Debugf("Transaction submitted for tx %s (update=%d,status=%s,hash=%s)", rsc.InMemoryTx.GetTxID(), rsc.StageOutputsToBePersisted.UpdateType, rsc.InMemoryTx.GetStatus(), rsc.InMemoryTx.GetTransactionHash())
										rsc.StageOutputsToBePersisted.TxUpdates.TransactionHash = &rsc.StageOutput.SubmitOutput.TxHash
									} else if rsIn.SubmitOutput.SubmissionOutcome == baseTypes.SubmissionOutcomeNonceTooLow {
										log.L(ctx).Debugf("Nonce too low for tx %s (update=%d,status=%s,hash=%s)", rsc.InMemoryTx.GetTxID(), rsc.StageOutputsToBePersisted.UpdateType, rsc.InMemoryTx.GetStatus(), rsc.InMemoryTx.GetTransactionHash())
										rsc.StageOutputsToBePersisted.AddSubStatusAction(baseTypes.BaseTxActionSubmitTransaction, fftypes.JSONAnyPtr(`{"txHash":"`+rsIn.SubmitOutput.TxHash+`"}`), nil)
									} else if rsIn.SubmitOutput.SubmissionOutcome == baseTypes.SubmissionOutcomeAlreadyKnown {
										// nothing to add for persistence, go to the tracking stage
										log.L(ctx).Debugf("Transaction already known for tx %s (update=%d,status=%s,hash=%s)", rsc.InMemoryTx.GetTxID(), rsc.StageOutputsToBePersisted.UpdateType, rsc.InMemoryTx.GetStatus(), rsc.InMemoryTx.GetTransactionHash())
									}
									// did the first submit
									if rsc.InMemoryTx.GetFirstSubmit() == nil {
										log.L(ctx).Debugf("Recorded the first submission for transaction %s", rsc.InMemoryTx.GetTxID())
										if rsc.StageOutputsToBePersisted.TxUpdates == nil {
											rsc.StageOutputsToBePersisted.TxUpdates = &baseTypes.BaseTXUpdates{}
										}
										rsc.StageOutputsToBePersisted.TxUpdates.FirstSubmit = rsIn.SubmitOutput.SubmissionTime
									}

									if rsc.InMemoryTx.GetTransactionHash() == "" {
										if rsc.StageOutputsToBePersisted.TxUpdates == nil {
											rsc.StageOutputsToBePersisted.TxUpdates = &baseTypes.BaseTXUpdates{}
										}
										log.L(ctx).Debugf("Recorded the tx hash %s for transaction %s", rsc.StageOutput.SubmitOutput.TxHash, rsc.InMemoryTx.GetTxID())
										rsc.StageOutputsToBePersisted.TxUpdates.TransactionHash = &rsc.StageOutput.SubmitOutput.TxHash
										rsc.StageOutputsToBePersisted.TxUpdates.LastSubmit = rsIn.SubmitOutput.SubmissionTime
									}

									rsc.StageOutputsToBePersisted.PolicyInfo = &baseTypes.EnterprisePolicyInfo{
										LastWarnTime:      fftypes.Now(),
										SubmittedTxHashes: rsc.InMemoryTx.GetPolicyInfo().SubmittedTxHashes,
									}
									if rsc.StageOutput.SubmitOutput.TxHash != "" {
										// as long as we add the tx hash in to the submitted transaction array
										// the pre-tracking logic will figure out which transaction hash is the right one to set
										// on the main tx record based on receipt checking
										existingHashes := rsc.InMemoryTx.GetPolicyInfo().SubmittedTxHashes
										if existingHashes == nil {
											existingHashes = make([]string, 0)
										}
										hashAlreadyTracked := false
										for _, hash := range existingHashes {
											if hash == rsIn.SubmitOutput.TxHash {
												hashAlreadyTracked = true
											}
										}
										if !hashAlreadyTracked {
											existingHashes = append(existingHashes, rsIn.SubmitOutput.TxHash)
											rsc.StageOutputsToBePersisted.PolicyInfo.SubmittedTxHashes = existingHashes
										}
									}
								}

								_ = it.TriggerPersistTxState(ctx)
							}
						case baseTypes.InFlightTxStageReceipting:
							// first check whether we've already completed the action and just waiting for required persistence to go to the next stage
							if rsIn.PersistenceOutput != nil {
								// record persistence information so that if the current stage is not complete
								// the logic outside the output check loop can use the information to determine the next step
								if rsIn.PersistenceOutput.PersistenceError != nil {
									if time.Since(rsIn.PersistenceOutput.Time) > it.persistenceRetryTimeout {
										// retry persistence
										_ = it.TriggerPersistTxState(ctx)
									} else {
										// wait for retry timeout
										unprocessedStageOutputs = append(unprocessedStageOutputs, rsIn)
									}
									continue
								}
								if rsIn.PersistenceOutput.PersistenceError == nil {
									// we've persisted successfully, it's safe to move to the next stage based on the latest state of the managed transaction
									it.TriggerNewStageRun(ctx, baseTypes.InFlightTxStageConfirming, baseTypes.BaseTxSubStatusTracking, nil) // just do the stage switch, listener is already set up.
									it.MarkInFlightTxStale()                                                                                // notify tx is stale as confirmation events might already in the queue
								}
							} else if rsIn.ReceiptOutput == nil {
								log.L(ctx).Errorf("receiptOutput should not be nil for transaction with ID: %s, in the stage output object: %+v.", rsc.InMemoryTx.GetTxID(), rsIn)
								tOut.Error = i18n.NewError(ctx, msgs.MsgInvalidStageOutput, "receiptOutput", rsIn)
								// unexpected error, reset the running stage context so that it gets retried
								it.stateManager.ClearRunningStageContext(ctx)
							} else {
								if rsIn.ReceiptOutput.Err != nil {
									// for now, we log the error and wait for the stale transaction timeout to re-trigger the signing
									log.L(ctx).Errorf("Setting up receipt listener for transaction with ID: %s failed due to error: %+v", rsc.InMemoryTx.GetTxID(), rsIn.ReceiptOutput.Err)
									rsc.StageErrored = true
								} else {
									// receipt received
									log.L(ctx).Debugf("Receipt received for transaction %s at nonce %s / %d - hash: %s", rsc.InMemoryTx.GetTxID(), rsc.InMemoryTx.GetFrom(), rsc.InMemoryTx.GetNonce().Int64(), rsc.InMemoryTx.GetTransactionHash())
									rsc.SetNewPersistenceUpdateOutput()
									rsc.StageOutputsToBePersisted.Receipt = rsIn.ReceiptOutput.Receipt
									rsc.StageOutputsToBePersisted.AddSubStatusAction(baseTypes.BaseTxActionReceiveReceipt, fftypes.JSONAnyPtr(`{"protocolId":"`+rsIn.ReceiptOutput.Receipt.ProtocolID+`"}`), nil)
									_ = it.TriggerPersistTxState(ctx) // we could have unnecessary persistence call here when transaction paused/crash during tracking state after the receipt is stored.
								}
							}
						case baseTypes.InFlightTxStageConfirming:
							// first check whether we've already completed the action and just waiting for required persistence to go to the next stage
							if rsIn.PersistenceOutput != nil {
								// record persistence information so that if the current stage is not complete
								// the logic outside the output check loop can use the information to determine the next step
								if rsIn.PersistenceOutput.PersistenceError != nil {
									if time.Since(rsIn.PersistenceOutput.Time) > it.persistenceRetryTimeout {
										// retry persistence
										_ = it.TriggerPersistTxState(ctx)
									} else {
										// wait for retry timeout
										unprocessedStageOutputs = append(unprocessedStageOutputs, rsIn)
									}
									continue
								}
								if rsIn.PersistenceOutput.PersistenceError == nil {
									// we've persisted successfully
									// check whether this is the last persistence required for this transaction
									if rsc.InMemoryTx.IsComplete() {
										// we've persisted successfully, perform completion actions when the transaction received all confirmations
										// dispatch an event to event handler and discard any handling errors
										if rsc.InMemoryTx.GetStatus() == baseTypes.BaseTxStatusSucceeded {
											_ = it.managedTXEventNotifier.Notify(ctx, baseTypes.ManagedTransactionEvent{
												Type:    baseTypes.ManagedTXProcessSucceeded,
												Tx:      rsc.InMemoryTx.GetTx(),
												Receipt: rsc.InMemoryTx.GetReceipt(),
											})
										} else if rsc.InMemoryTx.GetStatus() == baseTypes.BaseTxStatusFailed {
											_ = it.managedTXEventNotifier.Notify(ctx, baseTypes.ManagedTransactionEvent{
												Type:    baseTypes.ManagedTXProcessFailed,
												Tx:      rsc.InMemoryTx.GetTx(),
												Receipt: rsc.InMemoryTx.GetReceipt(),
											})
										}
										log.L(ctx).Infof("Complete: Transaction %s marked ready to be removed (status=%s) after %s (%s in-flight)", rsc.InMemoryTx.GetTxID(), rsc.InMemoryTx.GetStatus(), time.Since(it.txInDBTime), time.Since(it.txInflightTime))
										log.L(ctx).Debugf("Timeline: transaction %s (status=%s): %s (%s in-flight), timeline: %s", rsc.InMemoryTx.GetTxID(), rsc.InMemoryTx.GetStatus(), time.Since(it.txInDBTime), time.Since(it.txInflightTime), it.PrintTimeline())
										it.stateManager.ClearRunningStageContext(ctx)
										it.MarkInFlightTxStale()
									} else {
										// when there are still more confirmations, clear the stage output to be persisted to let new one go through
										rsc.StageOutputsToBePersisted = nil
									}
								}
							} else if rsIn.ConfirmationOutput == nil {
								log.L(ctx).Errorf("confirmationOutput should not be nil for transaction with ID: %s, in the stage output object: %+v.", rsc.InMemoryTx.GetTxID(), rsIn)
								tOut.Error = i18n.NewError(ctx, msgs.MsgInvalidStageOutput, "confirmationOutput", rsIn)
								// unexpected error, reset the running stage context so that it gets retried
								it.stateManager.ClearRunningStageContext(ctx)
							} else {
								if rsc.StageOutputsToBePersisted != nil {
									// there is already a confirmation wait for persistence, wait for that to finish first
									unprocessedStageOutputs = append(unprocessedStageOutputs, rsIn)
								} else {
									// new transaction confirmed received
									log.L(ctx).Debugf("Confirmed transaction %s at nonce %s / %d - hash: %s", rsc.InMemoryTx.GetTxID(), rsc.InMemoryTx.GetFrom(), rsc.InMemoryTx.GetNonce().Int64(), rsc.InMemoryTx.GetTransactionHash())
									rsc.SetNewPersistenceUpdateOutput()
									rsc.StageOutputsToBePersisted.Confirmations = rsIn.ConfirmationOutput.Confirmations
									_ = it.TriggerPersistTxState(ctx)
								}
							}
						case baseTypes.InFlightTxStageStatusUpdate:
							// only requires persistence output for this stage
							if rsIn.PersistenceOutput != nil {
								// record persistence information so that if the current stage is not complete
								// the logic outside the output check loop can use the information to determine the next step
								if rsIn.PersistenceOutput.PersistenceError != nil {
									if time.Since(rsIn.PersistenceOutput.Time) > it.persistenceRetryTimeout {
										// retry persistence
										_ = it.TriggerPersistTxState(ctx)
									} else {
										// wait for retry timeout
										unprocessedStageOutputs = append(unprocessedStageOutputs, rsIn)
									}
									continue
								} else {
									// we've persisted successfully, check the status and clean up the newStatus if we successfully switched to the latest desired status
									if *it.newStatus == it.stateManager.GetStatus() {
										log.L(ctx).Debugf("Transaction with ID %s reached desired new status: %s", it.stateManager.GetTxID(), it.stateManager.GetStatus())
										// already has the lock
										it.newStatus = nil
									}
									it.stateManager.ClearRunningStageContext(ctx)
								}
							} else {
								log.L(ctx).Errorf("persistenceOutput should not be nil for transaction with ID: %s, in the stage output object: %+v.", rsc.InMemoryTx.GetTxID(), rsIn)
								tOut.Error = i18n.NewError(ctx, msgs.MsgInvalidStageOutput, "persistenceOutput", rsIn)
								// unexpected error, reset the running stage context so that it gets retried
								it.stateManager.ClearRunningStageContext(ctx)
							}
						}

					} else {
						log.L(ctx).Tracef("Current stage: %s, received inputs for future stage %s for transaction with ID: %s", rsc.Stage, rsIn.Stage, rsc.InMemoryTx.GetTxID())
						unprocessedStageOutputs = append(unprocessedStageOutputs, rsIn)
					}
				}
				log.L(ctx).Debugf("ProduceLatestInFlightStageContext for tx %s, %d inputs is carrying on", rsc.InMemoryTx.GetTxID(), len(unprocessedStageOutputs))
				return unprocessedStageOutputs
			})

			if (rsc.Stage == baseTypes.InFlightTxStageConfirming || rsc.Stage == baseTypes.InFlightTxStageReceipting) && time.Since(*rsc.InMemoryTx.GetPolicyInfo().LastWarnTime.Time()) > it.resubmitInterval {
				// timed out waiting for receipt
				log.L(ctx).Debugf("Cancelling confirmation manager tracking of hash %s for TX %s", rsc.InMemoryTx.GetTransactionHash(), rsc.InMemoryTx.GetTxID())
				if err := it.txConfirmationListener.Remove(ctx, rsc.InMemoryTx.GetTransactionHash()); err != nil {
					// Unexpected error, ignore as fail to remove the listener is not a stop deal
					log.L(ctx).Errorf("Error detected notifying confirmation manager to remove transaction hash: %s", err.Error())
				}
				// trigger a resubmission when exceeded the resubmit interval
				it.stateManager.ClearRunningStageContext(ctx)
			}

			if rsc.StageErrored && time.Since(rsc.StageStartTime) > it.stageRetryTimeout {
				// if the stage didn't succeed, we retry the stage after the stage timeout
				log.L(ctx).Debugf("Retrying stage: %s, for transaction with ID: %s after %s", rsc.Stage, rsc.InMemoryTx.GetTxID(), time.Since(rsc.StageStartTime))
				it.stateManager.ClearRunningStageContext(ctx)
			}
		}
	}

	if it.stateManager.GetGasPriceObject() != nil {
		if it.stateManager.GetReceipt() != nil {
			// already has receipt so the cost to submit this transaction is zero
			tOut.Cost = big.NewInt(0)
		} else {
			gpo := it.stateManager.GetGasPriceObject()
			c, err := calculateGasRequiredForTransaction(ctx, gpo, it.stateManager.GetGasLimit())
			if err == nil {
				tOut.Cost = c
			}
		}
	}

	if it.stateManager.GetRunningStageContext(ctx) == nil {
		// no running context in flight
		// first apply any status update that's required
		if it.newStatus != nil && !it.stateManager.IsComplete() && *it.newStatus != it.stateManager.GetStatus() {
			log.L(ctx).Debugf("Transaction with ID %s entering status update, current status: %s, target status: %s", it.stateManager.GetTxID(), it.stateManager.GetStatus(), *it.newStatus)
			it.TriggerNewStageRun(ctx, baseTypes.InFlightTxStageStatusUpdate, baseTypes.BaseTxSubStatusReceived, nil)
		} else if it.stateManager.IsComplete() || it.stateManager.IsSuspended() {
			// then calculate the latest stage based on the managed transaction to kick off the next stage
			// if there isn't any running context and the transaction status is no longer in pending
			// we can wait for the transaction orchestrator to remove it from the in-flight transaction queue. It's either paused or completed
			log.L(ctx).Debugf("Transaction with ID %s is waiting for removal in status: %s.", it.stateManager.GetTxID(), it.stateManager.GetStatus())
		} else if it.stateManager.GetGasPriceObject() == nil {
			// no gas price fetched, go and fetch gas price
			log.L(ctx).Debugf("Transaction with ID %s entering retrieve gas price as no gas price available.", it.stateManager.GetTxID())
			it.TriggerNewStageRun(ctx, baseTypes.InFlightTxStageRetrieveGasPrice, baseTypes.BaseTxSubStatusReceived, nil)
		} else if it.stateManager.GetTransactionHash() == "" {
			if it.stateManager.CanSubmit(ctx, tOut.Cost) {
				// no transaction hash, do signing and submission
				log.L(ctx).Debugf("Transaction with ID %s entering signing stage as no transaction hash recorded.", it.stateManager.GetTxID())
				it.TriggerNewStageRun(ctx, baseTypes.InFlightTxStageSigning, baseTypes.BaseTxSubStatusReceived, nil)
			} else {
				log.L(ctx).Debugf("Transaction with ID %s no op, as cannot submit.", it.stateManager.GetTxID())
			}
		} else {
			// we have a transaction hash recorded, we must ensure we checks the hash matches
			// the state we persisted by triggering a submission
			if !it.stateManager.ValidatedTransactionHashMatchState(ctx) {
				if it.stateManager.CanSubmit(ctx, tOut.Cost) {
					log.L(ctx).Debugf("Transaction with ID %s entering signing stage as current state hasn't been validated.", it.stateManager.GetTxID())
					it.TriggerNewStageRun(ctx, baseTypes.InFlightTxStageSigning, baseTypes.BaseTxSubStatusReceived, nil)
				} else {
					log.L(ctx).Debugf("Transaction with ID %s no op, as cannot submit, state not validated.", it.stateManager.GetTxID())
				}
			} else {
				// once we validated the transaction hash matched the transaction state
				policyInfo := it.stateManager.GetPolicyInfo()
				if policyInfo != nil && policyInfo.LastWarnTime != nil && time.Since(*policyInfo.LastWarnTime.Time()) > it.resubmitInterval {
					// do a resubmission when exceeded the resubmit interval
					log.L(ctx).Debugf("Transaction with ID %s entering retrieve gas price as exceeded resubmit interval of %s.", it.stateManager.GetTxID(), it.resubmitInterval.String())
					it.TriggerNewStageRun(ctx, baseTypes.InFlightTxStageRetrieveGasPrice, baseTypes.BaseTxSubStatusStale, nil)
				} else {
					// check and track the existing transaction hash
					log.L(ctx).Debugf("Transaction with ID %s entering tracking stage", it.stateManager.GetTxID())
					it.TriggerNewStageRun(ctx, baseTypes.InFlightTxStageReceipting, baseTypes.BaseTxSubStatusTracking, nil)
				}
			}

		}
	}
	tOut.TransactionSubmitted = it.stateManager.GetTransactionHash() != ""

	return tOut
}

func (it *InFlightTransactionStageController) calculateNewGasPrice(ctx context.Context, existingGpo *baseTypes.GasPriceObject, newGpo *baseTypes.GasPriceObject) *baseTypes.GasPriceObject {
	if existingGpo == nil {
		log.L(ctx).Debugf("First time assigning gas price to transaction with ID: %s, gas price object: %+v.", it.stateManager.GetTxID(), newGpo)
		return newGpo
	}
	if newGpo.GasPrice != nil && existingGpo.GasPrice != nil && existingGpo.GasPrice.Cmp(newGpo.GasPrice) == 1 {
		// existing gas price already above the new gas price, increase using percentage
		newPercentage := big.NewInt(100)
		newPercentage = newPercentage.Add(newPercentage, it.gasPriceIncreasePercent)
		existingGpo.GasPrice = existingGpo.GasPrice.Mul(existingGpo.GasPrice, newPercentage)
		existingGpo.GasPrice = existingGpo.GasPrice.Div(existingGpo.GasPrice, big.NewInt(100))
		if it.gasPriceIncreaseMax != nil && existingGpo.GasPrice.Cmp(it.gasPriceIncreaseMax) == 1 {
			existingGpo.GasPrice = it.gasPriceIncreaseMax
		}
	} else if newGpo.MaxFeePerGas != nil && existingGpo.MaxFeePerGas != nil && existingGpo.MaxFeePerGas.Cmp(newGpo.MaxFeePerGas) == 1 {
		// existing MaxFeePerGas already above the new MaxFeePerGas, increase using percentage
		newPercentage := big.NewInt(100)

		newPercentage = newPercentage.Add(newPercentage, it.gasPriceIncreasePercent)
		existingGpo.MaxFeePerGas = existingGpo.MaxFeePerGas.Mul(existingGpo.MaxFeePerGas, newPercentage)
		existingGpo.MaxFeePerGas = existingGpo.MaxFeePerGas.Div(existingGpo.MaxFeePerGas, big.NewInt(100))
		if it.gasPriceIncreaseMax != nil && existingGpo.MaxFeePerGas.Cmp(it.gasPriceIncreaseMax) == 1 {
			existingGpo.MaxFeePerGas = it.gasPriceIncreaseMax
		}
	} else {
		return newGpo
	}

	return existingGpo
}

func calculateGasRequiredForTransaction(ctx context.Context, gpo *baseTypes.GasPriceObject, gasLimit *big.Int) (gasRequired *big.Int, err error) {
	if gasLimit == nil || gasLimit.Sign() <= 0 {
		return nil, i18n.NewError(ctx, msgs.MsgInvalidGasLimit)
	}
	if gpo.GasPrice != nil {
		log.L(ctx).Debugf("gas calculation using GasPrice (%v)", gpo.GasPrice)
		gasPriceCopy := big.NewInt(gpo.GasPrice.Int64())
		gasRequired = gasPriceCopy.Mul(gasPriceCopy, gasLimit)
	} else if gpo.MaxFeePerGas != nil && gpo.MaxPriorityFeePerGas != nil {
		// max-fee and max-priority-fee have been provided. We can only use
		// max-fee to calculate how much this TX could cost, but we ultimately
		// require both to be set (max-priority-fee will be needed when we send
		// the TX asking for fuel)
		log.L(ctx).Debugf("fuel gas calculation using MaxFeePerGas (%v)", gpo.MaxFeePerGas)
		MaxFeePerGasCopy := big.NewInt(gpo.MaxFeePerGas.Int64())
		gasRequired = MaxFeePerGasCopy.Mul(MaxFeePerGasCopy, gasLimit)
	}
	return gasRequired, nil

}

func (it *InFlightTransactionStageController) NotifyStatusUpdate(ctx context.Context, status *baseTypes.BaseTxStatus) (updateRequired bool, err error) {
	if it.stateManager.GetStatus() == *status {
		// already on the current status, no op
		return false, nil
	}
	if it.stateManager.IsComplete() {
		// cannot update status of a completed transaction, return error
		return false, i18n.NewError(ctx, msgs.MsgStatusUpdateForbidden)
	}
	// queue the status to be updated in future evaluation loops
	it.transactionMux.Lock() // acquire a lock here to prevent overrides from existing status update
	defer it.transactionMux.Unlock()
	it.newStatus = status
	return true, nil
}

func (it *InFlightTransactionStageController) TriggerRetrieveGasPrice(ctx context.Context) error {
	it.executeAsync(func() {
		gasPrice, err := it.gasPriceClient.GetGasPriceObject(ctx)
		it.stateManager.AddGasPriceOutput(ctx, gasPrice, err)
	}, ctx, it.stateManager.GetStage(ctx), false)
	return nil
}

func (it *InFlightTransactionStageController) TriggerStatusUpdate(ctx context.Context) error {
	it.executeAsync(func() {
		rsc := it.stateManager.GetRunningStageContext(ctx)
		rsc.SetNewPersistenceUpdateOutput()
		rsc.StageOutputsToBePersisted.AddSubStatusAction(baseTypes.BaseTxActionStateTransition, fftypes.JSONAnyPtr(`{"status":"`+string(*it.newStatus)+`"}`), nil)
		rsc.StageOutputsToBePersisted.TxUpdates = &baseTypes.BaseTXUpdates{
			Status: it.newStatus,
		}
		stage, persistenceTime, err := it.stateManager.PersistTxState(ctx)
		it.stateManager.AddPersistenceOutput(ctx, stage, persistenceTime, err)
	}, ctx, it.stateManager.GetStage(ctx), false)
	return nil
}
func (it *InFlightTransactionStageController) TriggerTracking(ctx context.Context) error {
	it.executeAsync(func() {
		transactionHashToBeTracked := it.stateManager.GetTransactionHash()
		policyInfo := it.stateManager.GetPolicyInfo()
		if len(policyInfo.SubmittedTxHashes) > 0 {
			// check which transaction hash can be tracked
			for _, hash := range policyInfo.SubmittedTxHashes {
				if hash != "" && hash != it.stateManager.GetTransactionHash() {
					_, err := it.ethClient.GetTransactionReceipt(ctx, hash)
					if err == nil {
						// found receipt for transaction hash use it for tracking
						transactionHashToBeTracked = hash
						break
					}
				}
			}
		}

		if transactionHashToBeTracked != it.stateManager.GetTransactionHash() {
			rsc := it.stateManager.GetRunningStageContext(ctx)
			rsc.SetNewPersistenceUpdateOutput()
			rsc.StageOutputsToBePersisted.AddSubStatusAction(baseTypes.BaseTxActionStateTransition, fftypes.JSONAnyPtr(`{"txHash":"`+transactionHashToBeTracked+`"}`), nil)
			rsc.StageOutputsToBePersisted.TxUpdates = &baseTypes.BaseTXUpdates{
				TransactionHash: &transactionHashToBeTracked,
			}
			_, _, err := it.stateManager.PersistTxState(ctx)
			if err != nil {
				// restart from scratch
				it.stateManager.ClearRunningStageContext(ctx)
				return
			}
			rsc.StageOutputsToBePersisted = nil
		}
		it.MarkTime("register_receipt_confirmation_handler")
		log.L(ctx).Debugf("Passing transaction to confirmation manager %s hash=%s", it.stateManager.GetTxID(), it.stateManager.GetTransactionHash())
		if err := it.txConfirmationListener.Add(ctx, it.stateManager.GetTxID(), it.stateManager.GetTransactionHash(), it.CreateTransactionReceiptReceivedHandler(it.stateManager.GetTxID()), it.CreateTransactionConfirmationsHandler(it.stateManager.GetTxID())); err != nil {
			it.stateManager.AddReceiptOutput(ctx, nil, err)
		}
	}, ctx, it.stateManager.GetStage(ctx), false)
	return nil
}
func (it *InFlightTransactionStageController) TriggerSignTx(ctx context.Context) error {
	it.executeAsync(func() {
		signedMessage, txHash, err := it.signTx(ctx, it.stateManager.GetTx())
		log.L(ctx).Debugf("Adding signed message to output, hash %s, signedMessage not nil %t, err %+v", txHash, signedMessage != nil, err)
		it.stateManager.AddSignOutput(ctx, signedMessage, txHash, err)
	}, ctx, it.stateManager.GetStage(ctx), false)
	return nil
}

func (it *InFlightTransactionStageController) TriggerSubmitTx(ctx context.Context, signedMessage []byte) error {
	it.executeAsync(func() {
		txHash, submissionTime, errReason, submissionOutcome, err := it.submitTX(ctx, it.stateManager.GetTx(), signedMessage)
		it.stateManager.AddSubmitOutput(ctx, txHash, submissionTime, submissionOutcome, errReason, err)
	}, ctx, it.stateManager.GetStage(ctx), false)
	return nil
}

func (it *InFlightTransactionStageController) TriggerPersistTxState(ctx context.Context) error {
	it.executeAsync(func() {
		stage, persistenceTime, err := it.stateManager.PersistTxState(ctx)
		it.stateManager.AddPersistenceOutput(ctx, stage, persistenceTime, err)
	}, ctx, it.stateManager.GetStage(ctx), true)
	return nil
}

type TriggerNextStageOutput struct {
	Cost                 *big.Int
	TransactionSubmitted bool
	Error                error
}

func (it *InFlightTransactionStageController) executeAsync(funcToExecute func(), ctx context.Context, stage baseTypes.InFlightTxStage, isPersistence bool) {
	if it.testOnlyNoActionMode {
		return
	}
	go func() {
		defer func() {
			if err := recover(); err != nil {
				// if the function panicked, catch it and write a panic error to the output queue
				log.L(ctx).Errorf("Panic error detected for transaction %s, when executing: %s, error: %+v", it.stateManager.GetTxID(), stage, err)
				it.stateManager.AddPanicOutput(ctx, stage)
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
