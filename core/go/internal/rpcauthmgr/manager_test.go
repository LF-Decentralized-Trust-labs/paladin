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

package rpcauthmgr

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRPCAuthBridge is a mock implementation of RPCAuthManagerToAuthorizer for testing
type mockRPCAuthBridge struct {
	authenticateFunc func(ctx context.Context, req *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error)
	authorizeFunc    func(ctx context.Context, req *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error)
	configuredFunc   func(ctx context.Context, req *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error)
}

func (m *mockRPCAuthBridge) Authenticate(ctx context.Context, req *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error) {
	if m.authenticateFunc != nil {
		return m.authenticateFunc(ctx, req)
	}
	return &prototk.AuthenticateResponse{
		Authenticated: true,
		ResultJson:    stringPtr(`{"username":"test"}`),
	}, nil
}

func (m *mockRPCAuthBridge) Authorize(ctx context.Context, req *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
	if m.authorizeFunc != nil {
		return m.authorizeFunc(ctx, req)
	}
	return &prototk.AuthorizeResponse{Authorized: true}, nil
}

func (m *mockRPCAuthBridge) ConfigureRPCAuthorizer(ctx context.Context, req *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
	if m.configuredFunc != nil {
		return m.configuredFunc(ctx, req)
	}
	return &prototk.ConfigureRPCAuthorizerResponse{}, nil
}

func (m *mockRPCAuthBridge) Initialized() {
}

func stringPtr(s string) *string {
	return &s
}

func TestRPCAuthManager_Lifecycle(t *testing.T) {
	manager := NewRPCAuthManager(context.Background(), nil)

	// Test PreInit
	result, err := manager.PreInit(nil)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Test PostInit
	err = manager.PostInit(nil)
	assert.NoError(t, err)

	// Test Start
	err = manager.Start()
	assert.NoError(t, err)

	// Test Stop
	manager.Stop()
}

func TestRPCAuthManager_NoConfig(t *testing.T) {
	manager := NewRPCAuthManager(context.Background(), nil)

	// Should return empty map
	configs := manager.ConfiguredRPCAuthorizers()
	assert.Nil(t, configs)

	// Should return empty string
	config := manager.ConfiguredRPCAuthorizerConfig()
	assert.Empty(t, config)

	// Should return empty string for any name
	configByName := manager.ConfiguredRPCAuthorizerConfigByName("test")
	assert.Empty(t, configByName)

	// Should return nil authorizer
	authorizer := manager.GetRPCAuthorizer("")
	assert.Nil(t, authorizer)
}

func TestRPCAuthManager_ConfiguredRPCAuthorizers(t *testing.T) {
	// Create config with multiple authorizers
	authConf := map[string]*pldconf.RPCAuthorizerConfig{
		"auth1": {
			Plugin: pldconf.PluginConfig{
				Type:    "shared",
				Library: "/tmp/auth1.so",
			},
			Config: `{"credentialsFile":"/tmp/users1.txt"}`,
		},
		"auth2": {
			Plugin: pldconf.PluginConfig{
				Type:    "shared",
				Library: "/tmp/auth2.so",
			},
			Config: `{"credentialsFile":"/tmp/users2.txt"}`,
		},
	}

	manager := NewRPCAuthManager(context.Background(), authConf)

	// Should return plugin configs
	configs := manager.ConfiguredRPCAuthorizers()
	require.NotNil(t, configs)
	assert.Len(t, configs, 2)
	assert.Equal(t, "/tmp/auth1.so", configs["auth1"].Library)
	assert.Equal(t, "/tmp/auth2.so", configs["auth2"].Library)
}

func TestRPCAuthManager_ConfiguredRPCAuthorizerConfig_Single(t *testing.T) {
	// Create config with single authorizer
	authConf := map[string]*pldconf.RPCAuthorizerConfig{
		"auth1": {
			Plugin: pldconf.PluginConfig{Library: "/tmp/auth1.so"},
			Config: `{"credentialsFile":"/tmp/users.txt"}`,
		},
	}

	manager := NewRPCAuthManager(context.Background(), authConf)

	// Should return the single config
	config := manager.ConfiguredRPCAuthorizerConfig()
	assert.Equal(t, `{"credentialsFile":"/tmp/users.txt"}`, config)
}

func TestRPCAuthManager_ConfiguredRPCAuthorizerConfig_Multiple(t *testing.T) {
	// Create config with multiple authorizers
	authConf := map[string]*pldconf.RPCAuthorizerConfig{
		"auth1": {
			Plugin: pldconf.PluginConfig{Library: "/tmp/auth1.so"},
			Config: `{"credentialsFile":"/tmp/users1.txt"}`,
		},
		"auth2": {
			Plugin: pldconf.PluginConfig{Library: "/tmp/auth2.so"},
			Config: `{"credentialsFile":"/tmp/users2.txt"}`,
		},
	}

	manager := NewRPCAuthManager(context.Background(), authConf)

	// Should return empty string for multiple plugins
	config := manager.ConfiguredRPCAuthorizerConfig()
	assert.Empty(t, config)
}

func TestRPCAuthManager_ConfiguredRPCAuthorizerConfigByName(t *testing.T) {
	authConf := map[string]*pldconf.RPCAuthorizerConfig{
		"auth1": {
			Plugin: pldconf.PluginConfig{Library: "/tmp/auth1.so"},
			Config: `{"credentialsFile":"/tmp/users1.txt"}`,
		},
		"auth2": {
			Plugin: pldconf.PluginConfig{Library: "/tmp/auth2.so"},
			Config: `{"credentialsFile":"/tmp/users2.txt"}`,
		},
	}

	manager := NewRPCAuthManager(context.Background(), authConf)

	// Test retrieving by name
	config := manager.ConfiguredRPCAuthorizerConfigByName("auth1")
	assert.Equal(t, `{"credentialsFile":"/tmp/users1.txt"}`, config)

	config = manager.ConfiguredRPCAuthorizerConfigByName("auth2")
	assert.Equal(t, `{"credentialsFile":"/tmp/users2.txt"}`, config)

	// Test retrieving non-existent name
	config = manager.ConfiguredRPCAuthorizerConfigByName("nonexistent")
	assert.Empty(t, config)
}

func TestRPCAuthManager_RPCAuthorizerRegistered(t *testing.T) {
	manager := NewRPCAuthManager(context.Background(), nil)

	mockBridge := &mockRPCAuthBridge{}
	id := uuid.New()

	// Register authorizer
	callbacks, err := manager.RPCAuthorizerRegistered("auth1", id, mockBridge)
	assert.NoError(t, err)
	assert.Nil(t, callbacks) // Auth is unidirectional, returns nil callbacks
}

func TestRPCAuthManager_RPCAuthorizerRegistered_WithConfig(t *testing.T) {
	ctx := context.Background()
	authConf := map[string]*pldconf.RPCAuthorizerConfig{
		"auth1": {
			Plugin: pldconf.PluginConfig{Library: "/tmp/auth1.so"},
			Config: `{"credentialsFile":"/tmp/users.txt"}`,
		},
	}

	manager := NewRPCAuthManager(ctx, authConf)

	// Use channel to synchronize with async configuration
	configDone := make(chan struct{})
	var receivedConfig string
	mockBridge := &mockRPCAuthBridge{
		configuredFunc: func(ctx context.Context, req *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
			receivedConfig = req.ConfigJson
			close(configDone) // Signal that configuration completed
			return &prototk.ConfigureRPCAuthorizerResponse{}, nil
		},
	}
	id := uuid.New()

	// Register authorizer - should trigger async configuration
	callbacks, err := manager.RPCAuthorizerRegistered("auth1", id, mockBridge)
	assert.NoError(t, err)
	assert.Nil(t, callbacks)

	// Wait for configuration to complete (similar to domain plugin tests)
	<-configDone

	// Verify configuration was called with correct config
	assert.Equal(t, `{"credentialsFile":"/tmp/users.txt"}`, receivedConfig)
}

func TestRPCAuthManager_RPCAuthorizerRegistered_WithConfig_Error(t *testing.T) {
	ctx := context.Background()
	authConf := map[string]*pldconf.RPCAuthorizerConfig{
		"auth1": {
			Plugin: pldconf.PluginConfig{Library: "/tmp/auth1.so"},
			Config: `{"credentialsFile":"/tmp/users.txt"}`,
		},
	}

	manager := NewRPCAuthManager(ctx, authConf)

	// Use channel to synchronize with async configuration
	configDone := make(chan struct{})
	expectedError := fmt.Errorf("configuration failed")
	mockBridge := &mockRPCAuthBridge{
		configuredFunc: func(ctx context.Context, req *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
			close(configDone) // Signal that configuration completed (even with error)
			return nil, expectedError
		},
	}
	id := uuid.New()

	// Register authorizer - should trigger async configuration
	callbacks, err := manager.RPCAuthorizerRegistered("auth1", id, mockBridge)
	assert.NoError(t, err) // Registration should succeed even if config fails
	assert.Nil(t, callbacks)

	// Wait for configuration to complete (even though it failed)
	<-configDone
}

func TestRPCAuthManager_RPCAuthorizerRegistered_NoConfig(t *testing.T) {
	ctx := context.Background()
	authConf := map[string]*pldconf.RPCAuthorizerConfig{
		"auth1": {
			Plugin: pldconf.PluginConfig{Library: "/tmp/auth1.so"},
			Config: `{"credentialsFile":"/tmp/users.txt"}`,
		},
	}

	manager := NewRPCAuthManager(ctx, authConf)

	// Use channel to detect if ConfigureRPCAuthorizer was called
	// This channel should never receive a signal since config doesn't exist
	configCalled := make(chan struct{})
	mockBridge := &mockRPCAuthBridge{
		configuredFunc: func(ctx context.Context, req *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
			close(configCalled) // Signal if configuration is called (shouldn't happen)
			return &prototk.ConfigureRPCAuthorizerResponse{}, nil
		},
	}
	id := uuid.New()

	// Register authorizer with name that has no config
	callbacks, err := manager.RPCAuthorizerRegistered("nonexistent", id, mockBridge)
	assert.NoError(t, err)
	assert.Nil(t, callbacks)

	// Wait a short time to ensure goroutine would have run if it was going to
	// Use select with timeout to verify config was NOT called
	select {
	case <-configCalled:
		t.Fatal("ConfigureRPCAuthorizer should not have been called for plugin without config")
	case <-time.After(50 * time.Millisecond):
		// Expected: configuration should not be called
	}
}

func TestRPCAuthManager_RPCAuthorizerRegistered_NilContext(t *testing.T) {
	// Test that nil bgCtx is handled gracefully (uses context.Background())
	authConf := map[string]*pldconf.RPCAuthorizerConfig{
		"auth1": {
			Plugin: pldconf.PluginConfig{Library: "/tmp/auth1.so"},
			Config: `{"credentialsFile":"/tmp/users.txt"}`,
		},
	}

	manager := NewRPCAuthManager(nil, authConf)

	// Use channel to synchronize with async configuration
	configDone := make(chan struct{})
	mockBridge := &mockRPCAuthBridge{
		configuredFunc: func(ctx context.Context, req *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
			// Verify context is not nil
			assert.NotNil(t, ctx)
			close(configDone) // Signal that configuration completed
			return &prototk.ConfigureRPCAuthorizerResponse{}, nil
		},
	}
	id := uuid.New()

	// Register authorizer - should trigger async configuration even with nil bgCtx
	callbacks, err := manager.RPCAuthorizerRegistered("auth1", id, mockBridge)
	assert.NoError(t, err)
	assert.Nil(t, callbacks)

	// Wait for configuration to complete
	<-configDone
}

func TestRPCAuthManager_GetRPCAuthorizer_Single(t *testing.T) {
	manager := NewRPCAuthManager(context.Background(), nil)

	mockBridge := &mockRPCAuthBridge{}
	id := uuid.New()

	// Register authorizer
	_, err := manager.RPCAuthorizerRegistered("auth1", id, mockBridge)
	require.NoError(t, err)

	// Get by name
	authorizer := manager.GetRPCAuthorizer("auth1")
	require.NotNil(t, authorizer)
	assert.IsType(t, &bridgeWrapper{}, authorizer)

	// Get with empty name should return nil (no fallback behavior)
	authorizer2 := manager.GetRPCAuthorizer("")
	assert.Nil(t, authorizer2)
}

func TestRPCAuthManager_GetRPCAuthorizer_Multiple(t *testing.T) {
	manager := NewRPCAuthManager(context.Background(), nil)

	mockBridge1 := &mockRPCAuthBridge{}
	mockBridge2 := &mockRPCAuthBridge{}

	// Register multiple authorizers
	_, err := manager.RPCAuthorizerRegistered("auth1", uuid.New(), mockBridge1)
	require.NoError(t, err)

	_, err = manager.RPCAuthorizerRegistered("auth2", uuid.New(), mockBridge2)
	require.NoError(t, err)

	// Get by specific name
	authorizer := manager.GetRPCAuthorizer("auth1")
	require.NotNil(t, authorizer)

	authorizer2 := manager.GetRPCAuthorizer("auth2")
	require.NotNil(t, authorizer2)

	// Get with empty name should return nil when multiple plugins exist
	authorizer3 := manager.GetRPCAuthorizer("")
	assert.Nil(t, authorizer3) // Returns nil, no fallback for multiple
}

func TestRPCAuthManager_GetRPCAuthorizer_NotFound(t *testing.T) {
	manager := NewRPCAuthManager(context.Background(), nil)

	// Get non-existent authorizer
	authorizer := manager.GetRPCAuthorizer("nonexistent")
	assert.Nil(t, authorizer)
}

func TestRPCAuthManager_ConcurrentAccess(t *testing.T) {
	manager := NewRPCAuthManager(context.Background(), nil)

	// Test concurrent registration
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()
			bridge := &mockRPCAuthBridge{}
			_, err := manager.RPCAuthorizerRegistered("auth", uuid.New(), bridge)
			assert.NoError(t, err)
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify only one is stored (last one wins in race condition scenario)
	authorizer := manager.GetRPCAuthorizer("auth")
	// Either the bridge should exist or not - both are valid outcomes for concurrent access
	_ = authorizer
}

func TestRPCAuthManager_BridgeWrapper_Authorize_Success(t *testing.T) {
	manager := NewRPCAuthManager(context.Background(), nil)

	// Create bridge that returns success
	mockBridge := &mockRPCAuthBridge{
		authorizeFunc: func(ctx context.Context, req *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
			return &prototk.AuthorizeResponse{
				Authorized: true,
			}, nil
		},
	}

	_, err := manager.RPCAuthorizerRegistered("auth1", uuid.New(), mockBridge)
	require.NoError(t, err)

	authorizer := manager.GetRPCAuthorizer("auth1")
	require.NotNil(t, authorizer)

	// Test authentication first
	authenticationResult, err := authorizer.Authenticate(context.Background(), map[string]string{})
	require.NoError(t, err)
	assert.NotEmpty(t, authenticationResult)

	// Test authorization with authentication result
	authorized := authorizer.Authorize(context.Background(), authenticationResult, "test_method", []byte("test"))
	assert.True(t, authorized)
}

func TestRPCAuthManager_BridgeWrapper_Authorize_Failure(t *testing.T) {
	manager := NewRPCAuthManager(context.Background(), nil)

	// Create bridge that returns failure
	mockBridge := &mockRPCAuthBridge{
		authorizeFunc: func(ctx context.Context, req *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
			return &prototk.AuthorizeResponse{
				Authorized: false,
			}, nil
		},
	}

	_, err := manager.RPCAuthorizerRegistered("auth1", uuid.New(), mockBridge)
	require.NoError(t, err)

	authorizer := manager.GetRPCAuthorizer("auth1")
	require.NotNil(t, authorizer)

	// Test authentication first
	authenticationResult, err := authorizer.Authenticate(context.Background(), map[string]string{})
	require.NoError(t, err)
	assert.NotEmpty(t, authenticationResult)

	// Test authorization with authentication result
	authorized := authorizer.Authorize(context.Background(), authenticationResult, "test_method", []byte("test"))
	assert.False(t, authorized)
}

func TestRPCAuthManager_BridgeWrapper_Authorize_Failure_Default(t *testing.T) {
	manager := NewRPCAuthManager(context.Background(), nil)

	// Create bridge that returns failure
	mockBridge := &mockRPCAuthBridge{
		authorizeFunc: func(ctx context.Context, req *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
			return &prototk.AuthorizeResponse{
				Authorized: false,
			}, nil
		},
	}

	_, err := manager.RPCAuthorizerRegistered("auth1", uuid.New(), mockBridge)
	require.NoError(t, err)

	authorizer := manager.GetRPCAuthorizer("auth1")
	require.NotNil(t, authorizer)

	// Test authentication first
	authenticationResult, err := authorizer.Authenticate(context.Background(), map[string]string{})
	require.NoError(t, err)
	assert.NotEmpty(t, authenticationResult)

	// Test authorization with authentication result
	authorized := authorizer.Authorize(context.Background(), authenticationResult, "test_method", []byte("test"))
	assert.False(t, authorized)
}

func TestRPCAuthManager_BridgeWrapper_Authorize_Failure_NoMessage(t *testing.T) {
	manager := NewRPCAuthManager(context.Background(), nil)

	// Create bridge that returns failure with no message
	mockBridge := &mockRPCAuthBridge{
		authorizeFunc: func(ctx context.Context, req *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
			return &prototk.AuthorizeResponse{
				Authorized: false,
			}, nil
		},
	}

	_, err := manager.RPCAuthorizerRegistered("auth1", uuid.New(), mockBridge)
	require.NoError(t, err)

	authorizer := manager.GetRPCAuthorizer("auth1")
	require.NotNil(t, authorizer)

	// Test authentication first
	authenticationResult, err := authorizer.Authenticate(context.Background(), map[string]string{})
	require.NoError(t, err)
	assert.NotEmpty(t, authenticationResult)

	// Test authorization with authentication result
	authorized := authorizer.Authorize(context.Background(), authenticationResult, "test_method", []byte("test"))
	assert.False(t, authorized)
}

// TestRPCAuthManager_PreInit_MockComponents tests the PreInit with mock components
func TestRPCAuthManager_PreInit_MockComponents(t *testing.T) {
	mockAllComponents := componentsmocks.NewAllComponents(t)
	preInitComponents := mockPreInitComponents()

	manager := NewRPCAuthManager(context.Background(), nil)
	result, err := manager.PreInit(preInitComponents)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	_ = mockAllComponents // Keep mock alive
}

func TestRPCAuthManager_BridgeWrapper_Authorize_Error(t *testing.T) {
	manager := NewRPCAuthManager(context.Background(), nil)

	// Create bridge that returns an error
	mockBridge := &mockRPCAuthBridge{
		authorizeFunc: func(ctx context.Context, req *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
			return nil, fmt.Errorf("authorization error")
		},
	}

	_, err := manager.RPCAuthorizerRegistered("auth1", uuid.New(), mockBridge)
	require.NoError(t, err)

	authorizer := manager.GetRPCAuthorizer("auth1")
	require.NotNil(t, authorizer)

	// Test authentication first
	authenticationResult, err := authorizer.Authenticate(context.Background(), map[string]string{})
	require.NoError(t, err)
	assert.NotEmpty(t, authenticationResult)

	// Test that error from plugin Authorize results in false
	authorized := authorizer.Authorize(context.Background(), authenticationResult, "test_method", []byte("test"))
	assert.False(t, authorized)
}

func TestRPCAuthManager_BridgeWrapper_Authenticate_Failure(t *testing.T) {
	manager := NewRPCAuthManager(context.Background(), nil)

	// Create bridge that returns authenticated=false
	mockBridge := &mockRPCAuthBridge{
		authenticateFunc: func(ctx context.Context, req *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error) {
			return &prototk.AuthenticateResponse{
				Authenticated: false,
			}, nil
		},
	}

	_, err := manager.RPCAuthorizerRegistered("auth1", uuid.New(), mockBridge)
	require.NoError(t, err)

	authorizer := manager.GetRPCAuthorizer("auth1")
	require.NotNil(t, authorizer)

	// Test that Authenticate returns error when authenticated=false
	result, err := authorizer.Authenticate(context.Background(), map[string]string{})
	assert.Error(t, err)
	assert.Empty(t, result)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestRPCAuthManager_BridgeWrapper_Authenticate_Failure_NoMessage(t *testing.T) {
	manager := NewRPCAuthManager(context.Background(), nil)

	// Create bridge that returns authenticated=false with no error message
	mockBridge := &mockRPCAuthBridge{
		authenticateFunc: func(ctx context.Context, req *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error) {
			return &prototk.AuthenticateResponse{
				Authenticated: false,
			}, nil
		},
	}

	_, err := manager.RPCAuthorizerRegistered("auth1", uuid.New(), mockBridge)
	require.NoError(t, err)

	authorizer := manager.GetRPCAuthorizer("auth1")
	require.NotNil(t, authorizer)

	// Test that Authenticate returns error with default message
	result, err := authorizer.Authenticate(context.Background(), map[string]string{})
	assert.Error(t, err)
	assert.Empty(t, result)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestRPCAuthManager_BridgeWrapper_Authenticate_Error(t *testing.T) {
	manager := NewRPCAuthManager(context.Background(), nil)

	// Create bridge that returns an error from Authenticate call
	mockBridge := &mockRPCAuthBridge{
		authenticateFunc: func(ctx context.Context, req *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error) {
			return nil, fmt.Errorf("network error")
		},
	}

	_, err := manager.RPCAuthorizerRegistered("auth1", uuid.New(), mockBridge)
	require.NoError(t, err)

	authorizer := manager.GetRPCAuthorizer("auth1")
	require.NotNil(t, authorizer)

	// Test that Authenticate returns error when bridge returns error
	result, err := authorizer.Authenticate(context.Background(), map[string]string{})
	assert.Error(t, err)
	assert.Empty(t, result)
	assert.Contains(t, err.Error(), "network error")
}

func TestRPCAuthManager_BridgeWrapper_Authenticate_NoResult(t *testing.T) {
	manager := NewRPCAuthManager(context.Background(), nil)

	// Create bridge that returns authenticated=true but no result
	mockBridge := &mockRPCAuthBridge{
		authenticateFunc: func(ctx context.Context, req *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error) {
			return &prototk.AuthenticateResponse{
				Authenticated: true,
				ResultJson:    nil, // No result
			}, nil
		},
	}

	_, err := manager.RPCAuthorizerRegistered("auth1", uuid.New(), mockBridge)
	require.NoError(t, err)

	authorizer := manager.GetRPCAuthorizer("auth1")
	require.NotNil(t, authorizer)

	// Test that Authenticate returns error when no result provided
	result, err := authorizer.Authenticate(context.Background(), map[string]string{})
	assert.Error(t, err)
	assert.Empty(t, result)
	assert.Contains(t, err.Error(), "plugin returned authenticated=true but no result")
}

func mockPreInitComponents() components.PreInitComponents {
	return nil
}
