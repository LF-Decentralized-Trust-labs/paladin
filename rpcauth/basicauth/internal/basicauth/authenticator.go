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
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// CredentialsStore holds the loaded credentials.
// This struct is read-only after LoadCredentials() completes, making it safe
// for concurrent reads. All methods are safe for concurrent use.
type CredentialsStore struct {
	users map[string]string // username -> bcrypt hash (standard htpasswd format)
}

// LoadCredentials loads credentials from the file
func LoadCredentials(filePath string) (*CredentialsStore, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open credentials file: %w", err)
	}
	defer file.Close()

	store := &CredentialsStore{
		users: make(map[string]string),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue // Skip malformed lines
		}

		username := strings.TrimSpace(parts[0])
		hash := strings.TrimSpace(parts[1])

		if username != "" && hash != "" {
			store.users[username] = hash
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	return store, nil
}

// Authenticate verifies username/password against the credentials store
func (cs *CredentialsStore) Authenticate(username, password string) bool {
	// Get the stored hash for this user
	storedHash, exists := cs.users[username]
	if !exists {
		return false
	}

	// Accept direct bcrypt format (htpasswd-style: $2a$/2b$/2y$)
	// bcrypt.CompareHashAndPassword accepts the string directly - no base64 encoding needed
	err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password))
	return err == nil
}

// ParseBasicAuthHeader extracts username and password from Authorization header
func ParseBasicAuthHeader(header string) (username, password string, ok bool) {
	const prefix = "Basic "

	if !strings.HasPrefix(header, prefix) {
		return "", "", false
	}

	encoded := strings.TrimPrefix(header, prefix)

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", false
	}

	credentials := string(decoded)

	// Find the first colon that separates username from password
	idx := strings.IndexByte(credentials, ':')
	if idx < 0 {
		return "", "", false
	}

	username = credentials[:idx]
	password = credentials[idx+1:]

	return username, password, true
}

// CheckHeaderAuthentication checks HTTP Basic Auth header and returns username if authenticated
// Returns empty string if authentication fails
func CheckHeaderAuthentication(headers map[string]string, store *CredentialsStore) string {
	authHeader, exists := headers["Authorization"]
	if !exists {
		// Also check lowercase key
		authHeader, exists = headers["authorization"]
	}

	if !exists {
		return ""
	}

	username, password, ok := ParseBasicAuthHeader(authHeader)
	if !ok {
		return ""
	}

	// Authenticate using bcrypt, which provides its own timing attack protection
	if !store.Authenticate(username, password) {
		return ""
	}

	// Return username if valid, empty string if not
	return username
}
