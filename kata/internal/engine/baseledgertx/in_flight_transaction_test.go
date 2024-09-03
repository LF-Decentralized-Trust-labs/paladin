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
	"testing"
	"time"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	baseTypes "github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"
	"github.com/kaleido-io/paladin/kata/mocks/enginemocks"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

func TestProduceLatestInFlightStageContextRetrieveGas(t *testing.T) {
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

	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	defer mTS.AssertExpectations(t)
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	retrievedGasPrice := &baseTypes.GasPriceObject{
		GasPrice: big.NewInt(10),
	}
	retrievedGasPriceJSON, _ := json.Marshal(retrievedGasPrice)
	// succeed retrieving gas price
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddGasPriceOutput(ctx, retrievedGasPrice, nil)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, retrievedGasPrice.GasPrice, rsc.StageOutputsToBePersisted.TxUpdates.GasPrice.BigInt())
	assert.Nil(t, rsc.StageOutputsToBePersisted.TxUpdates.MaxFeePerGas)
	assert.Nil(t, rsc.StageOutputsToBePersisted.TxUpdates.MaxPriorityFeePerGas)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.HistoryUpdates))
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionRetrieveGasPrice, fftypes.JSONAnyPtr(string(retrievedGasPriceJSON)), (*fftypes.JSONAny)(nil), mock.Anything).Return(nil).Maybe()
	rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
	// failed retrieving gas price
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddGasPriceOutput(ctx, nil, fmt.Errorf("gas retrieve error"))
	rsc = it.stateManager.GetRunningStageContext(ctx)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Nil(t, rsc.StageOutputsToBePersisted.TxUpdates)
	assert.GreaterOrEqual(t, len(rsc.StageOutputsToBePersisted.HistoryUpdates), 1)
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionRetrieveGasPrice, (*fftypes.JSONAny)(nil), fftypes.JSONAnyPtr(`{"error":"gas retrieve error"}`), mock.Anything).Return(nil).Maybe()

	// persisting error waiting for persistence retry timeout
	assert.False(t, rsc.StageErrored)
	it.persistenceRetryTimeout = 5 * time.Second
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageRetrieveGasPrice, time.Now().Add(it.persistenceRetryTimeout*2), fmt.Errorf("persist gas price error"))
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	// persisting error retrying
	assert.False(t, rsc.StageErrored)
	it.persistenceRetryTimeout = 0
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageRetrieveGasPrice, time.Now(), fmt.Errorf("persist gas price error"))
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	it.persistenceRetryTimeout = 5 * time.Second

	// persisted stage error
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageRetrieveGasPrice, time.Now(), nil)
	assert.NotNil(t, rsc.StageOutput.GasPriceOutput.Err)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	assert.True(t, rsc.StageErrored)

	// persisted stage success and move on
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageRetrieveGasPrice, time.Now(), nil)
	rsc.StageOutput.GasPriceOutput.Err = nil
	rsc.StageErrored = false
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	assert.False(t, it.stateManager.ValidatedTransactionHashMatchState(ctx))
	// switched running stage context
	assert.NotEqual(t, rsc, it.stateManager.GetRunningStageContext(ctx))
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

	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	defer mTS.AssertExpectations(t)
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

func TestProduceLatestInFlightStageContextRetrieveGasIncrements(t *testing.T) {
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

	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	defer mTS.AssertExpectations(t)
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	retrievedGasPrice := &baseTypes.GasPriceObject{
		GasPrice: big.NewInt(10),
	}
	it.gasPriceIncreasePercent = big.NewInt(50) // increase 50 percent

	mtx.GasPrice = ethtypes.NewHexInteger(big.NewInt(20))
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddGasPriceOutput(ctx, retrievedGasPrice, nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Equal(t, "40000", tOut.Cost.String())
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, big.NewInt(30), rsc.StageOutputsToBePersisted.TxUpdates.GasPrice.BigInt())
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.HistoryUpdates))
	called := make(chan bool, 3)
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionRetrieveGasPrice, mock.Anything, (*fftypes.JSONAny)(nil), mock.Anything).Run(func(args mock.Arguments) {
		called <- true
	}).Return(nil).Maybe()
	rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
	<-called
}

func TestProduceLatestInFlightStageContextRetrieveGasIncrementsReachedCap(t *testing.T) {
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

	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	retrievedGasPrice := &baseTypes.GasPriceObject{
		GasPrice: big.NewInt(10),
	}
	it.gasPriceIncreasePercent = big.NewInt(50) // increase 50 percent
	// when reached the max gas price cap
	mtx.GasPrice = ethtypes.NewHexInteger(big.NewInt(20))

	it.gasPriceIncreaseMax = big.NewInt(26)
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddGasPriceOutput(ctx, retrievedGasPrice, nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Equal(t, "40000", tOut.Cost.String())
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, big.NewInt(26), rsc.StageOutputsToBePersisted.TxUpdates.GasPrice.BigInt())
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.HistoryUpdates))
	called := make(chan bool, 3)
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionRetrieveGasPrice, mock.Anything, (*fftypes.JSONAny)(nil), mock.Anything).Run(func(args mock.Arguments) {
		called <- true
	}).Return(nil).Maybe()
	rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
	<-called
}

func TestProduceLatestInFlightStageContextRetrieveGasIncrementsRetrievedHigherPrice(t *testing.T) {
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

	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	defer mTS.AssertExpectations(t)
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	it.gasPriceIncreasePercent = big.NewInt(50) // increase 50 percent
	// retrieved price is higher
	mtx.GasPrice = ethtypes.NewHexInteger(big.NewInt(20))
	higherRetrievedPrice := &baseTypes.GasPriceObject{
		GasPrice: big.NewInt(21),
	}
	it.gasPriceIncreaseMax = big.NewInt(26)
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddGasPriceOutput(ctx, higherRetrievedPrice, nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Equal(t, "40000", tOut.Cost.String())
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, higherRetrievedPrice.GasPrice, rsc.StageOutputsToBePersisted.TxUpdates.GasPrice.BigInt())
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.HistoryUpdates))
	called := make(chan bool, 3)
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionRetrieveGasPrice, mock.Anything, (*fftypes.JSONAny)(nil), mock.Anything).Run(func(args mock.Arguments) {
		called <- true
	}).Return(nil).Maybe()
	rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
	<-called
}

func TestProduceLatestInFlightStageContextRetrieveGasIncrementsEIP1559HigherExistingPrice(t *testing.T) {
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

	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	defer mTS.AssertExpectations(t)
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	it.gasPriceIncreasePercent = big.NewInt(50) // increase 50 percent
	// EIP-1559 gas price
	retrievedGasPriceEIP1559 := &baseTypes.GasPriceObject{
		MaxFeePerGas:         big.NewInt(10),
		MaxPriorityFeePerGas: big.NewInt(1),
	}
	it.gasPriceIncreaseMax = nil
	// the highest gas price used is higher than the retrieved gas price
	mtx.MaxFeePerGas = ethtypes.NewHexInteger64(20)
	mtx.MaxPriorityFeePerGas = ethtypes.NewHexInteger64(1)

	it.gasPriceClient = NewTestFixedPriceGasPriceClientEIP1559(t)
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddGasPriceOutput(ctx, retrievedGasPriceEIP1559, nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Equal(t, "40000", tOut.Cost.String())
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, big.NewInt(30), rsc.StageOutputsToBePersisted.TxUpdates.MaxFeePerGas.BigInt())
	assert.Equal(t, big.NewInt(1), rsc.StageOutputsToBePersisted.TxUpdates.MaxPriorityFeePerGas.BigInt())
	assert.GreaterOrEqual(t, len(rsc.StageOutputsToBePersisted.HistoryUpdates), 1)
	called := make(chan bool, 3)
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionRetrieveGasPrice, mock.Anything, (*fftypes.JSONAny)(nil), mock.Anything).Run(func(args mock.Arguments) {
		called <- true
	}).Return(nil).Maybe()
	rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
	<-called
}
func TestProduceLatestInFlightStageContextRetrieveGasIncrementsEIP1559MismatchFormat(t *testing.T) {
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

	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	defer mTS.AssertExpectations(t)
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	retrievedGasPrice := &baseTypes.GasPriceObject{
		GasPrice: big.NewInt(10),
	}
	it.gasPriceIncreasePercent = big.NewInt(50) // increase 50 percent
	// when the old format doesn't match the new format, return the new gas price
	mtx.MaxFeePerGas = ethtypes.NewHexInteger64(20)
	mtx.MaxPriorityFeePerGas = ethtypes.NewHexInteger64(1)

	it.gasPriceIncreaseMax = big.NewInt(26)
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddGasPriceOutput(ctx, retrievedGasPrice, nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Equal(t, "40000", tOut.Cost.String())
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, retrievedGasPrice.GasPrice, rsc.StageOutputsToBePersisted.TxUpdates.GasPrice.BigInt())
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.HistoryUpdates))
	called := make(chan bool, 3)
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionRetrieveGasPrice, mock.Anything, (*fftypes.JSONAny)(nil), mock.Anything).Run(func(args mock.Arguments) {
		called <- true
	}).Return(nil).Maybe()
	rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
	<-called
}
func TestProduceLatestInFlightStageContextRetrieveGasIncrementsEIP1559ReachedCap(t *testing.T) {
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

	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	defer mTS.AssertExpectations(t)
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	retrievedGasPrice := &baseTypes.GasPriceObject{
		GasPrice: big.NewInt(10),
	}

	retrievedGasPriceEIP1559 := &baseTypes.GasPriceObject{
		MaxFeePerGas:         big.NewInt(10),
		MaxPriorityFeePerGas: big.NewInt(1),
	}
	it.gasPriceClient = NewTestFixedPriceGasPriceClientEIP1559(t)
	it.gasPriceIncreasePercent = big.NewInt(50) // increase 50 percent
	// when the old format doesn't match the new format, return the new gas price
	mtx.MaxFeePerGas = ethtypes.NewHexInteger64(20)
	mtx.MaxPriorityFeePerGas = ethtypes.NewHexInteger64(1)

	it.gasPriceIncreaseMax = big.NewInt(26)
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddGasPriceOutput(ctx, retrievedGasPrice, nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Equal(t, "40000", tOut.Cost.String())
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Nil(t, rsc.StageOutputsToBePersisted.TxUpdates.MaxFeePerGas)
	assert.Nil(t, rsc.StageOutputsToBePersisted.TxUpdates.MaxPriorityFeePerGas)
	assert.Equal(t, retrievedGasPrice.GasPrice, rsc.StageOutputsToBePersisted.TxUpdates.GasPrice.BigInt())
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.HistoryUpdates))
	called := make(chan bool, 3)
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionRetrieveGasPrice, mock.Anything, (*fftypes.JSONAny)(nil), mock.Anything).Run(func(args mock.Arguments) {
		called <- true
	}).Return(nil).Maybe()
	rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
	<-called

	// when reached the cap
	mtx.MaxFeePerGas = ethtypes.NewHexInteger64(20)
	mtx.MaxPriorityFeePerGas = ethtypes.NewHexInteger64(1)

	it.gasPriceIncreaseMax = big.NewInt(26)
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)

	it.stateManager.AddGasPriceOutput(ctx, retrievedGasPriceEIP1559, nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Equal(t, "40000", tOut.Cost.String())
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, big.NewInt(26), rsc.StageOutputsToBePersisted.TxUpdates.MaxFeePerGas.BigInt())
	assert.Equal(t, big.NewInt(1), rsc.StageOutputsToBePersisted.TxUpdates.MaxPriorityFeePerGas.BigInt())
	assert.Nil(t, rsc.StageOutputsToBePersisted.TxUpdates.GasPrice)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.HistoryUpdates))
	called = make(chan bool, 3)
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionRetrieveGasPrice, mock.Anything, (*fftypes.JSONAny)(nil), mock.Anything).Run(func(args mock.Arguments) {
		called <- true
	}).Return(nil).Maybe()
	rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
	<-called
}

func TestProduceLatestInFlightStageContextRetrieveGasPanic(t *testing.T) {
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

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// unexpected error
	rsc = it.stateManager.GetRunningStageContext(ctx)
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPanicOutput(ctx, baseTypes.InFlightTxStageRetrieveGasPrice)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Regexp(t, "PD011919", tOut.Error)
	assert.NotEqual(t, rsc, it.stateManager.GetRunningStageContext(ctx))
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
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

	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	defer mTS.AssertExpectations(t)
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

func TestProduceLatestInFlightStageContextSigning(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	// trigger signing
	mtx := it.stateManager.GetTx()
	mtx.TransactionHash = ""
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.NotNil(t, it.stateManager.GetRunningStageContext(ctx))
	rsc := it.stateManager.GetRunningStageContext(ctx)
	assert.Equal(t, baseTypes.InFlightTxStageSigning, rsc.Stage)
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mEC.On("BuildRawTransaction", ctx, ethclient.EIP1559, string(mtx.From), mtx.Transaction).Run(func(args mock.Arguments) {
		time.Sleep(1 * time.Hour) // make sure the async action never got returned as the test will mock the events
	}).Maybe()
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	signedMsg := []byte(testTransactionData)
	txHash := testTxHash
	// succeed signing
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	// test panic error that doesn't belong to the current stage gets ignored
	it.stateManager.AddPanicOutput(ctx, baseTypes.InFlightTxStageRetrieveGasPrice)
	it.stateManager.AddSignOutput(ctx, signedMsg, txHash, nil)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.Equal(t, baseTypes.InFlightTxStageSigning, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.HistoryUpdates))
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionSign, fftypes.JSONAnyPtr(`{"hash":"`+txHash+`"}`), (*fftypes.JSONAny)(nil), mock.Anything).Return(nil).Maybe()
	rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
	// failed signing
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddSignOutput(ctx, nil, "", fmt.Errorf("sign error"))
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.Equal(t, baseTypes.InFlightTxStageSigning, rsc.Stage)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.HistoryUpdates))
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionSign, (*fftypes.JSONAny)(nil), fftypes.JSONAnyPtr(`{"error":"sign error"}`), mock.Anything).Return(nil).Maybe()

	// persisting error waiting for persistence retry timeout
	assert.False(t, rsc.StageErrored)
	it.persistenceRetryTimeout = 5 * time.Second
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageSigning, time.Now().Add(it.persistenceRetryTimeout*2), fmt.Errorf("persist signing sub-status error"))
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())

	// persisting error retrying
	assert.False(t, rsc.StageErrored)
	it.persistenceRetryTimeout = 0
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageSigning, time.Now(), fmt.Errorf("persist signing sub-status error"))
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	it.persistenceRetryTimeout = 5 * time.Second

	// persisted stage error
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageSigning, time.Now(), nil)
	assert.NotNil(t, rsc.StageOutput.SignOutput.Err)
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionSign, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, rsc.StageErrored)

	// persisted stage success and move on
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)

	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageSigning, time.Now(), nil)
	rsc.StageOutput.SignOutput.Err = nil
	rsc.StageOutput.SignOutput.SignedMessage = signedMsg
	rsc.StageErrored = false
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	// switched running stage context
	assert.NotEqual(t, rsc, it.stateManager.GetRunningStageContext(ctx))
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.Equal(t, baseTypes.InFlightTxStageSubmitting, rsc.Stage)
	assert.Equal(t, signedMsg, inFlightStageMananger.TransientPreviousStageOutputs.SignedMessage)
}

func TestProduceLatestInFlightStageContextSigningPanic(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	// trigger signing
	mtx := it.stateManager.GetTx()
	mtx.TransactionHash = ""
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mEC.On("BuildRawTransaction", ctx, ethclient.EIP1559, string(mtx.From), mtx.Transaction).Run(func(args mock.Arguments) {
		time.Sleep(1 * time.Hour) // make sure the async action never got returned as the test will mock the events
	}).Maybe()
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.NotNil(t, it.stateManager.GetRunningStageContext(ctx))
	rsc := it.stateManager.GetRunningStageContext(ctx)
	assert.Equal(t, baseTypes.InFlightTxStageSigning, rsc.Stage)
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// unexpected error
	rsc = it.stateManager.GetRunningStageContext(ctx)
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPanicOutput(ctx, baseTypes.InFlightTxStageSigning)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Regexp(t, "PD011919", tOut.Error)
	assert.NotEqual(t, rsc, it.stateManager.GetRunningStageContext(ctx))
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)

}

func TestProduceLatestInFlightStageContextSubmitPanic(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mEC.On("SendRawTransaction", ctx, mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(1 * time.Hour) // make sure the async action never got returned as the test will mock the events
	}).Maybe()
	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	signedMsg := []byte("signedMessage")
	it.TriggerNewStageRun(ctx, baseTypes.InFlightTxStageSubmitting, baseTypes.BaseTxSubStatusReceived, signedMsg)
	rsc := it.stateManager.GetRunningStageContext(ctx)

	// unexpected error
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPanicOutput(ctx, baseTypes.InFlightTxStageSubmitting)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mEC.On("SendRawTransaction", ctx, mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(1 * time.Hour) // make sure the async action never got returned as the test will mock the events
	}).Maybe()
	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	signedMsg := []byte("signedMessage")
	txHash := "tx_hashnew"
	it.TriggerNewStageRun(ctx, baseTypes.InFlightTxStageSubmitting, baseTypes.BaseTxSubStatusReceived, signedMsg)
	rsc := it.stateManager.GetRunningStageContext(ctx)
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	defer mTS.AssertExpectations(t)
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
	it.stateManager.AddSubmitOutput(ctx, txHash, submissionTime, baseTypes.SubmissionOutcomeSubmittedNew, ethclient.ErrorReason(""), nil)
	rsc.StageOutputsToBePersisted = nil
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.Equal(t, baseTypes.InFlightTxStageSubmitting, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.HistoryUpdates))
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionSubmitTransaction, fftypes.JSONAnyPtr(`{"txHash":"`+txHash+`"}`), (*fftypes.JSONAny)(nil), mock.Anything).Return(nil).Maybe()
	rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
	assert.Equal(t, []string{txHash}, rsc.StageOutputsToBePersisted.PolicyInfo.SubmittedTxHashes)
	assert.Equal(t, submissionTime, rsc.StageOutputsToBePersisted.TxUpdates.FirstSubmit)
	assert.Equal(t, submissionTime, rsc.StageOutputsToBePersisted.TxUpdates.LastSubmit)

	// submission attempt completed - nonce too low
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	rsc = it.stateManager.GetRunningStageContext(ctx)
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{}
	it.stateManager.AddSubmitOutput(ctx, txHash, submissionTime, baseTypes.SubmissionOutcomeNonceTooLow, ethclient.ErrorReason(""), nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.Equal(t, baseTypes.InFlightTxStageSubmitting, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionSubmitTransaction, fftypes.JSONAnyPtr(`{"txHash":"`+txHash+`"}`), (*fftypes.JSONAny)(nil), mock.Anything).Return(nil).Maybe()
	rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
	assert.Equal(t, []string{txHash}, rsc.StageOutputsToBePersisted.PolicyInfo.SubmittedTxHashes)
	assert.Equal(t, submissionTime, rsc.StageOutputsToBePersisted.TxUpdates.LastSubmit)
	assert.Equal(t, txHash, *rsc.StageOutputsToBePersisted.TxUpdates.TransactionHash)
}

func TestProduceLatestInFlightStageContextCannotSubmit(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mEC.On("SendRawTransaction", ctx, mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(1 * time.Hour) // make sure the async action never got returned as the test will mock the events
	}).Maybe()
	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	defer mTS.AssertExpectations(t)
	// Previous cost unknown when state is not validated
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	mtx := it.stateManager.GetTx()
	mtx.MaxFeePerGas = ethtypes.NewHexInteger64(32247127816)
	mtx.MaxPriorityFeePerGas = ethtypes.NewHexInteger64(32146027800)

	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)
	mtx.TransactionHash = ""
	mtx.FirstSubmit = nil
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		SubmittedTxHashes: []string{},
	}

	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	defer mTS.AssertExpectations(t)
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
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mEC.On("SendRawTransaction", ctx, mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(1 * time.Hour) // make sure the async action never got returned as the test will mock the events
	}).Maybe()

	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	mTS.On("UpdateTransaction", mock.Anything, ctx, mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(1 * time.Hour) // make sure the async action never got returned as the test will mock the events
	}).Maybe()
	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	signedMsg := []byte("signedMessage")
	txHash := "tx_hashold"

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
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
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
	it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.True(t, rsc.StageErrored)

	// persisting error retrying
	it.persistenceRetryTimeout = 0
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageSubmitting, time.Now(), fmt.Errorf("persist submit error"))
	it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})

	// switched to tracking
	assert.Equal(t, baseTypes.InFlightTxStageReceipting, inFlightStageMananger.stage)
	assert.True(t, it.stateManager.ValidatedTransactionHashMatchState(ctx))

}

func TestProduceLatestInFlightStageContextReceipting(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mEC.On("GetTransactionReceipt", ctx, mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(1 * time.Hour) // make sure the async action never got returned as the test will mock the events
	}).Maybe()
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	mtx := it.stateManager.GetTx()

	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)

	assert.Equal(t, baseTypes.InFlightTxStageReceipting, inFlightStageMananger.stage)

	mTS := testInFlightTransactionStateManagerWithMocks.mTS

	// receipt error
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddReceiptOutput(ctx, nil, fmt.Errorf("receipt error"))
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)
	mtx := it.stateManager.GetTx()

	// set existing transaction hashes
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		SubmittedTxHashes: []string{mtx.TransactionHash, "hash1"},
	}
	called := make(chan bool, 3)
	mEC := it.ethClient.(*componentmocks.EthClient)
	mEC.On("GetTransactionReceipt", ctx, "hash1").Run(func(args mock.Arguments) {
		called <- true
	}).Return(nil, ethclient.ErrorReason(""), nil).Once()
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusTracking, baseTypes.BaseTxActionStateTransition, fftypes.JSONAnyPtr(`{"txHash":"hash1"}`), (*fftypes.JSONAny)(nil), mock.Anything).Return(nil).Maybe()
	persistenceCalled := make(chan bool, 3)
	mTS.On("UpdateTransaction", ctx, mtx.ID, mock.Anything).Run(func(args mock.Arguments) {
		persistenceCalled <- true
	}).Return(nil).Maybe()

	mCL := testInFlightTransactionStateManagerWithMocks.mCL
	addMock := mCL.On("Add", ctx, mtx.ID, mtx.TransactionHash, mock.Anything, mock.Anything)
	mCL.On("Remove", ctx, mtx.ID, mtx.TransactionHash).Return(nil).Maybe()
	eventHandlerCalled := make(chan bool, 3)
	addMock.Run(func(args mock.Arguments) {
		addMock.Return(nil)
		eventHandlerCalled <- true
	}).Maybe()
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)
	mtx := it.stateManager.GetTx()

	// set existing transaction hashes
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		SubmittedTxHashes: []string{mtx.TransactionHash, "hash1"},
	}
	called := make(chan bool, 3)
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mEC.On("GetTransactionReceipt", ctx, "hash1").Run(func(args mock.Arguments) {
		called <- true
	}).Return(nil, ethclient.ErrorReason(""), nil).Once()
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusTracking, baseTypes.BaseTxActionStateTransition, fftypes.JSONAnyPtr(`{"txHash":"hash1"}`), (*fftypes.JSONAny)(nil), mock.Anything).Return(nil).Maybe()
	persistenceCalled := make(chan bool, 3)
	mTS.On("UpdateTransaction", ctx, mtx.ID, mock.Anything).Run(func(args mock.Arguments) {
		mtx.Status = baseTypes.BaseTxStatusFailed
		persistenceCalled <- true
	}).Return(fmt.Errorf("failed")).Maybe()

	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	<-called
	<-persistenceCalled
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
}

func TestProduceLatestInFlightStageContextReceiptingCheckExistingHashesTrackingFailure(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)
	mtx := it.stateManager.GetTx()

	// set existing transaction hashes
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		SubmittedTxHashes: []string{mtx.TransactionHash, "hash1"},
	}
	called := make(chan bool, 3)
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mEC.On("GetTransactionReceipt", ctx, "hash1").Run(func(args mock.Arguments) {
		called <- true
	}).Return(nil, ethclient.ErrorReason(""), nil).Once()
	inFlightTxState := it.stateManager.(*inFlightTransactionState)
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	mTS.On("AddSubStatusAction", mock.Anything, mtx.ID, baseTypes.BaseTxSubStatusTracking, baseTypes.BaseTxActionStateTransition, fftypes.JSONAnyPtr(`{"txHash":"hash1"}`), (*fftypes.JSONAny)(nil), mock.Anything).Return(nil).Maybe()
	persistenceCalled := make(chan bool, 3)
	mTS.On("UpdateTransaction", ctx, mtx.ID, mock.Anything).Run(func(args mock.Arguments) {
		persistenceCalled <- true
	}).Return(nil).Maybe()

	mCL := testInFlightTransactionStateManagerWithMocks.mCL
	addMock := mCL.On("Add", ctx, mtx.ID, mtx.TransactionHash, mock.Anything, mock.Anything)
	mCL.On("Remove", ctx, mtx.ID, mtx.TransactionHash).Return(nil).Maybe()
	eventHandlerCalled := make(chan bool, 3)
	addMock.Run(func(args mock.Arguments) {
		addMock.Return(fmt.Errorf("failed to add"))
		eventHandlerCalled <- true
	}).Maybe()
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	assert.Equal(t, 1, len(inFlightTxState.bufferedStageOutputs))
}

func TestProduceLatestInFlightStageContextReceiptingExceededTimeout(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)

	assert.Equal(t, baseTypes.InFlightTxStageReceipting, inFlightStageMananger.stage)

	mCL := testInFlightTransactionStateManagerWithMocks.mCL
	removeMock := mCL.On("Remove", ctx, mock.Anything)
	called := make(chan bool)
	removeMock.Run(func(args mock.Arguments) {
		removeMock.Return(nil)
		close(called)
	}).Once()
	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)

	// no receipt but last warn time expired
	expiredTime := fftypes.FFTime(fftypes.Now().Time().Add(-(it.resubmitInterval)))
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		LastWarnTime: &expiredTime,
	}

	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	<-called
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	// goes into retrieve gas price stage
	assert.Equal(t, baseTypes.InFlightTxStageRetrieveGasPrice, inFlightStageMananger.stage)
}

func TestProduceLatestInFlightStageContextReceiptingExceededTimeoutIgnoreRemovalErrors(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mEC.On("GetTransactionReceipt", ctx, mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(1 * time.Hour) // make sure the async action never got returned as the test will mock the events
	}).Maybe()
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)

	assert.Equal(t, baseTypes.InFlightTxStageReceipting, inFlightStageMananger.stage)
	mCL := testInFlightTransactionStateManagerWithMocks.mCL
	removeMock := mCL.On("Remove", ctx, mock.Anything)

	removeMock.Run(func(args mock.Arguments) {
		removeMock.Return(fmt.Errorf("should be ignored"))
	}).Once()
	imtxs := inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState)

	// no receipt but last warn time expired
	expiredTime := fftypes.FFTime(fftypes.Now().Time().Add(-(it.resubmitInterval)).Add(-100 * time.Second))
	imtxs.policyInfo = &baseTypes.EnterprisePolicyInfo{
		LastWarnTime: &expiredTime,
	}

	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
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
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mEC.On("GetTransactionReceipt", ctx, mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(1 * time.Hour) // make sure the async action never got returned as the test will mock the events
	}).Maybe()
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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

func TestProduceLatestInFlightStageContextConfirming(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	assert.Equal(t, baseTypes.InFlightTxStageReceipting, inFlightStageMananger.stage)

	// move to confirming
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageReceipting, time.Now(), nil)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mEC.On("GetTransactionReceipt", ctx, mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(1 * time.Hour) // make sure the async action never got returned as the test will mock the events
	}).Maybe()
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mEC.On("GetTransactionReceipt", ctx, mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(1 * time.Hour) // make sure the async action never got returned as the test will mock the events
	}).Maybe()
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	mtx := it.stateManager.GetTx()
	mEN := testInFlightTransactionStateManagerWithMocks.mEN
	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	assert.Equal(t, baseTypes.InFlightTxStageReceipting, inFlightStageMananger.stage)

	// move to confirming
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageReceipting, time.Now(), nil)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	assert.Equal(t, baseTypes.InFlightTxStageConfirming, inFlightStageMananger.stage)

	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	defer mTS.AssertExpectations(t)

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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mEC.On("GetTransactionReceipt", ctx, mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(1 * time.Hour) // make sure the async action never got returned as the test will mock the events
	}).Maybe()
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// set validated to enter tracking
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	assert.Equal(t, baseTypes.InFlightTxStageReceipting, inFlightStageMananger.stage)

	// move to confirming
	it.stateManager.AddPersistenceOutput(ctx, baseTypes.InFlightTxStageReceipting, time.Now(), nil)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
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
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.TransactionEngineContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.Empty(t, tOut.Cost) // cost for completed transaction should be 0
	assert.True(t, tOut.TransactionSubmitted)

}
