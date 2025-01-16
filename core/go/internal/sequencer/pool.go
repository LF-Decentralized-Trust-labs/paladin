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

package sequencer

import "context"

type TransactionPool interface {
	AddTransaction(context.Context, *PooledTransaction) error
	GetTransactionsForSender(context.Context, string) ([]*PooledTransaction, error)
}

type transactionPool struct {
	transactionsBySender map[string][]*PooledTransaction
}

func NewTransactionPool(_ context.Context) TransactionPool {
	return &transactionPool{
		transactionsBySender: make(map[string][]*PooledTransaction),
	}
}

func (tp *transactionPool) GetTransactionsForSender(ctx context.Context, sender string) ([]*PooledTransaction, error) {
	return tp.transactionsBySender[sender], nil

}

func (tp *transactionPool) AddTransaction(ctx context.Context, transaction *PooledTransaction) error {
	if tp.transactionsBySender[transaction.Sender] == nil {
		tp.transactionsBySender[transaction.Sender] = make([]*PooledTransaction, 0)
	}
	tp.transactionsBySender[transaction.Sender] = append(tp.transactionsBySender[transaction.Sender], transaction)
	return nil
}
