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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
)

const testTransactionData string = "0x7369676e6564206d657373616765"

func NewTestInMemoryTxState(t *testing.T) InMemoryTxStateManager {
	oldTime := pldtypes.TimestampNow()
	oldFrom := pldtypes.MustEthAddress("0x4e598f6e918321dd47c86e7a077b4ab0e7414846")
	oldTxHash := pldtypes.RandBytes32()
	oldTo := pldtypes.MustEthAddress("0x6cee73cf4d5b0ac66ce2d1c0617bec4bedd09f39")
	oldNonce := pldtypes.HexUint64(1)
	oldGasLimit := pldtypes.HexUint64(2000)
	oldValue := pldtypes.Uint64ToUint256(200)
	oldGasPrice := pldtypes.Uint64ToUint256(10)
	oldErrorMessage := "old message"
	oldTransactionData := pldtypes.MustParseHexBytes(testTransactionData)
	testManagedTx := &DBPublicTxn{
		Created: oldTime,
		From:    *oldFrom,
		To:      oldTo,
		Nonce:   (*uint64)(&oldNonce),
		Gas:     oldGasLimit.Uint64(),
		Value:   oldValue,
		Data:    oldTransactionData,
	}

	imtxs := NewInMemoryTxStateManager(context.Background(), testManagedTx, nil)
	imtxs.ApplyInMemoryUpdates(context.Background(), &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			NewSubmission: &DBPubTxnSubmission{TransactionHash: oldTxHash},
			GasPricing: &pldapi.PublicTxGasPricing{
				MaxFeePerGas:         oldGasPrice,
				MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1),
			},
			FirstSubmit:  &oldTime,
			LastSubmit:   &oldTime,
			ErrorMessage: &oldErrorMessage,
		},
	})
	return imtxs

}

func TestSettersAndGetters(t *testing.T) {
	oldTime := pldtypes.TimestampNow()
	oldFrom := pldtypes.MustEthAddress("0xb3d9cf8e163bbc840195a97e81f8a34e295b8f39")
	oldTxHash := pldtypes.Bytes32Keccak([]byte("0x00000"))
	oldTo := pldtypes.MustEthAddress("0x1f9090aae28b8a3dceadf281b0f12828e676c326")
	oldNonce := pldtypes.HexUint64(1)
	oldGasLimit := pldtypes.HexUint64(2000)
	oldValue := pldtypes.Uint64ToUint256(200)
	oldGasPrice := pldtypes.Uint64ToUint256(10)
	oldErrorMessage := "old message"
	oldTransactionData := pldtypes.MustParseHexBytes(testTransactionData)

	testManagedTx := &DBPublicTxn{
		Created: oldTime,
		From:    *oldFrom,
		To:      oldTo,
		Nonce:   (*uint64)(&oldNonce),
		Gas:     uint64(oldGasLimit),
		Value:   oldValue,
		Data:    pldtypes.HexBytes(oldTransactionData),
	}

	imts := NewInMemoryTxStateManager(context.Background(), testManagedTx, nil)
	imts.ApplyInMemoryUpdates(context.Background(), &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			GasPricing:      &pldapi.PublicTxGasPricing{MaxFeePerGas: oldGasPrice, MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1)},
			TransactionHash: &oldTxHash,
			FirstSubmit:     &oldTime,
			LastSubmit:      &oldTime,
			ErrorMessage:    &oldErrorMessage,
		},
	})

	inMemoryTx := imts.(*inMemoryTxState)

	assert.Equal(t, fmt.Sprintf("%s:%d", oldFrom, oldNonce), imts.GetSignerNonce())

	assert.Equal(t, oldTime, *imts.GetCreatedTime())
	assert.Equal(t, oldTxHash, *imts.GetTransactionHash())
	assert.Equal(t, oldNonce.Uint64(), imts.GetNonce())
	assert.Equal(t, *oldFrom, imts.GetFrom())
	assert.Equal(t, InFlightStatusPending, imts.GetInFlightStatus())
	assert.Equal(t, oldGasPrice.Int(), imts.GetGasPriceObject().MaxFeePerGas.Int())
	assert.Equal(t, oldTime, *imts.GetFirstSubmit())
	assert.Equal(t, oldGasLimit.Uint64(), imts.GetGasLimit())
	assert.False(t, imts.IsReadyToExit())

	// mark the transaction complete
	confirmReceived := InFlightStatusConfirmReceived
	newTime := confutil.P(pldtypes.TimestampNow())
	newTxHash := pldtypes.Bytes32Keccak([]byte("0x000031"))
	newErrorMessage := "new message"
	newGasPricing := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(123),
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(2),
	}
	newGasPricingJSON, _ := json.Marshal(newGasPricing)

	imts.ApplyInMemoryUpdates(context.Background(), &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			InFlightStatus:  &confirmReceived,
			GasPricing:      newGasPricing,
			TransactionHash: &newTxHash,
			NewSubmission: &DBPubTxnSubmission{
				GasPricing: newGasPricingJSON,
			},
			FirstSubmit:  newTime,
			LastSubmit:   newTime,
			ErrorMessage: &newErrorMessage,
			Underpriced:  confutil.P(true),
		},
	})

	assert.Equal(t, InFlightStatusConfirmReceived, imts.GetInFlightStatus())
	assert.Equal(t, oldTime, *imts.GetCreatedTime())
	assert.Equal(t, newTime, imts.GetLastSubmitTime())
	assert.Equal(t, newTxHash, *imts.GetTransactionHash())
	assert.Equal(t, newGasPricing.MaxFeePerGas.Int(), imts.GetGasPriceObject().MaxFeePerGas.Int())
	assert.Equal(t, newGasPricing.MaxPriorityFeePerGas.Int(), imts.GetGasPriceObject().MaxPriorityFeePerGas.Int())
	assert.Equal(t, newGasPricing.MaxFeePerGas.Int(), imts.GetLastSubmittedGasPrice().MaxFeePerGas.Int())
	assert.Equal(t, newGasPricing.MaxPriorityFeePerGas.Int(), imts.GetLastSubmittedGasPrice().MaxPriorityFeePerGas.Int())
	assert.Equal(t, newTime, imts.GetFirstSubmit())
	assert.True(t, imts.GetUnderpriced())
	assert.True(t, imts.IsReadyToExit())

	// check immutable fields
	assert.Equal(t, oldNonce.Uint64(), imts.GetNonce())
	assert.Equal(t, *oldFrom, imts.GetFrom())
	assert.Equal(t, *oldTo, *imts.GetTo())
	assert.Equal(t, *oldValue, *imts.GetValue())
	assert.Equal(t, oldValue, inMemoryTx.mtx.ptx.Value)
	assert.Equal(t, oldTransactionData, inMemoryTx.mtx.ptx.Data)

	//check reset values
	imts.ApplyInMemoryUpdates(context.Background(), &BaseTXUpdates{
		ResetValues: BaseTXUpdateResetValues{
			GasPricing:      true,
			TransactionHash: true,
			Underpriced:     true,
		},
	})
	assert.Nil(t, imts.GetGasPriceObject())
	assert.Nil(t, imts.GetTransactionHash())
	assert.False(t, imts.GetUnderpriced())
}
