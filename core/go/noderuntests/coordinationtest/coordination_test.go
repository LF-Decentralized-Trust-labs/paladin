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
	"testing"
	"time"

	testutils "github.com/LF-Decentralized-Trust-labs/paladin/core/noderuntests/pkg"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/noderuntests/pkg/domains"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Map of node names to config paths. Each node needs its own DB and static signing key
var CONFIG_PATHS = map[string]string{
	"alice": "../../test/config/postgres.coordinationtest.alice.config.yaml",
	"bob":   "../../test/config/postgres.coordinationtest.bob.config.yaml",
}

func deployDomainRegistry(t *testing.T, nodeName string) *pldtypes.EthAddress {
	return testutils.DeployDomainRegistry(t, CONFIG_PATHS[nodeName])
}

func startNode(t *testing.T, party testutils.Party, domainConfig interface{}) {
	party.Start(t, domainConfig, CONFIG_PATHS[party.GetName()], true)
}

func stopNode(t *testing.T, party testutils.Party) {
	party.Stop(t)
}

func TestTransactionSuccessPrivacyGroupEndorsement(t *testing.T) {
	// Test a regular privacy group endorsement transaction
	ctx := context.Background()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
		stopNode(t, bob)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator(), bob.GetIdentityLocator()},
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	var aliceTxID uuid.UUID
	idempotencyKey := uuid.New().String()
	err := alice.GetClient().CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-alice-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           alice.GetIdentity(),
			Data: pldtypes.RawJSON(`{
                    "from": "",
                    "to": "` + bob.GetIdentityLocator() + `",
                    "amount": "123000000000000000000"
                }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, aliceTxID)
	assert.Eventually(t,
		transactionReceiptConditionExpectedPublicTXCount(t, ctx, aliceTxID, alice.GetClient(), 1),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt with 1 public TX",
	)
	// Check bob has the public TX info as well
	assert.Eventually(t,
		transactionReceiptConditionExpectedPublicTXCount(t, ctx, aliceTxID, bob.GetClient(), 1),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt with 1 public TX",
	)

	// Check Alice and Bob both have the same view of the world
	aliceTxFull := pldapi.TransactionFull{}
	err = alice.GetClient().CallRPC(ctx, &aliceTxFull, "ptx_getTransactionFull", aliceTxID)
	require.NoError(t, err)
	require.NotNil(t, aliceTxFull)

	bobTxFull := pldapi.TransactionFull{}
	err = bob.GetClient().CallRPC(ctx, &bobTxFull, "ptx_getTransactionFull", aliceTxID)
	require.NoError(t, err)
	require.NotNil(t, bobTxFull)

	assert.Equal(t, aliceTxFull.ABIReference, bobTxFull.ABIReference)
	assert.Equal(t, aliceTxFull.Domain, bobTxFull.Domain)
	assert.Equal(t, aliceTxFull.Function, bobTxFull.Function)
	assert.Equal(t, aliceTxFull.From, bobTxFull.From)
	assert.Equal(t, aliceTxFull.To, bobTxFull.To)
	assert.Equal(t, aliceTxFull.Gas, bobTxFull.Gas)
	assert.Equal(t, aliceTxFull.GasPrice, bobTxFull.GasPrice)
	assert.Equal(t, aliceTxFull.Data, bobTxFull.Data)
	assert.Equal(t, aliceTxFull.Public[0].TransactionHash, bobTxFull.Public[0].TransactionHash)
	assert.Equal(t, aliceTxFull.Public[0].From, bobTxFull.Public[0].From)
	assert.Equal(t, aliceTxFull.Public[0].To, bobTxFull.Public[0].To)
	assert.Equal(t, aliceTxFull.Public[0].Value, bobTxFull.Public[0].Value)
	assert.Equal(t, aliceTxFull.Public[0].Gas, bobTxFull.Public[0].Gas)
	assert.Equal(t, aliceTxFull.Public[0].GasPrice, bobTxFull.Public[0].GasPrice)
}

func TestTransactionSuccessAfterStartStopSingleNode(t *testing.T) {
	// We want to test that we can start some nodes, send a transaction, restart the nodes and send some more transactions

	ctx := context.Background()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)

	t.Cleanup(func() {
		stopNode(t, bob)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	var aliceTxID uuid.UUID
	idempotencyKey := uuid.New().String()
	err := alice.GetClient().CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-alice-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           alice.GetIdentity(),
			Data: pldtypes.RawJSON(`{
                    "from": "",
                    "to": "` + bob.GetIdentityLocator() + `",
                    "amount": "123000000000000000000"
                }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, aliceTxID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, aliceTxID, alice.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	// Start a private transaction on bob's node
	// This is a transfer which relies on bob's node being aware of the state created by alice's mint to bob above
	var bobTx1ID uuid.UUID
	idempotencyKey = uuid.New().String()
	err = bob.GetClient().CallRPC(ctx, &bobTx1ID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-bob-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           bob.GetIdentity(),
			Data: pldtypes.RawJSON(`{
                    "from": "` + bob.GetIdentityLocator() + `",
                    "to": "` + alice.GetIdentityLocator() + `",
                    "amount": "123000000000000000000"
                }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, bobTx1ID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, bobTx1ID, bob.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	stopNode(t, alice)

	var verifierResult string
	err = bob.GetClient().CallRPC(ctx, &verifierResult, "ptx_resolveVerifier",
		bob.GetIdentityLocator(),
		algorithms.ECDSA_SECP256K1,
		verifiers.ETH_ADDRESS,
	)
	require.NoError(t, err)
	require.NotNil(t, verifierResult)

	err = alice.GetClient().CallRPC(ctx, &verifierResult, "ptx_resolveVerifier",
		bob.GetIdentityLocator(),
		algorithms.ECDSA_SECP256K1,
		verifiers.ETH_ADDRESS,
	)
	require.Error(t, err)
	require.NotNil(t, verifierResult)

	startNode(t, alice, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
	})

	err = alice.GetClient().CallRPC(ctx, &verifierResult, "ptx_resolveVerifier",
		alice.GetIdentityLocator(),
		algorithms.ECDSA_SECP256K1,
		verifiers.ETH_ADDRESS,
	)
	require.NoError(t, err)
	require.NotNil(t, verifierResult)

	err = alice.GetClient().CallRPC(ctx, &verifierResult, "ptx_resolveVerifier",
		bob.GetIdentityLocator(),
		algorithms.ECDSA_SECP256K1,
		verifiers.ETH_ADDRESS,
	)
	require.NoError(t, err)
	require.NotNil(t, verifierResult)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	idempotencyKey = uuid.New().String()
	err = alice.GetClient().CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx2-alice-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           alice.GetIdentity(),
			Data: pldtypes.RawJSON(`{
                    "from": "",
                    "to": "` + bob.GetIdentityLocator() + `",
                    "amount": "123000000000000000000"
                }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, aliceTxID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, aliceTxID, alice.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
}

func TestTransactionSuccessIfOneNodeStoppedButNotARequiredVerifier(t *testing.T) {
	// Test that we can start 2 nodes, then submit a transaction while one of them is stopped.
	// The  node that is stopped is not a required verifier so the transaction should succeed
	// without restarting that node.
	ctx := context.Background()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, bob)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	var aliceTxID uuid.UUID
	idempotencyKey := uuid.New().String()
	err := alice.GetClient().CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-alice-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           alice.GetIdentity(),
			Data: pldtypes.RawJSON(`{
                    "from": "",
                    "to": "` + bob.GetIdentityLocator() + `",
                    "amount": "123000000000000000000"
                }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, aliceTxID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, aliceTxID, alice.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	// Stop alice's node before submitting a transaction request to bob's node.
	stopNode(t, alice)

	// Start a private transaction on bob's node, TO bob's identifier. Alice isn't involved at all so isn't a required verifier
	var bobTx1ID uuid.UUID
	idempotencyKey = uuid.New().String()
	err = bob.GetClient().CallRPC(ctx, &bobTx1ID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-bob-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           bob.GetIdentity(),
			Data: pldtypes.RawJSON(`{
	                "from": "` + bob.GetIdentityLocator() + `",
	                "to": "` + bob.GetIdentityLocator() + `",
	                "amount": "123000000000000000000"
	            }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, bobTx1ID)

	// Check that even though alice's node is stopped, since it is not a required verifier
	// the transaction should succeed.
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, bobTx1ID, bob.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
}

func TestTransactionSuccessIfOneRequiredVerifierStoppedDuringSubmission(t *testing.T) {
	// Test that we can start 2 nodes, stop one of them, then submit a transaction where both nodes
	// are required verifiers. While one node is offline we shouldn't get a receipt. After the node
	// is restarted the transaction should proceed to completion.
	ctx := context.Background()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, bob)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	var aliceTxID uuid.UUID
	idempotencyKey := uuid.New().String()
	err := alice.GetClient().CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-alice-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           alice.GetIdentity(),
			Data: pldtypes.RawJSON(`{
                    "from": "",
                    "to": "` + bob.GetIdentityLocator() + `",
                    "amount": "123000000000000000000"
                }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, aliceTxID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, aliceTxID, alice.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	// Stop alice's node before submitting a transaction request to bob's node.
	stopNode(t, alice)

	// Start a private transaction on bob's node, TO alice's identifier. This can't proceed while her node is stopped.
	var bobTx1ID uuid.UUID
	idempotencyKey = uuid.New().String()
	err = bob.GetClient().CallRPC(ctx, &bobTx1ID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-bob-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           bob.GetIdentity(),
			Data: pldtypes.RawJSON(`{
	                "from": "` + bob.GetIdentityLocator() + `",
	                "to": "` + alice.GetIdentityLocator() + `",
	                "amount": "123000000000000000000"
	            }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, bobTx1ID)

	// Check that we don't receive a receipt in the usual time while alice's node is offline
	assert.Never(t,
		transactionReceiptCondition(t, ctx, bobTx1ID, bob.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction received a receipt that it shouldn't have",
	)

	startNode(t, alice, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
	})

	// Check that we did receive a receipt once alice's node was restarted
	customThreshold := 15 * time.Second
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, bobTx1ID, bob.GetClient(), false),
		transactionLatencyThresholdCustom(t, &customThreshold),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
}

func TestTransactionResumesIfBothRequiredVerifiersAreStoppedBeforeCompletion(t *testing.T) {
	// Test that we can start 2 nodes, stop one of them, then submit a transaction where both nodes
	// are required verifiers. While one node is offline we shouldn't get a receipt. We then stop
	// the remaining node so there are no active nodes. On restarting both, one should resume coordination
	// and the transaction should be successful.
	ctx := context.Background()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	var aliceTxID uuid.UUID
	idempotencyKey := uuid.New().String()
	err := alice.GetClient().CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-alice-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           alice.GetIdentity(),
			Data: pldtypes.RawJSON(`{
                    "from": "",
                    "to": "` + bob.GetIdentityLocator() + `",
                    "amount": "123000000000000000000"
                }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, aliceTxID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, aliceTxID, alice.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	// Stop alice's node before submitting a transaction request to bob's node.
	stopNode(t, alice)

	// Start a private transaction on bob's node, TO alice's identifier. This can't proceed while her node is stopped.
	var bobTx1ID uuid.UUID
	idempotencyKey = uuid.New().String()
	err = bob.GetClient().CallRPC(ctx, &bobTx1ID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-bob-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           bob.GetIdentity(),
			Data: pldtypes.RawJSON(`{
	                "from": "` + bob.GetIdentityLocator() + `",
	                "to": "` + alice.GetIdentityLocator() + `",
	                "amount": "123000000000000000000"
	            }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, bobTx1ID)

	// Check that we don't receive a receipt in the usual time while alice's node is offline
	assert.Never(t,
		transactionReceiptCondition(t, ctx, bobTx1ID, bob.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction received a receipt that it shouldn't have",
	)

	// Now stop bob's node as well.
	stopNode(t, bob)

	// Restart both nodes
	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
		stopNode(t, bob)
	})

	// Check that we did receive a receipt once the nodes restarted. Allow a little
	// more time for coordination selection etc.
	// customThreshold := 3 * time.Second
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, bobTx1ID, bob.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
}

func TestTransactionSuccessChainedTransaction(t *testing.T) {

	ctx := context.Background()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	// Create 2 parties, configured to use a hook address when the simple domain is invoked
	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)

	t.Cleanup(func() {
		stopNode(t, bob)
		stopNode(t, alice)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	// Deploy a token that will be call as a chained transaction, e.g. like a Pente hook contract
	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	constructorParameters = &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken2",
		Symbol:          "FT2",
		EndorsementMode: domains.SelfEndorsement,
		HookAddress:     contractAddress.String(), // Cause the contract to pass the request on to the contract at the hook address
	}

	// Deploy a token that will create a chained private transaction to the previous token e.g. like a Noto with a Pente hook
	chainedContractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	// Start a private transaction on alice's node. This should result in 2 Paladin transactions and 1 public transaction. The
	// original transaction should return a success receipt.
	var aliceTxID uuid.UUID
	idempotencyKey := uuid.New().String()
	err := alice.GetClient().CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             chainedContractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-alice-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           alice.GetIdentity(),
			Data: pldtypes.RawJSON(`{
	                "from": "",
	                "to": "` + bob.GetIdentityLocator() + `",
	                "amount": "123000000000000000000"
	            }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, aliceTxID)

	// Alice's node should have the full transaction and receipt
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, aliceTxID, alice.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	// Bob's node has the receipt
	assert.Eventually(t,
		transactionReceiptConditionReceiptOnly(t, ctx, aliceTxID, bob.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
}

func TestTransactionSuccessChainedTransactionSelfEndorsementThenPrivacyGroupEndorsement(t *testing.T) {

	ctx := context.Background()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	// Create 2 parties, configured to use a hook address when the simple domain is invoked
	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)

	t.Cleanup(func() {
		stopNode(t, bob)
		stopNode(t, alice)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator(), bob.GetIdentityLocator()},
	}

	// Deploy a token that will be called as a chained transaction, e.g. like a Pente hook contract
	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	constructorParameters = &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken2",
		Symbol:          "FT2",
		EndorsementMode: domains.SelfEndorsement,
		HookAddress:     contractAddress.String(), // Cause the contract to pass the request on to the contract at the hook address
	}

	// Deploy a token that will create a chained private transaction to the previous token e.g. like a Noto with a Pente hook
	chainedContractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	// Start a private transaction on alice's node. This should result in 2 Paladin transactions and 1 public transaction. The
	// original transaction should return a success receipt.
	var aliceTxID uuid.UUID
	idempotencyKey := uuid.New().String()
	err := alice.GetClient().CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             chainedContractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-alice-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           alice.GetIdentity(),
			Data: pldtypes.RawJSON(`{
	                "from": "",
	                "to": "` + bob.GetIdentityLocator() + `",
	                "amount": "123000000000000000000"
	            }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, aliceTxID)

	// Alice's node should have the full transaction and receipt
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, aliceTxID, alice.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	// Bob's node has the receipt, but not necesarily the original transaction
	assert.Eventually(t,
		transactionReceiptConditionReceiptOnly(t, ctx, aliceTxID, bob.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
}

func TestTransactionSuccessChainedTransactionPrivacyGroupEndorsementThenSelfEndorsement(t *testing.T) {

	ctx := context.Background()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	// Create 2 parties, configured to use a hook address when the simple domain is invoked
	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)

	t.Cleanup(func() {
		stopNode(t, bob)
		stopNode(t, alice)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	// Deploy a token that will be call as a chained transaction, e.g. like a Pente hook contract
	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	constructorParameters = &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken2",
		Symbol:          "FT2",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator(), bob.GetIdentityLocator()},
		HookAddress:     contractAddress.String(), // Cause the contract to pass the request on to the contract at the hook address
	}

	// Deploy a token that will create a chained private transaction to the previous token e.g. like a Noto with a Pente hook
	chainedContractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	// Start a private transaction on alice's node. This should result in 2 Paladin transactions and 1 public transaction. The
	// original transaction should return a success receipt.
	var aliceTxID uuid.UUID
	idempotencyKey := uuid.New().String()
	err := alice.GetClient().CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             chainedContractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-alice-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           alice.GetIdentity(),
			Data: pldtypes.RawJSON(`{
	                "from": "",
	                "to": "` + bob.GetIdentityLocator() + `",
	                "amount": "123000000000000000000"
	            }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, aliceTxID)

	// Alice's node should have the full transaction and receipt
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, aliceTxID, alice.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	txFull := pldapi.TransactionFull{}
	err = alice.GetClient().CallRPC(ctx, &txFull, "ptx_getTransactionFull", aliceTxID)
	require.NoError(t, err)

	// Bob's node has the receipt
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, aliceTxID, bob.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
}

func TestTransactionSuccessChainedTransactionPrivacyGroupEndorsementThenPrivacyGroupEndorsement(t *testing.T) {

	ctx := context.Background()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	// Create 2 parties, configured to use a hook address when the simple domain is invoked
	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ONE_TIME_USE_KEYS,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)

	t.Cleanup(func() {
		stopNode(t, bob)
		stopNode(t, alice)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator(), bob.GetIdentityLocator()},
	}

	// Deploy a token that will be call as a chained transaction, e.g. like a Pente hook contract
	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	constructorParameters = &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken2",
		Symbol:          "FT2",
		EndorsementMode: domains.PrivacyGroupEndorsement,
		EndorsementSet:  []string{alice.GetIdentityLocator(), bob.GetIdentityLocator()},
		HookAddress:     contractAddress.String(), // Cause the contract to pass the request on to the contract at the hook address
	}

	// Deploy a token that will create a chained private transaction to the previous token e.g. like a Noto with a Pente hook
	chainedContractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	// Start a private transaction on alice's node. This should result in 2 Paladin transactions and 1 public transaction. The
	// original transaction should return a success receipt.
	var aliceTxID uuid.UUID
	idempotencyKey := uuid.New().String()
	err := alice.GetClient().CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             chainedContractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-alice-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           alice.GetIdentity(),
			Data: pldtypes.RawJSON(`{
	                "from": "",
	                "to": "` + bob.GetIdentityLocator() + `",
	                "amount": "123000000000000000000"
	            }`),
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, aliceTxID)

	// Alice's node should have the full transaction and receipt
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, aliceTxID, alice.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)

	// Bob's node has the receipt
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, aliceTxID, bob.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
}

func TestTransactionRevertDuringAssembly(t *testing.T) {
	// Test that we can start 2 nodes, then submit a transaction while one of them is stopped.
	// The  node that is stopped is not a required verifier so the transaction should succeed
	// without restarting that node.
	ctx := context.Background()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
		stopNode(t, bob)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	var aliceTxID uuid.UUID
	idempotencyKey := uuid.New().String()
	err := alice.GetClient().CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-alice-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           alice.GetIdentity(),
			Data: pldtypes.RawJSON(`{
                    "from": "",
                    "to": "` + bob.GetIdentityLocator() + `",
                    "amount": "1001"
                }`), // Special value 1001 in the simple domain causes revert at assembly time
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, aliceTxID)
	assert.Eventually(t,
		transactionRevertedCondition(t, ctx, aliceTxID, alice.GetClient()),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive expected revert receipt",
	)
}

func TestTransactionRevertDuringEndorsement(t *testing.T) {
	// Test that we can start 2 nodes, then submit a transaction while one of them is stopped.
	// The  node that is stopped is not a required verifier so the transaction should succeed
	// without restarting that node.
	ctx := context.Background()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
		stopNode(t, bob)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	var aliceTxID uuid.UUID
	idempotencyKey := uuid.New().String()
	err := alice.GetClient().CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-alice-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           alice.GetIdentity(),
			Data: pldtypes.RawJSON(`{
                    "from": "",
                    "to": "` + bob.GetIdentityLocator() + `",
                    "amount": "1002"
                }`), // Special value 1002 in the simple domain causes revert at endorsement time
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, aliceTxID)
	assert.Eventually(t,
		transactionReceiptCondition(t, ctx, aliceTxID, alice.GetClient(), false),
		transactionLatencyThreshold(t),
		100*time.Millisecond,
		"Transaction did not receive a receipt",
	)
}

func TestTransactionRevertOnBaseLedger(t *testing.T) {
	// Test that we can start 2 nodes, then submit a transaction while one of them is stopped.
	// The  node that is stopped is not a required verifier so the transaction should succeed
	// without restarting that node.
	ctx := context.Background()
	domainRegistryAddress := deployDomainRegistry(t, "alice")

	alice := testutils.NewPartyForTesting(t, "alice", domainRegistryAddress)
	bob := testutils.NewPartyForTesting(t, "bob", domainRegistryAddress)

	alice.AddPeer(bob.GetNodeConfig())
	bob.AddPeer(alice.GetNodeConfig())

	domainConfig := &domains.SimpleDomainConfig{
		SubmitMode: domains.ENDORSER_SUBMISSION,
	}

	startNode(t, alice, domainConfig)
	startNode(t, bob, domainConfig)
	t.Cleanup(func() {
		stopNode(t, alice)
		stopNode(t, bob)
	})

	constructorParameters := &domains.ConstructorParameters{
		From:            alice.GetIdentity(),
		Name:            "FakeToken1",
		Symbol:          "FT1",
		EndorsementMode: domains.SelfEndorsement,
	}

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	// Start a private transaction on alice's node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bob's node
	var aliceTxID uuid.UUID
	idempotencyKey := uuid.New().String()
	err := alice.GetClient().CallRPC(ctx, &aliceTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenTransferABI(),
		TransactionBase: pldapi.TransactionBase{
			To:             contractAddress,
			Domain:         "domain1",
			IdempotencyKey: "tx1-alice-" + idempotencyKey,
			Type:           pldapi.TransactionTypePrivate.Enum(),
			From:           alice.GetIdentity(),
			Data: pldtypes.RawJSON(`{
                    "from": "",
                    "to": "` + bob.GetIdentityLocator() + `",
                    "amount": "1003"
                }`), // Special value 1003 in the simple domain causes revert once on the base ledger, then subsequently be successful
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, aliceTxID)
	customDuration := 5 * time.Second
	assert.Eventually(t,
		transactionReceiptConditionExpectedPublicTXCount(t, ctx, aliceTxID, alice.GetClient(), 2),
		transactionLatencyThresholdCustom(t, &customDuration),
		100*time.Millisecond,
		"Transaction did not receive expected receipt or have the expected number of public transactions",
	)
}
