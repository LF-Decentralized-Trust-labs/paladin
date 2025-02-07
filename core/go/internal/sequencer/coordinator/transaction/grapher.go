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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

// TODO is this helpful as an interface?
//Interface Grapher defines a dependency of transaction that provides methods to look up transactions for a given state ID
/*type Grapher interface {
	LookupMinter(ctx context.Context, stateID tktypes.HexBytes) (*Transaction, error)
}*/

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
	TransactionByID(ctx context.Context, transactionID uuid.UUID) (*Transaction, error)
	LookupMinter(ctx context.Context, stateID tktypes.HexBytes) (*Transaction, error)
	AddMinter(ctx context.Context, stateID tktypes.HexBytes, transaction *Transaction) error
	Forget(transactionID uuid.UUID) error
}

// TODO: decide whether to keep the implementation in here or move it to an upstream package e.g. as a shim around domain context
// in which case, this simple implementation could be moved to test_utils
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
	//TODO return error if duplicate
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

func (s *grapher) TransactionByID(ctx context.Context, transactionID uuid.UUID) (*Transaction, error) {
	return s.transactionByID[transactionID], nil
}
