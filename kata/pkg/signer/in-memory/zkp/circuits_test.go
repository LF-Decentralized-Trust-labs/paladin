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

package zkp

import (
	"os"
	"path"
	"testing"

	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
	"github.com/stretchr/testify/assert"
)

func mockWASMModule() []byte {
	return []byte(`(module
  (func $add (param $lhs i32) (result i32)
    local.get $lhs)
  (export "init" (func $add))
)`)
}

func TestLoadCircuit(t *testing.T) {
	tmpDir := t.TempDir()
	err := os.Mkdir(path.Join(tmpDir, "test_js"), 0755)
	assert.NoError(t, err)
	err = os.WriteFile(path.Join(tmpDir, "test_js", "test.wasm"), mockWASMModule(), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(path.Join(tmpDir, "test.zkey"), []byte("test"), 0644)
	assert.NoError(t, err)

	config := api.SnarkProverConfig{}
	config.CircuitsDir = tmpDir
	config.ProvingKeysDir = tmpDir

	circuit, provingKey, err := loadCircuit("test", config)
	assert.EqualError(t, err, "Export `getFieldNumLen32` does not exist")
	assert.Nil(t, circuit)
	assert.Equal(t, []byte{}, provingKey)
}
