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
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
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

	imtxs := NewInMemoryTxStateManager(context.Background(), testManagedTx)
	imtxs.ApplyInMemoryUpdates(context.Background(), &BaseTXUpdates{
		NewSubmission: &DBPubTxnSubmission{TransactionHash: oldTxHash},
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: oldGasPrice,
		},
		FirstSubmit:  &oldTime,
		LastSubmit:   &oldTime,
		ErrorMessage: &oldErrorMessage,
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

	imts := NewInMemoryTxStateManager(context.Background(), testManagedTx)
	imts.ApplyInMemoryUpdates(context.Background(), &BaseTXUpdates{
		GasPricing:      &pldapi.PublicTxGasPricing{GasPrice: oldGasPrice},
		TransactionHash: &oldTxHash,
		FlushedSubmission: &DBPubTxnSubmission{
			TransactionHash: oldTxHash,
		},
		FirstSubmit:  &oldTime,
		LastSubmit:   &oldTime,
		ErrorMessage: &oldErrorMessage,
	})

	inMemoryTx := imts.(*inMemoryTxState)

	assert.Equal(t, fmt.Sprintf("%s:%d", oldFrom, oldNonce), imts.GetSignerNonce())

	assert.Equal(t, oldTime, *imts.GetCreatedTime())
	assert.Equal(t, oldTxHash, *imts.GetTransactionHash())
	assert.Equal(t, oldNonce.Uint64(), imts.GetNonce())
	assert.Equal(t, *oldFrom, imts.GetFrom())
	assert.Equal(t, InFlightStatusPending, imts.GetInFlightStatus())
	assert.Equal(t, oldGasPrice.Int(), imts.GetGasPriceObject().GasPrice.Int())
	assert.Equal(t, oldTime, *imts.GetFirstSubmit())
	assert.Equal(t, oldGasLimit.Uint64(), imts.GetGasLimit())
	assert.False(t, imts.IsReadyToExit())

	// dup flush
	imts.ApplyInMemoryUpdates(context.Background(), &BaseTXUpdates{
		FlushedSubmission: &DBPubTxnSubmission{
			TransactionHash: oldTxHash,
		},
	})
	assert.Equal(t, []*DBPubTxnSubmission{
		{TransactionHash: oldTxHash},
	}, inMemoryTx.mtx.ptx.Submissions)

	// mark the transaction complete
	confirmReceived := InFlightStatusConfirmReceived
	newTime := confutil.P(pldtypes.TimestampNow())
	newTxHash := pldtypes.Bytes32Keccak([]byte("0x000031"))
	newGasPrice := pldtypes.Uint64ToUint256(111)
	newErrorMessage := "new message"

	imts.ApplyInMemoryUpdates(context.Background(), &BaseTXUpdates{
		InFlightStatus:  &confirmReceived,
		GasPricing:      &pldapi.PublicTxGasPricing{GasPrice: newGasPrice},
		TransactionHash: &newTxHash,
		NewSubmission: &DBPubTxnSubmission{
			TransactionHash: newTxHash,
		},
		FirstSubmit:  newTime,
		LastSubmit:   newTime,
		ErrorMessage: &newErrorMessage,
	})

	assert.Equal(t, InFlightStatusConfirmReceived, imts.GetInFlightStatus())
	assert.Equal(t, oldTime, *imts.GetCreatedTime())
	assert.Equal(t, newTime, imts.GetLastSubmitTime())
	assert.Equal(t, newTxHash, *imts.GetTransactionHash())
	assert.Equal(t, newGasPrice.Int(), imts.GetGasPriceObject().GasPrice.Int())
	assert.Nil(t, imts.GetGasPriceObject().MaxFeePerGas)
	assert.Nil(t, imts.GetGasPriceObject().MaxPriorityFeePerGas)
	assert.Equal(t, newTime, imts.GetFirstSubmit())
	assert.Equal(t, &DBPubTxnSubmission{
		TransactionHash: newTxHash,
	}, imts.GetUnflushedSubmission())
	assert.True(t, imts.IsReadyToExit())

	// check immutable fields
	assert.Equal(t, oldNonce.Uint64(), imts.GetNonce())
	assert.Equal(t, *oldFrom, imts.GetFrom())
	assert.Equal(t, oldValue, inMemoryTx.mtx.ptx.Value)
	assert.Equal(t, oldTransactionData, inMemoryTx.mtx.ptx.Data)

	maxPriorityFeePerGas := pldtypes.Uint64ToUint256(2)
	maxFeePerGas := pldtypes.Uint64ToUint256(123)

	// test switch gas price format
	imts.ApplyInMemoryUpdates(context.Background(), &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			MaxPriorityFeePerGas: maxPriorityFeePerGas,
			MaxFeePerGas:         maxFeePerGas,
		},
	})

	assert.Nil(t, imts.GetGasPriceObject().GasPrice)
	assert.Equal(t, maxFeePerGas.Int(), imts.GetGasPriceObject().MaxFeePerGas.Int())
	assert.Equal(t, maxPriorityFeePerGas.Int(), imts.GetGasPriceObject().MaxPriorityFeePerGas.Int())

}
