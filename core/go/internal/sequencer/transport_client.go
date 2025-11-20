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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator"
	coordTransaction "github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator"
	originatorTransaction "github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"google.golang.org/protobuf/proto"
)

func (sMgr *sequencerManager) HandlePaladinMsg(ctx context.Context, message *components.ReceivedMessage) {
	//TODO this need to become an ultra low latency, non blocking, handover to the event loop thread.
	// need some thought on how to handle errors, retries, buffering, swapping idle sequencers in and out of memory etc...

	sMgr.logPaladinMessage(ctx, message)

	//Send the event to the sequencer handler
	switch message.MessageType {
	case transport.MessageType_AssembleRequest:
		go sMgr.handleAssembleRequest(sMgr.ctx, message)
	case transport.MessageType_AssembleResponse:
		go sMgr.handleAssembleResponse(sMgr.ctx, message)
	case transport.MessageType_AssembleError:
		go sMgr.handleAssembleError(sMgr.ctx, message)
	case transport.MessageType_CoordinatorHeartbeatNotification:
		go sMgr.handleCoordinatorHeartbeatNotification(sMgr.ctx, message)
	case transport.MessageType_DelegationRequest:
		go sMgr.handleDelegationRequest(sMgr.ctx, message)
	case transport.MessageType_DelegationRequestAcknowledgment:
		go sMgr.handleDelegationRequestAcknowledgment(sMgr.ctx, message)
	case transport.MessageType_Dispatched:
		go sMgr.handleDispatchedEvent(sMgr.ctx, message)
	case transport.MessageType_HandoverRequest:
		go sMgr.handleHandoverRequest(sMgr.ctx, message)
	case transport.MessageType_PreDispatchRequest:
		go sMgr.handlePreDispatchRequest(sMgr.ctx, message)
	case transport.MessageType_PreDispatchResponse:
		go sMgr.handlePreDispatchResponse(sMgr.ctx, message)
	case transport.MessageType_EndorsementRequest:
		go sMgr.handleEndorsementRequest(sMgr.ctx, message)
	case transport.MessageType_EndorsementResponse:
		go sMgr.handleEndorsementResponse(sMgr.ctx, message)
	case transport.MessageType_NonceAssigned:
		go sMgr.handleNonceAssigned(sMgr.ctx, message)
	case transport.MessageType_TransactionSubmitted:
		go sMgr.handleTransactionSubmitted(sMgr.ctx, message)
	case transport.MessageType_TransactionConfirmed:
		go sMgr.handleTransactionConfirmed(sMgr.ctx, message)
	default:
		log.L(ctx).Errorf("Unknown message type: %s", message.MessageType)
	}
}

func (sMgr *sequencerManager) logPaladinMessage(ctx context.Context, message *components.ReceivedMessage) {
	log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_MSGRX)).Debugf("%+v received from %s", message.MessageType, message.FromNode)
}

func (sMgr *sequencerManager) logPaladinMessageUnmarshalError(ctx context.Context, message *components.ReceivedMessage, err error) {
	log.L(ctx).Errorf("<< ERROR unmarshalling proto message%s from %s: %s", message.MessageType, message.FromNode, err)
}

func (sMgr *sequencerManager) logPaladinMessageFieldMissingError(ctx context.Context, message *components.ReceivedMessage, field string) {
	log.L(ctx).Errorf("<< field %s missing from proto message %s received from %s", field, message.MessageType, message.FromNode)
}

func (sMgr *sequencerManager) logPaladinMessageJsonUnmarshalError(ctx context.Context, jsonObject string, message *components.ReceivedMessage, err error) {
	log.L(ctx).Errorf("<< ERROR unmarshalling JSON object %s from proto message %s (received from %s): %s", jsonObject, message.MessageType, message.FromNode, err)
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

	preAssembly := &components.TransactionPreAssembly{}
	err = json.Unmarshal(assembleRequest.PreAssembly, preAssembly)
	if err != nil {
		sMgr.logPaladinMessageJsonUnmarshalError(ctx, "TransactionPreAssembly", message, err)
		return
	}
	log.L(ctx).Infof("handling assemble request with %d required verifiers, %d verifiers", len(preAssembly.RequiredVerifiers), len(preAssembly.Verifiers))

	contractAddress := sMgr.parseContractAddressString(ctx, assembleRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("failed to obtain sequencer to pass assemble request event to %v:", err)
		return
	}

	assembleRequestEvent := &originatorTransaction.AssembleRequestReceivedEvent{}
	assembleRequestEvent.TransactionID = uuid.MustParse(assembleRequest.TransactionId)
	assembleRequestEvent.RequestID = uuid.MustParse(assembleRequest.AssembleRequestId)
	assembleRequestEvent.Coordinator = seq.GetCoordinator().GetActiveCoordinatorNode(ctx, true)
	assembleRequestEvent.CoordinatorsBlockHeight = assembleRequest.BlockHeight
	assembleRequestEvent.StateLocksJSON = assembleRequest.StateLocks
	assembleRequestEvent.PreAssembly = assembleRequest.PreAssembly
	assembleRequestEvent.EventTime = time.Now()

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

	postAssembly := &components.TransactionPostAssembly{}
	err = json.Unmarshal(assembleResponse.PostAssembly, postAssembly)
	if err != nil {
		sMgr.logPaladinMessageJsonUnmarshalError(ctx, "TransactionPostAssembly", message, err)
		return
	}

	preAssembly := &components.TransactionPreAssembly{}
	err = json.Unmarshal(assembleResponse.PreAssembly, preAssembly)
	if err != nil {
		sMgr.logPaladinMessageJsonUnmarshalError(ctx, "TransactionPreAssembly", message, err)
		return
	}

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("failed to obtain sequencer to pass assemble response event %v:", err)
		return
	}

	switch postAssembly.AssemblyResult {
	case prototk.AssembleTransactionResponse_OK:
		assembleResponseEvent := &coordTransaction.AssembleSuccessEvent{}
		assembleResponseEvent.TransactionID = uuid.MustParse(assembleResponse.TransactionId)
		assembleResponseEvent.RequestID = uuid.MustParse(assembleResponse.AssembleRequestId)
		assembleResponseEvent.PostAssembly = postAssembly
		assembleResponseEvent.PreAssembly = preAssembly
		assembleResponseEvent.EventTime = time.Now()
		seq.GetCoordinator().QueueEvent(ctx, assembleResponseEvent)
	case prototk.AssembleTransactionResponse_PARK:
		log.L(ctx).Errorf("coordinator state machine cannot move from Assembling to Parked")
	case prototk.AssembleTransactionResponse_REVERT:
		assembleResponseEvent := &coordTransaction.AssembleRevertResponseEvent{}
		assembleResponseEvent.TransactionID = uuid.MustParse(assembleResponse.TransactionId)
		assembleResponseEvent.RequestID = uuid.MustParse(assembleResponse.AssembleRequestId)
		assembleResponseEvent.PostAssembly = postAssembly
		assembleResponseEvent.EventTime = time.Now()
		seq.GetCoordinator().QueueEvent(ctx, assembleResponseEvent)
	default:
		log.L(ctx).Errorf("received unexpected assemble response type %s", postAssembly.AssemblyResult)
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

	assembleErrorEvent := &originatorTransaction.AssembleErrorEvent{}
	assembleErrorEvent.TransactionID = uuid.MustParse(assembleError.TransactionId)
	assembleErrorEvent.EventTime = time.Now()

	errorString := assembleError.ErrorMessage
	log.L(ctx).Debugf("assemble error for TX %s: %s", assembleError.TransactionId, errorString)

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("failed to obtain sequencer to pass assemble error event %v:", err)
		return
	}

	seq.GetCoordinator().QueueEvent(ctx, assembleErrorEvent)
}

func (sMgr *sequencerManager) handleCoordinatorHeartbeatNotification(ctx context.Context, message *components.ReceivedMessage) {
	heartbeatNotification := &engineProto.CoordinatorHeartbeatNotification{}
	err := proto.Unmarshal(message.Payload, heartbeatNotification)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	from := heartbeatNotification.From
	if from == "" {
		sMgr.logPaladinMessageFieldMissingError(ctx, message, "From")
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, heartbeatNotification.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	coordinatorSnapshot := &common.CoordinatorSnapshot{}
	err = json.Unmarshal(heartbeatNotification.CoordinatorSnapshot, coordinatorSnapshot)
	if err != nil {
		sMgr.logPaladinMessageJsonUnmarshalError(ctx, "CoordinatorSnapshot", message, err)
		return
	}

	heartbeatEvent := &originator.HeartbeatReceivedEvent{}
	heartbeatEvent.From = from
	heartbeatEvent.ContractAddress = contractAddress
	heartbeatEvent.CoordinatorSnapshot = *coordinatorSnapshot
	heartbeatEvent.EventTime = time.Now()

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("failed to obtain sequencer to pass heartbeat event to %v:", err)
		return
	}

	for _, transaction := range coordinatorSnapshot.ConfirmedTransactions {
		log.L(ctx).Debugf("received a heartbeat containing a confirmed transaction: %s", transaction.ID.String())
		heartbeatIntervalEvent := &coordTransaction.HeartbeatIntervalEvent{}
		heartbeatIntervalEvent.TransactionID = transaction.ID
		heartbeatIntervalEvent.EventTime = time.Now()
		seq.GetCoordinator().QueueEvent(ctx, heartbeatIntervalEvent)
	}

	seq.GetOriginator().QueueEvent(ctx, heartbeatEvent)
}

func (sMgr *sequencerManager) handleHandoverRequest(ctx context.Context, message *components.ReceivedMessage) {

	handoverRequest := &engineProto.HandoverRequest{}
	err := proto.Unmarshal(message.Payload, handoverRequest)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, handoverRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("failed to obtain sequencer to pass handover request event to %v:", err)
		return
	}

	handoverRequestEvent := &coordinator.HandoverRequestEvent{}
	handoverRequestEvent.Requester = message.FromNode
	handoverRequestEvent.EventTime = time.Now()

	seq.GetCoordinator().QueueEvent(ctx, handoverRequestEvent)
}

func (sMgr *sequencerManager) handlePreDispatchRequest(ctx context.Context, message *components.ReceivedMessage) {
	preDispatchRequest := &engineProto.TransactionDispatched{}

	err := proto.Unmarshal(message.Payload, preDispatchRequest)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, preDispatchRequest.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("failed to obtain sequencer to pass pre dispatch event to %v:", err)
		return
	}

	postAssemblyHash := pldtypes.NewBytes32FromSlice(preDispatchRequest.PostAssembleHash)

	preDispatchRequestReceivedEvent := &originatorTransaction.PreDispatchRequestReceivedEvent{
		RequestID:        uuid.MustParse(preDispatchRequest.Id),
		Coordinator:      message.FromNode,
		PostAssemblyHash: &postAssemblyHash,
	}
	preDispatchRequestReceivedEvent.TransactionID = uuid.MustParse(preDispatchRequest.TransactionId[2:34])
	preDispatchRequestReceivedEvent.EventTime = time.Now()

	// TODO - not sure where we should make the decision as to whether or not to approve dispatch.
	// For now we just proceed and send an approval response. It's possible that the check belongs in the state machine
	// validator function for PreDispatchRequestReceivedEvent?

	seq.GetOriginator().QueueEvent(ctx, preDispatchRequestReceivedEvent)
}

func (sMgr *sequencerManager) handlePreDispatchResponse(ctx context.Context, message *components.ReceivedMessage) {
	preDispatchResponse := &engineProto.TransactionDispatched{}

	err := proto.Unmarshal(message.Payload, preDispatchResponse)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, preDispatchResponse.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("failed to obtain sequencer to pass pre dispatch event to %v:", err)
		return
	}

	// MRW TODO - we don't yet return anything other than approved.

	dispatchRequestApprovedEvent := &coordTransaction.DispatchRequestApprovedEvent{
		RequestID: uuid.MustParse(preDispatchResponse.Id),
	}
	dispatchRequestApprovedEvent.TransactionID = uuid.MustParse(preDispatchResponse.TransactionId[2:34])
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

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("failed to obtain sequencer to pass dispatch confirmation event to %v:", err)
		return
	}

	dispatchConfirmedEvent := &originatorTransaction.DispatchedEvent{}
	dispatchConfirmedEvent.TransactionID = uuid.MustParse(dispatchedEvent.TransactionId[2:34])
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

	privateTransaction := &components.PrivateTransaction{}
	err = json.Unmarshal(delegationRequest.PrivateTransaction, privateTransaction)
	if err != nil {
		sMgr.logPaladinMessageJsonUnmarshalError(ctx, "PrivateTransaction", message, err)
		return
	}

	contractAddress := sMgr.parseContractAddressString(ctx, privateTransaction.PreAssembly.TransactionSpecification.ContractInfo.ContractAddress, message)
	if contractAddress == nil {
		return
	}

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("failed to obtain sequencer to pass endorsement event %v:", err)
		return
	}

	transactionDelegatedEvent := &coordinator.TransactionsDelegatedEvent{}
	transactionDelegatedEvent.Originator = privateTransaction.PreAssembly.TransactionSpecification.From
	transactionDelegatedEvent.Transactions = append(transactionDelegatedEvent.Transactions, privateTransaction)
	transactionDelegatedEvent.OriginatorsBlockHeight = uint64(delegationRequest.BlockHeight)
	transactionDelegatedEvent.EventTime = time.Now()

	// Anyone who delegates a transaction to us is a candidate originator and should be sent heartbeats for TX confirmation processing
	seq.GetCoordinator().UpdateOriginatorNodePool(ctx, message.FromNode)

	seq.GetCoordinator().QueueEvent(ctx, transactionDelegatedEvent)
}

func (sMgr *sequencerManager) handleDelegationRequestAcknowledgment(ctx context.Context, message *components.ReceivedMessage) {
	delegationRequestAcknowledgment := &engineProto.DelegationRequestAcknowledgment{}
	err := proto.Unmarshal(message.Payload, delegationRequestAcknowledgment)
	if err != nil {
		sMgr.logPaladinMessageUnmarshalError(ctx, message, err)
		return
	}

	log.L(ctx).Debugf("delegationRequestAcknowledgment received for transaction ID %s", delegationRequestAcknowledgment.TransactionId)
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

	psc, err := sMgr.components.DomainManager().GetSmartContractByAddress(ctx, sMgr.components.Persistence().NOTX(), *contractAddress)
	if err != nil {
		return
	}

	transactionSpecification := &prototk.TransactionSpecification{}
	err = proto.Unmarshal(endorsementRequest.TransactionSpecification.Value, transactionSpecification)
	if err != nil {
		return
	}

	// If this TX ID doesn't exist in the "transactions" DB, insert here.
	theUUID := pldtypes.MustParseBytes32(transactionSpecification.TransactionId).UUIDFirst16()
	txID := theUUID
	if err != nil {
		log.L(ctx).Errorf("failed to parse transaction ID %s: %s", transactionSpecification.TransactionId, err)
		return
	}
	tx, err := sMgr.components.TxManager().GetTransactionByID(ctx, txID)
	if err != nil {
		log.L(ctx).Errorf("failed to get transaction %s from the 'transactions' DB: %s", transactionSpecification.TransactionId, err)
		return
	}
	if tx == nil {
		var functionABI abi.Entry // MRW TODO - parse function sig into ABI entry?
		err = json.Unmarshal([]byte(transactionSpecification.FunctionAbiJson), &functionABI)
		if err != nil {
			log.L(ctx).Errorf("failed to unmarshall function abi: %s", err)
			return
		}
		functionSig, err := functionABI.Signature()
		if err != nil {
			log.L(ctx).Errorf("failed to get signature for function ABI: %s", err)
			return
		}

		// Create pldtypes ethaddress from string
		fn, _, _, err := sMgr.components.TxManager().ResolveTransactionInputs(ctx, sMgr.components.Persistence().NOTX(), &pldapi.TransactionInput{
			ABI: abi.ABI{&functionABI},
			TransactionBase: pldapi.TransactionBase{
				Domain:   psc.Domain().Name(),
				From:     transactionSpecification.From,
				Function: functionSig,
				To:       pldtypes.MustEthAddress(transactionSpecification.ContractInfo.ContractAddress),
				Type:     pldapi.TransactionTypePrivate.Enum(),
				Data:     pldtypes.RawJSON(transactionSpecification.FunctionParamsJson),
			},
		})
		if err != nil {
			log.L(ctx).Errorf("failed to resolve transaction inputs: %s", err)
			return
		}
		log.L(ctx).Infof("transaction %s not found in the 'transactions' DB, inserting it", transactionSpecification.TransactionId)
		err = sMgr.components.Persistence().Transaction(ctx, func(ctx context.Context, dbTx persistence.DBTX) error {
			validatedTransaction := &components.ValidatedTransaction{
				ResolvedTransaction: components.ResolvedTransaction{
					Function: fn,
					Transaction: &pldapi.Transaction{
						ID: &txID,
						TransactionBase: pldapi.TransactionBase{
							From:         transactionSpecification.From,
							Function:     transactionSpecification.FunctionSignature,
							ABIReference: fn.ABIReference,
							To:           pldtypes.MustEthAddress(transactionSpecification.ContractInfo.ContractAddress),
							Domain:       psc.Domain().Name(),
							Type:         pldapi.TransactionTypePrivate.Enum(),
							Data:         pldtypes.RawJSON(transactionSpecification.FunctionParamsJson),
						},
					},
				},
			}
			_, err := sMgr.components.TxManager().InsertRemoteTransaction(ctx, dbTx, validatedTransaction, false)
			// If this fails we reject the request to endorse. We need to persist the remote TX. A future endorsement request will eventually be received again
			return err
		})
		if err != nil {
			log.L(ctx).Errorf("failed to insert transaction %s into the 'transactions' DB: %s", transactionSpecification.TransactionId, err)
			return
		}
	}

	transactionVerifiers := make([]*prototk.ResolvedVerifier, len(endorsementRequest.Verifiers))
	for i, v := range endorsementRequest.Verifiers {
		nextVerifier := &prototk.ResolvedVerifier{}
		err = proto.Unmarshal(v.Value, nextVerifier)
		if err != nil {
			log.L(ctx).Errorf("failed to unmarshal verifier %s for endorsement request: %s", v.String(), err)
			return
		}
		transactionVerifiers[i] = nextVerifier
	}

	transactionSignatures := make([]*prototk.AttestationResult, len(endorsementRequest.Signatures))
	for i, s := range endorsementRequest.Signatures {
		nextSignature := &prototk.AttestationResult{}
		err = proto.Unmarshal(s.Value, nextSignature)
		if err != nil {
			log.L(ctx).Errorf("failed to unmarshal signature %s for endorsement request: %s", s.String(), err)
			return
		}
		transactionSignatures[i] = nextSignature
	}

	transactionInputStates := make([]*prototk.EndorsableState, len(endorsementRequest.InputStates))
	for i, s := range endorsementRequest.InputStates {
		nextState := &prototk.EndorsableState{}
		err = proto.Unmarshal(s.Value, nextState)
		if err != nil {
			return
		}
		transactionInputStates[i] = nextState
	}

	transactionReadStates := make([]*prototk.EndorsableState, len(endorsementRequest.ReadStates))
	for i, s := range endorsementRequest.ReadStates {
		nextState := &prototk.EndorsableState{}
		err = proto.Unmarshal(s.Value, nextState)
		if err != nil {
			return
		}
		transactionReadStates[i] = nextState
	}

	transactionOutputStates := make([]*prototk.EndorsableState, len(endorsementRequest.OutputStates))
	for i, s := range endorsementRequest.OutputStates {
		nextState := &prototk.EndorsableState{}
		err = proto.Unmarshal(s.Value, nextState)
		if err != nil {
			return
		}
		transactionOutputStates[i] = nextState
	}

	transactionInfoStates := make([]*prototk.EndorsableState, len(endorsementRequest.InfoStates))
	for i, s := range endorsementRequest.InfoStates {
		nextState := &prototk.EndorsableState{}
		err = proto.Unmarshal(s.Value, nextState)
		if err != nil {
			return
		}
		transactionInfoStates[i] = nextState
	}

	transactionEndorsement := &prototk.AttestationRequest{}
	err = proto.Unmarshal(endorsementRequest.AttestationRequest.Value, transactionEndorsement)
	if err != nil {
		return
	}

	unqualifiedLookup, err := pldtypes.PrivateIdentityLocator(endorsementRequest.Party).Identity(ctx)
	if err != nil {
		log.L(ctx).Error(err)
		return
	}

	resolvedSigner, err := sMgr.components.KeyManager().ResolveKeyNewDatabaseTX(ctx, unqualifiedLookup, transactionEndorsement.Algorithm, transactionEndorsement.VerifierType)
	if err != nil {
		errorMessage := fmt.Sprintf("failed to resolve key for party %s", endorsementRequest.Party)
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
	for _, state := range privateEndorsementRequest.InfoStates {
		log.L(ctx).Debugf("private endorsement info state: %+v", state)
	}

	privateEndorsementRequest.Endorsement = transactionEndorsement
	privateEndorsementRequest.Endorser = &prototk.ResolvedVerifier{
		Lookup:       endorsementRequest.Party,
		Algorithm:    transactionEndorsement.Algorithm,
		Verifier:     resolvedSigner.Verifier.Verifier,
		VerifierType: transactionEndorsement.VerifierType,
	}

	// Create a throwaway domain context for this call
	dCtx := sMgr.components.StateManager().NewDomainContext(ctx, psc.Domain(), psc.Address())
	defer dCtx.Close()
	endorsementResult, err := psc.EndorseTransaction(dCtx, sMgr.components.Persistence().NOTX(), privateEndorsementRequest)
	if err != nil {
		log.L(ctx).Errorf("handleEndorsementRequest failed to endorse transaction: %s", err)
		return
	}
	transactionEndorsement.Payload = endorsementResult.Payload

	attResult := &prototk.AttestationResult{
		Name:            transactionEndorsement.Name,
		AttestationType: transactionEndorsement.AttestationType,
		Verifier:        endorsementResult.Endorser,
	}

	revertReason := ""

	switch endorsementResult.Result {
	case prototk.EndorseTransactionResponse_REVERT:
		revertReason = "(no revert reason)"
		if endorsementResult.RevertReason != nil {
			revertReason = *endorsementResult.RevertReason
		}
	case prototk.EndorseTransactionResponse_SIGN:
		unqualifiedLookup, signerNode, err := pldtypes.PrivateIdentityLocator(endorsementResult.Endorser.Lookup).Validate(ctx, sMgr.nodeName, true)
		if err != nil {
			log.L(ctx).Errorf("handleEndorsementRequest failed to validate endorser: %s", err)
			return
		}
		if signerNode == sMgr.nodeName {

			log.L(ctx).Info("endorsement response signing request includes us - signing it now")
			keyMgr := sMgr.components.KeyManager()
			resolvedKey, err := keyMgr.ResolveKeyNewDatabaseTX(ctx, unqualifiedLookup, transactionEndorsement.Algorithm, transactionEndorsement.VerifierType)
			if err != nil {
				log.L(ctx).Errorf("handleEndorsementRequest failed to resolve key for endorser: %s", err)
				return
			}

			signaturePayload, err := keyMgr.Sign(ctx, resolvedKey, transactionEndorsement.PayloadType, transactionEndorsement.Payload)
			if err != nil {
				log.L(ctx).Errorf("handleEndorsementRequest failed to sign endorsement request: %s", err)
				return
			}
			attResult.Payload = signaturePayload

		} else {
			// This can presumably never happen, since this endorsement request came to us
			log.L(ctx).Errorf("handleEndorsementRequest received isn't for this node: %s", signerNode)
		}
	case prototk.EndorseTransactionResponse_ENDORSER_SUBMIT:
		attResult.Constraints = append(attResult.Constraints, prototk.AttestationResult_ENDORSER_MUST_SUBMIT)
	}

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("handleEndorsementRequest failed to obtain sequencer to pass endorsement event %v:", err)
		return
	}

	// Take this opportunity to update the state machine that there is a currently active coordinator. Heartbeats serve the same
	// purpose but are on a set interval, so updating whenever we are given transactions to endorse is a useful optimisation
	seq.GetCoordinator().QueueEvent(ctx, &coordinator.EndorsementRequestedEvent{
		From: message.FromNode,
	})

	sMgr.metrics.IncEndorsedTransactions()
	err = seq.GetTransportWriter().SendEndorsementResponse(ctx, endorsementRequest.TransactionId, endorsementRequest.IdempotencyKey, contractAddress.String(), attResult, endorsementResult, revertReason, transactionEndorsement.Name, endorsementRequest.Party, message.FromNode)
	if err != nil {
		log.L(ctx).Errorf("handleEndorsementRequest failed to send endorsement response: %s", err)
		return
	}
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

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("handleEndorsementRequest failed to obtain sequencer to pass endorsement event %v:", err)
		return
	}

	// Endorsement reverted
	if endorsementResponse.GetRevertReason() != "" {
		endorsementRejectedEvent := &coordTransaction.EndorsedRejectedEvent{}
		endorsementRejectedEvent.TransactionID = uuid.MustParse(endorsementResponse.TransactionId)
		endorsementRejectedEvent.RequestID = uuid.MustParse(endorsementResponse.IdempotencyKey)
		endorsementRejectedEvent.EventTime = time.Now()
		endorsementRejectedEvent.RevertReason = endorsementResponse.GetRevertReason()
		endorsementRejectedEvent.AttestationRequestName = endorsementResponse.AttestationRequestName
		seq.GetCoordinator().QueueEvent(ctx, endorsementRejectedEvent)
		return
	}

	// Endorsement succeeded
	endorsement := &prototk.AttestationResult{}
	err = proto.Unmarshal(endorsementResponse.Endorsement.Value, endorsement)
	if err != nil {
		log.L(ctx).Errorf("handleEndorsementResponse failed to unmarshal endorsement: %s", err)
		return
	}

	endorsementResponseEvent := &coordTransaction.EndorsedEvent{}
	endorsementResponseEvent.TransactionID = uuid.MustParse(endorsementResponse.TransactionId)
	endorsementResponseEvent.RequestID = uuid.MustParse(endorsementResponse.IdempotencyKey)
	endorsementResponseEvent.Endorsement = endorsement
	endorsementResponseEvent.EventTime = time.Now()
	seq.GetCoordinator().QueueEvent(ctx, endorsementResponseEvent)
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

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("handleNonceAssignedEvent failed to obtain sequencer to pass nonce assigned event %v:", err)
		return
	}

	nonceAssignedEvent := &originatorTransaction.NonceAssignedEvent{}
	nonceAssignedEvent.TransactionID = uuid.MustParse(nonceAssigned.TransactionId)
	nonceAssignedEvent.Nonce = uint64(nonceAssigned.Nonce)
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

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("handleTransactionSubmitted failed to obtain sequencer to pass transaction submitted event %v:", err)
		return
	}

	transactionSubmittedEvent := &originatorTransaction.SubmittedEvent{}
	transactionSubmittedEvent.TransactionID = uuid.MustParse(transactionSubmitted.TransactionId)
	transactionSubmittedEvent.LatestSubmissionHash = pldtypes.Bytes32(transactionSubmitted.Hash)
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

	seq, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *contractAddress, nil, nil)
	if seq == nil || err != nil {
		log.L(ctx).Errorf("handleTransactionSubmitted failed to obtain sequencer to pass transaction submitted event %v:", err)
		return
	}

	if transactionConfirmed.RevertReason != nil {
		transactionSubmittedEvent := &originatorTransaction.ConfirmedRevertedEvent{}
		transactionSubmittedEvent.TransactionID = uuid.MustParse(transactionConfirmed.TransactionId)
		transactionSubmittedEvent.RevertReason = transactionConfirmed.RevertReason
		transactionSubmittedEvent.EventTime = time.Now()
		seq.GetOriginator().QueueEvent(ctx, transactionSubmittedEvent)
	} else {
		transactionSubmittedEvent := &originatorTransaction.ConfirmedSuccessEvent{}
		transactionSubmittedEvent.TransactionID = uuid.MustParse(transactionConfirmed.TransactionId)
		transactionSubmittedEvent.EventTime = time.Now()
		seq.GetOriginator().QueueEvent(ctx, transactionSubmittedEvent)
	}
}
