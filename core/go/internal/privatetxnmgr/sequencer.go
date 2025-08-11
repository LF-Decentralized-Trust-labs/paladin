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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr/syncpoints"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
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

type sequencerEnvironment struct {
	blockHeight int64
}

func (e *sequencerEnvironment) GetBlockHeight() int64 {
	return e.blockHeight
}

type Sequencer struct {
	ctx              context.Context
	privateTxManager components.PrivateTxManager

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

	pendingTransactionEvents chan ptmgrtypes.PrivateTransactionEvent

	contractAddress          pldtypes.EthAddress // the contract address managed by the current sequencer
	defaultSigner            string
	nodeName                 string
	domainAPI                components.DomainSmartContract
	coordinatorDomainContext components.DomainContext
	delegateDomainContext    components.DomainContext
	components               components.AllComponents
	endorsementGatherer      ptmgrtypes.EndorsementGatherer
	publisher                ptmgrtypes.Publisher
	identityResolver         components.IdentityResolver
	syncPoints               syncpoints.SyncPoints
	transportWriter          ptmgrtypes.TransportWriter
	graph                    Graph
	requestTimeout           time.Duration
	coordinatorSelector      ptmgrtypes.CoordinatorSelector
	newBlockEvents           chan int64
	assembleCoordinator      ptmgrtypes.AssembleCoordinator
	environment              *sequencerEnvironment
}

func NewSequencer(
	ctx context.Context,
	privateTxManager components.PrivateTxManager,
	nodeName string,
	contractAddress pldtypes.EthAddress,
	sequencerConfig *pldconf.PrivateTxManagerSequencerConfig,
	allComponents components.AllComponents,
	domainAPI components.DomainSmartContract,
	endorsementGatherer ptmgrtypes.EndorsementGatherer,
	publisher ptmgrtypes.Publisher,
	syncPoints syncpoints.SyncPoints,
	identityResolver components.IdentityResolver,
	transportWriter ptmgrtypes.TransportWriter,
	requestTimeout time.Duration,
	blockHeight int64,

) (*Sequencer, error) {

	newSequencer := &Sequencer{
		ctx:                  log.WithLogField(ctx, "role", fmt.Sprintf("sequencer-%s", contractAddress)),
		privateTxManager:     privateTxManager,
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
		pendingTransactionEvents:     make(chan ptmgrtypes.PrivateTransactionEvent, *pldconf.PrivateTxManagerDefaults.Sequencer.MaxPendingEvents),
		nodeName:                     nodeName,
		domainAPI:                    domainAPI,
		components:                   allComponents,
		endorsementGatherer:          endorsementGatherer,
		publisher:                    publisher,
		syncPoints:                   syncPoints,
		identityResolver:             identityResolver,
		transportWriter:              transportWriter,
		graph:                        NewGraph(),
		requestTimeout:               requestTimeout,
		environment: &sequencerEnvironment{
			blockHeight: blockHeight,
		},

		// Randomly allocate a signer.
		// TODO: rotation
		defaultSigner:  fmt.Sprintf("domains.%s.submit.%s", contractAddress, uuid.New()),
		newBlockEvents: make(chan int64, 10), //TODO do we want to make the buffer size configurable? Or should we put in non blocking mode? Does it matter if we miss a block?

	}

	log.L(ctx).Debugf("NewSequencer for contract address %s created: %+v", newSequencer.contractAddress, newSequencer)

	coordinatorSelector, err := NewCoordinatorSelector(ctx, nodeName, domainAPI.ContractConfig(), *sequencerConfig)
	if err != nil {
		log.L(ctx).Errorf("Failed to create coordinator selector: %s", err)
		return nil, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerNewSequencerError, domainAPI.ContractConfig().GetCoordinatorSelection())
	}
	newSequencer.coordinatorSelector = coordinatorSelector

	//TODO consolidate the initialization of the endorsement gatherer and the assemble coordinator.  Both need the same domain context - but maybe the assemble coordinator should provide the domain context to the endorsement gatherer on a per request basis
	//
	domainSmartContract, err := allComponents.DomainManager().GetSmartContractByAddress(ctx, allComponents.Persistence().NOTX(), contractAddress)
	if err != nil {
		log.L(ctx).Errorf("Failed to get domain smart contract for contract address %s: %s", contractAddress, err)

		return nil, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerNewSequencerError, contractAddress)
	}

	// create 2 domain contexts. One to keep track of all transactions that we are coordinating and one for assembling transactions on behalf of a remote coordinator
	newSequencer.coordinatorDomainContext = allComponents.StateManager().NewDomainContext(newSequencer.ctx /* background context */, domainSmartContract.Domain(), contractAddress)
	newSequencer.delegateDomainContext = allComponents.StateManager().NewDomainContext(newSequencer.ctx /* background context */, domainSmartContract.Domain(), contractAddress)

	newSequencer.assembleCoordinator = NewAssembleCoordinator(
		ctx,
		nodeName,
		confutil.Int(sequencerConfig.MaxInflightTransactions, *pldconf.PrivateTxManagerDefaults.Sequencer.MaxInflightTransactions)+1, // make sure the coordinator has more slots that we do so that we don't get stuck waiting for it
		allComponents,
		domainAPI,
		newSequencer.coordinatorDomainContext,
		transportWriter,
		contractAddress,
		newSequencer.environment,
		confutil.DurationMin(sequencerConfig.AssembleRequestTimeout, 1*time.Millisecond, *pldconf.PrivateTxManagerDefaults.Sequencer.AssembleRequestTimeout),
		newSequencer,
	)

	return newSequencer, nil
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
		log.L(s.ctx).Debugf("Transaction processor not found for transaction ID %s", txID)
		return nil
	}
	return transactionProcessor
}

func (s *Sequencer) removeTransactionProcessor(txID string) {
	s.incompleteTxProcessMapMutex.Lock()
	defer s.incompleteTxProcessMapMutex.Unlock()
	delete(s.incompleteTxSProcessMap, txID)
}

func (s *Sequencer) OnNewBlockHeight(ctx context.Context, blockHeight int64) {
	log.L(ctx).Debugf("Sequencer OnNewBlockHeight %d", blockHeight)
	s.environment.blockHeight = blockHeight

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
			s.incompleteTxSProcessMap[tx.ID.String()] = NewTransactionFlow(ctx, tx, s.nodeName, s.components, s.domainAPI, s.coordinatorDomainContext, s.publisher, s.endorsementGatherer, s.identityResolver, s.syncPoints, s.transportWriter, s.requestTimeout, s.coordinatorSelector, s.assembleCoordinator, s.environment)
		}
		s.pendingTransactionEvents <- &ptmgrtypes.TransactionSubmittedEvent{
			PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{TransactionID: tx.ID.String()},
		}
	}
	return false
}

func (s *Sequencer) ProcessInFlightTransaction(ctx context.Context, tx *components.PrivateTransaction, delegationBlockHeight *int64) (queued bool) {
	log.L(ctx).Infof("Processing in flight transaction %s", tx.ID)
	//a transaction that already has had some processing done on it
	// currently the only case this can happen is a transaction delegated from another node
	// but maybe in future, inflight transactions being coordinated locally could be swapped out of memory when they are blocked and/or if we are at max concurrency
	s.incompleteTxProcessMapMutex.Lock()
	defer s.incompleteTxProcessMapMutex.Unlock()
	_, alreadyInMemory := s.incompleteTxSProcessMap[tx.ID.String()]
	if alreadyInMemory {
		if delegationBlockHeight != nil {
			// We need to inform the in-flight processor that the delegation is received, because it might override a
			// delegating status that we have locally (depending on the block height)
			s.pendingTransactionEvents <- &ptmgrtypes.DelegationForInFlightEvent{
				PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{TransactionID: tx.ID.String()},
				BlockHeight:                 *delegationBlockHeight,
			}
		} else {
			log.L(ctx).Warnf("Transaction %s already in memory. Ignoring", tx.ID)
		}
		return false
	} else {
		if len(s.incompleteTxSProcessMap) >= s.maxConcurrentProcess {
			// TODO: decide how this map is managed, it shouldn't track the entire lifecycle
			// tx processing pool is full, queue the item
			return true
		} else {
			s.incompleteTxSProcessMap[tx.ID.String()] = NewTransactionFlow(ctx, tx, s.nodeName, s.components, s.domainAPI, s.coordinatorDomainContext, s.publisher, s.endorsementGatherer, s.identityResolver, s.syncPoints, s.transportWriter, s.requestTimeout, s.coordinatorSelector, s.assembleCoordinator, s.environment)
		}
		s.pendingTransactionEvents <- &ptmgrtypes.TransactionSwappedInEvent{
			PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{TransactionID: tx.ID.String()},
		}
	}
	return false
}

func (s *Sequencer) HandleEvent(ctx context.Context, event ptmgrtypes.PrivateTransactionEvent) {
	s.pendingTransactionEvents <- event
}

func (s *Sequencer) Start(ctx context.Context) (done <-chan struct{}, err error) {
	log.L(ctx).Info("Starting Sequencer")
	s.syncPoints.Start()
	s.sequencerLoopDone = make(chan struct{})
	s.assembleCoordinator.Start()
	go s.evaluationLoop()
	s.TriggerSequencerEvaluation()
	return s.sequencerLoopDone, nil
}

// Stop the InFlight transaction process.
func (s *Sequencer) Stop() {
	// try to send an item in `stopProcess` channel, which has a buffer of 1
	// if it already has an item in the channel, this function does nothing
	s.assembleCoordinator.Stop()
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

func (s *Sequencer) GetTxStatus(ctx context.Context, txID uuid.UUID) (components.PrivateTxStatus, error) {

	s.incompleteTxProcessMapMutex.Lock()
	defer s.incompleteTxProcessMapMutex.Unlock()
	if txProc, ok := s.incompleteTxSProcessMap[txID.String()]; ok {
		return txProc.GetTxStatus(ctx)
	}
	persistedTxn, err := s.components.TxManager().GetTransactionByIDFull(ctx, txID)
	if err != nil {
		log.L(ctx).Errorf("Error getting persisted transaction by ID: %s", err)
		return components.PrivateTxStatus{}, err
	}

	status := "unknown"
	failureMessage := ""
	if persistedTxn.Receipt != nil {
		if persistedTxn.Receipt.Success {
			status = "confirmed"
		} else {
			failureMessage = persistedTxn.Receipt.FailureMessage
			status = "failed"
		}

	}

	return components.PrivateTxStatus{
		TxID:           txID.String(),
		Status:         status,
		FailureMessage: failureMessage,
	}, nil
}
