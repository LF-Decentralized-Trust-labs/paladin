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

package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"
	"github.com/kaleido-io/paladin/kata/mocks/enginemocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewOrchestratorProcessNewTransaction(t *testing.T) {
	ctx := context.Background()
	mockAllComponents := componentmocks.NewAllComponents(t)
	mockStateStore := componentmocks.NewStateStore(t)
	mockAllComponents.On("StateStore").Return(mockStateStore).Maybe()
	mockTransportManger := componentmocks.NewTransportManager(t)
	mockAllComponents.On("TransportManager").Return(mockTransportManger).Maybe()
	mockPublisher := enginemocks.NewPublisher(t)
	mockSequencer := enginemocks.NewSequencer(t)
	testOc := NewOrchestrator(ctx, uuid.Must(uuid.NewUUID()), "test_contract_address", &OrchestratorConfig{}, mockAllComponents, &componentmocks.DomainSmartContract{}, mockPublisher, mockSequencer)
	newTxID := uuid.New()
	testTx := &transactionstore.TransactionWrapper{
		Transaction: transactionstore.Transaction{
			ID: newTxID,
		},
		PrivateTransaction: &components.PrivateTransaction{
			ID: newTxID,
		},
	}

	waitForAction := make(chan bool, 1)

	// fake stage controller for testing
	mSC := enginemocks.StageController{}
	testOc.StageController = &mSC
	mSC.On("CalculateStage", ctx, testTx).Once().Return("test")
	mSC.On("PerformActionForStage", ctx, mock.Anything, mock.Anything).Once().Run(func(args mock.Arguments) {
		waitForAction <- true
	}).Return(nil /*no synchronous output*/, nil)

	assert.Empty(t, testOc.incompleteTxSProcessMap)

	// when incomplete tx is more than max concurrent
	testOc.maxConcurrentProcess = 0
	assert.True(t, testOc.ProcessNewTransaction(ctx, testTx))

	// gets add when the queue is not full
	testOc.maxConcurrentProcess = 10
	assert.Empty(t, testOc.incompleteTxSProcessMap)
	assert.False(t, testOc.ProcessNewTransaction(ctx, testTx))
	assert.Equal(t, 1, len(testOc.incompleteTxSProcessMap))

	stageContext := testOc.incompleteTxSProcessMap[testTx.GetTxID(ctx)].GetStageContext(ctx)
	<-waitForAction // no events emitted as no synchronous output was returned
	assert.NotNil(t, stageContext)

	// add again doesn't cause a repeat process of the current stage context
	assert.False(t, testOc.ProcessNewTransaction(ctx, testTx))
	assert.Equal(t, 1, len(testOc.incompleteTxSProcessMap))

	newStageContext := testOc.incompleteTxSProcessMap[testTx.GetTxID(ctx)].GetStageContext(ctx)
	assert.Equal(t, stageContext, newStageContext)
}

func TestOrchestratorHandleEvents(t *testing.T) {
	ctx := context.Background()
	mockAllComponents := componentmocks.NewAllComponents(t)
	mockStateStore := componentmocks.NewStateStore(t)
	mockAllComponents.On("StateStore").Return(mockStateStore).Maybe()
	mockTransportManger := componentmocks.NewTransportManager(t)
	mockAllComponents.On("TransportManager").Return(mockTransportManger).Maybe()
	mockPublisher := enginemocks.NewPublisher(t)
	mockSequencer := enginemocks.NewSequencer(t)
	testOc := NewOrchestrator(ctx, uuid.Must(uuid.NewUUID()), "test_contract_address", &OrchestratorConfig{}, mockAllComponents, &componentmocks.DomainSmartContract{}, mockPublisher, mockSequencer)
	newTxID := uuid.New()
	testTx := &transactionstore.TransactionWrapper{
		Transaction: transactionstore.Transaction{
			ID: newTxID,
		},
		PrivateTransaction: &components.PrivateTransaction{
			ID: newTxID,
		},
	}

	waitForAction := make(chan bool, 1)

	// fake stage controller for testing
	mSC := enginemocks.StageController{}
	testOc.StageController = &mSC
	mSC.On("CalculateStage", ctx, testTx).Once().Return("test")
	mSC.On("PerformActionForStage", ctx, mock.Anything, mock.Anything).Once().Run(func(args mock.Arguments) {
		waitForAction <- true
	}).Return(nil /*no synchronous output*/, nil)

	assert.Empty(t, testOc.incompleteTxSProcessMap)

	// gets added when the queue is not full
	testOc.maxConcurrentProcess = 1
	assert.Empty(t, testOc.incompleteTxSProcessMap)
	assert.False(t, testOc.ProcessNewTransaction(ctx, testTx))
	assert.Equal(t, 1, len(testOc.incompleteTxSProcessMap))

	stageContext := testOc.incompleteTxSProcessMap[testTx.GetTxID(ctx)].GetStageContext(ctx)
	<-waitForAction // no events emitted as no synchronous output was returned
	assert.NotNil(t, stageContext)

	waitForProcessEvent := make(chan bool, 1)
	// feed in an event for process
	mSC.On("ProcessEventsForStage", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Once().Run(func(args mock.Arguments) {
		waitForProcessEvent <- true
	}).Return(nil, nil, types.NextStepWait /*just wait, don't trigger new stage etc*/)
	testOc.HandleEvent(ctx, &types.StageEvent{
		ID:    uuid.NewString(),
		Stage: "test",
		TxID:  testTx.GetTxID(ctx),
		Data:  "test",
	})
	<-waitForProcessEvent
	newStageContext := testOc.incompleteTxSProcessMap[testTx.GetTxID(ctx)].GetStageContext(ctx)
	assert.Equal(t, stageContext, newStageContext)

	delete(testOc.incompleteTxSProcessMap, testTx.GetTxID(ctx)) // clean up the queue
	assert.Empty(t, testOc.incompleteTxSProcessMap)

	// trigger again which should initiate the tx processor
	mSC.On("CalculateStage", ctx, testTx).Once().Return("test")
	mSC.On("ProcessEventsForStage", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Once().Run(func(args mock.Arguments) {
		waitForProcessEvent <- true
	}).Return(nil, nil, types.NextStepWait /*just wait, don't trigger new stage etc*/)
	testOc.HandleEvent(ctx, &types.StageEvent{
		ID:    uuid.NewString(),
		Stage: "test",
		TxID:  testTx.GetTxID(ctx),
		Data:  "test",
	})
	<-waitForProcessEvent
	newStageContext = testOc.incompleteTxSProcessMap[testTx.GetTxID(ctx)].GetStageContext(ctx)
	assert.NotEqual(t, stageContext, newStageContext)

}

func TestOrchestratorPollingLoopStop(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mockAllComponents := componentmocks.NewAllComponents(t)
	mockStateStore := componentmocks.NewStateStore(t)
	mockAllComponents.On("StateStore").Return(mockStateStore).Maybe()
	mockTransportManger := componentmocks.NewTransportManager(t)
	mockAllComponents.On("TransportManager").Return(mockTransportManger).Maybe()

	mockPublisher := enginemocks.NewPublisher(t)
	mockSequencer := enginemocks.NewSequencer(t)
	testOc := NewOrchestrator(ctx, uuid.Must(uuid.NewUUID()), "test_contract_address", &OrchestratorConfig{}, mockAllComponents, &componentmocks.DomainSmartContract{}, mockPublisher, mockSequencer)
	ocDone, err := testOc.Start(ctx)
	assert.NoError(t, err)
	testOc.TriggerOrchestratorEvaluation()
	testOc.Stop()
	<-ocDone
}

func TestOrchestratorPollingLoopCancelContext(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	mockAllComponents := componentmocks.NewAllComponents(t)
	mockStateStore := componentmocks.NewStateStore(t)
	mockAllComponents.On("StateStore").Return(mockStateStore).Maybe()
	mockTransportManger := componentmocks.NewTransportManager(t)
	mockAllComponents.On("TransportManager").Return(mockTransportManger).Maybe()
	mockPublisher := enginemocks.NewPublisher(t)
	mockSequencer := enginemocks.NewSequencer(t)
	testOc := NewOrchestrator(ctx, uuid.Must(uuid.NewUUID()), "test_contract_address", &OrchestratorConfig{}, mockAllComponents, &componentmocks.DomainSmartContract{}, mockPublisher, mockSequencer)

	cancel()
	ocDone, err := testOc.Start(ctx)
	assert.NoError(t, err)
	<-ocDone
}

func TestOrchestratorPollingLoopRemoveCompletedTx(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	newTxID := uuid.New()
	testTx := &transactionstore.TransactionWrapper{
		Transaction: transactionstore.Transaction{
			ID: newTxID,
		},
		PrivateTransaction: &components.PrivateTransaction{
			ID: newTxID,
		},
	}
	mockAllComponents := componentmocks.NewAllComponents(t)
	mockStateStore := componentmocks.NewStateStore(t)
	mockAllComponents.On("StateStore").Return(mockStateStore).Maybe()
	mockTransportManger := componentmocks.NewTransportManager(t)
	mockAllComponents.On("TransportManager").Return(mockTransportManger).Maybe()
	mockPublisher := enginemocks.NewPublisher(t)
	mockSequencer := enginemocks.NewSequencer(t)
	testOc := NewOrchestrator(ctx, uuid.Must(uuid.NewUUID()), "test_contract_address", &OrchestratorConfig{}, mockAllComponents, &componentmocks.DomainSmartContract{}, mockPublisher, mockSequencer)
	mSC := enginemocks.StageController{}
	testOc.StageController = &mSC

	ocDone, err := testOc.Start(ctx)
	assert.NoError(t, err)
	waitForAction := make(chan bool, 1)
	mSC.On("GetAllStages").Maybe().Return([]string{"test"})
	mSC.On("CalculateStage", ctx, testTx).Once().Return("remove")
	mSC.On("PerformActionForStage", ctx, mock.Anything, mock.Anything).Once().Run(func(args mock.Arguments) {
		waitForAction <- true
	}).Return(nil /*no synchronous output*/, nil)
	// gets add when the queue is not full
	testOc.maxConcurrentProcess = 10
	assert.Empty(t, testOc.incompleteTxSProcessMap)
	assert.False(t, testOc.ProcessNewTransaction(ctx, testTx))
	<-waitForAction                        // no events emitted as no synchronous output was returned
	testOc.TriggerOrchestratorEvaluation() // this should remove the process from the pool
	//workaround timing condition
	time.Sleep(100 * time.Millisecond)
	testOc.Stop()
	testOc.Stop() // do a second stop to ensure at least one stop has gone through as the channel has buffer size 1
	<-ocDone
	assert.Empty(t, testOc.incompleteTxSProcessMap)
	assert.Equal(t, 1, int(testOc.totalCompleted))
}
