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
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/kata/pkg/testbed"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/stretchr/testify/assert"
)

var (
	controllerEth     = "controller:eth"
	controllerBabyJub = "controller:babyjub"
	recipient1Eth     = "recipient1:eth"
	recipient1BabyJub = "recipient1:babyjub"
)

func toJSON(t *testing.T, v any) []byte {
	result, err := json.Marshal(v)
	assert.NoError(t, err)
	return result
}

func yamlConfig(t *testing.T, config *Config) (yn yaml.Node) {
	configYAML, err := yaml.Marshal(&config)
	assert.NoError(t, err)
	err = yaml.Unmarshal(configYAML, &yn)
	assert.NoError(t, err)
	return yn
}

func deployFactory(ctx context.Context, t *testing.T, factory SolidityBuild) string {
	tb := testbed.NewTestBed()
	url, done, err := tb.StartForTest("../../testbed.config.yaml", map[string]*testbed.TestbedDomain{})
	assert.NoError(t, err)
	defer done()
	rpc := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))

	log.L(ctx).Infof("Deploying Noto factory")
	factoryAddress, err := deployBytecode(ctx, rpc, factory)
	assert.NoError(t, err)
	log.L(ctx).Infof("Noto factory deployed to %s", factoryAddress)
	return factoryAddress
}

func newTestDomain(t *testing.T, domainName string, config *Config) (context.CancelFunc, *Zeto, rpcbackend.Backend) {
	var domain *Zeto
	tb := testbed.NewTestBed()
	plugin := plugintk.NewDomain(func(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {
		domain = New(callbacks)
		return domain
	})
	url, done, err := tb.StartForTest("../../testbed.config.yaml", map[string]*testbed.TestbedDomain{
		domainName: {
			Config: yamlConfig(t, config),
			Plugin: plugin,
		},
	})
	assert.NoError(t, err)
	rpc := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))
	return done, domain, rpc
}

func deployBytecode(ctx context.Context, rpc rpcbackend.Backend, build SolidityBuild) (string, error) {
	var addr string
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode",
		controllerEth, build.ABI, build.Bytecode.String(), `{}`)
	if rpcerr != nil {
		return "", rpcerr.Error()
	}
	return addr, nil
}

func TestZeto(t *testing.T) {
	ctx := context.Background()
	log.L(ctx).Infof("TestZeto")
	domainName := "noto_" + types.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)

	log.L(ctx).Infof("Deploying Zeto libraries")
	commonLibAddress := deployFactory(ctx, t, loadBuild(commonLibJSON))
	log.L(ctx).Infof("commonlib deployed to %s", commonLibAddress)

	verifierAddress := deployFactory(ctx, t, loadBuild(Groth16Verifier_Anon))
	log.L(ctx).Infof("verifier deployed to %s", verifierAddress)

	depositVerifierAddress := deployFactory(ctx, t, loadBuild(Groth16Verifier_CheckHashesValue))
	log.L(ctx).Infof("depositVerifier deployed to %s", depositVerifierAddress)

	withdrawVerifierAddress := deployFactory(ctx, t, loadBuild(Groth16Verifier_CheckInputsOutputsValue))
	log.L(ctx).Infof("withdrawVerifier deployed to %s", withdrawVerifierAddress)

	libraries := map[string]string{
		"Commonlib": commonLibAddress,
	}

	log.L(ctx).Infof("Deploying Zeto factory")
	factory := loadBuildLinked(zetoFactoryJSON, libraries)
	factoryAddress := deployFactory(ctx, t, factory)
	log.L(ctx).Infof("Zeto factory deployed to %s", factoryAddress)

	done, zeto, rpc := newTestDomain(t, domainName, &Config{
		FactoryAddress: factoryAddress,
	})
	defer done()

	log.L(ctx).Infof("Configuring Zeto domain")
	var boolResult bool
	domainConfig := Config{
		FactoryAddress: factoryAddress,
		Libraries:      libraries,
	}
	rpcerr := rpc.CallRPC(ctx, &boolResult, "testbed_configureInit",
		domainName, domainConfig)
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	assert.True(t, boolResult)

	log.L(ctx).Infof("Deploying an instance of Zeto")
	var zetoAddress ethtypes.Address0xHex
	rpcerr = rpc.CallRPC(ctx, &zetoAddress, "testbed_deploy",
		domainName, &ZetoConstructorParams{
			From:             controllerEth,
			Verifier:         verifierAddress,
			DepositVerifier:  depositVerifierAddress,
			WithdrawVerifier: withdrawVerifierAddress,
		})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	log.L(ctx).Infof("Zeto instance deployed to %s", zetoAddress)

	log.L(ctx).Infof("Mint 10 from controller to controller")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &types.PrivateContractInvoke{
		From:     controllerEth,
		To:       types.EthAddress(zetoAddress),
		Function: *zeto.Interface["mint"].ABI,
		Inputs: toJSON(t, &ZetoMintParams{
			To:           controllerEth,
			RecipientKey: controllerBabyJub,
			Amount:       ethtypes.NewHexInteger64(10),
		}),
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	assert.True(t, boolResult)

	coins, err := zeto.FindCoins(ctx, "{}")
	assert.NoError(t, err)
	assert.Len(t, coins, 1)
	assert.Equal(t, int64(10), coins[0].Amount.Int64())
	assert.Equal(t, controllerEth, coins[0].Owner)

	log.L(ctx).Infof("Mint 20 from controller to controller")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &types.PrivateContractInvoke{
		From:     controllerEth,
		To:       types.EthAddress(zetoAddress),
		Function: *zeto.Interface["mint"].ABI,
		Inputs: toJSON(t, &ZetoMintParams{
			To:           controllerEth,
			RecipientKey: controllerBabyJub,
			Amount:       ethtypes.NewHexInteger64(20),
		}),
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	assert.True(t, boolResult)

	coins, err = zeto.FindCoins(ctx, "{}")
	assert.NoError(t, err)
	assert.Len(t, coins, 2)
	assert.Equal(t, int64(10), coins[0].Amount.Int64())
	assert.Equal(t, controllerEth, coins[0].Owner)
	assert.Equal(t, int64(20), coins[1].Amount.Int64())
	assert.Equal(t, controllerEth, coins[1].Owner)

	log.L(ctx).Infof("Attempt mint from non-controller (should fail)")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &types.PrivateContractInvoke{
		From:     recipient1Eth,
		To:       types.EthAddress(zetoAddress),
		Function: *zeto.Interface["mint"].ABI,
		Inputs: toJSON(t, &ZetoMintParams{
			To:           recipient1Eth,
			RecipientKey: recipient1BabyJub,
			Amount:       ethtypes.NewHexInteger64(10),
		}),
	})
	assert.NotNil(t, rpcerr)
	assert.EqualError(t, rpcerr.Error(), "failed to send base ledger transaction: Execution reverted")
	assert.True(t, boolResult)

	log.L(ctx).Infof("Transfer 25 from controller to recipient1")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &types.PrivateContractInvoke{
		From:     controllerEth,
		To:       types.EthAddress(zetoAddress),
		Function: *zeto.Interface["transfer"].ABI,
		Inputs: toJSON(t, &ZetoTransferParams{
			To:           recipient1Eth,
			SenderKey:    controllerBabyJub,
			RecipientKey: recipient1BabyJub,
			Amount:       ethtypes.NewHexInteger64(25),
		}),
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
}
