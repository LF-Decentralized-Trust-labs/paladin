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

	_ "embed"

	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed abis/NotoSelfSubmit.json
var notoSelfSubmitJSON []byte

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

func deployContracts(ctx context.Context, t *testing.T, hdWalletSeed *testbed.UTInitFunction, contracts map[string][]byte) map[string]string {
	tb := testbed.NewTestBed()
	url, _, done, err := tb.StartForTest("../../testbed.config.yaml", map[string]*testbed.TestbedDomain{}, hdWalletSeed)
	require.NoError(t, err)
	defer done()
	rpc := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))

	deployed := make(map[string]string, len(contracts))
	for name, contract := range contracts {
		build := domain.LoadBuild(contract)
		var addr string
		rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode",
			notaryName, build.ABI, build.Bytecode.String(), tktypes.RawJSON(`{}`))
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
		RegistryAddress: tktypes.MustEthAddress(config.FactoryAddress),
	}
}

func newTestbed(t *testing.T, hdWalletSeed *testbed.UTInitFunction, domains map[string]*testbed.TestbedDomain) (context.CancelFunc, testbed.Testbed, rpcbackend.Backend) {
	tb := testbed.NewTestBed()
	url, _, done, err := tb.StartForTest("../../testbed.config.yaml", domains, hdWalletSeed)
	assert.NoError(t, err)
	rpc := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))
	return done, tb, rpc
}

func findAvailableCoins(t *testing.T, ctx context.Context, rpc rpcbackend.Backend, noto *Noto, address tktypes.EthAddress, jq *query.QueryJSON) []*types.NotoCoinState {
	if jq == nil {
		jq = query.NewQueryBuilder().Limit(100).Query()
	}
	var notoCoins []*types.NotoCoinState
	rpcerr := rpc.CallRPC(ctx, &notoCoins, "pstate_queryStates",
		noto.name,
		address,
		noto.coinSchema.Id,
		jq,
		"available")
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}
	return notoCoins
}

func TestNoto(t *testing.T) {
	ctx := context.Background()
	log.L(ctx).Infof("TestNoto")
	domainName := "noto_" + tktypes.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)

	hdWalletSeed := testbed.HDWalletSeedScopedToTest()

	log.L(ctx).Infof("Deploying Noto factory")
	contractSource := map[string][]byte{
		"factory": notoFactoryJSON,
	}
	contracts := deployContracts(ctx, t, hdWalletSeed, contractSource)
	for name, address := range contracts {
		log.L(ctx).Infof("%s deployed to %s", name, address)
	}

	noto, notoTestbed := newNotoDomain(t, &types.DomainConfig{
		FactoryAddress: contracts["factory"],
	})
	done, tb, rpc := newTestbed(t, hdWalletSeed, map[string]*testbed.TestbedDomain{
		domainName: notoTestbed,
	})
	defer done()

	notaryKey, err := tb.ResolveKey(ctx, notaryName, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	recipient1Key, err := tb.ResolveKey(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	recipient2Key, err := tb.ResolveKey(ctx, recipient2Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	log.L(ctx).Infof("Deploying an instance of Noto")
	var notoAddress tktypes.EthAddress
	rpcerr := rpc.CallRPC(ctx, &notoAddress, "testbed_deploy",
		domainName, &types.ConstructorParams{
			Notary: notaryName,
		})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}
	log.L(ctx).Infof("Noto instance deployed to %s", notoAddress)

	log.L(ctx).Infof("Mint 100 from notary to notary")
	var invokeResult tktypes.PrivateContractTransaction
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     notaryName,
		To:       notoAddress,
		Function: *types.NotoABI.Functions()["mint"],
		Inputs: toJSON(t, &types.MintParams{
			To:     notaryName,
			Amount: tktypes.Int64ToInt256(100),
		}),
	}, true)
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}

	coins := findAvailableCoins(t, ctx, rpc, noto, notoAddress, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(100), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey.Verifier.Verifier, coins[0].Data.Owner.String())

	log.L(ctx).Infof("Attempt mint from non-notary (should fail)")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     recipient1Name,
		To:       notoAddress,
		Function: *types.NotoABI.Functions()["mint"],
		Inputs: toJSON(t, &types.MintParams{
			To:     recipient1Name,
			Amount: tktypes.Int64ToInt256(100),
		}),
	}, true)
	require.NotNil(t, rpcerr)
	assert.ErrorContains(t, rpcerr.Error(), "PD200009")

	coins = findAvailableCoins(t, ctx, rpc, noto, notoAddress, nil)
	require.Len(t, coins, 1)

	log.L(ctx).Infof("Transfer 150 from notary (should fail)")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     notaryName,
		To:       notoAddress,
		Function: *types.NotoABI.Functions()["transfer"],
		Inputs: toJSON(t, &types.TransferParams{
			To:     recipient1Name,
			Amount: tktypes.Int64ToInt256(150),
		}),
	}, true)
	require.NotNil(t, rpcerr)
	assert.ErrorContains(t, rpcerr.Error(), "PD200005")

	coins = findAvailableCoins(t, ctx, rpc, noto, notoAddress, nil)
	require.Len(t, coins, 1)

	log.L(ctx).Infof("Transfer 50 from notary to recipient1")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     notaryName,
		To:       notoAddress,
		Function: *types.NotoABI.Functions()["transfer"],
		Inputs: toJSON(t, &types.TransferParams{
			To:     recipient1Name,
			Amount: tktypes.Int64ToInt256(50),
		}),
	}, true)
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}

	coins = findAvailableCoins(t, ctx, rpc, noto, notoAddress, nil)
	require.NoError(t, err)
	require.Len(t, coins, 2)

	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key.Verifier.Verifier, coins[0].Data.Owner.String())
	assert.Equal(t, int64(50), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey.Verifier.Verifier, coins[1].Data.Owner.String())

	log.L(ctx).Infof("Transfer 50 from recipient1 to recipient2")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     recipient1Name,
		To:       notoAddress,
		Function: *types.NotoABI.Functions()["transfer"],
		Inputs: toJSON(t, &types.TransferParams{
			To:     recipient2Name,
			Amount: tktypes.Int64ToInt256(50),
		}),
	}, true)
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}

	coins = findAvailableCoins(t, ctx, rpc, noto, notoAddress, nil)
	require.NoError(t, err)
	require.Len(t, coins, 2)

	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey.Verifier.Verifier, coins[0].Data.Owner.String())
	assert.Equal(t, int64(50), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key.Verifier.Verifier, coins[1].Data.Owner.String())
}

func TestNotoApprove(t *testing.T) {
	ctx := context.Background()
	log.L(ctx).Infof("TestNotoApprove")
	domainName := "noto_" + tktypes.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)

	hdWalletSeed := testbed.HDWalletSeedScopedToTest()

	log.L(ctx).Infof("Deploying Noto factory")
	contractSource := map[string][]byte{
		"factory": notoFactoryJSON,
	}
	contracts := deployContracts(ctx, t, hdWalletSeed, contractSource)
	for name, address := range contracts {
		log.L(ctx).Infof("%s deployed to %s", name, address)
	}

	noto, notoTestbed := newNotoDomain(t, &types.DomainConfig{
		FactoryAddress: contracts["factory"],
	})
	done, tb, rpc := newTestbed(t, hdWalletSeed, map[string]*testbed.TestbedDomain{
		domainName: notoTestbed,
	})
	defer done()

	recipient1Key, err := tb.ResolveKey(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	log.L(ctx).Infof("Deploying an instance of Noto")
	var notoAddress tktypes.EthAddress
	rpcerr := rpc.CallRPC(ctx, &notoAddress, "testbed_deploy",
		domainName, &types.ConstructorParams{
			Notary: notaryName,
		})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}
	log.L(ctx).Infof("Noto instance deployed to %s", notoAddress)

	log.L(ctx).Infof("Mint 100 from notary to notary")
	var invokeResult tktypes.PrivateContractTransaction
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     notaryName,
		To:       notoAddress,
		Function: *types.NotoABI.Functions()["mint"],
		Inputs: toJSON(t, &types.MintParams{
			To:     notaryName,
			Amount: tktypes.Int64ToInt256(100),
		}),
	}, true)
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}

	log.L(ctx).Infof("Approve recipient1 to claim 50")
	var prepared tktypes.PrivateContractTransaction
	rpcerr = rpc.CallRPC(ctx, &prepared, "testbed_prepare", &tktypes.PrivateContractInvoke{
		From:     notaryName,
		To:       notoAddress,
		Function: *types.NotoABI.Functions()["transfer"],
		Inputs: toJSON(t, &types.TransferParams{
			To:     recipient1Name,
			Amount: tktypes.Int64ToInt256(50),
		}),
	})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}

	var transferParams NotoTransferParams
	err = json.Unmarshal(prepared.ParamsJSON, &transferParams)
	require.NoError(t, err)

	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     notaryName,
		To:       notoAddress,
		Function: *types.NotoABI.Functions()["approveTransfer"],
		Inputs: toJSON(t, &types.ApproveParams{
			Inputs:   prepared.InputStates,
			Outputs:  prepared.OutputStates,
			Data:     transferParams.Data,
			Delegate: tktypes.MustEthAddress(recipient1Key.Verifier.Verifier),
		}),
	}, true)
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}

	log.L(ctx).Infof("Claim 50 using approval")
	receipt, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		Transaction: pldapi.Transaction{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: "transferWithApproval",
			From:     recipient1Name,
			To:       &notoAddress,
			Data:     tktypes.JSONString(transferParams),
		},
		ABI: noto.contractABI,
	})
	assert.NoError(t, err)
	log.L(ctx).Infof("Claimed with transaction: %s", receipt.TransactionHash)
}

func TestNotoSelfSubmit(t *testing.T) {
	ctx := context.Background()
	log.L(ctx).Infof("TestNotoSelfSubmit")
	domainName := "noto_" + tktypes.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)

	hdWalletSeed := testbed.HDWalletSeedScopedToTest()

	log.L(ctx).Infof("Deploying Noto factory")
	contractSource := map[string][]byte{
		"factory": notoFactoryJSON,
		"noto":    notoSelfSubmitJSON,
	}
	contracts := deployContracts(ctx, t, hdWalletSeed, contractSource)
	for name, address := range contracts {
		log.L(ctx).Infof("%s deployed to %s", name, address)
	}

	factoryAddress, err := tktypes.ParseEthAddress(contracts["factory"])
	require.NoError(t, err)

	noto, notoTestbed := newNotoDomain(t, &types.DomainConfig{
		FactoryAddress: factoryAddress.String(),
	})
	done, tb, rpc := newTestbed(t, hdWalletSeed, map[string]*testbed.TestbedDomain{
		domainName: notoTestbed,
	})
	defer done()

	notaryKey, err := tb.ResolveKey(ctx, notaryName, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	recipient1Key, err := tb.ResolveKey(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	recipient2Key, err := tb.ResolveKey(ctx, recipient2Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	notoFactory := domain.LoadBuild(notoFactoryJSON)
	_, err = tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		Transaction: pldapi.Transaction{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: "registerImplementation",
			From:     notaryName,
			To:       factoryAddress,
			Data: tktypes.JSONString(map[string]any{
				"name":           "selfsubmit",
				"implementation": contracts["noto"],
			}),
		},
		ABI: notoFactory.ABI,
	})
	require.NoError(t, err)

	var callResult map[string]any
	err = tb.ExecBaseLedgerCall(ctx, &callResult, &pldapi.TransactionInput{
		Transaction: pldapi.Transaction{
			Type:     pldapi.TransactionTypePublic.Enum(),
			To:       factoryAddress,
			Function: "getImplementation",
			From:     notaryName,
			Data: tktypes.JSONString(map[string]any{
				"name": "selfsubmit",
			}),
		},
		ABI: notoFactory.ABI,
	})
	require.NoError(t, err)
	require.NotEmpty(t, callResult["implementation"])

	log.L(ctx).Infof("Deploying an instance of Noto")
	var notoAddress tktypes.EthAddress
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
	var invokeResult tktypes.PrivateContractTransaction
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     notaryName,
		To:       notoAddress,
		Function: *types.NotoABI.Functions()["mint"],
		Inputs: toJSON(t, &types.MintParams{
			To:     notaryName,
			Amount: tktypes.Int64ToInt256(100),
		}),
	}, true)
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}

	coins := findAvailableCoins(t, ctx, rpc, noto, notoAddress, nil)
	require.NoError(t, err)
	assert.Len(t, coins, 1)
	assert.Equal(t, int64(100), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey.Verifier.Verifier, coins[0].Data.Owner.String())

	log.L(ctx).Infof("Transfer 50 from notary to recipient1")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     notaryName,
		To:       notoAddress,
		Function: *types.NotoABI.Functions()["transfer"],
		Inputs: toJSON(t, &types.TransferParams{
			To:     recipient1Name,
			Amount: tktypes.Int64ToInt256(50),
		}),
	}, true)
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}

	coins = findAvailableCoins(t, ctx, rpc, noto, notoAddress, nil)
	require.NoError(t, err)
	require.Len(t, coins, 2)

	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key.Verifier.Verifier, coins[0].Data.Owner.String())
	assert.Equal(t, int64(50), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey.Verifier.Verifier, coins[1].Data.Owner.String())

	log.L(ctx).Infof("Transfer 50 from recipient1 to recipient2")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     recipient1Name,
		To:       notoAddress,
		Function: *types.NotoABI.Functions()["transfer"],
		Inputs: toJSON(t, &types.TransferParams{
			To:     recipient2Name,
			Amount: tktypes.Int64ToInt256(50),
		}),
	}, true)
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}

	coins = findAvailableCoins(t, ctx, rpc, noto, notoAddress, nil)
	require.NoError(t, err)
	require.Len(t, coins, 2)

	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey.Verifier.Verifier, coins[0].Data.Owner.String())
	assert.Equal(t, int64(50), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key.Verifier.Verifier, coins[1].Data.Owner.String())
}
