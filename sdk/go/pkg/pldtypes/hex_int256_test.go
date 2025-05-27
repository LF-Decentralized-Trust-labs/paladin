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

func TestHexInt256(t *testing.T) {

	assert.Equal(t, int64(-10), Int64ToInt256(-10).Int().Int64())

	v := MustParseHexInt256("9223372036854775807")
	assert.Equal(t, uint64(9223372036854775807), v.Int().Uint64())
	dbv, err := v.Value()
	require.NoError(t, err)
	assert.Equal(t, "0x7fffffffffffffff", v.String())
	err = v.Scan(dbv)
	require.NoError(t, err)
	assert.Equal(t, int64(9223372036854775807), v.Int().Int64())

	v = MustParseHexInt256("-9223372036854775808")
	assert.Equal(t, int64(-9223372036854775808), v.Int().Int64())
	v = MustParseHexInt256("-0x8000000000000000")
	assert.Equal(t, int64(-9223372036854775808), v.Int().Int64())
	dbv, err = v.Value()
	require.NoError(t, err)
	assert.Equal(t, "0ffffffffffffffffffffffffffffffffffffffffffffffff8000000000000000", dbv)
	assert.Equal(t, "-0x8000000000000000", v.String())
	err = v.Scan(dbv)
	require.NoError(t, err)
	assert.Equal(t, int64(-9223372036854775808), v.Int().Int64())

	v = MustParseHexInt256("9223372036854775808")
	assert.Equal(t, uint64(0x8000000000000000), v.Int().Uint64())
	dbv, err = v.Value()
	require.NoError(t, err)
	assert.Equal(t, "0x8000000000000000", v.String())
	assert.Equal(t, "10000000000000000000000000000000000000000000000008000000000000000", dbv)

	v = MustParseHexInt256("0x8000000000000000")
	assert.Equal(t, uint64(0x8000000000000000), v.Int().Uint64())

	assert.Panics(t, func() {
		_ = MustParseHexInt256("wrong")
	})

	_, err = ParseHexInt256(context.Background(), "wrong")
	require.Regexp(t, "PD020009", err)

	type testStruct struct {
		F1 *HexInt256 `json:"f1"`
	}
	var ts testStruct
	err = json.Unmarshal([]byte(`{
		"f1": 1000000000000000000000001
	}`), &ts)
	require.NoError(t, err)
	require.Equal(t, "1000000000000000000000001", ts.F1.Int().Text(10))
	err = json.Unmarshal([]byte(`{
		"f1": "0x7fffffffffffffff"
	}`), &ts)
	require.NoError(t, err)
	assert.Equal(t, "7fffffffffffffff", ts.F1.HexString())
	err = json.Unmarshal([]byte(`{
		"f1": "wrong"
	}`), &ts)
	assert.Regexp(t, "PD020009", err)
	err = json.Unmarshal([]byte(`{
		"f1": false
	}`), &ts)
	assert.Regexp(t, "PD020002", err)

	err = ts.F1.Scan(int64(12345))
	require.NoError(t, err)
	assert.Equal(t, uint64(12345), ts.F1.Int().Uint64())

	err = ts.F1.Scan(false)
	assert.Regexp(t, "PD020002.*bool", err)

	b, err := json.Marshal(ts)
	require.NoError(t, err)
	assert.Equal(t, `{"f1":"0x3039"}`, string(b))

	err = ts.F1.Scan("0x12346")
	assert.Regexp(t, "PD020012", err)

	err = v.Scan("wrong000000000000000000000000000000000000000000007fffffffffffffff")
	assert.Regexp(t, "PD020012", err)

	assert.True(t, ((*HexInt256)(nil)).NilOrZero())

	dbv, err = ((*HexInt256)(nil)).Value()
	assert.NoError(t, err)
	assert.Nil(t, dbv)

}
