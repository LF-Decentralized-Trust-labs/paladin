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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/toolkit/pkg/i18n"
)

func (t *Transaction) applyDispatchConfirmation(_ context.Context, requestID uuid.UUID) error {
	t.pendingDispatchConfirmationRequest = nil
	return nil
}

func (t *Transaction) sendDispatchConfirmationRequest(ctx context.Context) error {

	if t.pendingDispatchConfirmationRequest == nil {
		hash, err := t.Hash(ctx)
		if err != nil {
			return err
		}
		t.pendingDispatchConfirmationRequest = common.NewIdempotentRequest(ctx, t.clock, t.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {

			return t.messageSender.SendDispatchConfirmationRequest(
				ctx,
				t.sender,
				idempotencyKey,
				t.PreAssembly.TransactionSpecification,
				hash,
			)
		})
		t.cancelDispatchConfirmationRequestTimeoutSchedule = t.clock.ScheduleInterval(ctx, t.requestTimeout, func() {
			t.emit(&RequestTimeoutIntervalEvent{
				BaseEvent: BaseEvent{
					TransactionID: t.ID,
				},
			})
		})
	}

	return t.pendingDispatchConfirmationRequest.Nudge(ctx)

}
func (t *Transaction) nudgeDispatchConfirmationRequest(ctx context.Context) error {
	if t.pendingDispatchConfirmationRequest == nil {
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "nudgeDispatchConfirmationRequest called with no pending request")
	}

	return t.pendingDispatchConfirmationRequest.Nudge(ctx)
}

func validator_MatchesPendingDispatchConfirmationRequest(ctx context.Context, txn *Transaction, event common.Event) (bool, error) {
	switch event := event.(type) {
	case *DispatchConfirmedEvent:
		return txn.pendingDispatchConfirmationRequest != nil && txn.pendingDispatchConfirmationRequest.IdempotencyKey() == event.RequestID, nil
	}
	return false, nil
}

func action_SendDispatchConfirmationRequest(ctx context.Context, txn *Transaction) error {
	return txn.sendDispatchConfirmationRequest(ctx)
}

func action_NudgeDispatchConfirmationRequest(ctx context.Context, txn *Transaction) error {
	return txn.nudgeDispatchConfirmationRequest(ctx)
}
