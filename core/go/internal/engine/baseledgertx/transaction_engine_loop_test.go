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

package baseledgertx

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/ffapi"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	baseTypes "github.com/kaleido-io/paladin/core/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/mocks/enginemocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewEngineNoNewOrchestrator(t *testing.T) {
	ctx := context.Background()
	ble, _ := NewTestTransactionEngine(t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	ble.maxInFlightOrchestrators = 1
	ble.enginePollingInterval = 1 * time.Hour

	// already has a running orchestrator for the address so no new orchestrator should be started
	ble.InFlightOrchestrators = map[string]*orchestrator{
		testMainSigningAddress: {state: OrchestratorStateIdle, stateEntryTime: time.Now()}, // already has an orchestrator for 0x1
	}
	_, _ = ble.Start(ctx)
}

func TestNewEnginePollingCancelledContext(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)

	ble, _ := NewTestTransactionEngine(t)

	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	ble.maxInFlightOrchestrators = 1
	ble.enginePollingInterval = 1 * time.Hour
	// already has a running orchestrator for the address so no new orchestrator should be started
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Once()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{}, nil, fmt.Errorf("error")).Run(func(args mock.Arguments) {
		cancelCtx()
	}).Once()
	ble.ctx = ctx
	polled, _ := ble.poll(ctx)
	assert.Equal(t, -1, polled)
}

func TestNewEnginePollingReAddStoppedOrchestrator(t *testing.T) {
	ctx := context.Background()
	mockManagedTx1 := &baseTypes.ManagedTX{
		ID: uuid.New().String(),
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(1),
		},
	}
	mockManagedTx2 := &baseTypes.ManagedTX{
		ID: uuid.New().String(),
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(2),
		},
	}
	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)

	ble, _ := NewTestTransactionEngine(t)

	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	ble.maxInFlightOrchestrators = 1
	ble.enginePollingInterval = 1 * time.Hour

	// already has a running orchestrator for the address so no new orchestrator should be started
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Once()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{mockManagedTx1, mockManagedTx2}, nil, nil).Once()
	ble.InFlightOrchestrators = map[string]*orchestrator{
		testMainSigningAddress: {state: OrchestratorStateStopped, stateEntryTime: time.Now()}, // already has an orchestrator for 0x1
	}
	ble.ctx = ctx
	ble.poll(ctx)
	assert.Equal(t, OrchestratorStateNew, ble.InFlightOrchestrators[testMainSigningAddress].state)
}

func TestNewEnginePollingStoppingAnOrchestratorAndSelf(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	mockManagedTx1 := &baseTypes.ManagedTX{
		ID: uuid.New().String(),
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(1),
		},
	}
	mockManagedTx2 := &baseTypes.ManagedTX{
		ID: uuid.New().String(),
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(2),
		},
	}
	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)

	ble, _ := NewTestTransactionEngine(t)

	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	ble.maxInFlightOrchestrators = 2
	ble.ctx = ctx
	ble.enginePollingInterval = 1 * time.Hour
	ble.engineLoopDone = make(chan struct{})
	// already has a running orchestrator for the address so no new orchestrator should be started
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Maybe()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{mockManagedTx1, mockManagedTx2}, nil, nil).Once()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{}, nil, nil).Maybe()
	go ble.engineLoop()
	existingOrchestrator := &orchestrator{
		baseLedgerTxEngine:          ble,
		orchestratorPollingInterval: ble.enginePollingInterval,
		state:                       OrchestratorStateIdle,
		stateEntryTime:              time.Now().Add(-ble.maxOrchestratorIdle).Add(-1 * time.Minute),
		InFlightTxsStale:            make(chan bool, 1),
		stopProcess:                 make(chan bool, 1),
		txStore:                     mTS,
		ethClient:                   mEC,
		managedTXEventNotifier:      mEN,
		txConfirmationListener:      mCL,
		maxInFlightTxs:              0,
	}
	ble.InFlightOrchestrators = map[string]*orchestrator{
		testMainSigningAddress: existingOrchestrator, // already has an orchestrator for 0x1
	}
	_, _ = existingOrchestrator.Start(ctx)
	ble.MarkInFlightOrchestratorsStale()
	<-existingOrchestrator.orchestratorLoopDone
	assert.Equal(t, OrchestratorStateStopped, existingOrchestrator.state)

	//stops OK
	cancelCtx()
	<-ble.engineLoopDone
	time.Sleep(2 * time.Second)
}

func TestNewEnginePollingStoppingAnOrchestratorForFairnessControl(t *testing.T) {
	ctx := context.Background()
	mockManagedTx1 := &baseTypes.ManagedTX{
		ID: uuid.New().String(),
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(1),
		},
	}
	mockManagedTx2 := &baseTypes.ManagedTX{
		ID: uuid.New().String(),
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(2),
		},
	}
	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)

	ble, _ := NewTestTransactionEngine(t)

	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	ble.maxInFlightOrchestrators = 1
	ble.ctx = ctx
	ble.enginePollingInterval = 1 * time.Hour
	ble.engineLoopDone = make(chan struct{})
	ble.maxInFlightOrchestrators = 1
	existingOrchestrator := &orchestrator{
		orchestratorBirthTime:       time.Now().Add(-1 * time.Hour),
		baseLedgerTxEngine:          ble,
		orchestratorPollingInterval: ble.enginePollingInterval,
		state:                       OrchestratorStateRunning,
		stateEntryTime:              time.Now().Add(-ble.maxOrchestratorIdle).Add(-1 * time.Minute),
		InFlightTxsStale:            make(chan bool, 1),
		stopProcess:                 make(chan bool, 1),
		txStore:                     mTS,
		ethClient:                   mEC,
		managedTXEventNotifier:      mEN,
		txConfirmationListener:      mCL,
	}
	// already has a running orchestrator for the address so no new orchestrator should be started
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Maybe()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{mockManagedTx1, mockManagedTx2}, nil, nil).Maybe()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{}, nil, nil).Maybe()
	ble.InFlightOrchestrators = map[string]*orchestrator{
		testMainSigningAddress: existingOrchestrator, // already has an orchestrator for 0x1
	}
	ble.ctx = ctx
	ble.poll(ctx)
	existingOrchestrator.orchestratorLoopDone = make(chan struct{})
	existingOrchestrator.orchestratorLoop()
	<-existingOrchestrator.orchestratorLoopDone
	assert.Equal(t, OrchestratorStateStopped, existingOrchestrator.state)
}

func TestNewEnginePollingExcludePausedOrchestrator(t *testing.T) {
	ctx := context.Background()
	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)

	ble, _ := NewTestTransactionEngine(t)

	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	ble.maxInFlightOrchestrators = 1
	ble.enginePollingInterval = 1 * time.Hour

	// already has a running orchestrator for the address so no new orchestrator should be started
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Once()
	listed := make(chan struct{})
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{}, nil, nil).Run(func(args mock.Arguments) {
		close(listed)
	}).Once()
	ble.InFlightOrchestrators = map[string]*orchestrator{}
	ble.SigningAddressesPausedUntil = map[string]time.Time{testMainSigningAddress: time.Now().Add(1 * time.Hour)}
	_, _ = ble.Start(ctx)
	<-listed
	assert.Empty(t, ble.InFlightOrchestrators)
}

func TestNewEngineGetPendingFuelingTxs(t *testing.T) {
	ctx := context.Background()
	mockManagedTx1 := &baseTypes.ManagedTX{
		ID: uuid.New().String(),
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(1),
		},
	}
	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)

	ble, _ := NewTestTransactionEngine(t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	ble.ctx = ctx
	ble.enginePollingInterval = 1 * time.Hour

	// already has a running orchestrator for the address so no new orchestrator should be started
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Maybe()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{mockManagedTx1}, nil, nil).Once()
	tx, err := ble.GetPendingFuelingTransaction(ctx, "0x0", testMainSigningAddress)
	assert.Equal(t, mockManagedTx1, tx)
	assert.NoError(t, err)
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return(nil, nil, fmt.Errorf("List transaction errored")).Once()
	tx, err = ble.GetPendingFuelingTransaction(ctx, "0x0", testMainSigningAddress)
	assert.Nil(t, tx)
	assert.Error(t, err)
	assert.Regexp(t, "errored", err)
}

func TestNewEngineCheckTxCompleteness(t *testing.T) {
	ctx := context.Background()
	mockManagedTx1 := &baseTypes.ManagedTX{
		ID:     uuid.New().String(),
		Status: baseTypes.BaseTxStatusSucceeded,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(0),
		},
	}
	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)

	ble, _ := NewTestTransactionEngine(t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	ble.ctx = ctx
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Maybe()
	ble.enginePollingInterval = 1 * time.Hour

	// when no nonce cached

	// return false for a transaction with nonce "0" that is still pending
	testTxWithZeroNonce := &baseTypes.ManagedTX{
		Status: baseTypes.BaseTxStatusPending,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(0),
		},
	}
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{}, nil, nil).Once()
	assert.False(t, ble.CheckTransactionCompleted(ctx, testTxWithZeroNonce))

	// for transactions with a non-zero nonce
	testTxToCheck := &baseTypes.ManagedTX{
		Status: baseTypes.BaseTxStatusPending,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(1),
		},
	}
	// return false when retrieve transactions failed
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return(nil, nil, fmt.Errorf("List transaction errored")).Once()
	assert.False(t, ble.CheckTransactionCompleted(ctx, testTxToCheck))
	// return false when no transactions retrieved
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{}, nil, nil).Once()
	assert.False(t, ble.CheckTransactionCompleted(ctx, testTxToCheck))
	// return false when the retrieved transaction has a lower nonce
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{mockManagedTx1}, nil, nil).Once()
	assert.False(t, ble.CheckTransactionCompleted(ctx, testTxToCheck))

	// try to update nonce when transaction incomplete shouldn't take affect

	ble.updateCompletedTxNonce(testTxToCheck) // nonce stayed at 0
	assert.False(t, ble.CheckTransactionCompleted(ctx, testTxToCheck))

	// try to update the nonce with a completed transaction works
	testTxToCheck.Status = baseTypes.BaseTxStatusFailed
	ble.updateCompletedTxNonce(testTxToCheck) // nonce stayed at 0
	assert.True(t, ble.CheckTransactionCompleted(ctx, testTxToCheck))
}
