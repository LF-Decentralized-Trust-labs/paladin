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
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

const testConfig = `contracts:
  factory:
    address: 
  implementations:
    - name: Zeto_Anon
      circuitId: anon
      address: "0x497eedc4299dea2f2a364be10025d0ad0f702de3"
      abi: |
        [{
          "anonymous": false,
          "inputs": [
            {
              "indexed": false,
              "internalType": "uint256[]",
              "name": "outputs",
              "type": "uint256[]"
            },
            {
              "indexed": true,
              "internalType": "address",
              "name": "submitter",
              "type": "address"
            }
          ],
          "name": "UTXOMint",
          "type": "event"
        }]
`

func TestLoadLocalConfig(t *testing.T) {
	_, err := loadLocalConfig()
	assert.EqualError(t, err, "LOCAL_CONFIG environment variable not set")

	configFile := path.Join(t.TempDir(), "test.yaml")
	os.Setenv("LOCAL_CONFIG", configFile)
	defer os.Unsetenv("LOCAL_CONFIG")
	err = os.WriteFile(configFile, []byte(testConfig), 0644)
	assert.NoError(t, err)

	config, err := loadLocalConfig()
	assert.NoError(t, err)
	assert.Equal(t, "", config.DomainContracts.Factory.ContractAddress)
	assert.Equal(t, "anon", config.DomainContracts.Implementations[0].CircuitId)
	assert.Equal(t, "0x497eedc4299dea2f2a364be10025d0ad0f702de3", config.DomainContracts.Implementations[0].ContractAddress)
	expectedAbi := `[{
  "anonymous": false,
  "inputs": [
    {
      "indexed": false,
      "internalType": "uint256[]",
      "name": "outputs",
      "type": "uint256[]"
    },
    {
      "indexed": true,
      "internalType": "address",
      "name": "submitter",
      "type": "address"
    }
  ],
  "name": "UTXOMint",
  "type": "event"
}]
`
	assert.Equal(t, expectedAbi, config.DomainContracts.Implementations[0].Abi)

	contractAbi, err := config.getContractAbi("Zeto_Anon")
	assert.NoError(t, err)
	assert.Equal(t, "UTXOMint", contractAbi.Events()["UTXOMint"].Name)

	_, err = config.getContractAbi("Zeto_AnonNullifier")
	assert.ErrorContains(t, err, "contract Zeto_AnonNullifier not found")
}
