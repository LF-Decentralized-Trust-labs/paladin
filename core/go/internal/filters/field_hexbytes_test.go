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

func TestHexBytesField(t *testing.T) {

	ctx := context.Background()

	_, err := HexBytesField("test").SQLValue(ctx, (pldtypes.RawJSON)(`!json`))
	assert.Error(t, err)

	_, err = HexBytesField("test").SQLValue(ctx, (pldtypes.RawJSON)(`[]`))
	assert.Regexp(t, "PD010705", err)

	_, err = HexBytesField("test").SQLValue(ctx, (pldtypes.RawJSON)(`"not hex"`))
	assert.Regexp(t, "PD010711", err)

	v, err := HexBytesField("test").SQLValue(ctx, (pldtypes.RawJSON)(`"0xAAbbCCdd"`))
	require.NoError(t, err)
	assert.Equal(t, "aabbccdd", v)
	assert.Equal(t, "test", HexBytesField("test").SQLColumn())

	nv, err := HexBytesField("test").SQLValue(ctx, (pldtypes.RawJSON)(`null`))
	require.NoError(t, err)
	assert.Nil(t, nv)

	assert.False(t, HexBytesField("test").SupportsLIKE())

}
