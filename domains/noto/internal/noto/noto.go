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
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"gopkg.in/yaml.v2"
)

//go:embed abis/NotoFactory.json
var notoFactoryJSON []byte // From "gradle copySolidity"

//go:embed abis/Noto.json
var notoJSON []byte // From "gradle copySolidity"

//go:embed abis/NotoSelfSubmitFactory.json
var notoSelfSubmitFactoryJSON []byte // From "gradle copySolidity"

//go:embed abis/NotoSelfSubmit.json
var notoSelfSubmitJSON []byte // From "gradle copySolidity"

type Config struct {
	FactoryAddress string `json:"factoryAddress" yaml:"factoryAddress"`
	Variant        string `json:"variant" yaml:"variant"`
}

type SolidityBuild struct {
	ABI      abi.ABI                   `json:"abi"`
	Bytecode ethtypes.HexBytes0xPrefix `json:"bytecode"`
}

type Noto struct {
	Interface DomainInterface

	config     *Config
	callbacks  plugintk.DomainCallbacks
	chainID    int64
	domainID   string
	coinSchema *pb.StateSchema
}

type NotoDomainConfig struct {
	NotaryLookup  string `json:"notaryLookup"`
	NotaryAddress string `json:"notaryAddress"`
}

var NotoDomainConfigABI = &abi.ParameterArray{
	{Name: "notaryLookup", Type: "string"},
	{Name: "notaryAddress", Type: "address"},
}

type NotoDeployParams struct {
	TransactionID string                    `json:"transactionId"`
	Notary        string                    `json:"notary"`
	Data          ethtypes.HexBytes0xPrefix `json:"data"`
}

type parsedTransaction struct {
	transaction     *pb.TransactionSpecification
	functionABI     *abi.Entry
	contractAddress *ethtypes.Address0xHex
	domainConfig    *NotoDomainConfig
	params          interface{}
}

type gatheredCoins struct {
	inCoins   []*NotoCoin
	inStates  []*pb.StateRef
	inTotal   *big.Int
	outCoins  []*NotoCoin
	outStates []*pb.StateRef
	outTotal  *big.Int
}

func loadBuild(buildOutput []byte) SolidityBuild {
	var build SolidityBuild
	err := json.Unmarshal(buildOutput, &build)
	if err != nil {
		panic(err)
	}
	return build
}

func New(callbacks plugintk.DomainCallbacks) *Noto {
	noto := &Noto{
		callbacks: callbacks,
	}
	noto.Interface = noto.getInterface()
	return noto
}

func (n *Noto) ConfigureDomain(ctx context.Context, req *pb.ConfigureDomainRequest) (*pb.ConfigureDomainResponse, error) {
	var config Config
	err := yaml.Unmarshal([]byte(req.ConfigYaml), &config)
	if err != nil {
		return nil, err
	}

	n.config = &config
	n.chainID = req.ChainId

	var factory SolidityBuild
	var contract SolidityBuild
	switch config.Variant {
	case "", "Noto":
		config.Variant = "Noto"
		factory = loadBuild(notoFactoryJSON)
		contract = loadBuild(notoJSON)
	case "NotoSelfSubmit":
		factory = loadBuild(notoSelfSubmitFactoryJSON)
		contract = loadBuild(notoSelfSubmitJSON)
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
	constructorJSON, err := json.Marshal(n.Interface["constructor"].ABI)
	if err != nil {
		return nil, err
	}
	schemaJSON, err := json.Marshal(NotoCoinABI)
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
	config := &NotoDomainConfig{
		NotaryLookup:  req.ResolvedVerifiers[0].Lookup,
		NotaryAddress: req.ResolvedVerifiers[0].Verifier,
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	data, err := NotoDomainConfigABI.EncodeABIDataJSONCtx(ctx, configJSON)
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
	tx, err := n.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return n.Interface[tx.functionABI.Name].handler.Init(ctx, tx, req)
}

func (n *Noto) AssembleTransaction(ctx context.Context, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	tx, err := n.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return n.Interface[tx.functionABI.Name].handler.Assemble(ctx, tx, req)
}

func (n *Noto) EndorseTransaction(ctx context.Context, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	tx, err := n.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return n.Interface[tx.functionABI.Name].handler.Endorse(ctx, tx, req)
}

func (n *Noto) PrepareTransaction(ctx context.Context, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	tx, err := n.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return n.Interface[tx.functionABI.Name].handler.Prepare(ctx, tx, req)
}

func (n *Noto) decodeDomainConfig(ctx context.Context, domainConfig []byte) (*NotoDomainConfig, error) {
	configValues, err := NotoDomainConfigABI.DecodeABIDataCtx(ctx, domainConfig, 0)
	if err != nil {
		return nil, err
	}
	configJSON, err := types.StandardABISerializer().SerializeJSON(configValues)
	if err != nil {
		return nil, err
	}
	var config NotoDomainConfig
	err = json.Unmarshal(configJSON, &config)
	return &config, err
}

func (n *Noto) validateDeploy(tx *pb.DeployTransactionSpecification) (*NotoConstructorParams, error) {
	var params NotoConstructorParams
	err := yaml.Unmarshal([]byte(tx.ConstructorParamsJson), &params)
	return &params, err
}

func (n *Noto) validateTransaction(ctx context.Context, tx *pb.TransactionSpecification) (*parsedTransaction, error) {
	var functionABI abi.Entry
	err := json.Unmarshal([]byte(tx.FunctionAbiJson), &functionABI)
	if err != nil {
		return nil, err
	}

	parser, found := n.Interface[functionABI.Name]
	if !found {
		return nil, fmt.Errorf("unknown function: %s", functionABI.Name)
	}
	params, err := parser.handler.ValidateParams(tx.FunctionParamsJson)
	if err != nil {
		return nil, err
	}

	signature, _, err := parser.ABI.SolidityDefCtx(ctx)
	if err != nil {
		return nil, err
	}
	if tx.FunctionSignature != signature {
		return nil, fmt.Errorf("unexpected signature for function '%s': expected=%s actual=%s", functionABI.Name, signature, tx.FunctionSignature)
	}

	domainConfig, err := n.decodeDomainConfig(ctx, tx.ContractConfig)
	if err != nil {
		return nil, err
	}

	contractAddress, err := ethtypes.NewAddress(tx.ContractAddress)
	if err != nil {
		return nil, err
	}

	return &parsedTransaction{
		transaction:     tx,
		functionABI:     &functionABI,
		contractAddress: contractAddress,
		domainConfig:    domainConfig,
		params:          params,
	}, nil
}

func (n *Noto) recoverSignature(ctx context.Context, payload ethtypes.HexBytes0xPrefix, signature []byte) (*ethtypes.Address0xHex, error) {
	sig, err := secp256k1.DecodeCompactRSV(ctx, signature)
	if err != nil {
		return nil, err
	}
	return sig.RecoverDirect(payload, n.chainID)
}

func (h *domainHandler) parseCoinList(label string, states []*pb.EndorsableState) ([]*NotoCoin, []*pb.StateRef, *big.Int, error) {
	var err error
	coins := make([]*NotoCoin, len(states))
	refs := make([]*pb.StateRef, len(states))
	total := big.NewInt(0)
	for i, input := range states {
		if input.SchemaId != h.noto.coinSchema.Id {
			return nil, nil, nil, fmt.Errorf("unknown schema ID: %s", input.SchemaId)
		}
		if coins[i], err = h.noto.makeCoin(input.StateDataJson); err != nil {
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

func (h *domainHandler) gatherCoins(inputs, outputs []*pb.EndorsableState) (*gatheredCoins, error) {
	inCoins, inStates, inTotal, err := h.parseCoinList("input", inputs)
	if err != nil {
		return nil, err
	}
	outCoins, outStates, outTotal, err := h.parseCoinList("output", outputs)
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
