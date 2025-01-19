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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type ConfirmedTransaction struct {
	TransactionID uuid.UUID
	Hash          tktypes.Bytes32
	RevertReason  tktypes.HexBytes
}

type ConfirmedTransactionIndex struct {
	TransactionsByID map[uuid.UUID]*ConfirmedTransaction
}

func NewConfirmedTransactionIndex(_ context.Context) *ConfirmedTransactionIndex {
	return &ConfirmedTransactionIndex{
		TransactionsByID: make(map[uuid.UUID]*ConfirmedTransaction),
	}
}

func (c *ConfirmedTransactionIndex) add(transaction *ConfirmedTransaction) {
	// Add the transaction to the list
	c.TransactionsByID[transaction.TransactionID] = transaction
}

func (c *ConfirmedTransactionIndex) remove(transaction *ConfirmedTransaction) {
	// Remove the transaction from the list
	delete(c.TransactionsByID, transaction.TransactionID)
}
