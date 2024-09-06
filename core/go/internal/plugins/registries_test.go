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
	"fmt"
	"os"
	"runtime/debug"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type testRegistryManager struct {
	registries         map[string]plugintk.Plugin
	registryRegistered func(name string, id uuid.UUID, toRegistry components.RegistryManagerToRegistry) (fromRegistry plugintk.RegistryCallbacks, err error)

	upsertTransportDetails func(ctx context.Context, req *prototk.UpsertTransportDetails) (*prototk.UpsertTransportDetailsResponse, error)
}

func registryConnectFactory(ctx context.Context, client prototk.PluginControllerClient) (grpc.BidiStreamingClient[prototk.RegistryMessage, prototk.RegistryMessage], error) {
	return client.ConnectRegistry(context.Background())
}

func registryHeaderAccessor(msg *prototk.RegistryMessage) *prototk.Header {
	if msg.Header == nil {
		msg.Header = &prototk.Header{}
	}
	return msg.Header
}

func (tp *testRegistryManager) mock(t *testing.T) *componentmocks.RegistryManager {
	mdm := componentmocks.NewRegistryManager(t)
	pluginMap := make(map[string]*components.PluginConfig)
	for name := range tp.registries {
		pluginMap[name] = &components.PluginConfig{
			Type:    components.LibraryTypeCShared.Enum(),
			Library: "/tmp/not/applicable",
		}
	}
	mdm.On("ConfiguredRegistries").Return(pluginMap).Maybe()
	mdr := mdm.On("RegistryRegistered", mock.Anything, mock.Anything, mock.Anything).Maybe()
	mdr.Run(func(args mock.Arguments) {
		m2p, err := tp.registryRegistered(args[0].(string), args[1].(uuid.UUID), args[2].(components.RegistryManagerToRegistry))
		mdr.Return(m2p, err)
	})
	return mdm
}

func (tdm *testRegistryManager) UpsertTransportDetails(ctx context.Context, req *prototk.UpsertTransportDetails) (*prototk.UpsertTransportDetailsResponse, error) {
	return tdm.upsertTransportDetails(ctx, req)
}

func newTestRegistryPluginManager(t *testing.T, setup *testManagers) (context.Context, *pluginManager, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	pc := newTestPluginManager(t, setup)

	tpl, err := NewUnitTestPluginLoader(pc.GRPCTargetURL(), pc.loaderID.String(), setup.allPlugins())
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		tpl.Run()
	}()

	return ctx, pc, func() {
		recovered := recover()
		if recovered != nil {
			fmt.Fprintf(os.Stderr, "%v: %s", recovered, debug.Stack())
			panic(recovered)
		}
		cancelCtx()
		pc.Stop()
		tpl.Stop()
		<-done
	}

}

func TestRegistryRequestsOK(t *testing.T) {

	waitForAPI := make(chan components.RegistryManagerToRegistry, 1)
	waitForCallbacks := make(chan plugintk.RegistryCallbacks, 1)

	registryFunctions := &plugintk.RegistryAPIFunctions{
		ConfigureRegistry: func(ctx context.Context, cdr *prototk.ConfigureRegistryRequest) (*prototk.ConfigureRegistryResponse, error) {
			return &prototk.ConfigureRegistryResponse{}, nil
		},
	}

	trm := &testRegistryManager{
		registries: map[string]plugintk.Plugin{
			"registry1": plugintk.NewRegistry(func(callbacks plugintk.RegistryCallbacks) plugintk.RegistryAPI {
				waitForCallbacks <- callbacks
				return &plugintk.RegistryAPIBase{Functions: registryFunctions}
			}),
		},
		upsertTransportDetails: func(ctx context.Context, req *prototk.UpsertTransportDetails) (*prototk.UpsertTransportDetailsResponse, error) {
			assert.Equal(t, "node1", req.Node)
			return &prototk.UpsertTransportDetailsResponse{}, nil
		},
	}
	trm.registryRegistered = func(name string, id uuid.UUID, toRegistry components.RegistryManagerToRegistry) (plugintk.RegistryCallbacks, error) {
		assert.Equal(t, "registry1", name)
		waitForAPI <- toRegistry
		return trm, nil
	}

	ctx, pc, done := newTestRegistryPluginManager(t, &testManagers{
		testRegistryManager: trm,
	})
	defer done()

	registryAPI := <-waitForAPI

	_, err := registryAPI.ConfigureRegistry(ctx, &prototk.ConfigureRegistryRequest{})
	require.NoError(t, err)

	// This is the point the registry manager would call us to say the registry is initialized
	// (once it's happy it's updated its internal state)
	registryAPI.Initialized()
	require.NoError(t, pc.WaitForInit(ctx))

	callbacks := <-waitForCallbacks

	utr, err := callbacks.UpsertTransportDetails(ctx, &prototk.UpsertTransportDetails{
		Node: "node1",
	})
	require.NoError(t, err)
	assert.NotNil(t, utr)

}

func TestRegistryRegisterFail(t *testing.T) {

	waitForError := make(chan error, 1)

	tdm := &testRegistryManager{
		registries: map[string]plugintk.Plugin{
			"registry1": &mockPlugin[prototk.RegistryMessage]{
				t:              t,
				connectFactory: registryConnectFactory,
				headerAccessor: registryHeaderAccessor,
				preRegister: func(registryID string) *prototk.RegistryMessage {
					return &prototk.RegistryMessage{
						Header: &prototk.Header{
							MessageType: prototk.Header_REGISTER,
							PluginId:    registryID,
							MessageId:   uuid.NewString(),
						},
					}
				},
				expectClose: func(err error) {
					waitForError <- err
				},
			},
		},
	}
	tdm.registryRegistered = func(name string, id uuid.UUID, toRegistry components.RegistryManagerToRegistry) (plugintk.RegistryCallbacks, error) {
		return nil, fmt.Errorf("pop")
	}

	_, _, done := newTestRegistryPluginManager(t, &testManagers{
		testRegistryManager: tdm,
	})
	defer done()

	assert.Regexp(t, "pop", <-waitForError)
}

func TestFromRegistryRequestBadReq(t *testing.T) {

	waitForResponse := make(chan struct{}, 1)

	msgID := uuid.NewString()
	trm := &testRegistryManager{
		registries: map[string]plugintk.Plugin{
			"registry1": &mockPlugin[prototk.RegistryMessage]{
				t:              t,
				connectFactory: registryConnectFactory,
				headerAccessor: registryHeaderAccessor,
				sendRequest: func(pluginID string) *prototk.RegistryMessage {
					return &prototk.RegistryMessage{
						Header: &prototk.Header{
							PluginId:    pluginID,
							MessageId:   msgID,
							MessageType: prototk.Header_REQUEST_FROM_PLUGIN,
							// Missing payload
						},
					}
				},
				handleResponse: func(dm *prototk.RegistryMessage) {
					assert.Equal(t, msgID, *dm.Header.CorrelationId)
					assert.Regexp(t, "PD011203", *dm.Header.ErrorMessage)
					close(waitForResponse)
				},
			},
		},
	}
	trm.registryRegistered = func(name string, id uuid.UUID, toRegistry components.RegistryManagerToRegistry) (fromRegistry plugintk.RegistryCallbacks, err error) {
		return trm, nil
	}

	_, _, done := newTestRegistryPluginManager(t, &testManagers{
		testRegistryManager: trm,
	})
	defer done()

	<-waitForResponse

}
