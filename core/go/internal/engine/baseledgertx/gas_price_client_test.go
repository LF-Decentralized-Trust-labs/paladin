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

package baseledgertx

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/hyperledger/firefly-common/pkg/cache"
	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func NewTestFixedPriceGasPriceClient(t *testing.T) GasPriceClient {
	ctx := context.Background()
	hgc := &HybridGasPriceClient{}
	hgc.fixedGasPrice = fftypes.JSONAnyPtr(`{"gasPrice": 10}`)
	hgc.gasPriceCache = cache.NewUmanagedCache(ctx, 100, 1*time.Minute)
	return hgc
}

func NewTestZeroGasPriceChainClient(t *testing.T) GasPriceClient {
	ctx := context.Background()
	hgc := &HybridGasPriceClient{}
	hgc.fixedGasPrice = fftypes.JSONAnyPtr(`0`)
	hgc.hasZeroGasPrice = true
	hgc.gasPriceCache = cache.NewUmanagedCache(ctx, 100, 1*time.Minute)
	return hgc
}

func NewTestFixedPriceGasPriceClientEIP1559(t *testing.T) GasPriceClient {
	ctx := context.Background()
	hgc := &HybridGasPriceClient{}
	hgc.fixedGasPrice = fftypes.JSONAnyPtr(`{
		"maxPriorityFeePerGas": 1,
		"maxFeePerGas": 10
	}`)
	hgc.gasPriceCache = cache.NewUmanagedCache(ctx, 100, 1*time.Minute)
	return hgc
}

func NewTestNodeGasPriceClient(t *testing.T, connectorAPI ethclient.EthClient) GasPriceClient {
	ctx := context.Background()
	hgc := &HybridGasPriceClient{}
	hgc.cAPI = connectorAPI
	hgc.gasPriceCache = cache.NewUmanagedCache(ctx, 100, 1*time.Minute)
	return hgc
}

func TestSetFixedGasPriceIfConfigured(t *testing.T) {
	ctx := context.Background()
	zeroHgc := NewTestZeroGasPriceChainClient(t)
	testTx := &ethsigner.Transaction{}
	zeroHgc.SetFixedGasPriceIfConfigured(ctx, testTx)
	assert.Equal(t, big.NewInt(0), testTx.GasPrice.BigInt())
	assert.Nil(t, testTx.MaxFeePerGas)
	assert.Nil(t, testTx.MaxPriorityFeePerGas)

	testTx = &ethsigner.Transaction{}
	tenHgc := NewTestFixedPriceGasPriceClient(t)
	tenHgc.SetFixedGasPriceIfConfigured(ctx, testTx)
	assert.Equal(t, big.NewInt(10), testTx.GasPrice.BigInt())
	assert.Nil(t, testTx.MaxFeePerGas)
	assert.Nil(t, testTx.MaxPriorityFeePerGas)

	testTx = &ethsigner.Transaction{}
	eip1559Hgc := NewTestFixedPriceGasPriceClientEIP1559(t)
	eip1559Hgc.SetFixedGasPriceIfConfigured(ctx, testTx)
	assert.Equal(t, big.NewInt(10), testTx.MaxFeePerGas.BigInt())
	assert.Equal(t, big.NewInt(1), testTx.MaxPriorityFeePerGas.BigInt())
	assert.Nil(t, testTx.GasPrice)
}

func TestGasPriceClientInit(t *testing.T) {
	ctx := context.Background()
	hgc := &HybridGasPriceClient{}
	hgc.fixedGasPrice = fftypes.JSONAnyPtr(`invalid`)
	hgc.gasPriceCache = cache.NewUmanagedCache(ctx, 100, 1*time.Minute)
	assert.False(t, hgc.hasZeroGasPrice)
	hgc.Init(ctx, nil)
	assert.False(t, hgc.hasZeroGasPrice)
	hgc.fixedGasPrice = fftypes.JSONAnyPtr(`0`)
	hgc.Init(ctx, nil)
	assert.True(t, hgc.hasZeroGasPrice)
}

func TestGasPriceClient(t *testing.T) {
	ctx := context.Background()
	gasPriceCache := cache.NewUmanagedCache(ctx, 100, 1*time.Minute)
	conf := config.RootSection("unittestgasprice")
	InitGasPriceConfig(conf)
	gasPriceConf := conf.SubSection(GasPriceSection)

	gasPriceClient := NewGasPriceClient(ctx, gasPriceConf, gasPriceCache)
	hgc := gasPriceClient.(*HybridGasPriceClient)

	mEC := componentmocks.NewEthClient(t)
	hgc.Init(ctx, mEC)
	// check functions
	assert.False(t, hgc.HasZeroGasPrice(ctx))

	testNodeGasPrice := `"1000"`
	// fall back to connector when get call failed
	mEC.On("GasPrice", ctx, mock.Anything).Return(ethtypes.NewHexInteger64(1000), nil).Once()
	gasPriceJSON, err := hgc.getGasPriceJSON(ctx)
	require.NoError(t, err)
	assert.Equal(t, testNodeGasPrice, gasPriceJSON.String())

	// gasPrice should be cached
	gasPriceJSON, err = hgc.getGasPriceJSON(ctx)
	require.NoError(t, err)
	assert.Equal(t, testNodeGasPrice, gasPriceJSON.String())
	fixedGpo, err := hgc.ParseGasPriceJSON(ctx, gasPriceJSON)
	require.NoError(t, err)
	assert.Equal(t, big.NewInt(1000), fixedGpo.GasPrice)
	assert.Nil(t, fixedGpo.MaxFeePerGas)
	assert.Nil(t, fixedGpo.MaxPriorityFeePerGas)

	// return error when connector also errored
	hgc.DeleteCache(ctx)
	mEC.On("GasPrice", ctx, mock.Anything).Return(nil, fmt.Errorf("doesn't work")).Once()
	gpo, err := hgc.GetGasPriceObject(ctx)
	assert.Regexp(t, "doesn't work", err)
	assert.Nil(t, gpo)
}
