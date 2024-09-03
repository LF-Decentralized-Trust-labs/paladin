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
	baseTypes "github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"
	"github.com/kaleido-io/paladin/kata/mocks/enginemocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewControllerNoNewEngine(t *testing.T) {
	ctx := context.Background()
	enh, _ := NewTestEnterpriseTransactionHandler(t)
	enh.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	enh.maxInFlightEngines = 1
	enh.controllerPollingInterval = 1 * time.Hour

	// already has a running engine for the address so no new engine should be started
	enh.InFlightEngines = map[string]*transactionEngine{
		testMainSigningAddress: {state: TransactionEngineStateIdle, stateEntryTime: time.Now()}, // already has an engine for 0x1
	}
	enh.Start(ctx)
}

func TestNewControllerPollingCancelledContext(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)

	enh, _ := NewTestEnterpriseTransactionHandler(t)

	enh.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	enh.maxInFlightEngines = 1
	enh.controllerPollingInterval = 1 * time.Hour
	// already has a running engine for the address so no new engine should be started
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Once()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{}, nil, fmt.Errorf("error")).Run(func(args mock.Arguments) {
		cancelCtx()
	}).Once()
	enh.ctx = ctx
	polled, _ := enh.poll(ctx)
	assert.Equal(t, -1, polled)
}

func TestNewControllerPollingReAddStoppedEngine(t *testing.T) {
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

	enh, _ := NewTestEnterpriseTransactionHandler(t)

	enh.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	enh.maxInFlightEngines = 1
	enh.controllerPollingInterval = 1 * time.Hour

	// already has a running engine for the address so no new engine should be started
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Once()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{mockManagedTx1, mockManagedTx2}, nil, nil).Once()
	enh.InFlightEngines = map[string]*transactionEngine{
		testMainSigningAddress: {state: TransactionEngineStateStopped, stateEntryTime: time.Now()}, // already has an engine for 0x1
	}
	enh.ctx = ctx
	enh.poll(ctx)
	assert.Equal(t, TransactionEngineStateNew, enh.InFlightEngines[testMainSigningAddress].state)
}

func TestNewControllerPollingStoppingAnEngineAndSelf(t *testing.T) {
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

	enh, _ := NewTestEnterpriseTransactionHandler(t)

	enh.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	enh.maxInFlightEngines = 2
	enh.ctx = ctx
	enh.controllerPollingInterval = 1 * time.Hour
	enh.controllerLoopDone = make(chan struct{})
	// already has a running engine for the address so no new engine should be started
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Maybe()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{mockManagedTx1, mockManagedTx2}, nil, nil).Once()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{}, nil, nil).Maybe()
	go enh.controllerLoop()
	existingEngine := &transactionEngine{
		enterpriseTransactionHandler: enh,
		enginePollingInterval:        enh.controllerPollingInterval,
		state:                        TransactionEngineStateIdle,
		stateEntryTime:               time.Now().Add(-enh.maxEngineIdle).Add(-1 * time.Minute),
		InFlightTxsStale:             make(chan bool, 1),
		stopProcess:                  make(chan bool, 1),
	}
	enh.InFlightEngines = map[string]*transactionEngine{
		testMainSigningAddress: existingEngine, // already has an engine for 0x1
	}
	existingEngine.engineLoopDone = make(chan struct{})
	go existingEngine.Start(ctx)
	enh.Start(ctx)
	enh.MarkInFlightEnginesStale()
	<-existingEngine.engineLoopDone
	assert.Equal(t, TransactionEngineStateStopped, existingEngine.state)

	//stops OK
	cancelCtx()
	<-enh.controllerLoopDone
}

func TestNewControllerPollingStoppingAnEngineForFairnessControl(t *testing.T) {
	ctx, _ := context.WithCancel(context.Background())
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

	enh, _ := NewTestEnterpriseTransactionHandler(t)

	enh.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	enh.maxInFlightEngines = 1
	enh.ctx = ctx
	enh.controllerPollingInterval = 1 * time.Hour
	enh.controllerLoopDone = make(chan struct{})
	enh.maxInFlightEngines = 1
	existingEngine := &transactionEngine{
		engineBirthTime:              time.Now().Add(-1 * time.Hour),
		enterpriseTransactionHandler: enh,
		enginePollingInterval:        enh.controllerPollingInterval,
		state:                        TransactionEngineStateRunning,
		stateEntryTime:               time.Now().Add(-enh.maxEngineIdle).Add(-1 * time.Minute),
		InFlightTxsStale:             make(chan bool, 1),
		stopProcess:                  make(chan bool, 1),
	}
	go existingEngine.Start(ctx)
	// already has a running engine for the address so no new engine should be started
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Maybe()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{mockManagedTx1, mockManagedTx2}, nil, nil).Maybe()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{}, nil, nil).Maybe()
	enh.InFlightEngines = map[string]*transactionEngine{
		testMainSigningAddress: existingEngine, // already has an engine for 0x1
	}
	enh.ctx = ctx
	enh.poll(ctx)
	<-existingEngine.engineLoopDone
	assert.Equal(t, TransactionEngineStateStopped, existingEngine.state)
}

func TestNewControllerPollingExcludePausedEngine(t *testing.T) {
	ctx := context.Background()
	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)

	enh, _ := NewTestEnterpriseTransactionHandler(t)

	enh.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	enh.maxInFlightEngines = 1
	enh.controllerPollingInterval = 1 * time.Hour

	// already has a running engine for the address so no new engine should be started
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Once()
	listed := make(chan struct{})
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{}, nil, nil).Run(func(args mock.Arguments) {
		close(listed)
	}).Once()
	enh.InFlightEngines = map[string]*transactionEngine{}
	enh.SigningAddressesPausedUntil = map[string]time.Time{testMainSigningAddress: time.Now().Add(1 * time.Hour)}
	enh.Start(ctx)
	<-listed
	assert.Empty(t, enh.InFlightEngines)
}

func TestNewControllerCheckNoDefaultHandlers(t *testing.T) {
	ctx := context.Background()

	enh, _ := NewTestEnterpriseTransactionHandler(t)
	enh.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	enh.controllerPollingInterval = 1 * time.Hour

	err := enh.HandleTransactionConfirmations(ctx, "", nil)
	assert.Regexp(t, "PD011922", err)
	err = enh.HandleTransactionReceiptReceived(ctx, "", nil)
	assert.Regexp(t, "PD011923", err)
}

func TestNewControllerGetPendingFuelingTxs(t *testing.T) {
	ctx, _ := context.WithCancel(context.Background())
	mockManagedTx1 := &baseTypes.ManagedTX{
		ID: uuid.New().String(),
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(1),
		},
	}
	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)

	enh, _ := NewTestEnterpriseTransactionHandler(t)
	enh.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	enh.ctx = ctx
	enh.controllerPollingInterval = 1 * time.Hour

	// already has a running engine for the address so no new engine should be started
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Maybe()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{mockManagedTx1}, nil, nil).Once()
	tx, err := enh.GetPendingFuelingTransaction(ctx, "0x0", testMainSigningAddress)
	assert.Equal(t, mockManagedTx1, tx)
	assert.NoError(t, err)
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return(nil, nil, fmt.Errorf("List transaction errored")).Once()
	tx, err = enh.GetPendingFuelingTransaction(ctx, "0x0", testMainSigningAddress)
	assert.Nil(t, tx)
	assert.Error(t, err)
	assert.Regexp(t, "errored", err)
}

func TestNewControllerCheckTxCompleteness(t *testing.T) {
	ctx, _ := context.WithCancel(context.Background())
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

	enh, _ := NewTestEnterpriseTransactionHandler(t)
	enh.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	enh.ctx = ctx
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Maybe()
	enh.controllerPollingInterval = 1 * time.Hour

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
	assert.False(t, enh.CheckTransactionCompleted(ctx, testTxWithZeroNonce))

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
	assert.False(t, enh.CheckTransactionCompleted(ctx, testTxToCheck))
	// return false when no transactions retrieved
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{}, nil, nil).Once()
	assert.False(t, enh.CheckTransactionCompleted(ctx, testTxToCheck))
	// return false when the retrieved transaction has a lower nonce
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{mockManagedTx1}, nil, nil).Once()
	assert.False(t, enh.CheckTransactionCompleted(ctx, testTxToCheck))

	// try to update nonce when transaction incomplete shouldn't take affect

	enh.updateCompletedTxNonce(ctx, testTxToCheck) // nonce stayed at 0
	assert.False(t, enh.CheckTransactionCompleted(ctx, testTxToCheck))

	// try to update the nonce with a completed transaction works
	testTxToCheck.Status = baseTypes.BaseTxStatusFailed
	enh.updateCompletedTxNonce(ctx, testTxToCheck) // nonce stayed at 0
	assert.True(t, enh.CheckTransactionCompleted(ctx, testTxToCheck))

}
