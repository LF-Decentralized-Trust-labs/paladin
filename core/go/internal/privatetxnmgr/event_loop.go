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
	"github.com/kaleido-io/paladin/core/internal/privatetxnmgr/privatetxnstore"
	"github.com/kaleido-io/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/kaleido-io/paladin/core/internal/statedistribution"

	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/ptxapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type OrchestratorState string

const (
	// brand new orchestrator
	OrchestratorStateNew OrchestratorState = "new"
	// orchestrator running normally
	OrchestratorStateRunning OrchestratorState = "running"
	// orchestrator is blocked and waiting for precondition to be fulfilled, e.g. pre-req tx blocking current stage
	OrchestratorStateWaiting OrchestratorState = "waiting"
	// transactions managed by an orchestrator stuck in the same state
	OrchestratorStateStale OrchestratorState = "stale"
	// no transactions in a specific orchestrator
	OrchestratorStateIdle OrchestratorState = "idle"
	// orchestrator is paused
	OrchestratorStatePaused OrchestratorState = "paused"
	// orchestrator is stopped
	OrchestratorStateStopped OrchestratorState = "stopped"
)

var AllOrchestratorStates = []string{
	string(OrchestratorStateNew),
	string(OrchestratorStateRunning),
	string(OrchestratorStateWaiting),
	string(OrchestratorStateStale),
	string(OrchestratorStateIdle),
	string(OrchestratorStatePaused),
	string(OrchestratorStateStopped),
}

type Orchestrator struct {
	ctx                     context.Context
	persistenceRetryTimeout time.Duration

	// each orchestrator has its own go routine
	initiated    time.Time     // when orchestrator is created
	evalInterval time.Duration // between how long the orchestrator will do an evaluation to check & remove transactions that missed events

	maxConcurrentProcess        int
	incompleteTxProcessMapMutex sync.Mutex
	incompleteTxSProcessMap     map[string]ptmgrtypes.TxProcessor // a map of all known transactions that are not completed

	processedTxIDs       map[string]bool // an internal record of completed transactions to handle persistence delays that causes reprocessing
	orchestratorLoopDone chan struct{}

	// input channels
	orchestrationEvalRequestChan chan bool
	stopProcess                  chan bool // a channel to tell the current orchestrator to stop processing all events and mark itself as to be deleted

	// Metrics provided for fairness control in the controler
	totalCompleted int64 // total number of transaction completed since initiated
	state          OrchestratorState
	stateEntryTime time.Time // when the orchestrator entered the current state

	staleTimeout time.Duration

	pendingEvents chan ptmgrtypes.PrivateTransactionEvent

	contractAddress     tktypes.EthAddress // the contract address managed by the current orchestrator
	nodeID              string
	domainAPI           components.DomainSmartContract
	sequencer           ptmgrtypes.Sequencer
	components          components.AllComponents
	endorsementGatherer ptmgrtypes.EndorsementGatherer
	publisher           ptmgrtypes.Publisher
	identityResolver    components.IdentityResolver
	store               privatetxnstore.Store
	stateDistributer    statedistribution.StateDistributer
}

func NewOrchestrator(
	ctx context.Context,
	nodeID string,
	contractAddress tktypes.EthAddress,
	oc *pldconf.PrivateTxManagerOrchestratorConfig,
	allComponents components.AllComponents,
	domainAPI components.DomainSmartContract,
	sequencer ptmgrtypes.Sequencer,
	endorsementGatherer ptmgrtypes.EndorsementGatherer,
	publisher ptmgrtypes.Publisher,
	store privatetxnstore.Store,
	identityResolver components.IdentityResolver,
	stateDistributer statedistribution.StateDistributer,
) *Orchestrator {

	newOrchestrator := &Orchestrator{
		ctx:                  log.WithLogField(ctx, "role", fmt.Sprintf("orchestrator-%s", contractAddress)),
		initiated:            time.Now(),
		contractAddress:      contractAddress,
		evalInterval:         confutil.DurationMin(oc.EvaluationInterval, 1*time.Millisecond, *pldconf.PrivateTxManagerDefaults.Orchestrator.EvaluationInterval),
		maxConcurrentProcess: confutil.Int(oc.MaxConcurrentProcess, *pldconf.PrivateTxManagerDefaults.Orchestrator.MaxConcurrentProcess),
		state:                OrchestratorStateNew,
		stateEntryTime:       time.Now(),

		incompleteTxSProcessMap: make(map[string]ptmgrtypes.TxProcessor),
		persistenceRetryTimeout: confutil.DurationMin(oc.PersistenceRetryTimeout, 1*time.Millisecond, *pldconf.PrivateTxManagerDefaults.Orchestrator.PersistenceRetryTimeout),

		staleTimeout:                 confutil.DurationMin(oc.StaleTimeout, 1*time.Millisecond, *pldconf.PrivateTxManagerDefaults.Orchestrator.StaleTimeout),
		processedTxIDs:               make(map[string]bool),
		orchestrationEvalRequestChan: make(chan bool, 1),
		stopProcess:                  make(chan bool, 1),
		pendingEvents:                make(chan ptmgrtypes.PrivateTransactionEvent, *pldconf.PrivateTxManagerDefaults.Orchestrator.MaxPendingEvents),
		nodeID:                       nodeID,
		domainAPI:                    domainAPI,
		sequencer:                    sequencer,
		components:                   allComponents,
		endorsementGatherer:          endorsementGatherer,
		publisher:                    publisher,
		store:                        store,
		identityResolver:             identityResolver,
		stateDistributer:             stateDistributer,
	}

	newOrchestrator.sequencer = sequencer
	sequencer.SetDispatcher(newOrchestrator)

	log.L(ctx).Debugf("NewOrchestrator for contract address %s created: %+v", newOrchestrator.contractAddress, newOrchestrator)

	return newOrchestrator
}

func (oc *Orchestrator) getTransactionProcessor(txID string) ptmgrtypes.TxProcessor {
	oc.incompleteTxProcessMapMutex.Lock()
	defer oc.incompleteTxProcessMapMutex.Unlock()
	//TODO if the transaction is not in memory, we need to load it from the database
	return oc.incompleteTxSProcessMap[txID]
}

func (oc *Orchestrator) handleEvent(ctx context.Context, event ptmgrtypes.PrivateTransactionEvent) {
	//For any event that is specific to a single transaction,
	// find (or create) the transaction processor for that transaction
	// and pass the event to it
	transactionID := event.GetTransactionID()
	transactionProccessor := oc.getTransactionProcessor(transactionID)
	var err error
	switch event := event.(type) {
	case *ptmgrtypes.TransactionSubmittedEvent:
		err = transactionProccessor.HandleTransactionSubmittedEvent(ctx, event)
	case *ptmgrtypes.TransactionAssembledEvent:
		err = transactionProccessor.HandleTransactionAssembledEvent(ctx, event)
	case *ptmgrtypes.TransactionSignedEvent:
		err = transactionProccessor.HandleTransactionSignedEvent(ctx, event)
	case *ptmgrtypes.TransactionEndorsedEvent:
		err = transactionProccessor.HandleTransactionEndorsedEvent(ctx, event)
	case *ptmgrtypes.TransactionDispatchedEvent:
		err = transactionProccessor.HandleTransactionDispatchedEvent(ctx, event)
	case *ptmgrtypes.TransactionConfirmedEvent:
		err = transactionProccessor.HandleTransactionConfirmedEvent(ctx, event)
	case *ptmgrtypes.TransactionRevertedEvent:
		err = transactionProccessor.HandleTransactionRevertedEvent(ctx, event)
	case *ptmgrtypes.TransactionDelegatedEvent:
		err = transactionProccessor.HandleTransactionDelegatedEvent(ctx, event)
	case *ptmgrtypes.ResolveVerifierResponseEvent:
		err = transactionProccessor.HandleResolveVerifierResponseEvent(ctx, event)
	case *ptmgrtypes.ResolveVerifierErrorEvent:
		err = transactionProccessor.HandleResolveVerifierErrorEvent(ctx, event)
	default:
		log.L(ctx).Warnf("Unknown event type: %T", event)
	}
	if err != nil {
		// Any expected errors like assembly failed or endorsement failed should have been handled by the transaction processor
		// Any errors that get back here mean that event has not been fully applied and we rely on the
		// event being re-sent, most likely after the transaction processor re-sends an async request
		// because it has detected a stale transaction that has timedout waiting for a response
		log.L(ctx).Errorf("Error handling %T event: %s ", event, err.Error())
	}
}

func (oc *Orchestrator) evaluationLoop() {
	// Select from the event channel and process each event on a single thread
	// individual event handlers are responsible for kicking off another go routine if they have
	// long running tasks that are not dependant on the order of events
	// TODO if the channel ever fills up, then we need to be aware that we have potentially missed some events
	// and we need to poll the database for any events that we missed

	ctx := log.WithLogField(oc.ctx, "role", fmt.Sprintf("pctm-loop-%s", oc.contractAddress))
	log.L(ctx).Infof("Orchestrator for contract address %s started evaluation loop based on interval %s", oc.contractAddress, oc.evalInterval)

	defer close(oc.orchestratorLoopDone)

	ticker := time.NewTicker(oc.evalInterval)
	for {
		// an InFlight
		select {
		case pendingEvent := <-oc.pendingEvents:
			oc.handleEvent(ctx, pendingEvent)
		case <-oc.orchestrationEvalRequestChan:
		case <-ticker.C:
		case <-ctx.Done():
			log.L(ctx).Infof("Orchestrator loop exit due to canceled context, it processed %d transaction during its lifetime.", oc.totalCompleted)
			return
		case <-oc.stopProcess:
			log.L(ctx).Infof("Orchestrator loop process stopped, it processed %d transaction during its lifetime.", oc.totalCompleted)
			oc.state = OrchestratorStateStopped
			oc.stateEntryTime = time.Now()
			// TODO: trigger parent loop for removal
			return
		}
		// TODO while we have woken up, iterate through all transactions in memory and check if any are stale or completed and query the database for any in flight transactions that need to be brougt into memory
	}
}

func (oc *Orchestrator) ProcessNewTransaction(ctx context.Context, tx *components.PrivateTransaction) (queued bool) {
	oc.incompleteTxProcessMapMutex.Lock()
	defer oc.incompleteTxProcessMapMutex.Unlock()
	if oc.incompleteTxSProcessMap[tx.ID.String()] == nil {
		if len(oc.incompleteTxSProcessMap) >= oc.maxConcurrentProcess {
			// TODO: decide how this map is managed, it shouldn't track the entire lifecycle
			// tx processing pool is full, queue the item
			return true
		} else {
			oc.incompleteTxSProcessMap[tx.ID.String()] = NewPaladinTransactionProcessor(ctx, tx, oc.nodeID, oc.components, oc.domainAPI, oc.sequencer, oc.publisher, oc.endorsementGatherer, oc.identityResolver)
		}
		oc.incompleteTxSProcessMap[tx.ID.String()].Init(ctx)
		oc.pendingEvents <- &ptmgrtypes.TransactionSubmittedEvent{
			PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{TransactionID: tx.ID.String()},
		}
	}
	return false
}

func (oc *Orchestrator) HandleEvent(ctx context.Context, event ptmgrtypes.PrivateTransactionEvent) {
	//TODO Better
	oc.incompleteTxProcessMapMutex.Lock()
	defer oc.incompleteTxProcessMapMutex.Unlock()
	txProc := oc.incompleteTxSProcessMap[event.GetTransactionID()]
	if txProc == nil {

		// TODO we have an event for a transaction that we have swapped out of memory.  Need to reload the transaction from the database before we can proces this event
		// and we need to do this in a way that doesn't exceed the maxConcurrentProcess and doesn't allow events from a runaway remote node to cause a noisy neighbor problem for events
		// from the local node
		panic("Transaction not found")

	}
	oc.pendingEvents <- event
}

func (oc *Orchestrator) Start(c context.Context) (done <-chan struct{}, err error) {
	oc.store.Start()
	oc.orchestratorLoopDone = make(chan struct{})
	go oc.evaluationLoop()
	oc.TriggerOrchestratorEvaluation()
	return oc.orchestratorLoopDone, nil
}

// Stop the InFlight transaction process.
func (oc *Orchestrator) Stop() {
	// try to send an item in `stopProcess` channel, which has a buffer of 1
	// if it already has an item in the channel, this function does nothing
	select {
	case oc.stopProcess <- true:
	default:
	}

}

func (oc *Orchestrator) TriggerOrchestratorEvaluation() {
	// try to send an item in `processNow` channel, which has a buffer of 1
	// if it already has an item in the channel, this function does nothing
	select {
	case oc.orchestrationEvalRequestChan <- true:
	default:
	}
}

func (oc *Orchestrator) GetTxStatus(ctx context.Context, txID string) (status components.PrivateTxStatus, err error) {
	//TODO This is primarily here to help with testing for now
	// this needs to be revisited ASAP as part of a holisitic review of the persistence model
	oc.incompleteTxProcessMapMutex.Lock()
	defer oc.incompleteTxProcessMapMutex.Unlock()
	if txProc, ok := oc.incompleteTxSProcessMap[txID]; ok {
		return txProc.GetTxStatus(ctx)
	}
	//TODO should be possible to query the status of a transaction that is not inflight
	return components.PrivateTxStatus{}, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "Transaction not found")
}

// synchronously prepare and dispatch all given transactions to their associated signing address
func (oc *Orchestrator) DispatchTransactions(ctx context.Context, dispatchableTransactions ptmgrtypes.DispatchableTransactions) error {
	log.L(ctx).Debug("DispatchTransactions")
	//prepare all transactions then dispatch them

	// array of sequences with space for one per signing address
	// dispatchableTransactions is a map of signing address to transaction IDs so we can group by signing address
	dispatchBatch := &privatetxnstore.DispatchBatch{
		DispatchSequences: make([]*privatetxnstore.DispatchSequence, 0, len(dispatchableTransactions)),
	}

	stateDistributions := make([]*statedistribution.StateDistribution, 0)

	completed := false // and include whether we committed the DB transaction or not
	for signingAddress, transactionIDs := range dispatchableTransactions {
		log.L(ctx).Debugf("DispatchTransactions: %d transactions for signingAddress %s", len(transactionIDs), signingAddress)

		preparedTransactions := make([]*components.PrivateTransaction, len(transactionIDs))

		sequence := &privatetxnstore.DispatchSequence{
			PrivateTransactionDispatches: make([]*privatetxnstore.DispatchPersisted, len(transactionIDs)),
		}

		for i, transactionID := range transactionIDs {
			// prepare all transactions for the given transaction IDs

			sequence.PrivateTransactionDispatches[i] = &privatetxnstore.DispatchPersisted{
				PrivateTransactionID: transactionID,
			}

			txProcessor := oc.getTransactionProcessor(transactionID)
			if txProcessor == nil {
				//TODO currently assume that all the transactions are in flight and in memory
				// need to reload from database if not in memory
				panic("Transaction not found")
			}

			preparedTransaction, err := txProcessor.PrepareTransaction(ctx)
			if err != nil {
				log.L(ctx).Errorf("Error preparing transaction: %s", err)
				//TODO this is a really bad time to be getting an error.  need to think carefully about how to handle this
				return err
			}
			if preparedTransaction.PreparedPublicTransaction == nil {
				// TODO: add handling
				panic("private transactions triggering private transactions currently supported only in testbed")
			}
			preparedTransactions[i] = preparedTransaction

			stateDistributions = append(stateDistributions, txProcessor.GetStateDistributions(ctx)...)
		}

		preparedTransactionPayloads := make([]*components.EthTransaction, len(preparedTransactions))

		for j, preparedTransaction := range preparedTransactions {
			preparedTransactionPayloads[j] = preparedTransaction.PreparedPublicTransaction
		}

		//Now we have the payloads, we can prepare the submission
		ec := oc.components.EthClientFactory().SharedWS()
		publicTransactionEngine := oc.components.PublicTxManager()

		publicTXs := make([]*components.PublicTxSubmission, len(preparedTransactions))
		for i, pt := range preparedTransactions {
			log.L(ctx).Debugf("DispatchTransactions: creating PublicTxSubmission from %s", pt.Signer)
			publicTXs[i] = &components.PublicTxSubmission{
				Bindings: []*components.PaladinTXReference{{TransactionID: pt.ID, TransactionType: ptxapi.TransactionTypePrivate.Enum()}},
				PublicTxInput: ptxapi.PublicTxInput{
					From:            pt.Signer,
					To:              &oc.contractAddress,
					PublicTxOptions: ptxapi.PublicTxOptions{}, // TODO: Consider propagation from paladin transaction input
				},
			}
			var req ethclient.ABIFunctionRequestBuilder
			abiFn, err := ec.ABIFunction(ctx, pt.PreparedPublicTransaction.FunctionABI)
			if err == nil {
				req = abiFn.R(ctx)
				err = req.Input(pt.PreparedPublicTransaction.Inputs).BuildCallData()
			}
			if err != nil {
				return err
			}
			publicTXs[i].Data = tktypes.HexBytes(req.TX().Data)
		}
		pubBatch, err := publicTransactionEngine.PrepareSubmissionBatch(ctx, publicTXs)
		if err != nil {
			return i18n.WrapError(ctx, err, msgs.MsgPrivTxMgrPublicTxFail)
		}
		// Must make sure from this point we return the nonces
		sequence.PublicTxBatch = pubBatch
		defer func() {
			pubBatch.Completed(ctx, completed)
		}()
		if len(pubBatch.Rejected()) > 0 {
			// We do not handle partial success - roll everything back
			return i18n.WrapError(ctx, pubBatch.Rejected()[0].RejectedError(), msgs.MsgPrivTxMgrPublicTxFail)
		}

		dispatchBatch.DispatchSequences = append(dispatchBatch.DispatchSequences, sequence)
	}

	err := oc.store.PersistDispatchBatch(ctx, oc.contractAddress, dispatchBatch, stateDistributions)
	if err != nil {
		log.L(ctx).Errorf("Error persisting batch: %s", err)
		return err
	}
	completed = true
	for signingAddress, sequence := range dispatchableTransactions {
		for _, privateTransactionID := range sequence {
			err := oc.publisher.PublishTransactionDispatchedEvent(ctx, privateTransactionID, uint64(0) /*TODO*/, signingAddress)

			if err != nil {
				//TODO think about how best to handle this error
				log.L(ctx).Errorf("Error publishing event: %s", err)
			}
		}
	}
	//now that the DB write has been persisted, we can trigger the in-memory state distribution
	oc.stateDistributer.DistributeStates(ctx, stateDistributions)

	return nil

}
