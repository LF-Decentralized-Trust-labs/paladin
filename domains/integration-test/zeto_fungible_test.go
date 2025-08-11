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
	_ "embed"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/integration-test/helpers"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/constants"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestFungibleZetoDomainTestSuite(t *testing.T) {
	contractsFile = "./zeto/config-for-deploy-fungible.yaml"
	suite.Run(t, new(fungibleTestSuiteHelper))
}

type fungibleTestSuiteHelper struct {
	zetoDomainTestSuite
}

func (s *fungibleTestSuiteHelper) TestZeto_Anon() {
	s.testZeto(s.T(), constants.TOKEN_ANON, false, false)
}

func (s *fungibleTestSuiteHelper) TestZeto_AnonBatch() {
	s.testZeto(s.T(), constants.TOKEN_ANON, true, false)
}

func (s *fungibleTestSuiteHelper) TestZeto_AnonEnc() {
	s.testZeto(s.T(), constants.TOKEN_ANON_ENC, false, false)
}

func (s *fungibleTestSuiteHelper) TestZeto_AnonEncBatch() {
	s.testZeto(s.T(), constants.TOKEN_ANON_ENC, true, false)
}

func (s *fungibleTestSuiteHelper) TestZeto_AnonNullifier() {
	s.testZeto(s.T(), constants.TOKEN_ANON_NULLIFIER, false, true)
}

func (s *fungibleTestSuiteHelper) TestZeto_AnonNullifierBatch() {
	s.testZeto(s.T(), constants.TOKEN_ANON_NULLIFIER, true, true)
}

func (s *fungibleTestSuiteHelper) TestZeto_AnonNullifierKyc() {
	s.testZeto(s.T(), constants.TOKEN_ANON_NULLIFIER_KYC, false, true, true)
}

func (s *fungibleTestSuiteHelper) TestZeto_AnonNullifierKycBatch() {
	s.testZeto(s.T(), constants.TOKEN_ANON_NULLIFIER_KYC, true, true, true)
}

func (s *fungibleTestSuiteHelper) testZeto(t *testing.T, tokenName string, useBatch bool, isNullifiersToken bool, isKycToken ...bool) {
	ctx := context.Background()
	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Deploying an instance of the %s token", tokenName)
	log.L(ctx).Info("*************************************")
	s.setupContractsAbi(t, ctx, tokenName)

	zeto := helpers.DeployZetoFungible(ctx, t, s.rpc, s.domainName, controllerName, tokenName)
	zetoAddress := zeto.Address
	log.L(ctx).Infof("Zeto instance deployed to %s", zetoAddress.String())

	var controllerEthAddr string
	rpcerr := s.rpc.CallRPC(ctx, &controllerEthAddr, "ptx_resolveVerifier", controllerName, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.Nil(t, rpcerr)

	log.L(ctx).Infof("Deploying the sample ERC20 with initialOwner %s", controllerEthAddr)
	erc20Address, err := helpers.DeployERC20(ctx, s.rpc, controllerName, controllerEthAddr)
	require.NoError(t, err)

	log.L(ctx).Infof("Setting the ERC20 contract (%s) to the Zeto instance", erc20Address)
	zeto.SetERC20(ctx, s.tb, controllerName, erc20Address)

	var controllerAddr pldtypes.Bytes32
	rpcerr = s.rpc.CallRPC(ctx, &controllerAddr, "ptx_resolveVerifier", controllerName, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	require.Nil(t, rpcerr)

	if len(isKycToken) > 0 && isKycToken[0] {
		log.L(ctx).Infof("Registering participant %s in the KYC registry (pubKey=%s)", controllerName, controllerAddr.String())
		pubKey, err := zetosigner.DecodeBabyJubJubPublicKey(controllerAddr.HexString())
		require.NoError(t, err)
		zeto.Register(ctx, s.tb, controllerName, []*big.Int{pubKey.X, pubKey.Y})

		var recipientAddr pldtypes.Bytes32
		rpcerr = s.rpc.CallRPC(ctx, &recipientAddr, "ptx_resolveVerifier", recipient1Name, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
		require.Nil(t, rpcerr)
		log.L(ctx).Infof("Registering participant %s in the KYC registry", recipient1Name)
		pubKey, err = zetosigner.DecodeBabyJubJubPublicKey(recipientAddr.HexString())
		require.NoError(t, err)
		zeto.Register(ctx, s.tb, controllerName, []*big.Int{pubKey.X, pubKey.Y})

		rpcerr = s.rpc.CallRPC(ctx, &recipientAddr, "ptx_resolveVerifier", recipient2Name, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
		require.Nil(t, rpcerr)
		log.L(ctx).Infof("Registering participant %s in the KYC registry", recipient2Name)
		pubKey, err = zetosigner.DecodeBabyJubJubPublicKey(recipientAddr.HexString())
		require.NoError(t, err)
		zeto.Register(ctx, s.tb, controllerName, []*big.Int{pubKey.X, pubKey.Y})

		rpcerr = s.rpc.CallRPC(ctx, &recipientAddr, "ptx_resolveVerifier", recipient3Name, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
		require.Nil(t, rpcerr)
		log.L(ctx).Infof("Registering participant %s in the KYC registry", recipient3Name)
		pubKey, err = zetosigner.DecodeBabyJubJubPublicKey(recipientAddr.HexString())
		require.NoError(t, err)
		zeto.Register(ctx, s.tb, controllerName, []*big.Int{pubKey.X, pubKey.Y})

		time.Sleep(5 * time.Second) // wait for the KYC registry to be updated
	}

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Mint two UTXOs (10, 20) from controller to controller")
	log.L(ctx).Info("*************************************")
	zeto.Mint(ctx, controllerName, []uint64{10, 20}).SignAndSend(controllerName, true).Wait()

	jq := query.NewQueryBuilder().Limit(100).Equal("locked", false).Query()
	methodName := "pstate_queryContractStates"
	if isNullifiersToken {
		methodName = "pstate_queryContractNullifiers"
	}

	coins := findAvailableCoins(t, ctx, s.rpc, s.domain.Name(), s.domain.CoinSchemaID(), methodName, zetoAddress, jq, func(coins []*types.ZetoCoinState) bool {
		return len(coins) >= 2
	})
	require.Len(t, coins, 2)
	assert.Equal(t, int64(10), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, controllerAddr.String(), coins[0].Data.Owner.String())
	assert.Equal(t, int64(20), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, controllerAddr.String(), coins[1].Data.Owner.String())

	balanceOfResult := zeto.BalanceOf(ctx, controllerName).SignAndCall(controllerName).Wait()
	assert.Equal(t, "30", balanceOfResult["totalBalance"].(string), "Balance of controller should be 30")
	// for testing the batch circuits, we mint the 3rd UTXO
	if useBatch {
		log.L(ctx).Info("*************************************")
		log.L(ctx).Infof("Mint 30 from controller to controller")
		log.L(ctx).Info("*************************************")
		zeto.Mint(ctx, controllerName, []uint64{30}).SignAndSend(controllerName, true).Wait()
		balanceOfResult = zeto.BalanceOf(ctx, controllerName).SignAndCall(controllerName).Wait()
		assert.Equal(t, "60", balanceOfResult["totalBalance"].(string), "Balance of controller should be 60")
	}

	if useBatch {
		// for testing the batch circuits, we transfer 55 which would require 3 UTXOs (>2)
		amount1 := 15
		amount2 := 40
		log.L(ctx).Info("*************************************")
		log.L(ctx).Infof("Transfer %d from controller to recipient1 (%d) and recipient2 (%d)", amount1+amount2, amount1, amount2)
		log.L(ctx).Info("*************************************")
		zeto.Transfer(ctx, []string{recipient1Name, recipient2Name}, []uint64{uint64(amount1), uint64(amount2)}).SignAndSend(controllerName, true).Wait()
	} else {
		amount := 25
		log.L(ctx).Info("*************************************")
		log.L(ctx).Infof("Transfer %d from controller to recipient1", amount)
		log.L(ctx).Info("*************************************")
		zeto.Transfer(ctx, []string{recipient1Name}, []uint64{uint64(amount)}).SignAndSend(controllerName, true).Wait()

	}
	balanceOfResult = zeto.BalanceOf(ctx, controllerName).SignAndCall(controllerName).Wait()
	assert.Equal(t, "5", balanceOfResult["totalBalance"].(string), "Balance of controller should be 5")

	// check that we now only have one unspent coin, of value 5
	// one for the controller from the successful transaction as change (value=5)
	// one for the recipient (value=25)
	expectedCoins := 2
	if useBatch {
		// one for the controller from the successful transaction as change (value=5)
		// one for the recipient1 (value=15)
		// one for the recipient2 (value=40)
		expectedCoins = 3
	}
	coins = findAvailableCoins(t, ctx, s.rpc, s.domain.Name(), s.domain.CoinSchemaID(), methodName, zetoAddress, jq, func(coins []*types.ZetoCoinState) bool {
		if len(coins) >= expectedCoins {
			if useBatch {
				return coins[0].Data.Amount.Int().Text(10) == "15" && coins[1].Data.Amount.Int().Text(10) == "40" && coins[2].Data.Amount.Int().Text(10) == "5"
			}
			return coins[0].Data.Amount.Int().Text(10) == "25" && coins[1].Data.Amount.Int().Text(10) == "5"
		}
		return false
	})
	if len(coins) != expectedCoins {
		for i, coin := range coins {
			fmt.Printf("==> Coin %d: %+v\n", i, coin)
		}
	}
	require.Len(t, coins, expectedCoins)

	if useBatch {
		assert.Equal(t, int64(15), coins[0].Data.Amount.Int().Int64()) // state for recipient1
		assert.Equal(t, int64(40), coins[1].Data.Amount.Int().Int64()) // state for recipient2
		assert.Equal(t, int64(5), coins[2].Data.Amount.Int().Int64())  // change for controller
		assert.Equal(t, controllerAddr.String(), coins[2].Data.Owner.String())
	} else {
		assert.Equal(t, int64(25), coins[0].Data.Amount.Int().Int64()) // state for recipient1
		assert.Equal(t, int64(5), coins[1].Data.Amount.Int().Int64())  // change for controller
		assert.Equal(t, controllerAddr.String(), coins[1].Data.Owner.String())
	}

	log.L(ctx).Infof("Mint 100 in ERC20 to controller")
	zeto.MintERC20(ctx, s.tb, *erc20Address, 100, controllerName, controllerEthAddr)

	log.L(ctx).Infof("Approve Zeto (%s) to spend from the controller account (%s)", zetoAddress.String(), controllerEthAddr)
	zeto.ApproveERC20(ctx, s.tb, *erc20Address, 100, controllerName)

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Deposit from ERC20 balance to Zeto")
	log.L(ctx).Info("*************************************")
	zeto.Deposit(ctx, 100).SignAndSend(controllerName, true).Wait()

	balanceOfResult = zeto.BalanceOf(ctx, controllerName).SignAndCall(controllerName).Wait()
	assert.Equal(t, "105", balanceOfResult["totalBalance"].(string), "Balance of controller should be 105")

	expectedCoins += 2 // the deposit call produces 2 output UTXOs for the receiver
	coins = findAvailableCoins(t, ctx, s.rpc, s.domain.Name(), s.domain.CoinSchemaID(), methodName, zetoAddress, jq, func(coins []*types.ZetoCoinState) bool {
		return len(coins) >= expectedCoins
	})
	require.Len(t, coins, expectedCoins)

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Withdraw back to ERC20 balance from Zeto")
	log.L(ctx).Info("*************************************")
	zeto.Withdraw(ctx, 100).SignAndSend(controllerName, true).Wait()

	if tokenName != constants.TOKEN_ANON {
		// for now the lock and transferLocked only works properly for the ANON token
		return
	}

	balanceOfResult = zeto.BalanceOf(ctx, controllerName).SignAndCall(controllerName).Wait()
	assert.Equal(t, "5", balanceOfResult["totalBalance"].(string), "Balance of controller should be 5")

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Lock some UTXOs and delegate the lock to recipient1")
	log.L(ctx).Info("*************************************")
	// the delegate being recipient1 is just for testing purposes, in a real scenario the delegate would be
	// a smart contract (such as playing the role of a trade orchestrator or escrow)
	var recipient1EthAddrStr string
	rpcerr = s.rpc.CallRPC(ctx, &recipient1EthAddrStr, "ptx_resolveVerifier", recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.Nil(t, rpcerr)
	recipient1EthAddr := pldtypes.MustEthAddress(recipient1EthAddrStr)
	zeto.Lock(ctx, recipient1EthAddr, 1).SignAndSend(controllerName, true).Wait()
	zeto.Lock(ctx, recipient1EthAddr, 1).SignAndSend(controllerName, true).Wait()

	balanceOfResult = zeto.BalanceOf(ctx, controllerName).SignAndCall(controllerName).Wait()
	assert.Equal(t, "3", balanceOfResult["totalBalance"].(string), "Balance of controller should be 3")

	jq = query.NewQueryBuilder().Limit(100).Equal("locked", true).Query()
	coins = findAvailableCoins(t, ctx, s.rpc, s.domain.Name(), s.domain.CoinSchemaID(), methodName, zetoAddress, jq, func(coins []*types.ZetoCoinState) bool {
		return len(coins) >= 2
	})
	require.Len(t, coins, 2)
	locked1, _ := coins[0].Data.Hash(ctx)
	locked2, _ := coins[1].Data.Hash(ctx)

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Recipient1 unlocks one of the locked UTXOs %s", locked2.String())
	log.L(ctx).Info("*************************************")
	// unlocking by calling transferlocked()
	zeto.TransferLocked(ctx, locked2, recipient1Name, controllerName, 1).SignAndSend(controllerName, true).Wait()

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Recipient1 delegates the lock to recipient2")
	log.L(ctx).Info("*************************************")
	var recipient2EthAddrStr string
	rpcerr = s.rpc.CallRPC(ctx, &recipient2EthAddrStr, "ptx_resolveVerifier", recipient2Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.Nil(t, rpcerr)

	recipient2EthAddr := pldtypes.MustEthAddress(recipient2EthAddrStr)
	zeto.DelegateLock(ctx, s.tb, locked1, recipient2EthAddr, recipient1Name)

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Transfer locked UTXO %s to recipient3", locked1.String())
	log.L(ctx).Info("*************************************")
	// the owner of the locked UTXO is the controller, who needs to generate the proof for the transfer.
	result := zeto.TransferLocked(ctx, locked1, recipient2Name, recipient3Name, 1).Prepare(controllerName)
	// the delegate of the locked UTXO is recipient2, who needs to send the prepared transaction
	zeto.SendTransferLocked(ctx, s.tb, recipient2Name, result)
}

func (s *zetoDomainTestSuite) setupContractsAbi(t *testing.T, ctx context.Context, tokenName string) {
	var result pldtypes.HexBytes

	contractAbi, ok := s.deployedContracts.DeployedContractAbis[tokenName]
	require.True(t, ok, "Missing ABI for contract %s", tokenName)
	rpcerr := s.rpc.CallRPC(ctx, &result, "ptx_storeABI", contractAbi)
	if rpcerr != nil {
		require.NoError(t, rpcerr.RPCError())
	}
}
