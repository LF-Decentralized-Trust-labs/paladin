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

	"github.com/kaleido-io/paladin/core/internal/components"
)

func (c *contractSequencerAgent) selectNewCoordinator(ctx context.Context) {
	var err error
	c.activeCoordinator, err = c.coordinatorSelector.Select(ctx, c.currentBlockRange)
	if err != nil {
		//TODO - should we return the error here or is there a sensible handling strategy?
		c.activeCoordinator = ""
	}
	return
}

func (c *contractSequencerAgent) delegateAllTransactions(ctx context.Context) error {
	//re-delegate all transactions to the new coordinator
	transactions := make([]*components.PrivateTransaction, 0, len(c.delegationsByTransactionID))
	for _, delegation := range c.delegationsByTransactionID {
		transactions = append(transactions, delegation.Txn())
	}
	//send delegation request
	delegationRequest := DelegationRequest{
		ContractAddress: c.contractAddress,
		Transactions:    transactions,
	}
	_ = c.transportManager.Send(ctx, &components.FireAndForgetMessageSend{
		Node:        c.activeCoordinator,
		MessageType: MessageType_DelegationRequest,
		//TODO figure out the payload
		Payload: delegationRequest.bytes(),
	})
	return nil
}

func (c *contractSequencerAgent) sendCoordinatorHeartbeatNotifications(ctx context.Context) error {
	switch c.coordinator.state {
	case CoordinatorState_Elect:
	case CoordinatorState_Prepared:
	case CoordinatorState_Active:
	case CoordinatorState_Flush:
	case CoordinatorState_Closing:
	}
	//send a heartbeat to each of the other nodes
	for _, node := range c.Committee() {
		if node.NodeName == c.nodeName {
			continue
		}
		//send a heartbeat

		pooledTransactions, err := c.getPooledTransactionsForSender(ctx, node.NodeName)
		if err != nil {
			//TODO
		}

		dispatchedTransactions, err := c.getTransactionsDispatchedLocally(ctx)
		if err != nil {
			//TODO
		}

		var confirmedTransactions []*ConfirmedTransaction = nil
		var flushPoints []*FlushPoint = nil
		if c.coordinator.state == CoordinatorState_Flush {
			confirmedTransactions, err = c.getConfirmedTransactionsSubmittedLocally(ctx)
			if err != nil {
				//TODO
			}
			flushPoints, err = c.getFlushPoints(ctx)
			if err != nil {
				//TODO
			}
		}

		coordinatorHeartbeatMessage := &CoordinatorHeartbeatNotification{
			From:                   c.activeCoordinator,
			ContractAddress:        c.contractAddress,
			CoordinatorState:       c.coordinator.state,
			BlockHeight:            c.currentBlockHeight,
			FlushPoints:            flushPoints,
			PooledTransactions:     pooledTransactions,
			DispatchedTransactions: dispatchedTransactions,
			ConfirmedTransactions:  confirmedTransactions,
		}

		c.transportManager.Send(ctx, &components.FireAndForgetMessageSend{
			Node:        node.NodeName,
			MessageType: MessageType_CoordinatorHeartbeatNotification,
			Payload:     coordinatorHeartbeatMessage.bytes(),
		})
	}
	return nil
}

func (c *contractSequencerAgent) sendDispatchConfirmationRequest(ctx context.Context, transaction *Delegation) {

	transactionHash, err := transaction.Hash()
	if err != nil {
		//TODO
	}

	request := &DispatchConfirmationRequest{

		ContractAddress: c.contractAddress,
		Coordinator:     c.nodeName,
		TransactionID:   transaction.ID,
		TransactionHash: transactionHash,
	}

	c.transportManager.Send(ctx, &components.FireAndForgetMessageSend{
		Node:        transaction.Sender(),
		MessageType: MessageType_DispatchConfirmationRequest,
		Payload:     request.bytes(),
	})
}
