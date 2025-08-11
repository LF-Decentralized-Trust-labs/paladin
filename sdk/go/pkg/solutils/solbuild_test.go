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

package solutils

import (
	"context"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLibraryLinkingSuccess(t *testing.T) {

	unlinkedBuildJSON := []byte(`{
		"abi": [],
		"bytecode": 	"0x018560800151815260200160018152509050__$84884a75d90cd75cf84106ee1c9b688bee$__6325cc70e8826040518263ffffffff1660e01b8152600401611d42919060__$c53acb61cae20e338b64554b1aeff74f64$__6329a5f2f66040518060400160405280866020015181526020",
		"linkReferences": {
			"contracts/lib/MyContract.sol": {
				"Dependency1": [
					{
					"length": 20,
					"start": 18
					}
				],
				"Dependency2": [
					{
					"length": 20,
					"start": 68
					}
				]
			}
		}
	}`)

	resolvedAddresses := map[string]*pldtypes.EthAddress{
		"Dependency1": pldtypes.RandAddress(),
		"contracts/lib/MyContract.sol:Dependency2": pldtypes.RandAddress(),
	}

	build, err := LoadBuildResolveLinks(context.Background(), unlinkedBuildJSON, resolvedAddresses)
	require.NoError(t, err)
	require.Contains(t, build.Bytecode.String(), resolvedAddresses["Dependency1"].HexString())
	require.Contains(t, build.Bytecode.String(), resolvedAddresses["contracts/lib/MyContract.sol:Dependency2"].HexString())

	_, err = LoadBuild(context.Background(), unlinkedBuildJSON)
	require.Regexp(t, "PD021001", err) // missing links

	assert.Panics(t, func() {
		_ = MustLoadBuild(unlinkedBuildJSON)
	})

	_ = MustLoadBuild([]byte(`{
		"abi": [],
		"bytecode": 	"0x018560800151815260200160018152509050"
	}`))

	unlinkedBuildJSON = []byte(`{
		"abi": [],
		"bytecode": 	"0x018560800151815260200160018152509050__$0102030405060708aabbccddeeff998877$__",
		"linkReferences": {
			"contracts/lib/MyContract.sol": {
				"Dependency1": [
					{
					"length": 20,
					"start": 18
					}
				]
			}
		}
	}`)
	_, err = LoadBuildResolveLinks(context.Background(), unlinkedBuildJSON, resolvedAddresses)
	require.Regexp(t, "PD021000.*0102030405060708aabbccddeeff998877", err) // bad links

}

func TestMustParseBuildABI(t *testing.T) {
	abi := MustParseBuildABI([]byte(`{
	"abi": [
		{
			"type":"function",
			"name":"foo",
			"inputs":[
				{
					"name":"a",
					"type":"uint256"
				}
			]
		}
	]}`))
	assert.Equal(t, 1, len(abi.Functions()))
	assert.Equal(t, "foo", abi.Functions()["foo"].Name)

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()

	MustParseBuildABI([]byte(`{
		"abi": [
			{
				"type":"function",
				"name":"foo",
				"inputs":[
					{
						"name":"a",
						"type":"uint256"
					}
				
			}
		]}`))
}
