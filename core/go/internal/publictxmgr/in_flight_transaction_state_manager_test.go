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
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/mocks/ethclientmocks"
	"github.com/kaleido-io/paladin/core/mocks/publictxmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type testInFlightTransactionStateManagerWithMocks struct {
	stateManager    InFlightTransactionStateManager
	mEC             *ethclientmocks.EthClient
	mBI             *componentmocks.BlockIndexer
	mBM             BalanceManager
	mAT             *publictxmocks.InFlightStageActionTriggers
	inMemoryTxState InMemoryTxStateManager
}

func newTestInFlightTransactionStateManager(t *testing.T) (*testInFlightTransactionStateManagerWithMocks, *mocksAndTestControl, func()) {
	_, balanceManager, ble, m, done := newTestBalanceManager(t, false)

	mockInMemoryState := NewTestInMemoryTxState(t)
	mockActionTriggers := publictxmocks.NewInFlightStageActionTriggers(t)
	iftxs := NewInFlightTransactionStateManager(&publicTxEngineMetrics{}, balanceManager, m.blockIndexer, mockActionTriggers, mockInMemoryState,
		retry.NewRetryIndefinite(&pldconf.RetryConfig{
			InitialDelay: confutil.P("1ms"),
			MaxDelay:     confutil.P("100ms"),
			Factor:       confutil.P(2.0),
		}), ble, ble.submissionWriter, false)
	return &testInFlightTransactionStateManagerWithMocks{
		iftxs,
		m.ethClient,
		m.blockIndexer,
		balanceManager,
		mockActionTriggers,
		mockInMemoryState,
	}, m, done

}

func TestStateManagerStageManagementBasic(t *testing.T) {
	ctx := context.Background()
	testStateManagerWithMocks, _, done := newTestInFlightTransactionStateManager(t)
	defer done()
	stateManager := testStateManagerWithMocks.stateManager
	assert.Nil(t, stateManager.GetRunningStageContext(ctx))
	assert.Nil(t, stateManager.GetStageTriggerError(ctx))
	assert.Empty(t, stateManager.GetStage(ctx))
	assert.NotNil(t, stateManager.GetStageStartTime(ctx))
	assert.False(t, stateManager.ValidatedTransactionHashMatchState(ctx))
	stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	assert.True(t, stateManager.ValidatedTransactionHashMatchState(ctx))
	stateManager.SetValidatedTransactionHashMatchState(ctx, false)
	assert.False(t, stateManager.ValidatedTransactionHashMatchState(ctx))
}

func TestStateManagerStageManagementCanSubmit(t *testing.T) {
	ctx := context.Background()
	testStateManagerWithMocks, _, done := newTestInFlightTransactionStateManager(t)
	defer done()
	stateManager := testStateManagerWithMocks.stateManager
	stateManager.SetOrchestratorContext(ctx, &OrchestratorContext{
		PreviousNonceCostUnknown: false,
		// no available to spent provided, this means we don't need to check balance
	})
	assert.True(t, testStateManagerWithMocks.stateManager.CanSubmit(context.Background(), big.NewInt(0)))
	stateManager.SetOrchestratorContext(ctx, &OrchestratorContext{
		PreviousNonceCostUnknown: false,
		AvailableToSpend:         big.NewInt(30),
	})
	assert.True(t, testStateManagerWithMocks.stateManager.CanSubmit(context.Background(), big.NewInt(29)))
	assert.True(t, testStateManagerWithMocks.stateManager.CanSubmit(context.Background(), big.NewInt(30)))
	assert.False(t, testStateManagerWithMocks.stateManager.CanSubmit(context.Background(), big.NewInt(31)))
	assert.False(t, testStateManagerWithMocks.stateManager.CanSubmit(context.Background(), nil)) //unknown cost for the current transaction

	stateManager.SetOrchestratorContext(ctx, &OrchestratorContext{
		PreviousNonceCostUnknown: true,
		AvailableToSpend:         big.NewInt(30),
	})

	assert.False(t, testStateManagerWithMocks.stateManager.CanSubmit(context.Background(), big.NewInt(29)))

}

func TestStateManagerStageManagementTransactionFromRetrieveGasPriceToTracking(t *testing.T) {
	testStateManagerWithMocks, _, done := newTestInFlightTransactionStateManager(t)
	defer done()

	ctx := context.Background()
	stateManager := testStateManagerWithMocks.stateManager

	mockActionTriggers := testStateManagerWithMocks.mAT
	defer mockActionTriggers.AssertExpectations(t)
	// retrieve gas price errored
	mockActionTriggers.On("TriggerRetrieveGasPrice", mock.Anything).Return(fmt.Errorf("gasPriceError")).Once()
	stateManager.StartNewStageContext(ctx, InFlightTxStageRetrieveGasPrice, BaseTxSubStatusReceived)
	assert.Regexp(t, "gasPriceError", stateManager.GetStageTriggerError(ctx))
	// retrieve gas price success
	mockActionTriggers.On("TriggerRetrieveGasPrice", mock.Anything).Return(nil).Once()
	stateManager.StartNewStageContext(ctx, InFlightTxStageRetrieveGasPrice, BaseTxSubStatusReceived)
	assert.Nil(t, stateManager.GetStageTriggerError(ctx))

	var nilBytes []byte
	// scenario A: no signer configured, do submission
	mockActionTriggers.On("TriggerSubmitTx", mock.Anything, nilBytes).Return(nil).Once()

	stateManager.StartNewStageContext(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived)
	assert.Nil(t, stateManager.GetStageTriggerError(ctx))

	// scenario B: signer configured sign the data
	mockActionTriggers.On("TriggerSignTx", mock.Anything).Return(nil).Once()
	stateManager.StartNewStageContext(ctx, InFlightTxStageSigning, BaseTxSubStatusReceived)
	assert.Nil(t, stateManager.GetStageTriggerError(ctx))
	// persist the signed data as transient output
	testSignedData := []byte("test signed data")
	stateManager.SetTransientPreviousStageOutputs(&TransientPreviousStageOutputs{
		SignedMessage: testSignedData,
	})
	// do the submission
	mockActionTriggers.On("TriggerSubmitTx", mock.Anything, testSignedData).Return(nil)
	stateManager.StartNewStageContext(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived)
	assert.Nil(t, stateManager.GetStageTriggerError(ctx))

	// start tracking  no actions need to be triggered
	stateManager.StartNewStageContext(ctx, InFlightTxStageSubmitting, BaseTxSubStatusTracking)
	assert.Nil(t, stateManager.GetStageTriggerError(ctx))

	// test clear running stage context
	assert.NotNil(t, stateManager.GetRunningStageContext(ctx))
	stateManager.ClearRunningStageContext(ctx)
	assert.Nil(t, stateManager.GetRunningStageContext(ctx))
	// try to clear running stage context when there isn't one shouldn't throw error
	stateManager.ClearRunningStageContext(ctx)
	assert.Nil(t, stateManager.GetRunningStageContext(ctx))

}

func TestStateManagerStageOutputManagement(t *testing.T) {
	ctx := context.Background()
	testStateManagerWithMocks, _, done := newTestInFlightTransactionStateManager(t)
	defer done()
	stateManager := testStateManagerWithMocks.stateManager
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
				stateManager.AddPersistenceOutput(ctx, InFlightTxStageQueued, time.Now(), nil)
				countChanel <- true
			}()
		}
	}()
	go func() {
		for i := 0; i < expectedNumberOfPersistenceErrorOutput; i++ {
			go func() {
				stateManager.AddPersistenceOutput(ctx, InFlightTxStageQueued, time.Now(), fmt.Errorf("error"))
				countChanel <- true
			}()
		}

	}()
	go func() {
		for i := 0; i < expectedNumberOfSubmitSuccessOutput; i++ {
			go func() {
				stateManager.AddSubmitOutput(ctx, confutil.P(tktypes.Bytes32Keccak([]byte("0x000031"))), confutil.P(tktypes.TimestampNow()), SubmissionOutcomeAlreadyKnown, "", nil)
				countChanel <- true
			}()
		}

	}()
	go func() {
		for i := 0; i < expectedNumberOfSubmitErrorOutput; i++ {
			go func() {
				stateManager.AddSubmitOutput(ctx, nil, confutil.P(tktypes.TimestampNow()), SubmissionOutcomeFailedRequiresRetry, "error", fmt.Errorf("error"))
				countChanel <- true
			}()
		}

	}()
	go func() {
		for i := 0; i < expectedNumberOfSignSuccessOutput; i++ {
			go func() {
				stateManager.AddSignOutput(ctx, []byte("data"), confutil.P(tktypes.Bytes32(tktypes.RandBytes(32))), nil)
				countChanel <- true
			}()
		}
	}()
	go func() {
		for i := 0; i < expectedNumberOfSignErrorOutput; i++ {
			go func() {
				stateManager.AddSignOutput(ctx, nil, nil, fmt.Errorf("error"))
				countChanel <- true
			}()
		}
	}()
	go func() {
		for i := 0; i < expectedNumberOfGasPriceSuccessOutput; i++ {
			go func() {
				stateManager.AddGasPriceOutput(ctx, &pldapi.PublicTxGasPricing{
					GasPrice: tktypes.Int64ToInt256(100),
				}, nil)
				countChanel <- true
			}()
		}
	}()
	go func() {
		for i := 0; i < expectedNumberOfGasPriceErrorOutput; i++ {
			go func() {
				stateManager.AddGasPriceOutput(ctx, nil, fmt.Errorf("error"))
				countChanel <- true
			}()
		}
	}()
	go func() {
		for i := 0; i < expectedNumberOfPanicOutput; i++ {
			go func() {
				stateManager.AddPanicOutput(ctx, InFlightTxStageSubmitting)
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

	stateManager.ProcessStageOutputs(ctx, func(stageOutputs []*StageOutput) (unprocessedStageOutputs []*StageOutput) {
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

	stateManager.ProcessStageOutputs(ctx, func(stageOutputs []*StageOutput) (unprocessedStageOutputs []*StageOutput) {
		// only panic outputs aren't processed
		assert.Equal(t, expectedNumberOfPanicOutput, len(stageOutputs))
		return
	})
}

func TestStateManagerTxPersistenceManagementUpdateErrors(t *testing.T) {
	ctx := context.Background()
	testStateManagerWithMocks, m, done := newTestInFlightTransactionStateManager(t)
	defer done()

	stateManager := testStateManagerWithMocks.stateManager

	// cannot persist when there is no running context
	_, _, err := stateManager.PersistTxState(ctx)
	assert.NotNil(t, err)
	assert.Regexp(t, "PD011918", err)

	// set a running context
	stateManager.StartNewStageContext(ctx, InFlightTxStageQueued, BaseTxSubStatusTracking)

	// cannot persist when running context hasn't set persistence output
	_, _, err = stateManager.PersistTxState(ctx)
	assert.NotNil(t, err)
	assert.Regexp(t, "PD011918", err)

	rsc := stateManager.GetRunningStageContext(ctx)
	rsc.SetNewPersistenceUpdateOutput()
	// now test different combinations of persistence
	_, _, err = stateManager.PersistTxState(ctx)
	assert.Nil(t, err)

	inMemoryTxState := testStateManagerWithMocks.inMemoryTxState

	assert.Equal(t, stateManager.GetSignerNonce(), inMemoryTxState.GetSignerNonce())

	rsc.StageOutputsToBePersisted.TxUpdates = &BaseTXUpdates{
		NewSubmission: &DBPubTxnSubmission{
			SignerNonce:     "signer:12345",
			TransactionHash: tktypes.Bytes32(tktypes.RandBytes(32)),
		},
	}

	m.db.ExpectBegin()
	m.db.ExpectExec("INSERT.*public_submissions").WillReturnError(fmt.Errorf("pop"))
	m.db.ExpectRollback()
	_, _, err = stateManager.PersistTxState(ctx)
	assert.NotNil(t, err)
	assert.Regexp(t, "pop", err)

}
