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

// This is an E2E test using the actual database, the flush-writer DB storage system, and the schema cache
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
				Type:    "uint256", // too big for an integer label, gets a 64 char hex string
				Indexed: true,
			},
			{
				Name:    "field2",
				Type:    "string",
				Indexed: true,
			},
			{
				Name:    "field3",
				Type:    "int64", // fits as an integer label
				Indexed: true,
			},
			{
				Name:    "field4",
				Type:    "bool",
				Indexed: true,
			},
			{
				Name:    "field5",
				Type:    "address",
				Indexed: true,
			},
			{
				Name:    "field6",
				Type:    "int256",
				Indexed: true,
			},
			{
				Name:    "field7",
				Type:    "bytes",
				Indexed: true,
			},
			{
				Name: "field8",
				Type: "string",
			},
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, SchemaTypeABI, as.Persisted().Type)
	assert.Equal(t, "type=MyStruct(uint256 field1,string field2,int64 field3,bool field4,address field5,int256 field6,bytes field7,string field8),labels=[field1,field2,field3,field4,field5,field6,field7]", as.Persisted().Signature)
	cacheKey := "domain1/0xf2fe6e1d0405d9607cf291dd0c24ce40f01b8cf9d84e48664aea4785c0e28926"
	assert.Equal(t, cacheKey, schemaCacheKey(as.Persisted().DomainID, &as.Persisted().Hash))

	err = ss.PersistSchema(ctx, as)
	assert.NoError(t, err)

	// Check it handles data
	state1 := &State{
		Schema:   as.Persisted().Hash,
		DomainID: "domain1",
		Data: `{
			"field1": "0x0123456789012345678901234567890123456789",
			"field2": "hello world",
			"field3": 42,
			"field4": true,
			"field5": "0x687414C0B8B4182B823Aec5436965cf19b197386",
			"field6": "10203040506070809",
			"field7": "0xfeedbeef",
			"field8": "things and stuff"
		}`,
	}
	err = ss.PersistState(ctx, state1)
	assert.NoError(t, err)
	assert.NoError(t, err)
	assert.Equal(t, []*StateLabel{
		{State: state1.Hash, Label: "field1", Value: "0000000000000000000000000123456789012345678901234567890123456789"},
		{State: state1.Hash, Label: "field2", Value: "hello world"},
		{State: state1.Hash, Label: "field5", Value: "000000000000000000000000687414c0b8b4182b823aec5436965cf19b197386"},
		{State: state1.Hash, Label: "field6", Value: "100000000000000000000000000000000000000000000000000243f9c7cfe4719"},
		{State: state1.Hash, Label: "field7", Value: "feedbeef"},
	}, state1.Labels)
	assert.Equal(t, []*StateInt64Label{
		{State: state1.Hash, Label: "field3", Value: 42},
		{State: state1.Hash, Label: "field4", Value: 1},
	}, state1.Int64Labels)
	assert.Equal(t, "0x67c4953ad84c34fef7efacd3b3642e11278bed482efa5ff92d35b2306084f15a", state1.Hash.String())

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
