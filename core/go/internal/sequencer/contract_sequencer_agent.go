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
	"fmt"

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
	GetDelegation(ctx context.Context, transactionID uuid.UUID) (*Delegation, error)
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
	delegationsByTransactionID          map[string]*Delegation
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
	defaultSignerLocator                string
	components                          components.AllComponents
}

func NewContractSequencerAgent(ctx context.Context, nodeName string, transportManager TransportManager, components components.AllComponents, coordinatorSelector CoordinatorSelector, committee []string, contractAddress *tktypes.EthAddress) ContractSequencerAgent {

	committeeMembers := make([]CommitteeMember, 0, len(committee))
	for _, member := range committee {
		committeeMembers = append(committeeMembers, CommitteeMember{
			NodeName:  member,
			Weighting: 1,
		})
	}

	coordinatorSelector.Initialize(ctx, committeeMembers)

	return &contractSequencerAgent{
		delegationsByTransactionID:          make(map[string]*Delegation),
		dispatchedTransactionsByCoordinator: make(map[string][]*DispatchedTransaction),
		confirmedTransactionsBySubmitter:    make(map[string][]*ConfirmedTransaction),
		blockRangeSize:                      1000, //TODO: make this configurable per contract
		currentBlockRange:                   0,
		currentBlockHeight:                  0,
		coordinatorSelector:                 coordinatorSelector,
		contractAddress:                     contractAddress,
		committeeMembers:                    committeeMembers,
		nodeName:                            nodeName,
		coordinator:                         NewCoordinator(ctx),
		defaultSignerLocator:                fmt.Sprintf("domains.%s.submit.%s", contractAddress, uuid.New()),
		components:                          components,
		transportManager:                    transportManager,
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

func (c *contractSequencerAgent) GetDelegation(ctx context.Context, transactionID uuid.UUID) (*Delegation, error) {
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
