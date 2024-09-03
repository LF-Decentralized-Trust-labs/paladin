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
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

type testPlugin struct {
	plugintk.TransportAPIBase
	initialized  atomic.Bool
	t            *transport
	sendMessages chan *prototk.SendMessageRequest
}

func (tp *testPlugin) Initialized() {
	tp.initialized.Store(true)
}

func newTestPlugin(transportFuncs *plugintk.TransportAPIFunctions) *testPlugin {
	return &testPlugin{
		TransportAPIBase: plugintk.TransportAPIBase{
			Functions: transportFuncs,
		},
		sendMessages: make(chan *prototk.SendMessageRequest, 1),
	}
}

func newTestTransport(t *testing.T, transportConfig *prototk.TransportConfig, extraSetup ...func(mc *mockComponents)) (context.Context, *transportManager, *testPlugin, func()) {
	ctx, tm, _, done := newTestTransportManager(t, &TransportManagerConfig{
		Transports: map[string]*TransportConfig{
			"test1": {
				Config: yamlNode(t, `{"some":"conf"}`),
			},
		},
	}, extraSetup...)

	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.TransportAPIFunctions{
		ConfigureTransport: func(ctx context.Context, ctr *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error) {
			assert.Equal(t, "test1", ctr.Name)
			assert.YAMLEq(t, `{"some":"conf"}`, ctr.ConfigYaml)
			return &prototk.ConfigureTransportResponse{
				TransportConfig: transportConfig,
			}, nil
		},
		InitTransport: func(ctx context.Context, idr *prototk.InitTransportRequest) (*prototk.InitTransportResponse, error) {
			return &prototk.InitTransportResponse{}, nil
		},
		SendMessage: func(ctx context.Context, smr *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
			tp.sendMessages <- smr
			return &prototk.SendMessageResponse{}, nil
		},
	}

	registerTestTransport(t, tm, tp)
	return ctx, tm, tp, done
}

func registerTestTransport(t *testing.T, tm *transportManager, tp *testPlugin) {
	transportID := uuid.New()
	_, err := tm.TransportRegistered("test1", transportID, tp)
	assert.NoError(t, err)

	ta, err := tm.GetTransportByName(context.Background(), "test1")
	assert.NoError(t, err)
	tp.t = ta.(*transport)
	tp.t.initRetry.UTSetMaxAttempts(1)
	<-tp.t.initDone
}

func TestSendMessage(t *testing.T) {
	ctx, _, tp0, done := newTestTransport(t, &prototk.TransportConfig{}, func(mc *mockComponents) {})
	defer done()

	message := &components.TransportMessage{
		MessageType: "something",
		Payload:     []byte("something"),
	}
	serializedMessage, err := yaml.Marshal(message)
	assert.NoError(t, err)

	transportDetails := ""

	err = tp0.t.Send(ctx, string(serializedMessage), transportDetails)
	assert.NoError(t, err)

	<-tp0.sendMessages
}

func TestRecieveMessages(t *testing.T) {
	ctx, tm, tp0, done := newTestTransport(t, &prototk.TransportConfig{}, func(mc *mockComponents) {})
	defer done()

	message := &components.TransportMessage{
		MessageType: "something",
		Payload:     []byte("something"),
	}
	serializedMessage, err := yaml.Marshal(message)
	assert.NoError(t, err)

	_, err = tp0.t.Receive(ctx, &prototk.ReceiveMessageRequest{
		Body: string(serializedMessage),
	})
	assert.NoError(t, err)

	<-tm.recvMessages
}

func TestRecieveMessagesFailsWhenNotInitialized(t *testing.T) {
	ctx, tm, tp0, done := newTestTransport(t, &prototk.TransportConfig{}, func(mc *mockComponents) {})
	defer done()

	message := &components.TransportMessage{
		MessageType: "something",
		Payload:     []byte("something"),
	}
	serializedMessage, err := yaml.Marshal(message)
	assert.NoError(t, err)

	_, err = tp0.t.Receive(ctx, &prototk.ReceiveMessageRequest{
		Body: string(serializedMessage),
	})
	assert.NoError(t, err)

	<-tm.recvMessages
}
