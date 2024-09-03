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

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	baseTypes "github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestProduceLatestInFlightStageContextRetrieveGas(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	// trigger retrieve gas price
	mtx := it.stateManager.GetTx()
	mtx.GasPrice = nil
	mtx.TransactionHash = ""
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
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
	_ = rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
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

func TestProduceLatestInFlightStageContextRetrieveGasIncrements(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	// trigger retrieve gas price
	mtx := it.stateManager.GetTx()
	mtx.GasPrice = nil
	mtx.TransactionHash = ""
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
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
	_ = rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
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
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
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
	_ = rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
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
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
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
	_ = rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
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
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
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
	_ = rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
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
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
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
	_ = rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
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
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
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
	_ = rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
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
	_ = rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
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
