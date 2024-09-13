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

package noto

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-resty/resty/v2"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	notaryName     = "notary"
	recipient1Name = "recipient1"
	recipient2Name = "recipient2"
)

func toJSON(t *testing.T, v any) []byte {
	result, err := json.Marshal(v)
	require.NoError(t, err)
	return result
}

func mapConfig(t *testing.T, config *types.DomainConfig) (m map[string]any) {
	configJSON, err := json.Marshal(&config)
	require.NoError(t, err)
	err = json.Unmarshal(configJSON, &m)
	require.NoError(t, err)
	return m
}

func deployContracts(ctx context.Context, t *testing.T, contracts map[string][]byte) map[string]string {
	tb := testbed.NewTestBed()
	url, done, err := tb.StartForTest("../../testbed.config.yaml", map[string]*testbed.TestbedDomain{})
	require.NoError(t, err)
	defer done()
	rpc := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))

	deployed := make(map[string]string, len(contracts))
	for name, contract := range contracts {
		build := domain.LoadBuild(contract)
		var addr string
		rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode",
			notaryName, build.ABI, build.Bytecode.String(), `{}`)
		if rpcerr != nil {
			assert.NoError(t, rpcerr.Error())
		}
		deployed[name] = addr
	}
	return deployed
}

func newNotoDomain(t *testing.T, config *types.DomainConfig) (*Noto, *testbed.TestbedDomain) {
	var domain Noto
	return &domain, &testbed.TestbedDomain{
		Config: mapConfig(t, config),
		Plugin: plugintk.NewDomain(func(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {
			domain.Callbacks = callbacks
			return &domain
		}),
	}
}

func newTestbed(t *testing.T, domains map[string]*testbed.TestbedDomain) (context.CancelFunc, testbed.Testbed, rpcbackend.Backend) {
	tb := testbed.NewTestBed()
	url, done, err := tb.StartForTest("../../testbed.config.yaml", domains)
	assert.NoError(t, err)
	rpc := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))
	return done, tb, rpc
}

func functionBuilder(ctx context.Context, t *testing.T, eth ethclient.EthClient, abi abi.ABI, functionName string) ethclient.ABIFunctionRequestBuilder {
	abiClient, err := eth.ABI(ctx, abi)
	assert.NoError(t, err)
	fn, err := abiClient.Function(ctx, functionName)
	assert.NoError(t, err)
	return fn.R(ctx)
}

func waitFor(ctx context.Context, t *testing.T, tb testbed.Testbed, txHash *tktypes.Bytes32, err error) *blockindexer.IndexedTransaction {
	require.NoError(t, err)
	tx, err := tb.Components().BlockIndexer().WaitForTransaction(ctx, *txHash)
	assert.NoError(t, err)
	return tx
}

func TestNoto(t *testing.T) {
	ctx := context.Background()
	log.L(ctx).Infof("TestNoto")
	domainName := "noto_" + tktypes.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)

	log.L(ctx).Infof("Deploying Noto factory")
	contractSource := map[string][]byte{
		"factory": notoFactoryJSON,
	}
	contracts := deployContracts(ctx, t, contractSource)
	for name, address := range contracts {
		log.L(ctx).Infof("%s deployed to %s", name, address)
	}

	noto, notoTestbed := newNotoDomain(t, &types.DomainConfig{
		FactoryAddress: contracts["factory"],
	})
	done, tb, rpc := newTestbed(t, map[string]*testbed.TestbedDomain{
		domainName: notoTestbed,
	})
	defer done()

	_, notaryKey, err := tb.Components().KeyManager().ResolveKey(ctx, notaryName, algorithms.ECDSA_SECP256K1_PLAINBYTES)
	require.NoError(t, err)
	_, recipient1Key, err := tb.Components().KeyManager().ResolveKey(ctx, recipient1Name, algorithms.ECDSA_SECP256K1_PLAINBYTES)
	require.NoError(t, err)

	log.L(ctx).Infof("Deploying an instance of Noto")
	var notoAddress ethtypes.Address0xHex
	rpcerr := rpc.CallRPC(ctx, &notoAddress, "testbed_deploy",
		domainName, &types.ConstructorParams{
			Notary: notaryName,
		})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}
	log.L(ctx).Infof("Noto instance deployed to %s", notoAddress)

	log.L(ctx).Infof("Mint 100 from notary to notary")
	var boolResult bool
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     notaryName,
		To:       tktypes.EthAddress(notoAddress),
		Function: *types.NotoABI.Functions()["mint"],
		Inputs: toJSON(t, &types.MintParams{
			To:     notaryName,
			Amount: ethtypes.NewHexInteger64(100),
		}),
	})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}
	assert.True(t, boolResult)

	coins, err := noto.FindCoins(ctx, "{}")
	require.NoError(t, err)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(100), coins[0].Amount.Int64())
	assert.Equal(t, notaryKey, coins[0].Owner.String())

	log.L(ctx).Infof("Attempt mint from non-notary (should fail)")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     recipient1Name,
		To:       tktypes.EthAddress(notoAddress),
		Function: *types.NotoABI.Functions()["mint"],
		Inputs: toJSON(t, &types.MintParams{
			To:     recipient1Name,
			Amount: ethtypes.NewHexInteger64(100),
		}),
	})
	require.NotNil(t, rpcerr)
	assert.ErrorContains(t, rpcerr.Error(), "mint can only be initiated by notary")
	assert.True(t, boolResult)

	log.L(ctx).Infof("Transfer 150 from notary (should fail)")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     notaryName,
		To:       tktypes.EthAddress(notoAddress),
		Function: *types.NotoABI.Functions()["transfer"],
		Inputs: toJSON(t, &types.TransferParams{
			To:     recipient1Name,
			Amount: ethtypes.NewHexInteger64(150),
		}),
	})
	require.NotNil(t, rpcerr)
	assert.Regexp(t, "insufficient funds", rpcerr.Error())

	log.L(ctx).Infof("Transfer 50 from notary to recipient1")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     notaryName,
		To:       tktypes.EthAddress(notoAddress),
		Function: *types.NotoABI.Functions()["transfer"],
		Inputs: toJSON(t, &types.TransferParams{
			To:     recipient1Name,
			Amount: ethtypes.NewHexInteger64(50),
		}),
	})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}

	coins, err = noto.FindCoins(ctx, "{}")
	require.NoError(t, err)
	require.Len(t, coins, 3)

	// This should have been spent
	// TODO: why does it still exist?
	assert.Equal(t, int64(100), coins[0].Amount.Int64())
	assert.Equal(t, notaryKey, coins[0].Owner.String())

	// These are the expected coins after the transfer
	assert.Equal(t, int64(50), coins[1].Amount.Int64())
	assert.Equal(t, recipient1Key, coins[1].Owner.String())
	assert.Equal(t, int64(50), coins[2].Amount.Int64())
	assert.Equal(t, notaryKey, coins[2].Owner.String())

	log.L(ctx).Infof("Transfer 50 from recipient1 to recipient2")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     recipient1Name,
		To:       tktypes.EthAddress(notoAddress),
		Function: *types.NotoABI.Functions()["transfer"],
		Inputs: toJSON(t, &types.TransferParams{
			To:     recipient2Name,
			Amount: ethtypes.NewHexInteger64(50),
		}),
	})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}

	coins, err = noto.FindCoins(ctx, "{}")
	require.NoError(t, err)
	require.Len(t, coins, 4) // TODO: verify coins
}

func TestNotoSelfSubmit(t *testing.T) {
	ctx := context.Background()
	log.L(ctx).Infof("TestNotoSelfSubmit")
	domainName := "noto_" + tktypes.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)

	log.L(ctx).Infof("Deploying Noto factory")
	contractSource := map[string][]byte{
		"factory": notoFactoryJSON,
		"noto":    notoSelfSubmitJSON,
	}
	contracts := deployContracts(ctx, t, contractSource)
	for name, address := range contracts {
		log.L(ctx).Infof("%s deployed to %s", name, address)
	}

	factoryAddress, err := ethtypes.NewAddress(contracts["factory"])
	require.NoError(t, err)

	noto, notoTestbed := newNotoDomain(t, &types.DomainConfig{
		FactoryAddress: factoryAddress.String(),
	})
	done, tb, rpc := newTestbed(t, map[string]*testbed.TestbedDomain{
		domainName: notoTestbed,
	})
	defer done()

	_, notaryKey, err := tb.Components().KeyManager().ResolveKey(ctx, notaryName, algorithms.ECDSA_SECP256K1_PLAINBYTES)
	require.NoError(t, err)

	eth := tb.Components().EthClientFactory().HTTPClient()
	notoFactory := domain.LoadBuild(notoFactoryJSON)
	txHash, err := functionBuilder(ctx, t, eth, notoFactory.ABI, "registerImplementation").
		Signer(notaryName).
		To(factoryAddress).
		Input(map[string]any{
			"name":           "selfsubmit",
			"implementation": contracts["noto"],
		}).
		SignAndSend()
	require.NoError(t, err)
	waitFor(ctx, t, tb, txHash, err)

	log.L(ctx).Infof("Deploying an instance of Noto")
	var notoAddress ethtypes.Address0xHex
	rpcerr := rpc.CallRPC(ctx, &notoAddress, "testbed_deploy",
		domainName, &types.ConstructorParams{
			Notary:         notaryName,
			Implementation: "selfsubmit",
		},
	)
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}
	log.L(ctx).Infof("Noto instance deployed to %s", notoAddress)

	log.L(ctx).Infof("Mint 100 from notary to notary")
	var boolResult bool
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     notaryName,
		To:       tktypes.EthAddress(notoAddress),
		Function: *types.NotoABI.Functions()["mint"],
		Inputs: toJSON(t, &types.MintParams{
			To:     notaryName,
			Amount: ethtypes.NewHexInteger64(100),
		}),
	})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}
	assert.True(t, boolResult)

	coins, err := noto.FindCoins(ctx, "{}")
	require.NoError(t, err)
	assert.Len(t, coins, 1)
	assert.Equal(t, int64(100), coins[0].Amount.Int64())
	assert.Equal(t, notaryKey, coins[0].Owner.String())

	log.L(ctx).Infof("Transfer 50 from notary to recipient1")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     notaryName,
		To:       tktypes.EthAddress(notoAddress),
		Function: *types.NotoABI.Functions()["transfer"],
		Inputs: toJSON(t, &types.TransferParams{
			To:     recipient1Name,
			Amount: ethtypes.NewHexInteger64(50),
		}),
	})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}

	coins, err = noto.FindCoins(ctx, "{}")
	require.NoError(t, err)
	assert.Len(t, coins, 3) // TODO: verify coins

	log.L(ctx).Infof("Transfer 50 from recipient1 to recipient2")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     recipient1Name,
		To:       tktypes.EthAddress(notoAddress),
		Function: *types.NotoABI.Functions()["transfer"],
		Inputs: toJSON(t, &types.TransferParams{
			To:     recipient2Name,
			Amount: ethtypes.NewHexInteger64(50),
		}),
	})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}

	coins, err = noto.FindCoins(ctx, "{}")
	require.NoError(t, err)
	assert.Len(t, coins, 4) // TODO: verify coins
}
