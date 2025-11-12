// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rpcserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSocketRPCRequestResponse(t *testing.T) {

	ctx, cancelCtx := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelCtx()
	url, s, done := newTestServerWebSockets(t, &pldconf.RPCServerConfig{})
	defer done()

	wsConfig := &pldconf.WSClientConfig{}
	wsConfig.URL = url
	client := rpcclient.WrapWSConfig(wsConfig)
	defer client.Close()
	err := client.Connect(ctx)
	require.NoError(t, err)

	regTestRPC(s, "stringy_method", RPCMethod2(func(ctx context.Context, p0, p1 string) (string, error) {
		assert.Equal(t, "v0", p0)
		assert.Equal(t, "v1", p1)
		return "result", nil
	}))

	var result string
	rpcErr := client.CallRPC(ctx, &result, "stringy_method", "v0", "v1")
	assert.Nil(t, rpcErr)
	assert.Equal(t, "result", result)

}

func TestWebSocketConnectionFailureHandling(t *testing.T) {
	url, s, done := newTestServerWebSockets(t, &pldconf.RPCServerConfig{})
	defer done()

	wsConfig := &pldconf.WSClientConfig{}
	wsConfig.URL = url
	client := rpcclient.WrapWSConfig(wsConfig)
	defer client.Close()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	var wsConn *webSocketConnection
	before := time.Now()
	for wsConn == nil {
		time.Sleep(1 * time.Millisecond)
		for _, wsConn = range s.wsConnections {
		}
		if time.Since(before) > 1*time.Second {
			t.Fatal("timed out waiting for connection")
		}
	}

	// Close the connection
	client.Close()
	<-wsConn.closing
	for !wsConn.closed {
		time.Sleep(1 * time.Microsecond)
	}

	// Run the send directly to give it an error to handle, which will make it return
	wsConn.closing = make(chan struct{})
	wsConn.send = make(chan []byte)
	go func() { wsConn.send <- ([]byte)(`{}`) }()
	wsConn.sender()

	// Give it some bad data to handle
	wsConn.sendMessage(map[bool]bool{false: true})

	// Give it some good data to discard
	wsConn.sendMessage("anything")

}

func TestNewWSConnectionWithAuthenticationResults(t *testing.T) {
	ctx := context.Background()
	server, err := NewRPCServer(ctx, &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{Disabled: true},
		WS: pldconf.RPCServerConfigWS{
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Port: confutil.P(0),
			},
		},
	})
	require.NoError(t, err)
	defer server.Stop()

	// Set up authorizers
	server.SetAuthorizers([]Authorizer{&mockAuthorizer{}})

	// Track if setAuthenticationResults was called
	resultsToStore := []string{`{"user":"test1"}`, `{"user":"test2"}`}

	// Channel to signal when connection is ready for verification
	connReady := make(chan *webSocketConnection, 1)

	// Create a test server that provides WebSocket upgrade
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create request WITH authentication results in context - this is what we're testing
		r = r.WithContext(context.WithValue(r.Context(), authResultKey, resultsToStore))

		// Upgrade the WebSocket to get the connection
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}

		// Call newWSConnection with request that HAS authentication results
		server.newWSConnection(conn, r)

		server.wsMux.Lock()
		defer server.wsMux.Unlock()
		for _, wsConn := range server.wsConnections {
			connReady <- wsConn
			break
		}
	}))
	defer testServer.Close()

	// Connect as WebSocket client to trigger the handler
	wsURL := "ws" + testServer.URL[4:]
	wsConfig := &pldconf.WSClientConfig{}
	wsConfig.URL = wsURL
	wsClient := rpcclient.WrapWSConfig(wsConfig)
	defer wsClient.Close()

	connectCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = wsClient.Connect(connectCtx)
	require.NoError(t, err)

	// Wait for connection to be ready
	wsConn := <-connReady

	storedResults := wsConn.getAuthenticationResults()
	assert.Equal(t, resultsToStore, storedResults, "Authentication results should be stored")
}

func TestNewWSConnectionWithoutAuthenticationResults(t *testing.T) {
	ctx := context.Background()
	server, err := NewRPCServer(ctx, &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{Disabled: true},
		WS: pldconf.RPCServerConfigWS{
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Port: confutil.P(0),
			},
		},
	})
	require.NoError(t, err)
	defer server.Stop()

	// Set up authorizers
	server.SetAuthorizers([]Authorizer{&mockAuthorizer{}})

	// Channel to signal when connection is ready
	connReady := make(chan *webSocketConnection, 1)

	// Create a test server that provides WebSocket upgrade
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// don't add auth results to the request context

		// Upgrade the WebSocket to get the connection
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}

		// Now call newWSConnection with request that has NO authentication results
		server.newWSConnection(conn, r)

		server.wsMux.Lock()
		defer server.wsMux.Unlock()
		for _, wsConn := range server.wsConnections {
			connReady <- wsConn
			break
		}
	}))
	defer testServer.Close()

	// Connect as WebSocket client to trigger the handler
	wsURL := "ws" + testServer.URL[4:]
	wsConfig := &pldconf.WSClientConfig{}
	wsConfig.URL = wsURL
	wsClient := rpcclient.WrapWSConfig(wsConfig)
	defer wsClient.Close()

	connectCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = wsClient.Connect(connectCtx)
	require.NoError(t, err)

	// Wait for connection to be ready
	wsConn := <-connReady

	storedResults := wsConn.getAuthenticationResults()
	assert.Empty(t, storedResults, "Authentication results should be empty")
}
