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
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestReceiveMessageStateSendAckRealDB(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t, true,
		mockGoodTransport,
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
			mc.stateManager.On("WriteReceivedStates", mock.Anything, mock.Anything, "domain1", mock.Anything).
				Return(nil, nil).Once()
		},
	)
	defer done()

	msgID := uuid.New()
	msg := &prototk.PaladinMsg{
		MessageId:     msgID.String(),
		CorrelationId: confutil.P(uuid.NewString()),
		Component:     prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
		MessageType:   RMHMessageTypeStateDistribution,
		Payload: tktypes.JSONString(&components.StateDistributionWithData{
			StateDistribution: components.StateDistribution{
				Domain:          "domain1",
				ContractAddress: tktypes.RandAddress().String(),
				SchemaID:        tktypes.RandHex(32),
				StateID:         tktypes.RandHex(32),
			},
			StateData: []byte(`{"some":"data"}`),
		}),
	}

	mockActivateDeactivateOk(tp)
	sentMessages := make(chan *prototk.PaladinMsg)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sent := req.Message
		sentMessages <- sent
		return nil, nil
	}

	// Receive the message that needs the ack
	rmr, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		FromNode: "node2",
		Message:  msg,
	})
	require.NoError(t, err)
	assert.NotNil(t, rmr)

	ack := <-sentMessages
	require.JSONEq(t, string(ack.Payload), `{}`)
	require.Equal(t, msgID.String(), *ack.CorrelationId)

}

func TestHandleStateDistroBadState(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
			mc.stateManager.On("WriteReceivedStates", mock.Anything, mock.Anything, "domain1", mock.Anything).
				Return(nil, fmt.Errorf("bad data")).Twice()
		},
	)
	defer done()

	msgID := uuid.New()
	msg := &prototk.PaladinMsg{
		MessageId:     msgID.String(),
		CorrelationId: confutil.P(uuid.NewString()),
		Component:     prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
		MessageType:   RMHMessageTypeStateDistribution,
		Payload: tktypes.JSONString(&components.StateDistributionWithData{
			StateDistribution: components.StateDistribution{
				Domain:          "domain1",
				ContractAddress: tktypes.RandAddress().String(),
				SchemaID:        tktypes.RandHex(32),
				StateID:         tktypes.RandHex(32),
			},
			StateData: []byte(`{"some":"data"}`),
		}),
	}

	mockActivateDeactivateOk(tp)
	sentMessages := make(chan *prototk.PaladinMsg)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sent := req.Message
		sentMessages <- sent
		return nil, nil
	}

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	postCommit, _, err := tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{msgID: msgID, p: p, msg: msg},
	})
	require.NoError(t, err)

	// Run the postCommit and check we get the nack
	postCommit(nil)

	expectedNack := <-sentMessages
	require.Equal(t, msgID.String(), *expectedNack.CorrelationId)
	require.Equal(t, prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER, expectedNack.Component)
	require.Equal(t, RMHMessageTypeNack, expectedNack.MessageType)
	var ai ackInfo
	err = json.Unmarshal(expectedNack.Payload, &ai)
	require.NoError(t, err)
	require.Regexp(t, "bad data", ai.Error)
}

func TestHandleStateDistroBadMsg(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
	)
	defer done()

	msgID := uuid.New()
	msg := &prototk.PaladinMsg{
		MessageId:     msgID.String(),
		CorrelationId: confutil.P(uuid.NewString()),
		Component:     prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
		MessageType:   RMHMessageTypeStateDistribution,
		Payload: tktypes.JSONString(&components.StateDistributionWithData{
			StateDistribution: components.StateDistribution{
				Domain:          "domain1",
				ContractAddress: tktypes.RandAddress().String(),
				SchemaID:        "wrongness",
				StateID:         tktypes.RandHex(32),
			},
			StateData: []byte(`{"some":"data"}`),
		}),
	}

	mockActivateDeactivateOk(tp)
	sentMessages := make(chan *prototk.PaladinMsg)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sent := req.Message
		sentMessages <- sent
		return nil, nil
	}

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	postCommit, _, err := tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{msgID: msgID, p: p, msg: msg},
	})
	require.NoError(t, err)

	// Run the postCommit and check we get the nack
	postCommit(nil)

	expectedNack := <-sentMessages
	require.Equal(t, msgID.String(), *expectedNack.CorrelationId)
	require.Equal(t, prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER, expectedNack.Component)
	require.Equal(t, RMHMessageTypeNack, expectedNack.MessageType)
	var ai ackInfo
	err = json.Unmarshal(expectedNack.Payload, &ai)
	require.NoError(t, err)
	require.Regexp(t, "PD012016", ai.Error)
}

func TestHandleStateDistroUnknownMsgType(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
	)
	defer done()

	msgID := uuid.New()
	msg := &prototk.PaladinMsg{
		MessageId:     msgID.String(),
		CorrelationId: confutil.P(uuid.NewString()),
		Component:     prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
		MessageType:   "unknown",
		Payload:       []byte(`{}`),
	}

	mockActivateDeactivateOk(tp)
	sentMessages := make(chan *prototk.PaladinMsg)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sent := req.Message
		sentMessages <- sent
		return nil, nil
	}

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	postCommit, _, err := tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{msgID: msgID, p: p, msg: msg},
	})
	require.NoError(t, err)

	// Run the postCommit and check we get the nack
	postCommit(nil)

	expectedNack := <-sentMessages
	require.Equal(t, msgID.String(), *expectedNack.CorrelationId)
	require.Equal(t, prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER, expectedNack.Component)
	require.Equal(t, RMHMessageTypeNack, expectedNack.MessageType)
	var ai ackInfo
	err = json.Unmarshal(expectedNack.Payload, &ai)
	require.NoError(t, err)
	require.Regexp(t, "PD012017", ai.Error)
}

func TestHandleAckFailReadMsg(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
		mc.db.Mock.ExpectQuery("SELECT.*reliable_msgs").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	msgID := uuid.New()
	msg := &prototk.PaladinMsg{
		MessageId:     msgID.String(),
		CorrelationId: confutil.P(uuid.NewString()),
		Component:     prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
		MessageType:   RMHMessageTypeAck,
		Payload:       []byte(`{}`),
	}

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	_, _, err = tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{msgID: msgID, p: p, msg: msg},
	})
	require.Regexp(t, "pop", err)

}

func TestHandleNackFailWriteAck(t *testing.T) {
	msgID := uuid.New()

	ctx, tm, _, done := newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
		mc.db.Mock.ExpectQuery("SELECT.*reliable_msgs").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(msgID.String()))
		mc.db.Mock.ExpectExec("INSERT.*reliable_msg_acks").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	msg := &prototk.PaladinMsg{
		MessageId:     uuid.NewString(),
		CorrelationId: confutil.P(msgID.String()),
		Component:     prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
		MessageType:   RMHMessageTypeNack,
		Payload:       []byte(`{}`),
	}

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	_, _, err = tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{msgID: msgID, p: p, msg: msg},
	})
	require.Regexp(t, "pop", err)

}

func TestHandleBadAckNoCorrelId(t *testing.T) {
	msgID := uuid.New()

	ctx, tm, _, done := newTestTransport(t, false)
	defer done()

	msg := &prototk.PaladinMsg{
		MessageId:   uuid.NewString(),
		Component:   prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
		MessageType: RMHMessageTypeAck,
		Payload:     []byte(`{}`),
	}

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	postCommit, _, err := tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{msgID: msgID, p: p, msg: msg},
	})
	require.NoError(t, err)
	postCommit(nil)
}
