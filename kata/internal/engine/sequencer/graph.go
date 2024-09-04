// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sequencer

import (
	"context"
	"fmt"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
)

type Graph interface {
	AddTransaction(ctx context.Context, txID string, inputStates []string, outputStates []string) error
	GetDispatchableTransactions(ctx context.Context) ([]string, error)
	RemoveTransactions(ctx context.Context, transactionsToRemove []string) error
	RecordEndorsement(ctx context.Context, txID string) error
	IncludesTransaction(txID string) bool
}

type graph struct {
	// This is the source of truth for all transaction
	allTransactions map[string]*transaction

	// all of the following are ephemeral and derived from allTransactions

	// implement graph of transactions as an adjacency matrix where the values in the matrix is an array of state hashes that connect those transactions
	// first dimension is the dependency ( i.e the minter of the state) and second dimension is the dependant (i.e. the consumer of the state)
	// and the third dimension is the array of state hashes that connect the two transactions
	//  this direction makes it easier to isolate a sequence of dispatchable transactions by doing a breadth first search starting at the layer of independant transactions
	transactionsMatrix [][][]string
	transactions       []*transaction
	//map of transaction id to index in the transactions array
	transactionIndex map[string]int
}

func NewGraph() Graph {
	return &graph{
		allTransactions: make(map[string]*transaction),
	}
}

func (g *graph) AddTransaction(ctx context.Context, txID string, inputStates []string, outputStates []string) error {
	g.allTransactions[txID] = &transaction{
		id:           txID,
		endorsed:     false,
		inputStates:  inputStates,
		outputStates: outputStates,
	}

	// TODO should probably cache this graph and only rebuild it when needed (e.g. on restart)
	// and incrementally update it when new transactions are added etc...
	err := g.buildMatrix(ctx)
	if err != nil {
		log.L(ctx).Errorf("Error building graph: %s", err)
		return err
	}
	return nil
}

func (g *graph) IncludesTransaction(txID string) bool {
	return g.allTransactions[txID] != nil
}

func (g *graph) RecordEndorsement(ctx context.Context, txID string) error {
	if g.allTransactions[txID] == nil {
		log.L(ctx).Errorf("Transaction %s does not exist", txID)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, fmt.Sprintf("Transaction %s does not exist", txID))
	}
	g.allTransactions[txID].endorsed = true
	return nil
}

func (g *graph) buildMatrix(ctx context.Context) error {
	g.transactionIndex = make(map[string]int)
	g.transactions = make([]*transaction, len(g.allTransactions))
	currentIndex := 0
	for txnId, txn := range g.allTransactions {
		g.transactionIndex[txnId] = currentIndex
		g.transactions[currentIndex] = txn

		currentIndex++

	}
	//for each unique state hash, create an index of its minter and/or spender
	stateToSpender := make(map[string]*int)
	for txnIndex, txn := range g.transactions {
		for _, stateID := range txn.inputStates {
			if stateToSpender[stateID] != nil {
				//TODO this is expected in some cases and represents a contention that needs to be resolved
				//TBC do we assert that it is resovled before we get to this point?
				log.L(ctx).Errorf("State hash %s is spent by multiple transactions", stateID)
				return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "State hash %s is spent by multiple transactions")
			}
			stateToSpender[stateID] = confutil.P(txnIndex)
		}
	}

	//now we can build the adjacency matrix
	g.transactionsMatrix = make([][][]string, len(g.transactions))
	for minterIndex, minter := range g.transactions {
		g.transactionsMatrix[minterIndex] = make([][]string, len(g.transactions))

		//look at all of the output states and see if we have a spender for any of them
		//TODO this is O(n^2) and could be optimised
		//TODO what about input states that are not output states of any transaction? Do we assume that the minter transactions are already dispatched /
		// or confirmed?
		for _, stateID := range minter.outputStates {
			if spenderIndex := stateToSpender[stateID]; spenderIndex != nil {
				//we have a dependency relationship
				g.transactionsMatrix[minterIndex][*spenderIndex] = append(g.transactionsMatrix[minterIndex][*spenderIndex], stateID)
			}
		}
	}

	return nil
}

// Function GetDispatchableTransactions returns a list of transactions that are ready to be dispatched to the base ledger
// by isolating a subgraph of transactions that have been endorsed and have no dependencies on transactions that have not been endorsed
// and then doing a topological sort of that subgraph
func (g *graph) GetDispatchableTransactions(ctx context.Context) ([]string, error) {

	//TODO there are many valid topilogical sorts of any given graph,
	// should we bias in favour of older transactions?
	// for now, we do a breath first search which is a close approximation of an bias in favour of older transactions

	queue := make([]int, 0, len(g.transactionsMatrix))
	//find all independent transactions - that have no input states in this graph and then do a breadth first search
	// of each of them to find all of its dependent transactions that are also dispatchable and recurse
	dispatchable := make([]string, 0, len(g.transactionsMatrix))
	//i.e. the input states are the output of transactions that are either in the dispatch stage or have been confirmed

	// calcaulate the number of dependencies of each transaction
	indegrees := make([]int, len(g.transactionsMatrix))
	for _, dependants := range g.transactionsMatrix {
		for dependant, states := range dependants {
			if len(states) > 0 {
				indegrees[dependant]++
			}
		}
	}

	//find all independent transactions and add them to the queue
	for txnIndex, indegree := range indegrees {
		if indegree == 0 {
			queue = append(queue, txnIndex)
		}
	}

	// process the queue until it is empty
	// for each transaction in the queue, check if it is dispatchable, if it is, add it to the dispatchable list and add its dependent transactions to the queue if the have no other dependencies
	// queue will become empty when there are no more dispatchable transactions
	for len(queue) > 0 {
		nextTransaction := queue[0]
		queue = queue[1:]

		if !g.transactions[nextTransaction].endorsed {
			//this transaction is not endorsed, so we cannot dispatch it
			continue
		}

		//transaction can be dispatched
		dispatchable = append(dispatchable, g.transactions[nextTransaction].id)

		//get this transaction's dependencies
		dependencies := g.transactionsMatrix[nextTransaction]
		//decrement the indegree of each of the dependent transactions
		for dependant, states := range dependencies {
			if len(states) > 0 {
				indegrees[dependant]--
				if indegrees[dependant] == 0 {
					// add the dependant to the queue
					queue = append(queue, dependant)
				}
			}
		}
	}

	return dispatchable, nil
}

func (g *graph) RemoveTransactions(ctx context.Context, transactionsToRemove []string) error {
	// no validation performed here
	// it is valid to remove transactions that have dependants.  In fact that is normal.
	//Transactions are removed when they are dispatched and dependencies are dispatched before their dependants
	// also, it is valid to remove transactions that are dependants of other transactions that are not being removed
	// maybe they got reverted before being endorsed or whatever it is not the concern of the graph to validate this
	// the graph just gets redrawn based on the dependencies that remain after a transaction is removed

	//Only real validation we do is to throw an error if a transaction to be removed does not exist
	// TODO - is that really an error?  Should we just ignore it and remove the others?  That would give us some idempotency behaviour that might be useful
	for _, txID := range transactionsToRemove {
		if g.allTransactions[txID] == nil {
			log.L(ctx).Errorf("Transaction %s does not exist", txID)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, fmt.Sprintf("Transaction %s does not exist", txID))
		}
		delete(g.allTransactions, txID)
	}
	err := g.buildMatrix(ctx)
	if err != nil {
		log.L(ctx).Errorf("Error building graph: %s", err)
		return err
	}
	return nil
}
