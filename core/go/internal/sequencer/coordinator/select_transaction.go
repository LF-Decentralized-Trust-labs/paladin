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
	"fmt"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator/transaction"
)

func (c *coordinator) selectNextTransaction(ctx context.Context, event *TransactionStateTransitionEvent) error {
	log.L(ctx).Infof("[Sequencer] selecting next transaction")
	txn, err := c.transactionSelector.SelectNextTransaction(ctx, event)
	if txn == nil {
		log.L(ctx).Info("[Sequencer] no transaction found")
		return nil
	}
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] error selecting transaction: %v", err)
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
	log.L(ctx).Infof("[Sequencer] pooled transactions: %d", len(pooledTransactions))
	transactionsBySenderNodeAndIdentity := make(map[string]map[string]*transaction.Transaction)
	for _, txn := range pooledTransactions {
		log.L(ctx).Infof("[Sequencer] found pooled transaction %s from %s", txn.ID.String(), txn.SenderNode())
		if _, ok := transactionsBySenderNodeAndIdentity[txn.SenderNode()]; !ok {
			transactionsBySenderNodeAndIdentity[txn.SenderNode()] = make(map[string]*transaction.Transaction)
		}
		log.L(ctx).Infof("[Sequencer] candidate txn from sender node '%s', sender ID '%s': TXID %s", txn.SenderNode(), txn.SenderIdentity(), txn.ID.String())
		transactionsBySenderNodeAndIdentity[txn.SenderNode()][txn.SenderIdentity()] = txn
	}
	return transactionsBySenderNodeAndIdentity
}

func (c *coordinator) GetCommittee(ctx context.Context) map[string][]string {
	log.L(ctx).Infof("[Sequencer] %d committee nodes for contract %s", len(c.committee), c.contractAddress.HexString())
	return c.committee
}

func (c *coordinator) GetTransactionByID(_ context.Context, txnID uuid.UUID) *transaction.Transaction {
	return c.transactionsByID[txnID]
}

// define the interface for the transaction selector algorithm
// we inject into the algorithm a dependency that allows it to query the state of the transaction pool under the control of the coordinator
type TransactionSelector interface {
	SelectNextTransaction(ctx context.Context, event *TransactionStateTransitionEvent) (*transaction.Transaction, error)
}

// define the interface that the transaction selector algorithm uses to query the state of the transaction pool
type TransactionPool interface {
	//return a list of all transactions in that are in the State_Pooled state, keyed by the sender node and identifier
	GetPooledTransactionsBySenderNodeAndIdentity(ctx context.Context) map[string]map[string]*transaction.Transaction

	GetTransactionByID(ctx context.Context, txnID uuid.UUID) *transaction.Transaction

	//return a list of all members of the committee organized by their node identity
	GetCommittee(ctx context.Context) map[string][]string
}

type transactionSelector struct {
	transactionPool         TransactionPool
	numSenders              int
	fastQueue               chan (*senderLocator)
	slowQueue               chan (*senderLocator)
	currentAssemblingSender *senderLocator
	fastQueueMode           bool
	unseenQueueEntries      int // count of how many times we still need to read from the fast queue before we are back to the start
	timesRoundQueue         int // count of how many times we have read all the fast queue entries since we last read from the slow queue
}

func NewTransactionSelector(ctx context.Context, transactionPool TransactionPool) TransactionSelector {

	//need a fixed order array of senders to ensure that we can iterate through them in a deterministic order
	committeeMap := transactionPool.GetCommittee(ctx)

	selector := &transactionSelector{
		transactionPool:         transactionPool,
		numSenders:              0,
		currentAssemblingSender: nil,
		fastQueueMode:           true,
	}

	for node := range committeeMap {
		selector.numSenders = selector.numSenders + len(committeeMap[node])
	}

	log.L(ctx).Infof("[Sequencer] committee size for transaction selector: %d", selector.numSenders)

	selector.fastQueue = make(chan *senderLocator, selector.numSenders)
	selector.slowQueue = make(chan *senderLocator, selector.numSenders)

	for node := range committeeMap {
		for _, sender := range committeeMap[node] {
			// MRW TODO - is this to emulate 2x the fast queue senders?
			selector.fastQueue <- &senderLocator{node: node, identity: sender}
			selector.slowQueue <- &senderLocator{node: node, identity: sender}
			selector.numSenders++
		}
	}
	selector.unseenQueueEntries = selector.numSenders
	return selector
}

type senderLocator struct {
	node     string
	identity string
}

func (ts *transactionSelector) SelectNextTransaction(ctx context.Context, event *TransactionStateTransitionEvent) (*transaction.Transaction, error) {
	//Super simple algorithm for fair (across senders) selection algorithm that biases against transaction from a sender that is unresponsive or has been tending to assemble transactions that are not getting endorsed
	//Transactions get added to the queue when they enter the State_Pooled state
	// given that only one transaction per sender can enter the State_Pooled state at a time, this gives us some natural fairness

	//There is a fast queue and a slow queue
	// when a transaction fails to assemble due to sender timeout, or if it fails endorsement, the error count is incremented and it is placed at the end of the slow queue

	//NOTE This algorithm currently only biases against the current transaction for each faulty sender.  As soon as that transaction is successful, the next transaction from the same sender is treated just like any other transaction.

	//TODO experiment with refactoring this to be a mini state machine
	if event != nil {
		log.L(ctx).Infof("[Sequencer] event for transaction selection: %+v", event)
		// validate the the event relates to a transaction for the current sender otherwise return an error
		txnID := event.TransactionID

		pooledTransaction := ts.transactionPool.GetTransactionByID(ctx, txnID)
		log.L(ctx).Info("[Sequencer] locate TX in pool")
		if pooledTransaction == nil {
			msg := fmt.Sprintf("[Sequencer] transaction %s not found in the transaction pool", txnID)
			log.L(ctx).Error(msg)
			return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
		if pooledTransaction.SenderNode() != ts.currentAssemblingSender.node || pooledTransaction.SenderIdentity() != ts.currentAssemblingSender.identity {
			msg := fmt.Sprintf("[Sequencer] transaction %s is not being assembled by the current sender %s", txnID, ts.currentAssemblingSender)
			log.L(ctx).Error(msg)
			return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}

		switch event.From {
		case transaction.State_Assembling:
			switch event.To {
			case transaction.State_Endorsement_Gathering:
				ts.fastQueue <- ts.currentAssemblingSender
				ts.currentAssemblingSender = nil
			case transaction.State_Pooled:
				// assuming the only reason for re-pooling is a timeout
				// might need to add a RePoolReason to the transaction object if we find other reasons for this transition
				ts.slowQueue <- ts.currentAssemblingSender
				ts.currentAssemblingSender = nil
			case transaction.State_Reverted:
				ts.slowQueue <- ts.currentAssemblingSender
				ts.currentAssemblingSender = nil
			default:
				msg := fmt.Sprintf("[Sequencer] unexpected transition of transaction %s from assembling state to %s", txnID.String(), event.To.String())
				log.L(ctx).Error(msg)
				return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
			}
		case transaction.State_Endorsement_Gathering:
			switch event.To {
			case transaction.State_Pooled:
				//TODO should somehow bias against senders that are frequently assembling transactions that are not getting endorsed
			}
		}

	} else {
		// there is a transaction currently being assembled and we have no event to tell us that situation has changed
		log.L(ctx).Infof("[Sequencer] no event to tell us that current assembling sender has changed, leaving current assembling sender unchanged")
		if ts.currentAssemblingSender != nil {
			log.L(ctx).Infof("[Sequencer] already have a current assembling sender '%s' for transaction selection, putting them back to the front of the queue", ts.currentAssemblingSender.identity)
			ts.fastQueue <- ts.currentAssemblingSender
		} else {
			log.L(ctx).Infof("[Sequencer] no current assembling sender, reset to the coordinator committee")
			committeeMap := ts.transactionPool.GetCommittee(ctx)
			if len(ts.fastQueue) == 0 && len(ts.slowQueue) == 0 {
				for node := range committeeMap {
					for _, sender := range committeeMap[node] {
						if len(ts.fastQueue) < ts.numSenders {
							log.L(ctx).Infof("[Sequencer] putting sender '%s' from committee '%s' back to the front of the fast queue", sender, node)
							ts.fastQueue <- &senderLocator{node: node, identity: sender}
						} else {
							log.L(ctx).Infof("[Sequencer] not putting sender '%s' from committee '%s' back to the front of the slow queue", sender, node)
						}
						if len(ts.slowQueue) < ts.numSenders {
							log.L(ctx).Infof("[Sequencer] putting sender '%s' from committee '%s' back to the front of the slow queue", sender, node)
							ts.slowQueue <- &senderLocator{node: node, identity: sender}
						} else {
							log.L(ctx).Infof("[Sequencer] not putting sender '%s' from committee '%s' back to the front of the slow queue", sender, node)
						}
					}
				}
			} else {
				log.L(ctx).Infof("[Sequencer] already have senders in at least one queue, leaving the queues unchanged")
			}
		}
	}

	selectableTransactionsMap := ts.transactionPool.GetPooledTransactionsBySenderNodeAndIdentity(ctx)

	getFromFastQueue := func() *senderLocator {
		log.L(ctx).Infof("[Sequencer] getting from fast queue")
		select {
		case sender := <-ts.fastQueue:
			log.L(ctx).Infof("[Sequencer] got sender '%s' from fast queue", sender.identity)
			return sender
		default:
			return nil
		}
	}

	getFromSlowQueue := func() *senderLocator {
		log.L(ctx).Infof("[Sequencer] getting from slow queue")
		select {
		case sender := <-ts.slowQueue:
			log.L(ctx).Infof("[Sequencer] got sender '%s' from slow queue", sender.identity)
			return sender
		default:
			return nil
		}
	}

	nextSender := func() *senderLocator {

		//only read from the slow queue once we have read all the fast queue entries twice or the fast queue is empty

		if ts.unseenQueueEntries == 0 {
			ts.timesRoundQueue++
			ts.unseenQueueEntries = len(ts.fastQueue)
		}

		if (ts.timesRoundQueue > 1 && len(ts.slowQueue) != 0) || len(ts.fastQueue) == 0 {
			ts.timesRoundQueue = 0
			log.L(ctx).Info("[Sequencer] timesRoundQueue > 1 and slow queue is not empty, returning slow queue")
			if sender := getFromSlowQueue(); sender != nil {
				return sender
			}
			return getFromFastQueue()
		}
		ts.unseenQueueEntries--
		log.L(ctx).Infof("[Sequencer] timesRoundQueue %d, returning fast queue", ts.timesRoundQueue)
		return getFromFastQueue()
	}

	log.L(ctx).Infof("[Sequencer] next sender selector, we have %d candidate senders", ts.numSenders)
	for i := 0; i < ts.numSenders; i++ {
		sender := nextSender()
		if sender == nil {
			//very strange situation where the fast queue and slow queue are both empty
			log.L(ctx).Error("[Sequencer] both fast and slow queues are empty")
			return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "Both fast and slow queues are empty")
		}
		log.L(ctx).Infof("[Sequencer] considering next sender identity '%s', node '%s'", sender.identity, sender.node)

		// MRW TODO - tidy up here
		log.L(ctx).Infof("[Sequencer] Selectable transaction identities by node: %d", len(selectableTransactionsMap[sender.node]))
		txn, ok := selectableTransactionsMap[sender.node][sender.identity]
		log.L(ctx).Infof("[Sequencer] Selectable transaction for that identity: %+v", txn)

		if txn != nil {
			log.L(ctx).Infof("[Sequencer] found transaction %s for sender identity '%s', node '%s'", txn.ID.String(), sender.identity, sender.node)
			if ok {
				ts.currentAssemblingSender = sender
				return txn, nil
			} else {
				//requeue the sender to the fast queue
				// even if it has came from the slow queue, it has served its time there
				ts.fastQueue <- sender
			}
		} else {
			log.L(ctx).Infof("[Sequencer] no candidate transaction found for sender identity '%s', node '%s'", sender.identity, sender.node)
		}
	}
	log.L(ctx).Error("[Sequencer] no TX found")
	//gave all senders a chance to have their transaction selected but none had a transaction in the State_Pooled state
	return nil, nil
}
