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

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/pkg/blockindexer"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/types"
)

type testbedDomain struct {
	tb                     *testbed
	name                   string
	schemas                []*statestore.Schema
	constructorABI         *abi.Entry
	factoryContractAddress *ethtypes.Address0xHex
	factoryContractABI     abi.ABI
}

type testbedContract struct {
	// domain *testbedDomain
}

func (tb *testbed) registerDomain(ctx context.Context, name string, config *proto.DomainConfig) (*proto.InitDomainRequest, error) {

	abiSchemas := make([]*abi.Parameter, len(config.AbiStateSchemasJson))
	for i, schemaJSON := range config.AbiStateSchemasJson {
		if err := json.Unmarshal([]byte(schemaJSON), &abiSchemas[i]); err != nil {
			return nil, fmt.Errorf("bad ABI state schema %d: %s", i, err)
		}
	}
	domain := &testbedDomain{tb: tb, name: name}

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

	domain.factoryContractAddress, err = ethtypes.NewAddress(config.FactoryContractAddress)
	if err != nil {
		return nil, fmt.Errorf("bad factory contract address: %s", err)
	}

	flushed := make(chan struct{})
	err = tb.stateStore.RunInDomainContext(name, func(ctx context.Context, dsi statestore.DomainStateInterface) (err error) {
		domain.schemas, err = dsi.EnsureABISchemas(abiSchemas)
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

	schemaIDs := make([]string, len(domain.schemas))
	for i, s := range domain.schemas {
		schemaIDs[i] = s.Signature
	}

	tb.domainLock.Lock()
	defer tb.domainLock.Unlock()
	tb.domainRegistry[name] = domain
	return &proto.InitDomainRequest{
		AbiStateSchemaIds: schemaIDs,
	}, nil
}

func (tb *testbed) validateDeploy(ctx context.Context, domain *testbedDomain, constructorParams types.RawJSON) (*proto.DeployTransactionSpecification, error) {

	contructorValues, err := domain.constructorABI.Inputs.ParseJSONCtx(ctx, constructorParams)
	if err != nil {
		return nil, fmt.Errorf("invalid parameters for constructor: %s", err)
	}

	paladinTxID := uuid.New().String()
	constructorABIJSON, _ := json.Marshal(domain.constructorABI)
	constructorParamsJSON, _ := types.StandardABISerializer().SerializeJSONCtx(ctx, contructorValues)

	return &proto.DeployTransactionSpecification{
		TransactionId:         paladinTxID,
		ConstructorAbi:        string(constructorABIJSON),
		ConstructorParamsJson: string(constructorParamsJSON),
	}, nil
}

func (tb *testbed) deployPrivateSmartContract(ctx context.Context, domain *testbedDomain, txInstruction *proto.BaseLedgerTransaction) (*blockindexer.IndexedEvent, error) {

	var abiFunc ethclient.ABIFunctionClient
	abiClient, err := tb.ethClient.ABI(ctx, domain.factoryContractABI)
	if err == nil {
		abiFunc, err = abiClient.Function(ctx, txInstruction.FunctionName)
	}
	if err != nil {
		return nil, fmt.Errorf("function %q does not exist on base ledger ABI: %s", txInstruction.FunctionName, err)
	}

	// Send the transaction
	var tx *blockindexer.IndexedTransaction
	txHash, err := abiFunc.R(ctx).
		Signer(txInstruction.SigningAddress).
		To(domain.factoryContractAddress).
		Input(txInstruction.ParamsJson).
		SignAndSend()
	if err == nil {
		tx, err = tb.blockindexer.WaitForTransaction(ctx, txHash.String())
	}
	if err != nil {
		return nil, fmt.Errorf("failed to send base ledger transaction: %s", err)
	}

	events, err := tb.blockindexer.GetTransactionEventsByHash(ctx, tx.Hash.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction events for deploy: %s", err)
	}

	if len(events) != 1 {
		return nil, fmt.Errorf("expected exactly one event from deploy function TX %s (received=%d)", &tx.Hash, len(events))
	}
	event := events[0]

	return event, nil
}

func (tb *testbed) getDomain(name string) (*testbedDomain, error) {
	tb.domainLock.Lock()
	defer tb.domainLock.Unlock()
	domain := tb.domainRegistry[name]
	if domain == nil {
		return nil, fmt.Errorf("domain %q not found", name)
	}
	return domain, nil

}
