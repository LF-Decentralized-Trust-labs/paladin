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

package tktypes

import (
	"context"
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

func TestABIsMustMatchSubMatch(t *testing.T) {

	var abiA abi.ABI
	err := json.Unmarshal(([]byte)(`[
		{
			"type": "function",
			"name": "mismatchedFunction",
			"inputs": [
			  {
			    "name": "nameInA",
				"type": "uint256"
			  }
			]
		},
		{
			"type": "event",
			"name": "MatchedEvent",
			"inputs": [
			  {
			    "name": "nameInBoth",
				"type": "uint256"
			  }
			]
		}
	]`), &abiA)
	assert.NoError(t, err)

	var abiB abi.ABI
	err = json.Unmarshal(([]byte)(`[
		{
			"type": "function",
			"name": "mismatchedFunction",
			"inputs": [
			  {
			    "name": "nameInB",
				"type": "uint256"
			  }
			]
		},
		{
			"type": "event",
			"name": "MatchedEvent",
			"inputs": [
			  {
			    "name": "nameInBoth",
				"type": "uint256"
			  }
			]
		}
	]`), &abiB)
	assert.NoError(t, err)

	// Fails match on whole (either direction)
	err = ABIsMustMatch(context.Background(), abiA, abiB)
	assert.Regexp(t, "PD020004.*mismatchedFunction", err)
	err = ABIsMustMatch(context.Background(), abiB, abiA)
	assert.Regexp(t, "PD020004.*mismatchedFunction", err)

	// Is ok for a sub-match on just the events (either direction)
	err = ABIsMustMatch(context.Background(), abiA, abiB, abi.Event)
	assert.NoError(t, err)
	err = ABIsMustMatch(context.Background(), abiB, abiA, abi.Event)
	assert.NoError(t, err)

}

func TestABIsMustMatchExtra(t *testing.T) {

	var abiA abi.ABI
	err := json.Unmarshal(([]byte)(`[
		{
			"type": "function",
			"name": "extraFunction",
			"inputs": [
			  {
			    "name": "nameInA",
				"type": "uint256"
			  }
			]
		},
		{
			"type": "event",
			"name": "MatchedEvent",
			"inputs": [
			  {
			    "name": "nameInBoth",
				"type": "uint256"
			  }
			]
		}
	]`), &abiA)
	assert.NoError(t, err)

	var abiB abi.ABI
	err = json.Unmarshal(([]byte)(`[
		{
			"type": "event",
			"name": "MatchedEvent",
			"inputs": [
			  {
			    "name": "nameInBoth",
				"type": "uint256"
			  }
			]
		}
	]`), &abiB)
	assert.NoError(t, err)

	// Fails match on whole (either direction)
	err = ABIsMustMatch(context.Background(), abiA, abiB)
	assert.Regexp(t, "PD020004.*extraFunction", err)
	err = ABIsMustMatch(context.Background(), abiB, abiA)
	assert.Regexp(t, "PD020004.*extraFunction", err)
	err = ABIsMustMatch(context.Background(), abiA, abiB, abi.Function)
	assert.Regexp(t, "PD020004.*extraFunction", err)
	err = ABIsMustMatch(context.Background(), abiB, abiA, abi.Function)
	assert.Regexp(t, "PD020004.*extraFunction", err)

	// Is ok for a sub-match on just the events (either direction)
	err = ABIsMustMatch(context.Background(), abiA, abiB, abi.Event)
	assert.NoError(t, err)
	err = ABIsMustMatch(context.Background(), abiB, abiA, abi.Event)
	assert.NoError(t, err)

}

func TestABIsMustMatchOrder(t *testing.T) {

	var abiA abi.ABI
	err := json.Unmarshal(([]byte)(`[
		{
			"type": "function",
			"name": "aaa",
			"inputs": [
			  {
			    "name": "nameInA",
				"type": "uint256"
			  }
			]
		},
		{
			"type": "function",
			"name": "bbb",
			"inputs": [
			  {
			    "name": "nameInB",
				"type": "uint256"
			  }
			]
		}
	]`), &abiA)
	assert.NoError(t, err)

	var abiB abi.ABI
	err = json.Unmarshal(([]byte)(`[
		{
			"type": "function",
			"name": "bbb",
			"inputs": [
			  {
			    "name": "nameInB",
				"type": "uint256"
			  }
			]
		},
		{
			"type": "function",
			"name": "aaa",
			"inputs": [
			  {
			    "name": "nameInA",
				"type": "uint256"
			  }
			]
		}
	]`), &abiB)
	assert.NoError(t, err)

	err = ABIsMustMatch(context.Background(), abiA, abiB)
	assert.NoError(t, err)
	err = ABIsMustMatch(context.Background(), abiB, abiA)
	assert.NoError(t, err)

	hashA, err := ABISolDefinitionHash(context.Background(), abiA)
	assert.NoError(t, err)
	hashB, err := ABISolDefinitionHash(context.Background(), abiB)
	assert.NoError(t, err)
	assert.Equal(t, *hashA, *hashB)

}

func TestABIsDeepMisMatchName(t *testing.T) {

	var abiA abi.ABI
	err := json.Unmarshal(([]byte)(`[
		{
			"type": "event",
			"name": "NestedTypeEvent",
			"inputs": [
			  {
			    "name": "widget",
				"type": "tuple",
				"internalType": "struct WidgetContract.Widget",
				"components": [
				   {
				     "name": "_sku",
					 "type": "uint256"
				   }
				]
			  }
			]
		}
	]`), &abiA)
	assert.NoError(t, err)

	var abiB abi.ABI
	err = json.Unmarshal(([]byte)(`[
		{
			"type": "event",
			"name": "NestedTypeEvent",
			"inputs": [
			  {
			    "name": "widget",
				"type": "tuple",
				"internalType": "struct WidgetContract.Widget",
				"components": [
				   {
				     "name": "sku",
					 "type": "uint256"
				   }
				]
			  }
			]
		}
	]`), &abiB)
	assert.NoError(t, err)

	// Fails match simply due to that one missing _ on _sku vs. sku
	err = ABIsMustMatch(context.Background(), abiA, abiB)
	assert.Regexp(t, "PD020004.*NestedTypeEvent", err)
	err = ABIsMustMatch(context.Background(), abiB, abiA)
	assert.Regexp(t, "PD020004.*NestedTypeEvent", err)

	hashA, err := ABISolDefinitionHash(context.Background(), abiA)
	assert.NoError(t, err)
	hashB, err := ABISolDefinitionHash(context.Background(), abiB)
	assert.NoError(t, err)
	assert.NotEqual(t, *hashA, *hashB)

}

func TestABIsBadTypes(t *testing.T) {

	var abiA abi.ABI
	err := json.Unmarshal(([]byte)(`[
		{
			"type": "event",
			"name": "Bad",
			"inputs": [
			  {
			    "name": "badness",
				"type": "wrong"
			  }
			]
		}
	]`), &abiA)
	assert.NoError(t, err)

	var abiB abi.ABI
	err = json.Unmarshal(([]byte)(`[]`), &abiB)
	assert.NoError(t, err)

	// Fails match simply due to that one missing _ on _sku vs. sku
	err = ABIsMustMatch(context.Background(), abiA, abiB)
	assert.Regexp(t, "FF22025", err)
	err = ABIsMustMatch(context.Background(), abiB, abiA)
	assert.Regexp(t, "FF22025", err)
	_, err = ABISolDefinitionHash(context.Background(), abiA)
	assert.Regexp(t, "FF22025", err)

}
