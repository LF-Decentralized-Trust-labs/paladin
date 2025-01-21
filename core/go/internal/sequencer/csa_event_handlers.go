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

package sequencer

import (
	"context"
	"errors"

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
)

func (c *contractSequencerAgent) HandleCoordinatorHeartbeatNotification(ctx context.Context, message *CoordinatorHeartbeatNotification) error {
	if c.activeCoordinator != message.From {
		c.activeCoordinator = message.From

		//re-delegate all transactions to the new coordinator
		c.delegateAllTransactions(ctx)

	}

	//Replace the list of dispatched transactions with the new list
	c.dispatchedTransactionsByCoordinator[message.From] = message.DispatchedTransactions
	c.missedHeartbeats = 0
	return nil
}

func (c *contractSequencerAgent) HandleDelegationRequest(ctx context.Context, message *DelegationRequest) error {
	for _, transaction := range message.Transactions {
		c.delegationsByTransactionID[transaction.ID.String()] = NewDelegation(message.Sender, transaction)
	}
	if c.activeCoordinator == "" {
		c.coordinator.state = CoordinatorState_Active
	} else {
		//We are aware of another active coordinator so we ask for handover information that will prevent us from
		// wastefully trying to dispatch transactions that will ultimately encounter double spend collision on the base ledger

		handoverRequest := HandoverRequest{
			ContractAddress: c.contractAddress,
		}
		_ = c.transportManager.Send(ctx, &components.FireAndForgetMessageSend{
			Node:        c.activeCoordinator,
			MessageType: MessageType_HandoverRequest,
			//TODO figure out the payload
			Payload: handoverRequest.bytes(),
		})

		c.coordinator.state = CoordinatorState_Elect

	}

	return nil
}

func (c *contractSequencerAgent) HandleMissedHeartbeat(ctx context.Context) error {
	if c.activeCoordinator != "" {
		//expected a heartbeat from the coordinator
		c.missedHeartbeats++
		if c.missedHeartbeats >= c.heartbeatFailureThreshold {
			c.coordinatorSelector.Eliminate(ctx, c.activeCoordinator)
			c.selectNewCoordinator(ctx)
			err := c.delegateAllTransactions(ctx)
			if err != nil {
				return err
			}
		}
	} else if c.coordinator.state != CoordinatorState_Idle {
		// we should be sending a heartbeat
		if c.coordinator.state == CoordinatorState_Closing {
			if c.coordinator.heartbeatIntervalsSinceStateChange >= c.heartbeatFailureThreshold {
				c.coordinator.state = CoordinatorState_Idle
				return nil
			}
			c.coordinator.heartbeatIntervalsSinceStateChange++
		}
		c.sendCoordinatorHeartbeatNotifications(ctx)
	}
	return nil
}

func (c *contractSequencerAgent) HandleIndexedBlocks(ctx context.Context, blocks []*pldapi.IndexedBlock, transactions []*blockindexer.IndexedTransactionNotify) error {
	//if any of the transactions are ones that we have submitted, then we need to update our in memory record of the transaction
	// if - or when we are in flush mode, it is important to include information about confirmed transactions in the heartbeat messages

	//if we are in flush mode, we need to ensure that we are aware of all the transactions that have been confirmed
	// and we need to ensure that we have a record of all the flush points that have been submitted by the coordinator
	for _, transaction := range transactions {
		c.coordinator.handleTransactionConfirmed(ctx, transaction)
	}

	return nil
}

func (c *contractSequencerAgent) HandleEndorsementResponse(ctx context.Context, message *EndorsementResponse) error {
	//Find the transaction in question and determine if we already have an endorsement request outstanding for it
	// if not, then add this response and then run the graph analysis to determine if we can dispatch this transaction and any other
	// previously endorsed transactions in the graph that it is blocking
	if c.coordinator.state != CoordinatorState_Active {
		//There are no other states where we should be receiving endorsement responses
		// in elect, preparing, standby state, we would not have sent any endorsement requests yet
		// in idle, flush or closing state, we might have sent endorsement requests, but it is too late to do anything with the responses
		// so we should just ignore them
		log.L(ctx).Info("Received an endorsement response in an unexpected state", "state", c.coordinator.state)
		return nil
	}
	transaction, err := c.coordinator.assembledTransactions.FindTransactionByID(message.TransactionID)
	if err != nil {
		//TODO - log an error
	}
	if transaction == nil {
		//TODO - log
	}
	transaction.handleEndorsementResponse(ctx, message)
	if transaction.IsEndorsed(ctx) {
		//we have a fully endorsed transaction, send a request to confirm approval to dispatch
		c.sendDispatchConfirmationRequest(ctx, transaction)
		//TODO - record request in a map so that we can handle the response and send retries if necessary
	}
	return nil
}

func (c *contractSequencerAgent) HandleDispatchConfirmationResponse(ctx context.Context, message *DispatchConfirmationResponse) error {
	if c.coordinator.state != CoordinatorState_Active {
		//There are no other states where we should be receiving DispatchConfirmationResponse
		// in elect, preparing, standby state, we would not have sent any DispatchConfirmationRequests yet
		// in idle, flush or closing state, we might have sent DispatchConfirmationRequests, but it is too late to do anything with the responses
		// so we should just ignore them
		log.L(ctx).Info("Received an DispatchConfirmationResponse in an unexpected state", "state", c.coordinator.state)
		return nil
	}
	transaction, err := c.coordinator.assembledTransactions.FindTransactionByID(message.TransactionID)
	if err != nil {
		//TODO - log an error
		return err
	}
	if transaction == nil {
		//TODO - log
		return errors.New("transaction not found")
	}
	transaction.handleDispatchConfirmationResponse(ctx, message)

	dispatchableTransactions, err := c.coordinator.assembledTransactions.GetDispatchableTransactions(ctx, transaction.ID)
	if err != nil {
		//TODO - log an error
		//TODO we are unlikely to ask the question about this transaction again.  Should we remove it and all its dependencies from the graph?
		// what are the likely error situations that would conceivably recover from?
	}

	c.DispatchTransactions(ctx, dispatchableTransactions)

	return nil

}

func (c *contractSequencerAgent) HandleBlockHeightChange(ctx context.Context, newBlockHeight uint64) error {
	//check if the newBlockHeight results in a new block range
	c.currentBlockHeight = newBlockHeight
	blockRange := newBlockHeight / c.blockRangeSize
	if blockRange > c.currentBlockRange {
		c.currentBlockRange = newBlockHeight / c.blockRangeSize
		//given that we have a new block range, select a new coordinator
		c.coordinatorSelector.Reset(ctx)
		c.selectNewCoordinator(ctx)
		c.delegateAllTransactions(ctx)
	}
	return nil
}

func (c *contractSequencerAgent) HandleTransaction(ctx context.Context, transaction *components.PrivateTransaction) error {
	return nil
}
