/*
 * Copyright © 2025 Kaleido, Inc.
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

package transaction

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/sequencermocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/signpayloads"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

type identityForTesting struct {
	identity        string
	identityLocator string
	verifier        string
	keyHandle       string
}

type SentMessageRecorder struct {
	hasSentAssembleRequest             bool
	hasSentDispatchConfirmationRequest bool
	numberOfSentEndorsementRequests    int
}

func (r *SentMessageRecorder) SendAssembleRequest(
	ctx context.Context,
	assemblingNode string,
	transactionID uuid.UUID,
	transactionPreassembly *components.TransactionPreAssembly,
) error {
	r.hasSentAssembleRequest = true
	return nil
}

func (r *SentMessageRecorder) SendEndorsementRequest(
	ctx context.Context,
	idempotencyKey string,
	party string,
	attRequest *prototk.AttestationRequest,
	transactionSpecification *prototk.TransactionSpecification,
	verifiers []*prototk.ResolvedVerifier,
	signatures []*prototk.AttestationResult,
	inputStates []*components.FullState,
	outputStates []*components.FullState,
	infoStates []*components.FullState,
) error {
	r.numberOfSentEndorsementRequests++
	return nil
}

func (r *SentMessageRecorder) SendDispatchConfirmationRequest(
	ctx context.Context,
	transactionSender string,
	idempotencyKey string,
	transactionSpecification *prototk.TransactionSpecification,
	hash *tktypes.Bytes32,
) error {
	r.hasSentDispatchConfirmationRequest = true
	return nil
}

func NewSentMessageRecorder() *SentMessageRecorder {
	return &SentMessageRecorder{}
}

type TransactionBuilderForTesting struct {
	t      *testing.T
	id     uuid.UUID
	sender *identityForTesting

	dispatchConfirmed    bool
	signerAddress        *tktypes.EthAddress
	latestSubmissionHash *tktypes.Bytes32
	nonce                *uint64
	state                State
	numberOfEndorsers    int
	numberOfEndorsements int
	endorsers            []*identityForTesting
	sentMessageRecorder  *SentMessageRecorder
	mockClock            *sequencermocks.Clock
}

// Function NewTransactionBuilderForTesting creates a TransactionBuilderForTesting with random values for all fields
// use the builder methods to set specific values for fields before calling Build to create a new Transaction
func NewTransactionBuilderForTesting(t *testing.T, state State) *TransactionBuilderForTesting {
	senderName := "sender"
	senderNode := "senderNode"
	builder := &TransactionBuilderForTesting{
		id: uuid.New(),
		sender: &identityForTesting{
			identityLocator: fmt.Sprintf("%s@%s", senderName, senderNode),
			identity:        senderName,
			verifier:        tktypes.RandAddress().String(),
			keyHandle:       senderName + "_KeyHandle",
		},
		dispatchConfirmed:    false,
		signerAddress:        nil,
		latestSubmissionHash: nil,
		numberOfEndorsers:    3,
		numberOfEndorsements: 0,
		t:                    t,
		state:                state,
		sentMessageRecorder:  NewSentMessageRecorder(),
		mockClock:            sequencermocks.NewClock(t),
	}

	switch state {
	case State_Submitted:
		nonce := rand.Uint64()
		builder.nonce = &nonce
		builder.signerAddress = tktypes.RandAddress()
		latestSubmissionHash := tktypes.Bytes32(tktypes.RandBytes(32))
		builder.latestSubmissionHash = &latestSubmissionHash
	case State_Endorsement_Gathering:

	}
	return builder
}

func (b *TransactionBuilderForTesting) NumberOfRequiredEndorsers(num int) *TransactionBuilderForTesting {
	b.numberOfEndorsers = num
	return b
}

func (b *TransactionBuilderForTesting) NumberOfEndorsements(num int) *TransactionBuilderForTesting {
	b.numberOfEndorsements = num
	return b
}

type transactionDependencyMocks struct {
	sentMessageRecorder *SentMessageRecorder
}

func (b *TransactionBuilderForTesting) BuildWithMocks() (*Transaction, *transactionDependencyMocks) {
	mocks := &transactionDependencyMocks{
		sentMessageRecorder: b.sentMessageRecorder,
	}
	return b.Build(), mocks
}

func (b *TransactionBuilderForTesting) Build() *Transaction {
	b.endorsers = make([]*identityForTesting, b.numberOfEndorsers)
	for i := 0; i < b.numberOfEndorsers; i++ {
		endorserName := fmt.Sprintf("endorser-%d", i)
		endorserNode := fmt.Sprintf("node-%d", i)
		b.endorsers[i] = &identityForTesting{
			identity:        endorserName,
			identityLocator: endorserName + "@" + endorserNode,
			verifier:        tktypes.RandAddress().String(),
			keyHandle:       endorserName + "KeyHandle",
		}
	}

	privateTransaction := &components.PrivateTransaction{
		ID: b.id,
		PreAssembly: &components.TransactionPreAssembly{
			Verifiers: make([]*prototk.ResolvedVerifier, b.numberOfEndorsers+1),
		},
	}

	privateTransaction.PreAssembly.Verifiers[0] = &prototk.ResolvedVerifier{
		Lookup:       b.sender.identityLocator,
		Algorithm:    algorithms.ECDSA_SECP256K1,
		VerifierType: verifiers.ETH_ADDRESS,
		Verifier:     tktypes.RandAddress().String(),
	}

	for i := 0; i < b.numberOfEndorsers; i++ {
		privateTransaction.PreAssembly.Verifiers[i+1] = &prototk.ResolvedVerifier{
			Lookup:       b.endorsers[i].identityLocator,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
			Verifier:     b.endorsers[i].verifier,
		}
	}

	switch b.state {
	case State_Endorsement_Gathering:
		privateTransaction.PostAssembly = b.BuildPostAssembly()

	}
	//TODO: do something more useful with clock mocking
	b.mockClock.On("Now").Return(time.Now()).Maybe()
	d := NewTransaction(b.sender.identity, privateTransaction, b.sentMessageRecorder, b.mockClock)
	d.dispatchConfirmed = b.dispatchConfirmed
	d.signerAddress = b.signerAddress
	d.latestSubmissionHash = b.latestSubmissionHash
	d.nonce = b.nonce
	d.stateMachine.currentState = b.state
	return d

}

func (b *TransactionBuilderForTesting) BuildEndorsement(endorserIndex int) *prototk.AttestationResult {
	return &prototk.AttestationResult{
		Name:            fmt.Sprintf("endorse-%d", endorserIndex),
		AttestationType: prototk.AttestationType_ENDORSE,
		Payload:         tktypes.RandBytes(32),
		Verifier: &prototk.ResolvedVerifier{
			Lookup:       b.endorsers[endorserIndex].identityLocator,
			Verifier:     b.endorsers[endorserIndex].verifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}
}

func (b *TransactionBuilderForTesting) BuildPostAssembly() *components.TransactionPostAssembly {
	postAssembly := &components.TransactionPostAssembly{}
	//it is normal to have one AttestationRequest for the sender to sign the pre-assembly

	postAssembly.AttestationPlan = make([]*prototk.AttestationRequest, b.numberOfEndorsers+1, b.numberOfEndorsers+1)
	postAssembly.AttestationPlan[0] = &prototk.AttestationRequest{
		Name:            "sign",
		AttestationType: prototk.AttestationType_SIGN,
		Algorithm:       algorithms.ECDSA_SECP256K1,
		VerifierType:    verifiers.ETH_ADDRESS,
		PayloadType:     signpayloads.OPAQUE_TO_RSV,
		Parties: []string{
			b.sender.identityLocator,
		},
	}

	postAssembly.Signatures = []*prototk.AttestationResult{
		{
			Name:            "sign",
			AttestationType: prototk.AttestationType_SIGN,
			Payload:         tktypes.RandBytes(32),
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       b.sender.identityLocator,
				Verifier:     b.sender.verifier,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
			PayloadType: ptrTo(signpayloads.OPAQUE_TO_RSV),
		},
	}

	for i := 0; i < b.numberOfEndorsers; i++ {
		postAssembly.AttestationPlan[i+1] = &prototk.AttestationRequest{
			Name:            fmt.Sprintf("endorse-%d", i),
			AttestationType: prototk.AttestationType_ENDORSE,
			Algorithm:       algorithms.ECDSA_SECP256K1,
			VerifierType:    verifiers.ETH_ADDRESS,
			PayloadType:     signpayloads.OPAQUE_TO_RSV,
			Parties: []string{
				b.endorsers[i].identityLocator,
			},
		}
	}

	//TODO a bunch of other stuff needs to be populated in the post assembly?

	postAssembly.Endorsements = make([]*prototk.AttestationResult, b.numberOfEndorsements, b.numberOfEndorsements)
	for i := 0; i < b.numberOfEndorsements; i++ {
		postAssembly.Endorsements[i] = b.BuildEndorsement(i)
	}
	return postAssembly

}

func ptrTo[T any](v T) *T {
	return &v
}
