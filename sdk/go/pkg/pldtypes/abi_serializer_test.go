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

package pldtypes

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)

	values, err := exampleABIFunc.Inputs.ParseJSON(([]byte)(`{
		"salt": "769838A38E4A8559266667738BDF99F0DEE9A6A1C72F2BFEB142640259C67829",
		"owner": "0x83A8c18967a939451Abebb01D72686EE5A91E132",
		"amount": 12345,
		"score": "-0x3E8",
		"shiny": "true"
	}`))
	require.NoError(t, err)

	standardizedJSON, err := StandardABISerializer().SerializeJSON(values)
	require.NoError(t, err)

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
	require.NoError(t, err)

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
	require.NoError(t, err)

	// Fails match on whole (either direction)
	err = ABIsMustMatch(context.Background(), abiA, abiB)
	assert.Regexp(t, "PD020004.*mismatchedFunction", err)
	err = ABIsMustMatch(context.Background(), abiB, abiA)
	assert.Regexp(t, "PD020004.*mismatchedFunction", err)

	// Is ok for a sub-match on just the events (either direction)
	err = ABIsMustMatch(context.Background(), abiA, abiB, abi.Event)
	require.NoError(t, err)
	err = ABIsMustMatch(context.Background(), abiB, abiA, abi.Event)
	require.NoError(t, err)

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
	require.NoError(t, err)

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
	require.NoError(t, err)

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
	require.NoError(t, err)
	err = ABIsMustMatch(context.Background(), abiB, abiA, abi.Event)
	require.NoError(t, err)

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
	require.NoError(t, err)

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
	require.NoError(t, err)

	err = ABIsMustMatch(context.Background(), abiA, abiB)
	require.NoError(t, err)
	err = ABIsMustMatch(context.Background(), abiB, abiA)
	require.NoError(t, err)

	hashA, err := ABISolDefinitionHash(context.Background(), abiA)
	require.NoError(t, err)
	hashB, err := ABISolDefinitionHash(context.Background(), abiB)
	require.NoError(t, err)
	assert.Equal(t, *hashA, *hashB)

}

func TestABIsHashSubSelect(t *testing.T) {

	var abiA abi.ABI
	err := json.Unmarshal(([]byte)(`[
		{
			"type": "event",
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
	require.NoError(t, err)

	var abiB abi.ABI
	err = json.Unmarshal(([]byte)(`[
		{
			"type": "function",
			"name": "ccc",
			"inputs": [
			  {
			    "name": "nameInC",
				"type": "uint256"
			  }
			]
		},
		{
			"type": "event",
			"name": "aaa",
			"inputs": [
			  {
			    "name": "nameInA",
				"type": "uint256"
			  }
			]
		}
	]`), &abiB)
	require.NoError(t, err)

	hashA, err := ABISolDefinitionHash(context.Background(), abiA, "event")
	require.NoError(t, err)
	hashB, err := ABISolDefinitionHash(context.Background(), abiB, "event")
	require.NoError(t, err)
	assert.Equal(t, *hashA, *hashB)

	hashA, err = ABISolDefinitionHash(context.Background(), abiA)
	require.NoError(t, err)
	hashB, err = ABISolDefinitionHash(context.Background(), abiB)
	require.NoError(t, err)
	assert.NotEqual(t, *hashA, *hashB)

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
	require.NoError(t, err)

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
	require.NoError(t, err)

	// Fails match simply due to that one missing _ on _sku vs. sku
	err = ABIsMustMatch(context.Background(), abiA, abiB)
	assert.Regexp(t, "PD020004.*NestedTypeEvent", err)
	err = ABIsMustMatch(context.Background(), abiB, abiA)
	assert.Regexp(t, "PD020004.*NestedTypeEvent", err)

	hashA, err := ABISolDefinitionHash(context.Background(), abiA)
	require.NoError(t, err)
	hashB, err := ABISolDefinitionHash(context.Background(), abiB)
	require.NoError(t, err)
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
	require.NoError(t, err)

	var abiB abi.ABI
	err = json.Unmarshal(([]byte)(`[]`), &abiB)
	require.NoError(t, err)

	// Fails match simply due to that one missing _ on _sku vs. sku
	err = ABIsMustMatch(context.Background(), abiA, abiB)
	assert.Regexp(t, "FF22025", err)
	err = ABIsMustMatch(context.Background(), abiB, abiA)
	assert.Regexp(t, "FF22025", err)
	_, err = ABISolDefinitionHash(context.Background(), abiA)
	assert.Regexp(t, "FF22025", err)

}

func TestJSONFormatOptions(t *testing.T) {

	ctx := context.Background()
	var params abi.ParameterArray
	err := json.Unmarshal(([]byte)(`[
		{
			"name": "date",
			"type": "uint64"
		},
		{
			"name": "stock",
			"type": "tuple[]",
			"internalType": "struct WidgetContract.Widget",
			"components": [
				{
					"name": "item",
					"type": "bytes32"
				},
				{
					"name": "description",
					"type": "string"
				},
				{
					"name": "count",
					"type": "uint256"
				},
				{
					"name": "valueDiff",
					"type": "int256"
				},
				{
					"name": "supplier",
					"type": "address"
				}
			]
		}
	]`), &params)
	require.NoError(t, err)

	cv, err := params.ParseExternalDataCtx(ctx, map[string]any{
		"date": 1729450200,
		"stock": []map[string]any{
			{
				"item":        "0xbb36636e2b58f2ca2538a966b95a253ed78c6bd1d176255be5a58c7ced3c21ea",
				"description": "widgetA",
				"count":       100,
				"valueDiff":   "-123456789012345678901234567890", // big number
				"supplier":    "0xB8F7764d413B518c49824fb5E6078b41B2549d4e",
			},
		},
	})
	require.NoError(t, err)

	type withOpts struct {
		ResultFormat JSONFormatOptions `json:"resultFormat"`
	}

	checkEqual := func(format, expectedJSON string) {
		o := withOpts{ResultFormat: JSONFormatOptions(format)}
		serializer, err := o.ResultFormat.GetABISerializer(ctx)
		require.NoError(t, err)
		json, err := serializer.SerializeJSON(cv)
		require.NoError(t, err)
		fmt.Println(RawJSON(json).Pretty())
		assert.JSONEq(t, expectedJSON, string(json))
	}

	checkEqual("", `{
		"date": "1729450200",
		"stock": [
			{
				"count": "100",
				"description": "widgetA",
				"valueDiff": "-123456789012345678901234567890",
				"item": "0xbb36636e2b58f2ca2538a966b95a253ed78c6bd1d176255be5a58c7ced3c21ea",
				"supplier": "0xb8f7764d413b518c49824fb5e6078b41b2549d4e"
			}
		]
	}`)
	checkEqual("mode=object&number=hex&bytes=hex-plain&address=checksum", `{
		"date": "0x671550d8",
		"stock": [
			{
				"count": "0x64",
				"description": "widgetA",
				"valueDiff": "-0x18ee90ff6c373e0ee4e3f0ad2",
				"item": "bb36636e2b58f2ca2538a966b95a253ed78c6bd1d176255be5a58c7ced3c21ea",
				"supplier": "0xB8F7764d413B518c49824fb5E6078b41B2549d4e"
			}
		]
	}`)
	checkEqual("mode=object&number=json-number&bytes=base64&address=hex-plain", `{
		"date": 1729450200,
		"stock": [
			{
				"count": 100,
				"description": "widgetA",
				"valueDiff": -123456789012345678901234567890,
				"item": "uzZjbitY8solOKlmuVolPteMa9HRdiVb5aWMfO08Ieo=",
				"supplier": "b8f7764d413b518c49824fb5e6078b41b2549d4e"
			}
		]
	}`)
	checkEqual("mode=array&number=string&bytes=hex-plain&address=hex", `[
		"1729450200",
		[
			["bb36636e2b58f2ca2538a966b95a253ed78c6bd1d176255be5a58c7ced3c21ea", "widgetA", "100", "-123456789012345678901234567890", "0xb8f7764d413b518c49824fb5e6078b41b2549d4e"]
		]
	]`)
	checkEqual("mode=self-describing&number=json-number&bytes=hex&pretty", `[
		{
			"name": "date",
			"type": "uint64",
			"value": 1729450200
		},
		{
			"name": "stock",
			"type": "(bytes32,string,uint256,int256,address)[]",
			"value": [
				[
					{
						"name": "item",
						"type": "bytes32",
						"value": "0xbb36636e2b58f2ca2538a966b95a253ed78c6bd1d176255be5a58c7ced3c21ea"
					},
					{
						"name": "description",
						"type": "string",
						"value": "widgetA"
					},
					{
						"name": "count",
						"type": "uint256",
						"value": 100
					},
					{
						"name": "valueDiff",
						"type": "int256",
						"value": -123456789012345678901234567890
					},
					{
						"name": "supplier",
						"type": "address",
						"value": "0xb8f7764d413b518c49824fb5e6078b41b2549d4e"
					}
				]
			]
		}
	]`)

}

func TestJSONFormatOptionErrors(t *testing.T) {
	ctx := context.Background()

	_, err := JSONFormatOptions("this;is;not;url;query").GetABISerializer(ctx)
	assert.Regexp(t, "PD020014", err)

	_, err = JSONFormatOptions("color=blue").GetABISerializer(ctx)
	assert.Regexp(t, "PD020015", err)

	_, err = JSONFormatOptions("mode=blue").GetABISerializer(ctx)
	assert.Regexp(t, "PD020015", err)

	_, err = JSONFormatOptions("number=blue").GetABISerializer(ctx)
	assert.Regexp(t, "PD020015", err)

	_, err = JSONFormatOptions("bytes=blue").GetABISerializer(ctx)
	assert.Regexp(t, "PD020015", err)

	_, err = JSONFormatOptions("address=blue").GetABISerializer(ctx)
	assert.Regexp(t, "PD020015", err)

	s := JSONFormatOptions("this;is;ignored").GetABISerializerIgnoreErrors(ctx)
	assert.NotNil(t, s)

}
