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
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/solutils"
	"github.com/stretchr/testify/assert"
)

func TestGetAllZetoEventAbis(t *testing.T) {
	events := getAllZetoEventAbis()
	assert.Equal(t, 8, len(events))
}

func TestBuildEvents(t *testing.T) {
	const testABI = `{ "abi": [
    {
      "inputs": [
        {
          "indexed": false,
          "internalType": "uint256[]",
          "name": "outputs",
          "type": "uint256[]"
        }
      ],
      "name": "UTXOMint",
      "type": "event"
    },
    {
      "inputs": [
        {
          "indexed": false,
          "internalType": "uint256[]",
          "name": "outputs",
          "type": "uint256[]"
        }
      ],
      "name": "UTXOTransfer",
      "type": "event"
    },
    {
      "inputs": [
        {
          "indexed": false,
          "internalType": "uint256[]",
          "name": "outputs",
          "type": "uint256[]"
        }
      ],
      "name": "UTXOMint",
      "type": "event"
    }
]}`

	contract := solutils.MustLoadBuild([]byte(testABI))
	events := buildEvents(nil, contract)
	assert.Equal(t, 2, len(events))
}
