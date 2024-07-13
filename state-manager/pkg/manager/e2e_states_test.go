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

package manager

import (
	"github.com/kaleido-io/paladin-state-manager/pkg/apitypes"
	"github.com/stretchr/testify/assert"
)

func (e *E2ESuite) TestStatesCRUD() {
	t := e.T()

	// Create an initial version
	created, err := e.mgr.UpsertState(e.ctx, &apitypes.State{
		ID:    ptrTo("state1"),
		State: &apitypes.StateProposed,
	})
	assert.NoError(t, err)
	assert.True(t, created)

	// Get it back
	states, _, err := e.mgr.ListStates(e.ctx, apitypes.StateFilters.NewFilter(e.ctx).And())
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	state1 := states[0]
	assert.Equal(t, "state1", *state1.ID)
}
