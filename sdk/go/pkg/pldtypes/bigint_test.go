// Copyright © 2025 Kaleido, Inc.
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

package pldtypes

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBigIntEmptyJSON(t *testing.T) {

	var myStruct struct {
		Field1 PLDBigInt  `json:"field1,omitempty"`
		Field2 *PLDBigInt `json:"field2,omitempty"`
		Field3 *PLDBigInt `json:"field3"`
	}

	jsonVal := []byte(`{}`)

	err := json.Unmarshal(jsonVal, &myStruct)
	assert.NoError(t, err)
	assert.Zero(t, myStruct.Field1.Int().Int64())
	assert.Nil(t, myStruct.Field2)
	assert.Nil(t, myStruct.Field3)

}

func TestBigIntSetJSONOk(t *testing.T) {

	var myStruct struct {
		Field1 PLDBigInt  `json:"field1"`
		Field2 *PLDBigInt `json:"field2"`
		Field3 *PLDBigInt `json:"field3"`
		Field4 *PLDBigInt `json:"field4"`
	}

	jsonVal := []byte(`{
		"field1": -111111,
		"field2": 2222.22,
		"field3": "333333",
		"field4": "0xfeedBEEF"
	}`)

	err := json.Unmarshal(jsonVal, &myStruct)
	assert.NoError(t, err)
	assert.Equal(t, int64(-111111), myStruct.Field1.Int().Int64())
	assert.Equal(t, int64(2222), myStruct.Field2.Int().Int64())
	assert.Equal(t, int64(333333), myStruct.Field3.Int().Int64())
	assert.Equal(t, int64(4276993775), myStruct.Field4.Int().Int64())

	jsonValSerialized, err := json.Marshal(&myStruct)

	assert.NoError(t, err)
	assert.JSONEq(t, `{
		"field1": "-111111",
		"field2": "2222",
		"field3": "333333",
		"field4": "4276993775"
	}`, string(jsonValSerialized))
}

func TestBigIntJSONBadString(t *testing.T) {

	jsonVal := []byte(`"0xZZ"`)

	var bi PLDBigInt
	err := json.Unmarshal(jsonVal, &bi)
	assert.Regexp(t, "PD020024", err)

}

func TestBigIntJSONBadType(t *testing.T) {

	jsonVal := []byte(`{
		"field1": { "not": "valid" }
	}`)

	var bi PLDBigInt
	err := json.Unmarshal(jsonVal, &bi)
	assert.Regexp(t, "PD020024", err)

}

func TestBigIntJSONBadJSON(t *testing.T) {

	jsonVal := []byte(`!JSON`)

	var bi PLDBigInt
	err := bi.UnmarshalJSON(jsonVal)
	assert.Regexp(t, "PD020024", err)

}

func TestLargePositiveBigIntValue(t *testing.T) {

	var iMax PLDBigInt
	_ = iMax.Int().Exp(big.NewInt(2), big.NewInt(256), nil)
	iMax.Int().Sub(iMax.Int(), big.NewInt(1))
	iMaxVal, err := iMax.Value()
	assert.NoError(t, err)
	assert.Equal(t, "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", iMaxVal)

	var iRead big.Int
	_, ok := iRead.SetString(iMaxVal.(string), 16)
	assert.True(t, ok)

}

func TestLargeNegativeBigIntValue(t *testing.T) {

	var iMax PLDBigInt
	_ = iMax.Int().Exp(big.NewInt(2), big.NewInt(256), nil)
	iMax.Int().Neg(iMax.Int())
	iMax.Int().Add(iMax.Int(), big.NewInt(1))
	iMaxVal, err := iMax.Value()
	assert.NoError(t, err)
	// Note that this is a "-" prefix with a variable width big-endian positive number (not a fixed width two's compliment)
	assert.Equal(t, "-ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", iMaxVal)

	var iRead big.Int
	_, ok := iRead.SetString(iMaxVal.(string), 16)
	assert.True(t, ok)

}

func TestLargeishPositiveBigIntValue(t *testing.T) {

	iLargeish, ok := (new(big.Int).SetString("123456789abcdef00fedcba987654321", 16))
	assert.True(t, ok)
	iLargeishVal, err := (*PLDBigInt)(iLargeish).Value()
	assert.Equal(t, "00000000000000000000000000000000123456789abcdef00fedcba987654321", iLargeishVal)
	assert.NoError(t, err)

	var iRead big.Int
	_, ok = iRead.SetString(iLargeishVal.(string), 16)
	assert.True(t, ok)
	assert.Equal(t, *iLargeish, iRead)

}

func TestLargeishNegativeBigIntValue(t *testing.T) {

	iLargeish, ok := (new(big.Int).SetString("-123456789abcdef00fedcba987654321", 16))
	assert.True(t, ok)
	iLargeishVal, err := (*PLDBigInt)(iLargeish).Value()
	assert.Equal(t, "-00000000000000000000000000000000123456789abcdef00fedcba987654321", iLargeishVal)
	assert.NoError(t, err)

	var iRead big.Int
	_, ok = iRead.SetString(iLargeishVal.(string), 16)
	assert.True(t, ok)
	assert.Equal(t, *iLargeish, iRead)

}

func TestTooLargeInteger(t *testing.T) {

	var iMax PLDBigInt
	_ = iMax.Int().Exp(big.NewInt(2), big.NewInt(256), nil)
	iMax.Int().Neg(iMax.Int())
	_, err := iMax.Value()
	assert.Regexp(t, "PD020025", err)

}

func TestScanNil(t *testing.T) {

	var nilVal interface{}
	var i PLDBigInt
	err := i.Scan(nilVal)
	assert.NoError(t, err)
	assert.Zero(t, i.Int().Int64())

}

func TestScanString(t *testing.T) {

	var i PLDBigInt
	err := i.Scan("-feedbeef")
	assert.NoError(t, err)
	assert.Equal(t, int64(-4276993775), i.Int().Int64())
	assert.Equal(t, "-4276993775", i.String())

}

func TestScanEmptyString(t *testing.T) {

	var i PLDBigInt
	err := i.Scan("")
	assert.NoError(t, err)
	assert.Zero(t, i.Int().Int64())

}

func TestScanBadString(t *testing.T) {

	var i PLDBigInt
	err := i.Scan("!hex")
	assert.Regexp(t, "PD020026", err)

}

func TestScanBadType(t *testing.T) {

	var i PLDBigInt
	err := i.Scan(123456)
	assert.Regexp(t, "PD020026", err)

}

func TestEquals(t *testing.T) {

	var pi1, pi2 *PLDBigInt
	assert.True(t, pi1.Equals(pi2))

	var i1 PLDBigInt
	i1.Int().Set(big.NewInt(1))

	assert.False(t, i1.Equals(pi2))
	assert.False(t, pi2.Equals(&i1))

	var i2 PLDBigInt
	i2.Int().Set(big.NewInt(1))

	assert.True(t, i1.Equals(&i2))
	assert.True(t, i2.Equals(&i1))

}

func TestNewBigInt(t *testing.T) {

	n := NewPLDBigInt(10)
	assert.Equal(t, int64(10), n.Int().Int64())

}

func TestBigIntInt64(t *testing.T) {
	var n *PLDBigInt
	assert.Equal(t, int64(0), n.Int64())
	n = NewPLDBigInt(10)
	assert.Equal(t, int64(10), n.Int64())
}

func TestBigIntUint64(t *testing.T) {
	var n *PLDBigInt
	assert.Equal(t, uint64(0), n.Uint64())
	n = NewPLDBigInt(10)
	assert.Equal(t, uint64(10), n.Uint64())
	n = NewPLDBigInt(-1)
	assert.Equal(t, uint64(0), n.Uint64())
}
