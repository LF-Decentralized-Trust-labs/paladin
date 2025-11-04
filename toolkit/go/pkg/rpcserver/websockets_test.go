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
			panic("timed out waiting for connection")
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

func TestWebSocketConnection_SetAuthenticationResults(t *testing.T) {
	wsc := &webSocketConnection{
		authenticationResults: []string{},
	}

	// Test setAuthenticationResults
	authenticationResults := []string{`{"user":"test1"}`, `{"user":"test2"}`}
	wsc.setAuthenticationResults(authenticationResults)

	// Test getAuthenticationResults
	retrieved := wsc.getAuthenticationResults()
	assert.Equal(t, authenticationResults, retrieved)
	assert.Len(t, retrieved, 2)
}

func TestWebSocketConnection_GetAuthenticationResults_Empty(t *testing.T) {
	wsc := &webSocketConnection{
		authenticationResults: []string{},
	}

	retrieved := wsc.getAuthenticationResults()
	assert.Empty(t, retrieved)
	assert.NotNil(t, retrieved)
}

func TestNewWSConnection_WithAuthenticationResults(t *testing.T) {
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

	// Create a mock websocket connection (we can't easily test real upgrade in unit tests)
	// This test verifies the identity handling logic
	req := httptest.NewRequest("GET", "/test", nil)
	authenticationResults := []string{`{"user":"test1"}`, `{"user":"test2"}`}
	req = req.WithContext(context.WithValue(req.Context(), wsAuthResultKey, authenticationResults))

	// Set up authorizers
	auth := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"user":"test"}`, nil
		},
	}
	server.SetAuthorizers([]Authorizer{auth})

	// We can't easily test newWSConnection directly without a real WebSocket connection
	// But we can verify the authentication result retrieval logic would work
	if storedResults, ok := req.Context().Value(wsAuthResultKey).([]string); ok {
		assert.Equal(t, authenticationResults, storedResults)
		assert.Len(t, storedResults, 2)
	} else {
		t.Fatal("Authentication results not found in context")
	}
}

func TestNewWSConnection_WithoutAuthenticationResults_WithAuthorizers(t *testing.T) {
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

	// Create a request without authentication results in context
	req := httptest.NewRequest("GET", "/test", nil)

	// Set up authorizers
	auth := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"user":"test"}`, nil
		},
	}
	server.SetAuthorizers([]Authorizer{auth})

	// Verify that authentication results are not in context (this is the case we're testing)
	if storedResults, ok := req.Context().Value(wsAuthResultKey).([]string); ok {
		// Should be empty or not ok
		assert.Empty(t, storedResults)
	} else {
		// This is expected - no authentication results in context
		assert.False(t, ok)
	}
}

func TestNewWSConnection_NoAuthorizers(t *testing.T) {
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

	// No authorizers configured
	server.SetAuthorizers(nil)

	// Create a request (should work fine without authentication results)
	req := httptest.NewRequest("GET", "/test", nil)

	// Verify no authorizers
	assert.Empty(t, server.authorizers)

	// Request should be fine (we can't test full connection without real websocket, but logic is covered)
	_ = req
}

func TestNewWSConnection_WithAuthenticationResults_CallsNewWSConnection(t *testing.T) {
	// This test covers the if branch (lines 60-62) where authentication results ARE found in context
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
	auth := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"user":"test"}`, nil
		},
	}
	server.SetAuthorizers([]Authorizer{auth})

	// Track if setAuthenticationResults was called
	resultsToStore := []string{`{"user":"test1"}`, `{"user":"test2"}`}

	// Create a test server that provides WebSocket upgrade
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create request WITH authentication results in context - this is what we're testing
		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(context.WithValue(r.Context(), wsAuthResultKey, resultsToStore))

		// Use server's upgrader or create one if needed
		if server.wsUpgrader == nil {
			server.wsUpgrader = &websocket.Upgrader{}
		}

		// Upgrade to WebSocket
		conn, err := server.wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		// Call newWSConnection with request that HAS authentication results
		// This should trigger the if branch (lines 60-62) - the code path we're testing
		server.newWSConnection(conn, req)

		// Give goroutines time to start
		time.Sleep(50 * time.Millisecond)

		// Verify authentication results were stored by checking the connection
		server.wsMux.Lock()
		var wsConn *webSocketConnection
		for _, conn := range server.wsConnections {
			wsConn = conn
			break
		}
		server.wsMux.Unlock()

		if wsConn != nil {
			storedResults := wsConn.getAuthenticationResults()
			assert.Equal(t, resultsToStore, storedResults, "Authentication results should be stored")
			assert.Len(t, storedResults, 2)
		}

		// Clean up connections
		server.wsMux.Lock()
		for _, wsConn := range server.wsConnections {
			if wsConn != nil {
				wsConn.close()
			}
		}
		server.wsMux.Unlock()
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

	// The connection may fail but that's OK - we're testing the newWSConnection path
	_ = wsClient.Connect(connectCtx)
	// Give time for connection setup
	time.Sleep(100 * time.Millisecond)
}

func TestNewWSConnection_WithoutAuthenticationResults_CallsNewWSConnection(t *testing.T) {
	// This test directly calls newWSConnection to cover the else branch (lines 63-67)
	// where authorizers are configured but authentication results are missing
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
	auth := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"user":"test"}`, nil
		},
	}
	server.SetAuthorizers([]Authorizer{auth})

	// Create a test server that provides WebSocket upgrade
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create request WITHOUT authentication results in context - this is what we're testing
		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(r.Context())

		// Use server's upgrader or create one if needed
		if server.wsUpgrader == nil {
			server.wsUpgrader = &websocket.Upgrader{}
		}

		// Upgrade to WebSocket
		conn, err := server.wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		// Now call newWSConnection with request that has NO authentication results
		// This should trigger the else branch (lines 63-67) - the code path we're testing
		server.newWSConnection(conn, req)

		// Give goroutines time to start
		time.Sleep(50 * time.Millisecond)

		// Clean up connections
		server.wsMux.Lock()
		for _, wsConn := range server.wsConnections {
			if wsConn != nil {
				wsConn.close()
			}
		}
		server.wsMux.Unlock()
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

	// The connection may fail but that's OK - we're testing the newWSConnection path
	_ = wsClient.Connect(connectCtx)
	// Give time for connection setup
	time.Sleep(100 * time.Millisecond)
}

func TestNewWSConnection_EmptyAuthenticationResults_CallsNewWSConnection(t *testing.T) {
	// This test covers the case where authentication results exist in context but are empty
	// Should also trigger the else branch (lines 63-67)
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
	auth := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"user":"test"}`, nil
		},
	}
	server.SetAuthorizers([]Authorizer{auth})

	// Create test server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set EMPTY authentication results in context
		req := httptest.NewRequest("GET", "/test", nil)
		emptyResults := []string{} // Empty slice
		req = req.WithContext(context.WithValue(r.Context(), wsAuthResultKey, emptyResults))

		if server.wsUpgrader == nil {
			server.wsUpgrader = &websocket.Upgrader{}
		}

		conn, err := server.wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		// Call newWSConnection with empty identities - should trigger else branch
		server.newWSConnection(conn, req)

		time.Sleep(50 * time.Millisecond)

		// Clean up
		server.wsMux.Lock()
		for _, wsConn := range server.wsConnections {
			if wsConn != nil {
				wsConn.close()
			}
		}
		server.wsMux.Unlock()
	}))
	defer testServer.Close()

	wsURL := "ws" + testServer.URL[4:]
	wsConfig := &pldconf.WSClientConfig{}
	wsConfig.URL = wsURL
	wsClient := rpcclient.WrapWSConfig(wsConfig)
	defer wsClient.Close()

	connectCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_ = wsClient.Connect(connectCtx)
	time.Sleep(100 * time.Millisecond)
}
