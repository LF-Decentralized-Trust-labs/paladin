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
	if err != nil {
		log.L(ctx).Errorf("error selecting transaction: %v", err)
		return err
	}
	if txn == nil {
		log.L(ctx).Info("no transaction found to process")
		return nil
	}

	transactionSelectedEvent := &transaction.SelectedEvent{}
	transactionSelectedEvent.TransactionID = txn.ID
	log.L(ctx).Infof("selected next transaction %s", txn.ID.String())
	err = txn.HandleEvent(ctx, transactionSelectedEvent)
	return err

}

/* Functions of the TransactionPool interface required by the transactionSelector */
func (c *coordinator) GetPooledTransactionsByOriginatorNodeAndIdentity(ctx context.Context) map[string]map[string]*transaction.Transaction {
	pooledTransactions := c.getTransactionsInStates(ctx, []transaction.State{transaction.State_Pooled})
	log.L(ctx).Debugf("pooled transactions: %d", len(pooledTransactions))
	transactionsByOriginatorNodeAndIdentity := make(map[string]map[string]*transaction.Transaction)
	for _, txn := range pooledTransactions {
		log.L(ctx).Debugf("found pooled transaction %s from %s", txn.ID.String(), txn.OriginatorNode())
		if _, ok := transactionsByOriginatorNodeAndIdentity[txn.OriginatorNode()]; !ok {
			transactionsByOriginatorNodeAndIdentity[txn.OriginatorNode()] = make(map[string]*transaction.Transaction)
		}
		log.L(ctx).Debugf("candidate txn from originator node '%s', originator ID '%s': TXID %s", txn.OriginatorNode(), txn.OriginatorIdentity(), txn.ID.String())
		transactionsByOriginatorNodeAndIdentity[txn.OriginatorNode()][txn.OriginatorIdentity()] = txn
	}
	return transactionsByOriginatorNodeAndIdentity
}

func (c *coordinator) GetCurrentOriginatorPool(ctx context.Context) []string {
	log.L(ctx).Debugf("%d originator pool identities for contract %s", len(c.originatorNodePool), c.contractAddress.HexString())
	return c.originatorNodePool
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
	// return a list of all transactions in that are in the State_Pooled state, keyed by the originator node and identifier
	GetPooledTransactionsByOriginatorNodeAndIdentity(ctx context.Context) map[string]map[string]*transaction.Transaction

	GetTransactionByID(ctx context.Context, txnID uuid.UUID) *transaction.Transaction

	// return a list of all members of the originators organized by their node identity
	GetCurrentOriginatorPool(ctx context.Context) []string
}

type transactionSelector struct {
	transactionPool             TransactionPool
	currentAssemblingOriginator *originatorLocator
}

func NewTransactionSelector(ctx context.Context, transactionPool TransactionPool) TransactionSelector {

	selector := &transactionSelector{
		transactionPool:             transactionPool,
		currentAssemblingOriginator: nil,
	}

	return selector
}

type originatorLocator struct {
	node     string
	identity string
}

// Determine if there is still an active originator, so we know if it's safe to select a new transaction to start processing
func (ts *transactionSelector) currentTransactionStillInProgress(ctx context.Context, event *TransactionStateTransitionEvent) (bool, error) {

	log.L(ctx).Debugf("checking event before transaction selection: %+v", event)

	// validate the the event relates to a transaction for the current originator otherwise return an error
	txnID := event.TransactionID

	pooledTransaction := ts.transactionPool.GetTransactionByID(ctx, txnID)
	log.L(ctx).Debugf("checking TX pool for event TX %s", txnID.String())
	if pooledTransaction == nil {
		// We're processing an event for a transaction the coordinator no longer knows about. Log a warning,
		// but allow the next transaction to be selected. Not sure if there is ever a valid reason for this to happen,
		// so return true to prevent a
		msg := fmt.Sprintf("transaction %s not found in the transaction pool", txnID)
		log.L(ctx).Warn(msg)
		return false, i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
	}

	if ts.currentAssemblingOriginator != nil {
		// The state machine event is for a different originator to the one who's currently assmebling. This could be a delayed event and
		// in the mean time we've started processing a TX from another originator. Disregard the event and wait for the current in-progress
		// TX to move to a point where we can select the next TX.
		if pooledTransaction.OriginatorNode() != ts.currentAssemblingOriginator.node || pooledTransaction.OriginatorIdentity() != ts.currentAssemblingOriginator.identity {
			log.L(ctx).Warnf("current assembling originator is %s, no TX selection changes to take for TX %s from %s", ts.currentAssemblingOriginator, txnID.String(), pooledTransaction.OriginatorNode())
			return true, nil
		}
	}

	switch event.From {
	case transaction.State_Assembling:
		switch event.To {
		case transaction.State_Endorsement_Gathering:
			log.L(ctx).Tracef("transaction %s moved to endorsement gathering state, no longer assembling. Can start next TX", txnID.String())
			ts.currentAssemblingOriginator = nil
		case transaction.State_Pooled:
			// assuming the only reason for re-pooling is a timeout
			// might need to add a RePoolReason to the transaction object if we find other reasons for this transition
			log.L(ctx).Tracef("transaction %s moved to pooled state, no longer assembling. Can start next TX", txnID.String())
			ts.currentAssemblingOriginator = nil
		case transaction.State_Reverted:
			log.L(ctx).Tracef("transaction %s moved to reverted state, no longer assembling. Can start next TX", txnID.String())
			ts.currentAssemblingOriginator = nil
		default:
			msg := fmt.Sprintf("unexpected transition of transaction %s from assembling state to %s", txnID.String(), event.To.String())
			log.L(ctx).Error(msg)
			return true, i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
	case transaction.State_Endorsement_Gathering:
		switch event.To {
		case transaction.State_Pooled:
			// A TX moved back into the pool, presumably after endorsement failed.
			log.L(ctx).Tracef("transaction %s moved to pooled state, no longer endorsing. Can start next TX", txnID.String())
			ts.currentAssemblingOriginator = nil
		}
	}

	return ts.currentAssemblingOriginator != nil, nil
}

// We select a new transaction to process in the following cases:
// 1. The current TX has moved to a state that we consider safe to select a new TX (e.g. the previous TX moved on to endorsing)
// 2. The coordinator has just become the active coordinator
// 3. The coordinator has just had a new TX delegated to it
func (ts *transactionSelector) SelectNextTransaction(ctx context.Context, event *TransactionStateTransitionEvent) (*transaction.Transaction, error) {

	// Certain event types are confirmation that we are safe to select a new transaction. Others mean we
	// need to wait for the current TX to progress
	if event != nil {
		currentTXStillInProgress, err := ts.currentTransactionStillInProgress(ctx, event)
		if err != nil {
			return nil, err
		}
		if currentTXStillInProgress {
			// Can't select a new TX yet
			return nil, nil
		}
	}

	// Initial distributed sequencer implementation just pulls transactions in FIFO order. This needs implementing to be fairer, potentially based
	// on the initial fast-queue/slow-queue in the PoC distributed sequencer.
	selectableTransactionsMap := ts.transactionPool.GetPooledTransactionsByOriginatorNodeAndIdentity(ctx)

	for _, originatorNodes := range selectableTransactionsMap {
		for _, pooledTx := range originatorNodes {
			if pooledTx != nil {
				log.L(ctx).Infof("returning next pooled TX %s", pooledTx.ID.String())
				ts.currentAssemblingOriginator = &originatorLocator{node: pooledTx.OriginatorNode(), identity: pooledTx.OriginatorIdentity()}
				return pooledTx, nil
			}
		}
	}

	return nil, nil
}
