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

package grpctransport

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/transports/grpc/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type testCallbacks struct {
	getTransportDetails func(context.Context, *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error)
	receiveMessage      func(context.Context, *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error)
}

func (tc *testCallbacks) GetTransportDetails(ctx context.Context, req *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error) {
	return tc.getTransportDetails(ctx, req)
}

func (tc *testCallbacks) ReceiveMessage(ctx context.Context, req *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
	return tc.receiveMessage(ctx, req)
}

func TestPluginLifecycle(t *testing.T) {
	pb := NewPlugin(context.Background())
	assert.NotNil(t, pb)
}

func TestBadConfigJSON(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := grpcTransportFactory(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{!!!!`,
	})
	assert.Regexp(t, "PD030001", err)

}

func TestMissingListenerPort(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := grpcTransportFactory(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{}`,
	})
	assert.Regexp(t, "PD030000", err)

}

func TestBadCertSubjectMatcher(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := grpcTransportFactory(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{"address": "127.0.0.1", "port": 0, "certSubjectMatcher": "[[[[[[[badness"}`,
	})
	assert.Regexp(t, "PD030003", err)

}

func TestBadTLSConf(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := grpcTransportFactory(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{"address": "127.0.0.1", "port": 0, "tls": { "caFile": "` + t.TempDir() + `" }}`,
	})
	assert.Regexp(t, "PD020401", err)

}

func TestBadDirectCertVerificationConf(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := grpcTransportFactory(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{"address": "127.0.0.1", "port": 0, "tls": { "requiredDNAttributes": {"cn":"anything"} }}`,
	})
	assert.Regexp(t, "PD030002", err)

}

func TestBadListenerConf(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := grpcTransportFactory(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{"address": "::::::::", "port": 0}`,
	})
	assert.Regexp(t, "listen", err)
}

func TestReceiveFail(t *testing.T) {

	ctx := context.Background()

	plugin1, _, done := newSuccessfulVerifiedConnection(t, func(_, callbacks2 *testCallbacks) {
		callbacks2.receiveMessage = func(ctx context.Context, rmr *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
			return nil, fmt.Errorf("pop")
		}
	})
	defer done()

	// Send and we should get an error as the server fails
	var err error
	for err == nil {
		_, err = plugin1.SendMessage(ctx, &prototk.SendMessageRequest{
			Message: &prototk.Message{
				ReplyTo:     "to.me@node1",
				Destination: "to.you@node2",
			},
		})
	}
	assert.Error(t, err)

}

func TestBadReplyTo(t *testing.T) {

	ctx := context.Background()

	plugin1, _, done := newSuccessfulVerifiedConnection(t)
	defer done()

	// Send and we should get an error as the server fails
	var err error
	for err == nil {
		_, err = plugin1.SendMessage(ctx, &prototk.SendMessageRequest{
			Message: &prototk.Message{
				ReplyTo:     "to.me@not.mine",
				Destination: "to.you@node2",
			},
		})
	}
	assert.Error(t, err)

}

func TestConnectFail(t *testing.T) {

	ctx := context.Background()

	plugin1, plugin2, done := newSuccessfulVerifiedConnection(t, func(_, callbacks2 *testCallbacks) {
		callbacks2.receiveMessage = func(ctx context.Context, rmr *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
			return &prototk.ReceiveMessageResponse{}, nil
		}
	})
	defer done()

	plugin2.grpcServer.Stop()

	_, err := plugin1.SendMessage(ctx, &prototk.SendMessageRequest{
		Message: &prototk.Message{
			ReplyTo:     "to.me@node1",
			Destination: "to.you@node2",
		},
	})
	assert.Regexp(t, "rpc error", err)

}

func TestConnectBadTransport(t *testing.T) {

	ctx := context.Background()

	plugin1, _, done := newSuccessfulVerifiedConnection(t, func(callbacks1, _ *testCallbacks) {
		callbacks1.getTransportDetails = func(ctx context.Context, gtdr *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error) {
			return &prototk.GetTransportDetailsResponse{
				TransportDetails: `{"endpoint": "WRONG:::::::"}`,
			}, nil
		}
	})
	defer done()

	_, err := plugin1.SendMessage(ctx, &prototk.SendMessageRequest{
		Message: &prototk.Message{
			ReplyTo:     "to.me@node1",
			Destination: "to.you@node2",
		},
	})
	assert.Regexp(t, "WRONG", err)

}

func TestSendBadDest(t *testing.T) {

	ctx := context.Background()

	plugin1, _, done := newSuccessfulVerifiedConnection(t, func(_, callbacks2 *testCallbacks) {
		callbacks2.receiveMessage = func(ctx context.Context, rmr *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
			return &prototk.ReceiveMessageResponse{}, nil
		}
	})
	defer done()

	_, err := plugin1.SendMessage(ctx, &prototk.SendMessageRequest{
		Message: &prototk.Message{
			ReplyTo:     "to.me@node1",
			Destination: "!wrong",
		},
	})
	assert.Regexp(t, "PD020006", err)

}

func TestConnectSendStreamBadSecurityCtx(t *testing.T) {

	plugin, _, _, done := newTestGRPCTransport(t, "", "", &Config{})
	defer done()

	// Create an unsecured server to the plugin using an unsecured server,
	// and check that the stream loop closes rather than accepting messages.
	unsecuredServer := grpc.NewServer()
	proto.RegisterPaladinGRPCTransportServer(unsecuredServer, plugin)

	serverDone := make(chan struct{})
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() {
		defer close(serverDone)
		_ = unsecuredServer.Serve(l)
	}()

	conn, err := grpc.NewClient("dns:///"+l.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	client := proto.NewPaladinGRPCTransportClient(conn)
	s, err := client.ConnectSendStream(context.Background())
	require.NoError(t, err)

	for err == nil {
		err = s.Send(&proto.Message{
			ReplyTo:     "to.me@not.mine",
			Destination: "to.you@node2",
		})
	}
	assert.Error(t, err)
}

func TestWaitNewConn(t *testing.T) {

	plugin, _, _, done := newTestGRPCTransport(t, "", "", &Config{})
	defer done()

	isNew, oc, err := plugin.waitExistingOrNewConn("node1")
	assert.True(t, isNew)
	assert.Nil(t, err)

	bgError := make(chan error)
	go func() {
		_, _, err := plugin.waitExistingOrNewConn("node1")
		bgError <- err
	}()

	for oc.waiting == 0 {
		time.Sleep(1 * time.Millisecond)
	}

	plugin.connLock.L.Lock()
	oc.connecting = false
	oc.connError = fmt.Errorf("pop")
	plugin.connLock.Broadcast()
	plugin.connLock.L.Unlock()

	assert.Regexp(t, "pop", <-bgError)

}
