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
	"math/big"
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"

	"github.com/kaleido-io/paladin/core/mocks/ethclientmocks"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func longLivedGasPriceTestCache() cache.Cache[string, pldtypes.RawJSON] {
	return cache.NewCache[string, pldtypes.RawJSON](&pldconf.CacheConfig{}, &pldconf.PublicTxManagerDefaults.GasPrice.Cache)
}

func NewTestFixedPriceGasPriceClient(t *testing.T) GasPriceClient {
	hgc := &HybridGasPriceClient{}
	hgc.fixedGasPrice = pldtypes.RawJSON(`{"gasPrice": 10}`)
	hgc.gasPriceCache = longLivedGasPriceTestCache()
	return hgc
}

func NewTestZeroGasPriceChainClient(t *testing.T) GasPriceClient {
	hgc := &HybridGasPriceClient{}
	hgc.fixedGasPrice = pldtypes.RawJSON(`0`)
	hgc.hasZeroGasPrice = true
	hgc.gasPriceCache = longLivedGasPriceTestCache()
	return hgc
}

func NewTestFixedPriceGasPriceClientEIP1559(t *testing.T) GasPriceClient {
	hgc := &HybridGasPriceClient{}
	hgc.fixedGasPrice = pldtypes.RawJSON(`{
		"maxPriorityFeePerGas": 1,
		"maxFeePerGas": 10
	}`)
	hgc.gasPriceCache = longLivedGasPriceTestCache()
	return hgc
}

func NewTestNodeGasPriceClient(t *testing.T, connectorAPI ethclient.EthClient) GasPriceClient {
	hgc := &HybridGasPriceClient{}
	hgc.ethClient = connectorAPI
	hgc.gasPriceCache = longLivedGasPriceTestCache()
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
	zeroGpo, err := zeroHgc.GetGasPriceObject(ctx)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), zeroGpo.GasPrice.Int())
	assert.Nil(t, zeroGpo.MaxFeePerGas)
	assert.Nil(t, zeroGpo.MaxPriorityFeePerGas)

	testTx = &ethsigner.Transaction{}
	tenHgc := NewTestFixedPriceGasPriceClient(t)
	tenHgc.SetFixedGasPriceIfConfigured(ctx, testTx)
	assert.Equal(t, big.NewInt(10), testTx.GasPrice.BigInt())
	assert.Nil(t, testTx.MaxFeePerGas)
	assert.Nil(t, testTx.MaxPriorityFeePerGas)
	tenGpo, err := tenHgc.GetGasPriceObject(ctx)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(10), tenGpo.GasPrice.Int())
	assert.Nil(t, tenGpo.MaxFeePerGas)
	assert.Nil(t, tenGpo.MaxPriorityFeePerGas)

	testTx = &ethsigner.Transaction{}
	eip1559Hgc := NewTestFixedPriceGasPriceClientEIP1559(t)
	eip1559Hgc.SetFixedGasPriceIfConfigured(ctx, testTx)
	assert.Equal(t, big.NewInt(10), testTx.MaxFeePerGas.BigInt())
	assert.Equal(t, big.NewInt(1), testTx.MaxPriorityFeePerGas.BigInt())
	assert.Nil(t, testTx.GasPrice)
	gpo, err := eip1559Hgc.GetGasPriceObject(ctx)
	assert.NoError(t, err)
	assert.Nil(t, gpo.GasPrice)
	assert.Equal(t, big.NewInt(10), gpo.MaxFeePerGas.Int())
	assert.Equal(t, big.NewInt(1), gpo.MaxPriorityFeePerGas.Int())
}

func TestGasPriceClientInit(t *testing.T) {
	ctx := context.Background()
	hgc := &HybridGasPriceClient{}
	hgc.fixedGasPrice = pldtypes.RawJSON(`invalid`)
	hgc.gasPriceCache = longLivedGasPriceTestCache()
	assert.False(t, hgc.hasZeroGasPrice)
	hgc.Init(ctx, nil)
	assert.False(t, hgc.hasZeroGasPrice)
	hgc.fixedGasPrice = pldtypes.RawJSON(`0`)
	hgc.Init(ctx, nil)
	assert.True(t, hgc.hasZeroGasPrice)
}

func TestFixedGasPrice(t *testing.T) {
	ctx := context.Background()

	gasPriceClient := NewGasPriceClient(ctx, &pldconf.PublicTxManagerConfig{
		GasPrice: pldconf.GasPriceConfig{
			FixedGasPrice: "1020304050",
		},
	})
	hgc := gasPriceClient.(*HybridGasPriceClient)

	gpo, err := hgc.GetGasPriceObject(ctx)
	require.NoError(t, err)
	assert.Equal(t, &pldapi.PublicTxGasPricing{
		GasPrice: pldtypes.Int64ToInt256(1020304050),
	}, gpo)
}

func TestGasPriceClient(t *testing.T) {
	ctx := context.Background()

	gasPriceClient := NewGasPriceClient(ctx, &pldconf.PublicTxManagerConfig{})
	hgc := gasPriceClient.(*HybridGasPriceClient)

	mEC := ethclientmocks.NewEthClient(t)
	hgc.Init(ctx, mEC)
	// check functions
	assert.True(t, hgc.HasZeroGasPrice(ctx))

	testNodeGasPrice := `"0x03e8"`
	// fall back to connector when get call failed
	mEC.On("GasPrice", ctx, mock.Anything).Return(pldtypes.Uint64ToUint256(1000), nil).Once()
	gasPriceJSON, err := hgc.getGasPriceJSON(ctx)
	require.NoError(t, err)
	assert.Equal(t, testNodeGasPrice, gasPriceJSON.String())

	// gasPrice should be cached
	gasPriceJSON, err = hgc.getGasPriceJSON(ctx)
	require.NoError(t, err)
	assert.Equal(t, testNodeGasPrice, gasPriceJSON.String())
	fixedGpo, err := hgc.ParseGasPriceJSON(ctx, gasPriceJSON)
	require.NoError(t, err)
	assert.Equal(t, big.NewInt(1000), fixedGpo.GasPrice.Int())
	assert.Nil(t, fixedGpo.MaxFeePerGas)
	assert.Nil(t, fixedGpo.MaxPriorityFeePerGas)

	// return error when connector also errored
	hgc.DeleteCache(ctx)
	mEC.On("GasPrice", ctx, mock.Anything).Return(nil, fmt.Errorf("doesn't work")).Once()
	gpo, err := hgc.GetGasPriceObject(ctx)
	assert.Regexp(t, "doesn't work", err)
	assert.Nil(t, gpo)
}
