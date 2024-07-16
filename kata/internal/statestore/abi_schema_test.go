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

	ctx, ss, done := newDBTestStateStore(t)
	defer done()

	as, err := NewABISchema(ctx, "domain1", &abi.Parameter{
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
	cacheKey := "domain1/0xfa09c5ccfdbd9fea4bbda7c565697c93cb3c27ffa3b1ae300070c41b7406d243"
	assert.Equal(t, cacheKey, schemaCacheKey(as.Persisted().DomainID, &as.Persisted().Hash))

	err = ss.PersistSchema(ctx, as)
	assert.NoError(t, err)

	// Check it handles data
	state1 := &State{
		Schema:   as.Persisted().Hash,
		DomainID: "domain1",
		Data:     `{"field1": 12345, "field2": "hello world", "field3": false}`,
	}
	err = ss.PersistState(ctx, state1)
	assert.NoError(t, err)
	assert.NoError(t, err)
	assert.Equal(t, []StateLabel{
		{State: state1.Hash, Label: "field1", Value: "0000000000000000000000000000000000000000000000000000000000003039"},
		{State: state1.Hash, Label: "field2", Value: "hello world"},
	}, state1.Labels)
	assert.Equal(t, "0x70f5850c0e7f3eeec9a4fd279f64f0e16123e179b42b0cadf31926ab7656d521", state1.Hash.String())

	// Second should succeed, but not do anything
	err = ss.PersistSchema(ctx, as)
	assert.NoError(t, err)

	getValidate := func() {
		as1, err := ss.GetSchema(ctx, as.Persisted().DomainID, &as.Persisted().Hash)
		assert.NoError(t, err)
		assert.NotNil(t, as1)
		as1Sig, err := as1.(*abiSchema).FullSignature(ctx)
		assert.NoError(t, err)
		assert.Equal(t, as1.Persisted().Signature, as1Sig)
	}

	// Get should be from the cache
	getValidate()

	// Next from the DB
	ss.abiSchemaCache.Delete(cacheKey)
	getValidate()

	// Again from the cache
	getValidate()

	// Get the state back too
	state1a, err := ss.GetState(ctx, as.Persisted().DomainID, &state1.Hash, true)
	assert.NoError(t, err)
	assert.Equal(t, state1, state1a)
}
