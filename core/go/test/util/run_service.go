/*
 * Copyright © 2024 Kaleido, Inc.
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

/*
Utilities for testing
*/

package util

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kaleido-io/paladin/core/pkg/bootstrap"
	"github.com/stretchr/testify/require"
)

func RunServiceForTesting(ctx context.Context, t *testing.T) (string, func()) {
	// get a valid file name for a temp file by first creating a temp file and then removing it
	file, err := os.CreateTemp("", "paladin.sock")
	require.NoError(t, err)
	socketAddress := file.Name()
	os.Remove(file.Name())

	// Create and write a config file
	configFile, err := os.CreateTemp("", "test_*.yaml")
	require.NoError(t, err)

	// Write YAML content to the temporary file
	yamlContent := []byte(`
persistence:
  type: sqlite
  sqlite:
    uri:           ":memory:"
    autoMigrate:   true
    migrationsDir: ../../db/migrations/sqlite
    debugQueries:  true
commsBus:  
  grpc:
    socketAddress: ` + socketAddress + `
plugins:
  - name: mock-transport-plugin
    type: interPaladinTransport  
`)
	_, err = configFile.Write(yamlContent)
	require.NoError(t, err)

	configFile.Close()

	// Start the server
	go bootstrap.TestCommsBusRun(ctx, configFile.Name())

	// todo do we really need to sleep here?
	time.Sleep(time.Second * 2)

	return socketAddress, func() {
		os.Remove(configFile.Name())
	}

}
