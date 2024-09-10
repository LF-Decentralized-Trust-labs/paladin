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
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	baseTypes "github.com/kaleido-io/paladin/core/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestProduceLatestInFlightStageContextSigning(t *testing.T) {
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	// trigger signing
	mtx := it.stateManager.GetTx()
	mtx.TransactionHash = ""
	mTS := testInFlightTransactionStateManagerWithMocks.mTS
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.NotNil(t, it.stateManager.GetRunningStageContext(ctx))
	rsc := it.stateManager.GetRunningStageContext(ctx)
	assert.Equal(t, baseTypes.InFlightTxStageSigning, rsc.Stage)
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)

	signedMsg := []byte(testTransactionData)
	txHash := testTxHash
	// succeed signing
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	// test panic error that doesn't belong to the current stage gets ignored
	it.stateManager.AddPanicOutput(ctx, baseTypes.InFlightTxStageRetrieveGasPrice)
	it.stateManager.AddSignOutput(ctx, signedMsg, txHash, nil)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
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
	_ = rsc.StageOutputsToBePersisted.HistoryUpdates[0](mTS)
	// failed signing
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)
	it.stateManager.AddSignOutput(ctx, nil, "", fmt.Errorf("sign error"))
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.Equal(t, baseTypes.InFlightTxStageSigning, rsc.Stage)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
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
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	tOut := it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
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
	tOut = it.ProduceLatestInFlightStageContext(ctx, &baseTypes.OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Regexp(t, "PD011919", tOut.Error)
	assert.NotEqual(t, rsc, it.stateManager.GetRunningStageContext(ctx))
	inFlightStageMananger.bufferedStageOutputs = make([]*baseTypes.StageOutput, 0)

}

func TestProduceLatestInFlightStageContextTriggerSign(t *testing.T) {
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
	buildRawTransactionMock := mEC.On("BuildRawTransaction", ctx, ethclient.EIP1559, string(mtx.From), mtx.Transaction)
	buildRawTransactionMock.Run(func(args mock.Arguments) {
		from := args[2].(string)
		txObj := args[3].(*ethsigner.Transaction)

		assert.Equal(t, "0x4e598f6e918321dd47c86e7a077b4ab0e7414846", from)
		assert.Equal(t, ethtypes.MustNewHexBytes0xPrefix(testTransactionData), txObj.Data)
		buildRawTransactionMock.Return(nil, fmt.Errorf("pop"))
		close(called)
	}).Once()
	err := it.TriggerSignTx(ctx)
	require.NoError(t, err)
	<-called
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	for len(inFlightStageMananger.bufferedStageOutputs) == 0 {
		// wait for event
	}
	assert.Len(t, inFlightStageMananger.bufferedStageOutputs, 1)
	assert.NotNil(t, inFlightStageMananger.bufferedStageOutputs[0].SignOutput)
	assert.NotNil(t, inFlightStageMananger.bufferedStageOutputs[0].SignOutput.Err)
	assert.Nil(t, inFlightStageMananger.bufferedStageOutputs[0].SignOutput.SignedMessage)
	assert.Empty(t, inFlightStageMananger.bufferedStageOutputs[0].SignOutput.TxHash)
}
