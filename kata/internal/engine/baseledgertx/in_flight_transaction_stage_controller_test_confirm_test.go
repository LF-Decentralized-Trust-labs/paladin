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
	baseTypes "github.com/kaleido-io/paladin/kata/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestProduceLatestInFlightStageContextConfirming(t *testing.T) {
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

	// move to confirming
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageReceipting, time.Now(), nil)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	assert.Equal(t, baseTypes.InFlightTxStageConfirming, inFlightStageMananger.stage)

	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	defer mTS.AssertExpectations(t)

	testConfirmation := &baseTypes.ConfirmationsNotification{
		Confirmed: false,
		NewFork:   true,
		Confirmations: []*baseTypes.Confirmation{
			{BlockNumber: fftypes.FFuint64(12)},
		},
	}

	// confirmation already persisting
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	rsc := it.stateManager.GetRunningStageContext(ctx)
	rsc.SetNewPersistenceUpdateOutput()
	it.stateManager.AddConfirmationsOutput(ctx, testConfirmation)
	assert.GreaterOrEqual(t, len(inFlightStageMananger.bufferedStageOutputs), 1)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	// check the queued confirmation is still there
	assert.GreaterOrEqual(t, len(inFlightStageMananger.bufferedStageOutputs), 1)

	// confirmation needs to be persisted
	rsc = it.stateManager.GetRunningStageContext(ctx)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, testConfirmation, rsc.StageOutputsToBePersisted.Confirmations)

	// persisting error waiting for persistence retry timeout
	rsc.StageErrored = false
	it.persistenceRetryTimeout = 5 * time.Second
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageConfirming, time.Now().Add(it.persistenceRetryTimeout*2), fmt.Errorf("persist confirmation error"))
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
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageConfirming, time.Now(), fmt.Errorf("persist confirmation error"))
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	it.persistenceRetryTimeout = 5 * time.Second

	// persisted stage success and wait for more confirmations
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageConfirming, time.Now(), nil)
	rsc.StageErrored = false
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.Nil(t, rsc.StageOutputsToBePersisted)
}

func TestProduceLatestInFlightStageContextConfirmingTxFailed(t *testing.T) {
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
	mtx := it.stateManager.GetTx()
	// move to confirming
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageReceipting, time.Now(), nil)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	assert.Equal(t, baseTypes.InFlightTxStageConfirming, inFlightStageMananger.stage)
	mEN := testInFlightTransactionStateManagerWithMocks.mEN

	rsc := it.stateManager.GetRunningStageContext(ctx)
	// persisted stage success and transaction completed with error
	notifyMock := mEN.On("Notify", ctx, mock.Anything)

	notifyMock.Run(func(args mock.Arguments) {
		transactionEvent := args[1].(baseTypes.ManagedTransactionEvent)
		assert.Equal(t, baseTypes.ManagedTXProcessFailed, transactionEvent.Type)
		notifyMock.Return(nil)
	}).Once()
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageConfirming, time.Now(), nil)
	mtx.Status = baseTypes.BaseTxStatusFailed
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	// switched running stage context
	assert.NotEqual(t, rsc, it.stateManager.GetRunningStageContext(ctx))

}

func TestProduceLatestInFlightStageContextConfirmingTxSucceeded(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	mtx := it.stateManager.GetTx()
	mEN := testInFlightTransactionStateManagerWithMocks.mEN
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

	// move to confirming
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageReceipting, time.Now(), nil)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	assert.Equal(t, baseTypes.InFlightTxStageConfirming, inFlightStageMananger.stage)

	rsc := it.stateManager.GetRunningStageContext(ctx)
	// persisted stage success and transaction completed without error
	notifyMock := mEN.On("Notify", ctx, mock.Anything)

	notifyMock.Run(func(args mock.Arguments) {
		transactionEvent := args[1].(baseTypes.ManagedTransactionEvent)
		assert.Equal(t, baseTypes.ManagedTXProcessSucceeded, transactionEvent.Type)
		notifyMock.Return(nil)
	}).Once()
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageConfirming, time.Now(), nil)
	mtx.Status = baseTypes.BaseTxStatusSucceeded
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
	assert.NotEqual(t, rsc, inFlightStageMananger.GetRunningStageContext(ctx))

}
func TestProduceLatestInFlightStageContextConfirmingPanic(t *testing.T) {
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

	// move to confirming
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageReceipting, time.Now(), nil)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	assert.Equal(t, baseTypes.InFlightTxStageConfirming, inFlightStageMananger.stage)

	// mTS := testInFlightTransactionStateManagerWithMocks.mTS
	// defer mTS.AssertExpectations(t)

	// unexpected error
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPanicOutput(ctx, baseTypes.InFlightTxStageConfirming)
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

func TestProduceLatestInFlightStageContextSanityChecksForCompletedTransactions(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)

	mtx := it.stateManager.GetTx()
	mtx.Status = baseTypes.BaseTxStatusSucceeded
	testReceipt := &ethclient.TransactionReceiptResponse{
		BlockNumber: fftypes.NewFFBigInt(2),
		ProtocolID:  "0000/0001",
	}

	imtxs.Receipt = testReceipt
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.Empty(t, tOut.Cost) // cost for completed transaction should be 0
	assert.True(t, tOut.TransactionSubmitted)

}
