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

package helpers

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"testing"

	"github.com/go-resty/resty/v2"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/testbed"
	zetotypes "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/solutils"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

//go:embed abis/ZetoFactory.json
var zetoFactoryJSON []byte

type ZetoDomainConfig struct {
	DomainContracts zetoDomainContracts `yaml:"contracts"`
}

type ZetoDomainContracts struct {
	FactoryAddress       *pldtypes.EthAddress
	factoryAbi           abi.ABI
	deployedContracts    map[string]*pldtypes.EthAddress
	DeployedContractAbis map[string]abi.ABI
	cloneableContracts   map[string]cloneableContract
}

type cloneableContract struct {
	circuits              *zetosignerapi.Circuits
	verifier              string
	batchVerifier         string
	depositVerifier       string
	withdrawVerifier      string
	batchWithdrawVerifier string
	lockVerifier          string
	batchLockVerifier     string
	burnVerifier          string
	batchBurnVerifier     string
}

type zetoDomainContracts struct {
	Factory         zetoDomainContract   `yaml:"factory"`
	Implementations []zetoDomainContract `yaml:"implementations"`
}

type zetoDomainContract struct {
	Name                  string                  `yaml:"name"`
	Verifier              string                  `yaml:"verifier"`
	BatchVerifier         string                  `yaml:"batchVerifier"`
	DepositVerifier       string                  `yaml:"depositVerifier"`
	WithdrawVerifier      string                  `yaml:"withdrawVerifier"`
	BatchWithdrawVerifier string                  `yaml:"batchWithdrawVerifier"`
	LockVerifier          string                  `yaml:"lockVerifier"`
	BatchLockVerifier     string                  `yaml:"batchLockVerifier"`
	Circuits              *zetosignerapi.Circuits `yaml:"circuits"`
	AbiAndBytecode        abiAndBytecode          `yaml:"abiAndBytecode"`
	Libraries             []string                `yaml:"libraries"`
	Cloneable             bool                    `yaml:"cloneable"`
}

type abiAndBytecode struct {
	Path string `yaml:"path"`
}

type setImplementationParams struct {
	Name           string             `json:"name"`
	Implementation implementationInfo `json:"implementation"`
}

type implementationInfo struct {
	Implementation string        `json:"implementation"`
	Verifiers      verifiersInfo `json:"verifiers"`
}

type verifiersInfo struct {
	Verifier              string `json:"verifier"`
	BatchVerifier         string `json:"batchVerifier"`
	DepositVerifier       string `json:"depositVerifier"`
	WithdrawVerifier      string `json:"withdrawVerifier"`
	BatchWithdrawVerifier string `json:"batchWithdrawVerifier"`
	LockVerifier          string `json:"lockVerifier"`
	BatchLockVerifier     string `json:"batchLockVerifier"`
	BurnVerifier          string `json:"burnVerifier"`
	BatchBurnVerifier     string `json:"batchBurnVerifier"`
}

func DeployZetoContracts(t *testing.T, hdWalletSeed *testbed.UTInitFunction, configFile string, controller string) *ZetoDomainContracts {
	ctx := context.Background()
	log.L(ctx).Infof("Deploy Zeto Contracts")

	tb := testbed.NewTestBed()
	url, _, done, err := tb.StartForTest("./testbed.config.yaml", map[string]*testbed.TestbedDomain{}, hdWalletSeed)
	require.NoError(t, err)
	defer done()
	rpc := rpcclient.WrapRestyClient(resty.New().SetBaseURL(url))

	var config ZetoDomainConfig
	testZetoConfigYaml, err := os.ReadFile(configFile)
	require.NoError(t, err)
	err = yaml.Unmarshal(testZetoConfigYaml, &config)
	require.NoError(t, err)

	deployedContracts, err := deployDomainContracts(ctx, rpc, controller, &config)
	require.NoError(t, err)

	err = configureFactoryContract(ctx, tb, controller, deployedContracts)
	require.NoError(t, err)

	return deployedContracts
}

func PrepareZetoConfig(t *testing.T, domainContracts *ZetoDomainContracts, zkpDir string) *zetotypes.DomainFactoryConfig {
	config := zetotypes.DomainFactoryConfig{
		SnarkProver: zetosignerapi.SnarkProverConfig{
			CircuitsDir:    zkpDir,
			ProvingKeysDir: zkpDir,
		},
	}

	var impls []*zetotypes.DomainContract
	for name, implContract := range domainContracts.cloneableContracts {
		implContract.circuits.Init()
		contract := zetotypes.DomainContract{
			Name:     name,
			Circuits: implContract.circuits,
		}
		impls = append(impls, &contract)
	}
	config.DomainContracts.Implementations = impls
	return &config
}

func newZetoDomainContracts() *ZetoDomainContracts {
	factory := solutils.MustLoadBuild(zetoFactoryJSON)

	return &ZetoDomainContracts{
		factoryAbi: factory.ABI,
	}
}

func deployDomainContracts(ctx context.Context, rpc rpcclient.Client, deployer string, config *ZetoDomainConfig) (*ZetoDomainContracts, error) {
	if len(config.DomainContracts.Implementations) == 0 {
		return nil, fmt.Errorf("no implementations specified for factory contract")
	}

	// the cloneable contracts are the ones that can be cloned by the factory
	// these are the top level Zeto token contracts
	cloneableContracts := findCloneableContracts(config)

	// deploy the implementation contracts
	deployedContracts, deployedContractAbis, err := deployImplementations(ctx, rpc, deployer, config.DomainContracts.Implementations)
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
	ctrs.FactoryAddress = factoryAddr
	ctrs.deployedContracts = deployedContracts
	ctrs.DeployedContractAbis = deployedContractAbis
	ctrs.cloneableContracts = cloneableContracts
	return ctrs, nil
}

func findCloneableContracts(config *ZetoDomainConfig) map[string]cloneableContract {
	cloneableContracts := make(map[string]cloneableContract)
	for _, contract := range config.DomainContracts.Implementations {
		if contract.Cloneable {
			cloneableContracts[contract.Name] = cloneableContract{
				circuits:              contract.Circuits,
				verifier:              contract.Verifier,
				batchVerifier:         contract.BatchVerifier,
				depositVerifier:       contract.DepositVerifier,
				withdrawVerifier:      contract.WithdrawVerifier,
				batchWithdrawVerifier: contract.BatchWithdrawVerifier,
				lockVerifier:          contract.LockVerifier,
				batchLockVerifier:     contract.BatchLockVerifier,
			}
		}
	}
	return cloneableContracts
}

func deployImplementations(ctx context.Context, rpc rpcclient.Client, deployer string, contracts []zetoDomainContract) (map[string]*pldtypes.EthAddress, map[string]abi.ABI, error) {
	deployedContracts := make(map[string]*pldtypes.EthAddress)
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

func deployContract(ctx context.Context, rpc rpcclient.Client, deployer string, contract *zetoDomainContract, deployedContracts map[string]*pldtypes.EthAddress) (*pldtypes.EthAddress, abi.ABI, error) {
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

func getContractSpec(contract *zetoDomainContract, deployedContracts map[string]*pldtypes.EthAddress) (*solutils.SolidityBuild, error) {
	bytes, err := os.ReadFile(contract.AbiAndBytecode.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read abi+bytecode file %s. %s", contract.AbiAndBytecode.Path, err)
	}
	build := solutils.MustLoadBuildResolveLinks(bytes, deployedContracts)
	return build, nil
}

func deployBytecode(ctx context.Context, rpc rpcclient.Client, deployer string, build *solutils.SolidityBuild) (*pldtypes.EthAddress, error) {
	var addr string
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode", deployer, build.ABI, build.Bytecode.String(), pldtypes.RawJSON(`{}`))
	if rpcerr != nil {
		return nil, rpcerr
	}
	return pldtypes.MustEthAddress(addr), nil
}

func configureFactoryContract(ctx context.Context, tb testbed.Testbed, deployer string, domainContracts *ZetoDomainContracts) error {
	abiFunc := domainContracts.factoryAbi.Functions()["registerImplementation"]

	// Send the transaction
	for contractName := range domainContracts.cloneableContracts {
		err := registerImpl(ctx, contractName, domainContracts, abiFunc, deployer, domainContracts.FactoryAddress, tb)
		if err != nil {
			return err
		}
	}

	return nil
}

func registerImpl(ctx context.Context, name string, domainContracts *ZetoDomainContracts, abiFunc *abi.Entry, deployer string, addr *pldtypes.EthAddress, tb testbed.Testbed) error {
	if name == "" {
		return fmt.Errorf("no name specified for implementation")
	}

	log.L(ctx).Infof("Registering implementation %s", name)
	verifierName := domainContracts.cloneableContracts[name].verifier
	batchVerifierName := domainContracts.cloneableContracts[name].batchVerifier
	depositVerifierName := domainContracts.cloneableContracts[name].depositVerifier
	withdrawVerifierName := domainContracts.cloneableContracts[name].withdrawVerifier
	batchWithdrawVerifierName := domainContracts.cloneableContracts[name].batchWithdrawVerifier
	lockVerifierName := domainContracts.cloneableContracts[name].lockVerifier
	batchLockVerifierName := domainContracts.cloneableContracts[name].batchLockVerifier
	burnVerifierName := domainContracts.cloneableContracts[name].burnVerifier
	batchBurnVerifierName := domainContracts.cloneableContracts[name].batchBurnVerifier

	params := &setImplementationParams{
		Name: name,
	}

	if verifierName == "" {
		return fmt.Errorf("verifierName not found among the deployed contracts. name: %s", name)
	}

	implAddr, ok := domainContracts.deployedContracts[name]
	if !ok {
		return fmt.Errorf("implementation contract %s not found among the deployed contracts", name)
	}
	params.Implementation.Implementation = implAddr.String()

	verifierAddr, ok := domainContracts.deployedContracts[verifierName]
	if !ok {
		return fmt.Errorf("verifier contract %s not found among the deployed contracts", verifierName)
	}
	params.Implementation.Verifiers.Verifier = verifierAddr.String()
	if params.Implementation.Verifiers.Verifier == "" {
		return nil
	}

	if batchVerifierName != "" {
		batchVerifierAddr, ok := domainContracts.deployedContracts[batchVerifierName]
		if !ok {
			return fmt.Errorf("batch verifier contract %s not found among the deployed contracts", batchVerifierName)
		}
		params.Implementation.Verifiers.BatchVerifier = batchVerifierAddr.String()
	}

	if depositVerifierName != "" {
		depositVerifierAddr, ok := domainContracts.deployedContracts[depositVerifierName]
		if !ok {
			return fmt.Errorf("deposit verifier contract not found among the deployed contracts")
		}
		params.Implementation.Verifiers.DepositVerifier = depositVerifierAddr.String()
	}

	if withdrawVerifierName != "" {
		withdrawVerifierAddr, ok := domainContracts.deployedContracts[withdrawVerifierName]
		if !ok {
			return fmt.Errorf("withdraw verifier contract not found among the deployed contracts")
		}
		params.Implementation.Verifiers.WithdrawVerifier = withdrawVerifierAddr.String()
	}

	if batchWithdrawVerifierName != "" {
		batchWithdrawVerifierAddr, ok := domainContracts.deployedContracts[batchWithdrawVerifierName]
		if !ok {
			return fmt.Errorf("batch withdraw verifier contract not found among the deployed contracts")
		}
		params.Implementation.Verifiers.BatchWithdrawVerifier = batchWithdrawVerifierAddr.String()
	}

	if lockVerifierName != "" {
		lockVerifierAddr, ok := domainContracts.deployedContracts[lockVerifierName]
		if !ok {
			return fmt.Errorf("lock verifier contract not found among the deployed contracts")
		}
		params.Implementation.Verifiers.LockVerifier = lockVerifierAddr.String()
	}

	if batchLockVerifierName != "" {
		batchLockVerifierAddr, ok := domainContracts.deployedContracts[batchLockVerifierName]
		if !ok {
			return fmt.Errorf("batch lock verifier contract not found among the deployed contracts")
		}
		params.Implementation.Verifiers.BatchLockVerifier = batchLockVerifierAddr.String()
	}

	if burnVerifierName != "" {
		burnVerifierAddr, ok := domainContracts.deployedContracts[burnVerifierName]
		if !ok {
			return fmt.Errorf("lock verifier contract not found among the deployed contracts")
		}
		params.Implementation.Verifiers.BurnVerifier = burnVerifierAddr.String()
	}

	if batchBurnVerifierName != "" {
		batchBurnVerifierAddr, ok := domainContracts.deployedContracts[batchBurnVerifierName]
		if !ok {
			return fmt.Errorf("batch lock verifier contract not found among the deployed contracts")
		}
		params.Implementation.Verifiers.BatchBurnVerifier = batchBurnVerifierAddr.String()
	}

	_, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     deployer,
			To:       addr,
			Data:     pldtypes.JSONString(params),
			Function: abiFunc.String(),
		},
		ABI: abi.ABI{abiFunc},
	})
	log.L(ctx).Infof("Registered implementation %s", name)
	return err
}
