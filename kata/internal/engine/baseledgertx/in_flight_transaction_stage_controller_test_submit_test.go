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
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	baseTypes "github.com/kaleido-io/paladin/kata/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestProduceLatestInFlightStageContextSubmitPanic(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	signedMsg := []byte("signedMessage")
	it.TriggerNewStageRun(ctx, baseTypes.InFlightTxStageSubmitting, baseTypes.BaseTxSubStatusReceived, signedMsg)
	rsc := it.stateManager.GetRunningStageContext(ctx)

	// unexpected error
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPanicOutput(ctx, baseTypes.InFlightTxStageSubmitting)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Regexp(t, "PD011919", tOut.Error)
	assert.NotEqual(t, rsc, it.stateManager.GetRunningStageContext(ctx))
	// rolled back to signing stage as per current design
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.Equal(t, baseTypes.InFlightTxStageSigning, rsc.Stage)

}

func TestProduceLatestInFlightStageContextSubmitComplete(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	signedMsg := []byte("signedMessage")
	txHash := "tx_hashnew"
	it.TriggerNewStageRun(ctx, baseTypes.InFlightTxStageSubmitting, baseTypes.BaseTxSubStatusReceived, signedMsg)
	rsc := it.stateManager.GetRunningStageContext(ctx)
	// submission attempt completed - new transaction submitted
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	mtx := it.stateManager.GetTx()
	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)
	mtx.TransactionHash = ""
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	mtx.FirstSubmit = nil
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		SubmittedTxHashes: []string{},
	}

	submissionTime := fftypes.Now()
	it.stateManager.AddSubmitOutput(ctx, txHash, submissionTime, baseTypes.SubmissionOutcomeSubmittedNew, ethclient.ErrorReason(""), nil)
	rsc.StageOutputsToBePersisted = nil
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.Equal(t, baseTypes.InFlightTxStageSubmitting, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.HistoryUpdates))
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionSubmitTransaction, fftypes.JSONAnyPtr(`{"txHash":"`+txHash+`"}`), (*fftypes.JSONAny)(nil), mock.Anything).Return(nil).Maybe()
	_ = rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
	assert.Equal(t, []string{txHash}, rsc.StageOutputsToBePersisted.PolicyInfo.SubmittedTxHashes)
	assert.Equal(t, submissionTime, rsc.StageOutputsToBePersisted.TxUpdates.FirstSubmit)
	assert.Equal(t, submissionTime, rsc.StageOutputsToBePersisted.TxUpdates.LastSubmit)

	// submission attempt completed - nonce too low
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	rsc = it.stateManager.GetRunningStageContext(ctx)
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{}
	it.stateManager.AddSubmitOutput(ctx, txHash, submissionTime, baseTypes.SubmissionOutcomeNonceTooLow, ethclient.ErrorReason(""), nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.Equal(t, baseTypes.InFlightTxStageSubmitting, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionSubmitTransaction, fftypes.JSONAnyPtr(`{"txHash":"`+txHash+`"}`), (*fftypes.JSONAny)(nil), mock.Anything).Return(nil).Maybe()
	_ = rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
	assert.Equal(t, []string{txHash}, rsc.StageOutputsToBePersisted.PolicyInfo.SubmittedTxHashes)
	assert.Equal(t, submissionTime, rsc.StageOutputsToBePersisted.TxUpdates.LastSubmit)
	assert.Equal(t, txHash, *rsc.StageOutputsToBePersisted.TxUpdates.TransactionHash)
}

func TestProduceLatestInFlightStageContextCannotSubmit(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	// Previous cost unknown when state is not validated
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	mtx := it.stateManager.GetTx()
	mtx.GasPrice = nil
	mtx.MaxFeePerGas = ethtypes.NewHexInteger64(32247127816)
	mtx.MaxPriorityFeePerGas = ethtypes.NewHexInteger64(32146027800)

	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)
	mtx.TransactionHash = ""
	mtx.FirstSubmit = nil
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		SubmittedTxHashes: []string{},
	}

	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         big.NewInt(0),
		PreviousNonceCostUnknown: true, // previous cost unknown, cannot submit
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "64494255632000", tOut.Cost.String())
	assert.False(t, tOut.TransactionSubmitted)

	// Previous cost unknown when state is validated
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	mtx.TransactionHash = "test"
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		SubmittedTxHashes: []string{},
	}
	mtx.GasLimit = ethtypes.NewHexInteger64(-1) // invalid limit
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, false)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         big.NewInt(0),
		PreviousNonceCostUnknown: true, // previous cost unknown, cannot submit
	})
	assert.NotEmpty(t, *tOut)
	assert.Nil(t, tOut.Cost) // cost cannot be calculated
	assert.True(t, tOut.TransactionSubmitted)
}
func TestProduceLatestInFlightStageContextSubmitCompleteAlreadyKnown(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	signedMsg := []byte("signedMessage")
	txHash := "tx_hashnew"
	it.TriggerNewStageRun(ctx, baseTypes.InFlightTxStageSubmitting, baseTypes.BaseTxSubStatusReceived, signedMsg)
	rsc := it.stateManager.GetRunningStageContext(ctx)
	// submission attempt completed - new transaction submitted
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	mtx := it.stateManager.GetTx()
	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)
	mtx.TransactionHash = ""
	mtx.FirstSubmit = nil
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		SubmittedTxHashes: []string{},
	}

	submissionTime := fftypes.Now()
	// // submission attempt completed - already known
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	rsc.StageOutputsToBePersisted = nil
	mtx.FirstSubmit = fftypes.Now()
	rsc = it.stateManager.GetRunningStageContext(ctx)
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		SubmittedTxHashes: []string{txHash},
	}
	it.stateManager.AddSubmitOutput(ctx, txHash, submissionTime, baseTypes.SubmissionOutcomeAlreadyKnown, ethclient.ErrorReason(""), nil)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.Equal(t, baseTypes.InFlightTxStageSubmitting, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Empty(t, rsc.StageOutputsToBePersisted.HistoryUpdates)
	assert.Equal(t, []string{txHash}, rsc.StageOutputsToBePersisted.PolicyInfo.SubmittedTxHashes)
}
func TestProduceLatestInFlightStageContextSubmitErrors(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	signedMsg := []byte("signedMessage")
	txHash := "tx_hashold"
	mTS := testInFlightTransactionStateManagerWithMocks.mTS

	it.TriggerNewStageRun(ctx, baseTypes.InFlightTxStageSubmitting, baseTypes.BaseTxSubStatusReceived, signedMsg)
	rsc := it.stateManager.GetRunningStageContext(ctx)

	mtx := it.stateManager.GetTx()
	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)
	mtx.TransactionHash = ""
	mtx.FirstSubmit = nil
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		SubmittedTxHashes: []string{},
	}

	submissionTime := fftypes.Now()
	submissionErr := fmt.Errorf("submission error")

	// submission attempt errored - required re-preparation
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	rsc.StageOutputsToBePersisted = nil
	rsc = it.stateManager.GetRunningStageContext(ctx)
	it.stateManager.AddSubmitOutput(ctx, txHash, submissionTime, baseTypes.SubmissionOutcomeFailedRequiresRetry, ethclient.ErrorReasonTransactionReverted, submissionErr)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})

	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.False(t, tOut.TransactionSubmitted)
	assert.Equal(t, baseTypes.InFlightTxStageSubmitting, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.HistoryUpdates))
	called := make(chan bool, 3)
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionSubmitTransaction, fftypes.JSONAnyPtr(`{"reason":"`+string(ethclient.ErrorReasonTransactionReverted)+`"}`), fftypes.JSONAnyPtr(`{"error":"`+submissionErr.Error()+`"}`), mock.Anything).Run(func(args mock.Arguments) {
		called <- true
	}).Return(nil).Maybe()
	_ = rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
	<-called
	assert.Equal(t, submissionErr.Error(), *rsc.StageOutputsToBePersisted.TxUpdates.ErrorMessage)
	assert.Equal(t, baseTypes.InFlightTxStageSubmitting, rsc.Stage)
	assert.Nil(t, rsc.StageOutputsToBePersisted.PolicyInfo)

	// submission attempt errored - required re-preparation during resubmission
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	rsc.StageOutputsToBePersisted = nil
	rsc = it.stateManager.GetRunningStageContext(ctx)
	newWarnTime := fftypes.Now()
	mtx.TransactionHash = txHash
	it.stateManager.AddSubmitOutput(ctx, "", newWarnTime, baseTypes.SubmissionOutcomeFailedRequiresRetry, ethclient.ErrorReasonTransactionReverted, submissionErr)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})

	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	assert.Equal(t, baseTypes.InFlightTxStageSubmitting, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.HistoryUpdates))
	called = make(chan bool, 3)
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionSubmitTransaction, fftypes.JSONAnyPtr(`{"reason":"`+string(ethclient.ErrorReasonTransactionReverted)+`"}`), fftypes.JSONAnyPtr(`{"error":"`+submissionErr.Error()+`"}`), mock.Anything).Run(func(args mock.Arguments) {
		called <- true
	}).Return(nil).Maybe()
	_ = rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
	<-called
	assert.Equal(t, submissionErr.Error(), *rsc.StageOutputsToBePersisted.TxUpdates.ErrorMessage)
	assert.Equal(t, baseTypes.InFlightTxStageSubmitting, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted.PolicyInfo)
	assert.NotNil(t, rsc.StageOutputsToBePersisted.PolicyInfo.LastWarnTime)

	// persisting error waiting for persistence retry timeout
	assert.False(t, rsc.StageErrored)
	it.persistenceRetryTimeout = 5 * time.Second
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageSubmitting, time.Now().Add(it.persistenceRetryTimeout*2), fmt.Errorf("persist signing sub-status error"))
	it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})

	// persisted stage error - required more funds
	rsc = it.stateManager.GetRunningStageContext(ctx)
	rsc.StageOutput = &baseTypes.StageOutput{
		SubmitOutput: &baseTypes.SubmitOutputs{
			SubmissionOutcome: baseTypes.SubmissionOutcomeFailedRequiresRetry,
			ErrorReason:       string(ethclient.ErrorReasonInsufficientFunds),
			Err:               fmt.Errorf("insufficient funds"),
		},
	}
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageSubmitting, time.Now(), nil)
	it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.True(t, rsc.StageErrored)

	// persisting error retrying
	it.persistenceRetryTimeout = 0
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageSubmitting, time.Now(), fmt.Errorf("persist submit error"))
	it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	it.persistenceRetryTimeout = 5 * time.Second

}

func TestProduceLatestInFlightStageContextSubmitRePrepare(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	signedMsg := []byte("signedMessage")

	it.TriggerNewStageRun(ctx, baseTypes.InFlightTxStageSubmitting, baseTypes.BaseTxSubStatusReceived, signedMsg)

	mtx := it.stateManager.GetTx()
	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)
	mtx.TransactionHash = ""
	mtx.FirstSubmit = nil
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		SubmittedTxHashes: []string{},
	}

	// persisted stage error - require re-preparation
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageSubmitting, time.Now(), nil)
	rsc := it.stateManager.GetRunningStageContext(ctx)
	rsc.StageOutput = &baseTypes.StageOutput{
		SubmitOutput: &baseTypes.SubmitOutputs{
			SubmissionOutcome: baseTypes.SubmissionOutcomeFailedRequiresRetry,
			ErrorReason:       string(ethclient.ErrorReasonInsufficientFunds),
			Err:               fmt.Errorf("insufficient funds"),
		},
	}
	it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.True(t, rsc.StageErrored)
	assert.Equal(t, baseTypes.InFlightTxStageSubmitting, inFlightStageMananger.stage)
}

func TestProduceLatestInFlightStageContextSubmitSuccess(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	signedMsg := []byte("signedMessage")

	it.TriggerNewStageRun(ctx, baseTypes.InFlightTxStageSubmitting, baseTypes.BaseTxSubStatusReceived, signedMsg)

	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		SubmittedTxHashes: []string{},
	}

	// persisted stage error - require re-preparation
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageSubmitting, time.Now(), nil)
	rsc := it.stateManager.GetRunningStageContext(ctx)
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, false)
	rsc.StageOutput = &baseTypes.StageOutput{
		SubmitOutput: &baseTypes.SubmitOutputs{
			SubmissionOutcome: baseTypes.SubmissionOutcomeSubmittedNew,
		},
	}
	it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})

	// switched to tracking
	assert.Equal(t, baseTypes.InFlightTxStageReceipting, inFlightStageMananger.stage)
	assert.True(t, it.stateManager.ValidatedTransactionHashMatchState(ctx))

}

func TestProduceLatestInFlightStageContextTriggerSubmit(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	it.testOnlyNoActionMode = false
	it.testOnlyNoEventMode = false
	// trigger signing
	mtx := it.stateManager.GetTx()
	mtx.TransactionHash = ""
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	called := make(chan struct{})

	sendRawTransactionMock := mEC.On("SendRawTransaction", ctx, mock.Anything)
	sendRawTransactionMock.Run(func(args mock.Arguments) {
		sendRawTransactionMock.Return(nil, fmt.Errorf("pop"))
		close(called)
	}).Once()
	err := it.TriggerSubmitTx(ctx, nil)
	assert.NoError(t, err)
	<-called
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	for len(inFlightStageMananger.bufferedStageOutputs) == 0 {
		// wait for event
	}
	assert.Len(t, inFlightStageMananger.bufferedStageOutputs, 1)
	assert.NotNil(t, inFlightStageMananger.bufferedStageOutputs[0].SubmitOutput)
	assert.NotNil(t, inFlightStageMananger.bufferedStageOutputs[0].SubmitOutput.Err)
	assert.Empty(t, inFlightStageMananger.bufferedStageOutputs[0].SubmitOutput.TxHash)
	assert.Empty(t, inFlightStageMananger.bufferedStageOutputs[0].SubmitOutput.ErrorReason)
	assert.NotEmpty(t, inFlightStageMananger.bufferedStageOutputs[0].SubmitOutput.SubmissionTime)
	assert.Equal(t, baseTypes.SubmissionOutcomeFailedRequiresRetry, inFlightStageMananger.bufferedStageOutputs[0].SubmitOutput.SubmissionOutcome)
}
