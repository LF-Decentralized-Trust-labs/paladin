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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	engineProto "github.com/kaleido-io/paladin/core/pkg/proto/engine"
	pb "github.com/kaleido-io/paladin/core/pkg/proto/engine"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func NewTransportWriter(domainName string, contractAddress *pldtypes.EthAddress, nodeID string, transportManager components.TransportManager, loopbackHandler func(ctx context.Context, message *components.ReceivedMessage)) *transportWriter {
	loopbackTransport := NewLoopbackTransportWriter(loopbackHandler)
	return &transportWriter{
		nodeID:            nodeID,
		transportManager:  transportManager,
		loopbackTransport: loopbackTransport,
		domainName:        domainName,
		contractAddress:   contractAddress,
	}
}

type transportWriter struct {
	nodeID            string
	transportManager  components.TransportManager
	loopbackTransport LoopbackTransportManager
	domainName        string
	contractAddress   *pldtypes.EthAddress
}

func (tw *transportWriter) SendDelegationRequest(
	ctx context.Context,
	coordinatorLocator string,
	transactions []*components.PrivateTransaction,
	blockHeight uint64,
) {
	log.L(ctx).Infof("[Sequencer] SendDelegationRequest coordinator locator: %s", coordinatorLocator)
	log.L(ctx).Infof("[Sequencer] SendDelegationRequest - we currently have %d transactions to handle", len(transactions))
	for _, transaction := range transactions {

		transactionBytes, err := json.Marshal(transaction)

		if err != nil {
			log.L(ctx).Errorf("[Sequencer] error marshalling transaction message: %s", err)
		}
		delegationRequest := &pb.DelegationRequest{
			// DelegationId:       delegationId,
			TransactionId: transaction.ID.String(),
			// DelegateNodeId:     delegateNodeId,
			PrivateTransaction: transactionBytes,
			BlockHeight:        int64(blockHeight),
		}
		delegationRequestBytes, err := proto.Marshal(delegationRequest)
		if err != nil {
			log.L(ctx).Errorf("[Sequencer] error marshalling delegationRequest  message: %s", err)
		}

		parts := strings.Split(coordinatorLocator, "@")
		node := parts[0]
		if len(parts) > 1 {
			node = parts[1]
		}

		if err = tw.Send(ctx, &components.FireAndForgetMessageSend{
			MessageType: "DelegationRequest",
			Payload:     delegationRequestBytes,
			Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
			Node:        node,
		}); err != nil {
			log.L(ctx).Errorf("[Sequencer] error sending delegationRequest message: %s", err)
		}
	}
}

func (tw *transportWriter) SendDelegationRequestAcknowledgment(
	ctx context.Context,
	delegatingNodeName string,
	delegationId string,
	delegateNodeName string,
	transactionID string,

) error {

	delegationRequestAcknowledgment := &pb.DelegationRequestAcknowledgment{
		DelegationId:    delegationId,
		TransactionId:   transactionID,
		DelegateNodeId:  delegateNodeName,
		ContractAddress: tw.contractAddress.String(),
	}
	delegationRequestAcknowledgmentBytes, err := proto.Marshal(delegationRequestAcknowledgment)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] error marshalling delegationRequestAcknowledgment  message: %s", err)
		return err
	}

	if err = tw.Send(ctx, &components.FireAndForgetMessageSend{
		MessageType: "DelegationRequestAcknowledgment",
		Payload:     delegationRequestAcknowledgmentBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        delegatingNodeName,
	}); err != nil {
		return err
	}
	return nil
}

func toEndorsableList(states []*components.FullState) []*prototk.EndorsableState {
	endorsableList := make([]*prototk.EndorsableState, len(states))
	for i, input := range states {
		endorsableList[i] = &prototk.EndorsableState{
			Id:            input.ID.String(),
			SchemaId:      input.Schema.String(),
			StateDataJson: string(input.Data),
		}
	}
	return endorsableList
}

// TODO do we have duplication here?  contractAddress and transactionID are in the transactionSpecification
func (tw *transportWriter) SendEndorsementRequest(ctx context.Context, idempotencyKey uuid.UUID, party string, attRequest *prototk.AttestationRequest, transactionSpecification *prototk.TransactionSpecification, verifiers []*prototk.ResolvedVerifier, signatures []*prototk.AttestationResult, inputStates []*prototk.EndorsableState, outputStates []*prototk.EndorsableState, infoStates []*prototk.EndorsableState) error {
	attRequestAny, err := anypb.New(attRequest)
	if err != nil {
		log.L(ctx).Error("[Sequencer] error marshalling attestation request", err)
		return err
	}

	transactionSpecificationAny, err := anypb.New(transactionSpecification)
	if err != nil {
		log.L(ctx).Error("[Sequencer] error marshalling transaction specification", err)
		return err
	}
	verifiersAny := make([]*anypb.Any, len(verifiers))
	for i, verifier := range verifiers {
		log.L(ctx).Debugf("[Sequencer] marshalling endorsement requestverifier %s", verifier.String())
		verifierAny, err := anypb.New(verifier)
		if err != nil {
			log.L(ctx).Error("[Sequencer] error marshalling verifier", err)
			return err
		}
		verifiersAny[i] = verifierAny
	}
	signaturesAny := make([]*anypb.Any, len(signatures))
	for i, signature := range signatures {
		log.L(ctx).Debugf("[Sequencer] marshalling endorsement signature %s", signature.String())
		signatureAny, err := anypb.New(signature)
		if err != nil {
			log.L(ctx).Error("[Sequencer] error marshalling signature", err)
			return err
		}
		signaturesAny[i] = signatureAny
	}

	inputStatesAny := make([]*anypb.Any, len(inputStates))
	for i, inputState := range inputStates {
		log.L(ctx).Debugf("[Sequencer] marshalling endorsement inputState %s", inputState.String())
		inputStateAny, err := anypb.New(inputState)
		if err != nil {
			log.L(ctx).Error("[Sequencer] error marshalling input state", err)
			//TODO return nil, err
		}
		inputStatesAny[i] = inputStateAny
	}

	outputStatesAny := make([]*anypb.Any, len(outputStates))
	for i, outputState := range outputStates {
		outputStateAny, err := anypb.New(outputState)
		if err != nil {
			log.L(ctx).Error("[Sequencer] error marshalling output state", err)
			return err
		}
		outputStatesAny[i] = outputStateAny
	}

	infoStatesAny := make([]*anypb.Any, len(infoStates))
	log.L(ctx).Debugf("[Sequencer] sending endorse request with %+v info states", len(infoStates))
	for i, infoState := range infoStates {
		infoStateAny, err := anypb.New(infoState)
		if err != nil {
			log.L(ctx).Error("[Sequencer] error marshalling output state", err)
			return err
		}
		infoStatesAny[i] = infoStateAny
	}

	log.L(ctx).Debugf("[Sequencer] sending endorse request with TX ID %+v", transactionSpecification.TransactionId)
	endorsementRequest := &engineProto.EndorsementRequest{
		IdempotencyKey:           idempotencyKey.String(),
		ContractAddress:          tw.contractAddress.HexString(),
		TransactionId:            transactionSpecification.TransactionId,
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
		log.L(ctx).Error("[Sequencer] error marshalling endorsement request", err)
		return err
	}

	partyFull := strings.Split(party, "@")
	partyNode := partyFull[0]
	if len(partyFull) > 1 {
		partyNode = partyFull[1]
	}

	err = tw.Send(ctx, &components.FireAndForgetMessageSend{
		MessageType: "EndorsementRequest",
		Node:        partyNode,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     endorsementRequestBytes,
	})
	return err
}

// MRW TODO - why are there no state locks passed in to here?
func (tw *transportWriter) SendAssembleRequest(ctx context.Context, assemblingNode string, txID uuid.UUID, idempotencyId uuid.UUID, preAssembly *components.TransactionPreAssembly, stateLocksJSON []byte, blockHeight int64) error {

	log.L(ctx).Infof("[Sequencer] transport writer attempting to send assemble request to assembling node %s", assemblingNode)

	preAssemblyBytes, err := json.Marshal(preAssembly)
	if err != nil {
		log.L(ctx).Error("[Sequencer] error marshalling preassembly", err)
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
	log.L(ctx).Infof("[Sequencer] assemble request: TX ID %s", assembleRequest.TransactionId)
	log.L(ctx).Infof("[Sequencer] assemble request: Idempotency ID %s", assembleRequest.AssembleRequestId)
	log.L(ctx).Infof("[Sequencer] assemble request: Contract Address %s", assembleRequest.ContractAddress)
	log.L(ctx).Infof("[Sequencer] assemble request: preassembly byte hex %s", hex.EncodeToString(assembleRequest.PreAssembly))

	assembleRequestBytes, err := proto.Marshal(assembleRequest)
	if err != nil {
		log.L(ctx).Error("[Sequencer] error marshalling assemble request", err)
		return err
	}

	payload := &components.FireAndForgetMessageSend{
		MessageType: "AssembleRequest",
		Node:        assemblingNode,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     assembleRequestBytes,
	}

	err = tw.Send(ctx, payload)

	return err
}

func (tw *transportWriter) SendAssembleResponse(ctx context.Context, txID uuid.UUID, assembleRequestId uuid.UUID, postAssembly *components.TransactionPostAssembly, recipient string) {

	postAssemblyBytes, err := json.Marshal(postAssembly)
	if err != nil {
		log.L(ctx).Error("[Sequencer] error marshalling postAssembly", err)
	}

	assembleResponse := &engineProto.AssembleResponse{
		TransactionId:     txID.String(),
		AssembleRequestId: assembleRequestId.String(),
		ContractAddress:   tw.contractAddress.HexString(),
		PostAssembly:      postAssemblyBytes,
	}
	assembleResponseBytes, err := proto.Marshal(assembleResponse)
	if err != nil {
		log.L(ctx).Error("[Sequencer] error marshalling assemble response", err)
	}

	err = tw.Send(ctx, &components.FireAndForgetMessageSend{
		MessageType: "AssembleResponse",
		Node:        recipient,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     assembleResponseBytes,
	})
	if err != nil {
		// Log the error but continue sending to the other recipients
		log.L(ctx).Errorf("[Sequencer] error sending assemble response to %s: %s", recipient, err)
	}
}

func (tw *transportWriter) SendHandoverRequest(ctx context.Context, activeCoordinator string, contractAddress *pldtypes.EthAddress) {
	if contractAddress == nil {
		err := fmt.Errorf("Attempt to send handover request without specifying contract address")
		log.L(ctx).Error(err.Error())
	}
	handoverRequest := &HandoverRequest{
		ContractAddress: contractAddress,
	}
	handoverRequestBytes, err := json.Marshal(handoverRequest)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] error marshalling handover request message: %s", err)
	}

	if err = tw.Send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_HandoverRequest,
		Payload:     handoverRequestBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        activeCoordinator,
	}); err != nil {
		log.L(ctx).Errorf("[Sequencer] error sending handover request message: %s", err)
	}
}

func (tw *transportWriter) SendHeartbeat(ctx context.Context, targetNode string, contractAddress *pldtypes.EthAddress, coordinatorSnapshot *common.CoordinatorSnapshot) {

	coordinatorSnapshotBytes, err := json.Marshal(coordinatorSnapshot)
	if err != nil {
		log.L(ctx).Error("[Sequencer] error marshalling heartbeat", err)
	}
	log.L(ctx).Debugf("[Sequencer] sending heartbeat to %s", targetNode)

	heartbeatRequest := &engineProto.CoordinatorHeartbeatNotification{
		From:                tw.transportManager.LocalNodeName(),
		ContractAddress:     contractAddress.HexString(),
		CoordinatorSnapshot: coordinatorSnapshotBytes,
	}
	log.L(ctx).Debugf("[Sequencer] sending heartbeat: From 	%s, Contract Address %s", tw.transportManager.LocalNodeName(), contractAddress.HexString())
	heartbeatRequestBytes, err := proto.Marshal(heartbeatRequest)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] error marshalling heartbeat request  message: %s", err)
	}

	if err = tw.Send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_CoordinatorHeartbeatNotification,
		Payload:     heartbeatRequestBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        targetNode,
	}); err != nil {
		log.L(ctx).Errorf("[Sequencer] error sending heartbeat request  message: %s", err)
	}
}

func (tw *transportWriter) SendDispatchConfirmationRequest(ctx context.Context, transactionSender string, idempotencyKey uuid.UUID, transactionSpecification *prototk.TransactionSpecification, hash *pldtypes.Bytes32) error {
	log.L(ctx).Infof("SendDispatchConfirmationRequest")

	// MRW TODO - should dispatch confirmations also take the hash?
	dispatchConfirmationRequest := &engineProto.TransactionDispatched{
		Id:              idempotencyKey.String(),
		TransactionId:   transactionSpecification.TransactionId,
		ContractAddress: tw.contractAddress.HexString(),
	}

	dispatchConfirmationRequestBytes, err := proto.Marshal(dispatchConfirmationRequest)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] error marshalling dispatch confirmation request  message: %s", err)
	}

	// Split transactionSender into node and domain
	parts := strings.Split(transactionSender, "@")
	node := parts[0]
	if len(parts) > 1 {
		node = parts[1]
	}

	if err = tw.Send(ctx, &components.FireAndForgetMessageSend{
		MessageType: MessageType_DispatchConfirmationRequest,
		Payload:     dispatchConfirmationRequestBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        node,
	}); err != nil {
		log.L(ctx).Errorf("[Sequencer] error sending dispatch confirmation request  message: %s", err)
	}
	return err
}

func (tw *transportWriter) SendDispatchConfirmationResponse(ctx context.Context) {
	log.L(ctx).Errorf("[Sequencer] send dispatch confirmation response not implemented")
	// MRW TODO - ?
}

func (tw *transportWriter) Send(ctx context.Context, payload *components.FireAndForgetMessageSend) error {
	if payload.Node == "" || payload.Node == tw.transportManager.LocalNodeName() {
		// "Localhost" loopback
		log.L(ctx).Debugf("[Sequencer] sending %s to loopback interface", payload.MessageType)
		err := tw.loopbackTransport.Send(ctx, payload)
		return err
	}
	log.L(ctx).Debugf("[Sequencer] sending %s to node: %s", payload.MessageType, payload.Node)
	err := tw.transportManager.Send(ctx, payload)
	return err
}
