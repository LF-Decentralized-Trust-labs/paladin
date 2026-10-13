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
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/google/uuid"
)

func (t *coordinatorTransaction) completePreDispatchRequest(_ context.Context) error {
	t.pendingPreDispatchRequest = nil
	t.clearTimeoutSchedules()
	return nil
}

func (t *coordinatorTransaction) sendPreDispatchRequest(ctx context.Context) error {

	if t.pendingPreDispatchRequest == nil {
		t.pendingPreDispatchRequest = common.NewIdempotentRequest(ctx, t.clock, t.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
			return t.transportWriter.SendPreDispatchRequest(ctx, t.originatorNode, &engineProto.PreDispatchRequest{
				Id:              idempotencyKey.String(),
				TransactionId:   t.pt.ID.String(),
				ContractAddress: t.pt.Address.HexString(),
			})
		})
		t.scheduleRequestTimeout(ctx)
	}

	sendErr := t.pendingPreDispatchRequest.Nudge(ctx)

	return sendErr

}

func (t *coordinatorTransaction) nudgePreDispatchRequest(ctx context.Context) error {
	if t.pendingPreDispatchRequest == nil {
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "nudgePreDispatchRequest called with no pending request")
	}

	return t.pendingPreDispatchRequest.Nudge(ctx)
}

func validator_MatchesPendingPreDispatchRequest(ctx context.Context, txn *coordinatorTransaction, event common.Event) (bool, error) {
	switch event := event.(type) {
	case *DispatchRequestApprovedEvent:
		return txn.pendingPreDispatchRequest != nil && txn.pendingPreDispatchRequest.IdempotencyKey() == event.RequestID, nil
	}
	return false, nil
}

func action_DispatchRequestApproved(ctx context.Context, t *coordinatorTransaction, _ common.Event) error {
	return t.completePreDispatchRequest(ctx)
}

func action_DispatchRequestRejected(ctx context.Context, t *coordinatorTransaction, _ common.Event) error {
	return t.completePreDispatchRequest(ctx)
}

func action_SendPreDispatchRequest(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	return txn.sendPreDispatchRequest(ctx)
}

func action_NudgePreDispatchRequest(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	return txn.nudgePreDispatchRequest(ctx)
}

func validator_IsPreDispatchNotCurrentDelegateRejection(_ context.Context, _ *coordinatorTransaction, event common.Event) (bool, error) {
	return event.(*PreDispatchRequestRejectedEvent).RejectionReason == engineProto.RejectionReason_NOT_CURRENT_DELEGATE, nil
}

func validator_IsPreDispatchTransactionUnknownRejection(_ context.Context, _ *coordinatorTransaction, event common.Event) (bool, error) {
	return event.(*PreDispatchRequestRejectedEvent).RejectionReason == engineProto.RejectionReason_TRANSACTION_UNKNOWN, nil
}
