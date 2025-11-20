/*
 * Copyright © 2025 Kaleido, Inc.
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

package originator

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func NewOriginatorForUnitTest(t *testing.T, ctx context.Context, originatorIdentityPool []string) (*originator, *originatorDependencyMocks) {

	if originatorIdentityPool == nil {
		originatorIdentityPool = []string{"member1@node1"}
	}
	mocks := &originatorDependencyMocks{
		transportWriter:   transport.NewMockTransportWriter(t),
		clock:             &common.FakeClockForTesting{},
		engineIntegration: common.NewMockEngineIntegration(t),
		emit:              func(event common.Event) {},
	}

	originator, err := NewOriginator(
		ctx,
		"member1@node1",
		mocks.transportWriter,
		mocks.clock,
		mocks.engineIntegration,
		100,
		pldtypes.RandAddress(),
		5,
		5,
		nil,
	)
	require.NoError(t, err)

	return originator, mocks
}

type originatorDependencyMocks struct {
	transportWriter   *transport.MockTransportWriter
	clock             *common.FakeClockForTesting
	engineIntegration *common.MockEngineIntegration
	emit              common.EmitEvent
}

func TestOriginator_SingleTransactionLifecycle(t *testing.T) {
	// Test the progression of a single transaction through the originator's lifecycle
	// Simulating coordinator node by inspecting the originator output messages and by sending events that would normally be triggered
	//  by coordinator node sending messages to the transaction originator.
	// At each stage, we inspect the state of the transaction by querying the seoriginatornder's status API

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)

	//ensure the originator is in observing mode by emulating a heartbeat from an active coordinator
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	err := s.ProcessEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)
	assert.True(t, s.GetCurrentState() == State_Observing)

	// Start by creating a transaction with the originator
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()
	err = s.ProcessEvent(ctx, &TransactionCreatedEvent{
		Transaction: txn,
	})
	assert.NoError(t, err)

	// Assert that a delegation request has been sent to the coordinator
	require.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())

	postAssembly, postAssemblyHash := transactionBuilder.BuildPostAssemblyAndHash()
	mocks.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(postAssembly, nil)

	//Simulate the coordinator sending an assemble request
	assembleRequestIdempotencyKey := uuid.New()
	err = s.ProcessEvent(ctx, &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:               assembleRequestIdempotencyKey,
		Coordinator:             coordinatorLocator,
		CoordinatorsBlockHeight: 1000,
		StateLocksJSON:          []byte("{}"),
	})
	assert.NoError(t, err)

	// Assert that the transaction was assembled and a response sent
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleSuccessResponse())

	//Simulate the coordinator sending a dispatch confirmation
	err = s.ProcessEvent(ctx, &transaction.PreDispatchRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:        assembleRequestIdempotencyKey,
		Coordinator:      coordinatorLocator,
		PostAssemblyHash: postAssemblyHash,
	})
	assert.NoError(t, err)

	// Assert that a dispatch confirmation was returned
	assert.True(t, mocks.SentMessageRecorder.HasSentPreDispatchResponse())

	//simulate the coordinator sending a heartbeat after the transaction was submitted
	signerAddress := pldtypes.RandAddress()
	submissionHash := pldtypes.RandBytes32()
	nonce := uint64(42)
	heartbeatEvent.DispatchedTransactions = []*common.DispatchedTransaction{
		{
			Transaction: common.Transaction{
				ID: txn.ID,
			},
			Signer:               *signerAddress,
			SignerLocator:        "signer@node2",
			Nonce:                &nonce,
			LatestSubmissionHash: &submissionHash,
		},
	}
	err = s.ProcessEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)

	// Simulate the block indexer confirming the transaction
	err = s.ProcessEvent(ctx, &TransactionConfirmedEvent{
		From:  signerAddress,
		Nonce: 42,
		Hash:  submissionHash,
	})
	assert.NoError(t, err)

}

func TestOriginator_DelegateDroppedTransactions(t *testing.T) {
	//delegate a transaction then receive a heartbeat that does not contain that transaction, and check that
	// it continues to get re-delegated until it is in included in a heartbeat

	ctx := context.Background()
	originatorLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(State_Idle).CommitteeMembers(originatorLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)

	//ensure the originator is in observing mode by emulating a heartbeat from an active coordinator
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	err := s.ProcessEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)
	assert.True(t, s.GetCurrentState() == State_Observing)

	transactionBuilder1 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn1 := transactionBuilder1.BuildSparse()
	err = s.ProcessEvent(ctx, &TransactionCreatedEvent{
		Transaction: txn1,
	})
	assert.NoError(t, err)

	// Assert that a delegation request has been sent to the coordinator
	require.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())
	mocks.SentMessageRecorder.Reset(ctx)

	transactionBuilder2 := testutil.
		NewPrivateTransactionBuilderForTesting().
		Address(builder.GetContractAddress()).
		Originator(originatorLocator).
		NumberOfRequiredEndorsers(1)
	txn2 := transactionBuilder2.BuildSparse()
	err = s.ProcessEvent(ctx, &TransactionCreatedEvent{
		Transaction: txn2,
	})
	assert.NoError(t, err)

	// Assert that a delegation request has been sent to the coordinator
	require.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())
	mocks.SentMessageRecorder.Reset(ctx)

	heartbeatEvent = &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	heartbeatEvent.ContractAddress = &contractAddress
	heartbeatEvent.PooledTransactions = []*common.Transaction{
		{
			ID:         txn1.ID,
			Originator: originatorLocator,
		},
	}

	err = s.ProcessEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)

	require.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())

	require.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(txn1.ID))
	require.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(txn2.ID))

}
