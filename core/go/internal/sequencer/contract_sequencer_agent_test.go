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

	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
)

type contractSequencerAgentDependencies struct {
	coordinatorSelector *MockCoordinatorSelector
	transportManager    *MockTransportManager
}

func NewContractSequencerAgentForUnitTesting(t *testing.T, committee []string) (*contractSequencerAgent, *contractSequencerAgentDependencies) {

	mockCoordinatorSelector := NewMockCoordinatorSelector(t)
	mockCoordinatorSelector.On("Initialize", mock.Anything, mock.Anything).Return(nil).Maybe()

	mockTransportManager := NewMockTransportManager(t)
	nodeName := "testNode"

	contractAddress := tktypes.RandAddress()

	return NewContractSequencerAgent(context.Background(), nodeName, mockTransportManager, mockCoordinatorSelector, committee, contractAddress).(*contractSequencerAgent), &contractSequencerAgentDependencies{
		coordinatorSelector: mockCoordinatorSelector,
		transportManager:    mockTransportManager,
	}
}

func TestIsObserver(t *testing.T) {
	csa, _ := NewContractSequencerAgentForUnitTesting(t, nil)
	// should always be in observing state while it is in memory
	assert.True(t, csa.IsObserver())
}

func TestIsCoordinatorFalse(t *testing.T) {
	csa, _ := NewContractSequencerAgentForUnitTesting(t, nil)
	assert.False(t, csa.IsCoordinator())
}

func TestIsCoordinatorTrueIfActive(t *testing.T) {
	csa, _ := NewContractSequencerAgentForUnitTesting(t, nil)
	csa.coordinator.state = CoordinatorState_Active
	assert.True(t, csa.IsCoordinator())
}

func TestIsCoordinatorTrueIfStandby(t *testing.T) {
	csa, _ := NewContractSequencerAgentForUnitTesting(t, nil)
	csa.coordinator.state = CoordinatorState_Standby
	assert.True(t, csa.IsCoordinator())
}

func TestIsCoordinatorTrueIfElect(t *testing.T) {
	csa, _ := NewContractSequencerAgentForUnitTesting(t, nil)
	csa.coordinator.state = CoordinatorState_Elect
	assert.True(t, csa.IsCoordinator())
}

func TestIsCoordinatorTrueIfPrepared(t *testing.T) {
	csa, _ := NewContractSequencerAgentForUnitTesting(t, nil)
	csa.coordinator.state = CoordinatorState_Prepared
	assert.True(t, csa.IsCoordinator())
}

func TestHandleCoordinatorHeartbeatNotification(t *testing.T) {
	ctx := context.Background()
	csa, mocks := NewContractSequencerAgentForUnitTesting(t, nil)
	mocks.transportManager.On("Send", ctx, mock.Anything).Return(nil).Once()
	chn := &CoordinatorHeartbeatNotification{
		From: "test",
	}
	err := csa.HandleCoordinatorHeartbeatNotification(ctx, chn)
	assert.NoError(t, err)
	assert.Equal(t, "test", csa.activeCoordinator)
}
