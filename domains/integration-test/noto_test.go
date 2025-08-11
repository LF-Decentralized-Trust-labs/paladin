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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/testbed"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/integration-test/helpers"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/noto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/solutils"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	notaryName = "notary@node1"
)

func TestNotoSuite(t *testing.T) {
	suite.Run(t, new(notoTestSuite))
}

type notoTestSuite struct {
	suite.Suite
	hdWalletSeed   *testbed.UTInitFunction
	domainName     string
	factoryAddress string
}

func (s *notoTestSuite) SetupSuite() {
	ctx := context.Background()
	s.domainName = "noto_" + pldtypes.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", s.domainName)

	s.hdWalletSeed = testbed.HDWalletSeedScopedToTest()

	log.L(ctx).Infof("Deploying Noto factory")
	contractSource := map[string][]byte{
		"factory": helpers.NotoFactoryJSON,
	}
	contracts := deployContracts(ctx, s.T(), s.hdWalletSeed, notaryName, contractSource)
	for name, address := range contracts {
		log.L(ctx).Infof("%s deployed to %s", name, address)
	}
	s.factoryAddress = contracts["factory"]
}

func toJSON(t *testing.T, v any) []byte {
	result, err := json.Marshal(v)
	require.NoError(t, err)
	return result
}

func (s *notoTestSuite) TestNoto() {
	ctx := context.Background()
	t := s.T()
	log.L(ctx).Infof("TestNoto")

	waitForNoto, notoTestbed := newNotoDomain(t, &types.DomainConfig{
		FactoryAddress: s.factoryAddress,
	})
	done, _, tb, rpc := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.domainName: notoTestbed,
	})
	defer done()

	notoDomain := <-waitForNoto

	notaryKey, err := tb.ResolveKey(ctx, notaryName, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	recipient1Key, err := tb.ResolveKey(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	recipient2Key, err := tb.ResolveKey(ctx, recipient2Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	log.L(ctx).Infof("Deploying an instance of Noto")
	noto := helpers.DeployNoto(ctx, t, rpc, s.domainName, notary, nil)
	log.L(ctx).Infof("Noto deployed to %s", noto.Address)

	log.L(ctx).Infof("Mint 100 from notary to notary")
	var invokeResult testbed.TransactionResult
	rpcerr := rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "mint",
			Data: toJSON(t, &types.MintParams{
				To:     notaryName,
				Amount: pldtypes.Int64ToInt256(100),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	coins := findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(100), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey.Verifier.Verifier, coins[0].Data.Owner.String())

	// check balance
	balanceOfResult := noto.BalanceOf(ctx, &types.BalanceOfParam{Account: notaryName}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "100", balanceOfResult["totalBalance"].(string), "Balance of notary should be 100")

	log.L(ctx).Infof("Attempt mint from non-notary (should fail)")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "mint",
			Data: toJSON(t, &types.MintParams{
				To:     recipient1Name,
				Amount: pldtypes.Int64ToInt256(100),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NotNil(t, rpcerr)
	assert.ErrorContains(t, rpcerr, "PD200009")

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)

	log.L(ctx).Infof("Transfer 150 from notary (should fail)")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "transfer",
			Data: toJSON(t, &types.TransferParams{
				To:     recipient1Name,
				Amount: pldtypes.Int64ToInt256(150),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NotNil(t, rpcerr)
	assert.ErrorContains(t, rpcerr, "assemble result was REVERT")

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)

	log.L(ctx).Infof("Transfer 50 from notary to recipient1")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "transfer",
			Data: toJSON(t, &types.TransferParams{
				To:     recipient1Name,
				Amount: pldtypes.Int64ToInt256(50),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.NoError(t, err)
	require.Len(t, coins, 2)

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: notaryName}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of notary should be 50")

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should be 50")

	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key.Verifier.Verifier, coins[0].Data.Owner.String())
	assert.Equal(t, int64(50), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey.Verifier.Verifier, coins[1].Data.Owner.String())

	log.L(ctx).Infof("Transfer 50 from recipient1 to recipient2")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "transfer",
			Data: toJSON(t, &types.TransferParams{
				To:     recipient2Name,
				Amount: pldtypes.Int64ToInt256(50),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.NoError(t, err)
	require.Len(t, coins, 2)

	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey.Verifier.Verifier, coins[0].Data.Owner.String())
	assert.Equal(t, int64(50), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key.Verifier.Verifier, coins[1].Data.Owner.String())

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "0", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should be 0")

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient2Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient2 should be 50")

	log.L(ctx).Infof("Burn 25 from recipient2")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient2Name,
			To:       noto.Address,
			Function: "burn",
			Data: toJSON(t, &types.BurnParams{
				Amount: pldtypes.Int64ToInt256(25),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.NoError(t, err)
	require.Len(t, coins, 2)

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient2Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "25", balanceOfResult["totalBalance"].(string), "Balance of recipient should be 25")

	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey.Verifier.Verifier, coins[0].Data.Owner.String())
	assert.Equal(t, int64(25), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key.Verifier.Verifier, coins[1].Data.Owner.String())
}

func (s *notoTestSuite) TestNotoApprove() {
	ctx := context.Background()
	t := s.T()
	log.L(ctx).Infof("TestNotoApprove")

	_, notoTestbed := newNotoDomain(t, &types.DomainConfig{
		FactoryAddress: s.factoryAddress,
	})
	done, _, tb, rpc := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.domainName: notoTestbed,
	})
	defer done()

	recipient1Key, err := tb.ResolveKey(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	log.L(ctx).Infof("Deploying an instance of Noto")
	noto := helpers.DeployNoto(ctx, t, rpc, s.domainName, notary, nil)
	log.L(ctx).Infof("Noto deployed to %s", noto.Address)

	log.L(ctx).Infof("Mint 100 from notary to notary")
	var invokeResult testbed.TransactionResult
	rpcerr := rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "mint",
			Data: toJSON(t, &types.MintParams{
				To:     notaryName,
				Amount: pldtypes.Int64ToInt256(100),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	log.L(ctx).Infof("Approve recipient1 to claim 50")
	var prepared testbed.TransactionResult
	rpcerr = rpc.CallRPC(ctx, &prepared, "testbed_prepare", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "transfer",
			Data: toJSON(t, &types.TransferParams{
				To:     recipient1Name,
				Amount: pldtypes.Int64ToInt256(50),
			}),
		},
		ABI: types.NotoABI,
	})
	require.NoError(t, rpcerr)

	var transferParams map[string]any
	err = json.Unmarshal(prepared.PreparedTransaction.Data, &transferParams)
	require.NoError(t, err)

	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "approveTransfer",
			Data: toJSON(t, &types.ApproveParams{
				Inputs:   prepared.InputStates,
				Outputs:  prepared.OutputStates,
				Data:     pldtypes.MustParseHexBytes(transferParams["data"].(string)),
				Delegate: pldtypes.MustEthAddress(recipient1Key.Verifier.Verifier),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	log.L(ctx).Infof("Claim 50 using approval")
	notoBuild := solutils.MustLoadBuild(helpers.NotoInterfaceJSON)
	receipt, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: "transferWithApproval",
			From:     recipient1Name,
			To:       noto.Address,
			Data:     pldtypes.JSONString(transferParams),
		},
		ABI: notoBuild.ABI,
	})
	assert.NoError(t, err)
	log.L(ctx).Infof("Claimed with transaction: %s", receipt.TransactionHash)
}

func (s *notoTestSuite) TestNotoLock() {
	ctx := context.Background()
	t := s.T()
	log.L(ctx).Infof("TestNotoLock")

	waitForNoto, notoTestbed := newNotoDomain(t, &types.DomainConfig{
		FactoryAddress: s.factoryAddress,
	})
	done, _, tb, rpc := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.domainName: notoTestbed,
	})
	defer done()
	pld := helpers.NewPaladinClient(t, ctx, tb)

	notoDomain := <-waitForNoto

	recipient1Key, err := tb.ResolveKey(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	recipient2Key, err := tb.ResolveKey(ctx, recipient2Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	log.L(ctx).Infof("Deploying an instance of Noto")
	noto := helpers.DeployNoto(ctx, t, rpc, s.domainName, notary, nil)
	log.L(ctx).Infof("Noto deployed to %s", noto.Address)

	log.L(ctx).Infof("Mint 100 from notary to recipient1")
	var invokeResult testbed.TransactionResult
	rpcerr := rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "mint",
			Data: toJSON(t, &types.MintParams{
				To:     recipient1Name,
				Amount: pldtypes.Int64ToInt256(100),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	coins := findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(100), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key.Verifier.Verifier, coins[0].Data.Owner.String())

	log.L(ctx).Infof("Lock 50 from recipient1")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "lock",
			Data: toJSON(t, &types.LockParams{
				Amount: pldtypes.Int64ToInt256(50),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	balanceOfResult := noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient should be 50")

	var lockReceipt types.NotoDomainReceipt
	err = json.Unmarshal(invokeResult.DomainReceipt, &lockReceipt)
	require.NoError(t, err)
	require.NotNil(t, lockReceipt.LockInfo)
	require.NotEmpty(t, lockReceipt.LockInfo.LockID)

	lockedCoins := findAvailableCoins[types.NotoLockedCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, lockedCoins, 1)
	assert.Equal(t, int64(50), lockedCoins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key.Verifier.Verifier, lockedCoins[0].Data.Owner.String())
	coins = findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key.Verifier.Verifier, coins[0].Data.Owner.String())

	log.L(ctx).Infof("Transfer 50 from recipient1 to recipient2 (succeeds but does not use locked state)")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "transfer",
			Data: toJSON(t, &types.TransferParams{
				To:     recipient2Name,
				Amount: pldtypes.Int64ToInt256(50),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "0", balanceOfResult["totalBalance"].(string), "Balance of recipient should be 0")

	lockedCoins = findAvailableCoins[types.NotoLockedCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, lockedCoins, 1)
	assert.Equal(t, int64(50), lockedCoins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key.Verifier.Verifier, lockedCoins[0].Data.Owner.String())
	coins = findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key.Verifier.Verifier, coins[0].Data.Owner.String())

	log.L(ctx).Infof("Prepare unlock that will send all 50 to recipient2")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "prepareUnlock",
			Data: toJSON(t, &types.UnlockParams{
				LockID: lockReceipt.LockInfo.LockID,
				From:   recipient1Name,
				Recipients: []*types.UnlockRecipient{{
					To:     recipient2Name,
					Amount: pldtypes.Int64ToInt256(50),
				}},
				Data: pldtypes.HexBytes{},
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	var unlockReceipt types.NotoDomainReceipt
	err = json.Unmarshal(invokeResult.DomainReceipt, &unlockReceipt)
	require.NoError(t, err)

	log.L(ctx).Infof("Delegate lock to recipient2")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "delegateLock",
			Data: toJSON(t, &types.DelegateLockParams{
				LockID:   unlockReceipt.LockInfo.LockID,
				Unlock:   unlockReceipt.LockInfo.UnlockParams,
				Delegate: pldtypes.MustEthAddress(recipient2Key.Verifier.Verifier),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	log.L(ctx).Infof("Unlock from recipient2")
	notoBuild := solutils.MustLoadBuild(helpers.NotoInterfaceJSON)
	tx := pld.ForABI(ctx, notoBuild.ABI).
		Public().
		From(recipient2Name).
		To(noto.Address).
		Function("unlock").
		Inputs(unlockReceipt.LockInfo.UnlockParams).
		Send().
		Wait(3 * time.Second)
	require.NoError(t, tx.Error())

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient2Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "100", balanceOfResult["totalBalance"].(string), "Balance of recipient should be 100")

	findAvailableCoins(t, ctx, rpc, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*types.NotoLockedCoinState) bool {
		return len(coins) == 0
	})
	coins = findAvailableCoins(t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*types.NotoCoinState) bool {
		return len(coins) == 2
	})
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key.Verifier.Verifier, coins[0].Data.Owner.String())
	assert.Equal(t, int64(50), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key.Verifier.Verifier, coins[1].Data.Owner.String())
}
