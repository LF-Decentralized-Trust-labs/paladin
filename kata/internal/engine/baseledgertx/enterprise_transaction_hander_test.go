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
	"fmt"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/ffapi"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/components"
	baseTypes "github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"
	"github.com/kaleido-io/paladin/kata/mocks/enginemocks"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const testMainSigningAddress = testDestAddress

func NewTestEnterpriseTransactionHandler(t *testing.T) (*enterpriseTransactionHandler, config.Section) {
	return NewTestEnterpriseTransactionHandlerWithSignPermissionResponse(t, nil)
}

func NewTestEnterpriseTransactionHandlerWithSignPermissionResponse(t *testing.T, responses map[string][][2]string) (*enterpriseTransactionHandler, config.Section) {
	ctx := context.Background()
	conf := config.RootSection("unittest")
	thf := &TransactionHandlerFactory{}
	thf.InitConfig(conf)
	controllerConf := conf.SubSection(TransactionControllerSection)
	controllerConf.Set(TransactionControllerIntervalDurationString, "1h")
	controllerConf.Set(TransactionControllerMaxInFlightEngineInt, -1)
	engConf := conf.SubSection(TransactionEngineSection)
	engConf.Set(TransactionEngineIntervalDurationString, "1h")
	engConf.Set(TransactionEngineMaxInFlightTransactionsInt, -1)
	engConf.Set(TransactionEngineSubmissionRetryCountInt, 0)

	th, err := thf.NewTransactionHandler(ctx, conf)
	assert.Nil(t, err)
	return th.(*enterpriseTransactionHandler), conf
}

func TestNewHandlerErrors(t *testing.T) {
	ctx := context.Background()

	thf := &TransactionHandlerFactory{}

	conf := config.RootSection("unittest")
	thf.InitConfig(conf)

	// gasPriceIncreaseMax parsing error
	engineConf := conf.SubSection(TransactionEngineSection)
	engineConf.Set(TransactionEngineGasPriceIncreaseMaxBigIntString, "not a big int")
	_, err := thf.NewTransactionHandler(ctx, conf)
	assert.NotNil(t, err)
	assert.Regexp(t, "PD011909", err)
	engineConf.Set(TransactionEngineGasPriceIncreaseMaxBigIntString, "")

	engineConf.Set(TransactionEngineGasPriceIncreaseMaxBigIntString, "1")
	h, err := thf.NewTransactionHandler(ctx, conf)
	enh := h.(*enterpriseTransactionHandler)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1), enh.gasPriceIncreaseMax)
	engineConf.Set(TransactionEngineGasPriceIncreaseMaxBigIntString, "")
}

func TestInit(t *testing.T) {
	ctx := context.Background()
	enh, _ := NewTestEnterpriseTransactionHandler(t)
	qFields := &ffapi.QueryFields{}

	mockTransactionFilter := qFields.NewFilter(ctx)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	listed := make(chan struct{})
	mTS.On("NewTransactionFilter", mock.Anything).Return(mockTransactionFilter).Once()
	mTS.On("ListTransactions", mock.Anything, mock.Anything).Return([]*baseTypes.ManagedTX{}, nil, nil).Run(func(args mock.Arguments) {
		listed <- struct{}{}
	}).Once()
	enh.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	enh.controllerPollingInterval = 1 * time.Hour
	enh.maxInFlightEngines = 1
	// starts ok
	enh.Start(ctx)
	<-listed
	// init errors
	afConfig := enh.balanceManagerConfig.SubSection(BalanceManagerAutoFuelingSection)
	afConfig.Set(BalanceManagerAutoFuelingSourceAddressMinimumBalanceBigIntString, "not a big int")
	assert.Panics(t, func() {
		enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	})
	afConfig.Set(BalanceManagerAutoFuelingSourceAddressMinimumBalanceBigIntString, "0")
}

func TestHandleNewTransactionForTransferOnly(t *testing.T) {
	ctx := context.Background()

	enh, _ := NewTestEnterpriseTransactionHandler(t)
	enh.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)
	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	testEthTxInput := &ethsigner.Transaction{
		From:  []byte(testAutoFuelingSourceAddress),
		To:    ethtypes.MustNewAddress(testDestAddress),
		Value: ethtypes.NewHexInteger64(100),
	}

	// estimation failure - for non-revert
	txID := uuid.New()
	mEC.On("GasEstimate", mock.Anything, &testEthTxInput).Return(nil, fmt.Errorf("GasEstimate error")).Once()
	_, submissionRejected, err := enh.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
	}, &components.EthTransfer{
		To:    *types.MustEthAddress(testEthTxInput.To.String()),
		Value: testEthTxInput.Value,
	})
	assert.NotNil(t, err)
	assert.False(t, submissionRejected)
	assert.Regexp(t, "GasEstimate error", err)

	// estimation failure - for revert
	txID = uuid.New()
	mEC.On("GasEstimate", mock.Anything, &testEthTxInput).Return(nil, fmt.Errorf("execution reverted")).Once()
	_, submissionRejected, err = enh.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
	}, &components.EthTransfer{
		To:    *types.MustEthAddress(testEthTxInput.To.String()),
		Value: testEthTxInput.Value,
	})
	assert.NotNil(t, err)
	assert.True(t, submissionRejected)
	assert.Regexp(t, "GasEstimate error", err)

	// insert transaction next nonce error
	mEC.On("GasEstimate", mock.Anything, &testEthTxInput).Return(ethtypes.NewHexInteger(big.NewInt(10)), nil).Once()
	insertMock := mTS.On("InsertTransactionWithNextNonce", ctx, mock.Anything, mock.Anything)
	mEC.On("GetTransactionCount", mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("pop")).Once()
	insertMock.Run(func(args mock.Arguments) {
		ctx := args[0].(context.Context)
		mtx := args[1].(*baseTypes.ManagedTX)
		nextNonceCB := args[2].(baseTypes.NextNonceCallback)
		_, err := nextNonceCB(ctx, string(mtx.From))
		insertMock.Return(err)
	}).Once()
	_, submissionRejected, err = enh.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
	}, &components.EthTransfer{
		To:    *types.MustEthAddress(testEthTxInput.To.String()),
		Value: testEthTxInput.Value,
	})

	assert.NotNil(t, err)
	assert.Regexp(t, "pop", err)
	assert.False(t, submissionRejected)

	// create transaction succeeded
	// gas estimate should be cached
	insertMock = mTS.On("InsertTransactionWithNextNonce", ctx, mock.Anything, mock.Anything)
	mEC.On("GetTransactionCount", mock.Anything, mock.Anything).
		Return(ethtypes.HexUint64(1), nil).Once()
	insertMock.Run(func(args mock.Arguments) {
		ctx := args[0].(context.Context)
		mtx := args[1].(*baseTypes.ManagedTX)
		nextNonceCB := args[2].(baseTypes.NextNonceCallback)
		nonce, err := nextNonceCB(ctx, string(mtx.From))
		assert.Nil(t, err)
		assert.NotNil(t, nonce)
		insertMock.Return(nil)
	}).Once()
	mTS.On("AddSubStatusAction", ctx, txID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionAssignNonce, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, _, err = enh.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
	}, &components.EthTransfer{
		To:    *types.MustEthAddress(testEthTxInput.To.String()),
		Value: testEthTxInput.Value,
	})
	assert.NoError(t, err)
	mEC.AssertNotCalled(t, "GasEstimate")
}

func TestHandleNewTransactionTransferOnlyWithProvideGas(t *testing.T) {
	ctx := context.Background()
	enh, _ := NewTestEnterpriseTransactionHandler(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	// fall back to connector when get call failed
	mEC.On("GasPrice", ctx, mock.Anything).Return(ethtypes.NewHexInteger(big.NewInt(10)), nil).Once()
	enh.gasPriceClient = NewTestNodeGasPriceClient(t, mEC)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	testEthTxInput := &ethsigner.Transaction{
		From:     []byte(testAutoFuelingSourceAddress),
		To:       ethtypes.MustNewAddress(testDestAddress),
		GasLimit: ethtypes.NewHexInteger64(1223451),
		Value:    ethtypes.NewHexInteger64(100),
	}
	// create transaction succeeded
	// gas estimate should be cached
	insertMock := mTS.On("InsertTransactionWithNextNonce", ctx, mock.Anything, mock.Anything)
	mEC.On("GetTransactionCount", mock.Anything, mock.Anything).
		Return(ethtypes.HexUint64(1), nil).Once()
	insertMock.Run(func(args mock.Arguments) {
		ctx := args[0].(context.Context)
		mtx := args[1].(*baseTypes.ManagedTX)
		nextNonceCB := args[2].(baseTypes.NextNonceCallback)
		nonce, err := nextNonceCB(ctx, string(mtx.From))
		assert.Equal(t, "1223451", mtx.GasLimit.String())
		assert.Nil(t, mtx.GasPrice)
		assert.Nil(t, err)
		assert.NotNil(t, nonce)
		insertMock.Return(nil)
	}).Once()
	txID := uuid.New()
	mTS.On("AddSubStatusAction", ctx, txID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionAssignNonce, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, _, err := enh.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
		GasLimit: testEthTxInput.GasLimit,
	}, &components.EthTransfer{
		To:    *types.MustEthAddress(testEthTxInput.To.String()),
		Value: testEthTxInput.Value,
	})
	assert.NoError(t, err)
	mEC.AssertNotCalled(t, "GasEstimate")
}

func TestHandleNewTransactionTransferOnlyForZeroGasPriceChain(t *testing.T) {
	ctx := context.Background()
	enh, _ := NewTestEnterpriseTransactionHandler(t)
	enh.gasPriceClient = NewTestZeroGasPriceChainClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	testEthTxInput := &ethsigner.Transaction{
		From:     []byte(testAutoFuelingSourceAddress),
		To:       ethtypes.MustNewAddress(testDestAddress),
		GasLimit: ethtypes.NewHexInteger64(1223451),
		Value:    ethtypes.NewHexInteger64(100),
	}
	// create transaction succeeded
	// gas estimate should be cached
	insertMock := mTS.On("InsertTransactionWithNextNonce", ctx, mock.Anything, mock.Anything)
	mEC.On("GetTransactionCount", mock.Anything, mock.Anything).
		Return(ethtypes.HexUint64(1), nil).Once()
	insertMock.Run(func(args mock.Arguments) {
		ctx := args[0].(context.Context)
		mtx := args[1].(*baseTypes.ManagedTX)
		nextNonceCB := args[2].(baseTypes.NextNonceCallback)
		nonce, err := nextNonceCB(ctx, string(mtx.From))
		assert.Equal(t, "1223451", mtx.GasLimit.String())
		assert.Equal(t, "0", mtx.GasPrice.String())
		assert.Nil(t, err)
		assert.NotNil(t, nonce)
		insertMock.Return(nil)
	}).Once()
	txID := uuid.New()
	mTS.On("AddSubStatusAction", ctx, txID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionAssignNonce, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, _, err := enh.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
		GasLimit: testEthTxInput.GasLimit,
	}, &components.EthTransfer{
		To:    *types.MustEthAddress(testEthTxInput.To.String()),
		Value: testEthTxInput.Value,
	})
	assert.NoError(t, err)
	mEC.AssertNotCalled(t, "GasEstimate")
}

func TestHandleNewTransaction(t *testing.T) {
	ctx := context.WithValue(context.Background(), ffapi.CtxHeadersKey{}, http.Header{
		"X-Kld-Authz": {"auth-header"},
	})

	signResponses := map[string][][2]string{
		"/api/v1/resolve/0xf101?intent=sign": {
			{"", `{"error": "not allowed"}`},
			{`{"data": "allowed"}`, ""},
			{`{"data": "allowed"}`, ""},
			{`{"data": "allowed"}`, ""},
			{`{"data": "allowed"}`, ""},
		},
	}
	enh, _ := NewTestEnterpriseTransactionHandlerWithSignPermissionResponse(t, signResponses)
	enh.gasPriceClient = NewTestFixedPriceGasPriceClient(t)

	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	testEthTxInput := &ethsigner.Transaction{
		From:  []byte(testMainSigningAddress),
		To:    ethtypes.MustNewAddress(testDestAddress),
		Value: ethtypes.NewHexInteger64(100),
		Data:  ethtypes.MustNewHexBytes0xPrefix(""),
	}
	// missing transaction ID
	_, submissionRejected, err := enh.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		SignerID: string(testEthTxInput.From),
		GasLimit: testEthTxInput.GasLimit,
	}, &components.EthTransaction{
		To:          *types.MustEthAddress(testEthTxInput.To.String()),
		FunctionABI: &abi.Entry{},
		Inputs:      &abi.ComponentValue{},
	})
	assert.NotNil(t, err)
	assert.True(t, submissionRejected)
	assert.Regexp(t, "PD011910", err)

	txID := uuid.New()
	// Build call data failure
	_, submissionRejected, err = enh.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
		GasLimit: testEthTxInput.GasLimit,
	}, &components.EthTransaction{
		To:          *types.MustEthAddress(testEthTxInput.To.String()),
		FunctionABI: nil,
		Inputs:      nil,
	})
	assert.NotNil(t, err)
	assert.False(t, submissionRejected)
	assert.Regexp(t, "TransactionPrepare error", err)

	// Gas estimate failure - non-revert
	mEC.On("GasEstimate", mock.Anything, &testEthTxInput).Return(nil, fmt.Errorf("something else")).Once()
	_, submissionRejected, err = enh.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
		GasLimit: testEthTxInput.GasLimit,
	}, &components.EthTransaction{
		To:          *types.MustEthAddress(testEthTxInput.To.String()),
		FunctionABI: nil,
		Inputs:      nil,
	})
	assert.NotNil(t, err)
	assert.False(t, submissionRejected)
	assert.Regexp(t, "TransactionPrepare error", err)

	// Gas estimate failure - revert
	mEC.On("GasEstimate", mock.Anything, &testEthTxInput).Return(nil, fmt.Errorf("execution reverted")).Once()
	_, submissionRejected, err = enh.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
		GasLimit: testEthTxInput.GasLimit,
	}, &components.EthTransaction{
		To:          *types.MustEthAddress(testEthTxInput.To.String()),
		FunctionABI: nil,
		Inputs:      nil,
	})
	assert.NotNil(t, err)
	assert.True(t, submissionRejected)
	assert.Regexp(t, "TransactionPrepare reverted", err)

	// create transaction succeeded
	mEC.On("GasEstimate", mock.Anything, &testEthTxInput).Return(ethtypes.NewHexInteger64(10), nil).Once()
	insertMock := mTS.On("InsertTransactionWithNextNonce", ctx, mock.Anything, mock.Anything)
	mEC.On("GetTransactionCount", mock.Anything, mock.Anything).
		Return(ethtypes.HexUint64(1), nil).Once()
	insertMock.Run(func(args mock.Arguments) {
		ctx := args[0].(context.Context)
		mtx := args[1].(*baseTypes.ManagedTX)
		assert.Equal(t, fftypes.NewFFBigInt(200), mtx.GasLimit)
		nextNonceCB := args[2].(baseTypes.NextNonceCallback)
		nonce, err := nextNonceCB(ctx, string(mtx.From))
		assert.Nil(t, err)
		assert.NotNil(t, nonce)
		insertMock.Return(nil)
	}).Once()
	mTS.On("AddSubStatusAction", ctx, txID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionAssignNonce, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	_, _, err = enh.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
		GasLimit: testEthTxInput.GasLimit,
	}, &components.EthTransaction{
		To:          *types.MustEthAddress(testEthTxInput.To.String()),
		FunctionABI: nil,
		Inputs:      nil,
	})
	assert.NoError(t, err)
}

func TestHandlerSuspend(t *testing.T) {
	ctx := context.Background()
	enh, _ := NewTestEnterpriseTransactionHandler(t)
	enh.gasPriceClient = NewTestFixedPriceGasPriceClient(t)

	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)

	imtxs := NewTestInMemoryTxState(t)
	mtx := imtxs.GetTx()

	// errored
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(nil, fmt.Errorf("get error")).Once()
	_, err := enh.HandleSuspendTransaction(ctx, mtx.ID)
	assert.Regexp(t, "get error", err)

	// controller update error
	suspendedStatus := baseTypes.BaseTxStatusSuspended
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	mTS.On("UpdateTransaction", ctx, mtx.ID, &baseTypes.BaseTXUpdates{
		Status: &suspendedStatus,
	}).Return(fmt.Errorf("update error")).Once()
	_, err = enh.HandleSuspendTransaction(ctx, mtx.ID)
	assert.Regexp(t, "update error", err)

	// controller update success
	mtx.Status = baseTypes.BaseTxStatusPending
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	mTS.On("UpdateTransaction", ctx, mtx.ID, &baseTypes.BaseTXUpdates{
		Status: &suspendedStatus,
	}).Return(nil).Once()
	tx, err := enh.HandleSuspendTransaction(ctx, mtx.ID)
	assert.NoError(t, err)
	assert.Equal(t, suspendedStatus, tx.Status)

	// engine handler tests
	enh.InFlightEngines = make(map[string]*transactionEngine)
	enh.InFlightEngines[string(mtx.From)] = &transactionEngine{
		enterpriseTransactionHandler: enh,
		enginePollingInterval:        enh.controllerPollingInterval,
		state:                        TransactionEngineStateIdle,
		stateEntryTime:               time.Now().Add(-enh.maxEngineIdle).Add(-1 * time.Minute),
		InFlightTxsStale:             make(chan bool, 1),
		stopProcess:                  make(chan bool, 1),
		transactionIDsInStatusUpdate: []string{"randomID"},
	}
	// engine update error
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	mTS.On("UpdateTransaction", ctx, mtx.ID, &baseTypes.BaseTXUpdates{
		Status: &suspendedStatus,
	}).Return(fmt.Errorf("update error")).Once()
	_, err = enh.HandleSuspendTransaction(ctx, mtx.ID)
	assert.Regexp(t, "update error", err)

	// engine update success
	mtx.Status = baseTypes.BaseTxStatusPending
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	mTS.On("UpdateTransaction", ctx, mtx.ID, &baseTypes.BaseTXUpdates{
		Status: &suspendedStatus,
	}).Return(nil).Once()
	tx, err = enh.HandleSuspendTransaction(ctx, mtx.ID)
	assert.NoError(t, err)
	assert.Equal(t, suspendedStatus, tx.Status)

	// in flight tx test
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	mtx = it.stateManager.GetTx()
	enh.InFlightEngines[string(mtx.From)].InFlightTxs = []*InFlightTransaction{
		it,
	}

	// async status update queued
	mtx.Status = baseTypes.BaseTxStatusPending
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	tx, err = enh.HandleSuspendTransaction(ctx, mtx.ID)
	assert.NoError(t, err)
	assert.Equal(t, baseTypes.BaseTxStatusPending, tx.Status)

	// already on the target status
	mtx.Status = baseTypes.BaseTxStatusSuspended
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	tx, err = enh.HandleSuspendTransaction(ctx, mtx.ID)
	assert.NoError(t, err)
	assert.Equal(t, baseTypes.BaseTxStatusSuspended, tx.Status)

	// error when try to update the status of a completed tx
	mtx.Status = baseTypes.BaseTxStatusFailed
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	_, err = enh.HandleSuspendTransaction(ctx, mtx.ID)
	assert.Regexp(t, "PD011921", err)
}

func TestHandlerResume(t *testing.T) {
	ctx := context.Background()

	enh, _ := NewTestEnterpriseTransactionHandler(t)
	enh.gasPriceClient = NewTestFixedPriceGasPriceClient(t)

	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)

	imtxs := NewTestInMemoryTxState(t)
	mtx := imtxs.GetTx()

	// errored
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(nil, fmt.Errorf("get error")).Once()
	_, err := enh.HandleResumeTransaction(ctx, mtx.ID)
	assert.Regexp(t, "get error", err)

	// controller update error
	pendingStatus := baseTypes.BaseTxStatusPending
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	mTS.On("UpdateTransaction", ctx, mtx.ID, &baseTypes.BaseTXUpdates{
		Status: &pendingStatus,
	}).Return(fmt.Errorf("update error")).Once()
	_, err = enh.HandleResumeTransaction(ctx, mtx.ID)
	assert.Regexp(t, "update error", err)

	// controller update success
	mtx.Status = baseTypes.BaseTxStatusSuspended
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	mTS.On("UpdateTransaction", ctx, mtx.ID, &baseTypes.BaseTXUpdates{
		Status: &pendingStatus,
	}).Return(nil).Once()
	tx, err := enh.HandleResumeTransaction(ctx, mtx.ID)
	assert.NoError(t, err)
	assert.Equal(t, pendingStatus, tx.Status)

	// engine handler tests
	enh.InFlightEngines = make(map[string]*transactionEngine)
	enh.InFlightEngines[string(mtx.From)] = &transactionEngine{
		enterpriseTransactionHandler: enh,
		enginePollingInterval:        enh.controllerPollingInterval,
		state:                        TransactionEngineStateIdle,
		stateEntryTime:               time.Now().Add(-enh.maxEngineIdle).Add(-1 * time.Minute),
		InFlightTxsStale:             make(chan bool, 1),
		stopProcess:                  make(chan bool, 1),
		transactionIDsInStatusUpdate: []string{"randomID"},
	}
	// engine update error
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	mTS.On("UpdateTransaction", ctx, mtx.ID, &baseTypes.BaseTXUpdates{
		Status: &pendingStatus,
	}).Return(fmt.Errorf("update error")).Once()
	_, err = enh.HandleResumeTransaction(ctx, mtx.ID)
	assert.Regexp(t, "update error", err)

	// engine update success
	mtx.Status = baseTypes.BaseTxStatusSuspended
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	mTS.On("UpdateTransaction", ctx, mtx.ID, &baseTypes.BaseTXUpdates{
		Status: &pendingStatus,
	}).Return(nil).Once()
	tx, err = enh.HandleResumeTransaction(ctx, mtx.ID)
	assert.NoError(t, err)
	assert.Equal(t, pendingStatus, tx.Status)

	// in flight tx test
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	mtx = it.stateManager.GetTx()
	enh.InFlightEngines[string(mtx.From)].InFlightTxs = []*InFlightTransaction{
		it,
	}

	// async status update queued
	mtx.Status = baseTypes.BaseTxStatusSuspended
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	tx, err = enh.HandleResumeTransaction(ctx, mtx.ID)
	assert.NoError(t, err)
	assert.Equal(t, baseTypes.BaseTxStatusSuspended, tx.Status)

	// already on the target status
	mtx.Status = baseTypes.BaseTxStatusPending
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	tx, err = enh.HandleResumeTransaction(ctx, mtx.ID)
	assert.NoError(t, err)
	assert.Equal(t, baseTypes.BaseTxStatusPending, tx.Status)

	// error when try to update the status of a completed tx
	mtx.Status = baseTypes.BaseTxStatusFailed
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	_, err = enh.HandleResumeTransaction(ctx, mtx.ID)
	assert.Regexp(t, "PD011921", err)
}

func TestHandlerCanceledContext(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	enh, _ := NewTestEnterpriseTransactionHandler(t)
	enh.gasPriceClient = NewTestFixedPriceGasPriceClient(t)

	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	enh.Init(ctx, mEC, mKM, mTS, mEN, mCL)

	imtxs := NewTestInMemoryTxState(t)
	mtx := imtxs.GetTx()
	mTS.On("UpdateTransaction", ctx, mtx.ID, mock.Anything).Return(nil).Maybe()

	// Suspend
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Run(func(args mock.Arguments) {
		cancelCtx()
	}).Once()
	_, err := enh.HandleSuspendTransaction(ctx, mtx.ID)
	assert.Regexp(t, "PD011926", err)

	// Resume
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Run(func(args mock.Arguments) {
		cancelCtx()
	}).Once()
	_, err = enh.HandleResumeTransaction(ctx, mtx.ID)
	assert.Regexp(t, "PD011926", err)
}
