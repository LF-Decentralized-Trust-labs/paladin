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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

func newTestTransport(t *testing.T, extraSetup ...func(mc *mockComponents)) (context.Context, *transportManager, *testPlugin, func()) {
	log.SetLevel("trace")

	ctx, tm, _, done := newTestTransportManager(t, &TransportManagerConfig{
		NodeName: "node1",
		Transports: map[string]*TransportConfig{
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
	assert.NoError(t, err)

	ta := tm.transportsByName["test1"]
	assert.NotNil(t, ta)
	tp.t = ta
	tp.t.initRetry.UTSetMaxAttempts(1)
	<-tp.t.initDone
}

func TestDoubleRegisterReplaces(t *testing.T) {

	_, rm, tp0, done := newTestTransport(t)
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

func testMessage() *components.TransportMessage {
	return &components.TransportMessage{
		Destination:   "me@node2",
		ReplyTo:       "me@node1",
		CorrelationID: confutil.P(uuid.New()),
		MessageType:   "myMessageType",
		Payload:       []byte("something"),
	}
}

func TestSendMessage(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, func(mc *mockComponents) {
		mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return([]*components.RegistryNodeTransportEntry{
			{
				Node:             "node2",
				Transport:        "test1",
				TransportDetails: `{"likely":"json stuff"}`,
			},
		}, nil)
	})
	defer done()

	message := testMessage()

	sentMessages := make(chan *prototk.Message, 1)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sent := req.Message
		assert.NotEmpty(t, sent.MessageId)
		assert.Equal(t, message.CorrelationID.String(), *sent.CorrelationId)
		assert.Equal(t, message.Destination.String(), sent.Destination)
		assert.Equal(t, message.ReplyTo.String(), sent.ReplyTo)
		assert.Equal(t, message.Payload, sent.Payload)

		// ... if we didn't have a connection established we'd expect to come back to request the details
		gtdr, err := tp.t.GetTransportDetails(ctx, &prototk.GetTransportDetailsRequest{
			Destination: message.Destination.String(),
		})
		assert.NoError(t, err)
		assert.NotEmpty(t, gtdr.TransportDetails)

		sentMessages <- sent
		return nil, nil
	}

	err := tm.Send(ctx, message)
	assert.NoError(t, err)

	<-sentMessages
}

func TestSendMessageNotInit(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, func(mc *mockComponents) {
		mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return([]*components.RegistryNodeTransportEntry{
			{
				Node:             "node1",
				Transport:        "test1",
				TransportDetails: `{"likely":"json stuff"}`,
			},
		}, nil)
	})
	defer done()

	tp.t.initialized.Store(false)

	message := testMessage()

	err := tm.Send(ctx, message)
	assert.Regexp(t, "PD011601", err)

}

func TestSendMessageFail(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, func(mc *mockComponents) {
		mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return([]*components.RegistryNodeTransportEntry{
			{
				Node:             "node1",
				Transport:        "test1",
				TransportDetails: `{"likely":"json stuff"}`,
			},
		}, nil)
	})
	defer done()

	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	message := testMessage()

	err := tm.Send(ctx, message)
	assert.Regexp(t, "pop", err)

}

func TestSendMessageDestNotFound(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, func(mc *mockComponents) {
		mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return(nil, fmt.Errorf("not found"))
	})
	defer done()

	message := testMessage()

	err := tm.Send(ctx, message)
	assert.Regexp(t, "not found", err)

}

func TestSendMessageDestNotAvailable(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, func(mc *mockComponents) {
		mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return([]*components.RegistryNodeTransportEntry{
			{
				Node:             "node1",
				Transport:        "another",
				TransportDetails: `{"not":"the stuff we need"}`,
			},
		}, nil)
	})
	defer done()

	message := testMessage()

	err := tm.Send(ctx, message)
	assert.Regexp(t, "PD012003.*another", err)

	_, err = tp.t.GetTransportDetails(ctx, &prototk.GetTransportDetailsRequest{
		Destination: message.Destination.String(),
	})
	assert.Regexp(t, "PD012004", err)

	_, err = tp.t.GetTransportDetails(ctx, &prototk.GetTransportDetailsRequest{
		Destination: "no_node",
	})
	assert.Regexp(t, "PD012007", err)

}

func TestSendMessageDestWrong(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t)
	defer done()

	message := testMessage()

	message.Destination = types.PrivateIdentityLocator("no_node")
	err := tm.Send(ctx, message)
	assert.Regexp(t, "PD012007", err)

	message.Destination = types.PrivateIdentityLocator("this_is_local@node1")
	err = tm.Send(ctx, message)
	assert.Regexp(t, "PD012007", err)

	message.Destination = types.PrivateIdentityLocator("ok@node2")
	message.ReplyTo = types.PrivateIdentityLocator("wrong@@syntax")
	err = tm.Send(ctx, message)
	assert.Regexp(t, "PD012006", err)

}

func TestSendInvalidMessageNoPayload(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t)
	defer done()

	message := &components.TransportMessage{}

	err := tm.Send(ctx, message)
	assert.Regexp(t, "PD012000", err)
}

func TestReceiveMessage(t *testing.T) {
	receivedMessages := make(chan *components.TransportMessage, 1)

	ctx, _, tp, done := newTestTransport(t, func(mc *mockComponents) {
		mc.engine.On("ReceiveTransportMessage", mock.Anything, mock.Anything).Return().Run(func(args mock.Arguments) {
			receivedMessages <- args[1].(*components.TransportMessage)
		})
	})
	defer done()

	msg := &prototk.Message{
		MessageId:     uuid.NewString(),
		CorrelationId: confutil.P(uuid.NewString()),
		Destination:   "to@node1",
		ReplyTo:       "from@node2",
		MessageType:   "myMessageType",
		Payload:       []byte("some data"),
	}
	rmr, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: msg,
	})
	assert.NoError(t, err)
	assert.NotNil(t, rmr)

	<-receivedMessages
}

func TestReceiveMessageNotInit(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t)
	defer done()

	tp.t.initialized.Store(false)

	msg := &prototk.Message{
		MessageId:     uuid.NewString(),
		CorrelationId: confutil.P(uuid.NewString()),
		Destination:   "to@node1",
		ReplyTo:       "from@node2",
		MessageType:   "myMessageType",
		Payload:       []byte("some data"),
	}
	_, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: msg,
	})
	assert.Regexp(t, "PD011601", err)
}

func TestReceiveMessageNoPayload(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t)
	defer done()

	msg := &prototk.Message{}
	_, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: msg,
	})
	assert.Regexp(t, "PD012000", err)
}

func TestReceiveMessageWrongNode(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t)
	defer done()

	msg := &prototk.Message{
		Destination: "to@node2",
		ReplyTo:     "from@node2",
		MessageType: "myMessageType",
		Payload:     []byte("some data"),
	}
	_, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: msg,
	})
	assert.Regexp(t, "PD012005", err)
}

func TestReceiveMessageBadDestination(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t)
	defer done()

	msg := &prototk.Message{
		MessageId:   uuid.NewString(),
		Destination: "to@node2",
		ReplyTo:     "from@node1",
		MessageType: "myMessageType",
		Payload:     []byte("some data"),
	}
	_, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: msg,
	})
	assert.Regexp(t, "PD012005", err)
}

func TestReceiveMessageBadReplyToDestination(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t)
	defer done()

	msg := &prototk.Message{
		MessageId:   uuid.NewString(),
		Destination: "to@node1",
		ReplyTo:     "from",
		MessageType: "myMessageType",
		Payload:     []byte("some data"),
	}
	_, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: msg,
	})
	assert.Regexp(t, "PD012006", err)
}

func TestReceiveMessageBadMsgID(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t)
	defer done()

	msg := &prototk.Message{
		Destination: "to@node1",
		ReplyTo:     "from@node2",
		MessageType: "myMessageType",
		Payload:     []byte("some data"),
	}
	_, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: msg,
	})
	assert.Regexp(t, "PD012000", err)
}

func TestReceiveMessageBadCorrelID(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t)
	defer done()

	msg := &prototk.Message{
		MessageId:     uuid.NewString(),
		CorrelationId: confutil.P("wrong"),
		Destination:   "to@node1",
		ReplyTo:       "from@node2",
		MessageType:   "myMessageType",
		Payload:       []byte("some data"),
	}
	_, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: msg,
	})
	assert.Regexp(t, "PD012000", err)
}
