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

/*
Test Kata component with no mocking of any internal units.
Starts the GRPC server and drives the internal functions via GRPC messages
*/
package coordinationtest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	testutils "github.com/LF-Decentralized-Trust-labs/paladin/core/noderuntests/pkg"
	domains "github.com/LF-Decentralized-Trust-labs/paladin/core/noderuntests/pkg/domains"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/solutils"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var CONFIG_PATH = "../../test/config/postgres.config.yaml"

func deployDomainRegistry(t *testing.T) *pldtypes.EthAddress {
	return testutils.DeployDomainRegistry(t, CONFIG_PATH)
}

func newInstanceForComponentTesting(t *testing.T, deployDomainRegistry func(*testing.T) *pldtypes.EthAddress, binding *any, peerNodes []interface{}, domainConfig interface{}, enableWS bool) testutils.ComponentTestInstance {
	return testutils.NewInstanceForComponentTesting(t, deployDomainRegistry(t), binding, peerNodes, domainConfig, enableWS, CONFIG_PATH)
}

func subscribeAndSendDataToChannel(ctx context.Context, t *testing.T, wsClient pldclient.PaladinWSClient, listenerName string, data chan string) {
	sub, err := wsClient.PTX().SubscribeBlockchainEvents(ctx, listenerName)
	require.NoError(t, err)
	go func() {
		for {
			select {
			case subNotification, ok := <-sub.Notifications():
				if ok {
					eventData := make([]string, 0)
					var batch pldapi.TransactionEventBatch
					_ = json.Unmarshal(subNotification.GetResult(), &batch)
					for _, e := range batch.Events {
						t.Logf("Received event on %s from %d/%d/%d : %s", listenerName, e.BlockNumber, e.TransactionIndex, e.LogIndex, e.Data.String())
						eventData = append(eventData, e.Data.String())
					}
					require.NoError(t, subNotification.Ack(ctx))
					// send after the ack otherwise the main test can complete when it receives the last values and the websocket is closed before the ack
					// can be sent
					for _, d := range eventData {
						data <- d
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func TestRunSimpleStorageEthTransaction2(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	logrus.SetLevel(logrus.DebugLevel)

	instance := newInstanceForComponentTesting(t, deployDomainRegistry, nil, nil, nil, true)
	c := pldclient.Wrap(instance.GetClient()).ReceiptPollingInterval(250 * time.Millisecond)

	build, err := solutils.LoadBuild(ctx, simpleStorageBuildJSON)
	require.NoError(t, err)

	simpleStorage := c.ForABI(ctx, build.ABI).Public().From("key1")

	res := simpleStorage.Clone().
		Constructor().
		Bytecode(build.Bytecode).
		Inputs(`{"x":11223344}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())
	contractAddr := res.Receipt().ContractAddress

	// set up the event listener
	success, err := c.PTX().CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name: "listener1",
		Sources: []pldapi.BlockchainEventListenerSource{{
			ABI:     abi.ABI{build.ABI.Events()["Changed"]},
			Address: contractAddr,
		}},
	})
	require.NoError(t, err)
	require.True(t, success)

	wsClient, err := c.WebSocket(ctx, instance.GetWSConfig())
	require.NoError(t, err)

	eventData := make(chan string)
	subscribeAndSendDataToChannel(ctx, t, wsClient, "listener1", eventData)

	success, err = c.PTX().StartBlockchainEventListener(ctx, "listener1")
	require.NoError(t, err)
	require.True(t, success)

	data := <-eventData
	assert.JSONEq(t, `{"x":"11223344"}`, data)

	var getX pldtypes.RawJSON
	err = simpleStorage.Clone().
		Function("get").
		To(contractAddr).
		Outputs(&getX).
		Call()
	require.NoError(t, err)
	assert.JSONEq(t, `{"x":"11223344"}`, getX.Pretty())

	res = simpleStorage.Clone().
		Function("set").
		To(contractAddr).
		Inputs(`{"_x":99887766}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())

	data = <-eventData
	assert.JSONEq(t, `{"x":"99887766"}`, data)

	res = simpleStorage.Clone().
		Function("set").
		To(contractAddr).
		Inputs(`{"_x":1234}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())

	data = <-eventData
	assert.JSONEq(t, `{"x":"1234"}`, data)
}

func TestBlockchainEventListeners2(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	logrus.SetLevel(logrus.DebugLevel)

	instance := newInstanceForComponentTesting(t, deployDomainRegistry, nil, nil, nil, true)
	c := pldclient.Wrap(instance.GetClient()).ReceiptPollingInterval(250 * time.Millisecond)

	build, err := solutils.LoadBuild(ctx, simpleStorageBuildJSON)
	require.NoError(t, err)

	simpleStorage := c.ForABI(ctx, build.ABI).Public().From("key1")

	res := simpleStorage.Clone().
		Constructor().
		Bytecode(build.Bytecode).
		Inputs(`{"x":1}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())
	contractAddr := res.Receipt().ContractAddress
	deployBlock := res.Receipt().BlockNumber

	// set up the event listener
	_, err = c.PTX().CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name:    "listener1",
		Started: confutil.P(false),
		Sources: []pldapi.BlockchainEventListenerSource{{
			ABI:     abi.ABI{build.ABI.Events()["Changed"]},
			Address: contractAddr,
		}},
	})
	require.NoError(t, err)

	status, err := c.PTX().GetBlockchainEventListenerStatus(ctx, "listener1")
	require.NoError(t, err)
	assert.Equal(t, int64(-1), status.Checkpoint.BlockNumber)

	wsClient, err := c.WebSocket(ctx, instance.GetWSConfig())
	require.NoError(t, err)

	listener1 := make(chan string)
	subscribeAndSendDataToChannel(ctx, t, wsClient, "listener1", listener1)

	require.Never(t, func() bool {
		select {
		case <-listener1:
			return true
		default:
			return false
		}
	}, 100*time.Millisecond, 5*time.Millisecond, "unexpected event received on stopped listener")

	_, err = c.PTX().StartBlockchainEventListener(ctx, "listener1")
	require.NoError(t, err)

	assert.JSONEq(t, `{"x":"1"}`, <-listener1)

	res = simpleStorage.Clone().
		Function("set").
		To(contractAddr).
		Inputs(`{"_x":2}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())

	assert.JSONEq(t, `{"x":"2"}`, <-listener1)

	// making this check immediately after receiving the event results in a race condition where the ack might not have been processed
	// and the checkpoint updated, so check that it is either equal to the block number of the deploy or the block number of the invoke
	status, err = c.PTX().GetBlockchainEventListenerStatus(ctx, "listener1")
	require.NoError(t, err)
	assert.True(t, status.Checkpoint.BlockNumber == deployBlock || status.Checkpoint.BlockNumber == res.Receipt().BlockNumber)

	// stop the event listener
	_, err = c.PTX().StopBlockchainEventListener(ctx, "listener1")
	require.NoError(t, err)

	res = simpleStorage.Clone().
		Function("set").
		To(contractAddr).
		Inputs(`{"_x":3}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())

	// pause to make sure that if an event was going to be received, it would have been
	ticker2 := time.NewTicker(10 * time.Millisecond)
	defer ticker2.Stop()

	select {
	case <-listener1:
		t.FailNow()
	case <-ticker2.C:
	}

	_, err = c.PTX().StartBlockchainEventListener(ctx, "listener1")
	require.NoError(t, err)

	assert.JSONEq(t, `{"x":"3"}`, <-listener1)

	// create a second listener with default fromBlock settings, it should receive all the events
	_, err = c.PTX().CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name: "listener2",
		Sources: []pldapi.BlockchainEventListenerSource{{
			ABI:     abi.ABI{build.ABI.Events()["Changed"]},
			Address: contractAddr,
		}},
	})
	require.NoError(t, err)

	listener2 := make(chan string)
	subscribeAndSendDataToChannel(ctx, t, wsClient, "listener2", listener2)

	assert.JSONEq(t, `{"x":"1"}`, <-listener2)
	assert.JSONEq(t, `{"x":"2"}`, <-listener2)
	assert.JSONEq(t, `{"x":"3"}`, <-listener2)

	// create a third listener that listeners from latest
	_, err = c.PTX().CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name: "listener3",
		Sources: []pldapi.BlockchainEventListenerSource{{
			ABI:     abi.ABI{build.ABI.Events()["Changed"]},
			Address: contractAddr,
		}},
		Options: pldapi.BlockchainEventListenerOptions{
			FromBlock: json.RawMessage(`"latest"`),
		},
	})
	require.NoError(t, err)

	listener3 := make(chan string)
	subscribeAndSendDataToChannel(ctx, t, wsClient, "listener3", listener3)

	// submit another transaction- this should be the next event that all the listeners receive
	res = simpleStorage.Clone().
		Function("set").
		To(contractAddr).
		Inputs(`{"_x":4}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())

	assert.JSONEq(t, `{"x":"4"}`, <-listener1)
	assert.JSONEq(t, `{"x":"4"}`, <-listener2)
	assert.JSONEq(t, `{"x":"4"}`, <-listener3)
}

func TestUpdatePublicTransaction3(t *testing.T) {
	ctx := context.Background()
	logrus.SetLevel(logrus.DebugLevel)

	instance := newInstanceForComponentTesting(t, deployDomainRegistry, nil, nil, nil, true)
	c := pldclient.Wrap(instance.GetClient()).ReceiptPollingInterval(250 * time.Millisecond)

	// set up the receipt listener
	success, err := c.PTX().CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
	})
	require.NoError(t, err)
	require.True(t, success)

	wsClient, err := c.WebSocket(ctx, instance.GetWSConfig())
	require.NoError(t, err)

	sub, err := wsClient.PTX().SubscribeReceipts(ctx, "listener1")
	require.NoError(t, err)

	build, err := solutils.LoadBuild(ctx, simpleStorageBuildJSON)
	require.NoError(t, err)

	simpleStorage := c.ForABI(ctx, build.ABI).Public().From("key1")

	res := simpleStorage.Clone().
		Constructor().
		Bytecode(build.Bytecode).
		Inputs(`{"x":11223344}`).
		Send()
	require.NoError(t, res.Error())

	var deployReceipt *pldapi.TransactionReceiptFull

	for deployReceipt == nil {
		subNotification, ok := <-sub.Notifications()
		if ok {
			var batch pldapi.TransactionReceiptBatch
			_ = json.Unmarshal(subNotification.GetResult(), &batch)
			for _, r := range batch.Receipts {
				if *res.ID() == r.ID {
					deployReceipt = r
				}
			}
			err := subNotification.Ack(ctx)
			require.NoError(t, err)
		}
	}

	tx, err := c.PTX().GetTransactionFull(ctx, *res.ID())
	require.NoError(t, err)
	contractAddr := tx.Receipt.ContractAddress

	setRes := simpleStorage.Clone().
		Function("set").
		To(contractAddr).
		Inputs(`{"_x":99887766}`).
		PublicTxOptions(pldapi.PublicTxOptions{
			// gas is set below instrinsic limit
			Gas: confutil.P(pldtypes.HexUint64(1)),
		}).
		Send()
	require.NoError(t, setRes.Error())
	require.NotNil(t, setRes.ID())

	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		tx, err = c.PTX().GetTransactionFull(ctx, *setRes.ID())
		require.NoError(ct, err)
		require.Len(ct, tx.Public, 1)
		require.NotNil(ct, tx.Public[0].Activity[0])
		assert.Regexp(ct, "ERROR.*Intrinsic", tx.Public[0].Activity[0])
	}, 10*time.Second, 100*time.Millisecond, "Transaction was not processed with error in time")

	_, err = c.PTX().UpdateTransaction(ctx, *setRes.ID(), &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:         "key1",
			Function:     "set",
			Data:         pldtypes.RawJSON(`{"_x":99887766}`),
			To:           contractAddr,
			ABIReference: tx.ABIReference,
			PublicTxOptions: pldapi.PublicTxOptions{
				Gas: confutil.P(pldtypes.HexUint64(10000000)),
			},
		},
	})
	require.NoError(t, err)

	var setReceipt *pldapi.TransactionReceiptFull
	for setReceipt == nil {
		subNotification, ok := <-sub.Notifications()
		if ok {
			var batch pldapi.TransactionReceiptBatch
			_ = json.Unmarshal(subNotification.GetResult(), &batch)
			for _, r := range batch.Receipts {
				if *setRes.ID() == r.ID {
					setReceipt = r
				}
			}
			err := subNotification.Ack(ctx)
			require.NoError(t, err)
		}

	}

	tx, err = c.PTX().GetTransactionFull(ctx, *setRes.ID())
	require.NoError(t, err)
	require.NotNil(t, tx.Receipt)
	require.True(t, tx.Receipt.Success)
	require.Len(t, tx.Public, 1)
	assert.Equal(t, tx.Public[0].Submissions[0].TransactionHash.HexString(), setReceipt.TransactionHash.HexString())
	assert.Len(t, tx.History, 2)

	// try to update the transaction again- it should fail now it is complete
	_, err = c.PTX().UpdateTransaction(ctx, *setRes.ID(), &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:         "key1",
			Function:     "set",
			Data:         pldtypes.RawJSON(`{"_x":99887765}`),
			To:           contractAddr,
			ABIReference: tx.ABIReference,
		},
	})
	assert.ErrorContains(t, err, "PD011937")
}

func TestPrivateTransactionsDeployAndExecute4(t *testing.T) {
	// Coarse grained black box test of the core component manager
	// no mocking although it does use a simple domain implementation that exists solely for testing
	// and is loaded directly through go function calls via the unit test plugin loader
	// (as opposed to compiling as a separate shared library)
	// Even though the domain is a fake, the test does deploy a real contract to the blockchain and the domain
	// manager does communicate with it via the grpc interface.
	// The bootstrap code that is the entry point to the java side is not tested here, we bootstrap the component manager by hand

	ctx := context.Background()
	instance := newInstanceForComponentTesting(t, deployDomainRegistry, nil, nil, nil, false)
	rpcClient := instance.GetClient()

	// Check there are no transactions before we start
	var txns []*pldapi.TransactionFull
	err := rpcClient.CallRPC(ctx, &txns, "ptx_queryTransactionsFull", query.NewQueryBuilder().Limit(1).Query())
	require.NoError(t, err)
	assert.Len(t, txns, 0)
	var dplyTxID uuid.UUID

	err = rpcClient.CallRPC(ctx, &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenConstructorABI(domains.SelfEndorsement),
		TransactionBase: pldapi.TransactionBase{
			IdempotencyKey: "deploy1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "domain1",
			From:           "wallets.org1.aaaaaa",
			Data: pldtypes.RawJSON(`{
                    "from": "wallets.org1.aaaaaa",
                    "name": "FakeToken1",
                    "symbol": "FT1",
					"endorsementMode": "` + domains.SelfEndorsement + `"
                }`),
		},
	})
	require.NoError(t, err)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, dplyTxID, rpcClient, true),
		transactionLatencyThreshold(t)+5*time.Second, //TODO deploy transaction seems to take longer than expected
		100*time.Millisecond,
		"Deploy transaction did not receive a receipt",
	)

	var dplyTxFull pldapi.TransactionFull
	err = rpcClient.CallRPC(ctx, &dplyTxFull, "ptx_getTransactionFull", dplyTxID)
	require.NoError(t, err)
	require.NotNil(t, dplyTxFull.Receipt)
	require.True(t, dplyTxFull.Receipt.Success)
	require.NotNil(t, dplyTxFull.Receipt.ContractAddress)
	contractAddress := dplyTxFull.Receipt.ContractAddress

	var receiptData pldapi.TransactionReceiptData
	err = rpcClient.CallRPC(ctx, &receiptData, "ptx_getTransactionReceipt", dplyTxID)
	assert.NoError(t, err)
	assert.True(t, receiptData.Success)
	assert.Equal(t, contractAddress, receiptData.ContractAddress)

	// Start a private transaction
	var tx1ID uuid.UUID
	err = rpcClient.CallRPC(ctx, &tx1ID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1", //TODO comments say that this is inferred from `to` for invoke
			IdempotencyKey: "tx1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           "wallets.org1.aaaaaa",
			Data: pldtypes.RawJSON(`{
                "from": "",
                "to": "wallets.org1.aaaaaa",
                "amount": "123000000000000000000"
            }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, tx1ID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, tx1ID, rpcClient, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	err = rpcClient.CallRPC(ctx, &txns, "ptx_queryTransactionsFull", query.NewQueryBuilder().Limit(2).Query())
	require.NoError(t, err)
	assert.Len(t, txns, 2)

	txFull := pldapi.TransactionFull{}
	err = rpcClient.CallRPC(ctx, &txFull, "ptx_getTransactionFull", tx1ID)
	require.NoError(t, err)

	require.NotNil(t, txFull.Receipt)
	assert.True(t, txFull.Receipt.Success)
}
