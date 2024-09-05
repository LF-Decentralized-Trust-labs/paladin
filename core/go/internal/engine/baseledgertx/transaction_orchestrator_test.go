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
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	baseTypes "github.com/kaleido-io/paladin/kata/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"
	"github.com/kaleido-io/paladin/kata/mocks/enginemocks"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewOrchestratorPolling(t *testing.T) {
	ctx := context.Background()
	mockManagedTx1 := &baseTypes.ManagedTX{
		ID:     uuid.New().String(),
		Status: baseTypes.BaseTxStatusPending,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(1),
		},
		Created: fftypes.Now(),
	}
	mockManagedTx2 := &baseTypes.ManagedTX{
		ID:     uuid.New().String(),
		Status: baseTypes.BaseTxStatusPending,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(2),
		},
		Created: fftypes.Now(),
	}

	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)

	ble, _ := NewTestTransactionEngine(t)
	mockBM, mEC, _ := NewTestBalanceManager(ctx, t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mTS := enginemocks.NewTransactionStore(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	ble.orchestratorConfig.Set(OrchestratorMaxInFlightTransactionsInt, 10)
	ble.ctx = ctx
	ble.enginePollingInterval = 1 * time.Hour

	te := NewOrchestrator(ble, string(mockManagedTx1.From), ble.orchestratorConfig)
	te.balanceManager = mockBM
	mockIT := NewInFlightTransactionStageController(ble, te, mockManagedTx1)
	te.InFlightTxs = []*InFlightTransactionStageController{mockIT}
	te.transactionIDsInStatusUpdate = []string{"randomID"}
	mTS.On("AddSubStatusAction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(1 * time.Hour) // make sure the async action never got returned as the test will mock the events
	}).Maybe()
	mTS.On("UpdateTransaction", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(1 * time.Hour) // make sure the async action never got returned as the test will mock the events
	}).Maybe()
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Maybe()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{mockManagedTx1, mockManagedTx2}, nil, nil).Once()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{}, nil, nil).Maybe()
	te.InFlightTxs = []*InFlightTransactionStageController{
		mockIT}

	addressBalanceChecked := make(chan bool)
	mEC.On("GetBalance", mock.Anything, testMainSigningAddress, "latest").Return(ethtypes.NewHexInteger64(100), nil).Run(func(args mock.Arguments) {
		close(addressBalanceChecked)
	}).Once()
	te.orchestratorPollingInterval = 1 * time.Hour
	_, _ = te.Start(ctx)
	<-addressBalanceChecked
}

func TestNewOrchestratorPollingContextCancelled(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	mockManagedTx1 := &baseTypes.ManagedTX{
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(1),
		},
	}

	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)

	ble, _ := NewTestTransactionEngine(t)
	mockBM, mEC, _ := NewTestBalanceManager(ctx, t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mTS := enginemocks.NewTransactionStore(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	ble.orchestratorConfig.Set(OrchestratorMaxInFlightTransactionsInt, 10)
	ble.ctx = ctx
	ble.enginePollingInterval = 1 * time.Hour

	te := NewOrchestrator(ble, string(mockManagedTx1.From), ble.orchestratorConfig)

	te.balanceManager = mockBM

	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Maybe()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return(nil, nil, fmt.Errorf("list transactions error")).Run(func(args mock.Arguments) {
		cancelCtx()
	}).Once()
	polled, _ := te.pollAndProcess(ctx)
	assert.Equal(t, -1, polled)
}

func TestNewOrchestratorPollingRemoveCompleted(t *testing.T) {
	ctx := context.Background()
	mockManagedTx1 := &baseTypes.ManagedTX{
		ID:     uuid.New().String(),
		Status: baseTypes.BaseTxStatusFailed,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(1),
		},
		Created: fftypes.Now(),
	}
	mockManagedTx2 := &baseTypes.ManagedTX{
		ID:     uuid.New().String(),
		Status: baseTypes.BaseTxStatusPending,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(2),
		},
		Created: fftypes.Now(),
	}
	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)

	ble, _ := NewTestTransactionEngine(t)
	mockBM, mEC, _ := NewTestBalanceManager(ctx, t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mTS := enginemocks.NewTransactionStore(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	ble.orchestratorConfig.Set(OrchestratorMaxInFlightTransactionsInt, 10)
	ble.ctx = ctx
	ble.enginePollingInterval = 1 * time.Hour

	te := NewOrchestrator(ble, string(mockManagedTx1.From), ble.orchestratorConfig)

	te.balanceManager = mockBM
	mockIT := NewInFlightTransactionStageController(ble, te, mockManagedTx1)

	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Once()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{mockManagedTx1, mockManagedTx2}, nil, nil).Once()
	te.InFlightTxs = []*InFlightTransactionStageController{
		mockIT,
	}
	addressBalanceChecked := make(chan bool)
	mEC.On("GetBalance", mock.Anything, testMainSigningAddress, "latest").Return(nil, fmt.Errorf("failed getting balance")).Run(func(args mock.Arguments) {
		close(addressBalanceChecked)
	}).Once()
	te.orchestratorPollingInterval = 1 * time.Hour
	_, _ = te.Start(ctx)
	<-addressBalanceChecked
}

func TestNewOrchestratorPollingRemoveSuspended(t *testing.T) {
	ctx := context.Background()
	mockManagedTx1 := &baseTypes.ManagedTX{
		ID:     uuid.New().String(),
		Status: baseTypes.BaseTxStatusSuspended,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(1),
		},
		Created: fftypes.Now(),
	}
	mockManagedTx2 := &baseTypes.ManagedTX{
		ID:     uuid.New().String(),
		Status: baseTypes.BaseTxStatusPending,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(2),
		},
		Created: fftypes.Now(),
	}

	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)

	ble, _ := NewTestTransactionEngine(t)
	mockBM, mEC, _ := NewTestBalanceManager(ctx, t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mTS := enginemocks.NewTransactionStore(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)

	ble.orchestratorConfig.Set(OrchestratorMaxInFlightTransactionsInt, 10)
	ble.ctx = ctx
	ble.enginePollingInterval = 1 * time.Hour

	te := NewOrchestrator(ble, string(mockManagedTx1.From), ble.orchestratorConfig)

	te.balanceManager = mockBM
	mockIT := NewInFlightTransactionStageController(ble, te, mockManagedTx1)

	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Once()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{mockManagedTx1, mockManagedTx2}, nil, nil).Once()
	te.InFlightTxs = []*InFlightTransactionStageController{
		mockIT,
	}

	addressBalanceChecked := make(chan bool)
	mEC.On("GetBalance", mock.Anything, testMainSigningAddress, "latest").Return(nil, fmt.Errorf("failed getting balance")).Run(func(args mock.Arguments) {
		close(addressBalanceChecked)
	}).Once()
	te.orchestratorPollingInterval = 1 * time.Hour
	_, _ = te.Start(ctx)
	<-addressBalanceChecked
}

func TestNewOrchestratorPollingMarkStale(t *testing.T) {
	ctx := context.Background()
	mockManagedTx1 := &baseTypes.ManagedTX{
		ID:     uuid.New().String(),
		Status: baseTypes.BaseTxStatusPending,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(1),
		},
		Created: fftypes.Now(),
	}

	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)

	ble, _ := NewTestTransactionEngine(t)
	mockBM, mEC, _ := NewTestBalanceManager(ctx, t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mTS := enginemocks.NewTransactionStore(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	ble.orchestratorConfig.Set(OrchestratorMaxInFlightTransactionsInt, 10)
	ble.ctx = ctx
	ble.enginePollingInterval = 1 * time.Hour

	te := NewOrchestrator(ble, string(mockManagedTx1.From), ble.orchestratorConfig)

	te.balanceManager = mockBM
	te.lastQueueUpdate = time.Now().Add(-1 * time.Hour)
	mockIT := NewInFlightTransactionStageController(ble, te, mockManagedTx1)

	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Maybe()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{mockManagedTx1}, nil, nil).Once()
	te.InFlightTxs = []*InFlightTransactionStageController{
		mockIT,
	}
	addressBalanceChecked := make(chan bool)
	mEC.On("GetBalance", mock.Anything, testMainSigningAddress, "latest").Return(nil, fmt.Errorf("failed getting balance")).Run(func(args mock.Arguments) {
		close(addressBalanceChecked)
	}).Once()
	te.orchestratorPollingInterval = 1 * time.Hour
	te.pollAndProcess(ctx)
	<-addressBalanceChecked
	assert.Equal(t, OrchestratorStateStale, te.state)
}

func TestOrchestratorStop(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	mockManagedTx1 := &baseTypes.ManagedTX{
		ID:     uuid.New().String(),
		Status: baseTypes.BaseTxStatusSuspended,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(1),
		},
		Created: fftypes.Now(),
	}
	mockManagedTx2 := &baseTypes.ManagedTX{
		ID:     uuid.New().String(),
		Status: baseTypes.BaseTxStatusPending,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(2),
		},
		Created: fftypes.Now(),
	}

	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)

	ble, _ := NewTestTransactionEngine(t)

	mTS := enginemocks.NewTransactionStore(t)
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Maybe()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{mockManagedTx1, mockManagedTx2}, nil, nil).Maybe()
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	ble.orchestratorConfig.Set(OrchestratorMaxInFlightTransactionsInt, 10)
	ble.ctx = ctx
	ble.enginePollingInterval = 1 * time.Hour

	te := NewOrchestrator(ble, string(mockManagedTx1.From), ble.orchestratorConfig)
	te.orchestratorPollingInterval = 1 * time.Hour
	te.ctx = ctx
	te.orchestratorLoopDone = make(chan struct{})
	go te.orchestratorLoop()

	//stops OK
	cancelCtx()
	<-te.orchestratorLoopDone
}

func TestOrchestratorStopWhenBalanceUnavailable(t *testing.T) {
	ctx := context.Background()
	mockManagedTx1 := &baseTypes.ManagedTX{
		ID:     uuid.New().String(),
		Status: baseTypes.BaseTxStatusSucceeded,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(1),
		},
		Created: fftypes.Now(),
	}

	ble, _ := NewTestTransactionEngine(t)
	mockBM, mEC, _ := NewTestBalanceManager(ctx, t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mTS := enginemocks.NewTransactionStore(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)

	ble.orchestratorConfig.Set(OrchestratorMaxInFlightTransactionsInt, 10)
	ble.ctx = ctx
	ble.enginePollingInterval = 1 * time.Hour

	te := NewOrchestrator(ble, string(mockManagedTx1.From), ble.orchestratorConfig)
	te.state = OrchestratorStateRunning
	te.balanceManager = mockBM
	mockIT := NewInFlightTransactionStageController(ble, te, mockManagedTx1)

	// continue process
	mEC.On("GetBalance", mock.Anything, testMainSigningAddress, "latest").Return(nil, fmt.Errorf("failed getting balance")).Once()
	te.unavailableBalanceHandlingStrategy = OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyContinue
	waitingForBalance, err := te.ProcessInFlightTransaction(ctx, []*InFlightTransactionStageController{mockIT})
	assert.NoError(t, err)
	assert.False(t, waitingForBalance)
	assert.Equal(t, OrchestratorStateRunning, te.state)

	// wait for the next round
	mEC.On("GetBalance", mock.Anything, testMainSigningAddress, "latest").Return(nil, fmt.Errorf("failed getting balance")).Once()
	te.unavailableBalanceHandlingStrategy = OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyWait
	waitingForBalance, err = te.ProcessInFlightTransaction(ctx, []*InFlightTransactionStageController{mockIT})
	assert.NoError(t, err)
	assert.True(t, waitingForBalance)
	assert.Equal(t, OrchestratorStateRunning, te.state)

	// stop the orchestrator
	mEC.On("GetBalance", mock.Anything, testMainSigningAddress, "latest").Return(nil, fmt.Errorf("failed getting balance")).Once()
	te.unavailableBalanceHandlingStrategy = OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyStop
	te.stopProcess = make(chan bool, 1)
	waitingForBalance, err = te.ProcessInFlightTransaction(ctx, []*InFlightTransactionStageController{mockIT})
	assert.NoError(t, err)
	assert.True(t, waitingForBalance)
	<-te.stopProcess
}

func TestOrchestratorTriggerTopUp(t *testing.T) {
	ctx := context.Background()
	mockManagedTx1 := &baseTypes.ManagedTX{
		ID:     uuid.New().String(),
		Status: baseTypes.BaseTxStatusSucceeded,
		Transaction: &ethsigner.Transaction{
			From:     json.RawMessage(testMainSigningAddress),
			Nonce:    ethtypes.NewHexInteger64(1),
			GasPrice: ethtypes.NewHexInteger64(1000),
			GasLimit: ethtypes.NewHexInteger64(100),
		},
		Created: fftypes.Now(),
	}

	ble, _ := NewTestTransactionEngine(t)
	mockBM, mEC, mockAFTxEngine := NewTestBalanceManager(ctx, t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mTS := enginemocks.NewTransactionStore(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	ble.orchestratorConfig.Set(OrchestratorMaxInFlightTransactionsInt, 10)
	ble.ctx = ctx
	ble.enginePollingInterval = 1 * time.Hour

	te := NewOrchestrator(ble, string(mockManagedTx1.From), ble.orchestratorConfig)
	te.state = OrchestratorStateRunning
	te.balanceManager = mockBM
	mockIT := NewInFlightTransactionStageController(ble, te, mockManagedTx1)

	// trigger top up if cost is known
	mockAFTxEngine.On("GetPendingFuelingTransaction", mock.Anything, "0x4e598f6e918321dd47c86e7a077b4ab0e7414846", testMainSigningAddress).Return(nil, fmt.Errorf("cannot get tx")).Once()
	mEC.On("GetBalance", mock.Anything, testMainSigningAddress, "latest").Return(ethtypes.NewHexInteger64(100), nil).Once()
	te.unavailableBalanceHandlingStrategy = OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyContinue
	waitingForBalance, err := te.ProcessInFlightTransaction(ctx, []*InFlightTransactionStageController{mockIT})
	assert.NoError(t, err)
	assert.True(t, waitingForBalance)
	assert.Equal(t, OrchestratorStateRunning, te.state)

	// skip top up when cost is unknown
	mockManagedTx1.GasPrice = nil
	te.unavailableBalanceHandlingStrategy = OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyContinue
	waitingForBalance, err = te.ProcessInFlightTransaction(ctx, []*InFlightTransactionStageController{mockIT})
	assert.NoError(t, err)
	assert.False(t, waitingForBalance)
	assert.Equal(t, OrchestratorStateRunning, te.state)
}

func TestOrchestratorReceiptHandler(t *testing.T) {
	ctx := context.Background()
	mockManagedTx1 := &baseTypes.ManagedTX{
		ID:     uuid.New().String(),
		Status: baseTypes.BaseTxStatusSucceeded,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(1),
		},
		Created: fftypes.Now(),
	}
	mockManagedTx2 := &baseTypes.ManagedTX{
		ID:     uuid.New().String(),
		Status: baseTypes.BaseTxStatusSucceeded,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(2),
		},
		Created: fftypes.Now(),
	}

	ble, _ := NewTestTransactionEngine(t)
	_, mEC, _ := NewTestBalanceManager(ctx, t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mTS := enginemocks.NewTransactionStore(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)

	ble.orchestratorConfig.Set(OrchestratorMaxInFlightTransactionsInt, 10)
	ble.ctx = ctx
	ble.enginePollingInterval = 1 * time.Hour

	te := NewOrchestrator(ble, string(mockManagedTx1.From), ble.orchestratorConfig)
	mockIT1 := NewInFlightTransactionStageController(ble, te, mockManagedTx1)
	mockIT1.timeLineLoggingEnabled = true
	mockIT2 := NewInFlightTransactionStageController(ble, te, mockManagedTx2)
	mockIT2.timeLineLoggingEnabled = true
	te.InFlightTxs = []*InFlightTransactionStageController{mockIT1, mockIT2}
	receiptHandler := te.CreateTransactionReceiptReceivedHandler(mockManagedTx1.ID)
	testReceipt := &ethclient.TransactionReceiptResponse{BlockNumber: fftypes.NewFFBigInt(1)}

	// added receipt
	err := receiptHandler(ctx, mockIT1.stateManager.GetTxID(), testReceipt)
	assert.NoError(t, err)
	time.Sleep(200 * time.Millisecond)
	iftxs := mockIT1.stateManager.(*inFlightTransactionState)
	assert.Equal(t, testReceipt, iftxs.bufferedStageOutputs[0].ReceiptOutput.Receipt)

	// transaction no longer in queue
	te.InFlightTxs = []*InFlightTransactionStageController{mockIT2}
	err = receiptHandler(ctx, mockIT1.stateManager.GetTxID(), testReceipt)
	assert.Regexp(t, "PD011924", err)
}

func TestOrchestratorConfirmationHandler(t *testing.T) {
	ctx := context.Background()
	mockManagedTx1 := &baseTypes.ManagedTX{
		ID:     uuid.New().String(),
		Status: baseTypes.BaseTxStatusSucceeded,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(1),
		},
		Created: fftypes.Now(),
	}
	mockManagedTx2 := &baseTypes.ManagedTX{
		ID:     uuid.New().String(),
		Status: baseTypes.BaseTxStatusSucceeded,
		Transaction: &ethsigner.Transaction{
			From:  json.RawMessage(testMainSigningAddress),
			Nonce: ethtypes.NewHexInteger64(2),
		},
		Created: fftypes.Now(),
	}

	ble, _ := NewTestTransactionEngine(t)
	_, mEC, _ := NewTestBalanceManager(ctx, t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mTS := enginemocks.NewTransactionStore(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)

	ble.orchestratorConfig.Set(OrchestratorMaxInFlightTransactionsInt, 10)
	ble.ctx = ctx
	ble.enginePollingInterval = 1 * time.Hour

	te := NewOrchestrator(ble, string(mockManagedTx1.From), ble.orchestratorConfig)
	mockIT1 := NewInFlightTransactionStageController(ble, te, mockManagedTx1)
	mockIT2 := NewInFlightTransactionStageController(ble, te, mockManagedTx2)
	te.InFlightTxs = []*InFlightTransactionStageController{mockIT1, mockIT2}
	confirmationHandler := te.CreateTransactionConfirmationsHandler(mockManagedTx1.ID)
	testConfirmation := &baseTypes.ConfirmationsNotification{Confirmed: true}

	// added confirmation
	err := confirmationHandler(ctx, mockIT1.stateManager.GetTxID(), testConfirmation)
	assert.NoError(t, err)
	time.Sleep(200 * time.Millisecond)
	iftxs := mockIT1.stateManager.(*inFlightTransactionState)
	assert.Equal(t, testConfirmation, iftxs.bufferedStageOutputs[0].ConfirmationOutput.Confirmations)

	// transaction no longer in queue
	te.InFlightTxs = []*InFlightTransactionStageController{mockIT2}
	err = confirmationHandler(ctx, mockIT1.stateManager.GetTxID(), testConfirmation)
	assert.Regexp(t, "PD011924", err)
}
