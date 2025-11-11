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
	"encoding/json"
	"errors"
	"sync"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/rpcserver"
	"github.com/google/uuid"
)

// rpcAuthManager implements components.RPCAuthManager
type rpcAuthManager struct {
	bgCtx       context.Context
	conf        map[string]*pldconf.RPCAuthorizerConfig
	authBridges map[string]components.RPCAuthManagerToAuthorizer
	mux         sync.Mutex
}

// NewRPCAuthManager creates a new RPC auth manager
func NewRPCAuthManager(bgCtx context.Context, authPlugins map[string]*pldconf.RPCAuthorizerConfig) components.RPCAuthManager {
	if len(authPlugins) == 0 {
		return &rpcAuthManager{bgCtx: bgCtx, conf: nil, authBridges: make(map[string]components.RPCAuthManagerToAuthorizer)}
	}
	return &rpcAuthManager{bgCtx: bgCtx, conf: authPlugins, authBridges: make(map[string]components.RPCAuthManagerToAuthorizer)}
}

// RPCAuthorizerRegistered registers an RPC authorizer bridge
func (am *rpcAuthManager) RPCAuthorizerRegistered(name string, id uuid.UUID, toAuthorizer components.RPCAuthManagerToAuthorizer) (fromAuthorizer plugintk.RPCAuthCallbacks, err error) {
	am.mux.Lock()

	// Store the bridge
	am.authBridges[name] = toAuthorizer

	// Get the configuration for this plugin
	var configJson string
	if am.conf != nil {
		if authConf, ok := am.conf[name]; ok {
			configJson = authConf.Config
		}
	}

	am.mux.Unlock()

	// Configure the plugin asynchronously (similar to how domain plugins do it)
	// This allows the registration to complete quickly while configuration happens in background
	if configJson != "" {
		go func() {
			ctx := am.bgCtx
			if ctx == nil {
				ctx = context.Background()
			}
			ctx = log.WithComponent(ctx, "rpcauthmgr")
			// Configure the plugin using the interface method
			req := &prototk.ConfigureRPCAuthorizerRequest{
				ConfigJson: configJson,
			}
			_, err := toAuthorizer.ConfigureRPCAuthorizer(ctx, req)
			if err != nil {
				// Log error but don't fail registration - plugin will return "not configured" on auth attempts
				// This matches the behavior where domain init failures are handled gracefully
				log.L(ctx).Errorf("Failed to configure RPC auth plugin %s: %v", name, err)
			} else {
				log.L(ctx).Infof("RPC auth plugin %s configured successfully", name)
			}
		}()
	}

	// Auth plugins don't have callbacks (unidirectional), return nil
	return nil, nil
}

// GetRPCAuthorizer returns the RPC authorizer bridge by name
func (am *rpcAuthManager) GetRPCAuthorizer(name string) rpcserver.Authorizer {
	am.mux.Lock()
	defer am.mux.Unlock()
	toAuthorizer := am.authBridges[name]
	if toAuthorizer == nil {
		return nil
	}
	// Convert RPCAuthManagerToAuthorizer to rpcserver.Authorizer
	return &bridgeWrapper{toAuthorizer: toAuthorizer}
}

// bridgeWrapper wraps RPCAuthManagerToAuthorizer to implement rpcserver.Authorizer
type bridgeWrapper struct {
	toAuthorizer components.RPCAuthManagerToAuthorizer
}

func (bw *bridgeWrapper) Authenticate(ctx context.Context, headers map[string]string) (string, error) {
	req := &prototk.AuthenticateRequest{
		HeadersJson: toJSON(headers),
	}
	res, err := bw.toAuthorizer.Authenticate(ctx, req)
	if err != nil {
		return "", err
	}
	if !res.Authenticated {
		return "", errors.New("authentication failed")
	}
	if res.ResultJson == nil {
		return "", errors.New("plugin returned authenticated=true but no result")
	}
	// Return the opaque authentication result string directly (format is plugin-specific, RPC server doesn't inspect it)
	return *res.ResultJson, nil
}

func (bw *bridgeWrapper) Authorize(ctx context.Context, result string, method string, payload []byte) bool {
	// result is an opaque authentication result string passed directly to plugin (format is plugin-specific)
	req := &prototk.AuthorizeRequest{
		ResultJson:  result, // Opaque string (plugin determines format)
		Method:      method,
		PayloadJson: string(payload),
	}
	res, err := bw.toAuthorizer.Authorize(ctx, req)
	if err != nil {
		log.L(ctx).Errorf("Authorization error from plugin: %s", err)
		return false
	}
	return res.Authorized
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// PreInit implements ManagerLifecycle
func (am *rpcAuthManager) PreInit(pic components.PreInitComponents) (*components.ManagerInitResult, error) {
	return &components.ManagerInitResult{}, nil
}

// PostInit implements ManagerLifecycle
func (am *rpcAuthManager) PostInit(c components.AllComponents) error {
	return nil
}

// Start implements ManagerLifecycle
func (am *rpcAuthManager) Start() error {
	return nil
}

// Stop implements ManagerLifecycle
func (am *rpcAuthManager) Stop() {
}

// ConfiguredRPCAuthorizers returns the configured RPC authorizers
func (am *rpcAuthManager) ConfiguredRPCAuthorizers() map[string]*pldconf.PluginConfig {
	if am.conf == nil {
		return nil
	}
	pluginConfig := make(map[string]*pldconf.PluginConfig)
	for name, authConf := range am.conf {
		pluginConfig[name] = &authConf.Plugin
	}
	return pluginConfig
}

// ConfiguredRPCAuthorizerConfig returns the RPC authorizer's configuration JSON string
func (am *rpcAuthManager) ConfiguredRPCAuthorizerConfig() string {
	if am.conf == nil {
		return ""
	}
	// If there's only one plugin, return its config
	if len(am.conf) == 1 {
		for _, authConf := range am.conf {
			return authConf.Config
		}
	}
	// For multiple plugins, return empty (caller should specify which one)
	return ""
}

// ConfiguredRPCAuthorizerConfigByName returns the RPC authorizer's configuration JSON string for a specific plugin name
func (am *rpcAuthManager) ConfiguredRPCAuthorizerConfigByName(name string) string {
	if am.conf == nil {
		return ""
	}
	if authConf, ok := am.conf[name]; ok {
		return authConf.Config
	}
	return ""
}
