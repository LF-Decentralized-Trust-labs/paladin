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
	"database/sql/driver"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type testPlugin struct {
	plugintk.TransportAPIBase
	initialized atomic.Bool
	t           *transport
}

func (tp *testPlugin) Initialized() {
	tp.initialized.Store(true)
}

func newTestPlugin(transportFuncs *plugintk.TransportAPIFunctions) *testPlugin {
	return &testPlugin{
		TransportAPIBase: plugintk.TransportAPIBase{
			Functions: transportFuncs,
		},
	}
}

func newTestTransport(t *testing.T, realDB bool, extraSetup ...func(mc *mockComponents) components.TransportClient) (context.Context, *transportManager, *testPlugin, func()) {

	ctx, tm, _, done := newTestTransportManager(t, realDB, &pldconf.TransportManagerConfig{
		NodeName: "node1",
		Transports: map[string]*pldconf.TransportConfig{
			"test1": {
				Config: map[string]any{"some": "conf"},
			},
		},
	}, extraSetup...)

	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.TransportAPIFunctions{
		ConfigureTransport: func(ctx context.Context, ctr *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error) {
			assert.Equal(t, "test1", ctr.Name)
			assert.JSONEq(t, `{"some":"conf"}`, ctr.ConfigJson)
			return &prototk.ConfigureTransportResponse{}, nil
		},
	}

	registerTestTransport(t, tm, tp)
	return ctx, tm, tp, done
}

func registerTestTransport(t *testing.T, tm *transportManager, tp *testPlugin) {
	transportID := uuid.New()
	_, err := tm.TransportRegistered("test1", transportID, tp)
	require.NoError(t, err)

	ta := tm.transportsByName["test1"]
	assert.NotNil(t, ta)
	tp.t = ta
	tp.t.initRetry.UTSetMaxAttempts(1)
	<-tp.t.initDone
}

func TestDoubleRegisterReplaces(t *testing.T) {

	_, rm, tp0, done := newTestTransport(t, false)
	defer done()
	assert.Nil(t, tp0.t.initError.Load())
	assert.True(t, tp0.initialized.Load())

	// Register again
	tp1 := newTestPlugin(nil)
	tp1.Functions = tp0.Functions
	registerTestTransport(t, rm, tp1)
	assert.Nil(t, tp1.t.initError.Load())
	assert.True(t, tp1.initialized.Load())

	// Check we get the second from all the maps
	byName := rm.transportsByName[tp1.t.name]
	assert.Same(t, tp1.t, byName)
	byUUID := rm.transportsByID[tp1.t.id]
	assert.Same(t, tp1.t, byUUID)

}

func testMessage() *components.FireAndForgetMessageSend {
	return &components.FireAndForgetMessageSend{
		Node:          "node2",
		CorrelationID: confutil.P(uuid.New()),
		MessageType:   "myMessageType",
		Payload:       []byte("something"),
	}
}

func mockEmptyReliableMsgs(mc *mockComponents) components.TransportClient {
	mc.db.Mock.ExpectQuery("SELECT.*reliable_msgs").WillReturnRows(sqlmock.NewRows([]string{}))
	mc.db.Mock.MatchExpectationsInOrder(false)
	return nil
}

func mockActivateDeactivateOk(tp *testPlugin) {
	tp.Functions.ActivateNode = func(ctx context.Context, anr *prototk.ActivateNodeRequest) (*prototk.ActivateNodeResponse, error) {
		return &prototk.ActivateNodeResponse{PeerInfoJson: `{"endpoint":"some.url"}`}, nil
	}
	tp.Functions.DeactivateNode = func(ctx context.Context, dnr *prototk.DeactivateNodeRequest) (*prototk.DeactivateNodeResponse, error) {
		return &prototk.DeactivateNodeResponse{}, nil
	}
}

func mockGoodTransport(mc *mockComponents) components.TransportClient {
	mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return([]*components.RegistryNodeTransportEntry{
		{
			Node:      "node2",
			Transport: "test1",
			Details:   `{"likely":"json stuff"}`,
		},
	}, nil)
	return nil
}

func TestSendMessage(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockEmptyReliableMsgs,
		mockGoodTransport)
	defer done()

	message := testMessage()

	sentMessages := make(chan *prototk.PaladinMsg, 1)
	mockActivateDeactivateOk(tp)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sent := req.Message
		assert.NotEmpty(t, sent.MessageId)
		assert.Equal(t, message.CorrelationID.String(), *sent.CorrelationId)
		assert.Equal(t, message.Payload, sent.Payload)
		sentMessages <- sent
		return nil, nil
	}

	err := tm.Send(ctx, message)
	require.NoError(t, err)

	<-sentMessages
}

func TestSendMessageNotInit(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockEmptyReliableMsgs,
		func(mc *mockComponents) components.TransportClient {
			mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return([]*components.RegistryNodeTransportEntry{
				{
					Node:      "node1",
					Transport: "test1",
					Details:   `{"likely":"json stuff"}`,
				},
			}, nil)
			return nil
		})
	defer done()

	tp.t.initialized.Store(false)

	message := testMessage()

	mockActivateDeactivateOk(tp)
	err := tm.Send(ctx, message)
	assert.Regexp(t, "PD011601", err)

}

func TestSendMessageFail(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockEmptyReliableMsgs,
		func(mc *mockComponents) components.TransportClient {
			mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return([]*components.RegistryNodeTransportEntry{
				{
					Node:      "node1",
					Transport: "test1",
					Details:   `{"likely":"json stuff"}`,
				},
			}, nil)
			return nil
		})
	defer done()

	sent := make(chan struct{})
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		close(sent)
		return nil, fmt.Errorf("pop")
	}

	message := testMessage()

	mockActivateDeactivateOk(tp)
	err := tm.Send(ctx, message)
	assert.NoError(t, err)
	<-sent

}

func TestSendMessageDestNotFound(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false, func(mc *mockComponents) components.TransportClient {
		mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return(nil, fmt.Errorf("not found"))
		return nil
	})
	defer done()

	message := testMessage()

	err := tm.Send(ctx, message)
	assert.Regexp(t, "not found", err)

}

func TestSendMessageDestNotAvailable(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false, func(mc *mockComponents) components.TransportClient {
		mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return([]*components.RegistryNodeTransportEntry{
			{
				Node:      "node1",
				Transport: "another",
				Details:   `{"not":"the stuff we need"}`,
			},
		}, nil)
		return nil
	})
	defer done()

	message := testMessage()

	err := tm.Send(ctx, message)
	assert.Regexp(t, "PD012003.*another", err)

	_, err = tp.t.GetTransportDetails(ctx, &prototk.GetTransportDetailsRequest{
		Node: "node2",
	})
	assert.Regexp(t, "PD012004", err)

	_, err = tp.t.GetTransportDetails(ctx, &prototk.GetTransportDetailsRequest{
		Node: "node1",
	})
	assert.Regexp(t, "PD012009", err)

}

func TestGetTransportDetailsOk(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t, false, func(mc *mockComponents) components.TransportClient {
		mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return([]*components.RegistryNodeTransportEntry{
			{
				Node:      "node1",
				Transport: "test1",
				Details:   `{"the":"stuff we need"}`,
			},
		}, nil)
		return nil
	})
	defer done()

	tspt, err := tp.t.GetTransportDetails(ctx, &prototk.GetTransportDetailsRequest{
		Node: "node2",
	})
	assert.NoError(t, err)
	require.NotEmpty(t, tspt.TransportDetails)

}

func TestSendMessageDestWrong(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false)
	defer done()

	message := testMessage()

	message.Component = prototk.PaladinMsg_TRANSACTION_ENGINE
	message.Node = ""
	err := tm.Send(ctx, message)
	assert.Regexp(t, "PD012016", err)

	message.Component = prototk.PaladinMsg_TRANSACTION_ENGINE
	message.Node = "node1"
	err = tm.Send(ctx, message)
	assert.Regexp(t, "PD012007", err)

}

func TestSendInvalidMessageNoPayload(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false)
	defer done()

	message := &components.FireAndForgetMessageSend{}

	err := tm.Send(ctx, message)
	assert.Regexp(t, "PD012000", err)
}

func TestReceiveMessage(t *testing.T) {
	receivedMessages := make(chan *prototk.PaladinMsg, 1)

	ctx, _, tp, done := newTestTransport(t, false, func(mc *mockComponents) components.TransportClient {
		receivingClient := componentmocks.NewTransportClient(t)
		receivingClient.On("Destination").Return(prototk.PaladinMsg_TRANSACTION_ENGINE)
		receivingClient.On("HandlePaladinMsg", mock.Anything, mock.Anything).Return().Run(func(args mock.Arguments) {
			receivedMessages <- args[1].(*prototk.PaladinMsg)
		})
		return receivingClient
	})
	defer done()

	msg := &prototk.PaladinMsg{
		MessageId:     uuid.NewString(),
		CorrelationId: confutil.P(uuid.NewString()),
		Component:     prototk.PaladinMsg_TRANSACTION_ENGINE,
		MessageType:   "myMessageType",
		Payload:       []byte("some data"),
	}

	rmr, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: msg,
	})
	require.NoError(t, err)
	assert.NotNil(t, rmr)

	<-receivedMessages
}

func TestReceiveMessageNoReceiver(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t, false)
	defer done()

	msg := &prototk.PaladinMsg{
		MessageId:     uuid.NewString(),
		CorrelationId: confutil.P(uuid.NewString()),
		Component:     prototk.PaladinMsg_TRANSACTION_ENGINE,
		MessageType:   "myMessageType",
		Payload:       []byte("some data"),
	}

	_, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: msg,
	})
	require.Regexp(t, "PD012011", err)
}

func TestReceiveMessageInvalidDestination(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t, false)
	defer done()

	msg := &prototk.PaladinMsg{
		MessageId:     uuid.NewString(),
		CorrelationId: confutil.P(uuid.NewString()),
		Component:     prototk.PaladinMsg_Component(42),
		MessageType:   "myMessageType",
		Payload:       []byte("some data"),
	}

	_, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: msg,
	})
	require.Regexp(t, "PD012011", err)
}

func TestReceiveMessageNotInit(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t, false)
	defer done()

	tp.t.initialized.Store(false)

	msg := &prototk.PaladinMsg{
		MessageId:     uuid.NewString(),
		CorrelationId: confutil.P(uuid.NewString()),
		Component:     prototk.PaladinMsg_TRANSACTION_ENGINE,
		MessageType:   "myMessageType",
		Payload:       []byte("some data"),
	}
	_, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: msg,
	})
	assert.Regexp(t, "PD011601", err)
}

func TestReceiveMessageNoPayload(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t, false)
	defer done()

	msg := &prototk.PaladinMsg{}
	_, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: msg,
	})
	assert.Regexp(t, "PD012000", err)
}

func TestReceiveMessageBadDestination(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t, false)
	defer done()

	msg := &prototk.PaladinMsg{
		MessageId:   uuid.NewString(),
		Component:   prototk.PaladinMsg_Component(42),
		MessageType: "myMessageType",
		Payload:     []byte("some data"),
	}
	_, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: msg,
	})
	assert.Regexp(t, "PD012011", err)
}

func TestReceiveMessageBadMsgID(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t, false)
	defer done()

	msg := &prototk.PaladinMsg{
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		MessageType: "myMessageType",
		Payload:     []byte("some data"),
	}
	_, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: msg,
	})
	assert.Regexp(t, "PD012000", err)
}

func TestReceiveMessageBadCorrelID(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t, false)
	defer done()

	msg := &prototk.PaladinMsg{
		MessageId:     uuid.NewString(),
		CorrelationId: confutil.P("wrong"),
		Component:     prototk.PaladinMsg_TRANSACTION_ENGINE,
		MessageType:   "myMessageType",
		Payload:       []byte("some data"),
	}
	_, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: msg,
	})
	assert.Regexp(t, "PD012000", err)
}

func TestSendContextClosed(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false)
	done()

	tm.peers = map[string]*peer{
		"node2": {
			transport: tp.t,
			sendQueue: make(chan *prototk.PaladinMsg),
		},
	}

	err := tm.Send(ctx, testMessage())
	assert.Regexp(t, "PD010301", err)

}

func TestSendReliableOk(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		func(mc *mockComponents) components.TransportClient {
			mc.db.Mock.ExpectExec("INSERT.*reliable_msgs").WillReturnResult(driver.ResultNoRows)
			return nil
		},
	)
	defer done()

	mockActivateDeactivateOk(tp)
	pc, err := tm.SendReliable(ctx, tm.persistence.DB(), &components.ReliableMessage{
		Node:        "node2",
		MessageType: components.RMTState.Enum(),
		Metadata:    []byte(`{"some":"data"}`),
	})
	require.NoError(t, err)
	pc()

}

func TestSendReliableFail(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		func(mc *mockComponents) components.TransportClient {
			mc.db.Mock.ExpectExec("INSERT.*reliable_msgs").WillReturnError(fmt.Errorf("pop"))
			return nil
		},
	)
	defer done()

	mockActivateDeactivateOk(tp)
	_, err := tm.SendReliable(ctx, tm.persistence.DB(), &components.ReliableMessage{
		Node:        "node2",
		MessageType: components.RMTState.Enum(),
		Metadata:    []byte(`{"some":"data"}`),
	})
	require.Regexp(t, "pop", err)

}
