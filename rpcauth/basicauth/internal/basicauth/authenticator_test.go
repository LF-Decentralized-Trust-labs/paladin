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
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func createTempCredsFile(t *testing.T, content string) string {
	tmpFile, err := os.CreateTemp("", "test_users_*.txt")
	require.NoError(t, err)
	defer tmpFile.Close()

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)

	return tmpFile.Name()
}

func TestParseBasicAuthHeader(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		expectOK   bool
		expectUser string
		expectPass string
	}{
		{
			name:       "Valid basic auth",
			header:     "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass")),
			expectOK:   true,
			expectUser: "user",
			expectPass: "pass",
		},
		{
			name:     "Invalid prefix",
			header:   "Bearer token",
			expectOK: false,
		},
		{
			name:     "Invalid base64",
			header:   "Basic invalid###base64",
			expectOK: false,
		},
		{
			name:     "Missing colon",
			header:   "Basic " + base64.StdEncoding.EncodeToString([]byte("userpass")),
			expectOK: false,
		},
		{
			name:       "Empty credentials",
			header:     "Basic " + base64.StdEncoding.EncodeToString([]byte(":")),
			expectOK:   true,
			expectUser: "",
			expectPass: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, pass, ok := ParseBasicAuthHeader(tt.header)

			assert.Equal(t, tt.expectOK, ok)
			if ok {
				assert.Equal(t, tt.expectUser, user)
				assert.Equal(t, tt.expectPass, pass)
			}
		})
	}
}

func TestLoadCredentials(t *testing.T) {
	// Create a test credentials file
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	require.NoError(t, err)

	hashString := string(hashedPassword)
	content := "user1:" + hashString + "\nuser2:another_hash\n"

	tmpFile := createTempCredsFile(t, content)
	defer os.Remove(tmpFile)

	store, err := LoadCredentials(tmpFile)
	require.NoError(t, err)
	require.NotNil(t, store)

	assert.Equal(t, hashString, store.users["user1"])
	assert.Equal(t, "another_hash", store.users["user2"])
}

func TestLoadCredentials_EmptyFile(t *testing.T) {
	tmpFile := createTempCredsFile(t, "")
	defer os.Remove(tmpFile)

	store, err := LoadCredentials(tmpFile)
	require.NoError(t, err)
	require.NotNil(t, store)
	assert.Empty(t, store.users)
}

func TestLoadCredentials_Comments(t *testing.T) {
	content := "# This is a comment\nuser1:hash1\n  # Another comment\nuser2:hash2\n"
	tmpFile := createTempCredsFile(t, content)
	defer os.Remove(tmpFile)

	store, err := LoadCredentials(tmpFile)
	require.NoError(t, err)
	require.NotNil(t, store)

	assert.Equal(t, "hash1", store.users["user1"])
	assert.Equal(t, "hash2", store.users["user2"])
}

func TestAuthenticate(t *testing.T) {
	// Create hashed password for testing
	correctPassword := "myPassword123"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(correctPassword), bcrypt.MinCost)
	require.NoError(t, err)

	hashString := string(hashedPassword)

	store := &CredentialsStore{
		users: map[string]string{
			"testuser": hashString,
		},
	}

	tests := []struct {
		name       string
		username   string
		password   string
		expectAuth bool
	}{
		{
			name:       "Correct credentials",
			username:   "testuser",
			password:   correctPassword,
			expectAuth: true,
		},
		{
			name:       "Wrong password",
			username:   "testuser",
			password:   "wrongpassword",
			expectAuth: false,
		},
		{
			name:       "Unknown user",
			username:   "unknown",
			password:   "any",
			expectAuth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorized := store.Authenticate(tt.username, tt.password)
			assert.Equal(t, tt.expectAuth, authorized)
		})
	}
}

func TestCheckHeaderAuthentication(t *testing.T) {
	// Create test credentials
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	require.NoError(t, err)

	hashString := string(hashedPassword)

	store := &CredentialsStore{
		users: map[string]string{
			"admin": hashString,
		},
	}

	tests := []struct {
		name             string
		headers          map[string]string
		expectedUsername string // Empty string if authentication fails
	}{
		{
			name: "Valid basic auth",
			headers: map[string]string{
				"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:secret")),
			},
			expectedUsername: "admin",
		},
		{
			name: "Lowercase authorization key",
			headers: map[string]string{
				"authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:secret")),
			},
			expectedUsername: "admin",
		},
		{
			name: "Wrong password",
			headers: map[string]string{
				"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:wrong")),
			},
			expectedUsername: "",
		},
		{
			name: "No auth header",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			expectedUsername: "",
		},
		{
			name:             "Empty headers",
			headers:          map[string]string{},
			expectedUsername: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username := CheckHeaderAuthentication(tt.headers, store)
			assert.Equal(t, tt.expectedUsername, username)
		})
	}
}

func TestLoadCredentials_NonExistentFile(t *testing.T) {
	nonExistent := filepath.Join(t.TempDir(), "nonexistent.txt")
	_, err := LoadCredentials(nonExistent)
	assert.Error(t, err)
}

func TestLoadCredentials_MalformedLines(t *testing.T) {
	content := "user1\nuser2:hash2\n:hash3\nbadlinewithnocolons"
	tmpFile := createTempCredsFile(t, content)
	defer os.Remove(tmpFile)

	store, err := LoadCredentials(tmpFile)
	require.NoError(t, err)
	require.NotNil(t, store)

	// Only user2 should be loaded (the well-formed line)
	assert.Equal(t, "hash2", store.users["user2"])
	assert.Empty(t, store.users["user1"])
}
