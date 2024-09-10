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

package zeto

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	_, err := New(nil)
	assert.ErrorContains(t, err, "")

	configFile := path.Join(t.TempDir(), "test.yaml")
	os.Setenv("LOCAL_CONFIG", configFile)
	defer os.Unsetenv("LOCAL_CONFIG")
	err = os.WriteFile(configFile, []byte(testConfig), 0644)
	assert.NoError(t, err)

	z, err := New(nil)
	assert.NoError(t, err)
	assert.NotNil(t, z)
}

func TestDecodeDomainConfig(t *testing.T) {
	config := &types.DomainInstanceConfig{
		CircuitId: "circuit-id",
		TokenName: "token-name",
	}
	configJSON, err := json.Marshal(config)
	assert.NoError(t, err)

	encoded, err := types.DomainInstanceConfigABI.EncodeABIDataJSON(configJSON)
	assert.NoError(t, err)

	z := &Zeto{}
	decoded, err := z.decodeDomainConfig(context.Background(), encoded)
	assert.NoError(t, err)
	assert.Equal(t, config, decoded)
}
