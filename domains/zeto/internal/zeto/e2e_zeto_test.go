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

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/domains/common/pkg/domain"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/kata/pkg/testbed"
	kata "github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	controllerName = "controller"
	recipient1Name = "recipient1"
)

func toJSON(t *testing.T, v any) []byte {
	result, err := json.Marshal(v)
	require.NoError(t, err)
	return result
}

func mapConfig(t *testing.T, config *types.Config) (m map[string]any) {
	configJSON, err := json.Marshal(&config)
	require.NoError(t, err)
	err = json.Unmarshal(configJSON, &m)
	require.NoError(t, err)
	return m
}

func deployContracts(ctx context.Context, t *testing.T, contracts []map[string][]byte) map[string]string {
	tb := testbed.NewTestBed()
	url, done, err := tb.StartForTest("../../testbed.config.yaml", map[string]*testbed.TestbedDomain{})
	require.NoError(t, err)
	defer done()
	rpc := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))

	deployed := make(map[string]string, len(contracts))
	for _, entry := range contracts {
		for name, contract := range entry {
			build := domain.LoadBuildLinked(contract, deployed)
			deployed[name], err = deployBytecode(ctx, rpc, build)
			require.NoError(t, err)
		}
	}
	return deployed
}

func newTestDomain(t *testing.T, domainName string, config *types.Config) (context.CancelFunc, *Zeto, rpcbackend.Backend) {
	var domain *Zeto
	tb := testbed.NewTestBed()
	plugin := plugintk.NewDomain(func(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {
		domain = &Zeto{Callbacks: callbacks}
		return domain
	})
	url, done, err := tb.StartForTest("../../testbed.config.yaml", map[string]*testbed.TestbedDomain{
		domainName: {
			Config: mapConfig(t, config),
			Plugin: plugin,
		},
	})
	require.NoError(t, err)
	rpc := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))
	return done, domain, rpc
}

func deployBytecode(ctx context.Context, rpc rpcbackend.Backend, build *domain.SolidityBuild) (string, error) {
	var addr string
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode",
		controllerName, build.ABI, build.Bytecode.String(), `{}`)
	if rpcerr != nil {
		return "", rpcerr.Error()
	}
	return addr, nil
}

func TestZeto(t *testing.T) {
	ctx := context.Background()
	log.L(ctx).Infof("TestZeto")
	domainName := "zeto_" + kata.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)

	log.L(ctx).Infof("Deploying Zeto libraries+factory")
	contractSource := []map[string][]byte{
		{
			"Commonlib":        commonLibJSON,
			"verifier":         Groth16Verifier_Anon,
			"depositVerifier":  Groth16Verifier_CheckHashesValue,
			"withdrawVerifier": Groth16Verifier_CheckInputsOutputsValue,
		},
		{
			"factory": zetoFactoryJSON, //depends on Commonlib from previous group
		},
	}
	contracts := deployContracts(ctx, t, contractSource)
	for name, address := range contracts {
		log.L(ctx).Infof("%s deployed to %s", name, address)
	}

	done, zeto, rpc := newTestDomain(t, domainName, &types.Config{
		FactoryAddress: contracts["factory"],
		Libraries:      contracts,
	})
	defer done()

	log.L(ctx).Infof("Deploying an instance of Zeto")
	var zetoAddress ethtypes.Address0xHex
	rpcerr := rpc.CallRPC(ctx, &zetoAddress, "testbed_deploy",
		domainName, &types.ConstructorParams{
			From:             controllerName,
			Verifier:         contracts["verifier"],
			DepositVerifier:  contracts["depositVerifier"],
			WithdrawVerifier: contracts["withdrawVerifier"],
		})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	log.L(ctx).Infof("Zeto instance deployed to %s", zetoAddress)

	log.L(ctx).Infof("Mint 10 from controller to controller")
	var boolResult bool
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &kata.PrivateContractInvoke{
		From:     controllerName,
		To:       kata.EthAddress(zetoAddress),
		Function: *types.ZetoABI.Functions()["mint"],
		Inputs: toJSON(t, &types.MintParams{
			To:     controllerName,
			Amount: ethtypes.NewHexInteger64(10),
		}),
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	assert.True(t, boolResult)

	coins, err := zeto.FindCoins(ctx, "{}")
	require.NoError(t, err)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(10), coins[0].Amount.Int64())
	assert.Equal(t, controllerName, coins[0].Owner)

	log.L(ctx).Infof("Mint 20 from controller to controller")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &kata.PrivateContractInvoke{
		From:     controllerName,
		To:       kata.EthAddress(zetoAddress),
		Function: *types.ZetoABI.Functions()["mint"],
		Inputs: toJSON(t, &types.MintParams{
			To:     controllerName,
			Amount: ethtypes.NewHexInteger64(20),
		}),
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
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
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &kata.PrivateContractInvoke{
		From:     recipient1Name,
		To:       kata.EthAddress(zetoAddress),
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
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &kata.PrivateContractInvoke{
		From:     controllerName,
		To:       kata.EthAddress(zetoAddress),
		Function: *types.ZetoABI.Functions()["transfer"],
		Inputs: toJSON(t, &types.TransferParams{
			To:     recipient1Name,
			Amount: ethtypes.NewHexInteger64(25),
		}),
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
}
