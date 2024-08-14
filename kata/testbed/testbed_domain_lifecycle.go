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

package main

import (
	"context"
	"encoding/json"
	"fmt"

	_ "embed"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/pkg/blockindexer"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"gorm.io/gorm"
)

//go:embed abis/IPaladinContract_V0.json
var iPaladinContractABIJSON []byte

var iPaladinContractABI = mustParseBuildABI(iPaladinContractABIJSON)
var iPaladinNewSmartContract_V0_Signature = mustEventSignatureHash(iPaladinContractABI, "PaladinNewSmartContract_V0")

type iPaladinNewSmartContract_V0_Type struct {
	Domain *ethtypes.Address0xHex    `json:"domain"`
	TXID   ethtypes.HexBytes0xPrefix `json:"txId"`
	Data   ethtypes.HexBytes0xPrefix `json:"data"`
}

type tbDomain struct {
	*transactionWaitUtils[*tbPrivateSmartContract]
	tb                     *testbed
	instanceUUID           uuid.UUID
	config                 *proto.DomainConfig
	name                   string
	schemasBySignature     map[string]statestore.Schema
	schemasByID            map[string]statestore.Schema
	constructorABI         *abi.Entry
	factoryContractAddress *ethtypes.Address0xHex
	factoryContractABI     abi.ABI
	privateContractABI     abi.ABI
}

func (tb *testbed) registerDomain(ctx context.Context, name string, config *proto.DomainConfig) (*proto.InitDomainRequest, error) {

	abiSchemas := make([]*abi.Parameter, len(config.AbiStateSchemasJson))
	for i, schemaJSON := range config.AbiStateSchemasJson {
		if err := json.Unmarshal([]byte(schemaJSON), &abiSchemas[i]); err != nil {
			return nil, fmt.Errorf("bad ABI state schema %d: %s", i, err)
		}
	}
	domain := &tbDomain{
		transactionWaitUtils: newTransactionWaitUtils[*tbPrivateSmartContract](),
		tb:                   tb,
		instanceUUID:         uuid.New(),
		config:               config,
		name:                 name,
		schemasByID:          make(map[string]statestore.Schema),
		schemasBySignature:   make(map[string]statestore.Schema),
	}

	err := json.Unmarshal(([]byte)(config.ConstructorAbiJson), &domain.constructorABI)
	if err != nil {
		return nil, fmt.Errorf("bad constructor ABI function definition: %s", err)
	}
	if domain.constructorABI.Type != abi.Constructor {
		return nil, fmt.Errorf("bad constructor ABI function definition: type not 'constructor'")
	}

	if err := json.Unmarshal(([]byte)(config.FactoryContractAbiJson), &domain.factoryContractABI); err != nil {
		return nil, fmt.Errorf("bad factory contract ABI: %s", err)
	}

	if err := json.Unmarshal(([]byte)(config.PrivateContractAbiJson), &domain.privateContractABI); err != nil {
		return nil, fmt.Errorf("bad private contract ABI: %s", err)
	}

	domain.factoryContractAddress, err = ethtypes.NewAddress(config.FactoryContractAddress)
	if err != nil {
		return nil, fmt.Errorf("bad factory contract address: %s", err)
	}

	if config.BaseLedgerSubmitConfig == nil {
		config.BaseLedgerSubmitConfig = &proto.BaseLedgerSubmitConfig{}
	}
	if config.BaseLedgerSubmitConfig.SubmitMode == proto.BaseLedgerSubmitConfig_ONE_TIME_USE_KEYS {
		if config.BaseLedgerSubmitConfig.OneTimeUsePrefix == "" {
			config.BaseLedgerSubmitConfig.OneTimeUsePrefix = fmt.Sprintf("domain/%s/tx/", domain.name)
		}
	}

	flushed := make(chan struct{})
	var schemas []statestore.Schema
	err = tb.stateStore.RunInDomainContext(name, func(ctx context.Context, dsi statestore.DomainStateInterface) (err error) {
		schemas, err = dsi.EnsureABISchemas(abiSchemas)
		if err == nil {
			err = dsi.Flush(func(ctx context.Context, dsi statestore.DomainStateInterface) error {
				close(flushed)
				return nil
			})
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	select {
	case <-flushed:
	case <-ctx.Done():
		return nil, fmt.Errorf("flush timed out")
	}

	schemasProto := make([]*proto.StateSchema, len(schemas))
	for i, s := range schemas {
		schemaID := s.ID()
		domain.schemasByID[schemaID] = s
		domain.schemasBySignature[s.Signature()] = s
		schemasProto[i] = &proto.StateSchema{
			Id:        schemaID,
			Signature: s.Signature(),
		}
	}

	tb.domainLock.Lock()
	defer tb.domainLock.Unlock()
	tb.domainsByName[name] = domain
	tb.domainsByUUID[domain.instanceUUID] = domain
	tb.domainsByAddress[*domain.factoryContractAddress] = domain
	return &proto.InitDomainRequest{
		DomainUuid:      domain.instanceUUID.String(),
		AbiStateSchemas: schemasProto,
	}, nil
}

func (tb *testbed) domainEventStream() *blockindexer.InternalEventStream {
	return &blockindexer.InternalEventStream{
		Definition: &blockindexer.EventStream{
			Name: "testbed_paladin_domain_abi_es",
			ABI:  iPaladinContractABI,
		},
		Handler: tb.handleNewSmartContract,
	}
}

func (tb *testbed) handleNewSmartContract(ctx context.Context, tx *gorm.DB, batch *blockindexer.EventDeliveryBatch) error {
	for _, e := range batch.Events {
		if iPaladinNewSmartContract_V0_Signature.Equals(e.Signature.Bytes()) {
			var eventParsed iPaladinNewSmartContract_V0_Type
			err := json.Unmarshal(e.Data, &eventParsed)
			if err == nil {
				err = tb.addDomainContractFromEvent(ctx, &e.Address, &eventParsed)
			}
			if err != nil {
				return fmt.Errorf("failed to parse event: %s", err)
			}
		}
	}
	return nil
}

func (tb *testbed) addDomainContractFromEvent(ctx context.Context, emitter *types.EthAddress, eventParsed *iPaladinNewSmartContract_V0_Type) error {
	domain := tb.getDomainByAddress(eventParsed.Domain)
	if domain == nil {
		log.L(ctx).Warnf("Received paladin smart contract notification for unknown domain: %s", eventParsed.Domain)
		return nil
	}

	// It is important that the event is emitted by the constructed contract, rather than the domain.
	// This allows:
	// 1) The domain to be written in any form - does not need to be a factory, but can be
	// 2) The created contract's globally unique ID to be trusted, because it is address of the emitter of the event (not data in the event)
	if tb.domainContracts[*emitter.Address0xHex()] == nil {
		newContract := &tbPrivateSmartContract{
			tb:      tb,
			domain:  domain,
			data:    eventParsed.Data,
			address: emitter.Address0xHex(),
		}
		tb.domainContracts[*emitter.Address0xHex()] = newContract
		domain.notifyTX(ctx, bytes32ToUUID(eventParsed.TXID), newContract)
	}
	return nil
}

func uuidToHexBytes32(id uuid.UUID) ethtypes.HexBytes0xPrefix {
	paladinTxID := make(ethtypes.HexBytes0xPrefix, 32)
	copy(paladinTxID, id[:])
	return paladinTxID
}

func bytes32ToUUID(bytes ethtypes.HexBytes0xPrefix) uuid.UUID {
	var id uuid.UUID
	copy(id[:], bytes[0:16])
	return id
}

func (domain *tbDomain) validateDeploy(ctx context.Context, constructorParams types.RawJSON) (*uuid.UUID, *proto.DeployTransactionSpecification, error) {

	contructorValues, err := domain.constructorABI.Inputs.ParseJSONCtx(ctx, constructorParams)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid parameters for constructor: %s", err)
	}

	constructorABIJSON, _ := json.Marshal(domain.constructorABI)
	constructorParamsJSON, _ := types.StandardABISerializer().SerializeJSONCtx(ctx, contructorValues)

	txID := uuid.New()
	return &txID, &proto.DeployTransactionSpecification{
		TransactionId:         uuidToHexBytes32(txID).String(),
		ConstructorAbi:        string(constructorABIJSON),
		ConstructorParamsJson: string(constructorParamsJSON),
	}, nil
}

func (tb *testbed) execBaseLedgerDeployTransaction(ctx context.Context, abi abi.ABI, signer string, txInstruction *proto.BaseLedgerDeployTransaction) error {

	var abiFunc ethclient.ABIFunctionClient
	abiClient, err := tb.ethClient.ABI(ctx, abi)
	if err == nil {
		abiFunc, err = abiClient.Constructor(ctx, txInstruction.Bytecode)
	}
	if err != nil {
		return fmt.Errorf("failed to process ABI constructor: %s", err)
	}

	// Send the transaction
	txHash, err := abiFunc.R(ctx).
		Signer(signer).
		Input(txInstruction.ParamsJson).
		SignAndSend()
	if err == nil {
		_, err = tb.blockindexer.WaitForTransaction(ctx, txHash.String())
	}
	if err != nil {
		return fmt.Errorf("failed to send base deploy ledger transaction: %s", err)
	}
	return nil
}

func (tb *testbed) execBaseLedgerTransaction(ctx context.Context, abi abi.ABI, to *ethtypes.Address0xHex, signer string, txInstruction *proto.BaseLedgerTransaction) error {

	var abiFunc ethclient.ABIFunctionClient
	abiClient, err := tb.ethClient.ABI(ctx, abi)
	if err == nil {
		abiFunc, err = abiClient.Function(ctx, txInstruction.FunctionName)
	}
	if err != nil {
		return fmt.Errorf("function %q does not exist on base ledger ABI: %s", txInstruction.FunctionName, err)
	}

	// Send the transaction
	txHash, err := abiFunc.R(ctx).
		Signer(signer).
		To(to).
		Input(txInstruction.ParamsJson).
		SignAndSend()
	if err == nil {
		_, err = tb.blockindexer.WaitForTransaction(ctx, txHash.String())
	}
	if err != nil {
		return fmt.Errorf("failed to send base ledger transaction: %s", err)
	}
	return nil
}

func (tb *testbed) getDomainByName(name string) (*tbDomain, error) {
	tb.domainLock.Lock()
	defer tb.domainLock.Unlock()
	domain := tb.domainsByName[name]
	if domain == nil {
		return nil, fmt.Errorf("domain %q not found", name)
	}
	return domain, nil

}

func (tb *testbed) getDomainByAddress(addr *ethtypes.Address0xHex) *tbDomain {
	tb.domainLock.Lock()
	defer tb.domainLock.Unlock()
	return tb.domainsByAddress[*addr]
}

func (tb *testbed) getDomainByUUID(uuidStr string) (domain *tbDomain) {
	tb.domainLock.Lock()
	defer tb.domainLock.Unlock()
	id, err := uuid.Parse(uuidStr)
	if err == nil {
		domain = tb.domainsByUUID[id]
	}
	return domain
}

func (tb *testbed) getDomainContract(addr *ethtypes.Address0xHex) *tbPrivateSmartContract {
	tb.domainLock.Lock()
	defer tb.domainLock.Unlock()
	return tb.domainContracts[*addr]
}
