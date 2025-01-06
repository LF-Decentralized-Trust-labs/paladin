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

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/domains/integration-test/helpers"
	nototypes "github.com/kaleido-io/paladin/domains/noto/pkg/types"
	zetotests "github.com/kaleido-io/paladin/domains/zeto/integration-test"
	zetotypes "github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	notary = "notary"
	alice  = "alice"
	bob    = "bob"
)

type NotoTransferParams struct {
	Inputs  []tktypes.Bytes32 `json:"inputs"`
	Outputs []tktypes.Bytes32 `json:"outputs"`
	Data    tktypes.HexBytes  `json:"data"`
}

type NotoTransferHookParams struct {
	Sender   *tktypes.EthAddress      `json:"sender"`
	From     *tktypes.EthAddress      `json:"from"`
	To       *tktypes.EthAddress      `json:"to"`
	Amount   *tktypes.HexUint256      `json:"amount"`
	Prepared PentePreparedTransaction `json:"prepared"`
}

type PentePreparedTransaction struct {
	ContractAddress tktypes.EthAddress `json:"contractAddress"`
	EncodedCall     tktypes.HexBytes   `json:"encodedCall"`
}

func TestNotoForNoto(t *testing.T) {
	pvpNotoNoto(t, testbed.HDWalletSeedScopedToTest(), false)
}

func TestNotoForNotoWithHooks(t *testing.T) {
	pvpNotoNoto(t, testbed.HDWalletSeedScopedToTest(), true)
}

func pvpNotoNoto(t *testing.T, hdWalletSeed *testbed.UTInitFunction, withHooks bool) {
	ctx := context.Background()
	log.L(ctx).Infof("TestNotoForNoto (withHooks=%t)", withHooks)
	domainName := "noto_" + tktypes.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)

	log.L(ctx).Infof("Deploying factories")
	contractSource := map[string][]byte{
		"noto": helpers.NotoFactoryJSON,
		"atom": helpers.AtomFactoryJSON,
	}
	contracts := deployContracts(ctx, t, hdWalletSeed, notary, contractSource)
	for name, address := range contracts {
		log.L(ctx).Infof("%s deployed to %s", name, address)
	}

	log.L(ctx).Infof("Initializing testbed")
	_, notoTestbed := newNotoDomain(t, &nototypes.DomainConfig{
		FactoryAddress: contracts["noto"],
	})
	done, _, tb, rpc := newTestbed(t, hdWalletSeed, map[string]*testbed.TestbedDomain{
		domainName: notoTestbed,
	})
	defer done()
	pld := helpers.NewPaladinClient(t, ctx, tb)

	aliceKey, err := tb.ResolveKey(ctx, alice, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	bobKey, err := tb.ResolveKey(ctx, bob, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	atomFactory := helpers.InitAtom(t, tb, pld, contracts["atom"])

	var tracker *helpers.NotoTrackerHelper
	var trackerAddress *tktypes.EthAddress
	if withHooks {
		tracker = helpers.DeployTracker(ctx, t, tb, pld, notary)
		trackerAddress = tracker.Address
	}

	log.L(ctx).Infof("Deploying 2 instances of Noto")
	notoGold := helpers.DeployNoto(ctx, t, rpc, domainName, notary, trackerAddress)
	notoSilver := helpers.DeployNoto(ctx, t, rpc, domainName, notary, nil)
	log.L(ctx).Infof("Noto gold deployed to %s", notoGold.Address)
	log.L(ctx).Infof("Noto silver deployed to %s", notoSilver.Address)

	log.L(ctx).Infof("Mint 10 gold to Alice")
	notoGold.Mint(ctx, alice, 10).SignAndSend(notary).Wait()
	log.L(ctx).Infof("Mint 100 silver to Bob")
	notoSilver.Mint(ctx, bob, 100).SignAndSend(notary).Wait()

	// TODO: this should be a Pente private contract, instead of a base ledger contract
	log.L(ctx).Infof("Propose a trade of 1 gold for 10 silver")
	swap := helpers.DeploySwap(ctx, t, tb, pld, alice, &helpers.TradeRequestInput{
		Holder1:       aliceKey.Verifier.Verifier,
		TokenAddress1: notoGold.Address,
		TokenValue1:   tktypes.Int64ToInt256(1),

		Holder2:       bobKey.Verifier.Verifier,
		TokenAddress2: notoSilver.Address,
		TokenValue2:   tktypes.Int64ToInt256(10),
	})

	log.L(ctx).Infof("Prepare the transfers")
	transferGold := notoGold.Transfer(ctx, bob, 1).Prepare(alice)
	transferSilver := notoSilver.Transfer(ctx, alice, 10).Prepare(bob)
	require.NotNil(t, transferGold)
	require.NotNil(t, transferGold.PreparedMetadata)
	require.NotNil(t, transferSilver)
	require.NotNil(t, transferSilver.PreparedMetadata)

	// TODO: this should actually be a Pente state transition
	log.L(ctx).Infof("Prepare the trade execute")
	encodedExecute := swap.Execute(ctx).Prepare()

	log.L(ctx).Infof("Record the prepared transfers")
	sent := swap.Prepare(ctx, &helpers.StateData{
		Inputs:  transferGold.InputStates,
		Outputs: transferGold.OutputStates,
	}).SignAndSend(alice).Wait(5 * time.Second)
	require.NoError(t, sent.Error())
	sent = swap.Prepare(ctx, &helpers.StateData{
		Inputs:  transferSilver.InputStates,
		Outputs: transferSilver.OutputStates,
	}).SignAndSend(bob).Wait(5 * time.Second)
	require.NoError(t, sent.Error())

	var transferGoldExtra nototypes.NotoTransferMetadata
	err = json.Unmarshal(transferGold.PreparedMetadata, &transferGoldExtra)
	require.NoError(t, err)
	var transferSilverExtra nototypes.NotoTransferMetadata
	err = json.Unmarshal(transferSilver.PreparedMetadata, &transferSilverExtra)
	require.NoError(t, err)

	log.L(ctx).Infof("Create Atom instance")
	transferAtom := atomFactory.Create(ctx, alice, []*helpers.AtomOperation{
		{
			ContractAddress: transferGold.PreparedTransaction.To,
			CallData:        transferGoldExtra.TransferWithApproval.EncodedCall,
		},
		{
			ContractAddress: transferSilver.PreparedTransaction.To,
			CallData:        transferSilverExtra.TransferWithApproval.EncodedCall,
		},
		{
			ContractAddress: swap.Address,
			CallData:        tktypes.HexBytes(encodedExecute),
		},
	})

	// TODO: all parties should verify the Atom against the original proposed trade
	// If any party found a discrepancy at this point, they could cancel the swap (last chance to back out)

	log.L(ctx).Infof("Approve both Noto transactions")
	goldDelegate := transferAtom.Address
	if withHooks {
		goldDelegate = trackerAddress
	}
	notoGold.ApproveTransfer(ctx, &nototypes.ApproveParams{
		Inputs:   transferGold.InputStates,
		Outputs:  transferGold.OutputStates,
		Data:     transferGoldExtra.ApprovalParams.Data,
		Delegate: goldDelegate,
	}).SignAndSend(alice).Wait()
	notoSilver.ApproveTransfer(ctx, &nototypes.ApproveParams{
		Inputs:   transferSilver.InputStates,
		Outputs:  transferSilver.OutputStates,
		Data:     transferSilverExtra.ApprovalParams.Data,
		Delegate: transferAtom.Address,
	}).SignAndSend(bob).Wait()

	log.L(ctx).Infof("Execute the atomic operation")
	sent = transferAtom.Execute(ctx).SignAndSend(alice).Wait(5 * time.Second)
	require.NoError(t, sent.Error())

	if withHooks {
		assert.Equal(t, int64(9), tracker.GetBalance(ctx, aliceKey.Verifier.Verifier))
		assert.Equal(t, int64(1), tracker.GetBalance(ctx, bobKey.Verifier.Verifier))
	}
}

func resolveZetoKey(t *testing.T, ctx context.Context, rpc rpcbackend.Backend, domainName, identity string) (verifier string) {
	err := rpc.CallRPC(ctx, &verifier, "ptx_resolveVerifier", identity, zetosignerapi.AlgoDomainZetoSnarkBJJ(domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	require.Nil(t, err)
	return
}

func findAvailableCoins[T any](t *testing.T, ctx context.Context, rpc rpcbackend.Backend, domainName, coinSchemaID string, address *tktypes.EthAddress, jq *query.QueryJSON, readiness ...func(coins []*T) bool) []*T {
	if jq == nil {
		jq = query.NewQueryBuilder().Limit(100).Query()
	}
	var states []*T
notReady:
	for {
		rpcerr := rpc.CallRPC(ctx, &states, "pstate_queryContractStates",
			domainName,
			address,
			coinSchemaID,
			jq,
			"available")
		if rpcerr != nil {
			require.NoError(t, rpcerr.Error())
		}
		for _, fn := range readiness {
			if t.Failed() {
				panic("test failed")
			}
			if !fn(states) {
				time.Sleep(100 * time.Millisecond)
				continue notReady
			}
		}
		break
	}
	return states
}

func TestNotoForZeto(t *testing.T) {
	ctx := context.Background()
	log.L(ctx).Infof("TestNotoForZeto")

	hdWalletSeed := testbed.HDWalletSeedScopedToTest()

	notoDomainName := "noto_" + tktypes.RandHex(8)
	zetoDomainName := "zeto_" + tktypes.RandHex(8)
	log.L(ctx).Infof("Noto domain = %s", notoDomainName)
	log.L(ctx).Infof("Zeto domain = %s", zetoDomainName)

	log.L(ctx).Infof("Deploying factories")
	contractSource := map[string][]byte{
		"noto": helpers.NotoFactoryJSON,
		"atom": helpers.AtomFactoryJSON,
	}
	contracts := deployContracts(ctx, t, hdWalletSeed, notary, contractSource)
	for name, address := range contracts {
		log.L(ctx).Infof("%s deployed to %s", name, address)
	}

	log.L(ctx).Infof("Deploying Zeto dependencies")
	zetoContracts := zetotests.DeployZetoContracts(t, hdWalletSeed, "./zeto/config-for-deploy.yaml", notary)
	zetoConfig := zetotests.PrepareZetoConfig(t, zetoContracts, "../../domains/zeto/zkp")

	log.L(ctx).Infof("Initializing testbed")
	waitForNoto, notoTestbed := newNotoDomain(t, &nototypes.DomainConfig{
		FactoryAddress: contracts["noto"],
	})
	waitForZeto, zetoTestbed := newZetoDomain(t, zetoConfig, zetoContracts.FactoryAddress)
	done, _, tb, rpc := newTestbed(t, hdWalletSeed, map[string]*testbed.TestbedDomain{
		notoDomainName: notoTestbed,
		zetoDomainName: zetoTestbed,
	})
	defer done()
	pld := helpers.NewPaladinClient(t, ctx, tb)

	notoDomain := <-waitForNoto
	zetoDomain := <-waitForZeto

	tokenName := "Zeto_Anon"
	contractAbi, ok := zetoContracts.DeployedContractAbis[tokenName]
	require.True(t, ok, "Missing ABI for contract %s", tokenName)
	var result tktypes.HexBytes
	rpcerr := rpc.CallRPC(ctx, &result, "ptx_storeABI", contractAbi)
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}

	aliceKey, err := tb.ResolveKey(ctx, alice, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	bobKey, err := tb.ResolveKey(ctx, bob, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	atomFactory := helpers.InitAtom(t, tb, pld, contracts["atom"])

	log.L(ctx).Infof("Deploying Noto and Zeto")
	noto := helpers.DeployNoto(ctx, t, rpc, notoDomainName, notary, nil)
	zeto := helpers.DeployZeto(ctx, t, rpc, zetoDomainName, notary, tokenName)
	log.L(ctx).Infof("Noto deployed to %s", noto.Address)
	log.L(ctx).Infof("Zeto deployed to %s", zeto.Address)

	log.L(ctx).Infof("Mint 10 Noto to Alice")
	noto.Mint(ctx, alice, 10).SignAndSend(notary).Wait()
	log.L(ctx).Infof("Mint 10 Zeto to Bob")
	zeto.Mint(ctx, bob, 10).SignAndSend(notary).Wait()

	notoCoins := findAvailableCoins(t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), noto.Address, nil, func(coins []*nototypes.NotoCoinState) bool {
		return len(coins) >= 1
	})
	require.Len(t, notoCoins, 1)
	assert.Equal(t, int64(10), notoCoins[0].Data.Amount.Int().Int64())
	assert.Equal(t, aliceKey.Verifier.Verifier, notoCoins[0].Data.Owner.String())

	zetoCoins := findAvailableCoins(t, ctx, rpc, zetoDomain.Name(), zetoDomain.CoinSchemaID(), zeto.Address, nil, func(coins []*zetotypes.ZetoCoinState) bool {
		return len(coins) >= 1
	})
	require.NoError(t, err)
	require.Len(t, zetoCoins, 1)
	assert.Equal(t, int64(10), zetoCoins[0].Data.Amount.Int().Int64())
	bobsKey := resolveZetoKey(t, ctx, rpc, zetoDomain.Name(), bob)
	assert.Equal(t, bobsKey, zetoCoins[0].Data.Owner.String())

	// TODO: this should be a Pente private contract, instead of a base ledger contract
	log.L(ctx).Infof("Propose a trade of 1 Noto for 1 Zeto")
	swap := helpers.DeploySwap(ctx, t, tb, pld, alice, &helpers.TradeRequestInput{
		Holder1:       aliceKey.Verifier.Verifier,
		TokenAddress1: noto.Address,
		TokenValue1:   tktypes.Int64ToInt256(1),

		Holder2:       bobKey.Verifier.Verifier,
		TokenAddress2: zeto.Address,
		TokenValue2:   tktypes.Int64ToInt256(1),
	})

	log.L(ctx).Infof("Prepare the transfers")
	transferNoto := noto.Transfer(ctx, bob, 1).Prepare(alice)
	transferZeto := zeto.Transfer(ctx, alice, 1).Prepare(bob)
	zeto.Lock(ctx, tktypes.MustEthAddress(bobKey.Verifier.Verifier), transferZeto.EncodedCall).SignAndSend(bob).Wait()

	// TODO: this should actually be a Pente state transition
	log.L(ctx).Infof("Prepare the trade execute")
	encodedExecute := swap.Execute(ctx).Prepare()

	// TODO: should probably include the full encoded calls (including the zkp)
	log.L(ctx).Infof("Record the prepared transfers")
	sent := swap.Prepare(ctx, &helpers.StateData{
		Inputs:  transferNoto.InputStates,
		Outputs: transferNoto.OutputStates,
	}).SignAndSend(alice).Wait(5 * time.Second)
	require.NoError(t, sent.Error())
	sent = swap.Prepare(ctx, &helpers.StateData{
		Inputs:  transferZeto.InputStates,
		Outputs: transferZeto.OutputStates,
	}).SignAndSend(bob).Wait(5 * time.Second)
	require.NoError(t, sent.Error())

	prepared := swap.GetTrade(ctx)
	aliceData := prepared["userTradeData1"].(map[string]any)
	aliceStates := aliceData["states"].(map[string]any)
	bobData := prepared["userTradeData2"].(map[string]any)
	bobStates := bobData["states"].(map[string]any)
	log.L(ctx).Infof("Alice proposes tokens: contract=%s value=%s inputs=%+v outputs=%+v",
		aliceData["tokenAddress"], aliceData["tokenValue"], aliceStates["inputs"], aliceStates["outputs"])
	log.L(ctx).Infof("Bob proposes tokens: contract=%s value=%s inputs=%+v outputs=%+v",
		bobData["tokenAddress"], bobData["tokenValue"], bobStates["inputs"], bobStates["outputs"])

	var transferNotoExtra nototypes.NotoTransferMetadata
	err = json.Unmarshal(transferNoto.PreparedMetadata, &transferNotoExtra)
	require.NoError(t, err)

	log.L(ctx).Infof("Create Atom instance")
	transferAtom := atomFactory.Create(ctx, alice, []*helpers.AtomOperation{
		{
			ContractAddress: noto.Address,
			CallData:        transferNotoExtra.TransferWithApproval.EncodedCall,
		},
		{
			ContractAddress: zeto.Address,
			CallData:        transferZeto.EncodedCall,
		},
		{
			ContractAddress: swap.Address,
			CallData:        encodedExecute,
		},
	})

	atomOperations := transferAtom.GetOperations(ctx)
	for i, op := range atomOperations {
		log.L(ctx).Infof("Prepared operation %d: contract=%s calldata=%s", i, op["contractAddress"], op["callData"])
	}

	// TODO: all parties should verify the Atom against the original proposed trade
	// If any party found a discrepancy at this point, they could cancel the swap (last chance to back out)

	var transferNotoParams NotoTransferParams
	err = json.Unmarshal(transferNoto.PreparedTransaction.Data, &transferNotoParams)
	require.NoError(t, err)

	log.L(ctx).Infof("Approve both transfers")
	noto.ApproveTransfer(ctx, &nototypes.ApproveParams{
		Inputs:   transferNoto.InputStates,
		Outputs:  transferNoto.OutputStates,
		Data:     transferNotoParams.Data,
		Delegate: transferAtom.Address,
	}).SignAndSend(alice).Wait()
	zeto.Lock(ctx, transferAtom.Address, transferZeto.EncodedCall).SignAndSend(bob).Wait()

	log.L(ctx).Infof("Execute the atomic operation")
	sent = transferAtom.Execute(ctx).SignAndSend(alice).Wait(5 * time.Second)
	require.NoError(t, sent.Error())

	// TODO: better way to wait for events to be indexed after Atom execution
	time.Sleep(3 * time.Second)

	notoCoins = findAvailableCoins[nototypes.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), noto.Address, nil)
	require.NoError(t, err)
	require.Len(t, notoCoins, 2)
	assert.Equal(t, int64(1), notoCoins[0].Data.Amount.Int().Int64())
	assert.Equal(t, bobKey.Verifier.Verifier, notoCoins[0].Data.Owner.String())
	assert.Equal(t, int64(9), notoCoins[1].Data.Amount.Int().Int64())
	assert.Equal(t, aliceKey.Verifier.Verifier, notoCoins[1].Data.Owner.String())

	zetoCoins = findAvailableCoins[zetotypes.ZetoCoinState](t, ctx, rpc, zetoDomain.Name(), zetoDomain.CoinSchemaID(), zeto.Address, nil)
	require.NoError(t, err)
	require.Len(t, zetoCoins, 2)
	assert.Equal(t, int64(1), zetoCoins[0].Data.Amount.Int().Int64())
	alicesKey := resolveZetoKey(t, ctx, rpc, zetoDomain.Name(), alice)
	assert.Equal(t, alicesKey, zetoCoins[0].Data.Owner.String())
	assert.Equal(t, int64(9), zetoCoins[1].Data.Amount.Int().Int64())
	assert.Equal(t, bobsKey, zetoCoins[1].Data.Owner.String())
}
