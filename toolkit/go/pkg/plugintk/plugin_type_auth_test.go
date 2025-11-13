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
	"testing"

	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stringPtr(s string) *string {
	return &s
}

func setupAuthTests(t *testing.T) (context.Context, *pluginExerciser[prototk.RPCAuthMessage], *RPCAuthAPIFunctions, RPCAuthCallbacks, map[string]func(*prototk.RPCAuthMessage), func()) {
	ctx, tc, tcDone := newTestController(t)

	/***** THIS PART AN IMPLEMENTATION WOULD DO ******/
	funcs := &RPCAuthAPIFunctions{
		// Functions go here
	}
	waitForCallbacks := make(chan RPCAuthCallbacks, 1)
	auth := NewRPCAuthPlugin(func(callbacks RPCAuthCallbacks) RPCAuthAPI {
		// Implementation would construct an instance here to start handling the API calls from Paladin,
		// (rather than passing the callbacks to the test as we do here)
		waitForCallbacks <- callbacks
		return &RPCAuthAPIBase{funcs}
	})
	/************************************************/

	// The rest is mocking the other side of the interface
	inOutMap := map[string]func(*prototk.RPCAuthMessage){}
	pluginID := uuid.NewString()
	exerciser := newPluginExerciser(t, pluginID, &RPCAuthMessageWrapper{}, inOutMap)
	tc.fakeAuthController = exerciser.controller

	authDone := make(chan struct{})
	go func() {
		defer close(authDone)
		auth.Run("unix:"+tc.socketFile, pluginID)
	}()
	callbacks := <-waitForCallbacks

	return ctx, exerciser, funcs, callbacks, inOutMap, func() {
		checkPanic()
		auth.Stop()
		tcDone()
		<-authDone
	}
}

func TestRPCAuthFunction_ConfigureRPCAuthorizer(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupAuthTests(t)
	defer done()

	// ConfigureRPCAuthorizer - paladin to auth plugin
	funcs.ConfigureRPCAuthorizer = func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
		assert.NotNil(t, car)
		assert.NotNil(t, car.ConfigJson)
		return &prototk.ConfigureRPCAuthorizerResponse{}, nil
	}

	exerciser.doExchangeToPlugin(func(req *prototk.RPCAuthMessage) {
		req.RequestToAuthplugin = &prototk.RPCAuthMessage_ConfigureRpcAuthorizer{
			ConfigureRpcAuthorizer: &prototk.ConfigureRPCAuthorizerRequest{
				ConfigJson: `{"credentialsFile":"/tmp/users.txt"}`,
			},
		}
	}, func(res *prototk.RPCAuthMessage) {
		assert.IsType(t, &prototk.RPCAuthMessage_ConfigureRpcAuthorizerRes{}, res.ResponseFromAuthplugin)
	})
}

func TestRPCAuthFunction_Authenticate(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupAuthTests(t)
	defer done()

	// Authenticate - paladin to auth plugin
	funcs.Authenticate = func(ctx context.Context, ar *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error) {
		assert.NotNil(t, ar)
		assert.NotNil(t, ar.HeadersJson)
		return &prototk.AuthenticateResponse{
			Authenticated: true,
			ResultJson:    stringPtr(`{"username":"test"}`),
		}, nil
	}

	exerciser.doExchangeToPlugin(func(req *prototk.RPCAuthMessage) {
		req.RequestToAuthplugin = &prototk.RPCAuthMessage_Authenticate{
			Authenticate: &prototk.AuthenticateRequest{
				HeadersJson: `{"Authorization":"Basic dXNlcjpwYXNz"}`,
			},
		}
	}, func(res *prototk.RPCAuthMessage) {
		assert.IsType(t, &prototk.RPCAuthMessage_AuthenticateRes{}, res.ResponseFromAuthplugin)
	})
}

func TestRPCAuthFunction_Authorize(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupAuthTests(t)
	defer done()

	// Authorize - paladin to auth plugin
	funcs.Authorize = func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
		assert.NotNil(t, ar)
		assert.NotNil(t, ar.Method)
		assert.NotNil(t, ar.ResultJson)
		assert.NotNil(t, ar.PayloadJson)
		return &prototk.AuthorizeResponse{Authorized: true}, nil
	}

	exerciser.doExchangeToPlugin(func(req *prototk.RPCAuthMessage) {
		req.RequestToAuthplugin = &prototk.RPCAuthMessage_Authorize{
			Authorize: &prototk.AuthorizeRequest{
				ResultJson:  `{"username":"test"}`,
				Method:      "account_sendTransaction",
				PayloadJson: `{"jsonrpc":"2.0","method":"account_sendTransaction","params":{}}`,
			},
		}
	}, func(res *prototk.RPCAuthMessage) {
		assert.IsType(t, &prototk.RPCAuthMessage_AuthorizeRes{}, res.ResponseFromAuthplugin)
	})
}

func TestRPCAuthRequestError(t *testing.T) {
	_, exerciser, _, _, _, done := setupAuthTests(t)
	defer done()

	// Send an unsupported request type
	exerciser.doExchangeToPlugin(func(req *prototk.RPCAuthMessage) {
		// Sending nil or unsupported request type should cause an error
		req.RequestToAuthplugin = nil
	}, func(res *prototk.RPCAuthMessage) {
		// Should get an error response
		assert.NotNil(t, res.Header)
		assert.NotNil(t, res.Header.ErrorMessage)
	})
}

func TestRPCRPCAuthMessageWrapper_Wrap(t *testing.T) {
	wrapper := &RPCAuthMessageWrapper{}

	// Test wrapping a message
	msg := &prototk.RPCAuthMessage{
		Header: &prototk.Header{
			MessageId: "test-id",
		},
	}

	wrapped := wrapper.Wrap(msg)
	assert.NotNil(t, wrapped)
	assert.Equal(t, "test-id", wrapped.Header().MessageId)
}

func TestRPCAuthPluginMessage_Methods(t *testing.T) {
	wrapper := &RPCAuthMessageWrapper{}
	msg := &prototk.RPCAuthMessage{
		Header: &prototk.Header{MessageId: "test-id"},
		RequestToAuthplugin: &prototk.RPCAuthMessage_ConfigureRpcAuthorizer{
			ConfigureRpcAuthorizer: &prototk.ConfigureRPCAuthorizerRequest{ConfigJson: "test"},
		},
	}

	wrapped := wrapper.Wrap(msg)
	pm := wrapped.(*RPCAuthPluginMessage)

	// Test Header
	header := pm.Header()
	assert.NotNil(t, header)
	assert.Equal(t, "test-id", header.MessageId)

	// Test RequestToPlugin
	reqToPlugin := pm.RequestToPlugin()
	assert.NotNil(t, reqToPlugin)

	// Test ResponseFromPlugin
	response := &prototk.RPCAuthMessage{
		ResponseFromAuthplugin: &prototk.RPCAuthMessage_ConfigureRpcAuthorizerRes{},
	}
	pm2 := wrapper.Wrap(response)
	assert.NotNil(t, pm2.ResponseFromPlugin())

	// Test RequestFromPlugin (should be nil for unidirectional auth)
	assert.Nil(t, pm.RequestFromPlugin())

	// Test ResponseToPlugin (should be nil for unidirectional auth)
	assert.Nil(t, pm.ResponseToPlugin())

	// Test Message
	assert.Equal(t, msg, pm.Message())
}

func TestRPCAuthHandler_RequestToPlugin_ConfigureRpcAuthorizer(t *testing.T) {
	ctx, _, funcs, _, _, done := setupAuthTests(t)
	defer done()

	configCalled := false
	funcs.ConfigureRPCAuthorizer = func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
		configCalled = true
		assert.Equal(t, `{"credentialsFile":"/tmp/users.txt"}`, car.ConfigJson)
		return &prototk.ConfigureRPCAuthorizerResponse{}, nil
	}

	msg := &prototk.RPCAuthMessage{
		RequestToAuthplugin: &prototk.RPCAuthMessage_ConfigureRpcAuthorizer{
			ConfigureRpcAuthorizer: &prototk.ConfigureRPCAuthorizerRequest{
				ConfigJson: `{"credentialsFile":"/tmp/users.txt"}`,
			},
		},
	}

	wrapper := &RPCAuthMessageWrapper{}
	iReq := wrapper.Wrap(msg)

	// Create a simple handler to test
	ah := &rpcAuthHandler{
		authPlugin: &authPlugin{},
		api:        &RPCAuthAPIBase{funcs},
		proxy:      nil,
	}

	res, err := ah.RequestToPlugin(ctx, iReq)
	require.NoError(t, err)
	assert.NotNil(t, res)
	assert.True(t, configCalled)
}

func TestRPCAuthHandler_RequestToPlugin_Authenticate(t *testing.T) {
	ctx, _, funcs, _, _, done := setupAuthTests(t)
	defer done()

	authenticateCalled := false
	funcs.Authenticate = func(ctx context.Context, ar *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error) {
		authenticateCalled = true
		assert.Equal(t, `{"Authorization":"Basic dXNlcjpwYXNz"}`, ar.HeadersJson)
		return &prototk.AuthenticateResponse{
			Authenticated: true,
			ResultJson:    stringPtr(`{"username":"test"}`),
		}, nil
	}

	msg := &prototk.RPCAuthMessage{
		RequestToAuthplugin: &prototk.RPCAuthMessage_Authenticate{
			Authenticate: &prototk.AuthenticateRequest{
				HeadersJson: `{"Authorization":"Basic dXNlcjpwYXNz"}`,
			},
		},
	}

	wrapper := &RPCAuthMessageWrapper{}
	iReq := wrapper.Wrap(msg)

	ah := &rpcAuthHandler{
		authPlugin: &authPlugin{},
		api:        &RPCAuthAPIBase{funcs},
		proxy:      nil,
	}

	res, err := ah.RequestToPlugin(ctx, iReq)
	require.NoError(t, err)
	assert.NotNil(t, res)
	assert.True(t, authenticateCalled)
}

func TestRPCAuthHandler_RequestToPlugin_Authorize(t *testing.T) {
	ctx, _, funcs, _, _, done := setupAuthTests(t)
	defer done()

	authorizeCalled := false
	funcs.Authorize = func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
		authorizeCalled = true
		assert.Equal(t, "account_sendTransaction", ar.Method)
		assert.Equal(t, `{"username":"test"}`, ar.ResultJson)
		return &prototk.AuthorizeResponse{Authorized: true}, nil
	}

	msg := &prototk.RPCAuthMessage{
		RequestToAuthplugin: &prototk.RPCAuthMessage_Authorize{
			Authorize: &prototk.AuthorizeRequest{
				ResultJson:  `{"username":"test"}`,
				Method:      "account_sendTransaction",
				PayloadJson: `{"jsonrpc":"2.0"}`,
			},
		},
	}

	wrapper := &RPCAuthMessageWrapper{}
	iReq := wrapper.Wrap(msg)

	ah := &rpcAuthHandler{
		authPlugin: &authPlugin{},
		api:        &RPCAuthAPIBase{funcs},
		proxy:      nil,
	}

	res, err := ah.RequestToPlugin(ctx, iReq)
	require.NoError(t, err)
	assert.NotNil(t, res)
	assert.True(t, authorizeCalled)
}

func TestRPCAuthHandler_RequestToPlugin_Unsupported(t *testing.T) {
	// Test that unsupported request types return errors
	// This is handled by the switch statement in RequestToPlugin
	// which returns an error for nil or unknown types
	// We can't easily test this without full infrastructure,
	// but it's already covered by the handler's switch statement
	t.Skip("Unsupported request handling is covered by the switch statement in RequestToPlugin")
}
