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

package registrymgr

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
	plugintk.RegistryAPIBase
	initialized  atomic.Bool
	t            *registry
	sendMessages chan *prototk.SendMessageRequest
}

func (tp *testPlugin) Initialized() {
	tp.initialized.Store(true)
}

func newTestPlugin(registryFuncs *plugintk.RegistryAPIFunctions) *testPlugin {
	return &testPlugin{
		RegistryAPIBase: plugintk.RegistryAPIBase{
			Functions: registryFuncs,
		},
		sendMessages: make(chan *prototk.SendMessageRequest, 1),
	}
}

func newTestRegistry(t *testing.T, registryConfig *prototk.RegistryConfig, extraSetup ...func(mc *mockComponents)) (context.Context, *registryManager, *testPlugin, func()) {
	ctx, tm, _, done := newTestRegistryManager(t, &RegistryManagerConfig{
		Registries: map[string]*RegistryConfig{
			"test1": {
				Config: yamlNode(t, `{"some":"conf"}`),
			},
		},
	}, extraSetup...)

	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.RegistryAPIFunctions{
		ConfigureRegistry: func(ctx context.Context, ctr *prototk.ConfigureRegistryRequest) (*prototk.ConfigureRegistryResponse, error) {
			assert.Equal(t, "test1", ctr.Name)
			assert.YAMLEq(t, `{"some":"conf"}`, ctr.ConfigYaml)
			return &prototk.ConfigureRegistryResponse{
				RegistryConfig: registryConfig,
			}, nil
		},
		InitRegistry: func(ctx context.Context, idr *prototk.InitRegistryRequest) (*prototk.InitRegistryResponse, error) {
			return &prototk.InitRegistryResponse{}, nil
		},
		SendMessage: func(ctx context.Context, smr *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
			tp.sendMessages <- smr
			return &prototk.SendMessageResponse{}, nil
		},
	}

	registerTestRegistry(t, tm, tp)
	return ctx, tm, tp, done
}

func registerTestRegistry(t *testing.T, tm *registryManager, tp *testPlugin) {
	registryID := uuid.New()
	_, err := tm.RegistryRegistered("test1", registryID, tp)
	assert.NoError(t, err)

	ta, err := tm.GetRegistryByName(context.Background(), "test1")
	assert.NoError(t, err)
	tp.t = ta.(*registry)
	tp.t.initRetry.UTSetMaxAttempts(1)
	<-tp.t.initDone
}

func TestSendMessage(t *testing.T) {
	ctx, _, tp0, done := newTestRegistry(t, &prototk.RegistryConfig{}, func(mc *mockComponents) {})
	defer done()

	message := &components.RegistryMessage{
		Node:    "node1",
		Payload: []byte("something"),
	}
	serializedMessage, err := yaml.Marshal(message)
	assert.NoError(t, err)

	registryDetails := ""

	err = tp0.t.Send(ctx, string(serializedMessage), registryDetails)
	assert.NoError(t, err)

	<-tp0.sendMessages
}

func TestRecieveMessages(t *testing.T) {
	ctx, tm, tp0, done := newTestRegistry(t, &prototk.RegistryConfig{}, func(mc *mockComponents) {})
	defer done()

	message := &components.RegistryMessage{
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
	ctx, tm, tp0, done := newTestRegistry(t, &prototk.RegistryConfig{}, func(mc *mockComponents) {})
	defer done()

	message := &components.RegistryMessage{
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
