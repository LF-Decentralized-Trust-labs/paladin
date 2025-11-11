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
	"fmt"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

// Interface Grapher allows transactions to link to each other in a dependency graph
// Transactions may know about their dependencies either explicitly via transactions IDs specified on the pre-assembly spec
// or implicitly via the post assembly input and read state IDs .
// In the former case, the Grapher helps to resolve a transaction ID to a pointer to the in-memory state machine for that transaction object
// In the latter case the Grapher helps to resolve a state ID to a pointer to the in-memory state machine for the transaction object that produced that state
// Transactions register themselves with the Grapher and can use the Grapher to look up each other
// The Grapher is not a graph data structure, but a simple index of transactions by ID and by state ID
// the actual graph is the emergent data structure of the transactions maintaining links to each other
type Grapher interface {
	Add(context.Context, *Transaction)
	TransactionByID(ctx context.Context, transactionID uuid.UUID) *Transaction
	LookupMinter(ctx context.Context, stateID tktypes.HexBytes) (*Transaction, error)
	AddMinter(ctx context.Context, stateID tktypes.HexBytes, transaction *Transaction) error
	Forget(transactionID uuid.UUID) error
}

type grapher struct {
	transactionByOutputState map[string]*Transaction
	transactionByID          map[uuid.UUID]*Transaction
	outputStatesByMinter     map[uuid.UUID][]string //used for reverse lookup to cleanup transactionByOutputState
}

func NewGrapher(ctx context.Context) Grapher {
	return &grapher{
		transactionByOutputState: make(map[string]*Transaction),
		transactionByID:          make(map[uuid.UUID]*Transaction),
		outputStatesByMinter:     make(map[uuid.UUID][]string),
	}
}

func (s *grapher) Add(ctx context.Context, txn *Transaction) {
	s.transactionByID[txn.ID] = txn
}

func (s *grapher) LookupMinter(ctx context.Context, stateID tktypes.HexBytes) (*Transaction, error) {
	return s.transactionByOutputState[stateID.String()], nil
}

func (s *grapher) AddMinter(ctx context.Context, stateID tktypes.HexBytes, transaction *Transaction) error {
	if txn, ok := s.transactionByOutputState[stateID.String()]; ok {
		msg := fmt.Sprintf("Duplicate minter. stateID %s already indexed as minted by %s but attempted to add minter %s", stateID.String(), txn.ID.String(), transaction.ID.String())
		log.L(ctx).Errorf(msg)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
	}
	s.transactionByOutputState[stateID.String()] = transaction
	s.outputStatesByMinter[transaction.ID] = append(s.outputStatesByMinter[transaction.ID], stateID.String())
	return nil
}

func (s *grapher) Forget(transactionID uuid.UUID) error {
	if outputStates, ok := s.outputStatesByMinter[transactionID]; ok {
		for _, stateID := range outputStates {
			delete(s.transactionByOutputState, stateID)
		}
		delete(s.outputStatesByMinter, transactionID)
	}
	delete(s.transactionByID, transactionID)
	return nil
}

func (s *grapher) TransactionByID(ctx context.Context, transactionID uuid.UUID) *Transaction {
	return s.transactionByID[transactionID]
}

// SortTransactions sorts the transactions based on their dependencies.
// It ensures that transactions are sequenced in such a way that all dependencies are resolved before the transaction itself is processed.
// It returns an error if a circular dependency is detected or if any transaction has dependencies that are not in the input list.
// This function is used to ensure that transactions are processed in the correct order, respecting their dependencies.
// It assumes that the transactions are provided in a state where they are ready to be sequenced.
func SortTransactions(ctx context.Context, transactions []*Transaction) ([]*Transaction, error) {
	sortedTransactions := make([]*Transaction, 0, len(transactions))
	// Ensure the returned array is sorted according to the dependency graph

	// continue to loop through all transactions picking off any who have no dependencies
	//  other than transactions that have already been sequenced
	for len(transactions) > 0 {

		//assume we don't find any transaction with no dependencies in this iteration
		found := false

		// Find the next transaction that has no dependencies
		for i, txn := range transactions {
			// If the transaction has no dependencies, we can sequence it
			if !txn.hasDependenciesNotIn(ctx, sortedTransactions) {
				// Add the transaction to the sequenced transactions
				sortedTransactions = append(sortedTransactions, txn)

				// Remove the transaction from the transactions array
				transactions = append(transactions[:i], transactions[i+1:]...)
				found = true
				break // Restart the loop to check for more transactions
			}
		}
		if !found {
			// If we didn't find any transaction with no dependencies, it means we have a circular dependency
			// or some transactions are not in the input list, which should not happen in normal usage
			msg := "Circular dependency detected or some transactions are not in the input list"
			log.L(ctx).Errorf(msg)
			return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
	}
	return sortedTransactions, nil
}
