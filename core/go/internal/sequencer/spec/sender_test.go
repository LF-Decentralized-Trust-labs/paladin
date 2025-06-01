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

	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/sender"
	"github.com/kaleido-io/paladin/core/internal/sequencer/sender/transaction"
	"github.com/kaleido-io/paladin/core/internal/sequencer/testutil"
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
	builder := sender.NewSenderBuilderForTesting(sender.State_Idle)
	s, mocks := builder.Build(ctx)
	assert.Equal(t, sender.State_Idle, s.GetCurrentState())

	txn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Sender("sender@node1").Build()
	err := s.HandleEvent(ctx, &sender.TransactionCreatedEvent{
		Transaction: txn,
	})
	assert.NoError(t, err)
	assert.Equal(t, sender.State_Sending, s.GetCurrentState(), "current state is %s", s.GetCurrentState().String())

	assert.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())
}

func TestStateMachine_Observing_ToSending_OnTransactionCreated(t *testing.T) {
	ctx := context.Background()
	builder := sender.NewSenderBuilderForTesting(sender.State_Observing)
	s, mocks := builder.Build(ctx)

	txn := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Sender("sender@node1").Build()
	err := s.HandleEvent(ctx, &sender.TransactionCreatedEvent{
		Transaction: txn,
	})
	assert.NoError(t, err)
	assert.Equal(t, sender.State_Sending, s.GetCurrentState(), "current state is %s", s.GetCurrentState().String())

	assert.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())
}

func TestStateMachine_Sending_ToObserving_OnTransactionConfirmed_IfNoTransactionsInflight(t *testing.T) {
	ctx := context.Background()

	soleTransaction := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	s, _ := sender.NewSenderBuilderForTesting(sender.State_Sending).
		Transactions(soleTransaction).
		Build(ctx)

	err := s.HandleEvent(ctx, &sender.TransactionConfirmedEvent{
		From:  soleTransaction.GetSignerAddress(),
		Nonce: *soleTransaction.GetNonce(),
		Hash:  *soleTransaction.GetLatestSubmissionHash(),
	})
	assert.NoError(t, err)
	assert.Equal(t, sender.State_Observing, s.GetCurrentState(), "current state is %s", s.GetCurrentState().String())
}

func TestStateMachine_Sending_NoTransition_OnTransactionConfirmed_IfHasTransactionsInflight(t *testing.T) {
	ctx := context.Background()
	txn1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	txn2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	s, _ := sender.NewSenderBuilderForTesting(sender.State_Sending).
		Transactions(txn1, txn2).
		Build(ctx)

	err := s.HandleEvent(ctx, &sender.TransactionConfirmedEvent{
		From:  txn1.GetSignerAddress(),
		Nonce: *txn1.GetNonce(),
		Hash:  *txn1.GetLatestSubmissionHash(),
	})
	assert.NoError(t, err)
	assert.Equal(t, sender.State_Sending, s.GetCurrentState(), "current state is %s", s.GetCurrentState().String())
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

func TestStateMachine_Sending_DoDelegateTransactions_OnHeartbeatReceived_IfHasDroppedTransaction(t *testing.T) {
	ctx := context.Background()
	coordinatorLocator := "coordinator@node1"

	builder := sender.NewSenderBuilderForTesting(sender.State_Sending)
	s, mocks := builder.Build(ctx)

	txn1 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Sender("sender@node1").Build()
	err := s.HandleEvent(ctx, &sender.TransactionCreatedEvent{
		Transaction: txn1,
	})
	assert.NoError(t, err)

	txn2 := testutil.NewPrivateTransactionBuilderForTesting().Address(builder.GetContractAddress()).Sender("sender@node1").Build()
	err = s.HandleEvent(ctx, &sender.TransactionCreatedEvent{
		Transaction: txn2,
	})
	assert.NoError(t, err)

	mocks.SentMessageRecorder.Reset(ctx)

	// Only one of the delegated transactions are included in the heartbeat
	heartbeatEvent := &sender.HeartbeatReceivedEvent{}
	heartbeatEvent.From = coordinatorLocator
	heartbeatEvent.PooledTransactions = []*common.Transaction{
		{
			ID:     txn1.ID,
			Sender: "sender@node1",
		},
	}

	err = s.HandleEvent(ctx, heartbeatEvent)
	assert.NoError(t, err)
	assert.True(t, mocks.SentMessageRecorder.HasSentDelegationRequest())
}
