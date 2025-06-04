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

	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/core/mocks/publictxmocks"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type testInFlightTransactionStateVersionWithMocks struct {
	version InFlightTransactionStateGeneration
	mAT     *publictxmocks.InFlightStageActionTriggers
}

func newTestInFlightTransactionStateVersion(t *testing.T) (*testInFlightTransactionStateVersionWithMocks, *mocksAndTestControl, func()) {
	_, balanceManager, ptm, m, done := newTestBalanceManager(t)

	mockInMemoryState := NewTestInMemoryTxState(t)
	mockActionTriggers := publictxmocks.NewInFlightStageActionTriggers(t)

	v := NewInFlightTransactionStateGeneration(&publicTxEngineMetrics{}, balanceManager, mockActionTriggers, mockInMemoryState, ptm, ptm.submissionWriter, false)
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
	assert.False(t, version.ValidatedTransactionHashMatchState(ctx))
	version.SetValidatedTransactionHashMatchState(ctx, true)
	assert.True(t, version.ValidatedTransactionHashMatchState(ctx))
	version.SetValidatedTransactionHashMatchState(ctx, false)
	assert.False(t, version.ValidatedTransactionHashMatchState(ctx))
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
	mockActionTriggers.On("TriggerSubmitTx", mock.Anything, nilBytes, nilHash).Return(nil).Once()

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
	mockActionTriggers.On("TriggerSubmitTx", mock.Anything, testSignedData, testHash).Return(nil)
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
					GasPrice: pldtypes.Int64ToInt256(100),
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
		NewSubmission: &DBPubTxnSubmission{
			from:            "0x12345",
			TransactionHash: pldtypes.RandBytes32(),
		},
	}

	m.db.ExpectBegin()
	m.db.ExpectExec("INSERT.*public_submissions").WillReturnError(fmt.Errorf("pop"))
	m.db.ExpectRollback()
	_, _, err = version.PersistTxState(ctx)
	assert.NotNil(t, err)
	assert.Regexp(t, "pop", err)

}
