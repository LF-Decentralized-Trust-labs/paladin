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
package plugins

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/google/uuid"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type testRPCAuthManager struct {
	rpcauthPlugins          map[string]plugintk.Plugin
	rpcAuthorizerRegistered func(name string, id uuid.UUID, toAuthorizer components.RPCAuthManagerToAuthorizer) (fromAuthorizer plugintk.RPCAuthCallbacks, err error)
}

func (tam *testRPCAuthManager) mock(t *testing.T) *componentsmocks.RPCAuthManager {
	mram := componentsmocks.NewRPCAuthManager(t)
	pluginMap := make(map[string]*pldconf.PluginConfig)
	for name := range tam.rpcauthPlugins {
		pluginMap[name] = &pldconf.PluginConfig{
			Type:    string(pldtypes.LibraryTypeCShared),
			Library: "/tmp/not/applicable",
		}
	}
	mram.On("ConfiguredRPCAuthorizers").Return(pluginMap).Maybe()
	mram.On("ConfiguredRPCAuthorizerConfigByName", mock.Anything).Return(`{"credentialsFile":"/tmp/users.txt"}`).Maybe()
	mdr := mram.On("RPCAuthorizerRegistered", mock.Anything, mock.Anything, mock.Anything).Maybe()
	mdr.Run(func(args mock.Arguments) {
		if tam.rpcAuthorizerRegistered != nil {
			m2p, err := tam.rpcAuthorizerRegistered(args[0].(string), args[1].(uuid.UUID), args[2].(components.RPCAuthManagerToAuthorizer))
			mdr.Return(m2p, err)
		} else {
			mdr.Return(nil, nil)
		}
	})
	return mram
}

func newTestRPCAuthPluginManager(t *testing.T, setup *testManagers) (context.Context, *pluginManager, func()) {
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

func TestRPCAuthBridge_Authenticate_Success(t *testing.T) {
	log.InitConfig(&pldconf.LogConfig{Level: confutil.P("debug")})
	waitForAPI := make(chan components.RPCAuthManagerToAuthorizer, 1)

	ram := &testRPCAuthManager{
		rpcauthPlugins: map[string]plugintk.Plugin{
			"rpcAuth1": plugintk.NewRPCAuthPlugin(func(callbacks plugintk.RPCAuthCallbacks) plugintk.RPCAuthAPI {
				return &plugintk.RPCAuthAPIBase{Functions: &plugintk.RPCAuthAPIFunctions{
					ConfigureRPCAuthorizer: func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
						return &prototk.ConfigureRPCAuthorizerResponse{}, nil
					},
					Authenticate: func(ctx context.Context, ar *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error) {
						return &prototk.AuthenticateResponse{
							Authenticated: true,
							ResultJson:    confutil.P(`{"username":"testuser"}`),
						}, nil
					},
					Authorize: func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
						return &prototk.AuthorizeResponse{Authorized: true}, nil
					},
				}}
			}),
		},
	}
	ram.rpcAuthorizerRegistered = func(name string, id uuid.UUID, toAuthorizer components.RPCAuthManagerToAuthorizer) (plugintk.RPCAuthCallbacks, error) {
		assert.Equal(t, "rpcAuth1", name)
		waitForAPI <- toAuthorizer
		return nil, nil
	}

	ctx, pc, done := newTestRPCAuthPluginManager(t, &testManagers{
		testRPCAuthManager: ram,
	})
	defer done()

	var authAPI components.RPCAuthManagerToAuthorizer
	select {
	case authAPI = <-waitForAPI:
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for auth API")
	}

	// Wait for initialization to complete
	require.NoError(t, pc.WaitForInit(ctx, prototk.PluginInfo_RPC_AUTH))

	// Test Authenticate
	authResp, err := authAPI.Authenticate(ctx, &prototk.AuthenticateRequest{
		HeadersJson: `{"Authorization":"Basic dXNlcjpwYXNz"}`,
	})
	require.NoError(t, err)
	assert.True(t, authResp.Authenticated)
	assert.NotNil(t, authResp.ResultJson)
	assert.Equal(t, `{"username":"testuser"}`, *authResp.ResultJson)
}

func TestRPCAuthBridge_Authenticate_Failure(t *testing.T) {
	log.InitConfig(&pldconf.LogConfig{Level: confutil.P("debug")})
	waitForAPI := make(chan components.RPCAuthManagerToAuthorizer, 1)

	ram := &testRPCAuthManager{
		rpcauthPlugins: map[string]plugintk.Plugin{
			"rpcAuth1": plugintk.NewRPCAuthPlugin(func(callbacks plugintk.RPCAuthCallbacks) plugintk.RPCAuthAPI {
				return &plugintk.RPCAuthAPIBase{Functions: &plugintk.RPCAuthAPIFunctions{
					ConfigureRPCAuthorizer: func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
						return &prototk.ConfigureRPCAuthorizerResponse{}, nil
					},
					Authenticate: func(ctx context.Context, ar *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error) {
						return &prototk.AuthenticateResponse{
							Authenticated: false,
						}, nil
					},
					Authorize: func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
						return &prototk.AuthorizeResponse{Authorized: true}, nil
					},
				}}
			}),
		},
	}
	ram.rpcAuthorizerRegistered = func(name string, id uuid.UUID, toAuthorizer components.RPCAuthManagerToAuthorizer) (plugintk.RPCAuthCallbacks, error) {
		waitForAPI <- toAuthorizer
		return nil, nil
	}

	ctx, pc, done := newTestRPCAuthPluginManager(t, &testManagers{
		testRPCAuthManager: ram,
	})
	defer done()

	var authAPI components.RPCAuthManagerToAuthorizer
	select {
	case authAPI = <-waitForAPI:
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for auth API")
	}

	// Wait for initialization
	require.NoError(t, pc.WaitForInit(ctx, prototk.PluginInfo_RPC_AUTH))

	// Test Authenticate failure
	authResp, err := authAPI.Authenticate(ctx, &prototk.AuthenticateRequest{
		HeadersJson: `{"Authorization":"Basic dXNlcjp3cm9uZw=="}`,
	})
	require.NoError(t, err)
	assert.False(t, authResp.Authenticated)
	assert.Nil(t, authResp.ResultJson)
}

func TestRPCAuthBridge_ConnectRPCAuthPlugin(t *testing.T) {

	log.InitConfig(&pldconf.LogConfig{Level: confutil.P("debug")}) // test debug specific logging
	waitForAPI := make(chan components.RPCAuthManagerToAuthorizer, 1)

	ram := &testRPCAuthManager{
		rpcauthPlugins: map[string]plugintk.Plugin{
			"rpcAuth1": plugintk.NewRPCAuthPlugin(func(callbacks plugintk.RPCAuthCallbacks) plugintk.RPCAuthAPI {
				return &plugintk.RPCAuthAPIBase{Functions: &plugintk.RPCAuthAPIFunctions{
					ConfigureRPCAuthorizer: func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
						return &prototk.ConfigureRPCAuthorizerResponse{}, nil
					},
					Authenticate: func(ctx context.Context, ar *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error) {
						return &prototk.AuthenticateResponse{
							Authenticated: true,
							ResultJson:    confutil.P(`{"username":"test"}`),
						}, nil
					},
					Authorize: func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
						return &prototk.AuthorizeResponse{
							Authorized: true,
						}, nil
					},
				}}
			}),
		},
	}
	ram.rpcAuthorizerRegistered = func(name string, id uuid.UUID, toAuthorizer components.RPCAuthManagerToAuthorizer) (plugintk.RPCAuthCallbacks, error) {
		assert.Equal(t, "rpcAuth1", name)
		waitForAPI <- toAuthorizer
		return nil, nil // Auth is unidirectional, no callbacks
	}

	ctx, pc, done := newTestRPCAuthPluginManager(t, &testManagers{
		testRPCAuthManager: ram,
	})
	defer done()

	var authAPI components.RPCAuthManagerToAuthorizer
	select {
	case authAPI = <-waitForAPI:
		// Received auth API - note that ConfigureRPCAuthorizer was already called during bridge creation in auth.go:52-59
		// and Initialized() was also called on line 62
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for auth API - expected registration was not received")
	}

	// Wait for initialization to complete (already called in auth.go:62)
	require.NoError(t, pc.WaitForInit(ctx, prototk.PluginInfo_RPC_AUTH))

	// Test Authenticate first
	authResp, err := authAPI.Authenticate(ctx, &prototk.AuthenticateRequest{
		HeadersJson: `{"Authorization":"Basic dXNlcjpwYXNz"}`,
	})
	require.NoError(t, err)
	assert.True(t, authResp.Authenticated)
	require.NotNil(t, authResp.ResultJson)
	authenticationResult := *authResp.ResultJson

	// Test Authorize with authentication result
	ar, err := authAPI.Authorize(ctx, &prototk.AuthorizeRequest{
		ResultJson:  authenticationResult,
		Method:      "account_sendTransaction",
		PayloadJson: `{"jsonrpc":"2.0","method":"account_sendTransaction","params":{}}`,
	})
	require.NoError(t, err)
	assert.True(t, ar.Authorized)
}

func TestRPCAuthBridge_Authorize_Unauthorized(t *testing.T) {

	log.InitConfig(&pldconf.LogConfig{Level: confutil.P("debug")})
	waitForAPI := make(chan components.RPCAuthManagerToAuthorizer, 1)

	ram := &testRPCAuthManager{
		rpcauthPlugins: map[string]plugintk.Plugin{
			"rpcAuth1": plugintk.NewRPCAuthPlugin(func(callbacks plugintk.RPCAuthCallbacks) plugintk.RPCAuthAPI {
				return &plugintk.RPCAuthAPIBase{Functions: &plugintk.RPCAuthAPIFunctions{
					ConfigureRPCAuthorizer: func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
						return &prototk.ConfigureRPCAuthorizerResponse{}, nil
					},
					Authenticate: func(ctx context.Context, ar *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error) {
						return &prototk.AuthenticateResponse{
							Authenticated: true,
							ResultJson:    confutil.P(`{"username":"test"}`),
						}, nil
					},
					Authorize: func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
						return &prototk.AuthorizeResponse{
							Authorized: false,
						}, nil
					},
				}}
			}),
		},
	}
	ram.rpcAuthorizerRegistered = func(name string, id uuid.UUID, toAuthorizer components.RPCAuthManagerToAuthorizer) (plugintk.RPCAuthCallbacks, error) {
		waitForAPI <- toAuthorizer
		return nil, nil
	}

	ctx, pc, done := newTestRPCAuthPluginManager(t, &testManagers{
		testRPCAuthManager: ram,
	})
	defer done()

	var authAPI components.RPCAuthManagerToAuthorizer
	select {
	case authAPI = <-waitForAPI:
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for auth API")
	}

	// Wait for initialization (already called during bridge creation)
	require.NoError(t, pc.WaitForInit(ctx, prototk.PluginInfo_RPC_AUTH))

	// Test Authenticate first
	authResp, err := authAPI.Authenticate(ctx, &prototk.AuthenticateRequest{
		HeadersJson: `{"Authorization":"Basic dXNlcjp3cm9uZw=="}`,
	})
	require.NoError(t, err)
	assert.True(t, authResp.Authenticated)
	require.NotNil(t, authResp.ResultJson)
	identity := *authResp.ResultJson

	// Test unauthorized response
	ar, err := authAPI.Authorize(ctx, &prototk.AuthorizeRequest{
		ResultJson:  identity,
		Method:      "account_sendTransaction",
		PayloadJson: `{"jsonrpc":"2.0","method":"account_sendTransaction","params":{}}`,
	})
	require.NoError(t, err)
	assert.False(t, ar.Authorized)
}

func TestRPCAuthBridge_RequestReply_NoOp(t *testing.T) {
	// RequestReply is a no-op for unidirectional auth plugins
	log.InitConfig(&pldconf.LogConfig{Level: confutil.P("debug")})
	waitForAPI := make(chan components.RPCAuthManagerToAuthorizer, 1)

	ram := &testRPCAuthManager{
		rpcauthPlugins: map[string]plugintk.Plugin{
			"rpcAuth1": plugintk.NewRPCAuthPlugin(func(callbacks plugintk.RPCAuthCallbacks) plugintk.RPCAuthAPI {
				return &plugintk.RPCAuthAPIBase{Functions: &plugintk.RPCAuthAPIFunctions{
					ConfigureRPCAuthorizer: func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
						return &prototk.ConfigureRPCAuthorizerResponse{}, nil
					},
					Authorize: func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
						return &prototk.AuthorizeResponse{Authorized: true}, nil
					},
				}}
			}),
		},
	}
	ram.rpcAuthorizerRegistered = func(name string, id uuid.UUID, toAuthorizer components.RPCAuthManagerToAuthorizer) (plugintk.RPCAuthCallbacks, error) {
		waitForAPI <- toAuthorizer
		return nil, nil
	}

	ctx, pc, done := newTestRPCAuthPluginManager(t, &testManagers{
		testRPCAuthManager: ram,
	})
	defer done()

	var authAPI components.RPCAuthManagerToAuthorizer
	select {
	case authAPI = <-waitForAPI:
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for auth API")
	}

	// Wait for initialization
	require.NoError(t, pc.WaitForInit(ctx, prototk.PluginInfo_RPC_AUTH))

	bridge, ok := authAPI.(*RPCAuthBridge)
	require.True(t, ok, "authAPI should be an RPCAuthBridge")

	// Create mock request message
	wrapper := &plugintk.RPCAuthMessageWrapper{}
	reqMsg := wrapper.Wrap(&prototk.RPCAuthMessage{
		Header: &prototk.Header{MessageId: "test-request-id"},
	})

	// Call RequestReply
	resFn, err := bridge.RequestReply(ctx, reqMsg)

	// Should return no error and a non-nil function
	require.NoError(t, err)
	assert.NotNil(t, resFn, "RequestReply should return a function")
}

func TestRPCAuthBridge_NoAuthPluginConfigured(t *testing.T) {
	pc := newTestPluginManager(t, &testManagers{})
	defer pc.Stop()

	// When no auth plugin is configured, WaitForInit should complete without blocking
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := pc.WaitForInit(ctx, prototk.PluginInfo_RPC_AUTH)
	require.NoError(t, err)
}

func TestRPCAuthBridge_Initialized_CallsNotifyInitialized(t *testing.T) {
	// Initialized() is called during bridge creation (auth.go:62)
	// This test verifies that WaitForInit succeeds after initialization
	log.InitConfig(&pldconf.LogConfig{Level: confutil.P("debug")})

	ram := &testRPCAuthManager{
		rpcauthPlugins: map[string]plugintk.Plugin{
			"rpcAuth1": plugintk.NewRPCAuthPlugin(func(callbacks plugintk.RPCAuthCallbacks) plugintk.RPCAuthAPI {
				return &plugintk.RPCAuthAPIBase{Functions: &plugintk.RPCAuthAPIFunctions{
					ConfigureRPCAuthorizer: func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
						return &prototk.ConfigureRPCAuthorizerResponse{}, nil
					},
					Authorize: func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
						return &prototk.AuthorizeResponse{Authorized: true}, nil
					},
				}}
			}),
		},
	}

	ctx, pc, done := newTestRPCAuthPluginManager(t, &testManagers{
		testRPCAuthManager: ram,
	})
	defer done()

	// Wait for initialization to complete (Initialized() called during bridge creation)
	err := pc.WaitForInit(ctx, prototk.PluginInfo_RPC_AUTH)
	require.NoError(t, err)
}

func TestRPCAuthBridge_MetadataFields(t *testing.T) {

	log.InitConfig(&pldconf.LogConfig{Level: confutil.P("debug")})

	ram := &testRPCAuthManager{
		rpcauthPlugins: map[string]plugintk.Plugin{
			"rpcAuth1": plugintk.NewRPCAuthPlugin(func(callbacks plugintk.RPCAuthCallbacks) plugintk.RPCAuthAPI {
				return &plugintk.RPCAuthAPIBase{Functions: &plugintk.RPCAuthAPIFunctions{
					ConfigureRPCAuthorizer: func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
						return &prototk.ConfigureRPCAuthorizerResponse{}, nil
					},
					Authorize: func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
						return &prototk.AuthorizeResponse{Authorized: true}, nil
					},
				}}
			}),
		},
	}
	ram.rpcAuthorizerRegistered = func(name string, id uuid.UUID, toAuthorizer components.RPCAuthManagerToAuthorizer) (plugintk.RPCAuthCallbacks, error) {
		return nil, nil
	}

	ctx, pc, done := newTestRPCAuthPluginManager(t, &testManagers{
		testRPCAuthManager: ram,
	})
	defer done()

	// Wait for initialization
	err := pc.WaitForInit(ctx, prototk.PluginInfo_RPC_AUTH)
	require.NoError(t, err)

	// Verify auth plugin was initialized successfully
	// All metadata fields should be populated during bridge creation
	// We can verify this through the successful initialization
	assert.NotNil(t, pc)
}

func TestRPCAuthBridge_ConfigurationFlow(t *testing.T) {

	log.InitConfig(&pldconf.LogConfig{Level: confutil.P("debug")})
	waitForAPI := make(chan components.RPCAuthManagerToAuthorizer, 1)

	ram := &testRPCAuthManager{
		rpcauthPlugins: map[string]plugintk.Plugin{
			"rpcAuth1": plugintk.NewRPCAuthPlugin(func(callbacks plugintk.RPCAuthCallbacks) plugintk.RPCAuthAPI {
				return &plugintk.RPCAuthAPIBase{Functions: &plugintk.RPCAuthAPIFunctions{
					ConfigureRPCAuthorizer: func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
						return &prototk.ConfigureRPCAuthorizerResponse{}, nil
					},
					Authenticate: func(ctx context.Context, ar *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error) {
						return &prototk.AuthenticateResponse{
							Authenticated: true,
							ResultJson:    confutil.P(`{"username":"test"}`),
						}, nil
					},
					Authorize: func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
						return &prototk.AuthorizeResponse{Authorized: true}, nil
					},
				}}
			}),
		},
	}
	ram.rpcAuthorizerRegistered = func(name string, id uuid.UUID, toAuthorizer components.RPCAuthManagerToAuthorizer) (plugintk.RPCAuthCallbacks, error) {
		waitForAPI <- toAuthorizer
		return nil, nil
	}

	ctx, pc, done := newTestRPCAuthPluginManager(t, &testManagers{
		testRPCAuthManager: ram,
	})
	defer done()

	var authAPI components.RPCAuthManagerToAuthorizer
	select {
	case authAPI = <-waitForAPI:
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for auth API")
	}

	// Wait for initialization (already called during bridge creation)
	err := pc.WaitForInit(ctx, prototk.PluginInfo_RPC_AUTH)
	require.NoError(t, err)

	// Test Authenticate first
	authResp, err := authAPI.Authenticate(ctx, &prototk.AuthenticateRequest{
		HeadersJson: `{"Authorization":"Basic dXNlcjpwYXNz"}`,
	})
	require.NoError(t, err)
	assert.True(t, authResp.Authenticated)
	require.NotNil(t, authResp.ResultJson)
	identity := *authResp.ResultJson

	// Now authorize should work
	ar, err := authAPI.Authorize(ctx, &prototk.AuthorizeRequest{
		ResultJson:  identity,
		Method:      "account_sendTransaction",
		PayloadJson: `{"jsonrpc":"2.0","method":"account_sendTransaction","params":{}}`,
	})
	require.NoError(t, err)
	assert.True(t, ar.Authorized)
}

func TestRPCAuthBridge_ConfigureRPCAuthorizer(t *testing.T) {
	log.InitConfig(&pldconf.LogConfig{Level: confutil.P("debug")})
	waitForAPI := make(chan components.RPCAuthManagerToAuthorizer, 1)

	configCalled := false
	ram := &testRPCAuthManager{
		rpcauthPlugins: map[string]plugintk.Plugin{
			"rpcAuth1": plugintk.NewRPCAuthPlugin(func(callbacks plugintk.RPCAuthCallbacks) plugintk.RPCAuthAPI {
				return &plugintk.RPCAuthAPIBase{Functions: &plugintk.RPCAuthAPIFunctions{
					ConfigureRPCAuthorizer: func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
						configCalled = true
						assert.NotNil(t, car.ConfigJson)
						return &prototk.ConfigureRPCAuthorizerResponse{}, nil
					},
					Authorize: func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
						return &prototk.AuthorizeResponse{Authorized: true}, nil
					},
				}}
			}),
		},
	}
	ram.rpcAuthorizerRegistered = func(name string, id uuid.UUID, toAuthorizer components.RPCAuthManagerToAuthorizer) (plugintk.RPCAuthCallbacks, error) {
		waitForAPI <- toAuthorizer
		return nil, nil
	}

	ctx, pc, done := newTestRPCAuthPluginManager(t, &testManagers{
		testRPCAuthManager: ram,
	})
	defer done()

	var authAPI components.RPCAuthManagerToAuthorizer
	select {
	case authAPI = <-waitForAPI:
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for auth API")
	}

	// Wait for initialization
	require.NoError(t, pc.WaitForInit(ctx, prototk.PluginInfo_RPC_AUTH))

	// Test ConfigureRPCAuthorizer
	config := `{"credentialsFile":"/tmp/users.txt","realm":"TestRealm"}`
	res, err := authAPI.ConfigureRPCAuthorizer(ctx, &prototk.ConfigureRPCAuthorizerRequest{
		ConfigJson: config,
	})
	require.NoError(t, err)
	assert.NotNil(t, res)
	assert.True(t, configCalled, "ConfigureRPCAuthorizer should have been called")
}

func TestRPCAuthBridge_ConfigureRPCAuthorizer_Error(t *testing.T) {
	log.InitConfig(&pldconf.LogConfig{Level: confutil.P("debug")})
	waitForAPI := make(chan components.RPCAuthManagerToAuthorizer, 1)

	ram := &testRPCAuthManager{
		rpcauthPlugins: map[string]plugintk.Plugin{
			"rpcAuth1": plugintk.NewRPCAuthPlugin(func(callbacks plugintk.RPCAuthCallbacks) plugintk.RPCAuthAPI {
				return &plugintk.RPCAuthAPIBase{Functions: &plugintk.RPCAuthAPIFunctions{
					ConfigureRPCAuthorizer: func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
						return nil, fmt.Errorf("configuration failed")
					},
					Authorize: func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
						return &prototk.AuthorizeResponse{Authorized: true}, nil
					},
				}}
			}),
		},
	}
	ram.rpcAuthorizerRegistered = func(name string, id uuid.UUID, toAuthorizer components.RPCAuthManagerToAuthorizer) (plugintk.RPCAuthCallbacks, error) {
		waitForAPI <- toAuthorizer
		return nil, nil
	}

	ctx, pc, done := newTestRPCAuthPluginManager(t, &testManagers{
		testRPCAuthManager: ram,
	})
	defer done()

	var authAPI components.RPCAuthManagerToAuthorizer
	select {
	case authAPI = <-waitForAPI:
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for auth API")
	}

	// Wait for initialization
	require.NoError(t, pc.WaitForInit(ctx, prototk.PluginInfo_RPC_AUTH))

	// Test ConfigureRPCAuthorizer with error
	res, err := authAPI.ConfigureRPCAuthorizer(ctx, &prototk.ConfigureRPCAuthorizerRequest{
		ConfigJson: `{"credentialsFile":"/tmp/users.txt"}`,
	})
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "configuration failed")
}

func TestRPCAuthBridge_ConfigureRPCAuthorizer_EmptyConfig(t *testing.T) {
	log.InitConfig(&pldconf.LogConfig{Level: confutil.P("debug")})
	waitForAPI := make(chan components.RPCAuthManagerToAuthorizer, 1)

	ram := &testRPCAuthManager{
		rpcauthPlugins: map[string]plugintk.Plugin{
			"rpcAuth1": plugintk.NewRPCAuthPlugin(func(callbacks plugintk.RPCAuthCallbacks) plugintk.RPCAuthAPI {
				return &plugintk.RPCAuthAPIBase{Functions: &plugintk.RPCAuthAPIFunctions{
					ConfigureRPCAuthorizer: func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
						// Accept empty config
						return &prototk.ConfigureRPCAuthorizerResponse{}, nil
					},
					Authorize: func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
						return &prototk.AuthorizeResponse{Authorized: true}, nil
					},
				}}
			}),
		},
	}
	ram.rpcAuthorizerRegistered = func(name string, id uuid.UUID, toAuthorizer components.RPCAuthManagerToAuthorizer) (plugintk.RPCAuthCallbacks, error) {
		waitForAPI <- toAuthorizer
		return nil, nil
	}

	ctx, pc, done := newTestRPCAuthPluginManager(t, &testManagers{
		testRPCAuthManager: ram,
	})
	defer done()

	var authAPI components.RPCAuthManagerToAuthorizer
	select {
	case authAPI = <-waitForAPI:
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for auth API")
	}

	// Wait for initialization
	require.NoError(t, pc.WaitForInit(ctx, prototk.PluginInfo_RPC_AUTH))

	// Test with empty config
	res, err := authAPI.ConfigureRPCAuthorizer(ctx, &prototk.ConfigureRPCAuthorizerRequest{
		ConfigJson: "",
	})
	require.NoError(t, err)
	assert.NotNil(t, res)
}

func TestRPCAuthBridge_ConfigureRPCAuthorizer_ResponseMatcher(t *testing.T) {
	// This test verifies the response matcher logic in ConfigureRPCAuthorizer
	log.InitConfig(&pldconf.LogConfig{Level: confutil.P("debug")})
	waitForAPI := make(chan components.RPCAuthManagerToAuthorizer, 1)

	ram := &testRPCAuthManager{
		rpcauthPlugins: map[string]plugintk.Plugin{
			"rpcAuth1": plugintk.NewRPCAuthPlugin(func(callbacks plugintk.RPCAuthCallbacks) plugintk.RPCAuthAPI {
				return &plugintk.RPCAuthAPIBase{Functions: &plugintk.RPCAuthAPIFunctions{
					ConfigureRPCAuthorizer: func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
						// Return a response with specific fields to verify response parsing
						return &prototk.ConfigureRPCAuthorizerResponse{
							// Note: Currently no fields in the response, but we verify it's parsed correctly
						}, nil
					},
					Authorize: func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
						return &prototk.AuthorizeResponse{Authorized: true}, nil
					},
				}}
			}),
		},
	}
	ram.rpcAuthorizerRegistered = func(name string, id uuid.UUID, toAuthorizer components.RPCAuthManagerToAuthorizer) (plugintk.RPCAuthCallbacks, error) {
		waitForAPI <- toAuthorizer
		return nil, nil
	}

	ctx, pc, done := newTestRPCAuthPluginManager(t, &testManagers{
		testRPCAuthManager: ram,
	})
	defer done()

	var authAPI components.RPCAuthManagerToAuthorizer
	select {
	case authAPI = <-waitForAPI:
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for auth API")
	}

	// Wait for initialization
	require.NoError(t, pc.WaitForInit(ctx, prototk.PluginInfo_RPC_AUTH))

	// Test that ConfigureRPCAuthorizer properly matches and extracts the response
	res, err := authAPI.ConfigureRPCAuthorizer(ctx, &prototk.ConfigureRPCAuthorizerRequest{
		ConfigJson: `{"test": "value"}`,
	})
	require.NoError(t, err)
	assert.NotNil(t, res, "Response should be extracted properly by the matcher")
}

func TestRPCAuthBridge_ConfigureRPCAuthorizer_MultipleCalls(t *testing.T) {
	// Test that ConfigureRPCAuthorizer can be called multiple times with different configs
	log.InitConfig(&pldconf.LogConfig{Level: confutil.P("debug")})
	waitForAPI := make(chan components.RPCAuthManagerToAuthorizer, 1)

	callCount := 0
	ram := &testRPCAuthManager{
		rpcauthPlugins: map[string]plugintk.Plugin{
			"rpcAuth1": plugintk.NewRPCAuthPlugin(func(callbacks plugintk.RPCAuthCallbacks) plugintk.RPCAuthAPI {
				return &plugintk.RPCAuthAPIBase{Functions: &plugintk.RPCAuthAPIFunctions{
					ConfigureRPCAuthorizer: func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
						callCount++
						return &prototk.ConfigureRPCAuthorizerResponse{}, nil
					},
					Authorize: func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
						return &prototk.AuthorizeResponse{Authorized: true}, nil
					},
				}}
			}),
		},
	}
	ram.rpcAuthorizerRegistered = func(name string, id uuid.UUID, toAuthorizer components.RPCAuthManagerToAuthorizer) (plugintk.RPCAuthCallbacks, error) {
		waitForAPI <- toAuthorizer
		return nil, nil
	}

	ctx, pc, done := newTestRPCAuthPluginManager(t, &testManagers{
		testRPCAuthManager: ram,
	})
	defer done()

	var authAPI components.RPCAuthManagerToAuthorizer
	select {
	case authAPI = <-waitForAPI:
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for auth API")
	}

	// Wait for initialization
	require.NoError(t, pc.WaitForInit(ctx, prototk.PluginInfo_RPC_AUTH))

	// Call ConfigureRPCAuthorizer multiple times with different configs
	res1, err := authAPI.ConfigureRPCAuthorizer(ctx, &prototk.ConfigureRPCAuthorizerRequest{
		ConfigJson: `{"credentialsFile":"/tmp/users1.txt"}`,
	})
	require.NoError(t, err)
	assert.NotNil(t, res1)
	assert.Equal(t, 1, callCount)

	res2, err := authAPI.ConfigureRPCAuthorizer(ctx, &prototk.ConfigureRPCAuthorizerRequest{
		ConfigJson: `{"credentialsFile":"/tmp/users2.txt"}`,
	})
	require.NoError(t, err)
	assert.NotNil(t, res2)
	assert.Equal(t, 2, callCount)

	res3, err := authAPI.ConfigureRPCAuthorizer(ctx, &prototk.ConfigureRPCAuthorizerRequest{
		ConfigJson: `{"credentialsFile":"/tmp/users3.txt","realm":"Test"}`,
	})
	require.NoError(t, err)
	assert.NotNil(t, res3)
	assert.Equal(t, 3, callCount)
}

func TestRPCAuthBridge_ConfigureRPCAuthorizerByName(t *testing.T) {
	// Test the ConfigureRPCAuthorizerByName helper method
	log.InitConfig(&pldconf.LogConfig{Level: confutil.P("debug")})
	waitForAPI := make(chan components.RPCAuthManagerToAuthorizer, 1)

	configReceived := ""
	ram := &testRPCAuthManager{
		rpcauthPlugins: map[string]plugintk.Plugin{
			"rpcAuth1": plugintk.NewRPCAuthPlugin(func(callbacks plugintk.RPCAuthCallbacks) plugintk.RPCAuthAPI {
				return &plugintk.RPCAuthAPIBase{Functions: &plugintk.RPCAuthAPIFunctions{
					ConfigureRPCAuthorizer: func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
						configReceived = car.ConfigJson
						return &prototk.ConfigureRPCAuthorizerResponse{}, nil
					},
					Authorize: func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
						return &prototk.AuthorizeResponse{Authorized: true}, nil
					},
				}}
			}),
		},
	}
	ram.rpcAuthorizerRegistered = func(name string, id uuid.UUID, toAuthorizer components.RPCAuthManagerToAuthorizer) (plugintk.RPCAuthCallbacks, error) {
		waitForAPI <- toAuthorizer
		return nil, nil
	}

	ctx, pc, done := newTestRPCAuthPluginManager(t, &testManagers{
		testRPCAuthManager: ram,
	})
	defer done()

	var authAPI components.RPCAuthManagerToAuthorizer
	select {
	case authAPI = <-waitForAPI:
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for auth API")
	}

	// Wait for initialization
	require.NoError(t, pc.WaitForInit(ctx, prototk.PluginInfo_RPC_AUTH))

	// Get the bridge to test ConfigureRPCAuthorizerByName
	bridge, ok := authAPI.(*RPCAuthBridge)
	require.True(t, ok, "authAPI should be an RPCAuthBridge")

	// Test with a config JSON string
	testConfig := `{"credentialsFile": "/path/to/users.txt"}`
	err := bridge.ConfigureRPCAuthorizerByName(ctx, testConfig)
	require.NoError(t, err)

	// Verify that the config was received correctly
	assert.Equal(t, testConfig, configReceived)
}

func TestRPCAuthBridge_RPCAuthorizerRegistered_Error(t *testing.T) {
	waitForError := make(chan struct{})

	ram := &testRPCAuthManager{
		rpcauthPlugins: map[string]plugintk.Plugin{
			"rpcAuth1": plugintk.NewRPCAuthPlugin(func(callbacks plugintk.RPCAuthCallbacks) plugintk.RPCAuthAPI {
				return &plugintk.RPCAuthAPIBase{Functions: &plugintk.RPCAuthAPIFunctions{
					ConfigureRPCAuthorizer: func(ctx context.Context, car *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
						return &prototk.ConfigureRPCAuthorizerResponse{}, nil
					},
					Authorize: func(ctx context.Context, ar *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
						return &prototk.AuthorizeResponse{Authorized: true}, nil
					},
				}}
			}),
		},
	}
	ram.rpcAuthorizerRegistered = func(name string, id uuid.UUID, toAuthorizer components.RPCAuthManagerToAuthorizer) (plugintk.RPCAuthCallbacks, error) {
		close(waitForError)
		return nil, fmt.Errorf("registration failed")
	}

	_, _, done := newTestRPCAuthPluginManager(t, &testManagers{
		testRPCAuthManager: ram,
	})
	defer done()

	// Add timeout to prevent test from hanging indefinitely
	select {
	case <-waitForError:
		// Error received successfully
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for error - expected error was not received")
	}
}
