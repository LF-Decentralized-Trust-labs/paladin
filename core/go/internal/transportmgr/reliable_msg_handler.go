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
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/flushwriter"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
)

const (
	RMHMessageTypeAck               = "ack"
	RMHMessageTypeNack              = "nack"
	RMHMessageTypeStateDistribution = string(components.RMTState)
	RMHMessageTypeStateReceipt      = string(components.RMTReceipt)
)

type reliableMsgOp struct {
	msgID uuid.UUID
	p     *peer
	msg   *prototk.PaladinMsg
}

func (op *reliableMsgOp) WriteKey() string {
	return op.p.Name
}

type noResult struct{}

type ackInfo struct {
	node  string
	id    uuid.UUID // sent in CID on wire
	Error error     `json:"error,omitempty"`
}

type stateAndAck struct {
	state *components.StateUpsertOutsideContext
	ack   *ackInfo
}

func (tm *transportManager) handleReliableMsgBatch(ctx context.Context, dbTX *gorm.DB, values []*reliableMsgOp) (func(error), []flushwriter.Result[*noResult], error) {

	var acksToWrite []*components.ReliableMessageAck
	var acksToSend []*ackInfo
	statesToAdd := make(map[string][]*stateAndAck)

	// The batch can contain different kinds of message that all need persistence activity
	for _, v := range values {

		switch v.msg.MessageType {
		case RMHMessageTypeStateDistribution:
			sd, stateToAdd, err := parseStateDistribution(ctx, v.msgID, v.msg.Payload)
			if err != nil {
				acksToSend = append(acksToSend,
					&ackInfo{node: v.p.Name, id: v.msgID, Error: err}, // reject the message permanently
				)
			} else {
				statesToAdd[sd.Domain] = append(statesToAdd[sd.Domain], &stateAndAck{
					state: stateToAdd,
					ack:   &ackInfo{node: v.p.Name, id: v.msgID},
				})
			}
		case RMHMessageTypeAck, RMHMessageTypeNack:
			ackNackToWrite := tm.parseReceivedAckNack(ctx, v.msg)
			if ackNackToWrite != nil {
				acksToWrite = append(acksToWrite, ackNackToWrite)
			}
		default:
			err := i18n.NewError(ctx, msgs.MsgTransportUnsupportedReliableMsgType, v.msg.MessageType)
			acksToSend = append(acksToSend,
				&ackInfo{node: v.p.Name, id: v.msgID, Error: err}, // reject the message permanently
			)
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
					log.L(ctx).Errorf("insert state %s from message %s for domain %s failed - attempting each individually: %s", s.state.ID, s.ack.id, domain, batchErr)
					s.ack.Error = err
				}
			}
		}
		for _, s := range states {
			acksToSend = append(acksToSend, s.ack)
		}
	}

	// Inserting the acks we receive over the wire is a simple activity
	if len(acksToWrite) > 0 {
		if err := tm.writeAcks(ctx, dbTX, acksToWrite...); err != nil {
			return nil, nil, err
		}
	}

	// We use a post-commit handler to send back any acks to the other side that are required
	return func(err error) {
		if err == nil {
			// We've committed the database work ok - send the acks/nacks to the other side
			for _, a := range acksToSend {
				cid := a.id.String()
				msgType := RMHMessageTypeAck
				if a.Error != nil {
					msgType = RMHMessageTypeNack
				}
				_ = tm.queueFireAndForget(ctx, a.node, &prototk.PaladinMsg{
					MessageId:     uuid.NewString(),
					Component:     prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
					MessageType:   msgType,
					CorrelationId: &cid,
					Payload:       tktypes.JSONString(a),
				})
			}
		}
	}, make([]flushwriter.Result[*noResult], len(values)), nil

}

func (tm *transportManager) parseReceivedAckNack(ctx context.Context, msg *prototk.PaladinMsg) *components.ReliableMessageAck {
	var info ackInfo
	var cid uuid.UUID
	err := json.Unmarshal(msg.Payload, &info)
	if msg.CorrelationId == nil {
		err = i18n.NewError(ctx, msgs.MsgTransportAckMissingCorrelationID)
	}
	if err == nil {
		cid, err = uuid.Parse(*msg.CorrelationId)
	}
	if err != nil {
		log.L(ctx).Errorf("Received invalid ack/nack: %s", msg.Payload)
		return nil
	}
	ackNackToWrite := &components.ReliableMessageAck{
		MessageID: cid,
		Time:      tktypes.TimestampNow(),
	}
	if msg.MessageType == RMHMessageTypeNack {
		if info.Error == nil {
			info.Error = i18n.NewError(ctx, msgs.MsgTransportNackMissingError)
		}
		ackNackToWrite.Error = info.Error.Error()
	}
	return ackNackToWrite
}

func parseStateDistribution(ctx context.Context, msgID uuid.UUID, data []byte) (sd *components.StateDistributionWithData, parsed *components.StateUpsertOutsideContext, err error) {
	parsed = &components.StateUpsertOutsideContext{}
	var contractAddr *tktypes.EthAddress
	err = json.Unmarshal(data, &sd)
	if err == nil {
		parsed.ID, err = tktypes.ParseHexBytes(ctx, sd.StateID)
	}
	if err == nil {
		parsed.SchemaID, err = tktypes.ParseBytes32(sd.SchemaID)
	}
	if err == nil {
		contractAddr, err = tktypes.ParseEthAddress(sd.ContractAddress)
	}
	if err == nil {
		parsed.ContractAddress = *contractAddr
	}
	if err != nil {
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgTransportInvalidMessageData, msgID)
	}
	return
}
