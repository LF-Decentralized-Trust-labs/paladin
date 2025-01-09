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

	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
)

func TestTransactionStatesHasUnavailable(t *testing.T) {
	assert.False(t, (&TransactionStates{}).HasUnavailable())
	assert.False(t, (&TransactionStates{
		Unavailable: &UnavailableStates{},
	}).HasUnavailable())
	assert.True(t, (&TransactionStates{
		Unavailable: &UnavailableStates{
			Confirmed: []tktypes.HexBytes{tktypes.RandBytes(32)},
		},
	}).HasUnavailable())
	assert.True(t, (&TransactionStates{
		Unavailable: &UnavailableStates{
			Spent: []tktypes.HexBytes{tktypes.RandBytes(32)},
		},
	}).HasUnavailable())
	assert.True(t, (&TransactionStates{
		Unavailable: &UnavailableStates{
			Read: []tktypes.HexBytes{tktypes.RandBytes(32)},
		},
	}).HasUnavailable())
	assert.True(t, (&TransactionStates{
		Unavailable: &UnavailableStates{
			Info: []tktypes.HexBytes{tktypes.RandBytes(32)},
		},
	}).HasUnavailable())
}
