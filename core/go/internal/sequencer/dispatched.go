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
	"fmt"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type DispatchedTransaction struct {
	TransactionID        uuid.UUID
	Signer               tktypes.EthAddress
	LatestSubmissionHash *tktypes.Bytes32
	Nonce                *uint64
}

type DispatchedTransactionIndex struct {
	TransactionsByID          map[uuid.UUID]*DispatchedTransaction
	transactionsBySignerNonce map[string]*DispatchedTransaction
}

func NewDispatchedTransactionIndex(_ context.Context) *DispatchedTransactionIndex {
	return &DispatchedTransactionIndex{
		TransactionsByID:          make(map[uuid.UUID]*DispatchedTransaction),
		transactionsBySignerNonce: make(map[string]*DispatchedTransaction),
	}
}

func (d *DispatchedTransactionIndex) add(transaction *DispatchedTransaction) {
	// Add the transaction to the list
	d.TransactionsByID[transaction.TransactionID] = transaction
	if transaction.Nonce != nil {
		signerNonce := fmt.Sprintf("%s:%d", transaction.Signer.String(), *transaction.Nonce)
		d.transactionsBySignerNonce[signerNonce] = transaction
	}
}

func (d *DispatchedTransactionIndex) remove(transaction *DispatchedTransaction) {
	signerNonce := fmt.Sprintf("%s:%d", transaction.Signer.String(), transaction.Nonce)
	delete(d.transactionsBySignerNonce, signerNonce)
	delete(d.TransactionsByID, transaction.TransactionID)
}

func (d *DispatchedTransactionIndex) empty() bool {
	return len(d.TransactionsByID) == 0
}

func (d *DispatchedTransactionIndex) findBySignerNonce(signer *tktypes.EthAddress, nonce uint64) *DispatchedTransaction {
	signerNonce := fmt.Sprintf("%s:%d", signer.String(), nonce)
	transaction, ok := d.transactionsBySignerNonce[signerNonce]
	if !ok {
		return nil
	}
	return transaction
}
