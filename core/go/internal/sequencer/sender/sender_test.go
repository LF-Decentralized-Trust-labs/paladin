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

package sender

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/sender/transaction"
	"github.com/kaleido-io/paladin/core/internal/sequencer/testutil"
	"github.com/kaleido-io/paladin/core/mocks/sequencermocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func NewSenderForUnitTest(t *testing.T, ctx context.Context, committeeMembers []string) (*sender, *senderDependencyMocks) {

	if committeeMembers == nil {
		committeeMembers = []string{"member1@node1"}
	}
	mocks := &senderDependencyMocks{
		messageSender:     NewMockMessageSender(t),
		clock:             &common.FakeClockForTesting{},
		engineIntegration: sequencermocks.NewEngineIntegration(t),
		emit:              func(event common.Event) {},
	}

	sender, err := NewSender(
		ctx,
		"member1@node1",
		mocks.messageSender,
		committeeMembers,
		mocks.clock,
		mocks.emit,
		mocks.engineIntegration,
		100,
		tktypes.RandAddress(),
		5,
		5,
	)
	require.NoError(t, err)

	return sender, mocks
}

type senderDependencyMocks struct {
	messageSender     *MockMessageSender
	clock             *common.FakeClockForTesting
	engineIntegration *sequencermocks.EngineIntegration
	emit              common.EmitEvent
}

func TestSender_SingleTransactionLifecycle(t *testing.T) {
	// Test the progression of a single transaction through the sender's lifecycle
	// Simulating coordinator node by inspecting the sender output messages and by sending events that would normally be triggered
	//  by coordinator node sending messages to the transaction sender.
	// At each stage, we inspect the state of the transaction by querying the sender's status API

	ctx := context.Background()
	senderLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewSenderBuilderForTesting(State_Idle).CommitteeMembers(senderLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)

	//ensure the sender is in observing mode by emulating a heartbeat from an active coordinator
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	err := s.HandleEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)

	// Start by creating a transaction with the sender
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Sender(senderLocator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()
	err = s.HandleEvent(ctx, &TransactionCreatedEvent{
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
	err = s.HandleEvent(ctx, &transaction.AssembleRequestReceivedEvent{
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
	err = s.HandleEvent(ctx, &transaction.DispatchConfirmationRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:        assembleRequestIdempotencyKey,
		Coordinator:      coordinatorLocator,
		PostAssemblyHash: postAssemblyHash,
	})
	assert.NoError(t, err)

	// Assert that a dispatch confirmation was returned
	assert.True(t, mocks.SentMessageRecorder.HasSentDispatchConfirmationResponse())

	//simulate the coordinator sending a heartbeat after the transaction was submitted
	signerAddress := tktypes.RandAddress()
	submissionHash := tktypes.RandBytes32()
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
	err = s.HandleEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)

	// Simulate the block indexer confirming the transaction
	err = s.HandleEvent(ctx, &TransactionConfirmedEvent{
		From:  signerAddress,
		Nonce: 42,
		Hash:  submissionHash,
	})
	assert.NoError(t, err)

}

func TestSender_DelegateDroppedTransactions(t *testing.T) {
	//delegate a transaction then receive a heartbeat that does not contain that transaction, and check that
	// it continues to get re-delegated until it is in included in a heartbeat

	ctx := context.Background()
	senderLocator := "sender@senderNode"
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewSenderBuilderForTesting(State_Idle).CommitteeMembers(senderLocator, coordinatorLocator)
	s, mocks := builder.Build(ctx)

	//ensure the sender is in observing mode by emulating a heartbeat from an active coordinator
	heartbeatEvent := &HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent.ContractAddress = &contractAddress

	err := s.HandleEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)

	transactionBuilder1 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Sender(senderLocator).NumberOfRequiredEndorsers(1)
	txn1 := transactionBuilder1.BuildSparse()
	err = s.HandleEvent(ctx, &TransactionCreatedEvent{
		Transaction: txn1,
	})
	assert.NoError(t, err)

	// Assert that a delegation request has been sent to the coordinator
	require.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())
	mocks.SentMessageRecorder.Reset(ctx)

	transactionBuilder2 := testutil.
		NewPrivateTransactionBuilderForTesting().
		Address(builder.GetContractAddress()).
		Sender(senderLocator).
		NumberOfRequiredEndorsers(1)
	txn2 := transactionBuilder2.BuildSparse()
	err = s.HandleEvent(ctx, &TransactionCreatedEvent{
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
			ID:     txn1.ID,
			Sender: senderLocator,
		},
	}

	err = s.HandleEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)

	require.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())
	require.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(txn1.ID))
	require.True(t, mocks.SentMessageRecorder.HasDelegatedTransaction(txn2.ID))

}
