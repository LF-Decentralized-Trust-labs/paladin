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

	"github.com/kaleido-io/paladin/core/internal/sequencer/sender"
	"github.com/stretchr/testify/assert"
)

func TestStateMachine_InitializeOK(t *testing.T) {
	ctx := context.Background()
	s, _ := sender.NewSenderBuilderForTesting(sender.State_Idle).Build(ctx)

	assert.Equal(t, sender.State_Idle, s.GetCurrentState(), "current state is %s", s.GetCurrentState().String())
}

func TestStateMachine_Idle_ToObserving_OnHeartbeatReceived(t *testing.T) {
	ctx := context.Background()
	s, _ := sender.NewSenderBuilderForTesting(sender.State_Idle).Build(ctx)
	assert.Equal(t, sender.State_Idle, s.GetCurrentState())

	err := s.HandleEvent(ctx, &sender.HeartbeatReceivedEvent{})
	assert.NoError(t, err)
	assert.Equal(t, sender.State_Observing, s.GetCurrentState(), "current state is %s", s.GetCurrentState().String())

}

func TestStateMachine_Idle_ToSending_OnTransactionCreated(t *testing.T) {
	ctx := context.Background()
	s, mocks := sender.NewSenderBuilderForTesting(sender.State_Idle).Build(ctx)
	assert.Equal(t, sender.State_Idle, s.GetCurrentState())

	err := s.HandleEvent(ctx, &sender.TransactionCreatedEvent{})
	assert.NoError(t, err)
	assert.Equal(t, sender.State_Sending, s.GetCurrentState(), "current state is %s", s.GetCurrentState().String())

	assert.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())
}

func TestStateMachine_Observing_ToSending_OnTransactionCreated(t *testing.T) {
	ctx := context.Background()
	s, mocks := sender.NewSenderBuilderForTesting(sender.State_Observing).Build(ctx)

	err := s.HandleEvent(ctx, &sender.TransactionCreatedEvent{})
	assert.NoError(t, err)
	assert.Equal(t, sender.State_Sending, s.GetCurrentState(), "current state is %s", s.GetCurrentState().String())

	assert.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())
}

func TestStateMachine_Sending_ToObserving_OnTransactionConfirmed_IfNoTransactionsInflight(t *testing.T) {
	ctx := context.Background()
	s, _ := sender.NewSenderBuilderForTesting(sender.State_Sending).Build(ctx)

	err := s.HandleEvent(ctx, &sender.TransactionConfirmedEvent{})
	assert.NoError(t, err)
	assert.Equal(t, sender.State_Observing, s.GetCurrentState(), "current state is %s", s.GetCurrentState().String())
}

func TestStateMachine_Observing_ToIdle_OnHeartbeatInterval_IfHeartbeatThresholdExpired(t *testing.T) {
	ctx := context.Background()
	builder := sender.NewSenderBuilderForTesting(sender.State_Observing)
	s, mocks := builder.Build(ctx)

	err := s.HandleEvent(ctx, &sender.HeartbeatReceivedEvent{})
	assert.NoError(t, err)

	mocks.Clock.Advance(builder.GetCoordinatorHeartbeatThresholdMs() + 1)

	err = s.HandleEvent(ctx, &sender.HeartbeatIntervalEvent{})
	assert.NoError(t, err)
	assert.Equal(t, sender.State_Idle, s.GetCurrentState(), "current state is %s", s.GetCurrentState().String())
}

func TestStateMachine_Observing_NoTransition_OnHeartbeatInterval_IfHeartbeatThresholdNotExpired(t *testing.T) {
	ctx := context.Background()
	builder := sender.NewSenderBuilderForTesting(sender.State_Observing)
	s, mocks := builder.Build(ctx)
	err := s.HandleEvent(ctx, &sender.HeartbeatReceivedEvent{})
	assert.NoError(t, err)

	mocks.Clock.Advance(builder.GetCoordinatorHeartbeatThresholdMs() - 1)

	err = s.HandleEvent(ctx, &sender.HeartbeatIntervalEvent{})
	assert.NoError(t, err)
	assert.Equal(t, sender.State_Observing, s.GetCurrentState(), "current state is %s", s.GetCurrentState().String())
}
