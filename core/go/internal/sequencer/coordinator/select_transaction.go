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

package coordinator

import (
	"context"

	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator/transaction"
)

func (c *coordinator) selectNextTransaction(ctx context.Context) error {
	selectableTransactions := c.getTransactionsInStates(ctx, []transaction.State{transaction.State_Pooled})

	//Super simple algorithm for fair (across senders) selection algorithm that biases against transaction from a sender that is unresponsive or has been tending to assemble transactions that are not getting endorsed
	//Transactions get added to the queue when they enter the State_Pooled state
	// given that only one transaction per sender can enter the State_Pooled state at a time, this gives us some natural fairness

	//There is a fast queue and a slow queue
	// when a transaction fails to assemble due to sender timeout, or if it fails endorsement, the error count is incremented and it is placed at the end of the slow queue

	//NOTE This algorithm currently only biases against the current transaction for each faulty sender.  As soon as that transaction is successful, the next transaction from the same sender is treated just like any other transaction.

	if len(selectableTransactions) > 0 {
		selectedTransaction := selectableTransactions[0]
		transactionSelectedEvent := &transaction.SelectedEvent{}
		transactionSelectedEvent.TransactionID = selectedTransaction.ID

		err := selectedTransaction.HandleEvent(ctx, transactionSelectedEvent)
		return err
	}
	return nil

}
