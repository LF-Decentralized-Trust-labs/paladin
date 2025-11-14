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

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOriginatorTransaction_InitializeOK(t *testing.T) {

	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Initial).Build()
	assert.NotNil(t, txn)
	assert.Equal(t, transaction.State_Initial, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Initial_ToPending_OnCreated(t *testing.T) {
	ctx := context.Background()

	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Initial).Build()
	assert.Equal(t, transaction.State_Initial, txn.GetCurrentState())

	err := txn.HandleEvent(ctx, &transaction.CreatedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, transaction.State_Pending, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Pending_ToDelegated_OnDelegated(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Pending)
	txn := builder.Build()

	err := txn.HandleEvent(ctx, &transaction.DelegatedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)
	assert.Equal(t, transaction.State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Delegated_ToAssembling_OnAssembleRequestReceived_OK(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated)
	txn, mocks := builder.BuildWithMocks()

	mocks.MockForAssembleAndSignRequestOK().Once()
	requestID := uuid.New()

	err := txn.HandleEvent(ctx, &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:   requestID,
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)
	assert.True(t, mocks.EngineIntegration.AssertExpectations(t))

	require.Len(t, mocks.GetEmittedEvents(), 1)
	require.IsType(t, &transaction.AssembleAndSignSuccessEvent{}, mocks.GetEmittedEvents()[0])

	//We haven't fed that event back into the state machine yet, so the state should still be Assembling
	currentState := txn.GetCurrentState()
	assert.Equal(t, transaction.State_Assembling, currentState, "current state is %s", currentState.String())
}

func TestOriginatorTransaction_Delegated_ToAssembling_OnAssembleRequestReceived_REVERT(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated)
	txn, mocks := builder.BuildWithMocks()

	mocks.MockForAssembleAndSignRequestRevert().Once()
	requestID := uuid.New()

	err := txn.HandleEvent(ctx, &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:   requestID,
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)
	assert.True(t, mocks.EngineIntegration.AssertExpectations(t))

	require.Len(t, mocks.GetEmittedEvents(), 1)
	require.IsType(t, &transaction.AssembleRevertEvent{}, mocks.GetEmittedEvents()[0])

	//We haven't fed that event back into the state machine yet, so the state should still be Assembling
	assert.Equal(t, transaction.State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Delegated_ToAssembling_OnAssembleRequestReceived_PARK(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated)
	txn, mocks := builder.BuildWithMocks()

	mocks.MockForAssembleAndSignRequestPark().Once()
	requestID := uuid.New()

	err := txn.HandleEvent(ctx, &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:   requestID,
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)
	assert.True(t, mocks.EngineIntegration.AssertExpectations(t))

	require.Len(t, mocks.GetEmittedEvents(), 1)
	require.IsType(t, &transaction.AssembleParkEvent{}, mocks.GetEmittedEvents()[0])

	//We haven't fed that event back into the state machine yet, so the state should still be Assembling
	assert.Equal(t, transaction.State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Assembling_ToEndorsement_Gathering_OnAssembleAndSignSuccess(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &transaction.AssembleAndSignSuccessEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		PostAssembly: &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			//TODO use a builder to create a more realistically populated PostAssembly
		},
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleSuccessResponse(), "assemble success response was not sent back to coordinator")
	assert.Equal(t, transaction.State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Assembling_ToReverted_OnAssembleRevert(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &transaction.AssembleRevertEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		PostAssembly: &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			RevertReason:   ptrTo("test revert reason"),
		},
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRevertResponse(), "assemble revert response was not sent back to coordinator")
	assert.Equal(t, transaction.State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Assembling_ToParked_OnAssemblePark(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Assembling)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &transaction.AssembleParkEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		PostAssembly: &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_PARK,
		},
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleParkResponse(), "assemble park response was not sent back to coordinator")
	assert.Equal(t, transaction.State_Parked, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Delegated_ToReverted_OnAssembleRequestReceived_AfterAssembleCompletesRevert(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated)
	txn, mocks := builder.BuildWithMocks()

	mocks.MockForAssembleAndSignRequestRevert().Once()

	err := txn.HandleEvent(ctx, &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)
	assert.True(t, mocks.EngineIntegration.AssertExpectations(t))

	require.Len(t, mocks.GetEmittedEvents(), 1)
	require.IsType(t, &transaction.AssembleRevertEvent{}, mocks.GetEmittedEvents()[0])
	err = txn.HandleEvent(ctx, mocks.GetEmittedEvents()[0])
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRevertResponse(), "assemble revert response was not sent back to coordinator")
	assert.Equal(t, transaction.State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	//TODO assert that transaction was finalized as Reverted in the database
}

func TestOriginatorTransaction_Delegated_ToParked_OnAssembleRequestReceived_AfterAssembleCompletesPark(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Delegated)
	txn, mocks := builder.BuildWithMocks()

	mocks.MockForAssembleAndSignRequestPark().Once()

	err := txn.HandleEvent(ctx, &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)
	assert.True(t, mocks.EngineIntegration.AssertExpectations(t))

	require.Len(t, mocks.GetEmittedEvents(), 1)
	require.IsType(t, &transaction.AssembleParkEvent{}, mocks.GetEmittedEvents()[0])
	err = txn.HandleEvent(ctx, mocks.GetEmittedEvents()[0])
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleParkResponse(), "assemble park response was not sent back to coordinator")
	assert.Equal(t, transaction.State_Parked, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
	//TODO assert that transaction was finalized as Parked in the database
}

func TestOriginatorTransaction_Endorsement_Gathering_NoTransition_OnAssembleRequest_IfMatchesPreviousRequest(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering)
	txn, mocks := builder.BuildWithMocks()

	// NOTE we do not mock AssembleAndSign function because we expect to resend the previous response

	err := txn.HandleEvent(ctx, &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:   builder.GetLatestFulfilledAssembleRequestID(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleSuccessResponse(), "assemble success response was not sent back to coordinator")
	assert.Equal(t, transaction.State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Reverted_DoResendAssembleResponse_OnAssembleRequest_IfMatchesPreviousRequest(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Reverted)
	txn, mocks := builder.BuildWithMocks()

	// NOTE we do not mock AssembleAndSign function because we expect to resend the previous response

	err := txn.HandleEvent(ctx, &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:   builder.GetLatestFulfilledAssembleRequestID(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleRevertResponse(), "assemble revert response was not sent back to coordinator")
	assert.Equal(t, transaction.State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Reverted_Ignore_OnAssembleRequest_IfNotMatchesPreviousRequest(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Reverted)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:   uuid.New(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.False(t, mocks.SentMessageRecorder.HasSentAssembleRevertResponse(), "assemble revert response was unexpectedly sent to coordinator")
	assert.Equal(t, transaction.State_Reverted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Parked_DoResendAssembleResponse_OnAssembleRequest_IfMatchesPreviousRequest(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Parked)
	txn, mocks := builder.BuildWithMocks()

	// NOTE we do not mock AssembleAndSign function because we expect to resend the previous response

	err := txn.HandleEvent(ctx, &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:   builder.GetLatestFulfilledAssembleRequestID(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentAssembleParkResponse(), "assemble park response was not sent back to coordinator")
	assert.Equal(t, transaction.State_Parked, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestOriginatorTransaction_Parked_Ignore_OnAssembleRequest_IfNotMatchesPreviousRequest(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Parked)
	txn, mocks := builder.BuildWithMocks()

	err := txn.HandleEvent(ctx, &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:   uuid.New(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.False(t, mocks.SentMessageRecorder.HasSentAssembleParkResponse(), "assemble park response was unexpectedly sent to coordinator")
	assert.Equal(t, transaction.State_Parked, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestOriginatorTransaction_Endorsement_Gathering_ToAssembling_OnAssembleRequest_IfNotMatchesPreviousRequest(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering)
	txn, mocks := builder.BuildWithMocks()
	// This should trigger a re-assembly
	mocks.MockForAssembleAndSignRequestOK().Once()

	err := txn.HandleEvent(ctx, &transaction.AssembleRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		RequestID:   uuid.New(),
		Coordinator: builder.GetCoordinator(),
	})
	assert.NoError(t, err)

	assert.True(t, mocks.EngineIntegration.AssertExpectations(t))

	require.Len(t, mocks.GetEmittedEvents(), 1)
	require.IsType(t, &transaction.AssembleAndSignSuccessEvent{}, mocks.GetEmittedEvents()[0])

	//We haven't fed that event back into the state machine yet, so the state should still be Assembling
	assert.Equal(t, transaction.State_Assembling, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestOriginatorTransaction_Endorsement_Gathering_ToPrepared_OnDispatchConfirmationRequestReceivedIfMatches(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering)
	txn, mocks := builder.BuildWithMocks()

	hash, err := txn.Hash(ctx)
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.PreDispatchRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		Coordinator:      builder.GetCoordinator(),
		PostAssemblyHash: hash,
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentPreDispatchResponse(), "dispatch confirmation response was not sent back to coordinator")
	assert.Equal(t, transaction.State_Prepared, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Endorsement_Gathering_NoTransition_OnDispatchConfirmationRequestReceivedIfNotMatches_WrongCoordinator(t *testing.T) {

	ctx := context.Background()

	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering)
	txn, mocks := builder.BuildWithMocks()

	hash, err := txn.Hash(ctx)
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.PreDispatchRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		Coordinator:      uuid.New().String(),
		PostAssemblyHash: hash,
	})
	assert.NoError(t, err)

	assert.False(t, mocks.SentMessageRecorder.HasSentPreDispatchResponse(), "dispatch confirmation response was unexpectedly sent back to coordinator")
	assert.Equal(t, transaction.State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Endorsement_Gathering_NoTransition_OnDispatchConfirmationRequestReceivedIfNotMatches_WrongHash(t *testing.T) {

	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering)
	txn, mocks := builder.BuildWithMocks()

	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))

	err := txn.HandleEvent(ctx, &transaction.PreDispatchRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		Coordinator:      builder.GetCoordinator(),
		PostAssemblyHash: &hash,
	})
	assert.NoError(t, err)

	assert.False(t, mocks.SentMessageRecorder.HasSentPreDispatchResponse(), "dispatch confirmation response was unexpectedly sent back to coordinator")
	assert.Equal(t, transaction.State_Endorsement_Gathering, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Endorsement_Gathering_ToDelegated_OnCoordinatorChanged(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsement_Gathering).Build()

	err := txn.HandleEvent(ctx, &transaction.CoordinatorChangedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		Coordinator: uuid.New().String(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestOriginatorTransaction_Prepared_ToDispatched_OnDispatched(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Prepared).Build()

	signerAddress := pldtypes.EthAddress(pldtypes.RandBytes(20))

	err := txn.HandleEvent(ctx, &transaction.DispatchedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		SignerAddress: signerAddress,
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Dispatched, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Prepared_NoTransition_Do_Resend_OnDispatchConfirmationRequestReceivedIfMatches(t *testing.T) {
	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Prepared)
	txn, mocks := builder.BuildWithMocks()

	hash, err := txn.Hash(ctx)
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.PreDispatchRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		Coordinator:      builder.GetCoordinator(),
		PostAssemblyHash: hash,
	})
	assert.NoError(t, err)

	assert.True(t, mocks.SentMessageRecorder.HasSentPreDispatchResponse(), "dispatch confirmation response was not sent back to coordinator")
	assert.Equal(t, transaction.State_Prepared, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Prepared_Ignore_OnDispatchConfirmationRequestReceivedIfNotMatches_WrongCoordinator(t *testing.T) {

	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Prepared)
	txn, mocks := builder.BuildWithMocks()

	hash, err := txn.Hash(ctx)
	require.NoError(t, err)

	err = txn.HandleEvent(ctx, &transaction.PreDispatchRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		Coordinator:      uuid.New().String(),
		PostAssemblyHash: hash,
	})
	assert.NoError(t, err)

	assert.False(t, mocks.SentMessageRecorder.HasSentPreDispatchResponse(), "dispatch confirmation response was unexpectedly sent back to coordinator")
	assert.Equal(t, transaction.State_Prepared, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Prepared_Ignore_OnDispatchConfirmationRequestReceivedIfNotMatches_WrongHash(t *testing.T) {

	ctx := context.Background()
	builder := transaction.NewTransactionBuilderForTesting(t, transaction.State_Prepared)
	txn, mocks := builder.BuildWithMocks()

	hash := pldtypes.Bytes32(pldtypes.RandBytes(32))

	err := txn.HandleEvent(ctx, &transaction.PreDispatchRequestReceivedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		Coordinator:      builder.GetCoordinator(),
		PostAssemblyHash: &hash,
	})
	assert.NoError(t, err)

	assert.False(t, mocks.SentMessageRecorder.HasSentPreDispatchResponse(), "dispatch confirmation response was unexpectedly sent back to coordinator")
	assert.Equal(t, transaction.State_Prepared, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Prepared_ToDelegated_OnCoordinatorChanged(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Prepared).Build()

	err := txn.HandleEvent(ctx, &transaction.CoordinatorChangedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		Coordinator: uuid.New().String(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestOriginatorTransaction_Dispatched_ToConfirmed_OnConfirmedSuccess(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()

	err := txn.HandleEvent(ctx, &transaction.ConfirmedSuccessEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Dispatched_ToSequenced_OnNonceAssigned(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()

	err := txn.HandleEvent(ctx, &transaction.NonceAssignedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		SignerAddress: pldtypes.EthAddress(pldtypes.RandBytes(20)),
		Nonce:         42,
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Sequenced, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Dispatched_ToSubmitted_OnSubmitted(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()

	err := txn.HandleEvent(ctx, &transaction.SubmittedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		SignerAddress:        pldtypes.EthAddress(pldtypes.RandBytes(20)),
		Nonce:                42,
		LatestSubmissionHash: pldtypes.Bytes32(pldtypes.RandBytes(32)),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Submitted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Dispatched_ToDelegated_OnConfirmedReverted(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()

	err := txn.HandleEvent(ctx, &transaction.ConfirmedRevertedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Dispatched_ToDelegated_OnCoordinatorChanged(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Dispatched).Build()

	err := txn.HandleEvent(ctx, &transaction.CoordinatorChangedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		Coordinator: uuid.New().String(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestOriginatorTransaction_Sequenced_ToConfirmed_OnConfirmedSuccess(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Sequenced).Build()

	err := txn.HandleEvent(ctx, &transaction.ConfirmedSuccessEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Sequenced_ToSubmitted_OnSubmitted(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Sequenced).Build()

	err := txn.HandleEvent(ctx, &transaction.SubmittedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		SignerAddress:        pldtypes.EthAddress(pldtypes.RandBytes(20)),
		Nonce:                42,
		LatestSubmissionHash: pldtypes.Bytes32(pldtypes.RandBytes(32)),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Submitted, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Sequenced_ToDelegated_OnConfirmedReverted(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Sequenced).Build()

	err := txn.HandleEvent(ctx, &transaction.ConfirmedRevertedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Sequenced_ToDelegated_OnCoordinatorChanged(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Sequenced).Build()

	err := txn.HandleEvent(ctx, &transaction.CoordinatorChangedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		Coordinator: uuid.New().String(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}

func TestOriginatorTransaction_Submitted_ToConfirmed_OnConfirmedSuccess(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	err := txn.HandleEvent(ctx, &transaction.ConfirmedSuccessEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Confirmed, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Submitted_ToDelegated_OnConfirmedReverted(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	err := txn.HandleEvent(ctx, &transaction.ConfirmedRevertedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func TestOriginatorTransaction_Submitted_ToDelegated_OnCoordinatorChanged(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	err := txn.HandleEvent(ctx, &transaction.CoordinatorChangedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
		Coordinator: uuid.New().String(),
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Delegated, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())

}
func TestOriginatorTransaction_Parked_ToPending_OnResumed(t *testing.T) {
	ctx := context.Background()
	txn := transaction.NewTransactionBuilderForTesting(t, transaction.State_Parked).Build()

	err := txn.HandleEvent(ctx, &transaction.ResumedEvent{
		BaseEvent: transaction.BaseEvent{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, transaction.State_Pending, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}

func ptrTo[T any](v T) *T {
	return &v
}
