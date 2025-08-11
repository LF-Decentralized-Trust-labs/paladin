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

package privatetxnmgr

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr/ptmgrtypes"
)

type Graph interface {
	AddTransaction(ctx context.Context, transaction ptmgrtypes.TransactionFlow)
	GetDispatchableTransactions(ctx context.Context) (ptmgrtypes.DispatchableTransactions, error)
	RemoveTransaction(ctx context.Context, txID string)
	RemoveTransactions(ctx context.Context, transactionsToRemove []string)
	IncludesTransaction(txID string) bool
}

type graph struct {
	// This is the source of truth for all transaction
	allTransactions map[string]ptmgrtypes.TransactionFlow

	// all of the following are ephemeral and derived from allTransactions

	// implement graph of transactions as an adjacency matrix where the values in the matrix is an array of state hashes that connect those transactions
	// first dimension is the dependency ( i.e the minter of the state) and second dimension is the dependant (i.e. the consumer of the state)
	// and the third dimension is the array of state hashes that connect the two transactions
	//  this direction makes it easier to isolate a sequence of dispatchable transactions by doing a breadth first search starting at the layer of independent transactions
	transactionsMatrix [][][]string
	transactions       []ptmgrtypes.TransactionFlow
	//map of transaction id to index in the transactions array
	transactionIndex map[string]int
}

func NewGraph() Graph {
	return &graph{
		allTransactions: make(map[string]ptmgrtypes.TransactionFlow),
	}
}

func (g *graph) AddTransaction(ctx context.Context, transaction ptmgrtypes.TransactionFlow) {
	log.L(ctx).Debugf("Adding transaction %s to graph", transaction.ID(ctx).String())
	g.allTransactions[transaction.ID(ctx).String()] = transaction

}

func (g *graph) IncludesTransaction(txID string) bool {
	return g.allTransactions[txID] != nil
}

func (g *graph) buildMatrix(ctx context.Context) error {
	log.L(ctx).Debugf("Building graph with %d transactions", len(g.allTransactions))
	g.transactionIndex = make(map[string]int)
	g.transactions = make([]ptmgrtypes.TransactionFlow, len(g.allTransactions))
	currentIndex := 0
	for txnId, txn := range g.allTransactions {
		g.transactionIndex[txnId] = currentIndex
		g.transactions[currentIndex] = txn

		currentIndex++

	}
	//for each unique state hash, create an index of its minter and/or spender
	stateToSpender := make(map[string]*int)
	for txnIndex, txn := range g.transactions {
		for _, stateID := range txn.InputStateIDs(ctx) {
			if stateToSpender[stateID] != nil {
				//TODO this is expected in some cases and represents a contention that needs to be resolved
				//TBC do we assert that it is resolved before we get to this point?
				log.L(ctx).Errorf("State hash %s is spent by multiple transactions", stateID)
				return i18n.NewError(ctx, msgs.MsgPrivateTxManagerStateHashContention, stateID)
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
		for _, stateID := range minter.OutputStateIDs(ctx) {
			if spenderIndex := stateToSpender[stateID]; spenderIndex != nil {
				//we have a dependency relationship
				if log.IsTraceEnabled() {
					log.L(ctx).Tracef("Graph.buildMatrix Transaction %s depends on transaction %s", minter.ID(ctx).String(), g.transactions[*spenderIndex].ID(ctx).String())
				}
				g.transactionsMatrix[minterIndex][*spenderIndex] = append(g.transactionsMatrix[minterIndex][*spenderIndex], stateID)
			}
		}
	}

	return nil
}

// Function GetDispatchableTransactions returns a list of transactions that are ready to be dispatched to the base ledger
// by isolating subgraphs (within each subgraph all transactions are to be dispatched with the same signing key)
// of transactions that have been endorsed and have no dependencies on transactions that have not been endorsed
// and then doing a topological sort of each of those subgraphs
func (g *graph) GetDispatchableTransactions(ctx context.Context) (ptmgrtypes.DispatchableTransactions, error) {
	log.L(ctx).Debug("Graph.GetDispatchableTransactions")

	// TODO should probably cache this graph and only rebuild it when needed (e.g. on restart)
	// and incrementally update it when new transactions are added etc...
	// if we do build it every time, might as well have the list of transactions passed in as a parameter rather than trying to maintain the list via AddTransaction and RemoveTransaction
	err := g.buildMatrix(ctx)
	if err != nil {
		log.L(ctx).Errorf("Error building graph: %s", err)
		return nil, err
	}

	//TODO there are many valid topological sorts of any given graph,
	// should we bias in favour of older transactions?
	// for now, we do a breath first search which is a close approximation of an bias in favour of older transactions

	queue := make([]int, 0, len(g.transactionsMatrix))
	//find all independent transactions - that have no input states in this graph and then do a breadth first search
	// of each of them to find all of its dependent transactions that are also dispatchable and recurse
	dispatchable := make([]ptmgrtypes.TransactionFlow, 0, len(g.transactionsMatrix))
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
			if log.IsTraceEnabled() {
				log.L(ctx).Tracef("Graph.GetDispatchableTransactions Transaction %s has no dependencies", g.transactions[txnIndex].ID(ctx).String())
			}
			queue = append(queue, txnIndex)
		}
	}

	// process the queue until it is empty
	// for each transaction in the queue, check if it is dispatchable, if it is, add it to the dispatchable list and add its dependent transactions to the queue if the have no other dependencies
	// queue will become empty when there are no more dispatchable transactions
	for len(queue) > 0 {
		nextTransaction := queue[0]
		queue = queue[1:]

		if !g.transactions[nextTransaction].IsEndorsed(ctx) {
			//this transaction is not endorsed, so we cannot dispatch it
			if log.IsTraceEnabled() {
				log.L(ctx).Tracef("Graph.GetDispatchableTransactions Transaction %s not endorsed so cannot be dispatched", g.transactions[nextTransaction].ID(ctx).String())
			}
			continue
		} else {
			if log.IsTraceEnabled() {
				log.L(ctx).Tracef("Graph.GetDispatchableTransactions Transaction %s is endorsed and will be dispatched", g.transactions[nextTransaction].ID(ctx).String())
			}
		}

		//transaction can be dispatched
		dispatchable = append(dispatchable, g.transactions[nextTransaction])

		//get this transaction's dependencies
		dependencies := g.transactionsMatrix[nextTransaction]
		//decrement the indegree of each of the dependent transactions
		for dependant, states := range dependencies {
			if len(states) > 0 {
				indegrees[dependant]--
				if indegrees[dependant] == 0 {
					if log.IsTraceEnabled() {
						log.L(ctx).Tracef("Graph.GetDispatchableTransactions Transaction %s dependencies are being dispatched", g.transactions[dependant].ID(ctx).String())
					}
					// add the dependant to the queue
					queue = append(queue, dependant)
				}
			}
		}
	}

	//TODO for now, we assume that all dispatchable transactions are to be dispatched by the same signing key
	// in reality, we need to maintain subgraphs per signing key because there is no way to guarantee ordering
	// across signing keys

	if len(dispatchable) > 0 {
		signingAddress := g.allTransactions[dispatchable[0].ID(ctx).String()].Signer(ctx)
		log.L(ctx).Debugf("Graph.GetDispatchableTransactions %d dispatchable transactions", len(dispatchable))
		return map[string][]ptmgrtypes.TransactionFlow{
			signingAddress: dispatchable,
		}, nil
	}
	log.L(ctx).Debug("Graph.GetDispatchableTransactions No dispatchable transactions")

	return map[string][]ptmgrtypes.TransactionFlow{}, nil
}
func (g *graph) RemoveTransaction(ctx context.Context, txID string) {
	log.L(ctx).Debugf("Graph.RemoveTransaction Removing transaction %s from graph", txID)
	delete(g.allTransactions, txID)
}

func (g *graph) RemoveTransactions(ctx context.Context, transactionIDsToRemove []string) {
	log.L(ctx).Debugf("Graph.RemoveTransactions Removing transactions from graph")
	// no validation performed here
	// it is valid to remove transactions that have dependents.  In fact that is normal.
	//Transactions are removed when they are dispatched and dependencies are dispatched before their dependents
	// also, it is valid to remove transactions that are dependents of other transactions that are not being removed
	// maybe they got reverted before being endorsed or whatever it is not the concern of the graph to validate this
	// the graph just gets redrawn based on the dependencies that remain after a transaction is removed

	for _, transactionID := range transactionIDsToRemove {
		if g.allTransactions[transactionID] == nil {
			log.L(ctx).Infof("Transaction %s already removed", transactionID)
		} else {
			delete(g.allTransactions, transactionID)
		}
	}
}
