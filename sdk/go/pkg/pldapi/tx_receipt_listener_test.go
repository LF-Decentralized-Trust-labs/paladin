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

package pldapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIncompleteStateReceiptBehavior(t *testing.T) {
	require.Equal(t, IncompleteStateReceiptBehaviorProcess, IncompleteStateReceiptBehaviorProcess.Enum().V())
	def, err := IncompleteStateReceiptBehavior("").Enum().Validate()
	require.NoError(t, err)
	require.Equal(t, IncompleteStateReceiptBehaviorBlockContract.Default(), string(def))
	_, err = IncompleteStateReceiptBehavior("wrong").Enum().Validate()
	require.Regexp(t, "PD020003", err)

	// Test the complete_only option
	require.Equal(t, IncompleteStateReceiptBehaviorCompleteOnly, IncompleteStateReceiptBehaviorCompleteOnly.Enum().V())
	completeOnly, err := IncompleteStateReceiptBehavior("complete_only").Enum().Validate()
	require.NoError(t, err)
	require.Equal(t, "complete_only", string(completeOnly))

	// Test that all options are available
	options := IncompleteStateReceiptBehaviorBlockContract.Options()
	require.Contains(t, options, "block_contract")
	require.Contains(t, options, "process")
	require.Contains(t, options, "complete_only")
}
