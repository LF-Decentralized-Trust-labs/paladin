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
	"github.com/kaleido-io/paladin/core/internal/sequencer/testutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type identityForTesting struct {
	identity        string
	identityLocator string
	verifier        string
	keyHandle       string
}

type SentMessageRecorder struct {
	hasSentAssembleRequest                        bool
	sentAssembleRequestIdempotencyKey             uuid.UUID
	numberOfSentAssembleRequests                  int
	hasSentDispatchConfirmationRequest            bool
	numberOfSentEndorsementRequests               int
	sentEndorsementRequestsForPartyIdempotencyKey map[string]uuid.UUID
	numberOfEndorsementRequestsForParty           map[string]int
	sentDispatchConfirmationRequestIdempotencyKey uuid.UUID
	numberOfSentDispatchConfirmationRequests      int
}

func (r *SentMessageRecorder) Reset(ctx context.Context) {
	r.hasSentAssembleRequest = false
	r.sentAssembleRequestIdempotencyKey = uuid.UUID{}
	r.numberOfSentAssembleRequests = 0
	r.hasSentDispatchConfirmationRequest = false
	r.numberOfSentEndorsementRequests = 0
	r.sentEndorsementRequestsForPartyIdempotencyKey = make(map[string]uuid.UUID)
	r.numberOfEndorsementRequestsForParty = make(map[string]int)
	r.sentDispatchConfirmationRequestIdempotencyKey = uuid.UUID{}
	r.numberOfSentDispatchConfirmationRequests = 0
}

func (r *SentMessageRecorder) HasSentAssembleRequest() bool {
	return r.hasSentAssembleRequest
}

func (r *SentMessageRecorder) HasSentDispatchConfirmationRequest() bool {
	return r.hasSentDispatchConfirmationRequest
}

func (r *SentMessageRecorder) NumberOfSentAssembleRequests() int {
	return r.numberOfSentAssembleRequests
}

func (r *SentMessageRecorder) NumberOfSentEndorsementRequests() int {
	return r.numberOfSentEndorsementRequests
}

func (r *SentMessageRecorder) SentEndorsementRequestsForPartyIdempotencyKey(party string) uuid.UUID {
	return r.sentEndorsementRequestsForPartyIdempotencyKey[party]
}

func (r *SentMessageRecorder) NumberOfEndorsementRequestsForParty(party string) int {
	return r.numberOfEndorsementRequestsForParty[party]
}

func (r *SentMessageRecorder) NumberOfSentDispatchConfirmationRequests() int {
	return r.numberOfSentDispatchConfirmationRequests
}

func (r *SentMessageRecorder) SentAssembleRequestIdempotencyKey() uuid.UUID {
	return r.sentAssembleRequestIdempotencyKey
}

func (r *SentMessageRecorder) SentDispatchConfirmationRequestIdempotencyKey() uuid.UUID {
	return r.sentDispatchConfirmationRequestIdempotencyKey
}

func (r *SentMessageRecorder) SendAssembleRequest(
	ctx context.Context,
	assemblingNode string,
	transactionID uuid.UUID,
	idempotencyKey uuid.UUID,
	transactionPreassembly *components.TransactionPreAssembly,
) error {
	r.hasSentAssembleRequest = true
	r.sentAssembleRequestIdempotencyKey = idempotencyKey
	r.numberOfSentAssembleRequests++
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
	inputStates []*prototk.EndorsableState,
	outputStates []*prototk.EndorsableState,
	infoStates []*prototk.EndorsableState,
) error {
	r.numberOfSentEndorsementRequests++
	if _, ok := r.numberOfEndorsementRequestsForParty[party]; ok {
		r.numberOfEndorsementRequestsForParty[party]++
	} else {
		r.numberOfEndorsementRequestsForParty[party] = 1
		r.sentEndorsementRequestsForPartyIdempotencyKey[party] = idempotencyKey
	}
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
	r.sentDispatchConfirmationRequestIdempotencyKey = idempotencyKey
	r.numberOfSentDispatchConfirmationRequests++
	return nil
}

func NewSentMessageRecorder() *SentMessageRecorder {
	return &SentMessageRecorder{
		sentEndorsementRequestsForPartyIdempotencyKey: make(map[string]uuid.UUID),
		numberOfEndorsementRequestsForParty:           make(map[string]int),
	}
}

type TransactionBuilderForTesting struct {
	privateTransactionBuilder          *testutil.PrivateTransactionBuilderForTesting
	sender                             *identityForTesting
	dispatchConfirmed                  bool
	signerAddress                      *tktypes.EthAddress
	latestSubmissionHash               *tktypes.Bytes32
	nonce                              *uint64
	state                              State
	sentMessageRecorder                *SentMessageRecorder
	fakeClock                          *common.FakeClockForTesting
	fakeEngineIntegration              *common.FakeEngineIntegrationForTesting
	grapher                            Grapher
	txn                                *Transaction
	requestTimeout                     int
	assembleTimeout                    int
	heartbeatIntervalsSinceStateChange int
}

// Function NewTransactionBuilderForTesting creates a TransactionBuilderForTesting with random values for all fields
// use the builder methods to set specific values for fields before calling Build to create a new Transaction
func NewTransactionBuilderForTesting(t *testing.T, state State) *TransactionBuilderForTesting {
	senderName := "sender"
	senderNode := "senderNode"
	builder := &TransactionBuilderForTesting{
		sender: &identityForTesting{
			identityLocator: fmt.Sprintf("%s@%s", senderName, senderNode),
			identity:        senderName,
			verifier:        tktypes.RandAddress().String(),
			keyHandle:       senderName + "_KeyHandle",
		},
		dispatchConfirmed:         false,
		signerAddress:             nil,
		latestSubmissionHash:      nil,
		state:                     state,
		sentMessageRecorder:       NewSentMessageRecorder(),
		fakeClock:                 &common.FakeClockForTesting{},
		fakeEngineIntegration:     &common.FakeEngineIntegrationForTesting{},
		assembleTimeout:           5000,
		requestTimeout:            100,
		privateTransactionBuilder: testutil.NewPrivateTransactionBuilderForTesting(),
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
		//we are emulating a transaction that has been passed State_Endorsement_Gathering so default to complete attestation plan
		builder.privateTransactionBuilder.EndorsementComplete()
	}
	return builder
}

func (b *TransactionBuilderForTesting) NumberOfRequiredEndorsers(num int) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.NumberOfRequiredEndorsers(num)
	return b
}

func (b *TransactionBuilderForTesting) NumberOfEndorsements(num int) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.NumberOfEndorsements(num)
	return b
}

func (b *TransactionBuilderForTesting) NumberOfOutputStates(num int) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.NumberOfOutputStates(num)
	return b
}

func (b *TransactionBuilderForTesting) InputStateIDs(stateIDs ...tktypes.HexBytes) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.InputStateIDs(stateIDs...)
	return b
}

func (b *TransactionBuilderForTesting) ReadStateIDs(stateIDs ...tktypes.HexBytes) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.ReadStateIDs(stateIDs...)
	return b
}

func (b *TransactionBuilderForTesting) PredefinedDependencies(transactionIDs ...uuid.UUID) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.PredefinedDependencies(transactionIDs...)
	return b
}

func (b *TransactionBuilderForTesting) Reverts(revertReason string) *TransactionBuilderForTesting {
	b.privateTransactionBuilder.Reverts(revertReason)
	return b
}

func (b *TransactionBuilderForTesting) Grapher(grapher Grapher) *TransactionBuilderForTesting {
	b.grapher = grapher
	return b
}

func (b *TransactionBuilderForTesting) Sender(sender *identityForTesting) *TransactionBuilderForTesting {
	b.sender = sender
	return b
}

func (b *TransactionBuilderForTesting) HeartbeatIntervalsSinceStateChange(heartbeatIntervalsSinceStateChange int) *TransactionBuilderForTesting {
	b.heartbeatIntervalsSinceStateChange = heartbeatIntervalsSinceStateChange
	return b
}

func (b *TransactionBuilderForTesting) GetSender() *identityForTesting {
	return b.sender
}

func (b *TransactionBuilderForTesting) GetAssembleTimeout() int {
	return b.assembleTimeout
}

func (b *TransactionBuilderForTesting) GetRequestTimeout() int {
	return b.requestTimeout
}

func (b *TransactionBuilderForTesting) GetEndorsers() []string {
	endorsers := make([]string, b.privateTransactionBuilder.GetNumberOfEndorsers())
	for i := range endorsers {
		endorsers[i] = b.privateTransactionBuilder.GetEndorserIdentityLocator(i)
	}
	return endorsers
}

type transactionDependencyFakes struct {
	SentMessageRecorder *SentMessageRecorder
	Clock               *common.FakeClockForTesting
	EngineIntegration   *common.FakeEngineIntegrationForTesting
}

func (b *TransactionBuilderForTesting) BuildWithMocks() (*Transaction, *transactionDependencyFakes) {
	mocks := &transactionDependencyFakes{
		SentMessageRecorder: b.sentMessageRecorder,
		Clock:               b.fakeClock,
		EngineIntegration:   b.fakeEngineIntegration,
	}
	return b.Build(), mocks
}

func (b *TransactionBuilderForTesting) Build() *Transaction {
	ctx := context.Background()
	if b.grapher == nil {
		b.grapher = NewGrapher(ctx)
	}

	privateTransaction := b.privateTransactionBuilder.Build()

	txn, err := NewTransaction(
		ctx,
		b.sender.identityLocator,
		privateTransaction,
		b.sentMessageRecorder,
		b.fakeClock,
		func(_ common.Event) {},
		b.fakeEngineIntegration,
		b.fakeClock.Duration(b.requestTimeout),
		b.fakeClock.Duration(b.assembleTimeout),
		5,
		b.grapher,
		nil,
		func(context.Context) {}, // onCleanup function, not used in tests
	)
	if err != nil {
		panic(fmt.Sprintf("Error from NewTransaction: %v", err))
	}
	b.txn = txn

	//Update the private transaction struct to the accumulation that resulted from what ever events that we expect to have happened leading up to the current state
	// We don't attempt to emulate any other history of those past events but rather assert that the state machine's behavior is determined purely by its current finite state
	// and the contents of the PrivateTransaction struct

	if b.state == State_Endorsement_Gathering ||
		b.state == State_Blocked ||
		b.state == State_Confirming_Dispatch ||
		b.state == State_Ready_For_Dispatch {

		err := b.txn.applyPostAssembly(ctx, b.BuildPostAssembly())
		if err != nil {
			panic("error from applyPostAssembly")
		}
	}

	if b.state == State_Confirmed ||
		b.state == State_Reverted {
		b.txn.heartbeatIntervalsSinceStateChange = b.heartbeatIntervalsSinceStateChange
	}

	//enter the current state
	onTransitionFunction := stateDefinitionsMap[b.state].OnTransitionTo
	if onTransitionFunction != nil {
		err := onTransitionFunction(ctx, b.txn)
		if err != nil {
			panic(fmt.Sprintf("Error from initializeDependencies: %v", err))
		}
	}

	b.txn.signerAddress = b.signerAddress
	b.txn.latestSubmissionHash = b.latestSubmissionHash
	b.txn.nonce = b.nonce
	b.txn.stateMachine.currentState = b.state
	return b.txn

}

func (b *TransactionBuilderForTesting) BuildEndorsedEvent(endorserIndex int) *EndorsedEvent {

	return &EndorsedEvent{
		BaseEvent: BaseEvent{
			TransactionID: b.txn.ID,
		},
		RequestID:   b.txn.pendingEndorsementRequests[b.privateTransactionBuilder.GetEndorsementName(endorserIndex)][b.privateTransactionBuilder.GetEndorserIdentityLocator(endorserIndex)].IdempotencyKey(),
		Endorsement: b.privateTransactionBuilder.BuildEndorsement(endorserIndex),
	}

}

func (b *TransactionBuilderForTesting) BuildEndorseRejectedEvent(endorserIndex int) *EndorsedRejectedEvent {

	attReqName := fmt.Sprintf("endorse-%d", endorserIndex)
	return &EndorsedRejectedEvent{
		BaseEvent: BaseEvent{
			TransactionID: b.txn.ID,
		},
		RevertReason:           "some reason for rejection",
		AttestationRequestName: attReqName,
		RequestID:              b.txn.pendingEndorsementRequests[attReqName][b.privateTransactionBuilder.GetEndorserIdentityLocator(endorserIndex)].IdempotencyKey(),
		Party:                  b.privateTransactionBuilder.GetEndorserIdentityLocator(endorserIndex),
	}

}

func (b *TransactionBuilderForTesting) BuildPostAssembly() *components.TransactionPostAssembly {
	return b.privateTransactionBuilder.BuildPostAssembly()
}

func ptrTo[T any](v T) *T {
	return &v
}
