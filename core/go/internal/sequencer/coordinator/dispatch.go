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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
)

func (c *coordinator) GetTransactionsReadyToDispatch(ctx context.Context) ([]*components.PrivateTransaction, error) {
	// Get the next transactions that are ready to be dispatched
	// This is the transactions that are in the State_Ready_For_Dispatch state
	// If there are no transactions in that state, return nil
	transactions := c.getTransactionsInStates(ctx, []transaction.State{transaction.State_Ready_For_Dispatch})
	if len(transactions) == 0 {
		return nil, nil
	}

	sortedTransactions, err := transaction.SortTransactions(ctx, transactions)
	if err != nil {
		log.L(ctx).Errorf("failed to sort transactions: %v", err)
		return nil, err
	}

	privateTransactions := make([]*components.PrivateTransaction, 0, len(sortedTransactions))
	for _, txn := range sortedTransactions {
		privateTransactions = append(privateTransactions, txn.PrivateTransaction)

	}

	return privateTransactions, nil
}
