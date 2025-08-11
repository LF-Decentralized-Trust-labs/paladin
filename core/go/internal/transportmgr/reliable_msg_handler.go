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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/flushwriter"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

const (
	RMHMessageTypeAck                 = "ack"
	RMHMessageTypeNack                = "nack"
	RMHMessageTypeStateDistribution   = string(pldapi.RMTState)
	RMHMessageTypeReceipt             = string(pldapi.RMTReceipt)
	RMHMessageTypePreparedTransaction = string(pldapi.RMTPreparedTransaction)
	RMHMessageTypePrivacyGroup        = string(pldapi.RMTPrivacyGroup)
	RMHMessageTypePrivacyGroupMessage = string(pldapi.RMTPrivacyGroupMessage)
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

type receivedPrivacyGroup struct {
	msgID        uuid.UUID
	id           pldtypes.HexBytes
	node         string
	domain       string
	genesisTx    uuid.UUID
	genesisState *components.StateUpsertOutsideContext
}

type receivedPrivacyGroupMessage struct {
	rMsgID  uuid.UUID
	node    string
	message *pldapi.PrivacyGroupMessage
}

func (tm *transportManager) handleReliableMsgBatch(ctx context.Context, dbTX persistence.DBTX, values []*reliableMsgOp) ([]flushwriter.Result[*noResult], error) {

	var acksToWrite []*pldapi.ReliableMessageAck
	var acksToSend []*ackInfo
	statesToAdd := make(map[string][]*stateAndAck)
	domainsWithPrivacyGroups := make(map[string]bool)
	nullifierUpserts := make(map[string][]*components.NullifierUpsert)
	var preparedTxnToAdd []*components.PreparedTransactionWithRefs
	var txReceiptsToFinalize []*components.ReceiptInput
	var msgsToReceive []*receivedPrivacyGroupMessage
	var privacyGroupsToAdd []*receivedPrivacyGroup

	dbTX.AddPostCommit(func(ctx context.Context) {
		// We've committed the database work ok - send the acks/nacks to the other side
		for _, a := range acksToSend {
			if a.Error != "" {
				log.L(ctx).Errorf("Sending nack to node '%s' for message %s: %s", a.node, a.id, a.Error)
			}
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
			receivedPG, err := parsePrivacyGroupDistribution(ctx, v.msg.MessageID, v.msg.Payload, v.p.Name)
			if err != nil {
				acksToSend = append(acksToSend,
					&ackInfo{node: v.p.Name, id: v.msg.MessageID, Error: err.Error()}, // reject the message permanently
				)
			} else {
				domainsWithPrivacyGroups[receivedPG.domain] = true
				statesToAdd[receivedPG.domain] = append(statesToAdd[receivedPG.domain], &stateAndAck{
					state: receivedPG.genesisState,
				})
				privacyGroupsToAdd = append(privacyGroupsToAdd, receivedPG)
			}
		case RMHMessageTypePrivacyGroupMessage:
			msg, err := parsePrivacyGroupMessage(ctx, v.p.Name, v.msg.MessageID, v.msg.Payload)
			if err != nil {
				acksToSend = append(acksToSend,
					&ackInfo{node: v.p.Name, id: v.msg.MessageID, Error: err.Error()}, // reject the message permanently
				)
			} else {
				msgsToReceive = append(msgsToReceive, &receivedPrivacyGroupMessage{node: v.p.Name, rMsgID: v.msg.MessageID, message: msg})
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

	for domain := range domainsWithPrivacyGroups {
		_, err := tm.stateManager.EnsureABISchemas(ctx, dbTX, domain, []*abi.Parameter{pldapi.PrivacyGroupABISchema()})
		if err != nil {
			// This is our built-in schema, it should not fail for any domain
			log.L(ctx).Errorf("ensure ABI failed: %s", err)
			return nil, err
		}
	}

	// Inserting the states is a performance critical activity that we ensure we batch as efficiently as possible,
	// while protecting ourselves from inserting states that we haven't done the local validation of.
	writtenStates := make(map[string][]*pldapi.State)
	for domain, states := range statesToAdd {
		batchStates := make([]*components.StateUpsertOutsideContext, len(states))
		for i, s := range states {
			batchStates[i] = s.state
		}
		domainStates, batchErr := tm.stateManager.WriteReceivedStates(ctx, dbTX, domain, batchStates)
		if batchErr != nil {
			// We have to try each individually (note if the error was transient in the DB we will rollback
			// the whole transaction and won't send any acks at all - which is good as sender will retry in that case)
			log.L(ctx).Errorf("batch insert of %d states for domain %s failed - attempting each individually: %s", len(states), domain, batchErr)
			for _, s := range states {
				_, err := tm.stateManager.WriteReceivedStates(ctx, dbTX, domain, []*components.StateUpsertOutsideContext{s.state})
				if err != nil {
					log.L(ctx).Errorf("insert state %s for domain %s failed: %s", s.state.ID, domain, batchErr)
					if s.ack != nil {
						s.ack.Error = err.Error()
					}
				}
			}
		}
		for _, s := range states {
			// Ack isn't set on privacy groups, as ack there is sent below
			if s.ack != nil {
				acksToSend = append(acksToSend, s.ack)
			}
		}
		writtenStates[domain] = domainStates
	}

	// We can only store acks for messages that are in our DB (due to foreign key relationship),
	// so we have to query them first to validate the acks before attempting insert.
	if len(acksToWrite) > 0 {
		ackQuery := make([]uuid.UUID, len(acksToWrite))
		for i, a := range acksToWrite {
			ackQuery[i] = a.MessageID
		}
		var matchedMsgs []*pldapi.ReliableMessage
		err := dbTX.DB().WithContext(ctx).Select("id").Find(&matchedMsgs).Error
		if err != nil {
			return nil, err
		}
		validatedAcks := make([]*pldapi.ReliableMessageAck, 0, len(acksToWrite))
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

	// Write any privacy groups that are now complete
	for _, pg := range privacyGroupsToAdd {
		var state *pldapi.State
		for _, s := range writtenStates[pg.domain] {
			if s.ID.Equals(pg.id) {
				state = s
				break
			}
		}
		var validationErr error
		if state == nil {
			validationErr = i18n.NewError(ctx, msgs.MsgTransportPrivacyGroupStateStorageFailed, pg.msgID)
		} else {
			// We didn't hit an error above, we can create the PG
			var persistErr error
			validationErr, persistErr = tm.groupManager.StoreReceivedGroup(ctx, dbTX, pg.domain, pg.genesisTx, state)
			if persistErr != nil {
				return nil, persistErr
			}
		}
		var ackErr string
		if validationErr != nil {
			ackErr = validationErr.Error()
		}
		acksToSend = append(acksToSend, &ackInfo{node: pg.node, id: pg.msgID, Error: ackErr})
	}

	// Write an received privacy group messages
	if len(msgsToReceive) > 0 {
		msgs := make([]*pldapi.PrivacyGroupMessage, len(msgsToReceive))
		for i, m := range msgsToReceive {
			msgs[i] = m.message
		}
		results, err := tm.groupManager.ReceiveMessages(ctx, dbTX, msgs)
		if err != nil {
			return nil, err
		}
		for _, m := range msgsToReceive {
			validateErr := results[m.message.ID]
			errStr := ""
			if validateErr != nil {
				errStr = validateErr.Error()
			}
			acksToSend = append(acksToSend, &ackInfo{node: m.node, id: m.rMsgID, Error: errStr})
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
		Payload:       pldtypes.JSONString(&ackInfo{Error: errString}),
	}
}

func (tm *transportManager) parseReceivedAckNack(ctx context.Context, msg *components.ReceivedMessage) *pldapi.ReliableMessageAck {
	var info ackInfo
	err := json.Unmarshal(msg.Payload, &info)
	if msg.CorrelationID == nil {
		err = i18n.NewError(ctx, msgs.MsgTransportAckMissingCorrelationID)
	}
	if err != nil {
		log.L(ctx).Errorf("Received invalid ack/nack: %s", msg.Payload)
		return nil
	}
	ackNackToWrite := &pldapi.ReliableMessageAck{
		MessageID: *msg.CorrelationID,
		Time:      pldtypes.TimestampNow(),
	}
	if msg.MessageType == RMHMessageTypeNack {
		if info.Error == "" {
			info.Error = i18n.NewError(ctx, msgs.MsgTransportNackMissingError).Error()
		}
		ackNackToWrite.Error = info.Error
	}
	return ackNackToWrite
}

func (tm *transportManager) buildStateDistributionMsg(ctx context.Context, dbTX persistence.DBTX, rm *pldapi.ReliableMessage) (*prototk.PaladinMsg, error, error) {

	// Validate the message first (not retryable)
	sd, parsed, parseErr := parseStateDistribution(ctx, rm.ID, rm.Metadata)
	if parseErr != nil {
		return nil, parseErr, nil
	}

	// Get the state - distinguishing between not found, vs. a retryable error
	states, err := tm.stateManager.GetStatesByID(ctx, dbTX, sd.Domain, parsed.ContractAddress, []pldtypes.HexBytes{parsed.ID}, false, false)
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
		Payload:     pldtypes.JSONString(sd),
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

func parsePrivacyGroupDistributionMetadata(ctx context.Context, msgID uuid.UUID, data []byte) (pgd *components.PrivacyGroupDistribution, parsed *components.StateUpsertOutsideContext, err error) {
	err = json.Unmarshal(data, &pgd)
	if err == nil {
		parsed, err = parseState(ctx, &pgd.GenesisState)
	}
	if err != nil {
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgTransportInvalidMessageData, msgID)
	}
	return
}

func (tm *transportManager) buildPrivacyGroupDistributionMsg(ctx context.Context, dbTX persistence.DBTX, rm *pldapi.ReliableMessage) (*prototk.PaladinMsg, error, error) {

	// Validate the message first (not retryable) - note the input is just a state distribution
	pgd, parsed, parseErr := parsePrivacyGroupDistributionMetadata(ctx, rm.ID, rm.Metadata)
	if parseErr != nil {
		return nil, parseErr, nil
	}
	domainName := pgd.GenesisState.Domain

	// Get the state - distinguishing between not found, vs. a retryable error
	states, err := tm.stateManager.GetStatesByID(ctx, dbTX, domainName, nil, []pldtypes.HexBytes{parsed.ID}, false, false)
	if err != nil {
		return nil, nil, err
	}
	if len(states) != 1 {
		return nil,
			i18n.NewError(ctx, msgs.MsgTransportStateNotAvailableLocally, domainName, nil, parsed.ID),
			nil
	}
	pgd.GenesisState.StateData = states[0].Data

	return &prototk.PaladinMsg{
		MessageId:   rm.ID.String(),
		Component:   prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
		MessageType: RMHMessageTypePrivacyGroup,
		Payload: pldtypes.JSONString(components.PrivacyGroupGenesis{
			GenesisTransaction: pgd.GenesisTransaction,
			GenesisState:       pgd.GenesisState,
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
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTransportInvalidMessageData, msgID)
	}
	msg.Node = node
	return
}

func (tm *transportManager) buildPrivacyGroupMessageMsg(ctx context.Context, dbTX persistence.DBTX, rm *pldapi.ReliableMessage) (*prototk.PaladinMsg, error, error) {

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
		Payload:     pldtypes.JSONString(msg),
	}, nil, nil
}

func parsePrivacyGroupDistribution(ctx context.Context, msgID uuid.UUID, data []byte, node string) (receivedPG *receivedPrivacyGroup, err error) {
	var pgInfo components.PrivacyGroupGenesis
	err = json.Unmarshal(data, &pgInfo)
	var id pldtypes.HexBytes
	if err == nil {
		id, err = pldtypes.ParseHexBytes(ctx, pgInfo.GenesisState.StateID)
	}
	if err == nil {
		receivedPG = &receivedPrivacyGroup{
			id:        id,
			node:      node,
			domain:    pgInfo.GenesisState.Domain,
			msgID:     msgID,
			genesisTx: pgInfo.GenesisTransaction,
		}
		receivedPG.genesisState, err = parseState(ctx, &pgInfo.GenesisState)
	}
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTransportInvalidMessageData, msgID)
	}
	return
}

func parseState(ctx context.Context, sd *components.StateDistributionWithData) (parsed *components.StateUpsertOutsideContext, err error) {
	parsed = &components.StateUpsertOutsideContext{}
	parsed.Data = sd.StateData
	parsed.ID, err = pldtypes.ParseHexBytes(ctx, sd.StateID)
	if err == nil {
		parsed.SchemaID, err = pldtypes.ParseBytes32(sd.SchemaID)
	}
	if err == nil && sd.ContractAddress != "" {
		parsed.ContractAddress, err = pldtypes.ParseEthAddress(sd.ContractAddress)
	}
	if err != nil {
		return nil, err
	}
	return parsed, err
}

func (tm *transportManager) buildReceiptDistributionMsg(ctx context.Context, dbTX persistence.DBTX, rm *pldapi.ReliableMessage) (*prototk.PaladinMsg, error, error) {

	// Validate the message first (not retryable)
	receipt, parseErr := parseMessageReceiptDistribution(ctx, rm.ID, rm.Metadata)
	if parseErr != nil {
		return nil, parseErr, nil
	}

	return &prototk.PaladinMsg{
		MessageId:   rm.ID.String(),
		Component:   prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
		MessageType: RMHMessageTypeReceipt,
		Payload:     pldtypes.JSONString(receipt),
	}, nil, nil
}

func parseMessageReceiptDistribution(ctx context.Context, msgID uuid.UUID, data []byte) (receipt *components.ReceiptInput, err error) {
	err = json.Unmarshal(data, &receipt)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTransportInvalidMessageData, msgID)
	}
	return
}
