/*
 * Copyright © 2026 Kaleido, Inc.
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
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_HandleEvent_ProcessesEvent(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Pending)
	txn, mocks := builder.BuildWithMocks()
	coordinator := "coord@node1"
	event := &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.pt.ID},
		Coordinator: coordinator,
	}
	err := txn.HandleEvent(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, coordinator, txn.currentDelegate)
	// Transition callback should have been invoked
	<-mocks.Events
}

func Test_initializeStateMachine_InvokesTransitionCallback(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, mocks := builder.BuildWithMocks()
	// Drive a transition so the callback runs (Created -> Pending)
	event := &CreatedEvent{
		BaseEvent: BaseEvent{TransactionID: txn.pt.ID},
	}
	err := txn.HandleEvent(ctx, event)
	require.NoError(t, err)
	// Should have emitted a state transition event
	_, ok := (<-mocks.Events).(*common.TransactionStateTransitionEvent[State])
	require.True(t, ok)
}

func Test_HandleEvent_ConfirmedReverted_WillRetry_TransitionsToDelegated(t *testing.T) {
	ctx := context.Background()
	// Post-dispatch states support retryable reverts by returning to State_Delegated.
	states := []State{
		State_Dispatched,
		State_Sequenced,
		State_Submitted,
	}

	for _, state := range states {
		t.Run(state.String(), func(t *testing.T) {
			builder := NewTransactionBuilderForTesting(t, state)
			txn, _ := builder.BuildWithMocks()
			err := txn.HandleEvent(ctx, &ConfirmedRevertedEvent{
				BaseEvent: BaseEvent{TransactionID: txn.pt.ID},
				WillRetry: true,
			})
			require.NoError(t, err)
			assert.Equal(t, State_Delegated, txn.GetCurrentState())
		})
	}
}

func Test_HandleEvent_ConfirmedReverted_WillNotRetry_TransitionsToConfirmed(t *testing.T) {
	ctx := context.Background()
	// All non-final states transition to State_Confirmed on a non-retryable revert,
	// mirroring the ConfirmedSuccess fast-forward path used by snapshot recovery.
	states := []State{
		State_Pending,
		State_Delegated,
		State_Assembling,
		State_Endorsement_Gathering,
		State_Prepared,
		State_Dispatched,
		State_Sequenced,
		State_Submitted,
		State_Parked,
	}

	for _, state := range states {
		t.Run(state.String(), func(t *testing.T) {
			builder := NewTransactionBuilderForTesting(t, state)
			txn, _ := builder.BuildWithMocks()
			err := txn.HandleEvent(ctx, &ConfirmedRevertedEvent{
				BaseEvent: BaseEvent{TransactionID: txn.pt.ID},
				WillRetry: false,
			})
			require.NoError(t, err)
			assert.Equal(t, State_Confirmed, txn.GetCurrentState())
		})
	}
}

func Test_HandleEvent_ConfirmedSuccess_AllNonFinalStates(t *testing.T) {
	ctx := context.Background()
	states := []State{
		State_Initial,
		State_Pending,
		State_Delegated,
		State_Assembling,
		State_Endorsement_Gathering,
		State_Prepared,
		State_Dispatched,
		State_Sequenced,
		State_Submitted,
		State_Parked,
	}

	for _, state := range states {
		t.Run(state.String(), func(t *testing.T) {
			builder := NewTransactionBuilderForTesting(t, state)
			txn, _ := builder.BuildWithMocks()
			err := txn.HandleEvent(ctx, &ConfirmedSuccessEvent{
				BaseEvent: BaseEvent{TransactionID: txn.pt.ID},
			})
			require.NoError(t, err)
			assert.Equal(t, State_Confirmed, txn.GetCurrentState())
		})
	}
}

func TestOriginatorTransaction_InitializeOK(t *testing.T) {
	txn := NewTransactionBuilderForTesting(t, State_Initial).Build()
	assert.NotNil(t, txn)
	assert.Equal(t, State_Initial, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Initial_ToResolving_OnCreated(t *testing.T) {
	ctx := context.Background()

	txn := NewTransactionBuilderForTesting(t, State_Initial).Build()
	assert.Equal(t, State_Initial, txn.GetCurrentState())

	err := txn.HandleEvent(ctx, &CreatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Resolving, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Initial_ToPending_OnCreated_NoRequiredVerifiers(t *testing.T) {
	ctx := context.Background()

	txn := NewTransactionBuilderForTesting(t, State_Initial).Build()
	// A transaction with no verifiers to resolve skips State_Resolving entirely.
	txn.pt.PreAssembly = &prototk.TransactionPreAssembly{}
	assert.Equal(t, State_Initial, txn.GetCurrentState())

	err := txn.HandleEvent(ctx, &CreatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Pending, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Resolving_ConfirmedSuccess_ToConfirmed(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Resolving).Build()

	require.NoError(t, txn.HandleEvent(ctx, &ConfirmedSuccessEvent{
		BaseEvent: BaseEvent{TransactionID: txn.GetID()},
	}))

	assert.Equal(t, State_Confirmed, txn.GetCurrentState())
}

func TestOriginatorTransaction_Resolving_ConfirmedReverted_ToConfirmed(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Resolving).Build()

	require.NoError(t, txn.HandleEvent(ctx, &ConfirmedRevertedEvent{
		BaseEvent: BaseEvent{TransactionID: txn.GetID()},
		WillRetry: false,
	}))

	assert.Equal(t, State_Confirmed, txn.GetCurrentState())
}

func TestOriginatorTransaction_Resolving_ConfirmedReverted_WillRetry_StaysResolving(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Resolving).Build()

	// The only ConfirmedReverted handler in State_Resolving requires a non-retryable revert. A retryable
	// revert is inherently being retried by the fact we're progressing a newly loaded transaction in State_Resolving.
	require.NoError(t, txn.HandleEvent(ctx, &ConfirmedRevertedEvent{
		BaseEvent: BaseEvent{TransactionID: txn.GetID()},
		WillRetry: true,
	}))

	assert.Equal(t, State_Resolving, txn.GetCurrentState())
}

func TestOriginatorTransaction_Resolving_ToPending_OnVerifiersResolved(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Resolving).Build()
	resolved := []*prototk.ResolvedVerifier{{Lookup: "alice@node1", Verifier: "0xabc"}}

	require.NoError(t, txn.HandleEvent(ctx, &VerifiersResolvedEvent{
		BaseEvent:         BaseEvent{TransactionID: txn.GetID()},
		ResolvedVerifiers: resolved,
	}))

	assert.Equal(t, State_Pending, txn.GetCurrentState())
	assert.Equal(t, resolved, txn.pt.ResolvedVerifiers)
}

func TestOriginatorTransaction_Resolving_LeavingStateCancelsRetry(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Resolving).Build()
	cancelled := false
	txn.cancelResolveRetry = func() { cancelled = true }

	require.NoError(t, txn.HandleEvent(ctx, &VerifiersResolvedEvent{
		BaseEvent: BaseEvent{TransactionID: txn.GetID()},
	}))

	assert.Equal(t, State_Pending, txn.GetCurrentState())
	assert.True(t, cancelled, "leaving State_Resolving must cancel any pending retry timer")
	assert.Nil(t, txn.cancelResolveRetry)
}

func TestOriginatorTransaction_Resolving_StaysResolving_OnFailure_SchedulesRetry(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Resolving).WithMockClock()
	txn, mocks := builder.BuildWithMocks()

	mocks.Clock.On("ScheduleTimer", mock.Anything, mock.Anything, mock.Anything).
		Return(func() {}).Once()

	require.NoError(t, txn.HandleEvent(ctx, &VerifierResolutionFailedEvent{
		BaseEvent: BaseEvent{TransactionID: txn.GetID()},
	}))

	assert.Equal(t, State_Resolving, txn.GetCurrentState())
	assert.NotNil(t, txn.cancelResolveRetry)
}

func TestOriginatorTransaction_Resolving_OnRetry_ReResolves(t *testing.T) {
	ctx := context.Background()
	resolved := []*prototk.ResolvedVerifier{{Lookup: "alice@node1", Verifier: "0xabc"}}
	builder := NewTransactionBuilderForTesting(t, State_Resolving).WithResolveVerifiersResult(resolved, nil)
	txn, mocks := builder.BuildWithMocks()
	cancelled := false
	txn.cancelResolveRetry = func() { cancelled = true }

	require.NoError(t, txn.HandleEvent(ctx, &VerifierResolutionRetryEvent{
		BaseEvent: BaseEvent{TransactionID: txn.GetID()},
	}))

	assert.Equal(t, State_Resolving, txn.GetCurrentState(), "a retry re-resolves but stays in Resolving until it succeeds")
	assert.True(t, cancelled, "the retry must release the fired timer's context before re-arming")
	assert.Nil(t, txn.cancelResolveRetry)
	event := <-mocks.Events
	_, ok := event.(*VerifiersResolvedEvent)
	assert.True(t, ok, "the retry must re-run resolution")
}

// An assemble request must be silently dropped in any state before the transaction has been delegated,
// so verifiers are guaranteed resolved by the time assembly runs. There is no handler for
// Event_AssembleRequestReceived in State_Initial, State_Resolving or State_Pending: the event is ignored
// and the transaction does not change state or invoke assembly.
func TestOriginatorTransaction_AssembleRequestIgnoredBeforeDelegation(t *testing.T) {
	ctx := context.Background()
	for _, state := range []State{State_Initial, State_Resolving, State_Pending} {
		t.Run(state.String(), func(t *testing.T) {
			builder := NewTransactionBuilderForTesting(t, state)
			txn, mocks := builder.BuildWithMocks()

			err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
				BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
				RequestID:   uuid.New(),
				Coordinator: builder.GetCoordinator(),
			})
			assert.NoError(t, err)
			assert.Equal(t, state, txn.GetCurrentState(), "assemble request must not change state")
			mocks.EngineIntegration.AssertNotCalled(t, "Assemble")
		})
	}
}

func TestOriginatorTransaction_Pending_ToDelegated_OnDelegated(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Pending)
	txn := builder.Build()

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Delegated_ToAssembling_OnAssembleRequestReceived_OK(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, mocks := builder.BuildWithMocks()

	mocks.MockForAssembleRequestOK().Once()
	requestID := uuid.New()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		RequestID:   requestID,
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	e0 := <-mocks.Events
	e1 := <-mocks.Events
	assert.True(t, mocks.EngineIntegration.AssertExpectations(t))
	require.IsType(t, &common.TransactionStateTransitionEvent[State]{}, e0)
	require.IsType(t, &AssembleSuccessEvent{}, e1)

	//We haven't fed that event back into the state machine yet, so the state should still be Assembling
	currentState := txn.GetCurrentState()
	assert.Equal(t, State_Assembling, currentState, "current state is %s", currentState.String())
}

func TestOriginatorTransaction_Delegated_ToAssembling_OnAssembleRequestReceived_REVERT(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, mocks := builder.BuildWithMocks()

	mocks.MockForAssembleRequestRevert().Once()
	requestID := uuid.New()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		RequestID:   requestID,
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	e0 := <-mocks.Events
	e1 := <-mocks.Events
	assert.True(t, mocks.EngineIntegration.AssertExpectations(t))
	require.IsType(t, &common.TransactionStateTransitionEvent[State]{}, e0)
	require.IsType(t, &AssembleRevertEvent{}, e1)

	//We haven't fed that event back into the state machine yet, so the state should still be Assembling
	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Delegated_ToAssembling_OnAssembleRequestReceived_PARK(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, mocks := builder.BuildWithMocks()

	mocks.MockForAssembleRequestPark().Once()
	requestID := uuid.New()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		RequestID:   requestID,
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	e0 := <-mocks.Events
	e1 := <-mocks.Events
	assert.True(t, mocks.EngineIntegration.AssertExpectations(t))
	require.IsType(t, &common.TransactionStateTransitionEvent[State]{}, e0)
	require.IsType(t, &AssembleParkEvent{}, e1)

	//We haven't fed that event back into the state machine yet, so the state should still be Assembling
	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Delegated_StaysInState_OnAssembleRequestReceived_BlockHeightToleranceExceeded(t *testing.T) {
	// When the coordinator's block height differs from the originator's by more than the configured
	// tolerance, the originator sends an AssembleRejection and stays in State_Delegated.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated).
		CurrentBlockHeight(50)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            builder.GetCoordinator(),
		CoordinatorBlockHeight: 100, // diff=50 > tolerance=0
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Delegated_StaysInState_OnAssembleRequestReceived_BlockHeightToleranceExceeded_OriginatorAhead(t *testing.T) {
	// Covers the branch in validator_AssembleBlockHeightToleranceExceeded where the originator's
	// block height is higher than the coordinator's (receiverBlockHeight > senderBH).
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated).
		CurrentBlockHeight(100)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            builder.GetCoordinator(),
		CoordinatorBlockHeight: 50, // originator(100) > coordinator(50), diff=50 > tolerance=0
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Delegated_StaysInState_OnAssembleRequestReceived_NotCurrentDelegate(t *testing.T) {
	// An assemble request from a node other than the current delegate is rejected immediately;
	// the originator sends an AssembleRejection and stays in State_Delegated.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		RequestID:   uuid.New(),
		Coordinator: uuid.New().String(), // different from builder.GetCoordinator()
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Delegated_StaysInState_OnAssembleRequestReceived_PrivateStateDataPending(t *testing.T) {
	// When private state is incomplete the originator sends an AssembleRejection and stays in State_Delegated.
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Delegated).
		CurrentBlockHeight(90). // within tolerance of coordinator's block height (diff ≤ tolerance, not >)
		WithCheckPendingPrivateStateData(false).
		BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            txn.currentDelegate,
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10, // lowWatermark = 90
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

// Test_Delegated_PrivateStateComplete_ProceedsToAssembly verifies that when the private state
// completeness check passes, the event falls through to the normal assembly handler and
// transitions to State_Assembling.
func Test_Delegated_PrivateStateComplete_ProceedsToAssembly(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Delegated).
		CurrentBlockHeight(90). // within tolerance of coordinator's block height (diff ≤ tolerance, not >)
		BuildWithMocks()

	// Builder default already returns true, nil; no override needed for the complete path.
	mocks.EngineIntegration.On(
		"Assemble", mock.Anything, txn.GetID(), mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return(&prototk.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}, nil).Maybe()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            txn.currentDelegate,
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10, // lowWatermark = 90
	})
	require.NoError(t, err)
	assert.False(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "must not send rejection when state is complete")
	assert.Equal(t, State_Assembling, txn.GetCurrentState())
}

func TestOriginatorTransaction_Assembling_ToDelegated_OnDelegated_IfDifferentCoordinator(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn := builder.Build()

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		Coordinator: uuid.New().String(), // different from current delegate
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Delegated, txn.GetCurrentState())
}

func TestOriginatorTransaction_Assembling_ToEndorsement_Gathering_OnAssembleSuccess(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleSuccessEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		RequestID: builder.GetAssembleRequestID(),
		PostAssembly: &components.TransactionPostAssembly{
			AssembleResponse: &prototk.TransactionPostAssembly{
				AssemblyResult: prototk.AssembleTransactionResponse_OK,
			},
		},
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleSuccessResponse(), "assemble success response was not sent back to coordinator")
	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Assembling_StaysInAssembling_OnAssembleSuccess_StaleRequestID(t *testing.T) {
	// When an AssembleSuccessEvent arrives with a request ID that does not match the
	// current outstanding assemble request, the validator must reject it so the state machine
	// stays in State_Assembling and no assemble response is sent to the coordinator.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	staleRequestID := uuid.New() // different from builder.GetAssembleRequestID()
	assert.NotEqual(t, builder.GetAssembleRequestID(), staleRequestID)

	err := txn.HandleEvent(ctx, &AssembleSuccessEvent{
		BaseEvent: BaseEvent{TransactionID: txn.GetID()},
		RequestID: staleRequestID,
		PostAssembly: &components.TransactionPostAssembly{
			AssembleResponse: &prototk.TransactionPostAssembly{
				AssemblyResult: prototk.AssembleTransactionResponse_OK,
			},
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "stale success event must not advance state; current state is %s", txn.GetCurrentState().String())
	assert.False(t, mocks.SentMessageRecorder.HasSentAssembleSuccessResponse(), "assemble success response must not be sent for a stale request ID")
}

func TestOriginatorTransaction_Assembling_ToReverted_OnAssembleRevert(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRevertEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		PostAssembly: &components.TransactionPostAssembly{
			AssembleResponse: &prototk.TransactionPostAssembly{
				AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
				RevertReason:   ptrTo("test revert reason"),
			},
		},
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRevertResponse(), "assemble revert response was not sent back to coordinator")
	assert.Equal(t, State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Assembling_ToParked_OnAssemblePark(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleParkEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		PostAssembly: &components.TransactionPostAssembly{
			AssembleResponse: &prototk.TransactionPostAssembly{
				AssemblyResult: prototk.AssembleTransactionResponse_PARK,
			},
		},
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleParkResponse(), "assemble park response was not sent back to coordinator")
	assert.Equal(t, State_Parked, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Assembling_ToDelegated_OnAssembleError(t *testing.T) {
	// action_AssembleError stores the error; action_SendAssembleError sends it back to the
	// coordinator; the transaction returns to State_Delegated for a future retry.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleErrorEvent{
		BaseEvent: BaseEvent{TransactionID: txn.GetID()},
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleError(), "assemble error response was not sent back to coordinator")
	assert.Equal(t, State_Delegated, txn.GetCurrentState())
}

func TestOriginatorTransaction_Assembling_StaysInState_OnAssembleRequestReceived_BlockHeightToleranceExceeded(t *testing.T) {
	// When the coordinator's block height differs from the originator's by more than the configured
	// tolerance, the originator sends an AssembleRejection and stays in State_Assembling.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling).
		CurrentBlockHeight(50)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            builder.GetCoordinator(),
		CoordinatorBlockHeight: 100, // diff=50 > tolerance=0
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Assembling_StaysInState_OnAssembleRequestReceived_NotCurrentDelegate(t *testing.T) {
	// An assemble request from a node other than the current delegate is rejected immediately;
	// the originator sends an AssembleRejection and stays in State_Assembling.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		RequestID:   uuid.New(),
		Coordinator: uuid.New().String(), // different from builder.GetCoordinator()
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Assembling_StaysInState_OnAssembleRequestReceived_PrivateStateDataPending(t *testing.T) {
	// When private state is incomplete the originator sends an AssembleRejection and stays in State_Assembling.
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Assembling).
		CurrentBlockHeight(90). // within tolerance of coordinator's block height (diff ≤ tolerance, not >)
		WithCheckPendingPrivateStateData(false).
		BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            txn.currentDelegate,
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10, // lowWatermark = 90
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Assembling_StaysInAssembling_OnAssembleRequestReceived_NewRequest_CancelsPreviousGoroutine(t *testing.T) {
	// When a new AssembleRequestReceived arrives while already in State_Assembling, the state
	// machine must call the previously stored cancel function (to abort the stale goroutine)
	// and store a new cancel function for the freshly spawned goroutine.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	// Install a cancel function that signals us when it is called
	previousCancelCalled := make(chan struct{})
	txn.cancelCurrentAssembly = func() { close(previousCancelCalled) }

	// The new goroutine spawned by action_Assemble will call Assemble.
	// Allow any number of calls (goroutine is async) and return quickly.
	mocks.EngineIntegration.On(
		"Assemble",
		mock.Anything, txn.GetID(), mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything,
	).Maybe().Return(nil, context.Canceled)

	newRequestID := uuid.New()
	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              newRequestID,
		Coordinator:            txn.currentDelegate,
		CoordinatorBlockHeight: 0,
		BlockHeightTolerance:   0,
	})
	assert.NoError(t, err)

	// action_Assemble is synchronous within HandleEvent, so the old cancel was called
	// before HandleEvent returned — this receive completes immediately.
	<-previousCancelCalled

	// A new cancel for the fresh goroutine must be stored
	assert.NotNil(t, txn.cancelCurrentAssembly)
	// No state transition — we remain assembling (just with a new request)
	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Delegated_ToReverted_OnAssembleRequestReceived_AfterAssembleCompletesRevert(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, mocks := builder.BuildWithMocks()

	mocks.MockForAssembleRequestRevert().Once()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	e0 := <-mocks.Events
	e1 := <-mocks.Events
	assert.True(t, mocks.EngineIntegration.AssertExpectations(t))
	require.IsType(t, &common.TransactionStateTransitionEvent[State]{}, e0)
	require.IsType(t, &AssembleRevertEvent{}, e1)
	err = txn.HandleEvent(ctx, e1)
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRevertResponse(), "assemble revert response was not sent back to coordinator")
	assert.Equal(t, State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	//TODO assert that transaction was finalized as Reverted in the database
}

func TestOriginatorTransaction_Delegated_ToParked_OnAssembleRequestReceived_AfterAssembleCompletesPark(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn, mocks := builder.BuildWithMocks()

	mocks.MockForAssembleRequestPark().Once()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	e0 := <-mocks.Events
	e1 := <-mocks.Events
	assert.True(t, mocks.EngineIntegration.AssertExpectations(t))
	require.IsType(t, &common.TransactionStateTransitionEvent[State]{}, e0)
	require.IsType(t, &AssembleParkEvent{}, e1)
	err = txn.HandleEvent(ctx, e1)
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleParkResponse(), "assemble park response was not sent back to coordinator")
	assert.Equal(t, State_Parked, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	//TODO assert that transaction was finalized as Parked in the database
}

func TestOriginatorTransaction_Delegated_ToDispatched_OnDispatched_IfCurrentDelegate(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Delegated)
	txn := builder.Build()

	err := txn.HandleEvent(ctx, &DispatchedEvent{
		BaseEvent:     BaseEvent{TransactionID: txn.GetID()},
		SignerAddress: pldtypes.EthAddress(pldtypes.RandBytes(20)),
		Coordinator:   builder.GetCoordinator(), // must match currentDelegate for validator to pass
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Dispatched, txn.GetCurrentState())
}

func TestOriginatorTransaction_EndorsementGathering_ToDelegated_OnDelegated_IfDifferentCoordinator(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn := builder.Build()

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		Coordinator: uuid.New().String(),
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Delegated, txn.GetCurrentState())
}

func TestOriginatorTransaction_Endorsement_Gathering_NoTransition_OnAssembleRequest_IfMatchesPreviousRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, mocks := builder.BuildWithMocks()

	// NOTE we do not mock Assemble function because we expect to resend the previous response

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		RequestID:   builder.GetLatestFulfilledAssembleRequestID(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleSuccessResponse(), "assemble success response was not sent back to coordinator")
	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Endorsement_Gathering_RejectsOnly_OnAssembleRequest_IfMatchesPreviousRequestButStaleDelegate(t *testing.T) {
	// The resend handler requires the request to be from the current delegate, so a matches-previous request
	// from a stale delegate is not resent to - it falls through to the not-current-delegate rejection.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		RequestID:   builder.GetLatestFulfilledAssembleRequestID(),
		Coordinator: uuid.New().String(), // different from builder.GetCoordinator()
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent for a matches-previous request from a stale delegate")
	assert.False(t, mocks.SentMessageRecorder.HasSentAssembleSuccessResponse(), "assemble success response was unexpectedly resent to a stale delegate")
	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Reverted_ToFinal_OnFinalize(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Reverted).Build()

	err := txn.HandleEvent(ctx, &FinalizeEvent{TransactionID: txn.GetID()})
	assert.NoError(t, err)
	assert.Equal(t, State_Final, txn.GetCurrentState())
}

func TestOriginatorTransaction_Reverted_DoResendAssembleResponse_OnAssembleRequest_IfMatchesPreviousRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Reverted)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		RequestID:   builder.GetLatestFulfilledAssembleRequestID(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRevertResponse(), "assemble revert response was not sent back to coordinator")
	assert.Equal(t, State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Reverted_Ignore_OnAssembleRequest_IfNotMatchesPreviousRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Reverted)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		RequestID:   uuid.New(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.False(t, mocks.SentMessageRecorder.HasSentAssembleRevertResponse(), "assemble revert response was unexpectedly sent to coordinator")
	assert.Equal(t, State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Reverted_StaysInState_OnAssembleRequestReceived_BlockHeightToleranceExceeded(t *testing.T) {
	// When the coordinator's block height differs from the originator's by more than the configured
	// tolerance, the originator sends an AssembleRejection and stays in State_Reverted.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Reverted).
		CurrentBlockHeight(50)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            builder.GetCoordinator(),
		CoordinatorBlockHeight: 100, // diff=50 > tolerance=0
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Reverted_StaysInState_OnAssembleRequestReceived_NotCurrentDelegate(t *testing.T) {
	// An assemble request from a node other than the current delegate is rejected immediately;
	// the originator sends an AssembleRejection and stays in State_Reverted.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Reverted)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		RequestID:   uuid.New(),
		Coordinator: uuid.New().String(), // different from builder.GetCoordinator()
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Reverted_StaysInState_OnAssembleRequestReceived_PrivateStateDataPending(t *testing.T) {
	// When private state is incomplete the originator sends an AssembleRejection and stays in State_Reverted.
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Reverted).
		CurrentBlockHeight(90). // within tolerance of coordinator's block height (diff ≤ tolerance, not >)
		WithCheckPendingPrivateStateData(false).
		BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            txn.currentDelegate,
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10, // lowWatermark = 90
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Parked_ToDelegated_OnDelegated_IfDifferentCoordinator(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Parked)
	txn := builder.Build()

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		Coordinator: uuid.New().String(),
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Delegated, txn.GetCurrentState())
}

func TestOriginatorTransaction_Parked_DoResendAssembleResponse_OnAssembleRequest_IfMatchesPreviousRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Parked)
	txn, mocks := builder.BuildWithMocks()

	// NOTE we do not mock Assemble function because we expect to resend the previous response

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		RequestID:   builder.GetLatestFulfilledAssembleRequestID(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleParkResponse(), "assemble park response was not sent back to coordinator")
	assert.Equal(t, State_Parked, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Parked_Ignore_OnAssembleRequest_IfNotMatchesPreviousRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Parked)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		RequestID:   uuid.New(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.False(t, mocks.SentMessageRecorder.HasSentAssembleParkResponse(), "assemble park response was unexpectedly sent to coordinator")
	assert.Equal(t, State_Parked, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Parked_StaysInState_OnAssembleRequestReceived_BlockHeightToleranceExceeded(t *testing.T) {
	// When the coordinator's block height differs from the originator's by more than the configured
	// tolerance, the originator sends an AssembleRejection and stays in State_Parked.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Parked).
		CurrentBlockHeight(50)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            builder.GetCoordinator(),
		CoordinatorBlockHeight: 100, // diff=50 > tolerance=0
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Parked, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Parked_StaysInState_OnAssembleRequestReceived_NotCurrentDelegate(t *testing.T) {
	// An assemble request from a node other than the current delegate is rejected immediately;
	// the originator sends an AssembleRejection and stays in State_Parked.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Parked)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		RequestID:   uuid.New(),
		Coordinator: uuid.New().String(), // different from builder.GetCoordinator()
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Parked, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Parked_StaysInState_OnAssembleRequestReceived_PrivateStateDataPending(t *testing.T) {
	// When private state is incomplete the originator sends an AssembleRejection and stays in State_Parked.
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Parked).
		CurrentBlockHeight(90). // within tolerance of coordinator's block height (diff ≤ tolerance, not >)
		WithCheckPendingPrivateStateData(false).
		BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            txn.currentDelegate,
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10, // lowWatermark = 90
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Parked, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Endorsement_Gathering_ToAssembling_OnAssembleRequest_IfNotMatchesPreviousRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, mocks := builder.BuildWithMocks()
	// This should trigger a re-assembly
	mocks.MockForAssembleRequestOK().Once()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		RequestID:   uuid.New(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	e0 := <-mocks.Events
	e1 := <-mocks.Events
	assert.True(t, mocks.EngineIntegration.AssertExpectations(t))
	require.IsType(t, &common.TransactionStateTransitionEvent[State]{}, e0)
	require.IsType(t, &AssembleSuccessEvent{}, e1)

	//We haven't fed that event back into the state machine yet, so the state should still be Assembling
	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Endorsement_Gathering_StaysInState_OnAssembleRequestReceived_BlockHeightToleranceExceeded(t *testing.T) {
	// When the coordinator's block height differs from the originator's by more than the configured
	// tolerance, the originator sends an AssembleRejection and stays in State_Endorsement_Gathering.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		CurrentBlockHeight(50)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            builder.GetCoordinator(),
		CoordinatorBlockHeight: 100, // diff=50 > tolerance=0
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Endorsement_Gathering_StaysInState_OnAssembleRequestReceived_NotCurrentDelegate(t *testing.T) {
	// An assemble request from a node other than the current delegate is rejected immediately;
	// the originator sends an AssembleRejection and stays in State_Endorsement_Gathering.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		RequestID:   uuid.New(),
		Coordinator: uuid.New().String(), // different from builder.GetCoordinator()
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Endorsement_Gathering_StaysInState_OnAssembleRequestReceived_PrivateStateDataPending(t *testing.T) {
	// When private state is incomplete the originator sends an AssembleRejection and stays in State_Endorsement_Gathering.
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		CurrentBlockHeight(90). // within tolerance of coordinator's block height (diff ≤ tolerance, not >)
		WithCheckPendingPrivateStateData(false).
		BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            txn.currentDelegate,
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10, // lowWatermark = 90
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Endorsement_Gathering_ToPrepared_OnDispatchConfirmationRequestReceivedIfMatches(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &PreDispatchRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentPreDispatchResponse(), "dispatch confirmation response was not sent back to coordinator")
	assert.Equal(t, State_Prepared, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Endorsement_Gathering_NoTransition_OnDispatchConfirmationRequestReceivedIfNotMatches_WrongCoordinator(t *testing.T) {
	ctx := context.Background()

	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &PreDispatchRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: uuid.New().String(),
	})
	assert.NoError(t, err)

	assert.False(t, mocks.SentMessageRecorder.HasSentPreDispatchResponse(), "dispatch confirmation response was unexpectedly sent back to coordinator")
	assert.Equal(t, State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Prepared_ToDelegated_OnDelegated_IfDifferentCoordinator(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Prepared)
	txn := builder.Build()

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		Coordinator: uuid.New().String(),
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Delegated, txn.GetCurrentState())
}

func TestOriginatorTransaction_Prepared_NoTransition_OnAssembleRequest_IfMatchesPreviousRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Prepared)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		RequestID:   builder.GetLatestFulfilledAssembleRequestID(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleSuccessResponse(), "assemble success response was not sent back to coordinator")
	assert.Equal(t, State_Prepared, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Prepared_ToAssembling_OnAssembleRequest_IfNotMatchesPreviousRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Prepared)
	txn, mocks := builder.BuildWithMocks()
	mocks.MockForAssembleRequestOK().Once()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		RequestID:   uuid.New(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	e0 := <-mocks.Events
	e1 := <-mocks.Events
	assert.True(t, mocks.EngineIntegration.AssertExpectations(t))
	require.IsType(t, &common.TransactionStateTransitionEvent[State]{}, e0)
	require.IsType(t, &AssembleSuccessEvent{}, e1)
	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Prepared_StaysInState_OnAssembleRequestReceived_BlockHeightToleranceExceeded(t *testing.T) {
	// When the coordinator's block height differs from the originator's by more than the configured
	// tolerance, the originator sends an AssembleRejection and stays in State_Prepared.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Prepared).
		CurrentBlockHeight(50)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            builder.GetCoordinator(),
		CoordinatorBlockHeight: 100, // diff=50 > tolerance=0
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Prepared, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Prepared_StaysInState_OnAssembleRequestReceived_NotCurrentDelegate(t *testing.T) {
	// An assemble request from a node other than the current delegate is rejected immediately;
	// the originator sends an AssembleRejection and stays in State_Prepared.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Prepared)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		RequestID:   uuid.New(),
		Coordinator: uuid.New().String(), // different from builder.GetCoordinator()
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Prepared, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Prepared_StaysInState_OnAssembleRequestReceived_PrivateStateDataPending(t *testing.T) {
	// When private state is incomplete the originator sends an AssembleRejection and stays in State_Prepared.
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Prepared).
		CurrentBlockHeight(90). // within tolerance of coordinator's block height (diff ≤ tolerance, not >)
		WithCheckPendingPrivateStateData(false).
		BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            txn.currentDelegate,
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10, // lowWatermark = 90
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Prepared, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Prepared_ToDispatched_OnDispatched(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Prepared)
	txn := builder.Build()

	signerAddress := pldtypes.EthAddress(pldtypes.RandBytes(20))

	err := txn.HandleEvent(ctx, &DispatchedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		SignerAddress: signerAddress,
		Coordinator:   builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Dispatched, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Prepared_NoTransition_Do_Resend_OnDispatchConfirmationRequestReceivedIfMatches(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Prepared)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &PreDispatchRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentPreDispatchResponse(), "dispatch confirmation response was not sent back to coordinator")
	assert.Equal(t, State_Prepared, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Prepared_Ignore_OnDispatchConfirmationRequestReceivedIfNotMatches_WrongCoordinator(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Prepared)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &PreDispatchRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		Coordinator: uuid.New().String(),
	})
	assert.NoError(t, err)

	assert.False(t, mocks.SentMessageRecorder.HasSentPreDispatchResponse(), "dispatch confirmation response was unexpectedly sent back to coordinator")
	assert.Equal(t, State_Prepared, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Dispatched_ToConfirmed_OnConfirmedSuccess(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	err := txn.HandleEvent(ctx, &ConfirmedSuccessEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Dispatched_ToAssembling_OnAssembleRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Dispatched)
	txn, mocks := builder.BuildWithMocks()
	mocks.MockForAssembleRequestOK().Once()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		RequestID:   uuid.New(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	e0 := <-mocks.Events
	e1 := <-mocks.Events
	assert.True(t, mocks.EngineIntegration.AssertExpectations(t))
	require.IsType(t, &common.TransactionStateTransitionEvent[State]{}, e0)
	require.IsType(t, &AssembleSuccessEvent{}, e1)
	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Dispatched_StaysInState_OnAssembleRequestReceived_BlockHeightToleranceExceeded(t *testing.T) {
	// When the coordinator's block height differs from the originator's by more than the configured
	// tolerance, the originator sends an AssembleRejection and stays in State_Dispatched.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Dispatched).
		CurrentBlockHeight(50)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            builder.GetCoordinator(),
		CoordinatorBlockHeight: 100, // diff=50 > tolerance=0
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Dispatched, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Dispatched_StaysInState_OnAssembleRequestReceived_NotCurrentDelegate(t *testing.T) {
	// An assemble request from a node other than the current delegate is rejected immediately;
	// the originator sends an AssembleRejection and stays in State_Dispatched.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Dispatched)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		RequestID:   uuid.New(),
		Coordinator: uuid.New().String(), // different from builder.GetCoordinator()
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Dispatched, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Dispatched_StaysInState_OnAssembleRequestReceived_PrivateStateDataPending(t *testing.T) {
	// When private state is incomplete the originator sends an AssembleRejection and stays in State_Dispatched.
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Dispatched).
		CurrentBlockHeight(90). // within tolerance of coordinator's block height (diff ≤ tolerance, not >)
		WithCheckPendingPrivateStateData(false).
		BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            txn.currentDelegate,
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10, // lowWatermark = 90
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Dispatched, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Dispatched_ToSequenced_OnNonceAssigned(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Dispatched)
	txn := builder.Build()

	err := txn.HandleEvent(ctx, &NonceAssignedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		SignerAddress: pldtypes.EthAddress(pldtypes.RandBytes(20)),
		Nonce:         42,
		Coordinator:   builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Sequenced, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Dispatched_ToSubmitted_OnSubmitted(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Dispatched)
	txn := builder.Build()

	err := txn.HandleEvent(ctx, &SubmittedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		SignerAddress:        pldtypes.EthAddress(pldtypes.RandBytes(20)),
		Nonce:                42,
		LatestSubmissionHash: pldtypes.Bytes32(pldtypes.RandBytes(32)),
		Coordinator:          builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Submitted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Dispatched_ToDelegated_OnConfirmedReverted(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	err := txn.HandleEvent(ctx, &ConfirmedRevertedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		WillRetry: true,
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Dispatched_ToDelegated_OnDelegated_IfDifferentCoordinator(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Dispatched)
	txn := builder.Build()

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		Coordinator: uuid.New().String(),
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Delegated, txn.GetCurrentState())
}

func TestOriginatorTransaction_Sequenced_ToConfirmed_OnConfirmedSuccess(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Sequenced).Build()

	err := txn.HandleEvent(ctx, &ConfirmedSuccessEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Sequenced_ToAssembling_OnAssembleRequest(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Sequenced)
	txn, mocks := builder.BuildWithMocks()
	mocks.MockForAssembleRequestOK().Once()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		RequestID:   uuid.New(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	e0 := <-mocks.Events
	e1 := <-mocks.Events
	assert.True(t, mocks.EngineIntegration.AssertExpectations(t))
	require.IsType(t, &common.TransactionStateTransitionEvent[State]{}, e0)
	require.IsType(t, &AssembleSuccessEvent{}, e1)
	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Sequenced_StaysInState_OnAssembleRequestReceived_BlockHeightToleranceExceeded(t *testing.T) {
	// When the coordinator's block height differs from the originator's by more than the configured
	// tolerance, the originator sends an AssembleRejection and stays in State_Sequenced.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Sequenced).
		CurrentBlockHeight(50)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            builder.GetCoordinator(),
		CoordinatorBlockHeight: 100, // diff=50 > tolerance=0
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Sequenced, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Sequenced_StaysInState_OnAssembleRequestReceived_NotCurrentDelegate(t *testing.T) {
	// An assemble request from a node other than the current delegate is rejected immediately;
	// the originator sends an AssembleRejection and stays in State_Sequenced.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Sequenced)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		RequestID:   uuid.New(),
		Coordinator: uuid.New().String(), // different from builder.GetCoordinator()
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Sequenced, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Sequenced_StaysInState_OnAssembleRequestReceived_PrivateStateDataPending(t *testing.T) {
	// When private state is incomplete the originator sends an AssembleRejection and stays in State_Sequenced.
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Sequenced).
		CurrentBlockHeight(90). // within tolerance of coordinator's block height (diff ≤ tolerance, not >)
		WithCheckPendingPrivateStateData(false).
		BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            txn.currentDelegate,
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10, // lowWatermark = 90
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Sequenced, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Sequenced_ToSubmitted_OnSubmitted(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Sequenced)
	txn := builder.Build()

	err := txn.HandleEvent(ctx, &SubmittedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		SignerAddress:        pldtypes.EthAddress(pldtypes.RandBytes(20)),
		Nonce:                42,
		LatestSubmissionHash: pldtypes.Bytes32(pldtypes.RandBytes(32)),
		Coordinator:          builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Submitted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Sequenced_ToDelegated_OnConfirmedReverted(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Sequenced).Build()

	err := txn.HandleEvent(ctx, &ConfirmedRevertedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		WillRetry: true,
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Sequenced_ToDelegated_OnDelegated_IfDifferentCoordinator(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Sequenced)
	txn := builder.Build()

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		Coordinator: uuid.New().String(),
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Delegated, txn.GetCurrentState())
}

func TestOriginatorTransaction_Submitted_StaysSubmitted_OnSubmitted_IfCurrentDelegate(t *testing.T) {
	// Re-submission in State_Submitted updates the hash but does not trigger a state change.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Submitted)
	txn := builder.Build()

	err := txn.HandleEvent(ctx, &SubmittedEvent{
		BaseEvent:            BaseEvent{TransactionID: txn.GetID()},
		SignerAddress:        pldtypes.EthAddress(pldtypes.RandBytes(20)),
		Nonce:                99,
		LatestSubmissionHash: pldtypes.Bytes32(pldtypes.RandBytes(32)),
		Coordinator:          builder.GetCoordinator(),
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Submitted, txn.GetCurrentState())
}

func TestOriginatorTransaction_Submitted_ToConfirmed_OnConfirmedSuccess(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Submitted).Build()

	err := txn.HandleEvent(ctx, &ConfirmedSuccessEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Submitted_ToDelegated_OnConfirmedReverted(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Submitted).Build()

	err := txn.HandleEvent(ctx, &ConfirmedRevertedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
		WillRetry: true,
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Submitted_ToDelegated_OnDelegated_IfDifferentCoordinator(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Submitted)
	txn := builder.Build()

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		Coordinator: uuid.New().String(),
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Delegated, txn.GetCurrentState())
}

func TestOriginatorTransaction_Submitted_ToAssembling_OnAssembleRequest(t *testing.T) {
	// After submission there's a race where the coordinator re-assembles; the originator
	// transitions to State_Assembling to process the new request.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Submitted)
	txn, mocks := builder.BuildWithMocks()
	mocks.MockForAssembleRequestOK().Once()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		RequestID:   uuid.New(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	e0 := <-mocks.Events
	e1 := <-mocks.Events
	assert.True(t, mocks.EngineIntegration.AssertExpectations(t))
	require.IsType(t, &common.TransactionStateTransitionEvent[State]{}, e0)
	require.IsType(t, &AssembleSuccessEvent{}, e1)
	assert.Equal(t, State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Submitted_StaysInState_OnAssembleRequestReceived_BlockHeightToleranceExceeded(t *testing.T) {
	// When the coordinator's block height differs from the originator's by more than the configured
	// tolerance, the originator sends an AssembleRejection and stays in State_Submitted.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Submitted).
		CurrentBlockHeight(50)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            builder.GetCoordinator(),
		CoordinatorBlockHeight: 100, // diff=50 > tolerance=0
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Submitted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Submitted_StaysInState_OnAssembleRequestReceived_NotCurrentDelegate(t *testing.T) {
	// An assemble request from a node other than the current delegate is rejected immediately;
	// the originator sends an AssembleRejection and stays in State_Submitted.
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Submitted)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.GetID()},
		RequestID:   uuid.New(),
		Coordinator: uuid.New().String(), // different from builder.GetCoordinator()
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Submitted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Submitted_StaysInState_OnAssembleRequestReceived_PrivateStateDataPending(t *testing.T) {
	// When private state is incomplete the originator sends an AssembleRejection and stays in State_Submitted.
	ctx := context.Background()
	txn, mocks := NewTransactionBuilderForTesting(t, State_Submitted).
		CurrentBlockHeight(90). // within tolerance of coordinator's block height (diff ≤ tolerance, not >)
		WithCheckPendingPrivateStateData(false).
		BuildWithMocks()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		BaseEvent:              BaseEvent{TransactionID: txn.GetID()},
		RequestID:              uuid.New(),
		Coordinator:            txn.currentDelegate,
		CoordinatorBlockHeight: 100,
		BlockHeightTolerance:   10, // lowWatermark = 90
	})
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRejection(), "assemble rejection was not sent to coordinator")
	assert.Equal(t, State_Submitted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Parked_ToPending_OnResumed(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Parked).Build()

	err := txn.HandleEvent(ctx, &ResumedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txn.GetID(),
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, State_Pending, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Confirmed_ToFinal_OnFinalize(t *testing.T) {
	// State_Confirmed.OnTransitionTo queues a FinalizeEvent automatically when entered via a
	// real transition. Here we verify the Event_Finalize handler by firing it manually.
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Confirmed).Build()

	err := txn.HandleEvent(ctx, &FinalizeEvent{TransactionID: txn.GetID()})
	assert.NoError(t, err)
	assert.Equal(t, State_Final, txn.GetCurrentState())
}
