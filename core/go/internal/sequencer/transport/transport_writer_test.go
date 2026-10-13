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

package transport

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
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

// ===== NewTransportWriter / Lifecycle Tests =====

func TestNewTransportWriter(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	mockTM := componentsmocks.NewTransportManager(t)

	tw := NewTransportWriter(ctx, contractAddress, "local-node", mockTM, func(_ context.Context, _ *components.ReceivedMessage) {})
	require.NotNil(t, tw)
}

func TestStartLoopbackWriter_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	mockTM := componentsmocks.NewTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()

	tw := NewTransportWriter(ctx, contractAddress, "local-node", mockTM, func(_ context.Context, _ *components.ReceivedMessage) {})
	tw.StartLoopbackWriter()

	// Cancel the writer context to trigger the tw.ctx.Done() branch in loopbackSender.
	cancel()
	tw.WaitForDone(context.Background())
}

func TestStartLoopbackWriter_QueueClosed(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	mockTM := componentsmocks.NewTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLT.On("LoopbackQueue").Return(loopbackQueue).Maybe()

	tw := &transportWriter{
		ctx:                   ctx,
		nodeID:                "local-node",
		transportManager:      mockTM,
		loopbackTransport:     mockLT,
		contractAddress:       contractAddress,
		loopbackSenderStopped: make(chan struct{}),
	}
	tw.StartLoopbackWriter()

	// Closing the channel triggers the !ok path in loopbackSender.
	close(loopbackQueue)
	tw.WaitForDone(context.Background())
}

func TestLoopbackSender_SendError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	mockTM := componentsmocks.NewTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLT.On("LoopbackQueue").Return(loopbackQueue).Maybe()

	sendCalled := make(chan struct{})
	mockLT.On("Send", mock.Anything, mock.Anything).
		Run(func(_ mock.Arguments) { close(sendCalled) }).
		Return(errors.New("loopback send error"))

	tw := &transportWriter{
		ctx:                   ctx,
		nodeID:                "local-node",
		transportManager:      mockTM,
		loopbackTransport:     mockLT,
		contractAddress:       contractAddress,
		loopbackSenderStopped: make(chan struct{}),
	}

	tw.StartLoopbackWriter()

	// Enqueue a message; the goroutine will process it and get the error.
	loopbackQueue <- &components.FireAndForgetMessageSend{Node: "local-node", MessageType: "test"}

	// Wait until Send has been called (and returned the error).
	<-sendCalled

	cancel()
	tw.WaitForDone(context.Background())
}

func TestWaitForDone_ContextExpired(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	mockTM := componentsmocks.NewTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLT.On("LoopbackQueue").Return(loopbackQueue).Maybe()

	tw := &transportWriter{
		ctx:                   ctx,
		nodeID:                "local-node",
		transportManager:      mockTM,
		loopbackTransport:     mockLT,
		contractAddress:       contractAddress,
		loopbackSenderStopped: make(chan struct{}),
	}
	tw.StartLoopbackWriter()

	// WaitForDone should return immediately when the supplied ctx is already cancelled.
	waitCtx, waitCancel := context.WithCancel(context.Background())
	waitCancel()
	tw.WaitForDone(waitCtx)

	// Now stop the sender cleanly.
	cancel()
	tw.WaitForDone(context.Background())
}

// ===== send() Internal Tests =====

func TestSend_EmptyNode(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.send(ctx, &components.FireAndForgetMessageSend{
		Node:        "",
		MessageType: "test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "destination node name")
}

func TestSend_ContextCancelled_LoopbackQueueFull(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	mockTM := componentsmocks.NewTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	// Fill the loopback queue (buffer=1) so the channel send blocks.
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	loopbackQueue <- &components.FireAndForgetMessageSend{Node: "local-node", MessageType: "dummy"}
	mockLT.On("LoopbackQueue").Return(loopbackQueue).Maybe()

	tw := &transportWriter{
		ctx:               context.Background(), // non-cancelled tw ctx
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.send(cancelledCtx, &components.FireAndForgetMessageSend{
		Node:        "local-node",
		MessageType: "test",
	})
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestSend_WriterContextCancelled_LoopbackQueueFull(t *testing.T) {
	twCtx, twCancel := context.WithCancel(context.Background())
	twCancel() // cancel tw context immediately

	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	mockTM := componentsmocks.NewTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	// Fill the loopback queue so the channel send blocks.
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	loopbackQueue <- &components.FireAndForgetMessageSend{Node: "local-node", MessageType: "dummy"}
	mockLT.On("LoopbackQueue").Return(loopbackQueue).Maybe()

	tw := &transportWriter{
		ctx:               twCtx, // cancelled writer context
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.send(context.Background(), &components.FireAndForgetMessageSend{
		Node:        "local-node",
		MessageType: "test",
	})
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

// ===== Proto message builder helpers =====

func testTransactionSubmittedMsg(txID uuid.UUID, contractAddress *pldtypes.EthAddress, txHash *pldtypes.Bytes32) *engineProto.TransactionSubmitted {
	return &engineProto.TransactionSubmitted{
		Id:              uuid.New().String(),
		TransactionId:   txID.String(),
		ContractAddress: contractAddress.HexString(),
		Hash:            txHash.Bytes(),
	}
}

func testDelegationRequestMsg(coordinatorNode string, contractAddress *pldtypes.EthAddress, transactions []*components.PrivateTransaction, blockHeight uint64) (*engineProto.DelegationRequest, error) {
	delegations := make([]*engineProto.PrivateTransactionDelegation, 0, len(transactions))
	for _, tx := range transactions {
		delegations = append(delegations, &engineProto.PrivateTransactionDelegation{
			Domain:      tx.Domain,
			Intent:      tx.Intent,
			PreAssembly: tx.PreAssembly,
		})
	}
	return &engineProto.DelegationRequest{
		DelegateNodeId:        coordinatorNode,
		OriginatorBlockHeight: int64(blockHeight),
		ContractAddress:       contractAddress.HexString(),
		Transactions:          delegations,
	}, nil
}

func testDelegationResponseMsg(delegatingNodeName, delegationId string, transactionIDs []string, contractAddress *pldtypes.EthAddress, errors []int64) *engineProto.DelegationResponse {
	return &engineProto.DelegationResponse{
		DelegationId:    delegationId,
		TransactionIds:  transactionIDs,
		DelegateNodeId:  delegatingNodeName,
		ContractAddress: contractAddress.String(),
		Errors:          errors,
	}
}

func testDelegationRejectionMsg(delegatingNodeName, delegationId string, contractAddress *pldtypes.EthAddress, rejectionReason engineProto.RejectionReason, activeCoordinator string, originatorBlockHeight, coordinatorBlockHeight, blockHeightTolerance int64) *engineProto.DelegationRejection {
	return &engineProto.DelegationRejection{
		DelegationId:           delegationId,
		DelegateNodeId:         delegatingNodeName,
		ContractAddress:        contractAddress.String(),
		RejectionReason:        rejectionReason,
		ActiveCoordinator:      activeCoordinator,
		OriginatorBlockHeight:  originatorBlockHeight,
		CoordinatorBlockHeight: coordinatorBlockHeight,
		BlockHeightTolerance:   blockHeightTolerance,
	}
}

func testEndorsementRequestMsg(idempotencyKey uuid.UUID, party string, attRequest *prototk.AttestationRequest, transactionSpecification *prototk.TransactionSpecification, verifiers []*prototk.ResolvedVerifier, signatures []*prototk.AttestationResult, inputStates, readStates, outputStates, infoStates []*prototk.EndorsableState, expiry time.Time, coordinatorBlockHeight, blockHeightTolerance int64) *engineProto.EndorsementRequest {
	return &engineProto.EndorsementRequest{
		IdempotencyKey:           idempotencyKey.String(),
		ContractAddress:          transactionSpecification.ContractInfo.ContractAddress,
		AttestationRequest:       attRequest,
		Party:                    party,
		TransactionSpecification: transactionSpecification,
		Verifiers:                verifiers,
		Signatures:               signatures,
		InputStates:              inputStates,
		ReadStates:               readStates,
		OutputStates:             outputStates,
		InfoStates:               infoStates,
		ExpiryTimeUnixMs:         expiry.UnixMilli(),
		CoordinatorBlockHeight:   coordinatorBlockHeight,
		BlockHeightTolerance:     blockHeightTolerance,
	}
}

func testEndorsementResponseMsg(transactionId, idempotencyKey, contractAddress, endorsementName, party string, attResult *prototk.AttestationResult, revertReason string) *engineProto.EndorsementResponse {
	msg := &engineProto.EndorsementResponse{
		Endorsement:            attResult,
		TransactionId:          transactionId,
		IdempotencyKey:         idempotencyKey,
		AttestationRequestName: endorsementName,
		Party:                  party,
		ContractAddress:        contractAddress,
	}
	if revertReason != "" {
		msg.RevertReason = &revertReason
	}
	return msg
}

func testEndorsementRejectionMsg(transactionId, idempotencyKey, contractAddress, endorsementName, party string, reason engineProto.RejectionReason, coordinatorBlockHeight, endorserBlockHeight, blockHeightTolerance int64) *engineProto.EndorsementRejection {
	return &engineProto.EndorsementRejection{
		TransactionId:          transactionId,
		IdempotencyKey:         idempotencyKey,
		ContractAddress:        contractAddress,
		AttestationRequestName: endorsementName,
		Party:                  party,
		RejectionReason:        reason,
		CoordinatorBlockHeight: coordinatorBlockHeight,
		EndorserBlockHeight:    endorserBlockHeight,
		BlockHeightTolerance:   blockHeightTolerance,
	}
}

func testEndorsementErrorMsg(transactionId, idempotencyKey, contractAddress, errorMessage, party, attestationRequestName string) *engineProto.EndorsementError {
	return &engineProto.EndorsementError{
		TransactionId:          transactionId,
		IdempotencyKey:         idempotencyKey,
		ContractAddress:        contractAddress,
		ErrorMessage:           errorMessage,
		Party:                  party,
		AttestationRequestName: attestationRequestName,
	}
}

func testAssembleRequestMsg(txID, idempotencyId uuid.UUID, contractAddress *pldtypes.EthAddress, blockHeight int64, expiry time.Time, blockHeightTolerance int64) (*engineProto.AssembleRequest, error) {
	return &engineProto.AssembleRequest{
		TransactionId:          txID.String(),
		AssembleRequestId:      idempotencyId.String(),
		ContractAddress:        contractAddress.HexString(),
		CoordinatorBlockHeight: blockHeight,
		ExpiryTimeUnixMs:       expiry.UnixMilli(),
		BlockHeightTolerance:   blockHeightTolerance,
	}, nil
}

func testAssembleResponseMsg(txID, assembleRequestId uuid.UUID, contractAddress *pldtypes.EthAddress, postAssembly *components.TransactionPostAssembly, _ *prototk.TransactionPreAssembly) (*engineProto.AssembleResponse, error) {
	return &engineProto.AssembleResponse{
		TransactionId:     txID.String(),
		AssembleRequestId: assembleRequestId.String(),
		ContractAddress:   contractAddress.HexString(),
		PostAssembly:      postAssembly.AssembleResponse,
	}, nil
}

func testAssembleErrorMsg(txID, assembleRequestId uuid.UUID, contractAddress *pldtypes.EthAddress) *engineProto.AssembleError {
	return &engineProto.AssembleError{
		TransactionId:     txID.String(),
		AssembleRequestId: assembleRequestId.String(),
		ContractAddress:   contractAddress.HexString(),
	}
}

func testAssembleRejectionMsg(txID, assembleRequestId uuid.UUID, contractAddress *pldtypes.EthAddress, reason engineProto.RejectionReason, coordinatorBlockHeight, assemblerBlockHeight int64) *engineProto.AssembleRejection {
	return &engineProto.AssembleRejection{
		TransactionId:          txID.String(),
		AssembleRequestId:      assembleRequestId.String(),
		ContractAddress:        contractAddress.HexString(),
		RejectionReason:        reason,
		CoordinatorBlockHeight: coordinatorBlockHeight,
		AssemblerBlockHeight:   assemblerBlockHeight,
	}
}

func testNonceAssignedMsg(txID uuid.UUID, contractAddress *pldtypes.EthAddress, nonce uint64) *engineProto.NonceAssigned {
	return &engineProto.NonceAssigned{
		Id:              uuid.New().String(),
		TransactionId:   txID.String(),
		ContractAddress: contractAddress.HexString(),
		Nonce:           int64(nonce),
	}
}

func testTransactionConfirmedMsg(txID uuid.UUID, contractAddress *pldtypes.EthAddress, nonce *pldtypes.HexUint64, outcome engineProto.TransactionConfirmed_Outcome, revertReason pldtypes.HexBytes, failureMessage string, willRetry bool) *engineProto.TransactionConfirmed {
	msg := &engineProto.TransactionConfirmed{
		Id:              uuid.New().String(),
		TransactionId:   txID.String(),
		ContractAddress: contractAddress.HexString(),
		Outcome:         outcome,
		RevertReason:    revertReason,
		FailureMessage:  failureMessage,
		WillRetry:       willRetry,
	}
	if nonce != nil {
		msg.Nonce = int64(*nonce)
	}
	return msg
}

func testHeartbeatMsg(contractAddress *pldtypes.EthAddress, coordinatorSnapshot *engineProto.CoordinatorSnapshot) (*engineProto.CoordinatorHeartbeatNotification, error) {
	return &engineProto.CoordinatorHeartbeatNotification{
		ContractAddress:     contractAddress.HexString(),
		CoordinatorSnapshot: coordinatorSnapshot,
	}, nil
}

func testPreDispatchRequestMsg(idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification, contractAddress *pldtypes.EthAddress) *engineProto.PreDispatchRequest {
	return &engineProto.PreDispatchRequest{
		Id:              idempotencyKey.String(),
		TransactionId:   transactionSpecification.TransactionId,
		ContractAddress: contractAddress.HexString(),
	}
}

func testPreDispatchResponseMsg(idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification, contractAddress *pldtypes.EthAddress) *engineProto.PreDispatchResponse {
	return &engineProto.PreDispatchResponse{
		Id:              idempotencyKey.String(),
		TransactionId:   transactionSpecification.TransactionId,
		ContractAddress: contractAddress.HexString(),
	}
}

func testDispatchedMsg(idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification, contractAddress *pldtypes.EthAddress, transactionOriginator string) *engineProto.TransactionDispatched {
	return &engineProto.TransactionDispatched{
		Id:              idempotencyKey.String(),
		TransactionId:   transactionSpecification.TransactionId,
		ContractAddress: contractAddress.HexString(),
		Signer:          transactionOriginator,
	}
}

func testPreDispatchRejectionMsg(txID, requestID uuid.UUID, contractAddress *pldtypes.EthAddress, reason engineProto.RejectionReason) *engineProto.PreDispatchRejection {
	return &engineProto.PreDispatchRejection{
		TransactionId:   txID.String(),
		RequestId:       requestID.String(),
		ContractAddress: contractAddress.HexString(),
		RejectionReason: reason,
	}
}

func TestSendTransactionSubmitted_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txHashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	txHash := &txHashVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		// Verify message type
		if msg.MessageType != MessageType_TransactionSubmitted {
			return false
		}
		// Verify node
		if msg.Node != originatorNode {
			return false
		}
		// Verify component
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		// Verify payload is valid proto
		var txSubmitted engineProto.TransactionSubmitted
		err := proto.Unmarshal(msg.Payload, &txSubmitted)
		if err != nil {
			return false
		}
		// Verify transaction ID
		if txSubmitted.TransactionId != txID.String() {
			return false
		}
		// Verify contract address
		if txSubmitted.ContractAddress != contractAddress.HexString() {
			return false
		}
		// Verify hash
		if len(txSubmitted.Hash) != 32 {
			return false
		}
		// Verify ID is set (should be a UUID)
		if txSubmitted.Id == "" {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionSubmitted(ctx, originatorNode, testTransactionSubmittedMsg(txID, contractAddress, txHash))
	require.NoError(t, err)
}

func TestSendTransactionSubmitted_Loopback(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "local-node" // Same as local node name
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txHashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	txHash := &txHashVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionSubmitted(ctx, originatorNode, testTransactionSubmittedMsg(txID, contractAddress, txHash))
	require.NoError(t, err)

	// Verify message was sent to loopback queue
	select {
	case msg := <-loopbackQueue:
		assert.Equal(t, MessageType_TransactionSubmitted, msg.MessageType)
		assert.Equal(t, originatorNode, msg.Node)
		// Verify payload
		var txSubmitted engineProto.TransactionSubmitted
		err := proto.Unmarshal(msg.Payload, &txSubmitted)
		require.NoError(t, err)
		assert.Equal(t, txID.String(), txSubmitted.TransactionId)
		assert.Equal(t, contractAddress.HexString(), txSubmitted.ContractAddress)
		assert.Equal(t, txHash.Bytes(), txSubmitted.Hash)
	default:
		t.Fatal("Expected message in loopback queue")
	}

}

func TestSendTransactionSubmitted_EmptyNode(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txHashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	txHash := &txHashVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionSubmitted(ctx, "", testTransactionSubmittedMsg(txID, contractAddress, txHash))
	require.NoError(t, err)

	// Verify no messages were sent
	mockTransportManager.AssertNotCalled(t, "Send")
}

func TestSendTransactionSubmitted_SendError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txHashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	txHash := &txHashVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionSubmitted(ctx, originatorNode, testTransactionSubmittedMsg(txID, contractAddress, txHash))
	require.NoError(t, err)
}

func TestSendTransactionSubmitted_VerifyProtoFields(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txHashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	txHash := &txHashVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	var capturedPayload []byte
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		capturedPayload = msg.Payload
		return true
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionSubmitted(ctx, originatorNode, testTransactionSubmittedMsg(txID, contractAddress, txHash))
	require.NoError(t, err)

	// Unmarshal and verify all fields
	var txSubmitted engineProto.TransactionSubmitted
	err = proto.Unmarshal(capturedPayload, &txSubmitted)
	require.NoError(t, err)

	assert.Equal(t, txID.String(), txSubmitted.TransactionId)
	assert.Equal(t, contractAddress.HexString(), txSubmitted.ContractAddress)
	assert.Equal(t, txHash.Bytes(), txSubmitted.Hash)
	assert.NotEmpty(t, txSubmitted.Id) // Should have a generated UUID

	// Verify the ID is a valid UUID
	_, err = uuid.Parse(txSubmitted.Id)
	assert.NoError(t, err)

}

func TestSendTransactionSubmitted_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txHashVal := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionSubmitted(ctx, "originator-node", testTransactionSubmittedMsg(uuid.New(), contractAddress, &txHashVal))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendDelegationRequest Tests =====

func TestSendDelegationRequest_Success(t *testing.T) {
	ctx := t.Context()
	coordinatorNode := "coordinator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	blockHeight := uint64(100)

	tx1ID := uuid.New()
	tx2ID := uuid.New()
	transactions := []*components.PrivateTransaction{
		{
			ID:      tx1ID,
			Domain:  "test-domain",
			Address: *contractAddress,
			PreAssembly: &prototk.TransactionPreAssembly{
				TransactionSpecification: &prototk.TransactionSpecification{TransactionId: pldtypes.Bytes32UUIDFirst16(tx1ID).String()},
			},
		},
		{
			ID:      tx2ID,
			Domain:  "test-domain",
			Address: *contractAddress,
			PreAssembly: &prototk.TransactionPreAssembly{
				TransactionSpecification: &prototk.TransactionSpecification{TransactionId: pldtypes.Bytes32UUIDFirst16(tx2ID).String()},
			},
		},
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	var capturedRequest engineProto.DelegationRequest
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_DelegationRequest {
			return false
		}
		if msg.Node != coordinatorNode {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		return proto.Unmarshal(msg.Payload, &capturedRequest) == nil
	})).Return(nil).Times(1)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	delegationMsg, err := testDelegationRequestMsg(coordinatorNode, contractAddress, transactions, blockHeight)
	require.NoError(t, err)
	err = tw.SendDelegationRequest(ctx, coordinatorNode, delegationMsg)
	require.NoError(t, err)

	assert.Equal(t, coordinatorNode, capturedRequest.DelegateNodeId)
	assert.Equal(t, int64(blockHeight), capturedRequest.OriginatorBlockHeight)
	require.Len(t, capturedRequest.Transactions, 2)

	assert.Equal(t, pldtypes.Bytes32UUIDFirst16(tx1ID).String(), capturedRequest.Transactions[0].GetPreAssembly().GetTransactionSpecification().GetTransactionId())
	assert.Equal(t, pldtypes.Bytes32UUIDFirst16(tx2ID).String(), capturedRequest.Transactions[1].GetPreAssembly().GetTransactionSpecification().GetTransactionId())
}

func TestSendDelegationRequest_SendError(t *testing.T) {
	ctx := context.Background()
	coordinatorNode := "coordinator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationRequest(ctx, coordinatorNode, &engineProto.DelegationRequest{})
	require.NoError(t, err) // send errors are logged but not propagated
}

func TestSendDelegationRequest_EmptyTransactions(t *testing.T) {
	ctx := context.Background()
	coordinatorNode := "coordinator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	blockHeight := uint64(100)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	var capturedRequest engineProto.DelegationRequest
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		return msg.MessageType == MessageType_DelegationRequest &&
			proto.Unmarshal(msg.Payload, &capturedRequest) == nil
	})).Return(nil).Times(1)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	delegationMsg, err := testDelegationRequestMsg(coordinatorNode, contractAddress, []*components.PrivateTransaction{}, blockHeight)
	require.NoError(t, err)
	err = tw.SendDelegationRequest(ctx, coordinatorNode, delegationMsg)
	require.NoError(t, err)
	assert.Empty(t, capturedRequest.Transactions)
}

func TestSendDelegationRequest_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationRequest(ctx, "coordinator-node", &engineProto.DelegationRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendDelegationResponse Tests =====

func TestSendDelegationResponse_Success(t *testing.T) {
	ctx := context.Background()
	delegatingNodeName := "delegating-node"
	delegationId := "delegation-123"
	transactionIDs := make([]string, 0)
	transactionIDs = append(transactionIDs, uuid.New().String())
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_DelegationResponse {
			return false
		}
		if msg.Node != delegatingNodeName {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var ack engineProto.DelegationResponse
		err := proto.Unmarshal(msg.Payload, &ack)
		if err != nil {
			return false
		}
		if ack.DelegationId != delegationId {
			return false
		}
		for _, transactionID := range ack.TransactionIds {
			if !slices.Contains(transactionIDs, transactionID) {
				return false
			}
		}
		if ack.DelegateNodeId != delegatingNodeName {
			return false
		}
		if ack.ContractAddress != contractAddress.String() {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationResponse(ctx, delegatingNodeName, testDelegationResponseMsg(delegatingNodeName, delegationId, transactionIDs, contractAddress, []int64{0}))
	require.NoError(t, err)
}

func TestSendDelegationResponse_SendError(t *testing.T) {
	ctx := context.Background()
	delegatingNodeName := "delegating-node"
	delegationId := "delegation-123"
	transactionID := uuid.New().String()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationResponse(ctx, delegatingNodeName, testDelegationResponseMsg(delegatingNodeName, delegationId, []string{transactionID}, contractAddress, []int64{0}))
	require.NoError(t, err)
}

func TestSendDelegationResponse_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationResponse(ctx, "delegating-node", &engineProto.DelegationResponse{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendDelegationRejection Tests =====

func TestSendDelegationRejection_Success(t *testing.T) {
	ctx := context.Background()
	delegatingNodeName := "delegating-node"
	delegationId := "delegation-123"
	blockHeight := uint64(100)
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_DelegationRejection {
			return false
		}
		if msg.Node != delegatingNodeName {
			return false
		}
		var ack engineProto.DelegationRejection
		if err := proto.Unmarshal(msg.Payload, &ack); err != nil {
			return false
		}
		return ack.DelegationId == delegationId &&
			ack.DelegateNodeId == delegatingNodeName &&
			ack.ContractAddress == contractAddress.String()
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationRejection(ctx, delegatingNodeName, testDelegationRejectionMsg(delegatingNodeName, delegationId, contractAddress, engineProto.RejectionReason_NOT_CURRENT_DELEGATE, "active-coordinator", int64(blockHeight), int64(90), int64(0)))
	require.NoError(t, err)
}

func TestSendDelegationRejection_SendError(t *testing.T) {
	ctx := context.Background()
	delegatingNodeName := "delegating-node"
	delegationId := "delegation-123"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationRejection(ctx, delegatingNodeName, testDelegationRejectionMsg(delegatingNodeName, delegationId, contractAddress, engineProto.RejectionReason_NOT_CURRENT_DELEGATE, "node1", int64(100), int64(90), int64(0)))
	require.NoError(t, err)
}

func TestSendDelegationRejection_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendDelegationRejection(ctx, "node", &engineProto.DelegationRejection{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendHandoverRequest Tests =====

func TestSendHandoverRequest_Success(t *testing.T) {
	ctx := context.Background()
	targetNode := "target-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()
	mockTM.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_HandoverRequest || msg.Node != targetNode {
			return false
		}
		var req engineProto.CoordinatorHandoverRequest
		if err := proto.Unmarshal(msg.Payload, &req); err != nil {
			return false
		}
		return req.ContractAddress == contractAddress.String()
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendHandoverRequest(ctx, targetNode, &engineProto.CoordinatorHandoverRequest{
		ContractAddress: contractAddress.String(),
	})
	require.NoError(t, err)
}

func TestSendHandoverRequest_SendError(t *testing.T) {
	ctx := context.Background()
	targetNode := "target-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()
	mockTM.On("Send", ctx, mock.Anything).Return(errors.New("send failed"))

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendHandoverRequest(ctx, targetNode, &engineProto.CoordinatorHandoverRequest{})
	require.NoError(t, err) // send errors are logged but not propagated
}

func TestSendHandoverRequest_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendHandoverRequest(ctx, "target-node", &engineProto.CoordinatorHandoverRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendEndorsementRequest Tests =====

func TestSendEndorsementRequest_Success(t *testing.T) {
	ctx := t.Context()
	txID := uuid.New()
	idempotencyKey := uuid.New()
	party := "party1@node1"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	attRequest := &prototk.AttestationRequest{
		Name: "test-attestation",
	}

	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: pldtypes.Bytes32UUIDFirst16(txID).String(),
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: contractAddress.HexString(),
		},
	}

	verifiers := []*prototk.ResolvedVerifier{
		{Lookup: "verifier1"},
	}

	signatures := []*prototk.AttestationResult{
		{Name: "signature1"},
	}

	inputStates := []*prototk.EndorsableState{
		{Id: "input1"},
	}

	readStates := []*prototk.EndorsableState{
		{Id: "read1"},
	}

	outputStates := []*prototk.EndorsableState{
		{Id: "output1"},
	}

	infoStates := []*prototk.EndorsableState{
		{Id: "info1"},
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_EndorsementRequest {
			return false
		}
		if msg.Node != "node1" {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var endorsementRequest engineProto.EndorsementRequest
		err := proto.Unmarshal(msg.Payload, &endorsementRequest)
		if err != nil {
			return false
		}
		if endorsementRequest.GetTransactionSpecification().GetTransactionId() != pldtypes.Bytes32UUIDFirst16(txID).String() {
			return false
		}
		if endorsementRequest.IdempotencyKey != idempotencyKey.String() {
			return false
		}
		if endorsementRequest.Party != party {
			return false
		}
		if endorsementRequest.ContractAddress != contractAddress.HexString() {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendEndorsementRequest(ctx, "node1", testEndorsementRequestMsg(idempotencyKey, party, attRequest, transactionSpecification, verifiers, signatures, inputStates, readStates, outputStates, infoStates, time.Time{}, 0, 0))
	require.NoError(t, err)
}

func TestSendEndorsementRequest_SerialisesExpiryTimeIntoProto(t *testing.T) {
	ctx := t.Context()
	txID := uuid.New()
	idempotencyKey := uuid.New()
	party := "party1@node1"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: pldtypes.Bytes32UUIDFirst16(txID).String(),
		ContractInfo:  &prototk.ContractInfo{ContractAddress: contractAddress.HexString()},
	}
	expiry := time.Now().Truncate(time.Millisecond) // truncate to ms precision to match UnixMilli roundtrip

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	var capturedExpiry int64
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		var req engineProto.EndorsementRequest
		if err := proto.Unmarshal(msg.Payload, &req); err != nil {
			return false
		}
		capturedExpiry = req.ExpiryTimeUnixMs
		return msg.MessageType == MessageType_EndorsementRequest
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}
	err := tw.SendEndorsementRequest(ctx, "node1", testEndorsementRequestMsg(idempotencyKey, party, nil, transactionSpecification, nil, nil, nil, nil, nil, nil, expiry, 0, 0))
	require.NoError(t, err)
	assert.Equal(t, expiry.UnixMilli(), capturedExpiry, "ExpiryTimeUnixMs should match expiry.UnixMilli()")
}

func TestSendEndorsementRequest_EmptyNode(t *testing.T) {
	ctx := t.Context()
	txID := uuid.New()
	idempotencyKey := uuid.New()
	party := "party1@node1"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	attRequest := &prototk.AttestationRequest{
		Name: "test-attestation",
	}

	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: pldtypes.Bytes32UUIDFirst16(txID).String(),
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: contractAddress.HexString(),
		},
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendEndorsementRequest(ctx, "", testEndorsementRequestMsg(idempotencyKey, party, attRequest, transactionSpecification, nil, nil, nil, nil, nil, nil, time.Time{}, 0, 0))
	require.NoError(t, err)
	mockTransportManager.AssertNotCalled(t, "Send")
}

func TestSendEndorsementRequest_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendEndorsementRequest(ctx, "node1", &engineProto.EndorsementRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendEndorsementResponse Tests =====

func TestSendEndorsementResponse_Success(t *testing.T) {
	ctx := context.Background()
	transactionId := uuid.New().String()
	idempotencyKey := uuid.New().String()
	contractAddress := "0x1234567890123456789012345678901234567890"
	endorsementName := "test-endorsement"
	party := "party1"
	node := "target-node"
	revertReason := ""

	attResult := &prototk.AttestationResult{
		Name: "test-result",
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_EndorsementResponse {
			return false
		}
		if msg.Node != node {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var response engineProto.EndorsementResponse
		err := proto.Unmarshal(msg.Payload, &response)
		if err != nil {
			return false
		}
		if response.TransactionId != transactionId {
			return false
		}
		if response.IdempotencyKey != idempotencyKey {
			return false
		}
		if response.ContractAddress != contractAddress {
			return false
		}
		if response.AttestationRequestName != endorsementName {
			return false
		}
		if response.Party != party {
			return false
		}
		if response.RevertReason != nil {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementResponse(ctx, node, testEndorsementResponseMsg(transactionId, idempotencyKey, contractAddress, endorsementName, party, attResult, revertReason))
	require.NoError(t, err)
}

func TestSendEndorsementResponse_WithRevertReason(t *testing.T) {
	ctx := context.Background()
	transactionId := uuid.New().String()
	idempotencyKey := uuid.New().String()
	contractAddress := "0x1234567890123456789012345678901234567890"
	endorsementName := "test-endorsement"
	party := "party1"
	node := "target-node"
	revertReason := "transaction reverted"

	attResult := &prototk.AttestationResult{
		Name: "test-result",
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		var response engineProto.EndorsementResponse
		err := proto.Unmarshal(msg.Payload, &response)
		if err != nil {
			return false
		}
		if response.RevertReason == nil || *response.RevertReason != revertReason {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementResponse(ctx, node, testEndorsementResponseMsg(transactionId, idempotencyKey, contractAddress, endorsementName, party, attResult, revertReason))
	require.NoError(t, err)
}

func TestSendEndorsementResponse_SendError(t *testing.T) {
	ctx := context.Background()
	transactionId := uuid.New().String()
	idempotencyKey := uuid.New().String()
	contractAddress := "0x1234567890123456789012345678901234567890"
	endorsementName := "test-endorsement"
	party := "party1"
	node := "target-node"

	attResult := &prototk.AttestationResult{
		Name: "test-result",
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementResponse(ctx, node, testEndorsementResponseMsg(transactionId, idempotencyKey, contractAddress, endorsementName, party, attResult, ""))
	require.NoError(t, err)
}

func TestSendEndorsementResponse_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := "0x1234567890123456789012345678901234567890"

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementResponse(ctx, "node", &engineProto.EndorsementResponse{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendEndorsementRejection Tests =====

func TestSendEndorsementRejection_Success(t *testing.T) {
	ctx := context.Background()
	transactionId := uuid.New().String()
	idempotencyKey := uuid.New().String()
	contractAddress := "0x1234567890123456789012345678901234567890"
	endorsementName := "att1"
	party := "party1@node2"
	node := "node2"
	coordinatorBlockHeight := int64(100)
	endorserBlockHeight := int64(95)
	blockHeightTolerance := int64(10)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_EndorsementRejection {
			return false
		}
		if msg.Node != node {
			return false
		}
		var r engineProto.EndorsementRejection
		if err := proto.Unmarshal(msg.Payload, &r); err != nil {
			return false
		}
		return r.TransactionId == transactionId &&
			r.IdempotencyKey == idempotencyKey &&
			r.ContractAddress == contractAddress &&
			r.AttestationRequestName == endorsementName &&
			r.Party == party &&
			r.RejectionReason == engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE && // reason passed explicitly
			r.CoordinatorBlockHeight == coordinatorBlockHeight &&
			r.EndorserBlockHeight == endorserBlockHeight &&
			r.BlockHeightTolerance == blockHeightTolerance
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementRejection(ctx, node, testEndorsementRejectionMsg(transactionId, idempotencyKey, contractAddress, endorsementName, party, engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, coordinatorBlockHeight, endorserBlockHeight, blockHeightTolerance))
	require.NoError(t, err)
}

func TestSendEndorsementRejection_SendError(t *testing.T) {
	ctx := context.Background()
	contractAddress := "0x1234567890123456789012345678901234567890"

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementRejection(ctx, "node2", testEndorsementRejectionMsg(uuid.New().String(), uuid.New().String(), contractAddress, "att1", "party1@node2", engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, 100, 95, 10))
	require.NoError(t, err)
}

func TestSendEndorsementRejection_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := "0x1234567890123456789012345678901234567890"

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementRejection(ctx, "node", &engineProto.EndorsementRejection{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendEndorsementError Tests =====

func TestSendEndorsementError_Success(t *testing.T) {
	ctx := context.Background()
	transactionId := uuid.New().String()
	idempotencyKey := uuid.New().String()
	contractAddress := "0x1234567890123456789012345678901234567890"
	errorMessage := "domain returned unexpected error"
	party := "party1@node2"
	attestationRequestName := "att1"
	node := "node2"

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_EndorsementError {
			return false
		}
		if msg.Node != node {
			return false
		}
		var e engineProto.EndorsementError
		if err := proto.Unmarshal(msg.Payload, &e); err != nil {
			return false
		}
		return e.TransactionId == transactionId &&
			e.IdempotencyKey == idempotencyKey &&
			e.ContractAddress == contractAddress &&
			e.ErrorMessage == errorMessage &&
			e.Party == party &&
			e.AttestationRequestName == attestationRequestName
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementError(ctx, node, testEndorsementErrorMsg(transactionId, idempotencyKey, contractAddress, errorMessage, party, attestationRequestName))
	require.NoError(t, err)
}

func TestSendEndorsementError_SendError(t *testing.T) {
	ctx := context.Background()
	contractAddress := "0x1234567890123456789012345678901234567890"

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementError(ctx, "node2", testEndorsementErrorMsg(uuid.New().String(), uuid.New().String(), contractAddress, "some error", "party1@node2", "att1"))
	require.NoError(t, err)
}

func TestSendEndorsementError_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := "0x1234567890123456789012345678901234567890"

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   pldtypes.MustEthAddress(contractAddress),
	}

	err := tw.SendEndorsementError(ctx, "node", &engineProto.EndorsementError{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendAssembleRequest Tests =====

func TestSendAssembleRequest_Success(t *testing.T) {
	ctx := context.Background()
	assemblingNode := "assembling-node"
	txID := uuid.New()
	idempotencyId := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	blockHeight := int64(100)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_AssembleRequest {
			return false
		}
		if msg.Node != assemblingNode {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var assembleRequest engineProto.AssembleRequest
		err := proto.Unmarshal(msg.Payload, &assembleRequest)
		if err != nil {
			return false
		}
		if assembleRequest.TransactionId != txID.String() {
			return false
		}
		if assembleRequest.AssembleRequestId != idempotencyId.String() {
			return false
		}
		if assembleRequest.ContractAddress != contractAddress.HexString() {
			return false
		}
		if assembleRequest.CoordinatorBlockHeight != blockHeight {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	assembleMsg, err := testAssembleRequestMsg(txID, idempotencyId, contractAddress, blockHeight, time.Time{}, 0)
	require.NoError(t, err)
	err = tw.SendAssembleRequest(ctx, assemblingNode, assembleMsg)
	require.NoError(t, err)
}

func TestSendAssembleRequest_SerialisesExpiryTimeIntoProto(t *testing.T) {
	ctx := context.Background()
	assemblingNode := "assembling-node"
	txID := uuid.New()
	idempotencyId := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	blockHeight := int64(100)
	expiry := time.Now().Truncate(time.Millisecond) // truncate to ms precision to match UnixMilli roundtrip

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	var capturedExpiry int64
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		var req engineProto.AssembleRequest
		if err := proto.Unmarshal(msg.Payload, &req); err != nil {
			return false
		}
		capturedExpiry = req.ExpiryTimeUnixMs
		return msg.MessageType == MessageType_AssembleRequest
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}
	assembleMsg, err := testAssembleRequestMsg(txID, idempotencyId, contractAddress, blockHeight, expiry, 0)
	require.NoError(t, err)
	err = tw.SendAssembleRequest(ctx, assemblingNode, assembleMsg)
	require.NoError(t, err)
	assert.Equal(t, expiry.UnixMilli(), capturedExpiry, "ExpiryTimeUnixMs should match expiry.UnixMilli()")
}

func TestSendAssembleRequest_SendError(t *testing.T) {
	ctx := context.Background()
	assemblingNode := "assembling-node"
	txID := uuid.New()
	idempotencyId := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	blockHeight := int64(100)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("send error"))

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	assembleMsg, err := testAssembleRequestMsg(txID, idempotencyId, contractAddress, blockHeight, time.Time{}, 0)
	require.NoError(t, err)
	err = tw.SendAssembleRequest(ctx, assemblingNode, assembleMsg)
	require.NoError(t, err)
}

func TestSendAssembleRequest_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleRequest(ctx, "node", &engineProto.AssembleRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendAssembleResponse Tests =====

func TestSendAssembleResponse_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	assembleRequestId := uuid.New()
	recipient := "recipient-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	preAssembly := &prototk.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{
			TransactionId: txID.String(),
		},
	}

	postAssembly := &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
		},
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_AssembleResponse {
			return false
		}
		if msg.Node != recipient {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var assembleResponse engineProto.AssembleResponse
		err := proto.Unmarshal(msg.Payload, &assembleResponse)
		if err != nil {
			return false
		}
		if assembleResponse.TransactionId != txID.String() {
			return false
		}
		if assembleResponse.AssembleRequestId != assembleRequestId.String() {
			return false
		}
		if assembleResponse.ContractAddress != contractAddress.HexString() {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	assembleResponseMsg, err := testAssembleResponseMsg(txID, assembleRequestId, contractAddress, postAssembly, preAssembly)
	require.NoError(t, err)
	err = tw.SendAssembleResponse(ctx, recipient, assembleResponseMsg)
	require.NoError(t, err)
}

func TestSendAssembleResponse_SendError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	assembleRequestId := uuid.New()
	recipient := "recipient-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	preAssembly := &prototk.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{
			TransactionId: txID.String(),
		},
	}

	postAssembly := &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
		},
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	assembleResponseMsg, err := testAssembleResponseMsg(txID, assembleRequestId, contractAddress, postAssembly, preAssembly)
	require.NoError(t, err)
	err = tw.SendAssembleResponse(ctx, recipient, assembleResponseMsg)
	require.NoError(t, err)
}

func TestSendAssembleResponse_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleResponse(ctx, "recipient", &engineProto.AssembleResponse{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendAssembleError Tests =====

func TestSendAssembleError_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	assembleRequestId := uuid.New()
	recipient := "recipient-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_AssembleError {
			return false
		}
		if msg.Node != recipient {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var assembleError engineProto.AssembleError
		err := proto.Unmarshal(msg.Payload, &assembleError)
		if err != nil {
			return false
		}
		if assembleError.TransactionId != txID.String() {
			return false
		}
		if assembleError.AssembleRequestId != assembleRequestId.String() {
			return false
		}
		if assembleError.ContractAddress != contractAddress.HexString() {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleError(ctx, recipient, testAssembleErrorMsg(txID, assembleRequestId, contractAddress))
	require.NoError(t, err)
}

func TestSendAssembleError_SendError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	assembleRequestId := uuid.New()
	recipient := "recipient-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleError(ctx, recipient, testAssembleErrorMsg(txID, assembleRequestId, contractAddress))
	require.NoError(t, err)
}

func TestSendAssembleError_Loopback(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	assembleRequestId := uuid.New()
	recipient := "local-node" // Same as local node name
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleError(ctx, recipient, testAssembleErrorMsg(txID, assembleRequestId, contractAddress))
	require.NoError(t, err)

	select {
	case msg := <-loopbackQueue:
		assert.Equal(t, MessageType_AssembleError, msg.MessageType)
		assert.Equal(t, recipient, msg.Node)
		var assembleError engineProto.AssembleError
		err := proto.Unmarshal(msg.Payload, &assembleError)
		require.NoError(t, err)
		assert.Equal(t, txID.String(), assembleError.TransactionId)
		assert.Equal(t, assembleRequestId.String(), assembleError.AssembleRequestId)
		assert.Equal(t, contractAddress.HexString(), assembleError.ContractAddress)
	default:
		t.Fatal("Expected message in loopback queue")
	}

}

func TestSendAssembleError_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleError(ctx, "recipient-node", &engineProto.AssembleError{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendAssembleRejection Tests =====

func TestSendAssembleRejection_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	assembleRequestId := uuid.New()
	recipient := "recipient-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	coordinatorBlockHeight := int64(100)
	assemblerBlockHeight := int64(95)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_AssembleRejection {
			return false
		}
		if msg.Node != recipient {
			return false
		}
		var r engineProto.AssembleRejection
		if err := proto.Unmarshal(msg.Payload, &r); err != nil {
			return false
		}
		return r.TransactionId == txID.String() &&
			r.AssembleRequestId == assembleRequestId.String() &&
			r.ContractAddress == contractAddress.HexString() &&
			r.RejectionReason == engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE &&
			r.CoordinatorBlockHeight == coordinatorBlockHeight &&
			r.AssemblerBlockHeight == assemblerBlockHeight
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleRejection(ctx, recipient, testAssembleRejectionMsg(txID, assembleRequestId, contractAddress, engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, coordinatorBlockHeight, assemblerBlockHeight))
	require.NoError(t, err)
}

func TestSendAssembleRejection_SendError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	assembleRequestId := uuid.New()
	recipient := "recipient-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleRejection(ctx, recipient, testAssembleRejectionMsg(txID, assembleRequestId, contractAddress, engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE, 100, 95))
	require.NoError(t, err)
}

func TestSendAssembleRejection_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendAssembleRejection(ctx, "recipient-node", &engineProto.AssembleRejection{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendNonceAssigned Tests =====

func TestSendNonceAssigned_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonce := uint64(42)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_NonceAssigned {
			return false
		}
		if msg.Node != originatorNode {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var nonceAssigned engineProto.NonceAssigned
		err := proto.Unmarshal(msg.Payload, &nonceAssigned)
		if err != nil {
			return false
		}
		if nonceAssigned.TransactionId != txID.String() {
			return false
		}
		if nonceAssigned.ContractAddress != contractAddress.HexString() {
			return false
		}
		if nonceAssigned.Nonce != int64(nonce) {
			return false
		}
		if nonceAssigned.Id == "" {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendNonceAssigned(ctx, originatorNode, testNonceAssignedMsg(txID, contractAddress, nonce))
	require.NoError(t, err)
}

func TestSendNonceAssigned_EmptyNode(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonce := uint64(42)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendNonceAssigned(ctx, "", testNonceAssignedMsg(txID, contractAddress, nonce))
	require.NoError(t, err)
	mockTransportManager.AssertNotCalled(t, "Send")
}

func TestSendNonceAssigned_SendError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonce := uint64(42)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendNonceAssigned(ctx, originatorNode, testNonceAssignedMsg(txID, contractAddress, nonce))
	require.NoError(t, err)
}

func TestSendNonceAssigned_VerifyGeneratedId(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonce := uint64(42)

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	var capturedPayload []byte
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		capturedPayload = msg.Payload
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendNonceAssigned(ctx, originatorNode, testNonceAssignedMsg(txID, contractAddress, nonce))
	require.NoError(t, err)

	var nonceAssigned engineProto.NonceAssigned
	err = proto.Unmarshal(capturedPayload, &nonceAssigned)
	require.NoError(t, err)

	// Verify the ID is a valid UUID
	_, err = uuid.Parse(nonceAssigned.Id)
	assert.NoError(t, err)

}

func TestSendNonceAssigned_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendNonceAssigned(ctx, "originator-node", &engineProto.NonceAssigned{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendTransactionConfirmed Tests =====

func TestSendTransactionConfirmed_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_TransactionConfirmed {
			return false
		}
		if msg.Node != originatorNode {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var txConfirmed engineProto.TransactionConfirmed
		err := proto.Unmarshal(msg.Payload, &txConfirmed)
		if err != nil {
			return false
		}
		if txConfirmed.TransactionId != txID.String() {
			return false
		}
		if txConfirmed.ContractAddress != contractAddress.HexString() {
			return false
		}
		if txConfirmed.Nonce != int64(*nonce) {
			return false
		}
		if string(txConfirmed.RevertReason) != string(revertReason) {
			return false
		}
		if txConfirmed.Id == "" {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, originatorNode, testTransactionConfirmedMsg(txID, contractAddress, nonce, engineProto.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "decoded failure", false))
	require.NoError(t, err)
}

func TestSendTransactionConfirmed_WithoutNonce(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		var txConfirmed engineProto.TransactionConfirmed
		err := proto.Unmarshal(msg.Payload, &txConfirmed)
		if err != nil {
			return false
		}
		// Nonce should be 0 when not provided
		if txConfirmed.Nonce != 0 {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, originatorNode, testTransactionConfirmedMsg(txID, contractAddress, nil, engineProto.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "decoded failure", false))
	require.NoError(t, err)
}

func TestSendTransactionConfirmed_EmptyNode(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, "", testTransactionConfirmedMsg(txID, contractAddress, nonce, engineProto.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "decoded failure", false))
	require.NoError(t, err)
	mockTransportManager.AssertNotCalled(t, "Send")
}

func TestSendTransactionConfirmed_SendError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, originatorNode, testTransactionConfirmedMsg(txID, contractAddress, nonce, engineProto.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "decoded failure", false))
	require.NoError(t, err)
}

func TestSendTransactionConfirmed_Loopback(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "local-node" // Same as local node name
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, originatorNode, testTransactionConfirmedMsg(txID, contractAddress, nonce, engineProto.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "decoded failure", true))
	require.NoError(t, err)

	// Verify message was sent to loopback queue
	select {
	case msg := <-loopbackQueue:
		assert.Equal(t, MessageType_TransactionConfirmed, msg.MessageType)
		assert.Equal(t, originatorNode, msg.Node)
		// Verify payload
		var txConfirmed engineProto.TransactionConfirmed
		err := proto.Unmarshal(msg.Payload, &txConfirmed)
		require.NoError(t, err)
		assert.Equal(t, txID.String(), txConfirmed.TransactionId)
		assert.Equal(t, contractAddress.HexString(), txConfirmed.ContractAddress)
		assert.Equal(t, int64(*nonce), txConfirmed.Nonce)
		assert.Equal(t, revertReason, pldtypes.HexBytes(txConfirmed.RevertReason))
		assert.True(t, txConfirmed.WillRetry)
	default:
		t.Fatal("Expected message in loopback queue")
	}

}

func TestSendTransactionConfirmed_WillRetryTrue(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		var txConfirmed engineProto.TransactionConfirmed
		err := proto.Unmarshal(msg.Payload, &txConfirmed)
		if err != nil {
			return false
		}
		if txConfirmed.TransactionId != txID.String() {
			return false
		}
		if !txConfirmed.WillRetry {
			return false
		}
		if string(txConfirmed.RevertReason) != string(revertReason) {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, originatorNode, testTransactionConfirmedMsg(txID, contractAddress, nonce, engineProto.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "decoded failure", true))
	require.NoError(t, err)
}

func TestSendTransactionConfirmed_WillRetryFalse(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		var txConfirmed engineProto.TransactionConfirmed
		err := proto.Unmarshal(msg.Payload, &txConfirmed)
		if err != nil {
			return false
		}
		if txConfirmed.WillRetry {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, originatorNode, testTransactionConfirmedMsg(txID, contractAddress, nonce, engineProto.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "decoded failure", false))
	require.NoError(t, err)
}

func TestSendTransactionConfirmed_NilRevertReason(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "originator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		var txConfirmed engineProto.TransactionConfirmed
		err := proto.Unmarshal(msg.Payload, &txConfirmed)
		if err != nil {
			return false
		}
		if txConfirmed.TransactionId != txID.String() {
			return false
		}
		if len(txConfirmed.RevertReason) != 0 {
			return false
		}
		if txConfirmed.WillRetry {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, originatorNode, testTransactionConfirmedMsg(txID, contractAddress, nonce, engineProto.TransactionConfirmed_OUTCOME_SUCCESS, nil, "", false))
	require.NoError(t, err)
}

func TestSendTransactionConfirmed_Loopback_WillRetryFalse(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	originatorNode := "local-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	nonceVal := pldtypes.HexUint64(42)
	nonce := &nonceVal
	revertReason := pldtypes.HexBytes([]byte("revert reason"))

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, originatorNode, testTransactionConfirmedMsg(txID, contractAddress, nonce, engineProto.TransactionConfirmed_OUTCOME_REVERTED, revertReason, "decoded failure", false))
	require.NoError(t, err)

	select {
	case msg := <-loopbackQueue:
		var txConfirmed engineProto.TransactionConfirmed
		err := proto.Unmarshal(msg.Payload, &txConfirmed)
		require.NoError(t, err)
		assert.Equal(t, txID.String(), txConfirmed.TransactionId)
		assert.Equal(t, revertReason, pldtypes.HexBytes(txConfirmed.RevertReason))
		assert.False(t, txConfirmed.WillRetry)
	default:
		t.Fatal("Expected message in loopback queue")
	}
}

func TestSendTransactionConfirmed_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendTransactionConfirmed(ctx, "originator-node", &engineProto.TransactionConfirmed{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendHeartbeat Tests =====

func TestSendHeartbeat_Success(t *testing.T) {
	ctx := context.Background()
	targetNode := "target-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	coordinatorSnapshot := &engineProto.CoordinatorSnapshot{
		CoordinatorState:       int32(common.CoordinatorState_Idle),
		BlockHeight:            100,
		PooledTransactions:     []*engineProto.SnapshotPooledTransaction{},
		DispatchedTransactions: []*engineProto.SnapshotDispatchedTransaction{},
		ConfirmedTransactions:  []*engineProto.SnapshotConfirmedTransaction{},
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_CoordinatorHeartbeatNotification {
			return false
		}
		if msg.Node != targetNode {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var heartbeat engineProto.CoordinatorHeartbeatNotification
		err := proto.Unmarshal(msg.Payload, &heartbeat)
		if err != nil {
			return false
		}
		if heartbeat.ContractAddress != contractAddress.HexString() {
			return false
		}
		// Verify coordinator snapshot is carried natively as a proto
		snapshot := heartbeat.GetCoordinatorSnapshot()
		if snapshot == nil {
			return false
		}
		if int(snapshot.GetCoordinatorState()) != int(coordinatorSnapshot.CoordinatorState) {
			return false
		}
		if snapshot.GetBlockHeight() != coordinatorSnapshot.BlockHeight {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	heartbeatMsg, err := testHeartbeatMsg(contractAddress, coordinatorSnapshot)
	require.NoError(t, err)
	err = tw.SendHeartbeat(ctx, targetNode, heartbeatMsg)
	require.NoError(t, err)
}

func TestSendHeartbeat_SendError(t *testing.T) {
	ctx := context.Background()
	targetNode := "target-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	coordinatorSnapshot := &engineProto.CoordinatorSnapshot{
		CoordinatorState: int32(common.CoordinatorState_Idle),
		BlockHeight:      100,
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	heartbeatMsg, err := testHeartbeatMsg(contractAddress, coordinatorSnapshot)
	require.NoError(t, err)
	err = tw.SendHeartbeat(ctx, targetNode, heartbeatMsg)
	require.NoError(t, err)
}

func TestSendHeartbeat_Loopback(t *testing.T) {
	ctx := context.Background()
	targetNode := "local-node" // Same as local node name
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	coordinatorSnapshot := &engineProto.CoordinatorSnapshot{
		CoordinatorState: int32(common.CoordinatorState_Idle),
		BlockHeight:      100,
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	heartbeatMsg, err := testHeartbeatMsg(contractAddress, coordinatorSnapshot)
	require.NoError(t, err)
	err = tw.SendHeartbeat(ctx, targetNode, heartbeatMsg)
	require.NoError(t, err)

	// Verify message was sent to loopback queue
	select {
	case msg := <-loopbackQueue:
		assert.Equal(t, MessageType_CoordinatorHeartbeatNotification, msg.MessageType)
		assert.Equal(t, targetNode, msg.Node)
		// Verify payload
		var heartbeat engineProto.CoordinatorHeartbeatNotification
		err := proto.Unmarshal(msg.Payload, &heartbeat)
		require.NoError(t, err)
		assert.Equal(t, contractAddress.HexString(), heartbeat.ContractAddress)
	default:
		t.Fatal("Expected message in loopback queue")
	}

}

func TestSendHeartbeat_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	origProto := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = origProto }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendHeartbeat(ctx, "target-node", &engineProto.CoordinatorHeartbeatNotification{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendPreDispatchRequest Tests =====

func TestSendPreDispatchRequest_Success(t *testing.T) {
	ctx := context.Background()
	originatorNode := "originator-node"
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_PreDispatchRequest {
			return false
		}
		if msg.Node != originatorNode {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var dispatchRequest engineProto.PreDispatchRequest
		err := proto.Unmarshal(msg.Payload, &dispatchRequest)
		if err != nil {
			return false
		}
		if dispatchRequest.Id != idempotencyKey.String() {
			return false
		}
		if dispatchRequest.TransactionId != transactionSpecification.TransactionId {
			return false
		}
		if dispatchRequest.ContractAddress != contractAddress.HexString() {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchRequest(ctx, originatorNode, testPreDispatchRequestMsg(idempotencyKey, transactionSpecification, contractAddress))
	require.NoError(t, err)
}

func TestSendPreDispatchRequest_SendError(t *testing.T) {
	ctx := context.Background()
	originatorNode := "originator-node"
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchRequest(ctx, originatorNode, testPreDispatchRequestMsg(idempotencyKey, transactionSpecification, contractAddress))
	require.NoError(t, err)
}

func TestSendPreDispatchRequest_Loopback(t *testing.T) {
	ctx := context.Background()
	originatorNode := "local-node" // Same as local node name
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchRequest(ctx, originatorNode, testPreDispatchRequestMsg(idempotencyKey, transactionSpecification, contractAddress))
	require.NoError(t, err)

	// Verify message was sent to loopback queue
	select {
	case msg := <-loopbackQueue:
		assert.Equal(t, MessageType_PreDispatchRequest, msg.MessageType)
		assert.Equal(t, originatorNode, msg.Node)
		// Verify payload
		var dispatchRequest engineProto.PreDispatchRequest
		err := proto.Unmarshal(msg.Payload, &dispatchRequest)
		require.NoError(t, err)
		assert.Equal(t, idempotencyKey.String(), dispatchRequest.Id)
		assert.Equal(t, transactionSpecification.TransactionId, dispatchRequest.TransactionId)
		assert.Equal(t, contractAddress.HexString(), dispatchRequest.ContractAddress)
	default:
		t.Fatal("Expected message in loopback queue")
	}

}

func TestSendPreDispatchRequest_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchRequest(ctx, "originator-node", &engineProto.PreDispatchRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendPreDispatchResponse Tests =====

func TestSendPreDispatchResponse_Success(t *testing.T) {
	ctx := context.Background()
	transactionOriginatorNode := "originator-node"
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_PreDispatchResponse {
			return false
		}
		if msg.Node != transactionOriginatorNode {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var dispatchResponse engineProto.PreDispatchResponse
		err := proto.Unmarshal(msg.Payload, &dispatchResponse)
		if err != nil {
			return false
		}
		if dispatchResponse.Id != idempotencyKey.String() {
			return false
		}
		if dispatchResponse.TransactionId != transactionSpecification.TransactionId {
			return false
		}
		if dispatchResponse.ContractAddress != contractAddress.HexString() {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchResponse(ctx, transactionOriginatorNode, testPreDispatchResponseMsg(idempotencyKey, transactionSpecification, contractAddress))
	require.NoError(t, err)
}

func TestSendPreDispatchResponse_SendError(t *testing.T) {
	ctx := context.Background()
	transactionOriginatorNode := "originator-node"
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchResponse(ctx, transactionOriginatorNode, testPreDispatchResponseMsg(idempotencyKey, transactionSpecification, contractAddress))
	require.NoError(t, err)
}

func TestSendPreDispatchResponse_Loopback(t *testing.T) {
	ctx := context.Background()
	transactionOriginatorNode := "local-node" // Same as local node name
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchResponse(ctx, transactionOriginatorNode, testPreDispatchResponseMsg(idempotencyKey, transactionSpecification, contractAddress))
	require.NoError(t, err)

	// Verify message was sent to loopback queue
	select {
	case msg := <-loopbackQueue:
		assert.Equal(t, MessageType_PreDispatchResponse, msg.MessageType)
		assert.Equal(t, transactionOriginatorNode, msg.Node)
		// Verify payload
		var dispatchResponse engineProto.PreDispatchResponse
		err := proto.Unmarshal(msg.Payload, &dispatchResponse)
		require.NoError(t, err)
		assert.Equal(t, idempotencyKey.String(), dispatchResponse.Id)
		assert.Equal(t, transactionSpecification.TransactionId, dispatchResponse.TransactionId)
		assert.Equal(t, contractAddress.HexString(), dispatchResponse.ContractAddress)
	default:
		t.Fatal("Expected message in loopback queue")
	}

}

func TestSendPreDispatchResponse_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchResponse(ctx, "originator-node", &engineProto.PreDispatchResponse{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendDispatched Tests =====

func TestSendDispatched_Success(t *testing.T) {
	ctx := context.Background()
	transactionOriginator := "originator@originator-node"
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_Dispatched {
			return false
		}
		if msg.Node != "originator-node" {
			return false
		}
		if msg.Component.String() != "TRANSACTION_ENGINE" {
			return false
		}
		var dispatched engineProto.TransactionDispatched
		err := proto.Unmarshal(msg.Payload, &dispatched)
		if err != nil {
			return false
		}
		if dispatched.Id != idempotencyKey.String() {
			return false
		}
		if dispatched.TransactionId != transactionSpecification.TransactionId {
			return false
		}
		if dispatched.ContractAddress != contractAddress.HexString() {
			return false
		}
		if dispatched.Signer != transactionOriginator {
			return false
		}
		return true
	})).Return(nil)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDispatched(ctx, "originator-node", testDispatchedMsg(idempotencyKey, transactionSpecification, contractAddress, transactionOriginator))
	require.NoError(t, err)
}

func TestSendDispatched_EmptyNode(t *testing.T) {
	ctx := context.Background()
	transactionOriginator := "originator@originator-node"
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDispatched(ctx, "", testDispatchedMsg(idempotencyKey, transactionSpecification, contractAddress, transactionOriginator))
	require.NoError(t, err)
	mockTransportManager.AssertNotCalled(t, "Send")
}

func TestSendDispatched_SendError(t *testing.T) {
	ctx := context.Background()
	transactionOriginator := "originator@originator-node"
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("transport send error"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDispatched(ctx, "originator-node", testDispatchedMsg(idempotencyKey, transactionSpecification, contractAddress, transactionOriginator))
	require.NoError(t, err)
}

func TestSendDispatched_Loopback(t *testing.T) {
	ctx := context.Background()
	transactionOriginator := "originator@local-node" // Node is same as local node name
	idempotencyKey := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()
	transactionSpecification := &prototk.TransactionSpecification{
		TransactionId: txID.String(),
	}

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	loopbackQueue := make(chan *components.FireAndForgetMessageSend, 1)
	mockLoopbackTransport.On("LoopbackQueue").Return(loopbackQueue).Maybe()
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendDispatched(ctx, "local-node", testDispatchedMsg(idempotencyKey, transactionSpecification, contractAddress, transactionOriginator))
	require.NoError(t, err)

	// Verify message was sent to loopback queue
	select {
	case msg := <-loopbackQueue:
		assert.Equal(t, MessageType_Dispatched, msg.MessageType)
		assert.Equal(t, "local-node", msg.Node)
		// Verify payload
		var dispatched engineProto.TransactionDispatched
		err := proto.Unmarshal(msg.Payload, &dispatched)
		require.NoError(t, err)
		assert.Equal(t, idempotencyKey.String(), dispatched.Id)
		assert.Equal(t, transactionSpecification.TransactionId, dispatched.TransactionId)
		assert.Equal(t, contractAddress.HexString(), dispatched.ContractAddress)
		assert.Equal(t, transactionOriginator, dispatched.Signer)
	default:
		t.Fatal("Expected message in loopback queue")
	}

}

func TestSendDispatched_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendDispatched(ctx, "originator-node", &engineProto.TransactionDispatched{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

// ===== SendPreDispatchRejection Tests =====

func TestSendPreDispatchRejection_Success(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	requestID := uuid.New()
	coordinatorNode := "coordinator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_PreDispatchRejection {
			return false
		}
		if msg.Node != coordinatorNode {
			return false
		}
		var r engineProto.PreDispatchRejection
		if err := proto.Unmarshal(msg.Payload, &r); err != nil {
			return false
		}
		return r.TransactionId == txID.String() &&
			r.RequestId == requestID.String() &&
			r.ContractAddress == contractAddress.HexString() &&
			r.RejectionReason == engineProto.RejectionReason_NOT_CURRENT_DELEGATE
	})).Return(nil)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchRejection(ctx, coordinatorNode, testPreDispatchRejectionMsg(txID, requestID, contractAddress, engineProto.RejectionReason_NOT_CURRENT_DELEGATE))
	require.NoError(t, err)
}

func TestSendPreDispatchRejection_EmptyNode(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	requestID := uuid.New()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchRejection(ctx, "", testPreDispatchRejectionMsg(txID, requestID, contractAddress, engineProto.RejectionReason_NOT_CURRENT_DELEGATE))
	require.NoError(t, err)
	mockTransportManager.AssertNotCalled(t, "Send")
}

func TestSendPreDispatchRejection_SendError(t *testing.T) {
	ctx := context.Background()
	txID := uuid.New()
	requestID := uuid.New()
	coordinatorNode := "coordinator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockLoopbackTransport := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTransportManager.On("LocalNodeName").Return("local-node").Maybe()
	mockTransportManager.On("Send", ctx, mock.Anything).Return(errors.New("send failed"))

	tw := &transportWriter{
		nodeID:            "local-node",
		transportManager:  mockTransportManager,
		loopbackTransport: mockLoopbackTransport,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchRejection(ctx, coordinatorNode, testPreDispatchRejectionMsg(txID, requestID, contractAddress, engineProto.RejectionReason_TRANSACTION_UNKNOWN))
	require.NoError(t, err)
}

func TestSendPreDispatchRejection_ProtoMarshalError(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	orig := protoMarshalFn
	protoMarshalFn = func(_ proto.Message) ([]byte, error) { return nil, errors.New("forced proto error") }
	defer func() { protoMarshalFn = orig }()

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)

	tw := &transportWriter{
		ctx:               ctx,
		nodeID:            "local-node",
		transportManager:  mockTM,
		loopbackTransport: mockLT,
		contractAddress:   contractAddress,
	}

	err := tw.SendPreDispatchRejection(ctx, "coordinator-node", &engineProto.PreDispatchRejection{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced proto error")
}

func TestSendSignResponse_Success(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()
	mockTM.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_SignResponse || msg.Node != "coordinator-node" {
			return false
		}
		var sr engineProto.SignResponse
		return proto.Unmarshal(msg.Payload, &sr) == nil && sr.TransactionId == "tx1"
	})).Return(nil)

	tw := &transportWriter{ctx: ctx, nodeID: "local-node", transportManager: mockTM, loopbackTransport: mockLT, contractAddress: contractAddress}

	err := tw.SendSignResponse(ctx, "coordinator-node", &engineProto.SignResponse{TransactionId: "tx1"})
	require.NoError(t, err)
}

func TestSendSignError_Success(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()
	mockTM.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		return msg.MessageType == MessageType_SignError && msg.Node == "coordinator-node"
	})).Return(nil)

	tw := &transportWriter{ctx: ctx, nodeID: "local-node", transportManager: mockTM, loopbackTransport: mockLT, contractAddress: contractAddress}

	err := tw.SendSignError(ctx, "coordinator-node", &engineProto.SignError{TransactionId: "tx1"})
	require.NoError(t, err)
}

func TestSendQueryAvailableStatesRequest_Success(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()
	mockTM.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_QueryAvailableStatesRequest || msg.Node != "coordinator-node" {
			return false
		}
		var req engineProto.QueryAvailableStatesRequest
		return proto.Unmarshal(msg.Payload, &req) == nil && req.RequestId == "req1" && req.SchemaId == "0xaa"
	})).Return(nil)

	tw := &transportWriter{ctx: ctx, nodeID: "local-node", transportManager: mockTM, loopbackTransport: mockLT, contractAddress: contractAddress}

	err := tw.SendQueryAvailableStatesRequest(ctx, "coordinator-node", &engineProto.QueryAvailableStatesRequest{RequestId: "req1", SchemaId: "0xaa", QueryJson: "{}"})
	require.NoError(t, err)
}

func TestSendQueryAvailableStatesResponse_Success(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()
	mockTM.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_QueryAvailableStatesResponse || msg.Node != "originator-node" {
			return false
		}
		var resp engineProto.QueryAvailableStatesResponse
		return proto.Unmarshal(msg.Payload, &resp) == nil && resp.RequestId == "req1" && len(resp.States) == 1
	})).Return(nil)

	tw := &transportWriter{ctx: ctx, nodeID: "local-node", transportManager: mockTM, loopbackTransport: mockLT, contractAddress: contractAddress}

	err := tw.SendQueryAvailableStatesResponse(ctx, "originator-node", &engineProto.QueryAvailableStatesResponse{
		RequestId: "req1",
		States:    []*prototk.QueriedState{{State: &prototk.EndorsableState{Id: "0xaa"}}},
	})
	require.NoError(t, err)
}

func TestSendGetSpentStateIDsRequest_Success(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()
	mockTM.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_GetSpentStateIDsRequest || msg.Node != "coordinator-node" {
			return false
		}
		var req engineProto.GetSpentStateIDsRequest
		return proto.Unmarshal(msg.Payload, &req) == nil && req.RequestId == "req1" && req.AssembleRequestId == "assemble-1"
	})).Return(nil)

	tw := &transportWriter{ctx: ctx, nodeID: "local-node", transportManager: mockTM, loopbackTransport: mockLT, contractAddress: contractAddress}

	err := tw.SendGetSpentStateIDsRequest(ctx, "coordinator-node", &engineProto.GetSpentStateIDsRequest{RequestId: "req1", AssembleRequestId: "assemble-1"})
	require.NoError(t, err)
}

func TestSendGetSpentStateIDsResponse_Success(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()
	mockTM.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_GetSpentStateIDsResponse || msg.Node != "originator-node" {
			return false
		}
		var resp engineProto.GetSpentStateIDsResponse
		return proto.Unmarshal(msg.Payload, &resp) == nil && resp.RequestId == "req1" && len(resp.SpentStateIds) == 1
	})).Return(nil)

	tw := &transportWriter{ctx: ctx, nodeID: "local-node", transportManager: mockTM, loopbackTransport: mockLT, contractAddress: contractAddress}

	err := tw.SendGetSpentStateIDsResponse(ctx, "originator-node", &engineProto.GetSpentStateIDsResponse{
		RequestId:     "req1",
		SpentStateIds: [][]byte{{0x01, 0x02}},
	})
	require.NoError(t, err)
}

func TestSendStateViewError_Success(t *testing.T) {
	ctx := context.Background()
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	mockTM := componentsmocks.NewTransportManager(t)
	mockLT := sequencertransportmocks.NewLoopbackTransportManager(t)
	mockTM.On("LocalNodeName").Return("local-node").Maybe()
	mockTM.On("Send", ctx, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		if msg.MessageType != MessageType_StateViewError || msg.Node != "originator-node" {
			return false
		}
		var errMsg engineProto.StateViewError
		return proto.Unmarshal(msg.Payload, &errMsg) == nil && errMsg.RequestId == "req1" && errMsg.ErrorMessage == "pop"
	})).Return(nil)

	tw := &transportWriter{ctx: ctx, nodeID: "local-node", transportManager: mockTM, loopbackTransport: mockLT, contractAddress: contractAddress}

	err := tw.SendStateViewError(ctx, "originator-node", &engineProto.StateViewError{RequestId: "req1", ErrorMessage: "pop"})
	require.NoError(t, err)
}
