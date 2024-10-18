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

package integration_test

import (
	"context"
	_ "embed"
	"fmt"
	"os"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

//go:embed abis/ZetoFactory.json
var zetoFactoryJSON []byte // From "gradle copySolidity"

type zetoDomainContracts struct {
	factoryAddress       *tktypes.EthAddress
	factoryAbi           abi.ABI
	deployedContracts    map[string]*tktypes.EthAddress
	deployedContractAbis map[string]abi.ABI
	cloneableContracts   map[string]cloneableContract
}

type cloneableContract struct {
	circuitId string
	verifier  string
}

func newZetoDomainContracts() *zetoDomainContracts {
	factory := domain.LoadBuildLinked(zetoFactoryJSON, map[string]*tktypes.EthAddress{})

	return &zetoDomainContracts{
		factoryAbi: factory.ABI,
	}
}

func deployDomainContracts(ctx context.Context, rpc rpcbackend.Backend, deployer string, config *domainConfig) (*zetoDomainContracts, error) {
	if len(config.DomainContracts.Implementations) == 0 {
		return nil, fmt.Errorf("no implementations specified for factory contract")
	}

	// the cloneable contracts are the ones that can be cloned by the factory
	// these are the top level Zeto token contracts
	cloneableContracts := findCloneableContracts(config)

	// deploy the implementation contracts
	deployedContracts, deployedContractAbis, err := deployContracts(ctx, rpc, deployer, config.DomainContracts.Implementations)
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

func findCloneableContracts(config *domainConfig) map[string]cloneableContract {
	cloneableContracts := make(map[string]cloneableContract)
	for _, contract := range config.DomainContracts.Implementations {
		if contract.Cloneable {
			cloneableContracts[contract.Name] = cloneableContract{
				circuitId: contract.CircuitId,
				verifier:  contract.Verifier,
			}
		}
	}
	return cloneableContracts
}

func deployContracts(ctx context.Context, rpc rpcbackend.Backend, deployer string, contracts []domainContract) (map[string]*tktypes.EthAddress, map[string]abi.ABI, error) {
	deployedContracts := make(map[string]*tktypes.EthAddress)
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

func deployContract(ctx context.Context, rpc rpcbackend.Backend, deployer string, contract *domainContract, deployedContracts map[string]*tktypes.EthAddress) (*tktypes.EthAddress, abi.ABI, error) {
	if contract.AbiAndBytecode.Path == "" {
		return nil, nil, fmt.Errorf("no path or JSON specified for the abi and bytecode for contract %s", contract.Name)
	}
	// deploy the contract
	build, err := getContractSpec(contract, deployedContracts)
	if err != nil {
		return nil, nil, err
	}
	addr, err := deployBytecode(ctx, rpc, deployer, build)
	if err != nil {
		return nil, nil, err
	}
	return addr, build.ABI, nil
}

func getContractSpec(contract *domainContract, deployedContracts map[string]*tktypes.EthAddress) (*domain.SolidityBuild, error) {
	bytes, err := os.ReadFile(contract.AbiAndBytecode.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read abi+bytecode file %s. %s", contract.AbiAndBytecode.Path, err)
	}
	build := domain.LoadBuildLinked(bytes, deployedContracts)
	return build, nil
}

func deployBytecode(ctx context.Context, rpc rpcbackend.Backend, deployer string, build *domain.SolidityBuild) (*tktypes.EthAddress, error) {
	var addr string
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode", deployer, build.ABI, build.Bytecode.String(), tktypes.RawJSON(`{}`))
	if rpcerr != nil {
		return nil, rpcerr.Error()
	}
	return tktypes.MustEthAddress(addr), nil
}

func configureFactoryContract(ctx context.Context, tb testbed.Testbed, deployer string, domainContracts *zetoDomainContracts) error {
	abiFunc := domainContracts.factoryAbi.Functions()["registerImplementation"]

	// Send the transaction
	for contractName := range domainContracts.cloneableContracts {
		err := registerImpl(ctx, contractName, domainContracts, abiFunc, deployer, domainContracts.factoryAddress, tb)
		if err != nil {
			return err
		}
	}

	return nil
}

func registerImpl(ctx context.Context, name string, domainContracts *zetoDomainContracts, abiFunc *abi.Entry, deployer string, addr *tktypes.EthAddress, tb testbed.Testbed) error {
	log.L(ctx).Infof("Registering implementation %s", name)
	verifierName := domainContracts.cloneableContracts[name].verifier
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
	params := &setImplementationParams{
		Name: name,
		Implementation: implementationInfo{
			Implementation:   implAddr.String(),
			Verifier:         verifierAddr.String(),
			DepositVerifier:  depositVerifierAddr.String(),
			WithdrawVerifier: withdrawVerifierAddr.String(),
		},
	}
	_, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		Transaction: pldapi.Transaction{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     deployer,
			To:       addr,
			Data:     tktypes.JSONString(params),
			Function: abiFunc.String(),
		},
		ABI: abi.ABI{abiFunc},
	})
	return err
}
