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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/componenttest/domains"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"

	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/signerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"sigs.k8s.io/yaml"
)

func TestRunSimpleStorageEthTransaction(t *testing.T) {
	//TODO refactor this to be more black box by using JSONRPC interface to invoke the public contract
	ctx := context.Background()
	logrus.SetLevel(logrus.DebugLevel)

	var testConfig pldconf.PaladinConfig

	err := yaml.Unmarshal([]byte(`
db:
  type: sqlite
  sqlite:
    dsn:           ":memory:"
    autoMigrate:   true
    migrationsDir: ../db/migrations/sqlite
    debugQueries:  false
blockIndexer:
  fromBlock: latest
blockchain:
  http:
    url: http://localhost:8545
  ws:
    url: ws://localhost:8546
    initialConnectAttempts: 25
wallets:
- name: wallet1
  keySelector: .*
  signer:
    keyDerivation:
      type: "bip32"
    keyStore:
      type: "static"
      static:
        keys:
          seed:
            encoding: none
            inline: polar mechanic crouch jungle field room dry sure machine brisk seed bulk student total ethics
`), &testConfig)
	require.NoError(t, err)

	p, err := persistence.NewPersistence(ctx, &testConfig.DB)
	require.NoError(t, err)
	defer p.Close()

	indexer, err := blockindexer.NewBlockIndexer(ctx, &pldconf.BlockIndexerConfig{
		FromBlock: json.RawMessage(`"latest"`), // don't want earlier events
	}, &testConfig.Blockchain.WS, p)
	require.NoError(t, err)

	type solBuild struct {
		ABI      abi.ABI                   `json:"abi"`
		Bytecode ethtypes.HexBytes0xPrefix `json:"bytecode"`
	}
	var simpleStorageBuild solBuild
	err = json.Unmarshal(simpleStorageBuildJSON, &simpleStorageBuild)
	require.NoError(t, err)

	eventStreamEvents := make(chan *blockindexer.EventWithData, 2 /* all the events we exepct */)
	err = indexer.Start(&blockindexer.InternalEventStream{
		Handler: func(ctx context.Context, tx *gorm.DB, batch *blockindexer.EventDeliveryBatch) (blockindexer.PostCommit, error) {
			// With SQLite we cannot hang in here with a DB TX - as there's only one per process.
			for _, e := range batch.Events {
				select {
				case eventStreamEvents <- e:
				default:
					assert.Fail(t, "more than expected number of events received")
				}
			}
			return nil, nil
		},
		Definition: &blockindexer.EventStream{
			Name: "unittest",
			Sources: []blockindexer.EventStreamSource{{
				ABI: abi.ABI{simpleStorageBuild.ABI.Events()["Changed"]},
			}},
		},
	})
	require.NoError(t, err)
	defer indexer.Stop()

	keyMgr, err := ethclient.NewSimpleTestKeyManager(ctx, (*signerapi.ConfigNoExt)(testConfig.Wallets[0].Signer))
	require.NoError(t, err)

	ecf, err := ethclient.NewEthClientFactoryWithKeyManager(ctx, keyMgr, &testConfig.Blockchain)
	require.NoError(t, err)
	err = ecf.Start()
	require.NoError(t, err)
	defer ecf.Stop()
	ethClient := ecf.HTTPClient()

	simpleStorage, err := ethClient.ABI(ctx, simpleStorageBuild.ABI)
	require.NoError(t, err)

	txHash1, err := simpleStorage.MustConstructor(tktypes.HexBytes(simpleStorageBuild.Bytecode)).R(ctx).
		Signer("key1").Input(`{"x":11223344}`).SignAndSend()
	require.NoError(t, err)
	deployTX, err := indexer.WaitForTransactionSuccess(ctx, *txHash1, simpleStorageBuild.ABI)
	require.NoError(t, err)
	contractAddr := deployTX.ContractAddress.Address0xHex()

	getX1, err := simpleStorage.MustFunction("get").R(ctx).To(contractAddr).CallResult()
	require.NoError(t, err)
	assert.JSONEq(t, `{"x":"11223344"}`, getX1.JSON())

	txHash2, err := simpleStorage.MustFunction("set").R(ctx).
		Signer("key1").To(contractAddr).Input(`{"_x":99887766}`).SignAndSend()
	require.NoError(t, err)
	_, err = indexer.WaitForTransactionSuccess(ctx, *txHash2, simpleStorageBuild.ABI)
	require.NoError(t, err)

	getX2, err := simpleStorage.MustFunction("get").R(ctx).To(contractAddr).CallResult()
	require.NoError(t, err)
	assert.JSONEq(t, `{"x":"99887766"}`, getX2.JSON())

	// Expect our event listener to be queued up with two Changed events
	event1 := <-eventStreamEvents
	assert.JSONEq(t, `{"x":"11223344"}`, string(event1.Data))
	event2 := <-eventStreamEvents
	assert.JSONEq(t, `{"x":"99887766"}`, string(event2.Data))

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
	instance := newInstanceForComponentTesting(t, deployDomainRegistry(t), nil, nil)
	rpcClient := instance.client

	// Check there are no transactions before we start
	var txns []*pldapi.TransactionFull
	err := rpcClient.CallRPC(ctx, &txns, "ptx_queryTransactions", query.NewQueryBuilder().Limit(1).Query(), true)
	require.NoError(t, err)
	assert.Len(t, txns, 0)
	var dplyTxID uuid.UUID

	err = rpcClient.CallRPC(ctx, &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenConstructorABI(),
		Transaction: pldapi.Transaction{
			IdempotencyKey: "deploy1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "domain1",
			From:           "wallets.org1.aaaaaa",
			Data: tktypes.RawJSON(`{
                    "notary": "domain1.contract1.notary",
                    "name": "FakeToken1",
                    "symbol": "FT1"
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
	err = rpcClient.CallRPC(ctx, &dplyTxFull, "ptx_getTransaction", dplyTxID, true)
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
		Transaction: pldapi.Transaction{
			To:             contractAddress,
			Domain:         "domain1", //TODO comments say that this is inferred from `to` for invoke
			IdempotencyKey: "tx1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           "wallets.org1.aaaaaa",
			Data: tktypes.RawJSON(`{
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

	err = rpcClient.CallRPC(ctx, &txns, "ptx_queryTransactions", query.NewQueryBuilder().Limit(2).Query(), true)
	require.NoError(t, err)
	assert.Len(t, txns, 2)

	txFull := pldapi.TransactionFull{}
	err = rpcClient.CallRPC(ctx, &txFull, "ptx_getTransaction", tx1ID, true)
	require.NoError(t, err)

	require.NotNil(t, txFull.Receipt)
	assert.True(t, txFull.Receipt.Success)
}

func TestPrivateTransactionsMintThenTransfer(t *testing.T) {
	// Invoke 2 transactions on the same contract where the second transaction relies on the state created by the first

	ctx := context.Background()
	instance := newInstanceForComponentTesting(t, deployDomainRegistry(t), nil, nil)
	rpcClient := instance.client

	// Check there are no transactions before we start
	var txns []*pldapi.TransactionFull
	err := rpcClient.CallRPC(ctx, &txns, "ptx_queryTransactions", query.NewQueryBuilder().Limit(1).Query(), true)
	require.NoError(t, err)
	assert.Len(t, txns, 0)
	var dplyTxID uuid.UUID
	err = rpcClient.CallRPC(ctx, &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenConstructorABI(),
		Transaction: pldapi.Transaction{
			IdempotencyKey: "deploy1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "domain1",
			From:           "wallets.org1.aaaaaa",
			Data: tktypes.RawJSON(`{
                    "notary": "domain1.contract1.notary",
                    "name": "FakeToken1",
                    "symbol": "FT1"
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
	err = rpcClient.CallRPC(ctx, &dplyTxFull, "ptx_getTransaction", dplyTxID, true)
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
		Transaction: pldapi.Transaction{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           "wallets.org1.aaaaaa",
			Data: tktypes.RawJSON(`{
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
		Transaction: pldapi.Transaction{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx2",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           "wallets.org1.bbbbbb",
			Data: tktypes.RawJSON(`{
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
	instance := newInstanceForComponentTesting(t, deployDomainRegistry(t), nil, nil)
	rpcClient := instance.client

	var dplyTxID uuid.UUID
	err := rpcClient.CallRPC(ctx, &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenConstructorABI(),
		Transaction: pldapi.Transaction{
			IdempotencyKey: "deploy1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "domain1",
			From:           "wallets.org1.aaaaaa",
			Data: tktypes.RawJSON(`{
					"notary": "domain1.contract1.notary",
					"name": "FakeToken1",
					"symbol": "FT1"
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
	err = rpcClient.CallRPC(ctx, &dplyTxFull, "ptx_getTransaction", dplyTxID, true)
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
		Transaction: pldapi.Transaction{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx2",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           "wallets.org1.bbbbbb",
			Data: tktypes.RawJSON(`{
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
	err = rpcClient.CallRPC(ctx, &txFull, "ptx_getTransaction", tx1ID, true)
	require.NoError(t, err)
	require.NotNil(t, txFull.Receipt)
	assert.False(t, txFull.Receipt.Success)
	assert.Regexp(t, domains.SimpleDomainInsufficientFundsError, txFull.Receipt.FailureMessage)
	assert.Regexp(t, "PD011802", txFull.Receipt.FailureMessage)

}

func TestDeployOnOneNodeInvokeOnAnother(t *testing.T) {
	// We use the simple token where there is no actual on chain checking of the notary
	// so either node can assemble a transaction with an attestation plan for a local notary
	// there is also no access control around minting so both nodes are able to mint tokens and we don't
	// need the complexity of cross node transfers in this test
	ctx := context.Background()

	domainRegistryAddress := deployDomainRegistry(t)

	instance1 := newInstanceForComponentTesting(t, domainRegistryAddress, nil, nil)
	client1 := instance1.client
	aliceIdentity := "wallets.org1.alice"
	aliceAddress := instance1.resolveEthereumAddress(aliceIdentity)
	t.Logf("Alice address: %s", aliceAddress)

	instance2 := newInstanceForComponentTesting(t, domainRegistryAddress, nil, nil)
	client2 := instance2.client
	bobIdentity := "wallets.org2.bob"
	bobAddress := instance2.resolveEthereumAddress(bobIdentity)
	t.Logf("Bob address: %s", bobAddress)

	//If this fails, it is most likely a bug in the test utils that configures each node with seed mnemonics
	assert.NotEqual(t, aliceAddress, bobAddress)

	// send JSON RPC message to node 1 to deploy a private contract, using alice's key
	var dplyTxID uuid.UUID
	err := client1.CallRPC(ctx, &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenConstructorABI(),
		Transaction: pldapi.Transaction{
			IdempotencyKey: "deploy1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "domain1",
			From:           aliceIdentity,
			Data: tktypes.RawJSON(`{
                    "notary": "domain1.contract1.notary",
                    "name": "FakeToken1",
                    "symbol": "FT1"
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
	err = client1.CallRPC(ctx, &dplyTxFull, "ptx_getTransaction", dplyTxID, true)
	require.NoError(t, err)
	contractAddress := dplyTxFull.Receipt.ContractAddress

	// Start a private transaction on alices node
	// this is a mint to alice
	var aliceTxID uuid.UUID
	err = client1.CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		Transaction: pldapi.Transaction{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-alice",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           aliceIdentity,
			Data: tktypes.RawJSON(`{
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
		Transaction: pldapi.Transaction{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-bob",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           bobIdentity,
			Data: tktypes.RawJSON(`{
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

	instance1 := newInstanceForComponentTesting(t, domainRegistryAddress, aliceNodeConfig, []*nodeConfiguration{bobNodeConfig})
	client1 := instance1.client
	aliceIdentity := "wallets.org1.alice@" + instance1.name
	aliceAddress := instance1.resolveEthereumAddress(aliceIdentity)
	t.Logf("Alice address: %s", aliceAddress)

	instance2 := newInstanceForComponentTesting(t, domainRegistryAddress, bobNodeConfig, []*nodeConfiguration{aliceNodeConfig})
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
	// We use the simple token where there is no actual on chain checking of the notary
	// so either node can assemble a transaction with an attestation plan for a local notary
	// however, in this test, Bob's transaction will only succeed if he can spend the coins that Alice transfers to him
	// so this tests that the state is shared between the nodes

	ctx := context.Background()

	aliceNodeConfig := newNodeConfiguration(t, "alice")
	bobNodeConfig := newNodeConfiguration(t, "bob")

	domainRegistryAddress := deployDomainRegistry(t)

	instance1 := newInstanceForComponentTesting(t, domainRegistryAddress, aliceNodeConfig, []*nodeConfiguration{bobNodeConfig})
	client1 := instance1.client
	aliceIdentity := "wallets.org1.alice@" + instance1.name
	aliceAddress := instance1.resolveEthereumAddress(aliceIdentity)
	t.Logf("Alice address: %s", aliceAddress)

	instance2 := newInstanceForComponentTesting(t, domainRegistryAddress, bobNodeConfig, []*nodeConfiguration{aliceNodeConfig})
	client2 := instance2.client
	bobIdentity := "wallets.org2.bob@" + instance2.name
	bobAddress := instance2.resolveEthereumAddress(bobIdentity)
	t.Logf("Bob address: %s", bobAddress)

	// send JSON RPC message to node 1 to deploy a private contract
	var dplyTxID uuid.UUID
	err := client1.CallRPC(ctx, &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenConstructorABI(),
		Transaction: pldapi.Transaction{
			IdempotencyKey: "deploy1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "domain1",
			From:           aliceIdentity,
			Data: tktypes.RawJSON(`{
                    "notary": "domain1.contract1.notary",
                    "name": "FakeToken1",
                    "symbol": "FT1"
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
	err = client1.CallRPC(ctx, &dplyTxFull, "ptx_getTransaction", dplyTxID, true)
	require.NoError(t, err)
	contractAddress := dplyTxFull.Receipt.ContractAddress

	// Start a private transaction on alices node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bobs node
	var aliceTxID uuid.UUID
	err = client1.CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		Transaction: pldapi.Transaction{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-alice",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           aliceIdentity,
			Data: tktypes.RawJSON(`{
                    "from": "",
                    "to": "` + bobIdentity + `",
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
	// This is a transfer which relies on bobs node being aware of the state created by alice's mint to bob above
	var bobTx1ID uuid.UUID
	err = client2.CallRPC(ctx, &bobTx1ID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		Transaction: pldapi.Transaction{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-bob",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           bobIdentity,
			Data: tktypes.RawJSON(`{
                    "from": "` + bobIdentity + `",
                    "to": "` + aliceIdentity + `",
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

	instance1 := newInstanceForComponentTesting(t, domainRegistryAddress, aliceNodeConfig, []*nodeConfiguration{bobNodeConfig, notaryNodeConfig})
	client1 := instance1.client
	aliceIdentity := "wallets.org1.alice@" + instance1.name
	aliceAddress := instance1.resolveEthereumAddress(aliceIdentity)
	t.Logf("Alice address: %s", aliceAddress)

	instance2 := newInstanceForComponentTesting(t, domainRegistryAddress, bobNodeConfig, []*nodeConfiguration{aliceNodeConfig, notaryNodeConfig})
	//client2 := instance2.client
	bobIdentity := "wallets.org2.bob@" + instance2.name
	bobAddress := instance2.resolveEthereumAddress(bobIdentity)
	t.Logf("Bob address: %s", bobAddress)

	instance3 := newInstanceForComponentTesting(t, domainRegistryAddress, notaryNodeConfig, []*nodeConfiguration{aliceNodeConfig, bobNodeConfig})
	client3 := instance3.client
	notaryIdentity := "wallets.org3.notary@" + instance3.name
	//notaryAddress := instance3.resolveEthereumAddress(notaryIdentity)
	t.Logf("Notary address: %s", bobAddress)

	// send JSON RPC message to node 3 ( notary) to deploy a private contract
	var dplyTxID uuid.UUID
	err := client3.CallRPC(ctx, &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenConstructorABI(),
		Transaction: pldapi.Transaction{
			IdempotencyKey: "deploy1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "domain1",
			From:           notaryIdentity,
			Data: tktypes.RawJSON(`{
					"notary": "` + notaryIdentity + `",
					"name": "FakeToken1",
					"symbol": "FT1"
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
	err = client3.CallRPC(ctx, &dplyTxFull, "ptx_getTransaction", dplyTxID, true)
	require.NoError(t, err)
	contractAddress := dplyTxFull.Receipt.ContractAddress

	// Start a private transaction on notary node
	// this is a mint to alice so alice should later be able to do a transfer to bob
	var mintTxID uuid.UUID
	err = client3.CallRPC(ctx, &mintTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		Transaction: pldapi.Transaction{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-mint",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           notaryIdentity,
			Data: tktypes.RawJSON(`{
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
		Transaction: pldapi.Transaction{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "transferA2B1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           aliceIdentity,
			Data: tktypes.RawJSON(`{
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
