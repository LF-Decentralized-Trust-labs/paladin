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
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestABIInference(t *testing.T) {

	inputJSON := RawJSON(`{
		"000_string": "bla",
	  	"100_number": 42000000000000000000000,
	  	"200_boolean": false,
	  	"300_strArray": [ "a", "b", "c" ],
	  	"400_objectArray": [ {
	    	"410_number": 99,
			"420_numberArray": [ 1, 2, 3 ]
	  	} ],
	  	"500_nestedObj": {
		  "510_string": "bla",
		  "520_string": "bla",
		  "530_string": "bla",
		  "540_number": 12345
	  	},
		"600_multiDimArray": [
		  [ 100, 200 ]
		]
	}`)

	p, err := ABIInferenceFromJSON(context.Background(), inputJSON)
	require.NoError(t, err)
	require.Equal(t, abi.ParameterArray{
		{
			Name:    "000_string",
			Type:    "string",
			Indexed: true,
		},
		{
			Name:    "100_number",
			Type:    "int256",
			Indexed: true,
		},
		{
			Name:    "200_boolean",
			Type:    "bool",
			Indexed: true,
		},
		{
			Name: "300_strArray",
			Type: "string[]",
		},
		{
			Name:         "400_objectArray",
			Type:         "tuple[]",
			InternalType: "struct 400_Objectarray[]",
			Components: abi.ParameterArray{
				{
					Name: "410_number",
					Type: "int256",
				},
				{
					Name: "420_numberArray",
					Type: "int256[]",
				},
			},
		},
		{
			Name:         "500_nestedObj",
			Type:         "tuple",
			InternalType: "struct 500_Nestedobj",
			Components: abi.ParameterArray{
				{
					Name: "510_string",
					Type: "string",
				},
				{
					Name: "520_string",
					Type: "string",
				},
				{
					Name: "530_string",
					Type: "string",
				},
				{
					Name: "540_number",
					Type: "int256",
				},
			},
		},
		{
			Name: "600_multiDimArray",
			Type: "int256[][]",
		},
	}, p)

	// Now the parameter set we've got should parse the data we supply successfully
	cv, err := p.ParseJSON(inputJSON)
	require.NoError(t, err)

	formattedJSON, err := abi.NewSerializer().SetIntSerializer(abi.JSONNumberIntSerializer).SerializeJSON(cv)
	require.NoError(t, err)
	assert.JSONEq(t, inputJSON.Pretty(), string(formattedJSON))

}

func TestABIInferenceBadJSON(t *testing.T) {

	_, err := ABIInferenceFromJSON(context.Background(), RawJSON(`{! wrong`))
	assert.Regexp(t, "PD020020", err)

}

func TestABIInferenceNil(t *testing.T) {

	p, err := ABIInferenceFromJSON(context.Background(), nil)
	assert.NoError(t, err)
	assert.Empty(t, p)

}

func TestABIInferenceEmpty(t *testing.T) {

	p, err := ABIInferenceFromJSON(context.Background(), RawJSON(`{}`))
	assert.NoError(t, err)
	assert.Empty(t, p)

}

func TestABIInferenceNonInt(t *testing.T) {

	_, err := ABIInferenceFromJSON(context.Background(), RawJSON(`{"nonInt": 1.2}`))
	assert.Regexp(t, "PD020022", err)

}

func TestABIInferenceEmptyArray(t *testing.T) {

	_, err := ABIInferenceFromJSON(context.Background(), RawJSON(`{"emptyArray": []}`))
	assert.Regexp(t, "PD020023", err)

}

func TestABIInferenceNestedArrayIssue(t *testing.T) {

	_, err := ABIInferenceFromJSON(context.Background(), RawJSON(`{"nestedArrayIssue": [[1.2]]}`))
	assert.Regexp(t, `PD020022.*nestedArrayIssue\[\]\[\]`, err)

}

func TestABIInferenceNestedObjectNullError(t *testing.T) {

	_, err := ABIInferenceFromJSON(context.Background(), RawJSON(`{"nested": { "isNull": null }}`))
	assert.Regexp(t, `PD020021.*isNull`, err)

}
