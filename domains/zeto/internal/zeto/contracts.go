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

	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
)

func deployDomainContracts(ctx context.Context, rpc rpcbackend.Backend, deployer string, config *ZetoDomainConfig) error {
	if len(config.DomainContracts.Factory.Implementations) == 0 {
		return fmt.Errorf("no implementations specified for factory contract")
	}
	// libraries contracts are deployed first. they are specified as regular contracts in the config
	// but are referenced from the main contracts by the `libraries` field
	libraries := make(map[string]bool)
	for _, contract := range config.DomainContracts.Factory.Implementations {
		if len(contract.Libraries) > 0 {
			for _, library := range contract.Libraries {
				libraries[library] = true
			}
		}
	}

	var libraryContracts []*ZetoDomainContract
	for lib := range libraries {
		var contract *ZetoDomainContract
		for _, impl := range config.DomainContracts.Factory.Implementations {
			if impl.Name == lib {
				contract = &impl
				break
			}
		}
		if contract == nil {
			return fmt.Errorf("library contract %s referenced but not found", lib)
		}
		libraryContracts = append(libraryContracts, contract)
	}

	// deploy libraries
	for _, contract := range libraryContracts {
		err := deployContract(ctx, rpc, deployer, contract)
		if err != nil {
			return err
		}
	}

	return nil
}

func deployContract(ctx context.Context, rpc rpcbackend.Backend, deployer string, contract *ZetoDomainContract) error {
	if contract.AbiAndBytecode.Path == "" && (contract.AbiAndBytecode.Json.Bytecode == "" || contract.AbiAndBytecode.Json.Abi == nil) {
		return fmt.Errorf("no path or JSON specified for the abi and bytecode for contract %s", contract.Name)
	}
	// deploy the contract
	if contract.AbiAndBytecode.Json.Bytecode != "" && contract.AbiAndBytecode.Json.Abi != nil {
		abiBytecode := make(map[string]interface{})
		abiBytecode["abi"] = contract.AbiAndBytecode.Json.Abi
		abiBytecode["bytecode"] = contract.AbiAndBytecode.Json.Bytecode
		bytes, err := json.Marshal(abiBytecode)
		if err != nil {
			return fmt.Errorf("failed to marshal abi and bytecode content in the Domain configuration. %s", err)
		}
		var build SolidityBuild
		err = json.Unmarshal(bytes, &build)
		if err != nil {
			return fmt.Errorf("failed to parse abi and bytecode content in the Domain configuration. %s", err)
		}
		_, err = deployBytecode(ctx, rpc, deployer, build)
	}
	return nil
}

func deployBytecode(ctx context.Context, rpc rpcbackend.Backend, deployer string, build SolidityBuild) (string, error) {
	var addr string
	// TODO: replace with the actual API call against the engine
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode",
		deployer, build.ABI, build.Bytecode.String(), `{}`)
	if rpcerr != nil {
		return "", rpcerr.Error()
	}
	return addr, nil
}
