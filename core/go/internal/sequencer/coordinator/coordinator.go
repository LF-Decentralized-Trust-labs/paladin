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

type coordinator struct {
	/* State */
	stateMachine                               *StateMachine
	activeCoordinator                          string
	activeCoordinatorBlockHeight               uint64
	heartbeatIntervalsSinceStateChange         int
	transactionsByID                           map[uuid.UUID]*transaction.Transaction
	currentBlockHeight                         uint64
	activeCoordinatorsFlushPointsBySignerNonce map[string]*common.FlushPoint
	stateIndex                                 transaction.StateIndex

	/* Config */
	blockRangeSize       uint64
	contractAddress      *tktypes.EthAddress
	blockHeightTolerance uint64
	closingGracePeriod   int // expressed as a multiple of heartbeat intervals

	/* Dependencies */
	messageSender MessageSender
	clock         common.Clock
}

func NewCoordinator(ctx context.Context, messageSender MessageSender, blockRangeSize uint64, contractAddress *tktypes.EthAddress, blockHeightTolerance uint64, closingGracePeriod int) *coordinator {
	c := &coordinator{
		heartbeatIntervalsSinceStateChange: 0,
		transactionsByID:                   make(map[uuid.UUID]*transaction.Transaction),
		messageSender:                      messageSender,
		blockRangeSize:                     blockRangeSize,
		contractAddress:                    contractAddress,
		blockHeightTolerance:               blockHeightTolerance,
		closingGracePeriod:                 closingGracePeriod,
		stateIndex:                         transaction.NewStateIndex(ctx),
	}
	c.InitializeStateMachine(State_Idle)
	return c

}

func (c *coordinator) sendHandoverRequest(ctx context.Context) {
	c.messageSender.SendHandoverRequest(ctx, c.activeCoordinator, c.contractAddress)
}

func (c *coordinator) addToDelegatedTransactions(_ context.Context, sender string, transactions []*components.PrivateTransaction) {
	for _, txn := range transactions {
		c.transactionsByID[txn.ID] = transaction.NewTransaction(sender, txn, c.messageSender, c.clock, c.stateIndex)
	}
}

func (c *coordinator) propagateEventToTransaction(ctx context.Context, event transaction.Event) {
	if txn := c.transactionsByID[event.GetTransactionID()]; txn != nil {
		txn.HandleEvent(ctx, event)
	} else {
		log.L(ctx).Debugf("Ignoring Event because transaction not known to this coordinator %s", event.GetTransactionID().String())
	}
}

func (c *coordinator) getTransactionsInStates(ctx context.Context, states []transaction.State) []*transaction.Transaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	matchingStates := make(map[transaction.State]bool)
	for _, state := range states {
		matchingStates[state] = true
	}
	dispatched := make([]*transaction.Transaction, 0, len(c.transactionsByID))
	for _, txn := range c.transactionsByID {
		if matchingStates[txn.GetState()] {
			dispatched = append(dispatched, txn)
		}
	}
	return dispatched
}

func (c *coordinator) getTransactionsNotInStates(ctx context.Context, states []transaction.State) []*transaction.Transaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	nonMatchingStates := make(map[transaction.State]bool)
	for _, state := range states {
		nonMatchingStates[state] = true
	}
	dispatched := make([]*transaction.Transaction, 0, len(c.transactionsByID))
	for _, txn := range c.transactionsByID {
		if !nonMatchingStates[txn.GetState()] {
			dispatched = append(dispatched, txn)
		}
	}
	return dispatched
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

func (c *coordinator) confirmDispatchedTransaction(ctx context.Context, from *tktypes.EthAddress, nonce uint64, hash tktypes.Bytes32, revertReason tktypes.HexBytes) bool {
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
		dispatchedTransaction.HandleEvent(ctx, event)

		return true

	}
	return false

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
