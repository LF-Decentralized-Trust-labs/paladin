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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
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
	idempotencyKey uuid.UUID,
	transactionPreassembly *components.TransactionPreAssembly,
) error {
	r.hasSentAssembleRequest = true
	return nil
}

func (r *SentMessageRecorder) SendEndorsementRequest(
	ctx context.Context,
	idempotencyKey uuid.UUID,
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
	idempotencyKey uuid.UUID,
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
	//TODO this is becoming a large flat list.  Should we group these into sub-structs? or at least order the fields and/or group into some sensible sections
	id     uuid.UUID
	sender *identityForTesting

	dispatchConfirmed    bool
	signerAddress        *tktypes.EthAddress
	latestSubmissionHash *tktypes.Bytes32
	nonce                *uint64
	state                State
	numberOfEndorsers    int
	numberOfEndorsements int
	numberOfOutputStates int
	inputStateIDs        []tktypes.HexBytes
	readStateIDs         []tktypes.HexBytes
	endorsers            []*identityForTesting
	sentMessageRecorder  *SentMessageRecorder
	fakeClock            *common.FakeClockForTesting
	stateIndex           StateIndex
	txn                  *Transaction
	requestTimeout       int
	assembleTimeout      int
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
		numberOfOutputStates: 1,
		state:                state,
		sentMessageRecorder:  NewSentMessageRecorder(),
		fakeClock:            &common.FakeClockForTesting{},
		assembleTimeout:      5000,
		requestTimeout:       100,
	}

	switch state {
	case State_Submitted:
		nonce := rand.Uint64()
		builder.nonce = &nonce
		builder.signerAddress = tktypes.RandAddress()
		latestSubmissionHash := tktypes.Bytes32(tktypes.RandBytes(32))
		builder.latestSubmissionHash = &latestSubmissionHash
	case State_Endorsement_Gathering:
		//fine grained detail in this state needed to emulate what has already happened wrt endorsement requests and responses so far

	case State_Blocked:
		fallthrough
	case State_Confirming_Dispatch:
		fallthrough
	case State_Ready_For_Dispatch:
		fallthrough
	case State_Dispatched:
		fallthrough
	case State_Confirmed:
		//we are emulating a transaction that has been passed  State_Endorsement_Gathering so default to complete attestation plan
		builder.numberOfEndorsements = builder.numberOfEndorsers

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

func (b *TransactionBuilderForTesting) NumberOfOutputStates(num int) *TransactionBuilderForTesting {
	b.numberOfOutputStates = num
	return b
}

func (b *TransactionBuilderForTesting) InputStateIDs(stateIDs ...tktypes.HexBytes) *TransactionBuilderForTesting {
	b.inputStateIDs = stateIDs
	return b
}

func (b *TransactionBuilderForTesting) ReadStateIDs(stateIDs ...tktypes.HexBytes) *TransactionBuilderForTesting {
	b.readStateIDs = stateIDs
	return b
}

func (b *TransactionBuilderForTesting) StateIndex(stateIndex StateIndex) *TransactionBuilderForTesting {
	b.stateIndex = stateIndex
	return b
}

type transactionDependencyFakes struct {
	sentMessageRecorder *SentMessageRecorder
	clock               *common.FakeClockForTesting
}

func (b *TransactionBuilderForTesting) BuildWithMocks() (*Transaction, *transactionDependencyFakes) {
	mocks := &transactionDependencyFakes{
		sentMessageRecorder: b.sentMessageRecorder,
		clock:               b.fakeClock,
	}
	return b.Build(), mocks
}

func (b *TransactionBuilderForTesting) Build() *Transaction {
	ctx := context.Background()
	if b.stateIndex == nil {
		b.stateIndex = &stateIndex{
			transactionByOutputState: make(map[string]*Transaction),
		}
	}
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

	b.txn = NewTransaction(b.sender.identity, privateTransaction, b.sentMessageRecorder, b.fakeClock, b.fakeClock.Duration(b.requestTimeout), b.fakeClock.Duration(b.assembleTimeout), b.stateIndex)

	//Assuming a "normal" path through the state machine to the current desired state

	//TODO this could all be a lot easier if every state had a enter function and an exit function that cleaned up the object to remove all history of the previous state ( other than the contents of the private transaction).  In fact, should probably make that explicit in the structure of the struct.  Maybe fields relating to a particular state should be in the state machine struct rather than the transaction struct?
	if b.state == State_Assembling {

		err := b.txn.sendAssembleRequest(ctx)
		if err != nil {
			panic(fmt.Sprintf("Error sending assemble request: %v", err))
		}

	}
	if b.state == State_Endorsement_Gathering {
		b.txn.applyPostAssembly(ctx, b.BuildPostAssembly())
		err := b.txn.sendEndorsementRequests(ctx)
		if err != nil {
			panic(fmt.Sprintf("Error sending endorsement requests: %v", err))
		}
	}

	if b.state == State_Blocked ||
		b.state == State_Confirming_Dispatch ||
		b.state == State_Ready_For_Dispatch {
		b.txn.applyPostAssembly(ctx, b.BuildPostAssembly())
	}

	b.txn.signerAddress = b.signerAddress
	b.txn.latestSubmissionHash = b.latestSubmissionHash
	b.txn.nonce = b.nonce
	b.txn.stateMachine.currentState = b.state
	return b.txn

}

func (b *TransactionBuilderForTesting) BuildEndorsedEvent(endorserIndex int) *EndorsedEvent {

	attReqName := fmt.Sprintf("endorse-%d", endorserIndex)
	return &EndorsedEvent{
		event: event{
			TransactionID: b.txn.ID,
		},
		RequestID: b.txn.pendingEndorsementRequests[attReqName][b.endorsers[endorserIndex].identityLocator].IdempotencyKey(),
		Endorsement: &prototk.AttestationResult{
			//TODO duplication here with func (b *PostAssemblyBuilderForTesting) BuildEndorsement
			Name:            attReqName,
			AttestationType: prototk.AttestationType_ENDORSE,
			Payload:         tktypes.RandBytes(32),
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       b.endorsers[endorserIndex].identityLocator,
				Verifier:     b.endorsers[endorserIndex].verifier,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
	}

}

func NewPostAssemblyBuilderForTesting() *PostAssemblyBuilderForTesting {
	return &PostAssemblyBuilderForTesting{}
}

func (b *TransactionBuilderForTesting) BuildPostAssembly() *components.TransactionPostAssembly {
	postAssemblyBuilder := &PostAssemblyBuilderForTesting{
		sender:               b.sender,
		numberOfEndorsers:    b.numberOfEndorsers,
		numberOfEndorsements: b.numberOfEndorsements,
		endorsers:            b.endorsers,
		numberOfOutputStates: b.numberOfOutputStates,
		inputStateIDs:        b.inputStateIDs,
		readStateIDs:         b.readStateIDs,
	}
	return postAssemblyBuilder.Build()

}

// TODO is it worth having this as a separate struct?
// only time that it makes sense to explicitly create a PostAssemblyBuilder when you don't have a TransactionBuilder that you can use
// to build the PostAssembly that you want is when you are deliberately building the wrong PostAssembly e.g. to test
// weird error conditions
type PostAssemblyBuilderForTesting struct {
	sender               *identityForTesting
	numberOfEndorsers    int
	numberOfEndorsements int
	numberOfOutputStates int
	inputStateIDs        []tktypes.HexBytes
	readStateIDs         []tktypes.HexBytes
	endorsers            []*identityForTesting
}

func (b *PostAssemblyBuilderForTesting) Build() *components.TransactionPostAssembly {
	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}

	//it is normal to have one AttestationRequest for the sender to sign the pre-assembly
	postAssembly.AttestationPlan = make([]*prototk.AttestationRequest, b.numberOfEndorsers+1)
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

	for i := 0; i < b.numberOfOutputStates; i++ {
		postAssembly.OutputStates = append(postAssembly.OutputStates, &components.FullState{
			ID: tktypes.HexBytes(tktypes.RandBytes(32)),
		})
	}

	for _, inputStateID := range b.inputStateIDs {
		postAssembly.InputStates = append(postAssembly.InputStates, &components.FullState{
			ID:     inputStateID,
			Schema: tktypes.Bytes32(tktypes.RandBytes(32)),
			Data:   tktypes.JSONString("{\"data\":\"hello\"}"),
		})
	}

	for _, readStateID := range b.readStateIDs {
		postAssembly.ReadStates = append(postAssembly.ReadStates, &components.FullState{
			ID:     readStateID,
			Schema: tktypes.Bytes32(tktypes.RandBytes(32)),
			Data:   tktypes.JSONString("{\"data\":\"hello\"}"),
		})
	}

	//TODO should we really add the endorsements here or would it be better to always create a sparse postassembly as per the assemble step
	// and force the transactionBuilder to add the endorsements separately?
	postAssembly.Endorsements = make([]*prototk.AttestationResult, b.numberOfEndorsements)
	for i := 0; i < b.numberOfEndorsements; i++ {
		postAssembly.Endorsements[i] = b.BuildEndorsement(i)
	}
	return postAssembly
}

func (b *PostAssemblyBuilderForTesting) BuildEndorsement(endorserIndex int) *prototk.AttestationResult {
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

func ptrTo[T any](v T) *T {
	return &v
}
