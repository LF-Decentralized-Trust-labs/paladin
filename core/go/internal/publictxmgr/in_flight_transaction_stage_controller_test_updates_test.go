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
	"errors"
	"testing"
	"time"

	"github.com/kaleido-io/paladin/core/mocks/publictxmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// add updates to the array and check how they get handled - cover all the cases where the way that
// a previous version is handled is different to if it is the current version

func TestTXStageControllerUpdateValidationError(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, _ := newInflightTransaction(o, 1, func(tx *DBPublicTxn) {
		tx.FixedGasPricing = tktypes.JSONString(pldapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(10),
		})
	})
	it.testOnlyNoActionMode = true

	response := make(chan error, 1)
	it.UpdateTransaction(ctx, &DBPublicTxn{Gas: 1000}, func() error { return nil }, response)

	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{})

	require.Len(t, it.stateManager.GetVersions(ctx), 1)

	// the response should be available on the channel before ProduceLatestInFlightStageContext returns
	select {
	case err := <-response:
		require.ErrorContains(t, err, "PD011940")
	default:
		t.Fail()
	}
}

func TestTXStageControllerUpdateDBError(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, _ := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true

	response := make(chan error, 1)
	it.UpdateTransaction(ctx, &DBPublicTxn{Gas: 1000}, func() error { return errors.New("bang") }, response)

	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{})

	require.Len(t, it.stateManager.GetVersions(ctx), 1)

	rsc := it.stateManager.GetCurrentVersion(ctx).GetRunningStageContext(ctx)
	require.NotNil(t, rsc)
	assert.Equal(t, InFlightTxStageRetrieveGasPrice, rsc.Stage)

	// the response should be available on the channel before ProduceLatestInFlightStageContext returns
	select {
	case err := <-response:
		require.EqualError(t, err, "bang")
	default:
		t.Fail()
	}
}

func TestTXStageControllerUpdateNoRunningStageContext(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, _ := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true

	response := make(chan error, 1)
	it.UpdateTransaction(ctx, &DBPublicTxn{
		Gas: 1000,
		FixedGasPricing: tktypes.JSONString(pldapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(10),
		}),
	}, func() error { return nil }, response)

	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{})

	require.Len(t, it.stateManager.GetVersions(ctx), 2)

	assert.Nil(t, it.stateManager.GetVersion(ctx, 0).GetRunningStageContext(ctx))

	rsc := it.stateManager.GetCurrentVersion(ctx).GetRunningStageContext(ctx)
	require.NotNil(t, rsc)
	assert.Equal(t, InFlightTxStageSigning, rsc.Stage)

	// the response should be available on the channel before ProduceLatestInFlightStageContext returns
	select {
	case err := <-response:
		require.NoError(t, err)
	default:
		t.Fail()
	}

}

func TestTXStageControllerUpdateRunningStagePersistance(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, _ := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	inMemoryTxState := it.stateManager.(*inFlightTransactionState).InMemoryTxStateManager

	// use a time from yesterday and tomorrow to clearly force the if and else in the retry timeout
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	tomorrow := now.AddDate(0, 0, 1)

	// set the existing version in a persistence stage
	previousVersion := it.stateManager.GetCurrentVersion(ctx).(*inFlightTransactionStateVersion)
	previousVersion.runningStageContext = NewRunningStageContext(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived, inMemoryTxState)
	previousVersion.bufferedStageOutputs = []*StageOutput{{
		Stage: InFlightTxStageSubmitting,
		PersistenceOutput: &PersistenceOutput{
			PersistenceError: errors.New("bang"),
			Time:             tomorrow,
		},
	}}

	response := make(chan error, 1)
	it.UpdateTransaction(ctx, &DBPublicTxn{Gas: 1000}, func() error { return nil }, response)
	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{})

	require.Len(t, it.stateManager.GetVersions(ctx), 2)

	// persistence hasn't yet reached the timeout- the stage output stays unprocessed
	require.Len(t, previousVersion.bufferedStageOutputs, 1)
	assert.Equal(t, &StageOutput{
		Stage: InFlightTxStageSubmitting,
		PersistenceOutput: &PersistenceOutput{
			PersistenceError: errors.New("bang"),
			Time:             tomorrow,
		},
	}, previousVersion.bufferedStageOutputs[0])

	// the response should be available on the channel before ProduceLatestInFlightStageContext returns
	select {
	case err := <-response:
		require.NoError(t, err)
	default:
		t.Fail()
	}

	// change the time to be past the timeout- the persistence is triggered again and removed
	previousVersion.bufferedStageOutputs[0].PersistenceOutput.Time = yesterday
	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{})

	require.Len(t, it.stateManager.GetVersions(ctx), 2)
	require.Len(t, previousVersion.bufferedStageOutputs, 0)

	// updates should have been cleared- we're not expecting anything on the channel
	select {
	case <-response:
		t.Fail()
	default:
	}

}

func TestTXStageControllerUpdateIgnoreSigningErrorAfterPersisted(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, _ := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	inMemoryTxState := it.stateManager.(*inFlightTransactionState).InMemoryTxStateManager

	// set the existing version in a persistence stage
	previousVersion := it.stateManager.GetCurrentVersion(ctx).(*inFlightTransactionStateVersion)
	previousVersion.runningStageContext = NewRunningStageContext(ctx, InFlightTxStageSigning, BaseTxSubStatusReceived, inMemoryTxState)
	previousVersion.runningStageContext.StageOutput = &StageOutput{
		SignOutput: &SignOutputs{
			Err: errors.New("bang"),
		},
	}
	previousVersion.bufferedStageOutputs = []*StageOutput{{
		Stage:             InFlightTxStageSigning,
		PersistenceOutput: &PersistenceOutput{},
	}}

	response := make(chan error, 1)
	it.UpdateTransaction(ctx, &DBPublicTxn{Gas: 1000}, func() error { return nil }, response)
	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{})

	require.Len(t, it.stateManager.GetVersions(ctx), 2)
	assert.Len(t, previousVersion.bufferedStageOutputs, 0)
	assert.Nil(t, previousVersion.GetRunningStageContext(ctx))
	assert.False(t, it.stateManager.GetCurrentVersion(ctx).GetRunningStageContext(ctx).StageErrored)

	// the response should be available on the channel before ProduceLatestInFlightStageContext returns
	select {
	case err := <-response:
		require.NoError(t, err)
	default:
		t.Fail()
	}
}

func TestTXStageControllerUpdateNoSubmit(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, _ := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	inMemoryTxState := it.stateManager.(*inFlightTransactionState).InMemoryTxStateManager

	// set the existing version in a sign persisted stage
	previousVersion := it.stateManager.GetCurrentVersion(ctx).(*inFlightTransactionStateVersion)
	previousVersion.runningStageContext = NewRunningStageContext(ctx, InFlightTxStageSigning, BaseTxSubStatusReceived, inMemoryTxState)
	previousVersion.runningStageContext.StageOutput = &StageOutput{
		SignOutput: &SignOutputs{
			SignedMessage: []byte("signed message"),
		},
	}
	previousVersion.bufferedStageOutputs = []*StageOutput{{
		Stage:             InFlightTxStageSigning,
		PersistenceOutput: &PersistenceOutput{},
	}}

	// assigning the mocks is enough to check that no action gets called
	previousVersion.InFlightStageActionTriggers = publictxmocks.NewInFlightStageActionTriggers(t)

	response := make(chan error, 1)
	it.UpdateTransaction(ctx, &DBPublicTxn{Gas: 1000}, func() error { return nil }, response)
	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{})

	require.Len(t, it.stateManager.GetVersions(ctx), 2)
	assert.Len(t, previousVersion.bufferedStageOutputs, 0)
	assert.Nil(t, previousVersion.GetRunningStageContext(ctx))

	// the response should be available on the channel before ProduceLatestInFlightStageContext returns
	select {
	case err := <-response:
		require.NoError(t, err)
	default:
		t.Fail()
	}
}

func TestTXStageControllerUpdateNoResubmit(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, _ := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	inMemoryTxState := it.stateManager.(*inFlightTransactionState).InMemoryTxStateManager

	// set the existing version in a submit persisted stage
	previousVersion := it.stateManager.GetCurrentVersion(ctx).(*inFlightTransactionStateVersion)
	previousVersion.runningStageContext = NewRunningStageContext(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived, inMemoryTxState)
	previousVersion.runningStageContext.StageOutput = &StageOutput{
		SubmitOutput: &SubmitOutputs{
			Err: errors.New("bang"),
		},
	}
	previousVersion.bufferedStageOutputs = []*StageOutput{{
		Stage:             InFlightTxStageSubmitting,
		PersistenceOutput: &PersistenceOutput{},
	}}

	response := make(chan error, 1)
	it.UpdateTransaction(ctx, &DBPublicTxn{Gas: 1000}, func() error { return nil }, response)
	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{})

	require.Len(t, it.stateManager.GetVersions(ctx), 2)
	assert.Len(t, previousVersion.bufferedStageOutputs, 0)
	assert.Nil(t, previousVersion.GetRunningStageContext(ctx))
	assert.False(t, it.stateManager.GetCurrentVersion(ctx).GetRunningStageContext(ctx).StageErrored)

	// the response should be available on the channel before ProduceLatestInFlightStageContext returns
	select {
	case err := <-response:
		require.NoError(t, err)
	default:
		t.Fail()
	}
}

func TestTXStageControllerComplete(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, _ := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true

	mtx := it.stateManager.(*inFlightTransactionState).InMemoryTxStateManager.(*inMemoryTxState).mtx
	mtx.InFlightStatus = InFlightStatusConfirmReceived

	response := make(chan error, 1)
	it.UpdateTransaction(ctx, nil, nil, response)

	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{})

	// the response should be available on the channel before ProduceLatestInFlightStageContext returns
	select {
	case err := <-response:
		require.ErrorContains(t, err, "PD011937")
	default:
		t.Fail()
	}
}
