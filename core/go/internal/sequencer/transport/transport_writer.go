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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/common"
	engineProto "github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/proto/engine"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type TransportWriter interface {
	SendDelegationRequest(ctx context.Context, coordinatorLocator string, transactions []*components.PrivateTransaction, blockHeight uint64) error
	SendDelegationRequestAcknowledgment(ctx context.Context, delegatingNodeName string, delegationId string, delegateNodeName string, transactionID string) error
	SendEndorsementRequest(ctx context.Context, txID uuid.UUID, idempotencyKey uuid.UUID, party string, attRequest *prototk.AttestationRequest, transactionSpecification *prototk.TransactionSpecification, verifiers []*prototk.ResolvedVerifier, signatures []*prototk.AttestationResult, inputStates []*prototk.EndorsableState, readStates []*prototk.EndorsableState, outputStates []*prototk.EndorsableState, infoStates []*prototk.EndorsableState) error
	SendEndorsementResponse(ctx context.Context, transactionId, idempotencyKey, contractAddress string, attResult *prototk.AttestationResult, endorsementResult *components.EndorsementResult, revertReason, endorsementName, party, node string) error
	SendAssembleRequest(ctx context.Context, assemblingNode string, txID uuid.UUID, idempotencyId uuid.UUID, preAssembly *components.TransactionPreAssembly, stateLocksJSON []byte, blockHeight int64) error
	SendAssembleResponse(ctx context.Context, txID uuid.UUID, assembleRequestId uuid.UUID, postAssembly *components.TransactionPostAssembly, preAssembly *components.TransactionPreAssembly, recipient string) error
	SendHandoverRequest(ctx context.Context, activeCoordinator string, contractAddress *pldtypes.EthAddress) error
	SendNonceAssigned(ctx context.Context, txID uuid.UUID, originatorNode string, contractAddress *pldtypes.EthAddress, nonce uint64) error
	SendTransactionSubmitted(ctx context.Context, txID uuid.UUID, originatorNode string, contractAddress *pldtypes.EthAddress, txHash *pldtypes.Bytes32) error
	SendTransactionConfirmed(ctx context.Context, txID uuid.UUID, originatorNode string, contractAddress *pldtypes.EthAddress, nonce *pldtypes.HexUint64, revertReason pldtypes.HexBytes) error
	SendHeartbeat(ctx context.Context, targetNode string, contractAddress *pldtypes.EthAddress, coordinatorSnapshot *common.CoordinatorSnapshot) error
	SendPreDispatchRequest(ctx context.Context, originatorNode string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification, hash *pldtypes.Bytes32) error
	SendPreDispatchResponse(ctx context.Context, transactionOriginator string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification) error
	SendDispatched(ctx context.Context, transactionOriginator string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification) error
}

func NewTransportWriter(contractAddress *pldtypes.EthAddress, nodeID string, transportManager components.TransportManager, loopbackHandler func(ctx context.Context, message *components.ReceivedMessage)) TransportWriter {
	loopbackTransport := NewLoopbackTransportWriter(loopbackHandler)
	return &transportWriter{
		nodeID:            nodeID,
		transportManager:  transportManager,
		loopbackTransport: loopbackTransport,
		contractAddress:   contractAddress,
	}
}

type transportWriter struct {
	nodeID            string
	transportManager  components.TransportManager
	loopbackTransport LoopbackTransportManager
	contractAddress   *pldtypes.EthAddress
}

func (tw *transportWriter) SendDelegationRequest(
	ctx context.Context,
	coordinatorLocator string,
	transactions []*components.PrivateTransaction,
	blockHeight uint64,
) error {
	for _, transaction := range transactions {

		transactionBytes, err := json.Marshal(transaction)

		if err != nil {
			log.L(ctx).Errorf("error marshalling transaction message: %s", err)
		}
		delegationRequest := &engineProto.DelegationRequest{
			// DelegationId:       delegationId,
			TransactionId: transaction.ID.String(),
			// DelegateNodeId:     delegateNodeId,
			PrivateTransaction: transactionBytes,
			BlockHeight:        int64(blockHeight),
		}
		delegationRequestBytes, err := proto.Marshal(delegationRequest)
		if err != nil {
			log.L(ctx).Errorf("error marshalling delegationRequest  message: %s", err)
		}

		parts := strings.Split(coordinatorLocator, "@")
		node := parts[0]
		if len(parts) > 1 {
			node = parts[1]
		}

		log.L(log.WithComponent(ctx, common.SUBCOMP_MSGTX)).Debugf("delegate | TX   | %s | %s", transaction.ID.String()[0:8], node)
		if err = tw.send(ctx, &components.FireAndForgetMessageSend{
			MessageType: "DelegationRequest",
			Payload:     delegationRequestBytes,
			Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
			Node:        node,
		}); err != nil {
			log.L(ctx).Errorf("error sending delegationRequest message: %s", err)
		}
	}
	return nil
}

func (tw *transportWriter) SendDelegationRequestAcknowledgment(
	ctx context.Context,
	delegatingNodeName string,
	delegationId string,
	delegateNodeName string,
	transactionID string,

) error {

	delegationRequestAcknowledgment := &engineProto.DelegationRequestAcknowledgment{
		DelegationId:    delegationId,
		TransactionId:   transactionID,
		DelegateNodeId:  delegateNodeName,
		ContractAddress: tw.contractAddress.String(),
	}
	delegationRequestAcknowledgmentBytes, err := proto.Marshal(delegationRequestAcknowledgment)
	if err != nil {
		log.L(ctx).Errorf("error marshalling delegationRequestAcknowledgment  message: %s", err)
		return err
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: "DelegationRequestAcknowledgment",
		Payload:     delegationRequestAcknowledgmentBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        delegatingNodeName,
	}); err != nil {
		return err
	}
	return nil
}

// TODO do we have duplication here?  contractAddress and transactionID are in the transactionSpecification
func (tw *transportWriter) SendEndorsementRequest(ctx context.Context, txID uuid.UUID, idempotencyKey uuid.UUID, party string, attRequest *prototk.AttestationRequest, transactionSpecification *prototk.TransactionSpecification, verifiers []*prototk.ResolvedVerifier, signatures []*prototk.AttestationResult, inputStates []*prototk.EndorsableState, readStates []*prototk.EndorsableState, outputStates []*prototk.EndorsableState, infoStates []*prototk.EndorsableState) error {
	attRequestAny, err := anypb.New(attRequest)
	if err != nil {
		log.L(ctx).Error("error marshalling attestation request", err)
		return err
	}

	transactionSpecificationAny, err := anypb.New(transactionSpecification)
	if err != nil {
		log.L(ctx).Error("error marshalling transaction specification", err)
		return err
	}

	verifiersAny := make([]*anypb.Any, len(verifiers))
	for i, verifier := range verifiers {
		log.L(ctx).Debugf("marshalling endorsement requestverifier %s", verifier.String())
		verifierAny, err := anypb.New(verifier)
		if err != nil {
			log.L(ctx).Error("error marshalling verifier", err)
			return err
		}
		verifiersAny[i] = verifierAny
	}
	signaturesAny := make([]*anypb.Any, len(signatures))
	for i, signature := range signatures {
		log.L(ctx).Debugf("marshalling endorsement signature %s", signature.String())
		signatureAny, err := anypb.New(signature)
		if err != nil {
			log.L(ctx).Error("error marshalling signature", err)
			return err
		}
		signaturesAny[i] = signatureAny
	}

	inputStatesAny := make([]*anypb.Any, len(inputStates))
	for i, inputState := range inputStates {
		log.L(ctx).Debugf("marshalling endorsement inputState %s", inputState.String())
		inputStateAny, err := anypb.New(inputState)
		if err != nil {
			log.L(ctx).Error("error marshalling input state", err)
			return err
		}
		inputStatesAny[i] = inputStateAny
	}

	readStatesAny := make([]*anypb.Any, len(readStates))
	for i, readState := range readStates {
		readStateAny, err := anypb.New(readState)
		if err != nil {
			log.L(ctx).Error("Error marshalling read state", err)
			return err
		}
		readStatesAny[i] = readStateAny
	}

	outputStatesAny := make([]*anypb.Any, len(outputStates))
	for i, outputState := range outputStates {
		outputStateAny, err := anypb.New(outputState)
		if err != nil {
			log.L(ctx).Error("error marshalling output state", err)
			return err
		}
		outputStatesAny[i] = outputStateAny
	}

	infoStatesAny := make([]*anypb.Any, len(infoStates))
	log.L(ctx).Debugf("sending endorse request with %+v info states", len(infoStates))
	for i, infoState := range infoStates {
		infoStateAny, err := anypb.New(infoState)
		if err != nil {
			log.L(ctx).Error("error marshalling output state", err)
			return err
		}
		infoStatesAny[i] = infoStateAny
	}

	log.L(ctx).Debugf("sending endorse request with TX ID %+v", transactionSpecification.TransactionId)
	endorsementRequest := &engineProto.EndorsementRequest{
		IdempotencyKey:           idempotencyKey.String(),
		ContractAddress:          transactionSpecification.ContractInfo.ContractAddress,
		TransactionId:            txID.String(),
		AttestationRequest:       attRequestAny,
		Party:                    party,
		TransactionSpecification: transactionSpecificationAny,
		Verifiers:                verifiersAny,
		Signatures:               signaturesAny,
		InputStates:              inputStatesAny,
		OutputStates:             outputStatesAny,
		InfoStates:               infoStatesAny,
	}

	endorsementRequestBytes, err := proto.Marshal(endorsementRequest)
	if err != nil {
		log.L(ctx).Error("error marshalling endorsement request", err)
		return err
	}

	partyFull := strings.Split(party, "@")
	partyNode := partyFull[0]
	if len(partyFull) > 1 {
		partyNode = partyFull[1]
	}

	err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: "EndorsementRequest",
		Node:        partyNode,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     endorsementRequestBytes,
	})
	return err
}

func (tw *transportWriter) SendEndorsementResponse(ctx context.Context, transactionId, idempotencyKey, contractAddress string, attResult *prototk.AttestationResult, endorsementResult *components.EndorsementResult, revertReason, endorsementName, party, node string) error {

	endorsementResponse := &engineProto.EndorsementResponse{}

	if revertReason != "" {
		endorsementResponse.RevertReason = &revertReason
	}

	attResultAny, err := anypb.New(attResult)
	if err != nil {
		log.L(ctx).Error("error marshalling transaction specification", err)
		return err
	}
	endorsementResponse.Endorsement = attResultAny
	endorsementResponse.TransactionId = transactionId
	endorsementResponse.IdempotencyKey = idempotencyKey
	endorsementResponse.AttestationRequestName = endorsementName
	endorsementResponse.Party = party
	endorsementResponse.ContractAddress = contractAddress

	endorsementResponseBytes, err := proto.Marshal(endorsementResponse)
	if err != nil {
		log.L(ctx).Error("error marshalling endorsement response", err)
	}

	payload := &components.FireAndForgetMessageSend{
		MessageType: MessageType_EndorsementResponse,
		Node:        node,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     endorsementResponseBytes,
	}

	err = tw.send(ctx, payload)

	return err
}

func (tw *transportWriter) SendAssembleRequest(ctx context.Context, assemblingNode string, txID uuid.UUID, idempotencyId uuid.UUID, preAssembly *components.TransactionPreAssembly, stateLocksJSON []byte, blockHeight int64) error {

	log.L(log.WithComponent(ctx, common.SUBCOMP_MSGTX)).Tracef("transport writer attempting to send assemble request to assembling node %s", assemblingNode)

	preAssemblyBytes, err := json.Marshal(preAssembly)
	if err != nil {
		log.L(ctx).Error("error marshalling preassembly", err)
		return err
	}

	assembleRequest := &engineProto.AssembleRequest{
		TransactionId:     txID.String(),
		AssembleRequestId: idempotencyId.String(),
		ContractAddress:   tw.contractAddress.HexString(),
		PreAssembly:       preAssemblyBytes,
		StateLocks:        stateLocksJSON,
		BlockHeight:       blockHeight,
	}

	assembleRequestBytes, err := proto.Marshal(assembleRequest)
	if err != nil {
		log.L(ctx).Error("error marshalling assemble request", err)
		return err
	}

	payload := &components.FireAndForgetMessageSend{
		MessageType: "AssembleRequest",
		Node:        assemblingNode,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     assembleRequestBytes,
	}

	err = tw.send(ctx, payload)

	return err
}

func (tw *transportWriter) SendAssembleResponse(ctx context.Context, txID uuid.UUID, assembleRequestId uuid.UUID, postAssembly *components.TransactionPostAssembly, preAssembly *components.TransactionPreAssembly, recipient string) error {

	log.L(log.WithComponent(ctx, common.SUBCOMP_MSGTX)).Tracef("transport writer attempting to send assemble response to node %s", recipient)

	postAssemblyBytes, err := json.Marshal(postAssembly)
	if err != nil {
		return err
	}

	preAssemblyBytes, err := json.Marshal(preAssembly)
	if err != nil {
		return err
	}

	assembleResponse := &engineProto.AssembleResponse{
		TransactionId:     txID.String(),
		AssembleRequestId: assembleRequestId.String(),
		ContractAddress:   tw.contractAddress.HexString(),
		PostAssembly:      postAssemblyBytes,
		PreAssembly:       preAssemblyBytes,
	}
	assembleResponseBytes, err := proto.Marshal(assembleResponse)
	if err != nil {
		return err
	}

	err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: "AssembleResponse",
		Node:        recipient,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     assembleResponseBytes,
	})
	if err != nil {
		// Log the error but continue sending to the other recipients
		log.L(ctx).Errorf("error sending assemble response to %s: %s", recipient, err)
	}

	return err
}

func (tw *transportWriter) SendHandoverRequest(ctx context.Context, activeCoordinator string, contractAddress *pldtypes.EthAddress) error {

	log.L(log.WithComponent(ctx, common.SUBCOMP_MSGTX)).Tracef("transport writer attempting to send handover request to node %s", activeCoordinator)

	if contractAddress == nil {
		err := fmt.Errorf("attempt to send handover request without specifying contract address")
		return err
	}
	handoverRequest := &HandoverRequest{
		ContractAddress: contractAddress,
	}
	handoverRequestBytes, err := json.Marshal(handoverRequest)
	if err != nil {
		log.L(ctx).Errorf("error marshalling handover request message: %s", err)
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_HandoverRequest,
		Payload:     handoverRequestBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        activeCoordinator,
	}); err != nil {
		log.L(ctx).Errorf("error sending handover request message: %s", err)
	}

	return err
}

func (tw *transportWriter) SendNonceAssigned(ctx context.Context, txID uuid.UUID, originatorNode string, contractAddress *pldtypes.EthAddress, nonce uint64) error {

	log.L(log.WithComponent(ctx, common.SUBCOMP_MSGTX)).Tracef("transport writer attempting to send nonce assigned message to node %s", originatorNode)

	if contractAddress == nil {
		err := fmt.Errorf("attempt to send nonce assigned event request without specifying contract address")
		return err
	}
	nonceAssigned := &engineProto.NonceAssigned{
		Id:              uuid.New().String(),
		TransactionId:   txID.String(),
		ContractAddress: contractAddress.HexString(),
		Nonce:           int64(nonce),
	}
	nonceAssignedBytes, err := proto.Marshal(nonceAssigned)
	if err != nil {
		log.L(ctx).Errorf("error marshalling nonce assigned event: %s", err)
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_NonceAssigned,
		Payload:     nonceAssignedBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        originatorNode,
	}); err != nil {
		log.L(ctx).Errorf("error sending nonce assigned event: %s", err)
	}

	return err
}

func (tw *transportWriter) SendTransactionSubmitted(ctx context.Context, txID uuid.UUID, originatorNode string, contractAddress *pldtypes.EthAddress, txHash *pldtypes.Bytes32) error {

	log.L(log.WithComponent(ctx, common.SUBCOMP_MSGTX)).Tracef("transport writer attempting to send transaction submitted message to node %s", originatorNode)

	if contractAddress == nil {
		err := fmt.Errorf("attempt to send TX submitted event without specifying contract address")
		return err
	}
	txSubmitted := &engineProto.TransactionSubmitted{
		Id:              uuid.New().String(),
		TransactionId:   txID.String(),
		ContractAddress: contractAddress.HexString(),
		Hash:            txHash.Bytes(),
	}
	txSubmittedBytes, err := proto.Marshal(txSubmitted)
	if err != nil {
		log.L(ctx).Errorf("error marshalling TX submitted event: %s", err)
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_TransactionSubmitted,
		Payload:     txSubmittedBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        originatorNode,
	}); err != nil {
		log.L(ctx).Errorf("error sending transaction submitted event: %s", err)
	}

	return err
}

func (tw *transportWriter) SendTransactionConfirmed(ctx context.Context, txID uuid.UUID, originatorNode string, contractAddress *pldtypes.EthAddress, nonce *pldtypes.HexUint64, revertReason pldtypes.HexBytes) error {

	log.L(log.WithComponent(ctx, common.SUBCOMP_MSGTX)).Tracef("transport writer attempting to send transaction confirmed message to node %s", originatorNode)

	if contractAddress == nil {
		err := fmt.Errorf("attempt to send TX submitted event without specifying contract address")
		return err
	}

	txConfirmed := &engineProto.TransactionConfirmed{
		Id:              uuid.New().String(),
		TransactionId:   txID.String(),
		ContractAddress: contractAddress.HexString(),
		RevertReason:    revertReason,
	}
	if nonce != nil {
		txConfirmed.Nonce = int64(*nonce)
	}
	txConfirmedBytes, err := proto.Marshal(txConfirmed)
	if err != nil {
		log.L(ctx).Errorf("error marshalling TX confirmed event: %s", err)
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_TransactionConfirmed,
		Payload:     txConfirmedBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        originatorNode,
	}); err != nil {
		log.L(ctx).Errorf("error sending transaction confirmed event: %s", err)
	}

	return err
}

func (tw *transportWriter) SendHeartbeat(ctx context.Context, targetNode string, contractAddress *pldtypes.EthAddress, coordinatorSnapshot *common.CoordinatorSnapshot) error {

	log.L(log.WithComponent(ctx, common.SUBCOMP_MSGTX)).Tracef("transport writer attempting to send haertbeat to node %s", targetNode)

	coordinatorSnapshotBytes, err := json.Marshal(coordinatorSnapshot)
	if err != nil {
		log.L(ctx).Error("error marshalling heartbeat", err)
	}

	heartbeatRequest := &engineProto.CoordinatorHeartbeatNotification{
		From:                tw.transportManager.LocalNodeName(),
		ContractAddress:     contractAddress.HexString(),
		CoordinatorSnapshot: coordinatorSnapshotBytes,
	}
	log.L(ctx).Debugf("sending heartbeat: From 	%s, Contract Address %s", tw.transportManager.LocalNodeName(), contractAddress.HexString())
	heartbeatRequestBytes, err := proto.Marshal(heartbeatRequest)
	if err != nil {
		log.L(ctx).Errorf("error marshalling heartbeat request  message: %s", err)
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_CoordinatorHeartbeatNotification,
		Payload:     heartbeatRequestBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        targetNode,
	}); err != nil {
		log.L(ctx).Errorf("error sending heartbeat request  message: %s", err)
	}

	return err
}

func (tw *transportWriter) SendPreDispatchRequest(ctx context.Context, originatorNode string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification, hash *pldtypes.Bytes32) error {

	log.L(log.WithComponent(ctx, common.SUBCOMP_MSGTX)).Tracef("transport writer attempting to send pre-dispatch request to node %s", originatorNode)

	// MRW TODO There should be a different proto message type instead of TransactionDispatched
	dispatchConfirmationRequest := &engineProto.TransactionDispatched{
		Id:               idempotencyKey.String(),
		TransactionId:    transactionSpecification.TransactionId,
		ContractAddress:  tw.contractAddress.HexString(),
		PostAssembleHash: hash.Bytes(),
	}

	dispatchConfirmationRequestBytes, err := proto.Marshal(dispatchConfirmationRequest)
	if err != nil {
		log.L(ctx).Errorf("error marshalling dispatch confirmation request  message: %s", err)
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_PreDispatchRequest,
		Payload:     dispatchConfirmationRequestBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        originatorNode,
	}); err != nil {
		log.L(ctx).Errorf("error sending pre-dispatch request  message: %s", err)
	}
	return err
}

func (tw *transportWriter) SendPreDispatchResponse(ctx context.Context, transactionOriginator string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification) error {

	log.L(log.WithComponent(ctx, common.SUBCOMP_MSGTX)).Tracef("transport writer attempting to send pre-dispatch response to node %s", transactionOriginator)

	dispatchResponseEvent := &engineProto.TransactionDispatched{
		Id:              idempotencyKey.String(),
		TransactionId:   transactionSpecification.TransactionId,
		ContractAddress: tw.contractAddress.HexString(),
		Signer:          transactionOriginator,
	}

	dispatchResponseEventBytes, err := proto.Marshal(dispatchResponseEvent)
	if err != nil {
		log.L(ctx).Errorf("error marshalling dispatch confirmation request  message: %s", err)
	}

	// Split transactionOriginator into node and domain
	parts := strings.Split(transactionOriginator, "@")
	node := parts[0]
	if len(parts) > 1 {
		node = parts[1]
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_PreDispatchResponse,
		Payload:     dispatchResponseEventBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        node,
	}); err != nil {
		log.L(ctx).Errorf("error sending dispatched event: %s", err)
	}
	return err
}

func (tw *transportWriter) SendDispatched(ctx context.Context, transactionOriginator string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification) error {

	log.L(log.WithComponent(ctx, common.SUBCOMP_MSGTX)).Tracef("transport writer attempting to send dispatched message to node %s", transactionOriginator)

	dispatchedEvent := &engineProto.TransactionDispatched{
		Id:              idempotencyKey.String(),
		TransactionId:   transactionSpecification.TransactionId,
		ContractAddress: tw.contractAddress.HexString(),
		Signer:          transactionOriginator,
	}

	dispatchedEventBytes, err := proto.Marshal(dispatchedEvent)
	if err != nil {
		log.L(ctx).Errorf("error marshalling dispatch confirmation request  message: %s", err)
	}

	// Split transactionOriginator into node and domain
	parts := strings.Split(transactionOriginator, "@")
	node := parts[0]
	if len(parts) > 1 {
		node = parts[1]
	}

	if err = tw.send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_Dispatched,
		Payload:     dispatchedEventBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        node,
	}); err != nil {
		log.L(ctx).Errorf("error sending dispatched event: %s", err)
	}
	return err
}

func (tw *transportWriter) send(ctx context.Context, payload *components.FireAndForgetMessageSend) error {
	if payload.Node == "" {
		err := fmt.Errorf("attempt to send message without specifying destination node name")
		return err
	}

	log.L(log.WithComponent(ctx, common.SUBCOMP_MSGTX)).Debugf("%+v sent to %s", payload.MessageType, payload.Node)
	if payload.Node == "" || payload.Node == tw.transportManager.LocalNodeName() {
		// "Localhost" loopback
		log.L(ctx).Debugf("sending %s to loopback interface", payload.MessageType)

		// Run the loopback transport in a goroutine to avoid blocking the main thread. This is important for the
		// channel-based event queue to ensure the queue consumer is not blocked when we happen to be sending
		// to ourselves
		go func() {
			err := tw.loopbackTransport.Send(ctx, payload)
			if err != nil {
				log.L(ctx).Errorf("error sending %s to loopback interface: %s", payload.MessageType, err)
			}
		}()
		return nil
	}
	log.L(ctx).Debugf("sending %s to node: %s", payload.MessageType, payload.Node)
	err := tw.transportManager.Send(ctx, payload)
	return err
}
