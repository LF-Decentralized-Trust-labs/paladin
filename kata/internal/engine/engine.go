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

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
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

type MockPluginProvider interface {
	Validate(ctx context.Context, contractAddress string, tsg transactionstore.TxStateGetters, ss statestore.StateStore) bool
}

// Mock engine

type MockEngine struct {
	txStore        transactionstore.TransactionStore
	stateStore     statestore.StateStore
	pluginProvider MockPluginProvider
	done           chan bool
	ocs            map[string]*Orchestrator
}

func (me *MockEngine) HandleNewTx(ctx context.Context, txID uuid.UUID) {
	tx := transactionstore.NewTransaction(ctx, txID)
	valid := me.pluginProvider.Validate(ctx, tx.GetContract(ctx), tx, me.stateStore)
	if valid {
		// TODO how to measure fairness/ per From address / contract address / something else
		if me.ocs[tx.GetContract(ctx)] == nil {
			me.ocs[tx.GetContract(ctx)] = NewOrchestrator(ctx, tx.GetContract(ctx) /** TODO: fill in the real plug-ins*/, nil, nil)
		}
		oc := me.ocs[tx.GetContract(ctx)]
		queued := oc.ProcessNewTransaction(ctx, tx)
		if queued {
			log.L(ctx).Debugf("Transaction with ID %s queued in database", tx.GetTxID(ctx))
		}
	}
}

func (me *MockEngine) handleNewEvents(ctx context.Context, stageEvent *types.StageEvent) {

}

func (me *MockEngine) StartEventListener(ctx context.Context) (done <-chan bool) {
	me.done = make(chan bool)
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

func (me *MockEngine) listenerLoop(ctx context.Context, mockEventsDoesNotHaveToBeAChannel <-chan *types.StageEvent) {
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

func NewMockEngine(ctx context.Context, txStore transactionstore.TransactionStore, stateStore statestore.StateStore) *MockEngine {
	return &MockEngine{
		txStore:        txStore,
		stateStore:     stateStore,
		pluginProvider: &MockPlugins{},
		done:           make(chan bool),
	}
}
