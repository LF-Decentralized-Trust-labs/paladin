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
	"encoding/json"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	internalZeto "github.com/kaleido-io/paladin/domains/zeto/internal/zeto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zeto"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

var (
	controllerName    = "controller"
	recipient1Name    = "recipient1"
	deployedContracts *zetoDomainContracts
)

//go:embed config-for-deploy.yaml
var testZetoConfigYaml []byte

//go:embed config-local.yaml
var testZetoLocalConfigYaml []byte

func toJSON(t *testing.T, v any) []byte {
	result, err := json.Marshal(v)
	require.NoError(t, err)
	return result
}

func mapConfig(t *testing.T, config *types.DomainFactoryConfig) (m map[string]any) {
	configJSON, err := json.Marshal(&config)
	require.NoError(t, err)
	err = json.Unmarshal(configJSON, &m)
	require.NoError(t, err)
	return m
}

func prepareLocalConfig(t *testing.T, domainContracts *zetoDomainContracts) {
	// emulate preparing the local config based on Zeto contracts deploy,
	// for the Paladin runtime's zeto domain
	var localConfig internalZeto.LocalDomainConfig
	err := yaml.Unmarshal(testZetoLocalConfigYaml, &localConfig)
	assert.NoError(t, err)
	localConfig.DomainContracts.Factory.ContractAddress = domainContracts.factoryAddress.String()
	for _, implContract := range localConfig.DomainContracts.Implementations {
		implContract.ContractAddress = domainContracts.deployedContracts[implContract.Name].String()
		abiJSON, err := json.Marshal(domainContracts.deployedContractAbis[implContract.Name])
		require.NoError(t, err)
		implContract.Abi = tktypes.RawJSON(abiJSON).String()
	}
	configFile := path.Join(t.TempDir(), "config-local.yaml")
	configTestBytes, err := yaml.Marshal(localConfig)
	require.NoError(t, err)
	fmt.Printf("Local config written to file %s\n", configFile)
	err = os.WriteFile(configFile, configTestBytes, 0644)
	require.NoError(t, err)
	os.Setenv("LOCAL_CONFIG", configFile)
}

func newTestDomain(t *testing.T, domainName string, config *types.DomainFactoryConfig, domainContracts *zetoDomainContracts) (context.CancelFunc, zeto.Zeto, rpcbackend.Backend) {
	prepareLocalConfig(t, domainContracts)

	var domain zeto.Zeto
	var err error
	tb := testbed.NewTestBed()
	plugin := plugintk.NewDomain(func(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {
		domain, err = zeto.New(callbacks)
		require.NoError(t, err)
		return domain
	})
	url, done, err := tb.StartForTest("./testbed.config.yaml", map[string]*testbed.TestbedDomain{
		domainName: {
			Config: mapConfig(t, config),
			Plugin: plugin,
		},
	})
	require.NoError(t, err)
	rpc := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))
	return done, domain, rpc
}

func TestZeto_DeployZetoContracts(t *testing.T) {
	ctx := context.Background()
	log.L(ctx).Infof("Deploy Zeto Contracts")

	tb := testbed.NewTestBed()
	url, done, err := tb.StartForTest("./testbed.config.yaml", map[string]*testbed.TestbedDomain{})
	bi := tb.Components().BlockIndexer()
	ec := tb.Components().EthClientFactory().HTTPClient()
	assert.NoError(t, err)
	defer done()
	rpc := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))

	var config domainConfig
	err = yaml.Unmarshal(testZetoConfigYaml, &config)
	assert.NoError(t, err)

	deployedContracts, err = deployDomainContracts(ctx, rpc, controllerName, &config)
	assert.NoError(t, err)

	err = configureFactoryContract(ctx, ec, bi, controllerName, deployedContracts)
	assert.NoError(t, err)
}

func TestZeto_Anon(t *testing.T) {
	testZetoFungible(t, "Zeto_Anon")
}

func TestZeto_AnonEnc(t *testing.T) {
	testZetoFungible(t, "Zeto_AnonEnc")
}

func testZetoFungible(t *testing.T, tokenName string) {
	ctx := context.Background()
	domainName := "zeto_" + tktypes.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)
	done, zeto, rpc := newTestDomain(t, domainName, &types.DomainFactoryConfig{
		FactoryAddress: deployedContracts.factoryAddress.String(),
		TokenName:      tokenName,
		CircuitId:      deployedContracts.cloneableContracts[tokenName].circuitId,
	}, deployedContracts)
	defer done()

	log.L(ctx).Infof("Deploying an instance of the %s token", tokenName)
	var zetoAddress ethtypes.Address0xHex
	rpcerr := rpc.CallRPC(ctx, &zetoAddress, "testbed_deploy",
		domainName, &types.ConstructorParams{
			From: controllerName,
		})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}
	log.L(ctx).Infof("Zeto instance deployed to %s", zetoAddress)

	log.L(ctx).Infof("Mint 10 from controller to controller")
	var boolResult bool
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     controllerName,
		To:       tktypes.EthAddress(zetoAddress),
		Function: *types.ZetoABI.Functions()["mint"],
		Inputs: toJSON(t, &types.MintParams{
			To:     controllerName,
			Amount: ethtypes.NewHexInteger64(10),
		}),
	})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}
	assert.True(t, boolResult)

	coins, err := zeto.FindCoins(ctx, "{}")
	require.NoError(t, err)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(10), coins[0].Amount.Int64())
	assert.Equal(t, controllerName, coins[0].Owner)

	log.L(ctx).Infof("Mint 20 from controller to controller")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     controllerName,
		To:       tktypes.EthAddress(zetoAddress),
		Function: *types.ZetoABI.Functions()["mint"],
		Inputs: toJSON(t, &types.MintParams{
			To:     controllerName,
			Amount: ethtypes.NewHexInteger64(20),
		}),
	})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}
	assert.True(t, boolResult)

	coins, err = zeto.FindCoins(ctx, "{}")
	require.NoError(t, err)
	require.Len(t, coins, 2)
	assert.Equal(t, int64(10), coins[0].Amount.Int64())
	assert.Equal(t, controllerName, coins[0].Owner)
	assert.Equal(t, int64(20), coins[1].Amount.Int64())
	assert.Equal(t, controllerName, coins[1].Owner)

	log.L(ctx).Infof("Attempt mint from non-controller (should fail)")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     recipient1Name,
		To:       tktypes.EthAddress(zetoAddress),
		Function: *types.ZetoABI.Functions()["mint"],
		Inputs: toJSON(t, &types.MintParams{
			To:     recipient1Name,
			Amount: ethtypes.NewHexInteger64(10),
		}),
	})
	require.NotNil(t, rpcerr)
	assert.EqualError(t, rpcerr.Error(), "failed to send base ledger transaction: Execution reverted")
	assert.True(t, boolResult)

	log.L(ctx).Infof("Transfer 25 from controller to recipient1")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     controllerName,
		To:       tktypes.EthAddress(zetoAddress),
		Function: *types.ZetoABI.Functions()["transfer"],
		Inputs: toJSON(t, &types.TransferParams{
			To:     recipient1Name,
			Amount: ethtypes.NewHexInteger64(25),
		}),
	})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}
}
