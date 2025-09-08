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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	},
	{
	  "type": "error",
	  "name": "WidgetError",
	  "inputs": [
	    {
	      "name": "sku",
	      "type": "uint256"
	    },
	    {
	      "name": "issue",
	      "type": "string"
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
		eth_getTransactionCount: func(ctx context.Context, a pldtypes.EthAddress, block string) (pldtypes.HexUint64, error) {
			assert.Equal(t, key1, a.String())
			assert.Equal(t, "latest", block)
			return 10, nil
		},
		eth_estimateGas: func(ctx context.Context, tx ethsigner.Transaction) (pldtypes.HexUint64, error) {
			assert.False(t, gasLimit)
			return 100000, nil
		},
		eth_sendRawTransaction: func(ctx context.Context, rawTX pldtypes.HexBytes) (pldtypes.HexBytes, error) {
			addr, tx, err := ethsigner.RecoverRawTransaction(ctx, ethtypes.HexBytes0xPrefix(rawTX), 12345)
			require.NoError(t, err)
			assert.Equal(t, key1, addr.String())
			assert.Equal(t, int64(10), tx.Nonce.Int64())
			if gasLimit {
				assert.Equal(t, int64(100000), tx.GasLimit.Int64())
			} else {
				assert.Equal(t, int64(200000 /* 2x estimate */), tx.GasLimit.Int64())
			}

			cv, err := testABI.ABI().Functions()["newWidget"].DecodeCallData(tx.Data)
			require.NoError(t, err)
			jsonData, err := pldtypes.StandardABISerializer().SerializeJSON(cv)
			require.NoError(t, err)
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

	var ec EthClientWithKeyManager
	if isWS {
		ec = ecf.SharedWS()
	} else {
		ec = ecf.HTTPClient()
	}

	_, key1, err := ecf.ecf.keymgr.ResolveKey(ctx, "key1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

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

	require.NoError(t, err)
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
	ctx, ecf, done := newTestClientAndServer(t, &mockEth{
		eth_call: func(ctx context.Context, tx ethsigner.Transaction, s string) (pldtypes.HexBytes, error) {
			if withBlock {
				assert.Equal(t, "0x3039", s)
			} else if withBlockRef {
				assert.Equal(t, "pending", s)
			} else {
				assert.Equal(t, "latest", s)
			}
			if withFrom {
				assert.Equal(t, pldtypes.JSONString(key1), pldtypes.RawJSON(tx.From))
			} else {
				assert.Nil(t, tx.From)
			}
			cv, err := testABI.ABI().Functions()["getWidgets"].DecodeCallData(tx.Data)
			require.NoError(t, err)
			require.NoError(t, err)
			jsonData, err := pldtypes.StandardABISerializer().SerializeJSON(cv)
			require.NoError(t, err)
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
		_, key1, err = ecf.ecf.keymgr.ResolveKey(ctx, "key1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
		require.NoError(t, err)
	}

	fakeContractAddr := ethtypes.MustNewAddress("0xCC3b61E636B395a4821Df122d652820361FF26f1")

	testABI = ecf.HTTPClient().MustABIJSON(testABIJSON)
	getWidgetsReq := testABI.MustFunction("getWidgets").R(ctx).
		To(fakeContractAddr).
		Input(`{"sku": 1122334455}`).
		Serializer(abi.NewSerializer().
			SetFormattingMode(abi.FormatAsFlatArrays).
			SetByteSerializer(abi.HexByteSerializer0xPrefix),
		)
	if withFrom {
		getWidgetsReq.
			Signer("key1")
	}
	if withBlock {
		getWidgetsReq.Block(12345)
	} else if withBlockRef {
		getWidgetsReq.BlockRef(PENDING)
	}
	res, err := getWidgetsReq.CallResult()
	require.NoError(t, err)
	assert.JSONEq(t, `[
		[
			[
				"0xfd33700f0511abb60ff31a8a533854db90b0a32a",
				"1122334455",
				["shiny", "spinny"]
			]
		]
	]`, res.JSON())

	var getWidgetsRes getWidgetsOutput
	err = getWidgetsReq.
		Output(&getWidgetsRes).
		Call()

	require.NoError(t, err)
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

	badFunction := &abi.Entry{
		Type: "function",
		Name: "wrong",
		Inputs: abi.ParameterArray{
			{Type: "!wrong"},
		},
	}
	abiFunctionWrong := &abiFunctionClient{ec: ec.HTTPClient().(*ethClient)}
	_, err = abiFunctionWrong.functionCommon(ctx, badFunction)
	assert.Regexp(t, "FF22025", err)

	_, err = ec.HTTPClient().ABIFunction(ctx, badFunction)
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

	badConstructor := &abi.Entry{
		Type:   abi.Constructor,
		Inputs: abi.ParameterArray{{Type: "!wrong"}},
	}
	tABI.(*abiClient).abi = abi.ABI{badConstructor}
	_, err := tABI.Constructor(ctx, []byte{})
	assert.Regexp(t, "FF22025", err)

	_, err = ec.HTTPClient().ABIConstructor(ctx, badConstructor, []byte{})
	assert.Regexp(t, "FF22025", err)

	assert.Panics(t, func() {
		_ = tABI.MustConstructor([]byte{})
	})
}

func TestABIFunctionShortcutsOK(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{})
	defer done()

	fc, err := ec.HTTPClient().ABIFunction(ctx, &abi.Entry{
		Type:    abi.Function,
		Name:    "foo",
		Inputs:  abi.ParameterArray{},
		Outputs: abi.ParameterArray{},
	})
	require.NoError(t, err)
	assert.NotNil(t, fc)

	cc, err := ec.HTTPClient().ABIConstructor(ctx, &abi.Entry{
		Type:    abi.Constructor,
		Inputs:  abi.ParameterArray{},
		Outputs: abi.ParameterArray{},
	}, []byte{})
	require.NoError(t, err)
	assert.NotNil(t, cc)
}

func TestCallFunctionFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_call: func(ctx context.Context, t ethsigner.Transaction, s string) (pldtypes.HexBytes, error) {
			return nil, fmt.Errorf("pop")
		},
	})
	defer done()
	getWidgets := ec.HTTPClient().MustABIJSON(testABIJSON).MustFunction("getWidgets")

	to := ethtypes.MustNewAddress("0xD9E54Ba3F1419e6AC71A795d819fdBAE883A6575")

	_, err := getWidgets.R(ctx).Input(`{"sku":12345}`).To(to).CallResult()
	assert.Regexp(t, "pop", err)
}

func TestCallFunctionNoResolveEmptyResult(t *testing.T) {
	ctx, ecf, done := newTestClientAndServer(t, &mockEth{
		eth_call: func(ctx context.Context, t ethsigner.Transaction, s string) (pldtypes.HexBytes, error) {
			return nil, nil
		},
	})
	defer done()
	ec := ecf.HTTPClient().(*ethClient)
	ec.keymgr = nil
	getWidgets := ec.MustABIJSON(testABIJSON).MustFunction("newWidget") // no return value

	to := ethtypes.MustNewAddress("0xD9E54Ba3F1419e6AC71A795d819fdBAE883A6575")

	res, err := getWidgets.R(ctx).Input(`[["0xD9E54Ba3F1419e6AC71A795d819fdBAE883A6575",123,[]]]`).Signer("0xD9E54Ba3F1419e6AC71A795d819fdBAE883A6575").To(to).CallResult()
	assert.NoError(t, err)
	assert.Equal(t, `{}`, res.JSON())
}

func TestCallFunctionNoResolveBadAddr(t *testing.T) {
	ctx, ecf, done := newTestClientAndServer(t, &mockEth{
		eth_call: func(ctx context.Context, t ethsigner.Transaction, s string) (pldtypes.HexBytes, error) {
			return nil, nil
		},
	})
	defer done()
	ec := ecf.HTTPClient().(*ethClient)
	ec.keymgr = nil
	getWidgets := ec.MustABIJSON(testABIJSON).MustFunction("newWidget") // no return value

	to := ethtypes.MustNewAddress("0xD9E54Ba3F1419e6AC71A795d819fdBAE883A6575")

	_, err := getWidgets.R(ctx).Input(`[["0xD9E54Ba3F1419e6AC71A795d819fdBAE883A6575",123,[]]]`).Signer("not.an.address").To(to).CallResult()
	assert.Regexp(t, "bad address", err)
}

func TestSignAndSendMissingFrom(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_call: func(ctx context.Context, t ethsigner.Transaction, s string) (pldtypes.HexBytes, error) {
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

	_, err = getWidgets.R(ctx).To(to).Output("supplied").EstimateGas()
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

	err = req.Input(pldtypes.RawJSON(`{
		"widget": {
			"id":       "0xfd33700f0511abb60ff31a8a533854db90b0a32a",
			"sku":      "1122334455",
			"features": ["shiny", "spinny"]
		}
	}`)).BuildCallData()
	require.NoError(t, err)
	assert.NotEmpty(t, req.TX().Data)

	err = req.Input(&newWidgetInput{
		Widget: widget{
			ID:       *ethtypes.MustNewAddress("0x9fF786fEf6742c066c5c0d7b12d264C7b390c37b"),
			SKU:      *ethtypes.NewHexInteger64(12345),
			Features: []string{},
		},
	}).BuildCallData()
	require.NoError(t, err)
	assert.NotEmpty(t, req.TX().Data)

	inMap := map[string]any{
		"widget": map[string]any{
			"id":       "0x9fF786fEf6742c066c5c0d7b12d264C7b390c37b",
			"sku":      12345,
			"features": []string{},
		},
	}
	err = req.Input(inMap).BuildCallData()
	require.NoError(t, err)
	assert.NotEmpty(t, req.TX().Data)

	assert.NotNil(t, newWidget.ABI())
	cv, err := newWidget.ABIEntry().Inputs.ParseExternalData(inMap)
	require.NoError(t, err)
	err = req.Input(cv).BuildCallData()
	require.NoError(t, err)
	assert.NotEmpty(t, req.TX().Data)
}

func TestInvokeConstructor(t *testing.T) {

	fakeBytecode := ([]byte)(`some_bytes`)

	var testABI ABIClient
	var key1 string
	ctx, ecf, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionCount: func(ctx context.Context, a pldtypes.EthAddress, block string) (pldtypes.HexUint64, error) {
			assert.Equal(t, key1, a.String())
			assert.Equal(t, "latest", block)
			return 10, nil
		},
		eth_estimateGas: func(ctx context.Context, tx ethsigner.Transaction) (pldtypes.HexUint64, error) {
			return 100000, nil
		},
		eth_sendRawTransaction: func(ctx context.Context, rawTX pldtypes.HexBytes) (pldtypes.HexBytes, error) {
			addr, tx, err := ethsigner.RecoverRawTransaction(ctx, ethtypes.HexBytes0xPrefix(rawTX), 12345)
			require.NoError(t, err)
			assert.Equal(t, key1, addr.String())
			assert.Equal(t, int64(10), tx.Nonce.Int64())
			assert.Equal(t, int64(200000 /* 2x estimate */), tx.GasLimit.Int64())

			cv, err := testABI.ABI().Constructor().Inputs.DecodeABIData(tx.Data, len(fakeBytecode))
			require.NoError(t, err)
			jsonData, err := pldtypes.StandardABISerializer().SerializeJSON(cv)
			require.NoError(t, err)
			assert.JSONEq(t, `{
				"supplier": "0xfb75836dc4130a9462fafd8fe96c8ee376e2f32e"
			}`, string(jsonData))

			hash := sha3.NewLegacyKeccak256()
			_, _ = hash.Write(rawTX)
			return hash.Sum(nil), nil
		},
	})
	defer done()

	_, key1, err := ecf.ecf.keymgr.ResolveKey(ctx, "key1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	testABI = ecf.HTTPClient().MustABIJSON(testABIJSON)
	req := testABI.MustConstructor(fakeBytecode).R(ctx).
		Signer("key1").
		Input(`{"supplier": "0xFB75836Dc4130a9462FAFD8fe96c8Ee376e2f32e"}`)
	txHash, err := req.SignAndSend()

	require.NoError(t, err)
	assert.NotEmpty(t, txHash)

}

func TestInvokeNewWidgetCustomError(t *testing.T) {

	var testABI abi.ABI
	err := json.Unmarshal(testABIJSON, &testABI)
	assert.NoError(t, err)
	errData, err := testABI.Errors()["WidgetError"].EncodeCallDataJSON([]byte(`{"sku": 1122334455, "issue": "not widgety enough"}`))
	assert.NoError(t, err)

	ctx, ecf, done := newTestClientAndServer(t, &mockEth{
		eth_estimateGas: func(ctx context.Context, tx ethsigner.Transaction) (pldtypes.HexUint64, error) {
			return 0, fmt.Errorf("pop")
		},
		eth_callErr: func(ctx context.Context, req *rpcclient.RPCRequest) *rpcclient.RPCResponse {
			return &rpcclient.RPCResponse{
				JSONRpc: "2.0",
				ID:      req.ID,
				Error: &rpcclient.RPCError{
					Code:    int64(rpcclient.RPCCodeInternalError),
					Message: "reverted",
					Data:    pldtypes.JSONString(pldtypes.HexBytes(errData)),
				},
			}
		},
	})
	defer done()

	fakeContractAddr := ethtypes.MustNewAddress("0xCC3b61E636B395a4821Df122d652820361FF26f1")

	widgetA := &widget{
		ID:       *ethtypes.MustNewAddress("0xFd33700f0511AbB60FF31A8A533854dB90B0a32A"),
		SKU:      *ethtypes.NewHexInteger64(1122334455),
		Features: []string{"shiny", "spinny"},
	}
	req := ecf.HTTPClient().MustABIJSON(testABIJSON).MustFunction("newWidget").R(ctx).
		To(fakeContractAddr).
		Input(&newWidgetInput{
			Widget: *widgetA,
		})
	_, err = req.CallResult()
	assert.EqualError(t, err, `PD011513: Reverted: WidgetError("1122334455","not widgety enough")`)
	_, err = req.EstimateGas()
	assert.EqualError(t, err, `PD011513: Reverted: WidgetError("1122334455","not widgety enough")`)

	// Check we can override the options if we wish, disabling ability to decode the errors
	_, err = req.CallOptions(WithErrorsFrom(abi.ABI{})).CallResult()
	assert.EqualError(t, err, `PD011513: Reverted: 0xf852c6da0000000000000000000000000000000000000000000000000000000042e576f7000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000126e6f74207769646765747920656e6f7567680000000000000000000000000000`)

}

func TestNewWSOk(t *testing.T) {

	expected := pldtypes.RandBytes32()
	ctx, ecf, done := newTestClientAndServer(t, &mockEth{
		eth_sendRawTransaction: func(ctx context.Context, rawTX pldtypes.HexBytes) (pldtypes.HexBytes, error) {
			return pldtypes.HexBytes(expected[:]), nil
		},
	})

	wsc, err := ecf.NewWS()
	require.NoError(t, err)
	res, err := wsc.SendRawTransaction(ctx, []byte{})
	require.NoError(t, err)
	assert.Equal(t, expected.String(), res.String())

	done()
	_, err = ecf.NewWS()
	require.Regexp(t, "PD021103", err)

}
