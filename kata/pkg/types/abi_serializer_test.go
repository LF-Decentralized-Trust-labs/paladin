// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package types

import (
	"encoding/json"
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
)

func TestStandardABISerializer(t *testing.T) {

	exampleABIFuncJSON := `{
		"type": "function",
		"inputs": [
			{
				"name": "salt",
				"type": "bytes32"
			},
			{
				"name": "owner",
				"type": "address"
			},
			{
				"name": "amount",
				"type": "uint256"
			},
			{
				"name": "score",
				"type": "int256"
			},
			{
				"name": "shiny",
				"type": "bool"
			}
		]
	}`
	var exampleABIFunc abi.Entry
	err := json.Unmarshal([]byte(exampleABIFuncJSON), &exampleABIFunc)
	assert.NoError(t, err)

	values, err := exampleABIFunc.Inputs.ParseJSON(([]byte)(`{
		"salt": "769838A38E4A8559266667738BDF99F0DEE9A6A1C72F2BFEB142640259C67829",
		"owner": "0x83A8c18967a939451Abebb01D72686EE5A91E132",
		"amount": 12345,
		"score": "-0x3E8",
		"shiny": "true"
	}`))
	assert.NoError(t, err)

	standardizedJSON, err := StandardABISerializer().SerializeJSON(values)
	assert.NoError(t, err)

	assert.JSONEq(t, `{
		"salt": "0x769838a38e4a8559266667738bdf99f0dee9a6a1c72f2bfeb142640259c67829",
		"owner": "0x83a8c18967a939451abebb01d72686ee5a91e132",
		"amount": "12345",
		"score": "-1000",
		"shiny": true
	}`, (string)(standardizedJSON))
}
