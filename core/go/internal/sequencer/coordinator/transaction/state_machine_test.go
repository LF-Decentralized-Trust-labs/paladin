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
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/stretchr/testify/assert"
)

func TestStateMachine_InitializeOK(t *testing.T) {
	ctx := context.Background()

	messageSender := NewMockMessageSender(t)
	clock := &common.FakeClockForTesting{}
	txn := NewTransaction(
		uuid.NewString(),
		&components.PrivateTransaction{
			ID: uuid.New(),
		},
		messageSender,
		clock,
		clock.Duration(1000),
		clock.Duration(5000),
		NewStateIndex(ctx),
	)

	assert.Equal(t, State_Pooled, txn.stateMachine.currentState, "current state is %s", txn.stateMachine.currentState.String())
}

func TestStateMachine_Pooled_ToAssembling_OnSelected(t *testing.T) {
	ctx := context.Background()

	txn, mocks := NewTransactionBuilderForTesting(t, State_Pooled).BuildWithMocks()

	txn.HandleEvent(ctx, &SelectedEvent{
		event: event{
			TransactionID: txn.ID,
		},
	})

	assert.Equal(t, State_Assembling, txn.stateMachine.currentState, "current state is %s", txn.stateMachine.currentState.String())
	assert.Equal(t, true, mocks.sentMessageRecorder.hasSentAssembleRequest, "expected an assemble request to be sent, but none were sent")
}

func TestStateMachine_Assembling_ToEndorsing_OnAssembleResponse(t *testing.T) {
	ctx := context.Background()
	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := txnBuilder.BuildWithMocks()

	txn.HandleEvent(ctx, &AssembleSuccessEvent{
		event: event{
			TransactionID: txn.ID,
		},
		postAssembly: txnBuilder.BuildPostAssembly(),
	})

	assert.Equal(t, State_Endorsement_Gathering, txn.stateMachine.currentState, "current state is %s", txn.stateMachine.currentState.String())
	assert.Equal(t, 3, mocks.sentMessageRecorder.numberOfSentEndorsementRequests, "expected 3 endorsement requests to be sent, but %d were sent", mocks.sentMessageRecorder.numberOfSentEndorsementRequests)

}

func TestStateMachine_Assembling_ToPooled_OnHeartbeat_IfAssembleTimeoutExpired(t *testing.T) {
	ctx := context.Background()
	txnBuilder := NewTransactionBuilderForTesting(t, State_Assembling)
	txn, mocks := txnBuilder.BuildWithMocks()

	mocks.clock.Advance(txnBuilder.assembleTimeout + 1)

	txn.HandleEvent(ctx, &HeartbeatIntervalEvent{})

	assert.Equal(t, State_Pooled, txn.stateMachine.currentState, "current state is %s", txn.stateMachine.currentState.String())
}

func TestStateMachine_EndorsementGathering_ToConfirmingDispatch_OnEndorsed_IfAttestationPlanComplete(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)

	txn, mocks := builder.BuildWithMocks()
	txn.HandleEvent(ctx, builder.BuildEndorsedEvent(2))

	assert.Equal(t, State_Confirming_Dispatch, txn.stateMachine.currentState, "current state is %s", txn.stateMachine.currentState.String())
	assert.True(t, mocks.sentMessageRecorder.hasSentDispatchConfirmationRequest, "expected a dispatch confirmation request to be sent, but none were sent")

}

func TestStateMachine_EndorsementGatheringNoTransition_IfNotAttestationPlanComplete(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(1) //only 1 existing endorsement so the next one does not complete the attestation plan

	txn, mocks := builder.BuildWithMocks()

	txn.HandleEvent(ctx, builder.BuildEndorsedEvent(1))

	assert.Equal(t, State_Endorsement_Gathering, txn.stateMachine.currentState, "current state is %s", txn.stateMachine.currentState.String())
	assert.False(t, mocks.sentMessageRecorder.hasSentDispatchConfirmationRequest, "did not expected a dispatch confirmation request to be sent, but one was sent")

}

func TestStateMachine_EndorsementGathering_ToBlocked_OnEndorsed_IfAttestationPlanCompleteAndHasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()

	//we need 2 transactions to know about each other so they need to share a state index
	stateIndex := NewStateIndex(ctx)

	builder1 := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		StateIndex(stateIndex).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	txn1 := builder1.Build()

	builder2 := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		StateIndex(stateIndex).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		InputStateIDs(txn1.PostAssembly.OutputStates[0].ID)
	txn2 := builder2.Build()

	txn2.HandleEvent(ctx, builder2.BuildEndorsedEvent(2))

	assert.Equal(t, State_Blocked, txn2.stateMachine.currentState, "current state is %s", txn2.stateMachine.currentState.String())

}

func TestStateMachine_ConfirmingDispatch_ToReadyForDispatch_OnDispatchConfirmed(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Confirming_Dispatch).Build()

	txn.HandleEvent(ctx, &DispatchConfirmedEvent{
		event: event{
			TransactionID: txn.ID,
		},
	})

	assert.Equal(t, State_Ready_For_Dispatch, txn.stateMachine.currentState, "current state is %s", txn.stateMachine.currentState.String())

}

func TestStateMachine_Blocked_ToConfirmingDispatch_OnDependencyReady_IfNotHasDependenciesNotReady(t *testing.T) {
	//TODO rethink naming of this test and/or the guard function because we end up with a double negative
	ctx := context.Background()

	//A transaction (A) is dependant on another 2 transactions (B and C).  One of which (B) is ready for dispatch and the other (C) becomes ready for dispatch,
	// triggering a transition for A to move from blocked to confirming dispatch

	//we need 3 transactions to know about each other so they need to share a state index
	stateIndex := NewStateIndex(ctx)

	builderB := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).
		StateIndex(stateIndex)
	txnB := builderB.Build()

	builderC := NewTransactionBuilderForTesting(t, State_Confirming_Dispatch).
		StateIndex(stateIndex)
	txnC := builderC.Build()

	builderA := NewTransactionBuilderForTesting(t, State_Blocked).
		StateIndex(stateIndex).
		InputStateIDs(
			txnB.PostAssembly.OutputStates[0].ID,
			txnC.PostAssembly.OutputStates[0].ID,
		)
	txnA := builderA.Build()

	//Was in 2 minds whether to a) trigger transaction A indirectly by causing C to become ready via a dispatch confirmation event or b) trigger it directly by sending a dependency ready event
	// decided on (a) as it is slightly more white box and less brittle to future refactoring of the implementation

	txnC.HandleEvent(ctx, &DispatchConfirmedEvent{
		event: event{
			TransactionID: txnC.ID,
		},
	})

	assert.Equal(t, State_Confirming_Dispatch, txnA.stateMachine.currentState, "current state is %s", txnA.stateMachine.currentState.String())

}

func TestStateMachine_BlockedNoTransition_OnDependencyReady_IfHasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()

	//A transaction (A) is dependant on another 2 transactions (B and C).  Neither of which a ready for dispatch. One of them (B) becomes ready for dispatch, but the other is still not ready
	// thus gating the triggering of a transition for A to move from blocked to confirming dispatch

	//we need 3 transactions to know about each other so they need to share a state index
	stateIndex := NewStateIndex(ctx)

	builderB := NewTransactionBuilderForTesting(t, State_Confirming_Dispatch).
		StateIndex(stateIndex)
	txnB := builderB.Build()

	builderC := NewTransactionBuilderForTesting(t, State_Confirming_Dispatch).
		StateIndex(stateIndex)
	txnC := builderC.Build()

	builderA := NewTransactionBuilderForTesting(t, State_Blocked).
		StateIndex(stateIndex).
		InputStateIDs(
			txnB.PostAssembly.OutputStates[0].ID,
			txnC.PostAssembly.OutputStates[0].ID,
		)
	txnA := builderA.Build()

	//Was in 2 minds whether to a) trigger transaction A indirectly by causing B to become ready via a dispatch confirmation event or b) trigger it directly by sending a dependency ready event
	// decided on (a) as it is slightly more white box and less brittle to future refactoring of the implementation

	txnB.HandleEvent(ctx, &DispatchConfirmedEvent{
		event: event{
			TransactionID: txnB.ID,
		},
	})

	assert.Equal(t, State_Blocked, txnA.stateMachine.currentState, "current state is %s", txnA.stateMachine.currentState.String())

}

func TestStateMachine_ReadyForDispatch_ToDispatched_OnCollected(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Ready_For_Dispatch).Build()

	txn.HandleEvent(ctx, &CollectedEvent{
		event: event{
			TransactionID: txn.ID,
		},
	})

	assert.Equal(t, State_Dispatched, txn.stateMachine.currentState, "current state is %s", txn.stateMachine.currentState.String())

}

func TestStateMachine_Dispatched_ToSubmitted_OnSubmitted(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Dispatched).Build()

	txn.HandleEvent(ctx, &SubmittedEvent{
		event: event{
			TransactionID: txn.ID,
		},
	})

	assert.Equal(t, State_Submitted, txn.stateMachine.currentState, "current state is %s", txn.stateMachine.currentState.String())
}

func TestStateMachine_Submitted_ToConfirmed_OnConfirmed(t *testing.T) {
	ctx := context.Background()
	txn := NewTransactionBuilderForTesting(t, State_Submitted).Build()

	txn.HandleEvent(ctx, &ConfirmedEvent{
		event: event{
			TransactionID: txn.ID,
		},
	})

	assert.Equal(t, State_Confirmed, txn.stateMachine.currentState, "current state is %s", txn.stateMachine.currentState.String())
}
