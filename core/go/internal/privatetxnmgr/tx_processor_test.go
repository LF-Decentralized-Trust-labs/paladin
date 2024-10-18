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
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/mocks/privatetxnmgrmocks"
	"github.com/kaleido-io/paladin/core/mocks/prvtxsyncpointsmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/signpayloads"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type transactionProcessorDepencyMocks struct {
	allComponents       *componentmocks.AllComponents
	domainSmartContract *componentmocks.DomainSmartContract
	domainContext       *componentmocks.DomainContext
	domainMgr           *componentmocks.DomainManager
	transportManager    *componentmocks.TransportManager
	stateStore          *componentmocks.StateManager
	keyManager          *componentmocks.KeyManager
	sequencer           *privatetxnmgrmocks.Sequencer
	endorsementGatherer *privatetxnmgrmocks.EndorsementGatherer
	publisher           *privatetxnmgrmocks.Publisher
	identityResolver    *componentmocks.IdentityResolver
	syncPoints          *prvtxsyncpointsmocks.SyncPoints
	transportWriter     *privatetxnmgrmocks.TransportWriter
}

func newPaladinTransactionProcessorForTesting(t *testing.T, ctx context.Context, transaction *components.PrivateTransaction) (*PaladinTxProcessor, *transactionProcessorDepencyMocks) {

	mocks := &transactionProcessorDepencyMocks{
		allComponents:       componentmocks.NewAllComponents(t),
		domainSmartContract: componentmocks.NewDomainSmartContract(t),
		domainContext:       componentmocks.NewDomainContext(t),
		domainMgr:           componentmocks.NewDomainManager(t),
		transportManager:    componentmocks.NewTransportManager(t),
		stateStore:          componentmocks.NewStateManager(t),
		keyManager:          componentmocks.NewKeyManager(t),
		sequencer:           privatetxnmgrmocks.NewSequencer(t),
		endorsementGatherer: privatetxnmgrmocks.NewEndorsementGatherer(t),
		publisher:           privatetxnmgrmocks.NewPublisher(t),
		identityResolver:    componentmocks.NewIdentityResolver(t),
		syncPoints:          prvtxsyncpointsmocks.NewSyncPoints(t),
		transportWriter:     privatetxnmgrmocks.NewTransportWriter(t),
	}
	contractAddress := tktypes.RandAddress()
	mocks.allComponents.On("StateManager").Return(mocks.stateStore).Maybe()
	mocks.allComponents.On("DomainManager").Return(mocks.domainMgr).Maybe()
	mocks.allComponents.On("TransportManager").Return(mocks.transportManager).Maybe()
	mocks.allComponents.On("KeyManager").Return(mocks.keyManager).Maybe()
	mocks.endorsementGatherer.On("DomainContext").Return(mocks.domainContext).Maybe()
	mocks.domainSmartContract.On("Address").Return(*contractAddress).Maybe()

	tp := NewPaladinTransactionProcessor(ctx, transaction, tktypes.RandHex(16), mocks.allComponents, mocks.domainSmartContract, mocks.sequencer, mocks.publisher, mocks.endorsementGatherer, mocks.identityResolver, mocks.syncPoints, mocks.transportWriter)

	return tp.(*PaladinTxProcessor), mocks
}

func TestTransactionProcessorHandleTransactionSubmittedEvent(t *testing.T) {
	ctx := context.Background()
	newTxID := uuid.New()
	testTx := &components.PrivateTransaction{
		ID:          newTxID,
		PreAssembly: &components.TransactionPreAssembly{},
	}
	tp, dependencyMocks := newPaladinTransactionProcessorForTesting(t, ctx, testTx)
	dependencyMocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)

		tx.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
		}
	}).Return(nil).Once()
	dependencyMocks.sequencer.On("HandleTransactionAssembledEvent", mock.Anything, mock.Anything).Return(nil).Once()
	dependencyMocks.sequencer.On("AssignTransaction", mock.Anything, newTxID.String()).Return(nil).Once()

	err := tp.HandleTransactionSubmittedEvent(ctx, &ptmgrtypes.TransactionSubmittedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID: newTxID.String(),
		},
	})
	assert.NoError(t, err)
}

func TestTransactionProcessorAssembleRevert(t *testing.T) {
	ctx := context.Background()
	newTxID := uuid.New()
	testTx := &components.PrivateTransaction{
		ID: newTxID,
		PreAssembly: &components.TransactionPreAssembly{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{}, // empty list so that we are immediately ready to assemble
		},
	}
	tp, dependencyMocks := newPaladinTransactionProcessorForTesting(t, ctx, testTx)

	dependencyMocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)

		tx.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
		}
	}).Return(nil).Once()

	dependencyMocks.syncPoints.On("QueueTransactionFinalize", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	err := tp.HandleTransactionSubmittedEvent(ctx, &ptmgrtypes.TransactionSubmittedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID: newTxID.String(),
		},
	})
	assert.NoError(t, err) //the event was successfully handled and part of that handling was to revert the transaction so there is no error

}

func TestTransactionProcessorAssembleError(t *testing.T) {
	ctx := context.Background()
	newTxID := uuid.New()
	testTx := &components.PrivateTransaction{
		ID: newTxID,
		PreAssembly: &components.TransactionPreAssembly{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{}, // empty list so that we are immediately ready to assemble
		},
	}

	tp, dependencyMocks := newPaladinTransactionProcessorForTesting(t, ctx, testTx)

	testRevertReason := "test revert reason"
	dependencyMocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)

		tx.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
		}
	}).Return(errors.New(testRevertReason)).Once()

	matchRevertReason := func(s string) bool {
		re := regexp.MustCompile(".*" + regexp.QuoteMeta(testRevertReason) + ".*")
		return re.MatchString(s)
	}
	dependencyMocks.syncPoints.On("QueueTransactionFinalize", mock.Anything, mock.Anything, newTxID, mock.MatchedBy(matchRevertReason), mock.Anything, mock.Anything).Return()

	err := tp.HandleTransactionSubmittedEvent(ctx, &ptmgrtypes.TransactionSubmittedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID: newTxID.String(),
		},
	})
	assert.NoError(t, err) //the event was successfully handled and part of that handling was to revert the transaction so there is no error

}

func TestHasOutstandingEndorsementRequestsMultipleRequestsIncomplete(t *testing.T) {
	ctx := context.Background()
	newTxID := uuid.New()
	aliceIdentityLocator := "alice@node1"
	aliceVerifier := tktypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := tktypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := tktypes.RandAddress().String()
	testTx := &components.PrivateTransaction{
		ID: newTxID,
		PreAssembly: &components.TransactionPreAssembly{
			Verifiers: []*prototk.ResolvedVerifier{
				{
					Lookup:       aliceIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     aliceVerifier,
				},
				{
					Lookup:       bobIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     bobVerifier,
				},
				{
					Lookup:       carolIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     carolVerifier,
				},
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						aliceIdentityLocator,
					},
				},
				{
					Name:            "bar",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						bobIdentityLocator,
					},
				},
				{
					Name:            "quz",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						carolIdentityLocator,
					},
				},
			},
			Endorsements: []*prototk.AttestationResult{
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Verifier: &prototk.ResolvedVerifier{
						Lookup:       aliceIdentityLocator,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						Verifier:     tktypes.RandAddress().String(),
						VerifierType: verifiers.ETH_ADDRESS,
					},
					Payload: tktypes.RandBytes(32),
				},
			},
		},
	}

	tp, _ := newPaladinTransactionProcessorForTesting(t, ctx, testTx)
	result, err := tp.hasOutstandingEndorsementRequests(ctx)
	assert.NoError(t, err)
	assert.True(t, result)

}

func TestHasOutstandingEndorsementRequestsMultipleRequestsComplete(t *testing.T) {
	ctx := context.Background()
	newTxID := uuid.New()
	aliceIdentityLocator := "alice@node1"
	aliceVerifier := tktypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := tktypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := tktypes.RandAddress().String()
	testTx := &components.PrivateTransaction{
		ID: newTxID,
		PreAssembly: &components.TransactionPreAssembly{
			Verifiers: []*prototk.ResolvedVerifier{
				{
					Lookup:       aliceIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     aliceVerifier,
				},
				{
					Lookup:       bobIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     bobVerifier,
				},
				{
					Lookup:       carolIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     carolVerifier,
				},
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						aliceIdentityLocator,
					},
				},
				{
					Name:            "bar",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						bobIdentityLocator,
					},
				},
				{
					Name:            "quz",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						carolIdentityLocator,
					},
				},
			},
			Endorsements: []*prototk.AttestationResult{
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Verifier: &prototk.ResolvedVerifier{
						Lookup:       aliceIdentityLocator,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						Verifier:     aliceVerifier,
						VerifierType: verifiers.ETH_ADDRESS,
					},
					Payload: tktypes.RandBytes(32),
				},
				{
					Name:            "bar",
					AttestationType: prototk.AttestationType_ENDORSE,
					Verifier: &prototk.ResolvedVerifier{
						Lookup:       bobIdentityLocator,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						Verifier:     bobVerifier,
						VerifierType: verifiers.ETH_ADDRESS,
					},
					Payload: tktypes.RandBytes(32),
				},
				{
					Name:            "quz",
					AttestationType: prototk.AttestationType_ENDORSE,
					Verifier: &prototk.ResolvedVerifier{
						Lookup:       carolIdentityLocator,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						Verifier:     carolVerifier,
						VerifierType: verifiers.ETH_ADDRESS,
					},
					Payload: tktypes.RandBytes(32),
				},
			},
		},
	}

	tp, _ := newPaladinTransactionProcessorForTesting(t, ctx, testTx)
	result, err := tp.hasOutstandingEndorsementRequests(ctx)
	assert.NoError(t, err)
	assert.False(t, result)

}

func TestHasOutstandingEndorsementRequestSingleRequestMultiplePartiesIncomplete(t *testing.T) {
	ctx := context.Background()
	newTxID := uuid.New()
	aliceIdentityLocator := "alice@node1"
	aliceVerifier := tktypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := tktypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := tktypes.RandAddress().String()
	testTx := &components.PrivateTransaction{
		ID: newTxID,
		PreAssembly: &components.TransactionPreAssembly{
			Verifiers: []*prototk.ResolvedVerifier{
				{
					Lookup:       aliceIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     aliceVerifier,
				},
				{
					Lookup:       bobIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     bobVerifier,
				},
				{
					Lookup:       carolIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     carolVerifier,
				},
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						aliceIdentityLocator,
						bobIdentityLocator,
						carolIdentityLocator,
					},
				},
			},
			Endorsements: []*prototk.AttestationResult{
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Verifier: &prototk.ResolvedVerifier{
						Lookup:       aliceIdentityLocator,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						Verifier:     aliceVerifier,
						VerifierType: verifiers.ETH_ADDRESS,
					},
					Payload: tktypes.RandBytes(32),
				},
			},
		},
	}

	tp, _ := newPaladinTransactionProcessorForTesting(t, ctx, testTx)
	result, err := tp.hasOutstandingEndorsementRequests(ctx)
	assert.NoError(t, err)
	assert.True(t, result)

}

func TestHasOutstandingEndorsementRequestSingleRequestMultiplePartiesComplete(t *testing.T) {
	ctx := context.Background()
	newTxID := uuid.New()
	aliceIdentityLocator := "alice@node1"
	aliceVerifier := tktypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := tktypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := tktypes.RandAddress().String()
	testTx := &components.PrivateTransaction{
		ID: newTxID,
		PreAssembly: &components.TransactionPreAssembly{
			Verifiers: []*prototk.ResolvedVerifier{
				{
					Lookup:       aliceIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     aliceVerifier,
				},
				{
					Lookup:       bobIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     bobVerifier,
				},
				{
					Lookup:       carolIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     carolVerifier,
				},
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						aliceIdentityLocator,
						bobIdentityLocator,
						carolIdentityLocator,
					},
				},
			},
			Endorsements: []*prototk.AttestationResult{
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Verifier: &prototk.ResolvedVerifier{
						Lookup:       aliceIdentityLocator,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						Verifier:     aliceVerifier,
						VerifierType: verifiers.ETH_ADDRESS,
					},
					Payload: tktypes.RandBytes(32),
				},
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Verifier: &prototk.ResolvedVerifier{
						Lookup:       bobIdentityLocator,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						Verifier:     bobVerifier,
						VerifierType: verifiers.ETH_ADDRESS,
					},
					Payload: tktypes.RandBytes(32),
				},
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Verifier: &prototk.ResolvedVerifier{
						Lookup:       carolIdentityLocator,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						Verifier:     carolVerifier,
						VerifierType: verifiers.ETH_ADDRESS,
					},
					Payload: tktypes.RandBytes(32),
				},
			},
		},
	}

	tp, _ := newPaladinTransactionProcessorForTesting(t, ctx, testTx)
	result, err := tp.hasOutstandingEndorsementRequests(ctx)
	assert.NoError(t, err)
	assert.False(t, result)

}

func TestHasOutstandingEndorsementRequestSingleRequestMultiplePartiesDuplicate(t *testing.T) {
	// when we have the right number of endorsements but they are all for the same party that counts as incomplete
	ctx := context.Background()
	newTxID := uuid.New()
	aliceIdentityLocator := "alice@node1"
	aliceVerifier := tktypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := tktypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := tktypes.RandAddress().String()
	testTx := &components.PrivateTransaction{
		ID: newTxID,
		PreAssembly: &components.TransactionPreAssembly{
			Verifiers: []*prototk.ResolvedVerifier{
				{
					Lookup:       aliceIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     aliceVerifier,
				},
				{
					Lookup:       bobIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     bobVerifier,
				},
				{
					Lookup:       carolIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     carolVerifier,
				},
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						aliceIdentityLocator,
						bobIdentityLocator,
						carolIdentityLocator,
					},
				},
			},
			Endorsements: []*prototk.AttestationResult{
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Verifier: &prototk.ResolvedVerifier{
						Lookup:       aliceIdentityLocator,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						Verifier:     aliceVerifier,
						VerifierType: verifiers.ETH_ADDRESS,
					},
					Payload: tktypes.RandBytes(32),
				},
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Verifier: &prototk.ResolvedVerifier{
						Lookup:       aliceIdentityLocator,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						Verifier:     aliceVerifier,
						VerifierType: verifiers.ETH_ADDRESS,
					},
					Payload: tktypes.RandBytes(32),
				},
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Verifier: &prototk.ResolvedVerifier{
						Lookup:       aliceIdentityLocator,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						Verifier:     aliceVerifier,
						VerifierType: verifiers.ETH_ADDRESS,
					},
					Payload: tktypes.RandBytes(32),
				},
			},
		},
	}

	tp, _ := newPaladinTransactionProcessorForTesting(t, ctx, testTx)
	result, err := tp.hasOutstandingEndorsementRequests(ctx)
	assert.NoError(t, err)
	assert.True(t, result)

}

func TestHasOutstandingEndorsementRequestSingleRequestMultiplePartiesCompleteMixedOrder(t *testing.T) {
	// order of responses does not need to match order of requests
	ctx := context.Background()
	newTxID := uuid.New()
	aliceIdentityLocator := "alice@node1"
	aliceVerifier := tktypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := tktypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := tktypes.RandAddress().String()
	testTx := &components.PrivateTransaction{
		ID: newTxID,
		PreAssembly: &components.TransactionPreAssembly{
			Verifiers: []*prototk.ResolvedVerifier{
				{
					Lookup:       aliceIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     aliceVerifier,
				},
				{
					Lookup:       bobIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     bobVerifier,
				},
				{
					Lookup:       carolIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     carolVerifier,
				},
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						aliceIdentityLocator,
						bobIdentityLocator,
						carolIdentityLocator,
					},
				},
			},
			Endorsements: []*prototk.AttestationResult{
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Verifier: &prototk.ResolvedVerifier{
						Lookup:       carolIdentityLocator,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						Verifier:     carolVerifier,
						VerifierType: verifiers.ETH_ADDRESS,
					},
					Payload: tktypes.RandBytes(32),
				},
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Verifier: &prototk.ResolvedVerifier{
						Lookup:       aliceIdentityLocator,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						Verifier:     aliceVerifier,
						VerifierType: verifiers.ETH_ADDRESS,
					},
					Payload: tktypes.RandBytes(32),
				},
				{
					Name:            "foo",
					AttestationType: prototk.AttestationType_ENDORSE,
					Verifier: &prototk.ResolvedVerifier{
						Lookup:       bobIdentityLocator,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						Verifier:     bobVerifier,
						VerifierType: verifiers.ETH_ADDRESS,
					},
					Payload: tktypes.RandBytes(32),
				},
			},
		},
	}

	tp, _ := newPaladinTransactionProcessorForTesting(t, ctx, testTx)
	result, err := tp.hasOutstandingEndorsementRequests(ctx)
	assert.NoError(t, err)
	assert.False(t, result)

}

func TestHasOutstandingEndorsementRequestsNilPreAssembly(t *testing.T) {
	ctx := context.Background()
	newTxID := uuid.New()

	testTx := &components.PrivateTransaction{
		ID: newTxID,
	}
	tp, _ := newPaladinTransactionProcessorForTesting(t, ctx, testTx)
	_, err := tp.hasOutstandingEndorsementRequests(ctx)
	assert.Error(t, err)
}

func TestHasOutstandingEndorsementRequestsNilPostAssembly(t *testing.T) {
	ctx := context.Background()
	newTxID := uuid.New()
	aliceIdentityLocator := "alice@node1"
	aliceVerifier := tktypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := tktypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := tktypes.RandAddress().String()
	testTx := &components.PrivateTransaction{
		ID: newTxID,
		PreAssembly: &components.TransactionPreAssembly{
			Verifiers: []*prototk.ResolvedVerifier{
				{
					Lookup:       aliceIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     aliceVerifier,
				},
				{
					Lookup:       bobIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     bobVerifier,
				},
				{
					Lookup:       carolIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     carolVerifier,
				},
			},
		},
	}
	tp, _ := newPaladinTransactionProcessorForTesting(t, ctx, testTx)
	_, err := tp.hasOutstandingEndorsementRequests(ctx)
	assert.Error(t, err)
}
