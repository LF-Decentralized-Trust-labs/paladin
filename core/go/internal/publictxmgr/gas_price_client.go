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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"sync"
	"text/template"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/pkg/ethclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldresty"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/cache"
	"github.com/go-resty/resty/v2"
)

type GasPriceClient interface {
	HasZeroGasPrice(ctx context.Context) bool
	GetGasPriceObject(ctx context.Context, txFixedGasPrice *pldapi.PublicTxGasPricing, previouslySubmittedGPO *pldapi.PublicTxGasPricing, underpriced bool) (gasPrice *pldapi.PublicTxGasPricing, err error)
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

	// Gas oracle HTTP client for external gas price retrieval
	gasOracleHTTPClient *resty.Client
	gasOracleTemplate   *template.Template
	gasOracleMethod     string
	gasOracleBody       *string

	// Eth fee history gas pricing configuration (always set with defaults so this works as a fallback option)
	priorityFeePercentile int
	historyBlockCount     int
	baseFeeBufferFactor   int

	// Shared cache for gas price data
	gasPriceCache         cache.Cache[string, *pldapi.PublicTxGasPricing]
	gasPriceRefreshTicker *time.Ticker
	refreshTime           time.Duration
	cacheMux              sync.RWMutex
}

func (hGpc *HybridGasPriceClient) HasZeroGasPrice(ctx context.Context) bool {
	return hGpc.hasZeroGasPrice
}

// estimateEIP1559Fees calculates optimal maxFeePerGas and maxPriorityFeePerGas using eth_feeHistory
func (hGpc *HybridGasPriceClient) estimateEIP1559Fees(ctx context.Context) (*pldapi.PublicTxGasPricing, error) {
	// Prepare reward percentiles for the RPC call
	rewardPercentiles := []float64{float64(hGpc.priorityFeePercentile)}

	// Fetch fee history
	feeHistory, err := hGpc.ethClient.FeeHistory(ctx, hGpc.historyBlockCount, "latest", rewardPercentiles)
	if err != nil {
		log.L(ctx).Errorf("Failed to fetch fee history: %+v", err)
		return nil, i18n.NewError(ctx, msgs.MsgPublicTxMgrFeeHistoryCallFailed, err)
	}

	if len(feeHistory.BaseFeePerGas) == 0 || len(feeHistory.Reward) == 0 {
		log.L(ctx).Errorf("Fee history returned empty data: len(baseFeePerGas)=%d, len(reward)=%d",
			len(feeHistory.BaseFeePerGas), len(feeHistory.Reward))
		return nil, i18n.NewError(ctx, msgs.MsgPublicTxMgrFeeHistoryEmpty, len(feeHistory.BaseFeePerGas), len(feeHistory.Reward))
	}

	// Calculate maxPriorityFeePerGas (the tip)
	var maxPriorityFeePerGas *pldtypes.HexUint256

	// Extract tips for the specified percentile
	tips := make([]*big.Int, 0, len(feeHistory.Reward))
	for _, blockRewards := range feeHistory.Reward {
		if len(blockRewards) > 0 {
			tips = append(tips, blockRewards[0].Int())
		}
	}

	if len(tips) == 0 {
		// This is a failure in the eth_feeHistory RPC response if the tip details we've requested
		// are not included in the response. There's not much we can do about it, so we'll return an error
		// which will cause this stage to be retried until it does succeed.
		errMsg := "no valid tips found in fee history"
		log.L(ctx).Error(errMsg)
		return nil, errors.New(errMsg)
	} else {
		// Find the highest tip for robustness
		maxPriorityFeePerGas = (*pldtypes.HexUint256)(tips[0])
		for _, tip := range tips[1:] {
			if tip.Cmp(maxPriorityFeePerGas.Int()) > 0 {
				maxPriorityFeePerGas = (*pldtypes.HexUint256)(tip)
			}
		}
	}

	// Calculate maxFeePerGas (the total bid)
	// Get the next block's base fee (last element in the array) then create a buffer by multiplying the base fee by
	// the configured factor to handle potential increases.
	nextBlockBaseFee := feeHistory.BaseFeePerGas[len(feeHistory.BaseFeePerGas)-1].Int()
	bufferedBaseFee := new(big.Int).Mul(nextBlockBaseFee, big.NewInt(int64(hGpc.baseFeeBufferFactor)))
	maxFeePerGas := (*pldtypes.HexUint256)(new(big.Int).Add(bufferedBaseFee, maxPriorityFeePerGas.Int()))

	result := &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         maxFeePerGas,
		MaxPriorityFeePerGas: maxPriorityFeePerGas,
	}
	return result, nil
}

func (hGpc *HybridGasPriceClient) getGasPriceFromGasOracle(ctx context.Context) (*pldapi.PublicTxGasPricing, error) {
	// Make HTTP request to the gas oracle API
	req := hGpc.gasOracleHTTPClient.R().SetContext(ctx)

	// Set body for methods that support it
	if hGpc.gasOracleMethod != "GET" && hGpc.gasOracleBody != nil {
		req = req.SetBody(*hGpc.gasOracleBody)
	}

	// Execute the request using the configured method
	resp, err := req.Execute(hGpc.gasOracleMethod, "")
	if err != nil {
		log.L(ctx).Errorf("Failed to call gas oracle API: %+v", err)
		return nil, i18n.NewError(ctx, msgs.MsgPublicTxMgrGasOracleAPICallFailed, err)
	}

	if !resp.IsSuccess() {
		log.L(ctx).Errorf("Gas oracle API returned error status: %d %s", resp.StatusCode(), resp.String())
		return nil, i18n.NewError(ctx, msgs.MsgPublicTxMgrGasOracleAPIErrorStatus, resp.StatusCode(), resp.String())
	}

	// Parse the response as JSON
	var responseData map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &responseData); err != nil {
		log.L(ctx).Errorf("Failed to parse gas oracle API response as JSON: %+v", err)
		return nil, i18n.NewError(ctx, msgs.MsgPublicTxMgrGasOracleResponseParseFailed, err)
	}

	// Apply the template to extract gas price data - it should be impossible to hit this error because
	// we've already validated the template in Init, but it's safest to still handle it here.
	var templateResult bytes.Buffer
	if templateErr := hGpc.gasOracleTemplate.Execute(&templateResult, responseData); templateErr != nil {
		log.L(ctx).Errorf("Failed to execute gas oracle template: %+v", templateErr)
		return nil, i18n.NewError(ctx, msgs.MsgPublicTxMgrGasOracleTemplateExecuteFailed, templateErr)
	}

	// Parse the template result directly into PublicTxGasPricing
	var gasPriceData pldapi.PublicTxGasPricing
	if templateErr := json.Unmarshal(templateResult.Bytes(), &gasPriceData); templateErr != nil {
		log.L(ctx).Errorf("Failed to parse template result as PublicTxGasPricing JSON: %+v", templateErr)
		return nil, i18n.NewError(ctx, msgs.MsgPublicTxMgrGasOracleTemplateResultParseFailed, templateErr)
	}

	// Validate that we have the required fields
	if gasPriceData.MaxFeePerGas == nil {
		return nil, i18n.NewError(ctx, msgs.MsgPublicTxMgrGasOracleMaxFeePerGasMissing)
	}

	if gasPriceData.MaxPriorityFeePerGas == nil {
		return nil, i18n.NewError(ctx, msgs.MsgPublicTxMgrGasOracleMaxPriorityFeePerGasMissing)
	}

	return &gasPriceData, nil
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
	// 4. cached gas price
	// 5. gas oracle api
	// 6. estimate EIP-1559 fees using eth_feeHistory
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
	} else if found, cached := hGpc.getCachedGasPrice(ctx); found {
		gpo = cached
	} else {
		// Get fresh data and cache it
		var err error
		if hGpc.gasOracleHTTPClient != nil {
			gpo, err = hGpc.getGasPriceFromGasOracle(ctx)
		} else {
			gpo, err = hGpc.estimateEIP1559Fees(ctx)
		}
		if err != nil {
			return nil, err
		}

		// Cache the result
		hGpc.setCachedGasPrice(gpo)
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

func (hGpc *HybridGasPriceClient) Init(ctx context.Context) error {
	// config that is relevant to all gas price retrieval methods
	if hGpc.conf.MaxPriorityFeePerGasCap != nil {
		maxPriorityFeePerGasCap, err := pldtypes.ParseHexUint256(ctx, *hGpc.conf.MaxPriorityFeePerGasCap)
		if err != nil {
			return err
		}
		hGpc.maxPriorityFeePerGasCap = maxPriorityFeePerGasCap
	}

	if hGpc.conf.MaxFeePerGasCap != nil {
		maxFeePerGasCap, err := pldtypes.ParseHexUint256(ctx, *hGpc.conf.MaxFeePerGasCap)
		if err != nil {
			return err
		}
		hGpc.maxFeePerGasCap = maxFeePerGasCap
	}

	hGpc.gasPriceIncreasePercent = confutil.Int(hGpc.conf.IncreasePercentage, *pldconf.PublicTxManagerDefaults.GasPrice.IncreasePercentage)

	// config that is specific to each gas price retrieval method

	// fixed gas price config takes precendence
	if hGpc.conf.FixedGasPrice != nil {
		fixedGasPrice, err := mapConfigToAPIGasPricing(ctx, hGpc.conf.FixedGasPrice)
		if err != nil {
			return err
		}
		// it will be nil if fixed gas price was set to an empty object in config- we consider this to be the same as not set
		if fixedGasPrice != nil {
			hGpc.fixedGasPrice = fixedGasPrice
			if (hGpc.fixedGasPrice.MaxFeePerGas != nil && hGpc.fixedGasPrice.MaxFeePerGas.Int().Sign() == 0) &&
				(hGpc.fixedGasPrice.MaxPriorityFeePerGas != nil && hGpc.fixedGasPrice.MaxPriorityFeePerGas.Int().Sign() == 0) {
				hGpc.hasZeroGasPrice = true
			}
			return nil
		}
	}

	// Gas oracle API config comes next in precedence
	if hGpc.conf.GasOracleAPI != nil {
		gasOracleClient, err := pldresty.New(ctx, &hGpc.conf.GasOracleAPI.HTTPClientConfig)
		if err != nil {
			log.L(ctx).Errorf("Failed to initialize gas oracle HTTP client: %+v", err)
			return err
		}
		hGpc.gasOracleHTTPClient = gasOracleClient

		// Set method and body with defaults from configuration
		defaults := pldconf.PublicTxManagerDefaults
		hGpc.gasOracleMethod = confutil.StringOrEmpty(hGpc.conf.GasOracleAPI.Method, *defaults.GasPrice.GasOracleAPI.Method)
		hGpc.gasOracleBody = hGpc.conf.GasOracleAPI.Body

		if hGpc.gasOracleMethod != resty.MethodGet &&
			hGpc.gasOracleMethod != resty.MethodPost &&
			hGpc.gasOracleMethod != resty.MethodPut &&
			hGpc.gasOracleMethod != resty.MethodPatch {
			log.L(ctx).Errorf("Invalid HTTP method: %s", hGpc.gasOracleMethod)
			return i18n.NewError(ctx, msgs.MsgPublicTxMgrGasOracleInvalidMethod, hGpc.gasOracleMethod)
		}

		// Parse the response template and return error if parsing fails
		templateStr := hGpc.conf.GasOracleAPI.ResponseTemplate
		if templateStr == "" {
			log.L(ctx).Error("Gas oracle response template is empty")
			return i18n.NewError(ctx, msgs.MsgPublicTxMgrGasOracleTemplateEmpty)
		}
		hGpc.gasOracleTemplate, err = template.New("gasOracle").Parse(templateStr)
		if err != nil {
			log.L(ctx).Errorf("Failed to parse gas oracle response template: %+v", err)
			return i18n.NewError(ctx, msgs.MsgPublicTxMgrGasOracleTemplateParseFailed, err)
		}

		log.L(ctx).Infof("Initialized gas oracle HTTP client for URL: %s", hGpc.conf.GasOracleAPI.URL)

		// Initialize caches based on configuration
		if err := hGpc.initializeCaches(ctx); err != nil {
			return err
		}

		return nil
	}

	// finally eth_feeHistory estimation is used if no other method is configured
	if hGpc.conf.EthFeeHistory.PriorityFeePercentile != nil &&
		(*hGpc.conf.EthFeeHistory.PriorityFeePercentile < 0 || *hGpc.conf.EthFeeHistory.PriorityFeePercentile > 100) {
		log.L(ctx).Errorf("Invalid priority fee percentile: %d. Must be between 0 and 100", *hGpc.conf.EthFeeHistory.PriorityFeePercentile)
		return i18n.NewError(ctx, msgs.MsgPublicTxMgrInvalidPriorityFeePercentile, *hGpc.conf.EthFeeHistory.PriorityFeePercentile)
	}

	hGpc.priorityFeePercentile = confutil.Int(hGpc.conf.EthFeeHistory.PriorityFeePercentile, *pldconf.PublicTxManagerDefaults.GasPrice.EthFeeHistory.PriorityFeePercentile)
	hGpc.historyBlockCount = confutil.Int(hGpc.conf.EthFeeHistory.HistoryBlockCount, *pldconf.PublicTxManagerDefaults.GasPrice.EthFeeHistory.HistoryBlockCount)
	hGpc.baseFeeBufferFactor = confutil.Int(hGpc.conf.EthFeeHistory.BaseFeeBufferFactor, *pldconf.PublicTxManagerDefaults.GasPrice.EthFeeHistory.BaseFeeBufferFactor)

	// Initialize caches based on configuration
	if err := hGpc.initializeCaches(ctx); err != nil {
		return err
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

	// Start cache refresh ticker if cache is enabled
	if hGpc.gasPriceCache != nil {
		hGpc.startGasPriceRefresh(ctx)
	}
}

func NewGasPriceClient(ctx context.Context, conf *pldconf.GasPriceConfig) GasPriceClient {
	return &HybridGasPriceClient{
		conf: conf,
	}
}

// initializeCaches initializes the shared gas price cache based on configuration
func (hGpc *HybridGasPriceClient) initializeCaches(ctx context.Context) error {
	// Determine which cache configuration to use (gas oracle takes precedence)
	var cacheConfig *pldconf.GasPriceCacheConfig
	var cacheType string

	// Get defaults from configuration
	defaults := pldconf.PublicTxManagerDefaults

	if hGpc.conf.GasOracleAPI != nil && confutil.Bool(hGpc.conf.GasOracleAPI.Cache.Enabled, *defaults.GasPrice.GasOracleAPI.Cache.Enabled) {
		cacheConfig = &hGpc.conf.GasOracleAPI.Cache
		cacheType = "gas oracle"
	} else if confutil.Bool(hGpc.conf.EthFeeHistory.Cache.Enabled, *defaults.GasPrice.EthFeeHistory.Cache.Enabled) {
		cacheConfig = &hGpc.conf.EthFeeHistory.Cache
		cacheType = "eth fee history"
	}

	// Initialize cache if enabled
	if cacheConfig != nil {
		// Parse refresh time using default from configuration
		var defaultRefreshTime string
		if cacheType == "gas oracle" {
			defaultRefreshTime = *defaults.GasPrice.GasOracleAPI.Cache.RefreshTime
		} else {
			defaultRefreshTime = *defaults.GasPrice.EthFeeHistory.Cache.RefreshTime
		}
		refreshTimeStr := confutil.StringOrEmpty(cacheConfig.RefreshTime, defaultRefreshTime)
		refreshTime, err := time.ParseDuration(refreshTimeStr)
		if err != nil {
			log.L(ctx).Errorf("Invalid %s cache refresh time: %s", cacheType, refreshTimeStr)
			return i18n.NewError(ctx, msgs.MsgPublicTxMgrInvalidCacheRefreshTime, refreshTimeStr)
		}
		hGpc.refreshTime = refreshTime

		// Create shared cache with capacity of 1 (only one gas price value)
		cacheConf := &pldconf.CacheConfig{
			Capacity: confutil.P(1),
		}
		hGpc.gasPriceCache = cache.NewCache[string, *pldapi.PublicTxGasPricing](cacheConf, cacheConf)

		log.L(ctx).Infof("Initialized shared gas price cache (%s) with refresh time: %s", cacheType, refreshTimeStr)
	}

	return nil
}

// getCachedGasPrice retrieves gas price from cache if available
// Returns true and the cached gas price if found, false otherwise
func (hGpc *HybridGasPriceClient) getCachedGasPrice(ctx context.Context) (bool, *pldapi.PublicTxGasPricing) {
	if hGpc.gasPriceCache == nil {
		return false, nil
	}

	hGpc.cacheMux.RLock()
	cached, found := hGpc.gasPriceCache.Get("gas_price")
	hGpc.cacheMux.RUnlock()

	if found {
		log.L(ctx).Tracef("Using cached gas price")
		return true, cached
	}

	return false, nil
}

// setCachedGasPrice stores gas price in cache if cache is available
func (hGpc *HybridGasPriceClient) setCachedGasPrice(gasPrice *pldapi.PublicTxGasPricing) {
	if hGpc.gasPriceCache == nil {
		return
	}

	hGpc.cacheMux.Lock()
	hGpc.gasPriceCache.Set("gas_price", gasPrice)
	hGpc.cacheMux.Unlock()
}

// startGasPriceRefresh starts the background refresh goroutine using a ticker
func (hGpc *HybridGasPriceClient) startGasPriceRefresh(ctx context.Context) {
	if hGpc.gasPriceCache == nil {
		return
	}

	// Create ticker for refresh interval
	hGpc.gasPriceRefreshTicker = time.NewTicker(hGpc.refreshTime)

	// Start background goroutine that waits for ticker or context cancellation
	go func() {
		defer hGpc.gasPriceRefreshTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				// Context cancelled, stop refreshing
				return
			case <-hGpc.gasPriceRefreshTicker.C:
				// Ticker fired, refresh cache
				hGpc.refreshGasPriceCache(ctx)
			}
		}
	}()
}

// refreshGasPriceCache refreshes the shared gas price cache
func (hGpc *HybridGasPriceClient) refreshGasPriceCache(ctx context.Context) {
	if hGpc.gasPriceCache == nil {
		return
	}

	log.L(ctx).Debugf("Refreshing gas price cache")

	// Get fresh data based on which method is configured
	var gasPrice *pldapi.PublicTxGasPricing
	var err error

	if hGpc.gasOracleHTTPClient != nil {
		gasPrice, err = hGpc.getGasPriceFromGasOracle(ctx)
	} else {
		gasPrice, err = hGpc.estimateEIP1559Fees(ctx)
	}

	if err != nil {
		log.L(ctx).Warnf("Failed to refresh gas price cache: %+v", err)
		return
	}

	// Update cache
	hGpc.setCachedGasPrice(gasPrice)
}

// mapConfigToAPIGasPricing converts configuration types to API types
func mapConfigToAPIGasPricing(ctx context.Context, config *pldconf.FixedGasPricing) (*pldapi.PublicTxGasPricing, error) {
	// Both fields must be set for valid fixed gas pricing
	if config.MaxFeePerGas == nil || config.MaxPriorityFeePerGas == nil {
		if config.MaxFeePerGas != nil {
			log.L(ctx).Error("Incomplete fixed gas pricing configuration: maxFeePerGas is set but maxPriorityFeePerGas is missing")
			return nil, i18n.NewError(ctx, msgs.MsgPublicTxMgrFixedGasPriceIncomplete, "maxPriorityFeePerGas")
		}
		if config.MaxPriorityFeePerGas != nil {
			log.L(ctx).Error("Incomplete fixed gas pricing configuration: maxPriorityFeePerGas is set but maxFeePerGas is missing")
			return nil, i18n.NewError(ctx, msgs.MsgPublicTxMgrFixedGasPriceIncomplete, "maxFeePerGas")
		}
		return nil, nil
	}

	maxFeePerGas, err := pldtypes.ParseHexUint256(ctx, *config.MaxFeePerGas)
	if err != nil {
		return nil, err
	}
	maxPriorityFeePerGas, err := pldtypes.ParseHexUint256(ctx, *config.MaxPriorityFeePerGas)
	if err != nil {
		return nil, err
	}

	// Both fields are set, create valid API gas pricing object
	return &pldapi.PublicTxGasPricing{
		MaxFeePerGas:         maxFeePerGas,
		MaxPriorityFeePerGas: maxPriorityFeePerGas,
	}, nil
}
