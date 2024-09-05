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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/core/pkg/types"

	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
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

type Config struct {
	FactoryAddress string            `json:"factoryAddress"`
	Libraries      map[string]string `json:"libraries"`
}

type SolidityBuild struct {
	ABI      abi.ABI                   `json:"abi"`
	Bytecode ethtypes.HexBytes0xPrefix `json:"bytecode"`
}

type SolidityBuildWithLinks struct {
	ABI            abi.ABI                                       `json:"abi"`
	Bytecode       string                                        `json:"bytecode"`
	LinkReferences map[string]map[string][]SolidityLinkReference `json:"linkReferences"`
}

type SolidityLinkReference struct {
	Start  int `json:"start"`
	Length int `json:"length"`
}

type Zeto struct {
	Interface DomainInterface

	config     *Config
	callbacks  plugintk.DomainCallbacks
	chainID    int64
	domainID   string
	coinSchema *pb.StateSchema
}

type ZetoDomainConfig struct {
}

var ZetoDomainConfigABI = &abi.ParameterArray{}

type ZetoDeployParams struct {
	TransactionID    string                    `json:"transactionId"`
	Data             ethtypes.HexBytes0xPrefix `json:"data"`
	Verifier         string                    `json:"_verifier"`
	DepositVerifier  string                    `json:"_depositVerifier"`
	WithdrawVerifier string                    `json:"_withdrawVerifier"`
}

func loadBuildLinked(buildOutput []byte, libraries map[string]string) *SolidityBuild {
	var build SolidityBuildWithLinks
	err := json.Unmarshal(buildOutput, &build)
	if err != nil {
		panic(err)
	}
	bytecode, err := linkBytecode(build, libraries)
	if err != nil {
		panic(err)
	}
	return &SolidityBuild{
		ABI:      build.ABI,
		Bytecode: bytecode,
	}
}

// linkBytecode: performs linking by replacing placeholders with deployed addresses
// Based on a workaround from Hardhat team here:
// https://github.com/nomiclabs/hardhat/issues/611#issuecomment-638891597
func linkBytecode(artifact SolidityBuildWithLinks, libraries map[string]string) (ethtypes.HexBytes0xPrefix, error) {
	bytecode := artifact.Bytecode
	for _, fileReferences := range artifact.LinkReferences {
		for libName, fixups := range fileReferences {
			addr, found := libraries[libName]
			if !found {
				continue
			}
			for _, fixup := range fixups {
				start := 2 + fixup.Start*2
				end := start + fixup.Length*2
				bytecode = bytecode[0:start] + addr[2:] + bytecode[end:]
			}
		}
	}
	return hex.DecodeString(strings.TrimPrefix(bytecode, "0x"))
}

func New(callbacks plugintk.DomainCallbacks) *Zeto {
	zeto := &Zeto{
		callbacks: callbacks,
	}
	zeto.Interface = zeto.getInterface()
	return zeto
}

func (z *Zeto) ConfigureDomain(ctx context.Context, req *pb.ConfigureDomainRequest) (*pb.ConfigureDomainResponse, error) {
	var config Config
	err := json.Unmarshal([]byte(req.ConfigJson), &config)
	if err != nil {
		return nil, err
	}

	z.config = &config
	z.chainID = req.ChainId

	factory := loadBuildLinked(zetoFactoryJSON, config.Libraries)
	contract := loadBuildLinked(zetoJSON, config.Libraries)

	factoryJSON, err := json.Marshal(factory.ABI)
	if err != nil {
		return nil, err
	}
	zetoJSON, err := json.Marshal(contract.ABI)
	if err != nil {
		return nil, err
	}
	constructorJSON, err := json.Marshal(z.Interface["constructor"].ABI)
	if err != nil {
		return nil, err
	}
	schemaJSON, err := json.Marshal(ZetoCoinABI)
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
	tx, err := z.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return z.Interface[tx.FunctionABI.Name].Handler.Init(ctx, tx, req)
}

func (z *Zeto) AssembleTransaction(ctx context.Context, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	tx, err := z.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return z.Interface[tx.FunctionABI.Name].Handler.Assemble(ctx, tx, req)
}

func (z *Zeto) EndorseTransaction(ctx context.Context, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	tx, err := z.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return z.Interface[tx.FunctionABI.Name].Handler.Endorse(ctx, tx, req)
}

func (z *Zeto) PrepareTransaction(ctx context.Context, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	tx, err := z.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return z.Interface[tx.FunctionABI.Name].Handler.Prepare(ctx, tx, req)
}

func (z *Zeto) decodeDomainConfig(ctx context.Context, domainConfig []byte) (*ZetoDomainConfig, error) {
	configValues, err := ZetoDomainConfigABI.DecodeABIDataCtx(ctx, domainConfig, 0)
	if err != nil {
		return nil, err
	}
	configJSON, err := types.StandardABISerializer().SerializeJSON(configValues)
	if err != nil {
		return nil, err
	}
	var config ZetoDomainConfig
	err = json.Unmarshal(configJSON, &config)
	return &config, err
}

func (z *Zeto) validateDeploy(tx *pb.DeployTransactionSpecification) (*ZetoConstructorParams, error) {
	var params ZetoConstructorParams
	err := json.Unmarshal([]byte(tx.ConstructorParamsJson), &params)
	return &params, err
}

func (z *Zeto) validateTransaction(ctx context.Context, tx *pb.TransactionSpecification) (*ParsedTransaction, error) {
	var functionABI abi.Entry
	err := json.Unmarshal([]byte(tx.FunctionAbiJson), &functionABI)
	if err != nil {
		return nil, err
	}

	parser, found := z.Interface[functionABI.Name]
	if !found {
		return nil, fmt.Errorf("unknown function: %s", functionABI.Name)
	}
	params, err := parser.Handler.ValidateParams(tx.FunctionParamsJson)
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

	domainConfig, err := z.decodeDomainConfig(ctx, tx.ContractConfig)
	if err != nil {
		return nil, err
	}

	contractAddress, err := ethtypes.NewAddress(tx.ContractAddress)
	if err != nil {
		return nil, err
	}

	return &ParsedTransaction{
		Transaction:     tx,
		FunctionABI:     &functionABI,
		ContractAddress: contractAddress,
		DomainConfig:    domainConfig,
		Params:          params,
	}, nil
}
