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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
)

// This is a subset of the components.TransportManager interface
type TransportManager interface {
	Send(ctx context.Context, send *components.FireAndForgetMessageSend) error
	SendReliable(ctx context.Context, dbTX *gorm.DB, msg ...*components.ReliableMessage) (preCommit func(), err error)
}

// ContractSequencerAgent defines the interface for an in memory component that maintains
// the state machine for a single paladin private contract in a single node
type ContractSequencerAgent interface {
	IsObserver() bool
	IsCoordinator() bool
	IsSender() bool
	ActiveCoordinator() string
	HandleCoordinatorHeartbeatNotification(ctx context.Context, message *CoordinatorHeartbeatNotification) error
	HandleTransaction(ctx context.Context, transaction *components.PrivateTransaction) error
	HandleMissedHeartbeat(ctx context.Context) error
	DispatchedTransactions(ctx context.Context) []*DispatchedTransaction
}

type ContractSequencerHeartbeatTicker interface {
	C() <-chan struct{}
}

type delegation struct {
	transaction *components.PrivateTransaction
}

type DispatchedTransaction struct {
	TransactionID        uuid.UUID
	Signer               tktypes.EthAddress
	LatestSubmissionHash []byte
	Nonce                uint64
}

type contractSequencerAgent struct {
	isCoordinator                       bool
	isSender                            bool
	activeCoordinator                   string
	missedHeartbeats                    int
	heartbeatFailureThreshold           int
	delegationsByTransactionID          map[string]*delegation
	dispatchedTransactionsByCoordinator map[string][]*DispatchedTransaction
	transportManager                    TransportManager
}

func (c *contractSequencerAgent) IsObserver() bool {
	return true
}

func (c *contractSequencerAgent) IsCoordinator() bool {
	return c.isCoordinator
}

func (c *contractSequencerAgent) IsSender() bool {
	return c.isSender
}

func (c *contractSequencerAgent) delegateAllTransactions(ctx context.Context) error {
	//re-delegate all transactions to the new coordinator
	transactions := make([]*components.PrivateTransaction, 0, len(c.delegationsByTransactionID))
	for _, delegation := range c.delegationsByTransactionID {
		transactions = append(transactions, delegation.transaction)
	}
	//send delegation request
	delegationRequest := DelegationRequest{
		Transactions: transactions,
	}
	_ = c.transportManager.Send(ctx, &components.FireAndForgetMessageSend{
		Node:        c.activeCoordinator,
		MessageType: "DelegationRequest",
		//TODO figure out the payload
		Payload: delegationRequest.bytes(),
	})
	return nil
}

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

func (c *contractSequencerAgent) HandleMissedHeartbeat(ctx context.Context) error {

	if c.activeCoordinator != "" {
		c.missedHeartbeats++
		//expected a heartbeat from the coordinator
		c.missedHeartbeats++
		if c.missedHeartbeats >= c.heartbeatFailureThreshold {
			c.activeCoordinator = ""
			err := c.delegateAllTransactions(ctx)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *contractSequencerAgent) ActiveCoordinator() string {
	return c.activeCoordinator
}

func (c *contractSequencerAgent) DispatchedTransactions(ctx context.Context) []*DispatchedTransaction {
	dispatchedTransactions := make([]*DispatchedTransaction, 0)
	for _, transactions := range c.dispatchedTransactionsByCoordinator {
		dispatchedTransactions = append(dispatchedTransactions, transactions...)
	}
	return dispatchedTransactions
}

func (c *contractSequencerAgent) HandleTransaction(ctx context.Context, transaction *components.PrivateTransaction) error {
	return nil
}

func NewContractSequencerAgent(transportManager TransportManager) ContractSequencerAgent {
	return &contractSequencerAgent{
		delegationsByTransactionID:          make(map[string]*delegation),
		dispatchedTransactionsByCoordinator: make(map[string][]*DispatchedTransaction),
		transportManager:                    transportManager,
	}
}
