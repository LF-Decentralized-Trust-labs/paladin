/*
 * Copyright © 2025 Kaleido, Inc.
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

package sequencer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func NewContractSequencerAgentForUnitTesting(t *testing.T) *contractSequencerAgent {
	return &contractSequencerAgent{
		isCoordinator: false,
	}
}

func TestIsObserver(t *testing.T) {
	csa := NewContractSequencerAgentForUnitTesting(t)
	// should always be in observing state while it is in memory
	assert.True(t, csa.IsObserver())
}

func TestIsControllerFalse(t *testing.T) {
	csa := NewContractSequencerAgentForUnitTesting(t)
	// should always be in observing state while it is in memory
	assert.False(t, csa.IsCoordinator())
}

func TestIsControllerTrue(t *testing.T) {
	csa := NewContractSequencerAgentForUnitTesting(t)
	csa.isCoordinator = true
	// should always be in observing state while it is in memory
	assert.True(t, csa.IsCoordinator())
}

func TestHandleCoordinatorHeartbeatNotification(t *testing.T) {
	ctx := context.Background()
	csa := NewContractSequencerAgentForUnitTesting(t)
	chn := &CoordinatorHeartbeatNotification{
		From: "test",
	}
	err := csa.HandleCoordinatorHeartbeatNotification(ctx, chn)
	assert.NoError(t, err)
	assert.Equal(t, "test", csa.activeCoordinator)
}
