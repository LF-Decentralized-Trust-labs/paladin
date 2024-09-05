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
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/domains/noto/pkg/noto"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
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

var (
	notaryName     = "notary1"
	recipient1Name = "recipient1"
	recipient2Name = "recipient2"
)

type OperationInput struct {
	ContractAddress ethtypes.Address0xHex     `json:"contractAddress"`
	CallData        ethtypes.HexBytes0xPrefix `json:"callData"`
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

func notoPrepareTransfer(ctx context.Context, t *testing.T, rpc rpcbackend.Backend, notoAddress ethtypes.Address0xHex, from, to string, amount int64) []byte {
	var encodedCall []byte
	rpcerr := rpc.CallRPC(ctx, &encodedCall, "testbed_prepare", &tktypes.PrivateContractInvoke{
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
	return encodedCall
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

func findEvent(t *testing.T, events []*blockindexer.EventWithData, abi abi.ABI, eventName string) *blockindexer.EventWithData {
	targetEvent := abi.Events()[eventName]
	assert.NotNil(t, targetEvent)
	assert.NotEmpty(t, targetEvent.SolString())
	for _, event := range events {
		if event.SoliditySignature == targetEvent.SolString() {
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

	// Prepare ABI clients for direct calls
	eth := tb.Components().EthClientFactory().HTTPClient()
	atomFactoryBuild := domain.LoadBuild(atomFactoryJSON)
	atomFactory, err := eth.ABI(ctx, atomFactoryBuild.ABI)
	assert.NoError(t, err)
	atomBuild := domain.LoadBuild(atomJSON)
	atom, err := eth.ABI(ctx, atomBuild.ABI)
	assert.NoError(t, err)

	log.L(ctx).Infof("Deploying 2 instances of Noto")
	notoGoldAddress := notoDeploy(ctx, t, rpc, domainName, notaryName)
	notoSilverAddress := notoDeploy(ctx, t, rpc, domainName, notaryName)
	log.L(ctx).Infof("Noto gold deployed to %s", notoGoldAddress)
	log.L(ctx).Infof("Noto silver deployed to %s", notoSilverAddress)

	log.L(ctx).Infof("Mint 10 gold to recipient 1")
	notoMint(ctx, t, rpc, notoGoldAddress, notaryName, recipient1Name, 10)
	log.L(ctx).Infof("Mint 100 silver to recipient 2")
	notoMint(ctx, t, rpc, notoSilverAddress, notaryName, recipient2Name, 100)

	log.L(ctx).Infof("Prepare an exchange of 1 gold for 10 silver")
	transferGold := notoPrepareTransfer(ctx, t, rpc, notoGoldAddress, recipient1Name, recipient2Name, 1)
	transferSiler := notoPrepareTransfer(ctx, t, rpc, notoSilverAddress, recipient2Name, recipient1Name, 10)

	log.L(ctx).Infof("Create Atom instance")
	create, err := atomFactory.Function(ctx, "create")
	assert.NoError(t, err)
	txHash, err := create.R(ctx).
		Signer(recipient1Name).
		To(atomAddress).
		Input(map[string][]*OperationInput{
			"operations": {
				{
					ContractAddress: notoGoldAddress,
					CallData:        transferGold,
				},
				{
					ContractAddress: notoSilverAddress,
					CallData:        transferSiler,
				},
			},
		}).
		SignAndSend()
	assert.NoError(t, err)
	_, err = tb.Components().BlockIndexer().WaitForTransaction(ctx, *txHash)
	assert.NoError(t, err)
	events, err := tb.Components().BlockIndexer().DecodeTransactionEvents(ctx, *txHash, atomFactory.ABI())
	assert.NoError(t, err)
	deployedEvent := findEvent(t, events, atomFactory.ABI(), "AtomDeployed")
	assert.NotNil(t, deployedEvent)
	var deployedParams AtomDeployed
	err = json.Unmarshal(deployedEvent.Data, &deployedParams)
	assert.NoError(t, err)
	assert.NotEmpty(t, deployedParams.Address)

	log.L(ctx).Infof("Approve both Noto transactions")
	notoApprove(ctx, t, rpc, notoGoldAddress, recipient1Name, deployedParams.Address, transferGold)
	notoApprove(ctx, t, rpc, notoSilverAddress, recipient2Name, deployedParams.Address, transferSiler)

	log.L(ctx).Infof("Execute the atomic operation")
	execute, err := atom.Function(ctx, "execute")
	assert.NoError(t, err)
	txHash, err = execute.R(ctx).
		Signer(recipient1Name).
		To(&deployedParams.Address).
		SignAndSend()
	require.NoError(t, err)
	_, err = tb.Components().BlockIndexer().WaitForTransaction(ctx, *txHash)
	assert.NoError(t, err)
}
