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
	"sync"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/engine/orchestrator"
	"github.com/kaleido-io/paladin/kata/internal/engine/sequencer"
	"github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
	pbEngine "github.com/kaleido-io/paladin/kata/pkg/proto/engine"
	"google.golang.org/protobuf/proto"

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
	HandleNewEvent(ctx context.Context, stageEvent *types.StageEvent)
	HandleNewTx(ctx context.Context, tx *components.PrivateTransaction) (txID string, err error)
	GetTxStatus(ctx context.Context, domainAddress string, txID string) (status types.TxStatus, err error)
	EngineName() string
	Init(components.AllComponents) (*components.ManagerInitResult, error)
	Start() error
	Stop()
	Subscribe(ctx context.Context, subscriber types.EventSubscriber)
}

type engine struct {
	ctx             context.Context
	ctxCancel       func()
	done            chan struct{}
	orchestrators   map[string]*orchestrator.Orchestrator
	components      components.AllComponents
	nodeID          uuid.UUID
	subscribers     []types.EventSubscriber
	subscribersLock sync.Mutex
}

// Init implements Engine.
func (e *engine) Init(c components.AllComponents) (*components.ManagerInitResult, error) {
	e.components = c
	return &components.ManagerInitResult{}, nil
}

// Name implements Engine.
func (e *engine) EngineName() string {
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

func NewEngine(nodeID uuid.UUID) Engine {
	return &engine{
		orchestrators: make(map[string]*orchestrator.Orchestrator),
		nodeID:        nodeID,
		subscribers:   make([]types.EventSubscriber, 0),
	}
}

// HandleNewTx implements Engine.
func (e *engine) HandleNewTx(ctx context.Context, tx *components.PrivateTransaction) (txID string, err error) { // TODO: this function currently assumes another layer initialize transactions and store them into DB
	log.L(ctx).Debugf("Handling new transaction: %v", tx)
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
		publisher := NewPublisher(e)
		delegator := NewDelegator()
		dispatcher := NewDispatcher(contractAddr.String(), publisher)
		seq := sequencer.NewSequencer(
			e.nodeID,
			publisher,
			delegator,
			dispatcher,
		)
		e.orchestrators[contractAddr.String()] =
			orchestrator.NewOrchestrator(
				ctx, e.nodeID,
				contractAddr.String(), /** TODO: fill in the real plug-ins*/
				&orchestrator.OrchestratorConfig{},
				e.components,
				domainAPI,
				publisher,
				seq,
			)
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

func (e *engine) HandleNewEvent(ctx context.Context, stageEvent *types.StageEvent) {
	targetOrchestrator := e.orchestrators[stageEvent.ContractAddress]
	if targetOrchestrator == nil { // this is an event that belongs to a contract that's not in flight, throw it away and rely on the engine to trigger the action again when the orchestrator is wake up. (an enhanced version is to add weight on queueing an orchestrator)
		log.L(ctx).Warnf("Ignored event for domain contract %s and transaction %s on stage %s. If this happens a lot, check the orchestrator idle timeout is set to a reasonable number", stageEvent.ContractAddress, stageEvent.TxID, stageEvent.Stage)
	} else {
		targetOrchestrator.HandleEvent(ctx, stageEvent)
	}
}

func (e *engine) StartEventListener(ctx context.Context) {
	e.done = make(chan struct{})

	err := e.components.TransportManager().RegisterReceiver(func(ctx context.Context, message components.TransportMessage) error {
		//Send the event to the orchestrator for the contract and any transaction manager for the signing key
		messagePayload := message.Payload
		stageMessage := &pbEngine.StageMessage{}

		err := proto.Unmarshal(messagePayload, stageMessage)
		if err != nil {
			log.L(ctx).Errorf("Failed to unmarshal message payload: %s", err)
			return i18n.WrapError(ctx, err, msgs.MsgEngineParseFailed)
		}
		if stageMessage.ContractAddress == "" {
			log.L(ctx).Errorf("Invalid message: contract address is empty")
			return i18n.NewError(ctx, msgs.MsgEngineInvalidMessage)
		}

		dataProto, err := stageMessage.Data.UnmarshalNew()
		if err != nil {
			log.L(ctx).Errorf("Failed to unmarshal from any: %s", err)
			return i18n.WrapError(ctx, err, msgs.MsgEngineParseFailed)
		}

		e.HandleNewEvent(ctx, &types.StageEvent{
			ContractAddress: stageMessage.ContractAddress,
			TxID:            stageMessage.TransactionId,
			Stage:           stageMessage.Stage,
			Data:            dataProto,
		})

		return nil
	})
	if err != nil {
		log.L(ctx).Errorf("Failed to start event listener: %s", err)
		panic(err)
	}
}

// For now, this is here to help with testing but it seems like it could be useful thing to have
// in the future if we want to have an eventing interface but at such time we would need to put more effort
// into the reliabilty of the event delivery or maybe there is only a consumer of the event and it is responsible
// for managing multiple subscribers and durability etc...
func (e *engine) Subscribe(ctx context.Context, subscriber types.EventSubscriber) {
	e.subscribersLock.Lock()
	defer e.subscribersLock.Unlock()
	//TODO implement this
	e.subscribers = append(e.subscribers, subscriber)
}

func (e *engine) publishToSubscribers(ctx context.Context, event types.EngineEvent) {
	log.L(ctx).Debugf("Publishing event to subscribers")
	e.subscribersLock.Lock()
	defer e.subscribersLock.Unlock()
	for _, subscriber := range e.subscribers {
		subscriber(event)
	}
}
