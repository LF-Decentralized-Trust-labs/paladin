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
	"errors"
	"math/big"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newTestBalanceManager(t *testing.T) (context.Context, *BalanceManagerWithInMemoryTracking, *pubTxManager, *mocksAndTestControl, func()) {
	ctx, ble, m, done := newTestPublicTxManager(t, false)
	balanceManager := NewBalanceManagerWithInMemoryTracking(ctx, ble.conf, ble)
	return ctx, balanceManager.(*BalanceManagerWithInMemoryTracking), ble, m, done
}

func TestNotifyAddressBalanceChanged(t *testing.T) {
	ctx, bm, _, _, done := newTestBalanceManager(t)
	defer done()

	exampleAddr := *pldtypes.RandAddress()
	assert.Equal(t, false, bm.addressBalanceChangedMap[exampleAddr])
	bm.NotifyAddressBalanceChanged(ctx, exampleAddr)
	assert.Equal(t, true, bm.addressBalanceChangedMap[exampleAddr])
}

func TestGetAddressBalance(t *testing.T) {
	ctx, bm, _, m, done := newTestBalanceManager(t)
	defer done()

	const balanceOld = uint64(400)
	const balanceNew = uint64(500)

	exampleAddr := *pldtypes.RandAddress()

	m.ethClient.On("GetBalance", mock.Anything, exampleAddr, "latest").Return(pldtypes.Uint64ToUint256(balanceOld), nil).Once()

	m.ethClient.On("GetBalance", mock.Anything, exampleAddr, "latest").Return(pldtypes.Uint64ToUint256(balanceNew), nil).Once()

	m.ethClient.On("GetBalance", mock.Anything, exampleAddr, "latest").Return(nil, errors.New("pop")).Once()

	addressAccount, err := bm.GetAddressBalance(ctx, exampleAddr)
	require.NoError(t, err)
	assert.NotNil(t, addressAccount)
	assert.Equal(t, balanceOld, addressAccount.Balance.Uint64())

	// next get should use the cache
	addressAccount, err = bm.GetAddressBalance(ctx, exampleAddr)
	require.NoError(t, err)
	assert.Equal(t, balanceOld, addressAccount.Balance.Uint64())

	// next get should retrieve the balance again
	bm.NotifyAddressBalanceChanged(ctx, exampleAddr)
	addressAccount, err = bm.GetAddressBalance(ctx, exampleAddr)
	require.NoError(t, err)
	assert.Equal(t, balanceNew, addressAccount.Balance.Uint64())

	// test error
	bm.NotifyAddressBalanceChanged(ctx, exampleAddr)
	_, err = bm.GetAddressBalance(ctx, exampleAddr)
	assert.Error(t, err)
}

func TestAddressAccountSpend(t *testing.T) {
	ctx, bm, _, m, done := newTestBalanceManager(t)
	defer done()

	exampleAddr := *pldtypes.RandAddress()

	const balanceOld = uint64(400)

	m.ethClient.On("GetBalance", mock.Anything, exampleAddr, "latest").Return(pldtypes.Uint64ToUint256(balanceOld), nil).Once()
	addressAccount, err := bm.GetAddressBalance(ctx, exampleAddr)
	require.NoError(t, err)
	assert.NotNil(t, addressAccount)
	assert.Equal(t, balanceOld, addressAccount.Balance.Uint64())
	assert.Equal(t, "0", addressAccount.Spent.String())
	assert.Equal(t, "0", addressAccount.MinCost.String())
	assert.Equal(t, "0", addressAccount.MaxCost.String())
	assert.Equal(t, 0, addressAccount.SpentTransactionCount)

	initialBalance := big.NewInt(int64(balanceOld))
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
