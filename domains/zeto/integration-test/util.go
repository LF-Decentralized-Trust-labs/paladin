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
	"os"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	zetotypes "github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

type domainConfig struct {
	DomainContracts domainContracts `yaml:"contracts"`
}

type domainContracts struct {
	Factory         domainContract   `yaml:"factory"`
	Implementations []domainContract `yaml:"implementations"`
}

type domainContract struct {
	Name                  string         `yaml:"name"`
	Verifier              string         `yaml:"verifier"`
	BatchVerifier         string         `yaml:"batchVerifier"`
	DepositVerifier       string         `yaml:"depositVerifier"`
	WithdrawVerifier      string         `yaml:"withdrawVerifier"`
	BatchWithdrawVerifier string         `yaml:"batchWithdrawVerifier"`
	LockVerifier          string         `yaml:"lockVerifier"`
	BatchLockVerifier     string         `yaml:"batchLockVerifier"`
	CircuitId             string         `yaml:"circuitId"`
	AbiAndBytecode        abiAndBytecode `yaml:"abiAndBytecode"`
	Libraries             []string       `yaml:"libraries"`
	Cloneable             bool           `yaml:"cloneable"`
}

type abiAndBytecode struct {
	Path string `yaml:"path"`
}

type setImplementationParams struct {
	Name           string             `json:"name"`
	Implementation implementationInfo `json:"implementation"`
}

type implementationInfo struct {
	Implementation        string `json:"implementation"`
	Verifier              string `json:"verifier"`
	BatchVerifier         string `json:"batchVerifier"`
	DepositVerifier       string `json:"depositVerifier"`
	WithdrawVerifier      string `json:"withdrawVerifier"`
	BatchWithdrawVerifier string `json:"batchWithdrawVerifier"`
	LockVerifier          string `json:"lockVerifier"`
	BatchLockVerifier     string `json:"batchLockVerifier"`
}

func DeployZetoContracts(t *testing.T, hdWalletSeed *testbed.UTInitFunction, configFile string, controller string) *ZetoDomainContracts {
	ctx := context.Background()
	log.L(ctx).Infof("Deploy Zeto Contracts")

	tb := testbed.NewTestBed()
	url, _, done, err := tb.StartForTest("./testbed.config.yaml", map[string]*testbed.TestbedDomain{}, hdWalletSeed)
	assert.NoError(t, err)
	defer done()
	rpc := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))

	var config domainConfig
	testZetoConfigYaml, err := os.ReadFile(configFile)
	assert.NoError(t, err)
	err = yaml.Unmarshal(testZetoConfigYaml, &config)
	assert.NoError(t, err)

	deployedContracts, err := deployDomainContracts(ctx, rpc, controller, &config)
	assert.NoError(t, err)

	err = configureFactoryContract(ctx, tb, controller, deployedContracts)
	assert.NoError(t, err)

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
		contract := zetotypes.DomainContract{
			Name:      name,
			CircuitId: implContract.circuitId,
		}
		impls = append(impls, &contract)
	}
	config.DomainContracts.Implementations = impls
	return &config
}
