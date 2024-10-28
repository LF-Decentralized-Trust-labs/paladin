/*
 * Copyright © 2024 Kaleido, Inc.
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

package privatetxnmgr

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/kaleido-io/paladin/core/internal/privatetxnmgr/syncpoints"
	"github.com/kaleido-io/paladin/core/internal/statedistribution"

	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

/*
 * This file contains the lifecycle functions and the basic entry points to a sequencer.
 * A sequencer can be created for a specific contract address and is responsible for maintaining an in-memory copy of the state of all inflight transactions for that contract.
 * It is expected that there will never be more than one sequencer for any given contract address in memory at one time.
 */
type SequencerState string

const (
	// brand new sequencer
	SequencerStateNew SequencerState = "new"
	// sequencer running normally
	SequencerStateRunning SequencerState = "running"
	// sequencer is blocked and waiting for precondition to be fulfilled, e.g. pre-req tx blocking current stage
	SequencerStateWaiting SequencerState = "waiting"
	// transactions managed by an sequencer stuck in the same state
	SequencerStateStale SequencerState = "stale"
	// no transactions in a specific sequencer
	SequencerStateIdle SequencerState = "idle"
	// sequencer is paused
	SequencerStatePaused SequencerState = "paused"
	// sequencer is stopped
	SequencerStateStopped SequencerState = "stopped"
)

var AllSequencerStates = []string{
	string(SequencerStateNew),
	string(SequencerStateRunning),
	string(SequencerStateWaiting),
	string(SequencerStateStale),
	string(SequencerStateIdle),
	string(SequencerStatePaused),
	string(SequencerStateStopped),
}

type Sequencer struct {
	ctx                     context.Context
	persistenceRetryTimeout time.Duration

	// each sequencer has its own go routine
	initiated    time.Time     // when sequencer is created
	evalInterval time.Duration // between how long the sequencer will do an evaluation to check & remove transactions that missed events

	maxConcurrentProcess        int
	incompleteTxProcessMapMutex sync.Mutex
	incompleteTxSProcessMap     map[string]ptmgrtypes.TransactionFlow // a map of all known transactions that are not completed

	processedTxIDs    map[string]bool // an internal record of completed transactions to handle persistence delays that causes reprocessing
	sequencerLoopDone chan struct{}

	// input channels
	orchestrationEvalRequestChan chan bool
	stopProcess                  chan bool // a channel to tell the current sequencer to stop processing all events and mark itself as to be deleted

	// Metrics provided for fairness control in the controller
	totalCompleted int64 // total number of transaction completed since initiated
	state          SequencerState
	stateEntryTime time.Time // when the sequencer entered the current state

	staleTimeout time.Duration

	pendingEvents chan ptmgrtypes.PrivateTransactionEvent

	contractAddress     tktypes.EthAddress // the contract address managed by the current sequencer
	nodeID              string
	domainAPI           components.DomainSmartContract
	components          components.AllComponents
	endorsementGatherer ptmgrtypes.EndorsementGatherer
	publisher           ptmgrtypes.Publisher
	identityResolver    components.IdentityResolver
	syncPoints          syncpoints.SyncPoints
	stateDistributer    statedistribution.StateDistributer
	transportWriter     ptmgrtypes.TransportWriter
	graph               Graph
	requestTimeout      time.Duration
}

func NewSequencer(
	ctx context.Context,
	nodeID string,
	contractAddress tktypes.EthAddress,
	sequencerConfig *pldconf.PrivateTxManagerSequencerConfig,
	allComponents components.AllComponents,
	domainAPI components.DomainSmartContract,
	endorsementGatherer ptmgrtypes.EndorsementGatherer,
	publisher ptmgrtypes.Publisher,
	syncPoints syncpoints.SyncPoints,
	identityResolver components.IdentityResolver,
	stateDistributer statedistribution.StateDistributer,
	transportWriter ptmgrtypes.TransportWriter,
	requestTimeout time.Duration,
) *Sequencer {

	newSequencer := &Sequencer{
		ctx:                  log.WithLogField(ctx, "role", fmt.Sprintf("sequencer-%s", contractAddress)),
		initiated:            time.Now(),
		contractAddress:      contractAddress,
		evalInterval:         confutil.DurationMin(sequencerConfig.EvaluationInterval, 1*time.Millisecond, *pldconf.PrivateTxManagerDefaults.Sequencer.EvaluationInterval),
		maxConcurrentProcess: confutil.Int(sequencerConfig.MaxConcurrentProcess, *pldconf.PrivateTxManagerDefaults.Sequencer.MaxConcurrentProcess),
		state:                SequencerStateNew,
		stateEntryTime:       time.Now(),

		incompleteTxSProcessMap: make(map[string]ptmgrtypes.TransactionFlow),
		persistenceRetryTimeout: confutil.DurationMin(sequencerConfig.PersistenceRetryTimeout, 1*time.Millisecond, *pldconf.PrivateTxManagerDefaults.Sequencer.PersistenceRetryTimeout),

		staleTimeout:                 confutil.DurationMin(sequencerConfig.StaleTimeout, 1*time.Millisecond, *pldconf.PrivateTxManagerDefaults.Sequencer.StaleTimeout),
		processedTxIDs:               make(map[string]bool),
		orchestrationEvalRequestChan: make(chan bool, 1),
		stopProcess:                  make(chan bool, 1),
		pendingEvents:                make(chan ptmgrtypes.PrivateTransactionEvent, *pldconf.PrivateTxManagerDefaults.Sequencer.MaxPendingEvents),
		nodeID:                       nodeID,
		domainAPI:                    domainAPI,
		components:                   allComponents,
		endorsementGatherer:          endorsementGatherer,
		publisher:                    publisher,
		syncPoints:                   syncPoints,
		identityResolver:             identityResolver,
		stateDistributer:             stateDistributer,
		transportWriter:              transportWriter,
		graph:                        NewGraph(),
		requestTimeout:               requestTimeout,
	}

	log.L(ctx).Debugf("NewSequencer for contract address %s created: %+v", newSequencer.contractAddress, newSequencer)

	return newSequencer
}

func (s *Sequencer) abort(err error) {
	log.L(s.ctx).Errorf("Sequencer aborting: %s", err)
	s.stopProcess <- true

}
func (s *Sequencer) getTransactionProcessor(txID string) ptmgrtypes.TransactionFlow {
	s.incompleteTxProcessMapMutex.Lock()
	defer s.incompleteTxProcessMapMutex.Unlock()
	transactionProcessor, ok := s.incompleteTxSProcessMap[txID]
	if !ok {
		log.L(s.ctx).Errorf("Transaction processor not found for transaction ID %s", txID)
		return nil
	}
	return transactionProcessor
}

func (s *Sequencer) removeTransactionProcessor(txID string) {
	s.incompleteTxProcessMapMutex.Lock()
	defer s.incompleteTxProcessMapMutex.Unlock()
	delete(s.incompleteTxSProcessMap, txID)
}

func (s *Sequencer) ProcessNewTransaction(ctx context.Context, tx *components.PrivateTransaction) (queued bool) {
	s.incompleteTxProcessMapMutex.Lock()
	defer s.incompleteTxProcessMapMutex.Unlock()
	if s.incompleteTxSProcessMap[tx.ID.String()] == nil {
		if len(s.incompleteTxSProcessMap) >= s.maxConcurrentProcess {
			// TODO: decide how this map is managed, it shouldn't track the entire lifecycle
			// tx processing pool is full, queue the item
			return true
		} else {
			s.incompleteTxSProcessMap[tx.ID.String()] = NewTransactionFlow(ctx, tx, s.nodeID, s.components, s.domainAPI, s.publisher, s.endorsementGatherer, s.identityResolver, s.syncPoints, s.transportWriter, s.requestTimeout)
		}
		s.pendingEvents <- &ptmgrtypes.TransactionSubmittedEvent{
			PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{TransactionID: tx.ID.String()},
		}
	}
	return false
}

func (s *Sequencer) ProcessInFlightTransaction(ctx context.Context, tx *components.PrivateTransaction) (queued bool) {
	log.L(ctx).Infof("Processing in flight transaction %s", tx.ID)
	//a transaction that already has had some processing done on it
	// currently the only case this can happen is a transaction delegated from another node
	// but maybe in future, inflight transactions being coordinated locally could be swapped out of memory when they are blocked and/or if we are at max concurrency
	s.incompleteTxProcessMapMutex.Lock()
	defer s.incompleteTxProcessMapMutex.Unlock()
	_, alreadyInMemory := s.incompleteTxSProcessMap[tx.ID.String()]
	if alreadyInMemory {
		log.L(ctx).Warnf("Transaction %s already in memory. Ignoring", tx.ID)
		return false
	}
	if s.incompleteTxSProcessMap[tx.ID.String()] == nil {
		if len(s.incompleteTxSProcessMap) >= s.maxConcurrentProcess {
			// TODO: decide how this map is managed, it shouldn't track the entire lifecycle
			// tx processing pool is full, queue the item
			return true
		} else {
			s.incompleteTxSProcessMap[tx.ID.String()] = NewTransactionFlow(ctx, tx, s.nodeID, s.components, s.domainAPI, s.publisher, s.endorsementGatherer, s.identityResolver, s.syncPoints, s.transportWriter, s.requestTimeout)
		}
		s.pendingEvents <- &ptmgrtypes.TransactionSwappedInEvent{
			PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{TransactionID: tx.ID.String()},
		}
	}
	return false
}

func (s *Sequencer) HandleEvent(ctx context.Context, event ptmgrtypes.PrivateTransactionEvent) {
	s.pendingEvents <- event
}

func (s *Sequencer) Start(c context.Context) (done <-chan struct{}, err error) {
	s.syncPoints.Start()
	s.sequencerLoopDone = make(chan struct{})
	go s.evaluationLoop()
	s.TriggerSequencerEvaluation()
	return s.sequencerLoopDone, nil
}

// Stop the InFlight transaction process.
func (s *Sequencer) Stop() {
	// try to send an item in `stopProcess` channel, which has a buffer of 1
	// if it already has an item in the channel, this function does nothing
	select {
	case s.stopProcess <- true:
	default:
	}

}

func (s *Sequencer) TriggerSequencerEvaluation() {
	// try to send an item in `processNow` channel, which has a buffer of 1
	// if it already has an item in the channel, this function does nothing
	select {
	case s.orchestrationEvalRequestChan <- true:
	default:
	}
}

func (s *Sequencer) GetTxStatus(ctx context.Context, txID string) (status components.PrivateTxStatus, err error) {
	//TODO This is primarily here to help with testing for now
	// this needs to be revisited ASAP as part of a holisitic review of the persistence model
	s.incompleteTxProcessMapMutex.Lock()
	defer s.incompleteTxProcessMapMutex.Unlock()
	if txProc, ok := s.incompleteTxSProcessMap[txID]; ok {
		return txProc.GetTxStatus(ctx)
	}
	//TODO should be possible to query the status of a transaction that is not inflight
	return components.PrivateTxStatus{}, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "Transaction not found")
}
