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
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/mocks/ethclientmocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/ethclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/jarcoal/httpmock"
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
		// Create different tip values across blocks where the provided tipWei is the highest
		// This ensures the else block that finds the highest tip gets exercised
		var currentTip uint64
		switch i {
		case 0:
			currentTip = tipWei / 2 // Half the provided tip
		case 1:
			currentTip = tipWei * 3 / 4 // Three quarters of the provided tip
		case blockCount - 1:
			currentTip = tipWei // Highest tip in the last block
		default:
			currentTip = tipWei / 2 // Default to lower tip for other blocks
		}
		rewards[i] = []pldtypes.HexUint256{*pldtypes.Uint64ToUint256(currentTip)}
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
	maxFeePerGasCapStr := pldtypes.Uint64ToUint256(50000000000).HexString0xPrefix() // 50 Gwei cap
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice:           nil,
		MaxPriorityFeePerGasCap: &maxPriorityFeePerGasCapStr,
		MaxFeePerGasCap:         &maxFeePerGasCapStr,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response with high tip that should be capped
	mockFeeHistoryResult := createMockFeeHistoryResult(20, 20000000000, 3000000000) // 20 Gwei base fee, 3 Gwei tip (above 1 Gwei cap)
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Once()

	gpo, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	assert.NotNil(t, gpo)

	// Verify the tip is capped to 1 Gwei (caps are applied in GetGasPriceObject, not in estimateEIP1559Fees)
	// MaxFeePerGas is not recalculated when priority fee is capped - it keeps the original value
	expectedMaxFeePerGas := int64(1*20000000000 + 3000000000) // (1 * 20 Gwei) + 3 Gwei (original tip) - default buffer factor is 1
	expectedMaxPriorityFeePerGas := int64(1000000000)         // 1 Gwei (capped)

	assert.Equal(t, expectedMaxFeePerGas, gpo.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFeePerGas, gpo.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestEthFeeHistoryGasPricingWithNoValidTips(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response with non-empty reward arrays but all empty inner arrays (no valid tips)
	mockFeeHistoryResult := &ethclient.FeeHistoryResult{
		OldestBlock:   pldtypes.HexUint64(100),
		BaseFeePerGas: []pldtypes.HexUint256{*pldtypes.Uint64ToUint256(20000000000)},
		GasUsedRatio:  []float64{0.5},
		Reward:        [][]pldtypes.HexUint256{{}}, // Non-empty outer array but empty inner arrays - no tips
	}
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Once()

	gpo, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	assert.Error(t, err)
	assert.Nil(t, gpo)
	assert.Contains(t, err.Error(), "no valid tips found in fee history")

	mockEthClient.AssertExpectations(t)
}

func TestEthFeeHistoryGasPricingWithZeroTips(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
		},
	}
	ctx, hgc, mockEthClient := NewTestGasPriceClient(t, conf, false)

	// Mock fee history response with zero tips
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

	// Verify zero tip is used
	expectedMaxFeePerGas := int64(1*20000000000 + 0) // (1 * 20 Gwei) + 0 Gwei (zero tip) - default buffer factor is 1
	expectedMaxPriorityFeePerGas := int64(0)         // 0 Gwei (zero tip)

	assert.Equal(t, expectedMaxFeePerGas, gpo.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFeePerGas, gpo.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestEthFeeHistoryGasPricingEmptyFeeHistory(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
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
	assert.Contains(t, err.Error(), "Fee history returned empty data")

	mockEthClient.AssertExpectations(t)
}

func TestEthFeeHistoryGasPricingRPCError(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
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

func TestInitValidation(t *testing.T) {
	ctx := context.Background()
	// Test with invalid percentile (should log error but not panic)
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(150), // Invalid: > 100
			HistoryBlockCount:     confutil.P(20),
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
}

func TestInitWithCacheEnabledEthFeeHistory(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			Cache: pldconf.GasPriceCacheConfig{
				Enabled:     confutil.P(true),
				RefreshTime: confutil.P("15s"),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.NoError(t, err)

	hgc := gasPriceClient.(*HybridGasPriceClient)
	// Should initialize cache
	assert.NotNil(t, hgc.gasPriceCache)
	assert.Equal(t, 15*time.Second, hgc.refreshTime)
}

func TestInitWithCacheEnabledGasOracle(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{"maxFeePerGas": "{{.maxFeePerGas}}", "maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"}`,
			Cache: pldconf.GasPriceCacheConfig{
				Enabled:     confutil.P(true),
				RefreshTime: confutil.P("45s"),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.NoError(t, err)

	hgc := gasPriceClient.(*HybridGasPriceClient)
	// Should initialize cache and gas oracle HTTP client
	assert.NotNil(t, hgc.gasOracleHTTPClient)
	assert.NotNil(t, hgc.gasPriceCache)
	assert.Equal(t, 45*time.Second, hgc.refreshTime)
}

func TestInitWithCacheDisabled(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			Cache: pldconf.GasPriceCacheConfig{
				Enabled: confutil.P(false),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.NoError(t, err)

	hgc := gasPriceClient.(*HybridGasPriceClient)
	// Should not initialize cache
	assert.Nil(t, hgc.gasPriceCache)
}

func TestInitWithInvalidCacheRefreshTime(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			Cache: pldconf.GasPriceCacheConfig{
				Enabled:     confutil.P(true),
				RefreshTime: confutil.P("invalid-duration"),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid cache refresh time")
}

func TestInitWithGasOracleCacheTakesPrecedence(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			Cache: pldconf.GasPriceCacheConfig{
				Enabled:     confutil.P(true),
				RefreshTime: confutil.P("30s"),
			},
		},
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{"maxFeePerGas": "{{.maxFeePerGas}}", "maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"}`,
			Cache: pldconf.GasPriceCacheConfig{
				Enabled:     confutil.P(true),
				RefreshTime: confutil.P("60s"),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.NoError(t, err)

	hgc := gasPriceClient.(*HybridGasPriceClient)
	// Should use gas oracle cache settings (60s, not 30s)
	assert.NotNil(t, hgc.gasOracleHTTPClient)
	assert.NotNil(t, hgc.gasPriceCache)
	assert.Equal(t, 60*time.Second, hgc.refreshTime)
}

func TestInitWithGasOracleEmptyTemplate(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: "", // Empty template
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Gas oracle template is empty")
}

func TestInitWithGasOracleTemplateParseError(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{{invalid template syntax`, // Invalid template syntax
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to parse gas oracle template")
}

func TestInitWithInvalidPriorityFeePercentile(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(150), // Invalid: > 100
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid priority fee percentile: 150")
}

func TestInitWithGasOracleInvalidURL(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "://invalid-url", // Invalid URL format
			},
			ResponseTemplate: `{"maxFeePerGas": "{{.maxFeePerGas}}", "maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"}`,
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid HTTP URL")
}

func TestInitWithGasOracleInvalidCacheRefreshTime(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{"maxFeePerGas": "{{.maxFeePerGas}}", "maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"}`,
			Cache: pldconf.GasPriceCacheConfig{
				Enabled:     confutil.P(true),
				RefreshTime: confutil.P("invalid-duration"), // Invalid duration
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid cache refresh time")
}

func TestGetCachedGasPriceWithCache(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			Cache: pldconf.GasPriceCacheConfig{
				Enabled:     confutil.P(true),
				RefreshTime: confutil.P("30s"),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.NoError(t, err)

	hgc := gasPriceClient.(*HybridGasPriceClient)

	// Test with no cached data
	found, cached := hgc.getCachedGasPrice(ctx)
	assert.False(t, found)
	assert.Nil(t, cached)

	// Set some cached data
	testGasPrice := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(10000000000),
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1000000000),
	}
	hgc.setCachedGasPrice(testGasPrice)

	// Test with cached data
	found, cached = hgc.getCachedGasPrice(ctx)
	assert.True(t, found)
	assert.Equal(t, testGasPrice.MaxFeePerGas.Int().Int64(), cached.MaxFeePerGas.Int().Int64())
	assert.Equal(t, testGasPrice.MaxPriorityFeePerGas.Int().Int64(), cached.MaxPriorityFeePerGas.Int().Int64())
}

func TestGetCachedGasPriceWithoutCache(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			Cache: pldconf.GasPriceCacheConfig{
				Enabled: confutil.P(false),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.NoError(t, err)

	hgc := gasPriceClient.(*HybridGasPriceClient)

	// Test with no cache
	found, cached := hgc.getCachedGasPrice(ctx)
	assert.False(t, found)
	assert.Nil(t, cached)
}

func TestSetCachedGasPriceWithCache(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			Cache: pldconf.GasPriceCacheConfig{
				Enabled:     confutil.P(true),
				RefreshTime: confutil.P("30s"),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.NoError(t, err)

	hgc := gasPriceClient.(*HybridGasPriceClient)

	// Set cached data
	testGasPrice := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(20000000000),
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(2000000000),
	}
	hgc.setCachedGasPrice(testGasPrice)

	// Verify it was cached
	found, cached := hgc.getCachedGasPrice(ctx)
	assert.True(t, found)
	assert.Equal(t, testGasPrice.MaxFeePerGas.Int().Int64(), cached.MaxFeePerGas.Int().Int64())
	assert.Equal(t, testGasPrice.MaxPriorityFeePerGas.Int().Int64(), cached.MaxPriorityFeePerGas.Int().Int64())
}

func TestSetCachedGasPriceWithoutCache(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			Cache: pldconf.GasPriceCacheConfig{
				Enabled: confutil.P(false),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.NoError(t, err)

	hgc := gasPriceClient.(*HybridGasPriceClient)

	// Set cached data (should be no-op)
	testGasPrice := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(20000000000),
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(2000000000),
	}
	hgc.setCachedGasPrice(testGasPrice)

	// Verify it was not cached
	found, cached := hgc.getCachedGasPrice(ctx)
	assert.False(t, found)
	assert.Nil(t, cached)
}

func TestInitValidationWithInvalidGasPriceCaps(t *testing.T) {
	ctx := context.Background()

	// Test with invalid MaxPriorityFeePerGasCap
	conf := &pldconf.GasPriceConfig{
		MaxPriorityFeePerGasCap: confutil.P("invalid_hex_string"),
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid integer")

	// Test with invalid MaxFeePerGasCap
	conf = &pldconf.GasPriceConfig{
		MaxFeePerGasCap: confutil.P("not_a_hex_value"),
	}

	gasPriceClient = NewGasPriceClient(ctx, conf)
	err = gasPriceClient.Init(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid integer")

	// Test with both invalid caps
	conf = &pldconf.GasPriceConfig{
		MaxPriorityFeePerGasCap: confutil.P("invalid_priority"),
		MaxFeePerGasCap:         confutil.P("invalid_fee"),
	}

	gasPriceClient = NewGasPriceClient(ctx, conf)
	err = gasPriceClient.Init(ctx)
	assert.Error(t, err)
	// Should fail on the first invalid cap (MaxPriorityFeePerGasCap)
	assert.Contains(t, err.Error(), "Invalid integer")
}

func TestInitValidationWithValidGasPriceCaps(t *testing.T) {
	ctx := context.Background()

	// Test with valid gas price caps
	maxPriorityFeePerGasCapStr := pldtypes.Uint64ToUint256(2000000000).HexString0xPrefix() // 2 Gwei
	maxFeePerGasCapStr := pldtypes.Uint64ToUint256(15000000000).HexString0xPrefix()        // 15 Gwei
	conf := &pldconf.GasPriceConfig{
		MaxPriorityFeePerGasCap: &maxPriorityFeePerGasCapStr,
		MaxFeePerGasCap:         &maxFeePerGasCapStr,
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	err := gasPriceClient.Init(ctx)
	require.NoError(t, err)

	hgc := gasPriceClient.(*HybridGasPriceClient)
	// Verify caps are set correctly
	assert.NotNil(t, hgc.maxPriorityFeePerGasCap)
	assert.NotNil(t, hgc.maxFeePerGasCap)
	assert.Equal(t, int64(2000000000), hgc.maxPriorityFeePerGasCap.Int().Int64())
	assert.Equal(t, int64(15000000000), hgc.maxFeePerGasCap.Int().Int64())
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
	assert.Contains(t, err.Error(), "missing field maxPriorityFeePerGas")

	// Test with only MaxPriorityFeePerGas set
	conf = &pldconf.FixedGasPricing{
		MaxFeePerGas:         nil,
		MaxPriorityFeePerGas: &maxPriorityFeePerGasStr,
	}

	result, err = mapConfigToAPIGasPricing(ctx, conf)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "missing field maxFeePerGas")

	// Test with both fields nil - this should return a valid object with nil fields
	conf = &pldconf.FixedGasPricing{
		MaxFeePerGas:         nil,
		MaxPriorityFeePerGas: nil,
	}

	result, err = mapConfigToAPIGasPricing(ctx, conf)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestMapConfigToAPIGasPricingParsingErrors(t *testing.T) {
	ctx := context.Background()

	// Test with invalid MaxFeePerGas hex string
	invalidMaxFeePerGasStr := "invalid_hex_string"
	validMaxPriorityFeePerGasStr := pldtypes.Uint64ToUint256(100).HexString0xPrefix()
	conf := &pldconf.FixedGasPricing{
		MaxFeePerGas:         &invalidMaxFeePerGasStr,
		MaxPriorityFeePerGas: &validMaxPriorityFeePerGasStr,
	}

	result, err := mapConfigToAPIGasPricing(ctx, conf)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Invalid integer")

	// Test with invalid MaxPriorityFeePerGas hex string
	validMaxFeePerGasStr := pldtypes.Uint64ToUint256(1000).HexString0xPrefix()
	invalidMaxPriorityFeePerGasStr := "not_a_hex_value"
	conf = &pldconf.FixedGasPricing{
		MaxFeePerGas:         &validMaxFeePerGasStr,
		MaxPriorityFeePerGas: &invalidMaxPriorityFeePerGasStr,
	}

	result, err = mapConfigToAPIGasPricing(ctx, conf)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Invalid integer")

	// Test with both invalid hex strings
	conf = &pldconf.FixedGasPricing{
		MaxFeePerGas:         &invalidMaxFeePerGasStr,
		MaxPriorityFeePerGas: &invalidMaxPriorityFeePerGasStr,
	}

	result, err = mapConfigToAPIGasPricing(ctx, conf)
	assert.Error(t, err)
	assert.Nil(t, result)
	// Should fail on the first invalid field (MaxFeePerGas)
	assert.Contains(t, err.Error(), "Invalid integer")
}

func TestMapConfigToAPIGasPricingValidConfig(t *testing.T) {
	ctx := context.Background()

	// Test with valid configuration
	validMaxFeePerGasStr := pldtypes.Uint64ToUint256(10000000000).HexString0xPrefix()        // 10 Gwei
	validMaxPriorityFeePerGasStr := pldtypes.Uint64ToUint256(1000000000).HexString0xPrefix() // 1 Gwei
	conf := &pldconf.FixedGasPricing{
		MaxFeePerGas:         &validMaxFeePerGasStr,
		MaxPriorityFeePerGas: &validMaxPriorityFeePerGasStr,
	}

	result, err := mapConfigToAPIGasPricing(ctx, conf)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(10000000000), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, int64(1000000000), result.MaxPriorityFeePerGas.Int().Int64())
}

func TestGetGasPriceObjectWithCacheHit(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			Cache: pldconf.GasPriceCacheConfig{
				Enabled:     confutil.P(true),
				RefreshTime: confutil.P("30s"),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	hgc := gasPriceClient.(*HybridGasPriceClient)
	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Pre-populate cache
	cachedGasPrice := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(15000000000),
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1500000000),
	}
	hgc.setCachedGasPrice(cachedGasPrice)

	// Mock eth client
	mockEthClient := ethclientmocks.NewEthClient(t)
	hgc.ethClient = mockEthClient

	// Mock GasPrice call
	gasPrice := pldtypes.Uint64ToUint256(20000000000)
	mockEthClient.On("GasPrice", ctx).Return(gasPrice, nil).Once()

	hgc.Start(ctx, mockEthClient)

	// Get gas price object - should use cached value
	result, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should return cached values
	assert.Equal(t, int64(15000000000), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, int64(1500000000), result.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestGetGasPriceObjectWithCacheMiss(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			Cache: pldconf.GasPriceCacheConfig{
				Enabled:     confutil.P(true),
				RefreshTime: confutil.P("30s"),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	hgc := gasPriceClient.(*HybridGasPriceClient)
	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Mock eth client
	mockEthClient := ethclientmocks.NewEthClient(t)
	hgc.ethClient = mockEthClient

	// Mock GasPrice call
	gasPrice := pldtypes.Uint64ToUint256(20000000000)
	mockEthClient.On("GasPrice", ctx).Return(gasPrice, nil).Once()

	// Mock fee history response
	mockFeeHistoryResult := createMockFeeHistoryResult(20, 20000000000, 1500000000)
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Once()

	hgc.Start(ctx, mockEthClient)

	// Get gas price object - should fetch fresh data and cache it
	result, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should return fresh values
	expectedMaxFee := int64(1*20000000000 + 1500000000) // (1 * 20 Gwei) + 1.5 Gwei
	expectedMaxPriorityFee := int64(1500000000)         // 1.5 Gwei
	assert.Equal(t, expectedMaxFee, result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFee, result.MaxPriorityFeePerGas.Int().Int64())

	// Verify it was cached
	found, cached := hgc.getCachedGasPrice(ctx)
	assert.True(t, found)
	assert.Equal(t, expectedMaxFee, cached.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFee, cached.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestGetGasPriceObjectWithGasOracleCache(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{"maxFeePerGas": "{{.maxFeePerGas}}", "maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"}`,
			Cache: pldconf.GasPriceCacheConfig{
				Enabled:     confutil.P(true),
				RefreshTime: confutil.P("30s"),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	hgc := gasPriceClient.(*HybridGasPriceClient)
	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Verify cache was initialized
	assert.NotNil(t, hgc.gasPriceCache)

	// Mock eth client
	mockEthClient := ethclientmocks.NewEthClient(t)
	hgc.ethClient = mockEthClient

	// Mock GasPrice call
	gasPrice := pldtypes.Uint64ToUint256(20000000000)
	mockEthClient.On("GasPrice", ctx).Return(gasPrice, nil).Once()

	// Setup httpmock
	httpmock.ActivateNonDefault(hgc.gasOracleHTTPClient.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.Reset()

	// Mock successful HTTP response
	mockResponse := `{
		"maxFeePerGas": "0x2FAF080",
		"maxPriorityFeePerGas": "0x3B9ACA0"
	}`
	httpmock.RegisterResponder("GET", "https://api.example.com/gas",
		httpmock.NewStringResponder(200, mockResponse))

	hgc.Start(ctx, mockEthClient)

	// Get gas price object - should fetch from gas oracle and cache it
	result, err := hgc.GetGasPriceObject(ctx, nil, nil, false)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should return gas oracle values
	assert.Equal(t, int64(50000000), result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, int64(62500000), result.MaxPriorityFeePerGas.Int().Int64())

	// Verify it was cached
	found, cached := hgc.getCachedGasPrice(ctx)
	assert.True(t, found)
	assert.Equal(t, int64(50000000), cached.MaxFeePerGas.Int().Int64())
	assert.Equal(t, int64(62500000), cached.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestStartWithNilGasPriceResponse(t *testing.T) {
	ctx := context.Background()
	mockEthClient := ethclientmocks.NewEthClient(t)

	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85),
			HistoryBlockCount:     confutil.P(20),
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

func TestStartWithCacheEnabled(t *testing.T) {
	ctx := context.Background()
	mockEthClient := ethclientmocks.NewEthClient(t)

	conf := &pldconf.GasPriceConfig{
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			Cache: pldconf.GasPriceCacheConfig{
				Enabled:     confutil.P(true),
				RefreshTime: confutil.P("30s"),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	hgc := gasPriceClient.(*HybridGasPriceClient)
	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Mock GasPrice call
	gasPrice := pldtypes.Uint64ToUint256(20000000000)
	mockEthClient.On("GasPrice", ctx).Return(gasPrice, nil).Once()

	hgc.Start(ctx, mockEthClient)

	// Should start refresh ticker
	assert.NotNil(t, hgc.gasPriceRefreshTicker)

	mockEthClient.AssertExpectations(t)
}

func TestStartWithCacheDisabled(t *testing.T) {
	ctx := context.Background()
	mockEthClient := ethclientmocks.NewEthClient(t)

	conf := &pldconf.GasPriceConfig{
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			Cache: pldconf.GasPriceCacheConfig{
				Enabled: confutil.P(false),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	hgc := gasPriceClient.(*HybridGasPriceClient)
	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Mock GasPrice call
	gasPrice := pldtypes.Uint64ToUint256(20000000000)
	mockEthClient.On("GasPrice", ctx).Return(gasPrice, nil).Once()

	hgc.Start(ctx, mockEthClient)

	// Should not start refresh ticker
	assert.Nil(t, hgc.gasPriceRefreshTicker)

	mockEthClient.AssertExpectations(t)
}

func TestRefreshGasPriceCacheWithGasOracle(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{"maxFeePerGas": "{{.maxFeePerGas}}", "maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"}`,
			Cache: pldconf.GasPriceCacheConfig{
				Enabled:     confutil.P(true),
				RefreshTime: confutil.P("30s"),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	hgc := gasPriceClient.(*HybridGasPriceClient)
	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Setup httpmock
	httpmock.ActivateNonDefault(hgc.gasOracleHTTPClient.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.Reset()

	// Mock successful HTTP response
	mockResponse := `{
		"maxFeePerGas": "0x2FAF080",
		"maxPriorityFeePerGas": "0x3B9ACA0"
	}`
	httpmock.RegisterResponder("GET", "https://api.example.com/gas",
		httpmock.NewStringResponder(200, mockResponse))

	// Call refresh
	hgc.refreshGasPriceCache(ctx)

	// Verify cache was updated
	found, cached := hgc.getCachedGasPrice(ctx)
	assert.True(t, found)
	assert.Equal(t, int64(50000000), cached.MaxFeePerGas.Int().Int64())
	assert.Equal(t, int64(62500000), cached.MaxPriorityFeePerGas.Int().Int64())
}

func TestRefreshGasPriceCacheWithEthFeeHistory(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			Cache: pldconf.GasPriceCacheConfig{
				Enabled:     confutil.P(true),
				RefreshTime: confutil.P("30s"),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	hgc := gasPriceClient.(*HybridGasPriceClient)
	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Mock eth client
	mockEthClient := ethclientmocks.NewEthClient(t)
	hgc.ethClient = mockEthClient

	// Mock fee history response
	mockFeeHistoryResult := createMockFeeHistoryResult(20, 20000000000, 1500000000)
	mockEthClient.On("FeeHistory", ctx, 20, "latest", []float64{85.0}).Return(mockFeeHistoryResult, nil).Once()

	// Call refresh
	hgc.refreshGasPriceCache(ctx)

	// Verify cache was updated
	found, cached := hgc.getCachedGasPrice(ctx)
	assert.True(t, found)
	expectedMaxFee := int64(1*20000000000 + 1500000000) // (1 * 20 Gwei) + 1.5 Gwei
	expectedMaxPriorityFee := int64(1500000000)         // 1.5 Gwei
	assert.Equal(t, expectedMaxFee, cached.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFee, cached.MaxPriorityFeePerGas.Int().Int64())

	mockEthClient.AssertExpectations(t)
}

func TestRefreshGasPriceCacheWithError(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{"maxFeePerGas": "{{.maxFeePerGas}}", "maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"}`,
			Cache: pldconf.GasPriceCacheConfig{
				Enabled:     confutil.P(true),
				RefreshTime: confutil.P("30s"),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	hgc := gasPriceClient.(*HybridGasPriceClient)
	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Setup httpmock
	httpmock.ActivateNonDefault(hgc.gasOracleHTTPClient.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.Reset()

	// Mock error response
	httpmock.RegisterResponder("GET", "https://api.example.com/gas",
		httpmock.NewStringResponder(500, "Internal Server Error"))

	// Call refresh (should not panic)
	hgc.refreshGasPriceCache(ctx)

	// Cache should remain empty
	found, cached := hgc.getCachedGasPrice(ctx)
	assert.False(t, found)
	assert.Nil(t, cached)
}

func TestRefreshGasPriceCacheWithoutCache(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			Cache: pldconf.GasPriceCacheConfig{
				Enabled: confutil.P(false),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	hgc := gasPriceClient.(*HybridGasPriceClient)
	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Call refresh (should be no-op)
	hgc.refreshGasPriceCache(ctx)

	// Should not panic and cache should remain nil
	assert.Nil(t, hgc.gasPriceCache)
}

func TestStartGasPriceRefreshWithTickerAndCancellation(t *testing.T) {
	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conf := &pldconf.GasPriceConfig{
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{"maxFeePerGas": "{{.maxFeePerGas}}", "maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"}`,
			Cache: pldconf.GasPriceCacheConfig{
				Enabled:     confutil.P(true),
				RefreshTime: confutil.P("50ms"), // Very short refresh period for testing
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	hgc := gasPriceClient.(*HybridGasPriceClient)
	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Setup httpmock
	httpmock.ActivateNonDefault(hgc.gasOracleHTTPClient.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.Reset()

	// Mock successful HTTP response
	mockResponse := `{
		"maxFeePerGas": "0x2FAF080",
		"maxPriorityFeePerGas": "0x3B9ACA0"
	}`
	httpmock.RegisterResponder("GET", "https://api.example.com/gas",
		httpmock.NewStringResponder(200, mockResponse))

	// Start the refresh mechanism
	hgc.startGasPriceRefresh(ctx)

	// Verify ticker was created
	assert.NotNil(t, hgc.gasPriceRefreshTicker)

	// Wait for the ticker to fire at least once (allow some buffer time)
	time.Sleep(100 * time.Millisecond)

	// Verify that the cache was populated by the ticker
	found, cached := hgc.getCachedGasPrice(ctx)
	assert.True(t, found)
	assert.Equal(t, int64(50000000), cached.MaxFeePerGas.Int().Int64())
	assert.Equal(t, int64(62500000), cached.MaxPriorityFeePerGas.Int().Int64())

	// Cancel the context to stop the ticker
	cancel()

	// Wait a bit to ensure the goroutine has time to exit
	time.Sleep(50 * time.Millisecond)

	// Verify the ticker is still accessible (it should be stopped by the defer in the goroutine)
	// We can't directly test if the goroutine exited, but we can verify the ticker exists
	assert.NotNil(t, hgc.gasPriceRefreshTicker)
}

func TestStartGasPriceRefreshWithoutCache(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.GasPriceConfig{
		EthFeeHistory: pldconf.EthFeeHistoryConfig{
			Cache: pldconf.GasPriceCacheConfig{
				Enabled: confutil.P(false),
			},
		},
	}

	gasPriceClient := NewGasPriceClient(ctx, conf)
	hgc := gasPriceClient.(*HybridGasPriceClient)
	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Should not start refresh when cache is nil
	hgc.startGasPriceRefresh(ctx)

	// Verify no ticker was created
	assert.Nil(t, hgc.gasPriceRefreshTicker)
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

	// Test case 8: Retrieved GPO has higher priority fee but lower total fee than previously submitted
	// This tests the Sign() == -1 condition in lines 219 and 225
	previouslySubmittedGPO = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(10000000000), // 10 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1000000000),  // 1 Gwei
	}
	retrievedGPO = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(5000000000), // 5 Gwei (lower than minNew 11 Gwei)
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1200000000), // 1.2 Gwei (higher than previous 1 Gwei, and higher than minNew 1.1 Gwei)
	}

	result = hgc.calculateNewGasPrice(previouslySubmittedGPO, retrievedGPO, false)
	require.NotNil(t, result)

	// Total fee should be minNew since retrieved < minNew, priority fee should be retrieved since retrieved > minNew
	expectedMaxFee = int64(11000000000)        // 11 Gwei (minNew, since retrieved < minNew)
	expectedMaxPriorityFee = int64(1200000000) // 1.2 Gwei (retrieved, since retrieved > minNew)
	assert.Equal(t, expectedMaxFee, result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFee, result.MaxPriorityFeePerGas.Int().Int64())

	// Test case 9: Retrieved GPO has higher total fee but lower priority fee than minimum new values
	// This specifically tests the Sign() == -1 condition for priority fee in line 219
	previouslySubmittedGPO = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(10000000000), // 10 Gwei
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(1000000000),  // 1 Gwei
	}
	retrievedGPO = &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         pldtypes.Uint64ToUint256(15000000000), // 15 Gwei (higher than previous 10 Gwei, and higher than minNew 11 Gwei)
		MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(900000000),   // 0.9 Gwei (higher than previous 1 Gwei, but lower than minNew 1.1 Gwei)
	}

	result = hgc.calculateNewGasPrice(previouslySubmittedGPO, retrievedGPO, false)
	require.NotNil(t, result)

	// Total fee should be retrieved since retrieved > minNew, priority fee should be minNew since retrieved < minNew
	expectedMaxFee = int64(15000000000)        // 15 Gwei (retrieved, since retrieved > minNew)
	expectedMaxPriorityFee = int64(1100000000) // 1.1 Gwei (minNew, since retrieved < minNew)
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

// TestGetGasPriceFromGasOracleSuccess tests successful gas oracle response
func TestGetGasPriceFromGasOracleSuccess(t *testing.T) {
	// Create gas price client with gas oracle config first
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{
				"maxFeePerGas": "{{.maxFeePerGas}}",
				"maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"
			}`,
		},
	}

	ctx, hgc, _ := NewTestGasPriceClient(t, conf, false)

	// Initialize the client to set up gas oracle
	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Setup httpmock for the specific client after it's created
	httpmock.ActivateNonDefault(hgc.gasOracleHTTPClient.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.Reset()

	// Mock successful HTTP response
	mockResponse := `{
		"maxFeePerGas": "0x2FAF080",
		"maxPriorityFeePerGas": "0x3B9ACA0"
	}`
	httpmock.RegisterResponder("GET", "https://api.example.com/gas",
		httpmock.NewStringResponder(200, mockResponse))

	// Call getGasPriceFromGasOracle
	result, err := hgc.getGasPriceFromGasOracle(ctx)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the result
	expectedMaxFee := int64(50000000)         // 0x2FAF080 = 50,000,000 Wei = 0.05 Gwei
	expectedMaxPriorityFee := int64(62500000) // 0x3B9ACA0 = 62,500,000 Wei = 0.0625 Gwei
	assert.Equal(t, expectedMaxFee, result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFee, result.MaxPriorityFeePerGas.Int().Int64())

	// Verify HTTP call was made
	assert.Equal(t, 1, httpmock.GetTotalCallCount())
}

// TestGetGasPriceFromGasOracleHTTPError tests HTTP error response
func TestGetGasPriceFromGasOracleHTTPError(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{
				"maxFeePerGas": "{{.maxFeePerGas}}",
				"maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"
			}`,
		},
	}

	ctx, hgc, _ := NewTestGasPriceClient(t, conf, false)

	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Setup httpmock for the specific client after it's created
	httpmock.ActivateNonDefault(hgc.gasOracleHTTPClient.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.Reset()

	// Mock HTTP error response
	httpmock.RegisterResponder("GET", "https://api.example.com/gas",
		httpmock.NewStringResponder(500, "Internal Server Error"))

	result, err := hgc.getGasPriceFromGasOracle(ctx)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "status 500")
}

// TestGetGasPriceFromGasOracleInvalidJSON tests invalid JSON response
func TestGetGasPriceFromGasOracleInvalidJSON(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{
				"maxFeePerGas": "{{.maxFeePerGas}}",
				"maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"
			}`,
		},
	}

	ctx, hgc, _ := NewTestGasPriceClient(t, conf, false)

	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Setup httpmock for the specific client after it's created
	httpmock.ActivateNonDefault(hgc.gasOracleHTTPClient.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.Reset()

	// Mock invalid JSON response
	httpmock.RegisterResponder("GET", "https://api.example.com/gas",
		httpmock.NewStringResponder(200, "invalid json"))

	result, err := hgc.getGasPriceFromGasOracle(ctx)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Failed to parse gas oracle API response as JSON")
}

// TestGetGasPriceFromGasOracleTemplateExecutionFailure tests template execution failure
func TestGetGasPriceFromGasOracleTemplateExecutionFailure(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{
				"maxFeePerGas": "{{.data.maxFeePerGas}}",
				"maxPriorityFeePerGas": "{{.data.maxPriorityFeePerGas}}"
			}`,
		},
	}

	ctx, hgc, _ := NewTestGasPriceClient(t, conf, false)

	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Setup httpmock for the specific client after it's created
	httpmock.ActivateNonDefault(hgc.gasOracleHTTPClient.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.Reset()

	// Mock response that will cause template execution to fail
	mockResponse := `{
		"maxFeePerGas": "0x2FAF080",
		"maxPriorityFeePerGas": "0x3B9ACA0"
	}`
	httpmock.RegisterResponder("GET", "https://api.example.com/gas",
		httpmock.NewStringResponder(200, mockResponse))

	result, err := hgc.getGasPriceFromGasOracle(ctx)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Failed to parse template result as PublicTxGasPricing JSON")
}

// TestGetGasPriceFromGasOracleMissingMaxFeePerGas tests missing maxFeePerGas in template result
func TestGetGasPriceFromGasOracleMissingMaxFeePerGas(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{
				"maxFeePerGas": "{{.maxFeePerGas}}",
				"maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"
			}`,
		},
	}

	ctx, hgc, _ := NewTestGasPriceClient(t, conf, false)

	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Setup httpmock for the specific client after it's created
	httpmock.ActivateNonDefault(hgc.gasOracleHTTPClient.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.Reset()

	// Mock response missing maxFeePerGas
	mockResponse := `{
		"maxPriorityFeePerGas": "0x3B9ACA0"
	}`
	httpmock.RegisterResponder("GET", "https://api.example.com/gas",
		httpmock.NewStringResponder(200, mockResponse))

	result, err := hgc.getGasPriceFromGasOracle(ctx)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Failed to parse template result as PublicTxGasPricing JSON")
}

// TestGetGasPriceFromGasOracleMissingMaxPriorityFeePerGas tests missing maxPriorityFeePerGas in template result
func TestGetGasPriceFromGasOracleMissingMaxPriorityFeePerGas(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{
				"maxFeePerGas": "{{.maxFeePerGas}}",
				"maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"
			}`,
		},
	}

	ctx, hgc, _ := NewTestGasPriceClient(t, conf, false)

	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Setup httpmock for the specific client after it's created
	httpmock.ActivateNonDefault(hgc.gasOracleHTTPClient.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.Reset()

	// Mock response missing maxPriorityFeePerGas
	mockResponse := `{
		"maxFeePerGas": "0x2FAF080"
	}`
	httpmock.RegisterResponder("GET", "https://api.example.com/gas",
		httpmock.NewStringResponder(200, mockResponse))

	result, err := hgc.getGasPriceFromGasOracle(ctx)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Failed to parse template result as PublicTxGasPricing JSON")
}

// TestGetGasPriceFromGasOracleDifferentURL tests different URL endpoint
func TestGetGasPriceFromGasOracleDifferentURL(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.different-provider.com/eth/gas",
			},
			ResponseTemplate: `{
				"maxFeePerGas": "{{.maxFeePerGas}}",
				"maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"
			}`,
		},
	}

	ctx, hgc, _ := NewTestGasPriceClient(t, conf, false)

	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Setup httpmock for the specific client after it's created
	httpmock.ActivateNonDefault(hgc.gasOracleHTTPClient.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.Reset()

	// Mock successful response from different provider
	mockResponse := `{
		"maxFeePerGas": "0x2FAF080",
		"maxPriorityFeePerGas": "0x3B9ACA0"
	}`
	httpmock.RegisterResponder("GET", "https://api.different-provider.com/eth/gas",
		httpmock.NewStringResponder(200, mockResponse))

	result, err := hgc.getGasPriceFromGasOracle(ctx)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the result
	expectedMaxFee := int64(50000000)
	expectedMaxPriorityFee := int64(62500000)
	assert.Equal(t, expectedMaxFee, result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFee, result.MaxPriorityFeePerGas.Int().Int64())

	// Verify GET call was made
	assert.Equal(t, 1, httpmock.GetTotalCallCount())
}

// TestGetGasPriceFromGasOracleComplexTemplate tests complex template with nested data
func TestGetGasPriceFromGasOracleComplexTemplate(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{
				"maxFeePerGas": "{{.result.gasPrice.maxFeePerGas}}",
				"maxPriorityFeePerGas": "{{.result.gasPrice.maxPriorityFeePerGas}}"
			}`,
		},
	}

	ctx, hgc, _ := NewTestGasPriceClient(t, conf, false)

	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Setup httpmock for the specific client after it's created
	httpmock.ActivateNonDefault(hgc.gasOracleHTTPClient.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.Reset()

	// Mock complex JSON response
	mockResponse := `{
		"result": {
			"gasPrice": {
				"maxFeePerGas": "0x2FAF080",
				"maxPriorityFeePerGas": "0x3B9ACA0"
			}
		}
	}`
	httpmock.RegisterResponder("GET", "https://api.example.com/gas",
		httpmock.NewStringResponder(200, mockResponse))

	result, err := hgc.getGasPriceFromGasOracle(ctx)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the result
	expectedMaxFee := int64(50000000)
	expectedMaxPriorityFee := int64(62500000)
	assert.Equal(t, expectedMaxFee, result.MaxFeePerGas.Int().Int64())
	assert.Equal(t, expectedMaxPriorityFee, result.MaxPriorityFeePerGas.Int().Int64())
}

func TestGetGasPriceFromGasOracleNetworkError(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{
				"maxFeePerGas": "{{.maxFeePerGas}}",
				"maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"
			}`,
		},
	}

	ctx, hgc, _ := NewTestGasPriceClient(t, conf, false)

	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Setup httpmock for the specific client after it's created
	httpmock.ActivateNonDefault(hgc.gasOracleHTTPClient.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.Reset()

	// Mock network error
	httpmock.RegisterResponder("GET", "https://api.example.com/gas",
		httpmock.NewErrorResponder(fmt.Errorf("network error")))

	result, err := hgc.getGasPriceFromGasOracle(ctx)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Failed to call gas oracle API")
}

func TestGetGasPriceFromGasOracleTemplateParseError(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{
				"maxFeePerGas": "{{.nonexistentField}}",
				"maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"
			}`,
		},
	}

	ctx, hgc, _ := NewTestGasPriceClient(t, conf, false)

	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Setup httpmock for the specific client after it's created
	httpmock.ActivateNonDefault(hgc.gasOracleHTTPClient.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.Reset()

	// Mock response that will cause template parsing to fail
	mockResponse := `{
		"maxFeePerGas": "0x2FAF080",
		"maxPriorityFeePerGas": "0x3B9ACA0"
	}`
	httpmock.RegisterResponder("GET", "https://api.example.com/gas",
		httpmock.NewStringResponder(200, mockResponse))

	result, err := hgc.getGasPriceFromGasOracle(ctx)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Failed to parse template result as PublicTxGasPricing JSON")
}

func TestGetGasPriceFromGasOracleTemplateExecutionError(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			ResponseTemplate: `{"maxFeePerGas": "{{.maxFeePerGas}}", "maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"}`,
		},
	}

	ctx, hgc, _ := NewTestGasPriceClient(t, conf, false)

	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Setup httpmock for the specific client after it's created
	httpmock.ActivateNonDefault(hgc.gasOracleHTTPClient.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.Reset()

	// Mock response that will cause template execution to fail (missing required fields)
	mockResponse := `{
		"someOtherField": "value"
	}`
	httpmock.RegisterResponder("GET", "https://api.example.com/gas",
		httpmock.NewStringResponder(200, mockResponse))

	result, err := hgc.getGasPriceFromGasOracle(ctx)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Failed to parse template result as PublicTxGasPricing JSON")
}

func TestGetGasPriceFromGasOracleWithPOSTMethod(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			Method: confutil.P("POST"),
			Body: confutil.P(`{
				"jsonrpc": "2.0",
				"method": "custom_feeHistory",
				"params": [20, "latest", [25, 50, 75]],
				"id": 1
			}`),
			ResponseTemplate: `{
				"maxFeePerGas": "{{.result.maxFeePerGas}}",
				"maxPriorityFeePerGas": "{{.result.maxPriorityFeePerGas}}"
			}`,
		},
	}

	ctx, hgc, _ := NewTestGasPriceClient(t, conf, false)

	err := hgc.Init(ctx)
	require.NoError(t, err)

	// Setup httpmock for the specific client after it's created
	httpmock.ActivateNonDefault(hgc.gasOracleHTTPClient.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.Reset()

	// Mock response for JSON/RPC call
	mockResponse := `{
		"jsonrpc": "2.0",
		"id": 1,
		"result": {
			"maxFeePerGas": "0x2FAF080",
			"maxPriorityFeePerGas": "0x3B9ACA0"
		}
	}`
	httpmock.RegisterResponder("POST", "https://api.example.com/gas",
		httpmock.NewStringResponder(200, mockResponse))

	result, err := hgc.getGasPriceFromGasOracle(ctx)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the gas price values
	assert.Equal(t, pldtypes.Uint64ToUint256(50000000), result.MaxFeePerGas)         // 0x2FAF080
	assert.Equal(t, pldtypes.Uint64ToUint256(62500000), result.MaxPriorityFeePerGas) // 0x3B9ACA0
}

func TestInitWithGasOracleInvalidMethod(t *testing.T) {
	conf := &pldconf.GasPriceConfig{
		FixedGasPrice: nil,
		GasOracleAPI: &pldconf.GasOracleAPIConfig{
			HTTPClientConfig: pldconf.HTTPClientConfig{
				URL: "https://api.example.com/gas",
			},
			Method: confutil.P("DELETE"), // Unsupported method
			ResponseTemplate: `{
				"maxFeePerGas": "{{.maxFeePerGas}}",
				"maxPriorityFeePerGas": "{{.maxPriorityFeePerGas}}"
			}`,
		},
	}

	ctx, hgc, _ := NewTestGasPriceClient(t, conf, false)

	err := hgc.Init(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid HTTP method for gas oracle API: DELETE")
}
