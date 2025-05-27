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
package componenttest

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/core/componenttest/domains"

	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldclient"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/query"
	"github.com/kaleido-io/paladin/sdk/go/pkg/rpcclient"
	"github.com/kaleido-io/paladin/sdk/go/pkg/solutils"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSimpleStorageEthTransaction(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	logrus.SetLevel(logrus.DebugLevel)

	instance := newInstanceForComponentTesting(t, deployDomainRegistry(t), nil, nil, nil, true)
	c := pldclient.Wrap(instance.client).ReceiptPollingInterval(250 * time.Millisecond)

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

	wsClient, err := c.WebSocket(ctx, instance.wsConfig)
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

func TestBlockchainEventListeners(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	logrus.SetLevel(logrus.DebugLevel)

	instance := newInstanceForComponentTesting(t, deployDomainRegistry(t), nil, nil, nil, true)
	c := pldclient.Wrap(instance.client).ReceiptPollingInterval(250 * time.Millisecond)

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

	wsClient, err := c.WebSocket(ctx, instance.wsConfig)
	require.NoError(t, err)

	listener1 := make(chan string)
	subscribeAndSendDataToChannel(ctx, t, wsClient, "listener1", listener1)

	// pause to make sure that if an event was going to be received, it would have been
	ticker1 := time.NewTicker(10 * time.Millisecond)
	defer ticker1.Stop()

	select {
	case <-listener1:
		t.FailNow()
	case <-ticker1.C:
	}

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

func TestUpdatePublicTransaction(t *testing.T) {
	ctx := context.Background()
	logrus.SetLevel(logrus.DebugLevel)

	instance := newInstanceForComponentTesting(t, deployDomainRegistry(t), nil, nil, nil, true)
	c := pldclient.Wrap(instance.client).ReceiptPollingInterval(250 * time.Millisecond)

	// set up the receipt listener
	success, err := c.PTX().CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
	})
	require.NoError(t, err)
	require.True(t, success)

	wsClient, err := c.WebSocket(ctx, instance.wsConfig)
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

	// wait for the submission to be tried and to fail
	time.Sleep(2 * time.Second)

	tx, err = c.PTX().GetTransactionFull(ctx, *setRes.ID())
	require.NoError(t, err)
	require.Len(t, tx.Public, 1)
	require.NotNil(t, tx.Public[0].Activity[0])
	assert.Regexp(t, "ERROR.*Intrinsic", tx.Public[0].Activity[0])

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

func TestPrivateTransactionsDeployAndExecute(t *testing.T) {
	// Coarse grained black box test of the core component manager
	// no mocking although it does use a simple domain implementation that exists solely for testing
	// and is loaded directly through go function calls via the unit test plugin loader
	// (as opposed to compiling as a separate shared library)
	// Even though the domain is a fake, the test does deploy a real contract to the blockchain and the domain
	// manager does communicate with it via the grpc interface.
	// The bootstrap code that is the entry point to the java side is not tested here, we bootstrap the component manager by hand

	ctx := context.Background()
	instance := newInstanceForComponentTesting(t, deployDomainRegistry(t), nil, nil, nil, false)
	rpcClient := instance.client

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

func TestPrivateTransactionsMintThenTransfer(t *testing.T) {
	// Invoke 2 transactions on the same contract where the second transaction relies on the state created by the first

	ctx := context.Background()
	instance := newInstanceForComponentTesting(t, deployDomainRegistry(t), nil, nil, nil, false)
	rpcClient := instance.client

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

	// Start a private transaction - Mint to alice
	var tx1ID uuid.UUID
	err = rpcClient.CallRPC(ctx, &tx1ID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           "wallets.org1.aaaaaa",
			Data: pldtypes.RawJSON(`{
                "from": "",
                "to": "wallets.org1.bbbbbb",
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

	// Start a private transaction - Transfer from alice to bob
	var tx2ID uuid.UUID
	err = rpcClient.CallRPC(ctx, &tx2ID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx2",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           "wallets.org1.bbbbbb",
			Data: pldtypes.RawJSON(`{
                "from": "wallets.org1.bbbbbb",
                "to": "wallets.org1.aaaaaa",
                "amount": "123000000000000000000"
            }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, tx2ID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, tx2ID, rpcClient, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

}

func TestPrivateTransactionRevertedAssembleFailed(t *testing.T) {
	// Invoke a transaction that will fail to assemble
	// in this case, we use the simple token domain and attempt to transfer from a wallet that has no tokens
	ctx := context.Background()
	instance := newInstanceForComponentTesting(t, deployDomainRegistry(t), nil, nil, nil, false)
	rpcClient := instance.client

	var dplyTxID uuid.UUID
	err := rpcClient.CallRPC(ctx, &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
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

	// Start a private transaction - Transfer from alice to bob but we expect that alice can't afford this
	// however, that wont be known until the transaction is assembled which is asynchronous so the initial submission
	// should succeed
	var tx1ID uuid.UUID
	err = rpcClient.CallRPC(ctx, &tx1ID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx2",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           "wallets.org1.bbbbbb",
			Data: pldtypes.RawJSON(`{
				"from": "wallets.org1.bbbbbb",
				"to": "wallets.org1.aaaaaa",
				"amount": "123000000000000000000"
			}`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, tx1ID)
	assert.Eventually(t,
		transactionRevertedCondition(t, ctx, tx1ID, rpcClient),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not revert",
	)

	var txFull pldapi.TransactionFull
	err = rpcClient.CallRPC(ctx, &txFull, "ptx_getTransactionFull", tx1ID)
	require.NoError(t, err)
	require.NotNil(t, txFull.Receipt)
	assert.False(t, txFull.Receipt.Success)
	assert.Regexp(t, domains.SimpleDomainInsufficientFundsError, txFull.Receipt.FailureMessage)
	assert.Regexp(t, "SDE0001", txFull.Receipt.FailureMessage)

	//Check that the domain is left in a healthy state and we can submit good transactions
	// Start a private transaction - Mint to alice
	var goodTxID uuid.UUID
	err = rpcClient.CallRPC(ctx, &goodTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "goodTx",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           "wallets.org1.aaaaaa",
			Data: pldtypes.RawJSON(`{
                "from": "",
                "to": "wallets.org1.bbbbbb",
                "amount": "123000000000000000000"
            }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, goodTxID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, goodTxID, rpcClient, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
}

func TestDeployOnOneNodeInvokeOnAnother(t *testing.T) {
	// We use the simple token where there is no actual on chain checking of the notary
	// so either node can assemble a transaction with an attestation plan for a local notary
	// there is also no access control around minting so both nodes are able to mint tokens and we don't
	// need the complexity of cross node transfers in this test
	ctx := context.Background()

	domainRegistryAddress := deployDomainRegistry(t)

	instance1 := newInstanceForComponentTesting(t, domainRegistryAddress, nil, nil, nil, false)
	client1 := instance1.client
	aliceIdentity := "wallets.org1.alice"
	aliceAddress := instance1.resolveEthereumAddress(aliceIdentity)
	t.Logf("Alice address: %s", aliceAddress)

	instance2 := newInstanceForComponentTesting(t, domainRegistryAddress, nil, nil, nil, false)
	client2 := instance2.client
	bobIdentity := "wallets.org2.bob"
	bobAddress := instance2.resolveEthereumAddress(bobIdentity)
	t.Logf("Bob address: %s", bobAddress)

	//If this fails, it is most likely a bug in the test utils that configures each node with seed mnemonics
	assert.NotEqual(t, aliceAddress, bobAddress)

	// send JSON RPC message to node 1 to deploy a private contract, using alice's key
	var dplyTxID uuid.UUID
	err := client1.CallRPC(ctx, &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenConstructorABI(domains.SelfEndorsement),
		TransactionBase: pldapi.TransactionBase{
			IdempotencyKey: "deploy1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "domain1",
			From:           aliceIdentity,
			Data: pldtypes.RawJSON(`{
                    "from": "` + aliceIdentity + `",
                    "name": "FakeToken1",
                    "symbol": "FT1",
					"endorsementMode": "` + domains.SelfEndorsement + `"
                }`),
		},
	})
	require.NoError(t, err)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, dplyTxID, client1, true),
		transactionLatencyThreshold(t)+5*time.Second, //TODO deploy transaction seems to take longer than expected
		100*time.Millisecond,
		"Deploy transaction did not receive a receipt",
	)

	var dplyTxFull pldapi.TransactionFull
	err = client1.CallRPC(ctx, &dplyTxFull, "ptx_getTransactionFull", dplyTxID)
	require.NoError(t, err)
	contractAddress := dplyTxFull.Receipt.ContractAddress

	// Start a private transaction on alices node
	// this is a mint to alice
	var aliceTxID uuid.UUID
	err = client1.CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-alice",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           aliceIdentity,
			Data: pldtypes.RawJSON(`{
                    "from": "",
                    "to": "` + aliceIdentity + `",
                    "amount": "123000000000000000000"
                }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, aliceTxID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, aliceTxID, client1, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	// Start a private transaction on bobs node
	// This is a mint to bob
	var bobTx1ID uuid.UUID
	err = client2.CallRPC(ctx, &bobTx1ID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-bob",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           bobIdentity,
			Data: pldtypes.RawJSON(`{
                    "from": "",
                    "to": "` + bobIdentity + `",
                    "amount": "123000000000000000000"
                }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, bobTx1ID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, bobTx1ID, client2, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
}

func TestResolveIdentityFromRemoteNode(t *testing.T) {
	// stand up 2 nodes, with different key managers
	// send an RPC request to one node to resolve the identity of a user@the-other-node
	// this forces both nodes to communicate with each other to resolve the identity

	ctx := context.Background()

	//TODO shouldn't need domain registry for this test
	domainRegistryAddress := deployDomainRegistry(t)

	aliceNodeConfig := newNodeConfiguration(t, "alice")
	bobNodeConfig := newNodeConfiguration(t, "bob")

	instance1 := newInstanceForComponentTesting(t, domainRegistryAddress, aliceNodeConfig, []*nodeConfiguration{bobNodeConfig}, nil, false)
	client1 := instance1.client
	aliceIdentity := "wallets.org1.alice@" + instance1.name
	aliceAddress := instance1.resolveEthereumAddress(aliceIdentity)
	t.Logf("Alice address: %s", aliceAddress)

	instance2 := newInstanceForComponentTesting(t, domainRegistryAddress, bobNodeConfig, []*nodeConfiguration{aliceNodeConfig}, nil, false)
	client2 := instance2.client
	bobUnqualifiedIdentity := "wallets.org2.bob"
	bobIdentity := bobUnqualifiedIdentity + "@" + instance2.name
	bobAddress := instance2.resolveEthereumAddress(bobIdentity)
	t.Logf("Bob address: %s", bobAddress)

	// send JSON RPC message to node 1 to resolve a verifier on node 2
	var verifierResult1 string
	var verifierResult2 string
	var verifierResult3 string
	err := client1.CallRPC(ctx, &verifierResult1, "ptx_resolveVerifier",
		bobIdentity,
		algorithms.ECDSA_SECP256K1,
		verifiers.ETH_ADDRESS,
	)
	require.NoError(t, err)
	require.NotNil(t, verifierResult1)

	// resolve the same verifier on node 2 directly
	err = client2.CallRPC(ctx, &verifierResult2, "ptx_resolveVerifier",
		bobIdentity,
		algorithms.ECDSA_SECP256K1,
		verifiers.ETH_ADDRESS,
	)
	require.NoError(t, err)
	require.NotNil(t, verifierResult2)

	// resolve the same verifier on node 2 directly using the unqualified identity
	err = client2.CallRPC(ctx, &verifierResult3, "ptx_resolveVerifier",
		bobUnqualifiedIdentity,
		algorithms.ECDSA_SECP256K1,
		verifiers.ETH_ADDRESS,
	)
	require.NoError(t, err)
	require.NotNil(t, verifierResult3)

	// all 3 results should be the same
	assert.Equal(t, verifierResult1, verifierResult2)
	assert.Equal(t, verifierResult1, verifierResult3)

}

func TestCreateStateOnOneNodeSpendOnAnother(t *testing.T) {
	// We use the simple token in SelfEndorsement mode (similar to zeto so either node can assemble a transaction
	// however, in this test, Bob's transaction will only succeed if he can spend the coins that Alice transfers to him
	// so this tests that the state is shared between the nodes

	ctx := context.Background()
	domainRegistryAddress := deployDomainRegistry(t)

	alice := newPartyForTesting(t, "alice", domainRegistryAddress)
	bob := newPartyForTesting(t, "bob", domainRegistryAddress)

	alice.peer(bob.nodeConfig)
	bob.peer(alice.nodeConfig)

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}
	alice.start(t, domainConfig)
	bob.start(t, domainConfig)

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.identity,
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.deploySimpleDomainInstanceContract(t, domains.SelfEndorsement, constructorParameters)

	// Start a private transaction on alices node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bobs node
	var aliceTxID uuid.UUID
	err := alice.client.CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-alice",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           alice.identity,
			Data: pldtypes.RawJSON(`{
                    "from": "",
                    "to": "` + bob.identityLocator + `",
                    "amount": "123000000000000000000"
                }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, aliceTxID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, aliceTxID, alice.client, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	// Start a private transaction on bobs node
	// This is a transfer which relies on bobs node being aware of the state created by alice's mint to bob above
	var bobTx1ID uuid.UUID
	err = bob.client.CallRPC(ctx, &bobTx1ID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-bob",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           bob.identity,
			Data: pldtypes.RawJSON(`{
                    "from": "` + bob.identityLocator + `",
                    "to": "` + alice.identityLocator + `",
                    "amount": "123000000000000000000"
                }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, bobTx1ID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, bobTx1ID, bob.client, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
}

func TestNotaryDelegated(t *testing.T) {
	//This is similar to the noto scenario
	// all transfers must be endorsed by the single notary and the notary must submit to the base ledger
	// it also happens to be the case in noto that only the notary can mint so we replicate that
	// constraint here too so this test serves as a reasonable contract test for the noto use case

	ctx := context.Background()

	aliceNodeConfig := newNodeConfiguration(t, "alice")
	bobNodeConfig := newNodeConfiguration(t, "bob")
	notaryNodeConfig := newNodeConfiguration(t, "notary")

	domainRegistryAddress := deployDomainRegistry(t)

	instance1 := newInstanceForComponentTesting(t, domainRegistryAddress, aliceNodeConfig, []*nodeConfiguration{bobNodeConfig, notaryNodeConfig}, nil, false)
	client1 := instance1.client
	aliceIdentity := "wallets.org1.alice@" + instance1.name

	instance2 := newInstanceForComponentTesting(t, domainRegistryAddress, bobNodeConfig, []*nodeConfiguration{aliceNodeConfig, notaryNodeConfig}, nil, false)
	bobIdentity := "wallets.org2.bob@" + instance2.name

	instance3 := newInstanceForComponentTesting(t, domainRegistryAddress, notaryNodeConfig, []*nodeConfiguration{aliceNodeConfig, bobNodeConfig}, nil, false)
	client3 := instance3.client
	notaryIdentity := "wallets.org3.notary@" + instance3.name

	// send JSON RPC message to node 3 ( notary) to deploy a private contract
	var dplyTxID uuid.UUID
	err := client3.CallRPC(ctx, &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenConstructorABI(domains.NotaryEndorsement),
		TransactionBase: pldapi.TransactionBase{
			IdempotencyKey: "deploy1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "domain1",
			From:           notaryIdentity,
			Data: pldtypes.RawJSON(`{
					"notary": "` + notaryIdentity + `",
					"name": "FakeToken1",
					"symbol": "FT1",
					"endorsementMode": "NotaryEndorsement"
				}`),
		},
	})
	require.NoError(t, err)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, dplyTxID, client3, true),
		transactionLatencyThreshold(t)+5*time.Second, //TODO deploy transaction seems to take longer than expected
		100*time.Millisecond,
		"Deploy transaction did not receive a receipt",
	)

	// As notary, mint some tokens to alice
	var dplyTxFull pldapi.TransactionFull
	err = client3.CallRPC(ctx, &dplyTxFull, "ptx_getTransactionFull", dplyTxID)
	require.NoError(t, err)
	contractAddress := dplyTxFull.Receipt.ContractAddress

	// Start a private transaction on notary node
	// this is a mint to alice so alice should later be able to do a transfer to bob
	var mintTxID uuid.UUID
	err = client3.CallRPC(ctx, &mintTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-mint",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           notaryIdentity,
			Data: pldtypes.RawJSON(`{
					"from": "",
					"to": "` + aliceIdentity + `",
					"amount": "100"
				}`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, mintTxID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, mintTxID, client3, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	// Start a private transaction on alices node to transfer to bob
	var transferA2BTxId uuid.UUID
	err = client1.CallRPC(ctx, &transferA2BTxId, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "transferA2B1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           aliceIdentity,
			Data: pldtypes.RawJSON(`{
					"from": "` + aliceIdentity + `",
					"to": "` + bobIdentity + `",
					"amount": "50"
				}`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, transferA2BTxId)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, transferA2BTxId, client1, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

}
func TestNotaryDelegatedPrepare(t *testing.T) {
	//Similar to the TestNotaryDelegated test except in this case, the transaction is not submitted to the base ledger by the notary.
	//instead, the assembled and prepared transaction is returned to the sender node to submit to the base ledger whenever it is deemed appropriate
	// NOTE the use of ptx_prepareTransaction instead of ptx_sendTransaction on the transfer

	ctx := context.Background()

	aliceNodeConfig := newNodeConfiguration(t, "alice")
	bobNodeConfig := newNodeConfiguration(t, "bob")
	notaryNodeConfig := newNodeConfiguration(t, "notary")

	domainRegistryAddress := deployDomainRegistry(t)

	instance1 := newInstanceForComponentTesting(t, domainRegistryAddress, aliceNodeConfig, []*nodeConfiguration{bobNodeConfig, notaryNodeConfig}, nil, false)
	client1 := instance1.client
	aliceIdentity := "wallets.org1.alice@" + instance1.name

	instance2 := newInstanceForComponentTesting(t, domainRegistryAddress, bobNodeConfig, []*nodeConfiguration{aliceNodeConfig, notaryNodeConfig}, nil, false)
	bobIdentity := "wallets.org2.bob@" + instance2.name

	instance3 := newInstanceForComponentTesting(t, domainRegistryAddress, notaryNodeConfig, []*nodeConfiguration{aliceNodeConfig, bobNodeConfig}, nil, false)
	client3 := instance3.client
	notaryIdentity := "wallets.org3.notary@" + instance3.name

	// send JSON RPC message to node 3 ( notary) to deploy a private contract
	var dplyTxID uuid.UUID
	err := client3.CallRPC(ctx, &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenConstructorABI(domains.NotaryEndorsement),
		TransactionBase: pldapi.TransactionBase{
			IdempotencyKey: "deploy1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "domain1",
			From:           notaryIdentity,
			Data: pldtypes.RawJSON(`{
					"notary": "` + notaryIdentity + `",
					"name": "FakeToken1",
					"symbol": "FT1",
					"endorsementMode": "NotaryEndorsement",
					"deleteSubmitToSender": true
				}`),
		},
	})
	require.NoError(t, err)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, dplyTxID, client3, true),
		transactionLatencyThreshold(t)+5*time.Second, //TODO deploy transaction seems to take longer than expected
		100*time.Millisecond,
		"Deploy transaction did not receive a receipt",
	)

	// As notary, mint some tokens to alice
	var dplyTxFull pldapi.TransactionFull
	err = client3.CallRPC(ctx, &dplyTxFull, "ptx_getTransactionFull", dplyTxID)
	require.NoError(t, err)
	contractAddress := dplyTxFull.Receipt.ContractAddress

	// Start a private transaction on notary node
	// this is a mint to alice so alice should later be able to do a transfer to bob
	var mintTxID uuid.UUID
	err = client3.CallRPC(ctx, &mintTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-mint",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           notaryIdentity,
			Data: pldtypes.RawJSON(`{
					"from": "",
					"to": "` + aliceIdentity + `",
					"amount": "100"
				}`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, mintTxID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, mintTxID, client3, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	// Prepare a private transaction on alices node to transfer to bob
	var transferA2BTxId uuid.UUID
	err = client1.CallRPC(ctx, &transferA2BTxId, "ptx_prepareTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "transferA2B1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           aliceIdentity,
			Data: pldtypes.RawJSON(`{
					"from": "` + aliceIdentity + `",
					"to": "` + bobIdentity + `",
					"amount": "25"
				}`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, transferA2BTxId)

	txFull1 := pldapi.TransactionFull{}
	err = client1.CallRPC(ctx, &txFull1, "ptx_getTransactionFull", transferA2BTxId)
	require.NoError(t, err)
	txFull2 := pldapi.TransactionFull{}

	err = client3.CallRPC(ctx, &txFull2, "ptx_getTransactionFull", transferA2BTxId)
	require.NoError(t, err)

	assert.Eventually(t,
		func() bool {
			var preparedTx *pldapi.PreparedTransaction

			// The transaction is prepared with a from-address that is local to node3 - so only
			// node3 will be able to send it. So that's where it gets persisted.
			err = client3.CallRPC(ctx, &preparedTx, "ptx_getPreparedTransaction", transferA2BTxId)
			require.NoError(t, err)

			if preparedTx == nil {
				return false
			}
			assert.Empty(t, preparedTx.Transaction.Domain)
			return preparedTx.ID == transferA2BTxId && len(preparedTx.States.Spent) == 1 && len(preparedTx.States.Confirmed) == 2

		},
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Prepared transaction not available on sender node",
	)

}

func TestSingleNodeSelfEndorseConcurrentSpends(t *testing.T) {
	// Invoke a bunch of transactions on the same contract on a single node, in self endorsement mode ( a la zeto )
	// where there is a reasonable possibility of contention between transactions

	//start by minting 5 coins then send 5 transactions to spend them
	// if there is no contention, each transfer should be able to spend a coin each

	ctx := context.Background()
	instance := newInstanceForComponentTesting(t, deployDomainRegistry(t), nil, nil, nil, false)
	rpcClient := instance.client

	var dplyTxID uuid.UUID
	err := rpcClient.CallRPC(ctx, &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
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

	// Do the 5 mints
	mint := func() (id uuid.UUID) {
		var txID uuid.UUID
		err = rpcClient.CallRPC(ctx, &txID, "ptx_sendTransaction", &pldapi.TransactionInput{
			ABI: *domains.SimpleTokenTransferABI(),
			TransactionBase: pldapi.TransactionBase{
				To:             contractAddress,
				Domain:         "domain1",
				IdempotencyKey: pldtypes.RandHex(8),
				Type:           pldapi.TransactionTypePrivate.Enum(),
				From:           "wallets.org1.aaaaaa",
				Data: pldtypes.RawJSON(`{
					"from": "",
					"to": "wallets.org1.aaaaaa",
					"amount": "1"
				}`),
			},
		})
		require.NoError(t, err)
		assert.NotEqual(t, uuid.UUID{}, txID)
		return txID
	}
	waitForTransaction := func(txID uuid.UUID) {
		assert.Eventually(t,
			transactionReceiptCondition(t, ctx, txID, rpcClient, false),
			transactionLatencyThreshold(t),
			100*time.Millisecond,
			"Transaction did not receive a receipt",
		)
	}

	mint1 := mint()
	mint2 := mint()
	mint3 := mint()
	mint4 := mint()
	mint5 := mint()

	waitForTransaction(mint1)
	waitForTransaction(mint2)
	waitForTransaction(mint3)
	waitForTransaction(mint4)
	waitForTransaction(mint5)
	//time.Sleep(1000 * time.Millisecond)
	// Now kick off the 5 transfers
	transfer := func() (id uuid.UUID) {
		var txID uuid.UUID
		err = rpcClient.CallRPC(ctx, &txID, "ptx_sendTransaction", &pldapi.TransactionInput{
			ABI: *domains.SimpleTokenTransferABI(),
			TransactionBase: pldapi.TransactionBase{
				To:             contractAddress,
				Domain:         "domain1",
				IdempotencyKey: pldtypes.RandHex(8),
				Type:           pldapi.TransactionTypePrivate.Enum(),
				From:           "wallets.org1.aaaaaa",
				Data: pldtypes.RawJSON(`{
					"from": "wallets.org1.aaaaaa",
					"to": "wallets.org1.bbbbbb",
					"amount": "1"
				}`),
			},
		})

		require.NoError(t, err)
		assert.NotEqual(t, uuid.UUID{}, txID)
		return txID
	}
	transfer1 := transfer()
	transfer2 := transfer()
	transfer3 := transfer()
	transfer4 := transfer()
	transfer5 := transfer()

	waitForTransaction(transfer1)
	waitForTransaction(transfer2)
	waitForTransaction(transfer3)
	waitForTransaction(transfer4)
	waitForTransaction(transfer5)

}

func TestSingleNodeSelfEndorseSeriesOfTransfers(t *testing.T) {
	// Invoke a series of transactions on the same contract on a single node, in self endorsement mode ( a la zeto )
	//where each transaction relies on the state created by the previous

	ctx := context.Background()
	instance := newInstanceForComponentTesting(t, deployDomainRegistry(t), nil, nil, nil, false)
	rpcClient := instance.client

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

	// Start a private transaction - Mint to alice
	var tx1ID uuid.UUID
	err = rpcClient.CallRPC(ctx, &tx1ID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           "wallets.org1.aaaaaa",
			Data: pldtypes.RawJSON(`{
                "from": "",
                "to": "wallets.org1.bbbbbb",
                "amount": "100"
            }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, tx1ID)

	// Start a private transaction - Transfer from alice to bob
	var tx2ID uuid.UUID
	err = rpcClient.CallRPC(ctx, &tx2ID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx2",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           "wallets.org1.bbbbbb",
			Data: pldtypes.RawJSON(`{
                "from": "wallets.org1.bbbbbb",
                "to": "wallets.org1.aaaaaa",
                "amount": "99"
            }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, tx2ID)
	//time.Sleep(1000 * time.Millisecond) // Add a small delay to avoid a tight loop
	// Start a private transaction - Transfer from alice to bob
	var tx3ID uuid.UUID
	err = rpcClient.CallRPC(ctx, &tx3ID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx3",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           "wallets.org1.aaaaaa",
			Data: pldtypes.RawJSON(`{
                "from": "wallets.org1.aaaaaa",
                "to": "wallets.org1.bbbbbb",
                "amount": "98"
            }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, tx3ID)

	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, tx1ID, rpcClient, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, tx2ID, rpcClient, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, tx3ID, rpcClient, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

}

func TestNotaryEndorseConcurrentSpends(t *testing.T) {
	// Invoke a bunch of transactions on the same contract in self endorsement mode ( a la noto )
	// perform the transfers from the same identity so that there is high likelihood of contention

	//start by minting 5 coins then send 5 transactions to spend them
	// if there is no contention, each transfer should be able to spend a coin each

	ctx := context.Background()

	aliceNodeConfig := newNodeConfiguration(t, "alice")
	bobNodeConfig := newNodeConfiguration(t, "bob")
	notaryNodeConfig := newNodeConfiguration(t, "notary")

	domainRegistryAddress := deployDomainRegistry(t)

	instance1 := newInstanceForComponentTesting(t, domainRegistryAddress, aliceNodeConfig, []*nodeConfiguration{bobNodeConfig, notaryNodeConfig}, nil, false)
	client1 := instance1.client
	aliceIdentity := "wallets.org1.alice@" + instance1.name

	instance2 := newInstanceForComponentTesting(t, domainRegistryAddress, bobNodeConfig, []*nodeConfiguration{aliceNodeConfig, notaryNodeConfig}, nil, false)
	bobIdentity := "wallets.org2.bob@" + instance2.name

	instance3 := newInstanceForComponentTesting(t, domainRegistryAddress, notaryNodeConfig, []*nodeConfiguration{aliceNodeConfig, bobNodeConfig}, nil, false)
	client3 := instance3.client
	notaryIdentity := "wallets.org3.notary@" + instance3.name

	// send JSON RPC message to node 3 ( notary) to deploy a private contract
	var dplyTxID uuid.UUID
	err := client3.CallRPC(ctx, &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenConstructorABI(domains.NotaryEndorsement),
		TransactionBase: pldapi.TransactionBase{
			IdempotencyKey: "deploy1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "domain1",
			From:           notaryIdentity,
			Data: pldtypes.RawJSON(`{
					"notary": "` + notaryIdentity + `",
					"name": "FakeToken1",
					"symbol": "FT1",
					"endorsementMode": "NotaryEndorsement"
				}`),
		},
	})
	require.NoError(t, err)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, dplyTxID, client3, true),
		transactionLatencyThreshold(t)+5*time.Second, //TODO deploy transaction seems to take longer than expected
		100*time.Millisecond,
		"Deploy transaction did not receive a receipt",
	)

	var dplyTxFull pldapi.TransactionFull
	err = client3.CallRPC(ctx, &dplyTxFull, "ptx_getTransactionFull", dplyTxID)
	require.NoError(t, err)
	contractAddress := dplyTxFull.Receipt.ContractAddress

	// Start a private transaction on notary node
	// this is a mint to alice so alice should later be able to do a transfer to bob

	// Do the 5 mints
	mint := func() (id uuid.UUID) {
		var txID uuid.UUID
		err = client3.CallRPC(ctx, &txID, "ptx_sendTransaction", &pldapi.TransactionInput{
			ABI: *domains.SimpleTokenTransferABI(),
			TransactionBase: pldapi.TransactionBase{
				To:             contractAddress,
				Domain:         "domain1",
				IdempotencyKey: pldtypes.RandHex(8),
				Type:           pldapi.TransactionTypePrivate.Enum(),
				From:           notaryIdentity,
				Data: pldtypes.RawJSON(`{
					"from": "",
					"to": "` + aliceIdentity + `",
					"amount": "100"
				}`),
			},
		})
		require.NoError(t, err)
		assert.NotEqual(t, uuid.UUID{}, txID)
		return txID
	}
	waitForTransaction := func(txID uuid.UUID, client rpcclient.Client) {
		assert.Eventually(t,
			transactionReceiptCondition(t, ctx, txID, client, false),
			transactionLatencyThreshold(t),
			100*time.Millisecond,
			"Transaction did not receive a receipt",
		)
	}

	mint1 := mint()
	mint2 := mint()
	mint3 := mint()
	mint4 := mint()
	mint5 := mint()

	waitForTransaction(mint1, client3)
	waitForTransaction(mint2, client3)
	waitForTransaction(mint3, client3)
	waitForTransaction(mint4, client3)
	waitForTransaction(mint5, client3)

	// Now kick off the 5 transfers
	transfer := func() (id uuid.UUID) {
		var txID uuid.UUID
		err = client1.CallRPC(ctx, &txID, "ptx_sendTransaction", &pldapi.TransactionInput{
			ABI: *domains.SimpleTokenTransferABI(),
			TransactionBase: pldapi.TransactionBase{
				To:             contractAddress,
				Domain:         "domain1",
				IdempotencyKey: pldtypes.RandHex(8),
				Type:           pldapi.TransactionTypePrivate.Enum(),
				From:           aliceIdentity,
				Data: pldtypes.RawJSON(`{
						"from": "` + aliceIdentity + `",
						"to": "` + bobIdentity + `",
						"amount": "100"
					}`),
			},
		})

		require.NoError(t, err)
		assert.NotEqual(t, uuid.UUID{}, txID)
		return txID
	}
	transfer1 := transfer()
	transfer2 := transfer()
	transfer3 := transfer()
	transfer4 := transfer()
	transfer5 := transfer()

	waitForTransaction(transfer1, client1)
	waitForTransaction(transfer2, client1)
	waitForTransaction(transfer3, client1)
	waitForTransaction(transfer4, client1)
	waitForTransaction(transfer5, client1)

}

func TestNotaryEndorseSeriesOfTransfers(t *testing.T) {

	// Invoke a series of transactions on the same contract in self endorsement mode ( a la neto )
	// where each transaction relies on the state created by the previous
	t.Skip("This is an invalid test because there is no meaning to the term 'previous' in a concurrent system")

	ctx := context.Background()

	aliceNodeConfig := newNodeConfiguration(t, "alice")
	bobNodeConfig := newNodeConfiguration(t, "bob")
	notaryNodeConfig := newNodeConfiguration(t, "notary")

	domainRegistryAddress := deployDomainRegistry(t)

	instance1 := newInstanceForComponentTesting(t, domainRegistryAddress, aliceNodeConfig, []*nodeConfiguration{bobNodeConfig, notaryNodeConfig}, nil, false)
	client1 := instance1.client
	aliceIdentity := "wallets.org1.alice@" + instance1.name

	instance2 := newInstanceForComponentTesting(t, domainRegistryAddress, bobNodeConfig, []*nodeConfiguration{aliceNodeConfig, notaryNodeConfig}, nil, false)
	client2 := instance2.client
	bobIdentity := "wallets.org2.bob@" + instance2.name

	instance3 := newInstanceForComponentTesting(t, domainRegistryAddress, notaryNodeConfig, []*nodeConfiguration{aliceNodeConfig, bobNodeConfig}, nil, false)
	client3 := instance3.client
	notaryIdentity := "wallets.org3.notary@" + instance3.name

	// send JSON RPC message to node 3 ( notary) to deploy a private contract
	var dplyTxID uuid.UUID
	err := client3.CallRPC(ctx, &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenConstructorABI(domains.NotaryEndorsement),
		TransactionBase: pldapi.TransactionBase{
			IdempotencyKey: "deploy1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "domain1",
			From:           notaryIdentity,
			Data: pldtypes.RawJSON(`{
					"notary": "` + notaryIdentity + `",
					"name": "FakeToken1",
					"symbol": "FT1",
					"endorsementMode": "NotaryEndorsement"
				}`),
		},
	})
	require.NoError(t, err)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, dplyTxID, client3, true),
		transactionLatencyThreshold(t)+5*time.Second, //TODO deploy transaction seems to take longer than expected
		100*time.Millisecond,
		"Deploy transaction did not receive a receipt",
	)
	var dplyTxFull pldapi.TransactionFull
	err = client3.CallRPC(ctx, &dplyTxFull, "ptx_getTransactionFull", dplyTxID)
	require.NoError(t, err)
	contractAddress := dplyTxFull.Receipt.ContractAddress

	//Start with a mint to alice
	var mintTxID uuid.UUID
	err = client3.CallRPC(ctx, &mintTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-mint",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           notaryIdentity,
			Data: pldtypes.RawJSON(`{
					"from": "",
					"to": "` + aliceIdentity + `",
					"amount": "100"
				}`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, mintTxID)

	// Alice transfers some of her recently minted tokens to bob
	var transferA2B1TxId uuid.UUID
	err = client1.CallRPC(ctx, &transferA2B1TxId, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "transferA2B1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           aliceIdentity,
			Data: pldtypes.RawJSON(`{
					"from": "` + aliceIdentity + `",
					"to": "` + bobIdentity + `",
					"amount": "99"
				}`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, transferA2B1TxId)

	// Bob sends some tokens back to alice
	var transferB2A1TxId uuid.UUID
	err = client2.CallRPC(ctx, &transferB2A1TxId, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "transferB2A1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           bobIdentity,
			Data: pldtypes.RawJSON(`{
					"from": "` + bobIdentity + `",
					"to": "` + aliceIdentity + `",
					"amount": "95"
				}`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, transferB2A1TxId)

	//Alice can transfer 99 to bob.  The 98 bob just transferred to here and the 1 change from her earlier transfer to bob
	var transferA2B2TxId uuid.UUID
	err = client1.CallRPC(ctx, &transferA2B2TxId, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "transferA2B2",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           aliceIdentity,
			Data: pldtypes.RawJSON(`{
					"from": "` + aliceIdentity + `",
					"to": "` + bobIdentity + `",
					"amount": "90"
				}`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, transferA2B2TxId)

	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, mintTxID, client3, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, transferA2B1TxId, client1, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, transferB2A1TxId, client2, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, transferA2B2TxId, client1, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

}

func TestPrivacyGroupEndorsement(t *testing.T) {
	// This test is intended to emulate the pente domain where all transactions must be endorsed by all parties in the predefined privacy group
	// in this case, we have 3 nodes, each representing a different party in the privacy group
	// and we expect that all transactions must be endorsed by all 3 nodes and that all output states are distributed to all 3 nodes
	// Unlike the coin based domains, this is a "world state" based domain so there is only ever one available state at any one time and each
	// transaction spends that state and creates a new one.  So there is contention between parties
	ctx := context.Background()
	domainRegistryAddress := deployDomainRegistry(t)

	alice := newPartyForTesting(t, "alice", domainRegistryAddress)
	bob := newPartyForTesting(t, "bob", domainRegistryAddress)
	carol := newPartyForTesting(t, "carol", domainRegistryAddress)

	alice.peer(bob.nodeConfig, carol.nodeConfig)
	bob.peer(alice.nodeConfig, carol.nodeConfig)
	carol.peer(alice.nodeConfig, bob.nodeConfig)

	domainConfig := &domains.SimpleStorageDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}
	alice.start(t, domainConfig)
	bob.start(t, domainConfig)
	carol.start(t, domainConfig)

	endorsementSet := []string{alice.identityLocator, bob.identityLocator, carol.identityLocator}

	constructorParameters := &domains.SimpleStorageConstructorParameters{
		EndorsementSet:  endorsementSet,
		Name:            "SimpleStorage1",
		EndorsementMode: domains.PrivacyGroupEndorsement,
	}
	// send JSON RPC message to node 1 to deploy a private contract
	contractAddress := alice.deploySimpleStorageDomainInstanceContract(t, domains.PrivacyGroupEndorsement, constructorParameters)

	// Start a private transaction on alice's node
	// this should require endorsement from bob and carol
	// Initialise a new map
	var aliceTxID uuid.UUID
	err := alice.client.CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleStorageInitABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "simpleStorageDomain",
			IdempotencyKey: "tx1-alice",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           alice.identity,
			Data: pldtypes.RawJSON(`{
					"map":"map1"
                }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, aliceTxID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, aliceTxID, alice.client, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	// Start a private transaction on bob's node
	// this should require endorsement from alice and carol
	var bobTxID uuid.UUID
	err = bob.client.CallRPC(ctx, &bobTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleStorageSetABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "simpleStorageDomain",
			IdempotencyKey: "tx1-bob",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           bob.identity,
			Data: pldtypes.RawJSON(`{
					"map":"map1",
                    "key": "foo",
					"value": "quz"
                }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, bobTxID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, bobTxID, bob.client, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	var bobSchemas []*pldapi.Schema
	err = bob.client.CallRPC(ctx, &bobSchemas, "pstate_listSchemas", "simpleStorageDomain")
	require.NoError(t, err)
	require.Len(t, bobSchemas, 1)

	var bobStates []*pldapi.State
	err = bob.client.CallRPC(ctx, &bobStates, "pstate_queryContractStates", "simpleStorageDomain", contractAddress.String(), bobSchemas[0].ID, pldtypes.RawJSON(`{}`), "available")
	require.NoError(t, err)
	require.Len(t, bobStates, 1)
	stateData := make(map[string]string)
	storage := make(map[string]string)
	jsonErr := json.Unmarshal(bobStates[0].Data.Bytes(), &stateData)
	require.NoError(t, jsonErr)

	jsonErr = json.Unmarshal([]byte(stateData["records"]), &storage)
	require.NoError(t, jsonErr)

	assert.Equal(t, "quz", storage["foo"])

	// Alice should see the same latest state of the world as Bob
	var aliceSchemas []*pldapi.Schema

	err = alice.client.CallRPC(ctx, &aliceSchemas, "pstate_listSchemas", "simpleStorageDomain")
	require.NoError(t, err)
	require.Len(t, aliceSchemas, 1)
	assert.Equal(t, bobSchemas[0].ID, aliceSchemas[0].ID)

	var aliceStates []*pldapi.State

	err = alice.client.CallRPC(ctx, &aliceStates, "pstate_queryContractStates", "simpleStorageDomain", contractAddress.String(), aliceSchemas[0].ID, pldtypes.RawJSON(`{}`), "available")
	require.NoError(t, err)
	require.Len(t, aliceStates, 1)
	assert.Equal(t, bobStates[0].ID, aliceStates[0].ID)
	assert.Equal(t, bobStates[0].Data, aliceStates[0].Data)

}

func TestPrivacyGroupEndorsementConcurrent(t *testing.T) {
	// This test is identical to TestPrivacyGroupEndorsement except that it sends the transactions concurrently
	// For manual exploratory testing of longevity , it is possible to increase the number of iterations and the test should still be valid
	// however, it is hard coded to a small number by default so that it can be run in CI
	NUM_ITERATIONS := 2
	NUM_TRANSACTIONS_PER_NODE_PER_ITERATION := 2
	ctx := context.Background()
	domainRegistryAddress := deployDomainRegistry(t)

	alice := newPartyForTesting(t, "alice", domainRegistryAddress)
	bob := newPartyForTesting(t, "bob", domainRegistryAddress)
	carol := newPartyForTesting(t, "carol", domainRegistryAddress)

	alice.peer(bob.nodeConfig, carol.nodeConfig)
	bob.peer(alice.nodeConfig, carol.nodeConfig)
	carol.peer(alice.nodeConfig, bob.nodeConfig)

	domainConfig := &domains.SimpleStorageDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}
	alice.start(t, domainConfig)
	bob.start(t, domainConfig)
	carol.start(t, domainConfig)

	endorsementSet := []string{alice.identityLocator, bob.identityLocator, carol.identityLocator}

	constructorParameters := &domains.SimpleStorageConstructorParameters{
		EndorsementSet:  endorsementSet,
		Name:            "SimpleStorage1",
		EndorsementMode: domains.PrivacyGroupEndorsement,
	}
	// send JSON RPC message to node 1 to deploy a private contract
	contractAddress := alice.deploySimpleStorageDomainInstanceContract(t, domains.PrivacyGroupEndorsement, constructorParameters)
	var initTxID uuid.UUID
	err := alice.client.CallRPC(ctx, &initTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleStorageInitABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "simpleStorageDomain",
			IdempotencyKey: "init-tx",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           alice.identity,
			Data: pldtypes.RawJSON(`{
                    "map":"TestPrivacyGroupEndorsementConcurrent"
                }`),
		},
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, initTxID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, initTxID, alice.client, false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Init map transaction did not receive a receipt",
	)

	//initialize a map that all parties should be able to access concurrently
	// we wait for the confirmation of this transaction to ensure that there is no race condition of someone trying to call `set` before the map is initialized
	// TODO - so long as we have the transaction id for the init transaction, we could declare a dependency on it for the set transactions
	for i := 0; i < NUM_ITERATIONS; i++ {
		// Start a number of private transaction on alice's node
		// this should require endorsement from bob and carol
		aliceTxID := make([]uuid.UUID, NUM_TRANSACTIONS_PER_NODE_PER_ITERATION)
		bobTxID := make([]uuid.UUID, NUM_TRANSACTIONS_PER_NODE_PER_ITERATION)
		carolTxID := make([]uuid.UUID, NUM_TRANSACTIONS_PER_NODE_PER_ITERATION)

		for j := 0; j < NUM_TRANSACTIONS_PER_NODE_PER_ITERATION; j++ {
			err := alice.client.CallRPC(ctx, &aliceTxID[j], "ptx_sendTransaction", &pldapi.TransactionInput{
				ABI: *domains.SimpleStorageSetABI(),
				TransactionBase: pldapi.TransactionBase{
					To:             contractAddress,
					Domain:         "simpleStorageDomain",
					IdempotencyKey: fmt.Sprintf("tx1-alice-%d-%d", i, j),
					Type:           pldapi.TransactionTypePrivate.Enum(),
					From:           alice.identity,
					Data: pldtypes.RawJSON(fmt.Sprintf(`{
				 	"map":"TestPrivacyGroupEndorsementConcurrent",
                    "key": "alice_key_%d_%d",
					"value": "alice_value_%d_%d"
                }`, i, j, i, j)),
				},
			})
			require.NoError(t, err)
			assert.NotEqual(t, uuid.UUID{}, aliceTxID[j])

			// Start a private transaction on bob's node
			// this should require endorsement from alice and carol
			err = bob.client.CallRPC(ctx, &bobTxID[j], "ptx_sendTransaction", &pldapi.TransactionInput{
				ABI: *domains.SimpleStorageSetABI(),
				TransactionBase: pldapi.TransactionBase{
					To:             contractAddress,
					Domain:         "simpleStorageDomain",
					IdempotencyKey: fmt.Sprintf("tx1-bob-%d-%d", i, j),
					Type:           pldapi.TransactionTypePrivate.Enum(),
					From:           bob.identity,
					Data: pldtypes.RawJSON(fmt.Sprintf(`{
				 	"map":"TestPrivacyGroupEndorsementConcurrent",
                    "key": "bob_key_%d_%d",
					"value": "bob_value_%d_%d"
                }`, i, j, i, j)),
				},
			})
			require.NoError(t, err)
			assert.NotEqual(t, uuid.UUID{}, bobTxID[j])

			err = carol.client.CallRPC(ctx, &carolTxID[j], "ptx_sendTransaction", &pldapi.TransactionInput{
				ABI: *domains.SimpleStorageSetABI(),
				TransactionBase: pldapi.TransactionBase{
					To:             contractAddress,
					Domain:         "simpleStorageDomain",
					IdempotencyKey: fmt.Sprintf("tx1-carol-%d-%d", i, j),
					Type:           pldapi.TransactionTypePrivate.Enum(),
					From:           bob.identity,
					Data: pldtypes.RawJSON(fmt.Sprintf(`{
				 	"map":"TestPrivacyGroupEndorsementConcurrent",
                    "key": "carol_key_%d_%d",
					"value": "carol_value_%d_%d"
                }`, i, j, i, j)),
				},
			})
			require.NoError(t, err)
			assert.NotEqual(t, uuid.UUID{}, carolTxID[j])
		}

		//once all transactions for this iteration are sent, wait for all of them to be confirmed before starting the next iteration
		for j := 0; j < NUM_TRANSACTIONS_PER_NODE_PER_ITERATION; j++ {
			assert.Eventually(t,
				transactionReceiptCondition(t, ctx, aliceTxID[j], alice.client, false),
				transactionLatencyThreshold(t),
				100*time.Millisecond,
				"Transaction did not receive a receipt",
			)

			assert.Eventually(t,
				transactionReceiptCondition(t, ctx, bobTxID[j], bob.client, false),
				transactionLatencyThreshold(t),
				100*time.Millisecond,
				"Transaction did not receive a receipt",
			)

			assert.Eventually(t,
				transactionReceiptCondition(t, ctx, carolTxID[j], carol.client, false),
				transactionLatencyThreshold(t),
				100*time.Millisecond,
				fmt.Sprintf("Carol's transaction did not receive a receipt on iteration %d", i),
			)
		}
	}

	var schemas []*pldapi.Schema

	err = alice.client.CallRPC(ctx, &schemas, "pstate_listSchemas", "simpleStorageDomain")
	require.NoError(t, err)
	require.Len(t, schemas, 1)

	err = bob.client.CallRPC(ctx, &schemas, "pstate_listSchemas", "simpleStorageDomain")
	require.NoError(t, err)
	require.Len(t, schemas, 1)

	err = carol.client.CallRPC(ctx, &schemas, "pstate_listSchemas", "simpleStorageDomain")
	require.NoError(t, err)
	require.Len(t, schemas, 1)

	var aliceStates []*pldapi.State
	err = alice.client.CallRPC(ctx, &aliceStates, "pstate_queryContractStates", "simpleStorageDomain", contractAddress.String(), schemas[0].ID, pldtypes.RawJSON(`{}`), "available")
	require.NoError(t, err)
	require.Len(t, aliceStates, 1)

	var bobStates []*pldapi.State
	err = bob.client.CallRPC(ctx, &bobStates, "pstate_queryContractStates", "simpleStorageDomain", contractAddress.String(), schemas[0].ID, pldtypes.RawJSON(`{}`), "available")
	require.NoError(t, err)
	require.Len(t, bobStates, 1)
	assert.Equal(t, aliceStates[0].Data, bobStates[0].Data)

	var carolStates []*pldapi.State
	err = carol.client.CallRPC(ctx, &carolStates, "pstate_queryContractStates", "simpleStorageDomain", contractAddress.String(), schemas[0].ID, pldtypes.RawJSON(`{}`), "available")
	require.NoError(t, err)
	require.Len(t, carolStates, 1)
	assert.Equal(t, aliceStates[0].Data, carolStates[0].Data)

	stateData := make(map[string]string)
	storage := make(map[string]string)
	jsonErr := json.Unmarshal(aliceStates[0].Data.Bytes(), &stateData)
	require.NoError(t, jsonErr)

	jsonErr = json.Unmarshal([]byte(stateData["records"]), &storage)
	require.NoError(t, jsonErr)

	for i := 0; i < NUM_ITERATIONS; i++ {
		for j := 0; j < NUM_TRANSACTIONS_PER_NODE_PER_ITERATION; j++ {
			assert.Equal(t, fmt.Sprintf("alice_value_%d_%d", i, j), storage[fmt.Sprintf("alice_key_%d_%d", i, j)])
			assert.Equal(t, fmt.Sprintf("bob_value_%d_%d", i, j), storage[fmt.Sprintf("bob_key_%d_%d", i, j)])
			assert.Equal(t, fmt.Sprintf("carol_value_%d_%d", i, j), storage[fmt.Sprintf("carol_key_%d_%d", i, j)])
		}
	}

}
