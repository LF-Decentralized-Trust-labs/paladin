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
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	internalZeto "github.com/kaleido-io/paladin/domains/zeto/internal/zeto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zeto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/solutils"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestFungibleZetoDomainTestSuite(t *testing.T) {
	contractsFile = "./config-for-deploy-fungible.yaml"
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

func (s *fungibleTestSuiteHelper) testZeto(t *testing.T, tokenName string, useBatch bool, isNullifiersToken bool) {
	ctx := context.Background()
	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Deploying an instance of the %s token", tokenName)
	log.L(ctx).Info("*************************************")
	s.setupContractsAbi(t, ctx, tokenName)
	var zetoAddress tktypes.EthAddress
	rpcerr := s.rpc.CallRPC(ctx, &zetoAddress, "testbed_deploy",
		s.domainName, controllerName, &types.InitializerParams{
			TokenName: tokenName,
		})
	if rpcerr != nil {
		require.NoError(t, rpcerr.RPCError())
	}
	log.L(ctx).Infof("Zeto instance deployed to %s", zetoAddress)

	var controllerEthAddr string
	rpcerr = s.rpc.CallRPC(ctx, &controllerEthAddr, "ptx_resolveVerifier", controllerName, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.Nil(t, rpcerr)

	log.L(ctx).Infof("Deploying the sample ERC20 with initialOwner %s", controllerEthAddr)
	erc20Address, err := deployERC20(ctx, s.rpc, controllerEthAddr)
	require.NoError(t, err)

	log.L(ctx).Infof("Setting the ERC20 contract (%s) to the Zeto instance", erc20Address)
	paramsJson, err := json.Marshal(&map[string]string{"erc20": erc20Address.String()})
	require.NoError(t, err)
	_, err = s.tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     controllerName,
			To:       &zetoAddress,
			Function: "setERC20",
			Data:     paramsJson,
		},
		ABI: types.ZetoFungibleABI,
	})
	require.NoError(t, err)

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Mint two UTXOs (10, 20) from controller to controller")
	log.L(ctx).Info("*************************************")
	_, err = s.mint(ctx, zetoAddress, controllerName, []int64{10, 20})
	require.NoError(t, err)

	var controllerAddr tktypes.Bytes32
	rpcerr = s.rpc.CallRPC(ctx, &controllerAddr, "ptx_resolveVerifier", controllerName, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	require.Nil(t, rpcerr)

	coins := findAvailableCoins(t, ctx, s.rpc, s.domain, zetoAddress, isNullifiersToken)
	require.Len(t, coins, 2)
	assert.Equal(t, int64(10), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, controllerAddr.String(), coins[0].Data.Owner.String())
	assert.Equal(t, int64(20), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, controllerAddr.String(), coins[1].Data.Owner.String())

	// for testing the batch circuits, we mint the 3rd UTXO
	if useBatch {
		log.L(ctx).Info("*************************************")
		log.L(ctx).Infof("Mint 30 from controller to controller")
		log.L(ctx).Info("*************************************")
		_, err = s.mint(ctx, zetoAddress, controllerName, []int64{30})
		require.NoError(t, err)
	}

	log.L(ctx).Infof("Attempt mint from non-controller (should fail)")
	_, err = s.mint(ctx, zetoAddress, recipient1Name, []int64{10})
	require.ErrorContains(t, err, "PD012216: Transaction reverted OwnableUnauthorizedAccount")
	assert.Regexp(t, "PD012216: Transaction reverted OwnableUnauthorizedAccount.*", err)

	if useBatch {
		// for testing the batch circuits, we transfer 55 which would require 3 UTXOs (>2)
		amount1 := 15
		amount2 := 40
		log.L(ctx).Info("*************************************")
		log.L(ctx).Infof("Transfer %d from controller to recipient1 (%d) and recipient2 (%d)", amount1+amount2, amount1, amount2)
		log.L(ctx).Info("*************************************")
		_, err = s.transfer(ctx, zetoAddress, controllerName, []string{recipient1Name, recipient2Name}, []int64{int64(amount1), int64(amount2)})
		require.NoError(t, err)
	} else {
		amount := 25
		log.L(ctx).Info("*************************************")
		log.L(ctx).Infof("Transfer %d from controller to recipient1", amount)
		log.L(ctx).Info("*************************************")
		_, err = s.transfer(ctx, zetoAddress, controllerName, []string{recipient1Name}, []int64{int64(amount)})
		require.NoError(t, err)
	}

	// check that we now only have one unspent coin, of value 5
	coins = findAvailableCoins(t, ctx, s.rpc, s.domain, zetoAddress, isNullifiersToken)
	// one for the controller from the successful transaction as change (value=5)
	// one for the recipient (value=25)
	expectedCoins := 2
	if useBatch {
		// one for the controller from the successful transaction as change (value=5)
		// one for the recipient1 (value=15)
		// one for the recipient2 (value=40)
		expectedCoins = 3
	}
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
	err = s.mintERC20(ctx, *erc20Address, 100, controllerName, controllerEthAddr)
	require.NoError(t, err)

	log.L(ctx).Infof("Approve Zeto (%s) to spend from the controller account (%s)", zetoAddress.String(), controllerEthAddr)
	err = s.approveERC20(ctx, *erc20Address, zetoAddress, 100, controllerName)
	require.NoError(t, err)

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Deposit from ERC20 balance to Zeto")
	log.L(ctx).Info("*************************************")
	_, err = s.deposit(ctx, zetoAddress, controllerName, 100)
	require.NoError(t, err)

	expectedCoins += 2 // the deposit call produces 2 output UTXOs for the receiver
	coins = findAvailableCoins(t, ctx, s.rpc, s.domain, zetoAddress, isNullifiersToken)
	require.Len(t, coins, expectedCoins)

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Withdraw back to ERC20 balance from Zeto")
	log.L(ctx).Info("*************************************")
	_, err = s.withdraw(ctx, zetoAddress, controllerName, 100)
	require.NoError(t, err)

	if tokenName != constants.TOKEN_ANON {
		return
	}

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Lock some UTXOs and delegate the lock to recipient1")
	log.L(ctx).Info("*************************************")
	// the delegate being recipient1 is just for testing purposes, in a real scenario the delegate would be
	// a smart contract (such as playing the role of a trade orchestrator or escrow)
	var recipient1EthAddrStr string
	rpcerr = s.rpc.CallRPC(ctx, &recipient1EthAddrStr, "ptx_resolveVerifier", recipient1Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.Nil(t, rpcerr)
	recipient1EthAddr := tktypes.MustEthAddress(recipient1EthAddrStr)
	_, err = s.lock(ctx, zetoAddress, controllerName, 1, recipient1EthAddr)
	require.NoError(t, err)
	_, err = s.lock(ctx, zetoAddress, controllerName, 1, recipient1EthAddr)
	require.NoError(t, err)

	coins = findAvailableCoins(t, ctx, s.rpc, s.domain, zetoAddress, isNullifiersToken, true /* locked */)
	require.Len(t, coins, 2)
	locked1, _ := coins[0].Data.Hash(ctx)
	locked2, _ := coins[1].Data.Hash(ctx)

	// for now unlock only works properly for the ANON token
	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Recipient1 unlocks one of the locked UTXOs %s", locked2.String())
	log.L(ctx).Info("*************************************")
	// unlocking by calling transferlocked()
	_, err = s.TransferLocked(ctx, &zetoAddress, locked2, recipient1Name, controllerName, controllerName, 1)
	require.NoError(t, err)

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Recipient1 delegates the lock to recipient2")
	log.L(ctx).Info("*************************************")
	var recipient2EthAddrStr string
	rpcerr = s.rpc.CallRPC(ctx, &recipient2EthAddrStr, "ptx_resolveVerifier", recipient2Name, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.Nil(t, rpcerr)
	txInput := map[string]any{
		"utxos":    []string{locked1.String()},
		"delegate": recipient2EthAddrStr,
		"data":     "0x",
	}
	txInputJson, err := json.Marshal(txInput)
	require.NoError(t, err)
	// submit a public transaction to delegate the lock
	build, err := getZetoAnonSpec()
	require.NoError(t, err)
	_, err = s.tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     recipient1Name,
			To:       &zetoAddress,
			Function: "delegateLock",
			Data:     txInputJson,
		},
		ABI: build.ABI,
	})
	require.NoError(t, err)

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Transfer locked UTXO %s to recipient3", locked1.String())
	log.L(ctx).Info("*************************************")
	// the owner of the locked UTXO is the controller, who needs to generate the proof for the transfer.
	result, err := s.PrepareTransferLocked(ctx, &zetoAddress, locked1, recipient2Name, controllerName, recipient3Name, 1)
	require.NoError(t, err)
	require.NotNil(t, result)
	_, err = s.tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     recipient2Name,
			To:       &zetoAddress,
			Function: "transferLocked",
			Data:     result.PreparedTransaction.Data,
		},
		ABI: build.ABI,
	})
	require.NoError(t, err)
}

func (s *zetoDomainTestSuite) setupContractsAbi(t *testing.T, ctx context.Context, tokenName string) {
	var result tktypes.HexBytes

	contractAbi, ok := s.deployedContracts.DeployedContractAbis[tokenName]
	require.True(t, ok, "Missing ABI for contract %s", tokenName)
	rpcerr := s.rpc.CallRPC(ctx, &result, "ptx_storeABI", contractAbi)
	if rpcerr != nil {
		require.NoError(t, rpcerr.RPCError())
	}
}

func (s *fungibleTestSuiteHelper) mint(ctx context.Context, zetoAddress tktypes.EthAddress, minter string, amounts []int64) (invokeResult *testbed.TransactionResult, err error) {
	var params []*types.FungibleTransferParamEntry
	for _, amount := range amounts {
		params = append(params, &types.FungibleTransferParamEntry{
			To:     minter,
			Amount: tktypes.Int64ToInt256(amount),
		})
	}
	mintParam := types.FungibleMintParams{
		Mints: params,
	}
	paramsJson, err := json.Marshal(&mintParam)
	if err != nil {
		return nil, err
	}
	rpcerr := s.rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     minter,
			To:       &zetoAddress,
			Function: "mint",
			Data:     paramsJson,
		},
		ABI: types.ZetoFungibleABI,
	}, true)
	if rpcerr != nil {
		return nil, rpcerr.RPCError()
	}
	return invokeResult, nil
}

func (s *fungibleTestSuiteHelper) transfer(ctx context.Context, zetoAddress tktypes.EthAddress, sender string, receivers []string, amounts []int64) (*testbed.TransactionResult, error) {
	var invokeResult testbed.TransactionResult
	var params []*types.FungibleTransferParamEntry
	for i, receiver := range receivers {
		params = append(params, &types.FungibleTransferParamEntry{
			To:     receiver,
			Amount: tktypes.Int64ToInt256(amounts[i]),
		})
	}
	transferParams := types.FungibleTransferParams{
		Transfers: params,
	}
	paramsJson, err := json.Marshal(&transferParams)
	if err != nil {
		return nil, err
	}
	rpcerr := s.rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     sender,
			To:       &zetoAddress,
			Function: "transfer",
			Data:     paramsJson,
		},
		ABI: types.ZetoFungibleABI,
	}, true)
	if rpcerr != nil {
		return nil, rpcerr.RPCError()
	}
	return &invokeResult, nil
}

func (s *fungibleTestSuiteHelper) deposit(ctx context.Context, zetoAddress tktypes.EthAddress, sender string, amount int64) (*testbed.TransactionResult, error) {
	params := &types.DepositParams{
		Amount: tktypes.Int64ToInt256(amount),
	}
	paramsJson, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	var invokeResult testbed.TransactionResult
	rpcerr := s.rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     sender,
			To:       &zetoAddress,
			Function: "deposit",
			Data:     paramsJson,
		},
		ABI: types.ZetoFungibleABI,
	}, true)
	if rpcerr != nil {
		return nil, rpcerr.RPCError()
	}
	return &invokeResult, nil
}

func (s *fungibleTestSuiteHelper) withdraw(ctx context.Context, zetoAddress tktypes.EthAddress, sender string, amount int64) (*testbed.TransactionResult, error) {
	params := &types.WithdrawParams{
		Amount: tktypes.Int64ToInt256(amount),
	}
	paramsJson, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	var invokeResult testbed.TransactionResult
	rpcerr := s.rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     sender,
			To:       &zetoAddress,
			Function: "withdraw",
			Data:     paramsJson,
		},
		ABI: types.ZetoFungibleABI,
	}, true)
	if rpcerr != nil {
		return nil, rpcerr.RPCError()
	}
	return &invokeResult, nil
}

func (s *fungibleTestSuiteHelper) mintERC20(ctx context.Context, erc20Address tktypes.EthAddress, amount int64, from, to string) error {
	paramsJson, err := json.Marshal(&map[string]any{"amount": amount, "to": to})
	if err != nil {
		return err
	}
	build, err := getERC20Spec()
	if err != nil {
		return err
	}
	_, err = s.tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     from,
			To:       &erc20Address,
			Function: "mint",
			Data:     paramsJson,
		},
		ABI: build.ABI,
	})
	return err
}

func (s *fungibleTestSuiteHelper) approveERC20(ctx context.Context, erc20Address, zetoAddress tktypes.EthAddress, amount int64, from string) error {
	paramsJson, err := json.Marshal(&map[string]any{"spender": zetoAddress.String(), "value": amount})
	if err != nil {
		return err
	}
	build, err := getERC20Spec()
	if err != nil {
		return err
	}
	_, err = s.tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     from,
			To:       &erc20Address,
			Function: "approve",
			Data:     paramsJson,
		},
		ABI: build.ABI,
	})
	return err
}

func (s *fungibleTestSuiteHelper) lock(ctx context.Context, zetoAddress tktypes.EthAddress, sender string, amount int, delegate *tktypes.EthAddress) (*testbed.TransactionResult, error) {
	var invokeResult testbed.TransactionResult
	params := types.LockParams{
		Amount:   tktypes.Uint64ToUint256(uint64(amount)),
		Delegate: delegate,
	}
	paramsJson, _ := json.Marshal(&params)
	rpcerr := s.rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     sender,
			To:       &zetoAddress,
			Function: "lock",
			Data:     paramsJson,
		},
		ABI: types.ZetoFungibleABI,
	}, true)
	if rpcerr != nil {
		return nil, rpcerr
	}
	return &invokeResult, nil
}

func (s *fungibleTestSuiteHelper) TransferLocked(ctx context.Context, zetoAddress *tktypes.EthAddress, lockedInput *tktypes.HexUint256, delegate, sender, receiver string, amount int) (*testbed.TransactionResult, error) {
	params := &types.FungibleTransferLockedParams{
		LockedInputs: []*tktypes.HexUint256{lockedInput},
		Delegate:     delegate,
		Transfers: []*types.FungibleTransferParamEntry{
			{
				To:     receiver,
				Amount: tktypes.Uint64ToUint256(uint64(amount)),
			},
		},
	}
	paramsJson, _ := json.Marshal(params)
	tx := &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			To:       zetoAddress,
			From:     sender,
			Function: "transferLocked",
			Data:     paramsJson,
		},
		ABI: types.ZetoFungibleABI,
	}

	var result testbed.TransactionResult
	rpcerr := s.rpc.CallRPC(ctx, &result, "testbed_invoke", tx, true)
	if rpcerr != nil {
		return nil, rpcerr
	}
	return &result, nil
}

func (s *fungibleTestSuiteHelper) PrepareTransferLocked(ctx context.Context, zetoAddress *tktypes.EthAddress, lockedInput *tktypes.HexUint256, delegate, sender, receiver string, amount int) (*testbed.TransactionResult, error) {
	params := &types.FungibleTransferLockedParams{
		LockedInputs: []*tktypes.HexUint256{lockedInput},
		Delegate:     delegate,
		Transfers: []*types.FungibleTransferParamEntry{
			{
				To:     receiver,
				Amount: tktypes.Uint64ToUint256(uint64(amount)),
			},
		},
	}
	paramsJson, _ := json.Marshal(params)
	tx := &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			To:       zetoAddress,
			From:     sender,
			Function: "transferLocked",
			Data:     paramsJson,
		},
		ABI: types.ZetoFungibleABI,
	}

	var result testbed.TransactionResult
	rpcerr := s.rpc.CallRPC(ctx, &result, "testbed_prepare", tx)
	if rpcerr != nil {
		return nil, rpcerr
	}
	return &result, nil
}

func findAvailableCoins(t *testing.T, ctx context.Context, rpc rpcclient.Client, zeto zeto.Zeto, address tktypes.EthAddress, useNullifiers bool, locked ...bool) []*types.ZetoCoinState {
	isLocked := len(locked) > 0 && locked[0]
	jq := query.NewQueryBuilder().Limit(100).Equal("locked", isLocked).Query()
	methodName := "pstate_queryContractStates"
	if useNullifiers {
		methodName = "pstate_queryContractNullifiers"
	}
	var zetoCoins []*types.ZetoCoinState
	rpcerr := rpc.CallRPC(ctx, &zetoCoins, methodName,
		zeto.Name(),
		address,
		zeto.CoinSchemaID(),
		jq,
		"available")
	if rpcerr != nil {
		require.NoError(t, rpcerr.RPCError())
	}
	return zetoCoins
}

func mapConfig(t *testing.T, config *types.DomainFactoryConfig) (m map[string]any) {
	configJSON, err := json.Marshal(&config)
	require.NoError(t, err)
	err = json.Unmarshal(configJSON, &m)
	require.NoError(t, err)
	return m
}

func newZetoDomain(t *testing.T, domainContracts *ZetoDomainContracts, config *types.DomainFactoryConfig) (zeto.Zeto, *testbed.TestbedDomain) {
	var domain internalZeto.Zeto
	return &domain, &testbed.TestbedDomain{
		Config: mapConfig(t, config),
		Plugin: plugintk.NewDomain(func(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {
			domain.Callbacks = callbacks
			return &domain
		}),
		RegistryAddress: tktypes.MustEthAddress(domainContracts.FactoryAddress.String()),
		AllowSigning:    true,
	}
}

func newTestbed(t *testing.T, hdWalletSeed *testbed.UTInitFunction, domains map[string]*testbed.TestbedDomain) (context.CancelFunc, testbed.Testbed, rpcclient.Client) {
	tb := testbed.NewTestBed()
	url, _, done, err := tb.StartForTest("./testbed.config.yaml", domains, hdWalletSeed)
	assert.NoError(t, err)
	rpc := rpcclient.WrapRestyClient(resty.New().SetBaseURL(url))
	return done, tb, rpc
}

func getERC20Spec() (*solutils.SolidityBuild, error) {
	build := solutils.MustLoadBuild(erc20ABI)
	return build, nil
}

func getZetoAnonSpec() (*solutils.SolidityBuild, error) {
	build := solutils.MustLoadBuild(zetoAnonAbi)
	return build, nil
}

func deployERC20(ctx context.Context, rpc rpcclient.Client, controllerAddr string) (*tktypes.EthAddress, error) {
	build, err := getERC20Spec()
	if err != nil {
		return nil, err
	}
	params := fmt.Sprintf(`{"initialOwner":"%s"}`, controllerAddr)
	var addr string
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode", controllerName, build.ABI, build.Bytecode.String(), tktypes.RawJSON(params))
	if rpcerr != nil {
		return nil, rpcerr.RPCError()
	}
	return tktypes.MustEthAddress(addr), nil
}

func functionBuilder(ctx context.Context, pld pldclient.PaladinClient, abi abi.ABI, functionName string) pldclient.TxBuilder {
	return pld.ForABI(ctx, abi).Function(functionName)
}
