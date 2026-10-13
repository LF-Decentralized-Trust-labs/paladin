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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
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

func captureTransportLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := new(bytes.Buffer)
	logger := logrus.StandardLogger()
	prevOut := logger.Out
	logger.SetOutput(buf)
	t.Cleanup(func() {
		logger.SetOutput(prevOut)
	})
	return buf
}

func TestHandleReliableMsgBatchLogsAckNackSendFailures(t *testing.T) {
	t.Run("queue failure", func(t *testing.T) {
		ctx, tm, _, done := newTestTransport(t, false,
			mockEmptyReliableMsgs,
			func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
				mc.db.Mock.ExpectBegin()
				mc.db.Mock.ExpectCommit()
			})
		defer done()

		logBuf := captureTransportLogs(t)
		msg := testReceivedReliableMsg("unsupported_message_type", nil)
		p := &peer{PeerInfo: pldapi.PeerInfo{Name: tm.localNodeName}}

		err := tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
			_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{{p: p, msg: msg}})
			return err
		})
		require.NoError(t, err)
		require.Contains(t, logBuf.String(), fmt.Sprintf("Failed to send nack to node '%s' for message %s", p.Name, msg.MessageID))
	})

	t.Run("send failure", func(t *testing.T) {
		ctx, tm, tp, done := newTestTransport(t, false,
			mockGoodTransport,
			mockEmptyReliableMsgs,
			func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
				mc.db.Mock.ExpectBegin()
				mc.db.Mock.ExpectCommit()
			})
		defer done()

		logBuf := captureTransportLogs(t)
		sendAttempted := make(chan struct{}, 1)
		mockActivateDeactivateOk(tp)
		tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
			select {
			case sendAttempted <- struct{}{}:
			default:
			}
			return nil, fmt.Errorf("pop")
		}

		msg := testReceivedReliableMsg("unsupported_message_type", nil)
		p := &peer{PeerInfo: pldapi.PeerInfo{Name: "node2"}}

		err := tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
			_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{{p: p, msg: msg}})
			return err
		})
		require.NoError(t, err)

		select {
		case <-sendAttempted:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for ack/nack send attempt")
		}
		require.Eventually(t, func() bool {
			return strings.Contains(logBuf.String(), fmt.Sprintf("Failed to send nack to node '%s' for message %s: pop", p.Name, msg.MessageID))
		}, 5*time.Second, 50*time.Millisecond)
	})
}

func TestReceiveMessageStateWithNullifierSendAckRealDB(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t, true,
		mockGoodTransport,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.stateManager.On("WriteReceivedStates", mock.Anything, mock.Anything, "domain1", mock.Anything).
				Return(nil, nil).Once()
			nullifier := &pldapi.StateNullifier{DomainName: "domain1", ID: pldtypes.RandBytes(32)}
			mc.stateManager.On("WriteNullifiersForReceivedStates", mock.Anything, mock.Anything, "domain1", []*pldapi.StateNullifier{nullifier}).
				Return(nil).Once()
			mkr := componentsmocks.NewKeyResolver(t)
			mc.sequencerManager.On("BuildNullifier", mock.Anything, mkr, mock.Anything).Return(nullifier, nil)
			mc.keyManager.On("KeyResolverForDBTX", mock.Anything).Return(mkr).Once()
		},
	)
	defer done()

	msgID := uuid.New()
	msg := &prototk.PaladinMsg{
		MessageId:     msgID.String(),
		CorrelationId: confutil.P(uuid.NewString()),
		Component:     prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
		MessageType:   RMHMessageTypeStateDistribution,
		Payload: pldtypes.JSONString(&components.StateDistributionWithData{
			StateDistribution: components.StateDistribution{
				Domain:                "domain1",
				ContractAddress:       pldtypes.RandAddress().String(),
				SchemaID:              pldtypes.RandHex(32),
				StateID:               pldtypes.RandHex(32),
				NullifierAlgorithm:    confutil.P("algo1"),
				NullifierVerifierType: confutil.P("vtype1"),
				NullifierPayloadType:  confutil.P("ptype1"),
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
		Payload:       pldtypes.JSONString(payloadObj),
	}
}

func TestHandleStateDistroBadState(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
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
				ContractAddress: pldtypes.RandAddress().String(),
				SchemaID:        pldtypes.RandHex(32),
				StateID:         pldtypes.RandHex(32),
			},
			StateData: []byte(`{"some":"data"}`),
		})

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "bad data")

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestHandleStateDistroMixedBatchBadAndGoodStates(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
			mc.stateManager.On("WriteReceivedStates", mock.Anything, mock.Anything, "domain1", mock.Anything).
				Return(nil, fmt.Errorf("bad data")).Once()
			mc.stateManager.On("WriteReceivedStates", mock.Anything, mock.Anything, "domain1", mock.Anything).
				Return(nil, nil).Once()
		},
	)
	defer done()

	msg := testReceivedReliableMsg(
		RMHMessageTypeStateDistribution,
		&components.StateDistributionWithData{
			StateDistribution: components.StateDistribution{
				Domain:          "domain1",
				ContractAddress: pldtypes.RandAddress().String(),
				SchemaID:        pldtypes.RandHex(32),
				StateID:         pldtypes.RandHex(32),
			},
			StateData: []byte(`{"some":"data"}`),
		})

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "")

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestHandleStateDistroBadNullifier(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
			mkr := componentsmocks.NewKeyResolver(t)
			mc.sequencerManager.On("BuildNullifier", mock.Anything, mkr, mock.Anything).Return(nil, fmt.Errorf("bad nullifier"))
			mc.keyManager.On("KeyResolverForDBTX", mock.Anything).Return(mkr).Once()
		},
	)
	defer done()

	msg := testReceivedReliableMsg(
		RMHMessageTypeStateDistribution,
		&components.StateDistributionWithData{
			StateDistribution: components.StateDistribution{
				Domain:                "domain1",
				ContractAddress:       pldtypes.RandAddress().String(),
				SchemaID:              pldtypes.RandHex(32),
				StateID:               pldtypes.RandHex(32),
				NullifierAlgorithm:    confutil.P("algo1"),
				NullifierVerifierType: confutil.P("vtype1"),
				NullifierPayloadType:  confutil.P("ptype1"),
			},
			StateData: []byte(`{"some":"data"}`),
		})

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "bad nullifier")

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestHandleStateDistroBadMsg(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	msg := testReceivedReliableMsg(
		RMHMessageTypeStateDistribution,
		&components.StateDistributionWithData{
			StateDistribution: components.StateDistribution{
				Domain:          "domain1",
				ContractAddress: pldtypes.RandAddress().String(),
				SchemaID:        "wrongness",
				StateID:         pldtypes.RandHex(32),
			},
			StateData: []byte(`{"some":"data"}`),
		})

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "PD012016")

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestHandleStateDistroUnknownMsgType(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	msg := testReceivedReliableMsg("unknown", struct{}{})

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "PD012017")

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestHandleAckFailReadMsg(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
		mc.db.Mock.ExpectBegin()
		mc.db.Mock.ExpectQuery("SELECT.*reliable_msgs").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	msg := testReceivedReliableMsg(RMHMessageTypeAck, struct{}{})

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err = tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.Regexp(t, "pop", err)

}

func TestHandleNackFailWriteAck(t *testing.T) {
	msgID := uuid.New()

	ctx, tm, _, done := newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
		mc.db.Mock.ExpectBegin()
		mc.db.Mock.ExpectQuery("SELECT.*reliable_msgs").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(msgID.String()))
		mc.db.Mock.ExpectExec("INSERT.*reliable_msg_acks").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	msg := testReceivedReliableMsg(RMHMessageTypeNack, struct{}{})
	msg.CorrelationID = &msgID

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err = tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.Regexp(t, "pop", err)

}

func TestHandleBadAckNoCorrelId(t *testing.T) {

	ctx, tm, _, done := newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
		mc.db.Mock.ExpectBegin()
		mc.db.Mock.ExpectCommit()
	})
	defer done()

	msg := testReceivedReliableMsg(RMHMessageTypeAck, struct{}{})
	msg.CorrelationID = nil

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)
}

func TestHandleReceiptFail(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
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
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err = tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.Regexp(t, "pop", err)

}

func TestHandleReceiptOk(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false,
		mockGoodTransport,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
			mc.txManager.On("FinalizeTransactions", mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
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

	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

}

func TestHandlePreparedTxFail(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.txManager.On("WritePreparedTransactions", mock.Anything, mock.Anything, mock.Anything).
				Return(fmt.Errorf("pop"))
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

	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err = tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.Regexp(t, "pop", err)

}

func TestHandleNullifierFail(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.stateManager.On("WriteReceivedStates", mock.Anything, mock.Anything, "domain1", mock.Anything).
				Return(nil, nil).Once()
			nullifier := &pldapi.StateNullifier{DomainName: "domain1", ID: pldtypes.RandBytes(32)}
			mc.stateManager.On("WriteNullifiersForReceivedStates", mock.Anything, mock.Anything, "domain1", []*pldapi.StateNullifier{nullifier}).
				Return(fmt.Errorf("pop")).Once()
			mkr := componentsmocks.NewKeyResolver(t)
			mc.sequencerManager.On("BuildNullifier", mock.Anything, mkr, mock.Anything).Return(nullifier, nil)
			mc.keyManager.On("KeyResolverForDBTX", mock.Anything).Return(mkr).Once()
		},
	)
	defer done()

	msg := testReceivedReliableMsg(
		RMHMessageTypeStateDistribution,
		&components.StateDistributionWithData{
			StateDistribution: components.StateDistribution{
				Domain:                "domain1",
				ContractAddress:       pldtypes.RandAddress().String(),
				SchemaID:              pldtypes.RandHex(32),
				StateID:               pldtypes.RandHex(32),
				NullifierAlgorithm:    confutil.P("algo1"),
				NullifierVerifierType: confutil.P("vtype1"),
				NullifierPayloadType:  confutil.P("ptype1"),
			},
			StateData: []byte(`{"some":"data"}`),
		})

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.Regexp(t, "pop", err)

}

func TestHandleReceiptBadData(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	msg := testReceivedReliableMsg(RMHMessageTypeReceipt, nil)
	msg.Payload = []byte(`!{ bad data`)

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "invalid character")

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestHandlePreparedTxBadData(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	msg := testReceivedReliableMsg(RMHMessageTypePreparedTransaction, nil)
	msg.Payload = []byte(`!{ bad data`)

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "invalid character")

	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestHandlePreparedOk(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
			mc.txManager.On("WritePreparedTransactions", mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
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
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestHandlePrivacyGroupOK(t *testing.T) {
	var stateID pldtypes.HexBytes = pldtypes.RandBytes(32)
	schemaID := pldtypes.RandBytes32()
	schema := componentsmocks.NewSchema(t)
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.stateManager.On("EnsureABISchemas", mock.Anything, mock.Anything, "domain1", mock.Anything).Return([]components.Schema{
				schema,
			}, nil).Once()
			mc.stateManager.On("WriteReceivedStates", mock.Anything, mock.Anything, "domain1", mock.Anything).
				Return([]*pldapi.State{
					{StateBase: pldapi.StateBase{ID: stateID, Schema: schemaID}},
				}, nil).Once()
			mc.groupManager.On("StoreReceivedGroup", mock.Anything, mock.Anything, "domain1", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	txID := uuid.New()
	msg := testReceivedReliableMsg(
		RMHMessageTypePrivacyGroup,
		&components.PrivacyGroupGenesis{
			GenesisTransaction: txID,
			GenesisState: components.StateDistributionWithData{
				StateDistribution: components.StateDistribution{
					Domain:          "domain1",
					ContractAddress: pldtypes.RandAddress().String(),
					SchemaID:        schemaID.String(),
					StateID:         stateID.String(),
				},
				StateData: []byte(`{"some":"data"}`),
			},
		})

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "")

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestHandlePrivacyGroupBadState(t *testing.T) {
	var stateID pldtypes.HexBytes = pldtypes.RandBytes(32)
	schemaID := pldtypes.RandBytes32()
	schema := componentsmocks.NewSchema(t)
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.stateManager.On("EnsureABISchemas", mock.Anything, mock.Anything, "domain1", mock.Anything).Return([]components.Schema{
				schema,
			}, nil).Once()
			mc.stateManager.On("WriteReceivedStates", mock.Anything, mock.Anything, "domain1", mock.Anything).Return(nil, fmt.Errorf("pop"))

			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	txID := uuid.New()
	msg := testReceivedReliableMsg(
		RMHMessageTypePrivacyGroup,
		&components.PrivacyGroupGenesis{
			GenesisTransaction: txID,
			GenesisState: components.StateDistributionWithData{
				StateDistribution: components.StateDistribution{
					Domain:          "domain1",
					ContractAddress: pldtypes.RandAddress().String(),
					SchemaID:        schemaID.String(),
					StateID:         stateID.String(),
				},
				StateData: []byte(`{"some":"data"}`),
			},
		})

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "PD012022")

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestHandlePrivacyGroupGroupFail(t *testing.T) {
	var stateID pldtypes.HexBytes = pldtypes.RandBytes(32)
	schemaID := pldtypes.RandBytes32()
	schema := componentsmocks.NewSchema(t)
	ctx, tm, _, done := newTestTransport(t, false,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.stateManager.On("EnsureABISchemas", mock.Anything, mock.Anything, "domain1", mock.Anything).Return([]components.Schema{
				schema,
			}, nil).Once()
			mc.stateManager.On("WriteReceivedStates", mock.Anything, mock.Anything, "domain1", mock.Anything).
				Return([]*pldapi.State{
					{StateBase: pldapi.StateBase{ID: stateID, Schema: schemaID}},
				}, nil).Once()
			mc.groupManager.On("StoreReceivedGroup", mock.Anything, mock.Anything, "domain1", mock.Anything, mock.Anything, mock.Anything).
				Return(nil, fmt.Errorf("pop"))

			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	txID := uuid.New()
	msg := testReceivedReliableMsg(
		RMHMessageTypePrivacyGroup,
		&components.PrivacyGroupGenesis{
			GenesisTransaction: txID,
			GenesisState: components.StateDistributionWithData{
				StateDistribution: components.StateDistribution{
					Domain:          "domain1",
					ContractAddress: pldtypes.RandAddress().String(),
					SchemaID:        schemaID.String(),
					StateID:         stateID.String(),
				},
				StateData: []byte(`{"some":"data"}`),
			},
		})

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.Regexp(t, "pop", err)
}

func TestHandlePrivacyGroupBuiltInABIFail(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.stateManager.On("EnsureABISchemas", mock.Anything, mock.Anything, "domain1", mock.Anything).Return(nil, fmt.Errorf("pop"))
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	msg := testReceivedReliableMsg(
		RMHMessageTypePrivacyGroup,
		&components.PrivacyGroupGenesis{
			GenesisState: components.StateDistributionWithData{
				StateDistribution: components.StateDistribution{
					Domain:          "domain1",
					ContractAddress: pldtypes.RandAddress().String(),
					SchemaID:        pldtypes.RandHex(32),
					StateID:         pldtypes.RandHex(32),
				},
				StateData: []byte(`{"some":"data"}`),
			},
		})

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.Regexp(t, "pop", err)

}

func TestHandlePrivacyGroupInvalid(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	msg := testReceivedReliableMsg(
		RMHMessageTypePrivacyGroup,
		&components.PrivacyGroupGenesis{
			/* invalid */
		})

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "PD012016")

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func mockReceiveMessagesOK(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
	mrm := mc.groupManager.On("ReceiveMessages", mock.Anything, mock.Anything, mock.Anything)
	mrm.Run(func(args mock.Arguments) {
		pms := args[2].([]*pldapi.PrivacyGroupMessage)
		res := map[uuid.UUID]error{}
		for _, pm := range pms {
			res[pm.ID] = nil
		}
		mrm.Return(res, nil)
	})
}

func testReceivedMesage() *components.ReceivedMessage {
	msgID := uuid.New()
	return &components.ReceivedMessage{
		MessageID:     msgID,
		CorrelationID: confutil.P(uuid.New()),
		MessageType:   RMHMessageTypePrivacyGroupMessage,
		Payload: pldtypes.JSONString(&pldapi.PrivacyGroupMessage{
			Sent: pldtypes.TimestampNow(),
			ID:   msgID,
			PrivacyGroupMessageInput: pldapi.PrivacyGroupMessageInput{
				Domain: "domain1",
				Group:  pldtypes.RandBytes(32),
				Topic:  "topic.1",
				Data:   pldtypes.JSONString("some data"),
			},
		}),
	}
}

func TestHandlePrivacyGroupMessageOK(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		mockReceiveMessagesOK,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	msg := testReceivedMesage()
	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "")

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestHandlePrivacyGroupMessageReject(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mrm := mc.groupManager.On("ReceiveMessages", mock.Anything, mock.Anything, mock.Anything)
			mrm.Run(func(args mock.Arguments) {
				pms := args[2].([]*pldapi.PrivacyGroupMessage)
				res := map[uuid.UUID]error{}
				for _, pm := range pms {
					res[pm.ID] = fmt.Errorf("badness")
				}
				mrm.Return(res, nil)
			})

			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	msg := testReceivedMesage()
	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "badness")

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestHandlePrivacyGroupMessageFail(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.groupManager.On("ReceiveMessages", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
			mc.db.Mock.ExpectBegin()
		},
	)
	defer done()

	msg := testReceivedMesage()

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.Regexp(t, "pop", err)
}

func TestHandlePrivacyGroupMessageBad(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	msg := testReceivedReliableMsg(
		RMHMessageTypePrivacyGroupMessage,
		"not an object")
	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "PD012016")

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	// Handle the batch - will fail to write the states
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestBuildPrivacyGroupDistributionMsgBadMsg(t *testing.T) {

	ctx, tm, _, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	_, parseErr, err := tm.buildPrivacyGroupDistributionMsg(ctx, tm.persistence.NOTX(), &pldapi.ReliableMessage{})
	require.NoError(t, err)
	require.Regexp(t, "PD012016", parseErr)

}

func TestBuildPrivacyGroupDistributionMsgGetStatesError(t *testing.T) {

	ctx, tm, _, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.stateManager.On("GetStatesByID", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, false, false).
				Return(nil, fmt.Errorf("pop")).Once()

			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	distroID := uuid.New()
	_, _, err := tm.buildPrivacyGroupDistributionMsg(ctx, tm.persistence.NOTX(), &pldapi.ReliableMessage{
		ID:          distroID,
		MessageType: pldapi.RMTPrivacyGroup.Enum(),
		Metadata: pldtypes.JSONString(&components.PrivacyGroupDistribution{
			GenesisTransaction: uuid.New(),
			GenesisState: components.StateDistributionWithData{
				StateDistribution: components.StateDistribution{
					Domain:          "domain1",
					ContractAddress: pldtypes.RandAddress().String(),
					SchemaID:        pldtypes.RandHex(32),
					StateID:         pldtypes.RandHex(32),
				},
			},
		}),
	})
	require.Regexp(t, "pop", err)

}

func TestBuildPrivacyGroupDistributionMsgGetStatesNotFound(t *testing.T) {

	ctx, tm, _, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.stateManager.On("GetStatesByID", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, false, false).
				Return(nil, nil).Once()

			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	distroID := uuid.New()
	_, parseErr, err := tm.buildPrivacyGroupDistributionMsg(ctx, tm.persistence.NOTX(), &pldapi.ReliableMessage{
		ID:          distroID,
		MessageType: pldapi.RMTPrivacyGroup.Enum(),
		Metadata: pldtypes.JSONString(&components.PrivacyGroupDistribution{
			GenesisTransaction: uuid.New(),
			GenesisState: components.StateDistributionWithData{
				StateDistribution: components.StateDistribution{
					Domain:          "domain1",
					ContractAddress: pldtypes.RandAddress().String(),
					SchemaID:        pldtypes.RandHex(32),
					StateID:         pldtypes.RandHex(32),
				},
			},
		}),
	})
	require.NoError(t, err)
	require.Regexp(t, "PD012014", parseErr)

}

func TestParsePrivacyGroupMessageDistributionFail(t *testing.T) {

	ctx, tm, _, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	distroID := uuid.New()
	_, parseErr, err := tm.buildPrivacyGroupMessageMsg(ctx, tm.persistence.NOTX(), &pldapi.ReliableMessage{
		ID:          distroID,
		MessageType: pldapi.RMTPrivacyGroup.Enum(),
		Metadata:    nil,
	})
	require.NoError(t, err)
	require.Regexp(t, "PD012016", parseErr)

}

func TestParsePrivacyGroupMessageGetMessageError(t *testing.T) {

	ctx, tm, _, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.groupManager.On("GetMessageByID", mock.Anything, mock.Anything, mock.Anything, false).Return(nil, fmt.Errorf("pop"))

			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	distroID := uuid.New()
	_, _, err := tm.buildPrivacyGroupMessageMsg(ctx, tm.persistence.NOTX(), &pldapi.ReliableMessage{
		ID:          distroID,
		MessageType: pldapi.RMTPrivacyGroup.Enum(),
		Metadata: pldtypes.JSONString(&components.PrivacyGroupMessageDistribution{
			Domain: "domain1",
			Group:  pldtypes.RandBytes(32),
			ID:     uuid.New(),
		}),
	})
	require.Regexp(t, "pop", err)

}

func TestParsePrivacyGroupMessageGetMessageNotFound(t *testing.T) {

	ctx, tm, _, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.groupManager.On("GetMessageByID", mock.Anything, mock.Anything, mock.Anything, false).Return(nil, nil)

			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	distroID := uuid.New()
	_, parseErr, err := tm.buildPrivacyGroupMessageMsg(ctx, tm.persistence.NOTX(), &pldapi.ReliableMessage{
		ID:          distroID,
		MessageType: pldapi.RMTPrivacyGroup.Enum(),
		Metadata: pldtypes.JSONString(&components.PrivacyGroupMessageDistribution{
			Domain: "domain1",
			Group:  pldtypes.RandBytes(32),
			ID:     uuid.New(),
		}),
	})
	require.NoError(t, err)
	require.Regexp(t, "PD012021", parseErr)

}

func TestBuildReceiptDistributionMsgBadMsg(t *testing.T) {

	ctx, tm, _, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	_, parseErr, err := tm.buildReceiptDistributionMsg(ctx, tm.persistence.NOTX(), &pldapi.ReliableMessage{})
	require.NoError(t, err)
	require.Regexp(t, "PD012016", parseErr)

}

func TestHandlePublicTransactionSubmissionOk(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
			mc.publicTxManager.On("WriteReceivedPublicTransactionSubmissions", mock.Anything, mock.Anything, mock.Anything).
				Return(nil)
		},
	)
	defer done()

	publicTxSubmission := &pldapi.PublicTxWithBinding{
		PublicTx: &pldapi.PublicTx{
			From:       *pldtypes.RandAddress(),
			Nonce:      confutil.P(pldtypes.HexUint64(1)),
			Created:    pldtypes.TimestampNow(),
			Dispatcher: "test-dispatcher",
		},
		PublicTxBinding: pldapi.PublicTxBinding{
			Transaction:     uuid.New(),
			TransactionType: pldapi.TransactionTypePublic.Enum(),
		},
	}

	msg := testReceivedReliableMsg(RMHMessageTypePublicTransactionSubmission, publicTxSubmission)
	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "")

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestHandlePublicTransactionSubmissionBadData(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	msg := testReceivedReliableMsg(RMHMessageTypePublicTransactionSubmission, nil)
	msg.Payload = []byte(`!{ bad data`)

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "invalid character")

	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestHandlePublicTransactionSubmissionFail(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.publicTxManager.On("WriteReceivedPublicTransactionSubmissions", mock.Anything, mock.Anything, mock.Anything).
				Return(fmt.Errorf("pop"))
		},
	)
	defer done()

	publicTxSubmission := &pldapi.PublicTxWithBinding{
		PublicTx: &pldapi.PublicTx{
			From:       *pldtypes.RandAddress(),
			Nonce:      confutil.P(pldtypes.HexUint64(1)),
			Created:    pldtypes.TimestampNow(),
			Dispatcher: "test-dispatcher",
		},
		PublicTxBinding: pldapi.PublicTxBinding{
			Transaction:     uuid.New(),
			TransactionType: pldapi.TransactionTypePublic.Enum(),
		},
	}

	msg := testReceivedReliableMsg(RMHMessageTypePublicTransactionSubmission, publicTxSubmission)

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err = tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.Regexp(t, "pop", err)
}

func TestBuildPublicTransactionSubmissionMsgBadMsg(t *testing.T) {

	ctx, tm, _, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	_, parseErr, err := tm.buildPublicTransactionSubmissionMsg(ctx, tm.persistence.NOTX(), &pldapi.ReliableMessage{})
	require.NoError(t, err)
	require.Regexp(t, "PD012016", parseErr)

}

func TestBuildSequencingProgressActivityMsgBadMsg(t *testing.T) {

	ctx, tm, _, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	_, parseErr, err := tm.buildSequencingProgressActivityMsg(ctx, tm.persistence.NOTX(), &pldapi.ReliableMessage{})
	require.NoError(t, err)
	require.Regexp(t, "PD012016", parseErr)

}

func TestHandleSequencingActivityOk(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectQuery(`INSERT INTO "sequencer_activities"`).
				WithArgs(
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
				).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	sequencingActivity := &components.SequencingActivity{
		TransactionID:  uuid.New(),
		ActivityType:   string(pldapi.SequencerActivityType_Dispatch),
		SequencingNode: "node2",
		Timestamp:      pldtypes.TimestampNow(),
	}

	msg := testReceivedReliableMsg(RMHMessageTypeSequencingActivity, sequencingActivity)
	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "")

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestHandleSequencingActivityBadData(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	msg := testReceivedReliableMsg(RMHMessageTypeSequencingActivity, nil)
	msg.Payload = []byte(`!{ bad data`)

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	ackNackCheck := setupAckOrNackCheck(t, tp, msg.MessageID, "invalid character")

	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.NoError(t, err)

	ackNackCheck()
}

func TestHandleSequencingActivityFail(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerInlineConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectQuery(`INSERT INTO "sequencer_activities"`).
				WithArgs(
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
				).
				WillReturnError(fmt.Errorf("pop"))
			mc.db.Mock.ExpectRollback()
		},
	)
	defer done()

	sequencingActivity := &components.SequencingActivity{
		TransactionID:  uuid.New(),
		ActivityType:   string(pldapi.SequencerActivityType_Dispatch),
		SequencingNode: "node2",
		Timestamp:      pldtypes.TimestampNow(),
	}

	msg := testReceivedReliableMsg(RMHMessageTypeSequencingActivity, sequencingActivity)

	p, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err = tm.handleReliableMsgBatch(ctx, dbTX, []*reliableMsgOp{
			{p: p, msg: msg},
		})
		return err
	})
	require.Regexp(t, "pop", err)
}

func TestParseMessageSequencingProgressBadData(t *testing.T) {
	ctx := context.Background()
	msgID := uuid.New()
	invalidData := []byte(`!{ bad data`)

	_, err := parseMessageSequencingProgress(ctx, msgID, invalidData)
	require.Error(t, err)
	require.Regexp(t, "PD012016", err)
}
