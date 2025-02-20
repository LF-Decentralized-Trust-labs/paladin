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

	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
)

const testTransactionData string = "0x7369676e6564206d657373616765"

func NewTestInMemoryTxState(t *testing.T) InMemoryTxStateManager {
	oldTime := tktypes.TimestampNow()
	oldFrom := tktypes.MustEthAddress("0x4e598f6e918321dd47c86e7a077b4ab0e7414846")
	oldTxHash := tktypes.RandBytes32()
	oldTo := tktypes.MustEthAddress("0x6cee73cf4d5b0ac66ce2d1c0617bec4bedd09f39")
	oldNonce := tktypes.HexUint64(1)
	oldGasLimit := tktypes.HexUint64(2000)
	oldValue := tktypes.Uint64ToUint256(200)
	oldGasPrice := tktypes.Uint64ToUint256(10)
	oldErrorMessage := "old message"
	oldTransactionData := tktypes.MustParseHexBytes(testTransactionData)
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
	oldTime := tktypes.TimestampNow()
	oldFrom := tktypes.MustEthAddress("0xb3d9cf8e163bbc840195a97e81f8a34e295b8f39")
	oldTxHash := tktypes.Bytes32Keccak([]byte("0x00000"))
	oldTo := tktypes.MustEthAddress("0x1f9090aae28b8a3dceadf281b0f12828e676c326")
	oldNonce := tktypes.HexUint64(1)
	oldGasLimit := tktypes.HexUint64(2000)
	oldValue := tktypes.Uint64ToUint256(200)
	oldGasPrice := tktypes.Uint64ToUint256(10)
	oldErrorMessage := "old message"
	oldTransactionData := tktypes.MustParseHexBytes(testTransactionData)

	testManagedTx := &DBPublicTxn{
		Created: oldTime,
		From:    *oldFrom,
		To:      oldTo,
		Nonce:   (*uint64)(&oldNonce),
		Gas:     uint64(oldGasLimit),
		Value:   oldValue,
		Data:    tktypes.HexBytes(oldTransactionData),
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
	newTime := confutil.P(tktypes.TimestampNow())
	newTxHash := tktypes.Bytes32Keccak([]byte("0x000031"))
	newGasPrice := tktypes.Uint64ToUint256(111)
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

	maxPriorityFeePerGas := tktypes.Uint64ToUint256(2)
	maxFeePerGas := tktypes.Uint64ToUint256(123)

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

func TestUpdateTransaction(t *testing.T) {
	oldTo := tktypes.MustEthAddress("0xb3d9cf8e163bbc840195a97e81f8a34e295b8f39")
	oldGas := uint64(1000)
	oldValue := tktypes.Uint64ToUint256(100)
	oldData := tktypes.MustParseHexBytes("0x1234")
	oldFixedGasPricing := tktypes.JSONString(pldapi.PublicTxGasPricing{
		GasPrice:             tktypes.Uint64ToUint256(100),
		MaxPriorityFeePerGas: tktypes.Uint64ToUint256(100),
		MaxFeePerGas:         tktypes.Uint64ToUint256(100),
	})

	ptx := &DBPublicTxn{
		To:              oldTo,
		Gas:             oldGas,
		Value:           oldValue,
		Data:            oldData,
		FixedGasPricing: oldFixedGasPricing,
	}
	imtxs := inMemoryTxState{
		mtx: &managedTx{
			ptx: ptx,
		},
	}

	// no changes- only default values
	imtxs.UpdateTransaction(&DBPublicTxn{})

	assert.Equal(t, oldTo, ptx.To)
	assert.Equal(t, oldGas, ptx.Gas)
	assert.Equal(t, oldValue, ptx.Value)
	assert.Equal(t, oldData, ptx.Data)
	assert.Equal(t, oldFixedGasPricing, ptx.FixedGasPricing)

	newTo := tktypes.MustEthAddress("0x1f9090aae28b8a3dceadf281b0f12828e676c326")
	newGas := uint64(2000)
	newValue := tktypes.Uint64ToUint256(200)
	newData := tktypes.MustParseHexBytes("0x5678")
	newFixedGasPricing := tktypes.JSONString(pldapi.PublicTxGasPricing{
		GasPrice:             tktypes.Uint64ToUint256(200),
		MaxPriorityFeePerGas: tktypes.Uint64ToUint256(200),
		MaxFeePerGas:         tktypes.Uint64ToUint256(200),
	})

	// all new values
	imtxs.UpdateTransaction(&DBPublicTxn{
		To:              newTo,
		Gas:             newGas,
		Value:           newValue,
		Data:            newData,
		FixedGasPricing: newFixedGasPricing,
	})

	assert.Equal(t, newTo, ptx.To)
	assert.Equal(t, newGas, ptx.Gas)
	assert.Equal(t, newValue, ptx.Value)
	assert.Equal(t, newData, ptx.Data)
	assert.Equal(t, newFixedGasPricing, ptx.FixedGasPricing)
}

func TestIsUpdate(t *testing.T) {
	ptx := &DBPublicTxn{
		To:  tktypes.MustEthAddress("0xb3d9cf8e163bbc840195a97e81f8a34e295b8f39"),
		Gas: 1000,
	}
	imtxs := inMemoryTxState{
		mtx: &managedTx{
			ptx: ptx,
		},
	}

	// no changes
	assert.False(t, imtxs.IsTransactionUpdate(&DBPublicTxn{}))

	// new to
	assert.True(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		To: tktypes.MustEthAddress("0x1f9090aae28b8a3dceadf281b0f12828e676c326"),
	}))
	// same to
	assert.False(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		To: tktypes.MustEthAddress("0xb3d9cf8e163bbc840195a97e81f8a34e295b8f39"),
	}))

	// new gas
	assert.True(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		Gas: 2000,
	}))
	// same gas
	assert.False(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		Gas: 1000,
	}))

	// value not previously set
	assert.True(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		Value: tktypes.Uint64ToUint256(100),
	}))
	ptx.Value = tktypes.Uint64ToUint256(100)
	// new value
	assert.True(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		Value: tktypes.Uint64ToUint256(200),
	}))
	// same value
	assert.False(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		Value: tktypes.Uint64ToUint256(100),
	}))

	// data not previously set
	assert.True(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		Data: tktypes.MustParseHexBytes("0x1234"),
	}))
	ptx.Data = tktypes.MustParseHexBytes("0x1234")
	// new data
	assert.True(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		Data: tktypes.MustParseHexBytes("0x123456"),
	}))
	// same data
	assert.False(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		Data: tktypes.MustParseHexBytes("0x1234"),
	}))

	// fixed gas pricing not previously set
	assert.True(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		FixedGasPricing: tktypes.JSONString(pldapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(100),
		}),
	}))
	imtxs.mtx.ptx.Data = tktypes.MustParseHexBytes("0x1234")

	// gas price not previously set
	ptx.FixedGasPricing = tktypes.JSONString(pldapi.PublicTxGasPricing{})
	assert.True(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		FixedGasPricing: tktypes.JSONString(pldapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(100),
		}),
	}))
	// new gas price
	ptx.FixedGasPricing = tktypes.JSONString(pldapi.PublicTxGasPricing{
		GasPrice: tktypes.Uint64ToUint256(100),
	})
	assert.True(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		FixedGasPricing: tktypes.JSONString(pldapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(200),
		}),
	}))
	// same gas price
	assert.False(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		FixedGasPricing: tktypes.JSONString(pldapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(100),
		}),
	}))

	// max priority gas fee not previously set
	ptx.FixedGasPricing = tktypes.JSONString(pldapi.PublicTxGasPricing{})
	assert.True(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		FixedGasPricing: tktypes.JSONString(pldapi.PublicTxGasPricing{
			MaxPriorityFeePerGas: tktypes.Uint64ToUint256(100),
		}),
	}))
	// new max priority gas fee
	ptx.FixedGasPricing = tktypes.JSONString(pldapi.PublicTxGasPricing{
		MaxPriorityFeePerGas: tktypes.Uint64ToUint256(100),
	})
	assert.True(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		FixedGasPricing: tktypes.JSONString(pldapi.PublicTxGasPricing{
			MaxPriorityFeePerGas: tktypes.Uint64ToUint256(200),
		}),
	}))
	// same max priority gas fee
	assert.False(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		FixedGasPricing: tktypes.JSONString(pldapi.PublicTxGasPricing{
			MaxPriorityFeePerGas: tktypes.Uint64ToUint256(100),
		}),
	}))

	// max fee per gas not previously set
	ptx.FixedGasPricing = tktypes.JSONString(pldapi.PublicTxGasPricing{})
	assert.True(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		FixedGasPricing: tktypes.JSONString(pldapi.PublicTxGasPricing{
			MaxFeePerGas: tktypes.Uint64ToUint256(100),
		}),
	}))
	// new  max fee per gas
	ptx.FixedGasPricing = tktypes.JSONString(pldapi.PublicTxGasPricing{
		MaxFeePerGas: tktypes.Uint64ToUint256(100),
	})
	assert.True(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		FixedGasPricing: tktypes.JSONString(pldapi.PublicTxGasPricing{
			MaxFeePerGas: tktypes.Uint64ToUint256(200),
		}),
	}))
	// same  max fee per gas
	assert.False(t, imtxs.IsTransactionUpdate(&DBPublicTxn{
		FixedGasPricing: tktypes.JSONString(pldapi.PublicTxGasPricing{
			MaxFeePerGas: tktypes.Uint64ToUint256(100),
		}),
	}))

}
