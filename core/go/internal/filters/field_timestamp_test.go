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

	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimestampField(t *testing.T) {

	ctx := context.Background()

	_, err := TimestampField("test").SQLValue(ctx, (pldtypes.RawJSON)(`!json`))
	assert.Error(t, err)

	_, err = TimestampField("test").SQLValue(ctx, (pldtypes.RawJSON)(`"not a valid timestamp"`))
	assert.Regexp(t, "PD020019", err)

	nv, err := TimestampField("test").SQLValue(ctx, (pldtypes.RawJSON)(`null`))
	require.NoError(t, err)
	assert.Nil(t, nv)

	assert.False(t, TimestampField("test").SupportsLIKE())

}
