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

package sequencer

import (
	"context"
	"fmt"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator"
	coordTransaction "github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator"
	originatorTransaction "github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

func (sMgr *sequencerManager) HandlePaladinMsg(ctx context.Context, message *components.ReceivedMessage) {
	//TODO this need to become an ultra low latency, non blocking, handover to the event loop thread.
	// need some thought on how to handle errors, retries, buffering, swapping idle sequencers in and out of memory etc...

	log.L(ctx).Debugf("%+v received from %s", message.MessageType, message.FromNode)

	//Send the event to the sequencer handler
	switch message.MessageType {
	case transport.MessageType_AssembleRequest:
		go sMgr.handleAssembleRequest(sMgr.ctx, message)
	case transport.MessageType_AssembleResponse:
		go sMgr.handleAssembleResponse(sMgr.ctx, message)
	case transport.MessageType_AssembleError:
		go sMgr.handleAssembleError(sMgr.ctx, message)
	case transport.MessageType_AssembleRejection:
		go sMgr.handleAssembleRejection(sMgr.ctx, message)
	case transport.MessageType_SignResponse:
		go sMgr.handleSignResponse(sMgr.ctx, message)
	case transport.MessageType_SignError:
		go sMgr.handleSignError(sMgr.ctx, message)
	case transport.MessageType_QueryAvailableStatesRequest:
		go sMgr.handleQueryAvailableStatesRequest(sMgr.ctx, message)
	case transport.MessageType_QueryAvailableStatesResponse:
		go sMgr.handleQueryAvailableStatesResponse(sMgr.ctx, message)
	case transport.MessageType_GetSpentStateIDsRequest:
		go sMgr.handleGetSpentStateIDsRequest(sMgr.ctx, message)
	case transport.MessageType_GetSpentStateIDsResponse:
		go sMgr.handleGetSpentStateIDsResponse(sMgr.ctx, message)
	case transport.MessageType_StateViewError:
		go sMgr.handleStateViewError(sMgr.ctx, message)
	case transport.MessageType_CoordinatorHeartbeatNotification:
		go sMgr.handleCoordinatorHeartbeatNotification(sMgr.ctx, message)
	case transport.MessageType_DelegationRequest:
		go sMgr.handleDelegationRequest(sMgr.ctx, message)
	case transport.MessageType_DelegationResponse:
		go sMgr.handleDelegationResponse(sMgr.ctx, message)
	case transport.MessageType_DelegationRejection:
		go sMgr.handleDelegationRejection(sMgr.ctx, message)
	case transport.MessageType_Dispatched:
		go sMgr.handleDispatchedEvent(sMgr.ctx, message)
	case transport.MessageType_PreDispatchRequest:
		go sMgr.handlePreDispatchRequest(sMgr.ctx, message)
	case transport.MessageType_PreDispatchResponse:
		go sMgr.handlePreDispatchResponse(sMgr.ctx, message)
	case transport.MessageType_EndorsementRequest:
		go sMgr.handleEndorsementRequest(sMgr.ctx, message)
	case transport.MessageType_EndorsementResponse:
		go sMgr.handleEndorsementResponse(sMgr.ctx, message)
	case transport.MessageType_EndorsementError:
		go sMgr.handleEndorsementError(sMgr.ctx, message)
	case transport.MessageType_EndorsementRejection:
		go sMgr.handleEndorsementRejection(sMgr.ctx, message)
	case transport.MessageType_NonceAssigned:
		go sMgr.handleNonceAssigned(sMgr.ctx, message)
	case transport.MessageType_TransactionSubmitted:
		go sMgr.handleTransactionSubmitted(sMgr.ctx, message)
	case transport.MessageType_TransactionConfirmed:
		go sMgr.handleTransactionConfirmed(sMgr.ctx, message)
	case transport.MessageType_PreDispatchRejection:
		go sMgr.handlePreDispatchRejection(sMgr.ctx, message)
	case transport.MessageType_HandoverRequest:
		go sMgr.handleHandoverRequest(sMgr.ctx, message)
	default:
		log.L(ctx).Errorf("Unknown message type: %s", message.MessageType)
	}
}

func (sMgr *sequencerManager) logPaladinMessageUnmarshalError(ctx context.Context, message *components.ReceivedMessage, err error) {
	log.L(ctx).Errorf("<< ERROR unmarshalling proto message %s from %s: %s", message.MessageType, message.FromNode, err)
}

func (sMgr *sequencerManager) logPaladinMessageFieldMissingError(ctx context.Context, message *components.ReceivedMessage, field string) {
	log.L(ctx).Errorf("<< field %s missing from proto message %s received from %s", field, message.MessageType, message.FromNode)
}

func (sMgr *sequencerManager) logPaladinMessageFieldInvalidError(ctx context.Context, message *components.ReceivedMessage, field string, value string, err error) {
	log.L(ctx).Errorf("<< field %s of proto message %s received from %s could not be parsed (value %q): %s", field, message.MessageType, message.FromNode, value, err)
}

func (sMgr *sequencerManager) parseUUIDField(ctx context.Context, message *components.ReceivedMessage, field string, value string) (uuid.UUID, bool) {
	if value == "" {
		sMgr.logPaladinMessageFieldMissingError(ctx, message, field)
		return uuid.UUID{}, false
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		sMgr.logPaladinMessageFieldInvalidError(ctx, message, field, value, err)
		return uuid.UUID{}, false
	}
	return parsed, true
}

// A transaction specification carries the transaction's UUID in the domain's bytes32 form: the UUID in
// the first 16 bytes, zero padded. It must be in exactly the form the originator produces so that no
// other encoding of the same UUID reaches the domain.
func (sMgr *sequencerManager) parseTransactionSpecificationID(ctx context.Context, message *components.ReceivedMessage, field string, spec *prototk.TransactionSpecification) (uuid.UUID, bool) {
	if spec.TransactionId == "" {
		sMgr.logPaladinMessageFieldMissingError(ctx, message, field)
		return uuid.UUID{}, false
	}
	b32, err := pldtypes.ParseBytes32Ctx(ctx, spec.TransactionId)
	if err != nil {
		sMgr.logPaladinMessageFieldInvalidError(ctx, message, field, spec.TransactionId, err)
		return uuid.UUID{}, false
	}
	transactionID := b32.UUIDFirst16()
	if expected := pldtypes.Bytes32UUIDFirst16(transactionID).String(); spec.TransactionId != expected {
		sMgr.logPaladinMessageFieldInvalidError(ctx, message, field, spec.TransactionId,
			fmt.Errorf("not a transaction UUID in bytes32 form (expected %s)", expected))
		return uuid.UUID{}, false
	}
	return transactionID, true
}

func (sMgr *sequencerManager) parseBytes32Field(ctx context.Context, message *components.ReceivedMessage, field string, value []byte) (pldtypes.Bytes32, bool) {
	if len(value) == 0 {
		sMgr.logPaladinMessageFieldMissingError(ctx, message, field)
		return pldtypes.Bytes32{}, false
	}
	if len(value) != 32 {
		sMgr.logPaladinMessageFieldInvalidError(ctx, message, field, pldtypes.HexBytes(value).String(), fmt.Errorf("expected 32 bytes, received %d", len(value)))
		return pldtypes.Bytes32{}, false
	}
	return pldtypes.Bytes32(value), true
}

func (sMgr *sequencerManager) parseContractAddressString(ctx context.Context, contractAddressString string, message *components.ReceivedMessage) *pldtypes.EthAddress {
	contractAddress, err := pldtypes.ParseEthAddress(contractAddressString)
	if err != nil {
		log.L(ctx).Errorf("<< ERROR unmarshalling contract address from proto message %s (received from %s): %s", message.MessageType, message.FromNode, err)
		return nil
	}
	return contractAddress
}

func (sMgr *sequencerManager) handleAssembleRequest(ctx context.Context, message *components.ReceivedMessage) {

	assembleRequest := &engineProto.AssembleRequest{}
	err := proto.Unmarshal(message.Payload, assembleRequest)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	log.L(ctx).Infof("handling assemble request for transaction %s", assembleRequest.TransactionId)

	contractAddress := sMgr.parseContractAddressString(ctx, assembleRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	transactionID, ok := sMgr.parseUUIDField(ctx, message, "transaction_id", assembleRequest.TransactionId)
	if !ok {
		return
	}

	requestID, ok := sMgr.parseUUIDField(ctx, message, "assemble_request_id", assembleRequest.AssembleRequestId)
	if !ok {
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the assemble request
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: assemble request for transaction %s cannot be processed unless already in memory",
			contractAddress, assembleRequest.TransactionId)
		return
	}

	assembleRequestEvent := &originatorTransaction.AssembleRequestReceivedEvent{}
	assembleRequestEvent.TransactionID = transactionID
	assembleRequestEvent.RequestID = requestID
	assembleRequestEvent.Coordinator = message.FromNode
	assembleRequestEvent.CoordinatorBlockHeight = assembleRequest.CoordinatorBlockHeight
	assembleRequestEvent.BlockHeightTolerance = assembleRequest.BlockHeightTolerance
	assembleRequestEvent.EventTime = time.Now()
	if assembleRequest.ExpiryTimeUnixMs != 0 {
		assembleRequestEvent.Expiry = time.UnixMilli(assembleRequest.ExpiryTimeUnixMs)
	}

	seq.GetOriginator().QueueEvent(ctx, assembleRequestEvent)
}

func (sMgr *sequencerManager) handleAssembleResponse(ctx context.Context, message *components.ReceivedMessage) {
	assembleResponse := &engineProto.AssembleResponse{}

	err := proto.Unmarshal(message.Payload, assembleResponse)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, assembleResponse.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	transactionID, ok := sMgr.parseUUIDField(ctx, message, "transaction_id", assembleResponse.TransactionId)
	if !ok {
		return
	}

	requestID, ok := sMgr.parseUUIDField(ctx, message, "assemble_request_id", assembleResponse.AssembleRequestId)
	if !ok {
		return
	}

	if assembleResponse.PostAssembly == nil {
		log.L(ctx).Warnf("assemble response for transaction %s has nil post_assembly", assembleResponse.TransactionId)
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the assembly response
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: assemble response for transaction %s cannot be processed unless already in memory",
			contractAddress, assembleResponse.TransactionId)
		return
	}

	switch assembleResponse.PostAssembly.GetAssemblyResult() {
	case prototk.AssembleTransactionResponse_OK:
		assembleResponseEvent := &coordTransaction.AssembleSuccessEvent{}
		assembleResponseEvent.TransactionID = transactionID
		assembleResponseEvent.RequestID = requestID
		assembleResponseEvent.PostAssembly = assembleResponse.PostAssembly
		assembleResponseEvent.EventTime = time.Now()
		seq.GetCoordinator().QueueEvent(ctx, assembleResponseEvent)
	case prototk.AssembleTransactionResponse_PARK:
		log.L(ctx).Errorf("coordinator state machine cannot move from Assembling to Parked")
	case prototk.AssembleTransactionResponse_REVERT:
		assembleResponseEvent := &coordTransaction.AssembleRevertEvent{}
		assembleResponseEvent.TransactionID = transactionID
		assembleResponseEvent.RequestID = requestID
		assembleResponseEvent.PostAssembly = assembleResponse.PostAssembly
		assembleResponseEvent.EventTime = time.Now()
		seq.GetCoordinator().QueueEvent(ctx, assembleResponseEvent)
	default:
		log.L(ctx).Errorf("received unexpected assemble response type %s", assembleResponse.PostAssembly.GetAssemblyResult())
	}
}

func (sMgr *sequencerManager) handleAssembleError(ctx context.Context, message *components.ReceivedMessage) {
	assembleError := &engineProto.AssembleError{}

	err := proto.Unmarshal(message.Payload, assembleError)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, assembleError.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	transactionID, ok := sMgr.parseUUIDField(ctx, message, "transaction_id", assembleError.TransactionId)
	if !ok {
		return
	}

	requestID, ok := sMgr.parseUUIDField(ctx, message, "assemble_request_id", assembleError.AssembleRequestId)
	if !ok {
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: assemble error for transaction %s cannot be processed unless already in memory",
			contractAddress, assembleError.TransactionId)
		return
	}

	assembleErrorEvent := &coordTransaction.AssembleErrorEvent{}
	assembleErrorEvent.RequestID = requestID
	assembleErrorEvent.TransactionID = transactionID
	assembleErrorEvent.EventTime = time.Now()
	seq.GetCoordinator().QueueEvent(ctx, assembleErrorEvent)
}

// The three state query message handlers below route directly to the state view provider/reader —
// never through the coordinator/originator event loops. Both are internally thread-safe; requests
// are answered from the coordinator's current view and responses are correlated by request ID.
// message.FromNode is transport-authenticated and is the identity used for visibility filtering.

func (sMgr *sequencerManager) handleQueryAvailableStatesRequest(ctx context.Context, message *components.ReceivedMessage) {
	queryAvailableStatesRequest := &engineProto.QueryAvailableStatesRequest{}
	if err := proto.Unmarshal(message.Payload, queryAvailableStatesRequest); err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, queryAvailableStatesRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	// Get rather than load the sequencer - a state query is only valid against a coordinator
	// that has an ahead-of-chain view in memory right now
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: state query request %s cannot be processed unless already in memory",
			contractAddress, queryAvailableStatesRequest.RequestId)
		return
	}

	seq.GetCoordinator().StateViewProvider().HandleQueryAvailableStates(ctx, message.FromNode, queryAvailableStatesRequest)
}

func (sMgr *sequencerManager) handleQueryAvailableStatesResponse(ctx context.Context, message *components.ReceivedMessage) {
	queryAvailableStatesResponse := &engineProto.QueryAvailableStatesResponse{}
	if err := proto.Unmarshal(message.Payload, queryAvailableStatesResponse); err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, queryAvailableStatesResponse.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: state query response %s cannot be processed unless already in memory",
			contractAddress, queryAvailableStatesResponse.RequestId)
		return
	}

	seq.GetOriginator().StateViewReader().HandleQueryAvailableStatesResponse(ctx, message.FromNode, queryAvailableStatesResponse)
}

func (sMgr *sequencerManager) handleGetSpentStateIDsRequest(ctx context.Context, message *components.ReceivedMessage) {
	getSpentStateIDsRequest := &engineProto.GetSpentStateIDsRequest{}
	if err := proto.Unmarshal(message.Payload, getSpentStateIDsRequest); err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, getSpentStateIDsRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	// Get rather than load the sequencer - a state view request is only valid against a coordinator
	// that has an ahead-of-chain view in memory right now
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: get spent state IDs request %s cannot be processed unless already in memory",
			contractAddress, getSpentStateIDsRequest.RequestId)
		return
	}

	seq.GetCoordinator().StateViewProvider().HandleGetSpentStateIDs(ctx, message.FromNode, getSpentStateIDsRequest)
}

func (sMgr *sequencerManager) handleGetSpentStateIDsResponse(ctx context.Context, message *components.ReceivedMessage) {
	getSpentStateIDsResponse := &engineProto.GetSpentStateIDsResponse{}
	if err := proto.Unmarshal(message.Payload, getSpentStateIDsResponse); err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, getSpentStateIDsResponse.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: get spent state IDs response %s cannot be processed unless already in memory",
			contractAddress, getSpentStateIDsResponse.RequestId)
		return
	}

	seq.GetOriginator().StateViewReader().HandleGetSpentStateIDsResponse(ctx, message.FromNode, getSpentStateIDsResponse)
}

func (sMgr *sequencerManager) handleStateViewError(ctx context.Context, message *components.ReceivedMessage) {
	stateViewError := &engineProto.StateViewError{}
	if err := proto.Unmarshal(message.Payload, stateViewError); err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, stateViewError.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: state view error %s cannot be processed unless already in memory",
			contractAddress, stateViewError.RequestId)
		return
	}

	seq.GetOriginator().StateViewReader().HandleError(ctx, message.FromNode, stateViewError)
}

func (sMgr *sequencerManager) handleSignResponse(ctx context.Context, message *components.ReceivedMessage) {
	signResponse := &engineProto.SignResponse{}

	err := proto.Unmarshal(message.Payload, signResponse)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, signResponse.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	transactionID, ok := sMgr.parseUUIDField(ctx, message, "transaction_id", signResponse.TransactionId)
	if !ok {
		return
	}

	requestID, ok := sMgr.parseUUIDField(ctx, message, "assemble_request_id", signResponse.AssembleRequestId)
	if !ok {
		return
	}

	if signResponse.AttestationResult == nil {
		sMgr.logPaladinMessageFieldMissingError(ctx, message, "attestation_result")
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: sign response for transaction %s cannot be processed unless already in memory",
			contractAddress, signResponse.TransactionId)
		return
	}

	signedEvent := &coordTransaction.SignedEvent{}
	signedEvent.TransactionID = transactionID
	signedEvent.RequestID = requestID
	signedEvent.AttestationResult = signResponse.AttestationResult
	signedEvent.PostAssembly = signResponse.PostAssembly
	signedEvent.EventTime = time.Now()
	seq.GetCoordinator().QueueEvent(ctx, signedEvent)
}

func (sMgr *sequencerManager) handleSignError(ctx context.Context, message *components.ReceivedMessage) {
	signError := &engineProto.SignError{}

	err := proto.Unmarshal(message.Payload, signError)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, signError.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	transactionID, ok := sMgr.parseUUIDField(ctx, message, "transaction_id", signError.TransactionId)
	if !ok {
		return
	}

	requestID, ok := sMgr.parseUUIDField(ctx, message, "assemble_request_id", signError.AssembleRequestId)
	if !ok {
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: sign error for transaction %s cannot be processed unless already in memory",
			contractAddress, signError.TransactionId)
		return
	}

	signErrorEvent := &coordTransaction.SignErrorEvent{}
	signErrorEvent.TransactionID = transactionID
	signErrorEvent.RequestID = requestID
	signErrorEvent.EventTime = time.Now()
	seq.GetCoordinator().QueueEvent(ctx, signErrorEvent)
}

func (sMgr *sequencerManager) handleAssembleRejection(ctx context.Context, message *components.ReceivedMessage) {
	assembleRejection := &engineProto.AssembleRejection{}

	err := proto.Unmarshal(message.Payload, assembleRejection)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, assembleRejection.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	transactionID, ok := sMgr.parseUUIDField(ctx, message, "transaction_id", assembleRejection.TransactionId)
	if !ok {
		return
	}

	requestID, ok := sMgr.parseUUIDField(ctx, message, "assemble_request_id", assembleRejection.AssembleRequestId)
	if !ok {
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: assemble rejection for transaction %s cannot be processed unless already in memory",
			contractAddress, assembleRejection.TransactionId)
		return
	}

	assembleRejectedEvent := &coordTransaction.AssembleRequestRejectedEvent{}
	assembleRejectedEvent.RequestID = requestID
	assembleRejectedEvent.TransactionID = transactionID
	assembleRejectedEvent.EventTime = time.Now()
	assembleRejectedEvent.RejectionReason = assembleRejection.RejectionReason
	assembleRejectedEvent.CoordinatorBlockHeight = assembleRejection.CoordinatorBlockHeight
	assembleRejectedEvent.AssemblerBlockHeight = assembleRejection.AssemblerBlockHeight
	seq.GetCoordinator().QueueEvent(ctx, assembleRejectedEvent)
}

func (sMgr *sequencerManager) handleCoordinatorHeartbeatNotification(ctx context.Context, message *components.ReceivedMessage) {
	heartbeatNotification := &engineProto.CoordinatorHeartbeatNotification{}
	err := proto.Unmarshal(message.Payload, heartbeatNotification)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, heartbeatNotification.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	coordinatorSnapshot, err := common.CoordinatorSnapshotFromProto(ctx, heartbeatNotification.GetCoordinatorSnapshot())
	if err != nil {
		log.L(ctx).Errorf("failed to parse coordinator snapshot from heartbeat: %v", err)
		return
	}

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("failed to obtain sequencer for contract %s to pass heartbeat event: %v", contractAddress, err)
		return
	}

	heartbeatEvent := &common.HeartbeatReceivedEvent{}
	heartbeatEvent.FromNode = message.FromNode
	heartbeatEvent.ContractAddress = contractAddress
	heartbeatEvent.CoordinatorSnapshot = coordinatorSnapshot
	heartbeatEvent.EventTime = time.Now()
	seq.GetOriginator().QueueEvent(ctx, heartbeatEvent)
	seq.GetCoordinator().QueueEvent(ctx, heartbeatEvent)
}

func (sMgr *sequencerManager) handlePreDispatchRequest(ctx context.Context, message *components.ReceivedMessage) {
	preDispatchRequest := &engineProto.PreDispatchRequest{}

	err := proto.Unmarshal(message.Payload, preDispatchRequest)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, preDispatchRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	transactionID, ok := sMgr.parseUUIDField(ctx, message, "transaction_id", preDispatchRequest.TransactionId)
	if !ok {
		return
	}

	requestID, ok := sMgr.parseUUIDField(ctx, message, "id", preDispatchRequest.Id)
	if !ok {
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the predispatch request
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: predispatch request for transaction %s cannot be processed unless already in memory",
			contractAddress, preDispatchRequest.TransactionId)
		return
	}

	preDispatchRequestReceivedEvent := &originatorTransaction.PreDispatchRequestReceivedEvent{
		RequestID:   requestID,
		Coordinator: message.FromNode,
	}
	preDispatchRequestReceivedEvent.TransactionID = transactionID
	preDispatchRequestReceivedEvent.EventTime = time.Now()

	// TODO - not sure where we should make the decision as to whether or not to approve dispatch.
	// For now we just proceed and send an approval response. It's possible that the check belongs in the state machine
	// validator function for PreDispatchRequestReceivedEvent?

	seq.GetOriginator().QueueEvent(ctx, preDispatchRequestReceivedEvent)
}

func (sMgr *sequencerManager) handlePreDispatchResponse(ctx context.Context, message *components.ReceivedMessage) {
	preDispatchResponse := &engineProto.PreDispatchResponse{}

	err := proto.Unmarshal(message.Payload, preDispatchResponse)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, preDispatchResponse.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	transactionID, ok := sMgr.parseUUIDField(ctx, message, "transaction_id", preDispatchResponse.TransactionId)
	if !ok {
		return
	}

	requestID, ok := sMgr.parseUUIDField(ctx, message, "id", preDispatchResponse.Id)
	if !ok {
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the pre dispatch response
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: predispatch response for transaction %s cannot be processed unless already in memory",
			contractAddress, preDispatchResponse.TransactionId)
		return
	}

	// TODO - we don't yet return anything other than approved.

	dispatchRequestApprovedEvent := &coordTransaction.DispatchRequestApprovedEvent{
		RequestID: requestID,
	}
	dispatchRequestApprovedEvent.TransactionID = transactionID
	dispatchRequestApprovedEvent.EventTime = time.Now()
	seq.GetCoordinator().QueueEvent(ctx, dispatchRequestApprovedEvent)
}

func (sMgr *sequencerManager) handleDispatchedEvent(ctx context.Context, message *components.ReceivedMessage) {
	dispatchedEvent := &engineProto.TransactionDispatched{}

	err := proto.Unmarshal(message.Payload, dispatchedEvent)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, dispatchedEvent.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	transactionID, ok := sMgr.parseUUIDField(ctx, message, "transaction_id", dispatchedEvent.TransactionId)
	if !ok {
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the dispatched event
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: dispatched event for transaction %s cannot be processed unless already in memory",
			contractAddress, dispatchedEvent.TransactionId)
		return
	}

	dispatchConfirmedEvent := &originatorTransaction.DispatchedEvent{}
	dispatchConfirmedEvent.TransactionID = transactionID
	dispatchConfirmedEvent.Coordinator = message.FromNode
	dispatchConfirmedEvent.EventTime = time.Now()

	seq.GetOriginator().QueueEvent(ctx, dispatchConfirmedEvent)
}

func (sMgr *sequencerManager) handleDelegationRequest(ctx context.Context, message *components.ReceivedMessage) {
	delegationRequest := &engineProto.DelegationRequest{}
	err := proto.Unmarshal(message.Payload, delegationRequest)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	transactionDelegatedEvent := &coordinator.TransactionsDelegatedEvent{}
	transactionDelegatedEvent.FromNode = message.FromNode
	transactionDelegatedEvent.OriginatorsBlockHeight = uint64(delegationRequest.OriginatorBlockHeight)
	transactionDelegatedEvent.DelegationID = delegationRequest.DelegationId
	transactionDelegatedEvent.EventTime = time.Now()

	contractAddress := sMgr.parseContractAddressString(ctx, delegationRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	for _, del := range delegationRequest.Transactions {
		privateTransaction := &components.PrivateTransaction{
			Domain:      del.GetDomain(),
			Address:     *contractAddress,
			Intent:      del.GetIntent(),
			PreAssembly: del.GetPreAssembly(),
		}
		if privateTransaction.PreAssembly == nil {
			sMgr.logPaladinMessageFieldMissingError(ctx, message, "delegation.pre_assembly")
			return
		}
		if privateTransaction.PreAssembly.TransactionSpecification == nil {
			sMgr.logPaladinMessageFieldMissingError(ctx, message, "delegation.pre_assembly.transaction_specification")
			return
		}
		id, ok := sMgr.parseTransactionSpecificationID(ctx, message, "delegation.pre_assembly.transaction_specification.transaction_id", privateTransaction.PreAssembly.TransactionSpecification)
		if !ok {
			return
		}
		privateTransaction.ID = id
		// The coordinator sends each transaction's traffic to the node named in its own From, so a node
		// may only delegate transactions whose From is one of its own identities
		from := privateTransaction.PreAssembly.TransactionSpecification.From
		originatorNode, err := pldtypes.PrivateIdentityLocator(from).Node(ctx, false)
		if err == nil && originatorNode != message.FromNode {
			err = fmt.Errorf("originator is not on the sending node %s", message.FromNode)
		}
		if err != nil {
			sMgr.logPaladinMessageFieldInvalidError(ctx, message, "delegation.pre_assembly.transaction_specification.from", from, err)
			return
		}
		if transactionDelegatedEvent.Originator == "" {
			transactionDelegatedEvent.Originator = privateTransaction.PreAssembly.TransactionSpecification.From
		}
		transactionDelegatedEvent.Transactions = append(transactionDelegatedEvent.Transactions, privateTransaction)
	}

	if len(transactionDelegatedEvent.Transactions) == 0 {
		log.L(ctx).Warnf("delegation request from %s contained no transactions", message.FromNode)
		return
	}

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("failed to obtain sequencer to handle delegation request event %v:", err)
		return
	}

	seq.GetCoordinator().QueueEvent(ctx, transactionDelegatedEvent)
}

func (sMgr *sequencerManager) handleDelegationResponse(ctx context.Context, message *components.ReceivedMessage) {
	delegationRequestAcknowledgment := &engineProto.DelegationResponse{}
	err := proto.Unmarshal(message.Payload, delegationRequestAcknowledgment)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, delegationRequestAcknowledgment.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	rejectedDelegationIDs := make([]string, 0, len(delegationRequestAcknowledgment.TransactionIds))
	rejectedDelegationMaxInFlight := 0
	rejectedDelegationCoordinatorError := 0

	// Currently we don't act on specific errors, but we have the option in the future to treat a specific delegate rejection
	// differently to just relying on re-delegate on the next heartbeat/timeout. For now log explicit rejections from the coordinator.
	for i, errorCode := range delegationRequestAcknowledgment.Errors {
		switch coordinator.DelegationAcknowledgementError(errorCode) {
		case coordinator.DelegationAcknowledgementError_MaxInflightTransactions:
			rejectedDelegationIDs = append(rejectedDelegationIDs, delegationRequestAcknowledgment.TransactionIds[i])
			rejectedDelegationMaxInFlight++
		case coordinator.DelegationAcknowledgementError_CoordinatorError, coordinator.DelegationAcknowledgementError_PreviousTransactionError:
			rejectedDelegationCoordinatorError++
		}
	}

	if rejectedDelegationMaxInFlight > 0 {
		log.L(ctx).Debugf("coordinator rejected %d delegations with max in flight limit", rejectedDelegationMaxInFlight)
		log.L(ctx).Tracef("rejected delegations: %+v", rejectedDelegationIDs)
	}
	if rejectedDelegationCoordinatorError > 0 {
		log.L(ctx).Warnf("coordinator error processing %d delegations", rejectedDelegationCoordinatorError)
	}
}

func (sMgr *sequencerManager) handleDelegationRejection(ctx context.Context, message *components.ReceivedMessage) {
	delegationRejection := &engineProto.DelegationRejection{}
	err := proto.Unmarshal(message.Payload, delegationRejection)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, delegationRejection.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: delegation rejection cannot be processed unless already in memory", contractAddress)
		return
	}

	rejectedEvent := &originator.DelegationRequestRejectedEvent{}
	rejectedEvent.ActiveCoordinator = delegationRejection.ActiveCoordinator
	rejectedEvent.RejectionReason = delegationRejection.RejectionReason
	rejectedEvent.OriginatorBlockHeight = delegationRejection.OriginatorBlockHeight
	rejectedEvent.CoordinatorBlockHeight = delegationRejection.CoordinatorBlockHeight
	rejectedEvent.BlockHeightTolerance = delegationRejection.BlockHeightTolerance
	rejectedEvent.EventTime = time.Now()
	seq.GetOriginator().QueueEvent(ctx, rejectedEvent)
}

func (sMgr *sequencerManager) handleHandoverRequest(ctx context.Context, message *components.ReceivedMessage) {
	handoverRequest := &engineProto.CoordinatorHandoverRequest{}
	if err := proto.Unmarshal(message.Payload, handoverRequest); err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, handoverRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	// Get rather than load the sequencer- it must already have the coordinator in memory to process the handover request
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: handover request cannot be processed unless already in memory", contractAddress)
		return
	}

	handoverEvent := &coordinator.HandoverRequestEvent{}
	handoverEvent.FromNode = message.FromNode
	handoverEvent.EventTime = time.Now()
	seq.GetCoordinator().QueueEvent(ctx, handoverEvent)
}

func (sMgr *sequencerManager) handleEndorsementRequest(ctx context.Context, message *components.ReceivedMessage) {
	endorsementRequest := &engineProto.EndorsementRequest{}
	err := proto.Unmarshal(message.Payload, endorsementRequest)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, endorsementRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	if endorsementRequest.TransactionSpecification == nil {
		sMgr.logPaladinMessageFieldMissingError(ctx, message, "transaction_specification")
		return
	}

	transactionID, ok := sMgr.parseTransactionSpecificationID(ctx, message, "transaction_specification.transaction_id", endorsementRequest.TransactionSpecification)
	if !ok {
		return
	}

	privateEndorsementRequest := &components.PrivateTransactionEndorseRequest{
		BlockContext:             endorsementRequest.BlockContext,
		TransactionSpecification: endorsementRequest.TransactionSpecification,
		Verifiers:                endorsementRequest.Verifiers,
		Signatures:               endorsementRequest.Signatures,
		InputStates:              endorsementRequest.InputStates,
		ReadStates:               endorsementRequest.ReadStates,
		OutputStates:             endorsementRequest.OutputStates,
		InfoStates:               endorsementRequest.InfoStates,
		Endorsement:              endorsementRequest.AttestationRequest,
		// Endorser is resolved by the coordinator goroutine via KeyManager.
	}

	for _, state := range privateEndorsementRequest.InfoStates {
		log.L(ctx).Debugf("private endorsement info state: %+v", state)
	}

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("handleEndorsementRequest failed to obtain sequencer: %v", err)
		return
	}

	endorsementRequestReceivedEvent := &coordinator.EndorsementRequestReceivedEvent{
		FromNode:                  message.FromNode,
		TransactionId:             transactionID.String(),
		IdempotencyKey:            endorsementRequest.IdempotencyKey,
		Party:                     endorsementRequest.Party,
		PrivateEndorsementRequest: privateEndorsementRequest,
		AttestationRequest:        endorsementRequest.AttestationRequest,
		CoordinatorBlockHeight:    endorsementRequest.CoordinatorBlockHeight,
		BlockHeightTolerance:      endorsementRequest.BlockHeightTolerance,
	}
	if endorsementRequest.ExpiryTimeUnixMs != 0 {
		endorsementRequestReceivedEvent.Expiry = time.UnixMilli(endorsementRequest.ExpiryTimeUnixMs)
	}
	seq.GetCoordinator().QueueEvent(ctx, endorsementRequestReceivedEvent)
}

func (sMgr *sequencerManager) handleEndorsementResponse(ctx context.Context, message *components.ReceivedMessage) {
	endorsementResponse := &engineProto.EndorsementResponse{}
	err := proto.Unmarshal(message.Payload, endorsementResponse)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, endorsementResponse.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	transactionID, ok := sMgr.parseUUIDField(ctx, message, "transaction_id", endorsementResponse.TransactionId)
	if !ok {
		return
	}

	requestID, ok := sMgr.parseUUIDField(ctx, message, "idempotency_key", endorsementResponse.IdempotencyKey)
	if !ok {
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the endorsement response
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: endorsement response for transaction %s cannot be processed unless already in memory",
			contractAddress, endorsementResponse.TransactionId)
		return
	}

	// Endorsement reverted
	if endorsementResponse.GetRevertReason() != "" {
		endorseRevertEvent := &coordTransaction.EndorseRevertEvent{}
		endorseRevertEvent.TransactionID = transactionID
		endorseRevertEvent.RequestID = requestID
		endorseRevertEvent.EventTime = time.Now()
		endorseRevertEvent.Party = endorsementResponse.Party
		endorseRevertEvent.RevertReason = endorsementResponse.GetRevertReason()
		endorseRevertEvent.AttestationRequestName = endorsementResponse.AttestationRequestName
		seq.GetCoordinator().QueueEvent(ctx, endorseRevertEvent)
		return
	}

	// Endorsement succeeded
	endorsement := endorsementResponse.Endorsement

	endorsementResponseEvent := &coordTransaction.EndorsedEvent{}
	endorsementResponseEvent.TransactionID = transactionID
	endorsementResponseEvent.RequestID = requestID
	endorsementResponseEvent.Endorsement = endorsement
	endorsementResponseEvent.EventTime = time.Now()
	seq.GetCoordinator().QueueEvent(ctx, endorsementResponseEvent)
}

func (sMgr *sequencerManager) handleEndorsementRejection(ctx context.Context, message *components.ReceivedMessage) {
	endorsementRejection := &engineProto.EndorsementRejection{}
	err := proto.Unmarshal(message.Payload, endorsementRejection)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, endorsementRejection.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	transactionID, ok := sMgr.parseUUIDField(ctx, message, "transaction_id", endorsementRejection.TransactionId)
	if !ok {
		return
	}

	requestID, ok := sMgr.parseUUIDField(ctx, message, "idempotency_key", endorsementRejection.IdempotencyKey)
	if !ok {
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: endorsement rejection for transaction %s cannot be processed unless already in memory",
			contractAddress, endorsementRejection.TransactionId)
		return
	}

	endorseRejectedEvent := &coordTransaction.EndorseRequestRejectedEvent{}
	endorseRejectedEvent.TransactionID = transactionID
	endorseRejectedEvent.RequestID = requestID
	endorseRejectedEvent.EventTime = time.Now()
	endorseRejectedEvent.Party = endorsementRejection.Party
	endorseRejectedEvent.AttestationRequestName = endorsementRejection.AttestationRequestName
	endorseRejectedEvent.RejectionReason = endorsementRejection.RejectionReason
	endorseRejectedEvent.CoordinatorBlockHeight = endorsementRejection.CoordinatorBlockHeight
	endorseRejectedEvent.EndorserBlockHeight = endorsementRejection.EndorserBlockHeight
	endorseRejectedEvent.BlockHeightTolerance = endorsementRejection.BlockHeightTolerance
	seq.GetCoordinator().QueueEvent(ctx, endorseRejectedEvent)
}

func (sMgr *sequencerManager) handleEndorsementError(ctx context.Context, message *components.ReceivedMessage) {
	endorsementError := &engineProto.EndorsementError{}
	err := proto.Unmarshal(message.Payload, endorsementError)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, endorsementError.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	transactionID, ok := sMgr.parseUUIDField(ctx, message, "transaction_id", endorsementError.TransactionId)
	if !ok {
		return
	}

	requestID, ok := sMgr.parseUUIDField(ctx, message, "idempotency_key", endorsementError.IdempotencyKey)
	if !ok {
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: endorsement error for transaction %s cannot be processed unless already in memory",
			contractAddress, endorsementError.TransactionId)
		return
	}

	endorseErrorEvent := &coordTransaction.EndorseErrorEvent{}
	endorseErrorEvent.TransactionID = transactionID
	endorseErrorEvent.RequestID = requestID
	endorseErrorEvent.EventTime = time.Now()
	endorseErrorEvent.Party = endorsementError.Party
	endorseErrorEvent.AttestationRequestName = endorsementError.AttestationRequestName
	seq.GetCoordinator().QueueEvent(ctx, endorseErrorEvent)
}

func (sMgr *sequencerManager) handleNonceAssigned(ctx context.Context, message *components.ReceivedMessage) {
	nonceAssigned := &engineProto.NonceAssigned{}
	err := proto.Unmarshal(message.Payload, nonceAssigned)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, nonceAssigned.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	transactionID, ok := sMgr.parseUUIDField(ctx, message, "transaction_id", nonceAssigned.TransactionId)
	if !ok {
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the nonce assigned event
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: nonce assigned event for transaction %s cannot be processed unless already in memory",
			contractAddress, nonceAssigned.TransactionId)
		return
	}

	nonceAssignedEvent := &originatorTransaction.NonceAssignedEvent{}
	nonceAssignedEvent.TransactionID = transactionID
	nonceAssignedEvent.Nonce = uint64(nonceAssigned.Nonce)
	nonceAssignedEvent.Coordinator = message.FromNode
	nonceAssignedEvent.EventTime = time.Now()

	seq.GetOriginator().QueueEvent(ctx, nonceAssignedEvent)
}

func (sMgr *sequencerManager) handleTransactionSubmitted(ctx context.Context, message *components.ReceivedMessage) {
	transactionSubmitted := &engineProto.TransactionSubmitted{}
	err := proto.Unmarshal(message.Payload, transactionSubmitted)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, transactionSubmitted.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	transactionID, ok := sMgr.parseUUIDField(ctx, message, "transaction_id", transactionSubmitted.TransactionId)
	if !ok {
		return
	}

	submissionHash, ok := sMgr.parseBytes32Field(ctx, message, "hash", transactionSubmitted.Hash)
	if !ok {
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the transaction submitted event
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: transaction submitted event for transaction %s cannot be processed unless already in memory",
			contractAddress, transactionSubmitted.TransactionId)
		return
	}

	transactionSubmittedEvent := &originatorTransaction.SubmittedEvent{}
	transactionSubmittedEvent.TransactionID = transactionID
	transactionSubmittedEvent.LatestSubmissionHash = submissionHash
	transactionSubmittedEvent.Coordinator = message.FromNode
	transactionSubmittedEvent.EventTime = time.Now()

	seq.GetOriginator().QueueEvent(ctx, transactionSubmittedEvent)
}

func (sMgr *sequencerManager) handleTransactionConfirmed(ctx context.Context, message *components.ReceivedMessage) {
	transactionConfirmed := &engineProto.TransactionConfirmed{}
	err := proto.Unmarshal(message.Payload, transactionConfirmed)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, transactionConfirmed.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	transactionID, ok := sMgr.parseUUIDField(ctx, message, "transaction_id", transactionConfirmed.TransactionId)
	if !ok {
		return
	}

	// Get rather than load the sequencer- it must already have the transaction in memory to process the transaction confirmed event
	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: transaction confirmed event for transaction %s cannot be processed unless already in memory",
			contractAddress, transactionConfirmed.TransactionId)
		return
	}

	if transactionConfirmed.GetOutcome() == engineProto.TransactionConfirmed_OUTCOME_REVERTED {
		transactionSubmittedEvent := &originatorTransaction.ConfirmedRevertedEvent{}
		transactionSubmittedEvent.TransactionID = transactionID
		transactionSubmittedEvent.RevertReason = transactionConfirmed.RevertReason
		transactionSubmittedEvent.FailureMessage = transactionConfirmed.GetFailureMessage()
		transactionSubmittedEvent.WillRetry = transactionConfirmed.WillRetry
		transactionSubmittedEvent.EventTime = time.Now()
		seq.GetOriginator().QueueEvent(ctx, transactionSubmittedEvent)
	} else {
		transactionSubmittedEvent := &originatorTransaction.ConfirmedSuccessEvent{}
		transactionSubmittedEvent.TransactionID = transactionID
		transactionSubmittedEvent.EventTime = time.Now()
		seq.GetOriginator().QueueEvent(ctx, transactionSubmittedEvent)
	}
}

func (sMgr *sequencerManager) handlePreDispatchRejection(ctx context.Context, message *components.ReceivedMessage) {
	rejection := &engineProto.PreDispatchRejection{}
	if err := proto.Unmarshal(message.Payload, rejection); err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, rejection.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	txID, err := uuid.Parse(rejection.TransactionId)
	if err != nil {
		log.L(ctx).Errorf("handlePreDispatchRejection: invalid transaction ID %q: %v", rejection.TransactionId, err)
		return
	}

	requestID, err := uuid.Parse(rejection.RequestId)
	if err != nil {
		log.L(ctx).Errorf("handlePreDispatchRejection: invalid request ID %q: %v", rejection.RequestId, err)
		return
	}

	seq := sMgr.GetSequencer(ctx, *contractAddress)
	if seq == nil {
		log.L(ctx).Warnf("sequencer for contract %s is not loaded: pre-dispatch rejection for transaction %s cannot be processed unless already in memory",
			contractAddress, rejection.TransactionId)
		return
	}

	log.L(ctx).Debugf("received pre-dispatch rejection for tx %s from originator %s (reason=%d)", txID, message.FromNode, rejection.RejectionReason)

	rejectedEvent := &coordTransaction.PreDispatchRequestRejectedEvent{
		BaseCoordinatorEvent: coordTransaction.BaseCoordinatorEvent{
			TransactionID: txID,
		},
		RequestID:       requestID,
		RejectionReason: rejection.RejectionReason,
	}
	rejectedEvent.EventTime = time.Now()
	seq.GetCoordinator().QueueEvent(ctx, rejectedEvent)
}
