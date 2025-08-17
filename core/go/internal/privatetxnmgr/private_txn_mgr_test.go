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

package privatetxnmgr

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signpayloads"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/blockindexer"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence/mockpersistence"
	pbEngine "github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/proto/engine"
)

/*
 * There are 2 flavours of test in this file
 * 1. Package level tests: Tests that assert the behavior of the private transaction manager package as a whole component in isolation from the rest of the system.
 * .  None of the code from this package is mocked in these tests and the only interfaces that the test calls are public interfaces, defined on the components package.
 * 2. Unit tests: Tests that assert the nuanced behavior of the functions in the private_txn_mgr.go file to provide more granular coverage of the codebase.
 * .  These tests are more white box in nature and mock other functions and interfaces within this package (and it `ptmgrtypes` subpackage)
 */

/* Package level tests */

func mockWritePublicTxsOk(mocks *dependencyMocks) chan struct{} {
	mockPublicTxManager := mocks.publicTxManager
	mockPublicTxManager.On("ValidateTransaction", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	dispatched := make(chan struct{})
	mwtx := mockPublicTxManager.On("WriteNewTransactions", mock.Anything, mock.Anything, mock.Anything)
	mwtx.Run(func(args mock.Arguments) {
		txs := args[2].([]*components.PublicTxSubmission)
		res := make([]*pldapi.PublicTx, len(txs))
		for i, tx := range txs {
			res[i] = &pldapi.PublicTx{
				LocalID:         confutil.P(uint64(1000 + i)),
				From:            *tx.From,
				To:              tx.To,
				Data:            tx.Data,
				PublicTxOptions: tx.PublicTxOptions,
			}
		}
		mwtx.Return(res, nil)
		if dispatched != nil {
			close(dispatched)
			dispatched = nil
		}
	})
	return dispatched
}

func inMsgToOut(fromNode string, send *components.FireAndForgetMessageSend) *components.ReceivedMessage {
	return &components.ReceivedMessage{
		FromNode:      fromNode,
		MessageID:     uuid.New(),
		CorrelationID: send.CorrelationID,
		MessageType:   send.MessageType,
		Payload:       send.Payload,
	}
}

func TestPrivateTxManagerInit(t *testing.T) {

	privateTxManager, mocks := NewPrivateTransactionMgrForPackageTesting(t, "node1")
	err := privateTxManager.PostInit(mocks.allComponents)
	require.NoError(t, err)
}

func TestPrivateTxManagerInvalidTransactionMissingDomain(t *testing.T) {
	t.Skip("This test is not valid because the code accepts empty domain. TODO: remove this test or change the code and migrate any consumers")
	ctx := context.Background()

	privateTxManager, mocks := NewPrivateTransactionMgrForPackageTesting(t, "node1")
	domainAddress := pldtypes.MustEthAddress(pldtypes.RandHex(20))
	mocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, mock.Anything, *domainAddress).Return(mocks.domainSmartContract, nil)

	err := privateTxManager.PostInit(mocks.allComponents)
	require.NoError(t, err)

	err = privateTxManager.Start()
	require.NoError(t, err)

	err = privateTxManager.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return privateTxManager.HandleNewTx(ctx, dbTX, &components.ValidatedTransaction{
			ResolvedTransaction: components.ResolvedTransaction{
				Function: &components.ResolvedFunction{
					Definition: testABI[0],
				},
				Transaction: &pldapi.Transaction{
					ID: confutil.P(uuid.New()),
					TransactionBase: pldapi.TransactionBase{
						To:   domainAddress,
						From: "alice@node1",
					},
				},
			},
		})
	})
	// no input domain should err
	assert.Regexp(t, "PD011800", err)
}

func TestPrivateTxManagerInvalidTransactionMismatchedDomain(t *testing.T) {
	ctx := context.Background()

	privateTxManager, mocks := NewPrivateTransactionMgrForPackageTesting(t, "node1")
	domainAddress := pldtypes.MustEthAddress(pldtypes.RandHex(20))
	mocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, mock.Anything, *domainAddress).Return(mocks.domainSmartContract, nil)
	mocks.domainSmartContract.On("Address").Return(*domainAddress)

	err := privateTxManager.PostInit(mocks.allComponents)
	require.NoError(t, err)

	err = privateTxManager.Start()
	require.NoError(t, err)

	err = privateTxManager.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return privateTxManager.HandleNewTx(ctx, dbTX, &components.ValidatedTransaction{
			ResolvedTransaction: components.ResolvedTransaction{
				Function: &components.ResolvedFunction{
					Definition: testABI[0],
				},
				Transaction: &pldapi.Transaction{
					ID: confutil.P(uuid.New()),
					TransactionBase: pldapi.TransactionBase{
						To:     domainAddress,
						Domain: "domain2",
						From:   "alice@node1",
					},
				},
			},
		})
	})
	// no input domain should err
	assert.Regexp(t, "PD011825", err)
}

func TestPrivateTxManagerInvalidTransactionEmptyAddress(t *testing.T) {
	ctx := context.Background()

	privateTxManager, mocks := NewPrivateTransactionMgrForPackageTesting(t, "node1")
	domainAddress := &pldtypes.EthAddress{}

	err := privateTxManager.PostInit(mocks.allComponents)
	require.NoError(t, err)

	err = privateTxManager.Start()
	require.NoError(t, err)

	err = privateTxManager.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return privateTxManager.HandleNewTx(ctx, dbTX, &components.ValidatedTransaction{
			ResolvedTransaction: components.ResolvedTransaction{
				Function: &components.ResolvedFunction{
					Definition: testABI[0],
				},
				Transaction: &pldapi.Transaction{
					ID: confutil.P(uuid.New()),
					TransactionBase: pldapi.TransactionBase{
						To:     domainAddress,
						Domain: "domain1",
						From:   "alice@node1",
					},
				},
			},
		})
	})
	// no input domain should err
	assert.Regexp(t, "PD011811", err)
}

func mockDCFlushWithWaiter(mocks *dependencyMocks) chan struct{} {
	dcFlushed := make(chan struct{})
	mocks.domainContext.On("Flush", mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			close(dcFlushed)
		})
	return dcFlushed
}

func TestPrivateTxManagerSimpleTransaction(t *testing.T) {
	//Submit a transaction that gets assembled with an attestation plan for a local endorser to sign the transaction
	ctx := context.Background()

	domainAddress := pldtypes.MustEthAddress(pldtypes.RandHex(20))
	privateTxManager, mocks := NewPrivateTransactionMgrForPackageTesting(t, "node1")
	mocks.mockDomain(domainAddress)

	domainAddressString := domainAddress.String()

	// unqualified lookup string because everything is local
	alice := newPartyForTesting(ctx, "alice", "node1", mocks)
	notary := newPartyForTesting(ctx, "notary", "node1", mocks)

	initialised := make(chan struct{}, 1)
	mocks.domainSmartContract.On("InitTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.PreAssembly = &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: alice.identityLocator,
			},
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{
					Lookup:       alice.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
				{
					Lookup:       notary.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
		}
		initialised <- struct{}{}
	}).Return(nil)

	mocks.identityResolver.On("ResolveVerifierAsync", mock.Anything, alice.identityLocator, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		resovleFn := args.Get(4).(func(context.Context, string))
		resovleFn(ctx, alice.verifier)
	}).Return(nil)
	mocks.identityResolver.On("ResolveVerifierAsync", mock.Anything, notary.identityLocator, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		resovleFn := args.Get(4).(func(context.Context, string))
		resovleFn(ctx, notary.verifier)
	}).Return(nil)
	// TODO check that the transaction is signed with this key

	assembled := make(chan struct{}, 1)
	mocks.domainSmartContract.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	mocks.domainSmartContract.On("Address").Return(*domainAddress)
	endorsePayload := []byte("some-endorsement-bytes")
	mocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(2).(*components.PrivateTransaction)

		tx.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			InputStates: []*components.FullState{
				{
					ID:     pldtypes.RandBytes(32),
					Schema: pldtypes.RandBytes32(),
					Data:   pldtypes.JSONString("foo"),
				},
			},
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "notary",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						notary.identityLocator,
					},
				},
			},
		}
		assembled <- struct{}{}

	}).Return(nil)

	notaryKeyMapping := &pldapi.KeyMappingAndVerifier{
		KeyMappingWithPath: &pldapi.KeyMappingWithPath{KeyMapping: &pldapi.KeyMapping{
			Identifier: notary.identity,
			KeyHandle:  notary.keyHandle,
		}},
		Verifier: &pldapi.KeyVerifier{Verifier: notary.verifier},
	}
	mocks.keyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, notary.identity, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).Return(notaryKeyMapping, nil)

	//TODO match endorsement request and verifier args
	mocks.domainSmartContract.On("EndorseTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: endorsePayload,
		Endorser: &prototk.ResolvedVerifier{
			Lookup:       notary.identityLocator,
			Verifier:     notary.verifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}, nil)

	mocks.keyManager.On("Sign", mock.Anything, notaryKeyMapping, signpayloads.OPAQUE_TO_RSV, mock.Anything).
		Return([]byte("notary-signature-bytes"), nil)

	mocks.domainSmartContract.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Return(nil).Run(
		func(args mock.Arguments) {
			cv, err := testABI[0].Inputs.ParseExternalData(map[string]any{
				"inputs":  []any{pldtypes.RandBytes32()},
				"outputs": []any{pldtypes.RandBytes32()},
				"data":    "0xfeedbeef",
			})
			require.NoError(t, err)
			tx := args[2].(*components.PrivateTransaction)
			tx.Signer = "signer1"
			jsonData, _ := cv.JSON()
			tx.PreparedPublicTransaction = &pldapi.TransactionInput{
				ABI: abi.ABI{testABI[0]},
				TransactionBase: pldapi.TransactionBase{
					To:              domainAddress,
					Data:            pldtypes.RawJSON(jsonData),
					PublicTxOptions: pldapi.PublicTxOptions{Gas: confutil.P(pldtypes.HexUint64(100000))},
				},
			}
		},
	)
	testTransactionID := confutil.P(uuid.New())

	signingAddr := pldtypes.RandAddress()
	mocks.keyManager.On("ResolveEthAddressBatchNewDatabaseTX", mock.Anything, []string{"signer1"}).
		Return([]*pldtypes.EthAddress{signingAddr}, nil)

	_ = mockWritePublicTxsOk(mocks)

	dcFlushed := mockDCFlushWithWaiter(mocks)

	err := privateTxManager.Start()
	require.NoError(t, err)

	tx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID: testTransactionID,
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					To:     domainAddress,
					From:   alice.identityLocator,
				},
			},
		},
	}
	mocks.txManager.On("GetResolvedTransactionByID", mock.Anything, mock.Anything).Return(&tx.ResolvedTransaction, nil)
	err = privateTxManager.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return privateTxManager.HandleNewTx(ctx, dbTX, tx)
	})
	require.NoError(t, err)

	// testTimeout := 2 * time.Second
	testTimeout := 100 * time.Minute
	status := pollForStatus(ctx, t, "dispatched", privateTxManager, domainAddressString, testTransactionID.String(), testTimeout)
	assert.Equal(t, "dispatched", status)

	<-dcFlushed

	privateTxManager.Stop()

}

func TestPrivateTxManagerSimplePreparedTransaction(t *testing.T) {
	//Prepare a transaction that gets assembled with an attestation plan for a local endorser to sign the transaction
	// submit mode external means the transaction does not get dispatched to the public tx manager
	// but should be distributed to the sender

	t.Skip("Test incomplete")
	//TODO: this is a challenging test to write because the nature of these tests is that we mock most things outside the privatetxnmgr package (and its subpackages)
	// however, in this use case, the integration between privatetxnmgr code and the TxManager includes database integration
	// specifically, privatetxnmgr writes prepared transaction distribtion records to the database which have a foreign key to the prepared transaction
	// we have a few options here
	// 1. In the mock for WritePreparedTransactions do a DB insert to the prepared_txns table.  This should be trivial because we receive the DB transaction
	// 2. Given that the privatetxnmgr package only deals with in memory objects, and all DB persistence is delegated to its subpackage `syncpoints` we could mock the syncpoints package
	// .  in these tests and rely on component tests for the full integration between packages and the database
	// Currently leaning towards option 2 but will need some refactoring of the code to allow us to inject the syncpoints mocks into the privatetxnmgr package.  We also call the real state distribution
	// package which really should be changed to be a mock to align with this testing strategy
	ctx := context.Background()

	domainAddress := pldtypes.MustEthAddress(pldtypes.RandHex(20))
	privateTxManager, mocks := NewPrivateTransactionMgrForPackageTesting(t, "node1")
	mocks.mockDomain(domainAddress)

	domainAddressString := domainAddress.String()

	// unqualified lookup string because everything is local
	alice := newPartyForTesting(ctx, "alice", "node1", mocks)
	notary := newPartyForTesting(ctx, "notary", "node1", mocks)

	initialised := make(chan struct{}, 1)
	mocks.domainSmartContract.On("InitTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.PreAssembly = &components.TransactionPreAssembly{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{
					Lookup:       alice.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
				{
					Lookup:       notary.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
		}
		initialised <- struct{}{}
	}).Return(nil)

	mocks.identityResolver.On("ResolveVerifierAsync", mock.Anything, alice.identityLocator, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		resovleFn := args.Get(4).(func(context.Context, string))
		resovleFn(ctx, alice.verifier)
	}).Return(nil)
	mocks.identityResolver.On("ResolveVerifierAsync", mock.Anything, notary.identityLocator, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		resovleFn := args.Get(4).(func(context.Context, string))
		resovleFn(ctx, notary.verifier)
	}).Return(nil)
	// TODO check that the transaction is signed with this key

	assembled := make(chan struct{}, 1)
	mocks.domainSmartContract.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	endorsePayload := []byte("some-endorsement-bytes")
	mocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(2).(*components.PrivateTransaction)

		tx.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			InputStates: []*components.FullState{
				{
					ID:     pldtypes.RandBytes(32),
					Schema: pldtypes.RandBytes32(),
					Data:   pldtypes.JSONString("foo"),
				},
			},
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "notary",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						notary.identityLocator,
					},
				},
			},
		}
		assembled <- struct{}{}

	}).Return(nil)

	notaryKeyMapping := &pldapi.KeyMappingAndVerifier{
		KeyMappingWithPath: &pldapi.KeyMappingWithPath{KeyMapping: &pldapi.KeyMapping{
			Identifier: notary.identity,
			KeyHandle:  notary.keyHandle,
		}},
		Verifier: &pldapi.KeyVerifier{Verifier: notary.verifier},
	}
	mocks.keyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, notary.identity, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).Return(notaryKeyMapping, nil)

	//TODO match endorsement request and verifier args
	mocks.domainSmartContract.On("EndorseTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: endorsePayload,
		Endorser: &prototk.ResolvedVerifier{
			Lookup:       notary.identityLocator,
			Verifier:     notary.verifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}, nil)

	mocks.keyManager.On("Sign", mock.Anything, notaryKeyMapping, signpayloads.OPAQUE_TO_RSV, mock.Anything).
		Return([]byte("notary-signature-bytes"), nil)

	mocks.domainSmartContract.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Return(nil).Run(
		func(args mock.Arguments) {
			cv, err := testABI[0].Inputs.ParseExternalData(map[string]any{
				"inputs":  []any{pldtypes.RandBytes32()},
				"outputs": []any{pldtypes.RandBytes32()},
				"data":    "0xfeedbeef",
			})
			require.NoError(t, err)
			tx := args[2].(*components.PrivateTransaction)
			tx.Signer = "signer1"
			jsonData, _ := cv.JSON()
			tx.PreparedPublicTransaction = &pldapi.TransactionInput{
				ABI: abi.ABI{testABI[0]},
				TransactionBase: pldapi.TransactionBase{
					To:              domainAddress,
					Data:            pldtypes.RawJSON(jsonData),
					PublicTxOptions: pldapi.PublicTxOptions{Gas: confutil.P(pldtypes.HexUint64(100000))},
				},
			}
		},
	)
	testTransactionID := confutil.P(uuid.New())

	_ = mockWritePublicTxsOk(mocks)

	mocks.txManager.On("WritePreparedTransactions", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	dcFlushed := mockDCFlushWithWaiter(mocks)

	err := privateTxManager.Start()
	require.NoError(t, err)

	tx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID:         testTransactionID,
				SubmitMode: pldapi.SubmitModeExternal.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					To:     domainAddress,
					From:   alice.identityLocator,
				},
			},
		},
	}
	err = privateTxManager.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return privateTxManager.HandleNewTx(ctx, dbTX, tx)
	})
	require.NoError(t, err)

	// testTimeout := 2 * time.Second
	testTimeout := 100 * time.Minute
	status := pollForStatus(ctx, t, "prepared", privateTxManager, domainAddressString, testTransactionID.String(), testTimeout)
	assert.Equal(t, "prepared", status)

	<-dcFlushed

	privateTxManager.Stop()

}

func TestPrivateTxManagerMultipleSignature(t *testing.T) {
	//Submit a transaction that gets assembled with an attestation plan for 2 signers
	ctx := context.Background()

	domainAddress := pldtypes.MustEthAddress(pldtypes.RandHex(20))
	privateTxManager, mocks := NewPrivateTransactionMgrForPackageTesting(t, "node1")
	mocks.mockDomain(domainAddress)

	domainAddressString := domainAddress.String()

	// unqualified lookup string because everything is local
	alice := newPartyForTesting(ctx, "alice", "node1", mocks)
	bob := newPartyForTesting(ctx, "bob", "node1", mocks)
	notary := newPartyForTesting(ctx, "notary", "node1", mocks)

	initialised := make(chan struct{}, 1)
	mocks.domainSmartContract.On("InitTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.PreAssembly = &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: alice.identityLocator,
			},
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{
					Lookup:       alice.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
				{
					Lookup:       bob.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
				{
					Lookup:       notary.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
		}
		initialised <- struct{}{}
	}).Return(nil)

	mocks.identityResolver.On("ResolveVerifierAsync", mock.Anything, alice.identityLocator, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		resovleFn := args.Get(4).(func(context.Context, string))
		resovleFn(ctx, alice.verifier)
	}).Return(nil)
	mocks.identityResolver.On("ResolveVerifierAsync", mock.Anything, bob.identityLocator, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		resovleFn := args.Get(4).(func(context.Context, string))
		resovleFn(ctx, alice.verifier)
	}).Return(nil)
	mocks.identityResolver.On("ResolveVerifierAsync", mock.Anything, notary.identityLocator, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		resovleFn := args.Get(4).(func(context.Context, string))
		resovleFn(ctx, notary.verifier)
	}).Return(nil)
	// TODO check that the transaction is signed with this key

	assembled := make(chan struct{}, 1)
	mocks.domainSmartContract.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	mocks.domainSmartContract.On("Address").Return(*domainAddress)
	endorsePayload := []byte("some-endorsement-bytes")
	mocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(2).(*components.PrivateTransaction)

		tx.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			InputStates: []*components.FullState{
				{
					ID:     pldtypes.RandBytes(32),
					Schema: pldtypes.RandBytes32(),
					Data:   pldtypes.JSONString("foo"),
				},
			},
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "alice",
					AttestationType: prototk.AttestationType_SIGN,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						alice.identityLocator,
					},
				},
				{
					Name:            "bob",
					AttestationType: prototk.AttestationType_SIGN,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						bob.identityLocator,
					},
				},
				{
					Name:            "notary",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						notary.identityLocator,
					},
				},
			},
		}
		assembled <- struct{}{}

	}).Return(nil)

	notaryKeyMapping := &pldapi.KeyMappingAndVerifier{
		KeyMappingWithPath: &pldapi.KeyMappingWithPath{KeyMapping: &pldapi.KeyMapping{
			Identifier: notary.identity,
			KeyHandle:  notary.keyHandle,
		}},
		Verifier: &pldapi.KeyVerifier{Verifier: notary.verifier},
	}
	mocks.keyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, notary.identity, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).Return(notaryKeyMapping, nil)

	//TODO match endorsement request and verifier args
	mocks.domainSmartContract.On("EndorseTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: endorsePayload,
		Endorser: &prototk.ResolvedVerifier{
			Lookup:       notary.identityLocator,
			Verifier:     notary.verifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}, nil)

	mocks.keyManager.On("Sign", mock.Anything, notaryKeyMapping, signpayloads.OPAQUE_TO_RSV, mock.Anything).
		Return([]byte("notary-signature-bytes"), nil)

	aliceKeyMapping := &pldapi.KeyMappingAndVerifier{
		KeyMappingWithPath: &pldapi.KeyMappingWithPath{KeyMapping: &pldapi.KeyMapping{
			Identifier: alice.identity,
			KeyHandle:  alice.keyHandle,
		}},
		Verifier: &pldapi.KeyVerifier{Verifier: alice.verifier},
	}

	mocks.keyManager.On("Sign", mock.Anything, aliceKeyMapping, signpayloads.OPAQUE_TO_RSV, mock.Anything).
		Return([]byte("notary-signature-bytes"), nil)

	bobKeyMapping := &pldapi.KeyMappingAndVerifier{
		KeyMappingWithPath: &pldapi.KeyMappingWithPath{KeyMapping: &pldapi.KeyMapping{
			Identifier: bob.identity,
			KeyHandle:  bob.keyHandle,
		}},
		Verifier: &pldapi.KeyVerifier{Verifier: bob.verifier},
	}

	mocks.keyManager.On("Sign", mock.Anything, bobKeyMapping, signpayloads.OPAQUE_TO_RSV, mock.Anything).
		Return([]byte("notary-signature-bytes"), nil)

	mocks.domainSmartContract.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Return(nil).Run(
		func(args mock.Arguments) {
			cv, err := testABI[0].Inputs.ParseExternalData(map[string]any{
				"inputs":  []any{pldtypes.RandBytes32()},
				"outputs": []any{pldtypes.RandBytes32()},
				"data":    "0xfeedbeef",
			})
			require.NoError(t, err)
			tx := args[2].(*components.PrivateTransaction)
			tx.Signer = "signer1"
			jsonData, _ := cv.JSON()
			tx.PreparedPublicTransaction = &pldapi.TransactionInput{
				ABI: abi.ABI{testABI[0]},
				TransactionBase: pldapi.TransactionBase{
					To:              domainAddress,
					Data:            pldtypes.RawJSON(jsonData),
					PublicTxOptions: pldapi.PublicTxOptions{Gas: confutil.P(pldtypes.HexUint64(100000))},
				},
			}
		},
	)
	testTransactionID := confutil.P(uuid.New())

	signingAddr := pldtypes.RandAddress()
	mocks.keyManager.On("ResolveEthAddressBatchNewDatabaseTX", mock.Anything, []string{"signer1"}).
		Return([]*pldtypes.EthAddress{signingAddr}, nil)

	_ = mockWritePublicTxsOk(mocks)

	dcFlushed := mockDCFlushWithWaiter(mocks)

	err := privateTxManager.Start()
	require.NoError(t, err)

	tx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID: testTransactionID,
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					To:     domainAddress,
					From:   alice.identityLocator,
				},
			},
		},
	}
	mocks.txManager.On("GetResolvedTransactionByID", mock.Anything, mock.Anything).Return(&tx.ResolvedTransaction, nil)
	err = privateTxManager.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return privateTxManager.HandleNewTx(ctx, dbTX, tx)
	})
	require.NoError(t, err)

	// testTimeout := 2 * time.Second
	testTimeout := 100 * time.Minute
	status := pollForStatus(ctx, t, "dispatched", privateTxManager, domainAddressString, testTransactionID.String(), testTimeout)
	assert.Equal(t, "dispatched", status)

	<-dcFlushed

	privateTxManager.Stop()

}

func TestPrivateTxManagerLocalEndorserSubmits(t *testing.T) {
}

func TestPrivateTxManagerRevertFromLocalEndorsement(t *testing.T) {
}

func TestPrivateTxManagerRemoteNotaryEndorser(t *testing.T) {
	ctx := context.Background()
	// A transaction that requires exactly one endorsement from a notary (as per noto) and therefore delegates coordination of the transaction to that node

	domainAddress := pldtypes.MustEthAddress(pldtypes.RandHex(20))
	localNodeName := "localNode"
	remoteNodeName := "remoteNode"
	privateTxManager, localNodeMocks := NewPrivateTransactionMgrForPackageTesting(t, localNodeName)
	localNodeMocks.mockDomain(domainAddress)

	domainAddressString := domainAddress.String()

	remoteEngine, remoteEngineMocks := NewPrivateTransactionMgrForPackageTesting(t, remoteNodeName)
	remoteEngineMocks.mockDomain(domainAddress)

	alice := newPartyForTesting(ctx, "alice", localNodeName, localNodeMocks)
	notary := newPartyForTesting(ctx, "notary", remoteNodeName, remoteEngineMocks)

	alice.mockResolve(ctx, notary)
	notary.mockResolve(ctx, alice)

	initialised := make(chan struct{}, 1)
	localNodeMocks.domainSmartContract.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    &notary.identityLocator,
	})
	localNodeMocks.domainSmartContract.On("Address").Return(*domainAddress)
	localNodeMocks.domainSmartContract.On("InitTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.PreAssembly = &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: alice.identityLocator,
			},
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{
					Lookup:       alice.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
				{
					Lookup:       notary.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
		}
		initialised <- struct{}{}
	}).Return(nil)

	assembled := make(chan struct{}, 1)

	localNodeMocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(2).(*components.PrivateTransaction)

		tx.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			InputStates: []*components.FullState{
				{
					ID:     pldtypes.RandBytes(32),
					Schema: pldtypes.RandBytes32(),
					Data:   pldtypes.JSONString("foo"),
				},
			},
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "notary",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						notary.identityLocator,
					},
				},
			},
		}
		assembled <- struct{}{}

	}).Return(nil)

	localNodeMocks.transportManager.On("Send", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		go func() {
			assert.Equal(t, remoteNodeName, args.Get(1).(*components.FireAndForgetMessageSend).Node)
			send := args.Get(1).(*components.FireAndForgetMessageSend)
			remoteEngine.HandlePaladinMsg(ctx, inMsgToOut(localNodeName, send))
		}()
	}).Return(nil).Maybe()

	remoteEngineMocks.transportManager.On("Send", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		go func() {
			send := args.Get(1).(*components.FireAndForgetMessageSend)
			privateTxManager.HandlePaladinMsg(ctx, inMsgToOut(remoteNodeName, send))
		}()
	}).Return(nil).Maybe()

	remoteEngineMocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, mock.Anything, *domainAddress).Return(remoteEngineMocks.domainSmartContract, nil)

	//TODO match endorsement request and verifier args
	remoteEngineMocks.domainSmartContract.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	remoteEngineMocks.domainSmartContract.On("EndorseTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: []byte("some-endorsement-bytes"),
		Endorser: &prototk.ResolvedVerifier{
			Lookup:       notary.identityLocator, //matches whatever was specified in PreAssembly.RequiredVerifiers
			Verifier:     notary.verifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}, nil)

	notary.mockSign([]byte("some-endorsement-bytes"), []byte("some-signature-bytes"))

	remoteEngineMocks.domainSmartContract.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Return(nil).Run(
		func(args mock.Arguments) {
			cv, err := testABI[0].Inputs.ParseExternalData(map[string]any{
				"inputs":  []any{pldtypes.RandBytes32()},
				"outputs": []any{pldtypes.RandBytes32()},
				"data":    "0xfeedbeef",
			})
			require.NoError(t, err)
			tx := args[2].(*components.PrivateTransaction)
			tx.Signer = "signer1"
			jsonData, _ := cv.JSON()
			tx.PreparedPublicTransaction = &pldapi.TransactionInput{
				ABI: abi.ABI{testABI[0]},
				TransactionBase: pldapi.TransactionBase{
					To:              domainAddress,
					Data:            pldtypes.RawJSON(jsonData),
					PublicTxOptions: pldapi.PublicTxOptions{Gas: confutil.P(pldtypes.HexUint64(100000))},
				},
			}
		},
	)
	testTransactionID := confutil.P(uuid.New())

	_ = mockWritePublicTxsOk(remoteEngineMocks)

	signingAddr := pldtypes.RandAddress()
	remoteEngineMocks.keyManager.On("ResolveEthAddressBatchNewDatabaseTX", mock.Anything, []string{"signer1"}).
		Return([]*pldtypes.EthAddress{signingAddr}, nil)

	// Flush of domain context happens on the remote node (the notary)
	dcFlushed := mockDCFlushWithWaiter(remoteEngineMocks)

	err := privateTxManager.Start()
	assert.NoError(t, err)

	tx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID: testTransactionID,
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					To:     domainAddress,
					From:   alice.identityLocator,
				},
			},
		},
	}
	localNodeMocks.txManager.On("GetResolvedTransactionByID", mock.Anything, mock.Anything).Return(&tx.ResolvedTransaction, nil)

	err = privateTxManager.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return privateTxManager.HandleNewTx(ctx, dbTX, tx)
	})
	assert.NoError(t, err)

	status := pollForStatus(ctx, t, "delegated", privateTxManager, domainAddressString, testTransactionID.String(), 200*time.Second)
	assert.Equal(t, "delegated", status)

	status = pollForStatus(ctx, t, "dispatched", remoteEngine, domainAddressString, testTransactionID.String(), 200*time.Second)
	assert.Equal(t, "dispatched", status)

	<-dcFlushed

}

func TestPrivateTxManagerRemoteNotaryEndorserRetry(t *testing.T) {
	if testing.Short() {
		// test test takes a second for the timeout to fire
		// TODO investigate ways to mock the time.AfterFunc function to avoid this delay
		t.Skip("skipping test in short mode.")
	}
	ctx := context.Background()
	// A transaction that requires exactly one endorsement from a notary (as per noto) and therefore delegates coordination of the transaction to that node

	domainAddress := pldtypes.MustEthAddress(pldtypes.RandHex(20))
	localNodeName := "localNode"
	remoteNodeName := "remoteNode"
	privateTxManager, localNodeMocks := NewPrivateTransactionMgrForPackageTesting(t, localNodeName)
	localNodeMocks.mockDomain(domainAddress)

	domainAddressString := domainAddress.String()

	remoteEngine, remoteEngineMocks := NewPrivateTransactionMgrForPackageTesting(t, remoteNodeName)
	remoteEngineMocks.mockDomain(domainAddress)

	alice := newPartyForTesting(ctx, "alice", localNodeName, localNodeMocks)
	notary := newPartyForTesting(ctx, "notary", remoteNodeName, remoteEngineMocks)

	alice.mockResolve(ctx, notary)
	notary.mockResolve(ctx, alice)

	initialised := make(chan struct{}, 1)
	localNodeMocks.domainSmartContract.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
		StaticCoordinator:    &notary.identityLocator,
	})
	localNodeMocks.domainSmartContract.On("InitTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.PreAssembly = &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: alice.identityLocator,
			},
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{
					Lookup:       alice.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
				{
					Lookup:       notary.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
		}
		initialised <- struct{}{}
	}).Return(nil)

	assembled := make(chan struct{}, 1)

	localNodeMocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(2).(*components.PrivateTransaction)

		tx.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			InputStates: []*components.FullState{
				{
					ID:     pldtypes.RandBytes(32),
					Schema: pldtypes.RandBytes32(),
					Data:   pldtypes.JSONString("foo"),
				},
			},
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "notary",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						notary.identityLocator,
					},
				},
			},
		}
		assembled <- struct{}{}

	}).Return(nil)

	ignoredDelegateRequest := false

	localNodeMocks.transportManager.On("Send", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		go func() {
			assert.Equal(t, remoteNodeName, args.Get(1).(*components.FireAndForgetMessageSend).Node)
			send := args.Get(1).(*components.FireAndForgetMessageSend)
			if send.MessageType == "DelegationRequest" && !ignoredDelegateRequest {
				//ignore the first delegate request and force a retry
				ignoredDelegateRequest = true
			} else {
				remoteEngine.HandlePaladinMsg(ctx, inMsgToOut(localNodeName, send))
			}
		}()
	}).Return(nil).Maybe()

	remoteEngineMocks.transportManager.On("Send", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		go func() {
			send := args.Get(1).(*components.FireAndForgetMessageSend)
			privateTxManager.HandlePaladinMsg(ctx, inMsgToOut(remoteNodeName, send))
		}()
	}).Return(nil).Maybe()

	remoteEngineMocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, mock.Anything, *domainAddress).Return(remoteEngineMocks.domainSmartContract, nil)

	//TODO match endorsement request and verifier args
	remoteEngineMocks.domainSmartContract.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	localNodeMocks.domainSmartContract.On("Address").Return(*domainAddress)
	remoteEngineMocks.domainSmartContract.On("EndorseTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: []byte("some-endorsement-bytes"),
		Endorser: &prototk.ResolvedVerifier{
			Lookup:       notary.identityLocator, //matches whatever was specified in PreAssembly.RequiredVerifiers
			Verifier:     notary.verifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}, nil)

	notary.mockSign([]byte("some-endorsement-bytes"), []byte("some-signature-bytes"))

	remoteEngineMocks.domainSmartContract.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Return(nil).Run(
		func(args mock.Arguments) {
			cv, err := testABI[0].Inputs.ParseExternalData(map[string]any{
				"inputs":  []any{pldtypes.RandBytes32()},
				"outputs": []any{pldtypes.RandBytes32()},
				"data":    "0xfeedbeef",
			})
			require.NoError(t, err)
			tx := args[2].(*components.PrivateTransaction)
			tx.Signer = "signer1"
			jsonData, _ := cv.JSON()
			tx.PreparedPublicTransaction = &pldapi.TransactionInput{
				ABI: abi.ABI{testABI[0]},
				TransactionBase: pldapi.TransactionBase{
					To:              domainAddress,
					Data:            pldtypes.RawJSON(jsonData),
					PublicTxOptions: pldapi.PublicTxOptions{Gas: confutil.P(pldtypes.HexUint64(100000))},
				},
			}
		},
	)
	testTransactionID := confutil.P(uuid.New())

	signingAddr := pldtypes.RandAddress()
	remoteEngineMocks.keyManager.On("ResolveEthAddressBatchNewDatabaseTX", mock.Anything, []string{"signer1"}).
		Return([]*pldtypes.EthAddress{signingAddr}, nil)

	_ = mockWritePublicTxsOk(remoteEngineMocks)

	// Flush of domain context happens on the remote node (the notary)
	dcFlushed := mockDCFlushWithWaiter(remoteEngineMocks)

	err := privateTxManager.Start()
	assert.NoError(t, err)

	tx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID: testTransactionID,
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					To:     domainAddress,
					From:   alice.identityLocator,
				},
			},
		},
	}
	localNodeMocks.txManager.On("GetResolvedTransactionByID", mock.Anything, mock.Anything).Return(&tx.ResolvedTransaction, nil)

	err = privateTxManager.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return privateTxManager.HandleNewTx(ctx, dbTX, tx)
	})
	assert.NoError(t, err)

	status := pollForStatus(ctx, t, "delegated", privateTxManager, domainAddressString, testTransactionID.String(), 200*time.Second)
	assert.Equal(t, "delegated", status)

	status = pollForStatus(ctx, t, "dispatched", remoteEngine, domainAddressString, testTransactionID.String(), 200*time.Second)
	assert.Equal(t, "dispatched", status)

	<-dcFlushed

}

func TestPrivateTxManagerEndorsementGroup(t *testing.T) {

	ctx := context.Background()
	// A transaction that requires endorsement from a group of remote endorsers (as per pente and its 100% endorsement policy)
	// In this scenario there is only one active transaction and therefore no risk of contention so the transactions is coordinated
	// and dispatched locally.  The only expected interaction with the remote nodes is to request endorsements and to distribute the new states

	domainAddress := pldtypes.MustEthAddress(pldtypes.RandHex(20))
	domainAddressString := domainAddress.String()

	aliceNodeName := "aliceNode"
	bobNodeName := "bobNode"
	carolNodeName := "carolNode"

	aliceEngine, aliceEngineMocks := NewPrivateTransactionMgrForPackageTesting(t, aliceNodeName)
	aliceEngineMocks.mockDomain(domainAddress)

	bobEngine, bobEngineMocks := NewPrivateTransactionMgrForPackageTesting(t, bobNodeName)
	bobEngineMocks.mockDomain(domainAddress)

	carolEngine, carolEngineMocks := NewPrivateTransactionMgrForPackageTesting(t, carolNodeName)
	carolEngineMocks.mockDomain(domainAddress)

	alice := newPartyForTesting(ctx, "alice", aliceNodeName, aliceEngineMocks)
	bob := newPartyForTesting(ctx, "bob", bobNodeName, bobEngineMocks)
	carol := newPartyForTesting(ctx, "carol", carolNodeName, carolEngineMocks)

	alice.mockResolve(ctx, bob)
	alice.mockResolve(ctx, carol)

	testTransactionID := confutil.P(uuid.New())

	aliceEngineMocks.domainSmartContract.On("InitTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.PreAssembly = &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				TransactionId: testTransactionID.String(),
				From:          alice.identityLocator,
			},
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{
					Lookup:       alice.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
				{
					Lookup:       bob.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
				{
					Lookup:       carol.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
		}
	}).Return(nil)

	aliceEngineMocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(2).(*components.PrivateTransaction)

		tx.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			InputStates: []*components.FullState{
				{
					ID:     pldtypes.RandBytes(32),
					Schema: pldtypes.RandBytes32(),
					Data:   pldtypes.JSONString("foo"),
				},
			},
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "endorsers",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						alice.identityLocator,
						bob.identityLocator,
						carol.identityLocator,
					},
				},
			},
		}

	}).Return(nil)

	//Set up mocks that allow nodes to exchange messages with each other
	mockNetwork(t, []privateTransactionMgrForPackageTesting{
		aliceEngine,
		bobEngine,
		carolEngine,
	})

	//set up the mocks on bob and carols engines that are need on the endorse code path (and of course also on alice's engine because she is an endorser too)

	bobEngineMocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, mock.Anything, *domainAddress).Return(bobEngineMocks.domainSmartContract, nil)
	carolEngineMocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, mock.Anything, *domainAddress).Return(carolEngineMocks.domainSmartContract, nil)

	//TODO match endorsement request and verifier args
	aliceEngineMocks.domainSmartContract.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	aliceEngineMocks.domainSmartContract.On("Address").Return(*domainAddress)

	aliceEngineMocks.mockForEndorsement(t, *testTransactionID, &alice, []byte("alice-endorsement-bytes"), []byte("alice-signature-bytes"))
	bobEngineMocks.mockForEndorsement(t, *testTransactionID, &bob, []byte("bob-endorsement-bytes"), []byte("bob-signature-bytes"))
	carolEngineMocks.mockForEndorsement(t, *testTransactionID, &carol, []byte("carol-endorsement-bytes"), []byte("carol-signature-bytes"))

	//Set up mocks on alice's engine that are needed for alice to be the submitter of the transaction
	dcFlushed := aliceEngineMocks.mockForSubmitter(t, testTransactionID, domainAddress,
		map[string][]byte{ //expected endorsement signatures
			alice.verifier: []byte("alice-signature-bytes"),
			bob.verifier:   []byte("bob-signature-bytes"),
			carol.verifier: []byte("carol-signature-bytes"),
		},
	)

	err := aliceEngine.Start()
	assert.NoError(t, err)

	tx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID: testTransactionID,
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					To:     domainAddress,
					From:   alice.identityLocator,
				},
			},
		},
	}
	aliceEngineMocks.txManager.On("GetResolvedTransactionByID", mock.Anything, mock.Anything).Return(&tx.ResolvedTransaction, nil)

	err = aliceEngine.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return aliceEngine.HandleNewTx(ctx, dbTX, tx)
	})
	assert.NoError(t, err)

	status := pollForStatus(ctx, t, "dispatched", aliceEngine, domainAddressString, testTransactionID.String(), 200*time.Second)
	assert.Equal(t, "dispatched", status)

	<-dcFlushed
}

func TestPrivateTxManagerEndorsementGroupDynamicCoordinator(t *testing.T) {

	// Extension to TestPrivateTxManagerEndorsementGroup with the addition of emulating the progression of block height
	// beyond the range boundaries so that nodes switch coordinator roles
	ctx := context.Background()

	domainAddress := pldtypes.MustEthAddress(pldtypes.RandHex(20))
	domainAddressString := domainAddress.String()

	testTransactionID1 := confutil.P(uuid.New())
	testTransactionID2 := confutil.P(uuid.New())

	aliceNodeName := "aliceNode"
	bobNodeName := "bobNode"
	carolNodeName := "carolNode"

	aliceEngine, aliceEngineMocks := NewPrivateTransactionMgrForPackageTesting(t, aliceNodeName)
	aliceEngineMocks.mockDomain(domainAddress)

	bobEngine, bobEngineMocks := NewPrivateTransactionMgrForPackageTesting(t, bobNodeName)
	bobEngineMocks.mockDomain(domainAddress)

	carolEngine, carolEngineMocks := NewPrivateTransactionMgrForPackageTesting(t, carolNodeName)
	carolEngineMocks.mockDomain(domainAddress)

	alice := newPartyForTesting(ctx, "alice", aliceNodeName, aliceEngineMocks)
	bob := newPartyForTesting(ctx, "bob", bobNodeName, bobEngineMocks)
	carol := newPartyForTesting(ctx, "carol", carolNodeName, carolEngineMocks)

	alice.mockResolve(ctx, bob)
	alice.mockResolve(ctx, carol)

	bob.mockResolve(ctx, alice)
	bob.mockResolve(ctx, carol)

	carol.mockResolve(ctx, bob)
	carol.mockResolve(ctx, alice)

	//Set up mocks on alice's transaction manager that are needed for it to be the sender (aka assembler) of transaction 1

	aliceEngineMocks.domainSmartContract.On("InitTransaction", mock.Anything, mock.MatchedBy(privateTransactionMatcher(*testTransactionID1, *testTransactionID2)), mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.PreAssembly = &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				TransactionId: tx.ID.String(),
				From:          alice.identityLocator,
			},
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{
					Lookup:       alice.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
				{
					Lookup:       bob.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
				{
					Lookup:       carol.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
		}
	}).Return(nil)

	aliceEngineMocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(2).(*components.PrivateTransaction)

		tx.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			InputStates: []*components.FullState{
				{
					ID:     pldtypes.RandBytes(32),
					Schema: pldtypes.RandBytes32(),
					Data:   pldtypes.JSONString("foo"),
				},
			},
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "endorsers",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						alice.identityLocator,
						bob.identityLocator,
						carol.identityLocator,
					},
				},
			},
		}

	}).Return(nil)

	//Set up mocks that allow nodes to exchange messages with each other
	mockNetwork(t, []privateTransactionMgrForPackageTesting{
		aliceEngine,
		bobEngine,
		carolEngine,
	})

	//Set up mocks on alice's engine that are needed for alice to be the coordinator of the first transaction
	aliceEngineMocks.domainSmartContract.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	aliceEngineMocks.domainSmartContract.On("Address").Return(*domainAddress)

	//set up the mocks on all 3 engines that are need on the endorse code path
	bobEngineMocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, mock.Anything, *domainAddress).Return(bobEngineMocks.domainSmartContract, nil)
	carolEngineMocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, mock.Anything, *domainAddress).Return(carolEngineMocks.domainSmartContract, nil)

	aliceEngineMocks.mockForEndorsement(t, *testTransactionID1, &alice, []byte("alice-endorsement-bytes1"), []byte("alice-signature-bytes1"))
	bobEngineMocks.mockForEndorsement(t, *testTransactionID1, &bob, []byte("bob-endorsement-bytes1"), []byte("bob-signature-bytes1"))
	carolEngineMocks.mockForEndorsement(t, *testTransactionID1, &carol, []byte("carol-endorsement-bytes1"), []byte("carol-signature-bytes1"))

	//Set up mocks on alice's engine that are needed for alice to be the submitter of the first transaction
	_ = aliceEngineMocks.mockForSubmitter(t, testTransactionID1, domainAddress,
		map[string][]byte{ //expected endorsement signatures
			alice.verifier: []byte("alice-signature-bytes1"),
			bob.verifier:   []byte("bob-signature-bytes1"),
			carol.verifier: []byte("carol-signature-bytes1"),
		},
	)

	err := aliceEngine.Start()
	assert.NoError(t, err)

	tx1 := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID: testTransactionID1,
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					To:     domainAddress,
					From:   alice.identityLocator,
				},
			},
		},
	}
	aliceEngineMocks.txManager.On("GetResolvedTransactionByID", mock.Anything, *testTransactionID1).Return(&tx1.ResolvedTransaction, nil)

	tx2 := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID: testTransactionID2,
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					To:     domainAddress,
					From:   alice.identityLocator,
				},
			},
		},
	}
	aliceEngineMocks.txManager.On("GetResolvedTransactionByID", mock.Anything, *testTransactionID2).Return(&tx2.ResolvedTransaction, nil)

	//Start off on block 99 where alice should be coordinator

	aliceEngine.SetBlockHeight(ctx, 99)
	bobEngine.SetBlockHeight(ctx, 99)
	carolEngine.SetBlockHeight(ctx, 99)

	err = aliceEngine.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return aliceEngine.HandleNewTx(ctx, dbTX, tx1)
	})
	assert.NoError(t, err)

	status := pollForStatus(ctx, t, "dispatched", aliceEngine, domainAddressString, testTransactionID1.String(), 200*time.Second)
	assert.Equal(t, "dispatched", status)

	// Setup mocks on bob to be coordinator
	bobEngineMocks.domainSmartContract.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})

	aliceEngineMocks.mockForEndorsement(t, *testTransactionID2, &alice, []byte("alice-endorsement-bytes2"), []byte("alice-signature-bytes2"))
	bobEngineMocks.mockForEndorsement(t, *testTransactionID2, &bob, []byte("bob-endorsement-bytes2"), []byte("bob-signature-bytes2"))
	carolEngineMocks.mockForEndorsement(t, *testTransactionID2, &carol, []byte("carol-endorsement-bytes2"), []byte("carol-signature-bytes2"))

	//Set up mocks on bob's engine that are needed for bob to be the submitter of the second transaction
	dcFlushed := bobEngineMocks.mockForSubmitter(t, testTransactionID2, domainAddress,
		map[string][]byte{ //expected endorsement signatures
			alice.verifier: []byte("alice-signature-bytes2"),
			bob.verifier:   []byte("bob-signature-bytes2"),
			carol.verifier: []byte("carol-signature-bytes2"),
		},
	)

	aliceEngine.SetBlockHeight(ctx, 100)
	bobEngine.SetBlockHeight(ctx, 100)
	carolEngine.SetBlockHeight(ctx, 100)
	err = aliceEngine.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return aliceEngine.HandleNewTx(ctx, dbTX, tx2)
	})
	assert.NoError(t, err)

	status = pollForStatus(ctx, t, "delegated", aliceEngine, domainAddressString, testTransactionID2.String(), 200*time.Second)
	assert.Equal(t, "delegated", status)

	status = pollForStatus(ctx, t, "dispatched", bobEngine, domainAddressString, testTransactionID2.String(), 200*time.Second)
	assert.Equal(t, "dispatched", status)

	<-dcFlushed
}

func TestPrivateTxManagerEndorsementGroupDynamicCoordinatorRangeBoundaryHandover(t *testing.T) {

	// Extension to TestPrivateTxManagerEndorsementGroupDynamicCoordinatorRangeBoundary where we simulate the case where
	// there are still some transactions in flight when the coordinator role switches
	// and assert that transactions that are not yet passed the point of no return (i.e. are sequenced but not dispatched ) are transferred to, and eventually submitted by, the new coordinator
	ctx := context.Background()

	domainAddress := pldtypes.MustEthAddress(pldtypes.RandHex(20))
	domainAddressString := domainAddress.String()

	testTransactionID1 := confutil.P(uuid.New())
	testTransactionID2 := confutil.P(uuid.New())

	aliceNodeName := "aliceNode"
	bobNodeName := "bobNode"
	carolNodeName := "carolNode"

	aliceEngine, aliceEngineMocks := NewPrivateTransactionMgrForPackageTesting(t, aliceNodeName)
	aliceEngineMocks.mockDomain(domainAddress)

	bobEngine, bobEngineMocks := NewPrivateTransactionMgrForPackageTesting(t, bobNodeName)
	bobEngineMocks.mockDomain(domainAddress)

	carolEngine, carolEngineMocks := NewPrivateTransactionMgrForPackageTesting(t, carolNodeName)
	carolEngineMocks.mockDomain(domainAddress)

	alice := newPartyForTesting(ctx, "alice", aliceNodeName, aliceEngineMocks)
	bob := newPartyForTesting(ctx, "bob", bobNodeName, bobEngineMocks)
	carol := newPartyForTesting(ctx, "carol", carolNodeName, carolEngineMocks)

	alice.mockResolve(ctx, bob)
	alice.mockResolve(ctx, carol)

	bob.mockResolve(ctx, alice)
	bob.mockResolve(ctx, carol)

	carol.mockResolve(ctx, bob)
	carol.mockResolve(ctx, alice)

	//Set up mocks on alice's transaction manager that are needed for it to be the sender (aka assembler) of transaction 1
	aliceEngineMocks.domainSmartContract.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})

	aliceEngineMocks.domainSmartContract.On("InitTransaction", mock.Anything, mock.MatchedBy(privateTransactionMatcher(*testTransactionID1, *testTransactionID2)), mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.PreAssembly = &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				TransactionId: tx.ID.String(),
				From:          alice.identityLocator,
			},
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{
					Lookup:       alice.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
				{
					Lookup:       bob.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
				{
					Lookup:       carol.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
		}
	}).Return(nil)

	aliceEngineMocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(2).(*components.PrivateTransaction)

		tx.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			InputStates: []*components.FullState{
				{
					ID:     pldtypes.RandBytes(32),
					Schema: pldtypes.RandBytes32(),
					Data:   pldtypes.JSONString("foo"),
				},
			},
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "endorsers",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						alice.identityLocator,
						bob.identityLocator,
						carol.identityLocator,
					},
				},
			},
		}

	}).Return(nil)

	//Set up mocks that allow nodes to exchange messages with each other
	mockNetwork(t, []privateTransactionMgrForPackageTesting{
		aliceEngine,
		bobEngine,
		carolEngine,
	})

	//Set up mocks on bob's engine that are needed for alice to be the coordinator of the first transaction
	bobEngineMocks.domainSmartContract.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})
	aliceEngineMocks.domainSmartContract.On("Address").Return(*domainAddress)

	//set up the mocks on all 3 engines that are need on the endorse code path
	bobEngineMocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, mock.Anything, *domainAddress).Return(bobEngineMocks.domainSmartContract, nil)
	carolEngineMocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, mock.Anything, *domainAddress).Return(carolEngineMocks.domainSmartContract, nil)

	aliceEngineMocks.mockForEndorsement(t, *testTransactionID1, &alice, []byte("alice-endorsement-bytes1"), []byte("alice-signature-bytes1"))
	bobEngineMocks.mockForEndorsement(t, *testTransactionID1, &bob, []byte("bob-endorsement-bytes1"), []byte("bob-signature-bytes1"))
	carolEngineMocks.mockForEndorsement(t, *testTransactionID1, &carol, []byte("carol-endorsement-bytes1"), []byte("carol-signature-bytes1"))

	//Set up mocks on bobs's engine that are needed to be the submitter of the first transaction
	_ = bobEngineMocks.mockForSubmitter(t, testTransactionID1, domainAddress,
		map[string][]byte{ //expected endorsement signatures
			alice.verifier: []byte("alice-signature-bytes1"),
			bob.verifier:   []byte("bob-signature-bytes1"),
			carol.verifier: []byte("carol-signature-bytes1"),
		},
	)

	err := aliceEngine.Start()
	assert.NoError(t, err)

	tx1 := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID: testTransactionID1,
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					To:     domainAddress,
					From:   alice.identityLocator,
				},
			},
		},
	}
	aliceEngineMocks.txManager.On("GetResolvedTransactionByID", mock.Anything, *testTransactionID1).Return(&tx1.ResolvedTransaction, nil)

	tx2 := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID: testTransactionID2,
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					To:     domainAddress,
					From:   alice.identityLocator,
				},
			},
		},
	}
	aliceEngineMocks.txManager.On("GetResolvedTransactionByID", mock.Anything, *testTransactionID2).Return(&tx2.ResolvedTransaction, nil)

	aliceEngine.SetBlockHeight(ctx, 199)
	bobEngine.SetBlockHeight(ctx, 199)
	carolEngine.SetBlockHeight(ctx, 199)

	err = aliceEngine.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return aliceEngine.HandleNewTx(ctx, dbTX, tx1)
	})
	assert.NoError(t, err)

	//wait until alice had delegated to bob and bob has dispatched
	status := pollForStatus(ctx, t, "delegated", aliceEngine, domainAddressString, testTransactionID1.String(), 200*time.Second)
	assert.Equal(t, "delegated", status)

	status = pollForStatus(ctx, t, "dispatched", bobEngine, domainAddressString, testTransactionID1.String(), 200*time.Second)
	assert.Equal(t, "dispatched", status)

	// tx1 is now past the point of no return

	// set up mocks for second transaction to be partially endorsed
	aliceEngineMocks.mockForEndorsement(t, *testTransactionID2, &alice, []byte("alice-endorsement-bytes2"), []byte("alice-signature-bytes2"))
	bobEngineMocks.mockForEndorsement(t, *testTransactionID2, &bob, []byte("bob-endorsement-bytes2"), []byte("bob-signature-bytes2"))

	//prepare carol to endorse transaction 2 but not until after the block height has been incremented
	carolEndorsementTrigger := carolEngineMocks.mockForEndorsementOnTrigger(t, *testTransactionID2, &carol, []byte("carol-endorsement-bytes2"), []byte("carol-signature-bytes2"))

	//send transaction 2 and it should get delegated to bob, because we have not moved the block height but it should not get dispatched yet because carol's endorsement is delayed
	err = aliceEngine.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return aliceEngine.HandleNewTx(ctx, dbTX, tx2)
	})
	assert.NoError(t, err)

	status = pollForStatus(ctx, t, "delegated", aliceEngine, domainAddressString, testTransactionID2.String(), 200*time.Second)
	assert.Equal(t, "delegated", status)

	//wait until bob has sequenced the transaction and is waiting for endorsement
	status = pollForStatus(ctx, t, "signed", bobEngine, domainAddressString, testTransactionID2.String(), 200*time.Second)
	assert.Equal(t, "signed", status)

	// Setup mocks on carol to be coordinator
	carolEngineMocks.domainSmartContract.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})

	//Set up mocks on carol's engine that are needed to be the submitter of the second transaction
	dcFlushed := carolEngineMocks.mockForSubmitter(t, testTransactionID2, domainAddress,
		map[string][]byte{ //expected endorsement signatures
			alice.verifier: []byte("alice-signature-bytes2"),
			bob.verifier:   []byte("bob-signature-bytes2"),
			carol.verifier: []byte("carol-signature-bytes2"),
		},
	)

	aliceEngine.SetBlockHeight(ctx, 200)
	bobEngine.SetBlockHeight(ctx, 200)
	carolEngine.SetBlockHeight(ctx, 200)

	carolEndorsementTrigger()

	status = pollForStatus(ctx, t, "delegated", aliceEngine, domainAddressString, testTransactionID2.String(), 200*time.Second)
	assert.Equal(t, "delegated", status)

	status = pollForStatus(ctx, t, "delegated", bobEngine, domainAddressString, testTransactionID2.String(), 200*time.Second)
	assert.Equal(t, "delegated", status)

	status = pollForStatus(ctx, t, "dispatched", carolEngine, domainAddressString, testTransactionID2.String(), 200*time.Second)
	assert.Equal(t, "dispatched", status)

	<-dcFlushed
}

func TestPrivateTxManagerDependantTransactionEndorsedOutOfOrder(t *testing.T) {
	// extension to the TestPrivateTxManagerEndorsementGroup test
	// 2 transactions, one dependant on the other
	// we purposely endorse the first transaction late to ensure that the 2nd transaction
	// is still sequenced behind the first

	ctx := context.Background()

	domainAddress := pldtypes.MustEthAddress(pldtypes.RandHex(20))
	domainAddressString := domainAddress.String()

	aliceNodeName := "aliceNode"
	bobNodeName := "bobNode"
	aliceEngine, aliceEngineMocks := NewPrivateTransactionMgrForPackageTesting(t, aliceNodeName)
	aliceEngineMocks.mockDomain(domainAddress)

	log.SetLevel("debug")

	_, bobEngineMocks := NewPrivateTransactionMgrForPackageTesting(t, bobNodeName)
	bobEngineMocks.mockDomain(domainAddress)

	alice := newPartyForTesting(ctx, "alice", aliceNodeName, aliceEngineMocks)
	bob := newPartyForTesting(ctx, "bob", bobNodeName, bobEngineMocks)

	alice.mockResolve(ctx, bob)

	aliceEngineMocks.domainSmartContract.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})

	aliceEngineMocks.domainSmartContract.On("InitTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.PreAssembly = &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				TransactionId: tx.ID.String(),
				From:          alice.identityLocator,
			},
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{
					Lookup:       alice.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
				{
					Lookup:       bob.identityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
		}
	}).Return(nil)
	aliceEngineMocks.domainSmartContract.On("Address").Return(*domainAddress)

	// TODO check that the transaction is signed with this key

	states := []*components.FullState{
		{
			ID:     pldtypes.RandBytes(32),
			Schema: pldtypes.RandBytes32(),
			Data:   pldtypes.JSONString("foo"),
		},
	}

	potentialStates := []*prototk.NewState{
		{
			SchemaId:      states[0].Schema.String(),
			StateDataJson: states[0].Data.String(),
		},
	}
	testTransactionID1 := confutil.P(uuid.New())
	testTransactionID2 := confutil.P(uuid.New())

	aliceEngineMocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(2).(*components.PrivateTransaction)
		switch tx.ID.String() {
		case testTransactionID1.String():
			tx.PostAssembly = &components.TransactionPostAssembly{
				AssemblyResult:        prototk.AssembleTransactionResponse_OK,
				OutputStates:          states,
				OutputStatesPotential: potentialStates,
				AttestationPlan: []*prototk.AttestationRequest{
					{
						Name:            "notary",
						AttestationType: prototk.AttestationType_ENDORSE,
						Algorithm:       algorithms.ECDSA_SECP256K1,
						VerifierType:    verifiers.ETH_ADDRESS,
						PayloadType:     signpayloads.OPAQUE_TO_RSV,
						Parties: []string{
							alice.identityLocator,
							bob.identityLocator,
						},
					},
				},
			}
		case testTransactionID2.String():
			tx.PostAssembly = &components.TransactionPostAssembly{
				AssemblyResult: prototk.AssembleTransactionResponse_OK,
				InputStates:    states,
				AttestationPlan: []*prototk.AttestationRequest{
					{
						Name:            "notary",
						AttestationType: prototk.AttestationType_ENDORSE,
						Algorithm:       algorithms.ECDSA_SECP256K1,
						VerifierType:    verifiers.ETH_ADDRESS,
						PayloadType:     signpayloads.OPAQUE_TO_RSV,
						Parties: []string{
							alice.identityLocator,
							bob.identityLocator,
						},
					},
				},
			}
		default:
			assert.Fail(t, "Unexpected transaction ID")
		}
	}).Times(2).Return(nil)

	sentEndorsementRequest := make(chan string, 2)
	aliceEngineMocks.transportManager.On("Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		send := args.Get(1).(*components.FireAndForgetMessageSend)
		endorsementRequest := &pbEngine.EndorsementRequest{}
		err := proto.Unmarshal(send.Payload, endorsementRequest)
		if err != nil {
			log.L(ctx).Errorf("Failed to unmarshal endorsement request: %s", err)
			return
		}
		log.L(ctx).Debugf("Sending endorsement request for %s", endorsementRequest.IdempotencyKey)
		sentEndorsementRequest <- endorsementRequest.IdempotencyKey
	}).Times(2).Return(nil)

	aliceEngineMocks.domainSmartContract.On("EndorseTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: []byte("alice-endorsement-bytes"),
		Endorser: &prototk.ResolvedVerifier{
			Lookup:       alice.identityLocator,
			Verifier:     alice.verifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}, nil)

	alice.mockSign([]byte("alice-endorsement-bytes"), []byte("alice-signature-bytes"))

	aliceEngineMocks.domainSmartContract.On("PrepareTransaction", mock.Anything, mock.Anything, mock.Anything).Return(nil).Run(
		func(args mock.Arguments) {
			cv, err := testABI[0].Inputs.ParseExternalData(map[string]any{
				"inputs":  []any{pldtypes.RandBytes32()},
				"outputs": []any{pldtypes.RandBytes32()},
				"data":    "0xfeedbeef",
			})
			require.NoError(t, err)
			tx := args[2].(*components.PrivateTransaction)
			tx.Signer = "signer1"
			jsonData, _ := cv.JSON()
			tx.PreparedPublicTransaction = &pldapi.TransactionInput{
				ABI: abi.ABI{testABI[0]},
				TransactionBase: pldapi.TransactionBase{
					To:              domainAddress,
					Data:            pldtypes.RawJSON(jsonData),
					PublicTxOptions: pldapi.PublicTxOptions{Gas: confutil.P(pldtypes.HexUint64(100000))},
				},
			}
		},
	)

	_ = mockWritePublicTxsOk(aliceEngineMocks)

	signingAddr := pldtypes.RandAddress()
	aliceEngineMocks.keyManager.On("ResolveEthAddressBatchNewDatabaseTX", mock.Anything, []string{"signer1", "signer1"}).
		Return([]*pldtypes.EthAddress{signingAddr, signingAddr}, nil)

	err := aliceEngine.Start()
	require.NoError(t, err)
	defer aliceEngine.Stop()

	tx1 := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID: testTransactionID1,
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					To:     domainAddress,
					From:   alice.identityLocator,
				},
			},
		},
	}
	aliceEngineMocks.txManager.On("GetResolvedTransactionByID", mock.Anything, *testTransactionID1).Return(&tx1.ResolvedTransaction, nil)

	err = aliceEngine.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return aliceEngine.HandleNewTx(ctx, dbTX, tx1)
	})
	require.NoError(t, err)

	tx2 := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID: testTransactionID2,
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					To:     domainAddress,
					From:   alice.identityLocator,
				},
			},
		},
	}
	aliceEngineMocks.txManager.On("GetResolvedTransactionByID", mock.Anything, *testTransactionID2).Return(&tx2.ResolvedTransaction, nil)

	err = aliceEngine.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return aliceEngine.HandleNewTx(ctx, dbTX, tx2)
	})
	require.NoError(t, err)

	// Neither transaction should be dispatched yet
	s, err := aliceEngine.GetTxStatus(ctx, domainAddressString, *testTransactionID1)
	require.NoError(t, err)
	assert.NotEqual(t, "dispatch", s.Status)

	s, err = aliceEngine.GetTxStatus(ctx, domainAddressString, *testTransactionID2)
	require.NoError(t, err)
	assert.NotEqual(t, "dispatch", s.Status)

	attestationResult := prototk.AttestationResult{
		Name:            "notary",
		AttestationType: prototk.AttestationType_ENDORSE,
		Payload:         pldtypes.RandBytes(32),
		Verifier: &prototk.ResolvedVerifier{
			Lookup:       bob.identityLocator,
			Verifier:     bob.verifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}

	attestationResultAny, err := anypb.New(&attestationResult)
	require.NoError(t, err)

	// Wait for both transactions to send an endorsement request each with timeout
	var idempotencyKey1, idempotencyKey2 string
	select {
	case idempotencyKey1 = <-sentEndorsementRequest:
		// Proceed with key
	case <-time.After(30 * time.Second):
		t.Fatal("Timed out waiting for first endorsement request")
	}
	select {
	case idempotencyKey2 = <-sentEndorsementRequest:
		// Proceed with key
	case <-time.After(30 * time.Second):
		t.Fatal("Timed out waiting for second endorsement request")
	}

	// endorse transaction 2 before 1 and check that 2 is not dispatched before 1
	endorsementResponse2 := &pbEngine.EndorsementResponse{
		ContractAddress:        domainAddressString,
		TransactionId:          testTransactionID2.String(),
		Endorsement:            attestationResultAny,
		IdempotencyKey:         idempotencyKey2,
		Party:                  bob.identityLocator,
		AttestationRequestName: "notary",
	}
	endorsementResponse2bytes, err := proto.Marshal(endorsementResponse2)
	require.NoError(t, err)

	//now send the endorsements back
	aliceEngine.HandlePaladinMsg(ctx, &components.ReceivedMessage{
		FromNode:    bobNodeName,
		MessageType: "EndorsementResponse",
		Payload:     endorsementResponse2bytes,
	})

	//unless the tests are running in short mode, wait a second to ensure that the transaction is not dispatched
	if !testing.Short() {
		time.Sleep(1 * time.Second)
	}
	s, err = aliceEngine.GetTxStatus(ctx, domainAddressString, *testTransactionID1)
	require.NoError(t, err)
	assert.NotEqual(t, "dispatch", s.Status)

	s, err = aliceEngine.GetTxStatus(ctx, domainAddressString, *testTransactionID2)
	require.NoError(t, err)
	assert.NotEqual(t, "dispatch", s.Status)

	// endorse transaction 1 and check that both it and 2 are dispatched
	endorsementResponse1 := &pbEngine.EndorsementResponse{
		ContractAddress:        domainAddressString,
		TransactionId:          testTransactionID1.String(),
		Endorsement:            attestationResultAny,
		IdempotencyKey:         idempotencyKey1,
		Party:                  bob.identityLocator,
		AttestationRequestName: "notary",
	}
	endorsementResponse1Bytes, err := proto.Marshal(endorsementResponse1)
	require.NoError(t, err)

	//now send the final endorsement back
	aliceEngine.HandlePaladinMsg(ctx, &components.ReceivedMessage{
		FromNode:    bobNodeName,
		MessageType: "EndorsementResponse",
		Payload:     endorsementResponse1Bytes,
	})

	// at this point we should get a flush of the states
	dcFlushed := mockDCFlushWithWaiter(aliceEngineMocks)

	status := pollForStatus(ctx, t, "dispatched", aliceEngine, domainAddressString, testTransactionID1.String(), 30*time.Second)
	assert.Equal(t, "dispatched", status)

	status = pollForStatus(ctx, t, "dispatched", aliceEngine, domainAddressString, testTransactionID2.String(), 30*time.Second)
	assert.Equal(t, "dispatched", status)

	<-dcFlushed

	//TODO assert that transaction 1 got dispatched before 2

}

func TestPrivateTxManagerLocalBlockedTransaction(t *testing.T) {
	//TODO
	// 3 transactions, for different signing addresses, but two are is blocked by the other
	// when the earlier transaction is confirmed, both blocked transactions should be dispatched
}

func TestPrivateTxManagerDeploy(t *testing.T) {
	ctx := context.Background()

	privateTxManager, mocks := NewPrivateTransactionMgrForPackageTesting(t, "node1")
	notary := newPartyForTesting(ctx, "notary", "node1", mocks)

	testTransactionID := confutil.P(uuid.New())

	mocks.domain.On("InitDeploy", mock.Anything, mock.MatchedBy(privateDeployTransactionMatcher(*testTransactionID))).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateContractDeploy)
		tx.RequiredVerifiers = []*prototk.ResolveVerifierRequest{
			{
				Lookup:       notary.identityLocator,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		}
	}).Return(nil)

	domainRegistryAddress := pldtypes.RandAddress()

	testConstructorABI := &abi.Entry{
		Name:   "constructor",
		Inputs: abi.ParameterArray{{Name: "foo", Type: "int32"}},
	}
	testConstructorParameters, err := testConstructorABI.Inputs.ParseJSON([]byte(`{"foo": "42"}`))
	require.NoError(t, err)

	mocks.domain.On("PrepareDeploy", mock.Anything, mock.MatchedBy(privateDeployTransactionMatcher(*testTransactionID))).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateContractDeploy)
		tx.InvokeTransaction = &components.EthTransaction{
			FunctionABI: testConstructorABI,
			To:          *domainRegistryAddress,
			Inputs:      testConstructorParameters,
		}
		tx.Signer = "signer1"
	}).Return(nil)

	signingAddr := pldtypes.RandAddress()
	mocks.keyManager.On("ResolveEthAddressBatchNewDatabaseTX", mock.Anything, []string{"signer1"}).
		Return([]*pldtypes.EthAddress{signingAddr}, nil)

	dispatched := mockWritePublicTxsOk(mocks)

	mocks.txManager.On("FinalizeTransactions", mock.Anything, mock.Anything, mock.Anything).Return(nil).Panic("did not expect transaction to be reverted").Maybe()

	err = privateTxManager.Start()
	require.NoError(t, err)

	deployTx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID:         testTransactionID,
				SubmitMode: pldapi.SubmitModeAuto.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					From:   "alice",
				},
			},
		},
	}
	err = privateTxManager.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return privateTxManager.HandleNewTx(ctx, dbTX, deployTx)
	})
	require.NoError(t, err)

	deadlineTimer := time.NewTimer(timeTillDeadline(t))
	select {
	case <-dispatched:
	case <-deadlineTimer.C:
		assert.Fail(t, "timed out")

	}
}

func TestPrivateTxManagerDeployErrorInvalidSubmitMode(t *testing.T) {
	ctx := context.Background()

	privateTxManager, mocks := NewPrivateTransactionMgrForPackageTesting(t, "node1")
	notary := newPartyForTesting(ctx, "notary", "node1", mocks)

	testTransactionID := confutil.P(uuid.New())

	err := privateTxManager.Start()
	require.NoError(t, err)

	deployTx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID:         testTransactionID,
				SubmitMode: pldapi.SubmitModeExternal.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					From:   notary.identityLocator,
					Data:   pldtypes.JSONString(`{"inputs": ["0xfeedbeef"]}`),
				},
			},
		},
	}
	err = privateTxManager.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return privateTxManager.HandleNewTx(ctx, dbTX, deployTx)
	})
	assert.Error(t, err)
	assert.Regexp(t, "PD011827", err.Error())

}

func TestPrivateTxManagerDeployFailInit(t *testing.T) {
	// Init errors should fail synchronously
	ctx := context.Background()

	privateTxManager, mocks := NewPrivateTransactionMgrForPackageTesting(t, "node1")

	testTransactionID := uuid.New()

	mocks.domain.On("InitDeploy", mock.Anything, mock.MatchedBy(privateDeployTransactionMatcher(testTransactionID))).Return(errors.New("failed to init"))

	err := privateTxManager.Start()
	require.NoError(t, err)

	tx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID:         &testTransactionID,
				SubmitMode: pldapi.SubmitModeAuto.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					From:   "alice@node1",
					Data:   pldtypes.JSONString(`{"inputs": ["0xfeedbeef"]}`),
				},
			},
		},
	}

	err = privateTxManager.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return privateTxManager.HandleNewTx(ctx, dbTX, tx)
	})
	assert.Error(t, err)
	assert.Regexp(t, regexp.MustCompile(".*failed to init.*"), err.Error())
}

func TestPrivateTxManagerDeployFailPrepare(t *testing.T) {
	ctx := context.Background()

	privateTxManager, mocks := NewPrivateTransactionMgrForPackageTesting(t, "node1")
	notary := newPartyForTesting(ctx, "notary", "node1", mocks)

	testTransactionID := uuid.New()
	vtx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID:         &testTransactionID,
				SubmitMode: pldapi.SubmitModeAuto.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					From:   "alice@node1",
					Data:   pldtypes.JSONString(`{"inputs": ["0xfeedbeef"]}`),
				},
			},
		},
	}
	mocks.domain.On("InitDeploy", mock.Anything, mock.MatchedBy(privateDeployTransactionMatcher(testTransactionID))).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateContractDeploy)
		tx.RequiredVerifiers = []*prototk.ResolveVerifierRequest{
			{
				Lookup:       notary.identityLocator,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		}
	}).Return(nil)

	mocks.domain.On("PrepareDeploy", mock.Anything, mock.MatchedBy(privateDeployTransactionMatcher(testTransactionID))).Return(errors.New("failed to prepare"))

	reverted := make(chan []*components.ReceiptInput, 1)

	mocks.txManager.On("FinalizeTransactions", mock.Anything, mock.Anything, mock.Anything).
		Run(
			func(args mock.Arguments) {
				reverted <- args.Get(2).([]*components.ReceiptInput)
			},
		).
		Return(nil)

	err := privateTxManager.Start()
	require.NoError(t, err)

	err = privateTxManager.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return privateTxManager.HandleNewTx(ctx, dbTX, vtx)
	})
	require.NoError(t, err)

	deadlineTimer := time.NewTimer(timeTillDeadline(t))
	select {
	case receipts := <-reverted:
		assert.Len(t, receipts, 1)
		assert.Equal(t, testTransactionID, receipts[0].TransactionID)
		assert.Regexp(t, regexp.MustCompile(".*failed to prepare.*"), receipts[0].FailureMessage)

	case <-deadlineTimer.C:
		assert.Fail(t, "timed out")

	}
}

func TestPrivateTxManagerFailSignerResolve(t *testing.T) {
	ctx := context.Background()

	privateTxManager, mocks := NewPrivateTransactionMgrForPackageTesting(t, "node1")
	notary := newPartyForTesting(ctx, "notary", "node1", mocks)

	testTransactionID := uuid.New()
	vtx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID:         &testTransactionID,
				SubmitMode: pldapi.SubmitModeAuto.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					From:   "alice@node1",
					Data:   pldtypes.JSONString(`{"inputs": ["0xfeedbeef"]}`),
				},
			},
		},
	}

	mocks.domain.On("InitDeploy", mock.Anything, mock.MatchedBy(privateDeployTransactionMatcher(testTransactionID))).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateContractDeploy)
		tx.RequiredVerifiers = []*prototk.ResolveVerifierRequest{
			{
				Lookup:       notary.identityLocator,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		}
	}).Return(nil)

	domainRegistryAddress := pldtypes.RandAddress()

	testConstructorABI := &abi.Entry{
		Name:   "constructor",
		Inputs: abi.ParameterArray{{Name: "foo", Type: "int32"}},
	}
	testConstructorParameters, err := testConstructorABI.Inputs.ParseJSON([]byte(`{"foo": "42"}`))
	require.NoError(t, err)

	mocks.domain.On("PrepareDeploy", mock.Anything, mock.MatchedBy(privateDeployTransactionMatcher(testTransactionID))).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateContractDeploy)
		tx.InvokeTransaction = &components.EthTransaction{
			FunctionABI: testConstructorABI,
			To:          *domainRegistryAddress,
			Inputs:      testConstructorParameters,
		}
		tx.Signer = "signer1"
	}).Return(nil)

	mocks.keyManager.On("ResolveEthAddressBatchNewDatabaseTX", mock.Anything, []string{"signer1"}).
		Return(nil, errors.New("failed to resolve"))

	reverted := make(chan []*components.ReceiptInput, 1)

	mocks.txManager.On("FinalizeTransactions", mock.Anything, mock.Anything, mock.Anything).
		Run(
			func(args mock.Arguments) {
				reverted <- args.Get(2).([]*components.ReceiptInput)
			},
		).
		Return(nil)

	err = privateTxManager.Start()
	require.NoError(t, err)

	err = privateTxManager.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return privateTxManager.HandleNewTx(ctx, dbTX, vtx)
	})
	require.NoError(t, err)

	deadlineTimer := time.NewTimer(timeTillDeadline(t))
	select {
	case receipts := <-reverted:
		assert.Len(t, receipts, 1)
		assert.Equal(t, testTransactionID, receipts[0].TransactionID)
		assert.Regexp(t, regexp.MustCompile(".*failed to resolve.*"), receipts[0].FailureMessage)

	case <-deadlineTimer.C:
		assert.Fail(t, "timed out")
	}
}

func TestPrivateTxManagerDeployFailNoInvokeOrDeploy(t *testing.T) {
	ctx := context.Background()

	privateTxManager, mocks := NewPrivateTransactionMgrForPackageTesting(t, "node1")
	notary := newPartyForTesting(ctx, "notary", "node1", mocks)

	testTransactionID := uuid.New()

	vtx := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Function: &components.ResolvedFunction{
				Definition: testABI[0],
			},
			Transaction: &pldapi.Transaction{
				ID:         &testTransactionID,
				SubmitMode: pldapi.SubmitModeAuto.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					From:   "alice@node1",
					Data:   pldtypes.JSONString(`{"inputs": ["0xfeedbeef"]}`),
				},
			},
		},
	}

	mocks.domain.On("InitDeploy", mock.Anything, mock.MatchedBy(privateDeployTransactionMatcher(testTransactionID))).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateContractDeploy)
		tx.RequiredVerifiers = []*prototk.ResolveVerifierRequest{
			{
				Lookup:       notary.identityLocator,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		}
	}).Return(nil)

	mocks.domain.On("PrepareDeploy", mock.Anything, mock.MatchedBy(privateDeployTransactionMatcher(testTransactionID))).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateContractDeploy)
		tx.InvokeTransaction = nil
		tx.DeployTransaction = nil
		tx.Signer = "signer1"
	}).Return(nil)

	signingAddr := pldtypes.RandAddress()
	mocks.keyManager.On("ResolveEthAddressBatchNewDatabaseTX", mock.Anything, []string{"signer1"}).
		Return([]*pldtypes.EthAddress{signingAddr}, nil)

	reverted := make(chan []*components.ReceiptInput, 1)

	mocks.txManager.On("FinalizeTransactions", mock.Anything, mock.Anything, mock.Anything).
		Run(
			func(args mock.Arguments) {
				reverted <- args.Get(2).([]*components.ReceiptInput)
			},
		).
		Return(nil)

	err := privateTxManager.Start()
	require.NoError(t, err)

	err = privateTxManager.P().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return privateTxManager.HandleNewTx(ctx, dbTX, vtx)
	})
	require.NoError(t, err)

	deadlineTimer := time.NewTimer(timeTillDeadline(t))
	select {
	case receipts := <-reverted:
		assert.Len(t, receipts, 1)
		assert.Equal(t, testTransactionID, receipts[0].TransactionID)
		assert.Regexp(t, regexp.MustCompile("PD011801"), receipts[0].FailureMessage)
		assert.Regexp(t, regexp.MustCompile("PD011820"), receipts[0].FailureMessage)

	case <-deadlineTimer.C:
		assert.Fail(t, "timed out")
	}
}

func TestCallPrivateSmartContractOk(t *testing.T) {

	ctx := context.Background()
	ptx, m := NewPrivateTransactionMgrForPackageTesting(t, "node1")

	_, mPSC := mockDomainSmartContractAndCtx(t, m)

	fnDef := &abi.Entry{Name: "getIt", Type: abi.Function, Outputs: abi.ParameterArray{
		{Name: "it", Type: "string"},
	}}
	resultCV, err := fnDef.Outputs.ParseJSON([]byte(`["thing"]`))
	require.NoError(t, err)

	bobAddr := pldtypes.RandAddress()
	m.identityResolver.On("ResolveVerifier", mock.Anything, "bob@node1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).
		Return(bobAddr.String(), nil)
	mPSC.On("InitCall", mock.Anything, mock.Anything).Return(
		[]*prototk.ResolveVerifierRequest{
			{Lookup: "bob@node1", Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS},
		}, nil,
	)
	mPSC.On("ExecCall", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(verifiers []*prototk.ResolvedVerifier) bool {
		require.Equal(t, bobAddr.String(), verifiers[0].Verifier)
		return true
	})).Return(
		resultCV, nil,
	)

	res, err := ptx.CallPrivateSmartContract(ctx, &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				To:   confutil.P(mPSC.Address()),
				Data: pldtypes.RawJSON(`{}`),
			},
		},
		Function: &components.ResolvedFunction{
			Definition: fnDef,
		},
	})
	require.NoError(t, err)
	jsonData, err := res.JSON()
	require.NoError(t, err)
	require.JSONEq(t, `{"it": "thing"}`, string(jsonData))

}

func TestCallPrivateSmartContractBadContract(t *testing.T) {

	ctx := context.Background()
	ptx, m := NewPrivateTransactionMgrForPackageTesting(t, "node1")

	m.domainMgr.On("GetSmartContractByAddress", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("not found"))

	_, err := ptx.CallPrivateSmartContract(ctx, &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				To:   pldtypes.RandAddress(),
				Data: pldtypes.RawJSON(`{}`),
			},
		},
	})
	assert.Regexp(t, "not found", err)

}
func TestCallPrivateSmartContractBadDomainName(t *testing.T) {

	ctx := context.Background()
	ptx, m := NewPrivateTransactionMgrForPackageTesting(t, "node1")

	_, mPSC := mockDomainSmartContractAndCtx(t, m)

	fnDef := &abi.Entry{Name: "getIt", Type: abi.Function, Outputs: abi.ParameterArray{
		{Name: "it", Type: "string"},
	}}

	_, err := ptx.CallPrivateSmartContract(ctx, &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				Domain: "does-not-match",
				To:     confutil.P(mPSC.Address()),
				Data:   pldtypes.RawJSON(`{}`),
			},
		},
		Function: &components.ResolvedFunction{
			Definition: fnDef,
		},
	})
	assert.Regexp(t, "PD011825", err)

}

func TestCallPrivateSmartContractInitCallFail(t *testing.T) {

	ctx := context.Background()
	ptx, m := NewPrivateTransactionMgrForPackageTesting(t, "node1")

	_, mPSC := mockDomainSmartContractAndCtx(t, m)

	mPSC.On("InitCall", mock.Anything, mock.Anything).Return(
		nil, fmt.Errorf("pop"),
	)

	_, err := ptx.CallPrivateSmartContract(ctx, &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				To:   confutil.P(mPSC.Address()),
				Data: pldtypes.RawJSON(`{}`),
			},
		},
	})
	require.Regexp(t, "pop", err)

}

func TestCallPrivateSmartContractResolveFail(t *testing.T) {

	ctx := context.Background()
	ptx, m := NewPrivateTransactionMgrForPackageTesting(t, "node1")

	_, mPSC := mockDomainSmartContractAndCtx(t, m)

	mPSC.On("InitCall", mock.Anything, mock.Anything).Return(
		[]*prototk.ResolveVerifierRequest{
			{Lookup: "bob@node1", Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS},
		}, nil,
	)
	m.identityResolver.On("ResolveVerifier", mock.Anything, "bob@node1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).
		Return("", fmt.Errorf("pop"))

	_, err := ptx.CallPrivateSmartContract(ctx, &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				To:   confutil.P(mPSC.Address()),
				Data: pldtypes.RawJSON(`{}`),
			},
		},
	})
	require.Regexp(t, "pop", err)

}

func TestCallPrivateSmartContractExecCallFail(t *testing.T) {

	ctx := context.Background()
	ptx, m := NewPrivateTransactionMgrForPackageTesting(t, "node1")

	_, mPSC := mockDomainSmartContractAndCtx(t, m)

	mPSC.On("InitCall", mock.Anything, mock.Anything).Return(
		[]*prototk.ResolveVerifierRequest{}, nil,
	)
	mPSC.On("ExecCall", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		nil, fmt.Errorf("pop"),
	)

	_, err := ptx.CallPrivateSmartContract(ctx, &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				To:   confutil.P(mPSC.Address()),
				Data: pldtypes.RawJSON(`{}`),
			},
		},
	})
	require.Regexp(t, "pop", err)

}

/* Unit tests */

/* Utils */

type dependencyMocks struct {
	preInitComponents   *componentsmocks.PreInitComponents
	allComponents       *componentsmocks.AllComponents
	db                  *mockpersistence.SQLMockProvider
	persistence         persistence.Persistence
	domain              *componentsmocks.Domain
	domainSmartContract *componentsmocks.DomainSmartContract
	domainContext       *componentsmocks.DomainContext
	domainMgr           *componentsmocks.DomainManager
	transportManager    *componentsmocks.TransportManager
	stateStore          *componentsmocks.StateManager
	keyManager          *componentsmocks.KeyManager
	keyResolver         *componentsmocks.KeyResolver
	publicTxManager     *componentsmocks.PublicTxManager
	identityResolver    *componentsmocks.IdentityResolver
	txManager           *componentsmocks.TXManager
}

func (m *dependencyMocks) mockDomain(domainAddress *pldtypes.EthAddress) {
	m.stateStore.On("NewDomainContext", mock.Anything, m.domain, *domainAddress, mock.Anything).Return(m.domainContext).Maybe()
	m.domainMgr.On("GetSmartContractByAddress", mock.Anything, mock.Anything, *domainAddress).Maybe().Return(m.domainSmartContract, nil)
	m.domain.On("Configuration").Return(&prototk.DomainConfig{}).Maybe()
}

// Some of the tests were getting quite verbose and was difficult to see the wood for the trees so moved a lot of the boilerplate into these utility functions
//   - some of these utility functions have potential to become complex so may need to think about tests for the utility functions themselves but for now, the complexity is low enough to not warrant it

func (m *dependencyMocks) mockForSubmitter(t *testing.T, transactionID *uuid.UUID, domainAddress *pldtypes.EthAddress, expectedEndorsements map[string][]byte /*map of verifier to endorsement signature*/) chan struct{} {
	signerName := "signer1"
	signingAddr := pldtypes.RandAddress()
	m.domainSmartContract.On("PrepareTransaction", mock.Anything, mock.Anything, mock.MatchedBy(privateTransactionMatcher(*transactionID))).Return(nil).Run(
		func(args mock.Arguments) {
			cv, err := testABI[0].Inputs.ParseExternalData(map[string]any{
				"inputs":  []any{pldtypes.RandBytes32()},
				"outputs": []any{pldtypes.RandBytes32()},
				"data":    "0xfeedbeef",
			})
			require.NoError(t, err)
			tx := args[2].(*components.PrivateTransaction)
			tx.Signer = signerName
			jsonData, _ := cv.JSON()
			tx.PreparedPublicTransaction = &pldapi.TransactionInput{
				ABI: abi.ABI{testABI[0]},
				TransactionBase: pldapi.TransactionBase{
					To:              domainAddress,
					Data:            pldtypes.RawJSON(jsonData),
					PublicTxOptions: pldapi.PublicTxOptions{Gas: confutil.P(pldtypes.HexUint64(100000))},
				},
			}
			endorsed := make(map[string]bool)
			for _, endorsement := range tx.PostAssembly.Endorsements {
				if expectedEndorsement, ok := expectedEndorsements[endorsement.Verifier.Verifier]; ok {
					assert.Equal(t, expectedEndorsement, endorsement.Payload)
					endorsed[endorsement.Verifier.Verifier] = true

				} else {
					assert.Failf(t, "unexpected endorsement from %s ", endorsement.Verifier.Verifier)
				}
			}
			assert.Len(t, endorsed, len(expectedEndorsements))
		},
	)

	m.keyManager.On("ResolveEthAddressBatchNewDatabaseTX", mock.Anything, []string{"signer1"}).
		Return([]*pldtypes.EthAddress{signingAddr}, nil)

	_ = mockWritePublicTxsOk(m)

	return mockDCFlushWithWaiter(m)

}

func mockNetwork(t *testing.T, transactionManagers []privateTransactionMgrForPackageTesting) {

	routeToNode := func(fromNode string) func(args mock.Arguments) {
		return func(args mock.Arguments) {
			go func() {
				send := args.Get(1).(*components.FireAndForgetMessageSend)
				for _, tm := range transactionManagers {
					if tm.NodeName() == send.Node {
						tm.HandlePaladinMsg(context.Background(), inMsgToOut(fromNode, send))
						return
					}
				}
				assert.Failf(t, "no transaction manager found for node %s", send.Node)
			}()
		}
	}
	for _, tm := range transactionManagers {
		tm.DependencyMocks().transportManager.On("Send", mock.Anything, mock.Anything).Run(routeToNode(tm.NodeName())).Return(nil).Maybe()
	}

}

func (m *dependencyMocks) mockForEndorsement(_ *testing.T, txID uuid.UUID, endorser *identityForTesting, endorsementPayload []byte, endorsementSignature []byte) {
	endorsementRequestMatcher := func(req *components.PrivateTransactionEndorseRequest) bool {
		return req.TransactionSpecification.TransactionId == txID.String()
	}
	m.domainSmartContract.On("EndorseTransaction", mock.Anything, mock.Anything, mock.MatchedBy(endorsementRequestMatcher)).Return(&components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: endorsementPayload,
		Endorser: &prototk.ResolvedVerifier{
			Lookup:       endorser.identityLocator,
			Verifier:     endorser.verifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}, nil)

	endorser.mockSign(endorsementPayload, endorsementSignature)
}

func (m *dependencyMocks) mockForEndorsementOnTrigger(_ *testing.T, txID uuid.UUID, endorser *identityForTesting, endorsementPayload []byte, endorsementSignature []byte) func() {
	triggered := false
	endorsementRequestMatcher := func(req *components.PrivateTransactionEndorseRequest) bool {
		return req.TransactionSpecification.TransactionId == txID.String()
	}
	trigger := make(chan struct{})
	m.domainSmartContract.On("EndorseTransaction", mock.Anything, mock.Anything, mock.MatchedBy(endorsementRequestMatcher)).Run(func(_ mock.Arguments) {
		if !triggered {
			<-trigger
		}
	}).Return(&components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: endorsementPayload,
		Endorser: &prototk.ResolvedVerifier{
			Lookup:       endorser.identityLocator,
			Verifier:     endorser.verifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}, nil)

	endorser.mockSign(endorsementPayload, endorsementSignature)

	return func() {
		//remember that the trigger has been called so that any future calls to the mock will not block
		triggered = true
		close(trigger)
	}
}

type privateTransactionMgrForPackageTesting interface {
	components.PrivateTxManager
	PreCommitHandler(ctx context.Context, dbTX persistence.DBTX, blocks []*pldapi.IndexedBlock, transactions []*blockindexer.IndexedTransactionNotify) error
	DependencyMocks() *dependencyMocks
	NodeName() string
	//Wrapper around a call to PreCommitHandler to notify of a new block with given height
	SetBlockHeight(ctx context.Context, height int64)
	P() persistence.Persistence
}
type privateTransactionMgrForPackageTestingStruct struct {
	*privateTxManager
	preCommitHandler blockindexer.PreCommitHandler
	dependencyMocks  *dependencyMocks
	nodeName         string
	t                *testing.T
}

// WriteOrDistributeReceipts implements privateTransactionMgrForPackageTesting.
func (p *privateTransactionMgrForPackageTestingStruct) WriteOrDistributeReceipts(ctx context.Context, dbTX persistence.DBTX, receipts []*components.ReceiptInputWithOriginator) error {
	panic("unimplemented")
}

func (p *privateTransactionMgrForPackageTestingStruct) PreCommitHandler(ctx context.Context, dbTX persistence.DBTX, blocks []*pldapi.IndexedBlock, transactions []*blockindexer.IndexedTransactionNotify) error {
	return p.preCommitHandler(ctx, dbTX, blocks, transactions)
}

func (p *privateTransactionMgrForPackageTestingStruct) DependencyMocks() *dependencyMocks {
	return p.dependencyMocks
}

func (p *privateTransactionMgrForPackageTestingStruct) NodeName() string {
	return p.nodeName
}

func (p *privateTransactionMgrForPackageTestingStruct) SetBlockHeight(ctx context.Context, height int64) {
	err := p.dependencyMocks.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return p.PreCommitHandler(
			ctx,
			dbTX,
			[]*pldapi.IndexedBlock{
				{
					Number: height,
				},
			},
			nil,
		)
	})
	assert.NoError(p.t, err)
}

func (p *privateTransactionMgrForPackageTestingStruct) P() persistence.Persistence {
	return p.privateTxManager.components.Persistence()
}

func NewPrivateTransactionMgrForPackageTesting(t *testing.T, nodeName string) (privateTransactionMgrForPackageTesting, *dependencyMocks) {

	defaultCoordinatorSelectionMode := EndorsementCoordinatorSelectionMode
	EndorsementCoordinatorSelectionMode = BlockHeightRoundRobin // unit tests all coded to this mode (work to do as production mode for leader election becomes established)
	t.Cleanup(func() {
		EndorsementCoordinatorSelectionMode = defaultCoordinatorSelectionMode
	})

	ctx := context.Background()

	p, persistenceCleanup, err := persistence.NewUnitTestPersistence(ctx, "privatetxmgr")
	require.NoError(t, err)
	t.Cleanup(persistenceCleanup)

	mocks := &dependencyMocks{
		preInitComponents:   componentsmocks.NewPreInitComponents(t),
		allComponents:       componentsmocks.NewAllComponents(t),
		domain:              componentsmocks.NewDomain(t),
		domainSmartContract: componentsmocks.NewDomainSmartContract(t),
		domainContext:       componentsmocks.NewDomainContext(t),
		domainMgr:           componentsmocks.NewDomainManager(t),
		transportManager:    componentsmocks.NewTransportManager(t),
		stateStore:          componentsmocks.NewStateManager(t),
		keyManager:          componentsmocks.NewKeyManager(t),
		keyResolver:         componentsmocks.NewKeyResolver(t),
		identityResolver:    componentsmocks.NewIdentityResolver(t),
		txManager:           componentsmocks.NewTXManager(t),
		publicTxManager:     componentsmocks.NewPublicTxManager(t),
		persistence:         p,
	}
	mocks.allComponents.On("StateManager").Return(mocks.stateStore).Maybe()
	mocks.allComponents.On("DomainManager").Return(mocks.domainMgr).Maybe()
	mocks.allComponents.On("TransportManager").Return(mocks.transportManager).Maybe()
	mocks.transportManager.On("LocalNodeName").Return(nodeName)
	mocks.allComponents.On("KeyManager").Return(mocks.keyManager).Maybe()
	mocks.allComponents.On("TxManager").Return(mocks.txManager).Maybe()
	mocks.allComponents.On("PublicTxManager").Return(mocks.publicTxManager).Maybe()
	mocks.allComponents.On("Persistence").Return(mocks.persistence).Maybe()
	mocks.domainSmartContract.On("Domain").Return(mocks.domain).Maybe()
	mocks.domainSmartContract.On("LockStates", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mocks.domainMgr.On("GetDomainByName", mock.Anything, "domain1").Return(mocks.domain, nil).Maybe()
	mocks.domain.On("Name").Return("domain1").Maybe()
	mocks.keyManager.On("KeyResolverForDBTXLazyDB", mock.Anything).Return(mocks.keyResolver).Maybe()

	mocks.domainContext.On("Ctx").Return(ctx).Maybe()
	mocks.domainContext.On("Info").Return(components.DomainContextInfo{ID: uuid.New()}).Maybe()
	mocks.domainContext.On("ExportSnapshot").Return([]byte("[]"), nil).Maybe()
	mocks.domainContext.On("ImportSnapshot", mock.Anything).Return(nil).Maybe()

	e := NewPrivateTransactionMgr(ctx, &pldconf.PrivateTxManagerConfig{
		Writer: pldconf.FlushWriterConfig{
			WorkerCount:  confutil.P(1),
			BatchMaxSize: confutil.P(1), // we don't want batching for our test
		},
		Sequencer: pldconf.PrivateTxManagerSequencerConfig{
			// StaleTimeout: ,
		},
	})

	//It is not valid to call other managers before PostInit
	mocks.transportManager.On("RegisterClient", mock.Anything, mock.Anything).Return(nil).Maybe()
	//It is not valid to reference LateBound components before PostInit
	mocks.allComponents.On("IdentityResolver").Return(mocks.identityResolver).Maybe()
	preInitResult, err := e.PreInit(mocks.preInitComponents)
	assert.NoError(t, err)
	err = mocks.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return preInitResult.PreCommitHandler(
			ctx,
			dbTX,
			[]*pldapi.IndexedBlock{
				{
					Number: 42,
				},
			},
			nil,
		)
	})
	assert.NoError(t, err)

	err = e.PostInit(mocks.allComponents)
	assert.NoError(t, err)

	return &privateTransactionMgrForPackageTestingStruct{
		privateTxManager: e.(*privateTxManager),
		preCommitHandler: preInitResult.PreCommitHandler,
		dependencyMocks:  mocks,
		nodeName:         nodeName,
		t:                t,
	}, mocks

}

type identityForTesting struct {
	identity        string
	identityLocator string
	verifier        string
	keyHandle       string
	mocks           *dependencyMocks
	mockSign        func(payload []byte, signature []byte)
}

func (i *identityForTesting) mockResolve(ctx context.Context, other identityForTesting) {
	// in addition to the default mocks set up in newPartyForTesting, we can set up mocks to resolve remote identities
	// we could have used a real IdentityResolver here but we are testing the private transaction manager in isolation and so we mock the IdentityResolver as we do with all other tests in this file
	i.mocks.identityResolver.On(
		"ResolveVerifierAsync",
		mock.Anything,
		other.identityLocator,
		algorithms.ECDSA_SECP256K1,
		verifiers.ETH_ADDRESS,
		mock.Anything,
		mock.Anything).
		Run(func(args mock.Arguments) {
			resolveFn := args.Get(4).(func(context.Context, string))
			resolveFn(ctx, other.verifier)
		}).Return(nil).Maybe()
}

var testABI = abi.ABI{
	{
		Name: "execute",
		Type: abi.Function,
		Inputs: abi.ParameterArray{
			{
				Name: "inputs",
				Type: "bytes32[]",
			},
			{
				Name: "outputs",
				Type: "bytes32[]",
			},
			{
				Name: "data",
				Type: "bytes",
			},
		},
	},
}

func privateDeployTransactionMatcher(txID uuid.UUID) func(*components.PrivateContractDeploy) bool {
	return func(tx *components.PrivateContractDeploy) bool {
		return tx.ID == txID
	}
}

func privateTransactionMatcher(txID ...uuid.UUID) func(*components.PrivateTransaction) bool {
	return func(tx *components.PrivateTransaction) bool {
		for _, id := range txID {
			if tx.ID == id {
				return true
			}
		}
		return false
	}
}

func newPartyForTesting(ctx context.Context, name, node string, mocks *dependencyMocks) identityForTesting {
	party := identityForTesting{
		identity:        name,
		identityLocator: name + "@" + node,
		verifier:        pldtypes.RandAddress().String(),
		keyHandle:       name + "KeyHandle",
		mocks:           mocks,
	}

	mocks.identityResolver.On("ResolveVerifierAsync", mock.Anything, party.identity, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		resolveFn := args.Get(4).(func(context.Context, string))
		resolveFn(ctx, party.verifier)
	}).Return(nil).Maybe()

	mocks.identityResolver.On("ResolveVerifierAsync", mock.Anything, party.identityLocator, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		resolveFn := args.Get(4).(func(context.Context, string))
		resolveFn(ctx, party.verifier)
	}).Return(nil).Maybe()

	mocks.identityResolver.On("ResolveVerifier", mock.Anything, party.identityLocator, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).
		Return(party.verifier, nil).Maybe()

	keyMapping := &pldapi.KeyMappingAndVerifier{
		KeyMappingWithPath: &pldapi.KeyMappingWithPath{KeyMapping: &pldapi.KeyMapping{
			Identifier: party.identity,
			KeyHandle:  party.keyHandle,
		}},
		Verifier: &pldapi.KeyVerifier{Verifier: party.verifier},
	}
	mocks.keyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, party.identity, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).Return(keyMapping, nil).Maybe()

	party.mockSign = func(payload []byte, signature []byte) {
		mocks.keyManager.On("Sign", mock.Anything, keyMapping, signpayloads.OPAQUE_TO_RSV, payload).
			Return(signature, nil)
	}

	return party
}

func timeTillDeadline(t *testing.T) time.Duration {
	deadline, ok := t.Deadline()
	if !ok {
		//there was no -timeout flag, default to 10 seconds
		deadline = time.Now().Add(10 * time.Second)
	}
	timeRemaining := time.Until(deadline)
	//Need to leave some time to ensure that polling assertions fail before the test itself timesout
	//otherwise we don't see diagnostic info for things like GoExit called by mocks etc
	if timeRemaining < 100*time.Millisecond {
		return 0
	}
	return timeRemaining - 100*time.Millisecond
}

func mockDomainSmartContractAndCtx(t *testing.T, m *dependencyMocks) (*componentsmocks.Domain, *componentsmocks.DomainSmartContract) {
	contractAddr := *pldtypes.RandAddress()

	mDomain := componentsmocks.NewDomain(t)
	mDomain.On("Name").Return("domain1").Maybe()

	mPSC := componentsmocks.NewDomainSmartContract(t)
	mPSC.On("Address").Return(contractAddr).Maybe()
	mPSC.On("Domain").Return(mDomain).Maybe()

	m.domainMgr.On("GetSmartContractByAddress", mock.Anything, mock.Anything, contractAddr).Return(mPSC, nil)

	mDC := componentsmocks.NewDomainContext(t)
	m.stateStore.On("NewDomainContext", mock.Anything, mDomain, contractAddr).Return(mDC).Maybe()
	mDC.On("Close").Return().Maybe()

	return mDomain, mPSC
}

func pollForStatus(ctx context.Context, t *testing.T, expectedStatus string, privateTxManager components.PrivateTxManager, domainAddressString, txID string, duration time.Duration) string {
	timeout := time.After(duration)
	tick := time.Tick(100 * time.Millisecond)

	for {
		if t.Failed() {
			panic("test failed")
		}
		select {
		case <-timeout:
			// Timeout reached, exit the loop
			assert.Failf(t, "Timed out waiting for status %s", expectedStatus)
			s, err := privateTxManager.GetTxStatus(ctx, domainAddressString, uuid.MustParse(txID))
			require.NoError(t, err)
			return s.Status
		case <-tick:
			s, err := privateTxManager.GetTxStatus(ctx, domainAddressString, uuid.MustParse(txID))
			if s.Status == expectedStatus {
				return s.Status
			}
			require.NoError(t, err)
		}
	}
}
