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
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func setupHandlerWithCredentials(t *testing.T) *BasicAuthHandler {
	password := "testpass123"

	// Generate bcrypt hash
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)

	// Store raw hash string (not base64 encoded)
	hashString := string(hashBytes)

	// Create temp credentials file
	tmpFile := createTempCredsFile(t, "testuser:"+hashString+"\n")

	handler := &BasicAuthHandler{}
	ctx := context.Background()

	req := &prototk.ConfigureRPCAuthorizerRequest{
		ConfigJson: `{"credentialsFile": "` + tmpFile + `"}`,
	}

	_, err = handler.ConfigureRPCAuthorizer(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, handler.store, "Store should be loaded")

	// Store temp file name in handler for cleanup
	t.Cleanup(func() {
		os.Remove(tmpFile)
	})

	return handler
}

func TestBasicAuthHandler_Configure(t *testing.T) {
	handler := &BasicAuthHandler{}
	ctx := context.Background()

	password := "mypassword"
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)

	// Store raw hash string
	hashString := string(hashBytes)
	tmpFile := createTempCredsFile(t, "user:"+hashString+"\n")
	t.Cleanup(func() {
		os.Remove(tmpFile)
	})

	req := &prototk.ConfigureRPCAuthorizerRequest{
		ConfigJson: `{"credentialsFile": "` + tmpFile + `"}`,
	}

	resp, err := handler.ConfigureRPCAuthorizer(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotNil(t, handler.store, "Store should be loaded")
	assert.Contains(t, handler.store.users, "user", "User should exist in store")
}

func TestBasicAuthHandler_Configure_MissingFile(t *testing.T) {
	handler := &BasicAuthHandler{}
	ctx := context.Background()

	req := &prototk.ConfigureRPCAuthorizerRequest{
		ConfigJson: `{"credentialsFile": "/nonexistent/file.txt"}`,
	}

	_, err := handler.ConfigureRPCAuthorizer(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, handler.store)
}

func TestBasicAuthHandler_Configure_EmptyConfig(t *testing.T) {
	handler := &BasicAuthHandler{}
	ctx := context.Background()

	req := &prototk.ConfigureRPCAuthorizerRequest{
		ConfigJson: "", // Empty config
	}

	_, err := handler.ConfigureRPCAuthorizer(ctx, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config is required")
}

func TestBasicAuthHandler_Configure_InvalidJSON(t *testing.T) {
	handler := &BasicAuthHandler{}
	ctx := context.Background()

	req := &prototk.ConfigureRPCAuthorizerRequest{
		ConfigJson: `{invalid json}`,
	}

	_, err := handler.ConfigureRPCAuthorizer(ctx, req)
	assert.Error(t, err)
}

// Authenticate tests
func TestBasicAuthHandler_Authenticate_Success(t *testing.T) {
	handler := setupHandlerWithCredentials(t)

	headers := map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass123")),
	}
	headersJSON, _ := json.Marshal(headers)

	req := &prototk.AuthenticateRequest{
		HeadersJson: string(headersJSON),
	}

	resp, err := handler.Authenticate(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, resp.Authenticated)
	assert.NotNil(t, resp.ResultJson)
}

func TestBasicAuthHandler_Authenticate_WrongPassword(t *testing.T) {
	handler := setupHandlerWithCredentials(t)

	headers := map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:wrongpass")),
	}
	headersJSON, _ := json.Marshal(headers)

	req := &prototk.AuthenticateRequest{
		HeadersJson: string(headersJSON),
	}

	resp, err := handler.Authenticate(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, resp.Authenticated)
}

func TestBasicAuthHandler_Authenticate_NoHeader(t *testing.T) {
	handler := setupHandlerWithCredentials(t)

	headers := map[string]string{
		"Content-Type": "application/json",
	}
	headersJSON, _ := json.Marshal(headers)

	req := &prototk.AuthenticateRequest{
		HeadersJson: string(headersJSON),
	}

	resp, err := handler.Authenticate(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, resp.Authenticated)
}

func TestBasicAuthHandler_Authenticate_InvalidHeader(t *testing.T) {
	handler := setupHandlerWithCredentials(t)

	headers := map[string]string{
		"Authorization": "NotBasic invalid",
	}
	headersJSON, _ := json.Marshal(headers)

	req := &prototk.AuthenticateRequest{
		HeadersJson: string(headersJSON),
	}

	resp, err := handler.Authenticate(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, resp.Authenticated)
}

func TestBasicAuthHandler_Authenticate_UnknownUser(t *testing.T) {
	handler := setupHandlerWithCredentials(t)

	headers := map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("unknownuser:anypass")),
	}
	headersJSON, _ := json.Marshal(headers)

	req := &prototk.AuthenticateRequest{
		HeadersJson: string(headersJSON),
	}

	resp, err := handler.Authenticate(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, resp.Authenticated)
}

func TestBasicAuthHandler_Authenticate_NotConfigured(t *testing.T) {
	handler := &BasicAuthHandler{} // No configuration

	headersJSON, _ := json.Marshal(map[string]string{})

	req := &prototk.AuthenticateRequest{
		HeadersJson: string(headersJSON),
	}

	resp, err := handler.Authenticate(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, resp.Authenticated)
}

func TestBasicAuthHandler_Authenticate_InvalidHeadersJSON(t *testing.T) {
	handler := setupHandlerWithCredentials(t)

	req := &prototk.AuthenticateRequest{
		HeadersJson: `{invalid json}`,
	}

	resp, err := handler.Authenticate(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, resp.Authenticated)
}

// Authorize tests
func TestBasicAuthHandler_Authorize_Success(t *testing.T) {
	handler := setupHandlerWithCredentials(t)

	// First authenticate to get authentication result
	headers := map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass123")),
	}
	headersJSON, _ := json.Marshal(headers)
	authReq := &prototk.AuthenticateRequest{
		HeadersJson: string(headersJSON),
	}
	authResp, err := handler.Authenticate(context.Background(), authReq)
	require.NoError(t, err)
	require.True(t, authResp.Authenticated)
	require.NotNil(t, authResp.ResultJson)

	// Now authorize with the authentication result
	req := &prototk.AuthorizeRequest{
		ResultJson:  *authResp.ResultJson,
		Method:      "account_sendTransaction",
		PayloadJson: `{"to": "0x123", "value": "1"}`,
	}

	resp, err := handler.Authorize(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, resp.Authorized)
}

func TestBasicAuthHandler_Authorize_InvalidResult(t *testing.T) {
	handler := setupHandlerWithCredentials(t)

	req := &prototk.AuthorizeRequest{
		ResultJson:  `{invalid json}`,
		Method:      "account_sendTransaction",
		PayloadJson: `{}`,
	}

	resp, err := handler.Authorize(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, resp.Authorized)
}

func TestBasicAuthHandler_Authorize_NotConfigured(t *testing.T) {
	handler := &BasicAuthHandler{} // No configuration

	req := &prototk.AuthorizeRequest{
		ResultJson:  `{"username":"test"}`,
		Method:      "account_sendTransaction",
		PayloadJson: `{}`,
	}

	resp, err := handler.Authorize(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, resp.Authorized)
}

func TestBasicAuthHandler_Authorize_MissingUsername(t *testing.T) {
	handler := setupHandlerWithCredentials(t)

	// Authentication result without "username" key
	req := &prototk.AuthorizeRequest{
		ResultJson:  `{"user":"test"}`, // Wrong key name
		Method:      "account_sendTransaction",
		PayloadJson: `{}`,
	}

	resp, err := handler.Authorize(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, resp.Authorized)
}

func TestBasicAuthHandler_Authorize_EmptyUsername(t *testing.T) {
	handler := setupHandlerWithCredentials(t)

	// Authentication result with empty username
	req := &prototk.AuthorizeRequest{
		ResultJson:  `{"username":""}`, // Empty username
		Method:      "account_sendTransaction",
		PayloadJson: `{}`,
	}

	resp, err := handler.Authorize(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, resp.Authorized)
}
