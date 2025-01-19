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

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func NewTransactionPoolForUnitTesting(_ *testing.T) *transactionPool {
	return &transactionPool{
		transactionsBySender: make(map[string][]*pooledTransaction),
	}
}

func TestPool_GetTransactionsForSender(t *testing.T) {
	ctx := context.Background()
	pool := NewTransactionPoolForUnitTesting(t)
	sender1NodeName := "sender1"
	sender2NodeName := "sender2"
	sender3NodeName := "sender3"

	txn1A := &pooledTransaction{}
	txn1A.sender = sender1NodeName

	txn1B := &pooledTransaction{}
	txn1B.sender = sender1NodeName

	txn2A := &pooledTransaction{}
	txn2A.sender = sender2NodeName

	pool.AddTransaction(ctx, txn1A)
	pool.AddTransaction(ctx, txn1B)
	pool.AddTransaction(ctx, txn2A)

	pooledTransactionsForSender1, err := pool.GetTransactionsForSender(ctx, sender1NodeName)
	assert.NoError(t, err)
	assert.Len(t, pooledTransactionsForSender1, 2)

	pooledTransactionsForSender2, err := pool.GetTransactionsForSender(ctx, sender2NodeName)
	assert.NoError(t, err)
	assert.Len(t, pooledTransactionsForSender2, 1)

	pooledTransactionsForSender3, err := pool.GetTransactionsForSender(ctx, sender3NodeName)
	assert.NoError(t, err)
	assert.Len(t, pooledTransactionsForSender3, 0)

}
