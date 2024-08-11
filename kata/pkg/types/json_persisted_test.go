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

package types

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
)

func TestJSONPValue(t *testing.T) {

	type testStructChild struct {
		Child1 string `json:"child1"`
	}

	type testStruct struct {
		Parent1 JSONP[abi.ABI]           `json:"parent1"`
		Parent2 JSONP[*testStructChild]  `json:"parent2"`
		Parent3 JSONP[testStructChild]   `json:"parent3"`
		Parent4 *JSONP[testStructChild]  `json:"parent4,omitempty"`
		Parent5 *JSONP[*testStructChild] `json:"parent5"`
		Parent6 *JSONP[*testStructChild] `json:"parent6"`
		Parent7 JSONP[int64]             `json:"parent7"`
	}

	var v1 *JSONP[abi.ABI]
	b, err := json.Marshal(v1)
	assert.NoError(t, err)
	assert.JSONEq(t, `null`, string(b))

	v2 := &testStruct{
		Parent1: *WrapJSONP(abi.ABI{
			{Name: "function1", Type: "function", Inputs: abi.ParameterArray{}, Outputs: abi.ParameterArray{}},
		}),
		Parent2: JSONP[*testStructChild]{},
		Parent3: *WrapJSONP(testStructChild{Child1: "test_parent3"}),
		Parent4: nil,
		Parent5: &JSONP[*testStructChild]{},
		Parent6: WrapJSONP(&testStructChild{Child1: "test_parent6"}),
		Parent7: *WrapJSONP(int64(12345)),
	}
	b, err = json.Marshal(v2)
	assert.NoError(t, err)
	assert.JSONEq(t, `{
		"parent1": [{
			"type": "function",
			"name": "function1",
			"inputs": [],
			"outputs": []
		}],
		"parent2": null,
		"parent3": {
		  "child1": "test_parent3"
		},
		"parent5": null,
		"parent6": {
		  "child1": "test_parent6"
		},
		"parent7": 12345
	}`, string(b))

	var v1P1Valuer driver.Valuer = &v2.Parent1
	sqlV2P1, err := v1P1Valuer.Value()
	assert.NoError(t, err)
	assert.JSONEq(t, `[{
		"type": "function",
		"name": "function1",
		"inputs": [],
		"outputs": []
	}]`, string(sqlV2P1.([]byte)))

	sqlV2P2, err := v2.Parent2.Value()
	assert.NoError(t, err)
	assert.Nil(t, sqlV2P2)

	sqlV2P5, err := v2.Parent5.Value()
	assert.NoError(t, err)
	assert.Nil(t, sqlV2P5)

	sqlV2P7, err := v2.Parent7.Value()
	assert.NoError(t, err)
	assert.Equal(t, `12345`, string(sqlV2P7.([]byte)))

	var v3 *testStruct
	err = json.Unmarshal(b, &v3)
	assert.NoError(t, err)
	assert.Equal(t, v2.Parent1.V(), v3.Parent1.V())
	assert.Equal(t, v2.Parent2.V(), v3.Parent2.V())
	assert.Equal(t, v2.Parent3.V(), v3.Parent3.V())
	assert.Equal(t, v2.Parent4.V(), v3.Parent4.V())
	assert.Equal(t, v2.Parent5.V(), v3.Parent5.V())
	assert.Equal(t, v2.Parent6.V(), v3.Parent6.V())
	assert.Equal(t, v2.Parent7.V(), v3.Parent7.V())
}

func TestJSONPScan(t *testing.T) {

	type testStruct struct {
		Child1 string `json:"child1"`
	}

	var v1 JSONP[*testStruct]
	var v1Scanner sql.Scanner = &v1
	err := v1Scanner.Scan(`{"child1": "hello"}`)
	assert.NoError(t, err)
	assert.Equal(t, v1.V(), &testStruct{Child1: "hello"})

	var v2 JSONP[*testStruct]
	err = v2.Scan(([]byte)(`{"child1": "hello"}`))
	assert.NoError(t, err)
	assert.Equal(t, v2.V(), &testStruct{Child1: "hello"})

	var v3 JSONP[*testStruct]
	err = v3.Scan(nil)
	assert.NoError(t, err)
	assert.Nil(t, v3.V())

	v4 := &JSONP[*testStruct]{}
	err = v4.Scan(nil)
	assert.NoError(t, err)
	assert.Nil(t, v4.V())

	v5 := &JSONP[*testStruct]{}
	err = v5.Scan(false)
	assert.Regexp(t, "PD011101", err)
}
