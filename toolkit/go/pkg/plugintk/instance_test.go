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
package plugintk

import (
	"context"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func newTestPluginRunner(connString string) *pluginRun[prototk.DomainMessage] {
	pf := NewDomain(func(callbacks DomainCallbacks) DomainAPI { return nil })
	pi := newPluginInstance(pf.(*pluginFactory[prototk.DomainMessage]), connString, uuid.NewString())
	return &pluginRun[prototk.DomainMessage]{pi: pi}
}

func TestPluginRunConnectFail(t *testing.T) {
	pr := newTestPluginRunner(t.TempDir() /* never going to work */)
	err := pr.run()
	assert.Regexp(t, "rpc", err)
}

func TestPluginRunSendAfterClose(t *testing.T) {
	ctx, tc, done := newTestController(t)
	defer done()

	pr := newTestPluginRunner("unix:" + tc.socketFile)

	waitConnected := make(chan struct{})
	tc.fakeDomainController = func(bss grpc.BidiStreamingServer[prototk.DomainMessage, prototk.DomainMessage]) error {
		close(waitConnected)
		return nil
	}

	waitStopped := make(chan struct{})
	go func() {
		defer close(waitStopped)
		_ = pr.run()
	}()

	<-waitConnected
	pr.pi.factory.Stop()
	<-waitStopped

	// Now run the sender again
	pr.senderDone = make(chan struct{})
	pr.senderChl = make(chan *prototk.DomainMessage)
	pr.ctx, pr.cancelCtx = context.WithCancel(context.Background())
	go func() {
		// push one message over to it to send
		pr.send(&prototk.DomainMessage{})
		// then cancel the context
		pr.cancelCtx()
	}()
	// Run the sender
	pr.sender()

	// Check we don't block after closed
	_, err := pr.RequestFromPlugin(ctx, pr.pi.impl.Wrap(&prototk.DomainMessage{}))
	assert.Regexp(t, "PD020100", err)

}

func TestPluginRunBadMessages(t *testing.T) {
	ctx, tc, done := newTestController(t)
	defer done()

	pr := newTestPluginRunner("unix:" + tc.socketFile)

	stop := make(chan struct{})
	waitConnected := make(chan grpc.BidiStreamingServer[prototk.DomainMessage, prototk.DomainMessage])
	tc.fakeDomainController = func(stream grpc.BidiStreamingServer[prototk.DomainMessage, prototk.DomainMessage]) error {
		waitConnected <- stream
		<-stop
		return nil
	}
	defer close(stop)

	go func() {
		_ = pr.run()
	}()
	stream := <-waitConnected

	// Put a request in flight
	reqID := uuid.New()
	req := pr.inflight.AddInflight(ctx, reqID)

	// Send problematic stuff to be ignored
	// 1... wrong type
	err := stream.Send(&prototk.DomainMessage{
		Header: &prototk.Header{
			PluginId:    pr.pi.id,
			MessageType: prototk.Header_REQUEST_FROM_PLUGIN,
		},
	})
	require.NoError(t, err)
	// 2... missing a correlation id
	err = stream.Send(&prototk.DomainMessage{
		Header: &prototk.Header{
			PluginId:    pr.pi.id,
			MessageType: prototk.Header_RESPONSE_TO_PLUGIN,
		},
	})
	require.NoError(t, err)
	// 3... an unknown correlation id
	anotherID := uuid.NewString()
	err = stream.Send(&prototk.DomainMessage{
		Header: &prototk.Header{
			PluginId:      pr.pi.id,
			MessageType:   prototk.Header_RESPONSE_TO_PLUGIN,
			CorrelationId: &anotherID,
		},
	})
	require.NoError(t, err)
	// 4... the one we want!
	correctID := reqID.String()
	err = stream.Send(&prototk.DomainMessage{
		Header: &prototk.Header{
			PluginId:      pr.pi.id,
			MessageType:   prototk.Header_RESPONSE_TO_PLUGIN,
			CorrelationId: &correctID,
		},
	})
	require.NoError(t, err)

	// Check we are completed
	msg, err := req.Wait()
	require.NoError(t, err)
	assert.Equal(t, correctID, *msg.Header().CorrelationId)
}

func TestErrorFromServer(t *testing.T) {
	ctx, tc, tcDone := newTestController(t)
	defer tcDone()

	funcs := &DomainAPIFunctions{}
	waitForCallbacks := make(chan DomainCallbacks, 1)
	domain := NewDomain(func(callbacks DomainCallbacks) DomainAPI {
		// Implementation would construct an instance here to start handling the API calls from Paladin,
		// (rather than passing the callbacks to the test as we do here)
		waitForCallbacks <- callbacks
		return &DomainAPIBase{funcs}
	})
	defer domain.Stop()

	pluginID := uuid.NewString()
	tc.fakeDomainController = func(bss grpc.BidiStreamingServer[prototk.DomainMessage, prototk.DomainMessage]) error {
		for {
			req, err := bss.Recv()
			if err != nil {
				return err
			}
			_ = bss.Send(&prototk.DomainMessage{
				Header: &prototk.Header{
					PluginId:      req.Header.PluginId,
					MessageId:     uuid.NewString(),
					CorrelationId: &req.Header.MessageId,
					ErrorMessage:  confutil.P("pop"),
					MessageType:   prototk.Header_ERROR_RESPONSE,
				},
			})
		}
	}

	domainDone := make(chan struct{})
	go func() {
		defer close(domainDone)
		domain.Run("unix:"+tc.socketFile, pluginID)
	}()
	callbacks := <-waitForCallbacks

	_, err := callbacks.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{})
	assert.Regexp(t, "pop", err)
}

func TestEmptyErrorFromServer(t *testing.T) {
	ctx, tc, tcDone := newTestController(t)
	defer tcDone()

	funcs := &DomainAPIFunctions{}
	waitForCallbacks := make(chan DomainCallbacks, 1)
	domain := NewDomain(func(callbacks DomainCallbacks) DomainAPI {
		// Implementation would construct an instance here to start handling the API calls from Paladin,
		// (rather than passing the callbacks to the test as we do here)
		waitForCallbacks <- callbacks
		return &DomainAPIBase{funcs}
	})
	defer domain.Stop()

	pluginID := uuid.NewString()
	tc.fakeDomainController = func(bss grpc.BidiStreamingServer[prototk.DomainMessage, prototk.DomainMessage]) error {
		for {
			req, err := bss.Recv()
			if err != nil {
				return err
			}
			_ = bss.Send(&prototk.DomainMessage{
				Header: &prototk.Header{
					PluginId:      req.Header.PluginId,
					MessageId:     uuid.NewString(),
					CorrelationId: &req.Header.MessageId,
					MessageType:   prototk.Header_ERROR_RESPONSE,
				},
			})
		}
	}

	domainDone := make(chan struct{})
	go func() {
		defer close(domainDone)
		domain.Run("unix:"+tc.socketFile, pluginID)
	}()
	callbacks := <-waitForCallbacks

	_, err := callbacks.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{})
	assert.Regexp(t, "PD020303", err)
}
