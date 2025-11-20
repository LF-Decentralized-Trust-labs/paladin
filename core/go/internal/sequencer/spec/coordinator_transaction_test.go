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

package spec

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCoordinatorTransaction_Initial_ToPooled_OnReceived_IfNoInflightDependencies(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).Build()

	err := txn.HandleEvent(ctx, &transaction.ReceivedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Initial_ToPreAssemblyBlocked_OnReceived_IfDependencyNotAssembled(t *testing.T) {

	ctx := context.Background()

	//we need 2 transactions to know about each other so they need to share a state index
	grapher := transaction.NewGrapher(ctx)

	//transaction2 depends on transaction 1 and transaction 1 gets reverted
	builder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).
		Grapher(grapher)

	txn1 := builder1.Build()

	builder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Initial).
		Grapher(grapher).
		Originator(builder1.GetOriginator()).
		PredefinedDependencies(txn1.ID)
	txn2 := builder2.Build()

	err := txn2.HandleEvent(ctx, &transaction.ReceivedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn2.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_PreAssembly_Blocked, txn2.GetCurrentState(), "current state is %s", txn2.GetCurrentState().String())

}

func TestCoordinatorTransaction_Initial_ToPreAssemblyBlocked_OnReceived_IfDependencyUnknown(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Initial).
		PredefinedDependencies(uuid.New())
	txn := builder.Build()

	err := txn.HandleEvent(ctx, &transaction.ReceivedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_PreAssembly_Blocked, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestCoordinatorTransaction_Pooled_ToAssembling_OnSelected(t *testing.T) {
	ctx := context.Background()

	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).BuildWithMocks()

	err := txn.HandleEvent(ctx, &transaction.SelectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, true, mocks.SentMessageRecorder.HasSentAssembleRequest())
}

func TestCoordinatorTransaction_Assembling_ToEndorsing_OnAssembleResponse(t *testing.T) {
	ctx := context.Background()
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn, mocks := txnBuilder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &transaction.AssembleSuccessEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		PreAssembly:  txnBuilder.BuildPreAssembly(),
		RequestID:    mocks.SentMessageRecorder.SentAssembleRequestIdempotencyKey(),
	})
	assert.NoError(t, err)
	assert.Equal(t, transaction.State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, 3, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests())
	//TODO some assertions that WriteLockAndDistributeStatesForTransaction was called with the expected states

}

func TestCoordinatorTransaction_Assembling_NoTransition_OnAssembleResponse_IfResponseDoesNotMatchPendingRequest(t *testing.T) {
	ctx := context.Background()
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn := txnBuilder.Build()

	err := txn.HandleEvent(ctx, &transaction.AssembleSuccessEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		RequestID:    uuid.New(), //generate a new random request ID so that it won't match the pending request
	})
	assert.NoError(t, err)
	assert.Equal(t, transaction.State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_NoTransition_OnRequestTimeout_IfNotAssembleTimeoutExpired(t *testing.T) {
	ctx := context.Background()
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn, mocks := txnBuilder.BuildWithMocks()

	mocks.Clock.Advance(txnBuilder.GetAssembleTimeout() - 1)

	assert.Equal(t, 1, mocks.SentMessageRecorder.NumberOfSentAssembleRequests())
	err := txn.HandleEvent(ctx, &transaction.RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, mocks.SentMessageRecorder.NumberOfSentAssembleRequests())

	assert.Equal(t, transaction.State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_ToPooled_OnRequestTimeout_IfAssembleTimeoutExpired(t *testing.T) {
	ctx := context.Background()
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn, mocks := txnBuilder.BuildWithMocks()

	mocks.Clock.Advance(txnBuilder.GetAssembleTimeout() + 1)

	err := txn.HandleEvent(ctx, &transaction.RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, 1, txn.GetErrorCount(), "expected error count to be 1, but it was %d", txn.GetErrorCount())
}

func TestCoordinatorTransaction_Assembling_ToReverted_OnAssembleRevertResponse(t *testing.T) {
	ctx := context.Background()
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).
		Reverts("some revert reason")

	txn, mocks := txnBuilder.BuildWithMocks()

	mocks.SyncPoints.(*syncpoints.MockSyncPoints).On("QueueTransactionFinalize", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := txn.HandleEvent(ctx, &transaction.AssembleRevertResponseEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		RequestID:    mocks.SentMessageRecorder.SentAssembleRequestIdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Assembling_NoTransition_OnAssembleRevertResponse_IfResponseDoesNotMatchPendingRequest(t *testing.T) {
	ctx := context.Background()
	txnBuilder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).
		Reverts("some revert reason")

	txn := txnBuilder.Build()

	err := txn.HandleEvent(ctx, &transaction.AssembleRevertResponseEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		PostAssembly: txnBuilder.BuildPostAssembly(),
		RequestID:    uuid.New(), //generate a new random request ID so that it won't match the pending request,
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Pooled_ToPreAssemblyBlocked_OnDependencyReverted(t *testing.T) {
	ctx := context.Background()

	//we need 2 transactions to know about each other so they need to share a state index
	grapher := transaction.NewGrapher(ctx)

	//transaction2 depends on transaction 1 and transaction 1 gets reverted
	builder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling).
		Grapher(grapher).
		Reverts("some revert reason")
	txn1, mocks1 := builder1.BuildWithMocks()

	mocks1.SyncPoints.(*syncpoints.MockSyncPoints).On("QueueTransactionFinalize", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	builder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pooled).
		Grapher(grapher).
		Originator(builder1.GetOriginator()).
		PredefinedDependencies(txn1.ID)
	txn2 := builder2.Build()

	err := txn1.HandleEvent(ctx, &transaction.AssembleRevertResponseEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn1.ID,
		},
		PostAssembly: builder1.BuildPostAssembly(),
		RequestID:    mocks1.SentMessageRecorder.SentAssembleRequestIdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_PreAssembly_Blocked, txn2.GetCurrentState(), "current state is %s", txn2.GetCurrentState().String())

}

func TestCoordinatorTransaction_Endorsement_Gathering_NudgeRequests_OnRequestTimeout_IfPendingRequests(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3)

	txn, mocks := builder.BuildWithMocks()
	assert.Equal(t, 3, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests())

	mocks.Clock.Advance(builder.GetRequestTimeout() + 1)

	err := txn.HandleEvent(ctx, &transaction.RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, 6, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests())

	assert.Equal(t, transaction.State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestCoordinatorTransaction_Endorsement_Gathering_NudgeRequests_OnRequestTimeout_IfPendingRequests_Partial(t *testing.T) {
	//emulate the case where only a subset of the endorsement requests have timed out
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(4)

	txn, mocks := builder.BuildWithMocks()
	assert.Equal(t, 4, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests())

	//2 endorsements come back in a timely manner
	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(0))
	assert.NoError(t, err)

	err = txn.HandleEvent(ctx, builder.BuildEndorsedEvent(1))
	assert.NoError(t, err)

	mocks.Clock.Advance(builder.GetRequestTimeout() + 1)

	err = txn.HandleEvent(ctx, &transaction.RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, 6, mocks.SentMessageRecorder.NumberOfSentEndorsementRequests()) // the 4 original requests plus 2 nudge requests
	assert.Equal(t, 1, mocks.SentMessageRecorder.NumberOfEndorsementRequestsForParty(builder.GetEndorsers()[0]))
	assert.Equal(t, 1, mocks.SentMessageRecorder.NumberOfEndorsementRequestsForParty(builder.GetEndorsers()[1]))
	assert.Equal(t, 2, mocks.SentMessageRecorder.NumberOfEndorsementRequestsForParty(builder.GetEndorsers()[2]))
	assert.Equal(t, 2, mocks.SentMessageRecorder.NumberOfEndorsementRequestsForParty(builder.GetEndorsers()[3]))

	assert.Equal(t, transaction.State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestCoordinatorTransaction_Endorsement_Gathering_ToConfirmingDispatch_OnEndorsed_IfAttestationPlanComplete(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)

	txn, mocks := builder.BuildWithMocks()
	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(2))
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Confirming_Dispatchable, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.True(t, mocks.SentMessageRecorder.HasSentDispatchConfirmationRequest(), "expected a dispatch confirmation request to be sent, but none were sent")

}

func TestCoordinatorTransaction_Endorsement_GatheringNoTransition_IfNotAttestationPlanComplete(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(1) //only 1 existing endorsement so the next one does not complete the attestation plan

	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, builder.BuildEndorsedEvent(1))
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.False(t, mocks.SentMessageRecorder.HasSentDispatchConfirmationRequest(), "did not expected a dispatch confirmation request to be sent, but one was sent")

}

func TestCoordinatorTransaction_Endorsement_Gathering_ToBlocked_OnEndorsed_IfAttestationPlanCompleteAndHasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()

	//we need 2 transactions to know about each other so they need to share a state index
	grapher := transaction.NewGrapher(ctx)

	builder1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	txn1 := builder1.Build()

	builder2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2).
		InputStateIDs(txn1.PostAssembly.OutputStates[0].ID)
	txn2 := builder2.Build()

	err := txn2.HandleEvent(ctx, builder2.BuildEndorsedEvent(2))
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Blocked, txn2.GetCurrentState(), "current state is %s", txn2.GetCurrentState().String())

}

func TestCoordinatorTransaction_Endorsement_Gathering_ToPooled_OnEndorseRejected(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)

	txn := builder.Build()
	err := txn.HandleEvent(ctx, builder.BuildEndorseRejectedEvent(2))
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	assert.Equal(t, 1, txn.GetErrorCount())

}

func TestCoordinatorTransaction_ConfirmingDispatch_NudgeRequest_OnRequestTimeout(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable)
	txn, mocks := builder.BuildWithMocks()
	assert.Equal(t, 1, mocks.SentMessageRecorder.NumberOfSentDispatchConfirmationRequests())

	mocks.Clock.Advance(builder.GetRequestTimeout() + 1)

	err := txn.HandleEvent(ctx, &transaction.RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, 2, mocks.SentMessageRecorder.NumberOfSentDispatchConfirmationRequests())
	assert.Equal(t, transaction.State_Confirming_Dispatchable, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_ConfirmingDispatch_ToReadyForDispatch_OnDispatchConfirmed(t *testing.T) {
	ctx := context.Background()
	txn, mocks := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).BuildWithMocks()

	err := txn.HandleEvent(ctx, &transaction.DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID: mocks.SentMessageRecorder.SentDispatchConfirmationRequestIdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Ready_For_Dispatch, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_ConfirmingDispatch_NoTransition_OnDispatchConfirmed_IfResponseDoesNotMatchPendingRequest(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).Build()

	err := txn.HandleEvent(ctx, &transaction.DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RequestID: uuid.New(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Confirming_Dispatchable, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Blocked_ToConfirmingDispatch_OnDependencyReady_IfNotHasDependenciesNotReady(t *testing.T) {
	//TODO rethink naming of this test and/or the guard function because we end up with a double negative
	ctx := context.Background()

	//A transaction (A) is dependant on another 2 transactions (B and C).  One of which (B) is ready for dispatch and the other (C) becomes ready for dispatch,
	// triggering a transition for A to move from blocked to confirming dispatch

	//we need 3 transactions to know about each other so they need to share a state index
	grapher := transaction.NewGrapher(ctx)

	builderB := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).
		Grapher(grapher)
	txnB := builderB.Build()

	builderC := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).
		Grapher(grapher)
	txnC, mocksC := builderC.BuildWithMocks()

	builderA := transaction.NewTransactionBuilderForTesting(t, transaction.State_Blocked).
		Grapher(grapher).
		InputStateIDs(
			txnB.PostAssembly.OutputStates[0].ID,
			txnC.PostAssembly.OutputStates[0].ID,
		)
	txnA := builderA.Build()

	//Was in 2 minds whether to a) trigger transaction A indirectly by causing C to become ready via a dispatch confirmation event or b) trigger it directly by sending a dependency ready event
	// decided on (a) as it is slightly less white box and less brittle to future refactoring of the implementation

	err := txnC.HandleEvent(ctx, &transaction.DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txnC.ID,
		},
		RequestID: mocksC.SentMessageRecorder.SentDispatchConfirmationRequestIdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Confirming_Dispatchable, txnA.GetCurrentState(), "current state is %s", txnA.GetCurrentState().String())

}

func TestCoordinatorTransaction_BlockedNoTransition_OnDependencyReady_IfHasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()

	//A transaction (A) is dependant on another 2 transactions (B and C).  Neither of which a ready for dispatch. One of them (B) becomes ready for dispatch, but the other is still not ready
	// thus gating the triggering of a transition for A to move from blocked to confirming dispatch

	//we need 3 transactions to know about each other so they need to share a state index
	grapher := transaction.NewGrapher(ctx)

	builderB := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).
		Grapher(grapher)
	txnB, mocksB := builderB.BuildWithMocks()

	builderC := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).
		Grapher(grapher)
	txnC := builderC.Build()

	builderA := transaction.NewTransactionBuilderForTesting(t, transaction.State_Blocked).
		Grapher(grapher).
		InputStateIDs(
			txnB.PostAssembly.OutputStates[0].ID,
			txnC.PostAssembly.OutputStates[0].ID,
		)
	txnA := builderA.Build()

	//Was in 2 minds whether to a) trigger transaction A indirectly by causing B to become ready via a dispatch confirmation event or b) trigger it directly by sending a dependency ready event
	// decided on (a) as it is slightly less white box and less brittle to future refactoring of the implementation

	err := txnB.HandleEvent(ctx, &transaction.DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txnB.ID,
		},
		RequestID: mocksB.SentMessageRecorder.SentDispatchConfirmationRequestIdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Blocked, txnA.GetCurrentState(), "current state is %s", txnA.GetCurrentState().String())

}

func TestCoordinatorTransaction_ReadyForDispatch_ToDispatched_OnDispatched(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Ready_For_Dispatch).Build()

	err := txn.HandleEvent(ctx, &transaction.DispatchedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Dispatched, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Dispatched_ToSubmissionPrepared_OnCollected(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()

	err := txn.HandleEvent(ctx, &transaction.CollectedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_SubmissionPrepared, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_SubmissionPrepared_ToSubmitted_OnSubmitted(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_SubmissionPrepared).Build()

	err := txn.HandleEvent(ctx, &transaction.SubmittedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Submitted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Submitted_ToPooled_OnConfirmed_IfRevert(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	err := txn.HandleEvent(ctx, &transaction.ConfirmedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
		RevertReason: pldtypes.HexBytes("0x01020304"),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Pooled, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Submitted_ToConfirmed_IfNoRevert(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	err := txn.HandleEvent(ctx, &transaction.ConfirmedEvent{
		BaseCoordinatorEvent: transaction.BaseCoordinatorEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Confirmed_ToFinal_OnHeartbeatInterval_IfHasBeenIncludedInEnoughHeartbeats(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).HeartbeatIntervalsSinceStateChange(4).Build()

	err := txn.HandleEvent(ctx, &transaction.HeartbeatIntervalEvent{})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Final, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestCoordinatorTransaction_Confirmed_NoTransition_OnHeartbeatInterval_IfNotHasBeenIncludedInEnoughHeartbeats(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirmed).HeartbeatIntervalsSinceStateChange(3).Build()

	err := txn.HandleEvent(ctx, &transaction.HeartbeatIntervalEvent{})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}
