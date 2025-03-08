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

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/testutil"
	"github.com/kaleido-io/paladin/core/mocks/sequencermocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateMachine_InitializeOK(t *testing.T) {
	ctx := context.Background()

	txn, _ := NewTransactionForUnitTest(t, ctx, testutil.NewPrivateTransactionBuilderForTesting().Build())
	assert.NotNil(t, txn)

	assert.Equal(t, State_Initial, txn.stateMachine.currentState, "current state is %s", txn.stateMachine.currentState.String())
}

func TestStateMachine_Initial_ToPending_OnCreated(t *testing.T) {

}

type transactionDependencyMocks struct {
	messageSender    *SentMessageRecorder
	clock            *common.FakeClockForTesting
	stateIntegration *sequencermocks.StateIntegration
	emit             common.EmitEvent
}

func NewTransactionForUnitTest(t *testing.T, ctx context.Context, pt *components.PrivateTransaction) (*Transaction, *transactionDependencyMocks) {

	mocks := &transactionDependencyMocks{
		messageSender:    NewSentMessageRecorder(),
		clock:            &common.FakeClockForTesting{},
		stateIntegration: sequencermocks.NewStateIntegration(t),
		emit:             func(event common.Event) {}, //TODO do something useful to allow tests to receive events from the state machine
	}

	coordinator, err := NewTransaction(ctx, pt, mocks.messageSender, mocks.clock, mocks.emit, mocks.stateIntegration)
	require.NoError(t, err)

	return coordinator, mocks
}
