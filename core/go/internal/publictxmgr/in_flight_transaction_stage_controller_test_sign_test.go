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

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/ptxapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestProduceLatestInFlightStageContextSigning(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}

	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &ptxapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(10),
		},
	})

	// trigger signing
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.NotNil(t, it.stateManager.GetRunningStageContext(ctx))
	rsc := it.stateManager.GetRunningStageContext(ctx)
	assert.Equal(t, InFlightTxStageSigning, rsc.Stage)
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	signedMsg := []byte(testTransactionData)
	txHash := tktypes.MustParseBytes32(testTxHash)
	// succeed signing
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	// test panic error that doesn't belong to the current stage gets ignored
	it.stateManager.AddPanicOutput(ctx, InFlightTxStageRetrieveGasPrice)
	it.stateManager.AddSignOutput(ctx, signedMsg, &txHash, nil)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.Equal(t, InFlightTxStageSigning, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.StatusUpdates))
	_ = rsc.StageOutputsToBePersisted.StatusUpdates[0](mTS.statusUpdater)
	// failed signing
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddSignOutput(ctx, nil, nil, fmt.Errorf("sign error"))
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.Equal(t, InFlightTxStageSigning, rsc.Stage)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.StatusUpdates))

	// persisting error waiting for persistence retry timeout
	assert.False(t, rsc.StageErrored)
	it.persistenceRetryTimeout = 5 * time.Second
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, InFlightTxStageSigning, time.Now().Add(it.persistenceRetryTimeout*2), fmt.Errorf("persist signing sub-status error"))
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())

	// persisting error retrying
	assert.False(t, rsc.StageErrored)
	it.persistenceRetryTimeout = 0
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, InFlightTxStageSigning, time.Now(), fmt.Errorf("persist signing sub-status error"))
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	it.persistenceRetryTimeout = 5 * time.Second

	// persisted stage error
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, InFlightTxStageSigning, time.Now(), nil)
	assert.NotNil(t, rsc.StageOutput.SignOutput.Err)
	_ = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.True(t, rsc.StageErrored)

	// persisted stage success and move on
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)

	it.stateManager.AddPersistenceOutput(ctx, InFlightTxStageSigning, time.Now(), nil)
	rsc.StageOutput.SignOutput.Err = nil
	rsc.StageOutput.SignOutput.SignedMessage = signedMsg
	rsc.StageErrored = false
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	// switched running stage context
	assert.NotEqual(t, rsc, it.stateManager.GetRunningStageContext(ctx))
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.Equal(t, InFlightTxStageSubmitting, rsc.Stage)
	assert.Equal(t, signedMsg, inFlightStageMananger.TransientPreviousStageOutputs.SignedMessage)
}

func TestProduceLatestInFlightStageContextSigningPanic(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}

	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &ptxapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(10),
		},
	})

	// trigger signing
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.NotNil(t, it.stateManager.GetRunningStageContext(ctx))
	rsc := it.stateManager.GetRunningStageContext(ctx)
	assert.Equal(t, InFlightTxStageSigning, rsc.Stage)
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	// unexpected error
	rsc = it.stateManager.GetRunningStageContext(ctx)
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddPanicOutput(ctx, InFlightTxStageSigning)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Regexp(t, "PD011919", tOut.Error)
	assert.NotEqual(t, rsc, it.stateManager.GetRunningStageContext(ctx))
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)

}

func TestProduceLatestInFlightStageContextTriggerSign(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}

	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &ptxapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(10),
		},
	})
	it.testOnlyNoActionMode = false
	it.testOnlyNoEventMode = false
	// trigger signing
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	buildRawTransactionMock := m.ethClient.On("BuildRawTransactionNoResolve", ctx, ethclient.EIP1559, mock.Anything, mock.Anything, mock.Anything)
	buildRawTransactionMock.Run(func(args mock.Arguments) {
		from := args[2].(*ethclient.ResolvedSigner)
		assert.Equal(t, o.signingAddress, from.Address)
		buildRawTransactionMock.Return(nil, fmt.Errorf("pop"))
	}).Once()
	err := it.TriggerSignTx(ctx)
	require.NoError(t, err)
	ticker := time.NewTicker(10 * time.Millisecond)
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	for !t.Failed() && len(inFlightStageMananger.bufferedStageOutputs) == 0 {
		// wait for event
		<-ticker.C
	}
	assert.Len(t, inFlightStageMananger.bufferedStageOutputs, 1)
	assert.NotNil(t, inFlightStageMananger.bufferedStageOutputs[0].SignOutput)
	assert.NotNil(t, inFlightStageMananger.bufferedStageOutputs[0].SignOutput.Err)
	assert.Nil(t, inFlightStageMananger.bufferedStageOutputs[0].SignOutput.SignedMessage)
	assert.Empty(t, inFlightStageMananger.bufferedStageOutputs[0].SignOutput.TxHash)
}