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
	"context"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate64SafeCharsStartEndAlphaNum(t *testing.T) {

	err := ValidateSafeCharsStartEndAlphaNum(context.Background(), "good.n_ess-4", DefaultNameMaxLen, "name")
	require.NoError(t, err)

	err = ValidateSafeCharsStartEndAlphaNum(context.Background(), "good.n_ess-4", DefaultNameMaxLen, "name")
	require.NoError(t, err)

	err = ValidateSafeCharsStartEndAlphaNum(context.Background(), "i_domain_mgr_0x91c02c04d77f397c4153f177736ebd19939bad5a4ee3849e1c70adbc96c2c9bb", DefaultNameMaxLen, "name")
	require.NoError(t, err)

	err = ValidateSafeCharsStartEndAlphaNum(context.Background(), "_wrong", DefaultNameMaxLen, "name")
	assert.Regexp(t, "PD020005.*name", err)

	err = ValidateSafeCharsStartEndAlphaNum(context.Background(), "w!ong", DefaultNameMaxLen, "name")
	assert.Regexp(t, "PD020005.*name", err)

	err = ValidateSafeCharsStartEndAlphaNum(context.Background(), "01234567890123456789012345678901234567890123456789012345678901234", 64, "name")
	assert.Regexp(t, "PD020005.*name", err)

	err = ValidateSafeCharsStartEndAlphaNum(context.Background(), "", DefaultNameMaxLen, "name")
	assert.Regexp(t, "PD020005.*name", err)
	assert.True(t, unicode.IsLetter('À'))
	err = ValidateSafeCharsStartEndAlphaNum(context.Background(), "not_Àll_ascii", DefaultNameMaxLen, "name")
	assert.Regexp(t, "PD020005.*name", err)
}
