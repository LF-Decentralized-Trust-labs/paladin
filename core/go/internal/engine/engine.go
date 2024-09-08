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

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/core/internal/engine/orchestrator"
	"github.com/kaleido-io/paladin/core/internal/engine/sequencer"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/transactionstore"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	pbEngine "github.com/kaleido-io/paladin/core/pkg/proto/engine"
	"github.com/kaleido-io/paladin/core/pkg/types"
	"google.golang.org/protobuf/proto"

	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type Engine interface {
	components.Engine
	HandleNewEvent(ctx context.Context, stageEvent *enginespi.StageEvent)
	HandleNewTx(ctx context.Context, tx *components.PrivateTransaction) (txID string, err error)
	HandleDeployTx(ctx context.Context, tx *components.PrivateContractDeploy) (txID string, contractAddress string, err error)

	GetTxStatus(ctx context.Context, domainAddress string, txID string) (status enginespi.TxStatus, err error)
	Subscribe(ctx context.Context, subscriber enginespi.EventSubscriber)
}

type engine struct {
	ctx             context.Context
	ctxCancel       func()
	orchestrators   map[string]*orchestrator.Orchestrator
	components      components.PreInitComponentsAndManagers
	nodeID          uuid.UUID
	subscribers     []enginespi.EventSubscriber
	subscribersLock sync.Mutex
}

// Init implements Engine.
func (e *engine) Init(c components.PreInitComponentsAndManagers) (*components.ManagerInitResult, error) {
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
		subscribers:   make([]enginespi.EventSubscriber, 0),
	}
}

func (e *engine) getOrchestratorForContract(ctx context.Context, contractAddr *types.EthAddress, domainAPI components.DomainSmartContract) (oc *orchestrator.Orchestrator, err error) {

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
		endorsementGatherer := NewEndorsementGatherer(domainAPI, e.components.KeyManager())
		e.orchestrators[contractAddr.String()] =
			orchestrator.NewOrchestrator(
				ctx, e.nodeID,
				contractAddr.String(), /** TODO: fill in the real plug-ins*/
				&orchestrator.OrchestratorConfig{},
				e.components,
				domainAPI,
				publisher,
				seq,
				endorsementGatherer,
			)
		orchestratorDone, err := e.orchestrators[contractAddr.String()].Start(ctx)
		if err != nil {
			log.L(ctx).Errorf("Failed to start orchestrator for contract %s: %s", contractAddr.String(), err)
			return nil, err
		}

		go func() {
			<-orchestratorDone
			log.L(ctx).Infof("Orchestrator for contract %s has stopped", contractAddr.String())
		}()
	}
	return e.orchestrators[contractAddr.String()], nil
}

// HandleNewTx implements Engine.
func (e *engine) HandleNewTx(ctx context.Context, tx *components.PrivateTransaction) (txID string, err error) { // TODO: this function currently assumes another layer initialize transactions and store them into DB
	log.L(ctx).Debugf("Handling new transaction: %v", tx)
	if tx.Inputs == nil || tx.Inputs.Domain == "" {
		return "", i18n.NewError(ctx, msgs.MsgDomainNotProvided)
	}

	emptyAddress := types.EthAddress{}
	if tx.Inputs.To == emptyAddress {
		return "", i18n.NewError(ctx, msgs.MsgContractAddressNotProvided)
	}

	contractAddr := tx.Inputs.To
	domainAPI, err := e.components.DomainManager().GetSmartContractByAddress(ctx, contractAddr)
	if err != nil {
		return "", err
	}
	err = domainAPI.InitTransaction(ctx, tx)
	if err != nil {
		return "", err
	}

	//Resolve keys synchronously (rather than having an orchestrator stage for it) so that we can return an error if any key resolution fails
	keyMgr := e.components.KeyManager()
	if tx.PreAssembly == nil {
		return "", i18n.NewError(ctx, msgs.MsgEngineInternalError, "PreAssembly is nil")
	}
	tx.PreAssembly.Verifiers = make([]*prototk.ResolvedVerifier, len(tx.PreAssembly.RequiredVerifiers))
	for i, v := range tx.PreAssembly.RequiredVerifiers {
		_, verifier, err := keyMgr.ResolveKey(ctx, v.Lookup, v.Algorithm)
		if err != nil {
			return "", i18n.WrapError(ctx, err, msgs.MsgKeyResolutionFailed, v.Lookup, v.Algorithm)
		}
		tx.PreAssembly.Verifiers[i] = &prototk.ResolvedVerifier{
			Lookup:    v.Lookup,
			Algorithm: v.Algorithm,
			Verifier:  verifier,
		}
	}

	txInstance := transactionstore.NewTransactionStageManager(ctx, tx)
	// TODO how to measure fairness/ per From address / contract address / something else

	oc, err := e.getOrchestratorForContract(ctx, &contractAddr, domainAPI)
	if err != nil {
		return "", err
	}
	queued := oc.ProcessNewTransaction(ctx, txInstance)
	if queued {
		log.L(ctx).Debugf("Transaction with ID %s queued in database", txInstance.GetTxID(ctx))
	}
	return txInstance.GetTxID(ctx), nil
}

// Synchronous function to deploy a domain smart contract
// TODO should this be async?  How does this plug into the dispatch stages given that we don't have an orchestrator yet?
func (e *engine) HandleDeployTx(ctx context.Context, tx *components.PrivateContractDeploy) (txID string, contractAddress string, err error) { // TODO: this function currently assumes another layer initialize transactions and store them into DB
	log.L(ctx).Debugf("Handling new private contract deploy transaction: %v", tx)
	if tx.Domain == "" {
		return "", "", i18n.NewError(ctx, msgs.MsgDomainNotProvided)
	}

	domain, err := e.components.DomainManager().GetDomainByName(ctx, tx.Domain)
	if err != nil {
		return "", "", i18n.WrapError(ctx, err, msgs.MsgDomainNotFound, tx.Domain)
	}

	err = domain.InitDeploy(ctx, tx)
	if err != nil {
		return "", "", i18n.WrapError(ctx, err, msgs.MsgDeployInitFailed)
	}

	//Resolve keys synchronously (rather than having an orchestrator stage for it) so that we can return an error if any key resolution fails
	keyMgr := e.components.KeyManager()
	tx.Verifiers = make([]*prototk.ResolvedVerifier, len(tx.RequiredVerifiers))
	for i, v := range tx.RequiredVerifiers {
		_, verifier, err := keyMgr.ResolveKey(ctx, v.Lookup, v.Algorithm)
		if err != nil {
			return "", "", i18n.WrapError(ctx, err, msgs.MsgKeyResolutionFailed, v.Lookup, v.Algorithm)
		}
		tx.Verifiers[i] = &prototk.ResolvedVerifier{
			Lookup:    v.Lookup,
			Algorithm: v.Algorithm,
			Verifier:  verifier,
		}
	}

	//TODO should the following be done asyncronously?

	err = domain.PrepareDeploy(ctx, tx)
	if err != nil {
		return "", "", i18n.WrapError(ctx, err, msgs.MsgDeployPrepareFailed)
	}

	//Placeholder for integration with baseledge transaction engine
	if tx.DeployTransaction != nil && tx.InvokeTransaction == nil {
		err = e.execBaseLedgerDeployTransaction(ctx, tx.Signer, tx.DeployTransaction)
	} else if tx.InvokeTransaction != nil && tx.DeployTransaction == nil {
		err = e.execBaseLedgerTransaction(ctx, tx.Signer, tx.InvokeTransaction)
	} else {
		return "", "", i18n.NewError(ctx, msgs.MsgDeployPrepareIncomplete)
	}
	if err != nil {
		return "", "", i18n.WrapError(ctx, err, msgs.MsgBaseLedgerTransactionFailed)
	}

	psc, err := e.components.DomainManager().WaitForDeploy(ctx, tx.ID)
	if err != nil {
		return "", "", i18n.WrapError(ctx, err, msgs.MsgBaseLedgerTransactionFailed)
	}
	addr := psc.Address()

	return tx.ID.String(), addr.String(), nil

}

func (e *engine) execBaseLedgerDeployTransaction(ctx context.Context, signer string, txInstruction *components.EthDeployTransaction) error {

	var abiFunc ethclient.ABIFunctionClient
	ec := e.components.EthClientFactory().HTTPClient()
	abiFunc, err := ec.ABIConstructor(ctx, txInstruction.ConstructorABI, types.HexBytes(txInstruction.Bytecode))
	if err != nil {
		return err
	}

	// Send the transaction
	txHash, err := abiFunc.R(ctx).
		Signer(signer).
		Input(txInstruction.Inputs).
		SignAndSend()
	if err == nil {
		_, err = e.components.BlockIndexer().WaitForTransaction(ctx, *txHash)
	}
	if err != nil {
		return fmt.Errorf("failed to send base deploy ledger transaction: %s", err)
	}
	return nil
}

func (e *engine) execBaseLedgerTransaction(ctx context.Context, signer string, txInstruction *components.EthTransaction) error {

	var abiFunc ethclient.ABIFunctionClient
	ec := e.components.EthClientFactory().HTTPClient()
	abiFunc, err := ec.ABIFunction(ctx, txInstruction.FunctionABI)
	if err != nil {
		return err
	}

	// Send the transaction
	addr := ethtypes.Address0xHex(txInstruction.To)
	txHash, err := abiFunc.R(ctx).
		Signer(signer).
		To(&addr).
		Input(txInstruction.Inputs).
		SignAndSend()
	if err == nil {
		_, err = e.components.BlockIndexer().WaitForTransaction(ctx, *txHash)
	}
	if err != nil {
		return fmt.Errorf("failed to send base ledger transaction: %s", err)
	}
	return nil
}

func (e *engine) GetTxStatus(ctx context.Context, domainAddress string, txID string) (status enginespi.TxStatus, err error) {
	//TODO This is primarily here to help with testing for now
	// this needs to be revisited ASAP as part of a holisitic review of the persistence model
	targetOrchestrator := e.orchestrators[domainAddress]
	if targetOrchestrator == nil {
		//TODO should be valid to query the status of a transaction that belongs to a domain instance that is not currently active
		return enginespi.TxStatus{}, i18n.NewError(ctx, msgs.MsgEngineInternalError)
	} else {
		return targetOrchestrator.GetTxStatus(ctx, txID)
	}

}

func (e *engine) HandleNewEvent(ctx context.Context, stageEvent *enginespi.StageEvent) {
	targetOrchestrator := e.orchestrators[stageEvent.ContractAddress]
	if targetOrchestrator == nil { // this is an event that belongs to a contract that's not in flight, throw it away and rely on the engine to trigger the action again when the orchestrator is wake up. (an enhanced version is to add weight on queueing an orchestrator)
		log.L(ctx).Warnf("Ignored event for domain contract %s and transaction %s on stage %s. If this happens a lot, check the orchestrator idle timeout is set to a reasonable number", stageEvent.ContractAddress, stageEvent.TxID, stageEvent.Stage)
	} else {
		targetOrchestrator.HandleEvent(ctx, stageEvent)
	}
}

func (e *engine) ReceiveTransportMessage(ctx context.Context, message *components.TransportMessage) {
	//Send the event to the orchestrator for the contract and any transaction manager for the signing key
	messagePayload := message.Payload
	stageMessage := &pbEngine.StageMessage{}

	err := proto.Unmarshal(messagePayload, stageMessage)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal message payload: %s", err)
		return
	}
	if stageMessage.ContractAddress == "" {
		log.L(ctx).Errorf("Invalid message: contract address is empty")
		return
	}

	dataProto, err := stageMessage.Data.UnmarshalNew()
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal from any: %s", err)
		return
	}

	e.HandleNewEvent(ctx, &enginespi.StageEvent{
		ContractAddress: stageMessage.ContractAddress,
		TxID:            stageMessage.TransactionId,
		Stage:           stageMessage.Stage,
		Data:            dataProto,
	})
}

// For now, this is here to help with testing but it seems like it could be useful thing to have
// in the future if we want to have an eventing interface but at such time we would need to put more effort
// into the reliabilty of the event delivery or maybe there is only a consumer of the event and it is responsible
// for managing multiple subscribers and durability etc...
func (e *engine) Subscribe(ctx context.Context, subscriber enginespi.EventSubscriber) {
	e.subscribersLock.Lock()
	defer e.subscribersLock.Unlock()
	//TODO implement this
	e.subscribers = append(e.subscribers, subscriber)
}

func (e *engine) publishToSubscribers(ctx context.Context, event enginespi.EngineEvent) {
	log.L(ctx).Debugf("Publishing event to subscribers")
	e.subscribersLock.Lock()
	defer e.subscribersLock.Unlock()
	for _, subscriber := range e.subscribers {
		subscriber(event)
	}
}
