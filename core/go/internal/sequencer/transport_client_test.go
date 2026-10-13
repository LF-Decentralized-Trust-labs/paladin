/*
 * Copyright © 2026 Kaleido, Inc.
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

package sequencer

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator"
	coordTransaction "github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator"
	originatorTransaction "github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/coordinatormocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/coordinatorstateviewmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/originatormocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/originatorstateviewmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/persistencemocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/sequencermetricsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/sequencertransportmocks"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type transportClientTestMocks struct {
	components         *componentsmocks.AllComponents
	domainManager      *componentsmocks.DomainManager
	stateManager       *componentsmocks.StateManager
	persistence        *persistencemocks.Persistence
	txManager          *componentsmocks.TXManager
	domainAPI          *componentsmocks.DomainSmartContract
	domain             *componentsmocks.Domain
	domainQueryContext *componentsmocks.DomainQueryContext
	transportWriter    *sequencertransportmocks.TransportWriter
	originator         *originatormocks.Originator
	coordinator        *coordinatormocks.Coordinator
	metrics            *sequencermetricsmocks.DistributedSequencerMetrics
}

func newTransportClientTestMocks(t *testing.T) *transportClientTestMocks {
	return &transportClientTestMocks{
		components:         componentsmocks.NewAllComponents(t),
		domainManager:      componentsmocks.NewDomainManager(t),
		stateManager:       componentsmocks.NewStateManager(t),
		persistence:        persistencemocks.NewPersistence(t),
		txManager:          componentsmocks.NewTXManager(t),
		domainAPI:          componentsmocks.NewDomainSmartContract(t),
		domain:             componentsmocks.NewDomain(t),
		domainQueryContext: componentsmocks.NewDomainQueryContext(t),
		transportWriter:    sequencertransportmocks.NewTransportWriter(t),
		originator:         originatormocks.NewOriginator(t),
		coordinator:        coordinatormocks.NewCoordinator(t),
		metrics:            sequencermetricsmocks.NewDistributedSequencerMetrics(t),
	}
}

func newSequencerManagerForTransportClientTesting(t *testing.T, mocks *transportClientTestMocks) *sequencerManager {
	ctx := context.Background()
	config := &pldconf.SequencerConfig{}

	sm := &sequencerManager{
		ctx:                         ctx,
		config:                      config,
		components:                  mocks.components,
		nodeName:                    "test-node",
		sequencersLock:              sync.RWMutex{},
		sequencers:                  make(map[string]*sequencer),
		metrics:                     mocks.metrics,
		targetActiveSequencersLimit: 2,
	}

	return sm
}

func newSequencerForTransportClientTesting(contractAddr *pldtypes.EthAddress, mocks *transportClientTestMocks) *sequencer {
	return &sequencer{
		contractAddress: contractAddr.String(),
		originator:      mocks.originator,
		coordinator:     mocks.coordinator,
		transportWriter: mocks.transportWriter,
		lastTXTime:      time.Now(),
	}
}

func setupDefaultMocks(ctx context.Context, mocks *transportClientTestMocks, contractAddr *pldtypes.EthAddress) {
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Maybe()
	mocks.components.EXPECT().StateManager().Return(mocks.stateManager).Maybe()
	mocks.components.EXPECT().TxManager().Return(mocks.txManager).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainAPI.EXPECT().Domain().Return(mocks.domain).Maybe()
	mocks.domainAPI.EXPECT().Address().Return(*contractAddr).Maybe()
	mocks.stateManager.EXPECT().NewDomainQueryContext(ctx, mocks.domain, *contractAddr).Return(mocks.domainQueryContext).Maybe()
}

func TestHandlePaladinMsg_Routing(t *testing.T) {
	tests := []struct {
		name        string
		messageType string
	}{
		{"AssembleRequest", transport.MessageType_AssembleRequest},
		{"AssembleResponse", transport.MessageType_AssembleResponse},
		{"AssembleError", transport.MessageType_AssembleError},
		{"SignResponse", transport.MessageType_SignResponse},
		{"SignError", transport.MessageType_SignError},
		{"CoordinatorHeartbeatNotification", transport.MessageType_CoordinatorHeartbeatNotification},
		{"DelegationRequest", transport.MessageType_DelegationRequest},
		{"DelegationResponse", transport.MessageType_DelegationResponse},
		{"Dispatched", transport.MessageType_Dispatched},
		{"PreDispatchRequest", transport.MessageType_PreDispatchRequest},
		{"PreDispatchResponse", transport.MessageType_PreDispatchResponse},
		{"EndorsementRequest", transport.MessageType_EndorsementRequest},
		{"EndorsementResponse", transport.MessageType_EndorsementResponse},
		{"NonceAssigned", transport.MessageType_NonceAssigned},
		{"TransactionSubmitted", transport.MessageType_TransactionSubmitted},
		{"TransactionConfirmed", transport.MessageType_TransactionConfirmed},
		{"QueryAvailableStatesRequest", transport.MessageType_QueryAvailableStatesRequest},
		{"QueryAvailableStatesResponse", transport.MessageType_QueryAvailableStatesResponse},
		{"GetSpentStateIDsRequest", transport.MessageType_GetSpentStateIDsRequest},
		{"GetSpentStateIDsResponse", transport.MessageType_GetSpentStateIDsResponse},
		{"StateViewError", transport.MessageType_StateViewError},
		{"Unknown", "UnknownMessageType"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			mocks := newTransportClientTestMocks(t)
			sm := newSequencerManagerForTransportClientTesting(t, mocks)

			message := &components.ReceivedMessage{
				FromNode:    "test-node",
				MessageID:   uuid.New(),
				MessageType: tt.messageType,
				Payload:     []byte("test-payload"),
			}

			// Should not panic
			sm.HandlePaladinMsg(ctx, message)
		})
	}
}

func TestHandleAssembleRequest_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	// Create test data
	txID := uuid.New()
	requestID := uuid.New()

	assembleRequest := &engineProto.AssembleRequest{
		TransactionId:          txID.String(),
		AssembleRequestId:      requestID.String(),
		ContractAddress:        contractAddr.String(),
		CoordinatorBlockHeight: 100,
	}
	payload, _ := proto.Marshal(assembleRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleRequest,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.AssembleRequestReceivedEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID
	})).Once()

	sm.handleAssembleRequest(ctx, message)

	mocks.originator.AssertExpectations(t)
	mocks.coordinator.AssertExpectations(t)
}

func TestHandleAssembleRequest_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleRequest,
		Payload:     []byte("invalid-proto"),
	}

	// Should not panic
	sm.handleAssembleRequest(ctx, message)
}

func TestHandleAssembleRequest_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	txID := uuid.New()
	requestID := uuid.New()

	assembleRequest := &engineProto.AssembleRequest{
		TransactionId:          txID.String(),
		AssembleRequestId:      requestID.String(),
		ContractAddress:        "invalid-address",
		CoordinatorBlockHeight: 100,
	}
	payload, _ := proto.Marshal(assembleRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleRequest,
		Payload:     payload,
	}

	// Should not panic
	sm.handleAssembleRequest(ctx, message)
}

func TestHandleAssembleRequest_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	requestID := uuid.New()

	assembleRequest := &engineProto.AssembleRequest{
		TransactionId:          txID.String(),
		AssembleRequestId:      requestID.String(),
		ContractAddress:        contractAddr.String(),
		CoordinatorBlockHeight: 100,
	}
	payload, _ := proto.Marshal(assembleRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleRequest,
		Payload:     payload,
	}

	// GetSequencer is used and returns nil since no sequencer is in memory - should not panic
	sm.handleAssembleRequest(ctx, message)
}

func TestHandleAssembleResponse_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	requestID := uuid.New()

	assembleResponse := &engineProto.AssembleResponse{
		TransactionId:     txID.String(),
		AssembleRequestId: requestID.String(),
		ContractAddress:   contractAddr.String(),
		PostAssembly: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
		},
	}
	payload, _ := proto.Marshal(assembleResponse)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleResponse,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.AssembleSuccessEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID
	})).Once()

	sm.handleAssembleResponse(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleAssembleResponse_Revert(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	requestID := uuid.New()

	assembleResponse := &engineProto.AssembleResponse{
		TransactionId:     txID.String(),
		AssembleRequestId: requestID.String(),
		ContractAddress:   contractAddr.String(),
		PostAssembly: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
		},
	}
	payload, _ := proto.Marshal(assembleResponse)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleResponse,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.AssembleRevertEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID
	})).Once()

	sm.handleAssembleResponse(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleAssembleError_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	requestID := uuid.New()
	assembleError := &engineProto.AssembleError{
		TransactionId:     txID.String(),
		AssembleRequestId: requestID.String(),
		ContractAddress:   contractAddr.String(),
		ErrorMessage:      "test error",
	}
	payload, _ := proto.Marshal(assembleError)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_AssembleError,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.AssembleErrorEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID
	})).Once()

	sm.handleAssembleError(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func newDelegationRequestMessage(fromNode string, contractAddr *pldtypes.EthAddress, blockHeight int64, txs ...*components.PrivateTransaction) *components.ReceivedMessage {
	delegations := make([]*engineProto.PrivateTransactionDelegation, 0, len(txs))
	for _, tx := range txs {
		delegations = append(delegations, &engineProto.PrivateTransactionDelegation{
			Domain:      tx.Domain,
			Intent:      tx.Intent,
			PreAssembly: tx.PreAssembly,
		})
	}
	delegationRequest := &engineProto.DelegationRequest{
		ContractAddress:       contractAddr.HexString(),
		Transactions:          delegations,
		OriginatorBlockHeight: blockHeight,
	}
	payload, _ := proto.Marshal(delegationRequest)
	return &components.ReceivedMessage{
		FromNode:    fromNode,
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationRequest,
		Payload:     payload,
	}
}

func specTransactionID(txID string) string {
	return pldtypes.Bytes32UUIDFirst16(uuid.MustParse(txID)).String()
}

func newTestPrivateTx(contractAddr *pldtypes.EthAddress) *components.PrivateTransaction {
	txID := uuid.New()
	return &components.PrivateTransaction{
		ID: txID,
		PreAssembly: &prototk.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				TransactionId: pldtypes.Bytes32UUIDFirst16(txID).String(),
				ContractInfo: &prototk.ContractInfo{
					ContractAddress: contractAddr.String(),
				},
				From: "originator@node1",
			},
		},
	}
}

func TestHandleDelegationRequest_Success(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	privateTx := newTestPrivateTx(contractAddr)
	message := newDelegationRequestMessage("node1", contractAddr, 100, privateTx)

	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.TransactionsDelegatedEvent)
		return ok &&
			len(event.Transactions) == 1 &&
			event.Transactions[0].ID == privateTx.ID &&
			event.FromNode == "node1" &&
			event.Originator == "originator@node1" &&
			event.OriginatorsBlockHeight == 100
	})).Once()

	sm.handleDelegationRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleDelegationRequest_MultipleTxBatch(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	tx1 := newTestPrivateTx(contractAddr)
	tx2 := newTestPrivateTx(contractAddr)
	tx3 := newTestPrivateTx(contractAddr)
	message := newDelegationRequestMessage("node1", contractAddr, 42, tx1, tx2, tx3)

	setupDefaultMocks(ctx, mocks, contractAddr)
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Maybe()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.TransactionsDelegatedEvent)
		if !ok || len(event.Transactions) != 3 {
			return false
		}
		return event.Transactions[0].ID == tx1.ID &&
			event.Transactions[1].ID == tx2.ID &&
			event.Transactions[2].ID == tx3.ID &&
			event.FromNode == "node1" &&
			event.OriginatorsBlockHeight == 42
	})).Once()

	sm.handleDelegationRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleDelegationRequest_EmptyTransactions(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	// A delegation request with no transactions — coordinator must not be called
	message := newDelegationRequestMessage("originator-node", contractAddr, 100)

	sm.handleDelegationRequest(ctx, message)

	// No QueueEvent call expected — mocks.coordinator has no registered expectations,
	// and NewCoordinator(t) will fail the test if QueueEvent is called unexpectedly.
}

func TestHandleDelegationRequest_OriginatorNotOnSendingNode(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	privateTx := newTestPrivateTx(contractAddr)
	privateTx.PreAssembly.TransactionSpecification.From = "victim@node2"
	message := newDelegationRequestMessage("node1", contractAddr, 100, privateTx)

	sm.handleDelegationRequest(ctx, message)
}

func TestHandleDelegationRequest_OriginatorNotFullyQualified(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	privateTx := newTestPrivateTx(contractAddr)
	privateTx.PreAssembly.TransactionSpecification.From = "originator"
	message := newDelegationRequestMessage("node1", contractAddr, 100, privateTx)

	sm.handleDelegationRequest(ctx, message)
}

func TestHandleDelegationRequest_BatchWithOriginatorNotOnSendingNode(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	tx1 := newTestPrivateTx(contractAddr)
	tx2 := newTestPrivateTx(contractAddr)
	tx2.PreAssembly.TransactionSpecification.From = "victim@node2"
	message := newDelegationRequestMessage("node1", contractAddr, 100, tx1, tx2)

	sm.handleDelegationRequest(ctx, message)
}

func TestHandleDelegationRequest_InvalidTransactionSpecificationID(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	privateTx := newTestPrivateTx(contractAddr)
	privateTx.PreAssembly.TransactionSpecification.TransactionId = "not-hex"
	message := newDelegationRequestMessage("node1", contractAddr, 100, privateTx)

	sm.handleDelegationRequest(ctx, message)
}

func TestHandleNonceAssigned_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	nonceAssigned := &engineProto.NonceAssigned{
		TransactionId:   txID.String(),
		ContractAddress: contractAddr.String(),
		Nonce:           42,
	}
	payload, _ := proto.Marshal(nonceAssigned)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_NonceAssigned,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.NonceAssignedEvent)
		return ok && event.TransactionID == txID && event.Nonce == 42
	})).Once()

	sm.handleNonceAssigned(ctx, message)

	mocks.originator.AssertExpectations(t)
}

func TestHandleTransactionSubmitted_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	hash := pldtypes.RandBytes32()
	transactionSubmitted := &engineProto.TransactionSubmitted{
		TransactionId:   txID.String(),
		ContractAddress: contractAddr.String(),
		Hash:            hash[:],
	}
	payload, _ := proto.Marshal(transactionSubmitted)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_TransactionSubmitted,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.SubmittedEvent)
		return ok && event.TransactionID == txID
	})).Once()

	sm.handleTransactionSubmitted(ctx, message)

	mocks.originator.AssertExpectations(t)
}

func TestHandleTransactionConfirmed_Success(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	transactionConfirmed := &engineProto.TransactionConfirmed{
		TransactionId:   txID.String(),
		ContractAddress: contractAddr.String(),
		Outcome:         engineProto.TransactionConfirmed_OUTCOME_SUCCESS,
	}
	payload, _ := proto.Marshal(transactionConfirmed)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_TransactionConfirmed,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.ConfirmedSuccessEvent)
		return ok && event.TransactionID == txID
	})).Once()

	sm.handleTransactionConfirmed(ctx, message)

	mocks.originator.AssertExpectations(t)
}

func TestHandleTransactionConfirmed_Reverted(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	revertReason := pldtypes.HexBytes("test revert reason")
	failureMessage := "decoded revert"
	transactionConfirmed := &engineProto.TransactionConfirmed{
		TransactionId:   txID.String(),
		ContractAddress: contractAddr.String(),
		Outcome:         engineProto.TransactionConfirmed_OUTCOME_REVERTED,
		RevertReason:    revertReason,
		FailureMessage:  failureMessage,
		WillRetry:       true,
	}
	payload, _ := proto.Marshal(transactionConfirmed)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_TransactionConfirmed,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.ConfirmedRevertedEvent)
		return ok && event.TransactionID == txID && string(event.RevertReason) == string(revertReason) && event.FailureMessage == failureMessage && event.WillRetry
	})).Once()

	sm.handleTransactionConfirmed(ctx, message)

	mocks.originator.AssertExpectations(t)
}

func TestHandleTransactionConfirmed_RevertedWhenWillRetryTrueAndNoRevertReason(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	transactionConfirmed := &engineProto.TransactionConfirmed{
		TransactionId:   txID.String(),
		ContractAddress: contractAddr.String(),
		Outcome:         engineProto.TransactionConfirmed_OUTCOME_REVERTED,
		WillRetry:       true,
	}
	payload, _ := proto.Marshal(transactionConfirmed)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_TransactionConfirmed,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.ConfirmedRevertedEvent)
		return ok && event.TransactionID == txID && event.WillRetry && len(event.RevertReason) == 0
	})).Once()

	sm.handleTransactionConfirmed(ctx, message)

	mocks.originator.AssertExpectations(t)
}

func TestHandleDispatchedEvent_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	dispatchedEvent := &engineProto.TransactionDispatched{
		TransactionId:   txID.String(),
		ContractAddress: contractAddr.String(),
	}
	payload, _ := proto.Marshal(dispatchedEvent)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_Dispatched,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.DispatchedEvent)
		return ok && event.TransactionID == txID
	})).Once()

	sm.handleDispatchedEvent(ctx, message)

	mocks.originator.AssertExpectations(t)
}

func TestParseContractAddressString_Valid(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	message := &components.ReceivedMessage{
		FromNode: "test-node",
	}

	result := sm.parseContractAddressString(ctx, contractAddr.String(), message)
	require.NotNil(t, result)
	assert.Equal(t, contractAddr.String(), result.String())
}

func TestParseContractAddressString_Invalid(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	message := &components.ReceivedMessage{
		FromNode: "test-node",
	}

	result := sm.parseContractAddressString(ctx, "invalid-address", message)
	assert.Nil(t, result)
}

func TestHandleCoordinatorHeartbeatNotification_SequencerNotLoaded(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	heartbeatNotification := &engineProto.CoordinatorHeartbeatNotification{
		ContractAddress:     contractAddr.String(),
		CoordinatorSnapshot: &engineProto.CoordinatorSnapshot{},
	}
	payload, _ := proto.Marshal(heartbeatNotification)

	message := &components.ReceivedMessage{
		FromNode:    "coordinator-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_CoordinatorHeartbeatNotification,
		Payload:     payload,
	}

	// LoadSequencer is called; returning an error causes it to return (nil, nil)
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("not found")).Once()

	sm.handleCoordinatorHeartbeatNotification(ctx, message)
	assert.Empty(t, sm.sequencers)
}

func TestHandlePreDispatchRequest_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	requestID := uuid.New()

	preDispatchRequest := &engineProto.PreDispatchRequest{
		Id:              requestID.String(),
		TransactionId:   txID.String(),
		ContractAddress: contractAddr.String(),
	}
	payload, _ := proto.Marshal(preDispatchRequest)

	message := &components.ReceivedMessage{
		FromNode:    "coordinator-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_PreDispatchRequest,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originatorTransaction.PreDispatchRequestReceivedEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID && event.Coordinator == "coordinator-node"
	})).Once()

	sm.handlePreDispatchRequest(ctx, message)

	mocks.originator.AssertExpectations(t)
}

func TestHandlePreDispatchResponse_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	requestID := uuid.New()

	preDispatchResponse := &engineProto.PreDispatchResponse{
		Id:              requestID.String(),
		TransactionId:   txID.String(),
		ContractAddress: contractAddr.String(),
	}
	payload, _ := proto.Marshal(preDispatchResponse)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_PreDispatchResponse,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.DispatchRequestApprovedEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID
	})).Once()

	sm.handlePreDispatchResponse(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleDelegationResponse_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	txID := uuid.New()
	delegationRequestAcknowledgment := &engineProto.DelegationResponse{
		TransactionIds: []string{txID.String()},
	}
	payload, _ := proto.Marshal(delegationRequestAcknowledgment)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationResponse,
		Payload:     payload,
	}

	// Should not panic - this handler just logs
	sm.handleDelegationResponse(ctx, message)
}

func TestHandleDelegationResponse_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationResponse,
		Payload:     []byte("invalid-proto"),
	}

	// Should not panic
	sm.handleDelegationResponse(ctx, message)
}

func TestHandleDelegationResponse_MaxInFlightRejection(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	txID1 := uuid.New().String()
	txID2 := uuid.New().String()
	delegationRequestAcknowledgment := &engineProto.DelegationResponse{
		TransactionIds: []string{txID1, txID2},
		Errors: []int64{
			int64(coordinator.DelegationAcknowledgementError_MaxInflightTransactions),
			int64(coordinator.DelegationAcknowledgementError_MaxInflightTransactions),
		},
	}
	payload, err := proto.Marshal(delegationRequestAcknowledgment)
	require.NoError(t, err)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationResponse,
		Payload:     payload,
	}

	// Should not panic; handler logs max in flight rejections
	sm.handleDelegationResponse(ctx, message)
}

func TestHandleDelegationResponse_UnknownError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	txID := uuid.New().String()
	unknownErrorCode := int64(99)
	delegationRequestAcknowledgment := &engineProto.DelegationResponse{
		TransactionIds: []string{txID},
		Errors:         []int64{unknownErrorCode},
	}
	payload, err := proto.Marshal(delegationRequestAcknowledgment)
	require.NoError(t, err)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationResponse,
		Payload:     payload,
	}

	sm.handleDelegationResponse(ctx, message)
}

func TestHandleDelegationResponse_MixedErrors(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	txIDAccepted := uuid.New().String()
	txIDMaxInFlight := uuid.New().String()
	txIDUnknown := uuid.New().String()
	delegationRequestAcknowledgment := &engineProto.DelegationResponse{
		TransactionIds: []string{txIDAccepted, txIDMaxInFlight, txIDUnknown},
		Errors: []int64{
			int64(coordinator.DelegationAcknowledgementError_None),
			int64(coordinator.DelegationAcknowledgementError_MaxInflightTransactions),
			42, // unknown error code
		},
	}
	payload, err := proto.Marshal(delegationRequestAcknowledgment)
	require.NoError(t, err)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationResponse,
		Payload:     payload,
	}

	sm.handleDelegationResponse(ctx, message)
}

func TestHandleDelegationResponse_Empty(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	delegationRequestAcknowledgment := &engineProto.DelegationResponse{
		TransactionIds: []string{},
		Errors:         []int64{},
	}
	payload, err := proto.Marshal(delegationRequestAcknowledgment)
	require.NoError(t, err)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_DelegationResponse,
		Payload:     payload,
	}

	sm.handleDelegationResponse(ctx, message)
}

func TestHandleEndorsementRequest_Success_Sign(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	idempotencyKey := uuid.New().String()
	party := "party@test-node"

	txSpec := &prototk.TransactionSpecification{TransactionId: specTransactionID(txID)}
	verifier := &prototk.ResolvedVerifier{Lookup: "verifier@node1"}
	signature := &prototk.AttestationResult{Name: "sig1"}
	state := &prototk.EndorsableState{Id: "state1"}
	attestationRequest := &prototk.AttestationRequest{
		Name: "endorsement1", Algorithm: "ECDSA", VerifierType: "eth_address",
		PayloadType: "raw", AttestationType: prototk.AttestationType_ENDORSE,
	}

	endorsementRequest := &engineProto.EndorsementRequest{
		IdempotencyKey:  idempotencyKey,
		ContractAddress: contractAddr.String(), Party: party,
		TransactionSpecification: txSpec,
		Verifiers:                []*prototk.ResolvedVerifier{verifier}, Signatures: []*prototk.AttestationResult{signature},
		InputStates: []*prototk.EndorsableState{state}, ReadStates: []*prototk.EndorsableState{state},
		OutputStates: []*prototk.EndorsableState{state}, InfoStates: []*prototk.EndorsableState{state},
		AttestationRequest: attestationRequest,
	}
	payload, _ := proto.Marshal(endorsementRequest)
	message := &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest, Payload: payload,
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.EndorsementRequestReceivedEvent)
		return ok && event.TransactionId == txID && event.IdempotencyKey == idempotencyKey
	})).Once()

	sm.handleEndorsementRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleEndorsementRequest_Success_Revert(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	idempotencyKey := uuid.New().String()
	party := "party@test-node"

	txSpec := &prototk.TransactionSpecification{TransactionId: specTransactionID(txID)}
	verifier := &prototk.ResolvedVerifier{Lookup: "verifier@node1"}
	signature := &prototk.AttestationResult{Name: "sig1"}
	state := &prototk.EndorsableState{Id: "state1"}
	attestationRequest := &prototk.AttestationRequest{
		Name: "endorsement1", Algorithm: "ECDSA", VerifierType: "eth_address",
		PayloadType: "raw", AttestationType: prototk.AttestationType_ENDORSE,
	}

	endorsementRequest := &engineProto.EndorsementRequest{
		IdempotencyKey:  idempotencyKey,
		ContractAddress: contractAddr.String(), Party: party,
		TransactionSpecification: txSpec,
		Verifiers:                []*prototk.ResolvedVerifier{verifier}, Signatures: []*prototk.AttestationResult{signature},
		InputStates: []*prototk.EndorsableState{state}, ReadStates: []*prototk.EndorsableState{state},
		OutputStates: []*prototk.EndorsableState{state}, InfoStates: []*prototk.EndorsableState{state},
		AttestationRequest: attestationRequest,
	}
	payload, _ := proto.Marshal(endorsementRequest)
	message := &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest, Payload: payload,
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.EndorsementRequestReceivedEvent)
		return ok && event.FromNode == "coordinator-node" && event.TransactionId == txID && event.IdempotencyKey == idempotencyKey && event.Party == party
	})).Once()

	sm.handleEndorsementRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleEndorsementRequest_Success_EndorserSubmit(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	idempotencyKey := uuid.New().String()
	party := "party@test-node"

	txSpec := &prototk.TransactionSpecification{TransactionId: specTransactionID(txID)}
	verifier := &prototk.ResolvedVerifier{Lookup: "verifier@node1"}
	signature := &prototk.AttestationResult{Name: "sig1"}
	state := &prototk.EndorsableState{Id: "state1"}
	attestationRequest := &prototk.AttestationRequest{
		Name: "endorsement1", Algorithm: "ECDSA", VerifierType: "eth_address",
		PayloadType: "raw", AttestationType: prototk.AttestationType_ENDORSE,
	}

	endorsementRequest := &engineProto.EndorsementRequest{
		IdempotencyKey:  idempotencyKey,
		ContractAddress: contractAddr.String(), Party: party,
		TransactionSpecification: txSpec,
		Verifiers:                []*prototk.ResolvedVerifier{verifier}, Signatures: []*prototk.AttestationResult{signature},
		InputStates: []*prototk.EndorsableState{state}, ReadStates: []*prototk.EndorsableState{state},
		OutputStates: []*prototk.EndorsableState{state}, InfoStates: []*prototk.EndorsableState{state},
		AttestationRequest: attestationRequest,
	}
	payload, _ := proto.Marshal(endorsementRequest)
	message := &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest, Payload: payload,
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.EndorsementRequestReceivedEvent)
		return ok && event.TransactionId == txID && event.IdempotencyKey == idempotencyKey && event.Party == party
	})).Once()

	sm.handleEndorsementRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleEndorsementRequest_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     []byte("invalid-proto"),
	}

	// Should not panic
	sm.handleEndorsementRequest(ctx, message)
}

func TestHandleEndorsementRequest_InvalidContractAddress(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	endorsementRequest := &engineProto.EndorsementRequest{
		ContractAddress: "invalid-address",
	}
	payload, _ := proto.Marshal(endorsementRequest)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest,
		Payload:     payload,
	}

	// Should not panic
	sm.handleEndorsementRequest(ctx, message)
}

func TestHandleEndorsementRequest_InvalidTransactionSpecificationID(t *testing.T) {
	txID := uuid.New()
	trailingBytes := pldtypes.Bytes32UUIDFirst16(txID)
	trailingBytes[31] = 1
	canonical := pldtypes.Bytes32UUIDFirst16(txID).String()

	for name, specTxID := range map[string]string{
		"missing":          "",
		"not hex":          "not-hex",
		"not 32 bytes":     "0x" + hex.EncodeToString(txID[:]),
		"non-zero padding": trailingBytes.String(),
		"upper case":       "0x" + strings.ToUpper(canonical[2:]),
		"no 0x prefix":     canonical[2:],
	} {
		t.Run(name, func(t *testing.T) {
			mocks := newTransportClientTestMocks(t)
			sm := newSequencerManagerForTransportClientTesting(t, mocks)
			payload, _ := proto.Marshal(&engineProto.EndorsementRequest{
				ContractAddress:          pldtypes.RandAddress().String(),
				TransactionSpecification: &prototk.TransactionSpecification{TransactionId: specTxID},
			})
			sm.handleEndorsementRequest(t.Context(), &components.ReceivedMessage{MessageType: transport.MessageType_EndorsementRequest, Payload: payload})
		})
	}
}

func TestHandleEndorsementRequest_NilTransactionSpecification(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.EndorsementRequest{
		ContractAddress: pldtypes.RandAddress().String(),
	})
	sm.handleEndorsementRequest(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_EndorsementRequest, Payload: payload})
}

// TestHandleEndorsementRequest_QueuesEventWithDecodedFields verifies that a valid endorsement
// request results in an EndorsementRequestReceivedEvent being queued on the coordinator with
// the correct decoded field values. (The former EndorseTransactionError test is superseded by
// tests in coordinator/endorsing_test.go now that that logic lives in the coordinator.)
func TestHandleEndorsementRequest_QueuesEventWithDecodedFields(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	idempotencyKey := uuid.New().String()
	party := "party@test-node"

	txSpec := &prototk.TransactionSpecification{TransactionId: specTransactionID(txID)}
	verifier := &prototk.ResolvedVerifier{Lookup: "verifier@node1"}
	signature := &prototk.AttestationResult{Name: "sig1"}
	state := &prototk.EndorsableState{Id: "state1"}
	attestationRequest := &prototk.AttestationRequest{
		Name: "endorsement1", Algorithm: "ECDSA", VerifierType: "eth_address",
		PayloadType: "raw", AttestationType: prototk.AttestationType_ENDORSE,
	}

	endorsementRequest := &engineProto.EndorsementRequest{
		IdempotencyKey:  idempotencyKey,
		ContractAddress: contractAddr.String(), Party: party,
		TransactionSpecification: txSpec,
		Verifiers:                []*prototk.ResolvedVerifier{verifier}, Signatures: []*prototk.AttestationResult{signature},
		InputStates: []*prototk.EndorsableState{state}, ReadStates: []*prototk.EndorsableState{state},
		OutputStates: []*prototk.EndorsableState{state}, InfoStates: []*prototk.EndorsableState{state},
		AttestationRequest: attestationRequest,
	}
	payload, _ := proto.Marshal(endorsementRequest)
	message := &components.ReceivedMessage{
		FromNode: "test-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest, Payload: payload,
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	var capturedEvent *coordinator.EndorsementRequestReceivedEvent
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		var ok bool
		capturedEvent, ok = e.(*coordinator.EndorsementRequestReceivedEvent)
		return ok
	})).Once()

	sm.handleEndorsementRequest(ctx, message)

	require.NotNil(t, capturedEvent)
	assert.Equal(t, "test-node", capturedEvent.FromNode)
	assert.Equal(t, txID, capturedEvent.TransactionId)
	assert.Equal(t, idempotencyKey, capturedEvent.IdempotencyKey)
	assert.Equal(t, party, capturedEvent.Party)
	assert.NotNil(t, capturedEvent.PrivateEndorsementRequest)
	assert.Equal(t, 1, len(capturedEvent.PrivateEndorsementRequest.Verifiers))
	assert.Equal(t, 1, len(capturedEvent.PrivateEndorsementRequest.InputStates))
	mocks.coordinator.AssertExpectations(t)
}

func TestHandleEndorsementRequest_LoadSequencerError(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	party := "party@test-node"

	txSpec := &prototk.TransactionSpecification{TransactionId: specTransactionID(txID)}
	verifier := &prototk.ResolvedVerifier{Lookup: "verifier@node1"}
	signature := &prototk.AttestationResult{Name: "sig1"}
	state := &prototk.EndorsableState{Id: "state1"}
	attestationRequest := &prototk.AttestationRequest{
		Name: "endorsement1", Algorithm: "ECDSA", VerifierType: "eth_address",
		PayloadType: "raw", AttestationType: prototk.AttestationType_ENDORSE,
	}

	endorsementRequest := &engineProto.EndorsementRequest{
		IdempotencyKey: uuid.NewString(), ContractAddress: contractAddr.String(), Party: party,
		TransactionSpecification: txSpec,
		Verifiers:                []*prototk.ResolvedVerifier{verifier}, Signatures: []*prototk.AttestationResult{signature},
		InputStates: []*prototk.EndorsableState{state}, ReadStates: []*prototk.EndorsableState{state},
		OutputStates: []*prototk.EndorsableState{state}, InfoStates: []*prototk.EndorsableState{state},
		AttestationRequest: attestationRequest,
	}
	payload, _ := proto.Marshal(endorsementRequest)
	message := &components.ReceivedMessage{
		FromNode: "test-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest, Payload: payload,
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()

	// LoadSequencer returns nil because the existence check (GetSmartContractByAddress) errors.
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("not found")).Once()

	// QueueEvent must NOT be called because LoadSequencer returned nil.
	sm.handleEndorsementRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

// TestHandleEndorsementRequest_SendEndorsementResponseError was testing a scenario that now
// lives inside coordinator/endorsing.go (covered by endorsing_test.go). This test is repurposed
// to verify that the endorsement request event is queued even when the from-node field differs
// from the coordinator's own node name.
func TestHandleEndorsementRequest_SendEndorsementResponseError(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	idempotencyKey := uuid.New().String()
	party := "party@test-node"

	txSpec := &prototk.TransactionSpecification{TransactionId: specTransactionID(txID)}
	attestationRequest := &prototk.AttestationRequest{
		Name: "endorsement1", Algorithm: "ECDSA", VerifierType: "eth_address",
	}

	endorsementRequest := &engineProto.EndorsementRequest{
		IdempotencyKey:  idempotencyKey,
		ContractAddress: contractAddr.String(), Party: party,
		TransactionSpecification: txSpec, AttestationRequest: attestationRequest,
	}
	payload, _ := proto.Marshal(endorsementRequest)
	message := &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest, Payload: payload,
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		_, ok := e.(*coordinator.EndorsementRequestReceivedEvent)
		return ok
	})).Once()

	sm.handleEndorsementRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

// TestHandleEndorsementRequest_SignValidationError verifies successful queueing when the party
// field contains a node name different from the local node.
func TestHandleEndorsementRequest_SignValidationError(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New().String()
	party := "party@other-node" // Different node — key is still resolved locally by the endorser

	txSpec := &prototk.TransactionSpecification{TransactionId: specTransactionID(txID)}
	attestationRequest := &prototk.AttestationRequest{
		Name: "endorsement1", Algorithm: "ECDSA", VerifierType: "eth_address",
	}

	endorsementRequest := &engineProto.EndorsementRequest{
		IdempotencyKey: uuid.NewString(), ContractAddress: contractAddr.String(), Party: party,
		TransactionSpecification: txSpec, AttestationRequest: attestationRequest,
	}
	payload, _ := proto.Marshal(endorsementRequest)
	message := &components.ReceivedMessage{
		FromNode: "test-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_EndorsementRequest, Payload: payload,
	}

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.EndorsementRequestReceivedEvent)
		return ok && event.Party == party
	})).Once()

	sm.handleEndorsementRequest(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

// Test handleEndorsementResponse
func TestHandleEndorsementResponse_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	idempotencyKey := uuid.New()
	attestationRequestName := "endorsement1"

	// Create endorsement
	endorsement := &prototk.AttestationResult{
		Name: "test-endorsement",
	}

	endorsementResponse := &engineProto.EndorsementResponse{
		TransactionId:          txID.String(),
		IdempotencyKey:         idempotencyKey.String(),
		ContractAddress:        contractAddr.String(),
		Endorsement:            endorsement,
		AttestationRequestName: attestationRequestName,
	}
	payload, _ := proto.Marshal(endorsementResponse)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementResponse,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.EndorsedEvent)
		return ok && event.TransactionID == txID && event.RequestID == idempotencyKey && event.Endorsement != nil
	})).Once()

	sm.handleEndorsementResponse(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleEndorsementResponse_Revert(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	idempotencyKey := uuid.New()
	revertReason := "transaction reverted"
	attestationRequestName := "endorsement1"

	endorsementResponse := &engineProto.EndorsementResponse{
		TransactionId:          txID.String(),
		IdempotencyKey:         idempotencyKey.String(),
		ContractAddress:        contractAddr.String(),
		RevertReason:           &revertReason,
		AttestationRequestName: attestationRequestName,
	}
	payload, _ := proto.Marshal(endorsementResponse)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementResponse,
		Payload:     payload,
	}

	// GetSequencer is used - sequencer must already be in memory
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.EndorseRevertEvent)
		return ok && event.TransactionID == txID && event.RequestID == idempotencyKey && event.RevertReason == revertReason && event.AttestationRequestName == attestationRequestName
	})).Once()

	sm.handleEndorsementResponse(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleEndorsementResponse_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementResponse,
		Payload:     []byte("invalid-proto"),
	}

	// Should not panic
	sm.handleEndorsementResponse(ctx, message)
}

func TestHandleEndorsementResponse_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	txID := uuid.New()
	idempotencyKey := uuid.New()

	endorsementResponse := &engineProto.EndorsementResponse{
		TransactionId:   txID.String(),
		IdempotencyKey:  idempotencyKey.String(),
		ContractAddress: "invalid-address",
	}
	payload, _ := proto.Marshal(endorsementResponse)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementResponse,
		Payload:     payload,
	}

	// Should not panic
	sm.handleEndorsementResponse(ctx, message)
}

func TestHandleEndorsementResponse_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	idempotencyKey := uuid.New()

	endorsementResponse := &engineProto.EndorsementResponse{
		TransactionId:   txID.String(),
		IdempotencyKey:  idempotencyKey.String(),
		ContractAddress: contractAddr.String(),
	}
	payload, _ := proto.Marshal(endorsementResponse)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementResponse,
		Payload:     payload,
	}

	// GetSequencer is used and returns nil since no sequencer is in memory - should not panic
	sm.handleEndorsementResponse(ctx, message)
}

// ===== handleEndorsementError Tests =====

func TestHandleEndorsementError_QueuesToCoordinator(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	idempotencyKey := uuid.New()

	endorsementError := &engineProto.EndorsementError{
		TransactionId:   txID.String(),
		IdempotencyKey:  idempotencyKey.String(),
		ContractAddress: contractAddr.String(),
		ErrorMessage:    "domain returned unexpected error",
	}
	payload, _ := proto.Marshal(endorsementError)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementError,
		Payload:     payload,
	}

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.EndorseErrorEvent)
		return ok && event.TransactionID == txID && event.RequestID == idempotencyKey
	})).Once()

	sm.handleEndorsementError(ctx, message)

	mocks.coordinator.AssertExpectations(t)
}

func TestHandleEndorsementError_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementError,
		Payload:     []byte("invalid-proto"),
	}

	// Should not panic
	sm.handleEndorsementError(ctx, message)
}

func TestHandleEndorsementError_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	idempotencyKey := uuid.New()

	endorsementError := &engineProto.EndorsementError{
		TransactionId:   txID.String(),
		IdempotencyKey:  idempotencyKey.String(),
		ContractAddress: contractAddr.String(),
		ErrorMessage:    "some error",
	}
	payload, _ := proto.Marshal(endorsementError)

	message := &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_EndorsementError,
		Payload:     payload,
	}

	// GetSequencer returns nil since no sequencer is in memory - should not panic
	sm.handleEndorsementError(ctx, message)
}

func TestHandleAssembleResponse_Park(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	requestID := uuid.New()
	assembleResponse := &engineProto.AssembleResponse{
		TransactionId: txID.String(), AssembleRequestId: requestID.String(),
		ContractAddress: contractAddr.String(),
		PostAssembly: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_PARK,
		},
	}
	payload, _ := proto.Marshal(assembleResponse)

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq
	sm.handleAssembleResponse(ctx, &components.ReceivedMessage{
		FromNode: "test-node", MessageType: transport.MessageType_AssembleResponse, Payload: payload,
	})
}

func TestHandleAssembleResponse_UnexpectedResult(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	requestID := uuid.New()
	assembleResponse := &engineProto.AssembleResponse{
		TransactionId: txID.String(), AssembleRequestId: requestID.String(),
		ContractAddress: contractAddr.String(),
		PostAssembly: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_Result(999),
		},
	}
	payload, _ := proto.Marshal(assembleResponse)

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq
	sm.handleAssembleResponse(ctx, &components.ReceivedMessage{
		FromNode: "test-node", MessageType: transport.MessageType_AssembleResponse, Payload: payload,
	})
}

func TestHandleAssembleRejection_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	reqID := uuid.New()

	msg := &engineProto.AssembleRejection{
		ContractAddress:        contractAddr.String(),
		TransactionId:          txID.String(),
		AssembleRequestId:      reqID.String(),
		RejectionReason:        engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE,
		CoordinatorBlockHeight: 10,
		AssemblerBlockHeight:   9,
	}
	payload, err := proto.Marshal(msg)
	require.NoError(t, err)

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq
	done := make(chan struct{})
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.AssembleRequestRejectedEvent)
		if ok {
			close(done)
			return event.TransactionID == txID && event.RequestID == reqID
		}
		return false
	})).Once()

	sm.handleAssembleRejection(ctx, &components.ReceivedMessage{
		FromNode: "other-node", MessageType: transport.MessageType_AssembleRejection, Payload: payload,
	})
	<-done
}

func TestHandleDelegationRejection_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	rejection := &engineProto.DelegationRejection{
		ContractAddress:        contractAddr.String(),
		ActiveCoordinator:      "coord@node2",
		RejectionReason:        engineProto.RejectionReason_NOT_CURRENT_DELEGATE,
		OriginatorBlockHeight:  5,
		CoordinatorBlockHeight: 10,
		BlockHeightTolerance:   2,
	}
	payload, err := proto.Marshal(rejection)
	require.NoError(t, err)

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq
	done := make(chan struct{})
	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		_, ok := e.(*originator.DelegationRequestRejectedEvent)
		if ok {
			close(done)
		}
		return ok
	})).Once()

	sm.handleDelegationRejection(ctx, &components.ReceivedMessage{
		FromNode: "other-node", MessageType: transport.MessageType_DelegationRejection, Payload: payload,
	})
	<-done
}

func TestHandleHandoverRequest_Success(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	handover := &engineProto.CoordinatorHandoverRequest{
		ContractAddress: contractAddr.String(),
	}
	payload, err := proto.Marshal(handover)
	require.NoError(t, err)

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq
	done := make(chan struct{})
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.HandoverRequestEvent)
		if ok && event.FromNode == "other-node" {
			close(done)
		}
		return ok
	})).Once()

	sm.handleHandoverRequest(ctx, &components.ReceivedMessage{
		FromNode: "other-node", MessageType: transport.MessageType_HandoverRequest, Payload: payload,
	})
	<-done
}

func TestHandleEndorsementRejection_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	reqID := uuid.New()

	rejection := &engineProto.EndorsementRejection{
		ContractAddress:        contractAddr.String(),
		TransactionId:          txID.String(),
		IdempotencyKey:         reqID.String(),
		Party:                  "party@test-node",
		AttestationRequestName: "endorse1",
		RejectionReason:        engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE,
		CoordinatorBlockHeight: 10,
		EndorserBlockHeight:    9,
		BlockHeightTolerance:   2,
	}
	payload, err := proto.Marshal(rejection)
	require.NoError(t, err)

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq
	done := make(chan struct{})
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.EndorseRequestRejectedEvent)
		if ok && event.TransactionID == txID {
			close(done)
		}
		return ok
	})).Once()

	sm.handleEndorsementRejection(ctx, &components.ReceivedMessage{
		FromNode: "other-node", MessageType: transport.MessageType_EndorsementRejection, Payload: payload,
	})
	<-done
}

func TestHandlePreDispatchRejection_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	reqID := uuid.New()

	rejection := &engineProto.PreDispatchRejection{
		ContractAddress: contractAddr.String(),
		TransactionId:   txID.String(),
		RequestId:       reqID.String(),
		RejectionReason: 1,
	}
	payload, err := proto.Marshal(rejection)
	require.NoError(t, err)

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq
	done := make(chan struct{})
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.PreDispatchRequestRejectedEvent)
		if ok && event.TransactionID == txID && event.RequestID == reqID {
			close(done)
		}
		return ok
	})).Once()

	sm.handlePreDispatchRejection(ctx, &components.ReceivedMessage{
		FromNode: "other-node", MessageType: transport.MessageType_PreDispatchRejection, Payload: payload,
	})
	<-done
}

func TestHandlePreDispatchRejection_InvalidTransactionID(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	rejection := &engineProto.PreDispatchRejection{
		ContractAddress: contractAddr.String(),
		TransactionId:   "not-a-uuid",
		RequestId:       uuid.New().String(),
	}
	payload, err := proto.Marshal(rejection)
	require.NoError(t, err)

	sm.handlePreDispatchRejection(ctx, &components.ReceivedMessage{
		FromNode: "other-node", MessageType: transport.MessageType_PreDispatchRejection, Payload: payload,
	})
}

func TestHandleCoordinatorHeartbeatNotification_UnparseableSnapshot(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	// A snapshot carrying a malformed transaction UUID must be dropped by CoordinatorSnapshotFromProto.
	heartbeatNotification := &engineProto.CoordinatorHeartbeatNotification{
		ContractAddress: contractAddr.String(),
		CoordinatorSnapshot: &engineProto.CoordinatorSnapshot{
			PooledTransactions: []*engineProto.SnapshotPooledTransaction{{Id: "not-a-uuid"}},
		},
	}
	payload, _ := proto.Marshal(heartbeatNotification)
	sm.handleCoordinatorHeartbeatNotification(ctx, &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageType: transport.MessageType_CoordinatorHeartbeatNotification, Payload: payload,
	})
}

func TestHandleDelegationResponse_CoordinatorErrors(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	delegationResponse := &engineProto.DelegationResponse{
		ContractAddress: contractAddr.String(),
		TransactionIds:  []string{uuid.New().String(), uuid.New().String()},
		Errors: []int64{
			int64(coordinator.DelegationAcknowledgementError_CoordinatorError),
			int64(coordinator.DelegationAcknowledgementError_PreviousTransactionError),
		},
	}
	payload, err := proto.Marshal(delegationResponse)
	require.NoError(t, err)

	sm.handleDelegationResponse(ctx, &components.ReceivedMessage{
		FromNode: "test-node", MessageType: transport.MessageType_DelegationResponse, Payload: payload,
	})
}

func TestHandleDelegationResponse_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	delegationResponse := &engineProto.DelegationResponse{
		ContractAddress: "invalid-address",
	}
	payload, err := proto.Marshal(delegationResponse)
	require.NoError(t, err)

	sm.handleDelegationResponse(ctx, &components.ReceivedMessage{
		FromNode: "test-node", MessageType: transport.MessageType_DelegationResponse, Payload: payload,
	})
}

func TestHandlePaladinMsg_RoutesAssembleRejection(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID, reqID := uuid.New(), uuid.New()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	payload, err := proto.Marshal(&engineProto.AssembleRejection{
		ContractAddress: contractAddr.String(), TransactionId: txID.String(), AssembleRequestId: reqID.String(),
	})
	require.NoError(t, err)

	done := make(chan struct{})
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.AssembleRequestRejectedEvent)
		if ok && event.TransactionID == txID && event.RequestID == reqID {
			close(done)
		}
		return ok
	})).Once()

	sm.HandlePaladinMsg(ctx, &components.ReceivedMessage{
		FromNode: "other-node", MessageType: transport.MessageType_AssembleRejection, Payload: payload,
	})
	<-done
}

func TestHandlePaladinMsg_RoutesDelegationRejection(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	payload, err := proto.Marshal(&engineProto.DelegationRejection{
		ContractAddress: contractAddr.String(), ActiveCoordinator: "coord@node2",
	})
	require.NoError(t, err)

	done := make(chan struct{})
	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*originator.DelegationRequestRejectedEvent)
		if ok && event.ActiveCoordinator == "coord@node2" {
			close(done)
		}
		return ok
	})).Once()

	sm.HandlePaladinMsg(ctx, &components.ReceivedMessage{
		FromNode: "other-node", MessageType: transport.MessageType_DelegationRejection, Payload: payload,
	})
	<-done
}

func TestHandlePaladinMsg_RoutesHandoverRequest(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	payload, err := proto.Marshal(&engineProto.CoordinatorHandoverRequest{
		ContractAddress: contractAddr.String(),
	})
	require.NoError(t, err)

	done := make(chan struct{})
	mocks.coordinator.EXPECT().QueueEvent(mock.Anything, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordinator.HandoverRequestEvent)
		if ok && event.FromNode == "other-node" {
			close(done)
		}
		return ok
	})).Once()

	sm.HandlePaladinMsg(ctx, &components.ReceivedMessage{
		FromNode: "other-node", MessageType: transport.MessageType_HandoverRequest, Payload: payload,
	})
	<-done
}

func TestHandlePaladinMsg_RoutesEndorsementRejection(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID, reqID := uuid.New(), uuid.New()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	payload, err := proto.Marshal(&engineProto.EndorsementRejection{
		ContractAddress: contractAddr.String(), TransactionId: txID.String(), IdempotencyKey: reqID.String(),
		Party: "party@test-node",
	})
	require.NoError(t, err)

	done := make(chan struct{})
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.EndorseRequestRejectedEvent)
		if ok && event.TransactionID == txID && event.RequestID == reqID {
			close(done)
		}
		return ok
	})).Once()

	sm.HandlePaladinMsg(ctx, &components.ReceivedMessage{
		FromNode: "other-node", MessageType: transport.MessageType_EndorsementRejection, Payload: payload,
	})
	<-done
}

func TestHandlePaladinMsg_RoutesPreDispatchRejection(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID, reqID := uuid.New(), uuid.New()

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	payload, err := proto.Marshal(&engineProto.PreDispatchRejection{
		ContractAddress: contractAddr.String(), TransactionId: txID.String(), RequestId: reqID.String(),
	})
	require.NoError(t, err)

	done := make(chan struct{})
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.PreDispatchRequestRejectedEvent)
		if ok && event.TransactionID == txID && event.RequestID == reqID {
			close(done)
		}
		return ok
	})).Once()

	sm.HandlePaladinMsg(ctx, &components.ReceivedMessage{
		FromNode: "other-node", MessageType: transport.MessageType_PreDispatchRejection, Payload: payload,
	})
	<-done
}

func TestHandlePaladinMsg_UnknownMessageType(t *testing.T) {
	ctx := context.Background()
	sm := newSequencerManagerForTransportClientTesting(t, newTransportClientTestMocks(t))
	// Unknown message type should log an error and not panic
	sm.HandlePaladinMsg(ctx, &components.ReceivedMessage{
		FromNode: "other-node", MessageType: "TotallyUnknownType", Payload: []byte("x"),
	})
}

func TestHandleAssembleRejection_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	sm := newSequencerManagerForTransportClientTesting(t, newTransportClientTestMocks(t))
	sm.handleAssembleRejection(ctx, &components.ReceivedMessage{
		MessageType: transport.MessageType_AssembleRejection, Payload: []byte("bad"),
	})
}

func TestHandleAssembleRejection_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	msg := &engineProto.AssembleRejection{
		ContractAddress: contractAddr.String(), TransactionId: uuid.New().String(), AssembleRequestId: uuid.New().String(),
	}
	payload, _ := proto.Marshal(msg)
	sm.handleAssembleRejection(ctx, &components.ReceivedMessage{
		MessageType: transport.MessageType_AssembleRejection, Payload: payload,
	})
}

func TestHandleDelegationRejection_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	sm := newSequencerManagerForTransportClientTesting(t, newTransportClientTestMocks(t))
	sm.handleDelegationRejection(ctx, &components.ReceivedMessage{
		MessageType: transport.MessageType_DelegationRejection, Payload: []byte("bad"),
	})
}

func TestHandleHandoverRequest_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	sm := newSequencerManagerForTransportClientTesting(t, newTransportClientTestMocks(t))
	sm.handleHandoverRequest(ctx, &components.ReceivedMessage{
		MessageType: transport.MessageType_HandoverRequest, Payload: []byte("bad"),
	})
}

func TestHandleHandoverRequest_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	sm := newSequencerManagerForTransportClientTesting(t, newTransportClientTestMocks(t))
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.CoordinatorHandoverRequest{ContractAddress: contractAddr.String()})
	sm.handleHandoverRequest(ctx, &components.ReceivedMessage{
		MessageType: transport.MessageType_HandoverRequest, Payload: payload,
	})
}

func TestHandleEndorsementRejection_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	sm := newSequencerManagerForTransportClientTesting(t, newTransportClientTestMocks(t))
	sm.handleEndorsementRejection(ctx, &components.ReceivedMessage{
		MessageType: transport.MessageType_EndorsementRejection, Payload: []byte("bad"),
	})
}

func TestHandlePreDispatchRejection_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	sm := newSequencerManagerForTransportClientTesting(t, newTransportClientTestMocks(t))
	sm.handlePreDispatchRejection(ctx, &components.ReceivedMessage{
		MessageType: transport.MessageType_PreDispatchRejection, Payload: []byte("bad"),
	})
}

func TestHandlePreDispatchRejection_InvalidRequestID(t *testing.T) {
	ctx := context.Background()
	sm := newSequencerManagerForTransportClientTesting(t, newTransportClientTestMocks(t))
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.PreDispatchRejection{
		ContractAddress: contractAddr.String(), TransactionId: uuid.New().String(), RequestId: "bad-id",
	})
	sm.handlePreDispatchRejection(ctx, &components.ReceivedMessage{
		MessageType: transport.MessageType_PreDispatchRejection, Payload: payload,
	})
}

func TestHandleCoordinatorHeartbeatNotification_Success(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.CoordinatorHeartbeatNotification{
		ContractAddress: contractAddr.String(), CoordinatorSnapshot: &engineProto.CoordinatorSnapshot{},
	})
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mocks.domainAPI, nil).Once()
	fromCoordinatorNode := mock.MatchedBy(func(e *common.HeartbeatReceivedEvent) bool {
		return e.FromNode == "coordinator-node"
	})
	mocks.originator.EXPECT().QueueEvent(ctx, fromCoordinatorNode).Once()
	mocks.coordinator.EXPECT().QueueEvent(ctx, fromCoordinatorNode).Once()
	sm.handleCoordinatorHeartbeatNotification(ctx, &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageType: transport.MessageType_CoordinatorHeartbeatNotification, Payload: payload,
	})
}

func TestHandlePaladinMsg_EndorsementErrorRouting(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	sm.HandlePaladinMsg(ctx, &components.ReceivedMessage{
		FromNode: "test-node", MessageType: transport.MessageType_EndorsementError, Payload: []byte("bad"),
	})
}

func TestHandleAssembleRequest_WithExpiry(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.AssembleRequest{
		TransactionId: uuid.New().String(), AssembleRequestId: uuid.New().String(),
		ContractAddress: contractAddr.String(), ExpiryTimeUnixMs: time.Now().Add(time.Hour).UnixMilli(),
	})
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq
	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		ev, ok := e.(*originatorTransaction.AssembleRequestReceivedEvent)
		return ok && !ev.Expiry.IsZero()
	})).Once()
	sm.handleAssembleRequest(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_AssembleRequest, Payload: payload})
}

func TestHandleAssembleResponse_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.AssembleResponse{
		ContractAddress: "invalid", TransactionId: uuid.New().String(), AssembleRequestId: uuid.New().String(),
		PostAssembly: &prototk.TransactionPostAssembly{AssemblyResult: prototk.AssembleTransactionResponse_OK},
	})
	sm.handleAssembleResponse(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_AssembleResponse, Payload: payload})
}

func TestHandleAssembleResponse_NilPostAssembly(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.AssembleResponse{
		ContractAddress: contractAddr.String(), TransactionId: uuid.New().String(), AssembleRequestId: uuid.New().String(),
		// PostAssembly intentionally nil
	})
	sm.handleAssembleResponse(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_AssembleResponse, Payload: payload})
}

func TestHandleAssembleResponse_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.AssembleResponse{
		ContractAddress: contractAddr.String(), TransactionId: uuid.New().String(), AssembleRequestId: uuid.New().String(),
		PostAssembly: &prototk.TransactionPostAssembly{AssemblyResult: prototk.AssembleTransactionResponse_OK},
	})
	sm.handleAssembleResponse(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_AssembleResponse, Payload: payload})
}

func TestHandleAssembleError_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.AssembleError{ContractAddress: "invalid", TransactionId: uuid.New().String(), AssembleRequestId: uuid.New().String()})
	sm.handleAssembleError(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_AssembleError, Payload: payload})
}

func TestHandleAssembleError_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.AssembleError{ContractAddress: contractAddr.String(), TransactionId: uuid.New().String(), AssembleRequestId: uuid.New().String()})
	sm.handleAssembleError(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_AssembleError, Payload: payload})
}

func TestHandleAssembleRejection_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.AssembleRejection{ContractAddress: "invalid", TransactionId: uuid.New().String(), AssembleRequestId: uuid.New().String()})
	sm.handleAssembleRejection(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_AssembleRejection, Payload: payload})
}

func TestHandleCoordinatorHeartbeatNotification_InvalidContractAddress(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.CoordinatorHeartbeatNotification{ContractAddress: "invalid", CoordinatorSnapshot: &engineProto.CoordinatorSnapshot{}})
	sm.handleCoordinatorHeartbeatNotification(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_CoordinatorHeartbeatNotification, Payload: payload})
}

func TestHandlePreDispatchRequest_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.PreDispatchRequest{ContractAddress: "invalid", Id: uuid.New().String(), TransactionId: uuid.New().String()})
	sm.handlePreDispatchRequest(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_PreDispatchRequest, Payload: payload})
}

func TestHandlePreDispatchRequest_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.PreDispatchRequest{ContractAddress: contractAddr.String(), Id: uuid.New().String(), TransactionId: uuid.New().String()})
	sm.handlePreDispatchRequest(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_PreDispatchRequest, Payload: payload})
}

func TestHandlePreDispatchResponse_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.PreDispatchResponse{ContractAddress: "invalid", Id: uuid.New().String(), TransactionId: uuid.New().String()})
	sm.handlePreDispatchResponse(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_PreDispatchResponse, Payload: payload})
}

func TestHandlePreDispatchResponse_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.PreDispatchResponse{ContractAddress: contractAddr.String(), Id: uuid.New().String(), TransactionId: uuid.New().String()})
	sm.handlePreDispatchResponse(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_PreDispatchResponse, Payload: payload})
}

func TestHandleDispatchedEvent_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.TransactionDispatched{ContractAddress: "invalid", TransactionId: uuid.New().String()})
	sm.handleDispatchedEvent(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_Dispatched, Payload: payload})
}

func TestHandleDispatchedEvent_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.TransactionDispatched{ContractAddress: contractAddr.String(), TransactionId: uuid.New().String()})
	sm.handleDispatchedEvent(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_Dispatched, Payload: payload})
}

func TestHandleDelegationRequest_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.DelegationRequest{ContractAddress: "invalid", OriginatorBlockHeight: 1})
	sm.handleDelegationRequest(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_DelegationRequest, Payload: payload})
}

func TestHandleDelegationRequest_LoadSequencerError(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	message := newDelegationRequestMessage("node1", contractAddr, 100, newTestPrivateTx(contractAddr))
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("not found")).Once()
	sm.handleDelegationRequest(ctx, message)
}

func TestHandleDelegationRequest_NilPreAssembly(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.DelegationRequest{
		ContractAddress:       contractAddr.HexString(),
		OriginatorBlockHeight: 100,
		Transactions: []*engineProto.PrivateTransactionDelegation{
			{Domain: "test-domain"},
		},
	})
	sm.handleDelegationRequest(ctx, &components.ReceivedMessage{
		MessageType: transport.MessageType_DelegationRequest,
		Payload:     payload,
	})
}

func TestHandleDelegationRequest_NilTransactionSpecification(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.DelegationRequest{
		ContractAddress:       contractAddr.HexString(),
		OriginatorBlockHeight: 100,
		Transactions: []*engineProto.PrivateTransactionDelegation{
			{
				PreAssembly: &prototk.TransactionPreAssembly{}, // no TransactionSpecification
			},
		},
	})
	sm.handleDelegationRequest(ctx, &components.ReceivedMessage{
		MessageType: transport.MessageType_DelegationRequest,
		Payload:     payload,
	})
}

func TestHandleDelegationResponse_MaxInFlightWithContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.DelegationResponse{
		ContractAddress: contractAddr.String(),
		TransactionIds:  []string{uuid.New().String()},
		Errors:          []int64{int64(coordinator.DelegationAcknowledgementError_MaxInflightTransactions)},
	})
	sm.handleDelegationResponse(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_DelegationResponse, Payload: payload})
}

func TestHandleDelegationResponse_CoordinatorErrorWithContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.DelegationResponse{
		ContractAddress: contractAddr.String(),
		TransactionIds:  []string{uuid.New().String()},
		Errors:          []int64{int64(coordinator.DelegationAcknowledgementError_CoordinatorError)},
	})
	sm.handleDelegationResponse(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_DelegationResponse, Payload: payload})
}

func TestHandleDelegationRejection_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.DelegationRejection{ContractAddress: "invalid"})
	sm.handleDelegationRejection(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_DelegationRejection, Payload: payload})
}

func TestHandleDelegationRejection_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.DelegationRejection{ContractAddress: contractAddr.String()})
	sm.handleDelegationRejection(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_DelegationRejection, Payload: payload})
}

func TestHandleHandoverRequest_InvalidContractAddress(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.CoordinatorHandoverRequest{ContractAddress: "invalid"})
	sm.handleHandoverRequest(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_HandoverRequest, Payload: payload})
}

func TestHandleEndorsementRequest_WithExpiry(t *testing.T) {
	ctx := t.Context()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New().String()
	payload, _ := proto.Marshal(&engineProto.EndorsementRequest{
		ContractAddress: contractAddr.String(), IdempotencyKey: uuid.New().String(),
		TransactionSpecification: &prototk.TransactionSpecification{TransactionId: specTransactionID(txID)},
		ExpiryTimeUnixMs:         time.Now().Add(time.Hour).UnixMilli(),
	})
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mocks.domainAPI, nil).Once()
	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq
	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		ev, ok := e.(*coordinator.EndorsementRequestReceivedEvent)
		return ok && !ev.Expiry.IsZero()
	})).Once()
	sm.handleEndorsementRequest(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_EndorsementRequest, Payload: payload})
}

func TestHandleEndorsementRejection_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.EndorsementRejection{ContractAddress: "invalid", TransactionId: uuid.New().String(), IdempotencyKey: uuid.New().String()})
	sm.handleEndorsementRejection(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_EndorsementRejection, Payload: payload})
}

func TestHandleEndorsementRejection_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.EndorsementRejection{ContractAddress: contractAddr.String(), TransactionId: uuid.New().String(), IdempotencyKey: uuid.New().String()})
	sm.handleEndorsementRejection(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_EndorsementRejection, Payload: payload})
}

func TestHandleEndorsementError_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.EndorsementError{ContractAddress: "invalid", TransactionId: uuid.New().String(), IdempotencyKey: uuid.New().String()})
	sm.handleEndorsementError(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_EndorsementError, Payload: payload})
}

func TestHandleNonceAssigned_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.NonceAssigned{ContractAddress: "invalid", TransactionId: uuid.New().String(), Nonce: 1})
	sm.handleNonceAssigned(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_NonceAssigned, Payload: payload})
}

func TestHandleNonceAssigned_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.NonceAssigned{ContractAddress: contractAddr.String(), TransactionId: uuid.New().String(), Nonce: 1})
	sm.handleNonceAssigned(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_NonceAssigned, Payload: payload})
}

func TestHandleTransactionSubmitted_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	hash := pldtypes.RandBytes32()
	payload, _ := proto.Marshal(&engineProto.TransactionSubmitted{ContractAddress: "invalid", TransactionId: uuid.New().String(), Hash: hash[:]})
	sm.handleTransactionSubmitted(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_TransactionSubmitted, Payload: payload})
}

func TestHandleTransactionSubmitted_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	hash := pldtypes.RandBytes32()
	payload, _ := proto.Marshal(&engineProto.TransactionSubmitted{ContractAddress: contractAddr.String(), TransactionId: uuid.New().String(), Hash: hash[:]})
	sm.handleTransactionSubmitted(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_TransactionSubmitted, Payload: payload})
}

func TestHandleTransactionConfirmed_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.TransactionConfirmed{ContractAddress: "invalid", TransactionId: uuid.New().String()})
	sm.handleTransactionConfirmed(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_TransactionConfirmed, Payload: payload})
}

func TestHandleTransactionConfirmed_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.TransactionConfirmed{ContractAddress: contractAddr.String(), TransactionId: uuid.New().String()})
	sm.handleTransactionConfirmed(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_TransactionConfirmed, Payload: payload})
}

func TestHandlePreDispatchRejection_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.PreDispatchRejection{ContractAddress: "invalid", TransactionId: uuid.New().String(), RequestId: uuid.New().String()})
	sm.handlePreDispatchRejection(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_PreDispatchRejection, Payload: payload})
}

func TestHandlePreDispatchRejection_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.PreDispatchRejection{
		ContractAddress: contractAddr.String(), TransactionId: uuid.New().String(), RequestId: uuid.New().String(),
	})
	sm.handlePreDispatchRejection(ctx, &components.ReceivedMessage{MessageType: transport.MessageType_PreDispatchRejection, Payload: payload})
}

func TestHandleSignResponse_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	requestID := uuid.New()
	payload, _ := proto.Marshal(&engineProto.SignResponse{
		TransactionId:     txID.String(),
		AssembleRequestId: requestID.String(),
		ContractAddress:   contractAddr.String(),
		AttestationResult: &prototk.AttestationResult{Name: "sig"},
		PostAssembly:      &prototk.TransactionPostAssembly{AssemblyResult: prototk.AssembleTransactionResponse_OK},
	})

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.SignedEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID && event.AttestationResult.Name == "sig" &&
			event.PostAssembly != nil && event.PostAssembly.GetAssemblyResult() == prototk.AssembleTransactionResponse_OK
	})).Once()

	sm.handleSignResponse(ctx, &components.ReceivedMessage{
		FromNode:    "test-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_SignResponse,
		Payload:     payload,
	})
	mocks.coordinator.AssertExpectations(t)
}

func TestHandleSignResponse_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	sm.handleSignResponse(ctx, &components.ReceivedMessage{
		FromNode: "test-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_SignResponse, Payload: []byte("invalid-proto"),
	})
}

func TestHandleSignResponse_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.SignResponse{
		TransactionId: uuid.New().String(), AssembleRequestId: uuid.New().String(),
		ContractAddress: "invalid-address", AttestationResult: &prototk.AttestationResult{Name: "sig"},
	})
	sm.handleSignResponse(ctx, &components.ReceivedMessage{
		FromNode: "test-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_SignResponse, Payload: payload,
	})
}

func TestHandleSignResponse_NilAttestationResult(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.SignResponse{
		TransactionId: uuid.New().String(), AssembleRequestId: uuid.New().String(),
		ContractAddress: contractAddr.String(),
	})
	sm.handleSignResponse(ctx, &components.ReceivedMessage{
		FromNode: "test-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_SignResponse, Payload: payload,
	})
}

func TestHandleSignResponse_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.SignResponse{
		TransactionId: uuid.New().String(), AssembleRequestId: uuid.New().String(),
		ContractAddress: contractAddr.String(), AttestationResult: &prototk.AttestationResult{Name: "sig"},
	})
	sm.handleSignResponse(ctx, &components.ReceivedMessage{
		FromNode: "test-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_SignResponse, Payload: payload,
	})
}

func TestHandleSignError_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	txID := uuid.New()
	requestID := uuid.New()
	payload, _ := proto.Marshal(&engineProto.SignError{
		TransactionId: txID.String(), AssembleRequestId: requestID.String(),
		ContractAddress: contractAddr.String(), ErrorMessage: "sign failed",
	})

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	mocks.coordinator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(e interface{}) bool {
		event, ok := e.(*coordTransaction.SignErrorEvent)
		return ok && event.TransactionID == txID && event.RequestID == requestID
	})).Once()

	sm.handleSignError(ctx, &components.ReceivedMessage{
		FromNode: "test-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_SignError, Payload: payload,
	})
	mocks.coordinator.AssertExpectations(t)
}

func TestHandleSignError_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	sm.handleSignError(ctx, &components.ReceivedMessage{
		FromNode: "test-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_SignError, Payload: []byte("invalid-proto"),
	})
}

func TestHandleSignError_InvalidContractAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	payload, _ := proto.Marshal(&engineProto.SignError{
		TransactionId: uuid.New().String(), AssembleRequestId: uuid.New().String(),
		ContractAddress: "invalid-address",
	})
	sm.handleSignError(ctx, &components.ReceivedMessage{
		FromNode: "test-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_SignError, Payload: payload,
	})
}

func TestHandleSignError_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()
	payload, _ := proto.Marshal(&engineProto.SignError{
		TransactionId: uuid.New().String(), AssembleRequestId: uuid.New().String(),
		ContractAddress: contractAddr.String(),
	})
	sm.handleSignError(ctx, &components.ReceivedMessage{
		FromNode: "test-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_SignError, Payload: payload,
	})
}

// --- State query message routing (off both event loops) ---

func TestHandleQueryAvailableStatesRequest_RoutedToCoordinatorProvider(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	request := &engineProto.QueryAvailableStatesRequest{
		ContractAddress: contractAddr.String(),
		RequestId:       "req1",
		SchemaId:        "0xaa",
		QueryJson:       "{}",
	}
	payload, _ := proto.Marshal(request)
	message := &components.ReceivedMessage{
		FromNode:    "requesting-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_QueryAvailableStatesRequest,
		Payload:     payload,
	}

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	provider := coordinatorstateviewmocks.NewProvider(t)
	mocks.coordinator.EXPECT().StateViewProvider().Return(provider).Once()
	// The transport-authenticated FromNode is passed for the visibility filtering — the routing
	// never queues onto the coordinator event loop.
	provider.EXPECT().HandleQueryAvailableStates(ctx, "requesting-node", mock.MatchedBy(func(req *engineProto.QueryAvailableStatesRequest) bool {
		return req.GetRequestId() == "req1" && req.GetSchemaId() == "0xaa"
	})).Once()

	sm.handleQueryAvailableStatesRequest(ctx, message)
}

func TestHandleQueryAvailableStatesRequest_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	payload, _ := proto.Marshal(&engineProto.QueryAvailableStatesRequest{ContractAddress: contractAddr.String(), RequestId: "req1"})
	// No sequencer registered: the message is dropped (never loads a sequencer).
	sm.handleQueryAvailableStatesRequest(ctx, &components.ReceivedMessage{
		FromNode: "requesting-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_QueryAvailableStatesRequest, Payload: payload,
	})
}

func TestHandleQueryAvailableStatesRequest_BadPayloadAndAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	sm.handleQueryAvailableStatesRequest(ctx, &components.ReceivedMessage{
		FromNode: "requesting-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_QueryAvailableStatesRequest, Payload: []byte("not-a-proto"),
	})

	payload, _ := proto.Marshal(&engineProto.QueryAvailableStatesRequest{ContractAddress: "not-an-address"})
	sm.handleQueryAvailableStatesRequest(ctx, &components.ReceivedMessage{
		FromNode: "requesting-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_QueryAvailableStatesRequest, Payload: payload,
	})
}

func TestHandleQueryAvailableStatesResponse_RoutedToOriginatorReader(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	response := &engineProto.QueryAvailableStatesResponse{
		ContractAddress: contractAddr.String(),
		RequestId:       "req1",
		States:          []*prototk.QueriedState{{State: &prototk.EndorsableState{Id: "0xaa"}}},
	}
	payload, _ := proto.Marshal(response)
	message := &components.ReceivedMessage{
		FromNode:    "coordinator-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_QueryAvailableStatesResponse,
		Payload:     payload,
	}

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	reader := originatorstateviewmocks.NewReader(t)
	mocks.originator.EXPECT().StateViewReader().Return(reader).Once()
	reader.EXPECT().HandleQueryAvailableStatesResponse(ctx, "coordinator-node", mock.MatchedBy(func(resp *engineProto.QueryAvailableStatesResponse) bool {
		return resp.GetRequestId() == "req1" && len(resp.GetStates()) == 1
	})).Once()

	sm.handleQueryAvailableStatesResponse(ctx, message)
}

func TestHandleQueryAvailableStatesResponse_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	payload, _ := proto.Marshal(&engineProto.QueryAvailableStatesResponse{ContractAddress: contractAddr.String(), RequestId: "req1"})
	sm.handleQueryAvailableStatesResponse(ctx, &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_QueryAvailableStatesResponse, Payload: payload,
	})
}

func TestHandleQueryAvailableStatesResponse_BadPayloadAndAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	sm.handleQueryAvailableStatesResponse(ctx, &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_QueryAvailableStatesResponse, Payload: []byte("not-a-proto"),
	})

	payload, _ := proto.Marshal(&engineProto.QueryAvailableStatesResponse{ContractAddress: "not-an-address"})
	sm.handleQueryAvailableStatesResponse(ctx, &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_QueryAvailableStatesResponse, Payload: payload,
	})
}

func TestHandleStateViewError_RoutedToOriginatorReader(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	stateViewError := &engineProto.StateViewError{
		ContractAddress: contractAddr.String(),
		RequestId:       "req1",
		ErrorMessage:    "pop",
	}
	payload, _ := proto.Marshal(stateViewError)
	message := &components.ReceivedMessage{
		FromNode:    "coordinator-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_StateViewError,
		Payload:     payload,
	}

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	reader := originatorstateviewmocks.NewReader(t)
	mocks.originator.EXPECT().StateViewReader().Return(reader).Once()
	reader.EXPECT().HandleError(ctx, "coordinator-node", mock.MatchedBy(func(errMsg *engineProto.StateViewError) bool {
		return errMsg.GetRequestId() == "req1" && errMsg.GetErrorMessage() == "pop"
	})).Once()

	sm.handleStateViewError(ctx, message)
}

func TestHandleStateViewError_SequencerNotLoaded(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	payload, _ := proto.Marshal(&engineProto.StateViewError{ContractAddress: contractAddr.String(), RequestId: "req1"})
	sm.handleStateViewError(ctx, &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_StateViewError, Payload: payload,
	})
}

func TestHandleStateViewError_BadPayloadAndAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)

	sm.handleStateViewError(ctx, &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_StateViewError, Payload: []byte("not-a-proto"),
	})

	payload, _ := proto.Marshal(&engineProto.StateViewError{ContractAddress: "not-an-address"})
	sm.handleStateViewError(ctx, &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_StateViewError, Payload: payload,
	})
}

func TestHandleGetSpentStateIDsRequest_RoutedToCoordinatorProvider(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	request := &engineProto.GetSpentStateIDsRequest{
		ContractAddress:   contractAddr.String(),
		RequestId:         "req1",
		AssembleRequestId: "assemble-1",
	}
	payload, _ := proto.Marshal(request)
	message := &components.ReceivedMessage{
		FromNode:    "requesting-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_GetSpentStateIDsRequest,
		Payload:     payload,
	}

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	provider := coordinatorstateviewmocks.NewProvider(t)
	mocks.coordinator.EXPECT().StateViewProvider().Return(provider).Once()
	provider.EXPECT().HandleGetSpentStateIDs(ctx, "requesting-node", mock.MatchedBy(func(req *engineProto.GetSpentStateIDsRequest) bool {
		return req.GetRequestId() == "req1" && req.GetAssembleRequestId() == "assemble-1"
	})).Once()

	sm.handleGetSpentStateIDsRequest(ctx, message)
}

func TestHandleGetSpentStateIDsRequest_SequencerNotLoaded_BadPayloadAndAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	// No sequencer registered: the message is dropped (never loads a sequencer).
	payload, _ := proto.Marshal(&engineProto.GetSpentStateIDsRequest{ContractAddress: contractAddr.String(), RequestId: "req1"})
	sm.handleGetSpentStateIDsRequest(ctx, &components.ReceivedMessage{
		FromNode: "requesting-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_GetSpentStateIDsRequest, Payload: payload,
	})

	sm.handleGetSpentStateIDsRequest(ctx, &components.ReceivedMessage{
		FromNode: "requesting-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_GetSpentStateIDsRequest, Payload: []byte("not-a-proto"),
	})

	payload, _ = proto.Marshal(&engineProto.GetSpentStateIDsRequest{ContractAddress: "not-an-address"})
	sm.handleGetSpentStateIDsRequest(ctx, &components.ReceivedMessage{
		FromNode: "requesting-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_GetSpentStateIDsRequest, Payload: payload,
	})
}

func TestHandleGetSpentStateIDsResponse_RoutedToOriginatorReader(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	response := &engineProto.GetSpentStateIDsResponse{
		ContractAddress: contractAddr.String(),
		RequestId:       "req1",
		SpentStateIds:   [][]byte{{0x01, 0x02}},
	}
	payload, _ := proto.Marshal(response)
	message := &components.ReceivedMessage{
		FromNode:    "coordinator-node",
		MessageID:   uuid.New(),
		MessageType: transport.MessageType_GetSpentStateIDsResponse,
		Payload:     payload,
	}

	seq := newSequencerForTransportClientTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = seq

	reader := originatorstateviewmocks.NewReader(t)
	mocks.originator.EXPECT().StateViewReader().Return(reader).Once()
	reader.EXPECT().HandleGetSpentStateIDsResponse(ctx, "coordinator-node", mock.MatchedBy(func(resp *engineProto.GetSpentStateIDsResponse) bool {
		return resp.GetRequestId() == "req1" && len(resp.GetSpentStateIds()) == 1
	})).Once()

	sm.handleGetSpentStateIDsResponse(ctx, message)
}

func TestHandleGetSpentStateIDsResponse_SequencerNotLoaded_BadPayloadAndAddress(t *testing.T) {
	ctx := context.Background()
	mocks := newTransportClientTestMocks(t)
	sm := newSequencerManagerForTransportClientTesting(t, mocks)
	contractAddr := pldtypes.RandAddress()

	payload, _ := proto.Marshal(&engineProto.GetSpentStateIDsResponse{ContractAddress: contractAddr.String(), RequestId: "req1"})
	sm.handleGetSpentStateIDsResponse(ctx, &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_GetSpentStateIDsResponse, Payload: payload,
	})

	sm.handleGetSpentStateIDsResponse(ctx, &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_GetSpentStateIDsResponse, Payload: []byte("not-a-proto"),
	})

	payload, _ = proto.Marshal(&engineProto.GetSpentStateIDsResponse{ContractAddress: "not-an-address"})
	sm.handleGetSpentStateIDsResponse(ctx, &components.ReceivedMessage{
		FromNode: "coordinator-node", MessageID: uuid.New(),
		MessageType: transport.MessageType_GetSpentStateIDsResponse, Payload: payload,
	})
}

// Every ID in a sequencer message is a string a peer node filled in, so one that is not a UUID has to
// be dropped rather than parsed with uuid.MustParse: these handlers run as bare goroutines with no
// recover, so a single bad value from any peer would take the node down instead of losing one message.
// The originator and coordinator mocks are strict and given no expectations, so the test fails if any
// handler queues an event off a message it should have dropped.
func TestHandleMessages_UnusableIDs_AreDroppedWithoutQueueingEvents(t *testing.T) {
	postAssembly := func(result prototk.AssembleTransactionResponse_Result) *prototk.TransactionPostAssembly {
		return &prototk.TransactionPostAssembly{AssemblyResult: result}
	}

	revertReason := "transaction reverted"

	messageKinds := []struct {
		name string
		// hasRequestID is false for the messages that carry only a transaction ID, so the request ID
		// cases below are skipped for them - there is nothing to make unusable
		hasRequestID bool
		messageType  string
		build        func(contractAddress, transactionID, requestID string) []byte
		handle       func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage)
	}{
		{
			name:         "endorsement response",
			hasRequestID: true,
			messageType:  transport.MessageType_EndorsementResponse,
			build: func(contractAddress, transactionID, requestID string) []byte {
				payload, _ := proto.Marshal(&engineProto.EndorsementResponse{
					TransactionId:          transactionID,
					IdempotencyKey:         requestID,
					ContractAddress:        contractAddress,
					Endorsement:            &prototk.AttestationResult{Name: "test-endorsement"},
					AttestationRequestName: "endorsement1",
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handleEndorsementResponse(ctx, message)
			},
		},
		{
			name:         "endorsement response revert",
			hasRequestID: true,
			messageType:  transport.MessageType_EndorsementResponse,
			build: func(contractAddress, transactionID, requestID string) []byte {
				payload, _ := proto.Marshal(&engineProto.EndorsementResponse{
					TransactionId:          transactionID,
					IdempotencyKey:         requestID,
					ContractAddress:        contractAddress,
					RevertReason:           &revertReason,
					AttestationRequestName: "endorsement1",
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handleEndorsementResponse(ctx, message)
			},
		},
		{
			name:         "endorsement rejection",
			hasRequestID: true,
			messageType:  transport.MessageType_EndorsementRejection,
			build: func(contractAddress, transactionID, requestID string) []byte {
				payload, _ := proto.Marshal(&engineProto.EndorsementRejection{
					TransactionId:          transactionID,
					IdempotencyKey:         requestID,
					ContractAddress:        contractAddress,
					AttestationRequestName: "endorsement1",
					RejectionReason:        engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE,
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handleEndorsementRejection(ctx, message)
			},
		},
		{
			name:         "endorsement error",
			hasRequestID: true,
			messageType:  transport.MessageType_EndorsementError,
			build: func(contractAddress, transactionID, requestID string) []byte {
				payload, _ := proto.Marshal(&engineProto.EndorsementError{
					TransactionId:          transactionID,
					IdempotencyKey:         requestID,
					ContractAddress:        contractAddress,
					AttestationRequestName: "endorsement1",
					ErrorMessage:           "something went wrong",
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handleEndorsementError(ctx, message)
			},
		},
		{
			name:         "assemble request",
			hasRequestID: true,
			messageType:  transport.MessageType_AssembleRequest,
			build: func(contractAddress, transactionID, requestID string) []byte {
				payload, _ := proto.Marshal(&engineProto.AssembleRequest{
					TransactionId:     transactionID,
					AssembleRequestId: requestID,
					ContractAddress:   contractAddress,
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handleAssembleRequest(ctx, message)
			},
		},
		{
			name:         "assemble response",
			hasRequestID: true,
			messageType:  transport.MessageType_AssembleResponse,
			build: func(contractAddress, transactionID, requestID string) []byte {
				payload, _ := proto.Marshal(&engineProto.AssembleResponse{
					TransactionId:     transactionID,
					AssembleRequestId: requestID,
					ContractAddress:   contractAddress,
					PostAssembly:      postAssembly(prototk.AssembleTransactionResponse_OK),
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handleAssembleResponse(ctx, message)
			},
		},
		{
			name:         "assemble response revert",
			hasRequestID: true,
			messageType:  transport.MessageType_AssembleResponse,
			build: func(contractAddress, transactionID, requestID string) []byte {
				payload, _ := proto.Marshal(&engineProto.AssembleResponse{
					TransactionId:     transactionID,
					AssembleRequestId: requestID,
					ContractAddress:   contractAddress,
					PostAssembly:      postAssembly(prototk.AssembleTransactionResponse_REVERT),
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handleAssembleResponse(ctx, message)
			},
		},
		{
			name:         "assemble error",
			hasRequestID: true,
			messageType:  transport.MessageType_AssembleError,
			build: func(contractAddress, transactionID, requestID string) []byte {
				payload, _ := proto.Marshal(&engineProto.AssembleError{
					TransactionId:     transactionID,
					AssembleRequestId: requestID,
					ContractAddress:   contractAddress,
					ErrorMessage:      "something went wrong",
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handleAssembleError(ctx, message)
			},
		},
		{
			name:         "assemble rejection",
			hasRequestID: true,
			messageType:  transport.MessageType_AssembleRejection,
			build: func(contractAddress, transactionID, requestID string) []byte {
				payload, _ := proto.Marshal(&engineProto.AssembleRejection{
					TransactionId:     transactionID,
					AssembleRequestId: requestID,
					ContractAddress:   contractAddress,
					RejectionReason:   engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE,
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handleAssembleRejection(ctx, message)
			},
		},
		{
			name:         "sign response",
			hasRequestID: true,
			messageType:  transport.MessageType_SignResponse,
			build: func(contractAddress, transactionID, requestID string) []byte {
				payload, _ := proto.Marshal(&engineProto.SignResponse{
					TransactionId:     transactionID,
					AssembleRequestId: requestID,
					ContractAddress:   contractAddress,
					AttestationResult: &prototk.AttestationResult{Name: "test-signature"},
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handleSignResponse(ctx, message)
			},
		},
		{
			name:         "sign error",
			hasRequestID: true,
			messageType:  transport.MessageType_SignError,
			build: func(contractAddress, transactionID, requestID string) []byte {
				payload, _ := proto.Marshal(&engineProto.SignError{
					TransactionId:     transactionID,
					AssembleRequestId: requestID,
					ContractAddress:   contractAddress,
					ErrorMessage:      "something went wrong",
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handleSignError(ctx, message)
			},
		},
		{
			name:         "pre-dispatch request",
			hasRequestID: true,
			messageType:  transport.MessageType_PreDispatchRequest,
			build: func(contractAddress, transactionID, requestID string) []byte {
				payload, _ := proto.Marshal(&engineProto.PreDispatchRequest{
					Id:              requestID,
					TransactionId:   transactionID,
					ContractAddress: contractAddress,
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handlePreDispatchRequest(ctx, message)
			},
		},
		{
			name:         "pre-dispatch response",
			hasRequestID: true,
			messageType:  transport.MessageType_PreDispatchResponse,
			build: func(contractAddress, transactionID, requestID string) []byte {
				payload, _ := proto.Marshal(&engineProto.PreDispatchResponse{
					Id:              requestID,
					TransactionId:   transactionID,
					ContractAddress: contractAddress,
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handlePreDispatchResponse(ctx, message)
			},
		},
		{
			name:        "dispatched",
			messageType: transport.MessageType_Dispatched,
			build: func(contractAddress, transactionID, _ string) []byte {
				payload, _ := proto.Marshal(&engineProto.TransactionDispatched{
					TransactionId:   transactionID,
					ContractAddress: contractAddress,
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handleDispatchedEvent(ctx, message)
			},
		},
		{
			name:        "nonce assigned",
			messageType: transport.MessageType_NonceAssigned,
			build: func(contractAddress, transactionID, _ string) []byte {
				payload, _ := proto.Marshal(&engineProto.NonceAssigned{
					TransactionId:   transactionID,
					ContractAddress: contractAddress,
					Nonce:           1,
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handleNonceAssigned(ctx, message)
			},
		},
		{
			name:        "transaction submitted",
			messageType: transport.MessageType_TransactionSubmitted,
			build: func(contractAddress, transactionID, _ string) []byte {
				payload, _ := proto.Marshal(&engineProto.TransactionSubmitted{
					TransactionId:   transactionID,
					ContractAddress: contractAddress,
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handleTransactionSubmitted(ctx, message)
			},
		},
		{
			name:        "transaction confirmed",
			messageType: transport.MessageType_TransactionConfirmed,
			build: func(contractAddress, transactionID, _ string) []byte {
				payload, _ := proto.Marshal(&engineProto.TransactionConfirmed{
					TransactionId:   transactionID,
					ContractAddress: contractAddress,
					Outcome:         engineProto.TransactionConfirmed_OUTCOME_SUCCESS,
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handleTransactionConfirmed(ctx, message)
			},
		},
		{
			name:        "transaction confirmed reverted",
			messageType: transport.MessageType_TransactionConfirmed,
			build: func(contractAddress, transactionID, _ string) []byte {
				payload, _ := proto.Marshal(&engineProto.TransactionConfirmed{
					TransactionId:   transactionID,
					ContractAddress: contractAddress,
					Outcome:         engineProto.TransactionConfirmed_OUTCOME_REVERTED,
					FailureMessage:  "reverted",
				})
				return payload
			},
			handle: func(sm *sequencerManager, ctx context.Context, message *components.ReceivedMessage) {
				sm.handleTransactionConfirmed(ctx, message)
			},
		},
	}

	idCases := []struct {
		name          string
		transactionID string
		requestID     string
		// onlyRequestID marks the cases that make the request ID unusable, which only apply to the
		// messages that carry one
		onlyRequestID bool
	}{
		{name: "no transaction ID", transactionID: "", requestID: uuid.NewString()},
		{name: "transaction ID is not a UUID", transactionID: "not-a-uuid", requestID: uuid.NewString()},
		{name: "no request ID", transactionID: uuid.NewString(), requestID: "", onlyRequestID: true},
		{name: "request ID is not a UUID", transactionID: uuid.NewString(), requestID: "not-a-uuid", onlyRequestID: true},
	}

	for _, messageKind := range messageKinds {
		for _, idCase := range idCases {
			if idCase.onlyRequestID && !messageKind.hasRequestID {
				continue
			}
			t.Run(messageKind.name+": "+idCase.name, func(t *testing.T) {
				ctx := context.Background()
				mocks := newTransportClientTestMocks(t)
				sm := newSequencerManagerForTransportClientTesting(t, mocks)
				contractAddr := pldtypes.RandAddress()
				sm.sequencers[contractAddr.String()] = newSequencerForTransportClientTesting(contractAddr, mocks)

				messageKind.handle(sm, ctx, &components.ReceivedMessage{
					FromNode:    "test-node",
					MessageID:   uuid.New(),
					MessageType: messageKind.messageType,
					Payload:     messageKind.build(contractAddr.String(), idCase.transactionID, idCase.requestID),
				})

				mocks.originator.AssertExpectations(t)
				mocks.coordinator.AssertExpectations(t)
			})
		}
	}
}

// The submission hash arrives as raw bytes from a peer, and converting a slice shorter than 32 bytes
// to a Bytes32 panics, so a wrong-length hash has to be dropped rather than converted. The strict
// originator mock has no expectations, so it fails the test if an event is queued.
func TestHandleTransactionSubmitted_UnusableHash_IsDroppedWithoutQueueingEvent(t *testing.T) {
	hashCases := []struct {
		name string
		hash []byte
	}{
		{name: "no hash", hash: nil},
		{name: "hash shorter than 32 bytes", hash: pldtypes.RandBytes(31)},
		{name: "hash longer than 32 bytes", hash: pldtypes.RandBytes(33)},
	}

	for _, hashCase := range hashCases {
		t.Run(hashCase.name, func(t *testing.T) {
			ctx := context.Background()
			mocks := newTransportClientTestMocks(t)
			sm := newSequencerManagerForTransportClientTesting(t, mocks)
			contractAddr := pldtypes.RandAddress()
			sm.sequencers[contractAddr.String()] = newSequencerForTransportClientTesting(contractAddr, mocks)

			payload, err := proto.Marshal(&engineProto.TransactionSubmitted{
				TransactionId:   uuid.NewString(),
				ContractAddress: contractAddr.String(),
				Hash:            hashCase.hash,
			})
			require.NoError(t, err)

			sm.handleTransactionSubmitted(ctx, &components.ReceivedMessage{
				FromNode:    "test-node",
				MessageID:   uuid.New(),
				MessageType: transport.MessageType_TransactionSubmitted,
				Payload:     payload,
			})

			mocks.originator.AssertExpectations(t)
		})
	}
}
