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

package filters

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testValuesResolved struct {
	vs ResolvingValueSet
}

func (v *testValuesResolved) ValueSet() ValueSet {
	return v.vs
}

func (v *testValuesResolved) MarshalJSON() ([]byte, error) {
	return json.Marshal((map[string]pldtypes.RawJSON)(v.vs))
}

type testValuesPassthrough struct {
	vs PassthroughValueSet
}

func (v *testValuesPassthrough) ValueSet() ValueSet {
	return v.vs
}

func TestValueSetSorter2D(t *testing.T) {

	values := []*testValuesResolved{
		{vs: ResolvingValueSet{"field1": pldtypes.RawJSON(`"MMM"`), "field2": pldtypes.RawJSON("500")}},
		{vs: ResolvingValueSet{"field1": pldtypes.RawJSON(`"EEE"`), "field2": pldtypes.RawJSON("600")}},
		{vs: ResolvingValueSet{"field1": pldtypes.RawJSON(`"AAA"`), "field2": pldtypes.RawJSON("300")}},
		{vs: ResolvingValueSet{"field1": pldtypes.RawJSON(`"aaa"`), "field2": pldtypes.RawJSON("300")}},
		{vs: ResolvingValueSet{"field1": pldtypes.RawJSON(`"TTT"`), "field2": pldtypes.RawJSON("600")}},
		{vs: ResolvingValueSet{"field1": pldtypes.RawJSON(`"EEE"`), "field2": pldtypes.RawJSON("100")}},
		{vs: ResolvingValueSet{"field1": pldtypes.RawJSON(`"EEE"`), "field2": pldtypes.RawJSON("600")}},
	}

	fieldSet := FieldMap{"field1": StringField("field_1"), "field2": Int64Field("field_2")}

	ctx := context.Background()
	sorted, err := SortedValueSetCopy(ctx, fieldSet, values, "field1", "-field2")
	require.NoError(t, err)

	resJSON, err := json.MarshalIndent(sorted, "", "  ")
	require.NoError(t, err)
	assert.JSONEq(t, `[
	  {"field1": "AAA", "field2": 300},
	  {"field1": "EEE", "field2": 600},
	  {"field1": "EEE", "field2": 600},
	  {"field1": "EEE", "field2": 100},
	  {"field1": "MMM", "field2": 500},
	  {"field1": "TTT", "field2": 600},
	  {"field1": "aaa", "field2": 300}
	]`, string(resJSON))
	assert.NotEqual(t, values, sorted)

	err = SortValueSetInPlace(ctx, fieldSet, sorted, "field2", "field1")
	require.NoError(t, err)

	resJSON, err = json.MarshalIndent(sorted, "", "  ")
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"field1": "EEE", "field2": 100},
		{"field1": "AAA", "field2": 300},
		{"field1": "aaa", "field2": 300},
		{"field1": "MMM", "field2": 500},
		{"field1": "EEE", "field2": 600},
		{"field1": "EEE", "field2": 600},
		{"field1": "TTT", "field2": 600}
	  ]`, string(resJSON))
	assert.NotEqual(t, values, sorted)

}

func TestValueSetSorterBadSortField(t *testing.T) {
	_, err := SortedValueSetCopy(context.Background(), FieldMap{}, []*testValuesResolved{}, "wrong")
	assert.Regexp(t, "PD010700", err)

	err = SortValueSetInPlace(context.Background(), FieldMap{}, []*testValuesResolved{}, "wrong")
	assert.Regexp(t, "PD010700", err)
}

func TestValueSetSorterMissingSortField(t *testing.T) {
	_, err := SortedValueSetCopy(context.Background(), FieldMap{}, []*testValuesResolved{})
	assert.Regexp(t, "PD010718", err)
}

func TestValueSetSorterBadValue(t *testing.T) {
	values := []*testValuesResolved{
		{vs: ResolvingValueSet{"field1": pldtypes.RawJSON(`100`)}},
		{vs: ResolvingValueSet{"field1": pldtypes.RawJSON(`"wrong"`)}},
		{vs: ResolvingValueSet{"field1": pldtypes.RawJSON("500")}},
	}
	_, err := SortedValueSetCopy(context.Background(), FieldMap{"field1": Int64Field("field_1")}, values, "field1")
	assert.Regexp(t, "PD010703", err)

	_, err = SortedValueSetCopy(context.Background(), FieldMap{"field1": Int64Field("field_1")}, values, "-field1")
	assert.Regexp(t, "PD010703", err)
}

func TestValueSetSorterMixValue(t *testing.T) {
	values := []*testValuesPassthrough{
		{vs: PassthroughValueSet{"field1": "500"}},
		{vs: PassthroughValueSet{"field1": int64(100)}},
		{vs: PassthroughValueSet{"field1": "200"}},
	}
	_, err := SortedValueSetCopy(context.Background(), FieldMap{"field1": Int64Field("field_1")}, values, "field1")
	assert.Regexp(t, "PD010717", err)

	_, err = SortedValueSetCopy(context.Background(), FieldMap{"field1": Int64Field("field_1")}, values, "-field1")
	assert.Regexp(t, "PD010717", err)
}

func TestValueSetSorterUnsupportedValue(t *testing.T) {
	values := []*testValuesPassthrough{
		{vs: PassthroughValueSet{"field1": false}},
		{vs: PassthroughValueSet{"field1": true}},
	}
	_, err := SortedValueSetCopy(context.Background(), FieldMap{"field1": Int64Field("field_1")}, values, "field1")
	assert.Regexp(t, "PD010717", err)
}
