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

package statemgr

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/eip712"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func testABIParam(t *testing.T, jsonParam string) *abi.Parameter {
	var a abi.Parameter
	err := json.Unmarshal([]byte(jsonParam), &a)
	require.NoError(t, err)
	return &a
}

func mockDomain(t *testing.T, m *mockComponents, name string, customHashFunction bool) *componentsmocks.Domain {
	md := componentsmocks.NewDomain(t)
	md.On("Name").Return(name).Maybe()
	md.On("CustomHashFunction").Return(customHashFunction)
	m.domainManager.On("GetDomainByName", mock.Anything, name).Return(md, nil)
	return md
}

func mockStateCallback(m *mockComponents) {
	m.txManager.On("NotifyStatesDBChanged", mock.Anything).Return()
}

// This is an E2E test using the actual database, the flush-writer DB storage system, and the schema cache
func TestStoreRetrieveABISchema(t *testing.T) {

	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	_ = mockDomain(t, m, "domain1", false)
	mockStateCallback(m)

	as, err := newABISchema(ctx, "domain1", &abi.Parameter{
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
				Name:    "field8",
				Type:    "uint32",
				Indexed: true,
			},
			{
				Name: "field9",
				Type: "string",
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, pldapi.SchemaTypeABI, as.Persisted().Type.V())
	assert.Equal(t, pldapi.SchemaTypeABI, as.Type())
	assert.NotNil(t, as.definition)
	assert.Equal(t, "type=MyStruct(uint256 field1,string field2,int64 field3,bool field4,address field5,int256 field6,bytes field7,uint32 field8,string field9),labels=[field1,field2,field3,field4,field5,field6,field7,field8]", as.Persisted().Signature)
	cacheKey := "domain1/0xcf41493c8bb9652d1483ee6cb5122efbec6fbdf67cc27363ba5b030b59244cad"
	assert.Equal(t, cacheKey, schemaCacheKey(as.Persisted().DomainName, as.Persisted().ID))

	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{as.Schema})
	require.NoError(t, err)
	schemaID := as.Persisted().ID
	contractAddress := pldtypes.RandAddress()

	// Check it handles data
	var states []*pldapi.State
	err = ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		states, err = ss.WriteReceivedStates(ctx, dbTX, "domain1", []*components.StateUpsertOutsideContext{
			{
				ID:       nil, // default hashing algo
				SchemaID: schemaID,
				Data: pldtypes.RawJSON(`{
					"field1": "0x0123456789012345678901234567890123456789",
					"field2": "hello world",
					"field3": 42,
					"field4": true,
					"field5": "0x687414C0B8B4182B823Aec5436965cf19b197386",
					"field6": "-10203040506070809",
					"field7": "0xfeedbeef",
					"field8": 12345,
					"field9": "things and stuff",
					"cruft": "to remove"
				}`),
				ContractAddress: contractAddress,
			},
		})
		return err
	})
	require.NoError(t, err)

	state1 := states[0]
	assert.Equal(t, []*pldapi.StateLabel{
		// uint256 written as zero padded string
		{DomainName: "domain1", State: state1.ID, Label: "field1", Value: "0000000000000000000000000123456789012345678901234567890123456789"},
		// string written as it is
		{DomainName: "domain1", State: state1.ID, Label: "field2", Value: "hello world"},
		// address is really a uint160, so that's how we handle it
		{DomainName: "domain1", State: state1.ID, Label: "field5", Value: "000000000000000000000000687414c0b8b4182b823aec5436965cf19b197386"},
		// int256 needs an extra byte ahead of the zero-padded string to say it's negative,
		// and is two's complement for that negative number so less negative number are string "higher"
		{DomainName: "domain1", State: state1.ID, Label: "field6", Value: "0ffffffffffffffffffffffffffffffffffffffffffffffffffdbc0638301b8e7"},
		// bytes are just bytes
		{DomainName: "domain1", State: state1.ID, Label: "field7", Value: "feedbeef"},
	}, state1.Labels)
	assert.Equal(t, []*pldapi.StateInt64Label{
		// int64 can just be stored directly in a numeric index
		{DomainName: "domain1", State: state1.ID, Label: "field3", Value: 42},
		// bool also gets an efficient numeric index - we don't attempt to allocate anything smaller than int64 to this
		{DomainName: "domain1", State: state1.ID, Label: "field4", Value: 1},
		// uint32 also
		{DomainName: "domain1", State: state1.ID, Label: "field8", Value: 12345},
	}, state1.Int64Labels)
	assert.Equal(t, "0x90c1f63e32a708ef59b3708c57d165a87bddf758709313c57448e85a10c59544", state1.ID.String())

	// Check we get all the data in the canonical format, with the cruft removed
	assert.JSONEq(t, `{
		"field1": "6495562831695638750381182724034531561381914505",
		"field2": "hello world",
		"field3": "42",
		"field4": true,
		"field5": "0x687414c0b8b4182b823aec5436965cf19b197386",
		"field6": "-10203040506070809",
		"field7": "0xfeedbeef",
		"field8": "12345",
		"field9": "things and stuff"
	}`, string(state1.Data))

	// Second should succeed, but not do anything
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{as.Schema})
	require.NoError(t, err)
	schemaID = as.ID()

	getValidate := func() {
		as1, err := ss.getSchemaByID(ctx, ss.p.NOTX(), as.Persisted().DomainName, schemaID, true)
		require.NoError(t, err)
		assert.NotNil(t, as1)
		as1Sig, err := as1.(*abiSchema).FullSignature(ctx)
		require.NoError(t, err)
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
	statesQuery, err := ss.GetStatesByID(ctx, ss.p.NOTX(), as.Persisted().DomainName, contractAddress, []pldtypes.HexBytes{state1.ID}, true, true)
	require.NoError(t, err)
	assert.Equal(t, state1, statesQuery[0])

	// Do a query on just one state, based on all the label fields
	var query *query.QueryJSON
	err = json.Unmarshal(([]byte)(`{
		"eq": [
		  {"field":"field1","value":"0x0123456789012345678901234567890123456789"},
		  {"field":"field2","value":"hello world"},
		  {"field":"field3","value":42},
		  {"field":"field4","value":true},
		  {"field":"field5","value":"0x687414C0B8B4182B823Aec5436965cf19b197386"},
		  {"field":"field6","value":"-10203040506070809"},
		  {"field":"field7","value":"0xfeedbeef"},
		  {"field":"field8","value":12345}
		]
	}`), &query)
	require.NoError(t, err)
	states, err = ss.FindContractStates(ctx, ss.p.NOTX(), as.Persisted().DomainName, contractAddress, schemaID, query, "all")
	require.NoError(t, err)
	assert.Len(t, states, 1)

	// Do a query that should fail on a string based label
	err = json.Unmarshal(([]byte)(`{
		"eq": [
		  {"field":"field2","value":"hello sun"}
		]
	}`), &query)
	require.NoError(t, err)
	states, err = ss.FindContractStates(ctx, ss.p.NOTX(), as.Persisted().DomainName, contractAddress, schemaID, query, "all")
	require.NoError(t, err)
	assert.Len(t, states, 0)

	// Do a query that should fail on an integer base label
	err = json.Unmarshal(([]byte)(`{
		"eq": [
		  {"field":"field3","value":43}
		]
	}`), &query)
	require.NoError(t, err)
	states, err = ss.FindContractStates(ctx, ss.p.NOTX(), as.Persisted().DomainName, contractAddress, schemaID, query, "all")
	require.NoError(t, err)
	assert.Len(t, states, 0)
}

func TestNewABISchemaInvalidTypedDataType(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	_, err := newABISchema(ctx, "domain1", &abi.Parameter{
		Type:         "tuple",
		Name:         "MyStruct",
		InternalType: "struct MyStruct",
		Components: abi.ParameterArray{
			{
				Name: "field1",
				Type: "function",
			},
		},
	})
	assert.Regexp(t, "FF22072", err)

}

func TestGetSchemaInvalidJSON(t *testing.T) {
	ctx, ss, mdb, _, done := newDBMockStateManager(t)
	defer done()

	mdb.ExpectQuery("SELECT.*schemas").WillReturnRows(sqlmock.NewRows(
		[]string{"type", "content"},
	).AddRow(pldapi.SchemaTypeABI, "!!! { bad json"))

	_, err := ss.GetSchemaByID(ctx, ss.p.NOTX(), "domain1", pldtypes.Bytes32Keccak(([]byte)("test")), true)
	assert.Regexp(t, "PD010113", err)
}

func TestRestoreABISchemaInvalidType(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	_, err := newABISchemaFromDB(ctx, &pldapi.Schema{
		Definition: pldtypes.RawJSON(`{}`),
	})
	assert.Regexp(t, "PD010114", err)

}

func TestRestoreABISchemaInvalidTypeTree(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	_, err := newABISchemaFromDB(ctx, &pldapi.Schema{
		Definition: pldtypes.RawJSON(`{"type":"tuple","internalType":"struct MyType","components":[{"type":"wrong"}]}`),
	})
	assert.Regexp(t, "FF22025.*wrong", err)

}

func TestABILabelSetupMissingName(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	_, err := newABISchema(ctx, "domain1", &abi.Parameter{
		Type:         "tuple",
		Name:         "MyStruct",
		InternalType: "struct MyStruct",
		Components: abi.ParameterArray{
			{
				Indexed: true,
				Type:    "uint256",
			},
		},
	})
	assert.Regexp(t, "PD010108", err)

}

func TestABILabelSetupBadTree(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	_, err := newABISchema(ctx, "domain1", &abi.Parameter{
		Type:         "tuple",
		Name:         "MyStruct",
		InternalType: "struct MyStruct",
		Components: abi.ParameterArray{
			{
				Indexed: true,
				Name:    "broken",
			},
		},
	})
	assert.Regexp(t, "FF22025", err)

}

func TestABILabelSetupDuplicateField(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	_, err := newABISchema(ctx, "domain1", &abi.Parameter{
		Type:         "tuple",
		Name:         "MyStruct",
		InternalType: "struct MyStruct",
		Components: abi.ParameterArray{
			{
				Indexed: true,
				Name:    "field1",
				Type:    "uint256",
			},
			{
				Indexed: true,
				Name:    "field1",
				Type:    "uint256",
			},
		},
	})
	assert.Regexp(t, "PD010115", err)
}

func TestABILabelSetupUnsupportedType(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	_, err := newABISchema(ctx, "domain1", &abi.Parameter{
		Type:         "tuple",
		Name:         "MyStruct",
		InternalType: "struct MyStruct",
		Components: abi.ParameterArray{
			{
				Indexed:      true,
				Name:         "nested",
				InternalType: "struct MyNested",
				Type:         "tuple",
				Components:   abi.ParameterArray{},
			},
		},
	})
	assert.Regexp(t, "PD010107", err)
}

func TestABISchemaGetLabelTypeBadType(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	as := &abiSchema{
		definition: &abi.Parameter{
			Type:         "tuple",
			Name:         "MyStruct",
			InternalType: "struct MyStruct",
			Components: abi.ParameterArray{
				{
					Indexed:    true,
					Type:       "fixed",
					Components: abi.ParameterArray{},
				},
			},
		},
	}
	tc, err := as.definition.TypeComponentTree()
	require.NoError(t, err)

	_, err = as.getLabelType(ctx, "f1", tc.TupleChildren()[0])
	assert.Regexp(t, "PD010103", err)
}

func TestABISchemaProcessStateInvalidType(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	as := &abiSchema{
		Schema: &pldapi.Schema{
			Labels: []string{"field1"},
		},
		definition: &abi.Parameter{
			Type:         "tuple",
			Name:         "MyStruct",
			InternalType: "struct MyStruct",
			Components: abi.ParameterArray{
				{
					Indexed: true,
					Type:    "fixed",
					Name:    "field1",
				},
			},
		},
		primaryType: "MyStruct",
		typeSet: eip712.TypeSet{
			"MyStruct": eip712.Type{
				{
					Name: "field1",
					Type: "uint256",
				},
			},
		},
	}
	var err error
	as.tc, err = as.definition.TypeComponentTreeCtx(ctx)
	require.NoError(t, err)
	_, err = as.ProcessState(ctx, pldtypes.RandAddress(), pldtypes.RawJSON(`{"field1": 12345}`), nil, false)
	assert.Regexp(t, "PD010103", err)
}

func TestABISchemaProcessStateLabelMissing(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	as := &abiSchema{
		Schema: &pldapi.Schema{
			Labels: []string{"field1"},
		},
		definition: &abi.Parameter{
			Type:         "tuple",
			Name:         "MyStruct",
			InternalType: "struct MyStruct",
			Components:   abi.ParameterArray{},
		},
		primaryType: "MyStruct",
		typeSet: eip712.TypeSet{
			"MyStruct": eip712.Type{
				{
					Name: "field1",
					Type: "uint256",
				},
			},
		},
	}
	var err error
	as.tc, err = as.definition.TypeComponentTreeCtx(ctx)
	require.NoError(t, err)
	_, err = as.ProcessState(ctx, pldtypes.RandAddress(), pldtypes.RawJSON(`{"field1": 12345}`), nil, false)
	assert.Regexp(t, "PD010110", err)
}

func TestABISchemaProcessStateBadDefinition(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	as := &abiSchema{

		definition: &abi.Parameter{},
	}
	_, err := as.definition.TypeComponentTreeCtx(ctx)
	assert.Regexp(t, "FF22025", err)
}

func TestABISchemaProcessStateBadValue(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	as := &abiSchema{
		Schema: &pldapi.Schema{
			Labels: []string{"field1"},
		},
		definition: &abi.Parameter{
			Type:         "tuple",
			Name:         "MyStruct",
			InternalType: "struct MyStruct",
			Components:   abi.ParameterArray{},
		},
	}
	var err error
	as.tc, err = as.definition.TypeComponentTreeCtx(ctx)
	require.NoError(t, err)
	_, err = as.ProcessState(ctx, pldtypes.RandAddress(), pldtypes.RawJSON(`{!!! wrong`), nil, false)
	assert.Regexp(t, "PD010116", err)
}

func TestABISchemaProcessStateMismatchValue(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	as := &abiSchema{
		Schema: &pldapi.Schema{
			Labels: []string{"field1"},
		},
		definition: &abi.Parameter{
			Type:         "tuple",
			Name:         "MyStruct",
			InternalType: "struct MyStruct",
			Components: abi.ParameterArray{
				{Name: "field1", Type: "uint256"},
			},
		},
	}
	var err error
	as.tc, err = as.definition.TypeComponentTreeCtx(ctx)
	require.NoError(t, err)
	_, err = as.ProcessState(ctx, pldtypes.RandAddress(), pldtypes.RawJSON(`{"field1":{}}`), nil, false)
	assert.Regexp(t, "FF22030", err)
}

func TestABISchemaProcessStateEIP712Failure(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	as := &abiSchema{
		Schema: &pldapi.Schema{
			Labels: []string{"field1"},
		},
		definition: &abi.Parameter{
			Type:         "tuple",
			Name:         "MyStruct",
			InternalType: "struct MyStruct",
			Components: abi.ParameterArray{
				{Name: "field1", Type: "function"},
			},
		},
	}
	var err error
	as.tc, err = as.definition.TypeComponentTreeCtx(ctx)
	require.NoError(t, err)
	_, err = as.ProcessState(ctx, pldtypes.RandAddress(), pldtypes.RawJSON(`{"field1":"0x753A7decf94E48a05Fa1B342D8984acA9bFaf6B2"}`), nil, false)
	assert.Regexp(t, "FF22073", err)
}

func TestABISchemaProcessStateDataFailure(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	as := &abiSchema{
		Schema: &pldapi.Schema{
			Labels: []string{"field1"},
		},
		definition: &abi.Parameter{
			Type:         "tuple",
			Name:         "MyStruct",
			InternalType: "struct MyStruct",
			Components: abi.ParameterArray{
				{Name: "field1", Type: "function"},
			},
		},
	}
	var err error
	as.tc, err = as.definition.TypeComponentTreeCtx(ctx)
	require.NoError(t, err)
	_, err = as.ProcessState(ctx, pldtypes.RandAddress(), pldtypes.RawJSON(`{"field1":"0x753A7decf94E48a05Fa1B342D8984acA9bFaf6B2"}`), nil, false)
	assert.Regexp(t, "FF22073", err)
}

func TestABISchemaMapLabelResolverBadType(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	as := &abiSchema{
		Schema: &pldapi.Schema{
			Labels: []string{"field1"},
		},
		definition: &abi.Parameter{
			Type:         "tuple",
			Name:         "MyStruct",
			InternalType: "struct MyStruct",
			Components: abi.ParameterArray{
				{Name: "field1", Type: "function"},
			},
		},
	}
	_, _, err := as.mapLabelResolver(ctx, "", -1)
	assert.Regexp(t, "PD010103", err)
}

func TestABISchemaInsertCustomHashNoID(t *testing.T) {

	as := &abiSchema{
		Schema:     &pldapi.Schema{},
		definition: &abi.Parameter{Components: abi.ParameterArray{}},
	}
	tc, err := as.definition.Components.TypeComponentTree()
	require.NoError(t, err)
	as.tc = tc
	_, err = as.ProcessState(context.Background(), pldtypes.RandAddress(), pldtypes.RawJSON(`{}`), nil, true)
	assert.Regexp(t, "PD010130", err)
}

func TestABISchemaInsertStandardHashMismatch(t *testing.T) {
	as, err := newABISchema(context.Background(), "domain1", &abi.Parameter{
		Type:         "tuple",
		Name:         "MyStruct",
		InternalType: "struct MyStruct",
		Components:   abi.ParameterArray{},
	})
	require.NoError(t, err)
	_, err = as.ProcessState(context.Background(), pldtypes.RandAddress(),
		pldtypes.RawJSON(`{}`), pldtypes.RandBytes(32), false)
	assert.Regexp(t, "PD010129", err)
}

func TestABISchemaInsertCustomHashBadData(t *testing.T) {
	as := &abiSchema{
		Schema: &pldapi.Schema{},
		definition: &abi.Parameter{Components: abi.ParameterArray{
			{Type: "uint256", Name: "field1"},
		}},
	}
	tc, err := as.definition.Components.TypeComponentTree()
	require.NoError(t, err)
	as.tc = tc
	_, err = as.ProcessState(context.Background(), pldtypes.RandAddress(), pldtypes.RawJSON(`{}`), pldtypes.RandBytes(32), false)
	assert.Regexp(t, "FF22040", err)
}

func TestABISchemaMapValueToLabelTypeErrors(t *testing.T) {

	ctx, _, _, _, done := newDBMockStateManager(t)
	defer done()

	as := &abiSchema{
		Schema: &pldapi.Schema{
			Labels: []string{"field1"},
		},
		definition: &abi.Parameter{
			Components: abi.ParameterArray{
				{Name: "field1", Type: "function"},
				{Name: "field2", Type: "uint256"},
			},
		},
	}
	tc, err := as.definition.Components[0].TypeComponentTree()
	require.NoError(t, err)
	cv, err := tc.ParseExternal("0x753A7decf94E48a05Fa1B342D8984acA9bFaf6B2")
	require.NoError(t, err)

	// bad type
	_, _, err = as.mapValueToLabel(ctx, "", -1, cv)
	assert.Regexp(t, "PD010103", err)

	// int64
	_, _, err = as.mapValueToLabel(ctx, "", labelTypeInt64, cv)
	assert.Regexp(t, "PD010109", err)

	// int256
	_, _, err = as.mapValueToLabel(ctx, "", labelTypeInt256, cv)
	assert.Regexp(t, "PD010109", err)

	// uint256
	_, _, err = as.mapValueToLabel(ctx, "", labelTypeUint256, cv)
	assert.Regexp(t, "PD010109", err)

	// string
	_, _, err = as.mapValueToLabel(ctx, "", labelTypeString, cv)
	assert.Regexp(t, "PD010109", err)

	// bool
	_, _, err = as.mapValueToLabel(ctx, "", labelTypeBool, cv)
	assert.Regexp(t, "PD010109", err)

	tc, err = as.definition.Components[1].TypeComponentTree()
	require.NoError(t, err)
	cv, err = tc.ParseExternal("0x12345")
	require.NoError(t, err)

	// bytes
	_, _, err = as.mapValueToLabel(ctx, "", labelTypeBytes, cv)
	assert.Regexp(t, "PD010109", err)

}
