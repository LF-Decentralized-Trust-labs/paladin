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
	txn, err := c.transactionSelector.SelectNextTransaction(ctx)
	if txn == nil || err != nil {
		return err
	}

	transactionSelectedEvent := &transaction.SelectedEvent{}
	transactionSelectedEvent.TransactionID = txn.ID
	err = txn.HandleEvent(ctx, transactionSelectedEvent)
	return err

}

/* Functions of the TransactionPool interface required by the transactionSelector */
func (c *coordinator) GetPooledTransactionsBySenderNodeAndIdentity(ctx context.Context) map[string]map[string]*transaction.Transaction {
	pooledTransactions := c.getTransactionsInStates(ctx, []transaction.State{transaction.State_Pooled})
	transactionsBySenderNodeAndIdentity := make(map[string]map[string]*transaction.Transaction)
	for _, txn := range pooledTransactions {

		if _, ok := transactionsBySenderNodeAndIdentity[txn.SenderNode()]; !ok {
			transactionsBySenderNodeAndIdentity[txn.SenderNode()] = make(map[string]*transaction.Transaction)
		}
		transactionsBySenderNodeAndIdentity[txn.SenderNode()][txn.SenderIdentity()] = txn
	}
	return transactionsBySenderNodeAndIdentity
}

func (c *coordinator) GetCommittee(ctx context.Context) map[string][]string {
	return c.committee
}

// define the interface for the transaction selector algorithm
// we inject into the algorithm a dependency that allows it to query the state of the transaction pool under the control of the coordinator
type TransactionSelector interface {
	SelectNextTransaction(ctx context.Context) (*transaction.Transaction, error)
}

// define the interface that the transaction selector algorithm uses to query the state of the transaction pool
type TransactionPool interface {
	//return a list of all transactions in that are in the State_Pooled state, keyed by the sender node and identifier
	GetPooledTransactionsBySenderNodeAndIdentity(ctx context.Context) map[string]map[string]*transaction.Transaction

	//return a list of all members of the committee organized by their node identity
	GetCommittee(ctx context.Context) map[string][]string
}

type transactionSelector struct {
	transactionPool TransactionPool
	nextNodeIndex   int
	nextSenderIndex int
}

func NewTransactionSelector(ctx context.Context, transactionPool TransactionPool) TransactionSelector {

	return &transactionSelector{
		nextNodeIndex:   0,
		nextSenderIndex: 0,
		transactionPool: transactionPool,
	}
}

func (ts *transactionSelector) SelectNextTransaction(ctx context.Context) (*transaction.Transaction, error) {

	//Super simple algorithm for fair (across senders) selection algorithm that biases against transaction from a sender that is unresponsive or has been tending to assemble transactions that are not getting endorsed
	//Transactions get added to the queue when they enter the State_Pooled state
	// given that only one transaction per sender can enter the State_Pooled state at a time, this gives us some natural fairness

	//There is a fast queue and a slow queue
	// when a transaction fails to assemble due to sender timeout, or if it fails endorsement, the error count is incremented and it is placed at the end of the slow queue

	//NOTE This algorithm currently only biases against the current transaction for each faulty sender.  As soon as that transaction is successful, the next transaction from the same sender is treated just like any other transaction.

	selectableTransactionsMap := ts.transactionPool.GetPooledTransactionsBySenderNodeAndIdentity(ctx)
	selectableTransactions := make([]*transaction.Transaction, 0)
	for _, transactions := range selectableTransactionsMap {
		for _, transaction := range transactions {
			selectableTransactions = append(selectableTransactions, transaction)
		}
	}

	if len(selectableTransactions) > 0 {
		selectedTransaction := selectableTransactions[0]

		return selectedTransaction, nil
	}
	return nil, nil
}
