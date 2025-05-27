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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/mocks/privatetxnmgrmocks"
	"github.com/kaleido-io/paladin/core/mocks/prvtxsyncpointsmocks"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/signpayloads"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type transactionFlowDepencyMocks struct {
	allComponents       *componentmocks.AllComponents
	domainSmartContract *componentmocks.DomainSmartContract
	domainContext       *componentmocks.DomainContext
	domainMgr           *componentmocks.DomainManager
	transportManager    *componentmocks.TransportManager
	stateStore          *componentmocks.StateManager
	keyManager          *componentmocks.KeyManager
	endorsementGatherer *privatetxnmgrmocks.EndorsementGatherer
	publisher           *privatetxnmgrmocks.Publisher
	identityResolver    *componentmocks.IdentityResolver
	syncPoints          *prvtxsyncpointsmocks.SyncPoints
	transportWriter     *privatetxnmgrmocks.TransportWriter
	environment         *privatetxnmgrmocks.SequencerEnvironment
	coordinatorSelector *privatetxnmgrmocks.CoordinatorSelector
	localAssembler      *privatetxnmgrmocks.LocalAssembler
}

func newTransactionFlowForTesting(t *testing.T, ctx context.Context, transaction *components.PrivateTransaction, nodeName string) (*transactionFlow, *transactionFlowDepencyMocks) {

	mocks := &transactionFlowDepencyMocks{
		allComponents:       componentmocks.NewAllComponents(t),
		domainSmartContract: componentmocks.NewDomainSmartContract(t),
		domainContext:       componentmocks.NewDomainContext(t),
		domainMgr:           componentmocks.NewDomainManager(t),
		transportManager:    componentmocks.NewTransportManager(t),
		stateStore:          componentmocks.NewStateManager(t),
		keyManager:          componentmocks.NewKeyManager(t),
		endorsementGatherer: privatetxnmgrmocks.NewEndorsementGatherer(t),
		publisher:           privatetxnmgrmocks.NewPublisher(t),
		identityResolver:    componentmocks.NewIdentityResolver(t),
		syncPoints:          prvtxsyncpointsmocks.NewSyncPoints(t),
		transportWriter:     privatetxnmgrmocks.NewTransportWriter(t),
		environment:         privatetxnmgrmocks.NewSequencerEnvironment(t),
		coordinatorSelector: privatetxnmgrmocks.NewCoordinatorSelector(t),
		localAssembler:      privatetxnmgrmocks.NewLocalAssembler(t),
	}
	contractAddress := pldtypes.RandAddress()
	mocks.allComponents.On("StateManager").Return(mocks.stateStore).Maybe()
	mocks.allComponents.On("DomainManager").Return(mocks.domainMgr).Maybe()
	mocks.allComponents.On("TransportManager").Return(mocks.transportManager).Maybe()
	mocks.allComponents.On("KeyManager").Return(mocks.keyManager).Maybe()
	mocks.endorsementGatherer.On("DomainContext").Return(mocks.domainContext).Maybe()
	mocks.domainSmartContract.On("Address").Return(*contractAddress).Maybe()
	mocks.domainSmartContract.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	}).Maybe()

	domain := componentmocks.NewDomain(t)
	domain.On("Configuration").Return(&prototk.DomainConfig{}).Maybe()
	mocks.domainSmartContract.On("Domain").Return(domain).Maybe()

	assembleCoordinator := NewAssembleCoordinator(ctx, nodeName, 1, mocks.allComponents, mocks.domainSmartContract, mocks.domainContext, mocks.transportWriter, *contractAddress, mocks.environment, 1*time.Second, mocks.localAssembler)

	tp := NewTransactionFlow(ctx, transaction, nodeName, mocks.allComponents, mocks.domainSmartContract, mocks.domainContext, mocks.publisher, mocks.endorsementGatherer, mocks.identityResolver, mocks.syncPoints, mocks.transportWriter, 1*time.Minute, mocks.coordinatorSelector, assembleCoordinator, mocks.environment)

	return tp.(*transactionFlow), mocks
}

func TestHasOutstandingEndorsementRequestsMultipleRequestsIncomplete(t *testing.T) {
	// When there is an attestation plan with multiple AttestationRequest
	// but not enough AttestationResult
	// then returns true
	ctx := context.Background()
	newTxID := uuid.New()
	aliceIdentityLocator := "alice@node1"
	aliceVerifier := pldtypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := pldtypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := pldtypes.RandAddress().String()

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
						Verifier:     pldtypes.RandAddress().String(),
						VerifierType: verifiers.ETH_ADDRESS,
					},
					Payload: pldtypes.RandBytes(32),
				},
			},
		},
	}

	tp, _ := newTransactionFlowForTesting(t, ctx, testTx, "node1")
	result := tp.hasOutstandingEndorsementRequests(ctx)
	assert.True(t, result)

}

func TestHasOutstandingEndorsementRequestsMultipleRequestsComplete(t *testing.T) {
	// When there is an attestation plan with multiple AttestationRequest
	// and we have an AttestationResult matching each one
	// then returns false
	ctx := context.Background()
	newTxID := uuid.New()
	aliceIdentityLocator := "alice@node1"
	aliceVerifier := pldtypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := pldtypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := pldtypes.RandAddress().String()
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
					Payload: pldtypes.RandBytes(32),
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
					Payload: pldtypes.RandBytes(32),
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
					Payload: pldtypes.RandBytes(32),
				},
			},
		},
	}

	tp, _ := newTransactionFlowForTesting(t, ctx, testTx, "node1")
	result := tp.hasOutstandingEndorsementRequests(ctx)
	assert.False(t, result)

}

func TestHasOutstandingEndorsementRequestSingleRequestMultiplePartiesIncomplete(t *testing.T) {
	// When there is an attestation plan with a single AttestationRequest listing multiple parties
	// and we dont have all of the matching AttestationResults
	// then returns true

	ctx := context.Background()
	newTxID := uuid.New()
	aliceIdentityLocator := "alice@node1"
	aliceVerifier := pldtypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := pldtypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := pldtypes.RandAddress().String()
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
					Payload: pldtypes.RandBytes(32),
				},
			},
		},
	}

	tp, _ := newTransactionFlowForTesting(t, ctx, testTx, "node1")
	result := tp.hasOutstandingEndorsementRequests(ctx)
	assert.True(t, result)

}

func TestHasOutstandingEndorsementRequestSingleRequestMultiplePartiesComplete(t *testing.T) {
	// When there is an attestation plan with a single AttestationRequest listing multiple parties
	// and we have an AttestationResult matching each one
	// then returns false
	ctx := context.Background()
	newTxID := uuid.New()
	aliceIdentityLocator := "alice@node1"
	aliceVerifier := pldtypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := pldtypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := pldtypes.RandAddress().String()
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
					Payload: pldtypes.RandBytes(32),
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
					Payload: pldtypes.RandBytes(32),
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
					Payload: pldtypes.RandBytes(32),
				},
			},
		},
	}

	tp, _ := newTransactionFlowForTesting(t, ctx, testTx, "node1")
	result := tp.hasOutstandingEndorsementRequests(ctx)
	assert.False(t, result)

}

func TestHasOutstandingEndorsementRequestSingleRequestMultiplePartiesDuplicate(t *testing.T) {
	// when we have the right number of endorsements but they are all for the same party that counts as incomplete
	ctx := context.Background()
	newTxID := uuid.New()
	aliceIdentityLocator := "alice@node1"
	aliceVerifier := pldtypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := pldtypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := pldtypes.RandAddress().String()
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
					Payload: pldtypes.RandBytes(32),
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
					Payload: pldtypes.RandBytes(32),
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
					Payload: pldtypes.RandBytes(32),
				},
			},
		},
	}

	tp, _ := newTransactionFlowForTesting(t, ctx, testTx, "node1")
	result := tp.hasOutstandingEndorsementRequests(ctx)
	assert.True(t, result)

}

func TestHasOutstandingEndorsementRequestSingleRequestMultiplePartiesCompleteMixedOrder(t *testing.T) {
	// order of responses does not need to match order of requests
	ctx := context.Background()
	newTxID := uuid.New()
	aliceIdentityLocator := "alice@node1"
	aliceVerifier := pldtypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := pldtypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := pldtypes.RandAddress().String()
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
					Payload: pldtypes.RandBytes(32),
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
					Payload: pldtypes.RandBytes(32),
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
					Payload: pldtypes.RandBytes(32),
				},
			},
		},
	}

	tp, _ := newTransactionFlowForTesting(t, ctx, testTx, "node1")
	result := tp.hasOutstandingEndorsementRequests(ctx)
	assert.False(t, result)
}

func TestRequestRemoteEndorsements(t *testing.T) {
	ctx := context.Background()
	newTxID := uuid.New()

	aliceIdentityLocator := "alice@node1"
	aliceVerifier := pldtypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := pldtypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := pldtypes.RandAddress().String()

	testContractAddress := *pldtypes.RandAddress()
	// create a transaction as if we have already
	// - resolved the verifiers
	// - assembled it
	// - signed it
	// so next step is to request endorsements
	testTx := &components.PrivateTransaction{
		ID:      newTxID,
		Address: testContractAddress,
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From:          aliceIdentityLocator,
				TransactionId: newTxID.String(),
			},
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
		},
	}

	sendingNodeName := "sendingNode"

	tp, mocks := newTransactionFlowForTesting(t, ctx, testTx, sendingNodeName)
	mocks.coordinatorSelector.On("SelectCoordinatorNode", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), sendingNodeName, nil)
	mocks.transportWriter.On("SendEndorsementRequest",
		mock.Anything,
		mock.Anything,
		"alice@node1",
		"node1",
		testContractAddress.String(),
		newTxID.String(),
		mock.Anything, //attRequest
		mock.Anything, //TransactionSpecification,
		mock.Anything, //Verifiers,
		mock.Anything, //Signatures,
		mock.Anything, //InputStates,
		mock.Anything, //OutputStates,
		mock.Anything, //InfoStates,
	).Return(nil).Once()
	mocks.transportWriter.On("SendEndorsementRequest",
		mock.Anything,
		mock.Anything,
		"bob@node2",
		"node2",
		testContractAddress.String(),
		newTxID.String(),
		mock.Anything, //attRequest
		mock.Anything, //TransactionSpecification,
		mock.Anything, //Verifiers,
		mock.Anything, //Signatures,
		mock.Anything, //InputStates,
		mock.Anything, //OutputStates,
		mock.Anything, //InfoStates,
	).Return(nil).Once()
	mocks.transportWriter.On("SendEndorsementRequest",
		mock.Anything,
		mock.Anything,
		"carol@node2",
		"node2",
		testContractAddress.String(),
		newTxID.String(),
		mock.Anything, //attRequest
		mock.Anything, //TransactionSpecification,
		mock.Anything, //Verifiers,
		mock.Anything, //Signatures,
		mock.Anything, //InputStates,
		mock.Anything, //OutputStates,
		mock.Anything, //InfoStates,
	).Return(nil).Once()
	tp.Action(ctx)

	mocks.transportWriter.AssertExpectations(t)

	//Check that we don't send the same requests again (we specified Once in the mocks above)
	tp.Action(ctx)

}

func TestRequestLocalEndorsements(t *testing.T) {
	ctx := context.Background()
	newTxID := uuid.New()
	//endorsers are on the same node as the sender
	sendingNodeName := "sendingNode"

	aliceIdentityLocator := "alice@" + sendingNodeName
	aliceVerifier := pldtypes.RandAddress().String()
	bobIdentityLocator := "bob@" + sendingNodeName
	bobVerifier := pldtypes.RandAddress().String()
	carolIdentityLocator := "carol@" + sendingNodeName
	carolVerifier := pldtypes.RandAddress().String()

	testContractAddress := *pldtypes.RandAddress()
	// create a transaction as if we have already
	// - resolved the verifiers
	// - assembled it
	// - signed it
	// so next step is to request endorsements
	testTx := &components.PrivateTransaction{
		ID:      newTxID,
		Address: testContractAddress,
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From:          aliceIdentityLocator,
				TransactionId: newTxID.String(),
			},
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
		},
	}

	tp, mocks := newTransactionFlowForTesting(t, ctx, testTx, sendingNodeName)
	mocks.coordinatorSelector.On("SelectCoordinatorNode", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), sendingNodeName, nil)
	mocks.endorsementGatherer.On("GatherEndorsement",
		mock.Anything, // context
		mock.Anything, //TransactionSpecification,
		mock.Anything, //Verifiers,
		mock.Anything, //Signatures,
		mock.Anything, //InputStates,
		mock.Anything, //ReadStates,
		mock.Anything, //OutputStates,
		mock.Anything, //InfoStates,
		"alice@"+sendingNodeName,
		mock.Anything, //attRequest
	).Return(
		&prototk.AttestationResult{
			Name: "foo",
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       aliceIdentityLocator,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				Verifier:     aliceVerifier,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
		nil,
		nil,
	).Once()
	mocks.endorsementGatherer.On("GatherEndorsement",
		mock.Anything, // context
		mock.Anything, //TransactionSpecification,
		mock.Anything, //Verifiers,
		mock.Anything, //Signatures,
		mock.Anything, //InputStates,
		mock.Anything, //ReadStates,
		mock.Anything, //OutputStates,
		mock.Anything, //InfoStates,
		"bob@"+sendingNodeName,
		mock.Anything, //attRequest
	).Return(
		&prototk.AttestationResult{
			Name: "foo",
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       aliceIdentityLocator,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				Verifier:     aliceVerifier,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
		nil,
		nil,
	).Once()
	mocks.endorsementGatherer.On("GatherEndorsement",
		mock.Anything, // context
		mock.Anything, //TransactionSpecification,
		mock.Anything, //Verifiers,
		mock.Anything, //Signatures,
		mock.Anything, //InputStates,
		mock.Anything, //ReadStates,
		mock.Anything, //OutputStates,
		mock.Anything, //InfoStates,
		"carol@"+sendingNodeName,
		mock.Anything, //attRequest
	).Return(
		&prototk.AttestationResult{
			Name: "foo",
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       aliceIdentityLocator,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				Verifier:     aliceVerifier,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
		nil,
		nil,
	).Once()

	mocks.publisher.On("PublishTransactionEndorsedEvent", mock.Anything, newTxID.String(), mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Times(3)
	tp.Action(ctx)

	mocks.transportWriter.AssertExpectations(t)

	//Check that we don't send the same requests again (we specified Once in the mocks above)
	tp.Action(ctx)

}

func TestTimedOutEndorsementRequest(t *testing.T) {
	// This can happen if the remote node is slow to respond
	//or the network is unreliable and the request or the response has been lost
	// we don't necessarily know which of those is the case so we just treat it as a timeout
	// this could / should be tested as part of an E2E reliability test but
	// we like to get confidence at a white box level because there is no guarantee
	// that the E2E test will be able to reproduce the conditions that cause the timeout

	ctx := context.Background()
	newTxID := uuid.New()

	aliceIdentityLocator := "alice@node1"
	aliceVerifier := pldtypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := pldtypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := pldtypes.RandAddress().String()

	testContractAddress := *pldtypes.RandAddress()
	// create a transaction as if we have already
	// - resolved the verifiers
	// - assembled it
	// - signed it
	// so next step is to request endorsements
	testTx := &components.PrivateTransaction{
		ID:      newTxID,
		Address: testContractAddress,
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From:          aliceIdentityLocator,
				TransactionId: newTxID.String(),
			},
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
						bobIdentityLocator,
						carolIdentityLocator,
					},
				},
			},
		},
	}

	tp, mocks := newTransactionFlowForTesting(t, ctx, testTx, "node1")
	mocks.coordinatorSelector.On("SelectCoordinatorNode", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), "node1", nil)

	fakeClock := &fakeClock{timePassed: 0}
	tp.clock = fakeClock

	expectEndorsementRequest := func(idempotencyKey *string, party, node string) {
		//set the idempotency key received to the pointer provided
		mocks.transportWriter.On("SendEndorsementRequest",
			mock.Anything,
			mock.Anything,
			party,
			node,
			testContractAddress.String(),
			newTxID.String(),
			mock.Anything, //attRequest
			mock.Anything, //TransactionSpecification,
			mock.Anything, //Verifiers,
			mock.Anything, //Signatures,
			mock.Anything, //InputStates,
			mock.Anything, //OutputStates,
			mock.Anything, //InfoStates,
		).Return(nil).Once().Run(func(args mock.Arguments) {
			if idempotencyKey != nil {
				receivedIdempotencyKey := args.Get(1).(string)
				*idempotencyKey = receivedIdempotencyKey
			}
		})
	}

	expectIdempotentEndorsementRequest := func(idempotencyKey, party, node string) {
		//asserts the given idempotency key is used in the request
		mocks.transportWriter.On("SendEndorsementRequest",
			mock.Anything,
			idempotencyKey,
			party,
			node,
			testContractAddress.String(),
			newTxID.String(),
			mock.Anything, //attRequest
			mock.Anything, //TransactionSpecification,
			mock.Anything, //Verifiers,
			mock.Anything, //Signatures,
			mock.Anything, //InputStates,
			mock.Anything, //OutputStates,
			mock.Anything, //InfoStates,
		).Return(nil).Once()
	}

	idempotencyKeyBob := ""
	idempotencyKeyCarol := ""

	expectEndorsementRequest(&idempotencyKeyBob, "bob@node2", "node2")
	expectEndorsementRequest(&idempotencyKeyCarol, "carol@node2", "node2")
	tp.Action(ctx)
	mocks.transportWriter.AssertExpectations(t)

	//Check that we don't send the same requests again because it hasn't timeout out yet
	tp.Action(ctx)

	//simulate the passing of time
	fakeClock.timePassed = 1*time.Minute + 1*time.Second
	expectIdempotentEndorsementRequest(idempotencyKeyBob, "bob@node2", "node2")
	expectIdempotentEndorsementRequest(idempotencyKeyCarol, "carol@node2", "node2")
	tp.Action(ctx)
	mocks.transportWriter.AssertExpectations(t)

	//Check that we don't send the same requests again because it hasn't timeout out yet
	tp.Action(ctx)

	//Receive response from bob
	tp.applyTransactionEndorsedEvent(ctx, &ptmgrtypes.TransactionEndorsedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID:   newTxID.String(),
			ContractAddress: testContractAddress.String(),
		},
		Endorsement: &prototk.AttestationResult{
			Name: "foo",
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       bobIdentityLocator,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				Verifier:     bobVerifier,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
		Party:                  bobIdentityLocator,
		AttestationRequestName: "foo",
		IdempotencyKey:         idempotencyKeyBob,
	})

	//simulate the passing of time
	fakeClock.timePassed = fakeClock.timePassed + 1*time.Minute + 1*time.Second
	expectIdempotentEndorsementRequest(idempotencyKeyCarol, "carol@node2", "node2")
	tp.Action(ctx)
}

func TestEndorsementResponseAfterRevert(t *testing.T) {
	// We send out 2 endorsement requests , the first one back causes a revert
	// the second one back should be ignored

	ctx := context.Background()
	newTxID := uuid.New()

	aliceIdentityLocator := "alice@node1"
	aliceVerifier := pldtypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := pldtypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := pldtypes.RandAddress().String()

	testContractAddress := *pldtypes.RandAddress()
	// create a transaction as if we have already
	// - resolved the verifiers
	// - assembled it
	// - signed it
	// so next step is to request endorsements
	testTx := &components.PrivateTransaction{
		ID:      newTxID,
		Address: testContractAddress,
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From:          aliceIdentityLocator,
				TransactionId: newTxID.String(),
			},
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
						bobIdentityLocator,
						carolIdentityLocator,
					},
				},
			},
		},
	}

	tp, mocks := newTransactionFlowForTesting(t, ctx, testTx, "node1")
	mocks.coordinatorSelector.On("SelectCoordinatorNode", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), "node1", nil)

	fakeClock := &fakeClock{timePassed: 0}
	tp.clock = fakeClock

	expectEndorsementRequest := func(idempotencyKey *string, party, node string) {
		mocks.transportWriter.On("SendEndorsementRequest",
			mock.Anything,
			mock.Anything, //idempotency key
			party,
			node,
			testContractAddress.String(),
			newTxID.String(),
			mock.Anything, //attRequest
			mock.Anything, //TransactionSpecification,
			mock.Anything, //Verifiers,
			mock.Anything, //Signatures,
			mock.Anything, //InputStates,
			mock.Anything, //OutputStates,
			mock.Anything, //InfoStates,
		).Return(nil).Once().Run(func(args mock.Arguments) {
			if idempotencyKey != nil {
				receivedIdempotencyKey := args.Get(1).(string)
				*idempotencyKey = receivedIdempotencyKey
			}
		})
	}

	bobIdempotencyKey := ""
	carolIdempotencyKey := ""
	expectEndorsementRequest(&bobIdempotencyKey, "bob@node2", "node2")
	expectEndorsementRequest(&carolIdempotencyKey, "carol@node2", "node2")
	tp.Action(ctx)
	mocks.transportWriter.AssertExpectations(t)

	//Receive revert response from bob
	tp.applyTransactionEndorsedEvent(ctx, &ptmgrtypes.TransactionEndorsedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID:   newTxID.String(),
			ContractAddress: testContractAddress.String(),
		},
		Party:                  bobIdentityLocator,
		AttestationRequestName: "foo",
		IdempotencyKey:         bobIdempotencyKey,
		RevertReason:           confutil.P("bob refused to endorse"),
	})

	tp.Action(ctx)

	//Receive successful response from carol
	tp.applyTransactionEndorsedEvent(ctx, &ptmgrtypes.TransactionEndorsedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID:   newTxID.String(),
			ContractAddress: testContractAddress.String(),
		},
		Endorsement: &prototk.AttestationResult{
			Name: "foo",
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       carolIdentityLocator,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				Verifier:     carolVerifier,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
		Party:                  carolIdentityLocator,
		AttestationRequestName: "foo",
		IdempotencyKey:         carolIdempotencyKey,
	})

	//transaction should be marked for reassemble
	assert.False(t, tp.ReadyForSequencing(ctx))

}

func TestEndorsementResponseAfterReassemble(t *testing.T) {
	// Similar to TestEndorsementResponseAfterRevert:
	//  We send out 2 endorsement requests , the first one back causes a revert
	//  the second one back should be ignored
	// however, in this case, we manage to reassemble the transaction and send out the endorsement request again
	// before the second endorsement response comes back
	// it should still be ignored because it has been made obsolete by the reassemble and a new endorsement request to that same party
	ctx := context.Background()
	newTxID := uuid.New()

	aliceIdentityLocator := "alice@node1"
	aliceVerifier := pldtypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := pldtypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := pldtypes.RandAddress().String()

	testContractAddress := *pldtypes.RandAddress()
	// create a transaction as if we have already
	// - resolved the verifiers
	// - assembled it
	// - signed it
	// so next step is to request endorsements
	payloadFromAssemble1 := pldtypes.RandBytes(32)
	testTx := &components.PrivateTransaction{
		ID:      newTxID,
		Address: testContractAddress,
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From:          aliceIdentityLocator,
				TransactionId: newTxID.String(),
			},
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
					Payload:         payloadFromAssemble1,
					Parties: []string{
						bobIdentityLocator,
						carolIdentityLocator,
					},
				},
			},
		},
	}

	tp, mocks := newTransactionFlowForTesting(t, ctx, testTx, "node1")
	mocks.coordinatorSelector.On("SelectCoordinatorNode", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), "node1", nil)

	fakeClock := &fakeClock{timePassed: 0}
	tp.clock = fakeClock

	expectEndorsementRequest := func(idempotencyKey *string, party, node string) {
		mocks.transportWriter.On("SendEndorsementRequest",
			mock.Anything,
			mock.Anything, //idempotency key
			party,
			node,
			testContractAddress.String(),
			newTxID.String(),
			mock.Anything, //attRequest
			mock.Anything, //TransactionSpecification,
			mock.Anything, //Verifiers,
			mock.Anything, //Signatures,
			mock.Anything, //InputStates,
			mock.Anything, //OutputStates,
			mock.Anything, //InfoStates,
		).Return(nil).Once().Run(func(args mock.Arguments) {
			if idempotencyKey != nil {
				receivedIdempotencyKey := args.Get(1).(string)
				*idempotencyKey = receivedIdempotencyKey
			}
		})
	}

	bobIdempotencyKey := ""
	carolIdempotencyKey := ""
	expectEndorsementRequest(&bobIdempotencyKey, "bob@node2", "node2")
	expectEndorsementRequest(&carolIdempotencyKey, "carol@node2", "node2")
	tp.Action(ctx)
	mocks.transportWriter.AssertExpectations(t)

	//Receive revert response from bob
	tp.applyTransactionEndorsedEvent(ctx, &ptmgrtypes.TransactionEndorsedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID:   newTxID.String(),
			ContractAddress: testContractAddress.String(),
		},
		Party:                  bobIdentityLocator,
		AttestationRequestName: "foo",
		RevertReason:           confutil.P("bob refused to endorse"),
		IdempotencyKey:         bobIdempotencyKey,
	})

	tp.Action(ctx)

	//re-assemble the transaction
	payloadFromAssemble2 := pldtypes.RandBytes(32)

	tp.transaction.PostAssembly = &components.TransactionPostAssembly{
		AttestationPlan: []*prototk.AttestationRequest{
			{
				Name:            "foo",
				AttestationType: prototk.AttestationType_ENDORSE,
				Algorithm:       algorithms.ECDSA_SECP256K1,
				VerifierType:    verifiers.ETH_ADDRESS,
				PayloadType:     signpayloads.OPAQUE_TO_RSV,
				Payload:         payloadFromAssemble2,
				Parties: []string{
					bobIdentityLocator,
					carolIdentityLocator,
				},
			},
		},
	}
	bobIdempotencyKey2 := ""
	carolIdempotencyKey2 := ""
	expectEndorsementRequest(&bobIdempotencyKey2, "bob@node2", "node2")
	expectEndorsementRequest(&carolIdempotencyKey2, "carol@node2", "node2")
	tp.Action(ctx)

	//Receive late successful response from carol
	tp.applyTransactionEndorsedEvent(ctx, &ptmgrtypes.TransactionEndorsedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID:   newTxID.String(),
			ContractAddress: testContractAddress.String(),
		},
		Endorsement: &prototk.AttestationResult{
			Name: "foo",
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       carolIdentityLocator,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				Verifier:     carolVerifier,
				VerifierType: verifiers.ETH_ADDRESS,
			},
			PayloadType: confutil.P(signpayloads.OPAQUE_TO_RSV),
			Payload:     payloadFromAssemble1,
		},
		Party:                  carolIdentityLocator,
		AttestationRequestName: "foo",
		IdempotencyKey:         carolIdempotencyKey,
	})

	// should still have 2 outstanding endorsement requests
	assert.Len(t, tp.outstandingEndorsementRequests(ctx), 2)

}

func TestDuplicateEndorsementResponse(t *testing.T) {
	// we effectively have an at least once delivery guarantee on the endorsement requests and responses
	// so we need to be able to handle duplicate responses
	ctx := context.Background()
	newTxID := uuid.New()

	senderNodeName := "senderNode"
	senderIdentityLocator := "sender@" + senderNodeName
	aliceIdentityLocator := "alice@node1"
	aliceVerifier := pldtypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := pldtypes.RandAddress().String()
	carolIdentityLocator := "carol@node2"
	carolVerifier := pldtypes.RandAddress().String()

	testContractAddress := *pldtypes.RandAddress()
	// create a transaction as if we have already:
	// - resolved the verifiers
	// - assembled it
	// - signed it
	// so next step is to request endorsements
	testTx := &components.PrivateTransaction{
		ID:      newTxID,
		Address: testContractAddress,
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From:          senderIdentityLocator,
				TransactionId: newTxID.String(),
			},
			Verifiers: []*prototk.ResolvedVerifier{
				{
					Lookup:       senderIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     aliceVerifier,
				},
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
		},
	}

	tp, mocks := newTransactionFlowForTesting(t, ctx, testTx, senderNodeName)
	mocks.coordinatorSelector.On("SelectCoordinatorNode", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), senderNodeName, nil)

	fakeClock := &fakeClock{timePassed: 0}
	tp.clock = fakeClock

	expectEndorsementRequest := func(idempotencyKey *string, party, node string) {
		mocks.transportWriter.On("SendEndorsementRequest",
			mock.Anything,
			mock.Anything, //idempotency key
			party,
			node,
			testContractAddress.String(),
			newTxID.String(),
			mock.Anything, //attRequest
			mock.Anything, //TransactionSpecification,
			mock.Anything, //Verifiers,
			mock.Anything, //Signatures,
			mock.Anything, //InputStates,
			mock.Anything, //OutputStates,
			mock.Anything, //InfoStates,
		).Return(nil).Once().Run(func(args mock.Arguments) {
			if idempotencyKey != nil {
				receivedIdempotencyKey := args.Get(1).(string)
				*idempotencyKey = receivedIdempotencyKey
			}
		})
	}

	expectIdempotentEndorsementRequest := func(idempotencyKey, party, node string) {
		//asserts the given idempotency key is used in the request
		mocks.transportWriter.On("SendEndorsementRequest",
			mock.Anything,
			idempotencyKey,
			party,
			node,
			testContractAddress.String(),
			newTxID.String(),
			mock.Anything, //attRequest
			mock.Anything, //TransactionSpecification,
			mock.Anything, //Verifiers,
			mock.Anything, //Signatures,
			mock.Anything, //InputStates,
			mock.Anything, //OutputStates,
			mock.Anything, //InfoStates,
		).Return(nil).Once()
	}

	aliceIdempotencyKey := ""
	bobIdempotencyKey := ""
	carolIdempotencyKey := ""
	expectEndorsementRequest(&aliceIdempotencyKey, "alice@node1", "node1")
	expectEndorsementRequest(&bobIdempotencyKey, "bob@node2", "node2")
	expectEndorsementRequest(&carolIdempotencyKey, "carol@node2", "node2")
	tp.Action(ctx)
	mocks.transportWriter.AssertExpectations(t)

	//Receive response from alice
	tp.applyTransactionEndorsedEvent(ctx, &ptmgrtypes.TransactionEndorsedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID:   newTxID.String(),
			ContractAddress: testContractAddress.String(),
		},
		Endorsement: &prototk.AttestationResult{
			Name: "foo",
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       aliceIdentityLocator,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				Verifier:     aliceVerifier,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
		Party:                  aliceIdentityLocator,
		AttestationRequestName: "foo",
		IdempotencyKey:         aliceIdempotencyKey,
	})

	//simulate the passing of time
	fakeClock.timePassed = 1*time.Minute + 1*time.Second
	expectIdempotentEndorsementRequest(bobIdempotencyKey, "bob@node2", "node2")
	expectIdempotentEndorsementRequest(carolIdempotencyKey, "carol@node2", "node2")
	tp.Action(ctx)
	mocks.transportWriter.AssertExpectations(t)

	//Receive both responses from carol
	tp.applyTransactionEndorsedEvent(ctx, &ptmgrtypes.TransactionEndorsedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID:   newTxID.String(),
			ContractAddress: testContractAddress.String(),
		},
		Endorsement: &prototk.AttestationResult{
			Name: "foo",
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       carolIdentityLocator,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				Verifier:     carolVerifier,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
		Party:                  carolIdentityLocator,
		AttestationRequestName: "foo",
		IdempotencyKey:         carolIdempotencyKey,
	})
	tp.applyTransactionEndorsedEvent(ctx, &ptmgrtypes.TransactionEndorsedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID:   newTxID.String(),
			ContractAddress: testContractAddress.String(),
		},
		Endorsement: &prototk.AttestationResult{
			Name: "foo",
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       carolIdentityLocator,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				Verifier:     carolVerifier,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
		Party:                  carolIdentityLocator,
		AttestationRequestName: "foo",
		IdempotencyKey:         carolIdempotencyKey,
	})

	// no further action because we are still waiting for a response from bob
	//even though we have received 3 responses, we are not ready for dispatch because 2 of them are duplicates
	tp.Action(ctx)
}

func TestGetTxStatusPendingEndorsements(t *testing.T) {
	ctx := context.Background()
	newTxID := uuid.New()

	senderNodeName := "senderNode"
	senderIdentityLocator := "sender@" + senderNodeName
	aliceIdentityLocator := "alice@node1"
	aliceVerifier := pldtypes.RandAddress().String()
	bobIdentityLocator := "bob@node2"
	bobVerifier := pldtypes.RandAddress().String()
	carolIdentityLocator := "carol@node3"
	carolVerifier := pldtypes.RandAddress().String()
	daveIdentityLocator := "dave@node4"
	daveVerifier := pldtypes.RandAddress().String()

	testContractAddress := *pldtypes.RandAddress()
	testTx := &components.PrivateTransaction{
		ID:      newTxID,
		Address: testContractAddress,
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From:          senderIdentityLocator,
				TransactionId: newTxID.String(),
			},
			Verifiers: []*prototk.ResolvedVerifier{
				{
					Lookup:       senderIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     aliceVerifier,
				},
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
				{
					Lookup:       daveIdentityLocator,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
					Verifier:     daveVerifier,
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
					},
				},
				{
					Name:            "bar",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					VerifierType:    verifiers.ETH_ADDRESS,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties: []string{
						carolIdentityLocator,
						daveIdentityLocator,
					},
				},
			},
		},
	}

	tp, mocks := newTransactionFlowForTesting(t, ctx, testTx, senderNodeName)
	mocks.coordinatorSelector.On("SelectCoordinatorNode", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), senderNodeName, nil)

	expectEndorsementRequest := func(idempotencyKey *string, party, node string) {
		mocks.transportWriter.On("SendEndorsementRequest",
			mock.Anything,
			mock.Anything, //idempotency key
			party,
			node,
			testContractAddress.String(),
			newTxID.String(),
			mock.Anything, //attRequest
			mock.Anything, //TransactionSpecification,
			mock.Anything, //Verifiers,
			mock.Anything, //Signatures,
			mock.Anything, //InputStates,
			mock.Anything, //OutputStates,
			mock.Anything, //InfoStates,
		).Return(nil).Once().Run(func(args mock.Arguments) {
			if idempotencyKey != nil {
				receivedIdempotencyKey := args.Get(1).(string)
				*idempotencyKey = receivedIdempotencyKey
			}
		})
	}
	aliceIdempotencyKey := ""
	bobIdempotencyKey := ""
	carolIdempotencyKey := ""
	daveIdempotencyKey := ""
	expectEndorsementRequest(&aliceIdempotencyKey, "alice@node1", "node1")
	expectEndorsementRequest(&bobIdempotencyKey, "bob@node2", "node2")
	expectEndorsementRequest(&carolIdempotencyKey, "carol@node3", "node3")
	expectEndorsementRequest(&daveIdempotencyKey, "dave@node4", "node4")

	tp.Action(ctx)
	tp.applyTransactionEndorsedEvent(ctx, &ptmgrtypes.TransactionEndorsedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID:   newTxID.String(),
			ContractAddress: testContractAddress.String(),
		},
		IdempotencyKey:         aliceIdempotencyKey,
		Party:                  aliceIdentityLocator,
		AttestationRequestName: "foo",
		Endorsement: &prototk.AttestationResult{
			Name: "foo",
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       aliceIdentityLocator,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				Verifier:     pldtypes.RandAddress().String(),
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
	})
	tp.applyTransactionEndorsedEvent(ctx, &ptmgrtypes.TransactionEndorsedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID:   newTxID.String(),
			ContractAddress: testContractAddress.String(),
		},
		IdempotencyKey:         daveIdempotencyKey,
		Party:                  daveIdentityLocator,
		AttestationRequestName: "bar",
		Endorsement: &prototk.AttestationResult{
			Name: "bar",
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       daveIdentityLocator,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				Verifier:     pldtypes.RandAddress().String(),
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
	})
	status, err := tp.GetTxStatus(ctx)
	assert.NoError(t, err)
	assert.Equal(t, newTxID.String(), status.TxID)
	assert.Len(t, status.Endorsements, 4)
	endorsementStatusForParty := func(party string) *components.PrivateTxEndorsementStatus {
		for _, e := range status.Endorsements {
			if e.Party == party {
				return &e
			}
		}
		return nil
	}
	require.NotNil(t, endorsementStatusForParty(aliceIdentityLocator))
	require.NotNil(t, endorsementStatusForParty(bobIdentityLocator))
	require.NotNil(t, endorsementStatusForParty(carolIdentityLocator))
	require.NotNil(t, endorsementStatusForParty(daveIdentityLocator))

	assert.Empty(t, endorsementStatusForParty(aliceIdentityLocator).RequestTime)
	assert.True(t, endorsementStatusForParty(aliceIdentityLocator).EndorsementReceived)

	assert.NotEmpty(t, endorsementStatusForParty(bobIdentityLocator).RequestTime)
	assert.False(t, endorsementStatusForParty(bobIdentityLocator).EndorsementReceived)

	assert.NotEmpty(t, endorsementStatusForParty(carolIdentityLocator).RequestTime)
	assert.False(t, endorsementStatusForParty(carolIdentityLocator).EndorsementReceived)

	assert.Empty(t, endorsementStatusForParty(daveIdentityLocator).RequestTime)
	assert.True(t, endorsementStatusForParty(daveIdentityLocator).EndorsementReceived)

	//	Status       string                       `json:"status"`
	//
	// LatestEvent  string                       `json:"latestEvent"`
	// LatestError  string                       `json:"latestError"`
	// Endorsements []PrivateTxEndorsementStatus `json:"endorsements"`
}

func TestDedupResolveVerifierRequests(t *testing.T) {
	// construct an array of resolve verifier requests
	// with duplicates
	// and check that we only send the unique ones
	requests := []*prototk.ResolveVerifierRequest{
		{
			Lookup:       "alice@node1",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
		{
			Lookup:       "bob@node2",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
		{
			Lookup:       "bob@node2",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
		{
			Lookup:       "carol@node3",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.HEX_ECDSA_PUBKEY_UNCOMPRESSED,
		},
		{
			Lookup:       "carol@node3",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}
	dedup := dedupResolveVerifierRequests(requests)
	assert.Len(t, dedup, 4)
	assert.Equal(t, requests[0], dedup[0])
	assert.Equal(t, requests[1], dedup[1])
	assert.Equal(t, requests[3], dedup[2])
	assert.Equal(t, requests[4], dedup[3])
}

type fakeClock struct {
	timePassed time.Duration
}

func (f *fakeClock) Now() time.Time {
	return time.Now().Add(f.timePassed)
}
