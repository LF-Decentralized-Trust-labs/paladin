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
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/ethclientmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/ethclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func NewTestGasPriceClient(t *testing.T, conf *pldconf.GasPriceConfig, zeroGasPrice bool) (context.Context, *HybridGasPriceClient, *ethclientmocks.EthClient) {
	ctx := context.Background()
	mockEthClient := ethclientmocks.NewEthClient(t)

	hgc := &HybridGasPriceClient{
		conf: conf,
	}
	hgc.Init(ctx)

	if hgc.fixedGasPrice == nil {
		// Add GasPrice mock since Start method calls it
		gasPrice := pldtypes.Uint64ToUint256(20000000000)
		if zeroGasPrice {
			gasPrice = pldtypes.Uint64ToUint256(0)
		}
		mockEthClient.On("GasPrice", ctx).Return(gasPrice, nil).Once()
	}

	hgc.Start(ctx, mockEthClient)
	return ctx, hgc, mockEthClient
}

func NewFixedPriceGasPriceClient(t *testing.T, maxFeePerGas, maxPriorityFeePerGas uint64) (context.Context, *HybridGasPriceClient, *ethclientmocks.EthClient) {
	return NewTestGasPriceClient(t, &pldconf.GasPriceConfig{
		FixedGasPrice: &pldconf.FixedGasPricing{
			MaxFeePerGas:         pldtypes.Uint64ToUint256(maxFeePerGas),
			MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(maxPriorityFeePerGas),
		},
	}, false)
}

func TestNewGasPriceClientFixedPricing(t *testing.T) {
	ctx, hgc, _ := NewFixedPriceGasPriceClient(t, 10, 1)
	gpo, err := hgc.GetGasPriceObject(ctx)
	require.NoError(t, err)
	assert.NotNil(t, gpo)
	assert.Equal(t, int64(10), gpo.MaxFeePerGas.Int().Int64())
	assert.Equal(t, int64(1), gpo.MaxPriorityFeePerGas.Int().Int64())
}

func TestHasZeroGasPrice(t *testing.T) {
	// Test with zero gas price retrieved from chain
	ctx, hgc, _ := NewTestGasPriceClient(t, &pldconf.GasPriceConfig{}, true)
	assert.True(t, hgc.HasZeroGasPrice(ctx))

	// Test with zero gas price set in config
	ctx, hgc, _ = NewFixedPriceGasPriceClient(t, 0, 0)
	assert.True(t, hgc.HasZeroGasPrice(ctx))

	// Test with non-zero gas price in config
	ctx, hgc, _ = NewFixedPriceGasPriceClient(t, 10, 1)
	assert.False(t, hgc.HasZeroGasPrice(ctx))

	// Test with non-zero gas price from chain
	ctx, hgc, _ = NewTestGasPriceClient(t, &pldconf.GasPriceConfig{}, false)
	assert.False(t, hgc.HasZeroGasPrice(ctx))
}

// Test helpers for dynamic gas pricing
func createMockFeeHistoryResult(blockCount int, baseFeeWei uint64, tipWei uint64) *ethclient.FeeHistoryResult {
	baseFees := make([]pldtypes.HexUint256, blockCount)
	rewards := make([][]pldtypes.HexUint256, blockCount)

	for i := 0; i < blockCount; i++ {
		baseFees[i] = *pldtypes.Uint64ToUint256(baseFeeWei)
		rewards[i] = []pldtypes.HexUint256{*pldtypes.Uint64ToUint256(tipWei)}
	}

	return &ethclient.FeeHistoryResult{
		OldestBlock:   pldtypes.HexUint64(100),
		BaseFeePerGas: baseFees,
		GasUsedRatio:  make([]float64, blockCount),
		Reward:        rewards,
	}
}

func TestDynamicGasPricingBasic(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		DynamicGasPricing: pldconf.DynamicGasPricingConfig{
			Percentile:          confutil.P(85),
			HistoryBlockCount:   confutil.P(20),
			BaseFeeBufferFactor: confutil.P(2),
			Cache: pldconf.DynamicGasPricingCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response
	mockFeeHistoryResult := createMockFeeHistoryResult(20, 20000000000, 1500000000) // 20 Gwei base fee, 1.5 Gwei tip
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Once()

	gpo, err := hgc.GetGasPriceObject(ctx)
	require.NoError(t, err)
	assert.NotNil(t, gpo)

	// Verify the calculation: maxFeePerGas = (2 * baseFee) + maxPriorityFeePerGas
	expectedMaxFeePerGas := int64(2*20000000000 + 1500000000) // (2 * 20 Gwei) + 1.5 Gwei
	expectedMaxPriorityFeePerGas := int64(1500000000)         // 1.5 Gwei

	assert.Equal(t, expectedMaxFeePerGas, gpo.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFeePerGas, gpo.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestDynamicGasPricingWithCustomConfig(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		DynamicGasPricing: pldconf.DynamicGasPricingConfig{
			Percentile:          confutil.P(75), // Custom percentile
			HistoryBlockCount:   confutil.P(10), // Custom block count
			BaseFeeBufferFactor: confutil.P(3),  // Custom buffer factor
			Cache: pldconf.DynamicGasPricingCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response
	mockFeeHistoryResult := createMockFeeHistoryResult(10, 30000000000, 2000000000) // 30 Gwei base fee, 2 Gwei tip
	mockEthClient.On("FeeHistory", ctx, 10, "latest", []float64{75.0}).Return(mockFeeHistoryResult, nil).Once()

	gpo, err := hgc.GetGasPriceObject(ctx)
	require.NoError(t, err)
	assert.NotNil(t, gpo)

	// Verify the calculation: maxFeePerGas = (3 * baseFee) + maxPriorityFeePerGas
	expectedMaxFeePerGas := int64(3*30000000000 + 2000000000) // (3 * 30 Gwei) + 2 Gwei
	expectedMaxPriorityFeePerGas := int64(2000000000)         // 2 Gwei

	assert.Equal(t, expectedMaxFeePerGas, gpo.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFeePerGas, gpo.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestDynamicGasPricingWithPriorityFeeCap(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		DynamicGasPricing: pldconf.DynamicGasPricingConfig{
			Percentile:        confutil.P(85),
			HistoryBlockCount: confutil.P(20),
			MaxPriorityFeeCap: pldtypes.Uint64ToUint256(1000000000), // 1 Gwei cap
			Cache: pldconf.DynamicGasPricingCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response with high tip that should be capped
	mockFeeHistoryResult := createMockFeeHistoryResult(20, 20000000000, 3000000000) // 20 Gwei base fee, 3 Gwei tip (above 1 Gwei cap)
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Once()

	gpo, err := hgc.GetGasPriceObject(ctx)
	require.NoError(t, err)
	assert.NotNil(t, gpo)

	// Verify the tip is capped to 1 Gwei
	expectedMaxFeePerGas := int64(1*20000000000 + 1000000000) // (1 * 20 Gwei) + 1 Gwei (capped)
	expectedMaxPriorityFeePerGas := int64(1000000000)         // 1 Gwei (capped)

	assert.Equal(t, expectedMaxFeePerGas, gpo.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFeePerGas, gpo.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestDynamicGasPricingFallbackTo1Gwei(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		DynamicGasPricing: pldconf.DynamicGasPricingConfig{
			Percentile:        confutil.P(85),
			HistoryBlockCount: confutil.P(20),
			Cache: pldconf.DynamicGasPricingCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response with no valid tips (all zero)
	mockFeeHistoryResult := &ethclient.FeeHistoryResult{
		OldestBlock:   pldtypes.HexUint64(100),
		BaseFeePerGas: []pldtypes.HexUint256{*pldtypes.Uint64ToUint256(20000000000)},
		GasUsedRatio:  []float64{0.5},
		Reward:        [][]pldtypes.HexUint256{{*pldtypes.Uint64ToUint256(0)}}, // Zero tip
	}
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Once()

	gpo, err := hgc.GetGasPriceObject(ctx)
	require.NoError(t, err)
	assert.NotNil(t, gpo)

	// Verify fallback to 1 Gwei
	expectedMaxFeePerGas := int64(1*20000000000 + 1000000000) // (1 * 20 Gwei) + 1 Gwei (fallback)
	expectedMaxPriorityFeePerGas := int64(1000000000)         // 1 Gwei (fallback)

	assert.Equal(t, expectedMaxFeePerGas, gpo.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFeePerGas, gpo.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestDynamicGasPricingFallbackWithCap(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		DynamicGasPricing: pldconf.DynamicGasPricingConfig{
			Percentile:        confutil.P(85),
			HistoryBlockCount: confutil.P(20),
			MaxPriorityFeeCap: pldtypes.Uint64ToUint256(500000000), // 0.5 Gwei cap
			Cache: pldconf.DynamicGasPricingCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response with no valid tips
	mockFeeHistoryResult := &ethclient.FeeHistoryResult{
		OldestBlock:   pldtypes.HexUint64(100),
		BaseFeePerGas: []pldtypes.HexUint256{*pldtypes.Uint64ToUint256(20000000000)},
		GasUsedRatio:  []float64{0.5},
		Reward:        [][]pldtypes.HexUint256{{*pldtypes.Uint64ToUint256(0)}}, // Zero tip
	}
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Once()

	gpo, err := hgc.GetGasPriceObject(ctx)
	require.NoError(t, err)
	assert.NotNil(t, gpo)

	// Verify fallback 1 Gwei is capped to 0.5 Gwei
	expectedMaxFeePerGas := int64(1*20000000000 + 500000000) // (1 * 20 Gwei) + 0.5 Gwei (capped fallback)
	expectedMaxPriorityFeePerGas := int64(500000000)         // 0.5 Gwei (capped fallback)

	assert.Equal(t, expectedMaxFeePerGas, gpo.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFeePerGas, gpo.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestDynamicGasPricingCaching(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		DynamicGasPricing: pldconf.DynamicGasPricingConfig{
			Percentile:        confutil.P(85),
			HistoryBlockCount: confutil.P(20),
			Cache: pldconf.DynamicGasPricingCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response
	mockFeeHistoryResult := createMockFeeHistoryResult(20, 20000000000, 1500000000)
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Once()

	// First call should hit the RPC
	gpo1, err := hgc.GetGasPriceObject(ctx)
	require.NoError(t, err)
	assert.NotNil(t, gpo1)

	// Second call should use cache (no additional RPC calls)
	gpo2, err := hgc.GetGasPriceObject(ctx)
	require.NoError(t, err)
	assert.NotNil(t, gpo2)

	// Results should be identical
	assert.Equal(t, gpo1.MaxFeePerGas.Int().Int64(), gpo2.MaxFeePerGas.Int().Int64())
	assert.Equal(t, gpo1.MaxPriorityFeePerGas.Int().Int64(), gpo2.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestDynamicGasPricingCacheDisabled(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		DynamicGasPricing: pldconf.DynamicGasPricingConfig{
			Percentile:        confutil.P(85),
			HistoryBlockCount: confutil.P(20),
			Cache: pldconf.DynamicGasPricingCacheConfig{
				Enabled: confutil.P(false),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response - should be called twice since caching is disabled
	mockFeeHistoryResult := createMockFeeHistoryResult(20, 20000000000, 1500000000)
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Twice()

	// First call
	gpo1, err := hgc.GetGasPriceObject(ctx)
	require.NoError(t, err)
	assert.NotNil(t, gpo1)

	// Second call should hit RPC again
	gpo2, err := hgc.GetGasPriceObject(ctx)
	require.NoError(t, err)
	assert.NotNil(t, gpo2)

	mockEthClient.AssertExpectations(t)
}

func TestDynamicGasPricingEmptyFeeHistory(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		DynamicGasPricing: pldconf.DynamicGasPricingConfig{
			Percentile:        confutil.P(85),
			HistoryBlockCount: confutil.P(20),
			Cache: pldconf.DynamicGasPricingCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock empty fee history response
	mockFeeHistoryResult := &ethclient.FeeHistoryResult{
		OldestBlock:   pldtypes.HexUint64(100),
		BaseFeePerGas: []pldtypes.HexUint256{}, // Empty
		GasUsedRatio:  []float64{},
		Reward:        [][]pldtypes.HexUint256{},
	}
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Once()

	gpo, err := hgc.GetGasPriceObject(ctx)
	assert.Error(t, err)
	assert.Nil(t, gpo)
	assert.Contains(t, err.Error(), "fee history returned empty data")

	mockEthClient.AssertExpectations(t)
}

func TestDynamicGasPricingRPCError(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		DynamicGasPricing: pldconf.DynamicGasPricingConfig{
			Percentile:        confutil.P(85),
			HistoryBlockCount: confutil.P(20),
			Cache: pldconf.DynamicGasPricingCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock RPC error
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(nil, fmt.Errorf("RPC error")).Once()

	gpo, err := hgc.GetGasPriceObject(ctx)
	assert.Error(t, err)
	assert.Nil(t, gpo)
	assert.Contains(t, err.Error(), "RPC error")

	mockEthClient.AssertExpectations(t)
}

func TestDeleteCache(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		DynamicGasPricing: pldconf.DynamicGasPricingConfig{
			Percentile:        confutil.P(85),
			HistoryBlockCount: confutil.P(20),
			Cache: pldconf.DynamicGasPricingCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response
	mockFeeHistoryResult := createMockFeeHistoryResult(20, 20000000000, 1500000000)
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Twice()

	// First call to populate cache
	gpo1, err := hgc.GetGasPriceObject(ctx)
	require.NoError(t, err)
	assert.NotNil(t, gpo1)

	// Delete cache
	hgc.DeleteCache(ctx)

	// Second call should hit RPC again since cache was cleared
	gpo2, err := hgc.GetGasPriceObject(ctx)
	require.NoError(t, err)
	assert.NotNil(t, gpo2)

	mockEthClient.AssertExpectations(t)
}

func TestInitValidation(t *testing.T) {
	ctx := context.Background()
	// Test with invalid percentile (should log error but not panic)
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		DynamicGasPricing: pldconf.DynamicGasPricingConfig{
			Percentile:        confutil.P(150), // Invalid: > 100
			HistoryBlockCount: confutil.P(20),
			Cache: pldconf.DynamicGasPricingCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	assert.Error(t, err)

	conf = &pldconf.GasPriceConfig{
		FixedGasPrice: &pldconf.FixedGasPricing{
			MaxFeePerGas: pldtypes.Uint64ToUint256(1000),
			// Missing MaxPriorityFeePerGas
		},
	}

	gasPriceClient = NewGasPriceClient(ctx, conf)
	err = gasPriceClient.Init(ctx)
	assert.Error(t, err)
}

func TestInitWithNilDynamicGasPricing(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice:     nil,
		DynamicGasPricing: pldconf.DynamicGasPricingConfig{}, // All fields nil
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.NoError(t, err)

	hgc := gasPriceClient.(*HybridGasPriceClient)
	// Should use defaults when config fields are nil
	assert.Equal(t, 85, hgc.percentile)
	assert.Equal(t, 20, hgc.historyBlockCount)
	assert.Equal(t, 1, hgc.baseFeeBufferFactor)
	assert.True(t, hgc.cacheEnabled)
}

func TestInitWithPartialConfig(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		DynamicGasPricing: pldconf.DynamicGasPricingConfig{
			Percentile:          confutil.P(75), // Only percentile set
			HistoryBlockCount:   nil,            // Missing
			BaseFeeBufferFactor: nil,            // Missing
			Cache: pldconf.DynamicGasPricingCacheConfig{
				Enabled: nil, // Missing
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.NoError(t, err)

	hgc := gasPriceClient.(*HybridGasPriceClient)
	// Should use defaults for missing fields
	assert.Equal(t, 75, hgc.percentile)
	assert.Equal(t, 20, hgc.historyBlockCount)
	assert.Equal(t, 1, hgc.baseFeeBufferFactor)
	assert.True(t, hgc.cacheEnabled)
}

// mapConfigToAPIGasPricing edge cases
func TestMapConfigToAPIGasPricingIncompleteConfig(t *testing.T) {
	ctx := context.Background()

	// Test with only MaxFeePerGas set
	conf := &pldconf.FixedGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(1000),
		MaxPriorityFeePerGas: nil,
	}

	result, err := mapConfigToAPIGasPricing(ctx, conf)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "maxPriorityFeePerGas is missing")

	// Test with only MaxPriorityFeePerGas set
	conf = &pldconf.FixedGasPricing{
		MaxFeePerGas:         nil,
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(100),
	}

	result, err = mapConfigToAPIGasPricing(ctx, conf)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "maxFeePerGas is missing")

	// Test with both fields nil - this should return a valid object with nil fields
	conf = &pldconf.FixedGasPricing{
		MaxFeePerGas:         nil,
		MaxPriorityFeePerGas: nil,
	}

	result, err = mapConfigToAPIGasPricing(ctx, conf)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Nil(t, result.MaxFeePerGas)
	assert.Nil(t, result.MaxPriorityFeePerGas)
}

func TestStartWithNilGasPriceResponse(t *testing.T) {
	ctx := context.Background()
	mockEthClient := ethclientmocks.NewEthClient(t)

	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		DynamicGasPricing: pldconf.DynamicGasPricingConfig{
			Percentile:        confutil.P(85),
			HistoryBlockCount: confutil.P(20),
			Cache: pldconf.DynamicGasPricingCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	hgc := gasPriceClient.(*HybridGasPriceClient)
	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Mock GasPrice returning nil
	mockEthClient.On("GasPrice", ctx).Return(nil, nil).Once()

	hgc.Start(ctx, mockEthClient)
	assert.False(t, hgc.hasZeroGasPrice) // Should not be set when gasPrice is nil

	mockEthClient.AssertExpectations(t)
}

func TestStartWithGasPriceError(t *testing.T) {
	ctx := context.Background()
	mockEthClient := ethclientmocks.NewEthClient(t)

	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		DynamicGasPricing: pldconf.DynamicGasPricingConfig{
			Percentile:        confutil.P(85),
			HistoryBlockCount: confutil.P(20),
			Cache: pldconf.DynamicGasPricingCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	hgc := gasPriceClient.(*HybridGasPriceClient)
	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Mock GasPrice returning error
	mockEthClient.On("GasPrice", ctx).Return(nil, fmt.Errorf("network error")).Once()

	hgc.Start(ctx, mockEthClient)
	assert.False(t, hgc.hasZeroGasPrice) // Should not be set when GasPrice fails

	mockEthClient.AssertExpectations(t)
}

func TestStartSkipsGasPriceWhenFixedPriceSet(t *testing.T) {
	ctx := context.Background()
	mockEthClient := ethclientmocks.NewEthClient(t)

	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: &pldconf.FixedGasPricing{
			MaxFeePerGas:         pldtypes.Uint64ToUint256(1000),
			MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(100),
		},
		DynamicGasPricing: pldconf.DynamicGasPricingConfig{
			Percentile:        confutil.P(85),
			HistoryBlockCount: confutil.P(20),
			Cache: pldconf.DynamicGasPricingCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	hgc := gasPriceClient.(*HybridGasPriceClient)
	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Should not call GasPrice since fixedGasPrice is set
	hgc.Start(ctx, mockEthClient)

	// No expectations to assert since GasPrice should not be called
}
