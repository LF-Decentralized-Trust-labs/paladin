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
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
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

func newTestTransport(t *testing.T, realDB bool, extraSetup ...func(mc *mockComponents, conf *pldconf.TransportManagerConfig)) (context.Context, *transportManager, *testPlugin, func()) {

	conf := &pldconf.TransportManagerConfig{
		NodeName: "node1",
		Transports: map[string]*pldconf.TransportConfig{
			"test1": {
				Config: map[string]any{"some": "conf"},
			},
		},
		ReliableMessageWriter: pldconf.FlushWriterConfig{
			BatchMaxSize: confutil.P(1),
		},
	}
	ctx, tm, _, done := newTestTransportManager(t, realDB, conf, extraSetup...)

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

func mockEmptyReliableMsgs(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
	mc.db.Mock.ExpectQuery("SELECT.*reliable_msgs").WillReturnRows(sqlmock.NewRows([]string{}))
	mc.db.Mock.MatchExpectationsInOrder(false)
}

func mockActivateDeactivateOk(tp *testPlugin) {
	tp.Functions.ActivatePeer = func(ctx context.Context, anr *prototk.ActivatePeerRequest) (*prototk.ActivatePeerResponse, error) {
		return &prototk.ActivatePeerResponse{PeerInfoJson: `{"endpoint":"some.url"}`}, nil
	}
	tp.Functions.DeactivatePeer = func(ctx context.Context, dnr *prototk.DeactivatePeerRequest) (*prototk.DeactivatePeerResponse, error) {
		return &prototk.DeactivatePeerResponse{}, nil
	}
}

func mockGoodTransport(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
	mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return([]*components.RegistryNodeTransportEntry{
		{
			Node:      "node2",
			Transport: "test1",
			Details:   `{"likely":"json stuff"}`,
		},
	}, nil)
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
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
			mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return([]*components.RegistryNodeTransportEntry{
				{
					Node:      "node1",
					Transport: "test1",
					Details:   `{"likely":"json stuff"}`,
				},
			}, nil)
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
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
			mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return([]*components.RegistryNodeTransportEntry{
				{
					Node:      "node1",
					Transport: "test1",
					Details:   `{"likely":"json stuff"}`,
				},
			}, nil)
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
	ctx, tm, _, done := newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
		mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return(nil, fmt.Errorf("not found"))
	})
	defer done()

	message := testMessage()

	err := tm.Send(ctx, message)
	assert.Regexp(t, "not found", err)

}

func TestSendMessageDestNotAvailable(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
		mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return([]*components.RegistryNodeTransportEntry{
			{
				Node:      "node1",
				Transport: "another",
				Details:   `{"not":"the stuff we need"}`,
			},
		}, nil)
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
	ctx, _, tp, done := newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
		mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return([]*components.RegistryNodeTransportEntry{
			{
				Node:      "node1",
				Transport: "test1",
				Details:   `{"the":"stuff we need"}`,
			},
		}, nil)
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
	assert.Regexp(t, "PD012015", err)

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

func TestReceiveMessageTransactionEngine(t *testing.T) {
	receivedMessages := make(chan *components.ReceivedMessage, 1)

	ctx, _, tp, done := newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
		mc.privateTxManager.On("HandlePaladinMsg", mock.Anything, mock.Anything).Return().Run(func(args mock.Arguments) {
			receivedMessages <- args[1].(*components.ReceivedMessage)
		})
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
		FromNode: "node2",
		Message:  msg,
	})
	require.NoError(t, err)
	assert.NotNil(t, rmr)

	<-receivedMessages
}

func TestReceiveMessageIdentityResolver(t *testing.T) {
	receivedMessages := make(chan *components.ReceivedMessage, 1)

	ctx, _, tp, done := newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
		mc.identityResolver.On("HandlePaladinMsg", mock.Anything, mock.Anything).Return().Run(func(args mock.Arguments) {
			receivedMessages <- args[1].(*components.ReceivedMessage)
		})
	})
	defer done()

	msg := &prototk.PaladinMsg{
		MessageId:     uuid.NewString(),
		CorrelationId: confutil.P(uuid.NewString()),
		Component:     prototk.PaladinMsg_IDENTITY_RESOLVER,
		MessageType:   "myMessageType",
		Payload:       []byte("some data"),
	}

	rmr, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		FromNode: "node2",
		Message:  msg,
	})
	require.NoError(t, err)
	assert.NotNil(t, rmr)

	<-receivedMessages
}

func TestReceiveMessageInvalidComponent(t *testing.T) {
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
		FromNode: "node2",
		Message:  msg,
	})
	require.Regexp(t, "PD012011", err)
}

func TestReceiveMessageInvalidNode(t *testing.T) {
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
		FromNode: ".wrong",
		Message:  msg,
	})
	require.Regexp(t, "PD012015", err)
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
		FromNode: "node2",
		Message:  msg,
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

	p := &peer{
		transport: tp.t,
		sendQueue: make(chan *prototk.PaladinMsg),
	}
	tm.peers = map[string]*peer{
		"node2": p,
	}
	p.senderStarted.Store(true)

	err := tm.Send(ctx, testMessage())
	assert.Regexp(t, "PD010301", err)

}

func TestSendReliableOk(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectQuery("INSERT.*reliable_msgs").WillReturnRows(sqlmock.NewRows([]string{"sequence"}).AddRow(12345))
			mc.db.Mock.ExpectCommit()
		},
	)
	defer done()

	mockActivateDeactivateOk(tp)
	err := tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return tm.SendReliable(ctx, dbTX, &pldapi.ReliableMessage{
			Node:        "node2",
			MessageType: pldapi.RMTState.Enum(),
			Metadata:    []byte(`{"some":"data"}`),
		})
	})
	require.NoError(t, err)

}

func TestSendReliableFail(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectExec("INSERT.*reliable_msgs").WillReturnError(fmt.Errorf("pop"))
		},
	)
	defer done()

	mockActivateDeactivateOk(tp)
	err := tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return tm.SendReliable(ctx, dbTX, &pldapi.ReliableMessage{
			Node:        "node2",
			MessageType: pldapi.RMTState.Enum(),
			Metadata:    []byte(`{"some":"data"}`),
		})
	})
	require.Regexp(t, "pop", err)

}
