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
	"os"
	"sort"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

//go:embed abis/ZetoFactory.json
var zetoFactoryJSON []byte // From "gradle copySolidity"

type zetoDomainContracts struct {
	factoryAddress       *ethtypes.Address0xHex
	factoryAbi           abi.ABI
	deployedContracts    map[string]*ethtypes.Address0xHex
	deployedContractAbis map[string]abi.ABI
	cloneableContracts   []string
}

func newZetoDomainContracts() *zetoDomainContracts {
	factory := domain.LoadBuildLinked(zetoFactoryJSON, map[string]string{})

	return &zetoDomainContracts{
		factoryAbi: factory.ABI,
	}
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
	deployedContracts, deployedContractAbis, err := deployContracts(ctx, rpc, deployer, sortedContractList)
	if err != nil {
		return nil, err
	}

	// deploy the factory contract
	factoryAddr, _, err := deployContract(ctx, rpc, deployer, &config.DomainContracts.Factory, deployedContracts)
	if err != nil {
		return nil, err
	}
	log.L(ctx).Infof("Deployed factory contract to %s", factoryAddr.String())

	ctrs := newZetoDomainContracts()
	ctrs.factoryAddress = factoryAddr
	ctrs.deployedContracts = deployedContracts
	ctrs.deployedContractAbis = deployedContractAbis
	ctrs.cloneableContracts = cloneableContracts
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

func deployContracts(ctx context.Context, rpc rpcbackend.Backend, deployer string, contracts []ZetoDomainContract) (map[string]*ethtypes.Address0xHex, map[string]abi.ABI, error) {
	deployedContracts := make(map[string]*ethtypes.Address0xHex)
	deployedContractAbis := make(map[string]abi.ABI)
	for _, contract := range contracts {
		addr, abi, err := deployContract(ctx, rpc, deployer, &contract, deployedContracts)
		if err != nil {
			return nil, nil, err
		}
		log.L(ctx).Infof("Deployed contract %s to %s", contract.Name, addr.String())
		deployedContracts[contract.Name] = addr
		deployedContractAbis[contract.Name] = abi
	}

	return deployedContracts, deployedContractAbis, nil
}

func deployContract(ctx context.Context, rpc rpcbackend.Backend, deployer string, contract *ZetoDomainContract, deployedContracts map[string]*ethtypes.Address0xHex) (*ethtypes.Address0xHex, abi.ABI, error) {
	if contract.AbiAndBytecode.Path == "" && (contract.AbiAndBytecode.Json.Bytecode == "" || contract.AbiAndBytecode.Json.Abi == nil) {
		return nil, nil, fmt.Errorf("no path or JSON specified for the abi and bytecode for contract %s", contract.Name)
	}
	// deploy the contract
	build, err := getContractSpec(contract)
	if err != nil {
		return nil, nil, err
	}
	addr, err := deployBytecode(ctx, rpc, deployer, build)
	if err != nil {
		return nil, nil, err
	}
	return addr, build.ABI, nil
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

func configureFactoryContract(ctx context.Context, ec ethclient.EthClient, bi blockindexer.BlockIndexer, deployer string, domainContracts *zetoDomainContracts) error {
	abiFunc, err := ec.ABIFunction(ctx, domainContracts.factoryAbi.Functions()["registerImplementation"])
	if err != nil {
		return err
	}

	// Send the transaction
	addr := ethtypes.Address0xHex(*domainContracts.factoryAddress)
	for _, contractName := range domainContracts.cloneableContracts {
		err = registerImpl(ctx, contractName, domainContracts, abiFunc, deployer, addr, bi)
		if err != nil {
			return err
		}
	}

	return nil
}

func registerImpl(ctx context.Context, name string, domainContracts *zetoDomainContracts, abiFunc ethclient.ABIFunctionClient, deployer string, addr ethtypes.Address0xHex, bi blockindexer.BlockIndexer) error {
	log.L(ctx).Infof("Registering implementation %s", name)
	// work out the verifier contract name, by taking the suffix from the implementation name
	// such as Zeto_Anon -> Groth16Verifier_Anon
	// TODO: think about more robust ways than relying on the naming convention
	verifierName := "Groth16Verifier_" + name[len("Zeto_"):]
	implAddr, ok := domainContracts.deployedContracts[name]
	if !ok {
		return fmt.Errorf("implementation contract %s not found among the deployed contracts", name)
	}
	verifierAddr, ok := domainContracts.deployedContracts[verifierName]
	if !ok {
		return fmt.Errorf("verifier contract %s not found among the deployed contracts", verifierName)
	}
	depositVerifierAddr, ok := domainContracts.deployedContracts["Groth16Verifier_CheckHashesValue"]
	if !ok {
		return fmt.Errorf("deposit verifier contract not found among the deployed contracts")
	}
	withdrawVerifierAddr, ok := domainContracts.deployedContracts["Groth16Verifier_CheckInputsOutputsValue"]
	if !ok {
		return fmt.Errorf("withdraw verifier contract not found among the deployed contracts")
	}
	params := &ZetoSetImplementationParams{
		Name: name,
		Implementation: ZetoImplementationInfo{
			Implementation:   implAddr.String(),
			Verifier:         verifierAddr.String(),
			DepositVerifier:  depositVerifierAddr.String(),
			WithdrawVerifier: withdrawVerifierAddr.String(),
		},
	}
	txHash, err := abiFunc.R(ctx).
		Signer(deployer).
		To(&addr).
		Input(params).
		SignAndSend()
	if err != nil {
		return err
	}
	_, err = bi.WaitForTransaction(ctx, *txHash)
	if err != nil {
		return err
	}
	return nil
}
