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
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/core/internal/publictxmgr/metrics"
	"github.com/LFDT-Paladin/paladin/core/mocks/publictxmgrmocks"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type testInFlightTransactionStateVersionWithMocks struct {
	version InFlightTransactionStateGeneration
	mAT     *publictxmgrmocks.InFlightStageActionTriggers
}

func newTestInFlightTransactionStateVersion(t *testing.T) (*testInFlightTransactionStateVersionWithMocks, *mocksAndTestControl, func()) {
	_, balanceManager, ptm, m, done := newTestBalanceManager(t)

	metrics := metrics.InitMetrics(context.Background(), prometheus.NewRegistry())
	mockInMemoryState := NewTestInMemoryTxState(t)
	mockActionTriggers := publictxmgrmocks.NewInFlightStageActionTriggers(t)

	v := NewInFlightTransactionStateGeneration(metrics, balanceManager, mockActionTriggers, mockInMemoryState, ptm, ptm.submissionWriter, false)
	return &testInFlightTransactionStateVersionWithMocks{
		v,
		mockActionTriggers,
	}, m, done
}

func TestStateVersionBasic(t *testing.T) {
	ctx := context.Background()
	testStateVersionWithMocks, _, done := newTestInFlightTransactionStateVersion(t)
	defer done()
	version := testStateVersionWithMocks.version
	assert.Nil(t, version.GetRunningStageContext(ctx))
	assert.Nil(t, version.GetStageTriggerError(ctx))
	assert.Empty(t, version.GetStage(ctx))
	assert.NotNil(t, version.GetStageStartTime(ctx))
}

func TestStateVersionTransactionFromRetrieveGasPriceToTracking(t *testing.T) {
	testStateVersionWithMocks, _, done := newTestInFlightTransactionStateVersion(t)
	defer done()

	ctx := context.Background()
	version := testStateVersionWithMocks.version

	mockActionTriggers := testStateVersionWithMocks.mAT
	defer mockActionTriggers.AssertExpectations(t)
	// retrieve gas price errored
	mockActionTriggers.On("TriggerRetrieveGasPrice", mock.Anything).Return(fmt.Errorf("gasPriceError")).Once()
	version.StartNewStageContext(ctx, InFlightTxStageRetrieveGasPrice, BaseTxSubStatusReceived)
	assert.Regexp(t, "gasPriceError", version.GetStageTriggerError(ctx))
	// retrieve gas price success
	mockActionTriggers.On("TriggerRetrieveGasPrice", mock.Anything).Return(nil).Once()
	version.StartNewStageContext(ctx, InFlightTxStageRetrieveGasPrice, BaseTxSubStatusReceived)
	assert.Nil(t, version.GetStageTriggerError(ctx))

	var nilBytes []byte
	var nilHash *pldtypes.Bytes32
	// scenario A: no signer configured, do submission
	mockActionTriggers.On("TriggerSubmitTx", mock.Anything, nilBytes, nilHash, mock.Anything, mock.Anything).Return(nil).Once()

	version.StartNewStageContext(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived)
	assert.Nil(t, version.GetStageTriggerError(ctx))

	// scenario B: signer configured sign the data
	mockActionTriggers.On("TriggerSignTx", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	version.StartNewStageContext(ctx, InFlightTxStageSigning, BaseTxSubStatusReceived)
	assert.Nil(t, version.GetStageTriggerError(ctx))
	// persist the signed data as transient output
	testSignedData := []byte("test signed data")
	testHash := confutil.P(pldtypes.RandBytes32())
	version.SetTransientPreviousStageOutputs(&TransientPreviousStageOutputs{
		SignedMessage:   testSignedData,
		TransactionHash: testHash,
	})
	// do the submission
	mockActionTriggers.On("TriggerSubmitTx", mock.Anything, testSignedData, testHash, mock.Anything, mock.Anything).Return(nil)
	version.StartNewStageContext(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived)
	assert.Nil(t, version.GetStageTriggerError(ctx))

	// start tracking  no actions need to be triggered
	version.StartNewStageContext(ctx, InFlightTxStageSubmitting, BaseTxSubStatusTracking)
	assert.Nil(t, version.GetStageTriggerError(ctx))

	// test clear running stage context
	assert.NotNil(t, version.GetRunningStageContext(ctx))
	version.ClearRunningStageContext(ctx)
	assert.Nil(t, version.GetRunningStageContext(ctx))
	// try to clear running stage context when there isn't one shouldn't throw error
	version.ClearRunningStageContext(ctx)
	assert.Nil(t, version.GetRunningStageContext(ctx))
}

func TestStateManagerStageOutputManagement(t *testing.T) {
	ctx := context.Background()
	testStateVersionWithMocks, _, done := newTestInFlightTransactionStateVersion(t)
	defer done()
	version := testStateVersionWithMocks.version

	expectedNumberOfPersistenceSuccessOutput := 342
	expectedNumberOfPersistenceErrorOutput := 50
	expectedNumberOfSubmitSuccessOutput := 234
	expectedNumberOfSubmitErrorOutput := 67
	expectedNumberOfSignSuccessOutput := 843
	expectedNumberOfSignErrorOutput := 452
	expectedNumberOfGasPriceSuccessOutput := 123
	expectedNumberOfGasPriceErrorOutput := 34
	expectedNumberOfPanicOutput := 19
	totalOutputAdd := expectedNumberOfPersistenceSuccessOutput + expectedNumberOfPersistenceErrorOutput + expectedNumberOfSubmitSuccessOutput + expectedNumberOfSubmitErrorOutput + expectedNumberOfSignSuccessOutput + expectedNumberOfSignErrorOutput + expectedNumberOfGasPriceSuccessOutput + expectedNumberOfGasPriceErrorOutput + expectedNumberOfPanicOutput
	countChanel := make(chan bool, totalOutputAdd)

	go func() {
		for i := 0; i < expectedNumberOfPersistenceSuccessOutput; i++ {
			go func() {
				version.AddPersistenceOutput(ctx, InFlightTxStageQueued, time.Now(), nil)
				countChanel <- true
			}()
		}
	}()
	go func() {
		for i := 0; i < expectedNumberOfPersistenceErrorOutput; i++ {
			go func() {
				version.AddPersistenceOutput(ctx, InFlightTxStageQueued, time.Now(), fmt.Errorf("error"))
				countChanel <- true
			}()
		}

	}()
	go func() {
		for i := 0; i < expectedNumberOfSubmitSuccessOutput; i++ {
			go func() {
				version.AddSubmitOutput(ctx, confutil.P(pldtypes.Bytes32Keccak([]byte("0x000031"))), confutil.P(pldtypes.TimestampNow()), SubmissionOutcomeAlreadyKnown, "", nil)
				countChanel <- true
			}()
		}

	}()
	go func() {
		for i := 0; i < expectedNumberOfSubmitErrorOutput; i++ {
			go func() {
				version.AddSubmitOutput(ctx, nil, confutil.P(pldtypes.TimestampNow()), SubmissionOutcomeFailedRequiresRetry, "error", fmt.Errorf("error"))
				countChanel <- true
			}()
		}

	}()
	go func() {
		for i := 0; i < expectedNumberOfSignSuccessOutput; i++ {
			go func() {
				version.AddSignOutput(ctx, []byte("data"), confutil.P(pldtypes.RandBytes32()), nil)
				countChanel <- true
			}()
		}
	}()
	go func() {
		for i := 0; i < expectedNumberOfSignErrorOutput; i++ {
			go func() {
				version.AddSignOutput(ctx, nil, nil, fmt.Errorf("error"))
				countChanel <- true
			}()
		}
	}()
	go func() {
		for i := 0; i < expectedNumberOfGasPriceSuccessOutput; i++ {
			go func() {
				version.AddGasPriceOutput(ctx, &pldapi.PublicTxGasPricing{
					MaxFeePerGas:         pldtypes.Uint64ToUint256(100),
					MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(10),
				}, nil)
				countChanel <- true
			}()
		}
	}()
	go func() {
		for i := 0; i < expectedNumberOfGasPriceErrorOutput; i++ {
			go func() {
				version.AddGasPriceOutput(ctx, nil, fmt.Errorf("error"))
				countChanel <- true
			}()
		}
	}()
	go func() {
		for i := 0; i < expectedNumberOfPanicOutput; i++ {
			go func() {
				version.AddPanicOutput(ctx, InFlightTxStageSubmitting)
				countChanel <- true
			}()
		}
	}()
	resultCount := 0

	// wait for all add output to complete
	for {
		select {
		case <-countChanel:
			resultCount++
		case <-ctx.Done():
			return
		}
		if resultCount == totalOutputAdd {
			break
		}
	}

	var actualNumberOfPersistenceSuccessOutput,
		actualNumberOfPersistenceErrorOutput,
		actualNumberOfSubmitSuccessOutput,
		actualNumberOfSubmitErrorOutput,
		actualNumberOfSignSuccessOutput,
		actualNumberOfSignErrorOutput,
		actualNumberOfGasPriceSuccessOutput,
		actualNumberOfGasPriceErrorOutput,
		actualNumberOfConfirmationsOutput,
		actualNumberOfPanicOutput int

	version.ProcessStageOutputs(ctx, func(stageOutputs []*StageOutput) (unprocessedStageOutputs []*StageOutput) {
		unprocessedStageOutputs = make([]*StageOutput, 0)
		for _, stageOutput := range stageOutputs {
			if stageOutput.ConfirmationOutput != nil {
				actualNumberOfConfirmationsOutput++
			} else if stageOutput.GasPriceOutput != nil {
				if stageOutput.GasPriceOutput.Err == nil {
					actualNumberOfGasPriceSuccessOutput++
				} else {
					actualNumberOfGasPriceErrorOutput++
				}
			} else if stageOutput.SubmitOutput != nil {
				if stageOutput.SubmitOutput.Err == nil {
					actualNumberOfSubmitSuccessOutput++
				} else {
					actualNumberOfSubmitErrorOutput++
				}
			} else if stageOutput.SignOutput != nil {
				if stageOutput.SignOutput.Err == nil {
					actualNumberOfSignSuccessOutput++
				} else {
					actualNumberOfSignErrorOutput++
				}
			} else if stageOutput.PersistenceOutput != nil {
				if stageOutput.PersistenceOutput.PersistenceError == nil {
					actualNumberOfPersistenceSuccessOutput++
				} else {
					actualNumberOfPersistenceErrorOutput++
				}
			} else {
				actualNumberOfPanicOutput++
				// pretend we don't want to process the panic out
				unprocessedStageOutputs = append(unprocessedStageOutputs, stageOutput)
			}
		}
		assert.Equal(t, expectedNumberOfPanicOutput, actualNumberOfPanicOutput)
		assert.Equal(t, expectedNumberOfGasPriceSuccessOutput, actualNumberOfGasPriceSuccessOutput)
		assert.Equal(t, expectedNumberOfGasPriceErrorOutput, actualNumberOfGasPriceErrorOutput)
		assert.Equal(t, expectedNumberOfSignSuccessOutput, actualNumberOfSignSuccessOutput)
		assert.Equal(t, expectedNumberOfSignErrorOutput, actualNumberOfSignErrorOutput)
		assert.Equal(t, expectedNumberOfSubmitSuccessOutput, actualNumberOfSubmitSuccessOutput)
		assert.Equal(t, expectedNumberOfSubmitErrorOutput, actualNumberOfSubmitErrorOutput)
		assert.Equal(t, expectedNumberOfPersistenceSuccessOutput, actualNumberOfPersistenceSuccessOutput)
		assert.Equal(t, expectedNumberOfPersistenceErrorOutput, actualNumberOfPersistenceErrorOutput)
		return
	})

	version.ProcessStageOutputs(ctx, func(stageOutputs []*StageOutput) (unprocessedStageOutputs []*StageOutput) {
		// only panic outputs aren't processed
		assert.Equal(t, expectedNumberOfPanicOutput, len(stageOutputs))
		return
	})
}

func TestStateManagerTxPersistenceManagementUpdateErrors(t *testing.T) {
	ctx := context.Background()
	testStateVersionWithMocks, m, done := newTestInFlightTransactionStateVersion(t)
	defer done()

	version := testStateVersionWithMocks.version

	// cannot persist when there is no running context
	_, _, err := version.PersistTxState(ctx)
	assert.NotNil(t, err)
	assert.Regexp(t, "PD011918", err)

	// set a running context
	version.StartNewStageContext(ctx, InFlightTxStageQueued, BaseTxSubStatusTracking)

	// cannot persist when running context hasn't set persistence output
	_, _, err = version.PersistTxState(ctx)
	assert.NotNil(t, err)
	assert.Regexp(t, "PD011918", err)

	rsc := version.GetRunningStageContext(ctx)
	rsc.SetNewPersistenceUpdateOutput()
	// now test different combinations of persistence
	_, _, err = version.PersistTxState(ctx)
	assert.Nil(t, err)

	rsc.StageOutputsToBePersisted.TxUpdates = &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			NewSubmission: &DBPubTxnSubmission{
				from:            "0x12345",
				TransactionHash: pldtypes.RandBytes32(),
			},
		},
	}

	m.db.ExpectBegin()
	m.db.ExpectExec("INSERT.*public_submissions").WillReturnError(fmt.Errorf("pop"))
	m.db.ExpectRollback()
	_, _, err = version.PersistTxState(ctx)
	assert.NotNil(t, err)
	assert.Regexp(t, "pop", err)

}

func TestStateVersionCancelAndIsCancelled(t *testing.T) {
	ctx := context.Background()
	testStateVersionWithMocks, _, done := newTestInFlightTransactionStateVersion(t)
	defer done()
	version := testStateVersionWithMocks.version

	// Initially not cancelled
	assert.False(t, version.IsCancelled(ctx))

	// Cancel it
	version.Cancel(ctx)
	assert.True(t, version.IsCancelled(ctx))

	// IsCancelled consumes the cancel signal, so calling it again should return false
	assert.False(t, version.IsCancelled(ctx))

	// Cancel again (this should hit the default case when channel is full)
	version.Cancel(ctx)
	version.Cancel(ctx) // Second cancel should hit default case
	assert.True(t, version.IsCancelled(ctx))
}

func TestStateVersionSetCurrentAndIsCurrent(t *testing.T) {
	ctx := context.Background()
	testStateVersionWithMocks, _, done := newTestInFlightTransactionStateVersion(t)
	defer done()
	version := testStateVersionWithMocks.version

	// Initially should be current
	assert.True(t, version.IsCurrent(ctx))

	// Set to not current
	version.SetCurrent(ctx, false)
	assert.False(t, version.IsCurrent(ctx))

	// Set back to current
	version.SetCurrent(ctx, true)
	assert.True(t, version.IsCurrent(ctx))
}

func TestStateVersionStartNewStageContext_EmptyStage(t *testing.T) {
	ctx := context.Background()
	testStateVersionWithMocks, _, done := newTestInFlightTransactionStateVersion(t)
	defer done()
	version := testStateVersionWithMocks.version
	mockActionTriggers := testStateVersionWithMocks.mAT
	defer mockActionTriggers.AssertExpectations(t)

	// Start with empty stage (first stage transition)
	// This should not record metrics since v.stage is ""
	mockActionTriggers.On("TriggerRetrieveGasPrice", mock.Anything).Return(nil).Once()
	version.StartNewStageContext(ctx, InFlightTxStageRetrieveGasPrice, BaseTxSubStatusReceived)
	assert.Equal(t, InFlightTxStageRetrieveGasPrice, version.GetStage(ctx))
	assert.Nil(t, version.GetStageTriggerError(ctx))
}

func TestStateVersionStartNewStageContext_SameStage(t *testing.T) {
	ctx := context.Background()
	testStateVersionWithMocks, _, done := newTestInFlightTransactionStateVersion(t)
	defer done()
	version := testStateVersionWithMocks.version
	mockActionTriggers := testStateVersionWithMocks.mAT
	defer mockActionTriggers.AssertExpectations(t)

	// Start a stage
	mockActionTriggers.On("TriggerRetrieveGasPrice", mock.Anything).Return(nil).Once()
	version.StartNewStageContext(ctx, InFlightTxStageRetrieveGasPrice, BaseTxSubStatusReceived)
	stageStartTime1 := version.GetStageStartTime(ctx)

	// Start the same stage again (should hit the else branch at line 150)
	// Note: Even when the stage is the same, the action is still triggered
	mockActionTriggers.On("TriggerRetrieveGasPrice", mock.Anything).Return(nil).Once()
	version.StartNewStageContext(ctx, InFlightTxStageRetrieveGasPrice, BaseTxSubStatusReceived)
	stageStartTime2 := version.GetStageStartTime(ctx)
	// Stage start time should not change when staying on same stage
	assert.Equal(t, stageStartTime1, stageStartTime2)
	assert.Equal(t, InFlightTxStageRetrieveGasPrice, version.GetStage(ctx))
}

func TestStateVersionStartNewStageContext_DefaultCase(t *testing.T) {
	ctx := context.Background()
	testStateVersionWithMocks, _, done := newTestInFlightTransactionStateVersion(t)
	defer done()
	version := testStateVersionWithMocks.version

	// Test default case with an unknown stage (like InFlightTxStageComplete or InFlightTxStageQueued)
	version.StartNewStageContext(ctx, InFlightTxStageComplete, BaseTxSubStatusReceived)
	assert.Equal(t, InFlightTxStageComplete, version.GetStage(ctx))
	assert.Nil(t, version.GetStageTriggerError(ctx)) // No error, just no action triggered
}

func TestStateVersionStartNewStageContext_SubmittingWithoutTransientOutputs(t *testing.T) {
	ctx := context.Background()
	testStateVersionWithMocks, _, done := newTestInFlightTransactionStateVersion(t)
	defer done()
	version := testStateVersionWithMocks.version
	mockActionTriggers := testStateVersionWithMocks.mAT
	defer mockActionTriggers.AssertExpectations(t)

	// Test submitting without TransientPreviousStageOutputs (nil case at line 166)
	var nilBytes []byte
	var nilHash *pldtypes.Bytes32
	mockActionTriggers.On("TriggerSubmitTx", mock.Anything, nilBytes, nilHash, mock.Anything).Return(nil).Once()
	version.StartNewStageContext(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived)
	assert.Nil(t, version.GetStageTriggerError(ctx))
}

func TestStateVersionStartNewStageContext_SubmittingWithToAddress(t *testing.T) {
	ctx := context.Background()
	testStateVersionWithMocks, _, done := newTestInFlightTransactionStateVersion(t)
	defer done()
	version := testStateVersionWithMocks.version
	mockActionTriggers := testStateVersionWithMocks.mAT
	defer mockActionTriggers.AssertExpectations(t)

	// Set up transient outputs
	testSignedData := []byte("test signed data")
	testHash := confutil.P(pldtypes.RandBytes32())
	version.SetTransientPreviousStageOutputs(&TransientPreviousStageOutputs{
		SignedMessage:   testSignedData,
		TransactionHash: testHash,
	})

	// The test setup has a To address set, so we need to match it
	// Get the actual To address from the in-memory tx through the InMemoryTxStateManager interface
	toAddress := version.(InMemoryTxStateManager).GetTo()
	var toAddressStr string
	if toAddress != nil {
		toAddressStr = toAddress.String()
	}
	mockActionTriggers.On("TriggerSubmitTx", mock.Anything, testSignedData, testHash, toAddressStr).Return(nil).Once()
	version.StartNewStageContext(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived)
	assert.Nil(t, version.GetStageTriggerError(ctx))
}

func TestStateVersionClearRunningStageContext_NoContext(t *testing.T) {
	ctx := context.Background()
	testStateVersionWithMocks, _, done := newTestInFlightTransactionStateVersion(t)
	defer done()
	version := testStateVersionWithMocks.version

	// Clear when there's no running context (should hit else branch at line 188)
	version.ClearRunningStageContext(ctx)
	assert.Nil(t, version.GetRunningStageContext(ctx))
}

func TestStateVersionAddStageOutputs_NoEventMode(t *testing.T) {
	ctx := context.Background()
	_, balanceManager, ptm, _, done := newTestBalanceManager(t)
	defer done()

	metrics := metrics.InitMetrics(context.Background(), prometheus.NewRegistry())
	mockInMemoryState := NewTestInMemoryTxState(t)
	mockActionTriggers := publictxmgrmocks.NewInFlightStageActionTriggers(t)

	// Create version with testOnlyNoEventMode = true
	v := NewInFlightTransactionStateGeneration(metrics, balanceManager, mockActionTriggers, mockInMemoryState, ptm, ptm.submissionWriter, true)
	version := v.(*inFlightTransactionStateGeneration)

	// Add stage output - should return early due to testOnlyNoEventMode
	version.AddStageOutputs(ctx, &StageOutput{
		Stage: InFlightTxStageQueued,
	})

	// Process outputs - should be empty since nothing was added
	version.ProcessStageOutputs(ctx, func(stageOutputs []*StageOutput) (unprocessedStageOutputs []*StageOutput) {
		assert.Empty(t, stageOutputs, "Should be empty in no event mode")
		return stageOutputs
	})
}

func TestStateVersionPersistTxState_StatusUpdateError(t *testing.T) {
	ctx := context.Background()
	testStateVersionWithMocks, _, done := newTestInFlightTransactionStateVersion(t)
	defer done()
	version := testStateVersionWithMocks.version

	// Set up a running context with status updates that will error
	version.StartNewStageContext(ctx, InFlightTxStageQueued, BaseTxSubStatusTracking)
	rsc := version.GetRunningStageContext(ctx)
	rsc.SetNewPersistenceUpdateOutput()

	// Add a status update that will return an error
	rsc.StageOutputsToBePersisted.StatusUpdates = []func(StatusUpdater) error{
		func(StatusUpdater) error {
			return fmt.Errorf("status update error")
		},
	}

	_, _, err := version.PersistTxState(ctx)
	assert.NotNil(t, err)
	assert.Regexp(t, "status update error", err)
}

func TestStateVersionPersistTxState_NewSubmissionWithBindingAlreadySet(t *testing.T) {
	ctx := context.Background()
	testStateVersionWithMocks, m, done := newTestInFlightTransactionStateVersion(t)
	defer done()
	version := testStateVersionWithMocks.version

	// Set up a running context
	version.StartNewStageContext(ctx, InFlightTxStageQueued, BaseTxSubStatusTracking)
	rsc := version.GetRunningStageContext(ctx)
	rsc.SetNewPersistenceUpdateOutput()

	// Create a new submission with binding already set (should skip building binding at line 303)
	existingTxHash := pldtypes.RandBytes32()
	existingBinding := &pldapi.PublicTx{
		TransactionHash: &existingTxHash,
	}
	rsc.StageOutputsToBePersisted.TxUpdates = &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			NewSubmission: &DBPubTxnSubmission{
				from:            "0x12345",
				TransactionHash: pldtypes.RandBytes32(),
				SequencerTXReference: SequencerTXReference{
					Binding: existingBinding,
				},
			},
		},
	}

	m.db.ExpectBegin()
	m.db.ExpectExec("INSERT.*public_submissions").WillReturnResult(sqlmock.NewResult(1, 1))
	m.db.ExpectCommit()

	_, _, err := version.PersistTxState(ctx)
	assert.Nil(t, err)
}

func TestStateVersionPersistTxState_ConfirmReceivedStatus(t *testing.T) {
	ctx := context.Background()
	testStateVersionWithMocks, m, done := newTestInFlightTransactionStateVersion(t)
	defer done()
	version := testStateVersionWithMocks.version

	// Set up a running context
	version.StartNewStageContext(ctx, InFlightTxStageQueued, BaseTxSubStatusTracking)
	rsc := version.GetRunningStageContext(ctx)
	rsc.SetNewPersistenceUpdateOutput()

	// Set InFlightStatus to ConfirmReceived
	confirmReceived := InFlightStatusConfirmReceived
	rsc.StageOutputsToBePersisted.TxUpdates = &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			InFlightStatus: &confirmReceived,
		},
	}

	m.db.ExpectBegin()
	m.db.ExpectCommit()

	_, _, err := version.PersistTxState(ctx)
	assert.Nil(t, err)
}

func TestStateVersionPersistTxState_WithFixedGasPrice(t *testing.T) {
	ctx := context.Background()
	testStateVersionWithMocks, m, done := newTestInFlightTransactionStateVersion(t)
	defer done()
	version := testStateVersionWithMocks.version

	// Set up a running context
	version.StartNewStageContext(ctx, InFlightTxStageQueued, BaseTxSubStatusTracking)
	rsc := version.GetRunningStageContext(ctx)
	rsc.SetNewPersistenceUpdateOutput()

	// Set fixed gas pricing on the in-memory transaction
	fixedMaxFeePerGas := pldtypes.Uint64ToUint256(50000)
	fixedMaxPriorityFeePerGas := pldtypes.Uint64ToUint256(5000)
	fixedGasPricing := pldapi.PublicTxGasPricing{
		MaxFeePerGas:         fixedMaxFeePerGas,
		MaxPriorityFeePerGas: fixedMaxPriorityFeePerGas,
	}

	// Update the transaction with fixed gas pricing
	imtxs := version.(InMemoryTxStateManager)
	updatedTx := &DBPublicTxn{
		FixedGasPricing: pldtypes.JSONString(fixedGasPricing),
	}
	imtxs.UpdateTransaction(ctx, updatedTx)

	// Verify that GetTransactionFixedGasPrice returns the fixed gas price
	fixedPrice := imtxs.GetTransactionFixedGasPrice()
	assert.NotNil(t, fixedPrice)
	assert.Equal(t, fixedMaxFeePerGas.Int(), fixedPrice.MaxFeePerGas.Int())
	assert.Equal(t, fixedMaxPriorityFeePerGas.Int(), fixedPrice.MaxPriorityFeePerGas.Int())

	// Create a new submission without binding - it will be built and should use fixed gas price
	txHash := pldtypes.RandBytes32()
	privateTXID := uuid.New()
	rsc.StageOutputsToBePersisted.TxUpdates = &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			NewSubmission: &DBPubTxnSubmission{
				from:            "0x12345",
				TransactionHash: txHash,
				SequencerTXReference: SequencerTXReference{
					PrivateTXID: privateTXID,
					Binding:     nil, // Will be built
				},
			},
		},
	}

	// The binding will be built from the in-memory tx
	// When GetTransactionFixedGasPrice() is not nil, it should set PublicTxGasPricing on the binding
	m.db.ExpectBegin()
	m.db.ExpectExec("INSERT.*public_submissions").WillReturnResult(sqlmock.NewResult(1, 1))
	m.db.ExpectCommit()

	_, _, err := version.PersistTxState(ctx)
	assert.Nil(t, err)

	// Verify that the binding was built and has the fixed gas pricing set
	// The binding should have PublicTxGasPricing set from the fixed gas price
	assert.NotNil(t, rsc.StageOutputsToBePersisted.TxUpdates.NewValues.NewSubmission.SequencerTXReference.Binding)
	assert.NotNil(t, rsc.StageOutputsToBePersisted.TxUpdates.NewValues.NewSubmission.SequencerTXReference.Binding.PublicTxGasPricing)
	assert.Equal(t, fixedMaxFeePerGas.Int(), rsc.StageOutputsToBePersisted.TxUpdates.NewValues.NewSubmission.SequencerTXReference.Binding.PublicTxGasPricing.MaxFeePerGas.Int())
	assert.Equal(t, fixedMaxPriorityFeePerGas.Int(), rsc.StageOutputsToBePersisted.TxUpdates.NewValues.NewSubmission.SequencerTXReference.Binding.PublicTxGasPricing.MaxPriorityFeePerGas.Int())
}
