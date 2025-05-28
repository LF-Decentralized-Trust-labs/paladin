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
package transaction

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/mocks/sequencermocks"
	"github.com/stretchr/testify/assert"
)

func TestStateMachine_InitializeOK(t *testing.T) {
	ctx := context.Background()

	messageSender := NewMockMessageSender(t)
	clock := &common.FakeClockForTesting{}
	engineIntegration := sequencermocks.NewEngineIntegration(t)
	txn, err := NewTransaction(
		ctx,
		"sender@node1",
		&components.PrivateTransaction{
			ID: uuid.New(),
		},
		messageSender,
		clock,
		func(event common.Event) {
			//don't expect any events during initialize
			assert.Failf(t, "unexpected event", "%T", event)
		},
		engineIntegration,
		clock.Duration(1000),
		clock.Duration(5000),
		5,
		NewGrapher(ctx),
		nil,
		func(context.Context) {}, // onCleanup function, not used in tests
	)
	assert.NoError(t, err)
	assert.NotNil(t, txn)

	assert.Equal(t, State_Initial, txn.GetCurrentState(), "current state is %s", txn.GetCurrentState().String())
}
