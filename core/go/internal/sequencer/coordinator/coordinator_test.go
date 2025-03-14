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

package coordinator

import (
	"context"
	"testing"

	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/mocks/sequencermocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/require"
)

func TestTransactionStateTransition(t *testing.T) {

}

func NewCoordinatorForUnitTest(t *testing.T, ctx context.Context, committeeMembers []string) (*coordinator, *coordinatorDependencyMocks) {

	if committeeMembers == nil {
		committeeMembers = []string{"member1@node1"}
	}
	mocks := &coordinatorDependencyMocks{
		messageSender:     NewMockMessageSender(t),
		clock:             &common.FakeClockForTesting{},
		engineIntegration: sequencermocks.NewEngineIntegration(t),
		emit:              func(event common.Event) {},
	}

	coordinator, err := NewCoordinator(ctx, mocks.messageSender, committeeMembers, mocks.clock, mocks.emit, mocks.engineIntegration, mocks.clock.Duration(1000), mocks.clock.Duration(5000), 100, tktypes.RandAddress(), 5, 5)
	require.NoError(t, err)

	return coordinator, mocks
}

type coordinatorDependencyMocks struct {
	messageSender     *MockMessageSender
	clock             *common.FakeClockForTesting
	engineIntegration *sequencermocks.EngineIntegration
	emit              common.EmitEvent
}
