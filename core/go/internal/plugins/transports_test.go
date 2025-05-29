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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"

	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
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

func (tp *testTransportManager) mock(t *testing.T) *componentmocks.TransportManager {
	mdm := componentmocks.NewTransportManager(t)
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
	require.NoError(t, pc.WaitForInit(ctx))

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

	waitForError := make(chan error, 1)

	tdm := &testTransportManager{
		transports: map[string]plugintk.Plugin{
			"transport1": &mockPlugin[prototk.TransportMessage]{
				t:                   t,
				allowRegisterErrors: true,
				connectFactory:      transportConnectFactory,
				headerAccessor:      transportHeaderAccessor,
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
				},
			},
		},
	}
	tdm.transportRegistered = func(name string, id uuid.UUID, toTransport components.TransportManagerToTransport) (plugintk.TransportCallbacks, error) {
		return nil, fmt.Errorf("pop")
	}

	_, _, done := newTestTransportPluginManager(t, &testManagers{
		testTransportManager: tdm,
	})
	defer done()

	var got error
	require.Eventually(t, func() bool {
		select {
		case got = <-waitForError:
			return true
		default:
			return false
		}
	}, 5*time.Second, 10*time.Millisecond, "transport failure callback never fired")
	assert.Regexp(t, "pop", got)
}

func TestFromTransportRequestBadReq(t *testing.T) {

	waitForResponse := make(chan struct{}, 1)

	msgID := uuid.NewString()
	ttm := &testTransportManager{
		transports: map[string]plugintk.Plugin{
			"transport1": &mockPlugin[prototk.TransportMessage]{
				t:              t,
				connectFactory: transportConnectFactory,
				headerAccessor: transportHeaderAccessor,
				sendRequest: func(pluginID string) *prototk.TransportMessage {
					return &prototk.TransportMessage{
						Header: &prototk.Header{
							PluginId:    pluginID,
							MessageId:   msgID,
							MessageType: prototk.Header_REQUEST_FROM_PLUGIN,
							// Missing payload
						},
					}
				},
				handleResponse: func(dm *prototk.TransportMessage) {
					assert.Equal(t, msgID, *dm.Header.CorrelationId)
					assert.Regexp(t, "PD011203", *dm.Header.ErrorMessage)
					close(waitForResponse)
				},
			},
		},
	}
	ttm.transportRegistered = func(name string, id uuid.UUID, toTransport components.TransportManagerToTransport) (fromTransport plugintk.TransportCallbacks, err error) {
		return ttm, nil
	}

	_, _, done := newTestTransportPluginManager(t, &testManagers{
		testTransportManager: ttm,
	})
	defer done()

	<-waitForResponse

}
