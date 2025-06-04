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

package publictxmgr

import (
	"context"
	"math/big"
	"testing"

	"github.com/kaleido-io/paladin/core/mocks/publictxmocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestInFlightTransactionStateManager(t *testing.T) (InFlightTransactionStateManager, func()) {
	_, balanceManager, ptm, _, done := newTestBalanceManager(t)

	mockInMemoryState := NewTestInMemoryTxState(t)
	mockActionTriggers := publictxmocks.NewInFlightStageActionTriggers(t)
	iftxs := NewInFlightTransactionStateManager(&publicTxEngineMetrics{}, balanceManager, mockActionTriggers, mockInMemoryState, ptm, ptm.submissionWriter, false)
	return iftxs, done

}

func TestStateManagerBasicLifecycle(t *testing.T) {
	ctx := context.Background()
	stateManager, done := newTestInFlightTransactionStateManager(t)
	defer done()
	stateManager.SetOrchestratorContext(ctx, &OrchestratorContext{})

	// check it has one generation which is current
	require.Len(t, stateManager.GetGenerations(ctx), 1)
	assert.True(t, stateManager.GetGeneration(ctx, 0).IsCurrent(ctx))

	// add a new generation, check that one becomes current and the previous one is not
	stateManager.NewGeneration(ctx)
	require.Len(t, stateManager.GetGenerations(ctx), 2)
	assert.False(t, stateManager.GetGeneration(ctx, 0).IsCurrent(ctx))
	assert.True(t, stateManager.GetGeneration(ctx, 1).IsCurrent(ctx))

	// check removal conditions
	mtx := stateManager.(*inFlightTransactionState).InMemoryTxStateManager.(*inMemoryTxState).mtx
	mtx.InFlightStatus = InFlightStatusPending
	currentGeneration := stateManager.GetCurrentGeneration(ctx).(*inFlightTransactionStateGeneration)
	currentGeneration.runningStageContext = NewRunningStageContext(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived, stateManager.(*inFlightTransactionState).InMemoryTxStateManager)
	assert.False(t, stateManager.CanBeRemoved(ctx))

	stateManager.GetCurrentGeneration(ctx).ClearRunningStageContext(ctx)
	assert.False(t, stateManager.CanBeRemoved(ctx))

	mtx.InFlightStatus = InFlightStatusConfirmReceived
	assert.True(t, stateManager.CanBeRemoved(ctx))
}

func TestStateManagerStageManagementCanSubmit(t *testing.T) {
	ctx := context.Background()
	stateManager, done := newTestInFlightTransactionStateManager(t)
	defer done()
	stateManager.SetOrchestratorContext(ctx, &OrchestratorContext{
		PreviousNonceCostUnknown: false,
		// no availableToSpend provided, this means we don't need to check balance
	})
	assert.True(t, stateManager.CanSubmit(context.Background(), big.NewInt(0)))
	stateManager.SetOrchestratorContext(ctx, &OrchestratorContext{
		PreviousNonceCostUnknown: false,
		AvailableToSpend:         big.NewInt(30),
	})
	assert.True(t, stateManager.CanSubmit(context.Background(), big.NewInt(29)))
	assert.True(t, stateManager.CanSubmit(context.Background(), big.NewInt(30)))
	assert.False(t, stateManager.CanSubmit(context.Background(), big.NewInt(31)))
	assert.False(t, stateManager.CanSubmit(context.Background(), nil)) //unknown cost for the current transaction

	stateManager.SetOrchestratorContext(ctx, &OrchestratorContext{
		PreviousNonceCostUnknown: true,
		AvailableToSpend:         big.NewInt(30),
	})

	assert.False(t, stateManager.CanSubmit(context.Background(), big.NewInt(29)))

}
