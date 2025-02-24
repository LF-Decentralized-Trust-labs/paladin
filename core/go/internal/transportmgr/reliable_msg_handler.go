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

package transportmgr

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/flushwriter"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

const (
	RMHMessageTypeAck                 = "ack"
	RMHMessageTypeNack                = "nack"
	RMHMessageTypeStateDistribution   = string(components.RMTState)
	RMHMessageTypeReceipt             = string(components.RMTReceipt)
	RMHMessageTypePreparedTransaction = string(components.RMTPreparedTransaction)
	RMHMessageTypePrivacyGroup        = string(components.RMTPrivacyGroup)
	RMHMessageTypePrivacyGroupMessage = string(components.RMTPrivacyGroupMessage)
)

type reliableMsgOp struct {
	p   *peer
	msg *components.ReceivedMessage
}

func (op *reliableMsgOp) WriteKey() string {
	return op.p.Name
}

type noResult struct{}

type ackInfo struct {
	node  string
	id    uuid.UUID // sent in CID on wire
	Error string    `json:"error,omitempty"`
}

type stateAndAck struct {
	state *components.StateUpsertOutsideContext
	ack   *ackInfo
}

func (tm *transportManager) handleReliableMsgBatch(ctx context.Context, dbTX persistence.DBTX, values []*reliableMsgOp) ([]flushwriter.Result[*noResult], error) {

	var acksToWrite []*components.ReliableMessageAck
	var acksToSend []*ackInfo
	statesToAdd := make(map[string][]*stateAndAck)
	abisToAdd := make(map[string][]*abi.Parameter)
	nullifierUpserts := make(map[string][]*components.NullifierUpsert)
	var preparedTxnToAdd []*components.PreparedTransactionWithRefs
	var txReceiptsToFinalize []*components.ReceiptInput
	var msgsToReceive []*pldapi.PrivacyGroupMessage

	dbTX.AddPostCommit(func(ctx context.Context) {
		// We've committed the database work ok - send the acks/nacks to the other side
		for _, a := range acksToSend {
			_ = tm.queueFireAndForget(ctx, a.node, buildAck(a.id, a.Error))
		}
	})

	// The batch can contain different kinds of message that all need persistence activity
	for _, v := range values {

		switch v.msg.MessageType {
		case RMHMessageTypeStateDistribution:
			sd, stateToAdd, err := parseStateDistribution(ctx, v.msg.MessageID, v.msg.Payload)
			if err == nil && sd.NullifierAlgorithm != nil && sd.NullifierVerifierType != nil && sd.NullifierPayloadType != nil {
				// We need to build any nullifiers that are required, before we dispatch to persistence
				var nullifier *components.NullifierUpsert
				nullifier, err = tm.privateTxManager.BuildNullifier(ctx, tm.keyManager.KeyResolverForDBTX(dbTX), sd)
				if err == nil {
					nullifierUpserts[sd.Domain] = append(nullifierUpserts[sd.Domain], nullifier)
				}
			}
			if err != nil {
				acksToSend = append(acksToSend,
					&ackInfo{node: v.p.Name, id: v.msg.MessageID, Error: err.Error()}, // reject the message permanently
				)
			} else {
				statesToAdd[sd.Domain] = append(statesToAdd[sd.Domain], &stateAndAck{
					state: stateToAdd,
					ack:   &ackInfo{node: v.p.Name, id: v.msg.MessageID},
				})
			}
		case RMHMessageTypePrivacyGroup:
			domain, genesisABI, genesisState, err := parsePrivacyGroupDistribution(ctx, v.msg.MessageID, v.msg.Payload)
			if err != nil {
				acksToSend = append(acksToSend,
					&ackInfo{node: v.p.Name, id: v.msg.MessageID, Error: err.Error()}, // reject the message permanently
				)
			} else {
				abisToAdd[domain] = append(abisToAdd[domain], genesisABI)
				statesToAdd[domain] = append(statesToAdd[domain], &stateAndAck{
					state: genesisState,
					ack:   &ackInfo{node: v.p.Name, id: v.msg.MessageID},
				})
			}
		case RMHMessageTypePrivacyGroupMessage:
			msg, err := parsePrivacyGroupMessage(ctx, v.p.Name, v.msg.MessageID, v.msg.Payload)
			if err != nil {
				acksToSend = append(acksToSend,
					&ackInfo{node: v.p.Name, id: v.msg.MessageID, Error: err.Error()}, // reject the message permanently
				)
			} else {
				msgsToReceive = append(msgsToReceive, msg)
			}
		case RMHMessageTypePreparedTransaction:
			var pt components.PreparedTransactionWithRefs
			err := json.Unmarshal(v.msg.Payload, &pt)
			if err != nil {
				acksToSend = append(acksToSend,
					&ackInfo{node: v.p.Name, id: v.msg.MessageID, Error: err.Error()}, // reject the message permanently
				)
			} else {
				// Build the ack now, as we'll fail the whole TX and not send any acks if the write fails
				acksToSend = append(acksToSend, &ackInfo{node: v.p.Name, id: v.msg.MessageID})
				preparedTxnToAdd = append(preparedTxnToAdd, &pt)
			}
		case RMHMessageTypeReceipt:
			var receipt components.ReceiptInput
			err := json.Unmarshal(v.msg.Payload, &receipt)
			if err != nil {
				acksToSend = append(acksToSend,
					&ackInfo{node: v.p.Name, id: v.msg.MessageID, Error: err.Error()}, // reject the message permanently
				)
			} else {
				// Build the ack now, as we'll fail the whole TX and not send any acks if the write fails
				acksToSend = append(acksToSend, &ackInfo{node: v.p.Name, id: v.msg.MessageID})
				txReceiptsToFinalize = append(txReceiptsToFinalize, &receipt)
			}
		case RMHMessageTypeAck, RMHMessageTypeNack:
			ackNackToWrite := tm.parseReceivedAckNack(ctx, v.msg)
			if ackNackToWrite != nil {
				acksToWrite = append(acksToWrite, ackNackToWrite)
			}
		default:
			err := i18n.NewError(ctx, msgs.MsgTransportUnsupportedReliableMsgType, v.msg.MessageType)
			acksToSend = append(acksToSend,
				&ackInfo{node: v.p.Name, id: v.msg.MessageID, Error: err.Error()}, // reject the message permanently
			)
		}
	}

	for domain, abis := range abisToAdd {
		if _, err := tm.stateManager.EnsureABISchemas(ctx, dbTX, domain, abis); err != nil {
			// We continue and fail on the associated state insertion
			log.L(ctx).Errorf("ensure ABI failed (later state insert failure anticipated): %s", err)
		}
	}

	// Inserting the states is a performance critical activity that we ensure we batch as efficiently as possible,
	// while protecting ourselves from inserting states that we haven't done the local validation of.
	for domain, states := range statesToAdd {
		batchStates := make([]*components.StateUpsertOutsideContext, len(states))
		for i, s := range states {
			batchStates[i] = s.state
		}
		_, batchErr := tm.stateManager.WriteReceivedStates(ctx, dbTX, domain, batchStates)
		if batchErr != nil {
			// We have to try each individually (note if the error was transient in the DB we will rollback
			// the whole transaction and won't send any acks at all - which is good as sender will retry in that case)
			log.L(ctx).Errorf("batch insert of %d states for domain %s failed - attempting each individually: %s", len(states), domain, batchErr)
			for _, s := range states {
				_, err := tm.stateManager.WriteReceivedStates(ctx, dbTX, domain, []*components.StateUpsertOutsideContext{s.state})
				if err != nil {
					log.L(ctx).Errorf("insert state %s from message %s for domain %s failed: %s", s.state.ID, s.ack.id, domain, batchErr)
					s.ack.Error = err.Error()
				}
			}
		}
		for _, s := range states {
			acksToSend = append(acksToSend, s.ack)
		}
	}

	// We can only store acks for messages that are in our DB (due to foreign key relationship),
	// so we have to query them first to validate the acks before attempting insert.
	if len(acksToWrite) > 0 {
		ackQuery := make([]uuid.UUID, len(acksToWrite))
		for i, a := range acksToWrite {
			ackQuery[i] = a.MessageID
		}
		var matchedMsgs []*components.ReliableMessage
		err := dbTX.DB().WithContext(ctx).Select("id").Find(&matchedMsgs).Error
		if err != nil {
			return nil, err
		}
		validatedAcks := make([]*components.ReliableMessageAck, 0, len(acksToWrite))
		for _, a := range acksToWrite {
			for _, mm := range matchedMsgs {
				if mm.ID == a.MessageID {
					log.L(ctx).Infof("Writing ack for message %s", a.MessageID)
					validatedAcks = append(validatedAcks, a)
				}
			}
		}
		if len(validatedAcks) > 0 {
			// Now we're actually ready to insert them
			if err := tm.writeAcks(ctx, dbTX, acksToWrite...); err != nil {
				return nil, err
			}
		}
	}

	// Insert the transaction receipts
	if len(txReceiptsToFinalize) > 0 {
		err := tm.txManager.FinalizeTransactions(ctx, dbTX, txReceiptsToFinalize)
		if err != nil {
			return nil, err
		}
	}

	// Insert the prepared transactions, capturing any post-commit
	if len(preparedTxnToAdd) > 0 {
		if err := tm.txManager.WritePreparedTransactions(ctx, dbTX, preparedTxnToAdd); err != nil {
			return nil, err
		}
	}

	// Write any nullifiers we generated
	for domain, nullifiers := range nullifierUpserts {
		if err := tm.stateManager.WriteNullifiersForReceivedStates(ctx, dbTX, domain, nullifiers); err != nil {
			return nil, err
		}
	}

	// Write an received privacy group messages
	if len(msgsToReceive) > 0 {
		results, err := tm.groupManager.ReceiveMessages(ctx, dbTX, msgsToReceive)
		if err != nil {
			return nil, err
		}
		for _, m := range msgsToReceive {
			validateErr := results[m.ID]
			errStr := ""
			if validateErr != nil {
				errStr = validateErr.Error()
			}
			acksToSend = append(acksToSend, &ackInfo{node: m.Node, id: m.ID, Error: errStr})
		}
	}

	// We use a post-commit handler to send back any acks to the other side that are required
	return make([]flushwriter.Result[*noResult], len(values)), nil

}

func buildAck(msgID uuid.UUID, errString string) *prototk.PaladinMsg {
	cid := msgID.String()
	msgType := RMHMessageTypeAck
	if errString != "" {
		msgType = RMHMessageTypeNack
	}
	return &prototk.PaladinMsg{
		MessageId:     uuid.NewString(),
		Component:     prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
		MessageType:   msgType,
		CorrelationId: &cid,
		Payload:       tktypes.JSONString(&ackInfo{Error: errString}),
	}
}

func (tm *transportManager) parseReceivedAckNack(ctx context.Context, msg *components.ReceivedMessage) *components.ReliableMessageAck {
	var info ackInfo
	err := json.Unmarshal(msg.Payload, &info)
	if msg.CorrelationID == nil {
		err = i18n.NewError(ctx, msgs.MsgTransportAckMissingCorrelationID)
	}
	if err != nil {
		log.L(ctx).Errorf("Received invalid ack/nack: %s", msg.Payload)
		return nil
	}
	ackNackToWrite := &components.ReliableMessageAck{
		MessageID: *msg.CorrelationID,
		Time:      tktypes.TimestampNow(),
	}
	if msg.MessageType == RMHMessageTypeNack {
		if info.Error == "" {
			info.Error = i18n.NewError(ctx, msgs.MsgTransportNackMissingError).Error()
		}
		ackNackToWrite.Error = info.Error
	}
	return ackNackToWrite
}

func (tm *transportManager) buildStateDistributionMsg(ctx context.Context, dbTX persistence.DBTX, rm *components.ReliableMessage) (*prototk.PaladinMsg, error, error) {

	// Validate the message first (not retryable)
	sd, parsed, parseErr := parseStateDistribution(ctx, rm.ID, rm.Metadata)
	if parseErr != nil {
		return nil, parseErr, nil
	}

	// Get the state - distinguishing between not found, vs. a retryable error
	states, err := tm.stateManager.GetStatesByID(ctx, dbTX, sd.Domain, parsed.ContractAddress, []tktypes.HexBytes{parsed.ID}, false, false)
	if err != nil {
		return nil, nil, err
	}
	if len(states) != 1 {
		return nil,
			i18n.NewError(ctx, msgs.MsgTransportStateNotAvailableLocally, sd.Domain, parsed.ContractAddress, parsed.ID),
			nil
	}
	sd.StateData = states[0].Data

	return &prototk.PaladinMsg{
		MessageId:   rm.ID.String(),
		Component:   prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
		MessageType: RMHMessageTypeStateDistribution,
		Payload:     tktypes.JSONString(sd),
	}, nil, nil
}

func parseStateDistribution(ctx context.Context, msgID uuid.UUID, data []byte) (sd *components.StateDistributionWithData, parsed *components.StateUpsertOutsideContext, err error) {
	err = json.Unmarshal(data, &sd)
	if err == nil {
		parsed, err = parseState(ctx, sd)
	}
	if err != nil {
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgTransportInvalidMessageData, msgID)
	}
	return
}

func (tm *transportManager) buildPrivacyGroupDistributionMsg(ctx context.Context, dbTX persistence.DBTX, rm *components.ReliableMessage) (*prototk.PaladinMsg, error, error) {

	// Validate the message first (not retryable) - note the input is just a state distribution
	sd, parsed, parseErr := parseStateDistribution(ctx, rm.ID, rm.Metadata)
	if parseErr != nil {
		return nil, parseErr, nil
	}

	// Get the ABI
	abiSchema, findErr := tm.stateManager.GetSchemaByID(ctx, dbTX, sd.Domain, parsed.SchemaID, false)
	if findErr != nil {
		return nil, nil, findErr // retryable
	}
	var abiDefinition abi.Parameter
	if abiSchema != nil {
		parseErr = json.Unmarshal(abiSchema.Definition, &abiDefinition)
	}
	if parseErr != nil || abiSchema == nil {
		return nil,
			i18n.WrapError(ctx, parseErr, msgs.MsgTransportStateSchemaNotAvailableLocally, sd.Domain, parsed.SchemaID),
			nil
	}

	// Get the state - distinguishing between not found, vs. a retryable error
	states, err := tm.stateManager.GetStatesByID(ctx, dbTX, sd.Domain, parsed.ContractAddress, []tktypes.HexBytes{parsed.ID}, false, false)
	if err != nil {
		return nil, nil, err
	}
	if len(states) != 1 {
		return nil,
			i18n.NewError(ctx, msgs.MsgTransportStateNotAvailableLocally, sd.Domain, parsed.ContractAddress, parsed.ID),
			nil
	}
	sd.StateData = states[0].Data

	return &prototk.PaladinMsg{
		MessageId:   rm.ID.String(),
		Component:   prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
		MessageType: RMHMessageTypePrivacyGroup,
		Payload: tktypes.JSONString(components.PrivacyGroupGenesisWithABI{
			GenesisState: *sd,
			GenesisABI:   abiDefinition,
		}),
	}, nil, nil
}

func parsePrivacyGroupMessageDistribution(ctx context.Context, msgID uuid.UUID, data []byte) (pmd *components.PrivacyGroupMessageDistribution, err error) {
	err = json.Unmarshal(data, &pmd)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTransportInvalidMessageData, msgID)
	}
	return
}

func parsePrivacyGroupMessage(ctx context.Context, node string, msgID uuid.UUID, data []byte) (msg *pldapi.PrivacyGroupMessage, err error) {
	err = json.Unmarshal(data, &msg)
	if err != nil || msg.ID != msgID {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTransportInvalidMessageData, msgID)
	}
	msg.Node = node
	return
}

func (tm *transportManager) buildPrivacyGroupMessageMsg(ctx context.Context, dbTX persistence.DBTX, rm *components.ReliableMessage) (*prototk.PaladinMsg, error, error) {

	// Validate the message first (not retryable)
	pmd, parseErr := parsePrivacyGroupMessageDistribution(ctx, rm.ID, rm.Metadata)
	if parseErr != nil {
		return nil, parseErr, nil
	}

	// Get the Message
	msg, err := tm.groupManager.GetMessageByID(ctx, dbTX, pmd.ID, false)
	if err != nil {
		return nil, nil, err
	}
	if msg == nil {
		return nil,
			i18n.NewError(ctx, msgs.MsgTransportMessageNotAvailableLocally, pmd.ID),
			nil
	}

	return &prototk.PaladinMsg{
		MessageId:   rm.ID.String(),
		Component:   prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
		MessageType: RMHMessageTypePrivacyGroupMessage,
		Payload:     tktypes.JSONString(msg),
	}, nil, nil
}

func parsePrivacyGroupDistribution(ctx context.Context, msgID uuid.UUID, data []byte) (domain string, genesisABI *abi.Parameter, genesisState *components.StateUpsertOutsideContext, err error) {
	var pgInfo components.PrivacyGroupGenesisWithABI
	err = json.Unmarshal(data, &pgInfo)
	genesisABI = &pgInfo.GenesisABI
	domain = pgInfo.GenesisState.Domain
	if err == nil {
		genesisState, err = parseState(ctx, &pgInfo.GenesisState)
	}
	if err != nil {
		return "", nil, nil, i18n.WrapError(ctx, err, msgs.MsgTransportInvalidMessageData, msgID)
	}
	return
}

func parseState(ctx context.Context, sd *components.StateDistributionWithData) (parsed *components.StateUpsertOutsideContext, err error) {
	parsed = &components.StateUpsertOutsideContext{}
	parsed.Data = sd.StateData
	parsed.ID, err = tktypes.ParseHexBytes(ctx, sd.StateID)
	if err == nil {
		parsed.SchemaID, err = tktypes.ParseBytes32(sd.SchemaID)
	}
	if err == nil {
		parsed.ContractAddress, err = tktypes.ParseEthAddress(sd.ContractAddress)
	}
	if err != nil {
		return nil, err
	}
	return parsed, err
}
