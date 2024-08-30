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
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

type zetoDomainContracts struct {
	factoryAddress     *ethtypes.Address0xHex
	factoryAbi         abi.ABI
	deployedContracts  map[string]*ethtypes.Address0xHex
	cloneableContracts []string
}

func deployDomainContracts(ctx context.Context, rpc rpcbackend.Backend, deployer string, config *ZetoDomainConfig) (*zetoDomainContracts, error) {
	if len(config.DomainContracts.Implementations) == 0 {
		return nil, fmt.Errorf("no implementations specified for factory contract")
	}

	// the cloneable contracts are the ones that can be cloned by the factory
	// these are the top level Zeto token contracts
	cloneableContracts := findCloneableContracts(config)

	// sort contracts so that the dependencies are deployed first
	sortedContractList, err := sortContracts(config)
	if err != nil {
		return nil, err
	}

	// deploy the implementation contracts
	deployedContracts, err := deployContracts(ctx, rpc, deployer, sortedContractList)
	if err != nil {
		return nil, err
	}

	// deploy the factory contract
	factoryAddr, err := deployContract(ctx, rpc, deployer, &config.DomainContracts.Factory, deployedContracts)
	if err != nil {
		return nil, err
	}

	// configure the factory contract with the implementation contracts
	factorySpec, err := getContractSpec(&config.DomainContracts.Factory)
	if err != nil {
		return nil, err
	}

	ctrs := &zetoDomainContracts{
		factoryAddress:     factoryAddr,
		factoryAbi:         factorySpec.ABI,
		deployedContracts:  deployedContracts,
		cloneableContracts: cloneableContracts,
	}
	return ctrs, nil
}

func findCloneableContracts(config *ZetoDomainConfig) []string {
	var cloneableContracts []string
	for _, contract := range config.DomainContracts.Implementations {
		if contract.Cloneable {
			cloneableContracts = append(cloneableContracts, contract.Name)
		}
	}
	return cloneableContracts
}

// when contracts include a `libraries` section, the libraries must be deployed first
// we build a sorted list of contracts, with the dependencies first, and the depending
// contracts later
func sortContracts(config *ZetoDomainConfig) ([]ZetoDomainContract, error) {
	var contracts []ZetoDomainContract
	contracts = append(contracts, config.DomainContracts.Implementations...)

	sort.Slice(contracts, func(i, j int) bool {
		if len(contracts[i].Libraries) == 0 && len(contracts[j].Libraries) == 0 {
			// order doesn't matter
			return false
		}
		if len(contracts[i].Libraries) > 0 && len(contracts[j].Libraries) > 0 {
			// the order is determined by the dependencies
			for _, lib := range contracts[i].Libraries {
				if lib == contracts[j].Name {
					// i depends on j
					return false
				}
			}
			for _, lib := range contracts[j].Libraries {
				if lib == contracts[i].Name {
					// j depends on i
					return true
				}
			}
			// no dependency relationship
			return false
		}
		return len(contracts[i].Libraries) < len(contracts[j].Libraries)
	})

	return contracts, nil
}

func deployContracts(ctx context.Context, rpc rpcbackend.Backend, deployer string, contracts []ZetoDomainContract) (map[string]*ethtypes.Address0xHex, error) {
	deployedContracts := make(map[string]*ethtypes.Address0xHex)

	for _, contract := range contracts {
		addr, err := deployContract(ctx, rpc, deployer, &contract, deployedContracts)
		if err != nil {
			return nil, err
		}
		log.L(ctx).Infof("Deployed contract %s to %s", contract.Name, addr.String())
		deployedContracts[contract.Name] = addr
	}

	return deployedContracts, nil
}

func deployContract(ctx context.Context, rpc rpcbackend.Backend, deployer string, contract *ZetoDomainContract, deployedContracts map[string]*ethtypes.Address0xHex) (*ethtypes.Address0xHex, error) {
	if contract.AbiAndBytecode.Path == "" && (contract.AbiAndBytecode.Json.Bytecode == "" || contract.AbiAndBytecode.Json.Abi == nil) {
		return nil, fmt.Errorf("no path or JSON specified for the abi and bytecode for contract %s", contract.Name)
	}
	// deploy the contract
	build, err := getContractSpec(contract)
	if err != nil {
		return nil, err
	}
	return deployBytecode(ctx, rpc, deployer, build)
}

func getContractSpec(contract *ZetoDomainContract) (*SolidityBuild, error) {
	var build SolidityBuild
	if contract.AbiAndBytecode.Json.Bytecode != "" && contract.AbiAndBytecode.Json.Abi != nil {
		abiBytecode := make(map[string]interface{})
		abiBytecode["abi"] = contract.AbiAndBytecode.Json.Abi
		abiBytecode["bytecode"] = contract.AbiAndBytecode.Json.Bytecode
		bytes, err := json.Marshal(abiBytecode)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal abi and bytecode content in the Domain configuration. %s", err)
		}
		err = json.Unmarshal(bytes, &build)
		if err != nil {
			return nil, fmt.Errorf("failed to parse abi and bytecode content in the Domain configuration. %s", err)
		}
	} else {
		bytes, err := os.ReadFile(contract.AbiAndBytecode.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to read abi+bytecode file %s. %s", contract.AbiAndBytecode.Path, err)
		}
		err = json.Unmarshal(bytes, &build)
		if err != nil {
			return nil, fmt.Errorf("failed to parse abi and bytecode content in the Domain configuration. %s", err)
		}
	}
	return &build, nil
}

func deployBytecode(ctx context.Context, rpc rpcbackend.Backend, deployer string, build *SolidityBuild) (*ethtypes.Address0xHex, error) {
	var addr string
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode", deployer, build.ABI, build.Bytecode.String(), `{}`)
	if rpcerr != nil {
		return nil, rpcerr.Error()
	}
	return ethtypes.MustNewAddress(addr), nil
}

func configureFactoryContract(ctx context.Context, rpc rpcbackend.Backend, deployer string, factoryAddr *ethtypes.Address0xHex, factoryAbi abi.ABI, deployedContracts map[string]*ethtypes.Address0xHex) error {
	var boolResult bool

	params := &ZetoSetImplementationParams{
		Name: "Zeto_Anon",
		Implementation: ZetoImplementationInfo{
			Implementation:   deployedContracts["Zeto_Anon"].String(),
			Verifier:         deployedContracts["Groth16Verifier_Anon"].String(),
			DepositVerifier:  deployedContracts["Groth16Verifier_CheckHashesValue"].String(),
			WithdrawVerifier: deployedContracts["Groth16Verifier_CheckInputsOutputsValue"].String(),
		},
	}

	jsonBytes, err := json.Marshal(params)
	if err != nil {
		return err
	}

	rpcerr := rpc.CallRPC(ctx, &boolResult, "testbed_invokePublic", deployer, factoryAddr.String(), factoryAbi, "registerImplementation", jsonBytes)
	if rpcerr != nil {
		return rpcerr.Error()
	}

	return nil
}
