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
	"github.com/kaleido-io/paladin/core/internal/sequencer/testutil"
	"github.com/kaleido-io/paladin/core/mocks/sequencermocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestStateMachine_InitializeOK(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionForUnitTest(t, ctx, testutil.NewPrivateTransactionBuilderForTesting().Build())
	assert.NotNil(t, txn)

	assert.Equal(t, State_Initial, txn.stateMachine.currentState, "current state is %s", txn.stateMachine.currentState.String())
}

func TestStateMachine_Initial_ToPending_OnCreated(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionForUnitTest(t, ctx, testutil.NewPrivateTransactionBuilderForTesting().Build())
	assert.Equal(t, State_Initial, txn.stateMachine.currentState)

	err := txn.HandleEvent(ctx, &CreatedEvent{
		event: event{
			TransactionID: txn.ID,
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Pending, txn.stateMachine.currentState, "current state is %s", txn.stateMachine.currentState.String())
}

func TestStateMachine_Pending_ToDelegated_OnDelegated(t *testing.T) {
	ctx := context.Background()
	txn, _ := NewTransactionForUnitTest(t, ctx, testutil.NewPrivateTransactionBuilderForTesting().Build())

	txn.stateMachine.currentState = State_Pending

	coordinator := uuid.New().String()

	err := txn.HandleEvent(ctx, &DelegatedEvent{
		event: event{
			TransactionID: txn.ID,
		},
		Coordinator: coordinator,
	})
	assert.NoError(t, err)
	assert.Equal(t, State_Delegated, txn.stateMachine.currentState, "current state is %s", txn.stateMachine.currentState.String())
}

func TestStateMachine_Delegated_OnAssembleRequestReceived_AfterAssembleCompletesOK(t *testing.T) {
	ctx := context.Background()
	txn, mocks := NewTransactionForUnitTest(t, ctx, testutil.NewPrivateTransactionBuilderForTesting().Build())
	//TODO move following complexity into utils e.g. using builder pattern as we do with coordianator.Transaction
	coordinator := uuid.New().String()
	txn.currentDelegate = coordinator
	txn.stateMachine.currentState = State_Delegated

	mocks.mockForAssembleAndSignRequestOK().Once()

	err := txn.HandleEvent(ctx, &AssembleRequestReceivedEvent{
		event: event{
			TransactionID: txn.ID,
		},
		Coordinator: coordinator,
	})
	assert.NoError(t, err)
	assert.True(t, mocks.engineIntegration.AssertExpectations(t))
	assert.Equal(t, State_Delegated, txn.stateMachine.currentState, "current state is %s", txn.stateMachine.currentState.String())
}

type transactionDependencyMocks struct {
	messageSender     *SentMessageRecorder
	clock             *common.FakeClockForTesting
	engineIntegration *sequencermocks.EngineIntegration
	emit              common.EmitEvent
	transactionID     uuid.UUID
}

func NewTransactionForUnitTest(t *testing.T, ctx context.Context, pt *components.PrivateTransaction) (*Transaction, *transactionDependencyMocks) {

	mocks := &transactionDependencyMocks{
		messageSender:     NewSentMessageRecorder(),
		clock:             &common.FakeClockForTesting{},
		engineIntegration: sequencermocks.NewEngineIntegration(t),
		emit:              func(event common.Event) {}, //TODO do something useful to allow tests to receive events from the state machine
	}

	txn, err := NewTransaction(ctx, pt, mocks.messageSender, mocks.clock, mocks.emit, mocks.engineIntegration)
	require.NoError(t, err)

	mocks.transactionID = txn.ID

	return txn, mocks
}

func (m *transactionDependencyMocks) mockForAssembleAndSignRequestOK() *mock.Call {

	return m.engineIntegration.On(
		"AssembleAndSign",
		mock.Anything, //ctx context.Contex
		m.transactionID,
		mock.Anything, //preAssembly *components.TransactionPreAssembly
		mock.Anything, //stateLocksJSON []byte
		mock.Anything, //blockHeight int64
	).Return(&components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}, nil)
}
