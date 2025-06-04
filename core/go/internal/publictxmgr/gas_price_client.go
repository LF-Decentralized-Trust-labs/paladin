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
	"math/big"

	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/msgs"

	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/cache"
)

type GasPriceClient interface {
	DeleteCache(ctx context.Context)
	HasZeroGasPrice(ctx context.Context) bool
	SetFixedGasPriceIfConfigured(ctx context.Context, ethTx *ethsigner.Transaction)
	GetFixedGasPriceJSON(ctx context.Context) (gasPrice pldtypes.RawJSON)
	ParseGasPriceJSON(ctx context.Context, input pldtypes.RawJSON) (gpo *pldapi.PublicTxGasPricing, err error)
	GetGasPriceObject(ctx context.Context) (gasPrice *pldapi.PublicTxGasPricing, err error)
	Init(ctx context.Context, cAPI ethclient.EthClient)
}

// The hybrid gas price client retrieves gas price using the following methods in order and will return as soon as the method succeeded unless there is an override
//   - Fixed gas price
//   - Cached gas price
//   - Gas Oracle
//   - Node gas_Price
type HybridGasPriceClient struct {
	hasZeroGasPrice bool
	fixedGasPrice   pldtypes.RawJSON
	ethClient       ethclient.EthClient
	gasPriceCache   cache.Cache[string, pldtypes.RawJSON]
}

func (hGpc *HybridGasPriceClient) HasZeroGasPrice(ctx context.Context) bool {
	return hGpc.hasZeroGasPrice
}

func (hGpc *HybridGasPriceClient) GetFixedGasPriceJSON(ctx context.Context) (gasPrice pldtypes.RawJSON) {
	return hGpc.fixedGasPrice
}

func (hGpc *HybridGasPriceClient) SetFixedGasPriceIfConfigured(ctx context.Context, ethTx *ethsigner.Transaction) {
	if hGpc.fixedGasPrice != nil {
		gpo, _ := hGpc.ParseGasPriceJSON(ctx, hGpc.fixedGasPrice)
		if gpo.GasPrice != nil {
			ethTx.GasPrice = (*ethtypes.HexInteger)(gpo.GasPrice)
		}
		if gpo.MaxFeePerGas != nil {
			ethTx.MaxFeePerGas = (*ethtypes.HexInteger)(gpo.MaxFeePerGas)
		}

		if gpo.MaxPriorityFeePerGas != nil {
			ethTx.MaxPriorityFeePerGas = (*ethtypes.HexInteger)(gpo.MaxPriorityFeePerGas)
		}
	}
}

func (hGpc *HybridGasPriceClient) GetGasPriceObject(ctx context.Context) (gasPrice *pldapi.PublicTxGasPricing, err error) {

	gasPriceJSON, err := hGpc.getGasPriceJSON(ctx)
	if err != nil {
		return nil, err
	}

	return hGpc.ParseGasPriceJSON(ctx, gasPriceJSON)
}

func (hGpc *HybridGasPriceClient) getGasPriceJSON(ctx context.Context) (gasPriceJSON pldtypes.RawJSON, err error) {

	//  fixed price overrides everything
	if !hGpc.fixedGasPrice.IsNil() {
		log.L(ctx).Debugf("Retrieving gas price from fixed gas price")
		gasPriceJSON = hGpc.fixedGasPrice
		return
	}

	// use the cache gas price first
	cachedGasPrice, _ := hGpc.gasPriceCache.Get("gasPrice")
	if cachedGasPrice != nil {
		return cachedGasPrice, nil
	}

	// then try to use the node eth call
	log.L(ctx).Debugf("Retrieving gas price from node eth call")
	gasPriceHexInt, err := hGpc.ethClient.GasPrice(ctx)
	if err != nil {
		// no fallback is available, return the error
		log.L(ctx).Errorf("Failed to retrieve gas price from the node")
		return nil, err
	} else {
		gasPriceJSON = pldtypes.RawJSON(fmt.Sprintf(`"%s"`, gasPriceHexInt))
	}

	hGpc.gasPriceCache.Set("gasPrice", gasPriceJSON)

	return gasPriceJSON, nil

}
func (hGpc *HybridGasPriceClient) Init(ctx context.Context, ethClient ethclient.EthClient) {
	hGpc.ethClient = ethClient
	// check whether it's a gasless chain
	gasPriceJson := hGpc.GetFixedGasPriceJSON(ctx)
	gpo, err := hGpc.ParseGasPriceJSON(ctx, gasPriceJson)
	if err != nil {
		log.L(ctx).Warnf("Cannot get gas price due to %+v", err)
	}

	if gpo != nil && ((gpo.GasPrice != nil && gpo.GasPrice.Int().Sign() == 0) ||
		(gpo.MaxFeePerGas != nil && gpo.MaxFeePerGas.Int().Sign() == 0 &&
			gpo.MaxPriorityFeePerGas != nil && gpo.MaxPriorityFeePerGas.Int().Sign() == 0)) {
		hGpc.hasZeroGasPrice = true
		hGpc.fixedGasPrice = gasPriceJson
	}
}

func (hGpc *HybridGasPriceClient) DeleteCache(ctx context.Context) {
	hGpc.gasPriceCache.Delete("gasPrice")
}

func NewGasPriceClient(ctx context.Context, conf *pldconf.PublicTxManagerConfig) GasPriceClient {
	gasPriceCache := cache.NewCache[string, pldtypes.RawJSON](&conf.GasPrice.Cache, &pldconf.PublicTxManagerDefaults.GasPrice.Cache)
	log.L(ctx).Debugf("Gas price cache size: %d", gasPriceCache.Capacity())
	gasPriceClient := &HybridGasPriceClient{}
	// initialize gas oracle
	// set fixed gas price
	b, _ := json.Marshal(conf.GasPrice.FixedGasPrice)
	if b != nil && string(b) != `null` {
		gasPriceClient.fixedGasPrice = pldtypes.RawJSON(b)
	}
	gasPriceClient.gasPriceCache = gasPriceCache
	return gasPriceClient
}

func (hGpc *HybridGasPriceClient) ParseGasPriceJSON(ctx context.Context, input pldtypes.RawJSON) (gpo *pldapi.PublicTxGasPricing, err error) {
	gpo = &pldapi.PublicTxGasPricing{}
	if input == nil {
		gpo.GasPrice = (*pldtypes.HexUint256)(big.NewInt(0))
		log.L(ctx).Tracef("Gas price object generated using empty input, gasPrice=%+v", gpo)
		return gpo, nil
	}

	err = json.Unmarshal(input.Bytes(), gpo)
	if err != nil {
		tempHexInt := (*pldtypes.HexUint256)(big.NewInt(0))
		err := json.Unmarshal(input.Bytes(), tempHexInt)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgGasPriceError, input.String(), err.Error())
		}
		gpo.GasPrice = tempHexInt
		log.L(ctx).Tracef("Gas price object generated using gasPrice number, gasPrice=%+v", gpo)
		return gpo, nil
	}

	if (gpo.MaxPriorityFeePerGas != nil && gpo.MaxPriorityFeePerGas.Int().Sign() > 0) ||
		(gpo.MaxFeePerGas != nil && gpo.MaxFeePerGas.Int().Sign() > 0) {
		// assign to a new object which only has the relevant fields for EIP1559
		gpo = &pldapi.PublicTxGasPricing{
			MaxPriorityFeePerGas: gpo.MaxPriorityFeePerGas,
			MaxFeePerGas:         gpo.MaxFeePerGas,
		}
		log.L(ctx).Tracef("Gas price object generated using EIP1559 fields, gasPrice=%+v", gpo)
		return gpo, nil
	}
	log.L(ctx).Tracef("Gas price object generated using gasPrice field, gasPrice=%+v", gpo)
	// assign to a new object which definitely only has the gas price field
	return &pldapi.PublicTxGasPricing{
		GasPrice: gpo.GasPrice,
	}, nil
}
