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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name        string
		configJSON  string
		expectError bool
		expected    *Config
	}{
		{
			name:        "Valid config",
			configJSON:  `{"credentialsFile": "/path/to/users.txt"}`,
			expectError: false,
			expected: &Config{
				CredentialsFile: "/path/to/users.txt",
			},
		},
		{
			name:        "Missing credentialsFile",
			configJSON:  `{}`,
			expectError: true,
		},
		{
			name:        "Empty credentialsFile",
			configJSON:  `{"credentialsFile": ""}`,
			expectError: true,
		},
		{
			name:        "Invalid JSON",
			configJSON:  `{credentialsFile: invalid}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := parseConfig(tt.configJSON)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, config)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, config)
			}
		})
	}
}

func TestParseConfig_ValidConfig(t *testing.T) {
	config, err := parseConfig(`{"credentialsFile": "/tmp/users.txt"}`)
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, "/tmp/users.txt", config.CredentialsFile)
}
