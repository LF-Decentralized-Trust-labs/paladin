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

	"github.com/hyperledger/firefly-common/pkg/config"
	baseTypes "github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"
	"github.com/kaleido-io/paladin/kata/mocks/enginemocks"
	"github.com/stretchr/testify/assert"
)

type testInFlightTransactionWithMocksAndConf struct {
	it   *InFlightTransaction
	mCL  *enginemocks.TransactionConfirmationListener
	mEC  *componentmocks.EthClient
	mEN  *enginemocks.ManagedTxEventNotifier
	mTS  *enginemocks.TransactionStore
	mKM  *componentmocks.KeyManager
	mBM  baseTypes.BalanceManager
	conf config.Section
}

func NewTestInFlightTransactionWithMocks(t *testing.T) *testInFlightTransactionWithMocksAndConf {
	ctx := context.Background()
	imtxs := NewTestInMemoryTxState(t)
	enh, conf := NewTestEnterpriseTransactionHandler(t)
	mockBalanceManager, mEC, _ := NewTestBalanceManager(context.Background(), t)
	enh.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mTS := enginemocks.NewTransactionStore(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)
	mKM := componentmocks.NewKeyManager(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	enh.ctx = ctx
	enh.balanceManager = mockBalanceManager
	engineConf := conf.SubSection(TransactionEngineSection)
	te := NewTransactionEngine(enh, imtxs.GetTx(), engineConf)
	it := NewInFlightTransaction(enh, te, imtxs.GetTx())
	it.timeLineLoggingEnabled = true
	it.testOnlyNoActionMode = true
	return &testInFlightTransactionWithMocksAndConf{
		it:   it,
		mCL:  mCL,
		mEC:  mEC,
		mEN:  mEN,
		mTS:  mTS,
		mKM:  mKM,
		mBM:  mockBalanceManager,
		conf: conf,
	}
}

func TestProduceLatestInFlightStageContextTriggerStageError(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	// trigger retrieve gas price
	mtx := it.stateManager.GetTx()
	mtx.GasPrice = nil
	mtx.TransactionHash = ""

	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	assert.NotNil(t, it.stateManager.GetRunningStageContext(ctx))
	rsc := it.stateManager.GetRunningStageContext(ctx)

	assert.Equal(t, baseTypes.InFlightTxStageRetrieveGasPrice, rsc.Stage)

	inFlightStageManager := it.stateManager.(*inFlightTransactionState)
	inFlightStageManager.stageTriggerError = fmt.Errorf("trigger stage error")

	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	assert.NotEqual(t, rsc, it.stateManager.GetRunningStageContext(ctx))
	assert.Nil(t, inFlightStageManager.stageTriggerError) // check stage trigger error has been reset
}

func TestProduceLatestInFlightStageContextStatusChange(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	// trigger status change
	mtx := it.stateManager.GetTx()
	mtx.GasPrice = nil
	mtx.TransactionHash = ""
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	suspended := baseTypes.BaseTxStatusSuspended
	it.newStatus = &suspended
	iftxms := it.stateManager.(*inFlightTransactionState)
	iftxms.runningStageContext = NewRunningStageContext(ctx, baseTypes.InFlightTxStageStatusUpdate, baseTypes.BaseTxSubStatusReceived, iftxms)

	assert.NotNil(t, it.stateManager.GetRunningStageContext(ctx))
	rsc := it.stateManager.GetRunningStageContext(ctx)

	assert.Equal(t, baseTypes.InFlightTxStageStatusUpdate, rsc.Stage)

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// persisting error waiting for persistence retry timeout
	it.persistenceRetryTimeout = 5 * time.Second
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageStatusUpdate, time.Now().Add(it.persistenceRetryTimeout*2), fmt.Errorf("persist gas price error"))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	// persisting error retrying
	it.persistenceRetryTimeout = 0
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageStatusUpdate, time.Now(), fmt.Errorf("persist gas price error"))
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	it.persistenceRetryTimeout = 5 * time.Second

	// persisted stage success and move on
	mtx.Status = suspended
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageStatusUpdate, time.Now(), nil)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	assert.Nil(t, it.newStatus)
	// switched running stage context
	assert.NotEqual(t, rsc, it.stateManager.GetRunningStageContext(ctx))
}

func TestProduceLatestInFlightStageContextTriggerStatusUpdate(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	it.testOnlyNoActionMode = false
	it.testOnlyNoEventMode = false
	// trigger signing
	mtx := it.stateManager.GetTx()
	mtx.TransactionHash = ""
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	err := it.TriggerStatusUpdate(ctx)
	assert.NoError(t, err)
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	for len(inFlightStageMananger.bufferedStageOutputs) == 0 {
		// wait for event
	}
	assert.Len(t, inFlightStageMananger.bufferedStageOutputs, 1)
	assert.Nil(t, inFlightStageMananger.bufferedStageOutputs[0].PersistenceOutput) // panicked
}

func TestProduceLatestInFlightStageContextStatusUpdatePanic(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	// trigger status change
	mtx := it.stateManager.GetTx()
	mtx.GasPrice = nil
	mtx.TransactionHash = ""
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	suspend := baseTypes.BaseTxStatusSuspended
	it.newStatus = &suspend
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)

	assert.NotNil(t, it.stateManager.GetRunningStageContext(ctx))
	rsc := it.stateManager.GetRunningStageContext(ctx)

	assert.Equal(t, baseTypes.InFlightTxStageStatusUpdate, rsc.Stage)

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// unexpected error
	rsc = it.stateManager.GetRunningStageContext(ctx)
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPanicOutput(ctx, baseTypes.InFlightTxStageStatusUpdate)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Regexp(t, "PD011919", tOut.Error)
	assert.NotEqual(t, rsc, it.stateManager.GetRunningStageContext(ctx))
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
}
