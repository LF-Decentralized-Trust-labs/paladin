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
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
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

func NewTestFixedPriceGasPriceClient(t *testing.T, maxFeePerGas, maxPriorityFeePerGas uint64) (context.Context, *HybridGasPriceClient, *ethclientmocks.EthClient) {
	maxFeePerGasStr := pldtypes.Uint64ToUint256(maxFeePerGas).HexString0xPrefix()
	maxPriorityFeePerGasStr := pldtypes.Uint64ToUint256(maxPriorityFeePerGas).HexString0xPrefix()
	return NewTestGasPriceClient(t, &pldconf.GasPriceConfig{
		FixedGasPrice: &pldconf.FixedGasPricing{
			MaxFeePerGas:         &maxFeePerGasStr,
			MaxPriorityFeePerGas: &maxPriorityFeePerGasStr,
		},
	}, false)
}

func TestNewGasPriceClientFixedPricing(t *testing.T) {
	ctx, hgc, _ := NewTestFixedPriceGasPriceClient(t, 10, 1)
	gpo, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
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
	ctx, hgc, _ = NewTestFixedPriceGasPriceClient(t, 0, 0)
	assert.True(t, hgc.HasZeroGasPrice(ctx))

	// Test with non-zero gas price in config
	ctx, hgc, _ = NewTestFixedPriceGasPriceClient(t, 10, 1)
	assert.False(t, hgc.HasZeroGasPrice(ctx))

	// Test with non-zero gas price from chain
	ctx, hgc, _ = NewTestGasPriceClient(t, &pldconf.GasPriceConfig{}, false)
	assert.False(t, hgc.HasZeroGasPrice(ctx))
}

// Test helpers for eth_feeHistory gas pricing
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

func TestEthFeeHistoryGasPricingBasic(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			BaseFeeBufferFactor:   confutil.P(2),
			Cache: pldconf.EthFeeHistoryCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response
	mockFeeHistoryResult := createMockFeeHistoryResult(20, 20000000000, 1500000000) // 20 Gwei base fee, 1.5 Gwei tip
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Once()

	gpo, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	assert.NotNil(t, gpo)

	// Verify the calculation: maxFeePerGas = (2 * baseFee) + maxPriorityFeePerGas
	expectedMaxFeePerGas := int64(2*20000000000 + 1500000000) // (2 * 20 Gwei) + 1.5 Gwei
	expectedMaxPriorityFeePerGas := int64(1500000000)         // 1.5 Gwei

	assert.Equal(t, expectedMaxFeePerGas, gpo.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFeePerGas, gpo.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestEthFeeHistoryGasPricingWithCustomConfig(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(75), // Custom percentile
			HistoryBlockCount:     confutil.P(10), // Custom block count
			BaseFeeBufferFactor:   confutil.P(3),  // Custom buffer factor
			Cache: pldconf.EthFeeHistoryCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response
	mockFeeHistoryResult := createMockFeeHistoryResult(10, 30000000000, 2000000000) // 30 Gwei base fee, 2 Gwei tip
	mockEthClient.On("FeeHistory", ctx, 10, "latest", []float64{75.0}).Return(mockFeeHistoryResult, nil).Once()

	gpo, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	assert.NotNil(t, gpo)

	// Verify the calculation: maxFeePerGas = (3 * baseFee) + maxPriorityFeePerGas
	expectedMaxFeePerGas := int64(3*30000000000 + 2000000000) // (3 * 30 Gwei) + 2 Gwei
	expectedMaxPriorityFeePerGas := int64(2000000000)         // 2 Gwei

	assert.Equal(t, expectedMaxFeePerGas, gpo.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFeePerGas, gpo.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestEthFeeHistoryGasPricingWithPriorityFeeCap(t *testing.T) {
	maxPriorityFeePerGasCapStr := pldtypes.Uint64ToUint256(1000000000).HexString0xPrefix()
	maxFeePerGasCapStr := pldtypes.Uint64ToUint256(20000000000).HexString0xPrefix()
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice:           nil,
		MaxPriorityFeePerGasCap: &maxPriorityFeePerGasCapStr,
		MaxFeePerGasCap:         &maxFeePerGasCapStr,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			Cache: pldconf.EthFeeHistoryCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response with high tip that should be capped
	mockFeeHistoryResult := createMockFeeHistoryResult(20, 20000000000, 3000000000) // 20 Gwei base fee, 3 Gwei tip (above 1 Gwei cap)
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Once()

	gpo, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	assert.NotNil(t, gpo)

	// Verify the tip is capped to 1 Gwei
	expectedMaxFeePerGas := int64(20000000000)        // capped at 20 Gwei  - would have been 21 Gwei without cap(1 * 20 Gwei) + 1 Gwei
	expectedMaxPriorityFeePerGas := int64(1000000000) // 1 Gwei (capped)

	assert.Equal(t, expectedMaxFeePerGas, gpo.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFeePerGas, gpo.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestEthFeeHistoryGasPricingFallbackTo1Gwei(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			Cache: pldconf.EthFeeHistoryCacheConfig{
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

	gpo, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	assert.NotNil(t, gpo)

	// Verify fallback to 1 Gwei
	expectedMaxFeePerGas := int64(1*20000000000 + 1000000000) // (1 * 20 Gwei) + 1 Gwei (fallback)
	expectedMaxPriorityFeePerGas := int64(1000000000)         // 1 Gwei (fallback)

	assert.Equal(t, expectedMaxFeePerGas, gpo.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFeePerGas, gpo.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestEthFeeHistoryGasPricingFallbackWithCap(t *testing.T) {
	maxPriorityFeePerGasCapStr := pldtypes.Uint64ToUint256(500000000).HexString0xPrefix()
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice:           nil,
		MaxPriorityFeePerGasCap: &maxPriorityFeePerGasCapStr,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			Cache: pldconf.EthFeeHistoryCacheConfig{
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

	gpo, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	assert.NotNil(t, gpo)

	// Verify fallback 1 Gwei is capped to 0.5 Gwei
	expectedMaxFeePerGas := int64(1*20000000000 + 500000000) // (1 * 20 Gwei) + 0.5 Gwei (capped fallback)
	expectedMaxPriorityFeePerGas := int64(500000000)         // 0.5 Gwei (capped fallback)

	assert.Equal(t, expectedMaxFeePerGas, gpo.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFeePerGas, gpo.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestEthFeeHistoryGasPricingCaching(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			Cache: pldconf.EthFeeHistoryCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response
	mockFeeHistoryResult := createMockFeeHistoryResult(20, 20000000000, 1500000000)
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Once()

	// First call should hit the RPC
	gpo1, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	assert.NotNil(t, gpo1)

	// Second call should use cache (no additional RPC calls)
	gpo2, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	assert.NotNil(t, gpo2)

	// Results should be identical
	assert.Equal(t, gpo1.MaxFeePerGas.Int().Int64(), gpo2.MaxFeePerGas.Int().Int64())
	assert.Equal(t, gpo1.MaxPriorityFeePerGas.Int().Int64(), gpo2.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestEthFeeHistoryGasPricingCacheDisabled(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			Cache: pldconf.EthFeeHistoryCacheConfig{
				Enabled: confutil.P(false),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response - should be called twice since caching is disabled
	mockFeeHistoryResult := createMockFeeHistoryResult(20, 20000000000, 1500000000)
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Twice()

	// First call
	gpo1, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	assert.NotNil(t, gpo1)

	// Second call should hit RPC again
	gpo2, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	assert.NotNil(t, gpo2)

	mockEthClient.AssertExpectations(t)
}

func TestEthFeeHistoryGasPricingEmptyFeeHistory(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			Cache: pldconf.EthFeeHistoryCacheConfig{
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

	gpo, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	assert.Error(t, err)
	assert.Nil(t, gpo)
	assert.Contains(t, err.Error(), "fee history returned empty data")

	mockEthClient.AssertExpectations(t)
}

func TestEthFeeHistoryGasPricingRPCError(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			Cache: pldconf.EthFeeHistoryCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock RPC error
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(nil, fmt.Errorf("RPC error")).Once()

	gpo, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	assert.Error(t, err)
	assert.Nil(t, gpo)
	assert.Contains(t, err.Error(), "RPC error")

	mockEthClient.AssertExpectations(t)
}

func TestDeleteCache(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			Cache: pldconf.EthFeeHistoryCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response
	mockFeeHistoryResult := createMockFeeHistoryResult(20, 20000000000, 1500000000)
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Twice()

	// First call to populate cache
	gpo1, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	assert.NotNil(t, gpo1)

	// Delete cache
	hgc.DeleteCache(ctx)

	// Second call should hit RPC again since cache was cleared
	gpo2, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	assert.NotNil(t, gpo2)

	mockEthClient.AssertExpectations(t)
}

func TestInitValidation(t *testing.T) {
	ctx := context.Background()
	// Test with invalid percentile (should log error but not panic)
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(150), // Invalid: > 100
			HistoryBlockCount:     confutil.P(20),
			Cache: pldconf.EthFeeHistoryCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	assert.Error(t, err)

	maxFeePerGasStr := pldtypes.Uint64ToUint256(1000).HexString0xPrefix()
	conf = &pldconf.GasPriceConfig{
		FixedGasPrice: &pldconf.FixedGasPricing{
			MaxFeePerGas: &maxFeePerGasStr,
			// Missing MaxPriorityFeePerGas
		},
	}

	gasPriceClient = NewGasPriceClient(ctx, conf)
	err = gasPriceClient.Init(ctx)
	assert.Error(t, err)
}

func TestInitWithDefaults(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.NoError(t, err)

	hgc := gasPriceClient.(*HybridGasPriceClient)
	// Should use defaults for missing fields
	assert.Equal(t, 85, hgc.priorityFeePercentile)
	assert.Equal(t, 20, hgc.historyBlockCount)
	assert.Equal(t, 1, hgc.baseFeeBufferFactor)
	assert.Equal(t, 10, hgc.gasPriceIncreasePercent)
	assert.True(t, hgc.ethFeeHistoryCacheEnabled)
}

// mapConfigToAPIGasPricing edge cases
func TestMapConfigToAPIGasPricingIncompleteConfig(t *testing.T) {
	ctx := context.Background()
	maxFeePerGasStr := pldtypes.Uint64ToUint256(1000).HexString0xPrefix()
	maxPriorityFeePerGasStr := pldtypes.Uint64ToUint256(100).HexString0xPrefix()

	// Test with only MaxFeePerGas set
	conf := &pldconf.FixedGasPricing{
		MaxFeePerGas:         &maxFeePerGasStr,
		MaxPriorityFeePerGas: nil,
	}

	result, err := mapConfigToAPIGasPricing(ctx, conf)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "maxPriorityFeePerGas is missing")

	// Test with only MaxPriorityFeePerGas set
	conf = &pldconf.FixedGasPricing{
		MaxFeePerGas:         nil,
		MaxPriorityFeePerGas: &maxPriorityFeePerGasStr,
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
	assert.Nil(t, result)
}

func TestStartWithNilGasPriceResponse(t *testing.T) {
	ctx := context.Background()
	mockEthClient := ethclientmocks.NewEthClient(t)

	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			Cache: pldconf.EthFeeHistoryCacheConfig{
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
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			Cache: pldconf.EthFeeHistoryCacheConfig{
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

	maxFeePerGasStr := pldtypes.Uint64ToUint256(1000).HexString0xPrefix()
	maxPriorityFeePerGasStr := pldtypes.Uint64ToUint256(100).HexString0xPrefix()

	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: &pldconf.FixedGasPricing{
			MaxFeePerGas:         &maxFeePerGasStr,
			MaxPriorityFeePerGas: &maxPriorityFeePerGasStr,
		},
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			Cache: pldconf.EthFeeHistoryCacheConfig{
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

func TestIncreaseGasPricingByPercentage(t *testing.T) {
	_, hgc, _ := NewTestGasPriceClient(t, &pldconf.GasPriceConfig{}, false)

	// Test case 1: Increase by 10%
	original := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(10000000000), // 10 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1000000000),  // 1 Gwei
	}

	result := hgc.increaseGasPricingByPercentage(original, 10)
	require.NotNil(t, result)

	// 10 Gwei + 10% = 11 Gwei = 11000000000 Wei
	assert.Equal(t, int64(11000000000), result.MaxFeePerGas.Int().Int64())

	// 1 Gwei + 10% = 1.1 Gwei = 1100000000 Wei
	assert.Equal(t, int64(1100000000), result.MaxPriorityFeePerGas.Int().Int64())

	// original has not changed
	assert.Equal(t, int64(10000000000), original.MaxFeePerGas.Int().Int64())
	assert.Equal(t, int64(1000000000), original.MaxPriorityFeePerGas.Int().Int64())

	// Test case 2: Increase by 25%
	result = hgc.increaseGasPricingByPercentage(original, 25)
	require.NotNil(t, result)

	// 10 Gwei + 25% = 12.5 Gwei = 12500000000 Wei
	assert.Equal(t, int64(12500000000), result.MaxFeePerGas.Int().Int64())

	// 1 Gwei + 25% = 1.25 Gwei = 1250000000 Wei
	assert.Equal(t, int64(1250000000), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 3: Increase by 0% (no change)
	result = hgc.increaseGasPricingByPercentage(original, 0)
	require.NotNil(t, result)

	// Should remain the same
	assert.Equal(t, original.MaxFeePerGas.Int().Int64(), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, original.MaxPriorityFeePerGas.Int().Int64(), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 4: Increase by 100% (double)
	result = hgc.increaseGasPricingByPercentage(original, 100)
	require.NotNil(t, result)

	// 10 Gwei + 100% = 20 Gwei = 20000000000 Wei
	assert.Equal(t, int64(20000000000), result.MaxFeePerGas.Int().Int64())

	// 1 Gwei + 100% = 2 Gwei = 2000000000 Wei
	assert.Equal(t, int64(2000000000), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 5: Handle partial nil fields
	partialOriginal := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(10000000000), // 10 Gwei
		MaxPriorityFeePerGas: nil,
	}

	result = hgc.increaseGasPricingByPercentage(partialOriginal, 20)
	require.NotNil(t, result)

	// MaxFeePerGas should be increased- 10 Gwei + 20% = 12 Gwei
	assert.Equal(t, int64(12000000000), result.MaxFeePerGas.Int().Int64())

	// MaxPriorityFeePerGas should remain nil
	assert.Nil(t, result.MaxPriorityFeePerGas)

	// Test case 7: Handle rounding up on integer division
	original = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(5),
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(5),
	}

	result = hgc.increaseGasPricingByPercentage(original, 10)
	require.NotNil(t, result)
	assert.Equal(t, int64(6), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, int64(6), result.MaxPriorityFeePerGas.Int().Int64())
}

func TestCapGasPricing(t *testing.T) {
	ctx := context.Background()
	// Create a gas price client with configured caps
	maxPriorityFeePerGasCapStr := pldtypes.Uint64ToUint256(2000000000).HexString0xPrefix()
	maxFeePerGasCapStr := pldtypes.Uint64ToUint256(15000000000).HexString0xPrefix()
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice:           nil,
		MaxPriorityFeePerGasCap: &maxPriorityFeePerGasCapStr,
		MaxFeePerGasCap:         &maxFeePerGasCapStr,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			Cache: pldconf.EthFeeHistoryCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}

	_, hgc, _ := NewTestGasPriceClient(t, conf, false)

	// Test case 1: Both values within caps (no capping needed)
	original := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(10000000000), // 10 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1000000000),  // 1 Gwei
	}

	result := hgc.capGasPricing(ctx, original)
	require.NotNil(t, result)

	// Values should remain unchanged since they're within caps
	assert.Equal(t, original.MaxFeePerGas.Int().Int64(), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, original.MaxPriorityFeePerGas.Int().Int64(), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 2: MaxFeePerGas exceeds cap (should be capped)
	original = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(20000000000), // 20 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1000000000),  // 1 Gwei
	}

	result = hgc.capGasPricing(ctx, original)
	require.NotNil(t, result)

	// MaxFeePerGas should be capped to 15 Gwei
	assert.Equal(t, int64(15000000000), result.MaxFeePerGas.Int().Int64())
	// MaxPriorityFeePerGas should remain unchanged
	assert.Equal(t, original.MaxPriorityFeePerGas.Int().Int64(), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 3: MaxPriorityFeePerGas exceeds cap (should be capped)
	original = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(10000000000), // 10 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(3000000000),  // 3 Gwei
	}

	result = hgc.capGasPricing(ctx, original)
	require.NotNil(t, result)

	// MaxFeePerGas should remain unchanged
	assert.Equal(t, original.MaxFeePerGas.Int().Int64(), result.MaxFeePerGas.Int().Int64())
	// MaxPriorityFeePerGas should be capped to 2 Gwei
	assert.Equal(t, int64(2000000000), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 4: Both values exceed caps (both should be capped)
	original = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(20000000000), // 20 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(3000000000),  // 3 Gwei
	}

	result = hgc.capGasPricing(ctx, original)
	require.NotNil(t, result)

	// Both should be capped
	assert.Equal(t, int64(15000000000), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, int64(2000000000), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 5: Handle partial nil fields in original
	partialOriginal := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(20000000000), // 20 Gwei
		MaxPriorityFeePerGas: nil,
	}

	result = hgc.capGasPricing(ctx, partialOriginal)
	require.NotNil(t, result)

	// MaxFeePerGas should be capped
	assert.Equal(t, int64(15000000000), result.MaxFeePerGas.Int().Int64())
	// MaxPriorityFeePerGas should remain nil
	assert.Nil(t, result.MaxPriorityFeePerGas)

	// Test case 6: Verify original is not modified
	original = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(20000000000), // 20 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(3000000000),  // 3 Gwei
	}

	originalMaxFee := original.MaxFeePerGas.Int().Int64()
	originalMaxPriorityFee := original.MaxPriorityFeePerGas.Int().Int64()

	result = hgc.capGasPricing(ctx, original)
	require.NotNil(t, result)

	// Original should remain unchanged
	assert.Equal(t, originalMaxFee, original.MaxFeePerGas.Int().Int64())
	assert.Equal(t, originalMaxPriorityFee, original.MaxPriorityFeePerGas.Int().Int64())

	// Test case 7: Test with no caps configured
	confNoCaps := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			// No caps configured
			Cache: pldconf.EthFeeHistoryCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}

	_, hgcNoCaps, _ := NewTestGasPriceClient(t, confNoCaps, false)

	original = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(20000000000), // 20 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(3000000000),  // 3 Gwei
	}

	result = hgcNoCaps.capGasPricing(ctx, original)
	require.NotNil(t, result)

	// Values should remain unchanged since no caps are configured
	assert.Equal(t, original.MaxFeePerGas.Int().Int64(), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, original.MaxPriorityFeePerGas.Int().Int64(), result.MaxPriorityFeePerGas.Int().Int64())
}

func TestCalculateNewGasPrice(t *testing.T) {
	// Create a gas price client with 10% increase percentage
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice:      nil,
		IncreasePercentage: confutil.P(10),
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			Cache: pldconf.EthFeeHistoryCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}

	_, hgc, _ := NewTestGasPriceClient(t, conf, false)

	// Test case 1: No previously submitted GPO - should return retrieved GPO as-is
	retrievedGPO := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(10000000000), // 10 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1000000000),  // 1 Gwei
	}

	result := hgc.calculateNewGasPrice(nil, retrievedGPO, false)
	require.NotNil(t, result)
	assert.Equal(t, retrievedGPO.MaxFeePerGas.Int().Int64(), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, retrievedGPO.MaxPriorityFeePerGas.Int().Int64(), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 2: Previously submitted GPO with nil fields - should return retrieved GPO as-is
	previouslySubmittedGPO := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         nil,
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1000000000), // 1 Gwei
	}

	result = hgc.calculateNewGasPrice(previouslySubmittedGPO, retrievedGPO, false)
	require.NotNil(t, result)
	assert.Equal(t, retrievedGPO.MaxFeePerGas.Int().Int64(), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, retrievedGPO.MaxPriorityFeePerGas.Int().Int64(), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 3: Previously submitted GPO with both fields nil - should return retrieved GPO as-is
	previouslySubmittedGPO = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         nil,
		MaxPriorityFeePerGas: nil,
	}

	result = hgc.calculateNewGasPrice(previouslySubmittedGPO, retrievedGPO, false)
	require.NotNil(t, result)
	assert.Equal(t, retrievedGPO.MaxFeePerGas.Int().Int64(), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, retrievedGPO.MaxPriorityFeePerGas.Int().Int64(), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 4: Retrieved GPO has higher values - should use retrieved values directly
	previouslySubmittedGPO = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(8000000000), // 8 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(800000000),  // 0.8 Gwei
	}
	retrievedGPO = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(10000000000), // 10 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1000000000),  // 1 Gwei
	}

	result = hgc.calculateNewGasPrice(previouslySubmittedGPO, retrievedGPO, false)
	require.NotNil(t, result)
	// Since retrieved values are higher than previous values, should use retrieved values directly
	assert.Equal(t, retrievedGPO.MaxFeePerGas.Int().Int64(), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, retrievedGPO.MaxPriorityFeePerGas.Int().Int64(), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 5: Retrieved GPO has lower values and underpriced flag is true - should increase retrieved GPO by 10%
	previouslySubmittedGPO = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(12000000000), // 12 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1200000000),  // 1.2 Gwei
	}
	retrievedGPO = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(10000000000), // 10 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1000000000),  // 1 Gwei
	}

	result = hgc.calculateNewGasPrice(previouslySubmittedGPO, retrievedGPO, true)
	require.NotNil(t, result)
	// Should increase previously submitted GPO by 10%
	expectedMaxFee := int64(13200000000)        // 10 Gwei + 10% = 11 Gwei
	expectedMaxPriorityFee := int64(1320000000) // 1 Gwei + 10% = 1.1 Gwei
	assert.Equal(t, expectedMaxFee, result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFee, result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 6: Retrieved GPO has lower values and underpriced flag is false - should return previously submitted GPO
	result = hgc.calculateNewGasPrice(previouslySubmittedGPO, retrievedGPO, false)
	require.NotNil(t, result)
	// Should return previously submitted GPO unchanged
	assert.Equal(t, previouslySubmittedGPO.MaxFeePerGas.Int().Int64(), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, previouslySubmittedGPO.MaxPriorityFeePerGas.Int().Int64(), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 7: Complex scenario - retrieved GPO has higher priority fee but lower total fee
	// Should use whichever is higher of the retrieved and increased previously submitted values
	previouslySubmittedGPO = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(10000000000), // 10 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1000000000),  // 1 Gwei
	}
	retrievedGPO = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(8000000000), // 8 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1200000000), // 1.2 Gwei
	}

	result = hgc.calculateNewGasPrice(previouslySubmittedGPO, retrievedGPO, false)
	require.NotNil(t, result)

	// Priority fee should be retrieved value (1.2 Gwei) since it's higher than minNew (1.1 Gwei)
	// Total fee should be minNew (11 Gwei) since retrieved (8 Gwei) is lower than minNew (11 Gwei)
	expectedMaxFee = int64(11000000000)        // 11 Gwei (minNew, since retrieved < minNew)
	expectedMaxPriorityFee = int64(1200000000) // 1.2 Gwei (retrieved, since retrieved > minNew)
	assert.Equal(t, expectedMaxFee, result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFee, result.MaxPriorityFeePerGas.Int().Int64())
}

func TestGetGasPriceObjectWithTxFixedGasPrice(t *testing.T) {
	ctx := context.Background()

	// Create a gas price client with caps configured
	maxPriorityFeePerGasCapStr := pldtypes.Uint64ToUint256(2000000000).HexString0xPrefix()
	maxFeePerGasCapStr := pldtypes.Uint64ToUint256(15000000000).HexString0xPrefix()
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice:           nil,
		MaxPriorityFeePerGasCap: &maxPriorityFeePerGasCapStr,
		MaxFeePerGasCap:         &maxFeePerGasCapStr,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			Cache: pldconf.EthFeeHistoryCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}

	_, hgc, _ := NewTestGasPriceClient(t, conf, false)

	// Test case 1: Transaction fixed gas price within caps
	txFixedGasPrice := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(10000000000), // 10 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1000000000),  // 1 Gwei
	}

	result, err := hgc.GetGasPriceObject(ctx, txFixedGasPrice, nil, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	// Should return transaction fixed gas price unchanged since it's within caps
	assert.Equal(t, txFixedGasPrice.MaxFeePerGas.Int().Int64(), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, txFixedGasPrice.MaxPriorityFeePerGas.Int().Int64(), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 2: Transaction fixed gas price exceeds caps - should be capped
	txFixedGasPrice = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(20000000000), // 20 Gwei (above 15 Gwei cap)
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(3000000000),  // 3 Gwei (above 2 Gwei cap)
	}

	result, err = hgc.GetGasPriceObject(ctx, txFixedGasPrice, nil, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	// Should return capped values
	assert.Equal(t, int64(15000000000), result.MaxFeePerGas.Int().Int64())        // 15 Gwei cap
	assert.Equal(t, int64(2000000000), result.MaxPriorityFeePerGas.Int().Int64()) // 2 Gwei cap
}

func TestGetGasPriceObjectWithPreviouslySubmittedGPO(t *testing.T) {
	ctx := context.Background()

	// Create a gas price client with 10% increase percentage
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice:      nil,
		IncreasePercentage: confutil.P(10),
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
			Cache: pldconf.EthFeeHistoryCacheConfig{
				Enabled: confutil.P(true),
			},
		},
	}

	_, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response for eth_feeHistory pricing
	mockFeeHistoryResult := createMockFeeHistoryResult(20, 20000000000, 1500000000) // 20 Gwei base fee, 1.5 Gwei tip
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil)

	// Test case 1: No previously submitted GPO, not underpriced
	result, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	// Should return eth_feeHistory gas pricing result
	expectedMaxFee := int64(1*20000000000 + 1500000000) // (1 * 20 Gwei) + 1.5 Gwei
	expectedMaxPriorityFee := int64(1500000000)         // 1.5 Gwei
	assert.Equal(t, expectedMaxFee, result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFee, result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 2: With previously submitted GPO, underpriced flag true
	previouslySubmittedGPO := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(10000000000), // 10 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1000000000),  // 1 Gwei
	}

	// Clear cache to force a new fee history call
	hgc.DeleteCache(ctx)

	result, err = hgc.GetGasPriceObject(ctx, nil, previouslySubmittedGPO, true)
	require.NoError(t, err)
	require.NotNil(t, result)
	// Should return increased eth_feeHistory gas pricing result
	// Since retrieved values are higher than previous with percentage increase applied, should use retrieved values
	assert.Equal(t, expectedMaxFee, result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFee, result.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

// Test the new GetGasPriceObject functionality with zero gas price chain
func TestGetGasPriceObjectWithZeroGasPriceChain(t *testing.T) {
	ctx := context.Background()

	// Create a gas price client configured for zero gas price chain
	maxFeePerGasStr := pldtypes.Uint64ToUint256(0).HexString0xPrefix()
	maxPriorityFeePerGasStr := pldtypes.Uint64ToUint256(0).HexString0xPrefix()
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: &pldconf.FixedGasPricing{
			MaxFeePerGas:         &maxFeePerGasStr,
			MaxPriorityFeePerGas: &maxPriorityFeePerGasStr,
		},
	}

	_, hgc, _ := NewTestGasPriceClient(t, conf, false)

	result, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	// Should return zero gas pricing object
	assert.True(t, result.MaxFeePerGas.NilOrZero())
	assert.True(t, result.MaxPriorityFeePerGas.NilOrZero())
}

// Test the new GetGasPriceObject functionality with fixed gas price from config
func TestGetGasPriceObjectWithFixedGasPriceFromConfig(t *testing.T) {
	ctx := context.Background()

	// Create a gas price client with fixed gas price from config
	maxFeePerGasStr := pldtypes.Uint64ToUint256(10000000000).HexString0xPrefix()
	maxPriorityFeePerGasStr := pldtypes.Uint64ToUint256(1000000000).HexString0xPrefix()
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: &pldconf.FixedGasPricing{
			MaxFeePerGas:         &maxFeePerGasStr,
			MaxPriorityFeePerGas: &maxPriorityFeePerGasStr,
		},
		IncreasePercentage: confutil.P(10),
	}

	_, hgc, _ := NewTestGasPriceClient(t, conf, false)

	// Test case 1: No previously submitted GPO, not underpriced
	result, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	// Should return fixed gas price from config
	assert.Equal(t, int64(10000000000), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, int64(1000000000), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 2: With previously submitted GPO, underpriced flag true
	previouslySubmittedGPO := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(8000000000), // 8 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(800000000),  // 0.8 Gwei
	}

	result, err = hgc.GetGasPriceObject(ctx, nil, previouslySubmittedGPO, true)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Since retrieved values (fixed config: 10 Gwei, 1 Gwei) are higher than previous values (8 Gwei, 0.8 Gwei)
	// with the percentage increase applied (8.8 Gwei, 0.88 Gwei),
	// the logic should use the retrieved values directly, not apply underpriced increase
	assert.Equal(t, int64(10000000000), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, int64(1000000000), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 3: With previously submitted GPO, underpriced flag false
	result, err = hgc.GetGasPriceObject(ctx, nil, previouslySubmittedGPO, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	// Since retrieved values (fixed config: 10 Gwei, 1 Gwei) are higher than previous values (8 Gwei, 0.8 Gwei),
	// the logic should use the retrieved values directly
	assert.Equal(t, int64(10000000000), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, int64(1000000000), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 4: With previously submitted GPO that has higher values, underpriced flag false
	previouslySubmittedGPO = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(12000000000), // 12 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1200000000),  // 1.2 Gwei
	}

	result, err = hgc.GetGasPriceObject(ctx, nil, previouslySubmittedGPO, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	// Should return previously submitted GPO unchanged since it has higher values
	assert.Equal(t, previouslySubmittedGPO.MaxFeePerGas.Int().Int64(), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, previouslySubmittedGPO.MaxPriorityFeePerGas.Int().Int64(), result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 5: With previously submitted GPO that has higher values, underpriced flag true
	// Since retrieved values (fixed config: 10 Gwei, 1 Gwei) are lower than previous values (12 Gwei, 1.2 Gwei),
	// the underpriced logic should increase the previously submitted GPO
	previouslySubmittedGPO = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(12000000000), // 12 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1200000000),  // 1.2 Gwei
	}

	result, err = hgc.GetGasPriceObject(ctx, nil, previouslySubmittedGPO, true)
	require.NoError(t, err)
	require.NotNil(t, result)
	// Should return increased previously submitted GPO (10% increase)
	expectedMaxFee := int64(13200000000)        // 12 Gwei + 10% = 13.2 Gwei
	expectedMaxPriorityFee := int64(1320000000) // 1.2 Gwei + 10% = 1.32 Gwei
	assert.Equal(t, expectedMaxFee, result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFee, result.MaxPriorityFeePerGas.Int().Int64())
}
