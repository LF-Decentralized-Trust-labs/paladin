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
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
)

type mockStatusUpdater struct {
	updateSubStatus func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info pldtypes.RawJSON, err pldtypes.RawJSON, actionOccurred *pldtypes.Timestamp) error
}

func (msu *mockStatusUpdater) UpdateSubStatus(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info pldtypes.RawJSON, err pldtypes.RawJSON, actionOccurred *pldtypes.Timestamp) error {
	return msu.updateSubStatus(ctx, imtx, subStatus, action, info, err, actionOccurred)
}

func TestProduceLatestInFlightStageContextRetrieveGas(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err pldtypes.RawJSON, actionOccurred *pldtypes.Timestamp) error {
			return nil
		},
	}

	// trigger retrieve gas price
	assert.Nil(t, it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)

	assert.NotNil(t, it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx))
	rsc := it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)

	assert.Equal(t, InFlightTxStageRetrieveGasPrice, rsc.Stage)

	currentGeneration := it.stateManager.GetCurrentGeneration(ctx).(*inFlightTransactionStateGeneration)

	retrievedGasPrice := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Int64ToInt256(10),
		MaxPriorityFeePerGas: pldtypes.Int64ToInt256(1),
	}
	// succeed retrieving gas price
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.GetCurrentGeneration(ctx).AddGasPriceOutput(ctx, retrievedGasPrice, nil)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	rsc = it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, retrievedGasPrice.MaxFeePerGas, rsc.StageOutputsToBePersisted.TxUpdates.NewValues.GasPricing.MaxFeePerGas)
	assert.Equal(t, retrievedGasPrice.MaxPriorityFeePerGas, rsc.StageOutputsToBePersisted.TxUpdates.NewValues.GasPricing.MaxPriorityFeePerGas)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.StatusUpdates))
	_ = rsc.StageOutputsToBePersisted.StatusUpdates[0](mTS.statusUpdater)
	// failed retrieving gas price
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.GetCurrentGeneration(ctx).AddGasPriceOutput(ctx, nil, fmt.Errorf("gas retrieve error"))
	rsc = it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
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
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.GetCurrentGeneration(ctx).AddPersistenceOutput(ctx, InFlightTxStageRetrieveGasPrice, time.Now().Add(it.persistenceRetryTimeout*2), fmt.Errorf("persist gas price error"))
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	// persisting error retrying
	assert.False(t, rsc.StageErrored)
	it.persistenceRetryTimeout = 0
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.GetCurrentGeneration(ctx).AddPersistenceOutput(ctx, InFlightTxStageRetrieveGasPrice, time.Now(), fmt.Errorf("persist gas price error"))
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	it.persistenceRetryTimeout = 5 * time.Second

	// persisted stage error
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.GetCurrentGeneration(ctx).AddPersistenceOutput(ctx, InFlightTxStageRetrieveGasPrice, time.Now(), nil)
	assert.NotNil(t, rsc.StageOutput.GasPriceOutput.Err)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	assert.True(t, rsc.StageErrored)

	// persisted stage success and move on
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.GetCurrentGeneration(ctx).AddPersistenceOutput(ctx, InFlightTxStageRetrieveGasPrice, time.Now(), nil)
	rsc.StageOutput.GasPriceOutput.Err = nil
	rsc.StageErrored = false
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	// switched running stage context
	assert.NotEqual(t, rsc, it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx))
}

func TestProduceLatestInFlightStageContextRetrieveGasPanic(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	it.testOnlyNoEventMode = true
	_, it.gasPriceClient, _ = NewTestFixedPriceGasPriceClient(t, uint64(10), uint64(1))
	mSU := &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err pldtypes.RawJSON, actionOccurred *pldtypes.Timestamp) error {
			return nil
		},
	}
	mTS.statusUpdater = mSU

	// trigger retrieve gas price
	assert.Nil(t, it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.Empty(t, *tOut)
	assert.NotNil(t, it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx))
	rsc := it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)

	assert.Equal(t, InFlightTxStageRetrieveGasPrice, rsc.Stage)

	currentGeneration := it.stateManager.GetCurrentGeneration(ctx).(*inFlightTransactionStateGeneration)

	// unexpected error
	rsc = it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.GetCurrentGeneration(ctx).AddPanicOutput(ctx, InFlightTxStageRetrieveGasPrice)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Regexp(t, "PD011919", tOut.Error)
	assert.NotEqual(t, rsc, it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx))
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
}
