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

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/sender/transaction"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

func action_SendDelegationRequest(ctx context.Context, s *sender) error {
	transactions, err := s.transactionsOrderedByCreatedTime(ctx)
	if err != nil {
		log.L(ctx).Errorf("Failed to get transactions ordered by created time: %v", err)
		return err
	}
	privateTransactions := make([]*components.PrivateTransaction, len(transactions), len(transactions))
	for i, txn := range transactions {
		privateTransactions[i] = txn.PrivateTransaction
	}
	s.messageSender.SendDelegationRequest(ctx, s.activeCoordinator, privateTransactions, s.currentBlockHeight)
	for _, txn := range transactions {
		txn.HandleEvent(ctx, &transaction.DelegatedEvent{
			BaseEvent: transaction.BaseEvent{
				TransactionID: txn.ID,
			},
			Coordinator: s.activeCoordinator,
		})
	}
	return nil

}

func guard_HasDroppedTransactions(ctx context.Context, s *sender) bool {
	//are there any transactions that the current active coordinator seems to have dropped ( as per its latest heartbeat)
	//NOTE: "dropped" is not a state in the transaction state machine, but rather a state in the sender's view of the world.
	// Reason for this is that it is not really a state of the transaction, it is a property of the heartbeat event and as such,
	// is reconciled as part of handling that event so immediately, the transaction is in Delegated state again
	for _, txn := range s.getTransactionsInStates(ctx, []transaction.State{transaction.State_Delegated}) {
		dropped := true
		for _, dispatchedTransaction := range s.latestCoordinatorSnapshot.PooledTransactions {
			if dispatchedTransaction.ID == txn.ID {
				dropped = false
				break
			}
		}
		if dropped {
			log.L(ctx).Debugf("Transaction %s is in Delegated state but not found in latest coordinator snapshot, assuming dropped", txn.ID)
			return true
		}
	}
	return false
}
