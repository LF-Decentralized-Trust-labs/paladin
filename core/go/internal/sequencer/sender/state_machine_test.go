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

package sender

import (
	"context"
	"testing"

	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/mocks/sequencermocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	TestDefault_HeartbeatThreshold  int = 5
	TestDefault_HeartbeatIntervalMs int = 100
)

func TestStateMachine_InitializeOK(t *testing.T) {
	ctx := context.Background()
	s, _ := NewSenderForUnitTest(t, ctx, nil)

	assert.Equal(t, State_Idle, s.stateMachine.currentState, "current state is %s", s.stateMachine.currentState.String())
}

func TestStateMachine_Idle_ToObserving_OnHeartbeatReceived(t *testing.T) {
	ctx := context.Background()
	s, _ := NewSenderForUnitTest(t, ctx, nil)
	assert.Equal(t, State_Idle, s.stateMachine.currentState)

	err := s.HandleEvent(ctx, &HeartbeatReceivedEvent{})
	assert.NoError(t, err)
	assert.Equal(t, State_Observing, s.stateMachine.currentState, "current state is %s", s.stateMachine.currentState.String())

}

func TestStateMachine_Idle_ToSending_OnTransactionCreated(t *testing.T) {
	ctx := context.Background()
	s, mocks := NewSenderForUnitTest(t, ctx, nil)
	assert.Equal(t, State_Idle, s.stateMachine.currentState)

	err := s.HandleEvent(ctx, &TransactionCreatedEvent{})
	assert.NoError(t, err)
	assert.Equal(t, State_Sending, s.stateMachine.currentState, "current state is %s", s.stateMachine.currentState.String())

	assert.True(t, mocks.messageSender.HasSentDelegationRequest())
}

func TestStateMachine_Observing_ToSending_OnTransactionCreated(t *testing.T) {
	ctx := context.Background()
	s, mocks := NewSenderForUnitTest(t, ctx, nil)
	s.stateMachine.currentState = State_Observing

	err := s.HandleEvent(ctx, &TransactionCreatedEvent{})
	assert.NoError(t, err)
	assert.Equal(t, State_Sending, s.stateMachine.currentState, "current state is %s", s.stateMachine.currentState.String())

	assert.True(t, mocks.messageSender.HasSentDelegationRequest())
}

func TestStateMachine_Sending_ToObserving_OnTransactionConfirmed_IfNoTransactionsInflight(t *testing.T) {
	ctx := context.Background()
	s, _ := NewSenderForUnitTest(t, ctx, nil)
	s.stateMachine.currentState = State_Sending

	err := s.HandleEvent(ctx, &TransactionConfirmedEvent{})
	assert.NoError(t, err)
	assert.Equal(t, State_Observing, s.stateMachine.currentState, "current state is %s", s.stateMachine.currentState.String())
}

func TestStateMachine_Observing_ToIdle_OnHeartbeatInterval_IfHeartbeatThresholdExpired(t *testing.T) {
	ctx := context.Background()
	s, mocks := NewSenderForUnitTest(t, ctx, nil)
	s.stateMachine.currentState = State_Observing
	err := s.HandleEvent(ctx, &HeartbeatReceivedEvent{})
	assert.NoError(t, err)

	mocks.clock.Advance(TestDefault_HeartbeatThreshold*TestDefault_HeartbeatIntervalMs + 1)

	err = s.HandleEvent(ctx, &HeartbeatIntervalEvent{})
	assert.NoError(t, err)
	assert.Equal(t, State_Idle, s.stateMachine.currentState, "current state is %s", s.stateMachine.currentState.String())
}

func TestStateMachine_Observing_NoTransition_OnHeartbeatInterval_IfHeartbeatThresholdNotExpired(t *testing.T) {
	ctx := context.Background()
	s, mocks := NewSenderForUnitTest(t, ctx, nil)
	s.stateMachine.currentState = State_Observing
	err := s.HandleEvent(ctx, &HeartbeatReceivedEvent{})
	assert.NoError(t, err)

	mocks.clock.Advance(TestDefault_HeartbeatThreshold*TestDefault_HeartbeatIntervalMs - 1)

	err = s.HandleEvent(ctx, &HeartbeatIntervalEvent{})
	assert.NoError(t, err)
	assert.Equal(t, State_Observing, s.stateMachine.currentState, "current state is %s", s.stateMachine.currentState.String())
}

// TODO should we move this to common and reuse it in e.g. coordinator tests?
type senderDependencyMocks struct {
	messageSender    *SentMessageRecorder
	clock            *common.FakeClockForTesting
	stateIntegration *sequencermocks.StateIntegration
	emit             common.EmitEvent
}

func NewSenderForUnitTest(t *testing.T, ctx context.Context, committeeMembers []string) (*sender, *senderDependencyMocks) {

	if committeeMembers == nil {
		committeeMembers = []string{"member1@node1"}
	}
	mocks := &senderDependencyMocks{
		messageSender:    NewSentMessageRecorder(),
		clock:            &common.FakeClockForTesting{},
		stateIntegration: sequencermocks.NewStateIntegration(t),
		emit:             func(event common.Event) {}, //TODO do something useful to allow tests to receive events from the state machine
	}

	coordinator, err := NewSender(ctx, mocks.messageSender, committeeMembers, mocks.clock, mocks.emit, mocks.stateIntegration, 100, tktypes.RandAddress(), TestDefault_HeartbeatIntervalMs, TestDefault_HeartbeatThreshold)
	require.NoError(t, err)

	return coordinator, mocks
}
