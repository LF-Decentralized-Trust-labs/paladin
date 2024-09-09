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

package test

import (
	"context"
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/go-resty/resty/v2"

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/domains/noto/pkg/noto"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed abis/NotoFactory.json
var notoFactoryJSON []byte // From "gradle copySolidity"

//go:embed abis/AtomFactory.json
var atomFactoryJSON []byte // From "gradle copySolidity"

//go:embed abis/Atom.json
var atomJSON []byte // From "gradle copySolidity"

//go:embed abis/Swap.json
var swapJSON []byte // From "gradle copySolidity"

var (
	notaryName     = "notary1"
	recipient1Name = "recipient1"
	recipient2Name = "recipient2"
)

type AtomOperation struct {
	ContractAddress ethtypes.Address0xHex     `json:"contractAddress"`
	CallData        ethtypes.HexBytes0xPrefix `json:"callData"`
}

type TradeRequestInput struct {
	Holder1       string                    `json:"holder1"`
	Holder2       string                    `json:"holder2"`
	TokenAddress1 ethtypes.Address0xHex     `json:"tokenAddress1"`
	TokenAddress2 ethtypes.Address0xHex     `json:"tokenAddress2"`
	TokenValue1   *ethtypes.HexInteger      `json:"tokenValue1"`
	TokenValue2   *ethtypes.HexInteger      `json:"tokenValue2"`
	TradeData1    ethtypes.HexBytes0xPrefix `json:"tradeData1"`
	TradeData2    ethtypes.HexBytes0xPrefix `json:"tradeData2"`
}

type StateData struct {
	Inputs  []*tktypes.FullState `json:"inputs"`
	Outputs []*tktypes.FullState `json:"outputs"`
}

type AtomDeployed struct {
	Address ethtypes.Address0xHex `json:"addr"`
}

func toJSON(t *testing.T, v any) []byte {
	result, err := json.Marshal(v)
	assert.NoError(t, err)
	return result
}

func mapConfig(t *testing.T, config *types.Config) (m map[string]any) {
	configJSON, err := json.Marshal(&config)
	assert.NoError(t, err)
	err = json.Unmarshal(configJSON, &m)
	assert.NoError(t, err)
	return m
}

func deployContracts(ctx context.Context, t *testing.T, contracts map[string][]byte) map[string]string {
	tb := testbed.NewTestBed()
	url, done, err := tb.StartForTest("./testbed.config.yaml", map[string]*testbed.TestbedDomain{})
	assert.NoError(t, err)
	defer done()
	rpc := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))

	deployed := make(map[string]string, len(contracts))
	for name, contract := range contracts {
		build := domain.LoadBuild(contract)
		var addr string
		rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode",
			notaryName, build.ABI, build.Bytecode.String(), `{}`)
		if rpcerr != nil {
			assert.NoError(t, rpcerr.Error())
		}
		deployed[name] = addr
	}
	return deployed
}

func newNotoDomain(t *testing.T, config *types.Config) (*noto.Noto, *testbed.TestbedDomain) {
	var domain noto.Noto
	return &domain, &testbed.TestbedDomain{
		Config: mapConfig(t, config),
		Plugin: plugintk.NewDomain(func(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {
			domain = noto.New(callbacks)
			return domain
		}),
	}
}

func notoDeploy(ctx context.Context, t *testing.T, rpc rpcbackend.Backend, domainName, notary string) ethtypes.Address0xHex {
	var addr ethtypes.Address0xHex
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deploy",
		domainName, &types.ConstructorParams{Notary: notary})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	return addr
}

func notoMint(ctx context.Context, t *testing.T, rpc rpcbackend.Backend, notoAddress ethtypes.Address0xHex, from, to string, amount int64) {
	var result bool
	rpcerr := rpc.CallRPC(ctx, &result, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     from,
		To:       tktypes.EthAddress(notoAddress),
		Function: *types.NotoABI.Functions()["mint"],
		Inputs: toJSON(t, &types.MintParams{
			To:     to,
			Amount: ethtypes.NewHexInteger64(amount),
		}),
	})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}
	assert.True(t, result)
}

func notoPrepareTransfer(ctx context.Context, t *testing.T, rpc rpcbackend.Backend, notoAddress ethtypes.Address0xHex, from, to string, amount int64) *tktypes.PrivateContractPreparedTransaction {
	var prepared tktypes.PrivateContractPreparedTransaction
	rpcerr := rpc.CallRPC(ctx, &prepared, "testbed_prepare", &tktypes.PrivateContractInvoke{
		From:     from,
		To:       tktypes.EthAddress(notoAddress),
		Function: *types.NotoABI.Functions()["approvedTransfer"],
		Inputs: toJSON(t, &types.TransferParams{
			To:     to,
			Amount: ethtypes.NewHexInteger64(amount),
		}),
	})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}
	return &prepared
}

func notoApprove(ctx context.Context, t *testing.T, rpc rpcbackend.Backend, notoAddress ethtypes.Address0xHex, from string, delegate ethtypes.Address0xHex, call []byte) {
	var result bool
	rpcerr := rpc.CallRPC(ctx, &result, "testbed_invoke", &tktypes.PrivateContractInvoke{
		From:     from,
		To:       tktypes.EthAddress(notoAddress),
		Function: *types.NotoABI.Functions()["approve"],
		Inputs: toJSON(t, &types.ApproveParams{
			Delegate: delegate,
			Call:     call,
		}),
	})
	if rpcerr != nil {
		require.NoError(t, rpcerr.Error())
	}
	assert.True(t, result)
}

func deployBuilder(ctx context.Context, t *testing.T, eth ethclient.EthClient, abi abi.ABI, bytecode []byte) ethclient.ABIFunctionRequestBuilder {
	abiClient, err := eth.ABI(ctx, abi)
	assert.NoError(t, err)
	construct, err := abiClient.Constructor(ctx, bytecode)
	assert.NoError(t, err)
	return construct.R(ctx)
}

func functionBuilder(ctx context.Context, t *testing.T, eth ethclient.EthClient, abi abi.ABI, functionName string) ethclient.ABIFunctionRequestBuilder {
	abiClient, err := eth.ABI(ctx, abi)
	assert.NoError(t, err)
	fn, err := abiClient.Function(ctx, functionName)
	assert.NoError(t, err)
	return fn.R(ctx)
}

func waitFor(ctx context.Context, t *testing.T, tb testbed.Testbed, txHash *tktypes.Bytes32, err error) *blockindexer.IndexedTransaction {
	require.NoError(t, err)
	tx, err := tb.Components().BlockIndexer().WaitForTransaction(ctx, *txHash)
	assert.NoError(t, err)
	return tx
}

func findEvent(ctx context.Context, t *testing.T, tb testbed.Testbed, txHash tktypes.Bytes32, abi abi.ABI, eventName string, eventParams interface{}) *blockindexer.EventWithData {
	targetEvent := abi.Events()[eventName]
	assert.NotNil(t, targetEvent)
	assert.NotEmpty(t, targetEvent.SolString())
	events, err := tb.Components().BlockIndexer().DecodeTransactionEvents(ctx, txHash, abi)
	assert.NoError(t, err)
	for _, event := range events {
		if event.SoliditySignature == targetEvent.SolString() {
			err = json.Unmarshal(event.Data, eventParams)
			assert.NoError(t, err)
			return event
		}
	}
	return nil
}

func newTestbed(t *testing.T, domains map[string]*testbed.TestbedDomain) (context.CancelFunc, testbed.Testbed, rpcbackend.Backend) {
	tb := testbed.NewTestBed()
	url, done, err := tb.StartForTest("./testbed.config.yaml", domains)
	assert.NoError(t, err)
	rpc := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))
	return done, tb, rpc
}

func TestPvP(t *testing.T) {
	ctx := context.Background()
	log.L(ctx).Infof("TestPvP")
	domainName := "noto_" + tktypes.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)

	log.L(ctx).Infof("Deploying factories")
	contractSource := map[string][]byte{
		"noto": notoFactoryJSON,
		"atom": atomFactoryJSON,
	}
	contracts := deployContracts(ctx, t, contractSource)
	for name, address := range contracts {
		log.L(ctx).Infof("%s deployed to %s", name, address)
	}

	_, notoTestbed := newNotoDomain(t, &types.Config{
		FactoryAddress: contracts["noto"],
	})
	done, tb, rpc := newTestbed(t, map[string]*testbed.TestbedDomain{
		domainName: notoTestbed,
	})
	defer done()

	atomAddress, err := ethtypes.NewAddress(contracts["atom"])
	assert.NoError(t, err)

	_, recipient1Key, err := tb.Components().KeyManager().ResolveKey(ctx, recipient1Name, algorithms.ECDSA_SECP256K1_PLAINBYTES)
	require.NoError(t, err)
	_, recipient2Key, err := tb.Components().KeyManager().ResolveKey(ctx, recipient2Name, algorithms.ECDSA_SECP256K1_PLAINBYTES)
	require.NoError(t, err)

	eth := tb.Components().EthClientFactory().HTTPClient()
	atomFactory := domain.LoadBuild(atomFactoryJSON)
	atom := domain.LoadBuild(atomJSON)
	swap := domain.LoadBuild(swapJSON)

	log.L(ctx).Infof("Deploying 2 instances of Noto")
	notoGoldAddress := notoDeploy(ctx, t, rpc, domainName, notaryName)
	notoSilverAddress := notoDeploy(ctx, t, rpc, domainName, notaryName)
	log.L(ctx).Infof("Noto gold deployed to %s", notoGoldAddress)
	log.L(ctx).Infof("Noto silver deployed to %s", notoSilverAddress)

	log.L(ctx).Infof("Mint 10 gold to recipient 1")
	notoMint(ctx, t, rpc, notoGoldAddress, notaryName, recipient1Name, 10)
	log.L(ctx).Infof("Mint 100 silver to recipient 2")
	notoMint(ctx, t, rpc, notoSilverAddress, notaryName, recipient2Name, 100)

	// TODO: this should be a Pente private contract, instead of a base ledger contract
	log.L(ctx).Infof("Propose a trade of 1 gold for 10 silver")
	txHash, err := deployBuilder(ctx, t, eth, swap.ABI, swap.Bytecode).
		Signer(recipient1Name).
		Input(toJSON(t, map[string]any{
			"inputData": TradeRequestInput{
				Holder1:       recipient1Key,
				TokenAddress1: notoGoldAddress,
				TokenValue1:   ethtypes.NewHexInteger64(1),

				Holder2:       recipient2Key,
				TokenAddress2: notoSilverAddress,
				TokenValue2:   ethtypes.NewHexInteger64(10),
			},
		})).
		SignAndSend()
	swapDeploy := waitFor(ctx, t, tb, txHash, err)
	swapAddress := ethtypes.Address0xHex(*swapDeploy.ContractAddress)

	log.L(ctx).Infof("Prepare the transfers")
	transferGold := notoPrepareTransfer(ctx, t, rpc, notoGoldAddress, recipient1Name, recipient2Name, 1)
	transferSilver := notoPrepareTransfer(ctx, t, rpc, notoSilverAddress, recipient2Name, recipient1Name, 10)

	log.L(ctx).Infof("Record the prepared transfers")
	txHash1, err1 := functionBuilder(ctx, t, eth, swap.ABI, "prepare").
		Signer(recipient1Name).
		To(&swapAddress).
		Input(toJSON(t, map[string]any{
			"holder": recipient1Name,
			"states": StateData{
				Inputs:  transferGold.InputStates,
				Outputs: transferGold.OutputStates,
			},
		})).
		SignAndSend()
	txHash2, err2 := functionBuilder(ctx, t, eth, swap.ABI, "prepare").
		Signer(recipient2Name).
		To(&swapAddress).
		Input(toJSON(t, map[string]any{
			"holder": recipient2Name,
			"states": &StateData{
				Inputs:  transferSilver.InputStates,
				Outputs: transferSilver.OutputStates,
			},
		})).
		SignAndSend()
	waitFor(ctx, t, tb, txHash1, err1)
	waitFor(ctx, t, tb, txHash2, err2)

	log.L(ctx).Infof("Prepare the trade execute")
	executeBuilder := functionBuilder(ctx, t, eth, swap.ABI, "execute").
		Signer(recipient1Name).
		To(&swapAddress)
	err = executeBuilder.BuildCallData()
	require.NoError(t, err)

	log.L(ctx).Infof("Create Atom instance")
	txHash, err = functionBuilder(ctx, t, eth, atomFactory.ABI, "create").
		Signer(recipient1Name).
		To(atomAddress).
		Input(toJSON(t, map[string]any{
			"operations": []*AtomOperation{
				{
					ContractAddress: notoGoldAddress,
					CallData:        transferGold.EncodedCall,
				},
				{
					ContractAddress: notoSilverAddress,
					CallData:        transferSilver.EncodedCall,
				},
				{
					ContractAddress: swapAddress,
					CallData:        executeBuilder.TX().Data,
				},
			},
		})).
		SignAndSend()
	waitFor(ctx, t, tb, txHash, err)

	var atomDeployed AtomDeployed
	findEvent(ctx, t, tb, *txHash, atomFactory.ABI, "AtomDeployed", &atomDeployed)
	assert.NotEmpty(t, atomDeployed.Address)

	log.L(ctx).Infof("Approve both Noto transactions")
	notoApprove(ctx, t, rpc, notoGoldAddress, recipient1Name, atomDeployed.Address, transferGold.EncodedCall)
	notoApprove(ctx, t, rpc, notoSilverAddress, recipient2Name, atomDeployed.Address, transferSilver.EncodedCall)

	log.L(ctx).Infof("Accept the swap")
	txHash1, err1 = functionBuilder(ctx, t, eth, swap.ABI, "accept").
		Signer(recipient1Name).
		To(&swapAddress).
		SignAndSend()
	txHash2, err2 = functionBuilder(ctx, t, eth, swap.ABI, "accept").
		Signer(recipient2Name).
		To(&swapAddress).
		SignAndSend()
	waitFor(ctx, t, tb, txHash1, err1)
	waitFor(ctx, t, tb, txHash2, err2)

	log.L(ctx).Infof("Execute the atomic operation")
	txHash, err = functionBuilder(ctx, t, eth, atom.ABI, "execute").
		Signer(recipient1Name).
		To(&atomDeployed.Address).
		SignAndSend()
	waitFor(ctx, t, tb, txHash, err)
}
