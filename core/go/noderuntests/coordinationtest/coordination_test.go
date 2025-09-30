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

func TestTransactionSuccessAfterStartStopSingleNode(t *testing.T) {
	// We want to test that we can start some nodes, send some transactions, restart the nodes and send some more transactions

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

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, domains.SelfEndorsement, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	// Start a private transaction on alices node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bobs node
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

	// Start a private transaction on bobs node
	// This is a transfer which relies on bobs node being aware of the state created by alice's mint to bob above
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

	// Start a private transaction on alices node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bobs node
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

	t.Cleanup(func() {
		stopNode(t, alice)
	})
}

// MRW TODO - tempoorarily run the test twice to ensure node start/stop is reliable
func TestTransactionSuccessAfterStartStopSingleNodeAgain(t *testing.T) {
	// We want to test that we can start some nodes, send some transactions, restart the nodes and send some more transactions

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

	contractAddress := alice.DeploySimpleDomainInstanceContract(t, domains.SelfEndorsement, constructorParameters, transactionReceiptCondition, transactionLatencyThreshold)

	// Start a private transaction on alices node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bobs node
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

	// Start a private transaction on bobs node
	// This is a transfer which relies on bobs node being aware of the state created by alice's mint to bob above
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

	// Start a private transaction on alices node
	// this is a mint to bob so bob should later be able to do a transfer without any mint taking place on bobs node
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

	t.Cleanup(func() {
		stopNode(t, alice)
	})
}
