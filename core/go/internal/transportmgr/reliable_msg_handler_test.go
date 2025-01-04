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
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func setupAckOrNackCheck(t *testing.T, tp *testPlugin, msgID uuid.UUID, expectedErr string) func() {
	mockActivateDeactivateOk(tp)
	sentMessages := make(chan *prototk.PaladinMsg)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sent := req.Message
		sentMessages <- sent
		return nil, nil
	}

	return func() {
		expectedAckOrNack := <-sentMessages
		require.Equal(t, msgID.String(), *expectedAckOrNack.CorrelationId)
		require.Equal(t, prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER, expectedAckOrNack.Component)
		var ai ackInfo
		err := json.Unmarshal(expectedAckOrNack.Payload, &ai)
		require.NoError(t, err)
		if expectedErr == "" {
			require.Equal(t, RMHMessageTypeAck, expectedAckOrNack.MessageType)
			require.Empty(t, ai.Error)
		} else {
			require.Equal(t, RMHMessageTypeNack, expectedAckOrNack.MessageType)
			require.Regexp(t, expectedErr, ai.Error)
		}

	}
}

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

	ackNackCheck := setupAckOrNackCheck(t, tp, msgID, "")

	// Receive the message that needs the ack
	rmr, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		FromNode: "node2",
		Message:  msg,
	})
	require.NoError(t, err)
	assert.NotNil(t, rmr)

	ackNackCheck()

}

func testReceivedReliableMsg(msgType string, payloadObj any) *components.ReceivedMessage {
	return &components.ReceivedMessage{
		MessageID:     uuid.New(),
		CorrelationID: confutil.P(uuid.New()),
		MessageType:   msgType,
		Payload:       tktypes.JSONString(payloadObj),
	}
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

	msg := testReceivedReliableMsg(
		RMHMessageTypeStateDistribution,
		&components.StateDistributionWithData{
			StateDistribution: components.StateDistribution{
				Domain:          "domain1",
				ContractAddress: tktypes.RandAddress().String(),
				SchemaID:        tktypes.RandHex(32),
				StateID:         tktypes.RandHex(32),
			},
			StateData: []byte(`{"some":"data"}`),
		})

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "bad data")

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	postCommit, _, err := tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{p: p, msg: msg},
	})
	require.NoError(t, err)

	// Run the postCommit and check we get the nack
	postCommit(nil)

	ackNackCheck()
}

func TestHandleStateDistroBadMsg(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
	)
	defer done()

	msg := testReceivedReliableMsg(
		RMHMessageTypeStateDistribution,
		&components.StateDistributionWithData{
			StateDistribution: components.StateDistribution{
				Domain:          "domain1",
				ContractAddress: tktypes.RandAddress().String(),
				SchemaID:        "wrongness",
				StateID:         tktypes.RandHex(32),
			},
			StateData: []byte(`{"some":"data"}`),
		})

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "PD012016")

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	postCommit, _, err := tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{p: p, msg: msg},
	})
	require.NoError(t, err)

	// Run the postCommit and check we get the nack
	postCommit(nil)

	ackNackCheck()
}

func TestHandleStateDistroUnknownMsgType(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
	)
	defer done()

	msg := testReceivedReliableMsg("unknown", struct{}{})

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "PD012017")

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	postCommit, _, err := tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{p: p, msg: msg},
	})
	require.NoError(t, err)

	// Run the postCommit and check we get the nack
	postCommit(nil)

	ackNackCheck()
}

func TestHandleAckFailReadMsg(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
		mc.db.Mock.ExpectQuery("SELECT.*reliable_msgs").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	msg := testReceivedReliableMsg(RMHMessageTypeAck, struct{}{})

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	_, _, err = tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{p: p, msg: msg},
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

	msg := testReceivedReliableMsg(RMHMessageTypeNack, struct{}{})
	msg.CorrelationID = &msgID

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	_, _, err = tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{p: p, msg: msg},
	})
	require.Regexp(t, "pop", err)

}

func TestHandleBadAckNoCorrelId(t *testing.T) {

	ctx, tm, _, done := newTestTransport(t, false)
	defer done()

	msg := testReceivedReliableMsg(RMHMessageTypeAck, struct{}{})
	msg.CorrelationID = nil

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	postCommit, _, err := tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{p: p, msg: msg},
	})
	require.NoError(t, err)
	postCommit(nil)
}

func TestHandleReceiptFail(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
			mc.txManager.On("FinalizeTransactions", mock.Anything, mock.Anything, mock.Anything).
				Return(fmt.Errorf("pop"))
		},
	)
	defer done()

	msg := testReceivedReliableMsg(
		RMHMessageTypeReceipt,
		&components.ReceiptInput{
			Domain:      "domain1",
			ReceiptType: components.RT_Success,
		})

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	_, _, err = tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{p: p, msg: msg},
	})
	require.Regexp(t, "pop", err)

}

func TestHandlePreparedTxFail(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
			mc.txManager.On("WritePreparedTransactions", mock.Anything, mock.Anything, mock.Anything).
				Return(nil, fmt.Errorf("pop"))
		},
	)
	defer done()

	msg := testReceivedReliableMsg(
		RMHMessageTypePreparedTransaction,
		&components.PreparedTransactionWithRefs{
			PreparedTransactionBase: &pldapi.PreparedTransactionBase{
				ID: uuid.New(),
			},
		})

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	_, _, err = tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{p: p, msg: msg},
	})
	require.Regexp(t, "pop", err)

}

func TestHandleReceiptBadData(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
	)
	defer done()

	msg := testReceivedReliableMsg(RMHMessageTypeReceipt, nil)
	msg.Payload = []byte(`!{ bad data`)

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "invalid character")

	// Handle the batch - will fail to write the states
	pc, _, err := tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{p: p, msg: msg},
	})
	require.NoError(t, err)

	pc(nil)

	ackNackCheck()
}

func TestHandlePreparedTxBadData(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
	)
	defer done()

	msg := testReceivedReliableMsg(RMHMessageTypePreparedTransaction, nil)
	msg.Payload = []byte(`!{ bad data`)

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "invalid character")

	pc, _, err := tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{p: p, msg: msg},
	})
	require.NoError(t, err)

	pc(nil)

	ackNackCheck()
}

func TestHandlePreparedOk(t *testing.T) {
	persistentTxPCCalled := make(chan struct{})
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
			mc.txManager.On("WritePreparedTransactions", mock.Anything, mock.Anything, mock.Anything).
				Return(func() {
					close(persistentTxPCCalled)
				}, nil)
		},
	)
	defer done()

	msg := testReceivedReliableMsg(
		RMHMessageTypePreparedTransaction,
		&components.PreparedTransactionWithRefs{
			PreparedTransactionBase: &pldapi.PreparedTransactionBase{
				ID: uuid.New(),
			},
		})

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "")
	pc, _, err := tm.handleReliableMsgBatch(ctx, tm.persistence.DB(), []*reliableMsgOp{
		{p: p, msg: msg},
	})
	require.NoError(t, err)

	pc(nil)

	<-persistentTxPCCalled

	ackNackCheck()
}
