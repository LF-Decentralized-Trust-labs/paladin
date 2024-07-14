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

package state

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHashIDStatic(t *testing.T) {

	var id1 *HashID
	assert.Equal(t, "", id1.String())
	assert.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000000000", id1.HexString0xPrefix())
	assert.Equal(t, "0000000000000000000000000000000000000000000000000000000000000000", id1.HexString())
	assert.Nil(t, id1.Bytes())
	assert.True(t, id1.IsZero()) // nil returns true for isZero (as Bytes32 would give zero)

	ctx := context.Background()
	_, err := ParseHashID(ctx, "0xfeedbeef")
	assert.Regexp(t, "PD010101.*32.*4", err)

	assert.Panics(t, func() {
		MustParseHashID("wrong")
	})

	checkFixedOK := func(id *HashID) {
		assert.Equal(t, "0x512d0e595c71863c47e803c565562f9284a48ee8984f4f9b55323eed72cf1414", id.String())
		assert.Equal(t, "0x512d0e595c71863c47e803c565562f9284a48ee8984f4f9b55323eed72cf1414", id.HexString0xPrefix())
		assert.Equal(t, "512d0e595c71863c47e803c565562f9284a48ee8984f4f9b55323eed72cf1414", id.HexString())
	}

	id2 := MustParseHashID("0x512d0e595c71863c47e803c565562f9284a48ee8984f4f9b55323eed72cf1414")
	checkFixedOK(id2)

	id3 := NewHashIDSlice32(id2.Bytes())
	checkFixedOK(id3)

	id4 := NewHashID(id2.Bytes32())
	checkFixedOK(id4)

}

func TestHashIDKeccak(t *testing.T) {

	id1 := HashIDKeccak(([]byte)("hello world"))
	assert.Equal(t, "47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad", id1.HexString())

}

func TestHashIDMarshalingJSON(t *testing.T) {

	type myStruct struct {
		ID1 *HashID `json:"id1"`
		ID2 *HashID `json:"id2,omitempty"`
		ID3 *HashID `json:"id3"`
		ID4 *HashID `json:"id4"`
		ID5 HashID  `json:"id5"`
		ID6 HashID  `json:"id6"`
		ID7 HashID  `json:"id7"`
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
