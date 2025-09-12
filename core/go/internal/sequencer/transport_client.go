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
	"encoding/json"
	"fmt"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/coordinator"
	coordTransaction "github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/sender"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/sender/transaction"
	senderTransaction "github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/sender/transaction"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/transport"
	engineProto "github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/proto/engine"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

func (d *distributedSequencerManager) HandlePaladinMsg(ctx context.Context, message *components.ReceivedMessage) {
	//TODO this need to become an ultra low latency, non blocking, handover to the event loop thread.
	// need some thought on how to handle errors, retries, buffering, swapping idle sequencers in and out of memory etc...

	d.logPaladinMessage(ctx, message)

	//Send the event to the sequencer handler
	switch message.MessageType {
	case transport.MessageType_AssembleRequest:
		go d.handleAssembleRequest(d.ctx, message)
	case transport.MessageType_AssembleResponse:
		go d.handleAssembleResponse(d.ctx, message)
	case transport.MessageType_AssembleError:
		go d.handleAssembleError(d.ctx, message)
	case transport.MessageType_CoordinatorHeartbeatNotification:
		go d.handleCoordinatorHeartbeatNotification(d.ctx, message)
	case transport.MessageType_DelegationRequest:
		go d.handleDelegationRequest(d.ctx, message)
	case transport.MessageType_DelegationRequestAcknowledgment:
		go d.handleDelegationRequestAcknowledgment(d.ctx, message)
	case transport.MessageType_Dispatched:
		go d.handleDispatchedEvent(d.ctx, message)
	case transport.MessageType_DispatchConfirmationRequest:
		go d.handleDispatchConfirmationRequest(d.ctx, message)
	case transport.MessageType_EndorsementRequest:
		go d.handleEndorsementRequest(d.ctx, message)
	case transport.MessageType_EndorsementResponse:
		go d.handleEndorsementResponse(d.ctx, message)
	case transport.MessageType_NonceAssigned:
		go d.handleNonceAssigned(d.ctx, message)
	case transport.MessageType_TransactionSubmitted:
		go d.handleTransactionSubmitted(d.ctx, message)
	case transport.MessageType_TransactionConfirmed:
		go d.handleTransactionConfirmed(d.ctx, message)
	default:
		log.L(ctx).Errorf("Unknown message type: %s", message.MessageType)
	}
}

func (d *distributedSequencerManager) logPaladinMessage(ctx context.Context, message *components.ReceivedMessage) {
	//log.L(ctx).Debugf("[Sequencer] << proto message %s received from %s", message.MessageType, message.FromNode)
	log.L(log.WithComponent(ctx, common.COMPONENT_SEQUENCER, common.SUBCOMP_MSGRX)).Debugf("%+v received from %s", message.MessageType, message.FromNode)
}

func (d *distributedSequencerManager) logPaladinMessageUnmarshalError(ctx context.Context, message *components.ReceivedMessage, err error) {
	log.L(ctx).Errorf("[Sequencer] << ERROR unmarshalling proto message%s from %s: %s", message.MessageType, message.FromNode, err)
}

func (d *distributedSequencerManager) logPaladinMessageFieldMissingError(ctx context.Context, message *components.ReceivedMessage, field string) {
	log.L(ctx).Errorf("[Sequencer] << field %s missing from proto message %s received from %s", field, message.MessageType, message.FromNode)
}

func (d *distributedSequencerManager) logPaladinMessageJsonUnmarshalError(ctx context.Context, jsonObject string, message *components.ReceivedMessage, err error) {
	log.L(ctx).Errorf("[Sequencer] << ERROR unmarshalling JSON object %s from proto message %s (received from %s): %s", jsonObject, message.MessageType, message.FromNode, err)
}

func (d *distributedSequencerManager) parseContractAddressString(ctx context.Context, contractAddressString string, message *components.ReceivedMessage) *pldtypes.EthAddress {
	contractAddress, err := pldtypes.ParseEthAddress(contractAddressString)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] << ERROR unmarshalling contract address from proto message %s (received from %s): %s", message.MessageType, message.FromNode, err)
		return nil
	}
	return contractAddress
}

func (d *distributedSequencerManager) handleAssembleRequest(ctx context.Context, message *components.ReceivedMessage) {
	assembleRequest := &engineProto.AssembleRequest{}
	err := proto.Unmarshal(message.Payload, assembleRequest)
	if err != nil {
		d.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	preAssembly := &components.TransactionPreAssembly{}
	err = json.Unmarshal(assembleRequest.PreAssembly, preAssembly)
	if err != nil {
		d.logPaladinMessageJsonUnmarshalError(ctx, "TransactionPreAssembly", message, err)
		return
	}
	log.L(ctx).Infof("[Sequencer] We've been asked to handle an assemble request. How many required verifiers are there? %d How many actual verifiers are there? %d", len(preAssembly.RequiredVerifiers), len(preAssembly.Verifiers))
	for _, verifier := range preAssembly.RequiredVerifiers {
		log.L(ctx).Infof("[Sequencer] Required verifier: %+v", verifier)
	}
	for _, verifier := range preAssembly.Verifiers {
		log.L(ctx).Infof("[Sequencer] Actual verifier: %+v", verifier)
	}

	contractAddress := d.parseContractAddressString(ctx, assembleRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] failed to obtain sequencer to pass assemble request event to %v:", err)
		return
	}
	if seq == nil {
		log.L(ctx).Errorf("[Sequencer] no sequencer found for contract %s", contractAddress.String())
		return
	}

	assembleRequestEvent := &transaction.AssembleRequestReceivedEvent{}
	assembleRequestEvent.TransactionID = uuid.MustParse(assembleRequest.TransactionId)
	assembleRequestEvent.RequestID = uuid.MustParse(assembleRequest.AssembleRequestId)
	assembleRequestEvent.Coordinator = seq.GetCoordinator().GetActiveCoordinatorNode(ctx)
	assembleRequestEvent.CoordinatorsBlockHeight = assembleRequest.BlockHeight
	assembleRequestEvent.StateLocksJSON = assembleRequest.StateLocks
	assembleRequestEvent.PreAssembly = assembleRequest.PreAssembly
	assembleRequestEvent.EventTime = time.Now()

	seq.GetSender().HandleEvent(ctx, assembleRequestEvent)
}

func (d *distributedSequencerManager) handleAssembleResponse(ctx context.Context, message *components.ReceivedMessage) {
	assembleResponse := &engineProto.AssembleResponse{}

	err := proto.Unmarshal(message.Payload, assembleResponse)
	if err != nil {
		d.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := d.parseContractAddressString(ctx, assembleResponse.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	postAssembly := &components.TransactionPostAssembly{}
	err = json.Unmarshal(assembleResponse.PostAssembly, postAssembly)
	if err != nil {
		d.logPaladinMessageJsonUnmarshalError(ctx, "TransactionPostAssembly", message, err)
		return
	}

	err = json.Unmarshal(assembleResponse.PostAssembly, postAssembly)
	if err != nil {
		d.logPaladinMessageJsonUnmarshalError(ctx, "TransactionPostAssembly", message, err)
		return
	}

	preAssembly := &components.TransactionPreAssembly{}
	err = json.Unmarshal(assembleResponse.PreAssembly, preAssembly)
	if err != nil {
		d.logPaladinMessageJsonUnmarshalError(ctx, "TransactionPreAssembly", message, err)
		return
	}

	err = json.Unmarshal(assembleResponse.PreAssembly, preAssembly)
	if err != nil {
		d.logPaladinMessageJsonUnmarshalError(ctx, "TransactionPreAssembly", message, err)
		return
	}

	log.L(ctx).Infof("[Sequencer] preAssembly required verifiers: %+v", preAssembly.RequiredVerifiers)
	log.L(ctx).Infof("[Sequencer] preAssembly verifiers: %+v", preAssembly.Verifiers)

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] failed to obtain sequencer to pass assemble response event %v:", err)
		return
	}
	if seq == nil {
		log.L(ctx).Errorf("[Sequencer] no sequencer found for contract %s", contractAddress.String())
		return
	}

	switch postAssembly.AssemblyResult {
	case prototk.AssembleTransactionResponse_OK:
		log.L(ctx).Infof("[Sequencer] handing off assemble request and sign success event to distributed sequencer coordinator")
		assembleResponseEvent := &coordTransaction.AssembleSuccessEvent{}
		assembleResponseEvent.TransactionID = uuid.MustParse(assembleResponse.TransactionId)
		assembleResponseEvent.RequestID = uuid.MustParse(assembleResponse.AssembleRequestId)
		assembleResponseEvent.PostAssembly = postAssembly
		assembleResponseEvent.PreAssembly = preAssembly
		assembleResponseEvent.EventTime = time.Now()
		seq.GetCoordinator().HandleEvent(ctx, assembleResponseEvent)
	case prototk.AssembleTransactionResponse_PARK:
		log.L(ctx).Errorf("[Sequencer] coordinator state machine cannot move from Assembling to Parked")
	case prototk.AssembleTransactionResponse_REVERT:
		log.L(ctx).Infof("[Sequencer] handing off assemble revert event to distributed sequencer coordinator")
		assembleResponseEvent := &coordTransaction.AssembleRevertResponseEvent{}
		assembleResponseEvent.TransactionID = uuid.MustParse(assembleResponse.TransactionId)
		assembleResponseEvent.RequestID = uuid.MustParse(assembleResponse.AssembleRequestId)
		assembleResponseEvent.PostAssembly = postAssembly
		assembleResponseEvent.EventTime = time.Now()
		seq.GetCoordinator().HandleEvent(ctx, assembleResponseEvent)
	default:
		log.L(ctx).Errorf("[Sequencer] received unexpected assemble response type %s", postAssembly.AssemblyResult)
	}
}

func (d *distributedSequencerManager) handleAssembleError(ctx context.Context, message *components.ReceivedMessage) {
	assembleError := &engineProto.AssembleError{}

	err := proto.Unmarshal(message.Payload, assembleError)
	if err != nil {
		d.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := d.parseContractAddressString(ctx, assembleError.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	assembleErrorEvent := &transaction.AssembleErrorEvent{}
	assembleErrorEvent.TransactionID = uuid.MustParse(assembleError.TransactionId)
	assembleErrorEvent.EventTime = time.Now()

	errorString := assembleError.ErrorMessage
	log.L(ctx).Infof("[Sequencer] assemble error for TX %s: %s", assembleError.TransactionId, errorString)

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] failed to obtain sequencer to pass assemble error event %v:", err)
		return
	}
	if seq == nil {
		log.L(ctx).Errorf("[Sequencer] no sequencer found for contract %s", contractAddress.String())
		return
	}
	seq.GetCoordinator().HandleEvent(ctx, assembleErrorEvent)
}

func (d *distributedSequencerManager) handleCoordinatorHeartbeatNotification(ctx context.Context, message *components.ReceivedMessage) {
	heartbeatNotification := &engineProto.CoordinatorHeartbeatNotification{}
	err := proto.Unmarshal(message.Payload, heartbeatNotification)
	if err != nil {
		d.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	from := heartbeatNotification.From
	if from == "" {
		d.logPaladinMessageFieldMissingError(ctx, message, "From")
		return
	}

	contractAddress := d.parseContractAddressString(ctx, heartbeatNotification.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	coordinatorSnapshot := &common.CoordinatorSnapshot{}
	err = json.Unmarshal(heartbeatNotification.CoordinatorSnapshot, coordinatorSnapshot)
	if err != nil {
		d.logPaladinMessageJsonUnmarshalError(ctx, "CoordinatorSnapshot", message, err)
		return
	}

	heartbeatEvent := &sender.HeartbeatReceivedEvent{}
	heartbeatEvent.From = from
	heartbeatEvent.ContractAddress = contractAddress
	heartbeatEvent.CoordinatorSnapshot = *coordinatorSnapshot
	heartbeatEvent.EventTime = time.Now()

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] failed to obtain sequencer to pass heartbeat event to %v:", err)
		return
	}
	if seq == nil {
		log.L(ctx).Errorf("[Sequencer] no sequencer found for contract %s", contractAddress.String())
		return
	}

	for _, transaction := range coordinatorSnapshot.ConfirmedTransactions {
		log.L(ctx).Infof("[Sequencer] received a heartbeat containing a confirmed transaction: %s", transaction.ID.String())
		heartbeatIntervalEvent := &coordTransaction.HeartbeatIntervalEvent{}
		heartbeatIntervalEvent.TransactionID = transaction.ID
		heartbeatIntervalEvent.EventTime = time.Now()
		seq.GetCoordinator().HandleEvent(ctx, heartbeatIntervalEvent)
	}

	seq.GetCoordinator().HandleEvent(ctx, heartbeatEvent)
}

func (d *distributedSequencerManager) handleDispatchConfirmationRequest(ctx context.Context, message *components.ReceivedMessage) {
	dispatchConfirmationRequest := &engineProto.TransactionDispatched{}

	err := proto.Unmarshal(message.Payload, dispatchConfirmationRequest)
	if err != nil {
		d.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := d.parseContractAddressString(ctx, dispatchConfirmationRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] failed to obtain sequencer to pass dispatch confirmation event to %v:", err)
		return
	}
	if seq == nil {
		log.L(ctx).Errorf("[Sequencer] no sequencer found for contract %s", contractAddress.String())
		return
	}

	postAssemblyHash := pldtypes.NewBytes32FromSlice(dispatchConfirmationRequest.PostAssembleHash)

	dispatchConfirmationRequestReceivedEvent := &transaction.DispatchConfirmationRequestReceivedEvent{
		RequestID:        uuid.MustParse(dispatchConfirmationRequest.Id),
		Coordinator:      message.FromNode,
		PostAssemblyHash: &postAssemblyHash,
	}
	dispatchConfirmationRequestReceivedEvent.TransactionID = uuid.MustParse(dispatchConfirmationRequest.TransactionId[2:34])
	dispatchConfirmationRequestReceivedEvent.EventTime = time.Now()
	seq.GetSender().HandleEvent(ctx, dispatchConfirmationRequestReceivedEvent)
}

func (d *distributedSequencerManager) handleDispatchedEvent(ctx context.Context, message *components.ReceivedMessage) {
	dispatchedEvent := &engineProto.TransactionDispatched{}

	err := proto.Unmarshal(message.Payload, dispatchedEvent)
	if err != nil {
		d.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := d.parseContractAddressString(ctx, dispatchedEvent.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] failed to obtain sequencer to pass dispatch confirmation event to %v:", err)
		return
	}
	if seq == nil {
		log.L(ctx).Errorf("[Sequencer] no sequencer found for contract %s", contractAddress.String())
		return
	}

	dispatchConfirmedEvent := &senderTransaction.DispatchedEvent{}
	dispatchConfirmedEvent.TransactionID = uuid.MustParse(dispatchedEvent.TransactionId[2:34])
	dispatchConfirmedEvent.EventTime = time.Now()

	seq.GetSender().HandleEvent(ctx, dispatchConfirmedEvent)
}

func (d *distributedSequencerManager) handleDelegationRequest(ctx context.Context, message *components.ReceivedMessage) {
	delegationRequest := &engineProto.DelegationRequest{}
	err := proto.Unmarshal(message.Payload, delegationRequest)
	if err != nil {
		d.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	privateTransaction := &components.PrivateTransaction{}
	err = json.Unmarshal(delegationRequest.PrivateTransaction, privateTransaction)
	if err != nil {
		d.logPaladinMessageJsonUnmarshalError(ctx, "PrivateTransaction", message, err)
		return
	}

	contractAddress := d.parseContractAddressString(ctx, privateTransaction.PreAssembly.TransactionSpecification.ContractInfo.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	// MRW TODO - do we need to check if this coordinator has successfully confirmed this transaction on chain already?

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] handleDelegationRequest failed to obtain sequencer to pass endorsement event %v:", err)
		return
	}
	if seq == nil {
		log.L(ctx).Errorf("[Sequencer] no sequencer found for contract %s", contractAddress.String())
		return
	}

	transactionDelegatedEvent := &coordinator.TransactionsDelegatedEvent{}
	transactionDelegatedEvent.Sender = privateTransaction.PreAssembly.TransactionSpecification.From
	transactionDelegatedEvent.Transactions = append(transactionDelegatedEvent.Transactions, privateTransaction)
	transactionDelegatedEvent.SendersBlockHeight = uint64(d.blockHeight) // MRW TODO - which is the senders block height here?
	transactionDelegatedEvent.EventTime = time.Now()

	// Anyone who delegates a transaction to us is a candidate sender and should be sent heartbeats for TX confirmation processing
	seq.GetCoordinator().UpdateSenderNodePool(ctx, message.FromNode)

	seq.GetCoordinator().HandleEvent(ctx, transactionDelegatedEvent)
}

func (d *distributedSequencerManager) handleDelegationRequestAcknowledgment(ctx context.Context, message *components.ReceivedMessage) {
	delegationRequestAcknowledgment := &engineProto.DelegationRequestAcknowledgment{}
	err := proto.Unmarshal(message.Payload, delegationRequestAcknowledgment)
	if err != nil {
		d.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	// MRW TODO - is any action required here?
	log.L(ctx).Infof("[Sequencer] handleDelegationRequestAcknowledgment received for transaction ID %s", delegationRequestAcknowledgment.TransactionId)
}

func (d *distributedSequencerManager) handleEndorsementRequest(ctx context.Context, message *components.ReceivedMessage) {
	endorsementRequest := &engineProto.EndorsementRequest{}

	err := proto.Unmarshal(message.Payload, endorsementRequest)
	if err != nil {
		d.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := d.parseContractAddressString(ctx, endorsementRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	psc, err := d.components.DomainManager().GetSmartContractByAddress(ctx, d.components.Persistence().NOTX(), *contractAddress)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to get domain for endorsement request: %s", err)
		return
	}

	transactionSpecification := &prototk.TransactionSpecification{}
	err = proto.Unmarshal(endorsementRequest.TransactionSpecification.Value, transactionSpecification)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to unmarshal transaction specification for endorsement request: %s", err)
		return
	}
	log.L(ctx).Debugf("Unmarshalled transaction specification: %+v", transactionSpecification)

	transactionVerifiers := make([]*prototk.ResolvedVerifier, len(endorsementRequest.Verifiers))
	for i, v := range endorsementRequest.Verifiers {
		nextVerifier := &prototk.ResolvedVerifier{}
		err = proto.Unmarshal(v.Value, nextVerifier)
		if err != nil {
			log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to unmarshal verifier %s for endorsement request: %s", v.String(), err)
			return
		}
		transactionVerifiers[i] = nextVerifier
	}
	log.L(ctx).Debugf("Unmarshalled transaction verifiers: %+v", transactionVerifiers)

	transactionSignatures := make([]*prototk.AttestationResult, len(endorsementRequest.Signatures))
	for i, s := range endorsementRequest.Signatures {
		nextSignature := &prototk.AttestationResult{}
		err = proto.Unmarshal(s.Value, nextSignature)
		if err != nil {
			log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to unmarshal signature %s for endorsement request: %s", s.String(), err)
			return
		}
		transactionSignatures[i] = nextSignature
	}

	transactionInputStates := make([]*prototk.EndorsableState, len(endorsementRequest.InputStates))
	for i, s := range endorsementRequest.InputStates {
		nextState := &prototk.EndorsableState{}
		err = proto.Unmarshal(s.Value, nextState)
		if err != nil {
			log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to unmarshal input state %s for endorsement request: %s", s.String(), err)
			return
		}
		transactionInputStates[i] = nextState
	}

	transactionReadStates := make([]*prototk.EndorsableState, len(endorsementRequest.ReadStates))
	for i, s := range endorsementRequest.ReadStates {
		nextState := &prototk.EndorsableState{}
		err = proto.Unmarshal(s.Value, nextState)
		if err != nil {
			log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to unmarshal read state %s for endorsement request: %s", s.String(), err)
			return
		}
		transactionReadStates[i] = nextState
	}

	transactionOutputStates := make([]*prototk.EndorsableState, len(endorsementRequest.OutputStates))
	for i, s := range endorsementRequest.OutputStates {
		nextState := &prototk.EndorsableState{}
		err = proto.Unmarshal(s.Value, nextState)
		if err != nil {
			log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to unmarshal output state %s for endorsement request: %s", s.String(), err)
			return
		}
		transactionOutputStates[i] = nextState
	}

	transactionInfoStates := make([]*prototk.EndorsableState, len(endorsementRequest.InfoStates))
	log.L(ctx).Debugf("[Sequencer] number of transaction info states: %+v", len(endorsementRequest.InfoStates))
	for i, s := range endorsementRequest.InfoStates {
		nextState := &prototk.EndorsableState{}
		err = proto.Unmarshal(s.Value, nextState)
		if err != nil {
			log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to unmarshal info state %s for endorsement request: %s", s.String(), err)
			return
		}
		transactionInfoStates[i] = nextState
	}

	transactionEndorsement := &prototk.AttestationRequest{}
	err = proto.Unmarshal(endorsementRequest.AttestationRequest.Value, transactionEndorsement)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to unmarshal endorsement request: %s", err)
		return
	}

	unqualifiedLookup, err := pldtypes.PrivateIdentityLocator(endorsementRequest.Party).Identity(ctx)
	if err != nil {
		errorMessage := fmt.Sprintf("[Sequencer] failed to parse lookup key for party %s : %s", endorsementRequest.Party, err)
		log.L(ctx).Error(errorMessage)
		return
	}

	resolvedSigner, err := d.components.KeyManager().ResolveKeyNewDatabaseTX(ctx, unqualifiedLookup, transactionEndorsement.Algorithm, transactionEndorsement.VerifierType)
	if err != nil {
		errorMessage := fmt.Sprintf("[Sequencer] failed to resolve key for party %s", endorsementRequest.Party)
		log.L(ctx).Error(errorMessage)
		return
	}

	privateEndorsementRequest := &components.PrivateTransactionEndorseRequest{}
	privateEndorsementRequest.TransactionSpecification = transactionSpecification
	privateEndorsementRequest.Verifiers = transactionVerifiers
	privateEndorsementRequest.Signatures = transactionSignatures
	privateEndorsementRequest.InputStates = transactionInputStates
	privateEndorsementRequest.ReadStates = transactionReadStates
	privateEndorsementRequest.OutputStates = transactionOutputStates
	privateEndorsementRequest.InfoStates = transactionInfoStates

	// Log private endorsement info states length
	log.L(ctx).Debugf("[Sequencer] private endorsement info states length: %+v", len(privateEndorsementRequest.InfoStates))
	for _, state := range privateEndorsementRequest.InfoStates {
		log.L(ctx).Debugf("[Sequencer] private endorsement info state: %+v", state)
	}

	privateEndorsementRequest.Endorsement = transactionEndorsement
	privateEndorsementRequest.Endorser = &prototk.ResolvedVerifier{
		Lookup:       endorsementRequest.Party,
		Algorithm:    transactionEndorsement.Algorithm,
		Verifier:     resolvedSigner.Verifier.Verifier,
		VerifierType: transactionEndorsement.VerifierType,
	}

	// Create a throwaway domain context for this call
	dCtx := d.components.StateManager().NewDomainContext(ctx, psc.Domain(), psc.Address())
	defer dCtx.Close()
	endorsementResult, err := psc.EndorseTransaction(dCtx, d.components.Persistence().NOTX(), privateEndorsementRequest)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to endorse transaction: %s", err)
		return
	}
	transactionEndorsement.Payload = endorsementResult.Payload

	attResult := &prototk.AttestationResult{
		Name:            transactionEndorsement.Name,
		AttestationType: transactionEndorsement.AttestationType,
		Verifier:        endorsementResult.Endorser,
	}

	switch endorsementResult.Result {
	case prototk.EndorseTransactionResponse_SIGN:
		//log.L(ctx).Infof("[Sequencer] endorsement response resulted in signing request for transaction %s", endorsementRequest.TransactionId)

		//log.L(ctx).Infof("[Sequencer] endorsement response, checking if the endorser of this endorsement request '%s'is us so we can sign and submit TX %s", endorseRes.Endorser.Lookup, endorsementRequest.TransactionId)
		unqualifiedLookup, signerNode, err := pldtypes.PrivateIdentityLocator(endorsementResult.Endorser.Lookup).Validate(ctx, d.nodeName, true)
		if err != nil {
			log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to validate endorser: %s", err)
			return
		}
		if signerNode == d.nodeName {

			log.L(ctx).Info("[Sequencer] endorsement response signing request includes us - signing it now")
			keyMgr := d.components.KeyManager()
			resolvedKey, err := keyMgr.ResolveKeyNewDatabaseTX(ctx, unqualifiedLookup, transactionEndorsement.Algorithm, transactionEndorsement.VerifierType)
			if err != nil {
				log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to resolve key for endorser: %s", err)
				return
			}

			signaturePayload, err := keyMgr.Sign(ctx, resolvedKey, transactionEndorsement.PayloadType, transactionEndorsement.Payload)
			if err != nil {
				log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to sign endorsement request: %s", err)
				return
			}
			attResult.Payload = signaturePayload
			//log.L(ctx).Debugf("[Sequencer] handleEndorsementRequest payload %x signed %x by %s (%s)", transactionEndorsement.Payload, signaturePayload, unqualifiedLookup, resolvedKey.Verifier.Verifier)
			attResult.Constraints = append(attResult.Constraints, prototk.AttestationResult_ENDORSER_MUST_SUBMIT)
			// MRW TODO - is this manual extrapolation of TX ID from 32 char hex string required?

		}
	}

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to obtain sequencer to pass endorsement event %v:", err)
		return
	}
	if seq == nil {
		log.L(ctx).Errorf("[Sequencer] no sequencer found for contract %s", contractAddress.String())
		return
	}

	d.metrics.IncEndorsedTransactions()
	seq.GetTransportWriter().SendEndorsementResponse(ctx, endorsementRequest.TransactionId, endorsementRequest.IdempotencyKey, contractAddress.String(), attResult, endorsementResult, transactionEndorsement.Name, endorsementRequest.Party, message.FromNode)
}

func (d *distributedSequencerManager) handleEndorsementResponse(ctx context.Context, message *components.ReceivedMessage) {
	endorsementResponse := &engineProto.EndorsementResponse{}
	err := proto.Unmarshal(message.Payload, endorsementResponse)
	if err != nil {
		d.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := d.parseContractAddressString(ctx, endorsementResponse.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	endorsement := &prototk.AttestationResult{}
	err = proto.Unmarshal(endorsementResponse.Endorsement.Value, endorsement)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] handleEndorsementResponse failed to unmarshal endorsement: %s", err)
		return
	}

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to obtain sequencer to pass endorsement event %v:", err)
		return
	}
	if seq == nil {
		log.L(ctx).Errorf("[Sequencer] no sequencer found for contract %s", contractAddress.String())
		return
	}

	endorsementResponseEvent := &coordTransaction.EndorsedEvent{}
	endorsementResponseEvent.TransactionID = uuid.MustParse(endorsementResponse.TransactionId)
	endorsementResponseEvent.RequestID = uuid.MustParse(endorsementResponse.IdempotencyKey)
	endorsementResponseEvent.Endorsement = endorsement
	endorsementResponseEvent.EventTime = time.Now()

	seq.GetCoordinator().HandleEvent(ctx, endorsementResponseEvent)
}

func (d *distributedSequencerManager) handleNonceAssigned(ctx context.Context, message *components.ReceivedMessage) {
	nonceAssigned := &engineProto.NonceAssigned{}
	err := proto.Unmarshal(message.Payload, nonceAssigned)
	if err != nil {
		d.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := d.parseContractAddressString(ctx, nonceAssigned.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] handleNonceAssignedEvent failed to obtain sequencer to pass nonce assigned event %v:", err)
		return
	}
	if seq == nil {
		log.L(ctx).Errorf("[Sequencer] no sequencer found for contract %s", contractAddress.String())
		return
	}

	nonceAssignedEvent := &senderTransaction.NonceAssignedEvent{}
	nonceAssignedEvent.TransactionID = uuid.MustParse(nonceAssigned.TransactionId)
	nonceAssignedEvent.Nonce = uint64(nonceAssigned.Nonce)
	nonceAssignedEvent.EventTime = time.Now()

	seq.GetSender().HandleEvent(ctx, nonceAssignedEvent)
}

func (d *distributedSequencerManager) handleTransactionSubmitted(ctx context.Context, message *components.ReceivedMessage) {
	transactionSubmitted := &engineProto.TransactionSubmitted{}
	err := proto.Unmarshal(message.Payload, transactionSubmitted)
	if err != nil {
		d.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := d.parseContractAddressString(ctx, transactionSubmitted.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] handleTransactionSubmitted failed to obtain sequencer to pass transaction submitted event %v:", err)
		return
	}
	if seq == nil {
		log.L(ctx).Errorf("[Sequencer] no sequencer found for contract %s", contractAddress.String())
		return
	}

	transactionSubmittedEvent := &senderTransaction.SubmittedEvent{}
	transactionSubmittedEvent.TransactionID = uuid.MustParse(transactionSubmitted.TransactionId)
	transactionSubmittedEvent.LatestSubmissionHash = pldtypes.Bytes32(transactionSubmitted.Hash)
	transactionSubmittedEvent.EventTime = time.Now()

	seq.GetSender().HandleEvent(ctx, transactionSubmittedEvent)
}

func (d *distributedSequencerManager) handleTransactionConfirmed(ctx context.Context, message *components.ReceivedMessage) {
	transactionConfirmed := &engineProto.TransactionConfirmed{}
	err := proto.Unmarshal(message.Payload, transactionConfirmed)
	if err != nil {
		d.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := d.parseContractAddressString(ctx, transactionConfirmed.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] handleTransactionSubmitted failed to obtain sequencer to pass transaction submitted event %v:", err)
		return
	}
	if seq == nil {
		log.L(ctx).Errorf("[Sequencer] no sequencer found for contract %s", contractAddress.String())
		return
	}

	if transactionConfirmed.RevertReason != nil {
		transactionSubmittedEvent := &senderTransaction.ConfirmedRevertedEvent{}
		transactionSubmittedEvent.TransactionID = uuid.MustParse(transactionConfirmed.TransactionId)
		transactionSubmittedEvent.RevertReason = transactionConfirmed.RevertReason
		transactionSubmittedEvent.EventTime = time.Now()
		seq.GetSender().HandleEvent(ctx, transactionSubmittedEvent)
	} else {
		transactionSubmittedEvent := &senderTransaction.ConfirmedSuccessEvent{}
		transactionSubmittedEvent.TransactionID = uuid.MustParse(transactionConfirmed.TransactionId)
		transactionSubmittedEvent.EventTime = time.Now()
		seq.GetSender().HandleEvent(ctx, transactionSubmittedEvent)
	}
}
