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

	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/LF-Decentralized-Trust-labs/paladin/transports/grpc/pkg/proto"
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
	transport := NewGRPCTransport(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{!!!!`,
	})
	assert.Regexp(t, "PD030001", err)

}

func TestMissingListenerPort(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := NewGRPCTransport(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{}`,
	})
	assert.Regexp(t, "PD030000", err)

}

func TestBadCertSubjectMatcher(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := NewGRPCTransport(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{"address": "127.0.0.1", "port": 0, "certSubjectMatcher": "[[[[[[[badness"}`,
	})
	assert.Regexp(t, "PD030003", err)

}

func TestBadTLSConf(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := NewGRPCTransport(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{"address": "127.0.0.1", "port": 0, "tls": { "caFile": "` + t.TempDir() + `" }}`,
	})
	assert.Regexp(t, "PD020401", err)

}

func TestBadDirectCertVerificationConf(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := NewGRPCTransport(callbacks).(*grpcTransport)
	_, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: `{"address": "127.0.0.1", "port": 0, "tls": { "requiredDNAttributes": {"cn":"anything"} }}`,
	})
	assert.Regexp(t, "PD030002", err)

}

func TestBadListenerConf(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := NewGRPCTransport(callbacks).(*grpcTransport)
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
			Node: "node2",
			Message: &prototk.PaladinMsg{
				Component: prototk.PaladinMsg_TRANSACTION_ENGINE,
			},
		})
	}
	assert.Error(t, err)

}

func TestConnectFail(t *testing.T) {

	ctx := context.Background()

	plugin1, plugin2, done := newSuccessfulVerifiedConnection(t, func(_, callbacks2 *testCallbacks) {
		callbacks2.receiveMessage = func(ctx context.Context, rmr *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
			require.Equal(t, "node1", rmr.FromNode)
			return &prototk.ReceiveMessageResponse{}, nil
		}
	})
	defer done()

	plugin2.grpcServer.Stop()

	// gRPC does not guarantee we get the error immediately
	var err error
	for err == nil {
		_, err = plugin1.SendMessage(ctx, &prototk.SendMessageRequest{
			Node: "node2",
			Message: &prototk.PaladinMsg{
				Component: prototk.PaladinMsg_TRANSACTION_ENGINE,
			},
		})
	}
	assert.Error(t, err)

}

func TestSendNotActivated(t *testing.T) {

	ctx := context.Background()

	plugin1, _, done := newSuccessfulVerifiedConnection(t, func(_, callbacks2 *testCallbacks) {
		callbacks2.receiveMessage = func(ctx context.Context, rmr *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
			return &prototk.ReceiveMessageResponse{}, nil
		}
	})
	defer done()

	_, err := plugin1.SendMessage(ctx, &prototk.SendMessageRequest{
		Node: "node3",
		Message: &prototk.PaladinMsg{
			Component: prototk.PaladinMsg_TRANSACTION_ENGINE,
		},
	})
	assert.Regexp(t, "PD030016", err)

}

func TestActivateBadTransportDetails(t *testing.T) {

	ctx := context.Background()

	plugin1, _, done := newSuccessfulVerifiedConnection(t, func(_, callbacks2 *testCallbacks) {
		callbacks2.receiveMessage = func(ctx context.Context, rmr *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
			return &prototk.ReceiveMessageResponse{}, nil
		}
	})
	defer done()

	_, err := plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node2",
		TransportDetails: `{"endpoint": false}`,
	})
	assert.Regexp(t, "PD030014", err)

}

func TestConnectBadTransport(t *testing.T) {

	ctx := context.Background()

	plugin1, _, done := newSuccessfulVerifiedConnection(t)
	defer done()

	_, err := plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node2",
		TransportDetails: `{"endpoint": "WRONG:::::::"}`,
	})
	assert.Regexp(t, "WRONG", err)

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
			Component: int32(prototk.PaladinMsg_TRANSACTION_ENGINE),
		})
	}
	assert.Error(t, err)
}
