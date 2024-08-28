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

func TestHexBytesStatic(t *testing.T) {

	var id1 HexBytes
	assert.Equal(t, "", id1.String())
	assert.Equal(t, "0x", id1.HexString0xPrefix())
	assert.Equal(t, "", id1.HexString())

	assert.Panics(t, func() {
		MustParseHexBytes("wrong")
	})

	id2 := MustParseHexBytes("0x512d0e595c71863c47e803c565562f9284a48ee8984f4f9b55323eed72cf1414")
	assert.Equal(t, "0x512d0e595c71863c47e803c565562f9284a48ee8984f4f9b55323eed72cf1414", id2.String())

	id3 := MustParseHexBytes("512D0E595C71863C47E803C565562F9284A48EE8984F4F9B55323EED72CF1414")
	assert.Equal(t, "0x512d0e595c71863c47e803c565562f9284a48ee8984f4f9b55323eed72cf1414", id3.String())
	assert.Equal(t, "0x512d0e595c71863c47e803c565562f9284a48ee8984f4f9b55323eed72cf1414", id3.HexString0xPrefix())
	assert.Equal(t, "512d0e595c71863c47e803c565562f9284a48ee8984f4f9b55323eed72cf1414", id3.HexString())

	assert.True(t, id2.Equals(id3))
	assert.False(t, id2.Equals(nil))
	assert.True(t, (HexBytes)(nil).Equals(nil))
	assert.False(t, (HexBytes)(nil).Equals(id2))

}

func TestHexBytesMarshalingJSON(t *testing.T) {

	type myStruct struct {
		ID1 *HexBytes `json:"id1"`
		ID2 *HexBytes `json:"id2,omitempty"`
		ID3 *HexBytes `json:"id3"`
		ID4 *HexBytes `json:"id4"`
		ID5 HexBytes  `json:"id5"`
		ID6 HexBytes  `json:"id6"`
		ID7 HexBytes  `json:"id7"`
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

	jOut, err := json.Marshal(&s1)
	assert.NoError(t, err)
	assert.JSONEq(t, `{
		"id1": null,
		"id3": "0x47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad",
		"id4": "0x47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad",
		"id5": "0x47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad",
		"id6": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"id7": "0x"
	}`, (string)(jOut))

	err = json.Unmarshal(([]byte)(`{"id1":"wrong"}`), &s1)
	assert.Regexp(t, "PD010100", err)

}

func TestHexBytesScanValue(t *testing.T) {

	v, err := MustParseHexBytes("0x47173285A8D7341E5E972FC677286384F802F8EF42A5EC5F03BBFA254CB01FAD").Value()
	assert.NoError(t, err)
	assert.Equal(t, "47173285a8d7341e5e972fc677286384f802f8ef42a5ec5f03bbfa254cb01fad", v)

	scanner := &HexBytes{}

	err = scanner.Scan("0xfeedbeef")
	assert.NoError(t, err)
	assert.Equal(t, "0xfeedbeef", scanner.String())

	err = scanner.Scan([]byte{0xfe, 0xed, 0xbe, 0xef})
	assert.NoError(t, err)
	assert.Equal(t, "0xfeedbeef", scanner.String())

	err = scanner.Scan("0xWRONG!85A8D7341E5E972FC677286384F802F8EF42A5EC5F03BBFA254CB01FAD")
	assert.Regexp(t, "PD010100", err)

	err = scanner.Scan(false)
	assert.Regexp(t, "PD011101", err)

}
