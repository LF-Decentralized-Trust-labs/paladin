/*
 * Copyright © 2026 Kaleido, Inc.
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
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/mocks/graphermocks"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_action_NudgePreDispatchRequest_NilPendingRequest_ReturnsError(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).Build()

	err := action_NudgePreDispatchRequest(ctx, txn, nil)
	require.ErrorContains(t, err, "nudgePreDispatchRequest called with no pending request")
}

func Test_action_NudgePreDispatchRequest_WithPendingRequest_Success(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		AddPendingPreDispatchRequest().
		Build()

	err := action_NudgePreDispatchRequest(ctx, txn, nil)
	require.NoError(t, err)
}

func Test_validator_MatchesPendingPreDispatchRequest_DispatchRequestApproved_Match(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		AddPendingPreDispatchRequest().
		Build()
	requestID := txn.pendingPreDispatchRequest.IdempotencyKey()

	event := &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            requestID,
	}

	matched, err := validator_MatchesPendingPreDispatchRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.True(t, matched)
}

func Test_validator_MatchesPendingPreDispatchRequest_DispatchRequestApproved_NoMatch_WrongRequestID(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		AddPendingPreDispatchRequest().
		Build()

	event := &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            uuid.New(), // different from pending request
	}

	matched, err := validator_MatchesPendingPreDispatchRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, matched)
}

func Test_validator_MatchesPendingPreDispatchRequest_DispatchRequestApproved_NilPendingRequest(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).Build()

	event := &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
		RequestID:            uuid.New(),
	}

	matched, err := validator_MatchesPendingPreDispatchRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, matched)
}

func Test_validator_MatchesPendingPreDispatchRequest_OtherEventType_ReturnsFalse(t *testing.T) {
	ctx := t.Context()
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		AddPendingPreDispatchRequest().
		Build()

	event := &ConfirmedSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{TransactionID: txn.pt.ID},
	}

	matched, err := validator_MatchesPendingPreDispatchRequest(ctx, txn, event)
	require.NoError(t, err)
	assert.False(t, matched)
}

func Test_action_DispatchRequestRejected_ClearsPendingRequestAndTimers(t *testing.T) {
	ctx := t.Context()
	var cancelRequest, cancelStateTimeout bool
	txn, _ := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		AddPendingPreDispatchRequestWithCallback(func(context.Context, uuid.UUID) error { return nil }).
		CancelRequestTimeoutSchedule(func() {
			cancelRequest = true
		}).
		CancelStateTimeoutSchedule(func() {
			cancelStateTimeout = true
		}).
		Build()

	err := action_DispatchRequestRejected(ctx, txn, nil)
	require.NoError(t, err)
	assert.Nil(t, txn.pendingPreDispatchRequest)
	assert.Nil(t, txn.cancelRequestTimeoutSchedule)
	assert.Nil(t, txn.cancelStateTimeoutSchedule)
	assert.True(t, cancelRequest)
	assert.True(t, cancelStateTimeout)
}

func Test_ConfirmingDispatch_Timeout_TransitionsToPooled_AndClearsPendingRequest(t *testing.T) {
	ctx := t.Context()
	mockGrapher := graphermocks.NewGrapher(t)
	builder := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		AddPendingPreDispatchRequest().Grapher(mockGrapher)
	txn, _ := builder.Build()
	require.NotNil(t, txn.pendingPreDispatchRequest)
	mockGrapher.EXPECT().ForgetTransactionAndLocks(mock.Anything, txn.pt.ID)

	err := txn.HandleEvent(ctx, &StateTimeoutIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txn.pt.ID,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, State_Pooled, txn.stateMachine.GetCurrentState())
	assert.Nil(t, txn.pendingPreDispatchRequest)
}

func Test_sendPreDispatchRequest_RequestTimeoutSchedulesTimer_QueueEventCalled(t *testing.T) {
	ctx := t.Context()
	timeoutEventReceived := false

	txn, mocks := NewTransactionBuilderForTesting(t, State_Confirming_Dispatchable).
		UseMockTransportWriter().
		UseMockClock().
		QueueEventForCoordinator(func(ctx context.Context, event common.Event) {
			if _, ok := event.(*RequestTimeoutIntervalEvent); ok {
				timeoutEventReceived = true
			}
		}).
		RequestTimeout(1).
		Build()

	mocks.TransportWriter.EXPECT().SendPreDispatchRequest(
		mock.Anything, txn.originatorNode, mock.MatchedBy(func(msg *engineProto.PreDispatchRequest) bool {
			return msg.TransactionId == txn.pt.ID.String()
		}),
	).Return(nil)

	mocks.Clock.On("Now").Return(time.Now()).Once()
	mocks.Clock.On("ScheduleTimer", mock.Anything, time.Duration(1), mock.Anything).Return(func() {}).Run(func(args mock.Arguments) {
		callback := args.Get(2).(func())
		callback()
	})

	err := txn.sendPreDispatchRequest(ctx)
	require.NoError(t, err)

	assert.True(t, timeoutEventReceived, "queueEventForCoordinator should have been called with RequestTimeoutIntervalEvent")
}
