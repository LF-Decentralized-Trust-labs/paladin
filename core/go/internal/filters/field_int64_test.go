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
	"math/big"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInt64Field(t *testing.T) {

	ctx := context.Background()

	_, err := Int64Field("test").SQLValue(ctx, (pldtypes.RawJSON)(`!json`))
	assert.Error(t, err)

	_, err = Int64Field("test").SQLValue(ctx, (pldtypes.RawJSON)(`[]`))
	assert.Regexp(t, "PD010703", err)

	// Too big to fit (by 1)
	tooBig := new(big.Int).Add(big.NewInt(9223372036854775807), big.NewInt(1))
	_, err = Int64Field("test").SQLValue(ctx, pldtypes.JSONString(tooBig.String()))
	assert.Regexp(t, "PD010703", err)

	// We handle bool -> Int64 conversion
	iTrue, err := Int64Field("test").SQLValue(ctx, (pldtypes.RawJSON)(`true`))
	require.NoError(t, err)
	assert.Equal(t, (int64)(1), iTrue)
	iFalse, err := Int64Field("test").SQLValue(ctx, (pldtypes.RawJSON)(`false`))
	require.NoError(t, err)
	assert.Equal(t, (int64)(0), iFalse)

	nv, err := Int64Field("test").SQLValue(ctx, (pldtypes.RawJSON)(`null`))
	require.NoError(t, err)
	assert.Nil(t, nv)

	assert.False(t, Int64Field("test").SupportsLIKE())

}
