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
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBytes32Static(t *testing.T) {

	var id1 Bytes32
	assert.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000000000", id1.HexString0xPrefix())
	assert.Equal(t, "0000000000000000000000000000000000000000000000000000000000000000", id1.HexString())
	assert.True(t, id1.IsZero()) // nil returns true for isZero (as Bytes32 would give zero)

	ctx := context.Background()
	_, err := ParseBytes32Ctx(ctx, "0xfeedbeef")
	assert.Regexp(t, "PD010719.*32.*4", err)

	assert.Panics(t, func() {
		MustParseBytes32("wrong")
	})

	checkFixedOK := func(id *Bytes32) {
		assert.Equal(t, "0x512d0e595c71863c47e803c565562f9284a48ee8984f4f9b55323eed72cf1414", id.String())
		assert.Equal(t, "0x512d0e595c71863c47e803c565562f9284a48ee8984f4f9b55323eed72cf1414", id.HexString0xPrefix())
		assert.Equal(t, "512d0e595c71863c47e803c565562f9284a48ee8984f4f9b55323eed72cf1414", id.HexString())
		assert.Equal(t, "512d0e59-5c71-863c-47e8-03c565562f92", id.UUIDLower16().String())
		assert.Equal(t, "512d0e595c71863c47e803c565562f9200000000000000000000000000000000", Bytes32UUIDLower16(id.UUIDLower16()).HexString())
	}

	id2 := MustParseBytes32("0x512d0e595c71863c47e803c565562f9284a48ee8984f4f9b55323eed72cf1414")
	checkFixedOK(&id2)

	id3 := NewBytes32FromSlice(id2.Bytes())
	checkFixedOK(&id3)

	assert.True(t, id2.Equals(&id3))
	assert.False(t, id2.Equals(nil))
	assert.True(t, (*Bytes32)(nil).Equals(nil))
	assert.False(t, (*Bytes32)(nil).Equals(&id2))
	id4 := MustParseBytes32("512d0e595c71863c47e803c565562f9284a48ee8984f4f9b55323eed72cf1414")
	assert.True(t, (*Bytes32)(&id2).Equals(&id4))

}

func TestBytes32Keccak(t *testing.T) {

	id1 := Bytes32Keccak(([]byte)("hello world"))
	assert.Equal(t, "47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad", id1.HexString())

}

func TestBytes32MarshalingJSON(t *testing.T) {

	type myStruct struct {
		ID1 *Bytes32 `json:"id1"`
		ID2 *Bytes32 `json:"id2,omitempty"`
		ID3 *Bytes32 `json:"id3"`
		ID4 *Bytes32 `json:"id4"`
		ID5 Bytes32  `json:"id5"`
		ID6 Bytes32  `json:"id6"`
		ID7 Bytes32  `json:"id7"`
	}

	inJSON := ([]byte)(`{
		"id1": null,
		"id3": "0x47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad",
		"id4": "47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad",
		"id5": "47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad",
		"id6": "0000000000000000000000000000000000000000000000000000000000000000"
	}`)

	var s1 myStruct
	err := json.Unmarshal(inJSON, &s1)
	assert.NoError(t, err)

	assert.Nil(t, s1.ID1)
	assert.Nil(t, s1.ID2)
	assert.Equal(t, "0x47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad", s1.ID3.String())
	assert.Equal(t, "0x47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad", s1.ID4.String())
	assert.Equal(t, "0x47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad", s1.ID5.String())
	assert.True(t, s1.ID6.IsZero())
	assert.True(t, s1.ID7.IsZero())

	jOut, err := json.Marshal(&s1)
	assert.NoError(t, err)
	assert.JSONEq(t, `{
		"id1": null,
		"id3": "0x47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad",
		"id4": "0x47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad",
		"id5": "0x47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad",
		"id6": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"id7": "0x0000000000000000000000000000000000000000000000000000000000000000"
	}`, (string)(jOut))

	err = json.Unmarshal(([]byte)(`{"id1":"wrong"}`), &s1)
	assert.Regexp(t, "PD010100", err)

}

func TestBytes32ScanValue(t *testing.T) {

	v, err := MustParseBytes32("0x47173285A8D7341E5E972FC677286384F802F8EF42A5EC5F03BBFA254CB01FAD").Value()
	assert.NoError(t, err)
	assert.Equal(t, "47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad", v)

	scanner := &Bytes32{}
	err = scanner.Scan(([]byte)("0x47173285A8D7341E5E972FC677286384F802F8EF42A5EC5F03BBFA254CB01FAD"))
	assert.NoError(t, err)
	assert.Equal(t, "47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad", scanner.HexString())

	scanner = &Bytes32{}
	err = scanner.Scan(MustParseBytes32("0x47173285A8D7341E5E972FC677286384F802F8EF42A5EC5F03BBFA254CB01FAD").Bytes())
	assert.NoError(t, err)
	assert.Equal(t, "47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad", scanner.HexString())

	scanner = &Bytes32{}
	err = scanner.Scan("0x47173285A8D7341E5E972FC677286384F802F8EF42A5EC5F03BBFA254CB01FAD")
	assert.NoError(t, err)
	assert.Equal(t, "47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad", scanner.HexString())

	scanner = &Bytes32{}
	err = scanner.Scan("0x47173285A8D7341E5E972FC677286384F802F8EF42A5EC5F03BBFA254CB01FAD")
	assert.NoError(t, err)
	assert.Equal(t, "47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad", scanner.HexString())

	err = scanner.Scan("0xfeedbeef")
	assert.Regexp(t, "PD010719.*4", err)

	err = scanner.Scan([]byte{0xfe, 0xed, 0xbe, 0xef})
	assert.Regexp(t, "PD010719.*4", err)

	err = scanner.Scan([]byte("0xWRONG!85A8D7341E5E972FC677286384F802F8EF42A5EC5F03BBFA254CB01FAD"))
	assert.Regexp(t, "PD010100", err)

	err = scanner.Scan(false)
	assert.Regexp(t, "PD011101", err)

}
