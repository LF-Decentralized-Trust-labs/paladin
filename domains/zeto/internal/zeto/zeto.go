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
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

//go:embed abis/Commonlib.json
var commonLibJSON []byte // From "gradle copySolidity"

//go:embed abis/Groth16Verifier_Anon.json
var Groth16Verifier_Anon []byte // From "gradle copySolidity"

//go:embed abis/Groth16Verifier_CheckHashesValue.json
var Groth16Verifier_CheckHashesValue []byte // From "gradle copySolidity"

//go:embed abis/Groth16Verifier_CheckInputsOutputsValue.json
var Groth16Verifier_CheckInputsOutputsValue []byte // From "gradle copySolidity"

//go:embed abis/ZetoSampleFactory.json
var zetoFactoryJSON []byte // From "gradle copySolidity"

//go:embed abis/ZetoSample.json
var zetoJSON []byte // From "gradle copySolidity"

type Zeto struct {
	Callbacks plugintk.DomainCallbacks

	config     *types.Config
	chainID    int64
	domainID   string
	coinSchema *pb.StateSchema
}

type ZetoDeployParams struct {
	TransactionID    string                    `json:"transactionId"`
	Data             ethtypes.HexBytes0xPrefix `json:"data"`
	Verifier         string                    `json:"_verifier"`
	DepositVerifier  string                    `json:"_depositVerifier"`
	WithdrawVerifier string                    `json:"_withdrawVerifier"`
}

func (z *Zeto) ConfigureDomain(ctx context.Context, req *pb.ConfigureDomainRequest) (*pb.ConfigureDomainResponse, error) {
	var config types.Config
	err := json.Unmarshal([]byte(req.ConfigJson), &config)
	if err != nil {
		return nil, err
	}

	z.config = &config
	z.chainID = req.ChainId

	factory := domain.LoadBuildLinked(zetoFactoryJSON, config.Libraries)
	contract := domain.LoadBuildLinked(zetoJSON, config.Libraries)

	factoryJSON, err := json.Marshal(factory.ABI)
	if err != nil {
		return nil, err
	}
	zetoJSON, err := json.Marshal(contract.ABI)
	if err != nil {
		return nil, err
	}
	constructorJSON, err := json.Marshal(types.ZetoABI.Constructor())
	if err != nil {
		return nil, err
	}
	schemaJSON, err := json.Marshal(types.ZetoCoinABI)
	if err != nil {
		return nil, err
	}

	return &pb.ConfigureDomainResponse{
		DomainConfig: &pb.DomainConfig{
			FactoryContractAddress: config.FactoryAddress,
			FactoryContractAbiJson: string(factoryJSON),
			PrivateContractAbiJson: string(zetoJSON),
			ConstructorAbiJson:     string(constructorJSON),
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
	_, err := z.validateDeploy(req.Transaction)
	if err != nil {
		return nil, err
	}
	return &pb.InitDeployResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			// TODO: should we resolve anything?
		},
	}, nil
}

func (z *Zeto) PrepareDeploy(ctx context.Context, req *pb.PrepareDeployRequest) (*pb.PrepareDeployResponse, error) {
	params, err := z.validateDeploy(req.Transaction)
	if err != nil {
		return nil, err
	}
	deployParams := &ZetoDeployParams{
		TransactionID:    req.Transaction.TransactionId,
		Data:             ethtypes.HexBytes0xPrefix(""),
		DepositVerifier:  params.DepositVerifier,
		WithdrawVerifier: params.WithdrawVerifier,
		Verifier:         params.Verifier,
	}
	paramsJSON, err := json.Marshal(deployParams)
	if err != nil {
		return nil, err
	}

	return &pb.PrepareDeployResponse{
		Transaction: &pb.BaseLedgerTransaction{
			FunctionName: "deploy",
			ParamsJson:   string(paramsJSON),
		},
		Signer: &params.From,
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

func (z *Zeto) decodeDomainConfig(ctx context.Context, domainConfig []byte) (*types.DomainConfig, error) {
	configValues, err := types.DomainConfigABI.DecodeABIDataCtx(ctx, domainConfig, 0)
	if err != nil {
		return nil, err
	}
	configJSON, err := tktypes.StandardABISerializer().SerializeJSON(configValues)
	if err != nil {
		return nil, err
	}
	var config types.DomainConfig
	err = json.Unmarshal(configJSON, &config)
	return &config, err
}

func (z *Zeto) validateDeploy(tx *pb.DeployTransactionSpecification) (*types.ConstructorParams, error) {
	var params types.ConstructorParams
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
	params, err := handler.ValidateParams(tx.FunctionParamsJson)
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

	coins := make([]*types.ZetoCoin, len(states))
	for i, state := range states {
		if coins[i], err = z.makeCoin(state.DataJson); err != nil {
			return nil, err
		}
	}
	return coins, err
}
