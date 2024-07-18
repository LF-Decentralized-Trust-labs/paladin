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

package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"

	"github.com/hyperledger/firefly-common/pkg/log"
)

// Orchestrator orchestrates transaction processing within a specific private preserving contract
// Key responsibilities:
// 1. Fairness control for transaction processing within a contract
// 1. Apply stage-aware ordering constraints between transactions
// 1. Detect and initiating stage processing of each transactions through 2 ways:
//    a. events through a buffered channel for back-pressure to drive the pace
//    b. fetching from DB as a fallback mechanism when events are missed
// 1. Provide transaction data access for stage processing with a consistent cache for efficiency
// 1. Provide an efficient lookup for pre req tx check

// TBD: decide whether this generic logic should be reused by the following levels of orchestrations:
// 1. Transactions in a chain
// 1. a private preserving contract instance and its transaction chains
// 1. a private preserving contract domain and its instance
// 1. runtime and private preserving contract domains

// configurations
const (
	OrchestratorSection = "orchestrator"
)



type OrchestratorConfig struct {
	MaxConcurrentProcess    *int    `yaml:"maxConcurrentProcess,omitempty"`
	StageRetry              *string `yaml:"stageRetry,omitempty"`
	EvaluationInterval      *string `yaml:"evalInterval,omitempty"`
	PersistenceRetryTimeout *string `yaml:"persistenceRetryTimeout,omitempty"`
	StaleTimeout            *string `yaml:"staleTimeout,omitempty"`
}

// metrics

// Gauge metrics

// Counter metrics

// Histograms metrics

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
	stageRetryTimeout       time.Duration
	persistenceRetryTimeout time.Duration

	stageController StageController

	// each orchestrator has its own go routine
	initiated       time.Time     // when orchestrator is created
	evalInterval    time.Duration // between how long the orchestrator will do an evaluation to check & remove transactions that missed events
	contractAddress string        // the contract address managed by the current orchestrator

	maxConcurrentProcess        int
	incompleteTxProcessMapMutex sync.Mutex
	incompleteTxSProcessMap     map[string]TxProcessor // a map of all known transactions that are not completed

	processedTxIDs       map[string]bool // an internal record of completed transactions to handle persistence delays that causes reprocessing
	orchestratorLoopDone chan struct{}

	// input channels
	orchestrationEvalRequestChan chan bool
	stopProcess                  chan bool // a channel to tell the current orchestrator to stop processing all events and mark itself as to be deleted

	// Metrics provided for fairness control in the controler
	totalCompleted int64 // total number of transaction completed since initiated
	state          OrchestratorState
	stateEntryTime time.Time // when the orchestrator entered the current state

	staleTimeout     time.Duration
	lastActivityTime time.Time
}

var orchestratorConfigDefault = OrchestratorConfig{
	MaxConcurrentProcess:    confutil.P(500),
	StageRetry:              confutil.P("5s"),
	EvaluationInterval:      confutil.P("5m"),
	PersistenceRetryTimeout: confutil.P("5s"),
	StaleTimeout:            confutil.P("10m"),
}

func NewOrchestrator(ctx context.Context, contractAddress string, oc *OrchestratorConfig, ss statestore.StateStore) *Orchestrator {

	newOrchestrator := &Orchestrator{
		ctx:                  log.WithLogField(ctx, "role", fmt.Sprintf("orchestrator-%s", contractAddress)),
		initiated:            time.Now(),
		contractAddress:      contractAddress,
		evalInterval:         confutil.Duration(oc.EvaluationInterval, *orchestratorConfigDefault.EvaluationInterval),
		maxConcurrentProcess: confutil.Int(oc.MaxConcurrentProcess, *orchestratorConfigDefault.MaxConcurrentProcess),
		state:                OrchestratorStateNew,
		stateEntryTime:       time.Now(),

		// in-flight transaction configs
		stageRetryTimeout:       confutil.Duration(oc.StageRetry, *orchestratorConfigDefault.StageRetry),
		persistenceRetryTimeout: confutil.Duration(oc.PersistenceRetryTimeout, *orchestratorConfigDefault.PersistenceRetryTimeout),

		staleTimeout:                 confutil.Duration(oc.StaleTimeout, *orchestratorConfigDefault.StageRetry),
		processedTxIDs:               make(map[string]bool),
		orchestrationEvalRequestChan: make(chan bool, 1),
		stopProcess:                  make(chan bool, 1),
	}

	newOrchestrator.stageController = NewPaladinStageController(ctx, &PaladinStageFoundationService{
		dependencyChecker: newOrchestrator,
		stateStore:        ss,
		talariaInfo:       &MockTalariaInfo{},
	})

	log.L(ctx).Debugf("NewOrchestrator for contract address %s created: %+v", newOrchestrator.contractAddress, newOrchestrator)

	return newOrchestrator
}

func (oc *Orchestrator) evaluationLoop() {
	ctx := log.WithLogField(oc.ctx, "role", fmt.Sprintf("oc-loop-%s", oc.contractAddress))
	log.L(ctx).Infof("Orchestrator for contract address %s started evaluation loop based on interval %s", oc.contractAddress, oc.evalInterval)

	defer close(oc.orchestratorLoopDone)

	ticker := time.NewTicker(oc.evalInterval)
	for {
		// an InFlight
		select {
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
		added, total := oc.evaluateTransactions(ctx)
		log.L(ctx).Debugf("Orchestrator loop added %d txs, there are %d txs in total", added, total)
	}

}

func (oc *Orchestrator) evaluateTransactions(ctx context.Context) (added int, newTotal int) {
	evalStart := time.Now()
	oc.incompleteTxProcessMapMutex.Lock()
	defer oc.incompleteTxProcessMapMutex.Unlock()
	hasActivity := false

	oldIncompleteMap := oc.incompleteTxSProcessMap
	oc.incompleteTxSProcessMap = make(map[string]TxProcessor, len(oldIncompleteMap))

	stageCounts := make(map[string]int)
	for _, stageName := range oc.stageController.GetAllStages() {
		// map for saving number of known incomplete transactions per stage
		stageCounts[stageName] = 0
	}

	// TODO: how to distinguish the engine states below
	stageCounts["remove"] = 0
	stageCounts["suspend"] = 0
	stageCounts["queued"] = 0

	for txID, txp := range oldIncompleteMap {
		oc.processedTxIDs[txID] = true
		sc := txp.GetStageContext(ctx)
		if sc != nil {
			if sc.Stage == "remove" {
				// no longer in an incomplete stage
				delete(oc.incompleteTxSProcessMap, txID)
				oc.totalCompleted = oc.totalCompleted + 1
				hasActivity = true
				log.L(ctx).Debugf("Orchestrator evaluate and process, marking %s as complete.", txID)
			} else if sc.Stage == "suspend" {
				log.L(ctx).Debugf("Orchestrator evaluate and process, removed suspended tx %s", txID)
			} else {
				stageCounts[sc.Stage] = stageCounts[sc.Stage] + 1
			}
		} else {
			stageCounts["queued"] = stageCounts["queued"] + 1

		}
	}

	log.L(ctx).Debugf("Orchestrator evaluate and process, stage counts: %+v", stageCounts)

	oldTotal := len(oc.incompleteTxSProcessMap)
	newTotal = oldTotal

	// in case there are event we missed
	// check and evaluate new transactions from the persistence if we can handle more
	// If we are not at maximum, then query if there are more candidates now

	spaces := oc.maxConcurrentProcess - oldTotal
	if spaces > 0 {
		completedTxIDsStillBeingPersisted := make(map[string]bool)
		// TODO: evaluate and put kick off stage processing for transactions
		oc.processedTxIDs = completedTxIDsStillBeingPersisted
		newTotal = len(oc.incompleteTxSProcessMap)
		added = newTotal - oldTotal
		if added > 0 {
			log.L(ctx).Infof("Evaluation loop added %d new transactions", added)
		}
		// TODO: emit metrics
	}
	log.L(ctx).Debugf("Orchestrator evaluate from DB took %s", time.Since(evalStart))
	// now check and process each transaction

	if newTotal > 0 {
		blockedByPreReq := oc.ProcessIncompleteTransactions(ctx, oc.incompleteTxSProcessMap)
		if hasActivity {
			oc.lastActivityTime = time.Now()
		}
		if time.Since(oc.lastActivityTime) > oc.staleTimeout && oc.state != OrchestratorStateStale {
			oc.state = OrchestratorStateStale
			oc.stateEntryTime = time.Now()
		} else if blockedByPreReq && oc.state != OrchestratorStateWaiting {
			oc.state = OrchestratorStateWaiting
			oc.stateEntryTime = time.Now()
		} else if oc.state != OrchestratorStateRunning {
			oc.state = OrchestratorStateRunning
			oc.stateEntryTime = time.Now()
		}
	} else if oc.state != OrchestratorStateIdle {
		oc.state = OrchestratorStateIdle
		oc.stateEntryTime = time.Now()
	}
	log.L(ctx).Debugf("Orchestrator process loop took %s", time.Since(evalStart))
	return added, newTotal
}

func (oc *Orchestrator) ProcessNewTransaction(ctx context.Context, tsm transactionstore.TxStateManager) (queued bool) {
	oc.incompleteTxProcessMapMutex.Lock()
	defer oc.incompleteTxProcessMapMutex.Unlock()
	if len(oc.incompleteTxSProcessMap) >= oc.maxConcurrentProcess {
		// TODO: decide how this map is managed, it shouldn't track the entire lifecycle
		// tx processing pool is full, queue the item
		return true
	} else {
		oc.incompleteTxSProcessMap[tsm.GetTxID(ctx)] = NewPaladinTransactionProcessor(ctx, tsm, oc.stageController)
		oc.incompleteTxSProcessMap[tsm.GetTxID(ctx)].Continue(ctx)
		return false
	}
}

func (oc *Orchestrator) HandleEvent(ctx context.Context, stageEvent *StageEvent) (queued bool) {
	oc.incompleteTxProcessMapMutex.Lock()
	defer oc.incompleteTxProcessMapMutex.Unlock()
	txProc := oc.incompleteTxSProcessMap[stageEvent.TxID]
	if txProc == nil {
		// TODO: decide what to do here,bypass max concurrent process check?
		// or throw the event away and waste another cycle to redo the actions
		// (doesn't feel right, maybe for some events only persistence is needed)
		return true
	} else {
		txProc.AddStageEvent(ctx, stageEvent)
	}
	return false
}

// this function should only have one running instance at any given time
func (oc *Orchestrator) ProcessIncompleteTransactions(ctx context.Context, txStages map[string]TxProcessor) (blockedByPreReq bool) {
	processStart := time.Now()
	blockedByPreReq = false
	log.L(ctx).Debugf("%s ProcessIncompleteTransactions entry for contract address %s", processStart.String(), oc.contractAddress)

	for txID, stage := range txStages {
		log.L(ctx).Tracef("%s ProcessIncompleteTransactions for contract address %s processing transaction with ID: %s, current stage: %s", processStart.String(), oc.contractAddress, txID, stage)
		// TODO: stage plugin here
	}

	log.L(ctx).Debugf("%s ProcessIncompleteTransactions exit for contract address: %s, process %d over %s", processStart.String(), oc.contractAddress, len(txStages), time.Since(processStart))
	return blockedByPreReq
}

func (oc *Orchestrator) Start(c context.Context) (done <-chan struct{}, err error) {
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
