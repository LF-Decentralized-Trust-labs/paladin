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
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator/transaction"

	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type Coordinator interface {
	GetTransactionsReadyToDispatch(ctx context.Context) ([]*components.PrivateTransaction, error)
}

type coordinator struct {
	/* State */
	stateMachine                               *StateMachine
	activeCoordinator                          string
	activeCoordinatorBlockHeight               uint64
	heartbeatIntervalsSinceStateChange         int
	transactionsByID                           map[uuid.UUID]*transaction.Transaction
	currentBlockHeight                         uint64
	activeCoordinatorsFlushPointsBySignerNonce map[string]*common.FlushPoint
	grapher                                    transaction.Grapher

	/* Config */
	blockRangeSize       uint64
	contractAddress      *tktypes.EthAddress
	blockHeightTolerance uint64
	closingGracePeriod   int // expressed as a multiple of heartbeat intervals
	requestTimeout       common.Duration
	assembleTimeout      common.Duration
	committee            map[string][]string

	/* Dependencies */
	messageSender     MessageSender
	clock             common.Clock
	engineIntegration common.EngineIntegration
	emit              common.EmitEvent

	/*Algorithms*/
	transactionSelector TransactionSelector
}

func NewCoordinator(
	ctx context.Context,
	messageSender MessageSender,
	committeeMembers []string,
	clock common.Clock,
	emit common.EmitEvent,
	engineIntegration common.EngineIntegration,
	requestTimeout,
	assembleTimeout common.Duration,
	blockRangeSize uint64,
	contractAddress *tktypes.EthAddress,
	blockHeightTolerance uint64,
	closingGracePeriod int,
) (*coordinator, error) {
	c := &coordinator{
		heartbeatIntervalsSinceStateChange: 0,
		transactionsByID:                   make(map[uuid.UUID]*transaction.Transaction),
		messageSender:                      messageSender,
		blockRangeSize:                     blockRangeSize,
		contractAddress:                    contractAddress,
		blockHeightTolerance:               blockHeightTolerance,
		closingGracePeriod:                 closingGracePeriod,
		grapher:                            transaction.NewGrapher(ctx),
		clock:                              clock,
		requestTimeout:                     requestTimeout,
		assembleTimeout:                    assembleTimeout,
		engineIntegration:                  engineIntegration,
		emit:                               emit,
	}
	c.committee = make(map[string][]string)
	for _, member := range committeeMembers {
		memberLocator := tktypes.PrivateIdentityLocator(member)
		memberNode, err := memberLocator.Node(ctx, false)
		if err != nil {
			log.L(ctx).Errorf("Error resolving node for member %s: %v", member, err)
			return nil, err
		}

		memberIdentity, err := memberLocator.Identity(ctx)
		if err != nil {
			log.L(ctx).Errorf("Error resolving identity for member %s: %v", member, err)
			return nil, err
		}

		if _, ok := c.committee[memberNode]; !ok {
			c.committee[memberNode] = make([]string, 0)
		}

		c.committee[memberNode] = append(c.committee[memberNode], memberIdentity)

	}
	c.InitializeStateMachine(State_Idle)
	c.transactionSelector = NewTransactionSelector(ctx, c)
	return c, nil

}

func (c *coordinator) sendHandoverRequest(ctx context.Context) {
	c.messageSender.SendHandoverRequest(ctx, c.activeCoordinator, c.contractAddress)
}

// TODO consider renaming to setDelegatedTransactionsForSender to make it clear that we expect senders to include all inflight transactions in every delegation request and therefore this is
// a replace, not an add.  Need to finalize the decision about whether we expect the sender to include all inflight delegated transactions in every delegation request. Currently the code assumes we do so need to make the spec clear on that point and
// record a decision record to explain why.  Every  time we come back to this point, we will be tempted to reverse that decision so we need to make sure we have a record of the known consequences.
// sender must be a fully qualified identity locator otherwise an error will be returned
func (c *coordinator) addToDelegatedTransactions(ctx context.Context, sender string, transactions []*components.PrivateTransaction) error {

	//TODO should remove any transactions from the same sender that we already have but are not in this list

	var previousTransaction *transaction.Transaction
	for _, txn := range transactions {
		newTransaction, err := transaction.NewTransaction(
			ctx,
			sender,
			txn,
			c.messageSender,
			c.clock,
			c.emit,
			c.engineIntegration,
			c.requestTimeout,
			c.assembleTimeout,
			c.closingGracePeriod,
			c.grapher,
			func(ctx context.Context, t *transaction.Transaction, to, from transaction.State) {
				//callback function to notify us when the transaction changes state
				log.L(ctx).Debugf("Transaction %s moved from %s to %s", t.ID.String(), from.String(), to.String())
				//TODO the following logic should be moved to the state machine so that all the rules are in one place
				if c.stateMachine.currentState == State_Active {
					if from == transaction.State_Assembling {
						err := c.selectNextTransaction(ctx, &TransactionStateTransitionEvent{
							TransactionID: t.ID,
							From:          from,
							To:            to,
						})
						if err != nil {
							log.L(ctx).Errorf("Error selecting next transaction after transaction %s moved from %s to %s: %v", t.ID.String(), from.String(), to.String(), err)
							//TODO figure out how to get this to the abend handler
						}
					}
				}
			},
			func(ctx context.Context) {
				//callback function to notify us when the transaction is cleaned up
				delete(c.transactionsByID, txn.ID)
				c.grapher.Forget(txn.ID)
				log.L(ctx).Debugf("Transaction %s cleaned up", txn.ID.String())
			})
		if err != nil {
			log.L(ctx).Errorf("Error creating transaction: %v", err)
			return err
		}

		if previousTransaction != nil {
			newTransaction.SetPreviousTransaction(ctx, previousTransaction)
			previousTransaction.SetNextTransaction(ctx, newTransaction)
		}
		c.transactionsByID[txn.ID] = newTransaction
		previousTransaction = newTransaction

		receivedEvent := &transaction.ReceivedEvent{}
		receivedEvent.TransactionID = txn.ID
		err = c.transactionsByID[txn.ID].HandleEvent(context.Background(), receivedEvent)
		if err != nil {
			log.L(ctx).Errorf("Error handling ReceivedEvent for transaction %s: %v", txn.ID.String(), err)
			return err
		}
	}
	return nil
}

func (c *coordinator) propagateEventToTransaction(ctx context.Context, event transaction.Event) error {
	if txn := c.transactionsByID[event.GetTransactionID()]; txn != nil {
		return txn.HandleEvent(ctx, event)
	} else {
		log.L(ctx).Debugf("Ignoring Event because transaction not known to this coordinator %s", event.GetTransactionID().String())
	}
	return nil
}

func (c *coordinator) propagateEventToAllTransactions(ctx context.Context, event common.Event) error {
	for _, txn := range c.transactionsByID {
		err := txn.HandleEvent(ctx, event)
		if err != nil {
			log.L(ctx).Errorf("Error handling event %v for transaction %s: %v", event.Type(), txn.ID.String(), err)
			return err
		}
	}
	return nil
}

func (c *coordinator) getTransactionsInStates(ctx context.Context, states []transaction.State) []*transaction.Transaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	matchingStates := make(map[transaction.State]bool)
	for _, state := range states {
		matchingStates[state] = true
	}
	matchingTxns := make([]*transaction.Transaction, 0, len(c.transactionsByID))
	for _, txn := range c.transactionsByID {
		if matchingStates[txn.GetState()] {
			matchingTxns = append(matchingTxns, txn)
		}
	}
	return matchingTxns
}

func (c *coordinator) getTransactionsNotInStates(ctx context.Context, states []transaction.State) []*transaction.Transaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	nonMatchingStates := make(map[transaction.State]bool)
	for _, state := range states {
		nonMatchingStates[state] = true
	}
	matchingTxns := make([]*transaction.Transaction, 0, len(c.transactionsByID))
	for _, txn := range c.transactionsByID {
		if !nonMatchingStates[txn.GetState()] {
			matchingTxns = append(matchingTxns, txn)
		}
	}
	return matchingTxns
}

func (c *coordinator) findTransactionBySignerNonce(ctx context.Context, signer *tktypes.EthAddress, nonce uint64) *transaction.Transaction {
	//TODO this would be more efficient by maintaining a separate index but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	for _, txn := range c.transactionsByID {
		if txn.GetSignerAddress() != nil && *txn.GetSignerAddress() == *signer && txn.GetNonce() != nil && *(txn.GetNonce()) == nonce {
			return txn
		}
	}
	return nil
}

func (c *coordinator) confirmDispatchedTransaction(ctx context.Context, from *tktypes.EthAddress, nonce uint64, hash tktypes.Bytes32, revertReason tktypes.HexBytes) (bool, error) {
	// First check whether it is one that we have been coordinating
	if dispatchedTransaction := c.findTransactionBySignerNonce(ctx, from, nonce); dispatchedTransaction != nil {
		if dispatchedTransaction.GetLatestSubmissionHash() == nil || *(dispatchedTransaction.GetLatestSubmissionHash()) != hash {
			// Is this not the transaction that we are looking for?
			// We have missed a submission?  Or is it possible that an earlier submission has managed to get confirmed?
			// It is interesting so we log it but either way,  this must be the transaction that we are looking for because we can't re-use a nonce
			log.L(ctx).Debugf("Transaction %s confirmed with a different hash than expected", dispatchedTransaction.ID.String())
		}
		event := &transaction.ConfirmedEvent{
			Hash:         hash,
			RevertReason: revertReason,
		}
		event.TransactionID = dispatchedTransaction.ID
		err := dispatchedTransaction.HandleEvent(ctx, event)
		if err != nil {
			log.L(ctx).Errorf("Error handling ConfirmedEvent for transaction %s: %v", dispatchedTransaction.ID.String(), err)
			return false, err
		}
		return true, nil
	}
	return false, nil

}

func (c *coordinator) confirmMonitoredTransaction(ctx context.Context, from *tktypes.EthAddress, nonce uint64) {
	if flushPoint := c.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", from.String(), nonce)]; flushPoint != nil {
		//We do not remove the flushPoint from the list because there is a chance that the coordinator hasn't seen this confirmation themselves and
		// when they send us the next heartbeat, it will contain this FlushPoint so it would get added back into the list and we would not see the confirmation again
		flushPoint.Confirmed = true
	}
}

func ptrTo[T any](v T) *T {
	return &v
}

//TODO the following getter methods are not safe to call on anything other than the sequencer goroutine because they are reading data structures that are being modified by the state machine.
// We should consider making them safe to call from any goroutine by maintaining a copy of the data structures that are updated async from the sequencer thread under a mutex

func (c *coordinator) GetCurrentState() State {
	return c.stateMachine.currentState
}
