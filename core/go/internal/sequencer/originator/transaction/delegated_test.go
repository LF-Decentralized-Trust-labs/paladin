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
package transaction

import (
	"context"
	"errors"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_Delegated_EmptyCoordinator_ReturnsError(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Pending)
	txn, _ := builder.BuildWithMocks()
	event := &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.pt.ID},
		Coordinator: "",
	}
	err := action_Delegated(ctx, txn, event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction delegate cannot be set to an empty node identity")
}

func Test_action_Delegated_SetsDelegateAndUpdatesTime(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Pending)
	txn, _ := builder.BuildWithMocks()
	coordinator := "coord@node1"
	event := &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.pt.ID},
		Coordinator: coordinator,
	}
	err := action_Delegated(ctx, txn, event)
	require.NoError(t, err)
	assert.Equal(t, coordinator, txn.currentDelegate)
	assert.NotNil(t, txn.lastDelegatedTime)
}

func Test_action_Delegated_FirstDelegatedTime_ResetOnlyOnCoordinatorChange(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Pending)
	txn, _ := builder.BuildWithMocks()

	// First delegation records the first-delegation timestamp for the coordinator.
	require.NoError(t, action_Delegated(ctx, txn, &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.pt.ID},
		Coordinator: "coord@node1",
	}))
	require.NotNil(t, txn.firstDelegatedTime)
	first := txn.firstDelegatedTime

	// Re-delegation to the same coordinator (the partial FIFO resend) must not move it, so the
	// dropped-transaction grace reflects how long the transaction has genuinely been in flight there.
	require.NoError(t, action_Delegated(ctx, txn, &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.pt.ID},
		Coordinator: "coord@node1",
	}))
	assert.Same(t, first, txn.firstDelegatedTime, "re-delegation to the same coordinator must not reset the first-delegation time")

	// Delegation to a different coordinator restarts the grace against the new coordinator's snapshots.
	require.NoError(t, action_Delegated(ctx, txn, &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.pt.ID},
		Coordinator: "coord@node2",
	}))
	assert.NotSame(t, first, txn.firstDelegatedTime, "delegation to a new coordinator must reset the first-delegation time")
}

func TestAction_SendPreDispatchResponse_Success(t *testing.T) {
	// Test that action_SendPreDispatchResponse calls SendPreDispatchResponse with correct parameters
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, mocks := builder.BuildWithMocks()

	// Set up required fields
	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	requestID := uuid.New()
	txn.latestPreDispatchRequestID = requestID

	// Ensure PreAssembly has TransactionSpecification
	transactionSpec := &prototk.TransactionSpecification{
		TransactionId: txn.GetID().String(),
		From:          "originator@node1",
	}
	if txn.pt.PreAssembly == nil {
		txn.pt.PreAssembly = &prototk.TransactionPreAssembly{}
	}
	txn.pt.PreAssembly.TransactionSpecification = transactionSpec

	// Execute the action
	err := action_SendPreDispatchResponse(ctx, txn, nil)

	// Verify no error
	assert.NoError(t, err)

	// Verify that SendPreDispatchResponse was called
	assert.True(t, mocks.SentMessageRecorder.HasSentPreDispatchResponse(), "SendPreDispatchResponse should have been called")
}

func TestAction_SendPreDispatchResponse_TransportError(t *testing.T) {
	// Test that action_SendPreDispatchResponse returns error when transport fails
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated).WithMockTransportWriter()
	txn, mocks := builder.BuildWithMocks()

	// Set up required fields first
	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	requestID := uuid.New()
	txn.latestPreDispatchRequestID = requestID

	// Ensure PreAssembly has TransactionSpecification
	transactionSpec := &prototk.TransactionSpecification{
		TransactionId: txn.GetID().String(),
		From:          "originator@node1",
	}
	if txn.pt.PreAssembly == nil {
		txn.pt.PreAssembly = &prototk.TransactionPreAssembly{}
	}
	txn.pt.PreAssembly.TransactionSpecification = transactionSpec

	expectedError := errors.New("transport error")
	mocks.TransportWriter.EXPECT().SendPreDispatchResponse(
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(expectedError)

	// Execute the action
	err := action_SendPreDispatchResponse(ctx, txn, nil)

	// Verify error is returned
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestValidator_AssembleRequestMatches_Matches(t *testing.T) {
	// Test that validator_AssembleRequestFromCurrentDelegate returns true when coordinator matches
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()

	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator

	event := &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: coordinator,
		RequestID:   uuid.New(),
	}

	matches, err := validator_AssembleRequestFromCurrentDelegate(ctx, txn, event)

	assert.NoError(t, err)
	assert.True(t, matches, "Should return true when coordinator matches")
}

func TestValidator_AssembleRequestMatches_DoesNotMatch(t *testing.T) {
	// Test that validator_AssembleRequestFromCurrentDelegate returns false when coordinator does not match
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()

	coordinator := "coordinator@node1"
	differentCoordinator := "coordinator@node2"
	txn.currentDelegate = coordinator

	event := &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: differentCoordinator,
		RequestID:   uuid.New(),
	}

	matches, err := validator_AssembleRequestFromCurrentDelegate(ctx, txn, event)

	assert.NoError(t, err)
	assert.False(t, matches, "Should return false when coordinator does not match")
}

func TestValidator_AssembleRequestMatches_WrongEventType(t *testing.T) {
	// Test that validator_AssembleRequestFromCurrentDelegate returns false when event type is wrong
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()

	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator

	// Use a different event type
	event := &DelegatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: coordinator,
	}

	matches, err := validator_AssembleRequestFromCurrentDelegate(ctx, txn, event)

	assert.NoError(t, err)
	assert.False(t, matches, "Should return false when event type is wrong")
}

func TestValidator_PreDispatchRequestMatchesAssembledDelegation_Success(t *testing.T) {
	// Test that validator_PreDispatchRequestFromCurrentDelegate returns true when coordinator and hash match
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	// Set up transaction with PostAssembly so Hash() works
	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			Signatures: []*prototk.AttestationResult{
				{
					Payload: []byte("test signature"),
				},
			},
		},
	}

	requestID := uuid.New()
	event := &PreDispatchRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: coordinator,
		RequestID:   requestID,
	}

	matches, err := validator_PreDispatchRequestFromCurrentDelegate(ctx, txn, event)

	assert.NoError(t, err)
	assert.True(t, matches, "Should return true when the request is from the current delegate")
	// Note: request ID is stored by action_PreDispatchRequestReceived (first action) when the event is processed via HandleEvent
}

func TestValidator_PreDispatchRequestFromCurrentDelegate_WrongCoordinator(t *testing.T) {
	// Test that validator_PreDispatchRequestFromCurrentDelegate returns false when coordinator does not match.
	// No transport call is expected — the send is handled by action_SendPreDispatchRejectionNotCurrentDelegate.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	coordinator := "coordinator@node1"
	differentCoordinator := "coordinator@node2"
	txn.currentDelegate = coordinator
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			Signatures: []*prototk.AttestationResult{
				{
					Payload: []byte("test signature"),
				},
			},
		},
	}

	event := &PreDispatchRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: differentCoordinator,
		RequestID:   uuid.New(),
	}

	matches, err := validator_PreDispatchRequestFromCurrentDelegate(ctx, txn, event)

	assert.NoError(t, err)
	assert.False(t, matches, "Should return false when coordinator does not match")
	assert.Equal(t, uuid.Nil, txn.latestPreDispatchRequestID, "Should not store request ID when validation fails")
}

func TestValidator_PreDispatchRequestMatchesAssembledDelegation_WrongEventType(t *testing.T) {
	// Test that validator_PreDispatchRequestFromCurrentDelegate returns false when event type is wrong
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, _ := builder.BuildWithMocks()

	coordinator := "coordinator@node1"
	txn.currentDelegate = coordinator

	// Use a different event type
	event := &DelegatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: coordinator,
	}

	matches, err := validator_PreDispatchRequestFromCurrentDelegate(ctx, txn, event)

	assert.NoError(t, err)
	assert.False(t, matches, "Should return false when event type is wrong")
}

func Test_action_ResetDelegationState_ClearsAssemblyAndDispatchState(t *testing.T) {
	ctx := context.Background()
	// Start in State_Assembling — state machine has a top-level validator for Event_Delegated in
	// this state that checks ValidatorNot(validator_CoordinatorIsCurrentDelegate), so a re-delegation
	// from a DIFFERENT coordinator triggers action_ResetDelegationState.
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, _ := builder.BuildWithMocks()

	// Populate assembly state that should be cleared on re-delegation.
	txn.latestFulfilledAssembleRequestID = uuid.New()
	txn.signerAddress = pldtypes.RandAddress()
	nonce := uint64(42)
	txn.nonce = &nonce
	txn.latestPreDispatchRequestID = uuid.New()
	submissionHash := pldtypes.RandBytes32()
	txn.latestSubmissionHash = &submissionHash

	// Send DelegatedEvent from a DIFFERENT coordinator than the current delegate.
	// The ValidatorNot(validator_CoordinatorIsCurrentDelegate) passes because coordinator differs.
	newCoordinator := "new-coordinator@node2"
	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.pt.ID},
		Coordinator: newCoordinator,
	})
	require.NoError(t, err)

	// State should transition back to Delegated.
	assert.Equal(t, State_Delegated, txn.GetCurrentState())

	// action_ResetDelegationState should have cleared all assembly/dispatch state.
	assert.Nil(t, txn.latestAssembleRequest)
	assert.Equal(t, uuid.Nil, txn.latestFulfilledAssembleRequestID)
	assert.Equal(t, uuid.Nil, txn.latestPreDispatchRequestID)
	assert.Nil(t, txn.signerAddress)
	assert.Nil(t, txn.latestSubmissionHash)
	assert.Nil(t, txn.nonce)
	// currentDelegate should now be the new coordinator (set by action_Delegated).
	assert.Equal(t, newCoordinator, txn.currentDelegate)
}

func Test_validator_CoordinatorIsCurrentDelegate_WrongCoordinator_ReturnsFalse(t *testing.T) {
	// Exercises the return false, nil branch — event implements EventWithCoordinator but
	// the coordinator doesn't match the current delegate.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, _ := builder.BuildWithMocks()

	txn.currentDelegate = "coord@node1"

	// DispatchedEvent implements EventWithCoordinator.
	event := &DispatchedEvent{
		BaseEvent:     BaseEvent{TransactionID: txn.GetID()},
		Coordinator:   "other@node2",
		SignerAddress: *pldtypes.RandAddress(),
	}

	ok, err := validator_CoordinatorIsCurrentDelegate(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, ok, "should return false when coordinator does not match currentDelegate")
}

func TestAction_SendAssembleRejectionNotCurrentDelegate_Success(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated).WithMockTransportWriter()
	txn, mocks := builder.BuildWithMocks()

	txn.currentDelegate = "coordinator@node1"
	reqID := uuid.New()
	event := &AssembleRequestReceivedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		Coordinator: "other@node2",
		RequestID:   reqID,
	}

	mocks.TransportWriter.EXPECT().
		SendAssembleRejection(mock.Anything, "other@node2", mock.MatchedBy(func(msg *engineProto.AssembleRejection) bool {
			return msg.TransactionId == txn.GetID().String() &&
				msg.AssembleRequestId == reqID.String() &&
				msg.RejectionReason == engineProto.RejectionReason_NOT_CURRENT_DELEGATE
		})).
		Return(nil)

	err := action_SendAssembleRejectionNotCurrentDelegate(ctx, txn, event)
	require.NoError(t, err)
}

func TestAction_SendAssembleRejectionNotCurrentDelegate_TransportError_LogsWarnAndReturnsNil(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated).WithMockTransportWriter()
	txn, mocks := builder.BuildWithMocks()

	txn.currentDelegate = "coordinator@node1"
	reqID := uuid.New()
	event := &AssembleRequestReceivedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		Coordinator: "other@node2",
		RequestID:   reqID,
	}

	mocks.TransportWriter.EXPECT().
		SendAssembleRejection(mock.Anything, "other@node2", mock.MatchedBy(func(msg *engineProto.AssembleRejection) bool {
			return msg.TransactionId == txn.GetID().String() &&
				msg.AssembleRequestId == reqID.String() &&
				msg.RejectionReason == engineProto.RejectionReason_NOT_CURRENT_DELEGATE
		})).
		Return(errors.New("transport error"))

	err := action_SendAssembleRejectionNotCurrentDelegate(ctx, txn, event)
	require.NoError(t, err, "transport error must be logged and swallowed, not returned as action error")
}

func TestAction_SendPreDispatchRejectionNotCurrentDelegate_Success(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Prepared).WithMockTransportWriter()
	txn, mocks := builder.BuildWithMocks()

	txn.currentDelegate = "coordinator@node1"
	reqID := uuid.New()
	event := &PreDispatchRequestReceivedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		Coordinator: "other@node2",
		RequestID:   reqID,
	}

	mocks.TransportWriter.EXPECT().
		SendPreDispatchRejection(mock.Anything, "other@node2", mock.MatchedBy(func(msg *engineProto.PreDispatchRejection) bool {
			return msg.TransactionId == txn.GetID().String() &&
				msg.RequestId == reqID.String() &&
				msg.RejectionReason == engineProto.RejectionReason_NOT_CURRENT_DELEGATE
		})).
		Return(nil)

	err := action_SendPreDispatchRejectionNotCurrentDelegate(ctx, txn, event)
	require.NoError(t, err)
}

func TestAction_SendPreDispatchRejectionNotCurrentDelegate_TransportError_LogsWarnAndReturnsNil(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Prepared).WithMockTransportWriter()
	txn, mocks := builder.BuildWithMocks()

	txn.currentDelegate = "coordinator@node1"
	reqID := uuid.New()
	event := &PreDispatchRequestReceivedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		Coordinator: "other@node2",
		RequestID:   reqID,
	}

	mocks.TransportWriter.EXPECT().
		SendPreDispatchRejection(mock.Anything, "other@node2", mock.MatchedBy(func(msg *engineProto.PreDispatchRejection) bool {
			return msg.TransactionId == txn.GetID().String() &&
				msg.RequestId == reqID.String() &&
				msg.RejectionReason == engineProto.RejectionReason_NOT_CURRENT_DELEGATE
		})).
		Return(errors.New("transport error"))

	err := action_SendPreDispatchRejectionNotCurrentDelegate(ctx, txn, event)
	require.NoError(t, err, "transport error must be logged and swallowed, not returned as action error")
}
