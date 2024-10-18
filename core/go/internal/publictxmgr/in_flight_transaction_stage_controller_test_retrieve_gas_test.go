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

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
)

type mockStatusUpdater struct {
	updateSubStatus func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info *fftypes.JSONAny, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error
}

func (msu *mockStatusUpdater) UpdateSubStatus(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info *fftypes.JSONAny, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
	return msu.updateSubStatus(ctx, imtx, subStatus, action, info, err, actionOccurred)
}

func TestProduceLatestInFlightStageContextRetrieveGas(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}

	// trigger retrieve gas price
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)

	assert.NotNil(t, it.stateManager.GetRunningStageContext(ctx))
	rsc := it.stateManager.GetRunningStageContext(ctx)

	assert.Equal(t, InFlightTxStageRetrieveGasPrice, rsc.Stage)

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	retrievedGasPrice := &pldapi.PublicTxGasPricing{
		GasPrice: tktypes.Int64ToInt256(10),
	}
	// succeed retrieving gas price
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddGasPriceOutput(ctx, retrievedGasPrice, nil)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, retrievedGasPrice.GasPrice, rsc.StageOutputsToBePersisted.TxUpdates.GasPricing.GasPrice)
	assert.Nil(t, rsc.StageOutputsToBePersisted.TxUpdates.GasPricing.MaxFeePerGas)
	assert.Nil(t, rsc.StageOutputsToBePersisted.TxUpdates.GasPricing.MaxPriorityFeePerGas)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.StatusUpdates))
	_ = rsc.StageOutputsToBePersisted.StatusUpdates[0](mTS.statusUpdater)
	// failed retrieving gas price
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddGasPriceOutput(ctx, nil, fmt.Errorf("gas retrieve error"))
	rsc = it.stateManager.GetRunningStageContext(ctx)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Nil(t, rsc.StageOutputsToBePersisted.TxUpdates)
	assert.GreaterOrEqual(t, len(rsc.StageOutputsToBePersisted.StatusUpdates), 1)

	// persisting error waiting for persistence retry timeout
	assert.False(t, rsc.StageErrored)
	it.persistenceRetryTimeout = 5 * time.Second
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, InFlightTxStageRetrieveGasPrice, time.Now().Add(it.persistenceRetryTimeout*2), fmt.Errorf("persist gas price error"))
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	// persisting error retrying
	assert.False(t, rsc.StageErrored)
	it.persistenceRetryTimeout = 0
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, InFlightTxStageRetrieveGasPrice, time.Now(), fmt.Errorf("persist gas price error"))
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	it.persistenceRetryTimeout = 5 * time.Second

	// persisted stage error
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, InFlightTxStageRetrieveGasPrice, time.Now(), nil)
	assert.NotNil(t, rsc.StageOutput.GasPriceOutput.Err)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	assert.True(t, rsc.StageErrored)

	// persisted stage success and move on
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, InFlightTxStageRetrieveGasPrice, time.Now(), nil)
	rsc.StageOutput.GasPriceOutput.Err = nil
	rsc.StageErrored = false
	it.stateManager.SetValidatedTransactionHashMatchState(ctx, true)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	assert.False(t, it.stateManager.ValidatedTransactionHashMatchState(ctx))
	// switched running stage context
	assert.NotEqual(t, rsc, it.stateManager.GetRunningStageContext(ctx))
}

func TestProduceLatestInFlightStageContextRetrieveGasIncrements(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	it.testOnlyNoEventMode = true
	mSU := &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}
	mTS.statusUpdater = mSU

	// trigger retrieve gas price
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)

	assert.NotNil(t, it.stateManager.GetRunningStageContext(ctx))
	rsc := it.stateManager.GetRunningStageContext(ctx)

	assert.Equal(t, InFlightTxStageRetrieveGasPrice, rsc.Stage)

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// Set old gas price in memory
	mTS.InMemoryTxStateManager.(*inMemoryTxState).mtx.GasPricing = &pldapi.PublicTxGasPricing{
		GasPrice: tktypes.Int64ToInt256(20),
	}

	// We will retrieve the new price of 10
	retrievedGasPrice := &pldapi.PublicTxGasPricing{
		GasPrice: tktypes.Int64ToInt256(10),
	}
	it.gasPriceIncreasePercent = 50 // increase 50 percent

	// Simulate the run of the stage
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddGasPriceOutput(ctx, retrievedGasPrice, nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Equal(t, "40000", tOut.Cost.String())
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, big.NewInt(30), rsc.StageOutputsToBePersisted.TxUpdates.GasPricing.GasPrice.Int())
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.StatusUpdates))
	_ = rsc.StageOutputsToBePersisted.StatusUpdates[0](mTS.statusUpdater)
}

func TestProduceLatestInFlightStageContextRetrieveGasIncrementsReachedCap(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	it.testOnlyNoEventMode = true
	it.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mSU := &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}
	mTS.statusUpdater = mSU

	// trigger retrieve gas price
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)

	assert.NotNil(t, it.stateManager.GetRunningStageContext(ctx))
	rsc := it.stateManager.GetRunningStageContext(ctx)

	assert.Equal(t, InFlightTxStageRetrieveGasPrice, rsc.Stage)

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(20),
		},
	})
	it.gasPriceIncreasePercent = 50 // increase 50 percent
	// when reached the max gas price cap
	retrievedGasPrice := &pldapi.PublicTxGasPricing{
		GasPrice: tktypes.Int64ToInt256(10),
	}

	it.gasPriceIncreaseMax = big.NewInt(26)
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddGasPriceOutput(ctx, retrievedGasPrice, nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Equal(t, "40000", tOut.Cost.String())
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, big.NewInt(26), rsc.StageOutputsToBePersisted.TxUpdates.GasPricing.GasPrice.Int())
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.StatusUpdates))
	_ = rsc.StageOutputsToBePersisted.StatusUpdates[0](mTS.statusUpdater)
}

func TestProduceLatestInFlightStageContextRetrieveGasIncrementsRetrievedHigherPrice(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	it.testOnlyNoEventMode = true
	it.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mSU := &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}
	mTS.statusUpdater = mSU

	// trigger retrieve gas price
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)

	assert.NotNil(t, it.stateManager.GetRunningStageContext(ctx))
	rsc := it.stateManager.GetRunningStageContext(ctx)

	assert.Equal(t, InFlightTxStageRetrieveGasPrice, rsc.Stage)

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	it.gasPriceIncreasePercent = 50 // increase 50 percent
	// retrieved price is higher
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(20),
		},
	})
	higherRetrievedPrice := &pldapi.PublicTxGasPricing{
		GasPrice: tktypes.Int64ToInt256(21),
	}
	it.gasPriceIncreaseMax = big.NewInt(26)
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddGasPriceOutput(ctx, higherRetrievedPrice, nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Equal(t, "40000", tOut.Cost.String())
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, higherRetrievedPrice.GasPrice, rsc.StageOutputsToBePersisted.TxUpdates.GasPricing.GasPrice)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.StatusUpdates))
	_ = rsc.StageOutputsToBePersisted.StatusUpdates[0](mTS.statusUpdater)
}

func TestProduceLatestInFlightStageContextRetrieveGasIncrementsEIP1559HigherExistingPrice(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	it.testOnlyNoEventMode = true
	it.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mSU := &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}
	mTS.statusUpdater = mSU

	// trigger retrieve gas price
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)

	assert.NotNil(t, it.stateManager.GetRunningStageContext(ctx))
	rsc := it.stateManager.GetRunningStageContext(ctx)

	assert.Equal(t, InFlightTxStageRetrieveGasPrice, rsc.Stage)

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	it.gasPriceIncreasePercent = 50 // increase 50 percent
	// EIP-1559 gas price
	retrievedGasPriceEIP1559 := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         tktypes.Int64ToInt256(10),
		MaxPriorityFeePerGas: tktypes.Int64ToInt256(1),
	}
	it.gasPriceIncreaseMax = nil
	// the highest gas price used is higher than the retrieved gas price
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			MaxFeePerGas:         tktypes.Uint64ToUint256(20),
			MaxPriorityFeePerGas: tktypes.Uint64ToUint256(1),
		},
	})

	it.gasPriceClient = NewTestFixedPriceGasPriceClientEIP1559(t)
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddGasPriceOutput(ctx, retrievedGasPriceEIP1559, nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Equal(t, "40000", tOut.Cost.String())
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, big.NewInt(30), rsc.StageOutputsToBePersisted.TxUpdates.GasPricing.MaxFeePerGas.Int())
	assert.Equal(t, big.NewInt(1), rsc.StageOutputsToBePersisted.TxUpdates.GasPricing.MaxPriorityFeePerGas.Int())
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.StatusUpdates))
	_ = rsc.StageOutputsToBePersisted.StatusUpdates[0](mTS.statusUpdater)

}

func TestProduceLatestInFlightStageContextRetrieveGasIncrementsEIP1559MismatchFormat(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	it.testOnlyNoEventMode = true
	it.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mSU := &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}
	mTS.statusUpdater = mSU

	// trigger retrieve gas price
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)

	assert.NotNil(t, it.stateManager.GetRunningStageContext(ctx))
	rsc := it.stateManager.GetRunningStageContext(ctx)

	assert.Equal(t, InFlightTxStageRetrieveGasPrice, rsc.Stage)

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	retrievedGasPrice := &pldapi.PublicTxGasPricing{
		GasPrice: tktypes.Int64ToInt256(10),
	}
	it.gasPriceIncreasePercent = 50 // increase 50 percent
	// when the old format doesn't match the new format, return the new gas price
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			MaxFeePerGas:         tktypes.Uint64ToUint256(20),
			MaxPriorityFeePerGas: tktypes.Uint64ToUint256(1),
		},
	})

	it.gasPriceIncreaseMax = big.NewInt(26)
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddGasPriceOutput(ctx, retrievedGasPrice, nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Equal(t, "40000", tOut.Cost.String())
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, retrievedGasPrice.GasPrice, rsc.StageOutputsToBePersisted.TxUpdates.GasPricing.GasPrice)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.StatusUpdates))
	_ = rsc.StageOutputsToBePersisted.StatusUpdates[0](mTS.statusUpdater)
}

func TestProduceLatestInFlightStageContextRetrieveGasIncrementsEIP1559ReachedCap(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	it.testOnlyNoEventMode = true
	it.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mSU := &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}
	mTS.statusUpdater = mSU

	// trigger retrieve gas price
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	assert.NotNil(t, it.stateManager.GetRunningStageContext(ctx))
	rsc := it.stateManager.GetRunningStageContext(ctx)

	assert.Equal(t, InFlightTxStageRetrieveGasPrice, rsc.Stage)
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	retrievedGasPrice := &pldapi.PublicTxGasPricing{
		GasPrice: tktypes.Uint64ToUint256(10),
	}

	retrievedGasPriceEIP1559 := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         tktypes.Uint64ToUint256(10),
		MaxPriorityFeePerGas: tktypes.Uint64ToUint256(1),
	}
	it.gasPriceClient = NewTestFixedPriceGasPriceClientEIP1559(t)
	it.gasPriceIncreasePercent = 50 // increase 50 percent
	// when the old format doesn't match the new format, return the new gas price
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			MaxFeePerGas:         tktypes.Uint64ToUint256(20),
			MaxPriorityFeePerGas: tktypes.Uint64ToUint256(1),
		},
	})

	it.gasPriceIncreaseMax = big.NewInt(26)
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddGasPriceOutput(ctx, retrievedGasPrice, nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Equal(t, "40000", tOut.Cost.String())
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, retrievedGasPrice.GasPrice, rsc.StageOutputsToBePersisted.TxUpdates.GasPricing.GasPrice)
	assert.Nil(t, rsc.StageOutputsToBePersisted.TxUpdates.GasPricing.MaxFeePerGas)
	assert.Nil(t, rsc.StageOutputsToBePersisted.TxUpdates.GasPricing.MaxPriorityFeePerGas)
	assert.Equal(t, retrievedGasPrice.GasPrice, rsc.StageOutputsToBePersisted.TxUpdates.GasPricing.GasPrice)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.StatusUpdates))
	_ = rsc.StageOutputsToBePersisted.StatusUpdates[0](mTS.statusUpdater)

	// when reached the cap
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			MaxFeePerGas:         tktypes.Uint64ToUint256(20),
			MaxPriorityFeePerGas: tktypes.Uint64ToUint256(1),
		},
	})

	it.gasPriceIncreaseMax = big.NewInt(26)
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)

	it.stateManager.AddGasPriceOutput(ctx, retrievedGasPriceEIP1559, nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Equal(t, "40000", tOut.Cost.String())
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, big.NewInt(26), rsc.StageOutputsToBePersisted.TxUpdates.GasPricing.MaxFeePerGas.Int())
	assert.Equal(t, big.NewInt(1), rsc.StageOutputsToBePersisted.TxUpdates.GasPricing.MaxPriorityFeePerGas.Int())
	assert.Nil(t, rsc.StageOutputsToBePersisted.TxUpdates.GasPricing.GasPrice)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.StatusUpdates))
	_ = rsc.StageOutputsToBePersisted.StatusUpdates[0](mTS.statusUpdater)

}

func TestProduceLatestInFlightStageContextRetrieveGasPanic(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	it.testOnlyNoEventMode = true
	it.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mSU := &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}
	mTS.statusUpdater = mSU

	// trigger retrieve gas price
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	assert.NotNil(t, it.stateManager.GetRunningStageContext(ctx))
	rsc := it.stateManager.GetRunningStageContext(ctx)

	assert.Equal(t, InFlightTxStageRetrieveGasPrice, rsc.Stage)

	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// unexpected error
	rsc = it.stateManager.GetRunningStageContext(ctx)
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddPanicOutput(ctx, InFlightTxStageRetrieveGasPrice)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Regexp(t, "PD011919", tOut.Error)
	assert.NotEqual(t, rsc, it.stateManager.GetRunningStageContext(ctx))
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
}
