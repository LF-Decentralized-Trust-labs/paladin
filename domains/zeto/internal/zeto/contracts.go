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

	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
)

// func deployDomainContracts(ctx context.Context, rpc rpcbackend.Backend, deployer string, config *ZetoDomainConfig) (map[string]*ethtypes.Address0xHex, error) {
// 	if len(config.DomainContracts.Factory.Implementations) == 0 {
// 		return nil, fmt.Errorf("no implementations specified for factory contract")
// 	}
// 	// libraries contracts are deployed first. they are specified as regular contracts in the config
// 	// but are referenced from the main contracts by the `libraries` field
// 	libraryContracts, err := findLibraryContracts(config)
// 	if err != nil {
// 		return nil, err
// 	}

// 	deployedContracts := make(map[string]*ethtypes.Address0xHex)

// 	// deploy libraries
// 	for _, contract := range libraryContracts {
// 		addr, err := deployContract(ctx, rpc, deployer, contract)
// 		if err != nil {
// 			return nil, err
// 		}
// 		deployedContracts[contract.Name] = addr
// 	}

// 	// deploy implementation (non-library) contracts
// 	for _, contract := range config.DomainContracts.Factory.Implementations {
// 		if libraryContracts[contract.Name] != nil {
// 			// already deployed as a library
// 			continue
// 		}
// 		addr, err := deployContract(ctx, rpc, deployer, &contract)
// 	}
// 	return nil
// }

// when contracts include a `libraries` section, the libraries must be deployed first
// we build a sorted list of contracts, with the dependencies first, and the depending
// contracts later
func sortContracts(config *ZetoDomainConfig) (map[string]*ZetoDomainContract, error) {
	var contracts []*ZetoDomainContract
	for _, contract := range config.DomainContracts.Factory.Implementations {
		contracts = append(contracts, &contract)
	}

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
}

// 	libraryContracts := make(map[string]*ZetoDomainContract)
// 	for lib := range libraries {
// 		var contract *ZetoDomainContract
// 		for _, impl := range config.DomainContracts.Factory.Implementations {
// 			if impl.Name == lib {
// 				contract = &impl
// 				break
// 			}
// 		}
// 		if contract == nil {
// 			return nil, fmt.Errorf("library contract %s referenced but not found", lib)
// 		}
// 		libraryContracts[contract.Name] = contract
// 	}
// 	return libraryContracts, nil
// }

func deployContract(ctx context.Context, rpc rpcbackend.Backend, deployer string, contract *ZetoDomainContract) (*ethtypes.Address0xHex, error) {
	if contract.AbiAndBytecode.Path == "" && (contract.AbiAndBytecode.Json.Bytecode == "" || contract.AbiAndBytecode.Json.Abi == nil) {
		return nil, fmt.Errorf("no path or JSON specified for the abi and bytecode for contract %s", contract.Name)
	}
	// deploy the contract
	if contract.AbiAndBytecode.Json.Bytecode != "" && contract.AbiAndBytecode.Json.Abi != nil {
		abiBytecode := make(map[string]interface{})
		abiBytecode["abi"] = contract.AbiAndBytecode.Json.Abi
		abiBytecode["bytecode"] = contract.AbiAndBytecode.Json.Bytecode
		bytes, err := json.Marshal(abiBytecode)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal abi and bytecode content in the Domain configuration. %s", err)
		}
		var build SolidityBuild
		err = json.Unmarshal(bytes, &build)
		if err != nil {
			return nil, fmt.Errorf("failed to parse abi and bytecode content in the Domain configuration. %s", err)
		}
		return deployBytecode(ctx, rpc, deployer, build)
	} else {
		// load the abi and bytecode from the file
		bytes, err := os.ReadFile(contract.AbiAndBytecode.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to read abi+bytecode file %s. %s", contract.AbiAndBytecode.Path, err)
		}
		var build SolidityBuild
		err = json.Unmarshal(bytes, &build)
		if err != nil {
			return nil, fmt.Errorf("failed to parse abi and bytecode content in the Domain configuration. %s", err)
		}
		return deployBytecode(ctx, rpc, deployer, build)
	}
}

func deployBytecode(ctx context.Context, rpc rpcbackend.Backend, deployer string, build SolidityBuild) (*ethtypes.Address0xHex, error) {
	var addr string
	// TODO: replace with the actual API call against the engine
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode",
		deployer, build.ABI, build.Bytecode.String(), `{}`)
	if rpcerr != nil {
		return nil, rpcerr.Error()
	}
	return ethtypes.MustNewAddress(addr), nil
}
