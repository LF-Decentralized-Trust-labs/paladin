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

package tktypes

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEthAddress(t *testing.T) {

	_, err := ParseEthAddress("wrong")
	assert.Regexp(t, "bad address", err)

	a := EthAddressBytes([]byte{0xfe, 0xed, 0xbe, 0xef})
	assert.Equal(t, "0xfeedbeef00000000000000000000000000000000", a.String())

	a, err = ParseEthAddress("0xacA6D8Ba6BFf0fa5c8a06A58368CB6097285d5c5")
	require.NoError(t, err)
	assert.Equal(t, "0xaca6d8ba6bff0fa5c8a06a58368cb6097285d5c5", a.String())
	assert.Equal(t, "0xacA6D8Ba6BFf0fa5c8a06A58368CB6097285d5c5", a.Checksummed())

	a = MustEthAddress("0xacA6D8Ba6BFf0fa5c8a06A58368CB6097285d5c5")
	assert.Equal(t, "0xaca6d8ba6bff0fa5c8a06a58368cb6097285d5c5", (*a).String())

	var a1 *EthAddress
	err = a1.Scan(nil)
	require.NoError(t, err)
	assert.Nil(t, a1)

	a2 := &EthAddress{}
	err = a2.Scan(a.String())
	require.NoError(t, err)
	assert.Equal(t, a, a2)

	v2, err := a2.Value()
	require.NoError(t, err)
	assert.Equal(t, strings.TrimPrefix(a.String(), "0x"), v2)

	a3 := &EthAddress{}
	err = a3.Scan(([]byte)(a[:]))
	require.NoError(t, err)
	assert.Equal(t, a, a3)

	a4 := &EthAddress{}
	err = a4.Scan(([]byte)(a.String()))
	require.NoError(t, err)
	assert.Equal(t, a, a4)

	a5 := &EthAddress{}
	err = a5.Scan([]byte{0x01})
	assert.Regexp(t, "FF00105", err)

	a6 := &EthAddress{}
	err = a6.Scan(false)
	assert.Regexp(t, "FF00105", err)

	a7 := &EthAddress{}
	err = a7.Scan(([]byte)("!!aca6d8ba6bff0fa5c8a06a58368cb6097285d5"))
	assert.Regexp(t, "bad address", err)

	a8 := &EthAddress{}
	err = a8.Scan("!!aca6d8ba6bff0fa5c8a06a58368cb6097285d5")
	assert.Regexp(t, "bad address", err)
}

func TestEthAddressJSON(t *testing.T) {
	type testStruct struct {
		A1 EthAddress  `json:"a1"`
		A2 *EthAddress `json:"a2"`
	}

	var s1 *testStruct
	err := json.Unmarshal([]byte(`{}`), &s1)
	require.NoError(t, err)

	b1, err := json.Marshal(s1)
	require.NoError(t, err)
	assert.JSONEq(t, `{
	  "a1": "0x0000000000000000000000000000000000000000",
	  "a2": null
	}`, string(b1))

	var s2 *testStruct
	err = json.Unmarshal([]byte(`{
	  "a1": "0x67377A61Bb38d8Cf2cc2A255E2f0e96f6b0874E7",
	  "a2": "16C076fDE0350249d200a960952e6c8c43eD7986"
	}`), &s2)
	require.NoError(t, err)

	b2, err := json.Marshal(s2)
	require.NoError(t, err)
	assert.JSONEq(t, `{
	  "a1": "0x67377a61bb38d8cf2cc2a255e2f0e96f6b0874e7",
	  "a2": "0x16c076fde0350249d200a960952e6c8c43ed7986"
	}`, string(b2))

	var s3 *testStruct
	err = json.Unmarshal([]byte(`{
	  "a1": "wrong"
	}`), &s3)
	assert.Regexp(t, "bad address", err)

	err = s3.A1.UnmarshalJSON([]byte(`!!!{ wrong`))
	assert.Error(t, err)
}
