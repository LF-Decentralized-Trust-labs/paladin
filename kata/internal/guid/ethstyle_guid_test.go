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

package guid

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGUIDValidity(t *testing.T) {

	var u1 *GUID
	assert.False(t, u1.IsValid())
	assert.Equal(t, uuid.Nil, u1.UUID())
	assert.Equal(t, "0x0000000000000000000000000000000000000000", u1.Address0xHex().String())

	ctx := context.Background()
	_, err := ParseGUID(ctx, u1.String())
	assert.Regexp(t, "len", err)

	var u2 GUID
	assert.False(t, u2.IsValid())

	_, err = ParseGUID(ctx, u2.String())
	assert.Regexp(t, "PD010100", err)

	u3 := NewGUID()
	assert.True(t, u3.IsValid())

	u4, err := ParseGUID(ctx, u3.String())
	assert.NoError(t, err)

	assert.Equal(t, u3, *u4)

	assert.Panics(t, func() {
		MustParseGUID("wrong")
	})

	u5 := MustParseGUID("a7522ea100000000000000000000000000000000")
	assert.Regexp(t, "PD010102", u5.CheckValid(ctx))

	u6 := MustParseGUID("a7522ea1ffeeddccbbaa99887766554433221100")
	assert.NoError(t, err)
	assert.Equal(t, "ffeeddcc-bbaa-9988-7766-554433221100", u6.UUID().String())
	assert.NoError(t, err)
	assert.Equal(t, "0xa7522ea1ffeeddccbbaa99887766554433221100", u6.Address0xHex().String())

}
