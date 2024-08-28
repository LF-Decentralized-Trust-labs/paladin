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
	"time"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/engine/orchestrator"
	"github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"

	ptypes "github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

// MOCK implementations of engine, plugins etc. Function signatures are just examples
// no formal interface proposed intentionally in this file

// Mock Plugin manager

type MockPlugins struct {
	installedPlugins  map[string]MockPlugin
	contractInstances map[string]string
}

type MockPlugin interface {
	Validate(ctx context.Context, tsg transactionstore.TxStateGetters, ss statestore.StateStore) bool
}

func (mpm *MockPlugins) Validate(ctx context.Context, contractAddress string, tsg transactionstore.TxStateGetters, ss statestore.StateStore) bool {
	return mpm.installedPlugins[mpm.contractInstances[contractAddress]].Validate(ctx, tsg, ss)
}

type Engine interface {
	HandleNewEvents(ctx context.Context, stageEvent *types.StageEvent)
	HandleNewTx(ctx context.Context, tx *components.PrivateTransaction) (txID string, err error)
	GetTxStatus(ctx context.Context, domainAddress string, txID string) (status types.TxStatus, err error)
	Name() string
	Init(components.AllComponents) (*components.ManagerInitResult, error)
	Start() error
	Stop()
}

type engine struct {
	ctx           context.Context
	ctxCancel     func()
	done          chan struct{}
	orchestrators map[string]*orchestrator.Orchestrator
	components    components.AllComponents
}

// Init implements Engine.
func (e *engine) Init(c components.AllComponents) (*components.ManagerInitResult, error) {
	e.components = c
	return nil, nil
}

// Name implements Engine.
func (e *engine) Name() string {
	return "Kata Engine"
}

// Start implements Engine.
func (e *engine) Start() error {
	e.ctx, e.ctxCancel = context.WithCancel(context.Background())
	e.StartEventListener(e.ctx)
	return nil
}

// Stop implements Engine.
func (e *engine) Stop() {
	panic("unimplemented")
}

func NewEngine() Engine {
	return &engine{
		orchestrators: make(map[string]*orchestrator.Orchestrator),
	}
}

// HandleNewTx implements Engine.
func (e *engine) HandleNewTx(ctx context.Context, tx *components.PrivateTransaction) (txID string, err error) { // TODO: this function currently assumes another layer initialize transactions and store them into DB
	if tx.Inputs == nil || tx.Inputs.Domain == "" {
		return "", i18n.NewError(ctx, msgs.MsgDomainNotProvided)
	}
	contractAddr, err := ptypes.ParseEthAddress(tx.Inputs.Domain)
	if err != nil {
		return "", err
	}
	domainAPI, err := e.components.DomainManager().GetSmartContractByAddress(ctx, *contractAddr)
	if err != nil {
		return "", err
	}
	err = domainAPI.InitTransaction(ctx, tx)
	if err != nil {
		return "", err
	}
	txInstance := transactionstore.NewTransactionStageManager(ctx, tx)
	// TODO how to measure fairness/ per From address / contract address / something else
	if e.orchestrators[contractAddr.String()] == nil {
		e.orchestrators[contractAddr.String()] = orchestrator.NewOrchestrator(ctx, contractAddr.String() /** TODO: fill in the real plug-ins*/, &orchestrator.OrchestratorConfig{}, e.components, domainAPI)
		orchestratorDone, err := e.orchestrators[contractAddr.String()].Start(ctx)
		if err != nil {
			log.L(ctx).Errorf("Failed to start orchestrator for contract %s: %s", contractAddr.String(), err)
			return "", err
		}

		go func() {
			<-orchestratorDone
			log.L(ctx).Infof("Orchestrator for contract %s has stopped", contractAddr.String())
		}()
	}
	oc := e.orchestrators[contractAddr.String()]
	queued := oc.ProcessNewTransaction(ctx, txInstance)
	if queued {
		log.L(ctx).Debugf("Transaction with ID %s queued in database", txInstance.GetTxID(ctx))
	}
	return txInstance.GetTxID(ctx), nil
}

func (e *engine) GetTxStatus(ctx context.Context, domainAddress string, txID string) (status types.TxStatus, err error) {
	targetOrchestrator := e.orchestrators[domainAddress]
	if targetOrchestrator == nil {
		//TODO should be valid to query the status of a transaction that belongs to a domain instance that is not currently active
		return types.TxStatus{}, i18n.NewError(ctx, msgs.MsgEngineInternalError)
	} else {
		return targetOrchestrator.GetTxStatus(ctx, txID)
	}

}

func (e *engine) HandleNewEvents(ctx context.Context, stageEvent *types.StageEvent) {
	targetOrchestrator := e.orchestrators[stageEvent.ContractAddress]
	if targetOrchestrator == nil { // this is an event that belongs to a contract that's not in flight, throw it away and rely on the engine to trigger the action again when the orchestrator is wake up. (an enhanced version is to add weight on queueing an orchestrator)
		log.L(ctx).Warnf("Ignored event for  domain contract %s and transaction %s on stage %s. If this happens a lot, check the orchestrator idle timeout is set to a reasonable number", stageEvent.ContractAddress, stageEvent.TxID, stageEvent.Stage)
	} else {
		targetOrchestrator.HandleEvent(ctx, stageEvent)
	}

}

func (e *engine) StartEventListener(ctx context.Context) (done <-chan bool) {
	e.done = make(chan struct{})
	mockEvents := make(chan *types.StageEvent)
	go generateMockEvents(ctx, mockEvents)
	go e.listenerLoop(ctx, mockEvents)
	return done
}

func generateMockEvents(ctx context.Context, receiver chan<- *types.StageEvent) {
	tick := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-tick.C:
			receiver <- &types.StageEvent{
				// TODO: figure out how to mock UUID of the event
				Stage: "test",
				TxID:  "test",
				Data:  "test",
			}
		case <-ctx.Done():
			return
		}
	}
}

func (e *engine) listenerLoop(ctx context.Context, mockEventsDoesNotHaveToBeAChannel <-chan *types.StageEvent) {
	defer close(e.done)
	for {
		select {
		case mevent := <-mockEventsDoesNotHaveToBeAChannel:
			e.HandleNewEvents(ctx, mevent)
		case <-ctx.Done():
			return
		}
	}
}
