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
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/mocks/sequencermocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransaction_HasDependenciesNotReady_FalseIfNoDependencies(t *testing.T) {
	ctx := context.Background()
	transaction, _ := newTransactionForUnitTesting(t, nil)
	assert.False(t, transaction.hasDependenciesNotReady(ctx))
}

func TestTransaction_HasDependenciesNotReady_TrueOK(t *testing.T) {
	grapher := NewGrapher(context.Background())

	transaction1Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	transaction1 := transaction1Builder.Build()

	transaction2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		InputStateIDs(transaction1.PostAssembly.OutputStates[0].ID)
	transaction2 := transaction2Builder.Build()

	err := transaction2.HandleEvent(context.Background(), &AssembleSuccessEvent{
		BaseEvent: BaseEvent{
			TransactionID: transaction2.ID,
		},
		PostAssembly: transaction2Builder.BuildPostAssembly(),
		RequestID:    transaction2.pendingAssembleRequest.IdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.True(t, transaction2.hasDependenciesNotReady(context.Background()))

}

func TestTransaction_HasDependenciesNotReady_TrueWhenStatesAreReadOnly(t *testing.T) {
	grapher := NewGrapher(context.Background())

	transaction1Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	transaction1 := transaction1Builder.Build()

	transaction2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		ReadStateIDs(transaction1.PostAssembly.OutputStates[0].ID)
	transaction2 := transaction2Builder.Build()

	err := transaction2.HandleEvent(context.Background(), &AssembleSuccessEvent{
		BaseEvent: BaseEvent{
			TransactionID: transaction2.ID,
		},
		PostAssembly: transaction2Builder.BuildPostAssembly(),
		RequestID:    transaction2.pendingAssembleRequest.IdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.True(t, transaction2.hasDependenciesNotReady(context.Background()))

}

func TestTransaction_HasDependenciesNotReady(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	transaction1Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	transaction1 := transaction1Builder.Build()

	transaction2Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Grapher(grapher).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	transaction2 := transaction2Builder.Build()

	transaction3Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		Grapher(grapher).
		InputStateIDs(transaction1.PostAssembly.OutputStates[0].ID, transaction2.PostAssembly.OutputStates[0].ID)
	transaction3 := transaction3Builder.Build()

	err := transaction3.HandleEvent(context.Background(), &AssembleSuccessEvent{
		BaseEvent: BaseEvent{
			TransactionID: transaction3.ID,
		},
		PostAssembly: transaction3Builder.BuildPostAssembly(),
		RequestID:    transaction3.pendingAssembleRequest.IdempotencyKey(),
	})
	assert.NoError(t, err)

	assert.True(t, transaction3.hasDependenciesNotReady(context.Background()))

	//move both dependencies forward
	err = transaction1.HandleEvent(ctx, transaction1Builder.BuildEndorsedEvent(2))
	assert.NoError(t, err)
	err = transaction2.HandleEvent(ctx, transaction2Builder.BuildEndorsedEvent(2))
	assert.NoError(t, err)

	//Should still be blocked because dependencies have not been confirmed for dispatch yet
	assert.Equal(t, State_Confirming_Dispatch, transaction1.stateMachine.currentState)
	assert.Equal(t, State_Confirming_Dispatch, transaction2.stateMachine.currentState)
	assert.True(t, transaction3.hasDependenciesNotReady(context.Background()))

	//move one dependency to ready to dispatch
	err = transaction1.HandleEvent(ctx, &DispatchConfirmedEvent{
		BaseEvent: BaseEvent{
			TransactionID: transaction1.ID,
		},
		RequestID: transaction1.pendingDispatchConfirmationRequest.IdempotencyKey(),
	})
	assert.NoError(t, err)

	//Should still be blocked because not all dependencies have been confirmed for dispatch yet
	assert.Equal(t, State_Ready_For_Dispatch, transaction1.stateMachine.currentState)
	assert.Equal(t, State_Confirming_Dispatch, transaction2.stateMachine.currentState)
	assert.True(t, transaction3.hasDependenciesNotReady(context.Background()))

	//finally move the last dependency to ready to dispatch
	err = transaction2.HandleEvent(ctx, &DispatchConfirmedEvent{
		BaseEvent: BaseEvent{
			TransactionID: transaction2.ID,
		},
		RequestID: transaction2.pendingDispatchConfirmationRequest.IdempotencyKey(),
	})
	assert.NoError(t, err)

	//Should still be blocked because not all dependencies have been confirmed for dispatch yet
	assert.Equal(t, State_Ready_For_Dispatch, transaction1.stateMachine.currentState)
	assert.Equal(t, State_Ready_For_Dispatch, transaction2.stateMachine.currentState)
	assert.False(t, transaction3.hasDependenciesNotReady(context.Background()))

}

func TestTransaction_HasDependenciesNotReady_FalseIfHasNoDependencies(t *testing.T) {

	transaction1 := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		Build()

	assert.False(t, transaction1.hasDependenciesNotReady(context.Background()))

}

func TestTransaction_AddsItselfToGrapher(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	transaction, _ := newTransactionForUnitTesting(t, grapher)

	txn := grapher.TransactionByID(ctx, transaction.ID)

	assert.NotNil(t, txn)
}

func TestTransaction_RemovesItselfFromGrapher(t *testing.T) {
	ctx := context.Background()
	grapher := NewGrapher(ctx)

	transaction, _ := newTransactionForUnitTesting(t, grapher)

	err := transaction.cleanup(ctx)
	assert.NoError(t, err)

	txn := grapher.TransactionByID(ctx, transaction.ID)
	assert.Nil(t, txn)
}

type transactionDependencyMocks struct {
	messageSender     *MockMessageSender
	clock             *common.FakeClockForTesting
	engineIntegration *sequencermocks.EngineIntegration
}

func newTransactionForUnitTesting(t *testing.T, grapher Grapher) (*Transaction, *transactionDependencyMocks) {
	if grapher == nil {
		grapher = NewGrapher(context.Background())
	}
	mocks := &transactionDependencyMocks{
		messageSender:     NewMockMessageSender(t),
		clock:             &common.FakeClockForTesting{},
		engineIntegration: sequencermocks.NewEngineIntegration(t),
	}
	txn, err := NewTransaction(
		context.Background(),
		fmt.Sprintf("%s@%s", uuid.NewString(), uuid.NewString()),
		&components.PrivateTransaction{
			ID: uuid.New(),
		},
		mocks.messageSender,
		mocks.clock,
		func(_ common.Event) {},
		mocks.engineIntegration,
		mocks.clock.Duration(1000),
		mocks.clock.Duration(5000),
		5,
		grapher,
		func(ctx context.Context, txn *Transaction, from, to State) {
		},
		func(context.Context) {}, // onCleanup function, not used in tests
	)
	require.NoError(t, err)

	return txn, mocks

}

//TODO add unit test for the guards and various different combinations of dependency not ready scenarios ( e.g. pre-assemble dependencies vs post-assemble dependencies) and for those dependencies being in various different states ( the state machine test only test for "not assembled" or "not ready" but each of these "not" states actually correspond to several possible finite states.)

//TODO add unit tests to assert that if a dependency arrives after its dependent, then the dependency is correctly updated with a reference to the dependent so that we can notify the dependent when the dependency state changes ( e.g. is dispatched, is assembled)
// . - or think about whether this should this be a state machine test?

//TODO add unit test for notification function being called
// - it should be able to cause a sequencer abend if it hits an error
