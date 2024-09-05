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
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/core/internal/components"
	baseTypes "github.com/kaleido-io/paladin/core/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/mocks/enginemocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const testAutoFuelingSourceAddress = "0x4e598f6e918321dd47c86e7a077b4ab0e7414846"
const testDestAddress = "0x6cee73cf4d5b0ac66ce2d1c0617bec4bedd09f39"
const testSourceAddressBalance = 400
const testSourceAddressBalanceString = "400"

const testSourceAddressBalanceNew = 500
const testSourceAddressBalanceNewString = "500"

func NewTestBalanceManager(ctx context.Context, t *testing.T) (*BalanceManagerWithInMemoryTracking, *componentmocks.EthClient, *enginemocks.BaseLedgerTxEngine) {
	conf := config.RootSection("unittest")
	InitBalanceManagerConfig(conf)
	ResetBalanceManagerConfig(conf)

	bmConf := conf.SubSection(BalanceManagerSection)
	afConfig := conf.SubSection(fmt.Sprintf("%s.%s", BalanceManagerSection, BalanceManagerAutoFuelingSection))
	afConfig.Set(BalanceManagerAutoFuelingSourceAddressString, testAutoFuelingSourceAddress)

	mEthClient := componentmocks.NewEthClient(t)

	mockAFTxEngine := enginemocks.NewBaseLedgerTxEngine(t)

	testManagerWithMocks, err := NewBalanceManagerWithInMemoryTracking(ctx, bmConf, mEthClient, mockAFTxEngine)
	assert.NoError(t, err)

	return testManagerWithMocks.(*BalanceManagerWithInMemoryTracking), mEthClient, mockAFTxEngine
}

func TestNewBalanceManagerError(t *testing.T) {
	ctx := context.Background()
	conf := config.RootSection("unittest")
	InitBalanceManagerConfig(conf)
	ResetBalanceManagerConfig(conf)
	bmConf := conf.SubSection(BalanceManagerSection)

	mEthClient := componentmocks.NewEthClient(t)

	mockAFTxEngine := enginemocks.NewBaseLedgerTxEngine(t)

	afConfig := conf.SubSection(fmt.Sprintf("%s.%s", BalanceManagerSection, BalanceManagerAutoFuelingSection))

	afConfig.Set(BalanceManagerAutoFuelingMinThresholdBigIntString, "not a big int string")

	_, err := NewBalanceManagerWithInMemoryTracking(ctx, bmConf, mEthClient, mockAFTxEngine)
	assert.Error(t, err)
	assert.Regexp(t, fmt.Sprintf("PD011902: Value of '%s'", BalanceManagerAutoFuelingMinThresholdBigIntString), err.Error())

	afConfig.Set(BalanceManagerAutoFuelingMaxDestBalanceBigIntString, "not a big int string")

	_, err = NewBalanceManagerWithInMemoryTracking(ctx, bmConf, mEthClient, mockAFTxEngine)
	assert.Error(t, err)
	assert.Regexp(t, fmt.Sprintf("PD011902: Value of '%s'", BalanceManagerAutoFuelingMaxDestBalanceBigIntString), err.Error())

	afConfig.Set(BalanceManagerAutoFuelingMinDestBalanceBigIntString, "not a big int string")

	_, err = NewBalanceManagerWithInMemoryTracking(ctx, bmConf, mEthClient, mockAFTxEngine)
	assert.Error(t, err)
	assert.Regexp(t, fmt.Sprintf("PD011902: Value of '%s'", BalanceManagerAutoFuelingMinDestBalanceBigIntString), err.Error())

	afConfig.Set(BalanceManagerAutoFuelingSourceAddressMinimumBalanceBigIntString, "not a big int string")

	_, err = NewBalanceManagerWithInMemoryTracking(ctx, bmConf, mEthClient, mockAFTxEngine)
	assert.Error(t, err)
	assert.Regexp(t, fmt.Sprintf("PD011902: Value of '%s'", BalanceManagerAutoFuelingSourceAddressMinimumBalanceBigIntString), err.Error())

	// test min is greater than max value
	afConfig.Set(BalanceManagerAutoFuelingMaxDestBalanceBigIntString, "2")
	afConfig.Set(BalanceManagerAutoFuelingMinDestBalanceBigIntString, "3")
	afConfig.Set(BalanceManagerAutoFuelingSourceAddressMinimumBalanceBigIntString, "")
	afConfig.Set(BalanceManagerAutoFuelingMinThresholdBigIntString, "1")

	_, err = NewBalanceManagerWithInMemoryTracking(ctx, bmConf, mEthClient, mockAFTxEngine)
	assert.Error(t, err)
	assert.Regexp(t, "PD011903", err.Error())

	// test min threshold is greater than max value
	afConfig.Set(BalanceManagerAutoFuelingMaxDestBalanceBigIntString, "4")
	afConfig.Set(BalanceManagerAutoFuelingMinThresholdBigIntString, "10")

	_, err = NewBalanceManagerWithInMemoryTracking(ctx, bmConf, mEthClient, mockAFTxEngine)
	assert.Error(t, err)
	assert.Regexp(t, "PD011904", err.Error())
}

func TestIsAutoFuelingEnabled(t *testing.T) {
	ctx := context.Background()
	conf := config.RootSection("unittest")
	InitBalanceManagerConfig(conf)
	ResetBalanceManagerConfig(conf)
	bmConf := conf.SubSection(BalanceManagerSection)

	mEthClient := componentmocks.NewEthClient(t)

	mockAFTxEngine := enginemocks.NewBaseLedgerTxEngine(t)

	afConfig := conf.SubSection(fmt.Sprintf("%s.%s", BalanceManagerSection, BalanceManagerAutoFuelingSection))
	afConfig.Set(BalanceManagerAutoFuelingSourceAddressString, testAutoFuelingSourceAddress)

	bm, err := NewBalanceManagerWithInMemoryTracking(ctx, bmConf, mEthClient, mockAFTxEngine)
	assert.NoError(t, err)

	assert.True(t, bm.IsAutoFuelingEnabled(ctx))

	afConfig.Set(BalanceManagerAutoFuelingSourceAddressString, "") // TODO: validate the address
	bm, err = NewBalanceManagerWithInMemoryTracking(ctx, bmConf, mEthClient, mockAFTxEngine)
	assert.NoError(t, err)
	assert.False(t, bm.IsAutoFuelingEnabled(ctx))
}

func TestNotifyAddressBalanceChanged(t *testing.T) {
	ctx := context.Background()
	bm, _, _ := NewTestBalanceManager(ctx, t)
	assert.Equal(t, false, bm.addressBalanceChangedMap[testAutoFuelingSourceAddress])
	bm.NotifyAddressBalanceChanged(ctx, testAutoFuelingSourceAddress)
	assert.Equal(t, true, bm.addressBalanceChangedMap[testAutoFuelingSourceAddress])
}

func TestGetAddressBalance(t *testing.T) {
	ctx := context.Background()
	bm, mEthClient, _ := NewTestBalanceManager(ctx, t)
	defer mEthClient.AssertExpectations(t)

	mEthClient.On("GetBalance", mock.Anything, testAutoFuelingSourceAddress, "latest").Return(ethtypes.NewHexInteger64(testSourceAddressBalance), nil).Once()

	mEthClient.On("GetBalance", mock.Anything, testAutoFuelingSourceAddress, "latest").Return(ethtypes.NewHexInteger64(testSourceAddressBalanceNew), nil).Once()

	mEthClient.On("GetBalance", mock.Anything, testAutoFuelingSourceAddress, "latest").Return(nil, errors.New("pop")).Once()
	addressAccount, err := bm.GetAddressBalance(ctx, testAutoFuelingSourceAddress)
	assert.NoError(t, err)
	assert.NotNil(t, addressAccount)
	assert.Equal(t, testSourceAddressBalanceString, addressAccount.Balance.String())

	// next get should use the cache
	addressAccount, err = bm.GetAddressBalance(ctx, testAutoFuelingSourceAddress)
	assert.NoError(t, err)
	assert.Equal(t, testSourceAddressBalanceString, addressAccount.Balance.String())

	// next get should retrieve the balance again
	bm.NotifyAddressBalanceChanged(ctx, testAutoFuelingSourceAddress)
	addressAccount, err = bm.GetAddressBalance(ctx, testAutoFuelingSourceAddress)
	assert.NoError(t, err)
	assert.Equal(t, testSourceAddressBalanceNewString, addressAccount.Balance.String())

	// test error
	bm.NotifyAddressBalanceChanged(ctx, testAutoFuelingSourceAddress)
	_, err = bm.GetAddressBalance(ctx, testAutoFuelingSourceAddress)
	assert.Error(t, err)
}

func TestAddressAccountSpend(t *testing.T) {
	ctx := context.Background()
	bm, mEthClient, _ := NewTestBalanceManager(ctx, t)
	defer mEthClient.AssertExpectations(t)

	mEthClient.On("GetBalance", mock.Anything, testAutoFuelingSourceAddress, "latest").Return(ethtypes.NewHexInteger64(testSourceAddressBalance), nil).Once()
	addressAccount, err := bm.GetAddressBalance(ctx, testAutoFuelingSourceAddress)
	assert.NoError(t, err)
	assert.NotNil(t, addressAccount)
	assert.Equal(t, testSourceAddressBalanceString, addressAccount.Balance.String())
	assert.Equal(t, "0", addressAccount.Spent.String())
	assert.Equal(t, "0", addressAccount.MinCost.String())
	assert.Equal(t, "0", addressAccount.MaxCost.String())
	assert.Equal(t, 0, addressAccount.SpentTransactionCount)

	initialBalance := big.NewInt(testSourceAddressBalance)
	spent1 := big.NewInt(10)

	ats := addressAccount.Spend(ctx, spent1)
	expectedNewAvailableToSpend := initialBalance.Sub(initialBalance, spent1)
	assert.Equal(t, expectedNewAvailableToSpend.String(), ats.String())
	assert.Equal(t, spent1.String(), addressAccount.MinCost.String())
	assert.Equal(t, spent1.String(), addressAccount.MaxCost.String())
	assert.Equal(t, 1, addressAccount.SpentTransactionCount)

	spent2 := big.NewInt(20)
	ats = addressAccount.Spend(ctx, spent2)
	expectedNewAvailableToSpend = expectedNewAvailableToSpend.Sub(expectedNewAvailableToSpend, spent2)
	assert.Equal(t, expectedNewAvailableToSpend.String(), ats.String())
	assert.Equal(t, spent1.String(), addressAccount.MinCost.String())
	assert.Equal(t, spent2.String(), addressAccount.MaxCost.String())
	assert.Equal(t, 2, addressAccount.SpentTransactionCount)

	// negative number gets ignored
	spent3 := big.NewInt(-30)
	ats = addressAccount.Spend(ctx, spent3)
	assert.Equal(t, expectedNewAvailableToSpend.String(), ats.String())
	assert.Equal(t, spent1.String(), addressAccount.MinCost.String())
	assert.Equal(t, spent2.String(), addressAccount.MaxCost.String())
	assert.Equal(t, 2, addressAccount.SpentTransactionCount)

	// set new min cost
	spent4 := big.NewInt(5)
	ats = addressAccount.Spend(ctx, spent4)
	expectedNewAvailableToSpend = expectedNewAvailableToSpend.Sub(expectedNewAvailableToSpend, spent4)
	assert.Equal(t, expectedNewAvailableToSpend.String(), ats.String())
	assert.Equal(t, spent4.String(), addressAccount.MinCost.String())
	assert.Equal(t, spent2.String(), addressAccount.MaxCost.String())
	assert.Equal(t, 3, addressAccount.SpentTransactionCount)

	// 0 gets ignored
	ats = addressAccount.Spend(ctx, big.NewInt(0))
	assert.Equal(t, expectedNewAvailableToSpend.String(), ats.String())
	assert.Equal(t, spent4.String(), addressAccount.MinCost.String())
	assert.Equal(t, spent2.String(), addressAccount.MaxCost.String())
	assert.Equal(t, 3, addressAccount.SpentTransactionCount)

	balanceCopy := new(big.Int)
	balanceCopy.Set(addressAccount.Balance)
	amountToSpendAllBalance := balanceCopy.Sub(balanceCopy, addressAccount.Spent)
	spent5 := amountToSpendAllBalance.Add(amountToSpendAllBalance, big.NewInt(1))
	expectedNewAvailableToSpend = expectedNewAvailableToSpend.Sub(expectedNewAvailableToSpend, spent5)
	ats = addressAccount.Spend(ctx, spent5)
	assert.Equal(t, -1, ats.Sign())
	assert.Equal(t, expectedNewAvailableToSpend.String(), ats.String())
	assert.Equal(t, spent4.String(), addressAccount.MinCost.String())
	assert.Equal(t, spent5.String(), addressAccount.MaxCost.String())
	assert.Equal(t, 4, addressAccount.SpentTransactionCount)

	// do spent5 again
	expectedNewAvailableToSpend = expectedNewAvailableToSpend.Sub(expectedNewAvailableToSpend, spent5)
	ats = addressAccount.Spend(ctx, spent5)
	assert.Equal(t, -1, ats.Sign())
	assert.Equal(t, expectedNewAvailableToSpend.String(), ats.String())
	assert.Equal(t, spent4.String(), addressAccount.MinCost.String())
	assert.Equal(t, spent5.String(), addressAccount.MaxCost.String())
	assert.Equal(t, 5, addressAccount.SpentTransactionCount)
}

func TestTopUpAddressNoOpScenarios(t *testing.T) {
	ctx := context.Background()
	bm, _, _ := NewTestBalanceManager(ctx, t)

	// no spent on the account
	fuelingTx, err := bm.TopUpAccount(ctx, &baseTypes.AddressAccount{
		Spent: big.NewInt(0),
	})
	assert.NoError(t, err)
	assert.Nil(t, fuelingTx)

	// enough balance
	fuelingTx, err = bm.TopUpAccount(ctx, &baseTypes.AddressAccount{
		Spent:   big.NewInt(10),
		Balance: big.NewInt(10),
	})
	assert.NoError(t, err)
	assert.Nil(t, fuelingTx)

	// destination address already reached max amount
	bm.maxDestBalance = big.NewInt(100)
	fuelingTx, err = bm.TopUpAccount(ctx, &baseTypes.AddressAccount{
		Spent:                 big.NewInt(10),
		Balance:               big.NewInt(100),
		SpentTransactionCount: 10,
	})
	assert.NoError(t, err)
	assert.Nil(t, fuelingTx)

	// no source address configured
	bm.sourceAddress = ""
	fuelingTx, err = bm.TopUpAccount(ctx, &baseTypes.AddressAccount{
		Spent:                 big.NewInt(10),
		Balance:               big.NewInt(0),
		SpentTransactionCount: 10,
	})
	assert.NoError(t, err)
	assert.Nil(t, fuelingTx)

}

func generateExpectedFuelingTransaction(amountToTransfer int64) *baseTypes.ManagedTX {
	return &baseTypes.ManagedTX{
		Status: baseTypes.BaseTxStatusPending,
		Transaction: &ethsigner.Transaction{

			From:  []byte(testAutoFuelingSourceAddress),
			To:    ethtypes.MustNewAddress(testDestAddress),
			Nonce: ethtypes.NewHexInteger(big.NewInt(0)),
			Value: ethtypes.NewHexInteger(big.NewInt(amountToTransfer)),
		},
	}
}
func TestTopUpWithNoAmountModificationWithMultipleFuelingTxs(t *testing.T) {
	ctx := context.Background()
	bm, mEthClient, mockAFTxEngine := NewTestBalanceManager(ctx, t)
	defer mEthClient.AssertExpectations(t)
	defer mockAFTxEngine.AssertExpectations(t)

	accountToTopUp := &baseTypes.AddressAccount{
		Balance:               big.NewInt(100),
		Spent:                 big.NewInt(200),
		Address:               testDestAddress,
		SpentTransactionCount: 2,
		MinCost:               big.NewInt(50),
		MaxCost:               big.NewInt(150),
	}
	// source account balance retrieval
	mEthClient.On("GetBalance", mock.Anything, testAutoFuelingSourceAddress, "latest").Return(ethtypes.NewHexInteger64(testSourceAddressBalance), nil).Once()
	// no existing pending transaction
	mockAFTxEngine.On("GetPendingFuelingTransaction", mock.Anything, testAutoFuelingSourceAddress, testDestAddress).Return(nil, nil).Once()

	expectedTopUpAmount := big.NewInt(100)

	expectedFuelingTransaction1 := generateExpectedFuelingTransaction(expectedTopUpAmount.Int64())
	mockAFTxEngine.On("HandleNewTransaction", mock.Anything, mock.MatchedBy(func(txOptions *baseTypes.RequestOptions) bool {
		return txOptions.SignerID == testAutoFuelingSourceAddress
	}), mock.MatchedBy(func(transfer *components.EthTransfer) bool {
		return transfer.Value.BigInt().Cmp(expectedTopUpAmount) == 0 && transfer.To.String() == testDestAddress
	})).Return(expectedFuelingTransaction1, false, nil).Once()
	fuelingTx, err := bm.TopUpAccount(ctx, accountToTopUp)
	assert.NoError(t, err)
	assert.Equal(t, expectedFuelingTransaction1, fuelingTx)

	// Test no new fueling transaction when the current one is pending
	accountToTopUp2 := &baseTypes.AddressAccount{
		Balance:               big.NewInt(200),
		Spent:                 big.NewInt(250),
		Address:               testDestAddress,
		SpentTransactionCount: 1,
		MinCost:               big.NewInt(50),
		MaxCost:               big.NewInt(50),
	}
	// return not yet completed, so should return the existing pending transaction
	mockAFTxEngine.On("CheckTransactionCompleted", mock.Anything, expectedFuelingTransaction1).Return(false).Once()
	newFuelingTx, err := bm.TopUpAccount(ctx, accountToTopUp2)
	assert.NoError(t, err)
	assert.Equal(t, expectedFuelingTransaction1, newFuelingTx)

	// current transaction completed, replace with new transaction
	expectedTopUpAmount2 := big.NewInt(50)
	expectedFuelingTransaction2 := generateExpectedFuelingTransaction(expectedTopUpAmount2.Int64())
	mockAFTxEngine.On("CheckTransactionCompleted", mock.Anything, expectedFuelingTransaction1).Return(true).Once()
	mockAFTxEngine.On("HandleNewTransaction", mock.Anything, mock.MatchedBy(func(txOptions *baseTypes.RequestOptions) bool {
		return txOptions.SignerID == testAutoFuelingSourceAddress
	}), mock.MatchedBy(func(transfer *components.EthTransfer) bool {
		fmt.Printf("%d - %d", transfer.Value.Int64(), expectedTopUpAmount2.Int64())
		return transfer.Value.BigInt().Cmp(expectedTopUpAmount2) == 0 && transfer.To.String() == testDestAddress
	})).Return(expectedFuelingTransaction2, false, nil).Once()
	fuelingTx2, err := bm.TopUpAccount(ctx, accountToTopUp2)
	assert.NoError(t, err)
	assert.Equal(t, expectedFuelingTransaction2, fuelingTx2)

	// test when couldn't record the result of the submitted transaction
	// also do a balance look up
	accountToTopUp3 := &baseTypes.AddressAccount{
		Balance:               big.NewInt(250),
		Spent:                 big.NewInt(300),
		Address:               testDestAddress,
		SpentTransactionCount: 1,
		MinCost:               big.NewInt(50),
		MaxCost:               big.NewInt(50),
	}
	expectedTopUpAmount3 := big.NewInt(50)
	expectedFuelingTransaction3 := generateExpectedFuelingTransaction(expectedTopUpAmount3.Int64())
	bm.NotifyAddressBalanceChanged(ctx, testAutoFuelingSourceAddress)
	mEthClient.On("GetBalance", mock.Anything, testAutoFuelingSourceAddress, "latest").Return(ethtypes.NewHexInteger64(50), nil).Once()
	mockAFTxEngine.On("CheckTransactionCompleted", mock.Anything, expectedFuelingTransaction2).Return(true).Once()
	mockAFTxEngine.On("HandleNewTransaction", mock.Anything, mock.MatchedBy(func(txOptions *baseTypes.RequestOptions) bool {
		return txOptions.SignerID == testAutoFuelingSourceAddress
	}), mock.MatchedBy(func(transfer *components.EthTransfer) bool {
		return transfer.Value.BigInt().Cmp(expectedTopUpAmount3) == 0 && transfer.To.String() == testDestAddress
	})).Return(nil, false, errors.New("pop")).Once()
	newFuelingTx, err = bm.TopUpAccount(ctx, accountToTopUp3)
	assert.Error(t, err)
	assert.Equal(t, "pop", err.Error())
	assert.Nil(t, newFuelingTx)

	// test that we can recover if the transaction was actually registered in DB
	// also do a address balance re-lookup
	mockAFTxEngine.On("GetPendingFuelingTransaction", mock.Anything, testAutoFuelingSourceAddress, testDestAddress).Return(expectedFuelingTransaction3, nil).Once()
	mockAFTxEngine.On("CheckTransactionCompleted", mock.Anything, expectedFuelingTransaction3).Return(false).Once()
	fuelingTx3, err := bm.TopUpAccount(ctx, accountToTopUp3)
	assert.NoError(t, err)
	assert.Equal(t, expectedFuelingTransaction3, fuelingTx3)
}

func TestTopUpSuccessTopUpMinAheadUseMin(t *testing.T) {
	ctx := context.Background()
	bm, mEthClient, mockAFTxEngine := NewTestBalanceManager(ctx, t)
	defer mEthClient.AssertExpectations(t)
	defer mockAFTxEngine.AssertExpectations(t)

	accountToTopUp := &baseTypes.AddressAccount{
		Balance:               big.NewInt(100),
		Spent:                 big.NewInt(200),
		Address:               testDestAddress,
		SpentTransactionCount: 2,
		MinCost:               big.NewInt(50),
		MaxCost:               big.NewInt(150),
	}
	// source account balance retrieval
	mEthClient.On("GetBalance", mock.Anything, testAutoFuelingSourceAddress, "latest").Return(ethtypes.NewHexInteger64(testSourceAddressBalance), nil).Once()
	// no existing pending transaction
	mockAFTxEngine.On("GetPendingFuelingTransaction", mock.Anything, testAutoFuelingSourceAddress, testDestAddress).Return(nil, nil).Once()

	// set the minimum to have 2 extra spaces
	bm.proactiveFuelingTransactionTotal = 4
	bm.proactiveFuelingTransactionTotalEmptySlotCostCalcMethod = BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethodMin

	// the expectTopUpAmount should include min Value (50) multiply 2 extra space we set
	expectedTopUpAmount := big.NewInt(200)

	expectedFuelingTransaction := generateExpectedFuelingTransaction(expectedTopUpAmount.Int64())
	mockAFTxEngine.On("HandleNewTransaction", mock.Anything, mock.MatchedBy(func(txOptions *baseTypes.RequestOptions) bool {
		return txOptions.SignerID == testAutoFuelingSourceAddress
	}), mock.MatchedBy(func(transfer *components.EthTransfer) bool {
		return transfer.Value.BigInt().Cmp(expectedTopUpAmount) == 0 && transfer.To.String() == testDestAddress
	})).Return(expectedFuelingTransaction, false, nil).Once()
	fuelingTx, err := bm.TopUpAccount(ctx, accountToTopUp)
	assert.NoError(t, err)
	assert.Equal(t, expectedFuelingTransaction, fuelingTx)

}

func TestTopUpSuccessTopUpMinAheadUseMax(t *testing.T) {
	ctx := context.Background()
	bm, mEthClient, mockAFTxEngine := NewTestBalanceManager(ctx, t)
	defer mEthClient.AssertExpectations(t)
	defer mockAFTxEngine.AssertExpectations(t)

	accountToTopUp := &baseTypes.AddressAccount{
		Balance:               big.NewInt(100),
		Spent:                 big.NewInt(200),
		Address:               testDestAddress,
		SpentTransactionCount: 2,
		MinCost:               big.NewInt(50),
		MaxCost:               big.NewInt(150),
	}
	// source account balance retrieval
	mEthClient.On("GetBalance", mock.Anything, testAutoFuelingSourceAddress, "latest").Return(ethtypes.NewHexInteger64(testSourceAddressBalance), nil).Once()
	// no existing pending transaction
	mockAFTxEngine.On("GetPendingFuelingTransaction", mock.Anything, testAutoFuelingSourceAddress, testDestAddress).Return(nil, nil).Once()

	// set the minimum to have 2 extra spaces
	bm.proactiveFuelingTransactionTotal = 4
	bm.proactiveFuelingTransactionTotalEmptySlotCostCalcMethod = BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethodMax

	// the expectTopUpAmount should include max Value (150) multiply 2 extra space we set
	expectedTopUpAmount := big.NewInt(400)

	expectedFuelingTransaction := generateExpectedFuelingTransaction(expectedTopUpAmount.Int64())
	mockAFTxEngine.On("HandleNewTransaction", mock.Anything, mock.MatchedBy(func(txOptions *baseTypes.RequestOptions) bool {
		return txOptions.SignerID == testAutoFuelingSourceAddress
	}), mock.MatchedBy(func(transfer *components.EthTransfer) bool {
		return transfer.Value.BigInt().Cmp(expectedTopUpAmount) == 0 && transfer.To.String() == testDestAddress
	})).Return(expectedFuelingTransaction, false, nil).Once()
	fuelingTx, err := bm.TopUpAccount(ctx, accountToTopUp)
	assert.NoError(t, err)
	assert.Equal(t, expectedFuelingTransaction, fuelingTx)

}

func TestTopUpSuccessTopUpMinAheadUseAvg(t *testing.T) {
	ctx := context.Background()
	bm, mEthClient, mockAFTxEngine := NewTestBalanceManager(ctx, t)
	defer mEthClient.AssertExpectations(t)
	defer mockAFTxEngine.AssertExpectations(t)

	accountToTopUp := &baseTypes.AddressAccount{
		Balance:               big.NewInt(100),
		Spent:                 big.NewInt(200),
		Address:               testDestAddress,
		SpentTransactionCount: 2,
		MinCost:               big.NewInt(50),
		MaxCost:               big.NewInt(150),
	}
	// source account balance retrieval
	mEthClient.On("GetBalance", mock.Anything, testAutoFuelingSourceAddress, "latest").Return(ethtypes.NewHexInteger64(testSourceAddressBalance), nil).Once()
	// no existing pending transaction
	mockAFTxEngine.On("GetPendingFuelingTransaction", mock.Anything, testAutoFuelingSourceAddress, testDestAddress).Return(nil, nil).Once()

	// set the minimum to have 2 extra spaces
	bm.proactiveFuelingTransactionTotal = 4
	bm.proactiveFuelingTransactionTotalEmptySlotCostCalcMethod = BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethodAverage

	// the expectTopUpAmount should include avg Value (100) multiply 2 extra space we set
	expectedTopUpAmount := big.NewInt(300)

	expectedFuelingTransaction := generateExpectedFuelingTransaction(expectedTopUpAmount.Int64())
	mockAFTxEngine.On("HandleNewTransaction", mock.Anything, mock.MatchedBy(func(txOptions *baseTypes.RequestOptions) bool {
		return txOptions.SignerID == testAutoFuelingSourceAddress
	}), mock.MatchedBy(func(transfer *components.EthTransfer) bool {
		return transfer.Value.BigInt().Cmp(expectedTopUpAmount) == 0 && transfer.To.String() == testDestAddress
	})).Return(expectedFuelingTransaction, false, nil).Once()
	fuelingTx, err := bm.TopUpAccount(ctx, accountToTopUp)
	assert.NoError(t, err)
	assert.Equal(t, expectedFuelingTransaction, fuelingTx)

}

func TestTopUpSuccessUseMinDestBalance(t *testing.T) {
	ctx := context.Background()
	bm, mEthClient, mockAFTxEngine := NewTestBalanceManager(ctx, t)
	defer mEthClient.AssertExpectations(t)
	defer mockAFTxEngine.AssertExpectations(t)

	accountToTopUp := &baseTypes.AddressAccount{
		Balance:               big.NewInt(100),
		Spent:                 big.NewInt(200),
		Address:               testDestAddress,
		SpentTransactionCount: 2,
		MinCost:               big.NewInt(50),
		MaxCost:               big.NewInt(150),
	}
	// source account balance retrieval
	mEthClient.On("GetBalance", mock.Anything, testAutoFuelingSourceAddress, "latest").Return(ethtypes.NewHexInteger64(testSourceAddressBalance), nil).Once()
	// no existing pending transaction
	mockAFTxEngine.On("GetPendingFuelingTransaction", mock.Anything, testAutoFuelingSourceAddress, testDestAddress).Return(nil, nil).Once()

	// set min top up to balance to 250 (50 above the required amount)
	bm.minDestBalance = big.NewInt(250)
	expectedTopUpAmount := big.NewInt(150)
	expectedFuelingTransaction := generateExpectedFuelingTransaction(expectedTopUpAmount.Int64())
	mockAFTxEngine.On("HandleNewTransaction", mock.Anything, mock.MatchedBy(func(txOptions *baseTypes.RequestOptions) bool {
		return txOptions.SignerID == testAutoFuelingSourceAddress
	}), mock.MatchedBy(func(transfer *components.EthTransfer) bool {
		return transfer.Value.BigInt().Cmp(expectedTopUpAmount) == 0 && transfer.To.String() == testDestAddress
	})).Return(expectedFuelingTransaction, false, nil).Once()
	fuelingTx, err := bm.TopUpAccount(ctx, accountToTopUp)
	assert.NoError(t, err)
	assert.Equal(t, expectedFuelingTransaction, fuelingTx)
}

func TestTopUpSuccessUseMaxDestBalance(t *testing.T) {
	ctx := context.Background()
	bm, mEthClient, mockAFTxEngine := NewTestBalanceManager(ctx, t)
	defer mEthClient.AssertExpectations(t)
	defer mockAFTxEngine.AssertExpectations(t)

	accountToTopUp := &baseTypes.AddressAccount{
		Balance:               big.NewInt(100),
		Spent:                 big.NewInt(200),
		Address:               testDestAddress,
		SpentTransactionCount: 2,
		MinCost:               big.NewInt(50),
		MaxCost:               big.NewInt(150),
	}
	// source account balance retrieval
	mEthClient.On("GetBalance", mock.Anything, testAutoFuelingSourceAddress, "latest").Return(ethtypes.NewHexInteger64(testSourceAddressBalance), nil).Once()
	// no existing pending transaction
	mockAFTxEngine.On("GetPendingFuelingTransaction", mock.Anything, testAutoFuelingSourceAddress, testDestAddress).Return(nil, nil).Once()

	// set max top up to balance to 150 (50 below the required amount)
	bm.maxDestBalance = big.NewInt(150)
	expectedTopUpAmount := big.NewInt(50)

	expectedFuelingTransaction := generateExpectedFuelingTransaction(expectedTopUpAmount.Int64())
	mockAFTxEngine.On("HandleNewTransaction", mock.Anything, mock.MatchedBy(func(txOptions *baseTypes.RequestOptions) bool {
		return txOptions.SignerID == testAutoFuelingSourceAddress
	}), mock.MatchedBy(func(transfer *components.EthTransfer) bool {
		return transfer.Value.BigInt().Cmp(expectedTopUpAmount) == 0 && transfer.To.String() == testDestAddress
	})).Return(expectedFuelingTransaction, false, nil).Once()
	fuelingTx, err := bm.TopUpAccount(ctx, accountToTopUp)
	assert.NoError(t, err)
	assert.Equal(t, expectedFuelingTransaction, fuelingTx)
}

func TestTopUpNoOpAlreadyAboveMaxDestBalance(t *testing.T) {
	ctx := context.Background()
	bm, mEthClient, mockAFTxEngine := NewTestBalanceManager(ctx, t)
	defer mEthClient.AssertExpectations(t)
	defer mockAFTxEngine.AssertExpectations(t)

	accountToTopUp := &baseTypes.AddressAccount{
		Balance:               big.NewInt(100),
		Spent:                 big.NewInt(200),
		Address:               testDestAddress,
		SpentTransactionCount: 2,
		MinCost:               big.NewInt(50),
		MaxCost:               big.NewInt(150),
	}

	// set max top up to balance to 90, which is below the balance
	bm.maxDestBalance = big.NewInt(90)
	fuelingTx, err := bm.TopUpAccount(ctx, accountToTopUp)
	assert.NoError(t, err)
	assert.Nil(t, fuelingTx)
}

func TestTopUpNoOpAmountBelowMinThreshold(t *testing.T) {
	ctx := context.Background()
	bm, mEthClient, mockAFTxEngine := NewTestBalanceManager(ctx, t)
	defer mEthClient.AssertExpectations(t)
	defer mockAFTxEngine.AssertExpectations(t)

	accountToTopUp := &baseTypes.AddressAccount{
		Balance:               big.NewInt(100),
		Spent:                 big.NewInt(200),
		Address:               testDestAddress,
		SpentTransactionCount: 2,
		MinCost:               big.NewInt(50),
		MaxCost:               big.NewInt(150),
	}

	// set minimum top up threshold to 150, which is above the required amount
	bm.minThreshold = big.NewInt(150)
	fuelingTx, err := bm.TopUpAccount(ctx, accountToTopUp)
	assert.NoError(t, err)
	assert.Nil(t, fuelingTx)
}

func TestTopUpFailedDueToSourceBalanceBelowMin(t *testing.T) {
	ctx := context.Background()
	bm, mEthClient, mockAFTxEngine := NewTestBalanceManager(ctx, t)
	defer mEthClient.AssertExpectations(t)
	defer mockAFTxEngine.AssertExpectations(t)

	accountToTopUp := &baseTypes.AddressAccount{
		Balance:               big.NewInt(100),
		Spent:                 big.NewInt(200),
		Address:               testDestAddress,
		SpentTransactionCount: 2,
		MinCost:               big.NewInt(50),
		MaxCost:               big.NewInt(150),
	}
	// source account balance retrieval
	mEthClient.On("GetBalance", mock.Anything, testAutoFuelingSourceAddress, "latest").Return(ethtypes.NewHexInteger64(testSourceAddressBalance), nil).Once()
	// no existing pending transaction
	mockAFTxEngine.On("GetPendingFuelingTransaction", mock.Anything, testAutoFuelingSourceAddress, testDestAddress).Return(nil, nil).Once()

	// set min source balance to 1000, which is way beyond 400
	bm.minSourceBalance = big.NewInt(1000)

	fuelingTx, err := bm.TopUpAccount(ctx, accountToTopUp)
	assert.Error(t, err)
	assert.Nil(t, fuelingTx)
	assert.Regexp(t, "PD011901: Balance 400 of fueling source address 0x4e598f6e918321dd47c86e7a077b4ab0e7414846 is below the configured minimum balance 1000", err.Error())
}

func TestTopUpFailedDueToSourceBalanceBelowRequestedAmount(t *testing.T) {
	ctx := context.Background()
	bm, mEthClient, mockAFTxEngine := NewTestBalanceManager(ctx, t)
	defer mEthClient.AssertExpectations(t)
	defer mockAFTxEngine.AssertExpectations(t)

	accountToTopUp := &baseTypes.AddressAccount{
		Balance:               big.NewInt(100),
		Spent:                 big.NewInt(2000),
		Address:               testDestAddress,
		SpentTransactionCount: 2,
		MinCost:               big.NewInt(500),
		MaxCost:               big.NewInt(1500),
	}
	// source account balance retrieval
	mEthClient.On("GetBalance", mock.Anything, testAutoFuelingSourceAddress, "latest").Return(ethtypes.NewHexInteger64(testSourceAddressBalance), nil).Once()
	// no existing pending transaction
	mockAFTxEngine.On("GetPendingFuelingTransaction", mock.Anything, testAutoFuelingSourceAddress, testDestAddress).Return(nil, nil).Once()

	fuelingTx, err := bm.TopUpAccount(ctx, accountToTopUp)
	assert.Error(t, err)
	assert.Nil(t, fuelingTx)
	assert.Regexp(t, "PD011900: Balance 400 of fueling source address 0x4e598f6e918321dd47c86e7a077b4ab0e7414846 is below the required amount 1900", err.Error())
}

func TestTopUpFailedDueToUnableToGetPendingFuelingTransaction(t *testing.T) {
	ctx := context.Background()
	bm, mEthClient, mockAFTxEngine := NewTestBalanceManager(ctx, t)
	defer mEthClient.AssertExpectations(t)
	defer mockAFTxEngine.AssertExpectations(t)

	accountToTopUp := &baseTypes.AddressAccount{
		Balance:               big.NewInt(100),
		Spent:                 big.NewInt(200),
		Address:               testDestAddress,
		SpentTransactionCount: 2,
		MinCost:               big.NewInt(50),
		MaxCost:               big.NewInt(150),
	}
	// fail to get existing fueling tx
	mockAFTxEngine.On("GetPendingFuelingTransaction", mock.Anything, testAutoFuelingSourceAddress, testDestAddress).Return(nil, errors.New("pop")).Once()

	fuelingTx, err := bm.TopUpAccount(ctx, accountToTopUp)
	assert.Error(t, err)
	assert.Nil(t, fuelingTx)
	assert.Regexp(t, "pop", err.Error())
}

func TestTopUpFailedDueToUnableToGetSourceAddressBalance(t *testing.T) {
	ctx := context.Background()
	bm, mEthClient, mockAFTxEngine := NewTestBalanceManager(ctx, t)
	defer mEthClient.AssertExpectations(t)
	defer mockAFTxEngine.AssertExpectations(t)

	accountToTopUp := &baseTypes.AddressAccount{
		Balance:               big.NewInt(100),
		Spent:                 big.NewInt(200),
		Address:               testDestAddress,
		SpentTransactionCount: 2,
		MinCost:               big.NewInt(50),
		MaxCost:               big.NewInt(150),
	}
	// source account balance retrieval failed
	mEthClient.On("GetBalance", mock.Anything, testAutoFuelingSourceAddress, "latest").Return(nil, errors.New("pop")).Once()
	// no existing pending transaction
	mockAFTxEngine.On("GetPendingFuelingTransaction", mock.Anything, testAutoFuelingSourceAddress, testDestAddress).Return(nil, nil).Once()

	fuelingTx, err := bm.TopUpAccount(ctx, accountToTopUp)
	assert.Error(t, err)
	assert.Nil(t, fuelingTx)
	assert.Regexp(t, "pop", err.Error())
}
