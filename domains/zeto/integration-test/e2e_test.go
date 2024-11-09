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
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	internalZeto "github.com/kaleido-io/paladin/domains/zeto/internal/zeto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zeto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	controllerName = "controller"
	recipient1Name = "recipient1"
	recipient2Name = "recipient2"
)

func TestZetoDomainTestSuite(t *testing.T) {
	suite.Run(t, new(zetoDomainTestSuite))
}

type zetoDomainTestSuite struct {
	suite.Suite
	hdWalletSeed      *testbed.UTInitFunction
	deployedContracts *ZetoDomainContracts
	domainName        string
	domain            zeto.Zeto
	rpc               rpcbackend.Backend
	done              func()
}

func (s *zetoDomainTestSuite) SetupSuite() {
	s.hdWalletSeed = testbed.HDWalletSeedScopedToTest()
	domainContracts := DeployZetoContracts(s.T(), s.hdWalletSeed, "./config-for-deploy.yaml", controllerName)
	s.deployedContracts = domainContracts
	ctx := context.Background()
	domainName := "zeto_" + tktypes.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)
	config := PrepareZetoConfig(s.T(), s.deployedContracts, "../zkp")
	zeto, zetoTestbed := newZetoDomain(s.T(), domainContracts, config)
	done, _, rpc := newTestbed(s.T(), s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		domainName: zetoTestbed,
	})
	s.domainName = domainName
	s.domain = zeto
	s.rpc = rpc
	s.done = done
}

func (s *zetoDomainTestSuite) TearDownSuite() {
	s.done()
}

func (s *zetoDomainTestSuite) TestZeto_Anon() {
	s.testZetoFungible(s.T(), constants.TOKEN_ANON, false, false)
}

func (s *zetoDomainTestSuite) TestZeto_AnonBatch() {
	s.T().Skip()
	s.testZetoFungible(s.T(), constants.TOKEN_ANON, true, false)
}

func (s *zetoDomainTestSuite) TestZeto_AnonEnc() {
	s.testZetoFungible(s.T(), constants.TOKEN_ANON_ENC, false, false)
}

func (s *zetoDomainTestSuite) TestZeto_AnonEncBatch() {
	s.testZetoFungible(s.T(), constants.TOKEN_ANON_ENC, true, false)
}

func (s *zetoDomainTestSuite) TestZeto_AnonNullifier() {
	s.testZetoFungible(s.T(), constants.TOKEN_ANON_NULLIFIER, false, true)
}

func (s *zetoDomainTestSuite) TestZeto_AnonNullifierBatch() {
	s.testZetoFungible(s.T(), constants.TOKEN_ANON_NULLIFIER, true, true)
}

func (s *zetoDomainTestSuite) testZetoFungible(t *testing.T, tokenName string, useBatch bool, isNullifiersToken bool) {
	ctx := context.Background()
	log.L(ctx).Infof("Deploying an instance of the %s token", tokenName)
	s.setupContractsAbi(t, ctx, tokenName)
	var zetoAddress tktypes.EthAddress
	rpcerr := s.rpc.CallRPC(ctx, &zetoAddress, "testbed_deploy",
		s.domainName, controllerName, &types.InitializerParams{
			TokenName: tokenName,
		})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}
	log.L(ctx).Infof("Zeto instance deployed to %s", zetoAddress)

	log.L(ctx).Infof("Mint two UTXOs (10, 20) from controller to controller")
	_, err := s.mint(ctx, zetoAddress, controllerName, []int64{10, 20})
	require.NoError(t, err)

	var controllerAddr tktypes.Bytes32
	rpcerr = s.rpc.CallRPC(ctx, &controllerAddr, "ptx_resolveVerifier", controllerName, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	require.Nil(t, rpcerr)

	coins := findAvailableCoins(t, ctx, s.rpc, s.domain, zetoAddress, nil, isNullifiersToken)
	require.Len(t, coins, 2)
	assert.Equal(t, int64(10), coins[0].Data.Amount.Int().Int64())
	assert.Equal(t, controllerAddr.String(), coins[0].Data.Owner.String())
	assert.Equal(t, int64(20), coins[1].Data.Amount.Int().Int64())
	assert.Equal(t, controllerAddr.String(), coins[1].Data.Owner.String())

	// for testing the batch circuits, we mint the 3rd UTXO
	if useBatch {
		log.L(ctx).Infof("Mint 30 from controller to controller")
		_, err = s.mint(ctx, zetoAddress, controllerName, []int64{30})
		require.NoError(t, err)
	}

	log.L(ctx).Infof("Attempt mint from non-controller (should fail)")
	_, err = s.mint(ctx, zetoAddress, recipient1Name, []int64{10})
	require.ErrorContains(t, err, "PD012216: Transaction reverted OwnableUnauthorizedAccount")
	assert.Regexp(t, "PD012216: Transaction reverted OwnableUnauthorizedAccount.*", err)

	if useBatch {
		// for testing the batch circuits, we transfer 50 which would require 3 UTXOs (>2)
		amount1 := 10
		amount2 := 40
		log.L(ctx).Infof("Transfer %d from controller to recipient1 (%d) and recipient2 (%d)", amount1+amount2, amount1, amount2)
		_, err = s.transfer(ctx, zetoAddress, controllerName, []string{recipient1Name, recipient2Name}, []int64{int64(amount1), int64(amount2)})
		require.NoError(t, err)
	} else {
		amount := 25
		log.L(ctx).Infof("Transfer %d from controller to recipient1", amount)
		_, err = s.transfer(ctx, zetoAddress, controllerName, []string{recipient1Name}, []int64{int64(amount)})
		require.NoError(t, err)
	}

	// check that we now only have one unspent coin, of value 5
	coins = findAvailableCoins(t, ctx, s.rpc, s.domain, zetoAddress, nil, isNullifiersToken)
	// one for the controller from the successful transaction as change (value=5)
	// one for the recipient (value=25)
	expectedCoins := 2
	if useBatch {
		expectedCoins = 3
	}

	// some tokens like Zeto_AnonNullifier with batch can take some time to process
	// so we need to retry at least once
	if len(coins) != expectedCoins {
		time.Sleep(1 * time.Second)
		coins = findAvailableCoins(t, ctx, s.rpc, s.domain, zetoAddress, nil, isNullifiersToken)
	}

	require.Len(t, coins, expectedCoins)

	if useBatch {
		assert.Equal(t, int64(10), coins[0].Data.Amount.Int().Int64()) // state for recipient1
		assert.Equal(t, int64(40), coins[1].Data.Amount.Int().Int64()) // state for recipient2
		assert.Equal(t, int64(10), coins[2].Data.Amount.Int().Int64()) // change for controller
		assert.Equal(t, controllerAddr.String(), coins[2].Data.Owner.String())
	} else {
		assert.Equal(t, int64(25), coins[0].Data.Amount.Int().Int64()) // state for recipient1
		assert.Equal(t, int64(5), coins[1].Data.Amount.Int().Int64())  // change for controller
		assert.Equal(t, controllerAddr.String(), coins[1].Data.Owner.String())
	}
}

func (s *zetoDomainTestSuite) setupContractsAbi(t *testing.T, ctx context.Context, tokenName string) {
	var result tktypes.HexBytes

	contractAbi, ok := s.deployedContracts.deployedContractAbis[tokenName]
	require.True(t, ok, "Missing ABI for contract %s", tokenName)
	rpcerr := s.rpc.CallRPC(ctx, &result, "ptx_storeABI", contractAbi)
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}
}

func (s *zetoDomainTestSuite) mint(ctx context.Context, zetoAddress tktypes.EthAddress, minter string, amounts []int64) (invokeResult *testbed.TransactionResult, err error) {
	var params []*types.TransferParamEntry
	for _, amount := range amounts {
		params = append(params, &types.TransferParamEntry{
			To:     minter,
			Amount: tktypes.Int64ToInt256(amount),
		})
	}
	mintParam := types.MintParams{
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
		ABI: types.ZetoABI,
	}, true)
	if rpcerr != nil {
		return nil, rpcerr.Error()
	}
	return invokeResult, nil
}

func (s *zetoDomainTestSuite) transfer(ctx context.Context, zetoAddress tktypes.EthAddress, sender string, receivers []string, amounts []int64) (*testbed.TransactionResult, error) {
	var invokeResult testbed.TransactionResult
	var params []*types.TransferParamEntry
	for i, receiver := range receivers {
		params = append(params, &types.TransferParamEntry{
			To:     receiver,
			Amount: tktypes.Int64ToInt256(amounts[i]),
		})
	}
	transferParams := types.TransferParams{
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
		ABI: types.ZetoABI,
	}, true)
	if rpcerr != nil {
		return nil, rpcerr.Error()
	}
	return &invokeResult, nil
}

func findAvailableCoins(t *testing.T, ctx context.Context, rpc rpcbackend.Backend, zeto zeto.Zeto, address tktypes.EthAddress, jq *query.QueryJSON, useNullifiers bool) []*types.ZetoCoinState {
	if jq == nil {
		jq = query.NewQueryBuilder().Limit(100).Query()
	}
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
		require.NoError(t, rpcerr.Error())
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

func newTestbed(t *testing.T, hdWalletSeed *testbed.UTInitFunction, domains map[string]*testbed.TestbedDomain) (context.CancelFunc, testbed.Testbed, rpcbackend.Backend) {
	tb := testbed.NewTestBed()
	url, _, done, err := tb.StartForTest("./testbed.config.yaml", domains, hdWalletSeed)
	assert.NoError(t, err)
	rpc := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))
	return done, tb, rpc
}
