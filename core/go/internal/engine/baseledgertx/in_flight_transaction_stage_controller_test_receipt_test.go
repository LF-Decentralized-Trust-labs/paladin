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
	"testing"
	"time"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	baseTypes "github.com/kaleido-io/paladin/core/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestProduceLatestInFlightStageContextReceipting(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	mtx := it.stateManager.GetTx()
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)

	assert.Equal(t, baseTypes.InFlightTxStageReceipting, inFlightStageMananger.stage)

	// receipt error
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddReceiptOutput(ctx, nil, fmt.Errorf("receipt error"))
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	rsc := it.stateManager.GetRunningStageContext(ctx)
	assert.True(t, rsc.StageErrored)

	// receipt received
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.Nil(t, rsc.StageOutputsToBePersisted)
	testReceipt := &ethclient.TransactionReceiptResponse{
		BlockNumber: fftypes.NewFFBigInt(2),
		ProtocolID:  "0000/0001",
	}
	it.stateManager.AddReceiptOutput(ctx, testReceipt, nil)
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionSubmitTransaction, fftypes.JSONAnyPtr(`{"protocolId":"`+testReceipt.ProtocolID+`"}`), (*fftypes.JSONAny)(nil), mock.Anything).Return(nil).Maybe()
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	assert.Equal(t, testReceipt, rsc.StageOutputsToBePersisted.Receipt)

	// persisting error waiting for persistence retry timeout
	rsc.StageErrored = false
	it.persistenceRetryTimeout = 5 * time.Second
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageReceipting, time.Now().Add(it.persistenceRetryTimeout*2), fmt.Errorf("persist receipt error"))
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)

	// persisting error retrying
	assert.False(t, rsc.StageErrored)
	it.persistenceRetryTimeout = 0
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageReceipting, time.Now(), fmt.Errorf("persist receipt error"))
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	it.persistenceRetryTimeout = 5 * time.Second

	// persisted stage success and move on
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageReceipting, time.Now(), nil)
	rsc.StageErrored = false
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	// switched running stage context
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.Equal(t, baseTypes.InFlightTxStageConfirming, rsc.Stage)
}

func TestProduceLatestInFlightStageContextReceiptingCheckExistingHashes(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	it.testOnlyNoActionMode = false
	it.testOnlyNoEventMode = true
	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)
	mtx := it.stateManager.GetTx()
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	// set existing transaction hashes
	oldHash := mtx.TransactionHash
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		SubmittedTxHashes: []string{oldHash, "hash1"},
	}
	called := make(chan bool, 3)
	mEC := it.ethClient.(*componentmocks.EthClient)
	mEC.On("GetTransactionReceipt", ctx, "hash1").Run(func(args mock.Arguments) {
		called <- true
	}).Return(nil, nil).Once()
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusTracking, baseTypes.BaseTxActionStateTransition, fftypes.JSONAnyPtr(`{"txHash":"hash1"}`), (*fftypes.JSONAny)(nil), mock.Anything).Return(nil).Maybe()
	persistenceCalled := make(chan bool, 3)
	mTS.On("UpdateTransaction", ctx, mtx.ID, mock.Anything).Run(func(args mock.Arguments) {
		persistenceCalled <- true
	}).Return(nil).Maybe()

	mCL := testInFlightTransactionStateManagerWithMocks.mCL
	addMock := mCL.On("Add", mock.Anything, mtx.ID, "hash1", mock.Anything, mock.Anything)
	mCL.On("Remove", mock.Anything, oldHash).Return(nil).Maybe()
	eventHandlerCalled := make(chan bool, 3)
	addMock.Run(func(args mock.Arguments) {
		addMock.Return(nil)
		eventHandlerCalled <- true
	}).Maybe()
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	<-called
	<-persistenceCalled
	<-eventHandlerCalled
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)

	assert.Equal(t, baseTypes.InFlightTxStageReceipting, inFlightStageMananger.stage)
}

func TestProduceLatestInFlightStageContextReceiptingCheckExistingHashesPersistenceFailure(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	it.testOnlyNoActionMode = false
	it.testOnlyNoEventMode = true
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)
	mtx := it.stateManager.GetTx()
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	// set existing transaction hashes
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		SubmittedTxHashes: []string{mtx.TransactionHash, "hash1"},
	}
	called := make(chan bool, 3)
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mEC.On("GetTransactionReceipt", ctx, "hash1").Run(func(args mock.Arguments) {
		called <- true
	}).Return(nil, nil).Once()
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusTracking, baseTypes.BaseTxActionStateTransition, fftypes.JSONAnyPtr(`{"txHash":"hash1"}`), (*fftypes.JSONAny)(nil), mock.Anything).Return(nil).Maybe()
	persistenceCalled := make(chan bool, 3)
	mTS.On("UpdateTransaction", ctx, mtx.ID, mock.Anything).Run(func(args mock.Arguments) {
		mtx.Status = baseTypes.BaseTxStatusFailed
		persistenceCalled <- true
	}).Return(fmt.Errorf("failed")).Maybe()

	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	<-called
	<-persistenceCalled
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
}

func TestProduceLatestInFlightStageContextReceiptingCheckExistingHashesTrackingFailure(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	it.testOnlyNoActionMode = false
	it.testOnlyNoEventMode = true
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)
	mtx := it.stateManager.GetTx()
	// set existing transaction hashes
	oldHash := mtx.TransactionHash
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		SubmittedTxHashes: []string{oldHash, "hash1"},
	}
	getReceiptCalled := make(chan bool, 3)
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mEC.On("GetTransactionReceipt", ctx, "hash1").Run(func(args mock.Arguments) {
		getReceiptCalled <- true
	}).Return(nil, nil).Once()
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusTracking, baseTypes.BaseTxActionStateTransition, fftypes.JSONAnyPtr(`{"txHash":"hash1"}`), (*fftypes.JSONAny)(nil), mock.Anything).Return(nil).Maybe()
	persistenceCalled := make(chan bool, 3)
	mTS.On("UpdateTransaction", ctx, mtx.ID, mock.Anything).Run(func(args mock.Arguments) {
		persistenceCalled <- true
	}).Return(nil).Maybe()

	mCL := testInFlightTransactionStateManagerWithMocks.mCL
	addMock := mCL.On("Add", mock.Anything, mtx.ID, "hash1", mock.Anything, mock.Anything)
	mCL.On("Remove", mock.Anything, oldHash).Return(nil).Maybe()
	eventHandlerCalled := make(chan bool, 3)
	addMock.Run(func(args mock.Arguments) {
		addMock.Return(fmt.Errorf("failed to add"))
		eventHandlerCalled <- true
	}).Maybe()
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	<-getReceiptCalled
	<-persistenceCalled
	<-eventHandlerCalled
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)

	assert.Equal(t, baseTypes.InFlightTxStageReceipting, inFlightStageMananger.stage)
}

func TestProduceLatestInFlightStageContextReceiptingExceededTimeout(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)

	assert.Equal(t, baseTypes.InFlightTxStageReceipting, inFlightStageMananger.stage)

	mCL := testInFlightTransactionStateManagerWithMocks.mCL
	mCL.On("Add", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	removeMock := mCL.On("Remove", mock.Anything, mock.Anything)
	removeMock.Run(func(args mock.Arguments) {
		removeMock.Return(nil)
	}).Maybe()
	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)
	// no receipt but last warn time expired
	expiredTime := fftypes.FFTime(fftypes.Now().Time().Add(-(it.resubmitInterval)))
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		LastWarnTime: &expiredTime,
	}

	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
}

func TestProduceLatestInFlightStageContextReceiptingExceededTimeoutIgnoreRemovalErrors(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)

	assert.Equal(t, baseTypes.InFlightTxStageReceipting, inFlightStageMananger.stage)
	mCL := testInFlightTransactionStateManagerWithMocks.mCL
	removeMock := mCL.On("Remove", mock.Anything, mock.Anything)
	mCL.On("Add", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	removeCalled := make(chan struct{})
	removeMock.Run(func(args mock.Arguments) {
		close(removeCalled)
		removeMock.Return(fmt.Errorf("should be ignored"))
	}).Once()
	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)

	// no receipt but last warn time expired
	expiredTime := fftypes.FFTime(fftypes.Now().Time().Add(-(it.resubmitInterval)).Add(-100 * time.Second))
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		LastWarnTime: &expiredTime,
	}

	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	<-removeCalled
	// goes into retrieve gas price stage
	assert.Equal(t, baseTypes.InFlightTxStageRetrieveGasPrice, inFlightStageMananger.stage)
}

func TestProduceLatestInFlightStageContextReceiptingErroredAndExceededStageTimeout(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)

	assert.Equal(t, baseTypes.InFlightTxStageReceipting, inFlightStageMananger.stage)
	// receipt errored and reached stage retry timeout
	it.stageRetryTimeout = 0
	rsc := it.stateManager.GetRunningStageContext(ctx)
	rsc.StageErrored = true
	assert.NotNil(t, rsc)
	assert.Equal(t, baseTypes.InFlightTxStageReceipting, rsc.Stage)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)

	assert.NotEqual(t, rsc, it.stateManager.GetRunningStageContext(ctx))
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.Equal(t, baseTypes.InFlightTxStageReceipting, rsc.Stage)
}

func TestProduceLatestInFlightStageContextReceiptPanic(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)

	assert.Equal(t, baseTypes.InFlightTxStageReceipting, inFlightStageMananger.stage)

	// mockMetrics := testInFlightTransactionStateManagerWithMocks.mockMetrics
	// mTS := testInFlightTransactionStateManagerWithMocks.mTS
	// defer mTS.AssertExpectations(t)

	// unexpected error
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPanicOutput(ctx, baseTypes.InFlightTxStageReceipting)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	assert.Regexp(t, "PD011919", tOut.Error)
	// re-enters tracking straight-away when panicked
	assert.Equal(t, baseTypes.InFlightTxStageReceipting, inFlightStageMananger.stage)
}
