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
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/domains/integration-test/helpers"
	nototypes "github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

var (
	notary = "notary"
	alice  = "alice"
	bob    = "bob"
)

type NotoTransferParams struct {
	Inputs  []interface{}    `json:"inputs"`
	Outputs []interface{}    `json:"outputs"`
	Data    tktypes.HexBytes `json:"data"`
}

func TestPvP(t *testing.T) {
	pvpNotoNoto(t, false)
}

func TestPvPWithNotoGuard(t *testing.T) {
	pvpNotoNoto(t, true)
}

func pvpNotoNoto(t *testing.T, withGuard bool) {
	ctx := context.Background()
	log.L(ctx).Infof("TestPvP (withGuard=%t)", withGuard)
	domainName := "noto_" + tktypes.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)

	log.L(ctx).Infof("Deploying factories")
	contractSource := map[string][]byte{
		"noto": helpers.NotoFactoryJSON,
		"atom": helpers.AtomFactoryJSON,
	}
	contracts := deployContracts(ctx, t, notary, contractSource)
	for name, address := range contracts {
		log.L(ctx).Infof("%s deployed to %s", name, address)
	}

	log.L(ctx).Infof("Initializing testbed")
	_, notoTestbed := newNotoDomain(t, &nototypes.DomainConfig{
		FactoryAddress: contracts["noto"],
	})
	done, tb, rpc := newTestbed(t, map[string]*testbed.TestbedDomain{
		domainName: notoTestbed,
	})
	defer done()

	_, aliceKey, err := tb.Components().KeyManager().ResolveKey(ctx, alice, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	_, bobKey, err := tb.Components().KeyManager().ResolveKey(ctx, bob, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	atomFactory := helpers.InitAtom(t, tb, rpc, contracts["atom"])

	var guardAddress *tktypes.EthAddress
	if withGuard {
		guard := helpers.DeployGuard(ctx, t, tb, notary)
		guardAddress = guard.Address
	}

	log.L(ctx).Infof("Deploying 2 instances of Noto")
	notoGold := helpers.DeployNoto(ctx, t, rpc, domainName, notary, guardAddress)
	notoSilver := helpers.DeployNoto(ctx, t, rpc, domainName, notary, nil)
	log.L(ctx).Infof("Noto gold deployed to %s", notoGold.Address)
	log.L(ctx).Infof("Noto silver deployed to %s", notoSilver.Address)

	log.L(ctx).Infof("Mint 10 gold to Alice")
	notoGold.Mint(ctx, alice, 10).SignAndSend(notary).Wait()
	log.L(ctx).Infof("Mint 100 silver to Bob")
	notoSilver.Mint(ctx, bob, 100).SignAndSend(notary).Wait()

	// TODO: this should be a Pente private contract, instead of a base ledger contract
	log.L(ctx).Infof("Propose a trade of 1 gold for 10 silver")
	swap := helpers.DeploySwap(ctx, t, tb, alice, &helpers.TradeRequestInput{
		Holder1:       aliceKey,
		TokenAddress1: notoGold.Address,
		TokenValue1:   tktypes.Int64ToInt256(1),

		Holder2:       bobKey,
		TokenAddress2: notoSilver.Address,
		TokenValue2:   tktypes.Int64ToInt256(10),
	})

	log.L(ctx).Infof("Prepare the transfers")
	transferGold := notoGold.Transfer(ctx, bob, 1).Prepare(alice)
	transferSilver := notoSilver.Transfer(ctx, alice, 10).Prepare(bob)

	// TODO: this should actually be a Pente state transition
	log.L(ctx).Infof("Prepare the trade execute")
	encodedExecute := swap.Execute(ctx).Prepare()

	log.L(ctx).Infof("Record the prepared transfers")
	swap.Prepare(ctx, &helpers.StateData{
		Inputs:  transferGold.InputStates,
		Outputs: transferGold.OutputStates,
	}).SignAndSend(alice).Wait()
	swap.Prepare(ctx, &helpers.StateData{
		Inputs:  transferSilver.InputStates,
		Outputs: transferSilver.OutputStates,
	}).SignAndSend(bob).Wait()

	var transferGoldParams NotoTransferParams
	err = json.Unmarshal(transferGold.ParamsJSON, &transferGoldParams)
	require.NoError(t, err)
	var transferSilverParams NotoTransferParams
	err = json.Unmarshal(transferSilver.ParamsJSON, &transferSilverParams)
	require.NoError(t, err)

	transferGoldEncoded, err := notoGold.ABI.Functions()["transferWithApproval"].EncodeCallDataJSONCtx(ctx, transferGold.ParamsJSON)
	require.NoError(t, err)
	transferSilverEncoded, err := notoSilver.ABI.Functions()["transferWithApproval"].EncodeCallDataJSONCtx(ctx, transferSilver.ParamsJSON)
	require.NoError(t, err)

	log.L(ctx).Infof("Create Atom instance")
	transferAtom := atomFactory.Create(ctx, alice, []*helpers.AtomOperation{
		{
			ContractAddress: notoGold.Address,
			CallData:        transferGoldEncoded,
		},
		{
			ContractAddress: notoSilver.Address,
			CallData:        transferSilverEncoded,
		},
		{
			ContractAddress: swap.Address,
			CallData:        tktypes.HexBytes(encodedExecute),
		},
	})

	// TODO: all parties should verify the Atom against the original proposed trade
	// If any party found a discrepancy at this point, they could cancel the swap (last chance to back out)

	log.L(ctx).Infof("Approve both Noto transactions")
	notoGold.ApproveTransfer(ctx, &nototypes.ApproveParams{
		Inputs:   transferGold.InputStates,
		Outputs:  transferGold.OutputStates,
		Data:     transferGoldParams.Data,
		Delegate: transferAtom.Address,
	}).SignAndSend(alice).Wait()
	notoSilver.ApproveTransfer(ctx, &nototypes.ApproveParams{
		Inputs:   transferSilver.InputStates,
		Outputs:  transferSilver.OutputStates,
		Data:     transferSilverParams.Data,
		Delegate: transferAtom.Address,
	}).SignAndSend(bob).Wait()

	log.L(ctx).Infof("Execute the atomic operation")
	transferAtom.Execute(ctx).SignAndSend(alice).Wait()
}

func TestNotoForZeto(t *testing.T) {
	ctx := context.Background()
	log.L(ctx).Infof("TestNotoForZeto")
	notoDomainName := "noto_" + tktypes.RandHex(8)
	zetoDomainName := "zeto_" + tktypes.RandHex(8)
	log.L(ctx).Infof("Noto domain = %s", notoDomainName)
	log.L(ctx).Infof("Zeto domain = %s", zetoDomainName)

	log.L(ctx).Infof("Deploying factories")
	contractSource := map[string][]byte{
		"noto": helpers.NotoFactoryJSON,
		"atom": helpers.AtomFactoryJSON,
	}
	contracts := deployContracts(ctx, t, notary, contractSource)
	for name, address := range contracts {
		log.L(ctx).Infof("%s deployed to %s", name, address)
	}

	log.L(ctx).Infof("Deploying Zeto dependencies")
	zetoContracts := deployZetoContracts(t, notary)
	zetoConfig := prepareZetoConfig(t, zetoContracts)

	log.L(ctx).Infof("Initializing testbed")
	notoDomain, notoTestbed := newNotoDomain(t, &nototypes.DomainConfig{
		FactoryAddress: contracts["noto"],
	})
	zetoDomain, zetoTestbed := newZetoDomain(t, zetoConfig)
	done, tb, rpc := newTestbed(t, map[string]*testbed.TestbedDomain{
		notoDomainName: notoTestbed,
		zetoDomainName: zetoTestbed,
	})
	defer done()

	_, aliceKey, err := tb.Components().KeyManager().ResolveKey(ctx, alice, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	_, bobKey, err := tb.Components().KeyManager().ResolveKey(ctx, bob, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	atomFactory := helpers.InitAtom(t, tb, rpc, contracts["atom"])

	log.L(ctx).Infof("Deploying Noto and Zeto")
	noto := helpers.DeployNoto(ctx, t, rpc, notoDomainName, notary, nil)
	zeto := helpers.DeployZeto(ctx, t, rpc, zetoDomainName, notary, "Zeto_Anon")
	log.L(ctx).Infof("Noto deployed to %s", noto.Address)
	log.L(ctx).Infof("Zeto deployed to %s", zeto.Address)

	log.L(ctx).Infof("Mint 10 Noto to Alice")
	noto.Mint(ctx, alice, 10).SignAndSend(notary).Wait()
	log.L(ctx).Infof("Mint 10 Zeto to Bob")
	zeto.Mint(ctx, bob, 10).SignAndSend(notary).Wait()

	notoCoins, err := (*notoDomain).FindCoins(ctx, noto.Address, "{}")
	require.NoError(t, err)
	require.Len(t, notoCoins, 1)
	assert.Equal(t, int64(10), notoCoins[0].Amount.Int().Int64())
	assert.Equal(t, aliceKey, notoCoins[0].Owner.String())

	zetoCoins, err := (*zetoDomain).FindCoins(ctx, zeto.Address, "{}")
	require.NoError(t, err)
	require.Len(t, zetoCoins, 1)
	assert.Equal(t, int64(10), zetoCoins[0].Amount.Int().Int64())
	assert.Equal(t, bob, zetoCoins[0].Owner) // TODO: this should really be bob's key, not by name

	// TODO: this should be a Pente private contract, instead of a base ledger contract
	log.L(ctx).Infof("Propose a trade of 1 Noto for 1 Zeto")
	swap := helpers.DeploySwap(ctx, t, tb, alice, &helpers.TradeRequestInput{
		Holder1:       aliceKey,
		TokenAddress1: noto.Address,
		TokenValue1:   tktypes.Int64ToInt256(1),

		Holder2:       bobKey,
		TokenAddress2: zeto.Address,
		TokenValue2:   tktypes.Int64ToInt256(1),
	})

	log.L(ctx).Infof("Prepare the transfers")
	transferNoto := noto.Transfer(ctx, bob, 1).Prepare(alice)
	transferZeto := zeto.Transfer(ctx, alice, 1).Prepare(bob)

	transferNotoEncoded, err := noto.ABI.Functions()["transferWithApproval"].EncodeCallDataJSONCtx(ctx, transferNoto.ParamsJSON)
	require.NoError(t, err)
	transferZetoEncoded, err := transferZeto.FunctionABI.EncodeCallDataJSONCtx(ctx, transferZeto.ParamsJSON)
	require.NoError(t, err)
	zeto.LockProof(ctx, tktypes.MustEthAddress(bobKey), transferZetoEncoded).SignAndSend(bob, false).Wait()

	// TODO: this should actually be a Pente state transition
	log.L(ctx).Infof("Prepare the trade execute")
	encodedExecute := swap.Execute(ctx).Prepare()

	// TODO: should probably include the full encoded calls (including the zkp)
	log.L(ctx).Infof("Record the prepared transfers")
	swap.Prepare(ctx, &helpers.StateData{
		Inputs:  transferNoto.InputStates,
		Outputs: transferNoto.OutputStates,
	}).SignAndSend(alice).Wait()
	swap.Prepare(ctx, &helpers.StateData{
		Inputs:  transferZeto.InputStates,
		Outputs: transferZeto.OutputStates,
	}).SignAndSend(bob).Wait()

	prepared := swap.GetTrade(ctx)
	aliceData := prepared["userTradeData1"].(map[string]any)
	aliceStates := aliceData["states"].(map[string]any)
	bobData := prepared["userTradeData2"].(map[string]any)
	bobStates := bobData["states"].(map[string]any)
	log.L(ctx).Infof("Alice proposes tokens: contract=%s value=%s inputs=%+v outputs=%+v",
		aliceData["tokenAddress"], aliceData["tokenValue"], aliceStates["inputs"], aliceStates["outputs"])
	log.L(ctx).Infof("Bob proposes tokens: contract=%s value=%s inputs=%+v outputs=%+v",
		bobData["tokenAddress"], bobData["tokenValue"], bobStates["inputs"], bobStates["outputs"])

	log.L(ctx).Infof("Create Atom instance")
	transferAtom := atomFactory.Create(ctx, alice, []*helpers.AtomOperation{
		{
			ContractAddress: noto.Address,
			CallData:        transferNotoEncoded,
		},
		{
			ContractAddress: zeto.Address,
			CallData:        transferZetoEncoded,
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
	err = json.Unmarshal(transferNoto.ParamsJSON, &transferNotoParams)
	require.NoError(t, err)

	log.L(ctx).Infof("Approve both transfers")
	noto.ApproveTransfer(ctx, &nototypes.ApproveParams{
		Inputs:   transferNoto.InputStates,
		Outputs:  transferNoto.OutputStates,
		Data:     transferNotoParams.Data,
		Delegate: transferAtom.Address,
	}).SignAndSend(alice).Wait()
	zeto.LockProof(ctx, transferAtom.Address, transferZetoEncoded).SignAndSend(bob, false).Wait()

	log.L(ctx).Infof("Execute the atomic operation")
	transferAtom.Execute(ctx).SignAndSend(alice).Wait()

	// TODO: better way to wait for events to be indexed after Atom execution
	time.Sleep(3 * time.Second)

	notoCoins, err = (*notoDomain).FindCoins(ctx, noto.Address, "{}")
	require.NoError(t, err)
	require.Len(t, notoCoins, 2)
	assert.Equal(t, int64(1), notoCoins[0].Amount.Int().Int64())
	assert.Equal(t, bobKey, notoCoins[0].Owner.String())
	assert.Equal(t, int64(9), notoCoins[1].Amount.Int().Int64())
	assert.Equal(t, aliceKey, notoCoins[1].Owner.String())

	zetoCoins, err = (*zetoDomain).FindCoins(ctx, zeto.Address, "{}")
	require.NoError(t, err)
	require.Len(t, zetoCoins, 2)
	assert.Equal(t, int64(1), zetoCoins[0].Amount.Int().Int64())
	assert.Equal(t, alice, zetoCoins[0].Owner)
	assert.Equal(t, int64(9), zetoCoins[1].Amount.Int().Int64())
	assert.Equal(t, bob, zetoCoins[1].Owner)
}
