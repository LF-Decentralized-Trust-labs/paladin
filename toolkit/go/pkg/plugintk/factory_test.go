/*
 * Copyright © 2025 Kaleido, Inc.
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
package plugintk

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime/debug"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type testController struct {
	prototk.UnimplementedPluginControllerServer
	server     *grpc.Server
	socketFile string

	fakeDomainController        func(grpc.BidiStreamingServer[prototk.DomainMessage, prototk.DomainMessage]) error
	fakeTransportController     func(grpc.BidiStreamingServer[prototk.TransportMessage, prototk.TransportMessage]) error
	fakeRegistryController      func(grpc.BidiStreamingServer[prototk.RegistryMessage, prototk.RegistryMessage]) error
	fakeSigningModuleController func(grpc.BidiStreamingServer[prototk.SigningModuleMessage, prototk.SigningModuleMessage]) error
}

func newTestController(t *testing.T) (context.Context, *testController, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	tc := &testController{
		server:     grpc.NewServer(),
		socketFile: tempSocketFile(t),
	}
	prototk.RegisterPluginControllerServer(tc.server, tc)

	l, err := net.Listen("unix", tc.socketFile)
	require.NoError(t, err)

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		_ = tc.server.Serve(l)
	}()

	return ctx, tc, func() {
		cancelCtx()
		tc.server.Stop()
		<-serverDone
	}
}

func (tc *testController) ConnectDomain(stream grpc.BidiStreamingServer[prototk.DomainMessage, prototk.DomainMessage]) error {
	return tc.fakeDomainController(stream)
}

func (tc *testController) ConnectTransport(stream grpc.BidiStreamingServer[prototk.TransportMessage, prototk.TransportMessage]) error {
	return tc.fakeTransportController(stream)
}

func (tc *testController) ConnectRegistry(stream grpc.BidiStreamingServer[prototk.RegistryMessage, prototk.RegistryMessage]) error {
	return tc.fakeRegistryController(stream)
}

func (tc *testController) ConnectSigningModule(stream grpc.BidiStreamingServer[prototk.SigningModuleMessage, prototk.SigningModuleMessage]) error {
	return tc.fakeSigningModuleController(stream)
}

func tempSocketFile(t *testing.T) string {
	// note socket filenames need to be <108 chars
	f, err := os.CreateTemp("", "ptk.*.sock")
	require.NoError(t, err)
	fileName := f.Name()
	err = f.Close()
	require.NoError(t, err)
	err = os.Remove(fileName)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Remove(fileName)
	})
	return fileName
}

func checkPanic() {
	panicked := recover()
	if panicked != nil {
		fmt.Fprintln(os.Stderr, (string)(debug.Stack()))
		panic(panicked)
	}

}

type pluginExerciser[M any] struct {
	t          *testing.T
	wrapper    PluginMessageWrapper[M]
	inOutMap   map[string]func(*M)
	pluginID   string
	registered chan bool
	sendChl    chan *M
	recvChl    chan *M
}

func newPluginExerciser[M any](t *testing.T, pluginID string, wrapper PluginMessageWrapper[M], inOutMap map[string]func(*M)) *pluginExerciser[M] {
	return &pluginExerciser[M]{
		t:          t,
		wrapper:    wrapper,
		inOutMap:   inOutMap,
		pluginID:   pluginID,
		registered: make(chan bool, 1),
		sendChl:    make(chan *M),
		recvChl:    make(chan *M),
	}
}

func (pe *pluginExerciser[M]) controller(stream grpc.BidiStreamingServer[M, M]) error {
	t := pe.t
	go func() {
		for msg := range pe.sendChl {
			err := stream.Send(msg)
			require.NoError(t, err)
		}
	}()
	for {
		iMsg, err := stream.Recv()
		if err != nil {
			return err
		}
		msg := pe.wrapper.Wrap(iMsg)
		assert.Equal(t, pe.pluginID, msg.Header().PluginId)
		switch msg.Header().MessageType {
		case prototk.Header_REGISTER:
			pe.registered <- true
		case prototk.Header_REQUEST_FROM_PLUGIN:
			reply := pe.wrapper.Wrap(new(M))
			reply.Header().PluginId = msg.Header().PluginId
			reply.Header().MessageId = uuid.NewString()
			reply.Header().CorrelationId = &msg.Header().MessageId
			reply.Header().MessageType = prototk.Header_RESPONSE_TO_PLUGIN

			// Use the map to determine what to do
			reqType := fmt.Sprintf("%T", msg.RequestFromPlugin())
			mapper := pe.inOutMap[reqType]
			assert.NotNil(t, mapper, fmt.Sprintf("MISSING: %s", reqType))
			mapper(reply.Message())
			err = stream.Send(reply.Message())
			require.NoError(t, err)
		case prototk.Header_RESPONSE_FROM_PLUGIN, prototk.Header_ERROR_RESPONSE:
			pe.recvChl <- msg.Message()
		default:
			assert.Failf(t, "", "Unexpected message: %s", msg.Header().MessageType)
		}
	}
}

func (pe *pluginExerciser[M]) doExchangeToPlugin(setReq func(*M), checkRes func(*M)) {
	t := pe.t

	req := pe.wrapper.Wrap(new(M))
	req.Header().PluginId = pe.pluginID
	req.Header().MessageId = uuid.NewString()
	req.Header().MessageType = prototk.Header_REQUEST_TO_PLUGIN
	log.L(context.Background()).Infof("IN %s", PluginMessageToJSON(req))
	setReq(req.Message())

	pe.sendChl <- req.Message()

	reply := pe.wrapper.Wrap(<-pe.recvChl)
	log.L(context.Background()).Infof("OUT %s", PluginMessageToJSON(reply))
	assert.Equal(t, pe.pluginID, reply.Header().PluginId)
	assert.Equal(t, req.Header().MessageId, *reply.Header().CorrelationId)
	if reply.Header().MessageType == prototk.Header_ERROR_RESPONSE {
		assert.NotNil(t, reply.Header().ErrorMessage)
		assert.NotEmpty(t, *reply.Header().ErrorMessage)
	} else {
		assert.Equal(t, prototk.Header_RESPONSE_FROM_PLUGIN, reply.Header().MessageType)
	}
	checkRes(reply.Message())
}

func TestUnimplementedDoesNotPanic(t *testing.T) {
	// Use domains for this test for convenience
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// This is what a domain would actually implement
	assert.Nil(t, funcs.ConfigureDomain)

	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_ConfigureDomain{}
	}, func(res *prototk.DomainMessage) {
		// Get an error back saying this request hasn't been implemented by the plugin
		assert.Regexp(t, "PD020302", *res.Header.ErrorMessage)
	})
}

func TestDualLoadPanics(t *testing.T) {
	pf := &pluginFactory[string]{
		instances: make(map[string]*pluginInstance[string]),
	}
	id := uuid.NewString()
	pf.instanceStarted(&pluginInstance[string]{id: id})
	assert.Panics(t, func() {
		pf.instanceStarted(&pluginInstance[string]{id: id})
	})
}
