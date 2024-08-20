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
	"fmt"
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/sha3"
)

var testABIJSON = ([]byte)(`[
	{
		"type": "constructor",
		"inputs": [
			{
				"name": "supplier",
				"type": "address"
			}
		]
	},
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
				"name": "",
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

type getWidgetsOutput struct {
	// In this example the output is anonymous, so gets converted to an index integer (better to name outputs)
	Zero []*widget `json:"0"`
}

func testInvokeNewWidgetOk(t *testing.T, isWS bool, txVersion EthTXVersion, gasLimit bool) {

	widgetA := &widget{
		ID:       *ethtypes.MustNewAddress("0xFd33700f0511AbB60FF31A8A533854dB90B0a32A"),
		SKU:      *ethtypes.NewHexInteger64(1122334455),
		Features: []string{"shiny", "spinny"},
	}

	var testABI ABIClient
	var key1 string
	ctx, ecf, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionCount: func(ctx context.Context, a ethtypes.Address0xHex, block string) (ethtypes.HexUint64, error) {
			assert.Equal(t, key1, a.String())
			assert.Equal(t, "latest", block)
			return 10, nil
		},
		eth_estimateGas: func(ctx context.Context, tx ethsigner.Transaction) (ethtypes.HexInteger, error) {
			assert.False(t, gasLimit)
			return *ethtypes.NewHexInteger64(100000), nil
		},
		eth_sendRawTransaction: func(ctx context.Context, rawTX ethtypes.HexBytes0xPrefix) (ethtypes.HexBytes0xPrefix, error) {
			addr, tx, err := ethsigner.RecoverRawTransaction(ctx, rawTX, 12345)
			assert.NoError(t, err)
			assert.Equal(t, key1, addr.String())
			assert.Equal(t, int64(10), tx.Nonce.Int64())
			if gasLimit {
				assert.Equal(t, int64(100000), tx.GasLimit.Int64())
			} else {
				assert.Equal(t, int64(200000 /* 2x estimate */), tx.GasLimit.Int64())
			}

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

	var ec EthClient
	if isWS {
		ec = ecf.SharedWS()
	} else {
		ec = ecf.HTTPClient()
	}

	_, key1, err := ecf.keymgr.ResolveKey(ctx, "key1", api.Algorithm_ECDSA_SECP256K1_PLAINBYTES)
	assert.NoError(t, err)

	fakeContractAddr := ethtypes.MustNewAddress("0xCC3b61E636B395a4821Df122d652820361FF26f1")

	testABI = ec.MustABIJSON(testABIJSON)
	req := testABI.MustFunction("newWidget").R(ctx).
		TXVersion(txVersion).
		Signer("key1").
		To(fakeContractAddr).
		Input(&newWidgetInput{
			Widget: *widgetA,
		})
	if gasLimit {
		req = req.GasLimit(100000)
	}
	txHash, err := req.SignAndSend()

	assert.NoError(t, err)
	assert.NotEmpty(t, txHash)

}

func TestInvokeNewWidgetOk_WS_EIP1559(t *testing.T) {
	testInvokeNewWidgetOk(t, true, EIP1559, false)
}

func TestInvokeNewWidgetOk_HTTP_LEGACY_EIP155(t *testing.T) {
	testInvokeNewWidgetOk(t, false, LEGACY_EIP155, false)
}

func TestInvokeNewWidgetOk_HTTP_gasLimit_LEGACY_ORIGINAL(t *testing.T) {
	testInvokeNewWidgetOk(t, true, LEGACY_ORIGINAL, true)
}

func testCallGetWidgetsOk(t *testing.T, withFrom, withBlock, withBlockRef bool) {

	var testABI ABIClient
	var key1 string
	var err error
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_call: func(ctx context.Context, tx ethsigner.Transaction, s string) (ethtypes.HexBytes0xPrefix, error) {
			if withBlock {
				assert.Equal(t, "0x3039", s)
			} else if withBlockRef {
				assert.Equal(t, "pending", s)
			} else {
				assert.Equal(t, "latest", s)
			}
			if withFrom {
				assert.Equal(t, types.JSONString(key1), types.RawJSON(tx.From))
			} else {
				assert.Nil(t, tx.From)
			}
			cv, err := testABI.ABI().Functions()["getWidgets"].DecodeCallData(tx.Data)
			assert.NoError(t, err)
			assert.NoError(t, err)
			jsonData, err := types.StandardABISerializer().SerializeJSON(cv)
			assert.NoError(t, err)
			assert.JSONEq(t, `{
				"sku":      "1122334455"
			}`, string(jsonData))

			// Note that the client handles unnamed outputs using an index numeral
			retJSON := ([]byte)(`{
				"0": [
					{
						"id":       "0xfd33700f0511abb60ff31a8a533854db90b0a32a",
						"sku":      "1122334455",
						"features": ["shiny", "spinny"]
					}
				]
			}`)
			return testABI.ABI().Functions()["getWidgets"].Outputs.EncodeABIDataJSON(retJSON)
		},
	})
	defer done()

	if withFrom {
		_, key1, err = ec.keymgr.ResolveKey(ctx, "key1", api.Algorithm_ECDSA_SECP256K1_PLAINBYTES)
		assert.NoError(t, err)
	}

	fakeContractAddr := ethtypes.MustNewAddress("0xCC3b61E636B395a4821Df122d652820361FF26f1")

	testABI = ec.HTTPClient().MustABIJSON(testABIJSON)
	getWidgetsReq := testABI.MustFunction("getWidgets").R(ctx).
		To(fakeContractAddr).
		Input(`{"sku": 1122334455}`)
	if withFrom {
		getWidgetsReq.
			Signer("key1")
	}
	if withBlock {
		getWidgetsReq.Block(12345)
	} else if withBlockRef {
		getWidgetsReq.BlockRef(PENDING)
	}
	jsonRes, err := getWidgetsReq.CallJSON()
	assert.NoError(t, err)
	assert.JSONEq(t, `{
		"0": [
			{
				"id":       "0xfd33700f0511abb60ff31a8a533854db90b0a32a",
				"sku":      "1122334455",
				"features": ["shiny", "spinny"]
			}
		]
	}`, string(jsonRes))

	var getWidgetsRes getWidgetsOutput
	err = getWidgetsReq.
		Output(&getWidgetsRes).
		Call()

	assert.NoError(t, err)
	assert.Len(t, getWidgetsRes.Zero, 1)
	assert.Equal(t, uint64(1122334455), getWidgetsRes.Zero[0].SKU.Uint64())

}

func TestCallGetWidgetsWithFromOk(t *testing.T) {
	testCallGetWidgetsOk(t, true, false, false)
}

func TestCallGetWidgetsNoFromWithBlockOk(t *testing.T) {
	testCallGetWidgetsOk(t, false, true, false)
}

func TestCallGetWidgetsFromWithBlockResOk(t *testing.T) {
	testCallGetWidgetsOk(t, true, false, true)
}

func TestABIFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{})
	defer done()

	assert.Panics(t, func() {
		ec.HTTPClient().MustABIJSON(([]byte)("!wrong"))
	})

	_, err := ec.HTTPClient().ABIJSON(ctx, ([]byte)(`[
		{
		  "type": "function",
		  "inputs": [
			 {
			   "type": "wrong!"
			 }
		  ]
		}
	  ]`))
	assert.Regexp(t, "FF22025", err)
}

func TestFunctionFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{})
	defer done()
	tABI := ec.HTTPClient().MustABIJSON(testABIJSON)
	_, err := tABI.Function(ctx, "missing")
	assert.Regexp(t, "PD011507", err)

	abiFunctionWrong := &abiFunctionClient{ec: ec.HTTPClient().(*ethClient)}
	_, err = abiFunctionWrong.functionCommon(ctx, &abi.Entry{
		Type: "function",
		Name: "wrong",
		Inputs: abi.ParameterArray{
			{Type: "!wrong"},
		},
	})
	assert.Regexp(t, "FF22025", err)

	assert.Panics(t, func() {
		_ = tABI.MustFunction("wrong")
	})
}

func TestConstructorFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{})
	defer done()

	tABI := ec.HTTPClient().MustABIJSON(([]byte)(`[]`))
	defaultConstructor := tABI.MustConstructor([]byte{})
	assert.Equal(t, "()", defaultConstructor.(*abiFunctionClient).inputs.String())

	tABI.(*abiClient).abi = abi.ABI{
		{
			Type:   abi.Constructor,
			Inputs: abi.ParameterArray{{Type: "!wrong"}},
		},
	}
	_, err := tABI.Constructor(ctx, []byte{})
	assert.Regexp(t, "FF22025", err)

	assert.Panics(t, func() {
		_ = tABI.MustConstructor([]byte{})
	})
}

func TestCallFunctionFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_call: func(ctx context.Context, t ethsigner.Transaction, s string) (ethtypes.HexBytes0xPrefix, error) {
			return nil, fmt.Errorf("pop")
		},
	})
	defer done()
	getWidgets := ec.HTTPClient().MustABIJSON(testABIJSON).MustFunction("getWidgets")

	to := ethtypes.MustNewAddress("0xD9E54Ba3F1419e6AC71A795d819fdBAE883A6575")

	_, err := getWidgets.R(ctx).Input(`{"sku":12345}`).To(to).CallJSON()
	assert.Regexp(t, "pop", err)
}

func TestSignAndSendMissingFrom(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_call: func(ctx context.Context, t ethsigner.Transaction, s string) (ethtypes.HexBytes0xPrefix, error) {
			return nil, fmt.Errorf("pop")
		},
	})
	defer done()
	newWidget := ec.HTTPClient().MustABIJSON(testABIJSON).MustFunction("newWidget")

	req := newWidget.R(ctx).Input(&newWidgetInput{
		Widget: widget{
			ID:       *ethtypes.MustNewAddress("0x9fF786fEf6742c066c5c0d7b12d264C7b390c37b"),
			SKU:      *ethtypes.NewHexInteger64(12345),
			Features: []string{},
		},
	}).To(ethtypes.MustNewAddress("0xD9E54Ba3F1419e6AC71A795d819fdBAE883A6575"))

	_, err := req.SignAndSend()
	assert.Regexp(t, "PD011501", err)
}

func TestMissingInputs(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{})
	defer done()
	getWidgets := ec.HTTPClient().MustABIJSON(testABIJSON).MustFunction("getWidgets")

	to := ethtypes.MustNewAddress("0xFB75836Dc4130a9462FAFD8fe96c8Ee376e2f32e")

	err := getWidgets.R(ctx).To(to).Call()
	assert.Regexp(t, "PD011504", err)

	err = getWidgets.R(ctx).To(to).Output("supplied").Call()
	assert.Regexp(t, "PD011503", err)

	err = getWidgets.R(ctx).Output("supplied").Input("supplied").Call()
	assert.Regexp(t, "PD011502", err)

	_, err = getWidgets.R(ctx).Output("supplied").Input("supplied").RawTransaction()
	assert.Regexp(t, "PD011502", err)

	err = ec.HTTPClient().MustABIJSON(testABIJSON).MustConstructor([]byte{}).R(ctx).Output("supplied").Input("supplied").To(to).Call()
	assert.Regexp(t, "PD011510", err)

}

func TestBuildCallData(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{})
	defer done()
	newWidget := ec.HTTPClient().MustABIJSON(testABIJSON).MustFunction("newWidget")

	to := ethtypes.MustNewAddress("0xD9E54Ba3F1419e6AC71A795d819fdBAE883A6575")

	err := newWidget.R(ctx).To(to).Input("! not JSON").BuildCallData()
	assert.Regexp(t, "PD011500.*invalid", err)

	err = newWidget.R(ctx).To(to).Input("{}").BuildCallData()
	assert.Regexp(t, "PD011500.*FF22040", err)

	err = newWidget.R(ctx).To(to).Input(([]byte)(`{
		"widget": {}
	}`)).BuildCallData()
	assert.Regexp(t, "PD011500.*FF22040.*id", err)

	req := newWidget.R(ctx).To(to)

	err = req.Input(types.RawJSON(`{
		"widget": {
			"id":       "0xfd33700f0511abb60ff31a8a533854db90b0a32a",
			"sku":      "1122334455",
			"features": ["shiny", "spinny"]
		}
	}`)).BuildCallData()
	assert.NoError(t, err)
	assert.NotEmpty(t, req.TX().Data)

	err = req.Input(&newWidgetInput{
		Widget: widget{
			ID:       *ethtypes.MustNewAddress("0x9fF786fEf6742c066c5c0d7b12d264C7b390c37b"),
			SKU:      *ethtypes.NewHexInteger64(12345),
			Features: []string{},
		},
	}).BuildCallData()
	assert.NoError(t, err)
	assert.NotEmpty(t, req.TX().Data)

	err = req.Input(map[string]any{
		"widget": map[string]any{
			"id":       "0x9fF786fEf6742c066c5c0d7b12d264C7b390c37b",
			"sku":      12345,
			"features": []string{},
		},
	}).BuildCallData()
	assert.NoError(t, err)
	assert.NotEmpty(t, req.TX().Data)

}

func TestInvokeConstructor(t *testing.T) {

	fakeBytecode := ([]byte)(`some_bytes`)

	var testABI ABIClient
	var key1 string
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
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
			assert.Equal(t, int64(200000 /* 2x estimate */), tx.GasLimit.Int64())

			cv, err := testABI.ABI().Constructor().Inputs.DecodeABIData(tx.Data, len(fakeBytecode))
			assert.NoError(t, err)
			jsonData, err := types.StandardABISerializer().SerializeJSON(cv)
			assert.NoError(t, err)
			assert.JSONEq(t, `{
				"supplier": "0xfb75836dc4130a9462fafd8fe96c8ee376e2f32e"
			}`, string(jsonData))

			hash := sha3.NewLegacyKeccak256()
			_, _ = hash.Write(rawTX)
			return hash.Sum(nil), nil
		},
	})
	defer done()

	_, key1, err := ec.keymgr.ResolveKey(ctx, "key1", api.Algorithm_ECDSA_SECP256K1_PLAINBYTES)
	assert.NoError(t, err)

	testABI = ec.HTTPClient().MustABIJSON(testABIJSON)
	req := testABI.MustConstructor(fakeBytecode).R(ctx).
		Signer("key1").
		Input(`{"supplier": "0xFB75836Dc4130a9462FAFD8fe96c8Ee376e2f32e"}`)
	txHash, err := req.SignAndSend()

	assert.NoError(t, err)
	assert.NotEmpty(t, txHash)

}
