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

func TestBooleanField(t *testing.T) {

	ctx := context.Background()

	assert.Equal(t, "test", BooleanField("test").SQLColumn())

	_, err := BooleanField("test").SQLValue(ctx, (pldtypes.RawJSON)(`!json`))
	assert.Error(t, err)

	_, err = BooleanField("test").SQLValue(ctx, (pldtypes.RawJSON)(`[]`))
	assert.Regexp(t, "PD010704", err)

	nv, err := BooleanField("test").SQLValue(ctx, (pldtypes.RawJSON)(`null`))
	require.NoError(t, err)
	assert.Nil(t, nv)

	v, err := BooleanField("test").SQLValue(ctx, (pldtypes.RawJSON)(`true`))
	assert.NoError(t, err)
	assert.True(t, v.(bool))

	v, err = BooleanField("test").SQLValue(ctx, (pldtypes.RawJSON)(`"true"`))
	assert.NoError(t, err)
	assert.True(t, v.(bool))

	v, err = BooleanField("test").SQLValue(ctx, (pldtypes.RawJSON)(`"false"`))
	assert.NoError(t, err)
	assert.False(t, v.(bool))

	assert.False(t, BooleanField("test").SupportsLIKE())

}
