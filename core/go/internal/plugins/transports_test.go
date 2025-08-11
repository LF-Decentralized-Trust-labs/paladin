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
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type testTransportManager struct {
	transports          map[string]plugintk.Plugin
	transportRegistered func(name string, id uuid.UUID, toTransport components.TransportManagerToTransport) (fromTransport plugintk.TransportCallbacks, err error)
	resolveTarget       func(context.Context, *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error)
	receiveMessage      func(context.Context, *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error)
}

func transportConnectFactory(ctx context.Context, client prototk.PluginControllerClient) (grpc.BidiStreamingClient[prototk.TransportMessage, prototk.TransportMessage], error) {
	return client.ConnectTransport(context.Background())
}

func transportHeaderAccessor(msg *prototk.TransportMessage) *prototk.Header {
	if msg.Header == nil {
		msg.Header = &prototk.Header{}
	}
	return msg.Header

}

func (tp *testTransportManager) mock(t *testing.T) *componentsmocks.TransportManager {
	mdm := componentsmocks.NewTransportManager(t)
	pluginMap := make(map[string]*pldconf.PluginConfig)
	for name := range tp.transports {
		pluginMap[name] = &pldconf.PluginConfig{
			Type:    string(pldtypes.LibraryTypeCShared),
			Library: "/tmp/not/applicable",
		}
	}
	mdm.On("ConfiguredTransports").Return(pluginMap).Maybe()
	mdr := mdm.On("TransportRegistered", mock.Anything, mock.Anything, mock.Anything).Maybe()
	mdr.Run(func(args mock.Arguments) {
		m2p, err := tp.transportRegistered(args[0].(string), args[1].(uuid.UUID), args[2].(components.TransportManagerToTransport))
		mdr.Return(m2p, err)
	})
	return mdm
}

func (tp *testTransportManager) ReceiveMessage(ctx context.Context, req *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
	return tp.receiveMessage(ctx, req)
}

func (tp *testTransportManager) GetTransportDetails(ctx context.Context, req *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error) {
	return tp.resolveTarget(ctx, req)
}

func (tdm *testTransportManager) TransportRegistered(name string, id uuid.UUID, toTransport components.TransportManagerToTransport) (fromTransport plugintk.TransportCallbacks, err error) {
	return tdm.transportRegistered(name, id, toTransport)
}

func newTestTransportPluginManager(t *testing.T, setup *testManagers) (context.Context, *pluginManager, func()) {
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

func TestTransportRequestsOK(t *testing.T) {

	waitForAPI := make(chan components.TransportManagerToTransport, 1)
	waitForCallbacks := make(chan plugintk.TransportCallbacks, 1)

	transportFunctions := &plugintk.TransportAPIFunctions{
		ConfigureTransport: func(ctx context.Context, cdr *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error) {
			return &prototk.ConfigureTransportResponse{}, nil
		},
		SendMessage: func(ctx context.Context, smr *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
			assert.Equal(t, "type1", smr.Message.MessageType)
			return &prototk.SendMessageResponse{}, nil
		},
		GetLocalDetails: func(ctx context.Context, gldr *prototk.GetLocalDetailsRequest) (*prototk.GetLocalDetailsResponse, error) {
			return &prototk.GetLocalDetailsResponse{TransportDetails: "endpoint stuff"}, nil
		},
		ActivatePeer: func(ctx context.Context, anr *prototk.ActivatePeerRequest) (*prototk.ActivatePeerResponse, error) {
			assert.Equal(t, "node1", anr.NodeName)
			return &prototk.ActivatePeerResponse{PeerInfoJson: `{"endpoint": "stuff"}`}, nil
		},
		DeactivatePeer: func(ctx context.Context, danr *prototk.DeactivatePeerRequest) (*prototk.DeactivatePeerResponse, error) {
			assert.Equal(t, "node1", danr.NodeName)
			return &prototk.DeactivatePeerResponse{}, nil
		},
	}

	ttm := &testTransportManager{
		transports: map[string]plugintk.Plugin{
			"transport1": plugintk.NewTransport(func(callbacks plugintk.TransportCallbacks) plugintk.TransportAPI {
				waitForCallbacks <- callbacks
				return &plugintk.TransportAPIBase{Functions: transportFunctions}
			}),
		},
	}
	ttm.transportRegistered = func(name string, id uuid.UUID, toTransport components.TransportManagerToTransport) (plugintk.TransportCallbacks, error) {
		assert.Equal(t, "transport1", name)
		waitForAPI <- toTransport
		return ttm, nil
	}

	ttm.resolveTarget = func(ctx context.Context, req *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error) {
		assert.Equal(t, "node1", req.Node)
		return &prototk.GetTransportDetailsResponse{
			TransportDetails: "node1_details",
		}, nil
	}
	ttm.receiveMessage = func(ctx context.Context, req *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
		assert.Equal(t, "body1", string(req.Message.Payload))
		return &prototk.ReceiveMessageResponse{}, nil
	}

	ctx, pc, done := newTestTransportPluginManager(t, &testManagers{
		testTransportManager: ttm,
	})
	defer done()

	transportAPI := <-waitForAPI

	_, err := transportAPI.ConfigureTransport(ctx, &prototk.ConfigureTransportRequest{})
	require.NoError(t, err)

	smr, err := transportAPI.SendMessage(ctx, &prototk.SendMessageRequest{
		Message: &prototk.PaladinMsg{MessageType: "type1"},
	})
	require.NoError(t, err)
	assert.NotNil(t, smr)

	gldr, err := transportAPI.GetLocalDetails(ctx, &prototk.GetLocalDetailsRequest{})
	require.NoError(t, err)
	assert.NotNil(t, smr)
	assert.Equal(t, "endpoint stuff", gldr.TransportDetails)

	anr, err := transportAPI.ActivatePeer(ctx, &prototk.ActivatePeerRequest{NodeName: "node1"})
	require.NoError(t, err)
	assert.NotNil(t, anr)
	assert.Equal(t, `{"endpoint": "stuff"}`, anr.PeerInfoJson)

	danr, err := transportAPI.DeactivatePeer(ctx, &prototk.DeactivatePeerRequest{NodeName: "node1"})
	require.NoError(t, err)
	assert.NotNil(t, danr)

	// This is the point the transport manager would call us to say the transport is initialized
	// (once it's happy it's updated its internal state)
	transportAPI.Initialized()
	require.NoError(t, pc.WaitForInit(ctx, prototk.PluginInfo_DOMAIN))

	callbacks := <-waitForCallbacks
	rts, err := callbacks.GetTransportDetails(ctx, &prototk.GetTransportDetailsRequest{
		Node: "node1",
	})
	require.NoError(t, err)
	assert.Equal(t, "node1_details", rts.TransportDetails)
	rms, err := callbacks.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		Message: &prototk.PaladinMsg{
			Payload: []byte("body1"),
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, rms)

}

func TestTransportRegisterFail(t *testing.T) {
	waitForErrors := make(chan error)
	registrationCount := make(chan string)

	tdm := &testTransportManager{
		transports: map[string]plugintk.Plugin{
			"transport1": &mockPlugin[prototk.TransportMessage]{
				t:              t,
				connectFactory: transportConnectFactory,
				headerAccessor: transportHeaderAccessor,
				preRegister: func(transportID string) *prototk.TransportMessage {
					return &prototk.TransportMessage{
						Header: &prototk.Header{
							MessageType: prototk.Header_REGISTER,
							PluginId:    transportID,
							MessageId:   uuid.NewString(),
						},
					}
				},
				expectClose: func(err error) {
					waitForErrors <- err
				},
			},
			"transport2": &mockPlugin[prototk.TransportMessage]{
				t:              t,
				connectFactory: transportConnectFactory,
				headerAccessor: transportHeaderAccessor,
				preRegister: func(transportID string) *prototk.TransportMessage {
					return &prototk.TransportMessage{
						Header: &prototk.Header{
							MessageType: prototk.Header_REGISTER,
							PluginId:    transportID,
							MessageId:   uuid.NewString(),
						},
					}
				},
				expectClose: func(err error) {
					waitForErrors <- err
				},
			},
		},
	}
	tdm.transportRegistered = func(name string, id uuid.UUID, toTransport components.TransportManagerToTransport) (plugintk.TransportCallbacks, error) {
		defer func() {
			registrationCount <- name
		}()
		return nil, fmt.Errorf("%s failed", name)
	}

	_, _, done := newTestTransportPluginManager(t, &testManagers{
		testTransportManager: tdm,
	})
	defer done()

	// Wait for all registrations to be attempted
	registeredNames := make(map[string]bool)
	for i := 0; i < len(tdm.transports); i++ {
		select {
		case name := <-registrationCount:
			registeredNames[name] = true
		case <-time.After(2 * time.Second):
			t.Fatal("not all transport registrations attempted")
		}
	}

	for name := range tdm.transports {
		assert.True(t, registeredNames[name])
	}

	// Wait for all error callbacks
	errorCount := 0
	for i := 0; i < len(tdm.transports); i++ {
		select {
		case err := <-waitForErrors:
			assert.NotNil(t, err)
			errorCount++
		case <-time.After(3 * time.Second):
			t.Fatalf("only %d of %d transport failure callbacks fired", errorCount, len(tdm.transports))
		}
	}
	assert.Equal(t, len(tdm.transports), errorCount)
}

func TestTransportRegisterPartialSuccess(t *testing.T) {
	waitForError := make(chan error, 1)
	waitForSuccess := make(chan components.TransportManagerToTransport, 1)
	registrationCount := make(chan string, 2)
	errorCallbackDone := make(chan struct{}, 1)

	tdm := &testTransportManager{
		transports: map[string]plugintk.Plugin{
			"transport_success": plugintk.NewTransport(func(callbacks plugintk.TransportCallbacks) plugintk.TransportAPI {
				return &plugintk.TransportAPIBase{
					Functions: &plugintk.TransportAPIFunctions{
						ConfigureTransport: func(ctx context.Context, req *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error) {
							return &prototk.ConfigureTransportResponse{}, nil
						},
					},
				}
			}),
			"transport_fail": &mockPlugin[prototk.TransportMessage]{
				t:              t,
				connectFactory: transportConnectFactory,
				headerAccessor: transportHeaderAccessor,
				preRegister: func(transportID string) *prototk.TransportMessage {
					return &prototk.TransportMessage{
						Header: &prototk.Header{
							MessageType: prototk.Header_REGISTER,
							PluginId:    transportID,
							MessageId:   uuid.NewString(),
						},
					}
				},
				expectClose: func(err error) {
					waitForError <- err
					errorCallbackDone <- struct{}{}
				},
			},
		},
	}
	tdm.transportRegistered = func(name string, id uuid.UUID, toTransport components.TransportManagerToTransport) (plugintk.TransportCallbacks, error) {
		defer func() {
			registrationCount <- name
		}()
		if name == "transport_success" {
			defer func() {
				waitForSuccess <- toTransport
			}()
			return tdm, nil
		}
		return nil, fmt.Errorf("transport_fail registration failed")
	}

	_, _, done := newTestTransportPluginManager(t, &testManagers{
		testTransportManager: tdm,
	})
	defer done()

	// Wait for both registrations to be attempted
	registeredNames := make(map[string]bool)
	for i := 0; i < len(tdm.transports); i++ {
		select {
		case name := <-registrationCount:
			registeredNames[name] = true
		case <-time.After(2 * time.Second):
			t.Fatal("not all transport registrations attempted")
		}
	}
	for name := range tdm.transports {
		assert.True(t, registeredNames[name])
	}

	// Verify successful transport is available
	select {
	case transportAPI := <-waitForSuccess:
		assert.NotNil(t, transportAPI)
	case <-time.After(1 * time.Second):
		t.Fatal("successful transport API not received")
	}

	// Verify failed transport triggers error callback
	select {
	case err := <-waitForError:
		assert.Contains(t, err.Error(), "transport_fail registration failed")
	case <-time.After(3 * time.Second):
		t.Fatal("transport failure callback never fired")
	}

	// Wait for the error callback to complete before test cleanup
	select {
	case <-errorCallbackDone:
	case <-time.After(1 * time.Second):
		t.Fatal("error callback did not complete in time")
	}
}
