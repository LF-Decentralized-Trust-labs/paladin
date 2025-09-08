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

package filters

import (
	"context"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBytes32Field(t *testing.T) {

	ctx := context.Background()

	_, err := Bytes32Field("test").SQLValue(ctx, (pldtypes.RawJSON)(`!json`))
	assert.Error(t, err)

	_, err = Bytes32Field("test").SQLValue(ctx, (pldtypes.RawJSON)(`[]`))
	assert.Regexp(t, "PD010705", err)

	_, err = Bytes32Field("test").SQLValue(ctx, (pldtypes.RawJSON)(`"not hex"`))
	assert.Regexp(t, "PD010719", err)

	_, err = Bytes32Field("test").SQLValue(ctx, (pldtypes.RawJSON)(`"0xAAbbCCdd"`))
	assert.Regexp(t, "PD010719.*4", err)

	v, err := Bytes32Field("test").SQLValue(ctx, (pldtypes.RawJSON)(`"0x0001020304050607080910111213141516171819202122232425262728293031"`))
	require.NoError(t, err)
	assert.Equal(t, "0001020304050607080910111213141516171819202122232425262728293031", v)
	assert.Equal(t, "test", Bytes32Field("test").SQLColumn())

	nv, err := Bytes32Field("test").SQLValue(ctx, (pldtypes.RawJSON)(`null`))
	require.NoError(t, err)
	assert.Nil(t, nv)

	assert.False(t, Bytes32Field("test").SupportsLIKE())

}
