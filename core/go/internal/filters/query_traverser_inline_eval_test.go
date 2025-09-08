// Copyright © 2021 Kaleido, Inc.
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

package filters

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var allTypesFieldMap = FieldMap{
	"stringField":  StringField("string_field"),
	"int64Field":   Int64Field("int65_field"),
	"boolField":    Int64BoolField("bool_field"),
	"int256Field":  Int256Field("int256_field"),
	"uint256Field": Uint256Field("uint256_field"),
}

type badTypeResolver string

func (r badTypeResolver) SQLColumn() string { return (string)(r) }

func (r badTypeResolver) SupportsLIKE() bool { return false }

func (r badTypeResolver) SQLValue(ctx context.Context, jsonValue pldtypes.RawJSON) (driver.Value, error) {
	// Return something the system cannot handle
	return map[bool]bool{false: true}, nil
}

func TestEvalQueryEquals(t *testing.T) {

	var qf *query.QueryJSON
	err := json.Unmarshal([]byte(`{
		"eq": [
			{"field": "stringField", "value": "test1"},
			{"field": "int64Field", "value": 22222},
			{"field": "boolField", "value": true},
			{"field": "int256Field", "value": 44444},
			{"field": "uint256Field", "value": 55555}
		],
	    "limit": 100,
		"sort": ["stringField"]
	}`), &qf)
	require.NoError(t, err)

	// Exact match, but with slightly different types for each
	match, err := EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.True(t, match)

	// String different
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test2"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// Int64 different
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"99999"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// Bool different
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"false"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// int256 different
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"99999"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// uint256 different
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"99999"`),
	})
	require.NoError(t, err)
	assert.False(t, match)
}

func TestEvalQueryNull(t *testing.T) {

	var qf *query.QueryJSON
	err := json.Unmarshal([]byte(`{
		"null": [
			{"field": "stringField"},
			{"field": "int64Field"},
			{"field": "boolField"},
			{"field": "int256Field"},
			{"field": "uint256Field"}
		]
	}`), &qf)
	require.NoError(t, err)

	// Test with the JSON null, which is equiv to nil
	match, err := EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`null`),
		"int64Field":   pldtypes.RawJSON(`null`),
		"boolField":    pldtypes.RawJSON(`null`),
		"int256Field":  pldtypes.RawJSON(`null`),
		"uint256Field": pldtypes.RawJSON(`null`),
	})
	require.NoError(t, err)
	assert.True(t, match)
	// Test with actual nil
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{})
	require.NoError(t, err)
	assert.True(t, match)

	// String different
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"something"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// Int64 different
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"int64Field": pldtypes.RawJSON(`"11111"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// Bool different
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"boolField": pldtypes.RawJSON(`"true"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// int256 different
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"int256Field": pldtypes.RawJSON(`"11111"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// uint256 different
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"uint256Field": pldtypes.RawJSON(`"11111"`),
	})
	require.NoError(t, err)
	assert.False(t, match)
}

func TestEvalQueryNotNull(t *testing.T) {

	var qf *query.QueryJSON
	err := json.Unmarshal([]byte(`{
		"null": [{"field": "stringField", "not": true}]
	}`), &qf)
	require.NoError(t, err)

	match, err := EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`null`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"any"`),
	})
	require.NoError(t, err)
	assert.True(t, match)

	_, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`12345`),
	})
	assert.Regexp(t, "PD010705", err)

}

func TestEvalQueryMatchStringCaseInsensitive(t *testing.T) {
	var qf *query.QueryJSON
	err := json.Unmarshal([]byte(`{"eq": [{"field": "stringField", "value": "test1", "caseInsensitive": true}]}`), &qf)
	require.NoError(t, err)
	match, err := EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"TesT1"`),
	})
	require.NoError(t, err)
	assert.True(t, match)
}

func TestEvalQueryMatchStringInvert(t *testing.T) {
	var qf *query.QueryJSON
	err := json.Unmarshal([]byte(`{"eq": [{"field": "stringField", "value": "test1", "not": true}]}`), &qf)
	require.NoError(t, err)

	match, err := EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"test1"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"anything else"`),
	})
	require.NoError(t, err)
	assert.True(t, match)
}

func TestEvalQueryInvalidValueTypes(t *testing.T) {
	var qf *query.QueryJSON
	err := json.Unmarshal([]byte(`{"eq": [
		{"field": "stringField", "value": "test1"},
		{"field": "int64Field", "value": 12345}
	]}`), &qf)
	require.NoError(t, err)

	match, err := EvalQuery(context.Background(), qf, allTypesFieldMap, PassthroughValueSet{
		"stringField": int64(12345),
	})
	assert.Regexp(t, "PD010713", err)
	assert.False(t, match)

	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, PassthroughValueSet{
		"int64Field": "test1",
	})
	assert.Regexp(t, "PD010713", err)
	assert.False(t, match)

	match, err = EvalQuery(context.Background(), qf, FieldMap{
		"stringField": badTypeResolver("wrong"),
		"int64Field":  badTypeResolver("wrong"),
	}, PassthroughValueSet{
		"stringField": "test1",
	})
	assert.Regexp(t, "PD010712", err)
	assert.False(t, match)

}

func TestEvalQueryMatchNullDoesNotMatch(t *testing.T) {

	// String test
	var qf *query.QueryJSON
	err := json.Unmarshal([]byte(`{"eq": [{"field": "stringField", "value": "test1"}]}`), &qf)
	require.NoError(t, err)
	match, err := EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{})
	require.NoError(t, err)
	assert.False(t, match)

	// int64 test
	err = json.Unmarshal([]byte(`{"eq": [{"field": "int64Field", "value": "12345"}]}`), &qf)
	require.NoError(t, err)
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{})
	require.NoError(t, err)
	assert.False(t, match)

	// bool test
	err = json.Unmarshal([]byte(`{"eq": [{"field": "boolField", "value": false}]}`), &qf)
	require.NoError(t, err)
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{})
	require.NoError(t, err)
	assert.False(t, match)

	// int256 test
	err = json.Unmarshal([]byte(`{"eq": [{"field": "int256Field", "value": "11223344"}]}`), &qf)
	require.NoError(t, err)
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{})
	require.NoError(t, err)
	assert.False(t, match)

	// uint256 test
	err = json.Unmarshal([]byte(`{"eq": [{"field": "uint256Field", "value": "-11223344"}]}`), &qf)
	require.NoError(t, err)
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{})
	require.NoError(t, err)
	assert.False(t, match)
}

func TestEvalQueryLessThan(t *testing.T) {

	var qf *query.QueryJSON
	err := json.Unmarshal([]byte(`{
		"lt": [
			{"field": "stringField", "value": "test1"},
			{"field": "int64Field", "value": 22222},
			{"field": "boolField", "value": true},
			{"field": "int256Field", "value": 44444},
			{"field": "uint256Field", "value": 55555}
		]
	}`), &qf)
	require.NoError(t, err)

	// Exact match, but with slightly different types for each
	match, err := EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test0"`),
		"int64Field":   pldtypes.RawJSON(`"11111"`),
		"boolField":    pldtypes.RawJSON(`"false"`),
		"int256Field":  pldtypes.RawJSON(`"33333"`),
		"uint256Field": pldtypes.RawJSON(`"0xD902"`),
	})
	require.NoError(t, err)
	assert.True(t, match)

	// string mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"11111"`),
		"boolField":    pldtypes.RawJSON(`"false"`),
		"int256Field":  pldtypes.RawJSON(`"33333"`),
		"uint256Field": pldtypes.RawJSON(`"0xD902"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// int64 mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test0"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"false"`),
		"int256Field":  pldtypes.RawJSON(`"33333"`),
		"uint256Field": pldtypes.RawJSON(`"0xD902"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// bool mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test0"`),
		"int64Field":   pldtypes.RawJSON(`"11111"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"33333"`),
		"uint256Field": pldtypes.RawJSON(`"0xD902"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// int256 mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test0"`),
		"int64Field":   pldtypes.RawJSON(`"11111"`),
		"boolField":    pldtypes.RawJSON(`"false"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"0xD902"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// uint256 mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test0"`),
		"int64Field":   pldtypes.RawJSON(`"11111"`),
		"boolField":    pldtypes.RawJSON(`"false"`),
		"int256Field":  pldtypes.RawJSON(`"33333"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.False(t, match)
}

func TestEvalQueryLessThanEqual(t *testing.T) {

	var qf *query.QueryJSON
	err := json.Unmarshal([]byte(`{
		"lte": [
			{"field": "stringField", "value": "test1"},
			{"field": "int64Field", "value": 22222},
			{"field": "boolField", "value": false},
			{"field": "int256Field", "value": 44444},
			{"field": "uint256Field", "value": 55555}
		]
	}`), &qf)
	require.NoError(t, err)

	// Exact match, but with slightly different types for each
	match, err := EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"false"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.True(t, match)

	// string mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test2"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"false"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// int64 mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"22223"`),
		"boolField":    pldtypes.RawJSON(`"false"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// bool mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// int256 mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"false"`),
		"int256Field":  pldtypes.RawJSON(`"44445"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// uint256 mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"false"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"0xD904"`),
	})
	require.NoError(t, err)
	assert.False(t, match)
}

func TestEvalQueryGreaterThan(t *testing.T) {

	var qf *query.QueryJSON
	err := json.Unmarshal([]byte(`{
		"gt": [
			{"field": "stringField", "value": "test1"},
			{"field": "int64Field", "value": 22222},
			{"field": "boolField", "value": false},
			{"field": "int256Field", "value": 44444},
			{"field": "uint256Field", "value": 55555}
		]
	}`), &qf)
	require.NoError(t, err)

	// Exact match, but with slightly different types for each
	match, err := EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test2"`),
		"int64Field":   pldtypes.RawJSON(`"22223"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"44445"`),
		"uint256Field": pldtypes.RawJSON(`"0xD904"`),
	})
	require.NoError(t, err)
	assert.True(t, match)

	// string mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"22223"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"44445"`),
		"uint256Field": pldtypes.RawJSON(`"0xD904"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// int64 mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test2"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"44445"`),
		"uint256Field": pldtypes.RawJSON(`"0xD904"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// bool mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test2"`),
		"int64Field":   pldtypes.RawJSON(`"22223"`),
		"boolField":    pldtypes.RawJSON(`"false"`),
		"int256Field":  pldtypes.RawJSON(`"44445"`),
		"uint256Field": pldtypes.RawJSON(`"0xD904"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// int256 mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test2"`),
		"int64Field":   pldtypes.RawJSON(`"22223"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"0xD904"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// uint256 mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test2"`),
		"int64Field":   pldtypes.RawJSON(`"22223"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"44445"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.False(t, match)
}

func TestEvalQueryGreaterThanOrEqual(t *testing.T) {

	var qf *query.QueryJSON
	err := json.Unmarshal([]byte(`{
		"gte": [
			{"field": "stringField", "value": "test1"},
			{"field": "int64Field", "value": 22222},
			{"field": "boolField", "value": true},
			{"field": "int256Field", "value": 44444},
			{"field": "uint256Field", "value": 55555}
		]
	}`), &qf)
	require.NoError(t, err)

	// Exact match, but with slightly different types for each
	match, err := EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.True(t, match)

	// string mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test0"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// int64 mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"22221"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// bool mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"false"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// int256 mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"44443"`),
		"uint256Field": pldtypes.RawJSON(`"0xD903"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// uint256 mismatch
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField":  pldtypes.RawJSON(`"test1"`),
		"int64Field":   pldtypes.RawJSON(`"22222"`),
		"boolField":    pldtypes.RawJSON(`"true"`),
		"int256Field":  pldtypes.RawJSON(`"44444"`),
		"uint256Field": pldtypes.RawJSON(`"0xD902"`),
	})
	require.NoError(t, err)
	assert.False(t, match)
}

func TestEvalQueryMatchLike(t *testing.T) {

	var qf *query.QueryJSON
	err := json.Unmarshal([]byte(`{"like": [{"field": "int64Field", "value": "111"}]}`), &qf)
	require.NoError(t, err)
	match, err := EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{})
	assert.Regexp(t, "PD010716", err)
	assert.False(t, match)

	err = json.Unmarshal([]byte(`{"like": [{"field": "stringField", "value": "hello%"}]}`), &qf)
	require.NoError(t, err)
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"hello world"`),
	})
	require.NoError(t, err)
	assert.True(t, match)
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"Hello world"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	err = json.Unmarshal([]byte(`{"like": [{"field": "stringField", "value": "%world%", "caseInsensitive": true}]}`), &qf)
	require.NoError(t, err)
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"Hello World"`),
	})
	require.NoError(t, err)
	assert.True(t, match)
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"Hello"`),
	})
	require.NoError(t, err)
	assert.False(t, match)
}

func TestEvalQueryLikeFail(t *testing.T) {

	eval := &inlineEval{
		inlineEvalRoot: &inlineEvalRoot{
			ctx: context.Background(),
			valueSet: ResolvingValueSet{
				"stringField": pldtypes.RawJSON(`"any"`),
			},
			convertLike: func(s string, caseInsensitive bool) (*regexp.Regexp, error) {
				return nil, fmt.Errorf("pop")
			},
		},
		matches: true,
	}
	res := eval.NewRoot().IsLike(&query.OpSingleVal{
		Op: query.Op{Field: "stringField"},
	}, "stringField", StringField("string_field"), "any")
	assert.Regexp(t, "PD010715", res.Error())

	res = eval.NewRoot()
	assert.False(t, res.T().int64LikeNotSupported(1, 1))
	assert.Regexp(t, "PD010714", res.Error())
}

func TestEvalQueryMatchIn(t *testing.T) {

	var qf *query.QueryJSON
	err := json.Unmarshal([]byte(`{"in": [{"field": "int64Field", "values": ["111","222"]}]}`), &qf)
	require.NoError(t, err)

	match, err := EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"int64Field": pldtypes.RawJSON(`111`),
	})
	require.NoError(t, err)
	assert.True(t, match)

	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"int64Field": pldtypes.RawJSON(`"0xDE"`),
	})
	require.NoError(t, err)
	assert.True(t, match)

	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"int64Field": pldtypes.RawJSON(`"333"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	err = json.Unmarshal([]byte(`{"in": [{"field": "stringField", "values": ["aaa","bbb"], "not": true}]}`), &qf)
	require.NoError(t, err)

	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"ccc"`),
	})
	require.NoError(t, err)
	assert.True(t, match)

	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"aaa"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	err = json.Unmarshal([]byte(`{"in": [{"field": "stringField", "values": ["aaa","bbb"], "not": true}]}`), &qf)
	require.NoError(t, err)

	_, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`false`),
	})
	assert.Regexp(t, "PD010705", err)

}

func TestEvalQueryAndOr(t *testing.T) {
	var qf *query.QueryJSON
	err := json.Unmarshal([]byte(`{
		"eq": [{"field": "stringField", "value": "test1", "caseInsensitive": true}],
		"or": [
		  {
		    "gt": [{"field": "int64Field", "value": 50}],
		    "lte": [{"field": "int64Field", "value": 100}]
		  },
		  {
		    "gt": [{"field": "int256Field", "value": 5000}],
		    "lte": [{"field": "int256Field", "value": 10000}]
		  }
		]
	}`), &qf)
	require.NoError(t, err)

	match, err := EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"TesT1"`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// Match the base AND match, and the int64 child
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"TesT1"`),
		"int64Field":  pldtypes.RawJSON(`100`),
	})
	require.NoError(t, err)
	assert.True(t, match)

	// Match the base AND match, and the int256 child
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"TesT1"`),
		"int256Field": pldtypes.RawJSON(`5001`),
	})
	require.NoError(t, err)
	assert.True(t, match)

	// Don't match the base requirement
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"test2"`),
		"int256Field": pldtypes.RawJSON(`5001`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// Don't match the either or criteria
	match, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"test1"`),
		"int64Field":  pldtypes.RawJSON(`50`),
		"int256Field": pldtypes.RawJSON(`5000`),
	})
	require.NoError(t, err)
	assert.False(t, match)

	// Roll up errors
	_, err = EvalQuery(context.Background(), qf, allTypesFieldMap, ResolvingValueSet{
		"stringField": pldtypes.RawJSON(`"test1"`),
		"int64Field":  pldtypes.RawJSON(`"wrong"`),
	})
	assert.Regexp(t, "PD010703", err)
}

func TestEvalQueryOrRollup(t *testing.T) {

	eval := &inlineEval{
		inlineEvalRoot: &inlineEvalRoot{
			ctx:      context.Background(),
			valueSet: ResolvingValueSet{},
		},
		matches: true,
	}
	res := eval.NewRoot().BuildOr(
		eval.NewRoot().WithError(fmt.Errorf("pop")).T(),
		eval.NewRoot().T(),
	)
	assert.Regexp(t, "pop", res.Error())

}
