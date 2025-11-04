// Copyright © 2022 Kaleido, Inc.
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
	"encoding/json"
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRCPMissingID(t *testing.T) {

	url, _, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	var errResponse rpcclient.RPCResponse
	res, err := resty.New().R().
		SetBody(`{}`).
		SetError(&errResponse).
		Post(url)
	require.NoError(t, err)
	assert.False(t, res.IsSuccess())
	assert.Equal(t, int64(rpcclient.RPCCodeInvalidRequest), errResponse.Error.Code)
	assert.Regexp(t, "PD020701", errResponse.Error.Message)

}

func TestRCPUnknownMethod(t *testing.T) {

	url, _, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	var errResponse rpcclient.RPCResponse
	res, err := resty.New().R().
		SetBody(`{
		  "id": 12345,
		  "method": "wrong"
		}`).
		SetError(&errResponse).
		Post(url)
	require.NoError(t, err)
	assert.False(t, res.IsSuccess())
	assert.Equal(t, int64(rpcclient.RPCCodeInvalidRequest), errResponse.Error.Code)
	assert.Regexp(t, "PD020702", errResponse.Error.Message)

}

// mockAuthorizer is a mock implementation of the Authorizer interface for testing
type mockAuthorizer struct {
	authenticateFunc func(ctx context.Context, headers map[string]string) (string, error)
	authorizeFunc    func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error)
}

func (m *mockAuthorizer) Authenticate(ctx context.Context, headers map[string]string) (string, error) {
	if m.authenticateFunc != nil {
		return m.authenticateFunc(ctx, headers)
	}
	return `{"username":"test"}`, nil
}

func (m *mockAuthorizer) Authorize(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
	if m.authorizeFunc != nil {
		return m.authorizeFunc(ctx, result, method, payload)
	}
	return &AuthResult{Authorized: true}, nil
}

func TestRPCProcessor_WithAuth_Success(t *testing.T) {
	_, server, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	// Set up mock authorizer that allows everything
	auth := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"username":"test"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			assert.NotEmpty(t, method)
			return &AuthResult{Authorized: true}, nil
		},
	}
	server.SetAuthorizers([]Authorizer{auth})

	// Test that authorizers were set
	assert.NotNil(t, server.authorizers)
	assert.Len(t, server.authorizers, 1)
}

func TestRPCProcessor_WithoutAuth_Bypass(t *testing.T) {
	// No authorizer configured - should bypass auth
	_, server, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	// Test that no authorizers are set
	assert.Empty(t, server.authorizers)
}

func TestExtractHeadersFromContext(t *testing.T) {
	// Test extracting headers from context
	expectedHeaders := map[string]string{
		"Authorization": "Basic dXNlcjpwYXNz",
		"Content-Type":  "application/json",
	}

	ctx := context.WithValue(context.Background(), httpHeadersKey, expectedHeaders)
	headers := extractHeadersFromContext(ctx)

	assert.Equal(t, "Basic dXNlcjpwYXNz", headers["Authorization"])
	assert.Equal(t, "application/json", headers["Content-Type"])
}

func TestExtractHeadersFromContext_NoRequest(t *testing.T) {
	// Test with no HTTP request in context
	ctx := context.Background()
	headers := extractHeadersFromContext(ctx)

	assert.Empty(t, headers)
}

func TestProcessRPC_AuthorizerCalled(t *testing.T) {
	url, server, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	// Track if authorizer was called
	authorizerCalled := false
	methodReceived := ""
	headersReceived := map[string]string{}

	auth := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			headersReceived = headers
			return `{"username":"test"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			authorizerCalled = true
			methodReceived = method
			return &AuthResult{Authorized: true}, nil
		},
	}
	server.SetAuthorizers([]Authorizer{auth})

	// Register a method so request doesn't fail on unknown method
	regTestRPC(server, "test_echo", RPCMethod0(func(ctx context.Context) (string, error) {
		return "hello", nil
	}))

	// Make request with Authorization header
	var response rpcclient.RPCResponse
	res, err := resty.New().R().
		SetHeader("Authorization", "Bearer token123").
		SetBody(`{"id": 1, "method": "test_echo"}`).
		SetResult(&response).
		Post(url)
	require.NoError(t, err)

	// Authorizer should have been called
	assert.True(t, authorizerCalled)
	assert.Equal(t, "test_echo", methodReceived)
	assert.Equal(t, "Bearer token123", headersReceived["Authorization"])
	assert.True(t, res.IsSuccess())
}

func TestProcessRPC_AuthorizerDeniesAccess(t *testing.T) {
	url, server, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	auth := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"username":"test"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			return &AuthResult{
				Authorized:   false,
				ErrorMessage: "access denied: invalid token",
			}, nil
		},
	}
	server.SetAuthorizers([]Authorizer{auth})

	// Make request - should be denied
	var errResponse rpcclient.RPCResponse
	res, err := resty.New().R().
		SetHeader("Authorization", "Bearer invalid").
		SetBody(`{"id": 1, "method": "test_method"}`).
		SetError(&errResponse).
		Post(url)
	require.NoError(t, err)

	assert.False(t, res.IsSuccess())
	assert.Equal(t, int64(rpcclient.RPCCodeUnauthorized), errResponse.Error.Code)
	assert.Contains(t, errResponse.Error.Message, "access denied")
}

func TestProcessRPC_AuthorizerReturnsError(t *testing.T) {
	url, server, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	auth := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return "", assert.AnError
		},
	}
	server.SetAuthorizers([]Authorizer{auth})

	// Make request - should fail due to authorizer error
	var errResponse rpcclient.RPCResponse
	res, err := resty.New().R().
		SetBody(`{"id": 1, "method": "test_method"}`).
		SetError(&errResponse).
		Post(url)
	require.NoError(t, err)

	assert.False(t, res.IsSuccess())
	assert.Equal(t, int64(rpcclient.RPCCodeUnauthorized), errResponse.Error.Code)
}

func TestProcessRPC_PayloadMarshaled(t *testing.T) {
	url, server, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	payloadReceived := []byte{}
	auth := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"username":"test"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			payloadReceived = payload
			return &AuthResult{Authorized: true}, nil
		},
	}
	server.SetAuthorizers([]Authorizer{auth})

	regTestRPC(server, "test_echo", RPCMethod0(func(ctx context.Context) (string, error) {
		return "hello", nil
	}))

	var response rpcclient.RPCResponse
	res, err := resty.New().R().
		SetBody(`{"id": 1, "method": "test_echo", "params": []}`).
		SetResult(&response).
		Post(url)
	require.NoError(t, err)

	// Verify payload was marshaled and contains the method
	assert.NotEmpty(t, payloadReceived)
	assert.Contains(t, string(payloadReceived), "test_echo")
	assert.True(t, res.IsSuccess())
}

func TestProcessRPC_NoAuthorizer_Bypass(t *testing.T) {
	url, s, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	// No authorizer set - should bypass
	regTestRPC(s, "test_method", RPCMethod0(func(ctx context.Context) (string, error) {
		return "success", nil
	}))

	var response rpcclient.RPCResponse
	res, err := resty.New().R().
		SetBody(`{"id": 1, "method": "test_method"}`).
		SetResult(&response).
		Post(url)
	require.NoError(t, err)
	assert.True(t, res.IsSuccess())
	// Result is a JSON-encoded string, need to unmarshal it
	var result string
	err = json.Unmarshal(response.Result, &result)
	require.NoError(t, err)
	assert.Equal(t, "success", result)
}

func TestRPCProcessor_ChainAuthenticate_Success(t *testing.T) {
	url, server, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	// Set up chain of two authorizers that both succeed
	auth1 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"plugin":"auth1","user":"test"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			return &AuthResult{Authorized: true}, nil
		},
	}
	auth2 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"plugin":"auth2","user":"test"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			return &AuthResult{Authorized: true}, nil
		},
	}
	server.SetAuthorizers([]Authorizer{auth1, auth2})

	regTestRPC(server, "test_echo", RPCMethod0(func(ctx context.Context) (string, error) {
		return "hello", nil
	}))

	var response rpcclient.RPCResponse
	res, err := resty.New().R().
		SetHeader("Authorization", "Bearer token123").
		SetBody(`{"id": 1, "method": "test_echo"}`).
		SetResult(&response).
		Post(url)
	require.NoError(t, err)
	assert.True(t, res.IsSuccess())
}

func TestRPCProcessor_ChainAuthenticate_FirstFails(t *testing.T) {
	url, server, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

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
	server.SetAuthorizers([]Authorizer{auth1, auth2})

	var errResponse rpcclient.RPCResponse
	res, err := resty.New().R().
		SetBody(`{"id": 1, "method": "test_method"}`).
		SetError(&errResponse).
		Post(url)
	require.NoError(t, err)

	assert.False(t, res.IsSuccess())
	assert.True(t, auth1Called)
	assert.False(t, auth2Called) // Should not be called after first failure
	assert.Equal(t, int64(rpcclient.RPCCodeUnauthorized), errResponse.Error.Code)
}

func TestRPCProcessor_ChainAuthenticate_MiddleFails(t *testing.T) {
	url, server, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

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
	server.SetAuthorizers([]Authorizer{auth1, auth2, auth3})

	var errResponse rpcclient.RPCResponse
	res, err := resty.New().R().
		SetBody(`{"id": 1, "method": "test_method"}`).
		SetError(&errResponse).
		Post(url)
	require.NoError(t, err)

	assert.False(t, res.IsSuccess())
	assert.True(t, auth1Called)
	assert.True(t, auth2Called)
	assert.False(t, auth3Called) // Should not be called after second failure
	assert.Equal(t, int64(rpcclient.RPCCodeUnauthorized), errResponse.Error.Code)
}

func TestRPCProcessor_ChainAuthorize_Success(t *testing.T) {
	url, server, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	// Both authorizers succeed in authenticate and authorize
	auth1 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"plugin":"auth1"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			// Verify authentication result matches what auth1 returned
			assert.Contains(t, result, "auth1")
			return &AuthResult{Authorized: true}, nil
		},
	}
	auth2 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"plugin":"auth2"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			// Verify authentication result matches what auth2 returned
			assert.Contains(t, result, "auth2")
			return &AuthResult{Authorized: true}, nil
		},
	}
	server.SetAuthorizers([]Authorizer{auth1, auth2})

	regTestRPC(server, "test_echo", RPCMethod0(func(ctx context.Context) (string, error) {
		return "hello", nil
	}))

	var response rpcclient.RPCResponse
	res, err := resty.New().R().
		SetBody(`{"id": 1, "method": "test_echo"}`).
		SetResult(&response).
		Post(url)
	require.NoError(t, err)
	assert.True(t, res.IsSuccess())
}

func TestRPCProcessor_ChainAuthorize_Fails(t *testing.T) {
	url, server, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	// Both succeed in authenticate, but first fails in authorize
	auth1Called := false
	auth2Called := false
	auth1 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"plugin":"auth1"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			auth1Called = true
			return &AuthResult{
				Authorized:   false,
				ErrorMessage: "access denied by auth1",
			}, nil
		},
	}
	auth2 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"plugin":"auth2"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			auth2Called = true
			return &AuthResult{Authorized: true}, nil
		},
	}
	server.SetAuthorizers([]Authorizer{auth1, auth2})

	var errResponse rpcclient.RPCResponse
	res, err := resty.New().R().
		SetBody(`{"id": 1, "method": "test_method"}`).
		SetError(&errResponse).
		Post(url)
	require.NoError(t, err)

	assert.False(t, res.IsSuccess())
	assert.True(t, auth1Called)
	assert.False(t, auth2Called) // Should not be called after first authorization failure
	assert.Equal(t, int64(rpcclient.RPCCodeUnauthorized), errResponse.Error.Code)
	assert.Contains(t, errResponse.Error.Message, "access denied by auth1")
}

func TestRPCProcessor_ChainAuthorize_MiddleFails(t *testing.T) {
	url, server, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	// First succeeds in both authenticate and authorize, second fails in authorize
	auth1AuthCalled := false
	auth1AuthzCalled := false
	auth2AuthCalled := false
	auth2AuthzCalled := false
	auth3AuthzCalled := false

	auth1 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			auth1AuthCalled = true
			return `{"plugin":"auth1"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			auth1AuthzCalled = true
			return &AuthResult{Authorized: true}, nil
		},
	}
	auth2 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			auth2AuthCalled = true
			return `{"plugin":"auth2"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			auth2AuthzCalled = true
			return &AuthResult{
				Authorized:   false,
				ErrorMessage: "access denied by auth2",
			}, nil
		},
	}
	auth3 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"plugin":"auth3"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			auth3AuthzCalled = true
			return &AuthResult{Authorized: true}, nil
		},
	}
	server.SetAuthorizers([]Authorizer{auth1, auth2, auth3})

	regTestRPC(server, "test_method", RPCMethod0(func(ctx context.Context) (string, error) {
		return "success", nil
	}))

	var errResponse rpcclient.RPCResponse
	res, err := resty.New().R().
		SetBody(`{"id": 1, "method": "test_method"}`).
		SetError(&errResponse).
		Post(url)
	require.NoError(t, err)

	assert.False(t, res.IsSuccess())
	assert.True(t, auth1AuthCalled)
	assert.True(t, auth1AuthzCalled)
	assert.True(t, auth2AuthCalled)
	assert.True(t, auth2AuthzCalled)
	assert.False(t, auth3AuthzCalled) // Should not be called after auth2 authorization failure
	assert.Equal(t, int64(rpcclient.RPCCodeUnauthorized), errResponse.Error.Code)
	assert.Contains(t, errResponse.Error.Message, "access denied by auth2")
}

func TestRPCProcessor_ChainAuthorize_ReturnsError(t *testing.T) {
	url, server, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	// Chain where second authorizer's Authorize() returns an error
	auth1AuthCalled := false
	auth1AuthzCalled := false
	auth2AuthCalled := false
	auth2AuthzCalled := false
	auth3AuthzCalled := false

	auth1 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			auth1AuthCalled = true
			return `{"plugin":"auth1"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			auth1AuthzCalled = true
			return &AuthResult{Authorized: true}, nil
		},
	}
	auth2 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			auth2AuthCalled = true
			return `{"plugin":"auth2"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			auth2AuthzCalled = true
			return nil, assert.AnError
		},
	}
	auth3 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"plugin":"auth3"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			auth3AuthzCalled = true
			return &AuthResult{Authorized: true}, nil
		},
	}
	server.SetAuthorizers([]Authorizer{auth1, auth2, auth3})

	regTestRPC(server, "test_method", RPCMethod0(func(ctx context.Context) (string, error) {
		return "success", nil
	}))

	var errResponse rpcclient.RPCResponse
	res, err := resty.New().R().
		SetBody(`{"id": 1, "method": "test_method"}`).
		SetError(&errResponse).
		Post(url)
	require.NoError(t, err)

	assert.False(t, res.IsSuccess())
	assert.True(t, auth1AuthCalled)
	assert.True(t, auth1AuthzCalled)
	assert.True(t, auth2AuthCalled)
	assert.True(t, auth2AuthzCalled)
	assert.False(t, auth3AuthzCalled) // Should not be called after auth2 returns error
	assert.Equal(t, int64(rpcclient.RPCCodeUnauthorized), errResponse.Error.Code)
}

func TestRPCProcessor_WebSocket_NoStoredIdentities(t *testing.T) {
	// Create a minimal webSocketConnection with no authentication results
	ctx := context.Background()
	wsc := &webSocketConnection{
		authenticationResults: []string{}, // Empty authentication results
	}

	// Create server with authorizers
	_, server, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	auth1 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"plugin":"auth1"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			return &AuthResult{Authorized: true}, nil
		},
	}
	server.SetAuthorizers([]Authorizer{auth1})

	regTestRPC(server, "test_method", RPCMethod0(func(ctx context.Context) (string, error) {
		return "success", nil
	}))

	// Create RPC request
	rpcReq := &rpcclient.RPCRequest{
		ID:     pldtypes.RawJSON(`1`),
		Method: "test_method",
	}

	// Call processRPC directly with WebSocket connection that has no identities
	response, isOK := server.processRPC(ctx, rpcReq, wsc)

	assert.False(t, isOK)
	assert.NotNil(t, response)
	assert.NotNil(t, response.Error)
	assert.Equal(t, int64(rpcclient.RPCCodeUnauthorized), response.Error.Code)
	assert.Contains(t, response.Error.Message, "no authentication results")
}

func TestRPCProcessor_AuthenticationResultMismatch(t *testing.T) {
	// Create a webSocketConnection with fewer authentication results than authorizers
	ctx := context.Background()
	wsc := &webSocketConnection{
		authenticationResults: []string{`{"plugin":"auth1"}`}, // Only one authentication result, but we'll have 2 authorizers
	}

	// Create server with 2 authorizers
	_, server, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	auth1 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"plugin":"auth1"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			return &AuthResult{Authorized: true}, nil
		},
	}
	auth2 := &mockAuthorizer{
		authenticateFunc: func(ctx context.Context, headers map[string]string) (string, error) {
			return `{"plugin":"auth2"}`, nil
		},
		authorizeFunc: func(ctx context.Context, result string, method string, payload []byte) (*AuthResult, error) {
			return &AuthResult{Authorized: true}, nil
		},
	}
	server.SetAuthorizers([]Authorizer{auth1, auth2})

	regTestRPC(server, "test_method", RPCMethod0(func(ctx context.Context) (string, error) {
		return "success", nil
	}))

	// Create RPC request
	rpcReq := &rpcclient.RPCRequest{
		ID:     pldtypes.RawJSON(`1`),
		Method: "test_method",
	}

	// Call processRPC directly with WebSocket connection that has authentication result mismatch
	response, isOK := server.processRPC(ctx, rpcReq, wsc)

	assert.False(t, isOK)
	assert.NotNil(t, response)
	assert.NotNil(t, response.Error)
	assert.Equal(t, int64(rpcclient.RPCCodeUnauthorized), response.Error.Code)
	assert.Contains(t, response.Error.Message, "authentication result mismatch")
}
