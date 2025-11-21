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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/pkg/testbed"
	"github.com/LFDT-Paladin/paladin/domains/integration-test/helpers"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
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

	waitForNoto, notoTestbed := newNotoDomain(t, pldtypes.MustEthAddress(s.factoryAddress))
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

func (s *notoTestSuite) TestNotoLock() {
	ctx := context.Background()
	t := s.T()
	log.L(ctx).Infof("TestNotoLock")

	waitForNoto, notoTestbed := newNotoDomain(t, pldtypes.MustEthAddress(s.factoryAddress))
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

	// Wait for locked coins to be consumed and new coins to be created
	findAvailableCoins(t, ctx, rpc, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*types.NotoLockedCoinState) bool {
		return len(coins) == 0
	})
	coins = findAvailableCoins(t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*types.NotoCoinState) bool {
		return len(coins) == 2
	})

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient2Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "100", balanceOfResult["totalBalance"].(string), "Balance of recipient should be 100")
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key.Verifier.Verifier, coins[0].Data.Owner.String())
	assert.Equal(t, int64(50), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key.Verifier.Verifier, coins[1].Data.Owner.String())
}

func (s *notoTestSuite) TestNotoCreateMintLock() {
	ctx := context.Background()
	t := s.T()
	log.L(ctx).Infof("TestNotoCreateMintLock")

	waitForNoto, notoTestbed := newNotoDomain(t, pldtypes.MustEthAddress(s.factoryAddress))
	done, _, tb, rpc := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.domainName: notoTestbed,
	})
	defer done()
	pld := helpers.NewPaladinClient(t, ctx, tb)

	notoDomain := <-waitForNoto

	log.L(ctx).Infof("Deploying an instance of Noto")
	noto := helpers.DeployNoto(ctx, t, rpc, s.domainName, notary, nil)
	log.L(ctx).Infof("Noto deployed to %s", noto.Address)

	log.L(ctx).Infof("Create mint lock with 100 to recipient1 and 50 to recipient2")
	var invokeResult testbed.TransactionResult
	rpcerr := rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "createMintLock",
			Data: toJSON(t, &types.CreateMintLockParams{
				Recipients: []*types.UnlockRecipient{
					{
						To:     recipient1Name,
						Amount: pldtypes.Int64ToInt256(100),
					},
					{
						To:     recipient2Name,
						Amount: pldtypes.Int64ToInt256(50),
					},
				},
				Data: pldtypes.HexBytes{},
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	var mintLockReceipt types.NotoDomainReceipt
	err := json.Unmarshal(invokeResult.DomainReceipt, &mintLockReceipt)
	require.NoError(t, err)
	require.NotNil(t, mintLockReceipt.LockInfo)
	require.NotEmpty(t, mintLockReceipt.LockInfo.LockID)

	// Verify no coins exist yet
	coins := findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 0)
	lockedCoins := findAvailableCoins[types.NotoLockedCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, lockedCoins, 0)

	// Verify balances are 0
	balanceOfResult := noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "0", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should be 0 before unlock")
	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient2Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "0", balanceOfResult["totalBalance"].(string), "Balance of recipient2 should be 0 before unlock")

	// Verify unlock params structure
	require.NotNil(t, mintLockReceipt.LockInfo.UnlockParams)
	unlockParamsMap := mintLockReceipt.LockInfo.UnlockParams
	paramsMap, ok := unlockParamsMap["params"].(map[string]any)
	require.True(t, ok, "params should be a map")
	lockedInputs, ok := paramsMap["lockedInputs"].([]any)
	require.True(t, ok, "lockedInputs should be an array")
	assert.Equal(t, 0, len(lockedInputs), "createMintLock should have no lockedInputs")
	outputs, ok := paramsMap["outputs"].([]any)
	require.True(t, ok, "outputs should be an array")
	assert.Equal(t, 2, len(outputs), "createMintLock should have 2 outputs")

	// Perform the unlock using the prepared unlock params
	log.L(ctx).Infof("Unlock the mint lock")
	notoBuild := solutils.MustLoadBuild(helpers.NotoInterfaceJSON)
	tx := pld.ForABI(ctx, notoBuild.ABI).
		Public().
		From(notaryName).
		To(noto.Address).
		Function("unlock").
		Inputs(mintLockReceipt.LockInfo.UnlockParams).
		Send().
		Wait(3 * time.Second)
	require.NoError(t, tx.Error())

	// Wait for coins to be created
	coins = findAvailableCoins(t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*types.NotoCoinState) bool {
		return len(coins) == 2
	})
	require.Len(t, coins, 2, "Should have 2 coins after unlock")

	// Verify coin amounts and owners
	var recipient1Coin, recipient2Coin *types.NotoCoinState
	recipient1Key, err := tb.ResolveKey(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	recipient2Key, err := tb.ResolveKey(ctx, recipient2Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	for _, coin := range coins {
		if coin.Data.Owner.String() == recipient1Key.Verifier.Verifier {
			recipient1Coin = coin
		} else if coin.Data.Owner.String() == recipient2Key.Verifier.Verifier {
			recipient2Coin = coin
		}
	}
	require.NotNil(t, recipient1Coin, "Should have coin for recipient1")
	require.NotNil(t, recipient2Coin, "Should have coin for recipient2")
	assert.Equal(t, int64(100), recipient1Coin.Data.Amount.Int().Int64(), "Recipient1 should have 100")
	assert.Equal(t, int64(50), recipient2Coin.Data.Amount.Int().Int64(), "Recipient2 should have 50")

	// Verify final balances
	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "100", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should be 100 after unlock")
	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient2Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient2 should be 50 after unlock")
}

func (s *notoTestSuite) TestNotoPrepareBurnUnlock() {
	ctx := context.Background()
	t := s.T()
	log.L(ctx).Infof("TestNotoPrepareBurnUnlock")

	waitForNoto, notoTestbed := newNotoDomain(t, pldtypes.MustEthAddress(s.factoryAddress))
	done, _, tb, rpc := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.domainName: notoTestbed,
	})
	defer done()
	pld := helpers.NewPaladinClient(t, ctx, tb)

	notoDomain := <-waitForNoto

	recipient1Key, err := tb.ResolveKey(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
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

	balanceOfResult := noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should be 50")

	log.L(ctx).Infof("Prepare burn unlock for the locked 50")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "prepareBurnUnlock",
			Data: toJSON(t, &types.PrepareBurnUnlockParams{
				LockID: lockReceipt.LockInfo.LockID,
				From:   recipient1Name,
				Amount: pldtypes.Int64ToInt256(50),
				Data:   pldtypes.HexBytes{},
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	var burnUnlockReceipt types.NotoDomainReceipt
	err = json.Unmarshal(invokeResult.DomainReceipt, &burnUnlockReceipt)
	require.NoError(t, err)
	require.NotNil(t, burnUnlockReceipt.LockInfo)
	require.NotNil(t, burnUnlockReceipt.LockInfo.UnlockParams)

	// Verify unlock params structure
	unlockParamsMap := burnUnlockReceipt.LockInfo.UnlockParams
	paramsMap, ok := unlockParamsMap["params"].(map[string]any)
	require.True(t, ok, "params should be a map")
	outputs, ok := paramsMap["outputs"].([]any)
	require.True(t, ok, "outputs should be an array")
	assert.Equal(t, 0, len(outputs), "prepareBurnUnlock should have no outputs")

	// Perform the unlock using the prepared unlock params (this should burn the locked value)
	log.L(ctx).Infof("Unlock the burn lock (should burn the locked value)")
	notoBuild := solutils.MustLoadBuild(helpers.NotoInterfaceJSON)
	tx := pld.ForABI(ctx, notoBuild.ABI).
		Public().
		From(notaryName).
		To(noto.Address).
		Function("unlock").
		Inputs(burnUnlockReceipt.LockInfo.UnlockParams).
		Send().
		Wait(3 * time.Second)
	require.NoError(t, tx.Error())

	// Wait for locked coins to be consumed (burned)
	lockedCoins = findAvailableCoins(t, ctx, rpc, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*types.NotoLockedCoinState) bool {
		return len(coins) == 0
	})
	require.Len(t, lockedCoins, 0, "Should have no locked coins after burn unlock")

	// Verify no new coins were created (the locked value was burned)
	coins = findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1, "Should still have only 1 coin (the original 50)")
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64(), "The remaining coin should still be 50")

	// Verify final balance (should still be 50, as the locked 50 was burned)
	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should still be 50 after burn")
}
