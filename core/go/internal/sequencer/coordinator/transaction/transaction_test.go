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

	transaction2.HandleEvent(context.Background(), &AssembleSuccessEvent{
		event: event{
			TransactionID: transaction2.ID,
		},
		postAssembly: transaction2Builder.BuildPostAssembly(),
	})

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

	transaction2.HandleEvent(context.Background(), &AssembleSuccessEvent{
		event: event{
			TransactionID: transaction2.ID,
		},
		postAssembly: transaction2Builder.BuildPostAssembly(),
	})

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

	transaction3.HandleEvent(context.Background(), &AssembleSuccessEvent{
		event: event{
			TransactionID: transaction3.ID,
		},
		postAssembly: transaction3Builder.BuildPostAssembly(),
	})

	assert.True(t, transaction3.hasDependenciesNotReady(context.Background()))

	//move both dependencies forward
	transaction1.HandleEvent(ctx, transaction1Builder.BuildEndorsedEvent(2))
	transaction2.HandleEvent(ctx, transaction2Builder.BuildEndorsedEvent(2))

	//Should still be blocked because dependencies have not been confirmed for dispatch yet
	assert.Equal(t, State_Confirming_Dispatch, transaction1.stateMachine.currentState)
	assert.Equal(t, State_Confirming_Dispatch, transaction2.stateMachine.currentState)
	assert.True(t, transaction3.hasDependenciesNotReady(context.Background()))

	//move one dependency to ready to dispatch
	transaction1.HandleEvent(ctx, &DispatchConfirmedEvent{
		event: event{
			TransactionID: transaction1.ID,
		},
	})

	//Should still be blocked because not all dependencies have been confirmed for dispatch yet
	assert.Equal(t, State_Ready_For_Dispatch, transaction1.stateMachine.currentState)
	assert.Equal(t, State_Confirming_Dispatch, transaction2.stateMachine.currentState)
	assert.True(t, transaction3.hasDependenciesNotReady(context.Background()))

	//finally move the last dependency to ready to dispatch
	transaction2.HandleEvent(ctx, &DispatchConfirmedEvent{
		event: event{
			TransactionID: transaction2.ID,
		},
	})

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
	messageSender *MockMessageSender
	clock         *common.FakeClockForTesting
}

func newTransactionForUnitTesting(t *testing.T, grapher Grapher) (*Transaction, *transactionDependencyMocks) {
	if grapher == nil {
		grapher = NewGrapher(context.Background())
	}
	mocks := &transactionDependencyMocks{
		messageSender: NewMockMessageSender(t),
		clock:         &common.FakeClockForTesting{},
	}
	txn := NewTransaction(
		uuid.NewString(),
		&components.PrivateTransaction{
			ID: uuid.New(),
		},
		mocks.messageSender,
		mocks.clock,
		mocks.clock.Duration(1000),
		mocks.clock.Duration(5000),
		grapher,
	)

	return txn, mocks

}

//TODO add unit test for the guards and various different combinations of dependency not read scenarios ( e.g. pre-assemble dependencies vs post-assemble dependencies) and for those dependencies being in various different states ( the state machine test only test for "not assembled" or "not ready" but each of these "not" states actually correspond to several possible finite states.)
