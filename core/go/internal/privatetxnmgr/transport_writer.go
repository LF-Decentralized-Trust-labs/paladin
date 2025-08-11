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
	"encoding/json"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/proto/engine"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func NewTransportWriter(domainName string, contractAddress *pldtypes.EthAddress, nodeID string, transportManager components.TransportManager) *transportWriter {
	return &transportWriter{
		nodeID:           nodeID,
		transportManager: transportManager,
		domainName:       domainName,
		contractAddress:  contractAddress,
	}
}

type transportWriter struct {
	nodeID           string
	transportManager components.TransportManager
	domainName       string
	contractAddress  *pldtypes.EthAddress
}

func (tw *transportWriter) SendDelegationRequest(
	ctx context.Context,
	delegationId string,
	delegateNodeId string,
	transaction *components.PrivateTransaction,
	blockHeight int64,
) error {

	transactionBytes, err := json.Marshal(transaction)

	if err != nil {
		log.L(ctx).Errorf("Error marshalling transaction message: %s", err)
		return err
	}
	delegationRequest := &pb.DelegationRequest{
		DelegationId:       delegationId,
		TransactionId:      transaction.ID.String(),
		DelegateNodeId:     delegateNodeId,
		PrivateTransaction: transactionBytes,
		BlockHeight:        blockHeight,
	}
	delegationRequestBytes, err := proto.Marshal(delegationRequest)
	if err != nil {
		log.L(ctx).Errorf("Error marshalling delegationRequest  message: %s", err)
		return err
	}

	if err = tw.transportManager.Send(ctx, &components.FireAndForgetMessageSend{
		MessageType: "DelegationRequest",
		Payload:     delegationRequestBytes,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Node:        delegateNodeId,
	}); err != nil {
		return err
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

	delegationRequestAcknowledgment := &pb.DelegationRequestAcknowledgment{
		DelegationId:    delegationId,
		TransactionId:   transactionID,
		DelegateNodeId:  delegateNodeName,
		ContractAddress: tw.contractAddress.String(),
	}
	delegationRequestAcknowledgmentBytes, err := proto.Marshal(delegationRequestAcknowledgment)
	if err != nil {
		log.L(ctx).Errorf("Error marshalling delegationRequestAcknowledgment  message: %s", err)
		return err
	}

	if err = tw.transportManager.Send(ctx, &components.FireAndForgetMessageSend{
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
func (tw *transportWriter) SendEndorsementRequest(ctx context.Context, idempotencyKey string, party string, targetNode string, contractAddress string, transactionID string, attRequest *prototk.AttestationRequest, transactionSpecification *prototk.TransactionSpecification, verifiers []*prototk.ResolvedVerifier, signatures []*prototk.AttestationResult, inputStates []*components.FullState, outputStates []*components.FullState, infoStates []*components.FullState) error {
	attRequestAny, err := anypb.New(attRequest)
	if err != nil {
		log.L(ctx).Error("Error marshalling attestation request", err)
		return err
	}

	transactionSpecificationAny, err := anypb.New(transactionSpecification)
	if err != nil {
		log.L(ctx).Error("Error marshalling transaction specification", err)
		return err
	}
	verifiersAny := make([]*anypb.Any, len(verifiers))
	for i, verifier := range verifiers {
		verifierAny, err := anypb.New(verifier)
		if err != nil {
			log.L(ctx).Error("Error marshalling verifier", err)
			return err
		}
		verifiersAny[i] = verifierAny
	}
	signaturesAny := make([]*anypb.Any, len(signatures))
	for i, signature := range signatures {
		signatureAny, err := anypb.New(signature)
		if err != nil {
			log.L(ctx).Error("Error marshalling signature", err)
			return err
		}
		signaturesAny[i] = signatureAny
	}

	inputStatesAny := make([]*anypb.Any, len(inputStates))
	endorseableInputStates := toEndorsableList(inputStates)
	for i, inputState := range endorseableInputStates {
		inputStateAny, err := anypb.New(inputState)
		if err != nil {
			log.L(ctx).Error("Error marshalling input state", err)
			//TODO return nil, err
		}
		inputStatesAny[i] = inputStateAny
	}

	outputStatesAny := make([]*anypb.Any, len(outputStates))
	endorseableOutputStates := toEndorsableList(outputStates)
	for i, outputState := range endorseableOutputStates {
		outputStateAny, err := anypb.New(outputState)
		if err != nil {
			log.L(ctx).Error("Error marshalling output state", err)
			return err
		}
		outputStatesAny[i] = outputStateAny
	}

	infoStatesAny := make([]*anypb.Any, len(infoStates))
	endorseableInfoStates := toEndorsableList(infoStates)
	for i, infoState := range endorseableInfoStates {
		infoStateAny, err := anypb.New(infoState)
		if err != nil {
			log.L(ctx).Error("Error marshalling output state", err)
			return err
		}
		infoStatesAny[i] = infoStateAny
	}

	endorsementRequest := &pb.EndorsementRequest{
		IdempotencyKey:           idempotencyKey,
		ContractAddress:          contractAddress,
		TransactionId:            transactionID,
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
		log.L(ctx).Error("Error marshalling endorsement request", err)
		return err
	}
	err = tw.transportManager.Send(ctx, &components.FireAndForgetMessageSend{
		MessageType: "EndorsementRequest",
		Node:        targetNode,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     endorsementRequestBytes,
	})
	return err
}

func (tw *transportWriter) SendAssembleRequest(ctx context.Context, assemblingNode string, assembleRequestID string, txID uuid.UUID, contractAddress string, preAssembly *components.TransactionPreAssembly, stateLocksJSON []byte, blockHeight int64) error {

	preAssemblyBytes, err := json.Marshal(preAssembly)
	if err != nil {
		log.L(ctx).Error("Error marshalling preassembly", err)
		return err
	}

	assembleRequest := &pb.AssembleRequest{
		TransactionId:     txID.String(),
		AssembleRequestId: assembleRequestID,
		ContractAddress:   contractAddress,
		PreAssembly:       preAssemblyBytes,
		StateLocks:        stateLocksJSON,
		BlockHeight:       blockHeight,
	}
	assembleRequestBytes, err := proto.Marshal(assembleRequest)
	if err != nil {
		log.L(ctx).Error("Error marshalling assemble request", err)
		return err
	}
	err = tw.transportManager.Send(ctx, &components.FireAndForgetMessageSend{
		MessageType: "AssembleRequest",
		Node:        assemblingNode,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		Payload:     assembleRequestBytes,
	})
	return err
}
