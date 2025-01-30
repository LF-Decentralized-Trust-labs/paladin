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
	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator/delegation"
	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator/pool"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type coordinator struct {
	/* State */
	stateMachine                               *StateMachine
	activeCoordinator                          string
	activeCoordinatorBlockHeight               uint64
	pooledTransactions                         pool.TransactionPool
	heartbeatIntervalsSinceStateChange         int
	delegationsByTransactionID                 map[uuid.UUID]*delegation.Delegation
	currentBlockHeight                         uint64
	activeCoordinatorsFlushPointsBySignerNonce map[string]*common.FlushPoint

	/* Config */
	blockRangeSize       uint64
	contractAddress      *tktypes.EthAddress
	blockHeightTolerance uint64
	closingGracePeriod   int // expressed as a multiple of heartbeat intervals

	/* Dependencies */
	messageSender MessageSender
}

func NewCoordinator(ctx context.Context, messageSender MessageSender, blockRangeSize uint64, contractAddress *tktypes.EthAddress, blockHeightTolerance uint64, closingGracePeriod int) *coordinator {
	c := &coordinator{
		pooledTransactions:                 pool.NewTransactionPool(ctx),
		heartbeatIntervalsSinceStateChange: 0,
		delegationsByTransactionID:         make(map[uuid.UUID]*delegation.Delegation),
		messageSender:                      messageSender,
		blockRangeSize:                     blockRangeSize,
		contractAddress:                    contractAddress,
		blockHeightTolerance:               blockHeightTolerance,
		closingGracePeriod:                 closingGracePeriod,
	}
	c.InitializeStateMachine(State_Idle)
	return c

}

func (c *coordinator) sendHandoverRequest(ctx context.Context) {
	c.messageSender.SendHandoverRequest(ctx, c.activeCoordinator, c.contractAddress)
}

func (c *coordinator) addToDelegatedTransactions(_ context.Context, sender string, transactions []*components.PrivateTransaction) {
	for _, transaction := range transactions {
		c.delegationsByTransactionID[transaction.ID] = delegation.NewDelegation(sender, transaction)
	}
}

func (c *coordinator) propagateEventToDelegation(ctx context.Context, event delegation.Event) {
	if delegation := c.delegationsByTransactionID[event.GetTransactionID()]; delegation != nil {
		delegation.HandleEvent(ctx, event)
	} else {
		log.L(ctx).Debugf("Ignoring Event because delegation not known to this coordinator %s", event.GetTransactionID().String())
	}
}

func (c *coordinator) getTransactionsInStates(ctx context.Context, states []delegation.State) []*delegation.Delegation {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	matchingStates := make(map[delegation.State]bool)
	for _, state := range states {
		matchingStates[state] = true
	}
	dispatched := make([]*delegation.Delegation, 0, len(c.delegationsByTransactionID))
	for _, delegation := range c.delegationsByTransactionID {
		if matchingStates[delegation.GetState()] {
			dispatched = append(dispatched, delegation)
		}
	}
	return dispatched
}

func (c *coordinator) getTransactionsNotInStates(ctx context.Context, states []delegation.State) []*delegation.Delegation {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	nonMatchingStates := make(map[delegation.State]bool)
	for _, state := range states {
		nonMatchingStates[state] = true
	}
	dispatched := make([]*delegation.Delegation, 0, len(c.delegationsByTransactionID))
	for _, delegation := range c.delegationsByTransactionID {
		if !nonMatchingStates[delegation.GetState()] {
			dispatched = append(dispatched, delegation)
		}
	}
	return dispatched
}

func (c *coordinator) findTransactionBySignerNonce(ctx context.Context, signer *tktypes.EthAddress, nonce uint64) *delegation.Delegation {
	//TODO this would be more efficient by maintaining a separate index but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	for _, delegation := range c.delegationsByTransactionID {
		if delegation.GetSignerAddress() != nil && *delegation.GetSignerAddress() == *signer && delegation.GetNonce() != nil && *(delegation.GetNonce()) == nonce {
			return delegation
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
		delegationEvent := &delegation.ConfirmedEvent{
			Hash:         hash,
			RevertReason: revertReason,
		}
		delegationEvent.TransactionID = dispatchedTransaction.ID
		dispatchedTransaction.HandleEvent(ctx, delegationEvent)

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
