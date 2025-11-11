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
package basicauth

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// BasicAuthHandler implements the RPCAuthAPI interface
type BasicAuthHandler struct {
	store *CredentialsStore
}

var _ plugintk.RPCAuthAPI = (*BasicAuthHandler)(nil)

// ConfigureRPCAuthorizer loads the configuration
func (h *BasicAuthHandler) ConfigureRPCAuthorizer(ctx context.Context, req *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
	if req.ConfigJson == "" {
		return nil, fmt.Errorf("config is required")
	}

	config, err := parseConfig(req.ConfigJson)
	if err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Load credentials from file
	store, err := LoadCredentials(config.CredentialsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load credentials: %w", err)
	}

	h.store = store

	return &prototk.ConfigureRPCAuthorizerResponse{}, nil
}

// Authenticate validates credentials and returns authentication result information
func (h *BasicAuthHandler) Authenticate(ctx context.Context, req *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error) {
	if h.store == nil {
		log.L(ctx).Warnf("Authentication failed: plugin not configured")
		return &prototk.AuthenticateResponse{
			Authenticated: false,
		}, nil
	}

	// Parse headers JSON
	var headers map[string]string
	if err := json.Unmarshal([]byte(req.HeadersJson), &headers); err != nil {
		log.L(ctx).Warnf("Authentication failed: invalid headers JSON: %v", err)
		return &prototk.AuthenticateResponse{
			Authenticated: false,
		}, nil
	}

	// Check authentication
	username := CheckHeaderAuthentication(headers, h.store)
	if username == "" {
		log.L(ctx).Warnf("Authentication failed: invalid credentials")
		return &prototk.AuthenticateResponse{
			Authenticated: false,
		}, nil
	}

	// Return username as authentication result (example: using JSON format)
	// Note: Plugin can return authentication result in any string format (JSON, plain string, etc.)
	authenticationResult := map[string]string{"username": username}
	resultJSON, _ := json.Marshal(authenticationResult)

	return &prototk.AuthenticateResponse{
		Authenticated: true,
		ResultJson:    stringPtr(string(resultJSON)), // Example: JSON format, but plugin could use plain string like username directly
	}, nil
}

// Authorize uses authentication result information to determine if an operation is permitted
func (h *BasicAuthHandler) Authorize(ctx context.Context, req *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
	if h.store == nil {
		log.L(ctx).Warnf("Authorization failed: plugin not configured")
		return &prototk.AuthorizeResponse{
			Authorized: false,
		}, nil
	}

	// Parse authentication result (this example expects JSON format, but format is plugin-specific)
	// The plugin receives the same string it returned from Authenticate
	var authenticationResult map[string]string
	if err := json.Unmarshal([]byte(req.ResultJson), &authenticationResult); err != nil {
		log.L(ctx).Warnf("Authorization failed: failed to parse authentication result: %v", err)
		return &prototk.AuthorizeResponse{
			Authorized: false,
		}, nil
	}

	// Verify authentication result has username (basic validation)
	username, ok := authenticationResult["username"]
	if !ok || username == "" {
		log.L(ctx).Warnf("Authorization failed: authentication result missing username")
		return &prototk.AuthorizeResponse{
			Authorized: false,
		}, nil
	}

	// Basic auth: all authenticated users are authorized
	return &prototk.AuthorizeResponse{
		Authorized: true,
	}, nil
}

func stringPtr(s string) *string {
	return &s
}
