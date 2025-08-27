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
	"fmt"
	"math/big"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/cache"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/ethclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

type GasPriceClient interface {
	HasZeroGasPrice(ctx context.Context) bool
	GetGasPriceObject(ctx context.Context) (gasPrice *pldapi.PublicTxGasPricing, err error)
	DeleteCache(ctx context.Context)
	Init(ctx context.Context) error
	Start(ctx context.Context, ethClient ethclient.EthClient)
}

// The hybrid gas price client handles fixed gas pricing from configuration
type HybridGasPriceClient struct {
	hasZeroGasPrice bool
	fixedGasPrice   *pldapi.PublicTxGasPricing
	ethClient       ethclient.EthClient

	conf *pldconf.GasPriceConfig
	// Dynamic gas pricing configuration (always set with defaults)
	percentile          int
	historyBlockCount   int
	maxPriorityFeeCap   *pldtypes.HexUint256
	baseFeeBufferFactor int
	cacheEnabled        bool

	// Cache for dynamic gas pricing results
	dynamicGasPriceCache cache.Cache[string, *pldapi.PublicTxGasPricing]
}

func (hGpc *HybridGasPriceClient) HasZeroGasPrice(ctx context.Context) bool {
	return hGpc.hasZeroGasPrice
}

// estimateEip1559Fees calculates optimal maxFeePerGas and maxPriorityFeePerGas using eth_feeHistory
func (hGpc *HybridGasPriceClient) estimateEip1559Fees(ctx context.Context) (*pldapi.PublicTxGasPricing, error) {
	// Check if we have valid cached results
	if hGpc.dynamicGasPriceCache != nil {
		if cached, found := hGpc.dynamicGasPriceCache.Get("dynamic_gas_pricing"); found {
			return cached, nil
		}
	}

	// Prepare reward percentiles for the RPC call
	rewardPercentiles := []float64{float64(hGpc.percentile)}

	// Fetch fee history
	feeHistory, err := hGpc.ethClient.FeeHistory(ctx, hGpc.historyBlockCount, "latest", rewardPercentiles)
	if err != nil {
		log.L(ctx).Errorf("Failed to fetch fee history: %+v", err)
		return nil, err
	}

	if len(feeHistory.BaseFeePerGas) == 0 || len(feeHistory.Reward) == 0 {
		errMsg := fmt.Sprintf("fee history returned empty data: BaseFeePerGas=%d, Reward=%d",
			len(feeHistory.BaseFeePerGas), len(feeHistory.Reward))
		log.L(ctx).Error(errMsg)
		return nil, errors.New(errMsg)
	}

	// Calculate maxPriorityFeePerGas (the tip)
	var maxPriorityFeePerGas *pldtypes.HexUint256

	// Extract tips for the specified percentile
	tips := make([]*big.Int, 0, len(feeHistory.Reward))
	for _, blockRewards := range feeHistory.Reward {
		if len(blockRewards) > 0 {
			tip := blockRewards[0].Int() // We only requested one percentile
			if tip.Sign() > 0 {          // Filter out zero tips
				tips = append(tips, tip)
			}
		}
	}

	if len(tips) == 0 {
		// Fallback to 1 Gwei if no valid tips found
		maxPriorityFeePerGas = (*pldtypes.HexUint256)(big.NewInt(1000000000)) // 1 Gwei in Wei
		log.L(ctx).Warnf("No valid tips found in fee history, using fallback: 1 Gwei")
	} else {
		// Find the highest tip for robustness
		maxPriorityFeePerGas = (*pldtypes.HexUint256)(tips[0])
		for _, tip := range tips[1:] {
			if tip.Cmp(maxPriorityFeePerGas.Int()) > 0 {
				maxPriorityFeePerGas = (*pldtypes.HexUint256)(tip)
			}
		}
	}

	// Apply optional cap if configured (applies to both historical tips and fallback)
	if hGpc.maxPriorityFeeCap != nil {
		capInWei := hGpc.maxPriorityFeeCap.Int() // Already in Wei
		if maxPriorityFeePerGas.Int().Cmp(capInWei) > 0 {
			maxPriorityFeePerGas = (*pldtypes.HexUint256)(capInWei)
			log.L(ctx).Debugf("Capped maxPriorityFeePerGas to %s", hGpc.maxPriorityFeeCap.HexString0xPrefix())
		}
	}

	// Calculate maxFeePerGas (the total bid)

	// Get the next block's base fee (last element in the array)
	// When the cache is enabled, this base fee will be used for all subsequent transactions until DeleteCache is called.
	// This is fine for chains where the base fee stays relatively stable since the baseFeeBufferFactor gives room for
	// potential increases, but for chains where the base fee is volatile it would be better to disable the cache.
	// Ideally we would have a cache with a configurable TTL.
	nextBlockBaseFee := feeHistory.BaseFeePerGas[len(feeHistory.BaseFeePerGas)-1].Int()

	// Create a buffer by multiplying the base fee by the configured factor to handle potential increases
	bufferedBaseFee := new(big.Int).Mul(nextBlockBaseFee, big.NewInt(int64(hGpc.baseFeeBufferFactor)))

	// maxFeePerGas = bufferedBaseFee + maxPriorityFeePerGas
	maxFeePerGas := (*pldtypes.HexUint256)(new(big.Int).Add(bufferedBaseFee, maxPriorityFeePerGas.Int()))

	result := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         maxFeePerGas,
		MaxPriorityFeePerGas: maxPriorityFeePerGas,
	}

	// Cache the results if caching is enabled
	if hGpc.cacheEnabled {
		// Store in cache
		hGpc.dynamicGasPriceCache.Set("dynamic_gas_pricing", result)
	}

	return result, nil
}

func (hGpc *HybridGasPriceClient) GetGasPriceObject(ctx context.Context) (gasPrice *pldapi.PublicTxGasPricing, err error) {
	// if zero gas price chain, return zero gas price
	if hGpc.hasZeroGasPrice {
		return &pldapi.PublicTxGasPricing{
			MaxFeePerGas:         (*pldtypes.HexUint256)(big.NewInt(0)),
			MaxPriorityFeePerGas: (*pldtypes.HexUint256)(big.NewInt(0)),
		}, nil
	}
	// First, try fixed gas pricing if available
	if hGpc.fixedGasPrice != nil {
		return hGpc.fixedGasPrice, nil
	}
	return hGpc.estimateEip1559Fees(ctx)
}

func (hGpc *HybridGasPriceClient) DeleteCache(ctx context.Context) {
	if hGpc.dynamicGasPriceCache != nil {
		hGpc.dynamicGasPriceCache.Delete("dynamic_gas_pricing")
	}
}

func (hGpc *HybridGasPriceClient) Init(ctx context.Context) error {
	if hGpc.conf.DynamicGasPricing.Percentile != nil &&
		(*hGpc.conf.DynamicGasPricing.Percentile < 0 || *hGpc.conf.DynamicGasPricing.Percentile > 100) {
		errMsg := fmt.Sprintf("Invalid dynamic gas pricing percentile: %d. Must be between 0 and 100", hGpc.percentile)
		log.L(ctx).Error(errMsg)
		return errors.New(errMsg)
	}

	hGpc.percentile = confutil.Int(hGpc.conf.DynamicGasPricing.Percentile, *pldconf.PublicTxManagerDefaults.GasPrice.DynamicGasPricing.Percentile)
	hGpc.historyBlockCount = confutil.Int(hGpc.conf.DynamicGasPricing.HistoryBlockCount, *pldconf.PublicTxManagerDefaults.GasPrice.DynamicGasPricing.HistoryBlockCount)
	hGpc.maxPriorityFeeCap = hGpc.conf.DynamicGasPricing.MaxPriorityFeeCap
	hGpc.baseFeeBufferFactor = confutil.Int(hGpc.conf.DynamicGasPricing.BaseFeeBufferFactor, *pldconf.PublicTxManagerDefaults.GasPrice.DynamicGasPricing.BaseFeeBufferFactor)
	hGpc.cacheEnabled = confutil.Bool(hGpc.conf.DynamicGasPricing.Cache.Enabled, *pldconf.PublicTxManagerDefaults.GasPrice.DynamicGasPricing.Cache.Enabled)

	if hGpc.conf.FixedGasPrice != nil {
		fixedGasPrice, err := mapConfigToAPIGasPricing(ctx, hGpc.conf.FixedGasPrice)
		if err != nil {
			return err
		}
		hGpc.fixedGasPrice = fixedGasPrice
		if (hGpc.fixedGasPrice.MaxFeePerGas != nil && hGpc.fixedGasPrice.MaxFeePerGas.Int().Sign() == 0) &&
			(hGpc.fixedGasPrice.MaxPriorityFeePerGas != nil && hGpc.fixedGasPrice.MaxPriorityFeePerGas.Int().Sign() == 0) {
			hGpc.hasZeroGasPrice = true
		}
	}

	if hGpc.cacheEnabled {
		hardcodedCacheConfig := &pldconf.CacheConfig{Capacity: confutil.P(1)} // we only cache one result so hardcode the capacity
		hGpc.dynamicGasPriceCache = cache.NewCache[string, *pldapi.PublicTxGasPricing](hardcodedCacheConfig, hardcodedCacheConfig)
	}
	return nil
}

func (hGpc *HybridGasPriceClient) Start(ctx context.Context, ethClient ethclient.EthClient) {
	hGpc.ethClient = ethClient

	// If we haven't already been told a fixed gas price, check whether it's a zero gas price chain
	// Although we cant use GasPrice for effective EIP-1559 dynamic gas pricing, we can still trust that if
	// the response from this call is zero then the chain has zero gas price
	if hGpc.fixedGasPrice == nil {
		gasPrice, err := ethClient.GasPrice(ctx)
		if err == nil && gasPrice != nil {
			if gasPrice.Int().Sign() == 0 {
				hGpc.hasZeroGasPrice = true
				log.L(ctx).Debugf("Detected zero gas price chain from eth_gasPrice")
			}
		} else if err != nil {
			log.L(ctx).Warnf("Could not determine gas price from eth_gasPrice: %v", err)
		}
	}
}

func NewGasPriceClient(ctx context.Context, conf *pldconf.GasPriceConfig) GasPriceClient {
	return &HybridGasPriceClient{
		conf: conf,
	}
}

// mapConfigToAPIGasPricing converts configuration types to API types
func mapConfigToAPIGasPricing(ctx context.Context, config *pldconf.FixedGasPricing) (*pldapi.PublicTxGasPricing, error) {
	// Both fields must be set for valid fixed gas pricing
	if config.MaxFeePerGas == nil || config.MaxPriorityFeePerGas == nil {
		if config.MaxFeePerGas != nil {
			errMsg := "Fixed gas pricing configuration incomplete: maxFeePerGas is set but maxPriorityFeePerGas is missing. Ignoring maxFeePerGas."
			log.L(ctx).Error(errMsg)
			return nil, errors.New(errMsg)
		}
		if config.MaxPriorityFeePerGas != nil {
			errMsg := "Fixed gas pricing configuration incomplete: maxPriorityFeePerGas is set but maxFeePerGas is missing. Ignoring maxPriorityFeePerGas."
			log.L(ctx).Error(errMsg)
			return nil, errors.New(errMsg)
		}
	}

	// Both fields are set, create valid API gas pricing object
	return &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         config.MaxFeePerGas,
		MaxPriorityFeePerGas: config.MaxPriorityFeePerGas,
	}, nil
}
