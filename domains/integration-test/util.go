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

package integrationtest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zeto"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/testbed"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/noto/pkg/noto"
	nototypes "github.com/LF-Decentralized-Trust-labs/paladin/domains/noto/pkg/types"
	zetotypes "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/solutils"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	recipient1Name = "recipient1@node1"
	recipient2Name = "recipient2@node1"
	recipient3Name = "recipient3@node1"
)

type zetoDomainConfig struct {
	DomainContracts zetoDomainContracts `yaml:"contracts"`
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
}

func mapConfig(t *testing.T, config any) (m map[string]any) {
	configJSON, err := json.Marshal(&config)
	assert.NoError(t, err)
	err = json.Unmarshal(configJSON, &m)
	assert.NoError(t, err)
	return m
}

func newTestbed(t *testing.T, hdWalletSeed *testbed.UTInitFunction, domains map[string]*testbed.TestbedDomain) (context.CancelFunc, *pldconf.PaladinConfig, testbed.Testbed, rpcclient.Client) {
	tb := testbed.NewTestBed()
	url, conf, done, err := tb.StartForTest("./testbed.config.yaml", domains, hdWalletSeed)
	assert.NoError(t, err)
	rc := resty.New().SetBaseURL(url)
	rpc := rpcclient.WrapRestyClient(rc)
	return done, conf, tb, rpc
}

func deployContracts(ctx context.Context, t *testing.T, hdWalletSeed *testbed.UTInitFunction, deployer string, contracts map[string][]byte) map[string]string {
	tb := testbed.NewTestBed()
	url, _, done, err := tb.StartForTest("./testbed.config.yaml", map[string]*testbed.TestbedDomain{}, hdWalletSeed)
	assert.NoError(t, err)
	defer done()
	rpc := rpcclient.WrapRestyClient(resty.New().SetBaseURL(url))

	deployed := make(map[string]string, len(contracts))
	for name, contract := range contracts {
		build := solutils.MustLoadBuild(contract)
		var addr string
		rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode",
			deployer, build.ABI, build.Bytecode.String(), pldtypes.RawJSON(`{}`))
		if rpcerr != nil {
			assert.NoError(t, rpcerr)
		}
		deployed[name] = addr
	}
	return deployed
}

func newNotoDomain(t *testing.T, config *nototypes.DomainConfig) (chan noto.Noto, *testbed.TestbedDomain) {
	waitForDomain := make(chan noto.Noto, 1)
	tbd := &testbed.TestbedDomain{
		Config: mapConfig(t, config),
		Plugin: plugintk.NewDomain(func(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {
			domain := noto.New(callbacks)
			waitForDomain <- domain
			return domain
		}),
		RegistryAddress: pldtypes.MustEthAddress(config.FactoryAddress),
	}
	return waitForDomain, tbd
}

func newZetoDomain(t *testing.T, config *zetotypes.DomainFactoryConfig, factoryAddress *pldtypes.EthAddress) (chan zeto.Zeto, *testbed.TestbedDomain) {
	waitForDomain := make(chan zeto.Zeto, 1)
	tbd := &testbed.TestbedDomain{
		Config: mapConfig(t, config),
		Plugin: plugintk.NewDomain(func(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {
			domain := zeto.New(callbacks)
			waitForDomain <- domain
			return domain
		}),
		RegistryAddress: factoryAddress,
		AllowSigning:    true,
	}
	return waitForDomain, tbd
}

func findAvailableCoins[T any](t *testing.T, ctx context.Context, rpc rpcclient.Client, domainName, coinSchemaID, methodName string, address *pldtypes.EthAddress, jq *query.QueryJSON, readiness ...func(coins []*T) bool) []*T {
	if jq == nil {
		jq = query.NewQueryBuilder().Limit(100).Query()
	}
	var states []*T
notReady:
	for {
		rpcerr := rpc.CallRPC(ctx, &states, methodName,
			domainName,
			address,
			coinSchemaID,
			jq,
			"available")
		if rpcerr != nil {
			require.NoError(t, rpcerr)
		}
		for _, fn := range readiness {
			if t.Failed() {
				panic("test failed")
			}
			if !fn(states) {
				time.Sleep(100 * time.Millisecond)
				continue notReady
			}
		}
		break
	}
	return states
}
