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
	"fmt"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/pkg/testbed"
	"github.com/LFDT-Paladin/paladin/domains/integration-test/helpers"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"
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

	log.L(ctx).Infof("Deploying Noto contracts")
	// Deploy: noto (default impl), noto_v0, factory_impl
	contractSource := map[string][]byte{
		"noto":            helpers.NotoJSON,
		"noto_v0":         helpers.NotoV0JSON,
		"SmtLib":          helpers.SmtLibJSON,
		"noto_nullifiers": helpers.NotoNullifiersJSON,
		"factory_impl":    helpers.NotoFactoryJSON,
	}
	deployOrder := []string{
		"noto",
		"factory_impl",
		"noto_v0",
		"SmtLib",
		"noto_nullifiers",
	}

	// After deploying implementations, deploy proxy and register v0
	deployProxyAndConfigure := func(deployed map[string]string, rpc rpcclient.Client) {
		// Deploy factory proxy with initialize(notoImplAddress)
		proxyAddr := deployFactoryProxy(ctx, s.T(), rpc, notaryName,
			deployed["factory_impl"], helpers.NotoFactoryJSON,
			fmt.Sprintf(`["%s"]`, deployed["noto"]))
		deployed["factory"] = proxyAddr
		log.L(ctx).Infof("Factory proxy deployed to %s", proxyAddr)

		// Register noto_v0 implementation on the factory (via proxy)
		result1 := pldclient.Wrap(rpc).ReceiptPollingInterval(200*time.Millisecond).
			ForABI(ctx, helpers.NotoFactoryABI).
			Public().
			From(notaryName).
			To(pldtypes.MustEthAddress(proxyAddr)).
			Function("registerImplementation").
			Inputs(pldtypes.RawJSON(fmt.Sprintf(`["noto_v0", "%s"]`, deployed["noto_v0"]))).
			BuildTX().
			Send().
			Wait(5 * time.Second)
		require.NoError(s.T(), result1.Error())

		result2 := pldclient.Wrap(rpc).ReceiptPollingInterval(200*time.Millisecond).
			ForABI(ctx, helpers.NotoFactoryABI).
			Public().
			From(notaryName).
			To(pldtypes.MustEthAddress(deployed["factory"])).
			Function("registerImplementation").
			Inputs(pldtypes.RawJSON(fmt.Sprintf(`["noto_nullifiers", "%s"]`, deployed["noto_nullifiers"]))).
			BuildTX().
			Send().
			Wait(5 * time.Second)
		require.NoError(s.T(), result2.Error())
	}
	contracts := deployContracts(ctx, s.T(), s.hdWalletSeed, notaryName, deployOrder, contractSource, deployProxyAndConfigure)
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

func (s *notoTestSuite) TestNotoV1Nullifiers() {
	s.testNoto("v1", "noto_nullifiers")
}

func (s *notoTestSuite) TestNotoV1() {
	s.testNoto("v1", "")
}

func (s *notoTestSuite) TestNotoV0() {
	s.testNoto("v0", "")
}

func (s *notoTestSuite) testNoto(version string, variant string) {
	t := s.T()
	ctx := t.Context()
	log.L(ctx).Infof("TestNoto")

	waitForNoto, notoTestbed := newNotoDomain(t, pldtypes.MustEthAddress(s.factoryAddress))
	done, _, _, _, paladinClient := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.domainName: notoTestbed,
	})
	defer done()

	notoDomain := <-waitForNoto

	notoReceipts := make(chan notoReceiptWithTXID)
	subscribeAndSendNotoReceiptsToChannel(t, paladinClient, notoDomain.Name(), notoReceipts)

	notaryKey, err := paladinClient.PTX().ResolveVerifier(ctx, notaryName, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	recipient1Key, err := paladinClient.PTX().ResolveVerifier(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	recipient2Key, err := paladinClient.PTX().ResolveVerifier(ctx, recipient2Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	log.L(ctx).Infof("Deploying an instance of Noto")
	var noto *helpers.NotoHelper
	if version == "v1" {
		noto = helpers.DeployNoto(ctx, t, paladinClient, s.domainName, variant, notary, nil)
	} else {
		noto = helpers.DeployNotoImplementation(ctx, t, paladinClient, s.domainName, "noto_v0", notary, nil)
	}
	log.L(ctx).Infof("Noto deployed to %s", noto.Address)

	log.L(ctx).Infof("Mint 100 from notary to notary")
	rpcerr := paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "mint",
			Data: toJSON(t, &types.MintParams{
				To:     notaryName,
				Amount: pldtypes.Uint64ToUint256(100),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	mintReceipt := <-notoReceipts
	require.Len(t, mintReceipt.Transfers, 1)
	assert.Equal(t, int64(100), mintReceipt.Transfers[0].Amount.Int().Int64())
	assert.Equal(t, notaryKey, mintReceipt.Transfers[0].To.String())

	// Nullifier-backed contracts record spends on nullifier ids; list spendable coins via queryContractNullifiers. Others use queryContractStates.
	coinQueryMethod := "pstate_queryContractStates"
	if variant == "noto_nullifiers" {
		coinQueryMethod = "pstate_queryContractNullifiers"
	}
	coins := findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), coinQueryMethod, noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(100), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey, coins[0].Data.Owner.String())

	// check balance
	balanceOfResult := noto.BalanceOf(ctx, &types.BalanceOfParam{Account: notaryName}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "100", balanceOfResult["totalBalance"].(string), "Balance of notary should be 100")

	log.L(ctx).Infof("Attempt mint from non-notary (should fail)")
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "mint",
			Data: toJSON(t, &types.MintParams{
				To:     recipient1Name,
				Amount: pldtypes.Uint64ToUint256(100),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NotNil(t, rpcerr)
	assert.ErrorContains(t, rpcerr, "PD200009")

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), coinQueryMethod, noto.Address, nil)
	require.Len(t, coins, 1)

	log.L(ctx).Infof("Transfer 150 from notary (should fail)")
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "transfer",
			Data: toJSON(t, &types.TransferParams{
				To:     recipient1Name,
				Amount: pldtypes.Uint64ToUint256(150),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NotNil(t, rpcerr)
	assert.ErrorContains(t, rpcerr, "assemble result was REVERT")

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), coinQueryMethod, noto.Address, nil)
	require.Len(t, coins, 1)

	log.L(ctx).Infof("Transfer 50 from notary to recipient1")
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "transfer",
			Data: toJSON(t, &types.TransferParams{
				To:     recipient1Name,
				Amount: pldtypes.Uint64ToUint256(50),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	transferReceipt := <-notoReceipts
	require.Len(t, transferReceipt.Transfers, 1)
	assert.Equal(t, int64(50), transferReceipt.Transfers[0].Amount.Int().Int64())
	assert.Equal(t, recipient1Key, transferReceipt.Transfers[0].To.String())

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), coinQueryMethod, noto.Address, nil)
	require.Len(t, coins, 2)

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: notaryName}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of notary should be 50")

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should be 50")

	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key, coins[0].Data.Owner.String())
	assert.Equal(t, int64(50), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey, coins[1].Data.Owner.String())

	log.L(ctx).Infof("Transfer 50 from recipient1 to recipient2")
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "transfer",
			Data: toJSON(t, &types.TransferParams{
				To:     recipient2Name,
				Amount: pldtypes.Uint64ToUint256(50),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	transferReceipt = <-notoReceipts
	require.Len(t, transferReceipt.Transfers, 1)
	assert.Equal(t, int64(50), transferReceipt.Transfers[0].Amount.Int().Int64())
	assert.Equal(t, recipient2Key, transferReceipt.Transfers[0].To.String())

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), coinQueryMethod, noto.Address, nil)
	require.Len(t, coins, 2)

	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey, coins[0].Data.Owner.String())
	assert.Equal(t, int64(50), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key, coins[1].Data.Owner.String())

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "0", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should be 0")

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient2Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient2 should be 50")

	log.L(ctx).Infof("Burn 25 from recipient2")
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient2Name,
			To:       noto.Address,
			Function: "burn",
			Data: toJSON(t, &types.BurnParams{
				Amount: pldtypes.Uint64ToUint256(25),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	burnReceipt := <-notoReceipts
	require.Len(t, burnReceipt.Transfers, 1)
	assert.Equal(t, int64(25), burnReceipt.Transfers[0].Amount.Int().Int64())
	assert.Equal(t, recipient2Key, burnReceipt.Transfers[0].From.String())
	assert.Nil(t, burnReceipt.Transfers[0].To)

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), coinQueryMethod, noto.Address, nil)
	require.Len(t, coins, 2)

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient2Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "25", balanceOfResult["totalBalance"].(string), "Balance of recipient should be 25")

	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, notaryKey, coins[0].Data.Owner.String())
	assert.Equal(t, int64(25), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key, coins[1].Data.Owner.String())
}

func (s *notoTestSuite) TestNotoLockV1() {
	s.testNotoLock("v1", "")
}

func (s *notoTestSuite) TestNotoLockV1Nullifiers() {
	s.testNotoLock("v1", "noto_nullifiers")
}

func (s *notoTestSuite) TestNotoLockV0() {
	s.testNotoLock("v0", "")
}

func (s *notoTestSuite) testNotoLock(version, variant string) {
	t := s.T()
	ctx := t.Context()
	log.L(ctx).Infof("TestNotoLock")

	waitForNoto, notoTestbed := newNotoDomain(t, pldtypes.MustEthAddress(s.factoryAddress))
	done, _, _, _, paladinClient := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.domainName: notoTestbed,
	})
	defer done()

	notoDomain := <-waitForNoto

	notoReceipts := make(chan notoReceiptWithTXID)
	subscribeAndSendNotoReceiptsToChannel(t, paladinClient, notoDomain.Name(), notoReceipts)

	recipient1Key, err := paladinClient.PTX().ResolveVerifier(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	recipient2Key, err := paladinClient.PTX().ResolveVerifier(ctx, recipient2Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	log.L(ctx).Infof("Deploying an instance of Noto")
	var noto *helpers.NotoHelper
	if version == "v1" {
		noto = helpers.DeployNoto(ctx, t, paladinClient, s.domainName, variant, notary, nil)
	} else {
		noto = helpers.DeployNotoImplementation(ctx, t, paladinClient, s.domainName, "noto_v0", notary, nil)
	}
	log.L(ctx).Infof("Noto deployed to %s", noto.Address)

	log.L(ctx).Infof("Mint 100 from notary to recipient1")
	rpcerr := paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "mint",
			Data: toJSON(t, &types.MintParams{
				To:     recipient1Name,
				Amount: pldtypes.Uint64ToUint256(100),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	mintReceipt := <-notoReceipts
	require.Len(t, mintReceipt.Transfers, 1)
	assert.Equal(t, int64(100), mintReceipt.Transfers[0].Amount.Int().Int64())
	assert.Equal(t, recipient1Key, mintReceipt.Transfers[0].To.String())

	// Nullifier-backed contracts record spends of unlocked coins on nullifier ids, so spendable
	// unlocked coins are listed via queryContractNullifiers. Locked coins are always spent by
	// state id, in every variant, so they are always listed via queryContractStates.
	coinQueryMethod := "pstate_queryContractStates"
	if variant == "noto_nullifiers" {
		coinQueryMethod = "pstate_queryContractNullifiers"
	}

	coins := findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), coinQueryMethod, noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(100), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key, coins[0].Data.Owner.String())

	log.L(ctx).Infof("Lock 50 from recipient1")
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "lock",
			Data: toJSON(t, &types.LockParams{
				Amount: pldtypes.Uint64ToUint256(50),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	lockReceipt := <-notoReceipts
	require.NotNil(t, lockReceipt.LockInfo)
	require.NotEmpty(t, lockReceipt.LockInfo.LockID)

	balanceOfResult := noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient should be 50")

	lockedCoins := findAvailableCoins[types.NotoLockedCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, lockedCoins, 1)
	assert.Equal(t, int64(50), lockedCoins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key, lockedCoins[0].Data.Owner.String())
	coins = findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), coinQueryMethod, noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key, coins[0].Data.Owner.String())

	log.L(ctx).Infof("Transfer 50 from recipient1 to recipient2 (succeeds but does not use locked state)")
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "transfer",
			Data: toJSON(t, &types.TransferParams{
				To:     recipient2Name,
				Amount: pldtypes.Uint64ToUint256(50),
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	transferReceipt := <-notoReceipts
	require.Len(t, transferReceipt.Transfers, 1)
	assert.Equal(t, int64(50), transferReceipt.Transfers[0].Amount.Int().Int64())
	assert.Equal(t, recipient2Key, transferReceipt.Transfers[0].To.String())

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "0", balanceOfResult["totalBalance"].(string), "Balance of recipient should be 0")

	lockedCoins = findAvailableCoins[types.NotoLockedCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, lockedCoins, 1)
	assert.Equal(t, int64(50), lockedCoins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key, lockedCoins[0].Data.Owner.String())
	coins = findAvailableCoins[types.NotoCoinState](t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), coinQueryMethod, noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key, coins[0].Data.Owner.String())

	log.L(ctx).Infof("Prepare unlock that will send all 50 to recipient2")
	prepareTxData := pldtypes.RandBytes(16)
	unlockTxData := pldtypes.RandBytes(16)
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "prepareUnlock",
			Data: toJSON(t, &types.PrepareUnlockParams{
				UnlockParams: types.UnlockParams{
					LockID: lockReceipt.LockInfo.LockID,
					From:   recipient1Name,
					Recipients: []*types.UnlockRecipient{{
						To:     recipient2Name,
						Amount: pldtypes.Uint64ToUint256(50),
					}},
					Data: prepareTxData,
				},
				UnlockData: unlockTxData,
			}),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	prepareUnlockReceipt := <-notoReceipts

	require.NotEmpty(t, prepareUnlockReceipt.LockInfo)
	require.NotEmpty(t, prepareUnlockReceipt.LockInfo.UnlockParams)
	require.NotEmpty(t, prepareUnlockReceipt.States.ReadLockedInputs)
	var unlockTXID uuid.UUID
	if version == "v0" {
		require.NotEmpty(t, prepareUnlockReceipt.LockInfo.UnlockParams["txId"])
	} else {
		require.NotEmpty(t, prepareUnlockReceipt.LockInfo.UnlockParams["lockId"])
		require.NotEmpty(t, prepareUnlockReceipt.LockInfo.SpendTxId)
		unlockTXID = pldtypes.MustParseBytes32(prepareUnlockReceipt.LockInfo.SpendTxId.HexString0xPrefix()).UUIDFirst16()
	}

	log.L(ctx).Infof("Delegate lock to recipient2")
	delegateLockParams := &types.DelegateLockParams{
		LockID:   prepareUnlockReceipt.LockInfo.LockID,
		Delegate: pldtypes.MustEthAddress(recipient2Key),
	}
	delegateLockABI := types.NotoABI
	if version == "v0" {
		var unlockParams types.UnlockPublicParams
		err = json.Unmarshal(toJSON(t, prepareUnlockReceipt.LockInfo.UnlockParams), &unlockParams)
		require.NoError(t, err)
		delegateLockParams.Unlock = &unlockParams
		delegateLockABI = types.NotoV0ABI
	}
	rpcerr = paladinClient.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "delegateLock",
			Data:     toJSON(t, delegateLockParams),
		},
		ABI: delegateLockABI,
	}, false)
	require.NoError(t, rpcerr)

	delegateLockReceipt := <-notoReceipts
	require.NotEmpty(t, delegateLockReceipt.LockInfo)
	assert.Equal(t, prepareUnlockReceipt.LockInfo.LockID, delegateLockReceipt.LockInfo.LockID)
	assert.Equal(t, recipient2Key, delegateLockReceipt.LockInfo.Delegate.String())

	log.L(ctx).Infof("Unlock from recipient2")
	var notoBuild *solutils.SolidityBuild
	if version == "v1" {
		notoBuild = solutils.MustLoadBuild(helpers.NotoInterfaceJSON)
		spendArgs, err := types.NotoSpendLockArgsABI.DecodeABIData(pldtypes.MustParseHexBytes(prepareUnlockReceipt.LockInfo.UnlockParams["spendArgs"].(string)), 0)
		require.NoError(t, err)
		spendArgsJSON, err := spendArgs.Children[0].JSON()
		require.NoError(t, err)
		log.L(ctx).Infof("Test unlocking %s with spendArgs: %s", prepareUnlockReceipt.LockInfo.LockID, spendArgsJSON)
	} else {
		notoBuild = solutils.MustLoadBuild(helpers.NotoV0InterfaceJSON)
	}
	tx := paladinClient.ForABI(ctx, notoBuild.ABI).
		Public().
		From(recipient2Name).
		To(noto.Address).
		Function(prepareUnlockReceipt.LockInfo.UnlockFunction).
		Inputs(prepareUnlockReceipt.LockInfo.UnlockParams).
		Send().
		Wait(3 * time.Second)
	require.NoError(t, tx.Error())

	unlockReceipt := <-notoReceipts
	if version == "v1" {
		require.Equal(t, unlockReceipt.txID, unlockTXID)
	}
	require.Len(t, unlockReceipt.Transfers, 1)
	assert.Equal(t, int64(50), unlockReceipt.Transfers[0].Amount.Int().Int64())
	assert.Equal(t, recipient2Key, unlockReceipt.Transfers[0].To.String())

	// Wait for locked coins to be consumed and new coins to be created
	findAvailableCoins(t, ctx, paladinClient, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*types.NotoLockedCoinState) bool {
		return len(coins) == 0
	})
	coins = findAvailableCoins(t, ctx, paladinClient, notoDomain.Name(), notoDomain.CoinSchemaID(), coinQueryMethod, noto.Address, nil, func(coins []*types.NotoCoinState) bool {
		return len(coins) == 2
	})

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient2Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "100", balanceOfResult["totalBalance"].(string), "Balance of recipient should be 100")
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key, coins[0].Data.Owner.String())
	assert.Equal(t, int64(50), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, recipient2Key, coins[1].Data.Owner.String())
}

type notoReceiptWithTXID struct {
	types.NotoDomainReceipt
	txID uuid.UUID
}

func subscribeAndSendNotoReceiptsToChannel(t *testing.T, wsClient pldclient.PaladinWSClient, domainName string, receipts chan notoReceiptWithTXID) {
	ctx := t.Context()

	privateType := pldtypes.Enum[pldapi.TransactionType](pldapi.TransactionTypePrivate)
	listenerName := fmt.Sprintf("listener-%s", domainName)
	_, err := wsClient.PTX().CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: listenerName,
		Filters: pldapi.TransactionReceiptFilters{
			Type:   &privateType,
			Domain: domainName,
		},
		Options: pldapi.TransactionReceiptListenerOptions{
			DomainReceipts: true,
		},
	})
	require.NoError(t, err)

	sub, err := wsClient.PTX().SubscribeReceipts(ctx, listenerName)
	require.NoError(t, err)
	go func() {
		// No test assertions in this routine, if there's an error, no receipts are sent and the test will fail
		for {
			select {
			case subNotification, ok := <-sub.Notifications():
				if ok {
					notoReceipts := make([]notoReceiptWithTXID, 0)
					var batch pldapi.TransactionReceiptBatch
					_ = json.Unmarshal(subNotification.GetResult(), &batch)
					for _, r := range batch.Receipts {
						if r.DomainReceipt == nil {
							continue
						}
						var notoReceipt types.NotoDomainReceipt
						err = json.Unmarshal(r.DomainReceipt, &notoReceipt)
						if err == nil {
							notoReceipts = append(notoReceipts, notoReceiptWithTXID{
								NotoDomainReceipt: notoReceipt,
								txID:              r.ID,
							})
						}
					}
					_ = subNotification.Ack(ctx)
					// send after the ack otherwise the main test can complete when it receives the last values and the websocket is closed before the ack
					// can be sent
					for _, n := range notoReceipts {
						receipts <- n
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (s *notoTestSuite) TestNotoCreateMintLock() {
	ctx := context.Background()
	t := s.T()
	log.L(ctx).Infof("TestNotoCreateMintLock")

	waitForNoto, notoTestbed := newNotoDomain(t, pldtypes.MustEthAddress(s.factoryAddress))
	done, _, _, rpc, pld := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.domainName: notoTestbed,
	})
	defer done()

	notoDomain := <-waitForNoto

	recipient1Key, err := pld.PTX().ResolveVerifier(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	log.L(ctx).Infof("Deploying an instance of Noto")
	noto := helpers.DeployNoto(ctx, t, rpc, s.domainName, "", notary, nil)
	log.L(ctx).Infof("Noto deployed to %s", noto.Address)

	balanceOfResult := noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "0", balanceOfResult["totalBalance"].(string))

	log.L(ctx).Infof("Create mint lock for 50 to recipient1")
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
						Amount: pldtypes.Uint64ToUint256(50),
					},
				},
				Data: pldtypes.HexBytes{},
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	var createMintLockReceipt types.NotoDomainReceipt
	err = json.Unmarshal(invokeResult.DomainReceipt, &createMintLockReceipt)
	require.NoError(t, err)
	require.NotNil(t, createMintLockReceipt.LockInfo)
	require.NotEmpty(t, createMintLockReceipt.LockInfo.LockID)
	require.NotNil(t, createMintLockReceipt.LockInfo.UnlockParams)
	require.Equal(t, "spendLock", createMintLockReceipt.LockInfo.UnlockFunction)

	log.L(ctx).Infof("Delegate lock to recipient1 so they can spend directly")
	delegateLockParams := &types.DelegateLockParams{
		LockID:   createMintLockReceipt.LockInfo.LockID,
		Delegate: pldtypes.MustEthAddress(recipient1Key),
	}
	rpcerr = pld.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "delegateLock",
			Data:     toJSON(t, delegateLockParams),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	log.L(ctx).Infof("Spend lock to mint coins to recipient1")
	notoBuild := solutils.MustLoadBuild(helpers.NotoInterfaceJSON)
	tx := pld.ForABI(ctx, notoBuild.ABI).
		Public().
		From(recipient1Name).
		To(noto.Address).
		Function(createMintLockReceipt.LockInfo.UnlockFunction).
		Inputs(createMintLockReceipt.LockInfo.UnlockParams).
		Send().
		Wait(3 * time.Second)
	require.NoError(t, tx.Error())

	coins := findAvailableCoins(t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*types.NotoCoinState) bool {
		return len(coins) == 1
	})
	require.Len(t, coins, 1)
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key, coins[0].Data.Owner.String())

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string))
}

func (s *notoTestSuite) TestNotoCreateBurnLock() {
	ctx := context.Background()
	t := s.T()
	log.L(ctx).Infof("TestNotoCreateBurnLock")

	waitForNoto, notoTestbed := newNotoDomain(t, pldtypes.MustEthAddress(s.factoryAddress))
	done, _, _, rpc, pld := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.domainName: notoTestbed,
	})
	defer done()

	notoDomain := <-waitForNoto

	recipient1Key, err := pld.PTX().ResolveVerifier(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	log.L(ctx).Infof("Deploying an instance of Noto")
	noto := helpers.DeployNoto(ctx, t, rpc, s.domainName, "", notary, nil)
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
				Amount: pldtypes.Uint64ToUint256(100),
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	coins := findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(100), coins[0].Data.Amount.Int().Int64())

	balanceOfResult := noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "100", balanceOfResult["totalBalance"].(string))

	log.L(ctx).Infof("Create burn lock for 50 from recipient1")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "createBurnLock",
			Data: toJSON(t, &types.CreateBurnLockParams{
				From:   recipient1Name,
				Amount: pldtypes.Uint64ToUint256(50),
				Data:   pldtypes.HexBytes{},
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	var createBurnLockReceipt types.NotoDomainReceipt
	err = json.Unmarshal(invokeResult.DomainReceipt, &createBurnLockReceipt)
	require.NoError(t, err)
	require.NotNil(t, createBurnLockReceipt.LockInfo)
	require.NotEmpty(t, createBurnLockReceipt.LockInfo.LockID)
	require.NotNil(t, createBurnLockReceipt.LockInfo.UnlockParams)
	require.Equal(t, "spendLock", createBurnLockReceipt.LockInfo.UnlockFunction)

	lockedCoins := findAvailableCoins[types.NotoLockedCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, lockedCoins, 1)
	assert.Equal(t, int64(50), lockedCoins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key, lockedCoins[0].Data.Owner.String())

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string))

	log.L(ctx).Infof("Delegate lock to recipient1 so they can spend directly")
	delegateLockParams := &types.DelegateLockParams{
		LockID:   createBurnLockReceipt.LockInfo.LockID,
		Delegate: pldtypes.MustEthAddress(recipient1Key),
	}
	rpcerr = pld.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "delegateLock",
			Data:     toJSON(t, delegateLockParams),
		},
		ABI: types.NotoABI,
	}, false)
	require.NoError(t, rpcerr)

	log.L(ctx).Infof("Spend lock to burn coins from recipient1")
	notoBuild := solutils.MustLoadBuild(helpers.NotoInterfaceJSON)
	tx := pld.ForABI(ctx, notoBuild.ABI).
		Public().
		From(recipient1Name).
		To(noto.Address).
		Function(createBurnLockReceipt.LockInfo.UnlockFunction).
		Inputs(createBurnLockReceipt.LockInfo.UnlockParams).
		Send().
		Wait(3 * time.Second)
	require.NoError(t, tx.Error())

	coins = findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1)
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())

	findAvailableCoins(t, ctx, rpc, notoDomain.Name(), notoDomain.LockedCoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*types.NotoLockedCoinState) bool {
		return len(coins) == 0
	})

	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string))
}

func (s *notoTestSuite) TestNotoPrepareMintUnlock() {

	ctx := context.Background()
	t := s.T()
	log.L(ctx).Infof("TestNotoPrepareMintUnlock")

	waitForNoto, notoTestbed := newNotoDomain(t, pldtypes.MustEthAddress(s.factoryAddress))
	done, _, _, rpc, pld := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.domainName: notoTestbed,
	})
	defer done()

	notoDomain := <-waitForNoto

	recipient1Key, err := pld.PTX().ResolveVerifier(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	log.L(ctx).Infof("Deploying an instance of Noto")
	noto := helpers.DeployNoto(ctx, t, rpc, s.domainName, "", notary, nil)
	log.L(ctx).Infof("Noto deployed to %s", noto.Address)

	log.L(ctx).Infof("Lock (0 value) from recipient1")
	var invokeResult testbed.TransactionResult
	rpcerr := rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "lock",
			Data: toJSON(t, &types.LockParams{
				Amount: pldtypes.Uint64ToUint256(0),
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
	require.Len(t, lockedCoins, 0)

	balanceOfResult := noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "0", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should be 0")

	log.L(ctx).Infof("Prepare mint unlock for the locked 50")
	rpcerr = rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "prepareMintUnlock",
			Data: toJSON(t, &types.PrepareMintUnlockParams{
				LockID: lockReceipt.LockInfo.LockID,
				Recipients: []*types.UnlockRecipient{
					{
						To:     recipient1Name,
						Amount: pldtypes.Uint64ToUint256(50),
					},
				},
				Data: pldtypes.HexBytes{},
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	var prepareMintUnlockReceipt types.NotoDomainReceipt
	err = json.Unmarshal(invokeResult.DomainReceipt, &prepareMintUnlockReceipt)
	require.NoError(t, err)
	require.NotNil(t, prepareMintUnlockReceipt.LockInfo)
	require.NotNil(t, prepareMintUnlockReceipt.LockInfo.UnlockParams)

	log.L(ctx).Infof("Delegate lock to recipient1")
	delegateLockParams := &types.DelegateLockParams{
		LockID:   prepareMintUnlockReceipt.LockInfo.LockID,
		Delegate: pldtypes.MustEthAddress(recipient1Key), // myself - otherwise only the notary can unlock
	}
	delegateLockABI := types.NotoABI
	rpcerr = pld.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     notaryName,
			To:       noto.Address,
			Function: "delegateLock",
			Data:     toJSON(t, delegateLockParams),
		},
		ABI: delegateLockABI,
	}, false)
	require.NoError(t, rpcerr)

	log.L(ctx).Infof("Unlock from notary")
	notoBuild := solutils.MustLoadBuild(helpers.NotoInterfaceJSON)
	spendArgs, err := types.NotoSpendLockArgsABI.DecodeABIData(pldtypes.MustParseHexBytes(prepareMintUnlockReceipt.LockInfo.UnlockParams["spendArgs"].(string)), 0)
	require.NoError(t, err)
	spendArgsJSON, err := spendArgs.Children[0].JSON()
	require.NoError(t, err)
	log.L(ctx).Infof("Test unlocking %s with spendArgs: %s", prepareMintUnlockReceipt.LockInfo.LockID, spendArgsJSON)
	tx := pld.ForABI(ctx, notoBuild.ABI).
		Public().
		From(recipient1Name).
		To(noto.Address).
		Function(prepareMintUnlockReceipt.LockInfo.UnlockFunction).
		Inputs(prepareMintUnlockReceipt.LockInfo.UnlockParams).
		Send().
		Wait(3 * time.Second)
	require.NoError(t, tx.Error())

	// Verify the new coin
	coins := findAvailableCoins(t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*types.NotoCoinState) bool {
		return len(coins) == 1
	})
	require.Len(t, coins, 1)
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, recipient1Key, coins[0].Data.Owner.String())

	// Checking the balance
	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should be 50")

}

func (s *notoTestSuite) TestNotoPrepareBurnUnlock() {
	ctx := context.Background()
	t := s.T()
	log.L(ctx).Infof("TestNotoPrepareBurnUnlock")

	waitForNoto, notoTestbed := newNotoDomain(t, pldtypes.MustEthAddress(s.factoryAddress))
	done, _, tb, rpc, _ := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.domainName: notoTestbed,
	})
	defer done()
	pld := helpers.NewPaladinClient(t, ctx, tb)

	notoDomain := <-waitForNoto

	recipient1Key, err := pld.PTX().ResolveVerifier(ctx, recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	log.L(ctx).Infof("Deploying an instance of Noto")
	noto := helpers.DeployNoto(ctx, t, rpc, s.domainName, "", notary, nil)
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
				Amount: pldtypes.Uint64ToUint256(100),
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
				Amount: pldtypes.Uint64ToUint256(50),
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
	assert.Equal(t, recipient1Key, lockedCoins[0].Data.Owner.String())

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
				Amount: pldtypes.Uint64ToUint256(50),
				Data:   pldtypes.HexBytes{},
			}),
		},
		ABI: types.NotoABI,
	}, true)
	require.NoError(t, rpcerr)

	var prepareBurnUnlockReceipt types.NotoDomainReceipt
	err = json.Unmarshal(invokeResult.DomainReceipt, &prepareBurnUnlockReceipt)
	require.NoError(t, err)
	require.NotNil(t, prepareBurnUnlockReceipt.LockInfo)
	require.NotNil(t, prepareBurnUnlockReceipt.LockInfo.UnlockParams)

	log.L(ctx).Infof("Delegate lock to recipient1")
	delegateLockParams := &types.DelegateLockParams{
		LockID:   prepareBurnUnlockReceipt.LockInfo.LockID,
		Delegate: pldtypes.MustEthAddress(recipient1Key), // myself - otherwise only the notary can unlock
	}
	delegateLockABI := types.NotoABI
	rpcerr = pld.CallRPC(ctx, nil, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     recipient1Name,
			To:       noto.Address,
			Function: "delegateLock",
			Data:     toJSON(t, delegateLockParams),
		},
		ABI: delegateLockABI,
	}, false)
	require.NoError(t, rpcerr)

	log.L(ctx).Infof("Unlock from recipient1")
	notoBuild := solutils.MustLoadBuild(helpers.NotoInterfaceJSON)
	spendArgs, err := types.NotoSpendLockArgsABI.DecodeABIData(pldtypes.MustParseHexBytes(prepareBurnUnlockReceipt.LockInfo.UnlockParams["spendArgs"].(string)), 0)
	require.NoError(t, err)
	spendArgsJSON, err := spendArgs.Children[0].JSON()
	require.NoError(t, err)
	log.L(ctx).Infof("Test unlocking %s with spendArgs: %s", prepareBurnUnlockReceipt.LockInfo.LockID, spendArgsJSON)
	tx := pld.ForABI(ctx, notoBuild.ABI).
		Public().
		From(recipient1Name).
		To(noto.Address).
		Function(prepareBurnUnlockReceipt.LockInfo.UnlockFunction).
		Inputs(prepareBurnUnlockReceipt.LockInfo.UnlockParams).
		Send().
		Wait(3 * time.Second)
	require.NoError(t, tx.Error())

	// Verify no new coins were created (the locked value was burned)
	coins = findAvailableCoins[types.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.Len(t, coins, 1, "Should still have only 1 coin (the original 50)")
	assert.Equal(t, int64(50), coins[0].Data.Amount.Int().Int64(), "The remaining coin should still be 50")

	// Verify final balance (should still be 50, as the locked 50 was burned)
	balanceOfResult = noto.BalanceOf(ctx, &types.BalanceOfParam{Account: recipient1Name}).SignAndCall(notaryName).Wait()
	assert.Equal(t, "50", balanceOfResult["totalBalance"].(string), "Balance of recipient1 should still be 50 after burn")
}
