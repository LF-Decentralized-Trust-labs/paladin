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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

type TestEnum string

func (te TestEnum) Options() []string {
	return []string{"option1", "option2", "option3"}
}

func (te TestEnum) Default() string {
	return "option2"
}

func TestEnumValue(t *testing.T) {

	var v1 Enum[TestEnum] = "OPTION1"
	sqlV1, err := v1.Value()
	assert.NoError(t, err)
	assert.Equal(t, "option1", sqlV1)

	assert.Equal(t, TestEnum("OPTION1"), v1.V())

	var v2 Enum[TestEnum] = "option4"
	_, err = v2.Value()
	assert.Regexp(t, "PD011102", err)
}

func TestEnumJSON(t *testing.T) {
	type myStruct struct {
		Field1 Enum[TestEnum]  `json:"field1"`
		Field2 *Enum[TestEnum] `json:"field2"`
	}

	var v1 myStruct
	err := json.Unmarshal(([]byte)(`{}`), &v1)
	assert.NoError(t, err)
	assert.Equal(t, myStruct{
		Field1: "",
		Field2: nil,
	}, v1)
	testVal, err := v1.Field1.Validate()
	assert.NoError(t, err)
	assert.Equal(t, "option2", testVal)
	testVal, err = v1.Field1.Validate()
	assert.NoError(t, err)
	assert.Equal(t, "option2", testVal)
}

func TestEnumScan(t *testing.T) {
	var v Enum[TestEnum]

	err := (&v).Scan(nil)
	assert.NoError(t, err)
	assert.Equal(t, "option2", string(v))

	err = (&v).Scan("OPTION1")
	assert.NoError(t, err)
	assert.Equal(t, "option1", string(v))

	err = (&v).Scan(([]byte)("Option3"))
	assert.NoError(t, err)
	assert.Equal(t, "option3", string(v))

	err = (&v).Scan(false)
	assert.Regexp(t, "PD011101", err)

	err = (&v).Scan("option4")
	assert.Regexp(t, "PD011102", err)
}
