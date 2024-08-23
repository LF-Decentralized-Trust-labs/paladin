/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package engine

import (
	"context"
	"testing"

	"github.com/kaleido-io/paladin/kata/internal/engine/orchestrator"
	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Attempt to assert the behaviour of the Engine as a whole component in isolation from the rest of the system
// Tests in this file do not mock anything else in this package or sub packages but does mock other components and managers in paladin as per their interfaces

func TestEngine(t *testing.T) {
	ctx := context.Background()
	engine, _ := newEngineForTesting(t)
	assert.Equal(t, "Kata Engine", engine.Name())

	orchestrator, err := engine.NewOrchestrator(ctx, "0x1234", &orchestrator.OrchestratorConfig{})
	assert.NoError(t, err)
	require.NotNil(t, orchestrator)

}

type engineDependencyMocks struct {
	mockStateStore    *componentmocks.StateStore
	mockAllComponents *componentmocks.AllComponents
}

func newEngineForTesting(t *testing.T) (Engine, engineDependencyMocks) {
	mockStateStore := componentmocks.NewStateStore(t)
	mockAllComponents := componentmocks.NewAllComponents(t)

	return NewEngine(mockStateStore),
		engineDependencyMocks{
			mockAllComponents: mockAllComponents,
			mockStateStore:    mockStateStore,
		}

}
