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

package zeto

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

//go:embed abis/ZetoFactory.json
var factoryJSONBytes []byte // From "gradle copySolidity"

type Zeto struct {
	Callbacks plugintk.DomainCallbacks

	config     *types.DomainFactoryConfig
	chainID    int64
	domainID   string
	coinSchema *pb.StateSchema
	factoryABI abi.ABI
}

func New(callbacks plugintk.DomainCallbacks) *Zeto {
	return &Zeto{
		Callbacks: callbacks,
	}
}

func (z *Zeto) ConfigureDomain(ctx context.Context, req *pb.ConfigureDomainRequest) (*pb.ConfigureDomainResponse, error) {
	var config types.DomainFactoryConfig
	err := json.Unmarshal([]byte(req.ConfigJson), &config)
	if err != nil {
		return nil, err
	}

	z.config = &config
	z.chainID = req.ChainId

	factory := domain.LoadBuildLinked(factoryJSONBytes, config.Libraries)
	z.factoryABI = factory.ABI

	schemaJSON, err := json.Marshal(types.ZetoCoinABI)
	if err != nil {
		return nil, err
	}

	return &pb.ConfigureDomainResponse{
		DomainConfig: &pb.DomainConfig{
			FactoryContractAddress: config.FactoryAddress,
			AbiStateSchemasJson:    []string{string(schemaJSON)},
			BaseLedgerSubmitConfig: &pb.BaseLedgerSubmitConfig{
				SubmitMode: pb.BaseLedgerSubmitConfig_ENDORSER_SUBMISSION,
			},
		},
	}, nil
}

func (z *Zeto) InitDomain(ctx context.Context, req *pb.InitDomainRequest) (*pb.InitDomainResponse, error) {
	z.domainID = req.DomainUuid
	z.coinSchema = req.AbiStateSchemas[0]
	return &pb.InitDomainResponse{}, nil
}

func (z *Zeto) InitDeploy(ctx context.Context, req *pb.InitDeployRequest) (*pb.InitDeployResponse, error) {
	initParams, err := z.validateDeploy(req.Transaction)
	if err != nil {
		return nil, err
	}
	return &pb.InitDeployResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:    initParams.From,
				Algorithm: algorithms.ECDSA_SECP256K1_PLAINBYTES,
			},
		},
	}, nil
}

func (z *Zeto) PrepareDeploy(ctx context.Context, req *pb.PrepareDeployRequest) (*pb.PrepareDeployResponse, error) {
	initParams, err := z.validateDeploy(req.Transaction)
	if err != nil {
		return nil, err
	}
	circuitId, err := z.config.GetCircuitId(initParams.TokenName)
	if err != nil {
		return nil, err
	}
	config := &types.DomainInstanceConfig{
		CircuitId: circuitId,
		TokenName: initParams.TokenName,
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	encoded, err := types.DomainInstanceConfigABI.EncodeABIDataJSONCtx(ctx, configJSON)
	if err != nil {
		return nil, err
	}

	deployParams := &types.DeployParams{
		TransactionID: req.Transaction.TransactionId,
		Data:          ethtypes.HexBytes0xPrefix(encoded),
		TokenName:     initParams.TokenName,
		InitialOwner:  req.ResolvedVerifiers[0].Verifier, // TODO: allow the initial owner to be specified by the deploy request
	}
	paramsJSON, err := json.Marshal(deployParams)
	if err != nil {
		return nil, err
	}
	functionJSON, err := json.Marshal(z.factoryABI.Functions()["deploy"])
	if err != nil {
		return nil, err
	}

	return &pb.PrepareDeployResponse{
		Transaction: &pb.BaseLedgerTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
		},
		Signer: &initParams.From,
	}, nil
}

func (z *Zeto) InitTransaction(ctx context.Context, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	tx, handler, err := z.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return handler.Init(ctx, tx, req)
}

func (z *Zeto) AssembleTransaction(ctx context.Context, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	tx, handler, err := z.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return handler.Assemble(ctx, tx, req)
}

func (z *Zeto) EndorseTransaction(ctx context.Context, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	tx, handler, err := z.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return handler.Endorse(ctx, tx, req)
}

func (z *Zeto) PrepareTransaction(ctx context.Context, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	tx, handler, err := z.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return handler.Prepare(ctx, tx, req)
}

func (z *Zeto) decodeDomainConfig(ctx context.Context, domainConfig []byte) (*types.DomainInstanceConfig, error) {
	configValues, err := types.DomainInstanceConfigABI.DecodeABIDataCtx(ctx, domainConfig, 0)
	if err != nil {
		return nil, err
	}
	configJSON, err := tktypes.StandardABISerializer().SerializeJSON(configValues)
	if err != nil {
		return nil, err
	}
	var config types.DomainInstanceConfig
	err = json.Unmarshal(configJSON, &config)
	return &config, err
}

func (z *Zeto) validateDeploy(tx *pb.DeployTransactionSpecification) (*types.InitializerParams, error) {
	var params types.InitializerParams
	err := json.Unmarshal([]byte(tx.ConstructorParamsJson), &params)
	return &params, err
}

func (z *Zeto) validateTransaction(ctx context.Context, tx *pb.TransactionSpecification) (*types.ParsedTransaction, types.DomainHandler, error) {
	var functionABI abi.Entry
	err := json.Unmarshal([]byte(tx.FunctionAbiJson), &functionABI)
	if err != nil {
		return nil, nil, err
	}

	abi := types.ZetoABI.Functions()[functionABI.Name]
	handler := z.GetHandler(functionABI.Name)
	if abi == nil || handler == nil {
		return nil, nil, fmt.Errorf("unknown function: %s", functionABI.Name)
	}
	params, err := handler.ValidateParams(ctx, tx.FunctionParamsJson)
	if err != nil {
		return nil, nil, err
	}

	signature, _, err := abi.SolidityDefCtx(ctx)
	if err != nil {
		return nil, nil, err
	}
	if tx.FunctionSignature != signature {
		return nil, nil, fmt.Errorf("unexpected signature for function '%s': expected=%s actual=%s", functionABI.Name, signature, tx.FunctionSignature)
	}

	domainConfig, err := z.decodeDomainConfig(ctx, tx.ContractConfig)
	if err != nil {
		return nil, nil, err
	}

	contractAddress, err := ethtypes.NewAddress(tx.ContractAddress)
	if err != nil {
		return nil, nil, err
	}

	return &types.ParsedTransaction{
		Transaction:     tx,
		FunctionABI:     &functionABI,
		ContractAddress: contractAddress,
		DomainConfig:    domainConfig,
		Params:          params,
	}, handler, nil
}

func (z *Zeto) FindCoins(ctx context.Context, query string) ([]*types.ZetoCoin, error) {
	states, err := z.findAvailableStates(ctx, query)
	if err != nil {
		return nil, err
	}
	fmt.Printf("states: %+v\n", states)

	coins := make([]*types.ZetoCoin, len(states))
	for i, state := range states {
		if coins[i], err = z.makeCoin(state.DataJson); err != nil {
			return nil, err
		}
	}
	return coins, err
}
