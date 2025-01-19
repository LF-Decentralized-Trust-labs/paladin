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
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
)

// This is a subset of the components.TransportManager interface
type TransportManager interface {
	Send(ctx context.Context, send *components.FireAndForgetMessageSend) error
	SendReliable(ctx context.Context, dbTX *gorm.DB, msg ...*components.ReliableMessage) (preCommit func(), err error)
}

type TransactionState string

const (
	TransactionState_Pooled                TransactionState = "TransactionState_Pooled"
	TransactionState_Assembled             TransactionState = "TransactionState_Assembled"
	TransactionState_ConfirmingForDispatch TransactionState = "TransactionState_ConfirmingForDispatch"
	TransactionState_Dispatched            TransactionState = "TransactionState_Dispatched"
	TransactionState_Submitted             TransactionState = "TransactionState_Submitted"
	TransactionState_Committed             TransactionState = "TransactionState_Committed"
	TransactionState_Rejected              TransactionState = "TransactionState_Rejected"
	TransactionState_ConfirmedSuccess      TransactionState = "TransactionState_ConfirmedSuccess"
	TransactionState_ConfirmedReverted     TransactionState = "TransactionState_ConfirmedReverted"
)

// ContractSequencerAgent defines the interface for an in memory component that maintains
// the state machine for a single paladin private contract in a single node
type ContractSequencerAgent interface {
	IsObserver() bool
	IsCoordinator() bool
	IsSender() bool
	ActiveCoordinator() string
	CoordinatorState() CoordinatorState
	HandleCoordinatorHeartbeatNotification(ctx context.Context, message *CoordinatorHeartbeatNotification) error
	HandleDelegationRequest(ctx context.Context, message *DelegationRequest) error
	HandleTransaction(ctx context.Context, transaction *components.PrivateTransaction) error
	HandleMissedHeartbeat(ctx context.Context) error
	HandleIndexedBlocks(ctx context.Context, blocks []*pldapi.IndexedBlock, transactions []*blockindexer.IndexedTransactionNotify) error
	DispatchedTransactions(ctx context.Context) []*DispatchedTransaction
	BlockHeight() uint64
	Committee() []CommitteeMember
	GetDelegation(ctx context.Context, transactionID uuid.UUID) (*delegation, error)
}

type ContractSequencerHeartbeatTicker interface {
	C() <-chan struct{}
}

type FlushPoint struct {
	TransactionID uuid.UUID
	Hash          tktypes.Bytes32
}

type contractSequencerAgent struct {
	isSender                            bool
	activeCoordinator                   string
	missedHeartbeats                    int
	heartbeatFailureThreshold           int
	delegationsByTransactionID          map[string]*delegation
	dispatchedTransactionsByCoordinator map[string][]*DispatchedTransaction // all transactions that have been dispatched by any recently active coordinator, ( including the local node)
	confirmedTransactionsBySubmitter    map[string][]*ConfirmedTransaction  //all transactions that have recently been confirmed, indexed by which coordinator submitted them ( including the local node)
	transportManager                    TransportManager
	blockRangeSize                      uint64
	currentBlockRange                   uint64
	currentBlockHeight                  uint64
	coordinatorSelector                 CoordinatorSelector
	coordinator                         *coordinator
	contractAddress                     *tktypes.EthAddress
	committeeMembers                    []CommitteeMember
	nodeName                            string
	flushPointsBySignerAddress          map[string]*FlushPoint
}

func NewContractSequencerAgent(ctx context.Context, nodeName string, transportManager TransportManager, coordinatorSelector CoordinatorSelector, committee []string, contractAddress *tktypes.EthAddress) ContractSequencerAgent {

	committeeMembers := make([]CommitteeMember, 0, len(committee))
	for _, member := range committee {
		committeeMembers = append(committeeMembers, CommitteeMember{
			NodeName:  member,
			Weighting: 1,
		})
	}

	coordinatorSelector.Initialize(ctx, committeeMembers)

	return &contractSequencerAgent{
		delegationsByTransactionID:          make(map[string]*delegation),
		dispatchedTransactionsByCoordinator: make(map[string][]*DispatchedTransaction),
		confirmedTransactionsBySubmitter:    make(map[string][]*ConfirmedTransaction),
		transportManager:                    transportManager,
		blockRangeSize:                      1000, //TODO: make this configurable per contract
		currentBlockRange:                   0,
		currentBlockHeight:                  0,
		coordinatorSelector:                 coordinatorSelector,
		contractAddress:                     contractAddress,
		committeeMembers:                    committeeMembers,
		nodeName:                            nodeName,
		coordinator:                         NewCoordinator(ctx),
	}
}

func (c *contractSequencerAgent) IsObserver() bool {
	return true
}

func (c *contractSequencerAgent) IsCoordinator() bool {
	return c.coordinator.state != CoordinatorState_Idle
}

func (c *contractSequencerAgent) CoordinatorState() CoordinatorState {
	return c.coordinator.state
}

func (c *contractSequencerAgent) IsSender() bool {
	return c.isSender
}

func (c *contractSequencerAgent) Committee() []CommitteeMember {
	return c.committeeMembers
}

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

func (c *contractSequencerAgent) HandleDispatchConfirmationResponse(ctx context.Context, message *DispatchConfirmationResponse) error {
	//TODO should there be some validation that the response message was sent by the sender for this transaction?
	//TODO
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

func (c *contractSequencerAgent) ActiveCoordinator() string {
	return c.activeCoordinator
}

func (c *contractSequencerAgent) BlockHeight() uint64 {
	return c.currentBlockHeight
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

func (c *contractSequencerAgent) GetDelegation(ctx context.Context, transactionID uuid.UUID) (*delegation, error) {
	if delegation, ok := c.delegationsByTransactionID[transactionID.String()]; ok {
		return delegation, nil
	}
	return nil, nil
}

func (c *contractSequencerAgent) getPooledTransactionsForSender(ctx context.Context, sender string) ([]*pooledTransaction, error) {
	return c.coordinator.pooledTransactions.GetTransactionsForSender(ctx, sender)
}

func (c *contractSequencerAgent) getTransactionsDispatchedLocally(_ context.Context) ([]*DispatchedTransaction, error) {
	return c.dispatchedTransactionsByCoordinator[c.nodeName], nil
}

func (c *contractSequencerAgent) getConfirmedTransactionsSubmittedLocally(_ context.Context) ([]*ConfirmedTransaction, error) {
	return c.confirmedTransactionsBySubmitter[c.nodeName], nil
}

func (c *contractSequencerAgent) getFlushPoints(_ context.Context) ([]*FlushPoint, error) {
	flushPoints := make([]*FlushPoint, 0, len(c.flushPointsBySignerAddress))
	for _, flushPoint := range c.flushPointsBySignerAddress {
		flushPoints = append(flushPoints, flushPoint)
	}
	return flushPoints, nil
}
