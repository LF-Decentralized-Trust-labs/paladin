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
	"fmt"
	"testing"

	"github.com/kaleido-io/paladin/kata/internal/types"
	"github.com/stretchr/testify/assert"
)

type testValues struct {
	vs ResolvingValueSet
}

func (v *testValues) ValueSet() ValueSet {
	return v.vs
}

func (v *testValues) MarshalJSON() ([]byte, error) {
	return json.Marshal((map[string]types.RawJSON)(v.vs))
}

func TestSort1(t *testing.T) {

	var qf *QueryJSON
	err := json.Unmarshal([]byte(`{
		"sort": ["field1","-field2"]
	}`), &qf)
	assert.NoError(t, err)

	values := []*testValues{
		{vs: ResolvingValueSet{"field1": types.RawJSON(`"MMM"`), "field2": types.RawJSON("500")}},
		{vs: ResolvingValueSet{"field1": types.RawJSON(`"EEE"`), "field2": types.RawJSON("600")}},
		{vs: ResolvingValueSet{"field1": types.RawJSON(`"AAA"`), "field2": types.RawJSON("300")}},
		{vs: ResolvingValueSet{"field1": types.RawJSON(`"aaa"`), "field2": types.RawJSON("300")}},
		{vs: ResolvingValueSet{"field1": types.RawJSON(`"TTT"`), "field2": types.RawJSON("600")}},
		{vs: ResolvingValueSet{"field1": types.RawJSON(`"EEE"`), "field2": types.RawJSON("100")}},
		{vs: ResolvingValueSet{"field1": types.RawJSON(`"EEE"`), "field2": types.RawJSON("600")}},
	}

	fieldSet := FieldMap{"field1": StringField("field_1"), "field2": Int64Field("field_2")}

	sorted, err := SortedValueSetCopy(context.Background(), qf, fieldSet, "field1", values)
	assert.NoError(t, err)

	resJSON, err := json.MarshalIndent(sorted, "", "  ")
	assert.NoError(t, err)

	fmt.Println((string)(resJSON))

	assert.JSONEq(t, `[
	  {"field1": "AAA", "field2": 300},
	  {"field1": "EEE", "field2": 600},
	  {"field1": "EEE", "field2": 600},
	  {"field1": "EEE", "field2": 100},
	  {"field1": "MMM", "field2": 500},
	  {"field1": "TTT", "field2": 600},
	  {"field1": "aaa", "field2": 300}
	]`, string(resJSON))

}
