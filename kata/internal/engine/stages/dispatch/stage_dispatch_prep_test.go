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

package stages

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"
	"github.com/kaleido-io/paladin/kata/mocks/enginemocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newDispatchPrepStageTx(ctx context.Context) *transactionstore.TransactionWrapper {
	txNodeID := "current_node_id"

	preReqTx := &transactionstore.TransactionWrapper{
		Transaction: transactionstore.Transaction{
			ID: uuid.New(),
		},
	}

	return &transactionstore.TransactionWrapper{
		Transaction: transactionstore.Transaction{
			ID:           uuid.New(),
			DispatchNode: txNodeID,
			PreReqTxs:    []string{preReqTx.GetTxID(ctx)},
		},
		PrivateTransaction: &components.PrivateTransaction{
			PreAssembly: &components.TransactionPreAssembly{
				TransactionSpecification: &prototk.TransactionSpecification{},
				RequiredVerifiers:        []*prototk.ResolveVerifierRequest{},
				Verifiers:                []*prototk.ResolvedVerifier{},
			},
			PostAssembly: &components.TransactionPostAssembly{
				AssemblyResult:        prototk.AssembleTransactionResponse_OK,
				OutputStatesPotential: []*prototk.NewState{},
				InputStates:           []*components.FullState{},
				ReadStates:            []*components.FullState{},
				OutputStates:          []*components.FullState{},
				AttestationPlan:       []*prototk.AttestationRequest{},
				Signatures:            []*prototk.AttestationResult{},
				Endorsements:          []*prototk.AttestationResult{},
			},
		},
	}
}
func TestDispatchPrepStage(t *testing.T) {
	ctx := context.Background()

	dps := &DispatchPrepStage{}
	assert.Equal(t, "dispatch_prep", dps.Name())

	testTx := newDispatchPrepStageTx(ctx)
	mSFS := enginemocks.NewStageFoundationService(t)
	assert.True(t, dps.MatchStage(ctx, testTx, mSFS))

	// pre-req check is a no op
	assert.Nil(t, dps.GetIncompletePreReqTxIDs(ctx, testTx, mSFS))
}

func TestDispatchPrepStageReturnsError(t *testing.T) {
	ctx := context.Background()

	dps := &DispatchPrepStage{}
	testTx := newDispatchPrepStageTx(ctx)
	mSFS := enginemocks.NewStageFoundationService(t)

	/// perform action calls the Domain API and hit error
	mDomainAPI := componentmocks.NewDomainSmartContract(t)
	mDomainAPI.On("PrepareTransaction", ctx, testTx.PrivateTransaction).Return(fmt.Errorf("pop"))
	mSFS.On("DomainAPI").Return(mDomainAPI)
	output, err := dps.PerformAction(ctx, testTx, mSFS)
	assert.Nil(t, output)
	assert.Regexp(t, "pop", err)
}

func TestDispatchPrepStageReturnsPreparedTx(t *testing.T) {
	ctx := context.Background()
	testEthTx := &components.EthTransaction{}

	dps := &DispatchPrepStage{}
	testTx := newDispatchPrepStageTx(ctx)
	mSFS := enginemocks.NewStageFoundationService(t)

	/// perform action calls the Domain API and hit error
	mDomainAPI := componentmocks.NewDomainSmartContract(t)
	mDomainAPI.On("PrepareTransaction", ctx, testTx.PrivateTransaction).Run(func(args mock.Arguments) {
		tx := args[1].(*components.PrivateTransaction)
		tx.PreparedTransaction = testEthTx
	}).Return(nil)
	mSFS.On("DomainAPI").Return(mDomainAPI)
	output, err := dps.PerformAction(ctx, testTx, mSFS)
	assert.NoError(t, err)
	assert.Equal(t, testEthTx, output)
}

func TestDispatchPrepStageProcessEvents(t *testing.T) {
	ctx := context.Background()
	testEthTx := &components.EthTransaction{}

	dps := &DispatchPrepStage{}
	testUnprocessedEvents := []*types.StageEvent{
		{
			Stage: "unknown",
			Data:  "unknown",
		},
		{
			Stage: dps.Name(),
			Data:  testEthTx,
		},
	}
	testTx := newDispatchPrepStageTx(ctx)
	mSFS := enginemocks.NewStageFoundationService(t)
	unprocessedStageEvents, txUpdate, nextStep := dps.ProcessEvents(ctx, testTx, mSFS, testUnprocessedEvents)
	assert.NotNil(t, txUpdate)
	assert.Equal(t, types.NextStepNewStage, nextStep)
	assert.Len(t, unprocessedStageEvents, 1)
	assert.Equal(t, "unknown", unprocessedStageEvents[0].Stage)
}
