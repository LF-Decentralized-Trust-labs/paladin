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
	"testing"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	baseTypes "github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"

	"github.com/stretchr/testify/assert"
)

func NewTestInMemoryTxState(t *testing.T) baseTypes.InMemoryTxStateManager {
	oldTime := fftypes.Now()
	oldFrom := "0x4e598f6e918321dd47c86e7a077b4ab0e7414846"
	oldTxHash := "0x00000"
	oldStatus := baseTypes.BaseTxStatusPending
	oldTo := "0x6cee73cf4d5b0ac66ce2d1c0617bec4bedd09f39"
	oldNonce := ethtypes.NewHexInteger64(1)
	oldGasLimit := ethtypes.NewHexInteger64(2000)
	oldValue := ethtypes.NewHexInteger64(200)
	oldGasPrice := ethtypes.NewHexInteger64(10)
	oldErrorMessage := "old message"
	oldTestPolicyInfo := &baseTypes.EnterprisePolicyInfo{
		LastWarnTime:      oldTime,
		SubmittedTxHashes: []string{"0x00000", "0x00001", "0x00002"},
	}
	oldTransactionData := ethtypes.MustNewHexBytes0xPrefix(testTransactionData)
	oldTestPolicyInfoBytes, _ := json.Marshal(oldTestPolicyInfo)
	testManagedTx := &baseTypes.ManagedTX{
		ID:              uuid.New().String(),
		Created:         oldTime,
		DeleteRequested: oldTime,
		Status:          oldStatus,
		TransactionHash: oldTxHash,
		Transaction: &ethsigner.Transaction{
			From:     json.RawMessage(oldFrom),
			To:       ethtypes.MustNewAddress(oldTo),
			Nonce:    oldNonce,
			GasLimit: oldGasLimit,
			Value:    oldValue,
			GasPrice: oldGasPrice,
			Data:     oldTransactionData,
		},
		PolicyInfo:   fftypes.JSONAnyPtrBytes(oldTestPolicyInfoBytes),
		FirstSubmit:  oldTime,
		LastSubmit:   oldTime,
		ErrorMessage: oldErrorMessage,
	}

	return NewInMemoryTxStateMananger(context.Background(), testManagedTx)

}

func TestSettersAndGetters(t *testing.T) {
	oldTime := fftypes.Now()
	oldFrom := "0xb3d9cf8e163bbc840195a97e81f8a34e295b8f39"
	oldTxHash := "0x00000"
	oldTo := "0x1f9090aae28b8a3dceadf281b0f12828e676c326"
	oldNonce := ethtypes.NewHexInteger64(1)
	oldGasLimit := ethtypes.NewHexInteger64(2000)
	oldValue := ethtypes.NewHexInteger64(200)
	oldGasPrice := ethtypes.NewHexInteger64(10)
	oldErrorMessage := "old message"
	oldTestPolicyInfo := &baseTypes.EnterprisePolicyInfo{
		LastWarnTime:      oldTime,
		SubmittedTxHashes: []string{"0x00000", "0x00001", "0x00002"},
	}
	oldTransactionData := ethtypes.MustNewHexBytes0xPrefix(testTransactionData)

	oldTestPolicyInfoBytes, _ := json.Marshal(oldTestPolicyInfo)
	testManagedTx := &baseTypes.ManagedTX{
		ID:              uuid.New().String(),
		Created:         oldTime,
		DeleteRequested: oldTime,
		Status:          baseTypes.BaseTxStatusPending,
		TransactionHash: oldTxHash,
		Transaction: &ethsigner.Transaction{
			From:     json.RawMessage(oldFrom),
			To:       ethtypes.MustNewAddress(oldTo),
			Nonce:    oldNonce,
			GasLimit: oldGasLimit,
			Value:    oldValue,
			GasPrice: oldGasPrice,
			Data:     oldTransactionData,
		},
		PolicyInfo:   fftypes.JSONAnyPtrBytes(oldTestPolicyInfoBytes),
		FirstSubmit:  oldTime,
		LastSubmit:   oldTime,
		ErrorMessage: oldErrorMessage,
	}

	inMemoryTxState := NewInMemoryTxStateMananger(context.Background(), testManagedTx)

	inMemoryTx := inMemoryTxState.GetTx()

	assert.Equal(t, testManagedTx.ID, inMemoryTxState.GetTxID())

	assert.Equal(t, oldTime, inMemoryTxState.GetCreatedTime())
	assert.Equal(t, oldTime, inMemoryTxState.GetDeleteRequestedTime())
	assert.Nil(t, inMemoryTxState.GetReceipt())
	assert.Equal(t, oldTxHash, inMemoryTxState.GetTransactionHash())
	assert.Equal(t, oldNonce.BigInt(), inMemoryTxState.GetNonce())
	assert.Equal(t, oldFrom, inMemoryTxState.GetFrom())
	assert.Equal(t, testManagedTx.Status, inMemoryTxState.GetStatus())
	assert.Equal(t, oldGasPrice.BigInt(), inMemoryTxState.GetGasPriceObject().GasPrice)
	assert.Equal(t, oldTime, inMemoryTxState.GetFirstSubmit())
	assert.Equal(t, oldTestPolicyInfo, inMemoryTxState.GetPolicyInfo())
	assert.Equal(t, testManagedTx, inMemoryTxState.GetTx())
	assert.Equal(t, oldGasLimit.BigInt(), inMemoryTxState.GetGasLimit())
	assert.False(t, inMemoryTxState.IsComplete())

	// add receipt to the pending transaction and mark it as complete with a policy info update
	testReceipt := ethclient.TransactionReceiptResponse{
		BlockNumber:      fftypes.NewFFBigInt(1233),
		TransactionIndex: fftypes.NewFFBigInt(23),
		BlockHash:        "0x000000000",
		Success:          true,
		ProtocolID:       "000000000/0023",
	}

	inMemoryTxState.SetReceipt(context.Background(), &testReceipt)
	assert.Equal(t, testReceipt, *inMemoryTxState.GetReceipt())
	successStatus := baseTypes.BaseTxStatusSucceeded
	newTime := fftypes.Now()
	newFrom := "0xf1031"
	newTxHash := "0x000031"
	newTo := "0x201"
	newNonce := ethtypes.NewHexInteger64(2)
	newGasLimit := ethtypes.NewHexInteger64(111)
	newValue := ethtypes.NewHexInteger64(222)
	newGasPrice := ethtypes.NewHexInteger64(111)
	newErrorMessage := "new message"
	newTestPolicyInfo := &baseTypes.EnterprisePolicyInfo{
		LastWarnTime:      newTime,
		SubmittedTxHashes: []string{"0x00000", "0x00001", "0x00002", "0x00003"},
	}
	newTestPolicyInfoBytes, _ := json.Marshal(newTestPolicyInfo)
	inMemoryTxState.ApplyTxUpdates(context.Background(), &baseTypes.BaseTXUpdates{
		Status:          &successStatus,
		DeleteRequested: newTime,
		GasPrice:        newGasPrice,
		TransactionHash: &newTxHash,
		PolicyInfo:      fftypes.JSONAnyPtrBytes(newTestPolicyInfoBytes),
		FirstSubmit:     newTime,
		LastSubmit:      newTime,
		ErrorMessage:    &newErrorMessage,
		GasLimit:        newGasLimit,
		// field that cannot be updated
		From:  &newFrom,
		To:    &newTo,
		Nonce: newNonce,
		Value: newValue,
	})

	assert.Equal(t, testManagedTx.ID, inMemoryTxState.GetTxID())

	assert.Equal(t, oldTime, inMemoryTxState.GetCreatedTime())
	assert.Equal(t, newTime, inMemoryTxState.GetDeleteRequestedTime())
	assert.Equal(t, testReceipt, *inMemoryTxState.GetReceipt())
	assert.Equal(t, newTxHash, inMemoryTxState.GetTransactionHash())
	assert.Equal(t, successStatus, inMemoryTxState.GetStatus())
	assert.Equal(t, newGasPrice.BigInt(), inMemoryTxState.GetGasPriceObject().GasPrice)
	assert.Nil(t, inMemoryTxState.GetGasPriceObject().MaxFeePerGas)
	assert.Nil(t, inMemoryTxState.GetGasPriceObject().MaxPriorityFeePerGas)
	assert.Equal(t, newTime, inMemoryTxState.GetFirstSubmit())
	assert.Equal(t, newTestPolicyInfo, inMemoryTxState.GetPolicyInfo())
	assert.Equal(t, testManagedTx, inMemoryTxState.GetTx())
	assert.Equal(t, newGasLimit.BigInt(), inMemoryTxState.GetGasLimit())
	assert.True(t, inMemoryTxState.IsComplete())

	// check immutable fields
	assert.Equal(t, oldNonce.BigInt(), inMemoryTxState.GetNonce())
	assert.Equal(t, oldFrom, inMemoryTxState.GetFrom())
	assert.Equal(t, oldValue, inMemoryTx.Value)
	assert.Equal(t, oldTransactionData, inMemoryTx.Data)

	maxPriorityFeePerGas := ethtypes.NewHexInteger64(2)
	maxFeePerGas := ethtypes.NewHexInteger64(123)

	// test switch gas price format
	inMemoryTxState.ApplyTxUpdates(context.Background(), &baseTypes.BaseTXUpdates{
		MaxPriorityFeePerGas: maxPriorityFeePerGas,
		MaxFeePerGas:         maxFeePerGas,
	})

	assert.Nil(t, inMemoryTxState.GetGasPriceObject().GasPrice)
	assert.Equal(t, maxFeePerGas.BigInt(), inMemoryTxState.GetGasPriceObject().MaxFeePerGas)
	assert.Equal(t, maxPriorityFeePerGas.BigInt(), inMemoryTxState.GetGasPriceObject().MaxPriorityFeePerGas)

	// test switch back and prefer legacy gas price

	maxPF := ethtypes.NewHexInteger64(3)
	maxF := ethtypes.NewHexInteger64(234)
	maxP := ethtypes.NewHexInteger64(10000)
	inMemoryTxState.ApplyTxUpdates(context.Background(), &baseTypes.BaseTXUpdates{
		MaxPriorityFeePerGas: maxPF,
		MaxFeePerGas:         maxF,
		GasPrice:             maxP,
	})

	assert.Equal(t, maxP.BigInt(), inMemoryTxState.GetGasPriceObject().GasPrice)
	assert.Nil(t, inMemoryTxState.GetGasPriceObject().MaxFeePerGas)
	assert.Nil(t, inMemoryTxState.GetGasPriceObject().MaxPriorityFeePerGas)
}
