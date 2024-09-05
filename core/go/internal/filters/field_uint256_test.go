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

	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
)

func TestUint256Field(t *testing.T) {

	ctx := context.Background()

	_, err := Uint256Field("test").SQLValue(ctx, (tktypes.RawJSON)(`!json`))
	assert.Error(t, err)

	_, err = Uint256Field("test").SQLValue(ctx, (tktypes.RawJSON)(`[]`))
	assert.Regexp(t, "FF22091", err)

	vBigPos, err := Uint256Field("test").SQLValue(ctx, (tktypes.RawJSON)(`"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"`))
	assert.NoError(t, err)
	assert.Equal(t, "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", vBigPos)
	assert.Len(t, vBigPos, 64)

	vZero, err := Uint256Field("test").SQLValue(ctx, (tktypes.RawJSON)(`0`))
	assert.NoError(t, err)
	assert.Equal(t, "0000000000000000000000000000000000000000000000000000000000000000", vZero)
	assert.Len(t, vZero, 64)

	vSmallPos, err := Uint256Field("test").SQLValue(ctx, (tktypes.RawJSON)(`12345`))
	assert.NoError(t, err)
	assert.Equal(t, "0000000000000000000000000000000000000000000000000000000000003039", vSmallPos)
	assert.Len(t, vSmallPos, 64)

	nv, err := Uint256Field("test").SQLValue(ctx, (tktypes.RawJSON)(`null`))
	assert.NoError(t, err)
	assert.Nil(t, nv)

	assert.False(t, Uint256Field("test").SupportsLIKE())

}
