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

package statestore

import (
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
)

func TestStoreRetrieveABISchema(t *testing.T) {

	ctx, ss, done := newTestStateStore(t)
	defer done()

	as, err := NewABISchema(ctx, &abi.Parameter{
		Type:         "tuple",
		Name:         "MyStruct",
		InternalType: "struct MyStruct",
		Components: abi.ParameterArray{
			{
				Name:    "field1",
				Type:    "uint256",
				Indexed: true,
			},
			{
				Name:    "field2",
				Type:    "string",
				Indexed: true,
			},
			{
				Name: "field3",
				Type: "bool",
			},
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, SchemaTypeABI, as.Persisted().Type)
	assert.Equal(t, "type=MyStruct(uint256 field1,string field2,bool field3),labels=[field1,field2]", as.Persisted().Signature)
	schemaHash := "0xfa09c5ccfdbd9fea4bbda7c565697c93cb3c27ffa3b1ae300070c41b7406d243"
	assert.Equal(t, schemaHash, as.Persisted().Hash.String())

	err = ss.PersistSchema(ctx, as)
	assert.NoError(t, err)

	// Second should succeed, but not do anything
	err = ss.PersistSchema(ctx, as)
	assert.NoError(t, err)

	getValidate := func() {
		as1, err := ss.GetSchema(ctx, MustParseHashID(schemaHash))
		assert.NoError(t, err)
		assert.NotNil(t, as1)
		as1Sig, err := as1.(*abiSchema).FullSignature(ctx)
		assert.NoError(t, err)
		assert.Equal(t, as1.Persisted().Signature, as1Sig)
	}

	// Get should be from the cache
	getValidate()

	// Next from the DB
	ss.abiSchemaCache.Delete(schemaHash)
	getValidate()

	// Again from the cache
	getValidate()

}
