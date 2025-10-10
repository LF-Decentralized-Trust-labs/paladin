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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUUIDField(t *testing.T) {

	ctx := context.Background()

	_, err := UUIDField("test").SQLValue(ctx, (pldtypes.RawJSON)(`!json`))
	assert.Error(t, err)

	_, err = UUIDField("test").SQLValue(ctx, (pldtypes.RawJSON)(`[]`))
	assert.Regexp(t, "PD010705", err)

	_, err = UUIDField("test").SQLValue(ctx, (pldtypes.RawJSON)(`"not uuid"`))
	assert.Regexp(t, "PD010720", err)

	v, err := UUIDField("test").SQLValue(ctx, (pldtypes.RawJSON)(`"F9E01529-6551-4C8E-8ACE-F1C4A6A1943F"`))
	require.NoError(t, err)
	assert.Equal(t, "f9e01529-6551-4c8e-8ace-f1c4a6a1943f", v.(uuid.UUID).String())
	assert.Equal(t, "test", UUIDField("test").SQLColumn())

	nv, err := UUIDField("test").SQLValue(ctx, (pldtypes.RawJSON)(`null`))
	require.NoError(t, err)
	assert.Nil(t, nv)

	assert.False(t, UUIDField("test").SupportsLIKE())

}
