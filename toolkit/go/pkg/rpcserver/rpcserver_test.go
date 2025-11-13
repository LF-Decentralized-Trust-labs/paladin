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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setTraceForTest(t *testing.T) {
	log.EnsureInit()
	l := log.GetLevel()
	log.SetLevel("trace")
	t.Cleanup(func() {
		log.SetLevel(l)
	})
}

func newTestServerHTTP(t *testing.T, conf *pldconf.RPCServerConfig) (string, *rpcServer, func()) {
	setTraceForTest(t)

	conf.HTTP.Address = confutil.P("127.0.0.1")
	conf.HTTP.Port = confutil.P(0)
	conf.WS.Disabled = true
	s, err := NewRPCServer(context.Background(), conf)
	require.NoError(t, err)
	err = s.Start()
	require.NoError(t, err)
	return fmt.Sprintf("http://%s", s.HTTPAddr()), s, s.Stop

}

func newTestServerWebSockets(t *testing.T, conf *pldconf.RPCServerConfig) (string, *rpcServer, func()) {

	conf.WS.Address = confutil.P("127.0.0.1")
	conf.WS.Port = confutil.P(0)
	conf.HTTP.Disabled = true
	s, err := NewRPCServer(context.Background(), conf)
	require.NoError(t, err)
	err = s.Start()
	require.NoError(t, err)
	return fmt.Sprintf("ws://%s", s.WSAddr()), s, s.Stop

}

func regTestRPC(s *rpcServer, method string, handler RPCHandler) {
	group := strings.SplitN(method, "_", 2)[0]
	module := s.rpcModules[group]
	if module == nil {
		module = NewRPCModule(group)
		s.Register(module)
	}
	module.Add(method, handler)
}

func TestBadHTTPConfig(t *testing.T) {

	_, err := NewRPCServer(context.Background(), &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Address: confutil.P("::::::wrong"),
			},
		},
		WS: pldconf.RPCServerConfigWS{Disabled: true},
	})
	assert.Regexp(t, "PD020601", err)

}

func TestBadWSConfig(t *testing.T) {

	_, err := NewRPCServer(context.Background(), &pldconf.RPCServerConfig{
		WS: pldconf.RPCServerConfigWS{
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Address: confutil.P("::::::wrong"),
			},
		},
		HTTP: pldconf.RPCServerConfigHTTP{Disabled: true},
	})
	assert.Regexp(t, "PD020601", err)

}

func TestBadHTTPMethod(t *testing.T) {

	url, _, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	res, err := http.DefaultClient.Get(url)
	require.NoError(t, err)
	assert.Equal(t, http.StatusMethodNotAllowed, res.StatusCode)

}

func TestBadWSUpgrade(t *testing.T) {

	_, s, done := newTestServerWebSockets(t, &pldconf.RPCServerConfig{})
	defer done()

	res, err := http.DefaultClient.Get(fmt.Sprintf("http://%s", s.WSAddr()))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)

}

func TestWSHandler(t *testing.T) {
	rpcServer, err := NewRPCServer(context.Background(), &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{Disabled: true},
		WS: pldconf.RPCServerConfigWS{
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Port: confutil.P(0),
			},
		},
	})
	require.NoError(t, err)
	defer rpcServer.Stop()

	req := httptest.NewRequest("GET", "/test", nil)
	res := httptest.NewRecorder()
	rpcServer.WSHandler(res, req)
	assert.Equal(t, http.StatusBadRequest, res.Code)
}

func TestWSHandler_AuthenticationFailure(t *testing.T) {
	rpcServer, err := NewRPCServer(context.Background(), &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{Disabled: true},
		WS: pldconf.RPCServerConfigWS{
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Port: confutil.P(0),
			},
		},
	})
	require.NoError(t, err)
	defer rpcServer.Stop()

	// Set up authorizer that fails
	auth := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return "", assert.AnError
		},
	}
	rpcServer.SetAuthorizers([]Authorizer{auth})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	res := httptest.NewRecorder()

	rpcServer.WSHandler(res, req)

	// Should return 401 Unauthorized without JSON body (native transport response)
	assert.Equal(t, http.StatusUnauthorized, res.Code)
	assert.Empty(t, res.Body.String())
}

func TestWSHandler_AuthenticationSuccess(t *testing.T) {
	rpcServer, err := NewRPCServer(context.Background(), &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{Disabled: true},
		WS: pldconf.RPCServerConfigWS{
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Port: confutil.P(0),
			},
		},
	})
	require.NoError(t, err)
	defer rpcServer.Stop()

	// Set up authorizer that succeeds
	auth := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"user":"test"}`, nil
		},
	}
	rpcServer.SetAuthorizers([]Authorizer{auth})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	res := httptest.NewRecorder()

	// Note: This will fail upgrade in test environment, but we're testing the auth path
	rpcServer.WSHandler(res, req)

	// Should not return 401 (auth succeeded), might fail on upgrade which is expected in test
	assert.NotEqual(t, http.StatusUnauthorized, res.Code)
}

func TestWSHandler_ChainAuthentication_Failure(t *testing.T) {
	rpcServer, err := NewRPCServer(context.Background(), &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{Disabled: true},
		WS: pldconf.RPCServerConfigWS{
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Port: confutil.P(0),
			},
		},
	})
	require.NoError(t, err)
	defer rpcServer.Stop()

	// First succeeds, second fails
	auth1Called := false
	auth2Called := false
	auth1 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			auth1Called = true
			return `{"user":"test"}`, nil
		},
	}
	auth2 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			auth2Called = true
			return "", assert.AnError
		},
	}
	rpcServer.SetAuthorizers([]Authorizer{auth1, auth2})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	res := httptest.NewRecorder()

	rpcServer.WSHandler(res, req)

	// Should return 401 because second authorizer failed
	assert.Equal(t, http.StatusUnauthorized, res.Code)
	assert.True(t, auth1Called)
	assert.True(t, auth2Called)
}
func TestHTTPHandler(t *testing.T) {
	rpcServer, err := NewRPCServer(context.Background(), &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{
			Disabled: false,
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Address: confutil.P("127.0.0.1"),
				Port:    confutil.P(0), // Use dynamic port
			},
		},
		WS: pldconf.RPCServerConfigWS{Disabled: true},
	})
	require.NoError(t, err)
	defer rpcServer.Stop()

	req := httptest.NewRequest("POST", "/", nil)
	res := httptest.NewRecorder()
	rpcServer.HTTPHandler(res, req)
	assert.Equal(t, http.StatusInternalServerError, res.Code)
}

func TestHTTPHandler_AuthenticationFailure(t *testing.T) {
	rpcServer, err := NewRPCServer(context.Background(), &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{
			Disabled: false,
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Address: confutil.P("127.0.0.1"),
				Port:    confutil.P(0), // Use dynamic port
			},
		},
		WS: pldconf.RPCServerConfigWS{Disabled: true},
	})
	require.NoError(t, err)
	defer rpcServer.Stop()

	// Set up authorizer that fails
	auth := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return "", assert.AnError
		},
	}
	rpcServer.SetAuthorizers([]Authorizer{auth})

	// Create a valid JSON-RPC request
	body := strings.NewReader(`{"jsonrpc":"2.0","method":"test_method","id":1}`)
	req := httptest.NewRequest("POST", "/", body)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	rpcServer.HTTPHandler(res, req)

	// Should return HTTP 401 without JSON body (native transport response)
	assert.Equal(t, http.StatusUnauthorized, res.Code)
	assert.Empty(t, res.Body.String())
}

func TestHTTPHandler_ChainAuthentication_Success(t *testing.T) {
	rpcServer, err := NewRPCServer(context.Background(), &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{
			Disabled: false,
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Address: confutil.P("127.0.0.1"),
				Port:    confutil.P(0),
			},
		},
		WS: pldconf.RPCServerConfigWS{Disabled: true},
	})
	require.NoError(t, err)
	defer rpcServer.Stop()

	// Set up chain of two authorizers that both succeed
	auth1Called := false
	auth2Called := false
	auth1 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			auth1Called = true
			return `{"plugin":"auth1","user":"test"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) bool {
			return true
		},
	}
	auth2 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			auth2Called = true
			return `{"plugin":"auth2","user":"test"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) bool {
			return true
		},
	}
	rpcServer.SetAuthorizers([]Authorizer{auth1, auth2})

	regTestRPC(rpcServer, "test_echo", RPCMethod0(func(ctx context.Context) (string, error) {
		return "hello", nil
	}))

	// Create a valid JSON-RPC request
	body := strings.NewReader(`{"jsonrpc":"2.0","method":"test_echo","id":1}`)
	req := httptest.NewRequest("POST", "/", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token123")
	res := httptest.NewRecorder()

	rpcServer.HTTPHandler(res, req)

	// Both authorizers should have been called
	assert.True(t, auth1Called)
	assert.True(t, auth2Called)
	// Should succeed (not return 401)
	assert.NotEqual(t, http.StatusUnauthorized, res.Code)
	assert.Equal(t, http.StatusOK, res.Code)
}

func TestHTTPHandler_ChainAuthentication_FirstFails(t *testing.T) {
	rpcServer, err := NewRPCServer(context.Background(), &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{
			Disabled: false,
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Address: confutil.P("127.0.0.1"),
				Port:    confutil.P(0),
			},
		},
		WS: pldconf.RPCServerConfigWS{Disabled: true},
	})
	require.NoError(t, err)
	defer rpcServer.Stop()

	// First authorizer fails, second should never be called
	auth1Called := false
	auth2Called := false
	auth1 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			auth1Called = true
			return "", assert.AnError
		},
	}
	auth2 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			auth2Called = true
			return `{"plugin":"auth2"}`, nil
		},
	}
	rpcServer.SetAuthorizers([]Authorizer{auth1, auth2})

	// Create a valid JSON-RPC request
	body := strings.NewReader(`{"jsonrpc":"2.0","method":"test_method","id":1}`)
	req := httptest.NewRequest("POST", "/", body)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	rpcServer.HTTPHandler(res, req)

	// First should be called, second should not
	assert.True(t, auth1Called)
	assert.False(t, auth2Called) // Should not be called after first failure
	// Should return HTTP 401 without JSON body (native transport response)
	assert.Equal(t, http.StatusUnauthorized, res.Code)
	assert.Empty(t, res.Body.String())
}

func TestHTTPHandler_ChainAuthentication_MiddleFails(t *testing.T) {
	rpcServer, err := NewRPCServer(context.Background(), &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{
			Disabled: false,
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Address: confutil.P("127.0.0.1"),
				Port:    confutil.P(0),
			},
		},
		WS: pldconf.RPCServerConfigWS{Disabled: true},
	})
	require.NoError(t, err)
	defer rpcServer.Stop()

	// First succeeds, second fails, third should never be called
	auth1Called := false
	auth2Called := false
	auth3Called := false
	auth1 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			auth1Called = true
			return `{"plugin":"auth1"}`, nil
		},
	}
	auth2 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			auth2Called = true
			return "", assert.AnError
		},
	}
	auth3 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			auth3Called = true
			return `{"plugin":"auth3"}`, nil
		},
	}
	rpcServer.SetAuthorizers([]Authorizer{auth1, auth2, auth3})

	// Create a valid JSON-RPC request
	body := strings.NewReader(`{"jsonrpc":"2.0","method":"test_method","id":1}`)
	req := httptest.NewRequest("POST", "/", body)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	rpcServer.HTTPHandler(res, req)

	// First and second should be called, third should not
	assert.True(t, auth1Called)
	assert.True(t, auth2Called)
	assert.False(t, auth3Called) // Should not be called after second failure
	// Should return HTTP 401 without JSON body (native transport response)
	assert.Equal(t, http.StatusUnauthorized, res.Code)
	assert.Empty(t, res.Body.String())
}

// Helper to create a temporary UI directory with files for testing
func setupUITestDir(t *testing.T, relativeDir string) string {
	tmpDir := t.TempDir()
	tmpDirP := filepath.Join(tmpDir, relativeDir)
	_ = os.Mkdir(tmpDirP, 0755)
	_ = os.WriteFile(filepath.Join(tmpDirP, "index.html"), []byte("<html><body>Some content</body></html>"), 0644)
	return tmpDirP
}
func TestNewRPCServerWithStaticServerEnabled(t *testing.T) {
	urlPath := "/ui"
	tmpDir := setupUITestDir(t, urlPath)

	// Configure RPC server with UI enabled
	conf := &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{
			Disabled: false,
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Address: confutil.P("127.0.0.1"),
				Port:    confutil.P(0), // Use dynamic port
			},
			StaticServers: []pldconf.StaticServerConfig{
				{
					Enabled:    true,
					StaticPath: tmpDir,
					URLPath:    urlPath,
				},
			},
		},
		WS: pldconf.RPCServerConfigWS{Disabled: true},
	}

	ctx := context.Background()
	rpcServer, err := NewRPCServer(ctx, conf)
	require.NoError(t, err)

	// Start the server
	err = rpcServer.Start()
	require.NoError(t, err)
	defer rpcServer.Stop()

	// Retrieve the HTTP server address (e.g., http://127.0.0.1:12345)
	serverURL := fmt.Sprintf("http://%s%s", rpcServer.HTTPAddr(), urlPath)

	// Perform a real HTTP GET request to the url path
	res, err := http.Get(serverURL)
	require.NoError(t, err)
	defer res.Body.Close()

	// Verify the status code and response content
	require.Equal(t, http.StatusOK, res.StatusCode)

	body, _ := io.ReadAll(res.Body)
	assert.Contains(t, string(body), "<html><body>Some content</body></html>")
}

func TestNewRPCServerWithStaticServerDisabled(t *testing.T) {
	urlPath := "/ui"
	tmpDir := setupUITestDir(t, urlPath)

	// Configure RPC server with UI enabled
	conf := &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{
			Disabled: false,
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Address: confutil.P("127.0.0.1"),
				Port:    confutil.P(0), // Use dynamic port
			},
			StaticServers: []pldconf.StaticServerConfig{
				{
					Enabled:    false,
					StaticPath: tmpDir,
					URLPath:    urlPath,
				},
			},
		},
		WS: pldconf.RPCServerConfigWS{Disabled: true},
	}

	ctx := context.Background()
	rpcServer, err := NewRPCServer(ctx, conf)
	require.NoError(t, err)

	// Start the server
	err = rpcServer.Start()
	require.NoError(t, err)
	defer rpcServer.Stop()

	// Retrieve the HTTP server address (e.g., http://127.0.0.1:12345)
	serverURL := fmt.Sprintf("http://%s%s", rpcServer.HTTPAddr(), urlPath)

	// Perform a real HTTP GET request to the url path
	res, err := http.Get(serverURL)
	require.NoError(t, err)
	defer res.Body.Close()

	// Verify the status code
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}
