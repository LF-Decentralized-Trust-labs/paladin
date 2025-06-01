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
	"github.com/stretchr/testify/mock"
)

type SentMessageRecorder struct {
	hasSentConfirmationResponse    bool
	hasSentAssembleSuccessResponse bool
	hasSentAssembleRevertResponse  bool
	hasSentAssembleParkResponse    bool
}

func NewSentMessageRecorder() *SentMessageRecorder {
	return &SentMessageRecorder{}
}

func (r *SentMessageRecorder) SendDispatchConfirmationResponse(ctx context.Context) {
	r.hasSentConfirmationResponse = true
}

func (r *SentMessageRecorder) HasSentDispatchConfirmationResponse() bool {
	return r.hasSentConfirmationResponse
}

func (r *SentMessageRecorder) HasSentAssembleSuccessResponse() bool {
	return r.hasSentAssembleSuccessResponse
}

func (r *SentMessageRecorder) HasSentAssembleRevertResponse() bool {
	return r.hasSentAssembleRevertResponse
}

func (r *SentMessageRecorder) HasSentAssembleParkResponse() bool {
	return r.hasSentAssembleParkResponse
}

func (r *SentMessageRecorder) SendAssembleResponse(ctx context.Context, requestID uuid.UUID, postAssembly *components.TransactionPostAssembly) {
	switch postAssembly.AssemblyResult {
	case prototk.AssembleTransactionResponse_OK:
		r.hasSentAssembleSuccessResponse = true
	case prototk.AssembleTransactionResponse_REVERT:
		r.hasSentAssembleRevertResponse = true
	case prototk.AssembleTransactionResponse_PARK:
		r.hasSentAssembleParkResponse = true
	}
}

func (r *SentMessageRecorder) Reset(_ context.Context) {
	r.hasSentConfirmationResponse = false
	r.hasSentAssembleSuccessResponse = false
	r.hasSentAssembleRevertResponse = false
	r.hasSentAssembleParkResponse = false
}

type TransactionBuilderForTesting struct {
	privateTransactionBuilder *testutil.PrivateTransactionBuilderForTesting
	state                     State
	currentDelegate           string
	txn                       *Transaction
	sentMessageRecorder       *SentMessageRecorder
	fakeClock                 *common.FakeClockForTesting
	fakeEngineIntegration     *common.FakeEngineIntegrationForTesting
	emitFunction              func(event common.Event)

	/* Assembling State*/
	assembleRequestID uuid.UUID

	/* Post Assembling States (e.g. endorsing, reverted, parked)*/
	latestFulfilledAssembleRequestID uuid.UUID

	latestSubmissionHash *tktypes.Bytes32
	signerAddress        *tktypes.EthAddress
	nonce                *uint64
}

// Function NewTransactionBuilderForTesting creates a TransactionBuilderForTesting with random values for all fields.
// Use the builder methods to set specific values for fields before calling Build to create a new Transaction
func NewTransactionBuilderForTesting(t *testing.T, state State) *TransactionBuilderForTesting {
	builder := &TransactionBuilderForTesting{
		state:                     state,
		currentDelegate:           uuid.New().String(),
		privateTransactionBuilder: testutil.NewPrivateTransactionBuilderForTesting(),
		fakeClock:                 &common.FakeClockForTesting{},
		fakeEngineIntegration:     &common.FakeEngineIntegrationForTesting{},
		sentMessageRecorder:       NewSentMessageRecorder(),
	}

	switch state {
	case State_Delegated:

	}
	return builder
}

func (b *TransactionBuilderForTesting) GetCoordinator() string {
	return b.currentDelegate
}

func (b *TransactionBuilderForTesting) GetLatestFulfilledAssembleRequestID() uuid.UUID {
	return b.latestFulfilledAssembleRequestID
}

func (b *TransactionBuilderForTesting) GetSignerAddress() tktypes.EthAddress {
	if b.signerAddress == nil {
		b.signerAddress = tktypes.RandAddress()
	}
	return *b.signerAddress
}

func (b *TransactionBuilderForTesting) GetNonce() uint64 {
	if b.nonce == nil {
		b.nonce = ptrTo(rand.Uint64())
	}
	return *b.nonce
}

func (b *TransactionBuilderForTesting) GetLatestSubmissionHash() tktypes.Bytes32 {
	if b.latestSubmissionHash == nil {
		b.latestSubmissionHash = ptrTo(tktypes.RandBytes32())
	}
	return *b.latestSubmissionHash
}

type TransactionDependencyFakes struct {
	SentMessageRecorder *SentMessageRecorder
	Clock               *common.FakeClockForTesting
	EngineIntegration   *common.FakeEngineIntegrationForTesting
	transactionBuilder  *TransactionBuilderForTesting
	emittedEvents       []common.Event
}

func (b *TransactionBuilderForTesting) BuildWithMocks() (*Transaction, *TransactionDependencyFakes) {
	mocks := &TransactionDependencyFakes{
		SentMessageRecorder: b.sentMessageRecorder,
		Clock:               b.fakeClock,
		EngineIntegration:   b.fakeEngineIntegration,
		transactionBuilder:  b,
	}
	b.emitFunction = func(event common.Event) {
		mocks.emittedEvents = append(mocks.emittedEvents, event)
	}
	return b.Build(), mocks
}

func (b *TransactionBuilderForTesting) Build() *Transaction {
	ctx := context.Background()

	privateTransaction := b.privateTransactionBuilder.Build()
	if b.emitFunction == nil {
		b.emitFunction = func(event common.Event) {}
	}
	txn, err := NewTransaction(ctx, privateTransaction, b.sentMessageRecorder, b.fakeClock, b.emitFunction, b.fakeEngineIntegration)

	txn.stateMachine.currentState = b.state

	// Update the private transaction struct to the accumulation that resulted from what ever events that we expect to have happened leading up to the current state
	// We don't attempt to emulate any other history of those past events but rather assert that the state machine's behavior is determined purely by its current finite state
	// and the contents of the PrivateTransaction struct

	switch b.state {
	case State_Delegated:
		txn.currentDelegate = b.currentDelegate
	case State_Assembling:
		txn.currentDelegate = b.currentDelegate
		b.assembleRequestID = uuid.New()
		txn.latestAssembleRequest = &assembleRequestFromCoordinator{
			requestID: b.assembleRequestID,
		}
	case State_EndorsementGathering:
		txn.currentDelegate = b.currentDelegate
		b.latestFulfilledAssembleRequestID = uuid.New()
		txn.latestFulfilledAssembleRequestID = b.latestFulfilledAssembleRequestID
	case State_Reverted:
		b.latestFulfilledAssembleRequestID = uuid.New()
		txn.latestFulfilledAssembleRequestID = b.latestFulfilledAssembleRequestID

		txn.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			RevertReason:   ptrTo("test revert reason"),
		}
	case State_Parked:
		b.latestFulfilledAssembleRequestID = uuid.New()
		txn.latestFulfilledAssembleRequestID = b.latestFulfilledAssembleRequestID

		txn.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_PARK,
		}
	case State_Prepared:
		txn.currentDelegate = b.currentDelegate

	case State_Submitted:
		txn.latestSubmissionHash = ptrTo(b.GetLatestSubmissionHash())
		fallthrough
	case State_Sequenced:
		txn.nonce = ptrTo(b.GetNonce())
		fallthrough
	case State_Dispatched:
		txn.currentDelegate = b.currentDelegate
		txn.signerAddress = ptrTo(b.GetSignerAddress())

	}

	if err != nil {
		panic(fmt.Sprintf("Error from NewTransaction: %v", err))
	}
	b.txn = txn

	b.txn.stateMachine.currentState = b.state
	return b.txn

}

func (m *TransactionDependencyFakes) MockForAssembleAndSignRequestOK() *mock.Call {

	return m.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything, //ctx context.Contex
		m.transactionBuilder.txn.ID,
		mock.Anything, //preAssembly *components.TransactionPreAssembly
		mock.Anything, //stateLocksJSON []byte
		mock.Anything, //blockHeight int64
	).Return(&components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}, nil)
}

func (m *TransactionDependencyFakes) MockForAssembleAndSignRequestRevert() *mock.Call {

	return m.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything, //ctx context.Contex
		m.transactionBuilder.txn.ID,
		mock.Anything, //preAssembly *components.TransactionPreAssembly
		mock.Anything, //stateLocksJSON []byte
		mock.Anything, //blockHeight int64
	).Return(&components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
		RevertReason:   ptrTo("test revert reason"),
	}, nil)
}

func (m *TransactionDependencyFakes) MockForAssembleAndSignRequestPark() *mock.Call {

	return m.EngineIntegration.On(
		"AssembleAndSign",
		mock.Anything, //ctx context.Contex
		m.transactionBuilder.txn.ID,
		mock.Anything, //preAssembly *components.TransactionPreAssembly
		mock.Anything, //stateLocksJSON []byte
		mock.Anything, //blockHeight int64
	).Return(&components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_PARK,
		RevertReason:   ptrTo("test revert reason"),
	}, nil)
}

func (m *TransactionDependencyFakes) GetEmittedEvents() []common.Event {
	return m.emittedEvents
}
