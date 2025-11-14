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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/google/uuid"
)

func (t *Transaction) applyDispatchConfirmation(_ context.Context, requestID uuid.UUID) error {
	t.pendingPreDispatchRequest = nil
	return nil
}

func (t *Transaction) sendPreDispatchRequest(ctx context.Context) error {

	if t.pendingPreDispatchRequest == nil {
		hash, err := t.Hash(ctx)
		if err != nil {
			log.L(ctx).Debugf("error hashing transaction for dispatch confirmation request: %s", err)
			return err
		}
		t.pendingPreDispatchRequest = common.NewIdempotentRequest(ctx, t.clock, t.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
			return t.transportWriter.SendPreDispatchRequest(
				ctx,
				t.originatorNode,
				idempotencyKey,
				t.PreAssembly.TransactionSpecification,
				hash,
			)
		})
		t.cancelDispatchConfirmationRequestTimeoutSchedule = t.clock.ScheduleInterval(ctx, t.requestTimeout, func() {
			err := t.eventHandler(ctx, &RequestTimeoutIntervalEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{
					TransactionID: t.ID,
				},
			})
			if err != nil {
				log.L(ctx).Errorf("error handling RequestTimeoutIntervalEvent: %s", err)
				return
			}
		})
	}

	sendErr := t.pendingPreDispatchRequest.Nudge(ctx)

	// MRW TODO - we are the ones doing the dispatching, so after we've informed the originator we can just update our own state?
	// t.HandleEvent(ctx, &DispatchConfirmedEvent{
	// 	BaseCoordinatorEvent: BaseCoordinatorEvent{
	// 		TransactionID: t.ID,
	// 	},
	// 	RequestID: t.pendingDispatchConfirmationRequest.IdempotencyKey(),
	// })

	return sendErr

}
func (t *Transaction) nudgePreDispatchRequest(ctx context.Context) error {
	if t.pendingPreDispatchRequest == nil {
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "nudgePreDispatchRequest called with no pending request")
	}

	return t.pendingPreDispatchRequest.Nudge(ctx)
}

func validator_MatchesPendingPreDispatchRequest(ctx context.Context, txn *Transaction, event common.Event) (bool, error) {
	switch event := event.(type) {
	case *DispatchRequestApprovedEvent:
		return txn.pendingPreDispatchRequest != nil && txn.pendingPreDispatchRequest.IdempotencyKey() == event.RequestID, nil
	}
	return false, nil
}

func action_SendPreDispatchRequest(ctx context.Context, txn *Transaction) error {
	return txn.sendPreDispatchRequest(ctx)
}

func action_NudgePreDispatchRequest(ctx context.Context, txn *Transaction) error {
	return txn.nudgePreDispatchRequest(ctx)
}
