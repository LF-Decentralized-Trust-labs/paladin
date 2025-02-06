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
	stateIndex := &stateIndex{
		transactionByOutputState: make(map[string]*Transaction),
	}

	transaction1Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		StateIndex(stateIndex).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	transaction1 := transaction1Builder.Build()

	transaction2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		StateIndex(stateIndex).
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
	stateIndex := &stateIndex{
		transactionByOutputState: make(map[string]*Transaction),
	}

	transaction1Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		StateIndex(stateIndex).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	transaction1 := transaction1Builder.Build()

	transaction2Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		StateIndex(stateIndex).
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
	stateIndex := &stateIndex{
		transactionByOutputState: make(map[string]*Transaction),
	}

	transaction1Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		StateIndex(stateIndex).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	transaction1 := transaction1Builder.Build()

	transaction2Builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
		StateIndex(stateIndex).
		NumberOfOutputStates(1).
		NumberOfRequiredEndorsers(3).
		NumberOfEndorsements(2)
	transaction2 := transaction2Builder.Build()

	transaction3Builder := NewTransactionBuilderForTesting(t, State_Assembling).
		StateIndex(stateIndex).
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

type transactionDependencyMocks struct {
	messageSender *MockMessageSender
	clock         *common.FakeClockForTesting
}

func newTransactionForUnitTesting(t *testing.T, stateIndex *stateIndex) (*Transaction, *transactionDependencyMocks) {
	if stateIndex == nil {
		stateIndex = NewStateIndex(context.Background())
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
		stateIndex,
	)

	return txn, mocks

}
