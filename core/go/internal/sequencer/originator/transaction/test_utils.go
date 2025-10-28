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

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/metrics"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/testutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
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

func (r *SentMessageRecorder) SendPreDispatchResponse(ctx context.Context, transactionOriginator string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification) error {
	r.hasSentConfirmationResponse = true
	return nil
}

func (r *SentMessageRecorder) HasSentPreDispatchResponse() bool {
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

func (r *SentMessageRecorder) SendAssembleRequest(ctx context.Context, assemblingNode string, transactionID uuid.UUID, idempotencyID uuid.UUID, transactionPreassembly *components.TransactionPreAssembly, stateLocksJSON []byte, blockHeight int64) error {
	return nil
}

func (r *SentMessageRecorder) SendDelegationRequest(ctx context.Context, coordinatorLocator string, transactions []*components.PrivateTransaction, originatorsBlockHeight uint64) error {
	return nil
}

func (r *SentMessageRecorder) SendDelegationRequestAcknowledgment(ctx context.Context, delegatingNodeName string, delegationId string, delegateNodeName string, transactionID string) error {
	return nil
}

func (r *SentMessageRecorder) SendEndorsementRequest(ctx context.Context, transactionId uuid.UUID, idempotencyKey uuid.UUID, party string, attRequest *prototk.AttestationRequest, transactionSpecification *prototk.TransactionSpecification, verifiers []*prototk.ResolvedVerifier, signatures []*prototk.AttestationResult, inputStates []*prototk.EndorsableState, readStates []*prototk.EndorsableState, outputStates []*prototk.EndorsableState, infoStates []*prototk.EndorsableState) error {
	return nil
}

func (r *SentMessageRecorder) SendEndorsementResponse(ctx context.Context, transactionId, idempotencyKey, contractAddress string, attResult *prototk.AttestationResult, endorsementResult *components.EndorsementResult, revertReason, endorsementName, party, node string) error {
	return nil
}

func (r *SentMessageRecorder) SendPreDispatchRequest(ctx context.Context, transactionOriginator string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification, hash *pldtypes.Bytes32) error {
	return nil
}

func (r *SentMessageRecorder) SendDispatched(ctx context.Context, transactionOriginator string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification) error {
	return nil
}

func (r *SentMessageRecorder) SendNonceAssigned(ctx context.Context, txID uuid.UUID, transactionOriginator string, contractAddress *pldtypes.EthAddress, nonce uint64) error {
	return nil
}

func (r *SentMessageRecorder) SendTransactionSubmitted(ctx context.Context, txID uuid.UUID, transactionOriginator string, contractAddress *pldtypes.EthAddress, txHash *pldtypes.Bytes32) error {
	return nil
}

func (r *SentMessageRecorder) SendTransactionConfirmed(ctx context.Context, txID uuid.UUID, transactionOriginator string, contractAddress *pldtypes.EthAddress, nonce *pldtypes.HexUint64, revertReason pldtypes.HexBytes) error {
	return nil
}

func (r *SentMessageRecorder) SendHandoverRequest(ctx context.Context, activeCoordinator string, contractAddress *pldtypes.EthAddress) error {
	return nil
}

func (r *SentMessageRecorder) SendHeartbeat(ctx context.Context, targetNode string, contractAddress *pldtypes.EthAddress, coordinatorSnapshot *common.CoordinatorSnapshot) error {
	return nil
}

func (r *SentMessageRecorder) SendAssembleResponse(ctx context.Context, txID uuid.UUID, requestID uuid.UUID, postAssembly *components.TransactionPostAssembly, preAssembly *components.TransactionPreAssembly, recipient string) error {
	switch postAssembly.AssemblyResult {
	case prototk.AssembleTransactionResponse_OK:
		r.hasSentAssembleSuccessResponse = true
	case prototk.AssembleTransactionResponse_REVERT:
		r.hasSentAssembleRevertResponse = true
	case prototk.AssembleTransactionResponse_PARK:
		r.hasSentAssembleParkResponse = true
	}
	return nil
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
	eventHandler              func(ctx context.Context, event common.Event) error

	/* Assembling State*/
	assembleRequestID uuid.UUID

	/* Post Assembling States (e.g. endorsing, reverted, parked)*/
	latestFulfilledAssembleRequestID uuid.UUID

	latestSubmissionHash *pldtypes.Bytes32
	signerAddress        *pldtypes.EthAddress
	nonce                *uint64

	metrics metrics.DistributedSequencerMetrics
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
		metrics:                   metrics.InitMetrics(context.Background(), prometheus.NewRegistry()),
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

func (b *TransactionBuilderForTesting) GetSignerAddress() pldtypes.EthAddress {
	if b.signerAddress == nil {
		b.signerAddress = pldtypes.RandAddress()
	}
	return *b.signerAddress
}

func (b *TransactionBuilderForTesting) GetNonce() uint64 {
	if b.nonce == nil {
		b.nonce = ptrTo(rand.Uint64())
	}
	return *b.nonce
}

func (b *TransactionBuilderForTesting) GetLatestSubmissionHash() pldtypes.Bytes32 {
	if b.latestSubmissionHash == nil {
		b.latestSubmissionHash = ptrTo(pldtypes.RandBytes32())
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
	b.eventHandler = func(ctx context.Context, event common.Event) error {
		mocks.emittedEvents = append(mocks.emittedEvents, event)
		return nil
	}
	return b.Build(), mocks
}

func (b *TransactionBuilderForTesting) Build() *Transaction {
	ctx := context.Background()

	privateTransaction := b.privateTransactionBuilder.Build()
	if b.eventHandler == nil {
		b.eventHandler = func(ctx context.Context, event common.Event) error {
			return nil
		}
	}
	txn, err := NewTransaction(ctx, privateTransaction, b.sentMessageRecorder, b.eventHandler, b.fakeEngineIntegration, b.metrics)

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
	case State_Endorsement_Gathering:
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
