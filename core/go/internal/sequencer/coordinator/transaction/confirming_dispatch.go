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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/common"
	"github.com/google/uuid"
)

func (t *Transaction) applyDispatchConfirmation(_ context.Context, requestID uuid.UUID) error {
	t.pendingPreDispatchRequest = nil
	return nil
}

func (t *Transaction) sendPreDispatchRequest(ctx context.Context) error {

	log.L(ctx).Debugf("[Sequencer] Sending dispatch confirmation request for transaction %s", t.ID)
	log.L(ctx).Debugf("[Sequencer] Do we have endorsements or signatures?")
	log.L(ctx).Debugf("[Sequencer] %d endorsements", len(t.PostAssembly.Endorsements))
	log.L(ctx).Debugf("[Sequencer] %d signatures", len(t.PostAssembly.Signatures))

	if t.pendingPreDispatchRequest == nil {
		hash, err := t.Hash(ctx)
		if err != nil {
			log.L(ctx).Debugf("[Sequencer] Error hashing transaction for dispatch confirmation request: %s", err)
			return err
		}
		log.L(ctx).Debugf("[Sequencer] Creating idempotent request for dispatch confirmation request")
		t.pendingPreDispatchRequest = common.NewIdempotentRequest(ctx, t.clock, t.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {

			log.L(ctx).Debugf("[Sequencer] Calling SendDispatchConfirmationRequest")
			return t.transportWriter.SendPreDispatchRequest(
				ctx,
				t.sender,
				idempotencyKey,
				t.PreAssembly.TransactionSpecification,
				hash,
			)
		})
		t.cancelDispatchConfirmationRequestTimeoutSchedule = t.clock.ScheduleInterval(ctx, t.requestTimeout, func() {
			t.emit(&RequestTimeoutIntervalEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{
					TransactionID: t.ID,
				},
			})
		})
	}

	sendErr := t.pendingPreDispatchRequest.Nudge(ctx)

	// MRW TODO - we are the ones doing the dispatching, so after we've informed the sender we can just update our own state?
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
