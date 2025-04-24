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

package pldtypes

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHexUint64(t *testing.T) {

	v := MustParseHexUint64("9223372036854775807")
	assert.Equal(t, uint64(9223372036854775807), v.Uint64())
	dbv, err := v.Value()
	require.NoError(t, err)
	assert.Equal(t, int64(0x7fffffffffffffff), dbv)
	assert.Equal(t, "0x7fffffffffffffff", v.String())

	v = MustParseHexUint64("9223372036854775808")
	assert.Equal(t, uint64(0x8000000000000000), v.Uint64())
	_, err = v.Value()
	require.Regexp(t, "PD020011", err)
	assert.Equal(t, "0x8000000000000000", v.String())

	v = MustParseHexUint64("0x8000000000000000")
	assert.Equal(t, uint64(0x8000000000000000), v.Uint64())

	assert.Panics(t, func() {
		_ = MustParseHexUint64("wrong")
	})

	_, err = ParseHexUint64(context.Background(), "wrong")
	require.Regexp(t, "PD020009", err)

	type testStruct struct {
		F1 HexUint64 `json:"f1"`
	}
	var ts testStruct
	err = json.Unmarshal([]byte(`{
		"f1": 1000000000000000000000001
	}`), &ts)
	assert.Regexp(t, "PD020010", err) // too big for uint256
	err = json.Unmarshal([]byte(`{
		"f1": "0x7fffffffffffffff"
	}`), &ts)
	require.NoError(t, err)
	assert.Equal(t, "7fffffffffffffff", ts.F1.HexString())

	err = ts.F1.Scan(int64(12345))
	require.NoError(t, err)
	assert.Equal(t, uint64(12345), ts.F1.Uint64())

	err = ts.F1.Scan(false)
	assert.Regexp(t, "PD020002.*bool", err)

	b, err := json.Marshal(ts)
	require.NoError(t, err)
	assert.Equal(t, `{"f1":"0x3039"}`, string(b))

}
