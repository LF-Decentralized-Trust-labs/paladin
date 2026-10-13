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
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHexUint256(t *testing.T) {

	assert.Equal(t, int64(10), Uint64ToUint256(10).Int().Int64())

	v := MustParseHexUint256("9223372036854775807")
	assert.Equal(t, uint64(9223372036854775807), v.Int().Uint64())
	dbv, err := v.Value()
	require.NoError(t, err)
	assert.Equal(t, "0x7fffffffffffffff", v.String())
	err = v.Scan(dbv)
	require.NoError(t, err)
	assert.Equal(t, uint64(9223372036854775807), v.Int().Uint64())

	v = MustParseHexUint256("1152921504606846975")
	assert.Equal(t, uint64(1152921504606846975), v.Int().Uint64())
	dbv, err = v.Value()
	require.NoError(t, err)
	assert.Equal(t, "0x0fffffffffffffff", v.String())
	err = v.Scan(dbv)
	require.NoError(t, err)
	assert.Equal(t, uint64(1152921504606846975), v.Int().Uint64())

	v = MustParseHexUint256("0x8000000000000000")
	assert.Equal(t, uint64(9223372036854775808), v.Int().Uint64())
	dbv, err = v.Value()
	require.NoError(t, err)
	assert.Equal(t, "0000000000000000000000000000000000000000000000008000000000000000", dbv)
	assert.Equal(t, "0x8000000000000000", v.String())
	err = v.Scan(dbv)
	require.NoError(t, err)
	assert.Equal(t, uint64(0x8000000000000000), v.Int().Uint64())

	v = MustParseHexUint256("0x8000000000000000")
	assert.Equal(t, uint64(0x8000000000000000), v.Int().Uint64())

	// The largest value the type holds is 256 bits, and round-trips through the DB
	v = MustParseHexUint256("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	assert.Equal(t, 256, v.Int().BitLen())
	dbv, err = v.Value()
	require.NoError(t, err)
	assert.Equal(t, "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", dbv)
	err = v.Scan(dbv)
	require.NoError(t, err)
	assert.Equal(t, 256, v.Int().BitLen())

	assert.Panics(t, func() {
		_ = MustParseHexUint256("wrong")
	})

	assert.Panics(t, func() {
		_ = MustParseHexUint256("-1")
	})

	assert.Panics(t, func() {
		_ = MustParseHexUint256("115792089237316195423570985008687907853269984665640564039457584007913129639936")
	})

	_, err = ParseHexUint256(t.Context(), "wrong")
	require.Regexp(t, "PD020009", err)

	// The type is unsigned, so a negative is not a valid value for it
	_, err = ParseHexUint256(t.Context(), "-1")
	assert.Regexp(t, "PD020027", err)
	_, err = ParseHexUint256(t.Context(), "-0x2a")
	assert.Regexp(t, "PD020027", err)
	_, err = ParseHexUint256(t.Context(), "-255")
	assert.Regexp(t, "PD020027", err)

	// Nor is a value of more than 256 bits
	_, err = ParseHexUint256(t.Context(), "115792089237316195423570985008687907853269984665640564039457584007913129639936")
	assert.Regexp(t, "PD020028", err)
	_, err = ParseHexUint256(t.Context(), "0x10000000000000000000000000000000000000000000000000000000000000000")
	assert.Regexp(t, "PD020028", err)

	type testStruct struct {
		F1 *HexUint256 `json:"f1"`
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
	err = json.Unmarshal([]byte(`{
		"f1": "-0x2a"
	}`), &ts)
	assert.Regexp(t, "PD020027", err)
	err = json.Unmarshal([]byte(`{
		"f1": -42
	}`), &ts)
	assert.Regexp(t, "PD020027", err)
	err = json.Unmarshal([]byte(`{
		"f1": 115792089237316195423570985008687907853269984665640564039457584007913129639936
	}`), &ts)
	assert.Regexp(t, "PD020028", err)

	err = ts.F1.Scan(int64(12345))
	require.NoError(t, err)
	assert.Equal(t, uint64(12345), ts.F1.Int().Uint64())

	err = ts.F1.Scan(false)
	assert.Regexp(t, "PD020002.*bool", err)

	b, err := json.Marshal(ts)
	require.NoError(t, err)
	assert.Equal(t, `{"f1":"0x3039"}`, string(b))

	err = ts.F1.Scan("0x12346")
	assert.Regexp(t, "PD020013", err)

	err = ts.F1.Scan(int64(-1))
	assert.Regexp(t, "PD020027", err)

	err = v.Scan("wrong000000000000000000000000000000000000000000007fffffffffffffff")
	assert.Regexp(t, "PD020013", err)

	// A value can also be handed to the type by conversion, bypassing the constructors.
	// The two hex accessors must agree on what it is, and neither may misreport its sign
	neg := (*HexUint256)(big.NewInt(-42))
	assert.Equal(t, "-0x2a", neg.HexString0xPrefix())
	assert.Equal(t, "-2a", neg.HexString())

	// Such a value cannot be represented in the DB, so it is rejected rather than
	// serialized as something else, and the caller's value is left as it was
	_, err = neg.Value()
	assert.Regexp(t, "PD020027", err)
	assert.Equal(t, int64(-42), neg.Int().Int64())

	over := (*HexUint256)(new(big.Int).Lsh(big.NewInt(1), 256))
	_, err = over.Value()
	assert.Regexp(t, "PD020028", err)
	assert.Equal(t, 257, over.Int().BitLen())

	// A negative is rejected rather than encoded as its absolute value, and is left unmodified
	bi := big.NewInt(-99)
	_, err = PadHexBigUint(t.Context(), bi, make([]byte, 64))
	assert.Regexp(t, "PD020027", err)
	assert.Equal(t, int64(-99), bi.Int64())

	padded, err := PadHexBigUint(t.Context(), big.NewInt(99), make([]byte, 64))
	require.NoError(t, err)
	assert.Equal(t, "0000000000000000000000000000000000000000000000000000000000000063", string(padded))

	assert.True(t, ((*HexUint256)(nil)).NilOrZero())

	dbv, err = ((*HexUint256)(nil)).Value()
	assert.NoError(t, err)
	assert.Nil(t, dbv)

}

func TestHexUint256ScanRejectsSignedDBValue(t *testing.T) {

	// big.Int.SetString accepts a leading sign, so a 64 character value carrying one would
	// otherwise scan into a negative - a value outside the range of this type, and one that
	// Value() never writes
	for _, v := range []string{
		"-" + strings.Repeat("f", 63),
		"+" + strings.Repeat("f", 63),
		"-" + strings.Repeat("0", 62) + "1",
	} {
		var hi HexUint256
		err := hi.Scan(v)
		assert.Regexp(t, "PD020013", err, "scanning %q", v)
	}

	// The full unsigned range of the type still scans
	for _, v := range []string{strings.Repeat("0", 64), strings.Repeat("f", 64)} {
		var hi HexUint256
		require.NoError(t, hi.Scan(v))
		assert.GreaterOrEqual(t, hi.Int().Sign(), 0)
	}
}

func TestPadHexBigUintBounds(t *testing.T) {

	maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

	padded, err := PadHexBigUint(t.Context(), maxUint256, make([]byte, 64))
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("f", 64), string(padded))

	// 2^256 does not fit the 64 character encoding. Without a range check the leading digit is
	// silently dropped, making it indistinguishable from the encoding of zero
	_, err = PadHexBigUint(t.Context(), new(big.Int).Lsh(big.NewInt(1), 256), make([]byte, 64))
	assert.Regexp(t, "PD020028", err)

	// A buffer too small for an in-range value truncates the same way, so is also an error
	_, err = PadHexBigUint(t.Context(), big.NewInt(0x1234), make([]byte, 3))
	assert.Regexp(t, "PD020030", err)
}
