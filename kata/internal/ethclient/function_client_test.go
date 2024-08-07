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

package ethclient

import (
	"context"
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/types"
	"github.com/kaleido-io/paladin/kata/pkg/signer"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/sha3"
)

var testABIJSON = ([]byte)(`[
	{
		"name": "newWidget",
		"type": "function",
		"inputs": [
			{
				"name": "widget",
				"type": "tuple",
				"components": [
					{
						"name": "id",
						"type": "address"
					},
					{
						"name": "sku",
						"type": "uint256"
					},
					{
						"name": "features",
						"type": "string[]"
					}
				]
			}
		],
		"outputs": []
	},
	{
		"name": "getWidgets",
		"type": "function",
		"inputs": [
			{
				"name": "sku",
				"type": "uint256"
			}
		],
		"outputs": [
			{
				"name": "widget",
				"type": "tuple[]",
				"components": [
					{
						"name": "id",
						"type": "address"
					},
					{
						"name": "sku",
						"type": "uint256"
					},
					{
						"name": "features",
						"type": "string[]"
					}
				]
			}
		]
	}
]`)

type widget struct {
	ID       ethtypes.Address0xHex `json:"id"`
	SKU      ethtypes.HexInteger   `json:"sku"`
	Features []string              `json:"features"`
}

type newWidgetInput struct {
	Widget widget `json:"widget"`
}

func testInvokeNewWidgetOk(t *testing.T, isWS bool, txVersion EthTXVersion) {

	widgetA := &widget{
		ID:       *ethtypes.MustNewAddress("0xFd33700f0511AbB60FF31A8A533854dB90B0a32A"),
		SKU:      *ethtypes.NewHexInteger64(1122334455),
		Features: []string{"shiny", "spinny"},
	}

	var testABI ABIClient
	var key1 string
	ctx, ec, done := newTestClientAndServer(t, isWS, &mockEth{
		eth_chainId: func(ctx context.Context) (ethtypes.HexUint64, error) {
			return 12345, nil
		},
		eth_getTransactionCount: func(ctx context.Context, a ethtypes.Address0xHex, block string) (ethtypes.HexUint64, error) {
			assert.Equal(t, key1, a.String())
			assert.Equal(t, "latest", block)
			return 10, nil
		},
		eth_estimateGas: func(ctx context.Context, tx ethsigner.Transaction) (ethtypes.HexInteger, error) {
			return *ethtypes.NewHexInteger64(100000), nil
		},
		eth_sendRawTransaction: func(ctx context.Context, rawTX ethtypes.HexBytes0xPrefix) (ethtypes.HexBytes0xPrefix, error) {
			addr, tx, err := ethsigner.RecoverRawTransaction(ctx, rawTX, 12345)
			assert.NoError(t, err)
			assert.Equal(t, key1, addr.String())
			assert.Equal(t, int64(10), tx.Nonce.Int64())
			assert.Equal(t, int64(200000 /* 2x */), tx.GasLimit.Int64())

			cv, err := testABI.ABI().Functions()["newWidget"].DecodeCallData(tx.Data)
			assert.NoError(t, err)
			jsonData, err := types.StandardABISerializer().SerializeJSON(cv)
			assert.NoError(t, err)
			assert.JSONEq(t, `{
				"widget": {
					"id":       "0xfd33700f0511abb60ff31a8a533854db90b0a32a",
					"sku":      "1122334455",
					"features": ["shiny", "spinny"]
				}
			}`, string(jsonData))

			hash := sha3.NewLegacyKeccak256()
			_, _ = hash.Write(rawTX)
			return hash.Sum(nil), nil
		},
	})
	defer done()

	_, key1, err := ec.keymgr.ResolveKey(ctx, "key1", signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES)
	assert.NoError(t, err)

	fakeContractAddr := ethtypes.MustNewAddress("0xCC3b61E636B395a4821Df122d652820361FF26f1")

	testABI = ec.MustABIJSON(testABIJSON)
	txHash, err := testABI.MustFunction("newWidget").R(ctx).
		TXVersion(txVersion).
		Signer("key1").
		To(fakeContractAddr).
		Input(&newWidgetInput{
			Widget: *widgetA,
		}).
		SignAndSend()

	assert.NoError(t, err)
	assert.NotEmpty(t, txHash)

}

func TestInvokeNewWidgetOk_WS_EIP1559(t *testing.T) {
	testInvokeNewWidgetOk(t, true, EIP1559)
}

func TestInvokeNewWidgetOk_HTTP_LEGACY_EIP155(t *testing.T) {
	testInvokeNewWidgetOk(t, false, LEGACY_EIP155)
}

func TestInvokeNewWidgetOk_HTTP_LEGACY_ORIGINAL(t *testing.T) {
	testInvokeNewWidgetOk(t, true, LEGACY_ORIGINAL)
}
