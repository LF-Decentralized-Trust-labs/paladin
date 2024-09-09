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

package noto

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

//go:embed abis/NotoFactory.json
var notoFactoryJSON []byte // From "gradle copySolidity"

//go:embed abis/Noto.json
var notoJSON []byte // From "gradle copySolidity"

//go:embed abis/NotoSelfSubmitFactory.json
var notoSelfSubmitFactoryJSON []byte // From "gradle copySolidity"

//go:embed abis/NotoSelfSubmit.json
var notoSelfSubmitJSON []byte // From "gradle copySolidity"

type Noto struct {
	Callbacks plugintk.DomainCallbacks

	config     *types.Config
	chainID    int64
	domainID   string
	coinSchema *pb.StateSchema
}

type NotoDeployParams struct {
	TransactionID string                    `json:"transactionId"`
	Notary        string                    `json:"notary"`
	Data          ethtypes.HexBytes0xPrefix `json:"data"`
}

type gatheredCoins struct {
	inCoins   []*types.NotoCoin
	inStates  []*pb.StateRef
	inTotal   *big.Int
	outCoins  []*types.NotoCoin
	outStates []*pb.StateRef
	outTotal  *big.Int
}

func (n *Noto) ConfigureDomain(ctx context.Context, req *pb.ConfigureDomainRequest) (*pb.ConfigureDomainResponse, error) {
	var config types.Config
	err := json.Unmarshal([]byte(req.ConfigJson), &config)
	if err != nil {
		return nil, err
	}

	n.config = &config
	n.chainID = req.ChainId

	var factory *domain.SolidityBuild
	var contract *domain.SolidityBuild
	switch config.Variant {
	case "", "Noto":
		config.Variant = "Noto"
		factory = domain.LoadBuild(notoFactoryJSON)
		contract = domain.LoadBuild(notoJSON)
	case "NotoSelfSubmit":
		factory = domain.LoadBuild(notoSelfSubmitFactoryJSON)
		contract = domain.LoadBuild(notoSelfSubmitJSON)
	default:
		return nil, fmt.Errorf("unrecognized variant: %s", config.Variant)
	}

	factoryJSON, err := json.Marshal(factory.ABI)
	if err != nil {
		return nil, err
	}
	notoJSON, err := json.Marshal(contract.ABI)
	if err != nil {
		return nil, err
	}
	constructorJSON, err := json.Marshal(types.NotoABI.Constructor())
	if err != nil {
		return nil, err
	}
	schemaJSON, err := json.Marshal(types.NotoCoinABI)
	if err != nil {
		return nil, err
	}

	return &pb.ConfigureDomainResponse{
		DomainConfig: &pb.DomainConfig{
			FactoryContractAddress: config.FactoryAddress,
			FactoryContractAbiJson: string(factoryJSON),
			PrivateContractAbiJson: string(notoJSON),
			ConstructorAbiJson:     string(constructorJSON),
			AbiStateSchemasJson:    []string{string(schemaJSON)},
			BaseLedgerSubmitConfig: &pb.BaseLedgerSubmitConfig{
				SubmitMode: pb.BaseLedgerSubmitConfig_ENDORSER_SUBMISSION,
			},
		},
	}, nil
}

func (n *Noto) InitDomain(ctx context.Context, req *pb.InitDomainRequest) (*pb.InitDomainResponse, error) {
	n.domainID = req.DomainUuid
	n.coinSchema = req.AbiStateSchemas[0]
	return &pb.InitDomainResponse{}, nil
}

func (n *Noto) InitDeploy(ctx context.Context, req *pb.InitDeployRequest) (*pb.InitDeployResponse, error) {
	params, err := n.validateDeploy(req.Transaction)
	if err != nil {
		return nil, err
	}
	return &pb.InitDeployResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:    params.Notary,
				Algorithm: algorithms.ECDSA_SECP256K1_PLAINBYTES,
			},
		},
	}, nil
}

func (n *Noto) PrepareDeploy(ctx context.Context, req *pb.PrepareDeployRequest) (*pb.PrepareDeployResponse, error) {
	_, err := n.validateDeploy(req.Transaction)
	if err != nil {
		return nil, err
	}
	config := &types.DomainConfig{
		NotaryLookup:  req.ResolvedVerifiers[0].Lookup,
		NotaryAddress: req.ResolvedVerifiers[0].Verifier,
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	data, err := types.DomainConfigABI.EncodeABIDataJSONCtx(ctx, configJSON)
	if err != nil {
		return nil, err
	}

	params := &NotoDeployParams{
		TransactionID: req.Transaction.TransactionId,
		Notary:        config.NotaryAddress,
		Data:          data,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	return &pb.PrepareDeployResponse{
		Transaction: &pb.BaseLedgerTransaction{
			FunctionName: "deploy",
			ParamsJson:   string(paramsJSON),
		},
		Signer: &config.NotaryLookup,
	}, nil
}

func (n *Noto) InitTransaction(ctx context.Context, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	tx, handler, err := n.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return handler.Init(ctx, tx, req)
}

func (n *Noto) AssembleTransaction(ctx context.Context, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	tx, handler, err := n.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return handler.Assemble(ctx, tx, req)
}

func (n *Noto) EndorseTransaction(ctx context.Context, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	tx, handler, err := n.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return handler.Endorse(ctx, tx, req)
}

func (n *Noto) PrepareTransaction(ctx context.Context, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	tx, handler, err := n.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return handler.Prepare(ctx, tx, req)
}

func (n *Noto) decodeDomainConfig(ctx context.Context, domainConfig []byte) (*types.DomainConfig, error) {
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

func (n *Noto) validateDeploy(tx *pb.DeployTransactionSpecification) (*types.ConstructorParams, error) {
	var params types.ConstructorParams
	err := json.Unmarshal([]byte(tx.ConstructorParamsJson), &params)
	return &params, err
}

func (n *Noto) validateTransaction(ctx context.Context, tx *pb.TransactionSpecification) (*types.ParsedTransaction, types.DomainHandler, error) {
	var functionABI abi.Entry
	err := json.Unmarshal([]byte(tx.FunctionAbiJson), &functionABI)
	if err != nil {
		return nil, nil, err
	}

	abi := types.NotoABI.Functions()[functionABI.Name]
	handler := n.GetHandler(functionABI.Name)
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

	domainConfig, err := n.decodeDomainConfig(ctx, tx.ContractConfig)
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

func (n *Noto) recoverSignature(ctx context.Context, payload ethtypes.HexBytes0xPrefix, signature []byte) (*ethtypes.Address0xHex, error) {
	sig, err := secp256k1.DecodeCompactRSV(ctx, signature)
	if err != nil {
		return nil, err
	}
	return sig.RecoverDirect(payload, n.chainID)
}

func (n *Noto) parseCoinList(label string, states []*pb.EndorsableState) ([]*types.NotoCoin, []*pb.StateRef, *big.Int, error) {
	var err error
	coins := make([]*types.NotoCoin, len(states))
	refs := make([]*pb.StateRef, len(states))
	total := big.NewInt(0)
	for i, input := range states {
		if input.SchemaId != n.coinSchema.Id {
			return nil, nil, nil, fmt.Errorf("unknown schema ID: %s", input.SchemaId)
		}
		if coins[i], err = n.unmarshalCoin(input.StateDataJson); err != nil {
			return nil, nil, nil, fmt.Errorf("invalid %s[%d] (%s): %s", label, i, input.Id, err)
		}
		refs[i] = &pb.StateRef{
			SchemaId: input.SchemaId,
			Id:       input.Id,
		}
		total = total.Add(total, coins[i].Amount.BigInt())
	}
	return coins, refs, total, nil
}

func (n *Noto) gatherCoins(inputs, outputs []*pb.EndorsableState) (*gatheredCoins, error) {
	inCoins, inStates, inTotal, err := n.parseCoinList("input", inputs)
	if err != nil {
		return nil, err
	}
	outCoins, outStates, outTotal, err := n.parseCoinList("output", outputs)
	if err != nil {
		return nil, err
	}
	return &gatheredCoins{
		inCoins:   inCoins,
		inStates:  inStates,
		inTotal:   inTotal,
		outCoins:  outCoins,
		outStates: outStates,
		outTotal:  outTotal,
	}, nil
}

func (n *Noto) FindCoins(ctx context.Context, query string) ([]*types.NotoCoin, error) {
	states, err := n.findAvailableStates(ctx, query)
	if err != nil {
		return nil, err
	}

	coins := make([]*types.NotoCoin, len(states))
	for i, state := range states {
		if coins[i], err = n.unmarshalCoin(state.DataJson); err != nil {
			return nil, err
		}
	}
	return coins, err
}
