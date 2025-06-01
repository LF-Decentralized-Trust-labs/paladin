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

package sender

import (
	"context"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/sender/transaction"

	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type sender struct {
	/* State */
	stateMachine                 *StateMachine
	activeCoordinator            string
	activeCoordinatorBlockHeight uint64
	timeOfMostRecentHeartbeat    common.Time
	transactionsByID             map[uuid.UUID]*transaction.Transaction
	submittedTransactionsByHash  map[tktypes.Bytes32]*uuid.UUID
	transactionsOrdered          []*uuid.UUID
	currentBlockHeight           uint64
	latestCoordinatorSnapshot    *common.CoordinatorSnapshot

	/* Config */
	nodeName             string
	blockRangeSize       uint64
	contractAddress      *tktypes.EthAddress
	blockHeightTolerance uint64
	committee            map[string][]string
	heartbeatThresholdMs common.Duration

	/* Dependencies */
	messageSender     MessageSender
	clock             common.Clock
	engineIntegration common.EngineIntegration
	emit              common.EmitEvent
}

func NewSender(
	ctx context.Context,
	nodeName string,
	messageSender MessageSender,
	committeeMembers []string,
	clock common.Clock,
	emit common.EmitEvent,
	engineIntegration common.EngineIntegration,
	blockRangeSize uint64,
	contractAddress *tktypes.EthAddress,
	heartbeatPeriodMs int,
	heartbeatThresholdIntervals int,
) (*sender, error) {
	s := &sender{
		nodeName:                    nodeName,
		transactionsByID:            make(map[uuid.UUID]*transaction.Transaction),
		submittedTransactionsByHash: make(map[tktypes.Bytes32]*uuid.UUID),
		messageSender:               messageSender,
		blockRangeSize:              blockRangeSize,
		contractAddress:             contractAddress,
		clock:                       clock,
		engineIntegration:           engineIntegration,
		emit:                        emit,
		heartbeatThresholdMs:        clock.Duration(heartbeatPeriodMs * heartbeatThresholdIntervals),
	}
	s.committee = make(map[string][]string)
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

		if _, ok := s.committee[memberNode]; !ok {
			s.committee[memberNode] = make([]string, 0)
		}

		s.committee[memberNode] = append(s.committee[memberNode], memberIdentity)

	}
	s.InitializeStateMachine(State_Idle)
	return s, nil
}

func (s *sender) propagateEventToTransaction(ctx context.Context, event transaction.Event) error {
	if txn := s.transactionsByID[event.GetTransactionID()]; txn != nil {
		return txn.HandleEvent(ctx, event)
	} else {
		log.L(ctx).Debugf("Ignoring Event because transaction not known to this coordinator %s", event.GetTransactionID().String())
	}
	return nil
}

func (s *sender) createTransaction(ctx context.Context, txn *components.PrivateTransaction) error {
	newTxn, err := transaction.NewTransaction(ctx, txn, s.messageSender, s.clock, s.emit, s.engineIntegration)
	if err != nil {
		log.L(ctx).Errorf("Error creating transaction: %v", err)
		return err
	}
	s.transactionsByID[txn.ID] = newTxn
	s.transactionsOrdered = append(s.transactionsOrdered, &txn.ID)
	createdEvent := &transaction.CreatedEvent{}
	createdEvent.TransactionID = txn.ID
	err = newTxn.HandleEvent(context.Background(), createdEvent)
	if err != nil {
		log.L(ctx).Errorf("Error handling CreatedEvent for transaction %s: %v", txn.ID.String(), err)
		return err
	}
	return nil
}

func (s *sender) transactionsOrderedByCreatedTime(ctx context.Context) ([]*transaction.Transaction, error) {
	//TODO are we actually saving anything by transactionsOrdered being an array of IDs rather than an array of *transaction.Transaction
	ordered := make([]*transaction.Transaction, len(s.transactionsOrdered))
	for i, id := range s.transactionsOrdered {
		ordered[i] = s.transactionsByID[*id]
	}
	return ordered, nil
}

func (s *sender) getTransactionsInStates(ctx context.Context, states []transaction.State) []*transaction.Transaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	matchingStates := make(map[transaction.State]bool)
	for _, state := range states {
		matchingStates[state] = true
	}
	matchingTxns := make([]*transaction.Transaction, 0, len(s.transactionsByID))
	for _, txn := range s.transactionsByID {
		if matchingStates[txn.GetCurrentState()] {
			matchingTxns = append(matchingTxns, txn)
		}
	}
	return matchingTxns
}

func (s *sender) getTransactionsNotInStates(ctx context.Context, states []transaction.State) []*transaction.Transaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	nonMatchingStates := make(map[transaction.State]bool)
	for _, state := range states {
		nonMatchingStates[state] = true
	}
	matchingTxns := make([]*transaction.Transaction, 0, len(s.transactionsByID))
	for _, txn := range s.transactionsByID {
		if !nonMatchingStates[txn.GetCurrentState()] {
			matchingTxns = append(matchingTxns, txn)
		}
	}
	return matchingTxns
}

func ptrTo[T any](v T) *T {
	return &v
}

//TODO the following getter methods are not safe to call on anything other than the sequencer goroutine because they are reading data structures that are being modified by the state machine.
// We should consider making them safe to call from any goroutine by reading maintaining a copy of the data structures that are updated async from the sequencer thread under a mutex

func (s *sender) GetCurrentState() State {
	return s.stateMachine.currentState
}
