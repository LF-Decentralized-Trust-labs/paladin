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
	"testing"

	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

func TestRawJSON(t *testing.T) {

	type myStruct struct {
		F1 RawJSON `json:"f1"`
		F2 RawJSON `json:"f2"`
		F3 RawJSON `json:"f3"`
	}

	var s1 myStruct
	err := json.Unmarshal(([]byte)(`{
		"f1": [ { "things": "and" }, "stuff" ],
		"f2": null
	}`), &s1)
	assert.NoError(t, err)
	assert.JSONEq(t, `[ { "things": "and" }, "stuff" ]`, s1.F1.String())
	assert.Equal(t, `null`, s1.F2.String())
	assert.Equal(t, `null`, s1.F3.String())
	assert.Equal(t, `[ { "things": "and" }, "stuff" ]`, s1.F1.Value())
	assert.Nil(t, s1.F2.Value())
	assert.Nil(t, s1.F3.Value())

	err = (*RawJSON)(nil).UnmarshalJSON(nil)
	assert.Regexp(t, "PD020001", err)

	err = (&s1.F1).Scan(nil)
	assert.NoError(t, err)
	assert.Nil(t, s1.F1)

	err = (&s1.F1).Scan(`[ { "more": "things" } ]`)
	assert.NoError(t, err)
	assert.JSONEq(t, `[ { "more": "things" } ]`, s1.F1.String())

	err = (&s1.F1).Scan(([]byte)(`[ { "yet": "more" }, "things" ]`))
	assert.NoError(t, err)
	assert.JSONEq(t, `[ { "yet": "more" }, "things" ]`, s1.F1.String())
	assert.JSONEq(t, `[ { "yet": "more" }, "things" ]`, s1.F1.Pretty())
	assert.YAMLEq(t, `[ { "yet": "more" }, "things" ]`, s1.F1.YAML())

	err = (&s1.F1).Scan(42)
	assert.Regexp(t, "PD020002", err)

	pettyErr := RawJSON(`[!!!! wrong`).Pretty()
	assert.Regexp(t, "invalid", pettyErr)
	yamlErr := RawJSON(`[!!!! wrong`).YAML()
	assert.Regexp(t, "invalid", yamlErr)

	assert.Equal(t, `This is a test with "quotes" of 'various' types`, JSONString(`This is a test with "quotes" of 'various' types`).StringValue())

	// check using json.Number we don't lose precision on StringValue
	assert.Equal(t, "123456789.123456789", RawJSON("123456789.123456789").StringValue())
	assert.Equal(t, "100000001.000000001", RawJSON("100000001.000000001").StringValue())

	// Nil is empty string for StringValue
	assert.Equal(t, "", RawJSON("null").StringValue())
	assert.Equal(t, "", RawJSON(nil).StringValue())

	// Others are JSON
	assert.JSONEq(t, `{"some":"thing"}`, RawJSON(`{"some":"thing"}`).StringValue())
	assert.JSONEq(t, `[{"some":"thing"}]`, RawJSON(`[{"some":"thing"}]`).StringValue())

}

func TestProtoToJSON(t *testing.T) {
	m := &prototk.Message{
		MessageId: "3d472892-8c5c-4290-910d-beeec5858e47",
	}
	assert.JSONEq(t, `{"messageId":"3d472892-8c5c-4290-910d-beeec5858e47"}`, ProtoToJSON(m).String())
}
