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
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/mocks/originatortransactionmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/sequencercommonmocks"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestOriginator_SingleTransactionLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	originatorLocator := "sender@senderNode"
	coordinatorNode := "coordinatorNode"
	builder := NewOriginatorBuilderForTesting(t, State_Idle).CurrentActiveCoordinator(coordinatorNode)
	o, mocks := builder.Build()
	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(0))
	mocks.EngineIntegration.On("CheckPendingPrivateStateData", mock.Anything, mock.Anything).Return(true, nil)
	o.Start(ctx)
	defer func() {
		cancel()
		o.WaitForDone(t.Context())
	}()
	// Ensure the originator is in observing mode by queuing a heartbeat from an active coordinator
	contractAddress := builder.GetContractAddress()
	heartbeatEvent := &common.HeartbeatReceivedEvent{
		FromNode:        coordinatorNode,
		ContractAddress: &contractAddress,
		CoordinatorSnapshot: &common.CoordinatorSnapshot{
			CoordinatorState: common.CoordinatorState_Active,
		},
	}
	o.QueueEvent(ctx, heartbeatEvent)
	sync := statemachine.NewSyncEvent()
	o.QueueEvent(ctx, sync)
	<-sync.Done
	require.Equal(t, State_Observing, o.GetCurrentState(), "Originator should transition to Observing")
	// Start by creating a transaction with the originator
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()
	postAssembly := transactionBuilder.BuildPostAssembly()
	mocks.EngineIntegration.On(
		"Assemble",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(postAssembly.AssembleResponse, nil)
	o.QueueEvent(ctx, &TransactionCreatedEvent{Transaction: txn})
	// Delegation happens asynchronously: the transaction first resolves its required verifiers
	// (Initial → Resolving) and is only delegated once resolution completes (Resolving → Pending).
	require.Eventually(t, mocks.SentMessageRecorder.HasSentDelegationRequest, 2*time.Second, 10*time.Millisecond, "Delegation request should be sent")
	// The transaction is now delegated, so the coordinator sends an assemble request.
	assembleRequestIdempotencyKey := uuid.New()
	o.QueueEvent(ctx, &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{TransactionID: txn.ID},
		RequestID: assembleRequestIdempotencyKey, Coordinator: coordinatorNode,
		CoordinatorBlockHeight: 0,
	})
	// Assembly runs in a background goroutine so use Eventually to wait for the response.
	require.Eventually(t, mocks.SentMessageRecorder.HasSentAssembleSuccessResponse, 2*time.Second, 10*time.Millisecond, "Assemble success response should be sent")
	// Simulate the coordinator sending a dispatch confirmation
	o.QueueEvent(ctx, &transaction.PreDispatchRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{TransactionID: txn.ID},
		RequestID: assembleRequestIdempotencyKey, Coordinator: coordinatorNode,
	})
	sync = statemachine.NewSyncEvent()
	o.QueueEvent(ctx, sync)
	<-sync.Done
	// Assert that a dispatch confirmation was returned
	require.True(t, mocks.SentMessageRecorder.HasSentPreDispatchResponse(), "Pre-dispatch response should be sent")
	// Simulate the coordinator having dispatched the transaction (Prepared → Dispatched) so the
	// following heartbeat with LatestSubmissionHash is accepted (State_Dispatched handles Event_Submitted).
	signerAddress := pldtypes.RandAddress()
	o.QueueEvent(ctx, &transaction.DispatchedEvent{
		BaseEvent:     transaction.BaseEvent{TransactionID: txn.ID},
		SignerAddress: *signerAddress,
		Coordinator:   coordinatorNode,
	})
	// Simulate the coordinator sending a heartbeat after the transaction was submitted
	submissionHash := pldtypes.RandBytes32()
	nonce := uint64(42)
	// Originator must match the originator's nodeName so the heartbeat is applied
	// (builder defaults nodeName to "member1@node1").
	heartbeatEvent.CoordinatorSnapshot = &common.CoordinatorSnapshot{
		DispatchedTransactions: []*common.SnapshotDispatchedTransaction{
			{
				SnapshotPooledTransaction: common.SnapshotPooledTransaction{
					ID: txn.ID,
				},
				Signer:               *signerAddress,
				Nonce:                &nonce,
				LatestSubmissionHash: &submissionHash,
			},
		},
	}
	o.QueueEvent(ctx, heartbeatEvent)
	// Simulate the block indexer confirming the transaction
	o.QueueEvent(ctx, &transaction.ConfirmedSuccessEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
	})
	// After confirmation: the transaction state machine transitions Submitted → Confirmed → Final,
	// and the originator removes the transaction from memory (removeTransaction). With no
	// unconfirmed transactions left, the originator transitions back to State_Observing.
	sync = statemachine.NewSyncEvent()
	o.QueueEvent(ctx, sync)
	<-sync.Done
	require.Nil(t, o.transactionsByID[txn.ID], "Transaction should be cleaned up from transactionsByID after confirmation")
	require.Equal(t, State_Observing, o.GetCurrentState(), "Originator should transition to Observing when all transactions are confirmed")
}

func Test_propagateEventToTransaction_UnknownTransaction_AssembleRequestSendsRejection(t *testing.T) {
	ctx := t.Context()
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(t, State_Observing)
	o, mocks := builder.Build()
	unknownTxID := uuid.New()
	assembleRequestIdempotencyKey := uuid.New()
	event := &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: unknownTxID,
		},
		RequestID:              assembleRequestIdempotencyKey,
		Coordinator:            coordinatorLocator,
		CoordinatorBlockHeight: 1000,
	}
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, event))
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "SendAssembleRejection should be called for unknown transaction")
}

func Test_propagateEventToTransaction_UnknownTransaction_NonRequestEventReturnsNil(t *testing.T) {
	ctx := t.Context()
	builder := NewOriginatorBuilderForTesting(t, State_Observing)
	o, mocks := builder.Build()
	// Create a ConfirmedSuccessEvent for an unknown transaction
	unknownTxID := uuid.New()
	event := &transaction.ConfirmedSuccessEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: unknownTxID,
		},
	}
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, event))
	// Verify that neither rejection was sent
	assert.False(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "Expected SendAssembleRejection to NOT be called for confirmation events")
	assert.False(t, mocks.SentMessageRecorder.HasSentPreDispatchRejection(), "Expected SendPreDispatchRejection to NOT be called for confirmation events")
}

func TestOriginator_CreateTransaction_ErrorFromNewTransaction(t *testing.T) {
	ctx := t.Context()
	builder := NewOriginatorBuilderForTesting(t, State_Observing)
	o, mocks := builder.Build()
	// Nil transaction triggers a handled error; state should remain Observing
	_ = o.stateMachineEventLoop.ProcessEvent(ctx, &TransactionCreatedEvent{Transaction: nil})
	assert.True(t, o.GetCurrentState() == State_Observing, "State should remain Observing after nil transaction event")
	assert.False(t, mocks.SentMessageRecorder.HasSentDelegationRequest(), "No delegation should be sent for nil transaction")
}

func TestOriginator_EventLoop_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	originatorLocator := "sender@senderNode"
	builder := NewOriginatorBuilderForTesting(t, State_Observing)
	o, mocks := builder.Build()
	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(0))
	o.Start(ctx)
	defer func() {
		cancel()
		o.WaitForDone(t.Context())
	}()
	// Queue a TransactionCreatedEvent with a nil transaction to trigger a handled error
	o.QueueEvent(ctx, &TransactionCreatedEvent{Transaction: nil})
	transactionBuilder := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Originator(originatorLocator).NumberOfRequiredEndorsers(1)
	txn := transactionBuilder.BuildSparse()
	// Queue a valid event to verify the originator is still working after the error. Delegation is
	// asynchronous (verifiers are resolved before delegation), so wait for it to be sent.
	o.QueueEvent(ctx, &TransactionCreatedEvent{Transaction: txn})
	require.Eventually(t, mocks.SentMessageRecorder.HasSentDelegationRequest, 2*time.Second, 10*time.Millisecond, "Originator should still process valid event after error")
}

func Test_getTransactionsInStates_ReturnsOnlyTransactionsWhoseStateIsListed(t *testing.T) {
	pendingID := uuid.New()
	delegatedID := uuid.New()
	assemblingID := uuid.New()
	mockPending := originatortransactionmocks.NewOriginatorTransaction(t)
	mockPending.On("GetID").Return(pendingID)
	mockPending.On("GetCurrentState").Return(transaction.State_Pending)
	mockDelegated := originatortransactionmocks.NewOriginatorTransaction(t)
	mockDelegated.On("GetID").Return(delegatedID)
	mockDelegated.On("GetCurrentState").Return(transaction.State_Delegated)
	mockAssembling := originatortransactionmocks.NewOriginatorTransaction(t)
	mockAssembling.On("GetID").Return(assemblingID)
	mockAssembling.On("GetCurrentState").Return(transaction.State_Assembling)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(mockPending, mockDelegated, mockAssembling).
		Build()
	matching := o.getTransactionsInStates([]transaction.State{transaction.State_Pending, transaction.State_Delegated})
	require.Len(t, matching, 2)
	byID := make(map[uuid.UUID]transaction.State)
	for _, txn := range matching {
		byID[txn.GetID()] = txn.GetCurrentState()
	}
	assert.Equal(t, transaction.State_Pending, byID[pendingID])
	assert.Equal(t, transaction.State_Delegated, byID[delegatedID])
}

func Test_getTransactionsInStates_EmptyStateListReturnsNoTransactions(t *testing.T) {
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetCurrentState").Return(transaction.State_Pending)
	o, _ := NewOriginatorBuilderForTesting(t, State_Sending).
		Transactions(mockTxn).
		Build()
	assert.Empty(t, o.getTransactionsInStates(nil))
	assert.Empty(t, o.getTransactionsInStates([]transaction.State{}))
}

func Test_propagateEventToTransaction_UnknownTransaction_PreDispatchRequestSendsRejection(t *testing.T) {
	ctx := t.Context()
	coordinatorLocator := "coordinator@coordinatorNode"
	builder := NewOriginatorBuilderForTesting(t, State_Observing)
	o, mocks := builder.Build()
	unknownTxID := uuid.New()
	event := &transaction.PreDispatchRequestReceivedEvent{
		BaseEvent:   transaction.BaseEvent{TransactionID: unknownTxID},
		Coordinator: coordinatorLocator,
		RequestID:   uuid.New(),
	}
	require.NoError(t, o.stateMachineEventLoop.ProcessEvent(ctx, event))
	assert.True(t, mocks.SentMessageRecorder.HasSentPreDispatchRejection(), "SendPreDispatchRejection should be called for unknown transaction")
	assert.Equal(t, engineProto.RejectionReason_TRANSACTION_UNKNOWN, mocks.SentMessageRecorder.PreDispatchRejectionReason())
}

func Test_GetTxStatus_KnownTransactionReturnsStatus(t *testing.T) {
	ctx := t.Context()
	txID := uuid.New()
	mockTxn := originatortransactionmocks.NewOriginatorTransaction(t)
	mockTxn.On("GetID").Return(txID)
	mockTxn.On("GetStatus", mock.Anything).Return(components.PrivateTxStatus{TxID: txID.String(), Status: "pending"})
	o, _ := NewOriginatorBuilderForTesting(t, State_Observing).Transactions(mockTxn).Build()
	status, err := o.GetTxStatus(ctx, txID)
	require.NoError(t, err)
	assert.Equal(t, txID.String(), status.TxID)
	assert.NotEmpty(t, status.Status)
}

func Test_GetTxStatus_UnknownTransactionReturnsUnknown(t *testing.T) {
	ctx := t.Context()
	builder := NewOriginatorBuilderForTesting(t, State_Observing)
	o, _ := builder.Build()
	unknownID := uuid.New()
	status, err := o.GetTxStatus(ctx, unknownID)
	require.NoError(t, err)
	assert.Equal(t, unknownID.String(), status.TxID)
	assert.Equal(t, "unknown", status.Status)
}

func TestOriginator_Start_Idempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	o, mocks := NewOriginatorBuilderForTesting(t, State_Idle).Build()
	mocks.EngineIntegration.On("GetBlockHeight", mock.Anything).Return(int64(0))
	o.Start(ctx)
	defer func() {
		cancel()
		o.WaitForDone(t.Context())
	}()
	// Second call should be a no-op (idempotent).
	o.Start(ctx)
}

func TestOriginator_WaitForDone_NotStarted_ReturnsImmediately(t *testing.T) {
	ctx := t.Context()
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).Build()
	// Without calling Start, WaitForDone should return immediately.
	done := make(chan struct{})
	go func() {
		o.WaitForDone(ctx)
		close(done)
	}()
	select {
	case <-done:
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("WaitForDone should have returned immediately when not started")
	}
}

func TestNewOriginator_SenderMode_SetsCurrentActiveCoordinatorToNodeName(t *testing.T) {
	const nodeName = "senderNode"
	o := NewOriginator(
		nodeName,
		testutil.NewSentMessageRecorder(),
		sequencercommonmocks.NewEngineIntegration(t),
		pldtypes.RandAddress(),
		&pldconf.SequencerDefaults,
		metrics.InitMetrics(context.Background(), prometheus.NewRegistry()),
		&common.CoordinatorSelectionConfig{
			Mode: prototk.ContractConfig_COORDINATOR_SENDER,
		},
	)
	assert.Equal(t, nodeName, o.currentActiveCoordinator)
}

func TestNewOriginator_EndorserMode_SetsEndorserCandidates(t *testing.T) {
	endorsers := []string{"endorser1@node1", "endorser2@node2"}
	o := NewOriginator(
		"node1",
		testutil.NewSentMessageRecorder(),
		sequencercommonmocks.NewEngineIntegration(t),
		pldtypes.RandAddress(),
		&pldconf.SequencerDefaults,
		metrics.InitMetrics(context.Background(), prometheus.NewRegistry()),
		&common.CoordinatorSelectionConfig{
			Mode:      prototk.ContractConfig_COORDINATOR_ENDORSER,
			Endorsers: endorsers,
		},
	)
	assert.Equal(t, endorsers, o.endorserCandidates)
}

func TestOriginator_StateViewReaderAccessor(t *testing.T) {
	o, _ := NewOriginatorBuilderForTesting(t, State_Idle).Build()
	assert.NotNil(t, o.StateViewReader())
}
