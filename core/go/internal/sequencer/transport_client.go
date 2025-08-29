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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator"
	coordTransaction "github.com/kaleido-io/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/kaleido-io/paladin/core/internal/sequencer/sender"
	"github.com/kaleido-io/paladin/core/internal/sequencer/sender/transaction"
	"github.com/kaleido-io/paladin/core/internal/sequencer/transport"
	engineProto "github.com/kaleido-io/paladin/core/pkg/proto/engine"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
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
	case transport.MessageType_DispatchConfirmationRequest:
		go d.handleDispatchConfirmationRequest(d.ctx, message)
	case transport.MessageType_EndorsementRequest:
		go d.handleEndorsementRequest(d.ctx, message)
	case transport.MessageType_EndorsementResponse:
		go d.handleEndorsementResponse(d.ctx, message)
	default:
		log.L(ctx).Errorf("Unknown message type: %s", message.MessageType)
	}
}

func (d *distributedSequencerManager) logPaladinMessage(ctx context.Context, message *components.ReceivedMessage) {
	log.L(ctx).Debugf("[Sequencer] << proto message %s received from %s", message.MessageType, message.FromNode)
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

	contractAddress := d.parseContractAddressString(ctx, assembleRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] failed to obtain sequencer to pass assemble request event to %v:", err)
		return
	}

	assembleRequestEvent := &transaction.AssembleRequestReceivedEvent{}
	assembleRequestEvent.TransactionID = uuid.MustParse(assembleRequest.TransactionId)
	assembleRequestEvent.RequestID = uuid.MustParse(assembleRequest.AssembleRequestId)
	assembleRequestEvent.Coordinator = seq.GetCoordinator().GetActiveCoordinatorNode(ctx)
	assembleRequestEvent.CoordinatorsBlockHeight = assembleRequest.BlockHeight
	assembleRequestEvent.StateLocksJSON = assembleRequest.StateLocks

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

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] failed to obtain sequencer to pass assemble response event %v:", err)
		return
	}

	switch postAssembly.AssemblyResult {
	case prototk.AssembleTransactionResponse_OK:
		log.L(ctx).Infof("[Sequencer] handing off assemble request and sign success event to distributed sequencer coordinator")
		assembleResponseEvent := &coordTransaction.AssembleSuccessEvent{}
		assembleResponseEvent.TransactionID = uuid.MustParse(assembleResponse.TransactionId)
		assembleResponseEvent.RequestID = uuid.MustParse(assembleResponse.AssembleRequestId)
		assembleResponseEvent.PostAssembly = postAssembly
		seq.GetCoordinator().HandleEvent(ctx, assembleResponseEvent)
	case prototk.AssembleTransactionResponse_PARK:
		log.L(ctx).Errorf("[Sequencer] coordinator state machine cannot move from Assembling to Parked")
	case prototk.AssembleTransactionResponse_REVERT:
		log.L(ctx).Infof("[Sequencer] handing off assemble revert event to distributed sequencer coordinator")
		assembleResponseEvent := &coordTransaction.AssembleRevertResponseEvent{}
		assembleResponseEvent.TransactionID = uuid.MustParse(assembleResponse.TransactionId)
		assembleResponseEvent.RequestID = uuid.MustParse(assembleResponse.AssembleRequestId)
		assembleResponseEvent.PostAssembly = postAssembly
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

	errorString := assembleError.ErrorMessage
	log.L(ctx).Infof("[Sequencer] assemble error for TX %s: %s", assembleError.TransactionId, errorString)

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] failed to obtain sequencer to pass assemble error event %v:", err)
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

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] failed to obtain sequencer to pass heartbeat event to %v:", err)
		return
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

	dispatchConfirmedEvent := &coordTransaction.DispatchConfirmedEvent{}
	dispatchConfirmedEvent.TransactionID = uuid.MustParse(dispatchConfirmationRequest.TransactionId[2:34])
	dispatchConfirmedEvent.RequestID = uuid.MustParse(dispatchConfirmationRequest.Id)

	seq.GetCoordinator().HandleEvent(ctx, dispatchConfirmedEvent)
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

	transactionDelegatedEvent := &coordinator.TransactionsDelegatedEvent{}
	log.L(d.ctx).Debugf("[Sequencer] handleDelegationRequest transaction specific from: %s", privateTransaction.PreAssembly.TransactionSpecification.From)
	transactionDelegatedEvent.Sender = privateTransaction.PreAssembly.TransactionSpecification.From

	transactionDelegatedEvent.Transactions = append(transactionDelegatedEvent.Transactions, privateTransaction)
	transactionDelegatedEvent.SendersBlockHeight = uint64(d.blockHeight) // MRW TODO - which is the senders block height here?

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] handleDelegationRequest failed to obtain sequencer to pass endorsement event %v:", err)
		return
	}

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
	endorseRes, err := psc.EndorseTransaction(dCtx, d.components.Persistence().NOTX(), privateEndorsementRequest)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to endorse transaction: %s", err)
		return
	}
	transactionEndorsement.Payload = endorseRes.Payload

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to obtain sequencer to pass endorsement event %v:", err)
		return
	}

	result := &prototk.AttestationResult{
		Name:            transactionEndorsement.Name,
		AttestationType: transactionEndorsement.AttestationType,
		Verifier:        endorseRes.Endorser,
	}
	switch endorseRes.Result {
	case prototk.EndorseTransactionResponse_REVERT:
		revertReason := "(no revert reason)"
		if endorseRes.RevertReason != nil {
			revertReason = *endorseRes.RevertReason
		}
		log.L(ctx).Infof("[Sequencer] endorsement response for transaction %s - %s rejected: %s", endorsementRequest.TransactionId, endorsementRequest.Party, revertReason)
		endorseRejectedEvent := &coordTransaction.EndorsedRejectedEvent{}
		endorseRejectedEvent.RevertReason = revertReason
		endorseRejectedEvent.Party = endorsementRequest.Party
		endorseRejectedEvent.AttestationRequestName = transactionEndorsement.Name
		endorseRejectedEvent.RequestID = uuid.MustParse(endorsementRequest.IdempotencyKey)
		seq.GetCoordinator().HandleEvent(ctx, endorseRejectedEvent)
	case prototk.EndorseTransactionResponse_SIGN:
		log.L(ctx).Infof("[Sequencer] endorsement response resulted in signing request for transaction %s", endorsementRequest.TransactionId)

		log.L(ctx).Infof("[Sequencer] endorsement response, checking if the endorser of this endorsement request '%s'is us so we can sign and submit TX %s", endorseRes.Endorser.Lookup, endorsementRequest.TransactionId)
		unqualifiedLookup, signerNode, err := pldtypes.PrivateIdentityLocator(endorseRes.Endorser.Lookup).Validate(ctx, d.nodeName, true)
		if err != nil {
			log.L(ctx).Errorf("handleEndorsementRequest failed to validate identity locator for signing party %s: %s", endorseRes.Endorser.Lookup, err)
			return
		}
		if signerNode == d.nodeName {

			log.L(ctx).Info("[Sequencer] endorsement response signing request includes us - signing it now")
			keyMgr := d.components.KeyManager()
			resolvedKey, err := keyMgr.ResolveKeyNewDatabaseTX(ctx, unqualifiedLookup, transactionEndorsement.Algorithm, transactionEndorsement.VerifierType)
			if err != nil {
				log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to resolve local signer for %s (algorithm=%s): %s", unqualifiedLookup, transactionEndorsement.Algorithm, err)
				return
			}

			signaturePayload, err := keyMgr.Sign(ctx, resolvedKey, transactionEndorsement.PayloadType, transactionEndorsement.Payload)
			if err != nil {
				log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to sign for party %s (verifier=%s,algorithm=%s): %s", unqualifiedLookup, resolvedKey.Verifier.Verifier, transactionEndorsement.Algorithm, err)
				return
			}
			result.Payload = signaturePayload
			log.L(ctx).Debugf("[Sequencer] handleEndorsementRequest payload %x signed %x by %s (%s)", transactionEndorsement.Payload, signaturePayload, unqualifiedLookup, resolvedKey.Verifier.Verifier)

			result.Constraints = append(result.Constraints, prototk.AttestationResult_ENDORSER_MUST_SUBMIT)
			endorsedEvent := &coordTransaction.EndorsedEvent{}

			// MRW TODO - is this manual extrapolation of TX ID from 32 char hex string required?
			endorsedEvent.TransactionID = uuid.MustParse(endorsementRequest.TransactionId[2:34])
			endorsedEvent.RequestID = uuid.MustParse(endorsementRequest.IdempotencyKey)
			endorsedEvent.Endorsement = result
			seq.GetCoordinator().HandleEvent(ctx, endorsedEvent)
		} else {
			log.L(ctx).Warnf("[Sequencer] handleEndorsementRequest ignoring endorsement for TX %s from remote verifier %s ", endorsementRequest.TransactionId, endorseRes.Endorser.Lookup)
		}
	case prototk.EndorseTransactionResponse_ENDORSER_SUBMIT:
		log.L(ctx).Infof("[Sequencer] endorsement response - submit it")
		result.Constraints = append(result.Constraints, prototk.AttestationResult_ENDORSER_MUST_SUBMIT)
		endorsedEvent := &coordTransaction.EndorsedEvent{}
		endorsedEvent.TransactionID = uuid.MustParse(endorsementRequest.TransactionId[2:34])
		endorsedEvent.RequestID = uuid.MustParse(endorsementRequest.IdempotencyKey)
		endorsedEvent.Endorsement = result
		seq.GetCoordinator().HandleEvent(ctx, endorsedEvent)
	default:
		log.L(ctx).Infof("[Sequencer] endorsement response - default result, raising endorses event")
		endorsedEvent := &coordTransaction.EndorsedEvent{}
		endorsedEvent.TransactionID = uuid.MustParse(endorsementRequest.TransactionId[2:34])
		endorsedEvent.RequestID = uuid.MustParse(endorsementRequest.IdempotencyKey)
		endorsedEvent.Endorsement = result
		seq.GetCoordinator().HandleEvent(ctx, endorsedEvent)
	}
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

	endorsementResponseEvent := &coordTransaction.EndorsedEvent{}
	endorsementResponseEvent.TransactionID = uuid.MustParse(endorsementResponse.TransactionId)
	endorsementResponseEvent.RequestID = uuid.MustParse(endorsementResponse.IdempotencyKey)
	endorsementResponseEvent.Endorsement = endorsement

	seq, err := d.LoadSequencer(ctx, d.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] handleEndorsementRequest failed to obtain sequencer to pass endorsement event %v:", err)
		return
	}

	seq.GetCoordinator().HandleEvent(ctx, endorsementResponseEvent)

}
