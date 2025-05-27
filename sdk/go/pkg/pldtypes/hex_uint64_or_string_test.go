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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHexUint64OrStringUnmarshal(t *testing.T) {

	type testStruct struct {
		Block HexUint64OrString `json:"block,omitempty"`
	}

	var ts *testStruct
	err := json.Unmarshal([]byte(`{"block":"latest"}`), &ts)
	require.NoError(t, err)
	assert.Equal(t, "latest", ts.Block.String())
	b, err := json.Marshal(ts)
	require.NoError(t, err)
	assert.JSONEq(t, `{"block":"latest"}`, string(b))

	err = json.Unmarshal([]byte(`{"block":"12345"}`), &ts)
	require.NoError(t, err)
	assert.Equal(t, "0x3039", ts.Block.String())
	b, err = json.Marshal(ts)
	require.NoError(t, err)
	assert.JSONEq(t, `{"block":"0x3039"}`, string(b))

	err = json.Unmarshal([]byte(`{"block":12345}`), &ts)
	require.NoError(t, err)
	assert.Equal(t, "0x3039", ts.Block.String())
	b, err = json.Marshal(ts)
	require.NoError(t, err)
	assert.JSONEq(t, `{"block":"0x3039"}`, string(b))

	err = json.Unmarshal([]byte(`{"block":false}`), &ts)
	assert.Regexp(t, "PD020002", err)
}
