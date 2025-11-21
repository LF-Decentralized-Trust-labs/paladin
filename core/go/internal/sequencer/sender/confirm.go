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
	"fmt"

	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/sequencer/sender/transaction"
	"github.com/kaleido-io/paladin/toolkit/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

func (s *sender) confirmTransaction(
	ctx context.Context,
	From *tktypes.EthAddress,
	nonce uint64,
	hash tktypes.Bytes32,
	revertReason tktypes.HexBytes,
) error {
	transactionID, ok := s.submittedTransactionsByHash[hash]
	if !ok {

		//assumed to be a transaction from another sender
		//TODO: should we keep track of this just in case we become the active coordinator soon and don't get a clean handover?
		//
		// TODO think about where the following comment should go.  THere is a lot of rationale documented here that is relevant to other parts of the code

		// Another explanation for this is that it is one of our transactions but we see the confirmation event before
		//  we see the coordinator's heartbeat telling us that it has been submitted with the given hash.
		// In normal operation, we will eventually see a coordinator heartbeat telling us that the transaction has been confirmed
		// In abnormal operation ( coordinator goes down or network partition ) then we might end up sending the transaction to the new coordinator.
		// Eventually, if the transaction submission from the old coordinator or the new, then we will see a blockchain event confirming transaction success
		//   and that event will have the transaction ID so even if we never see the submitted hash, we can break out of the retry loop.
		// In the meantime, it would be safe albeit possibly inefficient to delegate all unconfirmed transactions
		// to new coordinator on switchover.  Double intent protection in the base contract will ensure that we don't process the same transaction twice

		log.L(ctx).Debugf("Transaction %s not found in submitted transactions", hash)
		return nil
	}
	if transactionID == nil {
		//This should never happen and if it does, we can no longer trust any of the data structures we have in memory
		// for this sequencer instance so return an error to trigger an abend of the sequencer instance
		msg := fmt.Sprintf("Transaction %s found in submitted transactions but nil transaction ID", hash)
		log.L(ctx).Error(msg)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
	}
	txn, ok := s.transactionsByID[*transactionID]
	if txn == nil || !ok {
		//This should never happen and if it does, we can no longer trust any of the data structures we have in memory
		// for this sequencer instance so return an error to trigger an abend of the sequencer instance
		msg := fmt.Sprintf("Transaction %s found in submitted transactions but nil transaction", hash)
		log.L(ctx).Error(msg)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
	}
	if revertReason.String() == "" {
		txn.HandleEvent(ctx, &transaction.ConfirmedSuccessEvent{
			BaseEvent: transaction.BaseEvent{
				TransactionID: txn.ID,
			},
		})
	} else {
		txn.HandleEvent(ctx, &transaction.ConfirmedRevertedEvent{
			BaseEvent: transaction.BaseEvent{
				TransactionID: txn.ID,
			},
			RevertReason: revertReason,
		})
	}

	delete(s.submittedTransactionsByHash, hash)
	return nil

}
func guard_HasUnconfirmedTransactions(ctx context.Context, s *sender) bool {
	return len(
		s.getTransactionsNotInStates(ctx, []transaction.State{transaction.State_Confirmed}),
	) > 0
}
