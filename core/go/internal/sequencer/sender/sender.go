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

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/metrics"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/sender/transaction"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/transport"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

type SeqSender interface {
	// Asynchronously update the state machine by queueing an event to be processed. Most
	// callers should use this interface.
	QueueEvent(ctx context.Context, event common.Event) error

	// Synchronously update the state machine by processing this event. Primarily used for testing the state machine.
	ProcessEvent(ctx context.Context, event common.Event) error

	SetActiveCoordinator(ctx context.Context, coordinator string) error
	GetTxStatus(ctx context.Context, txID uuid.UUID) (status components.PrivateTxStatus, err error)
	Stop()
}

type sender struct {
	/* State */
	stateMachine                *StateMachine
	activeCoordinatorNode       string
	timeOfMostRecentHeartbeat   common.Time
	transactionsByID            map[uuid.UUID]*transaction.Transaction
	submittedTransactionsByHash map[pldtypes.Bytes32]*uuid.UUID
	transactionsOrdered         []*uuid.UUID
	currentBlockHeight          uint64
	latestCoordinatorSnapshot   *common.CoordinatorSnapshot

	/* Config */
	nodeName             string
	blockRangeSize       uint64
	contractAddress      *pldtypes.EthAddress
	heartbeatThresholdMs common.Duration

	/* Dependencies */
	transportWriter   transport.TransportWriter
	clock             common.Clock
	engineIntegration common.EngineIntegration
	metrics           metrics.DistributedSequencerMetrics

	/* Event loop */
	senderEvents  chan common.Event
	stopEventLoop chan struct{}
}

func NewSender(
	ctx context.Context,
	nodeName string,
	transportWriter transport.TransportWriter,
	clock common.Clock,
	engineIntegration common.EngineIntegration,
	blockRangeSize uint64,
	contractAddress *pldtypes.EthAddress,
	heartbeatPeriodMs int,
	heartbeatThresholdIntervals int,
	metrics metrics.DistributedSequencerMetrics,
) (*sender, error) {
	s := &sender{
		nodeName:                    nodeName,
		transactionsByID:            make(map[uuid.UUID]*transaction.Transaction),
		submittedTransactionsByHash: make(map[pldtypes.Bytes32]*uuid.UUID),
		transportWriter:             transportWriter,
		blockRangeSize:              blockRangeSize,
		contractAddress:             contractAddress,
		clock:                       clock,
		engineIntegration:           engineIntegration,
		heartbeatThresholdMs:        clock.Duration(heartbeatPeriodMs * heartbeatThresholdIntervals),
		metrics:                     metrics,
		senderEvents:                make(chan common.Event, 1),
		stopEventLoop:               make(chan struct{}),
	}
	s.InitializeStateMachine(State_Idle)
	go s.eventLoop(ctx)
	return s, nil
}

func (s *sender) eventLoop(ctx context.Context) error {
	for {
		log.L(ctx).Infof("[Sequencer] sender event loop waiting for next event")
		select {
		case event := <-s.senderEvents:
			log.L(ctx).Infof("[Sequencer] sender pulled event from the queue: %s", event.TypeString())
			s.ProcessEvent(ctx, event)
		case <-s.stopEventLoop:
			log.L(ctx).Infof("[Sequencer] sender event loop cancelled")
			return i18n.NewError(ctx, msgs.MsgContextCanceled)
		}
	}
}

func (s *sender) propagateEventToTransaction(ctx context.Context, event transaction.Event) error {
	if txn := s.transactionsByID[event.GetTransactionID()]; txn != nil {
		return txn.HandleEvent(ctx, event)
	} else {
		log.L(ctx).Debugf("[Sequencer] ignoring event because transaction not known to this sender %s", event.GetTransactionID().String())
	}
	return nil
}

func (s *sender) createTransaction(ctx context.Context, txn *components.PrivateTransaction) error {
	newTxn, err := transaction.NewTransaction(ctx, txn, s.transportWriter, s.ProcessEvent, s.engineIntegration, s.metrics)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] error creating transaction: %v", err)
		return err
	}
	s.transactionsByID[txn.ID] = newTxn
	s.transactionsOrdered = append(s.transactionsOrdered, &txn.ID)
	createdEvent := &transaction.CreatedEvent{}
	createdEvent.TransactionID = txn.ID
	err = newTxn.HandleEvent(ctx, createdEvent)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] error handling CreatedEvent for transaction %s: %v", txn.ID.String(), err)
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

// A sequencer can be asked to page itself out at any time to make space for other sequencers.
// This hook point provides a place to perform any tidy up actions needed in the sender
func (s *sender) Stop() {
	log.L(context.Background()).Infof("Stopping sender for contract %s", s.contractAddress.String())
	s.stopEventLoop <- struct{}{}
}

//TODO the following getter methods are not safe to call on anything other than the sequencer goroutine because they are reading data structures that are being modified by the state machine.
// We should consider making them safe to call from any goroutine by reading maintaining a copy of the data structures that are updated async from the sequencer thread under a mutex

func (s *sender) GetCurrentState() State {
	return s.stateMachine.currentState
}
