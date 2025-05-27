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

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/domains/integration-test/helpers"
	"github.com/kaleido-io/paladin/domains/noto/pkg/noto"
	nototypes "github.com/kaleido-io/paladin/domains/noto/pkg/types"
	zetotypes "github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/query"
	"github.com/kaleido-io/paladin/sdk/go/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	notary = "notary"
	alice  = "alice"
	bob    = "bob"
)

func TestPvPSuite(t *testing.T) {
	suite.Run(t, new(pvpTestSuite))
}

type pvpTestSuite struct {
	suite.Suite
	hdWalletSeed       *testbed.UTInitFunction
	notoDomainName     string
	zetoDomainName     string
	notoFactoryAddress string
	atomFactoryAddress string
	zetoContracts      *helpers.ZetoDomainContracts
	zetoConfig         *zetotypes.DomainFactoryConfig
}

func (s *pvpTestSuite) SetupSuite() {
	ctx := context.Background()
	s.notoDomainName = "noto_" + pldtypes.RandHex(8)
	s.zetoDomainName = "zeto_" + pldtypes.RandHex(8)
	log.L(ctx).Infof("Noto domain = %s", s.notoDomainName)
	log.L(ctx).Infof("Zeto domain = %s", s.zetoDomainName)

	s.hdWalletSeed = testbed.HDWalletSeedScopedToTest()

	log.L(ctx).Infof("Deploying factories")
	contractSource := map[string][]byte{
		"noto": helpers.NotoFactoryJSON,
		"atom": helpers.AtomFactoryJSON,
	}
	contracts := deployContracts(ctx, s.T(), s.hdWalletSeed, notary, contractSource)
	for name, address := range contracts {
		log.L(ctx).Infof("%s deployed to %s", name, address)
	}
	s.notoFactoryAddress = contracts["noto"]
	s.atomFactoryAddress = contracts["atom"]

	log.L(ctx).Infof("Deploying Zeto dependencies")
	s.zetoContracts = helpers.DeployZetoContracts(s.T(), s.hdWalletSeed, "./zeto/config-for-deploy.yaml", notary)
	s.zetoConfig = helpers.PrepareZetoConfig(s.T(), s.zetoContracts, "../../domains/zeto/zkp")
}

func decodeTransactionResult(t *testing.T, resultInput map[string]any) *testbed.TransactionResult {
	resultJSON, err := json.Marshal(resultInput)
	require.NoError(t, err)
	var result testbed.TransactionResult
	err = json.Unmarshal(resultJSON, &result)
	require.NoError(t, err)
	return &result
}

// TODO: this should be retrieved from the domain receipt (not currently available in testbed)
func extractLockInfo(noto noto.Noto, invokeResult *testbed.TransactionResult) (*nototypes.ReceiptLockInfo, error) {
	for _, state := range invokeResult.InfoStates {
		if state.Schema.String() == noto.LockInfoSchemaID() {
			var lockInfo map[string]any
			err := json.Unmarshal(state.Data, &lockInfo)
			if err != nil {
				return nil, err
			}
			receiptInfo := &nototypes.ReceiptLockInfo{
				LockID: pldtypes.MustParseBytes32(lockInfo["lockId"].(string)),
			}
			return receiptInfo, nil
		}
	}
	return nil, nil
}

// TODO: this should be retrieved from the domain receipt (not currently available in testbed)
func buildUnlock(ctx context.Context, notoDomain noto.Noto, abi abi.ABI, prepareUnlockResult *testbed.TransactionResult) ([]*pldapi.StateEncoded, []*pldapi.StateEncoded, *nototypes.UnlockPublicParams, []byte, error) {
	notoInputStates := make([]*pldapi.StateEncoded, 0, len(prepareUnlockResult.ReadStates))
	notoOutputStates := make([]*pldapi.StateEncoded, 0, len(prepareUnlockResult.InfoStates))
	lockedInputs := make([]string, 0)
	lockedOutputs := make([]string, 0)
	unlockedOutputs := make([]string, 0)
	for _, input := range prepareUnlockResult.ReadStates {
		notoInputStates = append(notoInputStates, input)
		lockedInputs = append(lockedInputs, input.ID.String())
	}
	for _, output := range prepareUnlockResult.InfoStates {
		switch output.Schema.String() {
		case notoDomain.CoinSchemaID():
			notoOutputStates = append(notoOutputStates, output)
			unlockedOutputs = append(unlockedOutputs, output.ID.String())
		case notoDomain.LockedCoinSchemaID():
			notoOutputStates = append(notoOutputStates, output)
			lockedOutputs = append(lockedOutputs, output.ID.String())
		}
	}

	unlockParams := &nototypes.UnlockPublicParams{
		LockedInputs:  lockedInputs,
		LockedOutputs: lockedOutputs,
		Outputs:       unlockedOutputs,
		Signature:     pldtypes.HexBytes{},
		Data:          pldtypes.HexBytes{},
	}
	unlockParamsJSON, err := json.Marshal(unlockParams)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	encodedCall, err := abi.Functions()["unlock"].EncodeCallDataJSONCtx(ctx, unlockParamsJSON)
	return notoInputStates, notoOutputStates, unlockParams, encodedCall, err
}

func (s *pvpTestSuite) TestNotoForNoto() {
	s.pvpNotoNoto(false)
}

func (s *pvpTestSuite) TestNotoForNotoWithHooks() {
	s.pvpNotoNoto(true)
}

func (s *pvpTestSuite) pvpNotoNoto(withHooks bool) {
	ctx := context.Background()
	t := s.T()
	log.L(ctx).Infof("TestNotoForNoto (withHooks=%t)", withHooks)

	log.L(ctx).Infof("Initializing testbed")
	_, notoTestbed := newNotoDomain(t, &nototypes.DomainConfig{
		FactoryAddress: s.notoFactoryAddress,
	})
	done, _, tb, rpc := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.notoDomainName: notoTestbed,
	})
	defer done()
	pld := helpers.NewPaladinClient(t, ctx, tb)

	aliceKey, err := tb.ResolveKey(ctx, alice, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	bobKey, err := tb.ResolveKey(ctx, bob, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	atomFactory := helpers.InitAtom(t, tb, pld, s.atomFactoryAddress)

	var tracker *helpers.NotoTrackerHelper
	var trackerAddress *pldtypes.EthAddress
	if withHooks {
		tracker = helpers.DeployTracker(ctx, t, tb, pld, notary)
		trackerAddress = tracker.Address
	}

	log.L(ctx).Infof("Deploying 2 instances of Noto")
	notoGold := helpers.DeployNoto(ctx, t, rpc, s.notoDomainName, notary, trackerAddress)
	notoSilver := helpers.DeployNoto(ctx, t, rpc, s.notoDomainName, notary, nil)
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
		TokenValue1:   pldtypes.Int64ToInt256(1),

		Holder2:       bobKey.Verifier.Verifier,
		TokenAddress2: notoSilver.Address,
		TokenValue2:   pldtypes.Int64ToInt256(10),
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
			CallData:        pldtypes.HexBytes(encodedExecute),
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

func resolveZetoKey(t *testing.T, ctx context.Context, rpc rpcclient.Client, domainName, identity string) (verifier string) {
	err := rpc.CallRPC(ctx, &verifier, "ptx_resolveVerifier", identity, zetosignerapi.AlgoDomainZetoSnarkBJJ(domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	require.Nil(t, err)
	return
}

func (s *pvpTestSuite) TestNotoForZeto() {
	ctx := context.Background()
	t := s.T()
	log.L(ctx).Infof("TestNotoForZeto")

	log.L(ctx).Infof("Initializing testbed")
	waitForNoto, notoTestbed := newNotoDomain(t, &nototypes.DomainConfig{
		FactoryAddress: s.notoFactoryAddress,
	})
	waitForZeto, zetoTestbed := newZetoDomain(t, s.zetoConfig, s.zetoContracts.FactoryAddress)
	done, _, tb, rpc := newTestbed(t, s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		s.notoDomainName: notoTestbed,
		s.zetoDomainName: zetoTestbed,
	})
	defer done()
	pld := helpers.NewPaladinClient(t, ctx, tb)

	notoDomain := <-waitForNoto
	zetoDomain := <-waitForZeto

	tokenName := "Zeto_Anon"
	contractAbi, ok := s.zetoContracts.DeployedContractAbis[tokenName]
	require.True(t, ok, "Missing ABI for contract %s", tokenName)
	var result pldtypes.HexBytes
	rpcerr := rpc.CallRPC(ctx, &result, "ptx_storeABI", contractAbi)
	if rpcerr != nil {
		require.NoError(t, rpcerr)
	}

	aliceKey, err := tb.ResolveKey(ctx, alice, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	bobKey, err := tb.ResolveKey(ctx, bob, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	atomFactory := helpers.InitAtom(t, tb, pld, s.atomFactoryAddress)

	log.L(ctx).Infof("Deploying Noto and Zeto")
	noto := helpers.DeployNoto(ctx, t, rpc, s.notoDomainName, notary, nil)
	zeto := helpers.DeployZetoFungible(ctx, t, rpc, s.zetoDomainName, notary, tokenName)
	log.L(ctx).Infof("Noto deployed to %s", noto.Address)
	log.L(ctx).Infof("Zeto deployed to %s", zeto.Address)

	log.L(ctx).Infof("Mint 10 Noto to Alice")
	noto.Mint(ctx, alice, 10).SignAndSend(notary).Wait()
	log.L(ctx).Infof("Mint 10 Zeto to Bob")
	zeto.Mint(ctx, bob, []uint64{10}).SignAndSend(notary).Wait()

	notoCoins := findAvailableCoins(t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil, func(coins []*nototypes.NotoCoinState) bool {
		return len(coins) >= 1
	})
	require.Len(t, notoCoins, 1)
	assert.Equal(t, int64(10), notoCoins[0].Data.Amount.Int().Int64())
	assert.Equal(t, aliceKey.Verifier.Verifier, notoCoins[0].Data.Owner.String())

	zetoCoins := findAvailableCoins(t, ctx, rpc, zetoDomain.Name(), zetoDomain.CoinSchemaID(), "pstate_queryContractStates", zeto.Address, nil, func(coins []*zetotypes.ZetoCoinState) bool {
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
		TokenValue1:   pldtypes.Int64ToInt256(1),

		Holder2:       bobKey.Verifier.Verifier,
		TokenAddress2: zeto.Address,
		TokenValue2:   pldtypes.Int64ToInt256(1),
	})

	log.L(ctx).Infof("Prepare the Noto transfer")
	notoLock := noto.Lock(ctx, &nototypes.LockParams{
		Amount: pldtypes.Int64ToInt256(1),
	}).SignAndSend(alice).Wait()
	notoLockResult := decodeTransactionResult(t, notoLock)

	lockInfo, err := extractLockInfo(notoDomain, notoLockResult)
	require.NoError(t, err)
	require.NotNil(t, lockInfo)
	require.NotEmpty(t, lockInfo.LockID)

	time.Sleep(1 * time.Second) // TODO: remove
	notoPrepareUnlock := noto.PrepareUnlock(ctx, &nototypes.UnlockParams{
		LockID: lockInfo.LockID,
		From:   alice,
		Recipients: []*nototypes.UnlockRecipient{{
			To:     bob,
			Amount: pldtypes.Int64ToInt256(1),
		}},
	}).SignAndSend(alice).Wait()
	require.NotNil(t, notoPrepareUnlock)
	prepareUnlockResult := decodeTransactionResult(t, notoPrepareUnlock)

	notoInputStates, notoOutputStates, unlockParams, transferNoto, err := buildUnlock(ctx, notoDomain, noto.ABI, prepareUnlockResult)
	require.NoError(t, err)

	log.L(ctx).Infof("Prepare the Zeto transfer")
	zeto.Lock(ctx, pldtypes.MustEthAddress(bobKey.Verifier.Verifier), 1).SignAndSend(bob).Wait()

	jq := query.NewQueryBuilder().Limit(100).Equal("locked", true).Query()
	lockedZetoCoins := findAvailableCoins(t, ctx, rpc, zetoDomain.Name(), zetoDomain.CoinSchemaID(), "pstate_queryContractStates", zeto.Address, jq, func(coins []*zetotypes.ZetoCoinState) bool {
		locked := len(coins) >= 1
		if locked {
			log.L(ctx).Infof("Found %d locked Zeto coins", len(coins))
			for _, coin := range coins {
				hash, err := coin.Data.Hash(ctx)
				require.NoError(t, err)
				log.L(ctx).Infof("Locked Zeto coin: amount=%s, locked=%t, hash=%s\n", coin.Data.Amount.String(), coin.Data.Locked, hash.String())
			}
		}
		return locked
	})
	lockedZeto, _ := lockedZetoCoins[0].Data.Hash(ctx)

	transferZeto := zeto.TransferLocked(ctx, lockedZeto, bobKey.Verifier.Verifier, alice, 1).Prepare(bob)

	// TODO: this should actually be a Pente state transition
	log.L(ctx).Infof("Prepare the trade execute")
	encodedExecute := swap.Execute(ctx).Prepare()

	// TODO: should probably include the full encoded calls (including the zkp)
	log.L(ctx).Infof("Record the prepared transfers")
	sent := swap.Prepare(ctx, &helpers.StateData{
		Inputs:  notoInputStates,
		Outputs: notoOutputStates,
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

	log.L(ctx).Infof("Create Atom instance")
	transferAtom := atomFactory.Create(ctx, alice, []*helpers.AtomOperation{
		{
			ContractAddress: noto.Address,
			CallData:        transferNoto,
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

	log.L(ctx).Infof("Approve both transfers")
	noto.DelegateLock(ctx, &nototypes.DelegateLockParams{
		LockID:   lockInfo.LockID,
		Unlock:   unlockParams,
		Delegate: transferAtom.Address,
	}).SignAndSend(alice).Wait()
	zeto.DelegateLock(ctx, tb, lockedZeto, transferAtom.Address, bobKey.Identifier)

	log.L(ctx).Infof("Execute the atomic operation")
	sent = transferAtom.Execute(ctx).SignAndSend(alice).Wait(5 * time.Second)
	require.NoError(t, sent.Error())

	// TODO: better way to wait for events to be indexed after Atom execution
	time.Sleep(1 * time.Second)

	notoCoins = findAvailableCoins[nototypes.NotoCoinState](t, ctx, rpc, notoDomain.Name(), notoDomain.CoinSchemaID(), "pstate_queryContractStates", noto.Address, nil)
	require.NoError(t, err)
	require.Len(t, notoCoins, 2)
	assert.Equal(t, int64(9), notoCoins[0].Data.Amount.Int().Int64())
	assert.Equal(t, aliceKey.Verifier.Verifier, notoCoins[0].Data.Owner.String())
	assert.Equal(t, int64(1), notoCoins[1].Data.Amount.Int().Int64())
	assert.Equal(t, bobKey.Verifier.Verifier, notoCoins[1].Data.Owner.String())

	zetoCoins = findAvailableCoins[zetotypes.ZetoCoinState](t, ctx, rpc, zetoDomain.Name(), zetoDomain.CoinSchemaID(), "pstate_queryContractStates", zeto.Address, nil)
	require.NoError(t, err)
	require.Len(t, zetoCoins, 2)
	assert.Equal(t, int64(1), zetoCoins[1].Data.Amount.Int().Int64())
	alicesKey := resolveZetoKey(t, ctx, rpc, zetoDomain.Name(), alice)
	assert.Equal(t, alicesKey, zetoCoins[1].Data.Owner.String())
	assert.Equal(t, int64(9), zetoCoins[0].Data.Amount.Int().Int64())
	assert.Equal(t, bobsKey, zetoCoins[0].Data.Owner.String())
}
