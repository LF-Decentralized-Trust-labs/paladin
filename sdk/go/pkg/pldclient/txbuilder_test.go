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

package pldclient

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/solutils"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

var testABI = New().TxBuilder(context.Background()).ABIJSON(testABIJSON).GetABI()

func getTransaction(t *testing.T, txID uuid.UUID) testRPCMethod {
	return testRPCMethod{
		name: "ptx_getTransaction",
		handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
			var suppliedID uuid.UUID
			if err := json.Unmarshal(rpcReq.Params[0], &suppliedID); err != nil {
				return errorResponse(rpcReq.ID, err)
			}
			require.Equal(t, txID, suppliedID)
			return successResponse(rpcReq.ID, pldtypes.RawJSON(`{
				"id": "`+txID.String()+`"
			}`))
		},
	}
}

func TestBuildAndSubmitPublicTXHTTPOk(t *testing.T) {
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	txHash := pldtypes.RandBytes32()

	methods := []testRPCMethod{
		{
			name: "ptx_sendTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var tx pldapi.TransactionInput
				err := json.Unmarshal(rpcReq.Params[0], &tx)
				if err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.JSONEq(t, `{
					"widget": {
						"id": "0x172ea50b3535721154ae5b368e850825615882bb",
						"sku": "12345",
						"features": ["blue", "round"]
					}
				}`, string(tx.Data))
				require.Equal(t, pldapi.TransactionTypePublic, tx.Type.V())
				require.Equal(t, "newWidget", tx.Function)
				require.Equal(t, contractAddr, tx.To)
				require.Equal(t, "tx.sender", tx.From)
				require.Equal(t, pldtypes.HexUint64(100000), *tx.PublicTxOptions.Gas)
				return successResponse(rpcReq.ID, pldtypes.JSONString(txID.String()))
			},
		},
		getTransaction(t, txID),
		{
			name: "ptx_getTransactionReceipt",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var suppliedID uuid.UUID
				if err := json.Unmarshal(rpcReq.Params[0], &suppliedID); err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.Equal(t, txID, suppliedID)
				return successResponse(rpcReq.ID, pldtypes.RawJSON(`{
					"id": "`+txID.String()+`",
					"transactionHash": "`+txHash.String()+`",
					"success": true
				}`))
			},
		},
	}
	ctx, c, done := newTestClientAndServerHTTP(t, methods...)
	defer done()

	sent := c.ForABI(ctx, testABI).
		Public().
		Function("newWidget").
		Inputs(map[string]any{
			"widget": map[string]any{
				"id":       "0x172EA50B3535721154ae5B368E850825615882BB",
				"sku":      12345,
				"features": []string{"blue", "round"},
			},
		}).
		From("tx.sender").
		To(contractAddr).
		PublicTxOptions(pldapi.PublicTxOptions{
			Gas: confutil.P(pldtypes.HexUint64(100000)),
		}).
		Send()

	res := sent.Wait(100 * time.Millisecond)
	require.NoError(t, res.Error())
	require.Equal(t, txHash, *res.TransactionHash())
	require.Equal(t, txHash, *res.Receipt().TransactionHash)
	require.Equal(t, txID, res.ID())

	// Check directly getting TX and receipt
	tx, err := sent.GetTransaction()
	require.NoError(t, err)
	require.Equal(t, txID, *tx.ID)

	// Check directly getting TX and receipt
	receipt, err := sent.GetReceipt()
	require.NoError(t, err)
	require.Equal(t, txID, receipt.ID)

}

func TestBuildAndSubmitPrivateTXHTTPRevert(t *testing.T) {
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()

	methods := []testRPCMethod{
		{
			name: "ptx_sendTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var tx pldapi.TransactionInput
				err := json.Unmarshal(rpcReq.Params[0], &tx)
				if err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.JSONEq(t, `{
				"widget": {
					"id": "0x172ea50b3535721154ae5b368e850825615882bb",
					"sku": "12345",
					"features": ["blue", "round"]
				}
			}`, string(tx.Data))
				require.Equal(t, pldapi.TransactionTypePrivate, tx.Type.V())
				require.Equal(t, "neeto", tx.Domain)
				require.Equal(t, "newWidget", tx.Function)
				require.Equal(t, contractAddr, tx.To)
				require.Equal(t, "tx.sender", tx.From)
				require.Equal(t, pldtypes.HexUint64(100000), *tx.PublicTxOptions.Gas)
				return successResponse(rpcReq.ID, pldtypes.JSONString(txID.String()))
			},
		},
		getTransaction(t, txID),
		{
			name: "ptx_getTransactionReceipt",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var suppliedID uuid.UUID
				if err := json.Unmarshal(rpcReq.Params[0], &suppliedID); err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.Equal(t, txID, suppliedID)
				return successResponse(rpcReq.ID, pldtypes.RawJSON(`{
				"id": "`+txID.String()+`",
				"success": false,
				"failureMessage": "something went wrong"
			}`))
			},
		}}

	ctx, c, done := newTestClientAndServerHTTP(t, methods...)
	defer done()

	sent := c.ForABI(ctx, testABI).
		Private().Domain("neeto").
		Function("newWidget").
		Inputs(map[string]any{
			"widget": map[string]any{
				"id":       "0x172EA50B3535721154ae5B368E850825615882BB",
				"sku":      12345,
				"features": []string{"blue", "round"},
			},
		}).
		From("tx.sender").
		To(contractAddr).
		PublicTxOptions(pldapi.PublicTxOptions{
			Gas: confutil.P(pldtypes.HexUint64(100000)),
		}).
		Send()

	res := sent.Wait(100 * time.Millisecond)
	assert.EqualError(t, res.Error(), "something went wrong")
	assert.Nil(t, res.TransactionHash())
}

func TestBuildAndPreparePrivateTXHTTPOk(t *testing.T) {
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()

	methods := []testRPCMethod{
		{
			name: "ptx_prepareTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var tx pldapi.TransactionInput
				err := json.Unmarshal(rpcReq.Params[0], &tx)
				if err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.JSONEq(t, `{
					"widget": {
						"id": "0x172ea50b3535721154ae5b368e850825615882bb",
						"sku": "12345",
						"features": ["blue", "round"]
					}
				}`, string(tx.Data))
				require.Equal(t, pldapi.TransactionTypePrivate, tx.Type.V())
				require.Equal(t, "neeto", tx.Domain)
				require.Equal(t, "newWidget", tx.Function)
				require.Equal(t, contractAddr, tx.To)
				require.Equal(t, "tx.sender", tx.From)
				require.Equal(t, pldtypes.HexUint64(100000), *tx.PublicTxOptions.Gas)
				return successResponse(rpcReq.ID, pldtypes.JSONString(txID.String()))
			},
		},
		getTransaction(t, txID),
		{
			name: "ptx_getPreparedTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var suppliedID uuid.UUID
				if err := json.Unmarshal(rpcReq.Params[0], &suppliedID); err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.Equal(t, txID, suppliedID)
				return successResponse(rpcReq.ID, pldtypes.RawJSON(`{
					"id": "`+txID.String()+`",
					"transaction": {
						"idempotencyKey": "tx1"
					}
				}`))
			},
		},
	}

	ctx, c, done := newTestClientAndServerHTTP(t, methods...)
	defer done()

	prepare := c.ForABI(ctx, testABI).
		Private().Domain("neeto").
		Function("newWidget").
		Inputs(map[string]any{
			"widget": map[string]any{
				"id":       "0x172EA50B3535721154ae5B368E850825615882BB",
				"sku":      12345,
				"features": []string{"blue", "round"},
			},
		}).
		From("tx.sender").
		To(contractAddr).
		PublicTxOptions(pldapi.PublicTxOptions{
			Gas: confutil.P(pldtypes.HexUint64(100000)),
		}).
		Prepare()

	res := prepare.Wait(100 * time.Millisecond)
	require.NoError(t, res.Error())
	assert.Equal(t, txID, *prepare.ID())
	assert.Equal(t, txID, res.ID())
	assert.Equal(t, "tx1", res.PreparedTransaction().Transaction.IdempotencyKey)

	// Check directly getting TX and receipt
	tx, err := prepare.GetTransaction()
	require.NoError(t, err)
	require.Equal(t, txID, *tx.ID)

	// Check directly getting TX and receipt
	prepared, err := prepare.GetPreparedTransaction()
	require.NoError(t, err)
	require.Equal(t, txID, prepared.ID)

}

func TestBuildAndSubmitPublicCallHTTPOk(t *testing.T) {
	contractAddr := pldtypes.RandAddress()

	ctx, c, done := newTestClientAndServerHTTP(t, testRPCMethod{
		name: "ptx_call",
		handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
			var tx pldapi.TransactionCall
			err := json.Unmarshal(rpcReq.Params[0], &tx)
			if err != nil {
				return errorResponse(rpcReq.ID, err)
			}
			require.JSONEq(t, `["12345"]`, string(tx.Data))
			require.Equal(t, pldapi.TransactionTypePublic, tx.Type.V())
			require.Equal(t, "getWidgets", tx.Function)
			require.Equal(t, contractAddr, tx.To)
			require.Equal(t, "latest", tx.Block.String())
			return successResponse(rpcReq.ID, pldtypes.RawJSON(`[[
				"0x172ea50b3535721154ae5b368e850825615882bb",
				"12345",
				["blue", "round"]
			]]`))
		},
	})
	defer done()

	var widgets []any
	called := c.ForABI(ctx, testABI).
		Public().
		Function("getWidgets").
		To(contractAddr).
		Inputs([]int{12345}).
		Outputs(&widgets).
		DataFormat("mode=array").
		PublicCallOptions(pldapi.PublicCallOptions{
			Block: "latest",
		}).
		Call()
	require.NoError(t, called)
	assert.Equal(t, []any{
		[]any{
			"0x172ea50b3535721154ae5b368e850825615882bb",
			"12345",
			[]any{"blue", "round"},
		},
	}, widgets)

}

func TestBuildAndSubmitPublicDeployWSFail(t *testing.T) {
	bytecode := pldtypes.HexBytes(pldtypes.RandBytes(64))
	txID := uuid.New()

	methods := []testRPCMethod{
		{
			name: "ptx_sendTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var tx pldapi.TransactionInput
				err := json.Unmarshal(rpcReq.Params[0], &tx)
				if err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.JSONEq(t, `{"supplier": "0x172ea50b3535721154ae5b368e850825615882bb"}`, string(tx.Data))
				require.Equal(t, bytecode, tx.Bytecode)
				return successResponse(rpcReq.ID, pldtypes.JSONString(txID.String()))
			},
		}, {
			name: "ptx_getTransactionReceipt",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var suppliedID uuid.UUID
				if err := json.Unmarshal(rpcReq.Params[0], &suppliedID); err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.Equal(t, txID, suppliedID)
				return errorResponse(rpcReq.ID, fmt.Errorf("server throws an error"))
			},
		},
	}

	ctx, c, done := newTestClientAndServerHTTP(t, methods...)
	defer done()

	cancellable, cancelCtx := context.WithCancel(ctx)

	sent := c.ReceiptPollingInterval(1 * time.Millisecond).
		TxBuilder(cancellable).
		Public().
		SolidityBuild(&solutils.SolidityBuild{
			ABI:      testABI,
			Bytecode: bytecode,
		}).
		From("tx.sender").
		Inputs(`{"supplier": "0x172EA50B3535721154ae5B368E850825615882BB"}`).
		Send()

	res := sent.Wait(25 * time.Millisecond)
	require.Regexp(t, "PD020216.*timed out.*server throws an error", res.Error())
	require.Nil(t, res.TransactionHash())

	cancelCtx()
	res = sent.Wait(1 * time.Minute)
	require.Regexp(t, "PD020000", res.Error())
	require.Nil(t, res.TransactionHash())

}

func TestBuildAndPreparePrivateHTTPFail(t *testing.T) {
	bytecode := pldtypes.HexBytes(pldtypes.RandBytes(64))
	txID := uuid.New()
	methods := []testRPCMethod{
		{
			name: "ptx_prepareTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var tx pldapi.TransactionInput
				err := json.Unmarshal(rpcReq.Params[0], &tx)
				if err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.JSONEq(t, `{"supplier": "0x172ea50b3535721154ae5b368e850825615882bb"}`, string(tx.Data))
				require.Equal(t, bytecode, tx.Bytecode)
				return successResponse(rpcReq.ID, pldtypes.JSONString(txID.String()))
			},
		},
		{
			name: "ptx_getPreparedTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var suppliedID uuid.UUID
				if err := json.Unmarshal(rpcReq.Params[0], &suppliedID); err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.Equal(t, txID, suppliedID)
				return errorResponse(rpcReq.ID, fmt.Errorf("server throws an error"))
			},
		},
	}
	ctx, c, done := newTestClientAndServerWebSockets(t, methods...)
	defer done()

	cancellable, cancelCtx := context.WithCancel(ctx)

	sent := c.ReceiptPollingInterval(1 * time.Millisecond).
		TxBuilder(cancellable).
		Public().
		SolidityBuild(&solutils.SolidityBuild{
			ABI:      testABI,
			Bytecode: bytecode,
		}).
		From("tx.sender").
		Inputs(`{"supplier": "0x172EA50B3535721154ae5B368E850825615882BB"}`).
		Prepare()

	res := sent.Wait(25 * time.Millisecond)
	require.Regexp(t, "PD020216.*timed out.*server throws an error", res.Error())
	require.Nil(t, res.PreparedTransaction())

	cancelCtx()
	res = sent.Wait(1 * time.Minute)
	require.Regexp(t, "PD020000", res.Error())
	require.Nil(t, res.PreparedTransaction())

}

func TestSendUnconnectedFail(t *testing.T) {

	res := New().TxBuilder(context.Background()).
		Public().
		From("tx.sender").
		Function("someFunc").
		To(pldtypes.RandAddress()).
		Inputs(`{"supplier": "0x172EA50B3535721154ae5B368E850825615882BB"}`).
		Send()
	require.Regexp(t, "PD020210", res.Error())

}

func TestIdempotentSubmit(t *testing.T) {
	txID := uuid.New()
	methods := []testRPCMethod{
		{
			name: "ptx_sendTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				return errorResponse(rpcReq.ID, fmt.Errorf("PD012220: key clash" /* note important error code in Paladin */))
			},
		},
		{
			name: "ptx_prepareTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				return errorResponse(rpcReq.ID, fmt.Errorf("PD012220: key clash" /* note important error code in Paladin */))
			},
		},
		{
			name: "ptx_getTransactionByIdempotencyKey",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var suppliedID string
				if err := json.Unmarshal(rpcReq.Params[0], &suppliedID); err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.Equal(t, "tx.12345", suppliedID)
				return successResponse(rpcReq.ID, pldtypes.RawJSON(`{
					"id": "`+txID.String()+`"
				}`))
			},
		},
	}
	ctx, c, done := newTestClientAndServerHTTP(t, methods...)
	defer done()

	txb := c.TxBuilder(ctx).
		Private().
		Domain("domain1").
		IdempotencyKey("tx.12345").
		ABIJSON(testABIJSON).
		From("tx.sender").
		Inputs(`{"supplier": "0x172EA50B3535721154ae5B368E850825615882BB"}`)

	send := txb.Send()
	require.NoError(t, send.Error())
	assert.Equal(t, txID, *send.ID())

	prepare := txb.Prepare()
	require.NoError(t, prepare.Error())
	assert.Equal(t, txID, *prepare.ID())

}

func TestDeferFunctionSelectError(t *testing.T) {
	ctx, c, done := newTestClientAndServerHTTP(t)
	defer done()

	res := c.ReceiptPollingInterval(1 * time.Millisecond).
		TxBuilder(ctx).
		Public().
		ABIJSON(testABIJSON).
		Function("wrong").
		To(pldtypes.RandAddress()).
		Send().
		Wait(25 * time.Millisecond)
	require.Regexp(t, "PD020208", res.Error()) // function not found

}

func TestBuildABIDataJSONArray(t *testing.T) {
	ctx, c, done := newTestClientAndServerHTTP(t)
	defer done()

	data, err := c.ReceiptPollingInterval(1 * time.Millisecond).
		TxBuilder(ctx).
		Public().
		ABIJSON(testABIJSON).
		Function("getWidgets(uint256)").
		To(pldtypes.RandAddress()).
		Inputs(`{"sku": 73588229205}`).
		DataFormat("mode=array&number=hex").
		BuildInputDataJSON()
	require.NoError(t, err)
	require.JSONEq(t, `["0x1122334455"]`, string(data))

}

func TestSendNoABI(t *testing.T) {
	txID := uuid.New()
	expectNil := false

	ctx, c, done := newTestClientAndServerHTTP(t, testRPCMethod{
		name: "ptx_sendTransaction",
		handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
			var tx pldapi.TransactionInput
			err := json.Unmarshal(rpcReq.Params[0], &tx)
			if err != nil {
				return errorResponse(rpcReq.ID, err)
			}
			if expectNil {
				require.Nil(t, tx.Data)
			} else {
				require.JSONEq(t, `{"sku": 73588229205}`, string(tx.Data))
			}
			return successResponse(rpcReq.ID, pldtypes.JSONString(txID))
		},
	})
	defer done()

	builder := c.ReceiptPollingInterval(1 * time.Millisecond).
		TxBuilder(ctx).
		Public().
		ABIReference((*pldtypes.Bytes32)(pldtypes.RandBytes(32))).
		Function("getWidgets(uint256)").
		From("tx.sender").
		To(pldtypes.RandAddress())

	res := builder.Inputs(`{"sku": 73588229205}`).Send()
	require.NoError(t, res.Error())
	require.Equal(t, txID, *res.ID())

	res = builder.Inputs([]byte(`{"sku": 73588229205}`)).Send()
	require.NoError(t, res.Error())
	require.Equal(t, txID, *res.ID())

	res = builder.Inputs(map[string]any{"sku": 73588229205}).Send()
	require.NoError(t, res.Error())
	require.Equal(t, txID, *res.ID())

	expectNil = true
	res = builder.Inputs(nil).Send()
	require.NoError(t, res.Error())
	require.Equal(t, txID, *res.ID())
}

func TestBuildBadABIFunction(t *testing.T) {
	ctx, c, done := newTestClientAndServerHTTP(t)
	defer done()

	res := c.ReceiptPollingInterval(1 * time.Millisecond).
		TxBuilder(ctx).
		ABIFunction(&abi.Entry{Type: abi.Function, Inputs: abi.ParameterArray{{Type: "wrongness"}}}).
		Public().
		ABIReference((*pldtypes.Bytes32)(pldtypes.RandBytes(32))).
		Function("getWidgets(uint256)").
		From("tx.sender").
		To(pldtypes.RandAddress()).
		Inputs(`{"sku": 73588229205}`).
		Send()
	assert.Regexp(t, "FF22025", res.Error())
}

func TestErrChainingTXAndReceipt(t *testing.T) {

	builder := New().ForABI(context.Background(), abi.ABI{})

	send := builder.Send()
	require.Regexp(t, "PD020211", send.Error()) // missing public or private

	err := builder.Call()
	require.Regexp(t, "PD020211", err)

	_, err = send.GetTransaction()
	require.Regexp(t, "PD020211", err)

	_, err = send.GetReceipt()
	require.Regexp(t, "PD020211", err)

	prepare := builder.Prepare()
	require.Regexp(t, "PD020211", prepare.Error())
	require.Regexp(t, "PD020211", prepare.Wait(100*time.Microsecond).Error())

	_, err = prepare.GetTransaction()
	require.Regexp(t, "PD020211", err)

	_, err = prepare.GetPreparedTransaction()
	require.Regexp(t, "PD020211", err)

}

func TestBuildBadABIJSON(t *testing.T) {
	ctx, c, done := newTestClientAndServerHTTP(t)
	defer done()

	res := c.ReceiptPollingInterval(1 * time.Millisecond).
		TxBuilder(ctx).
		ABIJSON([]byte(`{!!!! wrong`)).
		Public().
		ABIReference((*pldtypes.Bytes32)(pldtypes.RandBytes(32))).
		Function("getWidgets(uint256)").
		From("tx.sender").
		To(pldtypes.RandAddress()).
		Inputs(`{"sku": 73588229205}`).
		Send()
	assert.Regexp(t, "PD020207", res.Error())
}

func TestGetters(t *testing.T) {

	tx := &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			IdempotencyKey: "tx1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "domain1",
			ABIReference:   confutil.P(pldtypes.RandBytes32()),
			From:           "tx.sender",
			To:             pldtypes.RandAddress(),
			Function:       "function1",
			PublicTxOptions: pldapi.PublicTxOptions{
				Gas: confutil.P(pldtypes.HexUint64(100000)),
			},
		},
		ABI:      abi.ABI{{Type: abi.Constructor}},
		Bytecode: pldtypes.HexBytes(pldtypes.RandBytes(64)),
	}

	// This isn't a valid TX, but we're just testing getters
	b := New().TxBuilder(context.Background()).Wrap(tx).Clone()
	assert.Equal(t, tx.ABI, b.GetABI())
	assert.Equal(t, tx.IdempotencyKey, b.GetIdempotencyKey())
	assert.Equal(t, pldapi.TransactionTypePrivate, b.GetType())
	assert.Equal(t, "domain1", b.GetDomain())
	assert.Same(t, tx.ABIReference, b.GetABIReference())
	assert.Equal(t, "tx.sender", b.GetFrom())
	assert.Equal(t, tx.To, b.GetTo())
	assert.Equal(t, tx.Data, b.GetInputs())
	assert.Equal(t, "function1", b.GetFunction())
	assert.Equal(t, tx.Bytecode, b.GetBytecode())
	assert.Equal(t, tx.PublicTxOptions, b.GetPublicTxOptions())

	require.NotNil(t, b.Client())

	// Check it doesn't change in the round trip
	tx2 := b.BuildTX().TX()
	require.Equal(t, tx, tx2)

	callTX := &pldapi.TransactionCall{
		TransactionInput: *b.BuildTX().TX(),
		DataFormat:       "mode=array",
		PublicCallOptions: pldapi.PublicCallOptions{
			Block: "latest",
		},
	}
	b = b.WrapCall(callTX)
	require.Equal(t, callTX.PublicCallOptions, b.GetPublicCallOptions())
	require.Equal(t, pldtypes.JSONFormatOptions("mode=array"), b.GetDataFormat())
	var result pldtypes.RawJSON
	b.Outputs(&result)
	require.Equal(t, &result, b.GetOutputs())

	tx3 := b.BuildTX().CallTX()
	require.Equal(t, callTX, tx3)
}

func TestBuildCallDataFunction(t *testing.T) {

	builder := New().ForABI(context.Background(), testABI).Function("getWidgets(uint256)")

	type skuInput struct {
		SKU pldtypes.HexUint64 `json:"sku"`
	}

	// A JSON serializable structure
	callData, err := builder.Inputs(&skuInput{SKU: 0x1122334455}).BuildCallData()
	require.NoError(t, err)
	require.Equal(t, "0x4f8989ff0000000000000000000000000000000000000000000000000000001122334455", callData.String())

	// A generic structure serializable to an array
	callData, err = builder.Inputs([]string{"0x1122334455"}).BuildCallData()
	require.NoError(t, err)
	require.Equal(t, "0x4f8989ff0000000000000000000000000000000000000000000000000000001122334455", callData.String())

	// A generic structure serializable to an object
	callData, err = builder.Inputs(map[string]any{"sku": 0x1122334455}).BuildCallData()
	require.NoError(t, err)
	require.Equal(t, "0x4f8989ff0000000000000000000000000000000000000000000000000000001122334455", callData.String())

	// A string JSON array
	callData, err = builder.Inputs(`["0x1122334455"]`).BuildCallData()
	require.NoError(t, err)
	require.Equal(t, "0x4f8989ff0000000000000000000000000000000000000000000000000000001122334455", callData.String())

	// A bytes JSON object
	callData, err = builder.Inputs([]byte(`{"sku": "0x1122334455"}`)).BuildCallData()
	require.NoError(t, err)
	require.Equal(t, "0x4f8989ff0000000000000000000000000000000000000000000000000000001122334455", callData.String())

	// A pre-parsed component value tree ready to go
	cv, err := testABI.Functions()["getWidgets"].Inputs.ParseJSON([]byte(`{"sku": "0x1122334455"}`))
	require.NoError(t, err)
	callData, err = builder.Inputs(cv).BuildCallData()
	require.NoError(t, err)
	require.Equal(t, "0x4f8989ff0000000000000000000000000000000000000000000000000000001122334455", callData.String())

	// Nil when no value is required (default constructor)
	callData, err = New().ForABI(context.Background(), abi.ABI{}).
		Constructor().Inputs(nil).
		Bytecode(pldtypes.MustParseHexBytes("0xfeedbeef")).
		BuildCallData()
	require.NoError(t, err)
	require.Equal(t, "0xfeedbeef", callData.String())

	// Nil when a value is required
	_, err = builder.Inputs(nil).BuildCallData()
	assert.Regexp(t, "PD020203", err)

	// Some broken JSON
	_, err = builder.Inputs(pldtypes.RawJSON(`{!!!! bad json`)).BuildCallData()
	assert.Regexp(t, "PD020200", err)

}

func TestResolveDefinitionNoABI(t *testing.T) {
	_, err := New().TxBuilder(context.Background()).ResolveDefinition()
	assert.Regexp(t, "PD020213", err)
}

func TestNoDomain(t *testing.T) {
	res := New().TxBuilder(context.Background()).Private().Send()
	assert.Regexp(t, "PD020214", res.Error())
}

func TestMissingFunction(t *testing.T) {
	res := New().TxBuilder(context.Background()).
		Private().
		Domain("noto").
		To(pldtypes.RandAddress()).
		Send()
	assert.Regexp(t, "PD020215", res.Error())
}

func TestMissingTo(t *testing.T) {
	res := New().TxBuilder(context.Background()).
		Private().
		Domain("noto").
		Function("someFunc").
		Send()
	assert.Regexp(t, "PD020202", res.Error())
}

func TestIncorrectlyAddingBytecode(t *testing.T) {
	res := New().TxBuilder(context.Background()).
		Private().
		Domain("noto").
		Constructor().
		Bytecode(pldtypes.MustParseHexBytes("0xfeedbeef")).
		Send()
	assert.Regexp(t, "PD020205", res.Error())
}

func TestMissingBytecode(t *testing.T) {
	res := New().TxBuilder(context.Background()).
		Public().
		Constructor().
		Send()
	assert.Regexp(t, "PD020206", res.Error())
}
