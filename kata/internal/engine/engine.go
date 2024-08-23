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

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/engine/orchestrator"
	"github.com/kaleido-io/paladin/kata/internal/engine/types"
)

type Engine interface {
	NewOrchestrator(ctx context.Context, contractAddress string, config *orchestrator.OrchestratorConfig) (*orchestrator.Orchestrator, error)
	HandleNewTx(ctx context.Context, txID string) error
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

func (e *engine) NewOrchestrator(ctx context.Context, contractAddress string, config *orchestrator.OrchestratorConfig) (*orchestrator.Orchestrator, error) {
	if e.orchestrators[contractAddress] == nil {
		e.orchestrators[contractAddress] = orchestrator.NewOrchestrator(ctx, contractAddress, config, e.components.StateStore())
	}
	orchestratorDone, err := e.orchestrators[contractAddress].Start(ctx)
	if err != nil {
		log.L(ctx).Errorf("Failed to start orchestrator for contract %s: %s", contractAddress, err)
		return nil, err
	}

	go func() {
		<-orchestratorDone
		log.L(ctx).Infof("Orchestrator for contract %s has stopped", contractAddress)
	}()
	return e.orchestrators[contractAddress], nil
}

// HandleNewTx implements Engine.
func (e *engine) HandleNewTx(ctx context.Context, txID string) error {
	panic("unimplemented")
}

func (me *engine) handleNewEvents(ctx context.Context, stageEvent *types.StageEvent) {

}

func (me *engine) StartEventListener(ctx context.Context) (done <-chan bool) {
	me.done = make(chan struct{})
	mockEvents := make(chan *types.StageEvent)
	go generateMockEvents(ctx, mockEvents)
	go me.listenerLoop(ctx, mockEvents)
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

func (me *engine) listenerLoop(ctx context.Context, mockEventsDoesNotHaveToBeAChannel <-chan *types.StageEvent) {
	defer close(me.done)
	for {
		select {
		case mevent := <-mockEventsDoesNotHaveToBeAChannel:
			me.handleNewEvents(ctx, mevent)
		case <-ctx.Done():
			return
		}
	}
}
