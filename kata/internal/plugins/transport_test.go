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
package plugins

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

type testTransportManager struct {
	transports          map[string]plugintk.Plugin
	transportRegistered func(name string, id uuid.UUID, toTransport TransportManagerToTransport) (fromTransport plugintk.TransportCallbacks, err error)
	receive             func(context.Context, *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error)
}

func (tp *testTransportManager) ConfiguredTransports() map[string]*PluginConfig {
	pluginMap := make(map[string]*PluginConfig)
	for name := range tp.transports {
		pluginMap[name] = &PluginConfig{
			Type:     LibraryTypeCShared.Enum(),
			Location: "/tmp/not/applicable",
		}
	}
	return pluginMap
}

func (tp *testTransportManager) Receive(ctx context.Context, req *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
	return tp.receive(ctx, req)
}

func (tdm *testTransportManager) TransportRegistered(name string, id uuid.UUID, toTransport TransportManagerToTransport) (fromTransport plugintk.TransportCallbacks, err error) {
	return tdm.transportRegistered(name, id, toTransport)
}

func TestTransportRequestsOK(t *testing.T) {

	waitForAPI := make(chan TransportManagerToTransport, 1)
	waitForCallbacks := make(chan plugintk.TransportCallbacks, 1)

	var transportID string
	transportFunctions := &plugintk.TransportAPIFunctions{
		ConfigureTransport: func(ctx context.Context, cdr *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error) {
			return &prototk.ConfigureTransportResponse{
				TransportConfig: &prototk.TransportConfig{},
			}, nil
		},
		InitTransport: func(ctx context.Context, idr *prototk.InitTransportRequest) (*prototk.InitTransportResponse, error) {
			assert.Equal(t, transportID, idr.TransportUuid)
			return &prototk.InitTransportResponse{}, nil
		},
	}

	tdm := &testTransportManager{
		transports: map[string]plugintk.Plugin{
			"transport1": plugintk.NewTransport(func(callbacks plugintk.TransportCallbacks) plugintk.TransportAPI {
				waitForCallbacks <- callbacks
				return &plugintk.TransportAPIBase{Functions: transportFunctions}
			}),
		},
	}
	tdm.transportRegistered = func(name string, id uuid.UUID, toTransport TransportManagerToTransport) (plugintk.TransportCallbacks, error) {
		assert.Equal(t, "transport1", name)
		transportID = id.String()
		waitForAPI <- toTransport
		return tdm, nil
	}

	tdm.receive = func(ctx context.Context, req *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
		return &prototk.ReceiveMessageResponse{
			// States: []*prototk.StoredState{
			// 	{Id: "12345"},
			// },
		}, nil
	}

	ctx, pc, done := newTestTransportPluginController(t, &testManagers{
		testTransportManager: tdm,
	})
	defer done()

	transportAPI := <-waitForAPI

	_, err := transportAPI.ConfigureTransport(ctx, &prototk.ConfigureTransportRequest{})
	assert.NoError(t, err)

	_, err = transportAPI.InitTransport(ctx, &prototk.InitTransportRequest{
		TransportUuid: transportID,
	})
	assert.NoError(t, err)

	// This is the point the transport manager would call us to say the transport is initialized
	// (once it's happy it's updated its internal state)
	transportAPI.Initialized()
	assert.NoError(t, pc.WaitForInit(ctx))
}
