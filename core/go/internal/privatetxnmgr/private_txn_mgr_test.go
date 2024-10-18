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
	"math/rand"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/signerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/signpayloads"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"gorm.io/gorm"

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	pbEngine "github.com/kaleido-io/paladin/core/pkg/proto/engine"
)

// Attempt to assert the behaviour of the private transaction manager as a whole component in isolation from the rest of the system
// Tests in this file do not mock anything else in this package or sub packages but does mock other components and managers in paladin as per their interfaces

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

func TestPrivateTxManagerInit(t *testing.T) {

	privateTxManager, mocks, _ := NewPrivateTransactionMgrForTesting(t, tktypes.MustEthAddress(tktypes.RandHex(20)))
	err := privateTxManager.PostInit(mocks.allComponents)
	require.NoError(t, err)
}

func TestPrivateTxManagerInvalidTransaction(t *testing.T) {
	ctx := context.Background()

	privateTxManager, mocks, _ := NewPrivateTransactionMgrForTesting(t, tktypes.MustEthAddress(tktypes.RandHex(20)))
	err := privateTxManager.PostInit(mocks.allComponents)
	require.NoError(t, err)

	err = privateTxManager.Start()
	require.NoError(t, err)

	err = privateTxManager.HandleNewTx(ctx, &components.PrivateTransaction{})
	// no input domain should err
	assert.Regexp(t, "PD011800", err)
}

func TestPrivateTxManagerSimpleTransaction(t *testing.T) {
	//Submit a transaction that gets assembled with an attestation plan for a local endorser to sign the transaction
	ctx := context.Background()

	domainAddress := tktypes.MustEthAddress(tktypes.RandHex(20))
	privateTxManager, mocks, _ := NewPrivateTransactionMgrForTesting(t, domainAddress)
	domainAddressString := domainAddress.String()

	// unqualified lookup string because everything is local
	aliceIdentity := "alice"
	aliceVerifier := tktypes.RandAddress().String()
	notaryIdentity := "domain1.contract1.notary"
	notaryVerifier := tktypes.RandAddress().String()
	notaryKeyHandle := "notaryKeyHandle"

	initialised := make(chan struct{}, 1)
	mocks.domainSmartContract.On("InitTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.PreAssembly = &components.TransactionPreAssembly{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{
					Lookup:       aliceIdentity, // unqualified lookup string because everything is local
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
				{
					Lookup:       notaryIdentity,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
		}
		initialised <- struct{}{}
	}).Return(nil)

	mocks.identityResolver.On("ResolveVerifierAsync", mock.Anything, aliceIdentity, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		resovleFn := args.Get(4).(func(context.Context, string))
		resovleFn(ctx, aliceVerifier)
	}).Return(nil)
	mocks.identityResolver.On("ResolveVerifierAsync", mock.Anything, notaryIdentity, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		resovleFn := args.Get(4).(func(context.Context, string))
		resovleFn(ctx, notaryVerifier)
	}).Return(nil)
	// TODO check that the transaction is signed with this key

	assembled := make(chan struct{}, 1)
	mocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)

		tx.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			InputStates: []*components.FullState{
				{
					ID:     tktypes.RandBytes(32),
					Schema: tktypes.Bytes32(tktypes.RandBytes(32)),
					Data:   tktypes.JSONString("foo"),
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
						notaryIdentity,
					},
				},
			},
		}
		assembled <- struct{}{}

	}).Return(nil)

	notaryKeyMapping := &pldapi.KeyMappingAndVerifier{
		KeyMappingWithPath: &pldapi.KeyMappingWithPath{KeyMapping: &pldapi.KeyMapping{
			Identifier: notaryIdentity,
			KeyHandle:  notaryKeyHandle,
		}},
		Verifier: &pldapi.KeyVerifier{Verifier: notaryVerifier},
	}
	mocks.keyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, notaryIdentity, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).Return(notaryKeyMapping, nil)

	signingAddress := tktypes.RandHex(32)

	mocks.domainSmartContract.On("ResolveDispatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.Signer = signingAddress
	}).Return(nil)

	//TODO match endorsement request and verifier args
	mocks.domainSmartContract.On("EndorseTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: []byte("some-endorsement-bytes"),
		Endorser: &prototk.ResolvedVerifier{
			Lookup:       notaryIdentity,
			Verifier:     notaryVerifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}, nil)

	mocks.keyManager.On("Sign", mock.Anything, notaryKeyMapping, signpayloads.OPAQUE_TO_RSV, mock.Anything).
		Return([]byte("notary-signature-bytes"), nil)

	mocks.domainSmartContract.On("PrepareTransaction", mock.Anything, mock.Anything).Return(nil).Run(
		func(args mock.Arguments) {
			cv, err := testABI[0].Inputs.ParseExternalData(map[string]any{
				"inputs":  []any{tktypes.Bytes32(tktypes.RandBytes(32))},
				"outputs": []any{tktypes.Bytes32(tktypes.RandBytes(32))},
				"data":    "0xfeedbeef",
			})
			require.NoError(t, err)
			tx := args[1].(*components.PrivateTransaction)
			tx.Signer = "signer1"
			jsonData, _ := cv.JSON()
			tx.PreparedPublicTransaction = &pldapi.TransactionInput{
				ABI: abi.ABI{testABI[0]},
				Transaction: pldapi.Transaction{
					To:   domainAddress,
					Data: tktypes.RawJSON(jsonData),
				},
			}
		},
	)

	tx := &components.PrivateTransaction{
		ID: uuid.New(),
		Inputs: &components.TransactionInputs{
			Domain: "domain1",
			To:     *domainAddress,
		},
	}

	mockPublicTxBatch := componentmocks.NewPublicTxBatch(t)
	mockPublicTxBatch.On("Finalize", mock.Anything).Return().Maybe()
	mockPublicTxBatch.On("CleanUp", mock.Anything).Return().Maybe()

	mockPublicTxManager := mocks.publicTxManager.(*componentmocks.PublicTxManager)
	mockPublicTxManager.On("PrepareSubmissionBatch", mock.Anything, mock.Anything).Return(mockPublicTxBatch, nil)

	signingAddr := tktypes.RandAddress()
	mocks.keyManager.On("ResolveEthAddressBatchNewDatabaseTX", mock.Anything, []string{"signer1"}).
		Return([]*tktypes.EthAddress{signingAddr}, nil)

	publicTransactions := []components.PublicTxAccepted{
		newFakePublicTx(&components.PublicTxSubmission{
			Bindings: []*components.PaladinTXReference{{TransactionID: tx.ID, TransactionType: pldapi.TransactionTypePrivate.Enum()}},
			PublicTxInput: pldapi.PublicTxInput{
				From: signingAddr,
			},
		}, nil),
	}
	mockPublicTxBatch.On("Submit", mock.Anything, mock.Anything).Return(nil)
	mockPublicTxBatch.On("Rejected").Return([]components.PublicTxRejected{})
	mockPublicTxBatch.On("Accepted").Return(publicTransactions)
	mockPublicTxBatch.On("Completed", mock.Anything, true).Return()

	err := privateTxManager.Start()
	require.NoError(t, err)
	err = privateTxManager.HandleNewTx(ctx, tx)
	require.NoError(t, err)

	// testTimeout := 2 * time.Second
	testTimeout := 100 * time.Minute
	status := pollForStatus(ctx, t, "dispatched", privateTxManager, domainAddressString, tx.ID.String(), testTimeout)
	assert.Equal(t, "dispatched", status)
}

func TestPrivateTxManagerLocalEndorserSubmits(t *testing.T) {
}

func TestPrivateTxManagerRevertFromLocalEndorsement(t *testing.T) {
}

type identityForTesting struct {
	identity        string
	identityLocator string
	verifier        string
	keyHandle       string
	mocks           *dependencyMocks
	mockSign        func(signature []byte)
}

func (i *identityForTesting) mockResolve(ctx context.Context, other identityForTesting) {
	// in addition to the default mocks set up in newPartyForTesting, we can set up mocks to resolve remote identitys
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

func newPartyForTesting(ctx context.Context, name, node string, mocks *dependencyMocks) identityForTesting {
	party := identityForTesting{
		identity:        name,
		identityLocator: name + "@" + node,
		verifier:        tktypes.RandAddress().String(),
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

	keyMapping := &pldapi.KeyMappingAndVerifier{
		KeyMappingWithPath: &pldapi.KeyMappingWithPath{KeyMapping: &pldapi.KeyMapping{
			Identifier: party.identity,
			KeyHandle:  party.keyHandle,
		}},
		Verifier: &pldapi.KeyVerifier{Verifier: party.verifier},
	}
	mocks.keyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, party.identity, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).Return(keyMapping, nil).Maybe()

	party.mockSign = func(signature []byte) {
		mocks.keyManager.On("Sign", mock.Anything, keyMapping, signpayloads.OPAQUE_TO_RSV, mock.Anything).
			Return(signature, nil)
	}

	return party
}

func TestPrivateTxManagerRemoteNotaryEndorser(t *testing.T) {
	ctx := context.Background()
	// A transaction that requires exactly one endorsement from a notary (as per noto) and therefore delegates coordination of the transaction to that node

	domainAddress := tktypes.MustEthAddress(tktypes.RandHex(20))
	privateTxManager, localNodeMocks, localNodeID := NewPrivateTransactionMgrForTesting(t, domainAddress)
	domainAddressString := domainAddress.String()

	remoteEngine, remoteEngineMocks, remoteNodeID := NewPrivateTransactionMgrForTesting(t, domainAddress)

	alice := newPartyForTesting(ctx, "alice", localNodeID, localNodeMocks)
	notary := newPartyForTesting(ctx, "notary", remoteNodeID, remoteEngineMocks)

	alice.mockResolve(ctx, notary)

	initialised := make(chan struct{}, 1)
	localNodeMocks.domainSmartContract.On("InitTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.PreAssembly = &components.TransactionPreAssembly{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{
					Lookup:       alice.identity,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
				{
					Lookup:       notary.identityLocator, // as it is a remote id, we need to use the locator
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
		}
		initialised <- struct{}{}
	}).Return(nil)

	assembled := make(chan struct{}, 1)
	delegated := make(chan struct{}, 1)

	localNodeMocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)

		tx.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			InputStates: []*components.FullState{
				{
					ID:     tktypes.RandBytes(32),
					Schema: tktypes.Bytes32(tktypes.RandBytes(32)),
					Data:   tktypes.JSONString("foo"),
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
			assert.Equal(t, remoteNodeID, args.Get(1).(*components.TransportMessage).Node)
			transportMessage := args.Get(1).(*components.TransportMessage)
			remoteEngine.ReceiveTransportMessage(ctx, transportMessage)
		}()
	}).Return(nil).Maybe()

	remoteEngineMocks.transportManager.On("Send", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		go func() {
			transportMessage := args.Get(1).(*components.TransportMessage)
			privateTxManager.ReceiveTransportMessage(ctx, transportMessage)
		}()
	}).Return(nil).Maybe()

	remoteEngineMocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, *domainAddress).Return(remoteEngineMocks.domainSmartContract, nil)

	signingAddress := tktypes.RandHex(32)

	//Dispatch should happen on the remote node
	remoteEngineMocks.domainSmartContract.On("ResolveDispatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.Signer = signingAddress
		delegated <- struct{}{}
	}).Return(nil)

	//TODO match endorsement request and verifier args
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

	notary.mockSign([]byte("some-signature-bytes"))

	remoteEngineMocks.domainSmartContract.On("PrepareTransaction", mock.Anything, mock.Anything).Return(nil).Run(
		func(args mock.Arguments) {
			cv, err := testABI[0].Inputs.ParseExternalData(map[string]any{
				"inputs":  []any{tktypes.Bytes32(tktypes.RandBytes(32))},
				"outputs": []any{tktypes.Bytes32(tktypes.RandBytes(32))},
				"data":    "0xfeedbeef",
			})
			require.NoError(t, err)
			tx := args[1].(*components.PrivateTransaction)
			tx.Signer = "signer1"
			jsonData, _ := cv.JSON()
			tx.PreparedPublicTransaction = &pldapi.TransactionInput{
				ABI: abi.ABI{testABI[0]},
				Transaction: pldapi.Transaction{
					To:   domainAddress,
					Data: tktypes.RawJSON(jsonData),
				},
			}
		},
	)

	tx := &components.PrivateTransaction{
		ID: uuid.New(),
		Inputs: &components.TransactionInputs{
			Domain: "domain1",
			To:     *domainAddress,
		},
	}

	mockPublicTxBatch := componentmocks.NewPublicTxBatch(t)
	mockPublicTxBatch.On("Finalize", mock.Anything).Return().Maybe()
	mockPublicTxBatch.On("CleanUp", mock.Anything).Return().Maybe()

	mockPublicTxManager := remoteEngineMocks.publicTxManager.(*componentmocks.PublicTxManager)
	mockPublicTxManager.On("PrepareSubmissionBatch", mock.Anything, mock.Anything).Return(mockPublicTxBatch, nil)

	signingAddr := tktypes.RandAddress()
	remoteEngineMocks.keyManager.On("ResolveEthAddressBatchNewDatabaseTX", mock.Anything, []string{"signer1"}).
		Return([]*tktypes.EthAddress{signingAddr}, nil)

	publicTransactions := []components.PublicTxAccepted{
		newFakePublicTx(&components.PublicTxSubmission{
			Bindings: []*components.PaladinTXReference{{TransactionID: tx.ID, TransactionType: pldapi.TransactionTypePrivate.Enum()}},
			PublicTxInput: pldapi.PublicTxInput{
				From: signingAddr,
			},
		}, nil),
	}
	mockPublicTxBatch.On("Submit", mock.Anything, mock.Anything).Return(nil)
	mockPublicTxBatch.On("Rejected").Return([]components.PublicTxRejected{})
	mockPublicTxBatch.On("Accepted").Return(publicTransactions)
	mockPublicTxBatch.On("Completed", mock.Anything, true).Return()

	err := privateTxManager.Start()
	assert.NoError(t, err)

	err = privateTxManager.HandleNewTx(ctx, tx)
	assert.NoError(t, err)

	<-delegated
	status := pollForStatus(ctx, t, "dispatched", remoteEngine, domainAddressString, tx.ID.String(), 200*time.Second)
	assert.Equal(t, "dispatched", status)

}

func TestPrivateTxManagerEndorsementGroup(t *testing.T) {

	ctx := context.Background()
	// A transaction that requires endorsement from a group of remote endorsers (as per pente and its 100% endorsement policy)
	// In this scenario there is only one active transaction and therefore no risk of contention so the transactions is coordinated
	// and dispatched locally.  The only expected interaction with the remote nodes is to request endorsements and to distribute the new states

	domainAddress := tktypes.MustEthAddress(tktypes.RandHex(20))
	domainAddressString := domainAddress.String()

	aliceEngine, aliceEngineMocks, aliceNodeID := NewPrivateTransactionMgrForTesting(t, domainAddress)
	bobEngine, bobEngineMocks, bobNodeID := NewPrivateTransactionMgrForTesting(t, domainAddress)
	carolEngine, carolEngineMocks, carolNodeID := NewPrivateTransactionMgrForTesting(t, domainAddress)

	alice := newPartyForTesting(ctx, "alice", aliceNodeID, aliceEngineMocks)
	bob := newPartyForTesting(ctx, "bob", bobNodeID, bobEngineMocks)
	carol := newPartyForTesting(ctx, "carol", carolNodeID, carolEngineMocks)

	alice.mockResolve(ctx, bob)
	alice.mockResolve(ctx, carol)

	initialised := make(chan struct{}, 1)

	aliceEngineMocks.domainSmartContract.On("InitTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.PreAssembly = &components.TransactionPreAssembly{
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
		initialised <- struct{}{}
	}).Return(nil)

	aliceEngineMocks.identityResolver.On("ResolveVerifierAsync", mock.Anything, alice.identityLocator, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		resovleFn := args.Get(4).(func(context.Context, string))
		resovleFn(ctx, alice.verifier)
	}).Return(nil)

	aliceEngineMocks.identityResolver.On("ResolveVerifierAsync", mock.Anything, bob.identityLocator, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		resovleFn := args.Get(4).(func(context.Context, string))
		resovleFn(ctx, bob.verifier)
	}).Return(nil)

	aliceEngineMocks.identityResolver.On("ResolveVerifierAsync", mock.Anything, carol.identityLocator, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		resovleFn := args.Get(4).(func(context.Context, string))
		resovleFn(ctx, carol.verifier)
	}).Return(nil)

	assembled := make(chan struct{}, 1)
	aliceEngineMocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)

		tx.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			InputStates: []*components.FullState{
				{
					ID:     tktypes.RandBytes(32),
					Schema: tktypes.Bytes32(tktypes.RandBytes(32)),
					Data:   tktypes.JSONString("foo"),
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
		assembled <- struct{}{}

	}).Return(nil)

	routeToNode := func(args mock.Arguments) {
		go func() {
			transportMessage := args.Get(1).(*components.TransportMessage)
			switch transportMessage.Node {
			case aliceNodeID:
				aliceEngine.ReceiveTransportMessage(ctx, transportMessage)
			case bobNodeID:
				bobEngine.ReceiveTransportMessage(ctx, transportMessage)
			case carolNodeID:
				carolEngine.ReceiveTransportMessage(ctx, transportMessage)
			}
		}()
	}

	aliceEngineMocks.transportManager.On("Send", mock.Anything, mock.Anything).Run(routeToNode).Return(nil).Maybe()
	bobEngineMocks.transportManager.On("Send", mock.Anything, mock.Anything).Run(routeToNode).Return(nil).Maybe()
	carolEngineMocks.transportManager.On("Send", mock.Anything, mock.Anything).Run(routeToNode).Return(nil).Maybe()

	//set up the mocks on bob and carols engines that are need on the endorse code path (and of course also on alice's engine because she is an endorser too)

	bobEngineMocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, *domainAddress).Return(bobEngineMocks.domainSmartContract, nil)
	carolEngineMocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, *domainAddress).Return(carolEngineMocks.domainSmartContract, nil)

	signingAddress := tktypes.RandHex(32)

	aliceEngineMocks.domainSmartContract.On("ResolveDispatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.Signer = signingAddress
	}).Return(nil)

	//TODO match endorsement request and verifier args
	aliceEngineMocks.domainSmartContract.On("EndorseTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: []byte("alice-endorsement-bytes"),
		Endorser: &prototk.ResolvedVerifier{
			Lookup:       alice.keyHandle,
			Verifier:     alice.verifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}, nil)

	alice.mockSign([]byte("alice-signature-bytes"))

	bobEngineMocks.domainSmartContract.On("EndorseTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: []byte("bob-endorsement-bytes"),
		Endorser: &prototk.ResolvedVerifier{
			Lookup:       bob.keyHandle,
			Verifier:     bob.verifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}, nil)

	bob.mockSign([]byte("bob-signature-bytes"))

	carolEngineMocks.domainSmartContract.On("EndorseTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: []byte("carol-endorsement-bytes"),
		Endorser: &prototk.ResolvedVerifier{
			Lookup:       carol.keyHandle,
			Verifier:     carol.verifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}, nil)

	carol.mockSign([]byte("carol-signature-bytes"))

	aliceEngineMocks.domainSmartContract.On("PrepareTransaction", mock.Anything, mock.Anything).Return(nil).Run(
		func(args mock.Arguments) {
			cv, err := testABI[0].Inputs.ParseExternalData(map[string]any{
				"inputs":  []any{tktypes.Bytes32(tktypes.RandBytes(32))},
				"outputs": []any{tktypes.Bytes32(tktypes.RandBytes(32))},
				"data":    "0xfeedbeef",
			})
			require.NoError(t, err)
			tx := args[1].(*components.PrivateTransaction)
			tx.Signer = "signer1"
			jsonData, _ := cv.JSON()
			tx.PreparedPublicTransaction = &pldapi.TransactionInput{
				ABI: abi.ABI{testABI[0]},
				Transaction: pldapi.Transaction{
					To:   domainAddress,
					Data: tktypes.RawJSON(jsonData),
				},
			}
			aliceEndorsed, bobEndorsed, carolEndorsed := false, false, false
			for _, endorsement := range tx.PostAssembly.Endorsements {
				switch endorsement.Verifier.Verifier {
				case alice.verifier:
					if reflect.DeepEqual(endorsement.Payload, []byte("alice-signature-bytes")) {
						aliceEndorsed = true
					}
				case bob.verifier:
					if reflect.DeepEqual(endorsement.Payload, []byte("bob-signature-bytes")) {
						bobEndorsed = true
					}
				case carol.verifier:
					if reflect.DeepEqual(endorsement.Payload, []byte("carol-signature-bytes")) {
						carolEndorsed = true
					}
				}
			}
			assert.True(t, aliceEndorsed)
			assert.True(t, bobEndorsed)
			assert.True(t, carolEndorsed)
		},
	)

	tx := &components.PrivateTransaction{
		ID: uuid.New(),
		Inputs: &components.TransactionInputs{
			Domain: "domain1",
			To:     *domainAddress,
		},
	}

	mockPublicTxBatch := componentmocks.NewPublicTxBatch(t)
	mockPublicTxBatch.On("Finalize", mock.Anything).Return().Maybe()
	mockPublicTxBatch.On("CleanUp", mock.Anything).Return().Maybe()

	mockPublicTxManager := aliceEngineMocks.publicTxManager.(*componentmocks.PublicTxManager)
	mockPublicTxManager.On("PrepareSubmissionBatch", mock.Anything, mock.Anything).Return(mockPublicTxBatch, nil)

	signingAddr := tktypes.RandAddress()
	aliceEngineMocks.keyManager.On("ResolveEthAddressBatchNewDatabaseTX", mock.Anything, []string{"signer1"}).
		Return([]*tktypes.EthAddress{signingAddr}, nil)

	publicTransactions := []components.PublicTxAccepted{
		newFakePublicTx(&components.PublicTxSubmission{
			Bindings: []*components.PaladinTXReference{{TransactionID: tx.ID, TransactionType: pldapi.TransactionTypePrivate.Enum()}},
			PublicTxInput: pldapi.PublicTxInput{
				From: signingAddr,
			},
		}, nil),
	}
	mockPublicTxBatch.On("Submit", mock.Anything, mock.Anything).Return(nil)
	mockPublicTxBatch.On("Rejected").Return([]components.PublicTxRejected{})
	mockPublicTxBatch.On("Accepted").Return(publicTransactions)
	mockPublicTxBatch.On("Completed", mock.Anything, true).Return()

	err := aliceEngine.Start()
	assert.NoError(t, err)

	err = aliceEngine.HandleNewTx(ctx, tx)
	assert.NoError(t, err)

	status := pollForStatus(ctx, t, "dispatched", aliceEngine, domainAddressString, tx.ID.String(), 200*time.Second)
	assert.Equal(t, "dispatched", status)

}

func TestPrivateTxManagerDependantTransactionEndorsedOutOfOrder(t *testing.T) {
	// extension to the TestPrivateTxManagerEndorsementGroup test
	// 2 transactions, one dependant on the other
	// we purposely endorse the first transaction late to ensure that the 2nd transaction
	// is still sequenced behind the first

	ctx := context.Background()

	domainAddress := tktypes.MustEthAddress(tktypes.RandHex(20))
	domainAddressString := domainAddress.String()

	aliceEngine, aliceEngineMocks, aliceNodeID := NewPrivateTransactionMgrForTesting(t, domainAddress)
	_, bobEngineMocks, bobNodeID := NewPrivateTransactionMgrForTesting(t, domainAddress)

	alice := newPartyForTesting(ctx, "alice", aliceNodeID, aliceEngineMocks)
	bob := newPartyForTesting(ctx, "bob", bobNodeID, bobEngineMocks)

	alice.mockResolve(ctx, bob)

	aliceEngineMocks.domainSmartContract.On("InitTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.PreAssembly = &components.TransactionPreAssembly{
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

	// TODO check that the transaction is signed with this key

	states := []*components.FullState{
		{
			ID:     tktypes.RandBytes(32),
			Schema: tktypes.Bytes32(tktypes.RandBytes(32)),
			Data:   tktypes.JSONString("foo"),
		},
	}

	potentialStates := []*prototk.NewState{
		{
			SchemaId:      states[0].Schema.String(),
			StateDataJson: states[0].Data.String(),
		},
	}

	tx1 := &components.PrivateTransaction{
		ID: uuid.New(),
		Inputs: &components.TransactionInputs{
			Domain: "domain1",
			To:     *domainAddress,
			From:   alice.identity,
		},
	}

	tx2 := &components.PrivateTransaction{
		ID: uuid.New(),
		Inputs: &components.TransactionInputs{
			Domain: "domain1",
			To:     *domainAddress,
			From:   alice.identity,
		},
	}

	aliceEngineMocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		switch tx.ID.String() {
		case tx1.ID.String():
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
		case tx2.ID.String():
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

	sentEndorsementRequest := make(chan struct{}, 1)
	aliceEngineMocks.transportManager.On("Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		sentEndorsementRequest <- struct{}{}
	}).Return(nil).Maybe()

	aliceEngineMocks.domainSmartContract.On("EndorseTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&components.EndorsementResult{
		Result:  prototk.EndorseTransactionResponse_SIGN,
		Payload: []byte("alice-endorsement-bytes"),
		Endorser: &prototk.ResolvedVerifier{
			Lookup:       alice.keyHandle,
			Verifier:     alice.verifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}, nil)

	alice.mockSign([]byte("alice-signature-bytes"))

	signingAddress := tktypes.RandHex(32)
	aliceEngineMocks.domainSmartContract.On("ResolveDispatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		tx.Signer = signingAddress
	}).Return(nil)

	aliceEngineMocks.domainSmartContract.On("PrepareTransaction", mock.Anything, mock.Anything).Return(nil).Run(
		func(args mock.Arguments) {
			cv, err := testABI[0].Inputs.ParseExternalData(map[string]any{
				"inputs":  []any{tktypes.Bytes32(tktypes.RandBytes(32))},
				"outputs": []any{tktypes.Bytes32(tktypes.RandBytes(32))},
				"data":    "0xfeedbeef",
			})
			require.NoError(t, err)
			tx := args[1].(*components.PrivateTransaction)
			tx.Signer = "signer1"
			jsonData, _ := cv.JSON()
			tx.PreparedPublicTransaction = &pldapi.TransactionInput{
				ABI: abi.ABI{testABI[0]},
				Transaction: pldapi.Transaction{
					To:   domainAddress,
					Data: tktypes.RawJSON(jsonData),
				},
			}
		},
	)
	tx := &components.PrivateTransaction{
		ID: uuid.New(),
	}

	mockPublicTxBatch := componentmocks.NewPublicTxBatch(t)
	mockPublicTxBatch.On("Finalize", mock.Anything).Return().Maybe()
	mockPublicTxBatch.On("CleanUp", mock.Anything).Return().Maybe()

	mockPublicTxManager := aliceEngineMocks.publicTxManager.(*componentmocks.PublicTxManager)
	mockPublicTxManager.On("PrepareSubmissionBatch", mock.Anything, mock.Anything).Return(mockPublicTxBatch, nil)

	signingAddr := tktypes.RandAddress()
	aliceEngineMocks.keyManager.On("ResolveEthAddressBatchNewDatabaseTX", mock.Anything, []string{"signer1", "signer1"}).
		Return([]*tktypes.EthAddress{signingAddr, signingAddr}, nil)

	publicTransactions := []components.PublicTxAccepted{
		newFakePublicTx(&components.PublicTxSubmission{
			Bindings: []*components.PaladinTXReference{{TransactionID: tx.ID, TransactionType: pldapi.TransactionTypePrivate.Enum()}},
			PublicTxInput: pldapi.PublicTxInput{
				From: signingAddr,
			},
		}, nil),
		newFakePublicTx(&components.PublicTxSubmission{
			Bindings: []*components.PaladinTXReference{{TransactionID: tx.ID, TransactionType: pldapi.TransactionTypePrivate.Enum()}},
			PublicTxInput: pldapi.PublicTxInput{
				From: signingAddr,
			},
		}, nil),
	}
	mockPublicTxBatch.On("Submit", mock.Anything, mock.Anything).Return(nil)
	mockPublicTxBatch.On("Rejected").Return([]components.PublicTxRejected{})
	mockPublicTxBatch.On("Accepted").Return(publicTransactions)
	mockPublicTxBatch.On("Completed", mock.Anything, true).Return()

	err := aliceEngine.Start()
	require.NoError(t, err)

	err = aliceEngine.HandleNewTx(ctx, tx1)
	require.NoError(t, err)

	err = aliceEngine.HandleNewTx(ctx, tx2)
	require.NoError(t, err)

	// Neither transaction should be dispatched yet
	s, err := aliceEngine.GetTxStatus(ctx, domainAddressString, tx1.ID.String())
	require.NoError(t, err)
	assert.NotEqual(t, "dispatch", s.Status)

	s, err = aliceEngine.GetTxStatus(ctx, domainAddressString, tx2.ID.String())
	require.NoError(t, err)
	assert.NotEqual(t, "dispatch", s.Status)

	attestationResult := prototk.AttestationResult{
		Name:            "notary",
		AttestationType: prototk.AttestationType_ENDORSE,
		Payload:         tktypes.RandBytes(32),
		Verifier: &prototk.ResolvedVerifier{
			Verifier: bob.verifier,
		},
	}

	attestationResultAny, err := anypb.New(&attestationResult)
	require.NoError(t, err)

	//wait for both transactions to send an endorsement request each
	<-sentEndorsementRequest
	<-sentEndorsementRequest

	// endorse transaction 2 before 1 and check that 2 is not dispatched before 1
	endorsementResponse2 := &pbEngine.EndorsementResponse{
		ContractAddress: domainAddressString,
		TransactionId:   tx2.ID.String(),
		Endorsement:     attestationResultAny,
	}
	endorsementResponse2bytes, err := proto.Marshal(endorsementResponse2)
	require.NoError(t, err)

	//now send the endorsements back
	aliceEngine.ReceiveTransportMessage(ctx, &components.TransportMessage{
		MessageType: "EndorsementResponse",
		Payload:     endorsementResponse2bytes,
	})

	//unless the tests are running in short mode, wait a second to ensure that the transaction is not dispatched
	if !testing.Short() {
		time.Sleep(1 * time.Second)
	}
	s, err = aliceEngine.GetTxStatus(ctx, domainAddressString, tx1.ID.String())
	require.NoError(t, err)
	assert.NotEqual(t, "dispatch", s.Status)

	s, err = aliceEngine.GetTxStatus(ctx, domainAddressString, tx2.ID.String())
	require.NoError(t, err)
	assert.NotEqual(t, "dispatch", s.Status)

	// endorse transaction 1 and check that both it and 2 are dispatched
	endorsementResponse1 := &pbEngine.EndorsementResponse{
		ContractAddress: domainAddressString,
		TransactionId:   tx1.ID.String(),
		Endorsement:     attestationResultAny,
	}
	endorsementResponse1Bytes, err := proto.Marshal(endorsementResponse1)
	require.NoError(t, err)

	//now send the final endorsement back
	aliceEngine.ReceiveTransportMessage(ctx, &components.TransportMessage{
		MessageType: "EndorsementResponse",
		Payload:     endorsementResponse1Bytes,
	})

	status := pollForStatus(ctx, t, "dispatched", aliceEngine, domainAddressString, tx1.ID.String(), 200*time.Second)
	assert.Equal(t, "dispatched", status)

	status = pollForStatus(ctx, t, "dispatched", aliceEngine, domainAddressString, tx2.ID.String(), 200*time.Second)
	assert.Equal(t, "dispatched", status)

	//TODO assert that transaction 1 got dispatched before 2

}

func TestPrivateTxManagerLocalBlockedTransaction(t *testing.T) {
	//TODO
	// 3 transactions, for different signing addresses, but two are is blocked by the other
	// when the earlier transaction is confirmed, both blocked transactions should be dispatched
}

func TestPrivateTxManagerMiniLoad(t *testing.T) {
	t.Skip("This test takes too long to be included by default.  It is still useful for local testing")
	//TODO this is actually quite a complex test given all the mocking.  Maybe this should be converted to a wider component test
	// where the real publicTxManager is used rather than a mock
	r := rand.New(rand.NewSource(42))
	loadTests := []struct {
		name            string
		latency         func() time.Duration
		numTransactions int
	}{
		{"no-latency", func() time.Duration { return 0 }, 5},
		{"low-latency", func() time.Duration { return 10 * time.Millisecond }, 500},
		{"medium-latency", func() time.Duration { return 50 * time.Millisecond }, 500},
		{"high-latency", func() time.Duration { return 100 * time.Millisecond }, 500},
		{"random-none-to-low-latency", func() time.Duration { return time.Duration(r.Intn(10)) * time.Millisecond }, 500},
		{"random-none-to-high-latency", func() time.Duration { return time.Duration(r.Intn(100)) * time.Millisecond }, 500},
	}
	//500 is the maximum we can do in this test for now until either
	//a) implement config to allow us to define MaxConcurrentTransactions
	//b) implement ( or mock) transaction dispatch processing all the way to confirmation

	for _, test := range loadTests {
		t.Run(test.name, func(t *testing.T) {

			ctx := context.Background()

			domainAddress := tktypes.MustEthAddress(tktypes.RandHex(20))
			privateTxManager, mocks, _ := NewPrivateTransactionMgrForTestingWithFakePublicTxManager(t, domainAddress, newFakePublicTxManager(t))

			remoteEngine, remoteEngineMocks, _ := NewPrivateTransactionMgrForTestingWithFakePublicTxManager(t, domainAddress, newFakePublicTxManager(t))

			dependenciesByTransactionID := make(map[string][]string) // populated during assembly stage
			nonceByTransactionID := make(map[string]uint64)          // populated when dispatch event recieved and used later to check that the nonce order matchs the dependency order

			unclaimedPendingStatesToMintingTransaction := make(map[tktypes.Bytes32]string)

			mocks.domainSmartContract.On("InitTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				tx := args.Get(1).(*components.PrivateTransaction)
				tx.PreAssembly = &components.TransactionPreAssembly{
					RequiredVerifiers: []*prototk.ResolveVerifierRequest{
						{
							Lookup:       "alice",
							Algorithm:    algorithms.ECDSA_SECP256K1,
							VerifierType: verifiers.ETH_ADDRESS,
						},
					},
				}
			}).Return(nil)
			mocks.identityResolver.On("ResolveVerifierAsync", mock.Anything, "alice", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				resovleFn := args.Get(4).(func(context.Context, string))
				resovleFn(ctx, "aliceVerifier")
			}).Return(nil)

			failEarly := make(chan string, 1)

			assembleConcurrency := 0
			mocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				//assert that we are not assembling more than 1 transaction at a time
				if assembleConcurrency > 0 {
					failEarly <- "Assembling more than one transaction at a time"
				}
				require.Equal(t, assembleConcurrency, 0, "Assembling more than one transaction at a time")

				assembleConcurrency++
				defer func() { assembleConcurrency-- }()

				// chose a number of dependencies at random 0, 1, 2, 3
				// for each dependency, chose a different unclaimed pending state to spend
				tx := args.Get(1).(*components.PrivateTransaction)

				var inputStates []*components.FullState
				numDependencies := min(r.Intn(4), len(unclaimedPendingStatesToMintingTransaction))
				dependencies := make([]string, numDependencies)
				for i := 0; i < numDependencies; i++ {
					// chose a random unclaimed pending state to spend
					stateIndex := r.Intn(len(unclaimedPendingStatesToMintingTransaction))

					keys := make([]tktypes.Bytes32, len(unclaimedPendingStatesToMintingTransaction))
					keyIndex := 0
					for keyName := range unclaimedPendingStatesToMintingTransaction {

						keys[keyIndex] = keyName
						keyIndex++
					}
					stateID := keys[stateIndex]
					inputStates = append(inputStates, &components.FullState{
						ID: stateID[:],
					})

					log.L(ctx).Infof("input state %s, numDependencies %d i %d", stateID, numDependencies, i)
					dependencies[i] = unclaimedPendingStatesToMintingTransaction[stateID]
					delete(unclaimedPendingStatesToMintingTransaction, stateID)
				}
				dependenciesByTransactionID[tx.ID.String()] = dependencies

				numOutputStates := r.Intn(4)
				outputStates := make([]*components.FullState, numOutputStates)
				for i := 0; i < numOutputStates; i++ {
					stateID := tktypes.Bytes32(tktypes.RandBytes(32))
					outputStates[i] = &components.FullState{
						ID: stateID[:],
					}
					unclaimedPendingStatesToMintingTransaction[stateID] = tx.ID.String()
				}

				tx.PostAssembly = &components.TransactionPostAssembly{

					AssemblyResult: prototk.AssembleTransactionResponse_OK,
					OutputStates:   outputStates,
					InputStates:    inputStates,
					AttestationPlan: []*prototk.AttestationRequest{
						{
							Name:            "notary",
							AttestationType: prototk.AttestationType_ENDORSE,
							Algorithm:       algorithms.ECDSA_SECP256K1,
							VerifierType:    verifiers.ETH_ADDRESS,
							PayloadType:     signpayloads.OPAQUE_TO_RSV,
							Parties: []string{
								"domain1.contract1.notary@othernode",
							},
						},
					},
				}
			}).Return(nil)

			mocks.transportManager.On("Send", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				go func() {
					//inject random latency on the network
					time.Sleep(test.latency())
					transportMessage := args.Get(1).(*components.TransportMessage)
					remoteEngine.ReceiveTransportMessage(ctx, transportMessage)
				}()
			}).Return(nil).Maybe()

			remoteEngineMocks.transportManager.On("Send", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				go func() {
					//inject random latency on the network
					time.Sleep(test.latency())
					transportMessage := args.Get(1).(*components.TransportMessage)
					privateTxManager.ReceiveTransportMessage(ctx, transportMessage)
				}()
			}).Return(nil).Maybe()
			remoteEngineMocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, *domainAddress).Return(remoteEngineMocks.domainSmartContract, nil)

			remoteEngineMocks.keyManager.On("ResolveKey", mock.Anything, "domain1.contract1.notary@othernode", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).Return("domain1.contract1.notary", "notaryVerifier", nil)

			signingAddress := tktypes.RandHex(32)

			mocks.domainSmartContract.On("ResolveDispatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				tx := args.Get(1).(*components.PrivateTransaction)
				tx.Signer = signingAddress
			}).Return(nil)

			//TODO match endorsement request and verifier args
			remoteEngineMocks.domainSmartContract.On("EndorseTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&components.EndorsementResult{
				Result:  prototk.EndorseTransactionResponse_SIGN,
				Payload: []byte("some-endorsement-bytes"),
				Endorser: &prototk.ResolvedVerifier{
					Lookup:       "domain1.contract1.notary",
					Verifier:     "notaryVerifier",
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			}, nil)
			remoteEngineMocks.keyManager.On("Sign", mock.Anything, &signerapi.SignRequest{
				KeyHandle:   "domain1.contract1.notary",
				Algorithm:   algorithms.ECDSA_SECP256K1,
				PayloadType: signpayloads.OPAQUE_TO_RSV,
				Payload:     []byte("some-endorsement-bytes"),
			}).Return(&signerapi.SignResponse{
				Payload: []byte("some-signature-bytes"),
			}, nil)

			mocks.domainSmartContract.On("PrepareTransaction", mock.Anything, mock.Anything).Return(nil)

			expectedNonce := uint64(0)

			numDispatched := 0
			allDispatched := make(chan bool, 1)
			nonceWriterLock := sync.Mutex{}
			privateTxManager.Subscribe(ctx, func(event components.PrivateTxEvent) {
				nonceWriterLock.Lock()
				defer nonceWriterLock.Unlock()
				numDispatched++
				switch event := event.(type) {
				case *components.TransactionDispatchedEvent:
					assert.Equal(t, expectedNonce, event.Nonce)
					expectedNonce++
					nonceByTransactionID[event.TransactionID] = event.Nonce
				}
				if numDispatched == test.numTransactions {
					allDispatched <- true
				}
			})

			err := privateTxManager.Start()
			require.NoError(t, err)

			for i := 0; i < test.numTransactions; i++ {
				tx := &components.PrivateTransaction{
					ID: uuid.New(),
					Inputs: &components.TransactionInputs{
						Domain: "domain1",
						To:     *domainAddress,
						From:   "Alice",
					},
				}
				err = privateTxManager.HandleNewTx(ctx, tx)
				require.NoError(t, err)
			}

			haveAllDispatched := false
		out:
			for {
				select {
				case <-time.After(timeTillDeadline(t)):
					log.L(ctx).Errorf("Timed out waiting for all transactions to be dispatched")
					assert.Fail(t, "Timed out waiting for all transactions to be dispatched")
					break out
				case <-allDispatched:
					haveAllDispatched = true
					break out
				case reason := <-failEarly:
					require.Fail(t, reason)
				}
			}

			if haveAllDispatched {
				//check that they were dispatched a valid order ( i.e. no transaction was dispatched before its dependencies)
				for txId, nonce := range nonceByTransactionID {
					dependencies := dependenciesByTransactionID[txId]
					for _, depTxID := range dependencies {
						depNonce, ok := nonceByTransactionID[depTxID]
						assert.True(t, ok)
						assert.True(t, depNonce < nonce, "Transaction %s (nonce %d) was dispatched before its dependency %s (nonce %d)", txId, nonce, depTxID, depNonce)
					}
				}
			}
		})
	}
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
			s, err := privateTxManager.GetTxStatus(ctx, domainAddressString, txID)
			require.NoError(t, err)
			return s.Status
		case <-tick:
			s, err := privateTxManager.GetTxStatus(ctx, domainAddressString, txID)
			if s.Status == expectedStatus {
				return s.Status
			}
			require.NoError(t, err)
		}
	}
}

type dependencyMocks struct {
	allComponents       *componentmocks.AllComponents
	domain              *componentmocks.Domain
	domainSmartContract *componentmocks.DomainSmartContract
	domainContext       *componentmocks.DomainContext
	domainMgr           *componentmocks.DomainManager
	transportManager    *componentmocks.TransportManager
	stateStore          *componentmocks.StateManager
	keyManager          *componentmocks.KeyManager
	publicTxManager     components.PublicTxManager /* could be fake or mock */
	identityResolver    *componentmocks.IdentityResolver
	txManager           *componentmocks.TXManager
}

// For Black box testing we return components.PrivateTxManager
func NewPrivateTransactionMgrForTesting(t *testing.T, domainAddress *tktypes.EthAddress) (components.PrivateTxManager, *dependencyMocks, string) {
	// by default create a mock publicTxManager if no fake was provided
	fakePublicTxManager := componentmocks.NewPublicTxManager(t)
	privateTxManager, mocks, nodeID := NewPrivateTransactionMgrForTestingWithFakePublicTxManager(t, domainAddress, fakePublicTxManager)
	return privateTxManager, mocks, nodeID
}

type fakePublicTxManager struct {
	t          *testing.T
	rejectErr  error
	prepareErr error
}

// GetPublicTransactionForHash implements components.PublicTxManager.
func (f *fakePublicTxManager) GetPublicTransactionForHash(ctx context.Context, dbTX *gorm.DB, hash tktypes.Bytes32) (*pldapi.PublicTxWithBinding, error) {
	panic("unimplemented")
}

// QueryPublicTxForTransactions implements components.PublicTxManager.
func (f *fakePublicTxManager) QueryPublicTxForTransactions(ctx context.Context, dbTX *gorm.DB, boundToTxns []uuid.UUID, jq *query.QueryJSON) (map[uuid.UUID][]*pldapi.PublicTx, error) {
	panic("unimplemented")
}

// QueryPublicTxWithBindings implements components.PublicTxManager.
func (f *fakePublicTxManager) QueryPublicTxWithBindings(ctx context.Context, dbTX *gorm.DB, jq *query.QueryJSON) ([]*pldapi.PublicTxWithBinding, error) {
	panic("unimplemented")
}

// MatchUpdateConfirmedTransactions implements components.PublicTxManager.
func (f *fakePublicTxManager) MatchUpdateConfirmedTransactions(ctx context.Context, dbTX *gorm.DB, itxs []*blockindexer.IndexedTransactionNotify) ([]*components.PublicTxMatch, error) {
	panic("unimplemented")
}

// NotifyConfirmPersisted implements components.PublicTxManager.
func (f *fakePublicTxManager) NotifyConfirmPersisted(ctx context.Context, confirms []*components.PublicTxMatch) {
	panic("unimplemented")
}

// PostInit implements components.PublicTxManager.
func (f *fakePublicTxManager) PostInit(components.AllComponents) error {
	panic("unimplemented")
}

// PreInit implements components.PublicTxManager.
func (f *fakePublicTxManager) PreInit(components.PreInitComponents) (*components.ManagerInitResult, error) {
	panic("unimplemented")
}

// Start implements components.PublicTxManager.
func (f *fakePublicTxManager) Start() error {
	panic("unimplemented")
}

// Stop implements components.PublicTxManager.
func (f *fakePublicTxManager) Stop() {
	panic("unimplemented")
}

type fakePublicTxBatch struct {
	t              *testing.T
	transactions   []*components.PublicTxSubmission
	accepted       []components.PublicTxAccepted
	rejected       []components.PublicTxRejected
	completeCalled bool
	committed      bool
	submitErr      error
}

func (f *fakePublicTxBatch) Accepted() []components.PublicTxAccepted {
	return f.accepted
}

func (f *fakePublicTxBatch) Completed(ctx context.Context, committed bool) {
	f.completeCalled = true
	f.committed = committed
}

func (f *fakePublicTxBatch) Rejected() []components.PublicTxRejected {
	return f.rejected
}

type fakePublicTx struct {
	t         *components.PublicTxSubmission
	rejectErr error
	pubTx     *pldapi.PublicTx
}

func newFakePublicTx(t *components.PublicTxSubmission, rejectErr error) *fakePublicTx {
	return &fakePublicTx{
		t:         t,
		rejectErr: rejectErr,
		pubTx: &pldapi.PublicTx{
			To:              t.To,
			Data:            t.Data,
			From:            *t.From,
			Created:         tktypes.TimestampNow(),
			PublicTxOptions: t.PublicTxOptions,
		},
	}
}

func (f *fakePublicTx) RejectedError() error {
	return f.rejectErr
}

func (f *fakePublicTx) RevertData() tktypes.HexBytes {
	return []byte("some data")
}

func (f *fakePublicTx) Bindings() []*components.PaladinTXReference {
	return f.t.Bindings
}

func (f *fakePublicTx) PublicTx() *pldapi.PublicTx {
	return f.pubTx
}

//for this test, we need a hand written fake rather than a simple mock for publicTxManager

// PrepareSubmissionBatch implements components.PublicTxManager.
func (f *fakePublicTxManager) PrepareSubmissionBatch(ctx context.Context, transactions []*components.PublicTxSubmission) (batch components.PublicTxBatch, err error) {
	b := &fakePublicTxBatch{t: f.t, transactions: transactions}
	if f.rejectErr != nil {
		for _, t := range transactions {
			b.rejected = append(b.rejected, newFakePublicTx(t, f.rejectErr))
		}
	} else {
		for _, t := range transactions {
			b.accepted = append(b.accepted, newFakePublicTx(t, nil))
		}
	}
	return b, f.prepareErr
}

// SubmitBatch implements components.PublicTxManager.
func (f *fakePublicTxBatch) Submit(ctx context.Context, dbTX *gorm.DB) error {
	nonceBase := 1000
	for i, tx := range f.accepted {
		tx.(*fakePublicTx).pubTx.Nonce = tktypes.HexUint64(nonceBase + i)
	}
	return f.submitErr
}

func newFakePublicTxManager(t *testing.T) *fakePublicTxManager {
	return &fakePublicTxManager{
		t: t,
	}
}

func NewPrivateTransactionMgrForTestingWithFakePublicTxManager(t *testing.T, domainAddress *tktypes.EthAddress, publicTxMgr components.PublicTxManager) (components.PrivateTxManager, *dependencyMocks, string) {

	nodeID := tktypes.RandHex(16)
	ctx := context.Background()
	mocks := &dependencyMocks{
		allComponents:       componentmocks.NewAllComponents(t),
		domain:              componentmocks.NewDomain(t),
		domainSmartContract: componentmocks.NewDomainSmartContract(t),
		domainContext:       componentmocks.NewDomainContext(t),
		domainMgr:           componentmocks.NewDomainManager(t),
		transportManager:    componentmocks.NewTransportManager(t),
		stateStore:          componentmocks.NewStateManager(t),
		keyManager:          componentmocks.NewKeyManager(t),
		publicTxManager:     publicTxMgr,
		identityResolver:    componentmocks.NewIdentityResolver(t),
		txManager:           componentmocks.NewTXManager(t),
	}
	mocks.allComponents.On("StateManager").Return(mocks.stateStore).Maybe()
	mocks.allComponents.On("DomainManager").Return(mocks.domainMgr).Maybe()
	mocks.allComponents.On("TransportManager").Return(mocks.transportManager).Maybe()
	mocks.transportManager.On("LocalNodeName").Return(nodeID)
	mocks.allComponents.On("KeyManager").Return(mocks.keyManager).Maybe()
	mocks.allComponents.On("TxManager").Return(mocks.txManager).Maybe()
	mocks.allComponents.On("PublicTxManager").Return(publicTxMgr).Maybe()
	mocks.allComponents.On("Persistence").Return(persistence.NewUnitTestPersistence(ctx)).Maybe()
	mocks.domainSmartContract.On("Domain").Return(mocks.domain).Maybe()
	mocks.stateStore.On("NewDomainContext", mock.Anything, mocks.domain, *domainAddress, mock.Anything).Return(mocks.domainContext).Maybe()
	mocks.domain.On("Name").Return("domain1").Maybe()

	e := NewPrivateTransactionMgr(ctx, &pldconf.PrivateTxManagerConfig{
		Writer: pldconf.FlushWriterConfig{
			WorkerCount:  confutil.P(1),
			BatchMaxSize: confutil.P(1), // we don't want batching for our test
		},
		Orchestrator: pldconf.PrivateTxManagerOrchestratorConfig{
			// StaleTimeout: ,
		},
	})

	//It is not valid to call other managers before PostInit
	mocks.transportManager.On("RegisterClient", mock.Anything, mock.Anything).Return(nil).Maybe()
	mocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, *domainAddress).Maybe().Return(mocks.domainSmartContract, nil)
	//It is not valid to reference LateBound components before PostInit
	mocks.allComponents.On("IdentityResolver").Return(mocks.identityResolver).Maybe()
	err := e.PostInit(mocks.allComponents)
	assert.NoError(t, err)
	return e, mocks, nodeID

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
