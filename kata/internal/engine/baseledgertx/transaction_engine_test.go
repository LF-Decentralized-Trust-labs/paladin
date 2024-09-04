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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/ffapi"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/components"
	baseTypes "github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"
	"github.com/kaleido-io/paladin/kata/mocks/enginemocks"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const testMainSigningAddress = testDestAddress

func NewTestTransactionEngine(t *testing.T) (*baseLedgerTxEngine, config.Section) {
	ctx := context.Background()
	conf := config.RootSection("unittest")
	InitConfig(conf)
	engineConf := conf.SubSection(TransactionEngineSection)
	engineConf.Set(TransactionEngineIntervalDurationString, "1h")
	engineConf.Set(TransactionEngineMaxInFlightOrchestratorsInt, -1)
	engConf := conf.SubSection(OrchestratorSection)
	engConf.Set(OrchestratorIntervalDurationString, "1h")
	engConf.Set(OrchestratorMaxInFlightTransactionsInt, -1)
	engConf.Set(OrchestratorSubmissionRetryCountInt, 0)

	th, err := NewTransactionEngine(ctx, conf)
	assert.Nil(t, err)
	return th.(*baseLedgerTxEngine), conf
}

func TestNewEngineErrors(t *testing.T) {
	ctx := context.Background()

	conf := config.RootSection("unittest")
	InitConfig(conf)

	// gasPriceIncreaseMax parsing error
	orchestratorConf := conf.SubSection(OrchestratorSection)
	orchestratorConf.Set(OrchestratorGasPriceIncreaseMaxBigIntString, "not a big int")
	_, err := NewTransactionEngine(ctx, conf)
	assert.NotNil(t, err)
	assert.Regexp(t, "PD011909", err)
	orchestratorConf.Set(OrchestratorGasPriceIncreaseMaxBigIntString, "")

	orchestratorConf.Set(OrchestratorGasPriceIncreaseMaxBigIntString, "1")
	h, err := NewTransactionEngine(ctx, conf)
	ble := h.(*baseLedgerTxEngine)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1), ble.gasPriceIncreaseMax)
	orchestratorConf.Set(OrchestratorGasPriceIncreaseMaxBigIntString, "")
}

func TestInit(t *testing.T) {
	ctx := context.Background()
	ble, _ := NewTestTransactionEngine(t)
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
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	ble.enginePollingInterval = 1 * time.Hour
	ble.maxInFlightOrchestrators = 1
	// starts ok
	_, _ = ble.Start(ctx)
	<-listed
	// init errors
	afConfig := ble.balanceManagerConfig.SubSection(BalanceManagerAutoFuelingSection)
	afConfig.Set(BalanceManagerAutoFuelingSourceAddressMinimumBalanceBigIntString, "not a big int")
	assert.Panics(t, func() {
		ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	})
	afConfig.Set(BalanceManagerAutoFuelingSourceAddressMinimumBalanceBigIntString, "0")
}

func TestHandleNewTransactionForTransferOnly(t *testing.T) {
	ctx := context.Background()

	ble, _ := NewTestTransactionEngine(t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)
	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	testEthTxInput := &ethsigner.Transaction{
		From:  []byte(testAutoFuelingSourceAddress),
		To:    ethtypes.MustNewAddress(testDestAddress),
		Value: ethtypes.NewHexInteger64(100),
	}
	txID := uuid.New()

	// resolve key failure
	mKM.On("ResolveKey", ctx, testAutoFuelingSourceAddress, algorithms.ECDSA_SECP256K1_PLAINBYTES).Return("", "", fmt.Errorf("pop")).Once()
	_, submissionRejected, err := ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
	}, &components.EthTransfer{
		To:    *types.MustEthAddress(testEthTxInput.To.String()),
		Value: testEthTxInput.Value,
	})
	assert.NotNil(t, err)
	assert.False(t, submissionRejected)
	assert.Regexp(t, "pop", err)

	mKM.On("ResolveKey", ctx, testAutoFuelingSourceAddress, algorithms.ECDSA_SECP256K1_PLAINBYTES).Return("", testAutoFuelingSourceAddress, nil)
	// estimation failure - for non-revert
	mEC.On("GasEstimate", mock.Anything, testEthTxInput).Return(nil, fmt.Errorf("GasEstimate error")).Once()
	_, submissionRejected, err = ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
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
	mEC.On("GasEstimate", mock.Anything, testEthTxInput).Return(nil, fmt.Errorf("execution reverted")).Once()
	_, submissionRejected, err = ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
	}, &components.EthTransfer{
		To:    *types.MustEthAddress(testEthTxInput.To.String()),
		Value: testEthTxInput.Value,
	})
	assert.NotNil(t, err)
	assert.True(t, submissionRejected)
	assert.Regexp(t, "execution reverted", err)

	// insert transaction next nonce error
	mEC.On("GasEstimate", mock.Anything, testEthTxInput).Return(ethtypes.NewHexInteger(big.NewInt(10)), nil)
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
	_, submissionRejected, err = ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
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
		Return(confutil.P(ethtypes.HexUint64(1)), nil).Once()
	insertMock.Run(func(args mock.Arguments) {
		ctx := args[0].(context.Context)
		mtx := args[1].(*baseTypes.ManagedTX)
		nextNonceCB := args[2].(baseTypes.NextNonceCallback)
		nonce, err := nextNonceCB(ctx, string(mtx.From))
		assert.Nil(t, err)
		assert.NotNil(t, nonce)
		insertMock.Return(nil)
	}).Once()
	mTS.On("AddSubStatusAction", ctx, txID.String(), baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionAssignNonce, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, _, err = ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
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
	ble, _ := NewTestTransactionEngine(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)
	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	mKM.On("ResolveKey", ctx, testAutoFuelingSourceAddress, algorithms.ECDSA_SECP256K1_PLAINBYTES).Return("", testAutoFuelingSourceAddress, nil)
	// fall back to connector when get call failed
	ble.gasPriceClient = NewTestNodeGasPriceClient(t, mEC)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
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
		Return(confutil.P(ethtypes.HexUint64(1)), nil).Once()
	insertMock.Run(func(args mock.Arguments) {
		ctx := args[0].(context.Context)
		mtx := args[1].(*baseTypes.ManagedTX)
		nextNonceCB := args[2].(baseTypes.NextNonceCallback)
		nonce, err := nextNonceCB(ctx, string(mtx.From))
		assert.Equal(t, "1223451", mtx.GasLimit.BigInt().String())
		assert.Nil(t, mtx.GasPrice)
		assert.Nil(t, err)
		assert.NotNil(t, nonce)
		insertMock.Return(nil)
	}).Once()
	txID := uuid.New()
	mTS.On("AddSubStatusAction", ctx, txID.String(), baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionAssignNonce, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, _, err := ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
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

func TestHandleNewTransactionTransferAndInvalidType(t *testing.T) {
	ctx := context.Background()
	ble, _ := NewTestTransactionEngine(t)
	ble.gasPriceClient = NewTestZeroGasPriceChainClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)
	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	mKM.On("ResolveKey", ctx, testAutoFuelingSourceAddress, algorithms.ECDSA_SECP256K1_PLAINBYTES).Return("", testAutoFuelingSourceAddress, nil)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
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
		Return(confutil.P(ethtypes.HexUint64(1)), nil).Once()
	insertMock.Run(func(args mock.Arguments) {
		ctx := args[0].(context.Context)
		mtx := args[1].(*baseTypes.ManagedTX)
		nextNonceCB := args[2].(baseTypes.NextNonceCallback)
		nonce, err := nextNonceCB(ctx, string(mtx.From))
		assert.Equal(t, "1223451", mtx.GasLimit.BigInt().String())
		assert.Equal(t, "0", mtx.GasPrice.BigInt().String())
		assert.Nil(t, err)
		assert.NotNil(t, nonce)
		insertMock.Return(nil)
	}).Once()
	txID := uuid.New()
	mTS.On("AddSubStatusAction", ctx, txID.String(), baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionAssignNonce, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, _, err := ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
		GasLimit: testEthTxInput.GasLimit,
	}, &components.EthTransfer{
		To:    *types.MustEthAddress(testEthTxInput.To.String()),
		Value: testEthTxInput.Value,
	})
	assert.NoError(t, err)
	mEC.AssertNotCalled(t, "GasEstimate")

	_, submissionRejected, err := ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
		GasLimit: testEthTxInput.GasLimit,
	}, "not a valid object")
	assert.Regexp(t, "PD011929", err)
	assert.True(t, submissionRejected)
	mEC.AssertNotCalled(t, "GasEstimate")
}

func TestHandleNewTransaction(t *testing.T) {
	ctx := context.Background()
	ble, _ := NewTestTransactionEngine(t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)
	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	mKM.On("ResolveKey", ctx, testDestAddress, algorithms.ECDSA_SECP256K1_PLAINBYTES).Return("", testDestAddress, nil)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	testEthTxInput := &ethsigner.Transaction{
		From:  []byte(testMainSigningAddress),
		To:    ethtypes.MustNewAddress(testDestAddress),
		Value: ethtypes.NewHexInteger64(100),
		Data:  ethtypes.MustNewHexBytes0xPrefix(""),
	}
	// missing transaction ID
	_, submissionRejected, err := ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
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
	// Parse API failure
	mEC.On("ABIFunction", ctx, mock.Anything).Return(nil, fmt.Errorf("ABI function parsing error")).Once()
	_, submissionRejected, err = ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
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
	assert.Regexp(t, "ABI function parsing error", err)

	// Build call data failure
	mABIBuilder := componentmocks.NewABIFunctionRequestBuilder(t)
	mABIBuilder.On("BuildCallData").Return(fmt.Errorf("Build data error")).Once()
	mABIF := componentmocks.NewABIFunctionClient(t)
	mABIF.On("R", ctx).Return(mABIBuilder).Once()
	mABIBuilder.On("To", ethtypes.MustNewAddress(testEthTxInput.To.String())).Return(mABIBuilder).Once()
	mABIBuilder.On("Input", mock.Anything).Return(mABIBuilder).Once()
	mEC.On("ABIFunction", ctx, mock.Anything).Return(mABIF, nil).Once()
	_, submissionRejected, err = ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
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
	assert.Regexp(t, "Build data error", err)

	// Gas estimate failure - non-revert
	mEC.On("GasEstimate", mock.Anything, testEthTxInput).Return(nil, fmt.Errorf("something else")).Once()
	mABIBuilder.On("BuildCallData").Return(nil).Once()
	mABIF.On("R", ctx).Return(mABIBuilder).Once()
	mABIBuilder.On("To", ethtypes.MustNewAddress(testEthTxInput.To.String())).Return(mABIBuilder).Once()
	mABIBuilder.On("Input", mock.Anything).Return(mABIBuilder).Once()
	mABIBuilder.On("TX", mock.Anything).Return(testEthTxInput).Once()
	mEC.On("ABIFunction", ctx, mock.Anything).Return(mABIF, nil).Once()
	_, submissionRejected, err = ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
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
	assert.Regexp(t, "something else", err)

	// Gas estimate failure - revert
	mEC.On("GasEstimate", mock.Anything, testEthTxInput).Return(nil, fmt.Errorf("execution reverted")).Once()
	mABIBuilder.On("BuildCallData").Return(nil).Once()
	mABIF.On("R", ctx).Return(mABIBuilder).Once()
	mABIBuilder.On("To", ethtypes.MustNewAddress(testEthTxInput.To.String())).Return(mABIBuilder).Once()
	mABIBuilder.On("Input", mock.Anything).Return(mABIBuilder).Once()
	mABIBuilder.On("TX", mock.Anything).Return(testEthTxInput).Once()
	mEC.On("ABIFunction", ctx, mock.Anything).Return(mABIF, nil).Once()
	_, submissionRejected, err = ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
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
	assert.Regexp(t, "execution reverted", err)

	// create transaction succeeded
	mEC.On("GasEstimate", mock.Anything, testEthTxInput).Return(ethtypes.NewHexInteger64(200), nil).Once()
	mABIBuilder.On("BuildCallData").Return(nil).Once()
	mABIF.On("R", ctx).Return(mABIBuilder).Once()
	mABIBuilder.On("To", ethtypes.MustNewAddress(testEthTxInput.To.String())).Return(mABIBuilder).Once()
	mABIBuilder.On("Input", mock.Anything).Return(mABIBuilder).Once()
	mABIBuilder.On("TX", mock.Anything).Return(testEthTxInput).Once()
	mEC.On("ABIFunction", ctx, mock.Anything).Return(mABIF, nil).Once()
	insertMock := mTS.On("InsertTransactionWithNextNonce", ctx, mock.Anything, mock.Anything)
	mEC.On("GetTransactionCount", mock.Anything, mock.Anything).
		Return(confutil.P(ethtypes.HexUint64(1)), nil).Once()
	insertMock.Run(func(args mock.Arguments) {
		ctx := args[0].(context.Context)
		mtx := args[1].(*baseTypes.ManagedTX)
		assert.Equal(t, big.NewInt(200), mtx.GasLimit.BigInt())
		nextNonceCB := args[2].(baseTypes.NextNonceCallback)
		nonce, err := nextNonceCB(ctx, string(mtx.From))
		assert.Nil(t, err)
		assert.NotNil(t, nonce)
		insertMock.Return(nil)
	}).Once()
	mTS.On("AddSubStatusAction", ctx, txID.String(), baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionAssignNonce, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	_, _, err = ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
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

func TestHandleNewDeployment(t *testing.T) {
	ctx := context.Background()
	ble, _ := NewTestTransactionEngine(t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)
	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	mKM.On("ResolveKey", ctx, testDestAddress, algorithms.ECDSA_SECP256K1_PLAINBYTES).Return("", testDestAddress, nil)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)
	testEthTxInput := &ethsigner.Transaction{
		From: []byte(testMainSigningAddress),
		Data: ethtypes.MustNewHexBytes0xPrefix(""),
	}
	txID := uuid.New()
	// Parse API failure
	mEC.On("ABIConstructor", ctx, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("ABI function parsing error")).Once()
	_, submissionRejected, err := ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
		GasLimit: testEthTxInput.GasLimit,
	}, &components.EthDeployTransaction{
		ConstructorABI: nil,
		Bytecode:       nil,
		Inputs:         nil,
	})
	assert.NotNil(t, err)
	assert.False(t, submissionRejected)
	assert.Regexp(t, "ABI function parsing error", err)

	// Build call data failure
	mABIBuilder := componentmocks.NewABIFunctionRequestBuilder(t)
	mABIBuilder.On("BuildCallData").Return(fmt.Errorf("Build data error")).Once()
	mABIF := componentmocks.NewABIFunctionClient(t)
	mABIF.On("R", ctx).Return(mABIBuilder).Once()
	mABIBuilder.On("Input", mock.Anything).Return(mABIBuilder).Once()
	mEC.On("ABIConstructor", ctx, mock.Anything, mock.Anything).Return(mABIF, nil).Once()
	_, submissionRejected, err = ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
		GasLimit: testEthTxInput.GasLimit,
	}, &components.EthDeployTransaction{
		ConstructorABI: nil,
		Bytecode:       types.HexBytes(testTransactionData),
		Inputs:         nil,
	})
	assert.NotNil(t, err)
	assert.False(t, submissionRejected)
	assert.Regexp(t, "Build data error", err)

	// Gas estimate failure - non-revert
	mEC.On("GasEstimate", mock.Anything, testEthTxInput).Return(nil, fmt.Errorf("something else")).Once()
	mABIBuilder.On("BuildCallData").Return(nil).Once()
	mABIF.On("R", ctx).Return(mABIBuilder).Once()
	mABIBuilder.On("Input", mock.Anything).Return(mABIBuilder).Once()
	mABIBuilder.On("TX", mock.Anything).Return(testEthTxInput).Once()
	mEC.On("ABIConstructor", ctx, mock.Anything, mock.Anything).Return(mABIF, nil).Once()
	_, submissionRejected, err = ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
		GasLimit: testEthTxInput.GasLimit,
	}, &components.EthDeployTransaction{
		ConstructorABI: nil,
		Bytecode:       types.HexBytes(testTransactionData),
		Inputs:         nil,
	})
	assert.NotNil(t, err)
	assert.False(t, submissionRejected)
	assert.Regexp(t, "something else", err)

	// Gas estimate failure - revert
	mEC.On("GasEstimate", mock.Anything, testEthTxInput).Return(nil, fmt.Errorf("execution reverted")).Once()
	mABIBuilder.On("BuildCallData").Return(nil).Once()
	mABIF.On("R", ctx).Return(mABIBuilder).Once()
	mABIBuilder.On("Input", mock.Anything).Return(mABIBuilder).Once()
	mABIBuilder.On("TX", mock.Anything).Return(testEthTxInput).Once()
	mEC.On("ABIConstructor", ctx, mock.Anything, mock.Anything).Return(mABIF, nil).Once()
	_, submissionRejected, err = ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
		GasLimit: testEthTxInput.GasLimit,
	}, &components.EthDeployTransaction{
		ConstructorABI: nil,
		Bytecode:       types.HexBytes(testTransactionData),
		Inputs:         nil,
	})
	assert.NotNil(t, err)
	assert.True(t, submissionRejected)
	assert.Regexp(t, "execution reverted", err)

	// create transaction succeeded
	mEC.On("GasEstimate", mock.Anything, testEthTxInput).Return(ethtypes.NewHexInteger64(200), nil).Once()
	mABIBuilder.On("BuildCallData").Return(nil).Once()
	mABIF.On("R", ctx).Return(mABIBuilder).Once()
	mABIBuilder.On("Input", mock.Anything).Return(mABIBuilder).Once()
	mABIBuilder.On("TX", mock.Anything).Return(testEthTxInput).Once()
	mEC.On("ABIConstructor", ctx, mock.Anything, mock.Anything).Return(mABIF, nil).Once()
	insertMock := mTS.On("InsertTransactionWithNextNonce", ctx, mock.Anything, mock.Anything)
	mEC.On("GetTransactionCount", mock.Anything, mock.Anything).
		Return(confutil.P(ethtypes.HexUint64(1)), nil).Once()
	insertMock.Run(func(args mock.Arguments) {
		ctx := args[0].(context.Context)
		mtx := args[1].(*baseTypes.ManagedTX)
		assert.Equal(t, big.NewInt(200), mtx.GasLimit.BigInt())
		nextNonceCB := args[2].(baseTypes.NextNonceCallback)
		nonce, err := nextNonceCB(ctx, string(mtx.From))
		assert.Nil(t, err)
		assert.NotNil(t, nonce)
		insertMock.Return(nil)
	}).Once()
	mTS.On("AddSubStatusAction", ctx, txID.String(), baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionAssignNonce, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	_, _, err = ble.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: string(testEthTxInput.From),
		GasLimit: testEthTxInput.GasLimit,
	}, &components.EthDeployTransaction{
		ConstructorABI: nil,
		Bytecode:       types.HexBytes(testTransactionData),
		Inputs:         nil,
	})
	assert.NoError(t, err)
}

func TestEngineSuspend(t *testing.T) {
	ctx := context.Background()
	ble, _ := NewTestTransactionEngine(t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)

	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)

	imtxs := NewTestInMemoryTxState(t)
	mtx := imtxs.GetTx()

	// errored
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(nil, fmt.Errorf("get error")).Once()
	_, err := ble.HandleSuspendTransaction(ctx, mtx.ID)
	assert.Regexp(t, "get error", err)

	// engine update error
	suspendedStatus := baseTypes.BaseTxStatusSuspended
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	mTS.On("UpdateTransaction", ctx, mtx.ID, &baseTypes.BaseTXUpdates{
		Status: &suspendedStatus,
	}).Return(fmt.Errorf("update error")).Once()
	_, err = ble.HandleSuspendTransaction(ctx, mtx.ID)
	assert.Regexp(t, "update error", err)

	// engine update success
	mtx.Status = baseTypes.BaseTxStatusPending
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	mTS.On("UpdateTransaction", ctx, mtx.ID, &baseTypes.BaseTXUpdates{
		Status: &suspendedStatus,
	}).Return(nil).Once()
	tx, err := ble.HandleSuspendTransaction(ctx, mtx.ID)
	assert.NoError(t, err)
	assert.Equal(t, suspendedStatus, tx.Status)

	// orchestrator handler tests
	ble.InFlightOrchestrators = make(map[string]*orchestrator)
	ble.InFlightOrchestrators[string(mtx.From)] = &orchestrator{
		baseLedgerTxEngine:           ble,
		orchestratorPollingInterval:  ble.enginePollingInterval,
		state:                        OrchestratorStateIdle,
		stateEntryTime:               time.Now().Add(-ble.maxOrchestratorIdle).Add(-1 * time.Minute),
		InFlightTxsStale:             make(chan bool, 1),
		stopProcess:                  make(chan bool, 1),
		transactionIDsInStatusUpdate: []string{"randomID"},
		txStore:                      mTS,
		ethClient:                    mEC,
		managedTXEventNotifier:       mEN,
		txConfirmationListener:       mCL,
	}
	// orchestrator update error
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	mTS.On("UpdateTransaction", ctx, mtx.ID, &baseTypes.BaseTXUpdates{
		Status: &suspendedStatus,
	}).Return(fmt.Errorf("update error")).Once()
	_, err = ble.HandleSuspendTransaction(ctx, mtx.ID)
	assert.Regexp(t, "update error", err)

	// orchestrator update success
	mtx.Status = baseTypes.BaseTxStatusPending
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	mTS.On("UpdateTransaction", ctx, mtx.ID, &baseTypes.BaseTXUpdates{
		Status: &suspendedStatus,
	}).Return(nil).Once()
	tx, err = ble.HandleSuspendTransaction(ctx, mtx.ID)
	assert.NoError(t, err)
	assert.Equal(t, suspendedStatus, tx.Status)

	// in flight tx test
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	mtx = it.stateManager.GetTx()
	ble.InFlightOrchestrators[string(mtx.From)].InFlightTxs = []*InFlightTransactionStageController{
		it,
	}

	// async status update queued
	mtx.Status = baseTypes.BaseTxStatusPending
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	tx, err = ble.HandleSuspendTransaction(ctx, mtx.ID)
	assert.NoError(t, err)
	assert.Equal(t, baseTypes.BaseTxStatusPending, tx.Status)

	// already on the target status
	mtx.Status = baseTypes.BaseTxStatusSuspended
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	tx, err = ble.HandleSuspendTransaction(ctx, mtx.ID)
	assert.NoError(t, err)
	assert.Equal(t, baseTypes.BaseTxStatusSuspended, tx.Status)

	// error when try to update the status of a completed tx
	mtx.Status = baseTypes.BaseTxStatusFailed
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	_, err = ble.HandleSuspendTransaction(ctx, mtx.ID)
	assert.Regexp(t, "PD011921", err)
}

func TestEngineResume(t *testing.T) {
	ctx := context.Background()

	ble, _ := NewTestTransactionEngine(t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)

	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)

	imtxs := NewTestInMemoryTxState(t)
	mtx := imtxs.GetTx()

	// errored
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(nil, fmt.Errorf("get error")).Once()
	_, err := ble.HandleResumeTransaction(ctx, mtx.ID)
	assert.Regexp(t, "get error", err)

	// engine update error
	pendingStatus := baseTypes.BaseTxStatusPending
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	mTS.On("UpdateTransaction", ctx, mtx.ID, &baseTypes.BaseTXUpdates{
		Status: &pendingStatus,
	}).Return(fmt.Errorf("update error")).Once()
	_, err = ble.HandleResumeTransaction(ctx, mtx.ID)
	assert.Regexp(t, "update error", err)

	// engine update success
	mtx.Status = baseTypes.BaseTxStatusSuspended
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	mTS.On("UpdateTransaction", ctx, mtx.ID, &baseTypes.BaseTXUpdates{
		Status: &pendingStatus,
	}).Return(nil).Once()
	tx, err := ble.HandleResumeTransaction(ctx, mtx.ID)
	assert.NoError(t, err)
	assert.Equal(t, pendingStatus, tx.Status)

	// orchestrator handler tests
	ble.InFlightOrchestrators = make(map[string]*orchestrator)
	ble.InFlightOrchestrators[string(mtx.From)] = &orchestrator{
		baseLedgerTxEngine:           ble,
		orchestratorPollingInterval:  ble.enginePollingInterval,
		state:                        OrchestratorStateIdle,
		stateEntryTime:               time.Now().Add(-ble.maxOrchestratorIdle).Add(-1 * time.Minute),
		InFlightTxsStale:             make(chan bool, 1),
		stopProcess:                  make(chan bool, 1),
		transactionIDsInStatusUpdate: []string{"randomID"},
		txStore:                      mTS,
		ethClient:                    mEC,
		managedTXEventNotifier:       mEN,
		txConfirmationListener:       mCL,
	}
	// orchestrator update error
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	mTS.On("UpdateTransaction", ctx, mtx.ID, &baseTypes.BaseTXUpdates{
		Status: &pendingStatus,
	}).Return(fmt.Errorf("update error")).Once()
	_, err = ble.HandleResumeTransaction(ctx, mtx.ID)
	assert.Regexp(t, "update error", err)

	// orchestrator update success
	mtx.Status = baseTypes.BaseTxStatusSuspended
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	mTS.On("UpdateTransaction", ctx, mtx.ID, &baseTypes.BaseTXUpdates{
		Status: &pendingStatus,
	}).Return(nil).Once()
	tx, err = ble.HandleResumeTransaction(ctx, mtx.ID)
	assert.NoError(t, err)
	assert.Equal(t, pendingStatus, tx.Status)

	// in flight tx test
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	mtx = it.stateManager.GetTx()
	ble.InFlightOrchestrators[string(mtx.From)].InFlightTxs = []*InFlightTransactionStageController{
		it,
	}

	// async status update queued
	mtx.Status = baseTypes.BaseTxStatusSuspended
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	tx, err = ble.HandleResumeTransaction(ctx, mtx.ID)
	assert.NoError(t, err)
	assert.Equal(t, baseTypes.BaseTxStatusSuspended, tx.Status)

	// already on the target status
	mtx.Status = baseTypes.BaseTxStatusPending
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	tx, err = ble.HandleResumeTransaction(ctx, mtx.ID)
	assert.NoError(t, err)
	assert.Equal(t, baseTypes.BaseTxStatusPending, tx.Status)

	// error when try to update the status of a completed tx
	mtx.Status = baseTypes.BaseTxStatusFailed
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Once()
	_, err = ble.HandleResumeTransaction(ctx, mtx.ID)
	assert.Regexp(t, "PD011921", err)
}

func TestEngineCanceledContext(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	ble, _ := NewTestTransactionEngine(t)
	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)

	mTS := enginemocks.NewTransactionStore(t)
	mCL := enginemocks.NewTransactionConfirmationListener(t)
	mEN := enginemocks.NewManagedTxEventNotifier(t)

	mEC := componentmocks.NewEthClient(t)
	mKM := componentmocks.NewKeyManager(t)
	ble.Init(ctx, mEC, mKM, mTS, mEN, mCL)

	imtxs := NewTestInMemoryTxState(t)
	mtx := imtxs.GetTx()
	mTS.On("UpdateTransaction", ctx, mtx.ID, mock.Anything).Return(nil).Maybe()

	// Suspend
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Run(func(args mock.Arguments) {
		cancelCtx()
	}).Once()
	_, err := ble.HandleSuspendTransaction(ctx, mtx.ID)
	assert.Regexp(t, "PD011926", err)

	// Resume
	mTS.On("GetTransactionByID", ctx, mtx.ID).Return(mtx, nil).Run(func(args mock.Arguments) {
		cancelCtx()
	}).Once()
	_, err = ble.HandleResumeTransaction(ctx, mtx.ID)
	assert.Regexp(t, "PD011926", err)
}
