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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/google/uuid"
)

func (c *coordinator) selectNextTransaction(ctx context.Context, event *TransactionStateTransitionEvent) error {
	log.L(ctx).Infof("selecting next transaction")
	txn, err := c.transactionSelector.SelectNextTransaction(ctx, event)
	if txn == nil {
		log.L(ctx).Info("no transaction found")
		return nil
	}
	if err != nil {
		log.L(ctx).Errorf("error selecting transaction: %v", err)
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
	log.L(ctx).Infof("pooled transactions: %d", len(pooledTransactions))
	transactionsBySenderNodeAndIdentity := make(map[string]map[string]*transaction.Transaction)
	for _, txn := range pooledTransactions {
		log.L(ctx).Infof("found pooled transaction %s from %s", txn.ID.String(), txn.SenderNode())
		if _, ok := transactionsBySenderNodeAndIdentity[txn.SenderNode()]; !ok {
			transactionsBySenderNodeAndIdentity[txn.SenderNode()] = make(map[string]*transaction.Transaction)
		}
		log.L(ctx).Infof("candidate txn from sender node '%s', sender ID '%s': TXID %s", txn.SenderNode(), txn.SenderIdentity(), txn.ID.String())
		transactionsBySenderNodeAndIdentity[txn.SenderNode()][txn.SenderIdentity()] = txn
	}
	return transactionsBySenderNodeAndIdentity
}

func (c *coordinator) GetCurrentSenderPool(ctx context.Context) []string {
	log.L(ctx).Infof("%d sender pool identities for contract %s", len(c.senderNodePool), c.contractAddress.HexString())
	return c.senderNodePool
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
	GetCurrentSenderPool(ctx context.Context) []string
}

type transactionSelector struct {
	transactionPool         TransactionPool
	numSenders              int
	currentAssemblingSender *senderLocator
}

func NewTransactionSelector(ctx context.Context, transactionPool TransactionPool) TransactionSelector {

	//need a fixed order array of senders to ensure that we can iterate through them in a deterministic order
	senderPoolMap := transactionPool.GetCurrentSenderPool(ctx)

	selector := &transactionSelector{
		transactionPool:         transactionPool,
		numSenders:              0,
		currentAssemblingSender: nil,
	}

	for node := range senderPoolMap {
		selector.numSenders = selector.numSenders + len(senderPoolMap[node])
	}

	log.L(ctx).Infof("sender pool size for transaction selector: %d", selector.numSenders)
	return selector
}

type senderLocator struct {
	node     string
	identity string
}

func (ts *transactionSelector) SelectNextTransaction(ctx context.Context, event *TransactionStateTransitionEvent) (*transaction.Transaction, error) {
	// Initial distributed sequencer implementation just pulls transactions in FIFO order. This needs implementing to be fairer, potentially based
	// on the initial fast-queue/slow-queue in the PoC distributed sequencer.
	if event != nil {
		log.L(ctx).Infof("event for transaction selection: %+v", event)
		// validate the the event relates to a transaction for the current sender otherwise return an error
		txnID := event.TransactionID

		pooledTransaction := ts.transactionPool.GetTransactionByID(ctx, txnID)
		log.L(ctx).Info("locate TX in pool")
		if pooledTransaction == nil {
			msg := fmt.Sprintf("transaction %s not found in the transaction pool", txnID)
			log.L(ctx).Error(msg)
			return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}

		if ts.currentAssemblingSender != nil {
			// Check we're still the current assembler
			if pooledTransaction.SenderNode() != ts.currentAssemblingSender.node || pooledTransaction.SenderIdentity() != ts.currentAssemblingSender.identity {
				msg := fmt.Sprintf("transaction %s is not being assembled by the current sender %s", txnID, ts.currentAssemblingSender)
				log.L(ctx).Error(msg)
				return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
			}
		}

		switch event.From {
		case transaction.State_Assembling:
			switch event.To {
			case transaction.State_Endorsement_Gathering:
				ts.currentAssemblingSender = nil
			case transaction.State_Pooled:
				// assuming the only reason for re-pooling is a timeout
				// might need to add a RePoolReason to the transaction object if we find other reasons for this transition
				ts.currentAssemblingSender = nil
			case transaction.State_Reverted:
				ts.currentAssemblingSender = nil
			default:
				msg := fmt.Sprintf("unexpected transition of transaction %s from assembling state to %s", txnID.String(), event.To.String())
				log.L(ctx).Error(msg)
				return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
			}
		case transaction.State_Endorsement_Gathering:
			switch event.To {
			case transaction.State_Pooled:
				//TODO should somehow bias against senders that are frequently assembling transactions that are not getting endorsed
			}
		}

	}

	selectableTransactionsMap := ts.transactionPool.GetPooledTransactionsBySenderNodeAndIdentity(ctx)

	log.L(ctx).Infof("next sender selector, we have %d candidate senders", ts.numSenders)
	if ts.numSenders == 0 {
		log.L(ctx).Info("no candidate senders, just returning the next pooled transaction")
		for _, senderNodes := range selectableTransactionsMap {
			for _, pooledTx := range senderNodes {
				if pooledTx != nil {
					log.L(ctx).Infof("[Sequencer] returning next pooled TX %s", pooledTx.ID.String())
					ts.currentAssemblingSender = &senderLocator{node: pooledTx.SenderNode(), identity: pooledTx.SenderIdentity()}
					return pooledTx, nil
				}
			}
		}
	}
	return nil, nil
}
