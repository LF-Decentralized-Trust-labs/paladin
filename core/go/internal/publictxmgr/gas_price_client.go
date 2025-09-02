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
	GetGasPriceObject(ctx context.Context, txFixedGasPrice *pldapi.PublicTxGasPricing, previouslySubmittedGPO *pldapi.PublicTxGasPricing, underpriced bool) (gasPrice *pldapi.PublicTxGasPricing, err error)
	DeleteCache(ctx context.Context)
	Init(ctx context.Context) error
	Start(ctx context.Context, ethClient ethclient.EthClient)
}

// The hybrid gas price client handles fixed gas pricing from configuration or on a transaction
// as well as dynamic gas pricing using eth_feeHistory.

// An important implementation details for the functions on this struct is that if they are passed a
// pointer to a gas pricing struct, they never change the original struct, but instead return a new one.
// This means that is is safe to pass "fixed" prices to it without them being unintentionally changed
// for future transactions.
type HybridGasPriceClient struct {
	hasZeroGasPrice bool
	fixedGasPrice   *pldapi.PublicTxGasPricing
	ethClient       ethclient.EthClient

	conf *pldconf.GasPriceConfig

	maxPriorityFeePerGasCap *pldtypes.HexUint256
	maxFeePerGasCap         *pldtypes.HexUint256

	gasPriceIncreasePercent int

	// Eth fee history gas pricing configuration (always set with defaults so this works as a fallback option)
	priorityFeePercentile     int
	historyBlockCount         int
	baseFeeBufferFactor       int
	ethFeeHistoryCacheEnabled bool

	// Cache for eth fee history gas pricing results
	ethFeeHistoryGasPriceCache cache.Cache[string, *pldapi.PublicTxGasPricing]
}

func (hGpc *HybridGasPriceClient) HasZeroGasPrice(ctx context.Context) bool {
	return hGpc.hasZeroGasPrice
}

// estimateEIP1559Fees calculates optimal maxFeePerGas and maxPriorityFeePerGas using eth_feeHistory
func (hGpc *HybridGasPriceClient) estimateEIP1559Fees(ctx context.Context) (*pldapi.PublicTxGasPricing, error) {
	// Check if we have valid cached results
	if hGpc.ethFeeHistoryGasPriceCache != nil {
		if cached, found := hGpc.ethFeeHistoryGasPriceCache.Get("eth_feeHistory_gas_pricing"); found {
			return cached, nil
		}
	}

	// Prepare reward percentiles for the RPC call
	rewardPercentiles := []float64{float64(hGpc.priorityFeePercentile)}

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
	if hGpc.maxPriorityFeePerGasCap != nil {
		cap := hGpc.maxPriorityFeePerGasCap.Int()
		if maxPriorityFeePerGas.Int().Cmp(cap) > 0 {
			maxPriorityFeePerGas = (*pldtypes.HexUint256)(cap)
			log.L(ctx).Warnf("Capped maxPriorityFeePerGas to %s", hGpc.maxPriorityFeePerGasCap.HexString0xPrefix())
		}
	}

	// Calculate maxFeePerGas (the total bid)

	// Get the next block's base fee (last element in the array)
	// When the cache is enabled, this base fee will be used for all subsequent transactions until DeleteCache is called.
	// This is fine for chains where the base fee stays relatively stable since the baseFeeBufferFactor gives room for
	// potential increases, but for chains where the base fee is volatile it would be better to disable the cache.
	// Ideally we would have a cache with a configurable TTL or which self refreshed after every N blocks
	nextBlockBaseFee := feeHistory.BaseFeePerGas[len(feeHistory.BaseFeePerGas)-1].Int()

	// Create a buffer by multiplying the base fee by the configured factor to handle potential increases
	bufferedBaseFee := new(big.Int).Mul(nextBlockBaseFee, big.NewInt(int64(hGpc.baseFeeBufferFactor)))

	// maxFeePerGas = bufferedBaseFee + maxPriorityFeePerGas
	maxFeePerGas := (*pldtypes.HexUint256)(new(big.Int).Add(bufferedBaseFee, maxPriorityFeePerGas.Int()))

	if hGpc.maxFeePerGasCap != nil {
		cap := hGpc.maxFeePerGasCap.Int()
		if maxFeePerGas.Int().Cmp(cap) > 0 {
			maxFeePerGas = (*pldtypes.HexUint256)(cap)
			log.L(ctx).Warnf("Capped maxFeePerGas to %s", hGpc.maxPriorityFeePerGasCap.HexString0xPrefix())
		}
	}

	result := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         maxFeePerGas,
		MaxPriorityFeePerGas: maxPriorityFeePerGas,
	}

	// Cache the results if caching is enabled
	if hGpc.ethFeeHistoryCacheEnabled {
		// Store in cache
		hGpc.ethFeeHistoryGasPriceCache.Set("eth_feeHistory_gas_pricing", result)
	}

	return result, nil
}

func (hGpc *HybridGasPriceClient) increaseGasPricingByPercentage(gasPricing *pldapi.PublicTxGasPricing, percentage int) *pldapi.PublicTxGasPricing {
	// Calculate the multiplier (e.g., 1.10 for 10% increase)
	// But if we simply express this multiplier as a float, we run into precision issues, so we need to take
	// a different approach.
	// Instead we're going to keep everything as integers, express the multiplier as 100 + percentage, then divide by 100.
	// However, integer division will return the quotient, which could result in a lower increase than expected.
	// To avoid this we will express our division as (x + y - 1) / y, instead of x / y, which results in the division rounding up.
	// Concretely, this means that after the multiplication, we will add 99 to the result before diving by 100
	multiplier := big.NewInt(int64(100 + percentage))
	// returning a new struct so that we don't change the original
	result := &pldapi.PublicTxGasPricing{}

	// Increase MaxFeePerGas if present
	if gasPricing.MaxFeePerGas != nil {
		// new(big.Int) so that we don't change the original
		increasedValue := new(big.Int).Mul(gasPricing.MaxFeePerGas.Int(), multiplier)
		increasedValue.Add(increasedValue, big.NewInt(99))
		increasedValue.Div(increasedValue, big.NewInt(100))
		result.MaxFeePerGas = (*pldtypes.HexUint256)(increasedValue)
	}

	// Increase MaxPriorityFeePerGas if present
	if gasPricing.MaxPriorityFeePerGas != nil {
		// new(big.Int) so that we don't change the original
		increasedValue := new(big.Int).Mul(gasPricing.MaxPriorityFeePerGas.Int(), multiplier)
		increasedValue.Add(increasedValue, big.NewInt(99))
		increasedValue.Div(increasedValue, big.NewInt(100))
		result.MaxPriorityFeePerGas = (*pldtypes.HexUint256)(increasedValue)
	}

	return result
}

func (hGpc *HybridGasPriceClient) capGasPricing(ctx context.Context, gasPricing *pldapi.PublicTxGasPricing) *pldapi.PublicTxGasPricing {
	result := &pldapi.PublicTxGasPricing{}

	// Cap MaxFeePerGas if cap is provided and value exceeds it
	if gasPricing.MaxFeePerGas != nil {
		if hGpc.maxFeePerGasCap != nil && gasPricing.MaxFeePerGas.Int().Cmp(hGpc.maxFeePerGasCap.Int()) > 0 {
			// Value exceeds cap, use cap value
			log.L(ctx).Warnf("Capping MaxFeePerGas to %s", hGpc.maxFeePerGasCap.HexString0xPrefix())
			result.MaxFeePerGas = hGpc.maxFeePerGasCap
		} else {
			// Value is within cap or no cap provided, use original value
			result.MaxFeePerGas = gasPricing.MaxFeePerGas
		}
	}

	// Cap MaxPriorityFeePerGas if cap is provided and value exceeds it
	if gasPricing.MaxPriorityFeePerGas != nil {
		if hGpc.maxPriorityFeePerGasCap != nil && gasPricing.MaxPriorityFeePerGas.Int().Cmp(hGpc.maxPriorityFeePerGasCap.Int()) > 0 {
			// Value exceeds cap, use cap value
			log.L(ctx).Warnf("Capping MaxPriorityFeePerGas to %s", hGpc.maxPriorityFeePerGasCap.HexString0xPrefix())
			result.MaxPriorityFeePerGas = hGpc.maxPriorityFeePerGasCap
		} else {
			// Value is within cap or no cap provided, use original value
			result.MaxPriorityFeePerGas = gasPricing.MaxPriorityFeePerGas
		}
	}

	return result
}

// calculateNewGasPrice ensures that if the gas price is always
// * at least the previous gas price
// * if it has been increased that it has increased by at least the configured percentage
// * if the transaction was previously underpriced, that it has increased by at least the configured percentage
//
// The comparisons to do this are complicated by the fact that we are actually comparing two figures and need to ensure that
// if one has increased from the previous amount, then what we use on the next submission represents the minimum percentage
// increase on both values.
func (hGpc *HybridGasPriceClient) calculateNewGasPrice(previouslySubmittedGPO *pldapi.PublicTxGasPricing, retrievedGPO *pldapi.PublicTxGasPricing, underpriced bool) *pldapi.PublicTxGasPricing {
	// if we've never submitted a gas price for this transaction then we can just return the retrieved gas price as is
	if previouslySubmittedGPO == nil || previouslySubmittedGPO.MaxFeePerGas == nil || previouslySubmittedGPO.MaxPriorityFeePerGas == nil {
		return retrievedGPO
	}

	// compare priority fee
	priorityFeeCmp := retrievedGPO.MaxPriorityFeePerGas.Int().Cmp(previouslySubmittedGPO.MaxPriorityFeePerGas.Int())
	// compare total fee
	totalFeeCmp := retrievedGPO.MaxFeePerGas.Int().Cmp(previouslySubmittedGPO.MaxFeePerGas.Int())

	// If either maxPriorityFeePerGas and maxFeePerGas have increased we use whichever is higher of the retrieved values, or
	// the minimum percentage increase on the previous gas price.
	// Otherwise we use the previous values, but increased by the configured percentage if we were previously underpriced
	if priorityFeeCmp == 1 || totalFeeCmp == 1 {
		gpo := &pldapi.PublicTxGasPricing{}
		minNewGPO := hGpc.increaseGasPricingByPercentage(previouslySubmittedGPO, hGpc.gasPriceIncreasePercent)

		if new(big.Int).Sub(retrievedGPO.MaxPriorityFeePerGas.Int(), minNewGPO.MaxPriorityFeePerGas.Int()).Sign() == -1 {
			gpo.MaxPriorityFeePerGas = minNewGPO.MaxPriorityFeePerGas
		} else {
			gpo.MaxPriorityFeePerGas = retrievedGPO.MaxPriorityFeePerGas
		}

		if new(big.Int).Sub(retrievedGPO.MaxFeePerGas.Int(), minNewGPO.MaxFeePerGas.Int()).Sign() == -1 {
			gpo.MaxFeePerGas = minNewGPO.MaxFeePerGas
		} else {
			gpo.MaxFeePerGas = retrievedGPO.MaxFeePerGas
		}
		return gpo
	}

	if underpriced {
		return hGpc.increaseGasPricingByPercentage(previouslySubmittedGPO, hGpc.gasPriceIncreasePercent)
	}

	return previouslySubmittedGPO
}

func (hGpc *HybridGasPriceClient) GetGasPriceObject(ctx context.Context, txFixedGasPrice *pldapi.PublicTxGasPricing, previouslySubmittedGPO *pldapi.PublicTxGasPricing, underpriced bool) (gasPrice *pldapi.PublicTxGasPricing, err error) {
	// priority order for retrieving a gas price object:
	// 1. zero gas price chain
	// 2. transaction fixed gas price
	// 3. fixed gas price
	// 4. estimate EIP-1559 fees
	if hGpc.hasZeroGasPrice {
		// if zero gas price chain, return zero gas price without any kind of retrieval/estimation
		// there's no validation that we can do on a zero gas price chain so just return right away
		return &pldapi.PublicTxGasPricing{
			MaxFeePerGas:         pldtypes.Uint64ToUint256(0),
			MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(0),
		}, nil
	}

	if txFixedGasPrice != nil {
		// A fixed gas price on the transaction cannot be incremented in the case of being underpriced-
		// resolving this is the responsibility of the user submitting the transaction.
		// The price cap does need to apply though.
		return hGpc.capGasPricing(ctx, txFixedGasPrice), nil
	}

	var gpo *pldapi.PublicTxGasPricing

	if hGpc.fixedGasPrice != nil {
		gpo = hGpc.fixedGasPrice
	} else {
		if underpriced {
			hGpc.DeleteCache(ctx)
		}
		var err error
		gpo, err = hGpc.estimateEIP1559Fees(ctx)
		if err != nil {
			return nil, err
		}
	}

	gpo = hGpc.calculateNewGasPrice(previouslySubmittedGPO, gpo, underpriced)

	// Finally cap the fees to the configured maximums if the new gas price exceeds them
	// This could technically result in an increment that is less than the accepted replacement percentage on chain
	// but at this point we're pretty limited on options- it's a question of which failure, rather than avoiding failure
	//
	// estimateEip1559Fees might already have applied the cap, because estimating maxFeePerGas off a value of
	// maxPriorityFeePerGas that is over the cap could result in an excess estimation of how much gas will be required
	// for the transaction. We still have to reapply it here because of the logic in calculateNewGasPrice that
	// works out if a percentage increase is needed may have raised the prices above the cap again.
	return hGpc.capGasPricing(ctx, gpo), nil
}

func (hGpc *HybridGasPriceClient) DeleteCache(ctx context.Context) {
	if hGpc.ethFeeHistoryGasPriceCache != nil {
		hGpc.ethFeeHistoryGasPriceCache.Delete("eth_feeHistory_gas_pricing")
	}
}

func (hGpc *HybridGasPriceClient) Init(ctx context.Context) error {
	if hGpc.conf.EthFeeHistory.PriorityFeePercentile != nil &&
		(*hGpc.conf.EthFeeHistory.PriorityFeePercentile < 0 || *hGpc.conf.EthFeeHistory.PriorityFeePercentile > 100) {
		errMsg := fmt.Sprintf("Invalid priority fee percentile: %d. Must be between 0 and 100", hGpc.priorityFeePercentile)
		log.L(ctx).Error(errMsg)
		return errors.New(errMsg)
	}

	hGpc.priorityFeePercentile = confutil.Int(hGpc.conf.EthFeeHistory.PriorityFeePercentile, *pldconf.PublicTxManagerDefaults.GasPrice.EthFeeHistory.PriorityFeePercentile)
	hGpc.historyBlockCount = confutil.Int(hGpc.conf.EthFeeHistory.HistoryBlockCount, *pldconf.PublicTxManagerDefaults.GasPrice.EthFeeHistory.HistoryBlockCount)
	hGpc.maxPriorityFeePerGasCap = hGpc.conf.MaxPriorityFeePerGasCap
	hGpc.maxFeePerGasCap = hGpc.conf.MaxFeePerGasCap
	hGpc.baseFeeBufferFactor = confutil.Int(hGpc.conf.EthFeeHistory.BaseFeeBufferFactor, *pldconf.PublicTxManagerDefaults.GasPrice.EthFeeHistory.BaseFeeBufferFactor)
	hGpc.gasPriceIncreasePercent = confutil.Int(hGpc.conf.IncreasePercentage, *pldconf.PublicTxManagerDefaults.GasPrice.IncreasePercentage)
	hGpc.ethFeeHistoryCacheEnabled = confutil.Bool(hGpc.conf.EthFeeHistory.Cache.Enabled, *pldconf.PublicTxManagerDefaults.GasPrice.EthFeeHistory.Cache.Enabled)

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

	if hGpc.ethFeeHistoryCacheEnabled {
		hardcodedCacheConfig := &pldconf.CacheConfig{Capacity: confutil.P(1)} // we only cache one result so hardcode the capacity
		hGpc.ethFeeHistoryGasPriceCache = cache.NewCache[string, *pldapi.PublicTxGasPricing](hardcodedCacheConfig, hardcodedCacheConfig)
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
			errMsg := "fixed gas pricing configuration incomplete: maxFeePerGas is set but maxPriorityFeePerGas is missing- ignoring maxFeePerGas"
			log.L(ctx).Error(errMsg)
			return nil, errors.New(errMsg)
		}
		if config.MaxPriorityFeePerGas != nil {
			errMsg := "fixed gas pricing configuration incomplete: maxPriorityFeePerGas is set but maxFeePerGas is missing- ignoring maxPriorityFeePerGas"
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
