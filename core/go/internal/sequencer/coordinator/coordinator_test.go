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

package coordinator

import (
	"context"
	"testing"

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/kaleido-io/paladin/core/internal/sequencer/testutil"
	"github.com/kaleido-io/paladin/core/mocks/sequencermocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionStateTransition(t *testing.T) {

}

func NewCoordinatorForUnitTest(t *testing.T, ctx context.Context, committeeMembers []string) (*coordinator, *coordinatorDependencyMocks) {

	if committeeMembers == nil {
		committeeMembers = []string{"member1@node1"}
	}
	mocks := &coordinatorDependencyMocks{
		messageSender:     NewMockMessageSender(t),
		clock:             &common.FakeClockForTesting{},
		engineIntegration: sequencermocks.NewEngineIntegration(t),
		emit:              func(event common.Event) {},
	}

	coordinator, err := NewCoordinator(ctx, mocks.messageSender, committeeMembers, mocks.clock, mocks.emit, mocks.engineIntegration, mocks.clock.Duration(1000), mocks.clock.Duration(5000), 100, tktypes.RandAddress(), 5, 5)
	require.NoError(t, err)

	return coordinator, mocks
}

type coordinatorDependencyMocks struct {
	messageSender     *MockMessageSender
	clock             *common.FakeClockForTesting
	engineIntegration *sequencermocks.EngineIntegration
	emit              common.EmitEvent
}

func TestCoordinator_SingleTransactionLifecycle(t *testing.T) {
	// Test the progression of a single transaction through the coordinator's lifecycle
	// Simulating sender node, endorser node and the public transaction manager (submitter)
	// by inspecting the coordinator output messages and by sending events that would normally be triggered by those components sending messages to the coordinator.
	// At each stage, we inspect the state of the coordinator by checking the snapshot it produces on heartbeat messages

	ctx := context.Background()
	sender := "sender@senderNode"
	builder := NewCoordinatorBuilderForTesting(State_Idle).CommitteeMembers(sender)
	c, mocks := builder.Build(ctx)

	// Start by simulating the sender and delegate a transaction to the coordinator
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Sender(sender).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()
	err := c.HandleEvent(ctx, &TransactionsDelegatedEvent{
		Sender:       sender,
		Transactions: []*components.PrivateTransaction{txn},
	})
	assert.NoError(t, err)

	// Assert that snapshot contains a transaction with matching ID
	snapshot := c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	require.Equal(t, 1, len(snapshot.PooledTransactions))
	assert.Equal(t, txn.ID.String(), snapshot.PooledTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())

	// Assert that a request has been sent to the sender and respond with an assembled transaction
	require.True(t, mocks.SentMessageRecorder.HasSentAssembleRequest())
	err = c.HandleEvent(ctx, &transaction.AssembleSuccessEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:    mocks.SentMessageRecorder.SentAssembleRequestIdempotencyKey(),
		PostAssembly: transactionBuilder.BuildPostAssembly(),
	})
	assert.NoError(t, err)

	// Assert that snapshot still contains the same single transaction in the pooled transactions
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	require.Equal(t, 1, len(snapshot.PooledTransactions))
	assert.Equal(t, txn.ID.String(), snapshot.PooledTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())

	// Assert that the coordinator has sent an endorsement request to the endorser and respond with an endorsement
	require.Equal(t, 1, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests())
	err = c.HandleEvent(ctx, &transaction.EndorsedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:   mocks.SentMessageRecorder.SentEndorsementRequestsForPartyIdempotencyKey(transactionBuilder.GetEndorserIdentityLocator(0)),
		Endorsement: transactionBuilder.BuildEndorsement(0),
	})
	assert.NoError(t, err)

	// Assert that snapshot still contains the same single transaction in the pooled transactions
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	require.Equal(t, 1, len(snapshot.PooledTransactions))
	assert.Equal(t, txn.ID.String(), snapshot.PooledTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())

	// Assert that the coordinator has sent a dispatch confirmation request to the transaction sender and respond with a dispatch confirmation
	require.True(t, mocks.SentMessageRecorder.HasSentDispatchConfirmationRequest())
	err = c.HandleEvent(ctx, &transaction.DispatchConfirmedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID: mocks.SentMessageRecorder.SentDispatchConfirmationRequestIdempotencyKey(),
	})
	assert.NoError(t, err)

	// Assert that snapshot no longer contains that transaction in the pooled transactions but does contain it in the dispatched transactions
	//NOTE: This is a key design point.  When a transaction is ready to be dispatched, we communicate to other nodes, via the heartbeat snapshot, that the transaction is dispatched.
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	assert.Equal(t, 0, len(snapshot.PooledTransactions))
	require.Equal(t, 1, len(snapshot.DispatchedTransactions), "Snapshot should contain exactly one dispatched transaction")
	assert.Equal(t, txn.ID.String(), snapshot.DispatchedTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())

	// Assert that the transaction is ready to be collected by the dispatcher thread
	readyTransactions, err := c.GetTransactionsReadyToDispatch(ctx)
	require.NoError(t, err)
	require.NotNil(t, readyTransactions)
	require.Equal(t, 1, len(readyTransactions), "There should be exactly one transaction ready to dispatch")
	assert.Equal(t, txn.ID.String(), readyTransactions[0].ID.String(), "The transaction ready to dispatch should match the delegated transaction ID")

	// Simulate the dispatcher thread collecting the transaction and dispatching it to a public transaction manager with a signing address
	signerAddress := tktypes.RandAddress()
	err = c.HandleEvent(ctx, &transaction.CollectedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		SignerAddress: *signerAddress,
	})
	assert.NoError(t, err)

	// Assert that we now have a signer address in the snapshot
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	assert.Equal(t, 0, len(snapshot.PooledTransactions))
	require.Equal(t, 1, len(snapshot.DispatchedTransactions), "Snapshot should contain exactly one dispatched transaction")
	assert.Equal(t, txn.ID.String(), snapshot.DispatchedTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())
	assert.Equal(t, signerAddress.String(), snapshot.DispatchedTransactions[0].Signer.String(), "Snapshot should contain the dispatched transaction with signer address %s", signerAddress.String())

	// Simulate the dispatcher thread allocating a nonce for the transaction
	err = c.HandleEvent(ctx, &transaction.NonceAllocatedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		Nonce: 42,
	})
	assert.NoError(t, err)

	// Assert that the nonce is now included in the snapshot
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	assert.Equal(t, 0, len(snapshot.PooledTransactions))
	require.Equal(t, 1, len(snapshot.DispatchedTransactions), "Snapshot should contain exactly one dispatched transaction")
	assert.Equal(t, txn.ID.String(), snapshot.DispatchedTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())
	assert.Equal(t, signerAddress.String(), snapshot.DispatchedTransactions[0].Signer.String(), "Snapshot should contain the dispatched transaction with signer address %s", signerAddress.String())
	require.NotNil(t, snapshot.DispatchedTransactions[0].Nonce, "Snapshot should contain the dispatched transaction with a nonce")
	assert.Equal(t, uint64(42), *snapshot.DispatchedTransactions[0].Nonce, "Snapshot should contain the dispatched transaction with nonce 42")

	// Simulate the public transaction manager submitting the transaction
	submissionHash := tktypes.Bytes32(tktypes.RandBytes(32))
	err = c.HandleEvent(ctx, &transaction.SubmittedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		SubmissionHash: submissionHash,
	})
	assert.NoError(t, err)

	// Assert that the hash is now included in the snapshot
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	assert.Equal(t, 0, len(snapshot.PooledTransactions))
	require.Equal(t, 1, len(snapshot.DispatchedTransactions), "Snapshot should contain exactly one dispatched transaction")
	assert.Equal(t, txn.ID.String(), snapshot.DispatchedTransactions[0].ID.String(), "Snapshot should contain the dispatched transaction with ID %s", txn.ID.String())
	assert.Equal(t, signerAddress.String(), snapshot.DispatchedTransactions[0].Signer.String(), "Snapshot should contain the dispatched transaction with signer address %s", signerAddress.String())
	require.NotNil(t, snapshot.DispatchedTransactions[0].Nonce, "Snapshot should contain the dispatched transaction with a nonce")
	assert.Equal(t, uint64(42), *snapshot.DispatchedTransactions[0].Nonce, "Snapshot should contain the dispatched transaction with nonce 42")
	require.NotNil(t, snapshot.DispatchedTransactions[0].LatestSubmissionHash, "Snapshot should contain the dispatched transaction with a submission hash")

	// Simulate the block indexer confirming the transaction
	err = c.HandleEvent(ctx, &transaction.ConfirmedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		Nonce: 42,
		Hash:  submissionHash,
	})
	assert.NoError(t, err)

	// Assert that snapshot contains a transaction with matching ID
	snapshot = c.getSnapshot(ctx)
	require.NotNil(t, snapshot)

	assert.Equal(t, 0, len(snapshot.DispatchedTransactions))
	assert.Equal(t, 0, len(snapshot.PooledTransactions))
	assert.Equal(t, 1, len(snapshot.ConfirmedTransactions))
	assert.Equal(t, txn.ID.String(), snapshot.ConfirmedTransactions[0].ID.String())
	assert.Equal(t, signerAddress.String(), snapshot.ConfirmedTransactions[0].Signer.String())
	require.NotNil(t, snapshot.ConfirmedTransactions[0].Nonce)
	assert.Equal(t, uint64(42), *snapshot.ConfirmedTransactions[0].Nonce)
	assert.Equal(t, submissionHash, *snapshot.ConfirmedTransactions[0].LatestSubmissionHash)

}

func TestCoordinator_HandoverFlush(t *testing.T) {
}
