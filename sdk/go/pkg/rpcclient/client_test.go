// Copyright © 2022 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rpcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testRPCHander func(rpcReq *RPCRequest) (int, *RPCResponse)

func newTestServer(t *testing.T, rpcHandler testRPCHander) (context.Context, *rpcClient, func()) {

	ctx, cancelCtx := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		var rpcReq *RPCRequest
		err := json.NewDecoder(r.Body).Decode(&rpcReq)
		assert.NoError(t, err)

		status, rpcRes := rpcHandler(rpcReq)
		b := []byte(`{}`)
		if rpcRes != nil {
			b, err = json.Marshal(rpcRes)
			assert.NoError(t, err)
		}
		w.Header().Add("Content-Type", "application/json")
		w.Header().Add("Content-Length", strconv.Itoa(len(b)))
		w.WriteHeader(status)
		_, _ = w.Write(b)

	}))

	c, err := NewHTTPClient(ctx, &pldconf.HTTPClientConfig{
		URL: fmt.Sprintf("http://%s", server.Listener.Addr()),
	})
	assert.NoError(t, err)

	rb := c.(*rpcClient)

	return ctx, rb, func() {
		cancelCtx()
		server.Close()
	}
}

func TestNewHTTPClientError(t *testing.T) {
	_, err := NewHTTPClient(context.Background(), &pldconf.HTTPClientConfig{})
	require.Error(t, err)
}

func TestSyncRequestOK(t *testing.T) {

	rpcRequestBytes := []byte(`{
		"id": 2,
		"method": "eth_getTransactionByHash",
		"params": [
			"0x61ca9c99c1d752fb3bda568b8566edf33ba93585c64a970566e6dfb540a5cbc1"
		]
	}`)

	rpcServerResponseBytes := []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"result": {
			"accessList": [],
			"blockHash": "0x471a236bac44222faf63e3d7808a2a68a704a75ca2f0774f072764867f458268",
			"blockNumber": "0xd536bc",
			"chainId": "0x1",
			"from": "0xfb075bb99f2aa4c49955bf703509a227d7a12248",
			"gas": "0x2b13d",
			"gasPrice": "0x3b6e7f5f09",
			"hash": "0x61ca9c99c1d752fb3bda568b8566edf33ba93585c64a970566e6dfb540a5cbc1",
			"input": "0xa0712d680000000000000000000000000000000000000000000000000000000000000001",
			"maxFeePerGas": "0x4e58be5c3c",
			"maxPriorityFeePerGas": "0x59682f00",
			"nonce": "0x24",
			"r": "0xea6e1513d716146af3a02e1497fbe7fc3b2ffb08ccb4a1bfef4eaa2a122f62df",
			"s": "0xddc23aec20948a55d3e1f8afd29b5570d8d279450a472b55561ef6afe4a07ff",
			"to": "0x3c99f2a4b366d46bcf2277639a135a6d1288eceb",
			"transactionIndex": "0x1d",
			"type": "0x2",
			"v": "0x1",
			"value": "0x8e1bc9bf040000"
		}
	}`)

	var rpcRequest RPCRequest
	err := json.Unmarshal(rpcRequestBytes, &rpcRequest)
	assert.NoError(t, err)

	var rpcServerResponse RPCResponse
	err = json.Unmarshal(rpcServerResponseBytes, &rpcServerResponse)
	assert.NoError(t, err)

	ctx, rb, done := newTestServer(t, func(rpcReq *RPCRequest) (status int, rpcRes *RPCResponse) {
		assert.Equal(t, "2.0", rpcReq.JSONRpc)
		assert.Equal(t, "eth_getTransactionByHash", rpcReq.Method)
		assert.Equal(t, `"000012346"`, rpcReq.ID.String())
		assert.Equal(t, `"0x61ca9c99c1d752fb3bda568b8566edf33ba93585c64a970566e6dfb540a5cbc1"`, rpcReq.Params[0].String())
		rpcServerResponse.ID = rpcReq.ID
		return 200, &rpcServerResponse
	})
	rb.requestCounter = 12345
	defer done()

	rpcRes, err := rb.SyncRequest(ctx, &rpcRequest)
	assert.NoError(t, err)
	assert.Equal(t, `2`, rpcRes.ID.String())
	assert.Equal(t, `0x24`, rpcRes.Result.ToMap()["nonce"])
}

func TestSyncRPCCallOK(t *testing.T) {

	logrus.SetLevel(logrus.TraceLevel)

	ctx, rb, done := newTestServer(t, func(rpcReq *RPCRequest) (status int, rpcRes *RPCResponse) {
		assert.Equal(t, "2.0", rpcReq.JSONRpc)
		assert.Equal(t, "eth_getTransactionCount", rpcReq.Method)
		assert.Equal(t, `"000012346"`, rpcReq.ID.String())
		assert.Equal(t, `"0xfb075bb99f2aa4c49955bf703509a227d7a12248"`, rpcReq.Params[0].String())
		assert.Equal(t, `"pending"`, rpcReq.Params[1].String())
		return 200, &RPCResponse{
			JSONRpc: "2.0",
			ID:      rpcReq.ID,
			Result:  pldtypes.RawJSON(`"0x26"`),
		}
	})
	rb.requestCounter = 12345
	defer done()

	var txCount pldtypes.HexUint64
	err := rb.CallRPC(ctx, &txCount, "eth_getTransactionCount", pldtypes.MustEthAddress("0xfb075bb99f2aa4c49955bf703509a227d7a12248"), "pending")
	assert.Empty(t, err)
	assert.Equal(t, uint64(0x26), txCount.Uint64())
}

func TestSyncRPCCallNullResponse(t *testing.T) {

	ctx, rb, done := newTestServer(t, func(rpcReq *RPCRequest) (status int, rpcRes *RPCResponse) {
		assert.Equal(t, "2.0", rpcReq.JSONRpc)
		assert.Equal(t, "eth_getTransactionReceipt", rpcReq.Method)
		assert.Equal(t, `"000012346"`, rpcReq.ID.String())
		assert.Equal(t, `"0xf44d5387087f61237bdb5132e9cf0f38ab20437128f7291b8df595305a1a8284"`, rpcReq.Params[0].String())
		return 200, &RPCResponse{
			JSONRpc: "2.0",
			ID:      rpcReq.ID,
			Result:  nil,
		}
	})
	rb.requestCounter = 12345
	defer done()

	rpcRes, err := rb.SyncRequest(ctx, &RPCRequest{
		ID:     pldtypes.RawJSON("1"),
		Method: "eth_getTransactionReceipt",
		Params: []pldtypes.RawJSON{
			pldtypes.RawJSON(`"0xf44d5387087f61237bdb5132e9cf0f38ab20437128f7291b8df595305a1a8284"`),
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, `null`, rpcRes.Result.String())
}

func TestSyncRPCCallErrorResponse(t *testing.T) {

	ctx, rb, done := newTestServer(t, func(rpcReq *RPCRequest) (status int, rpcRes *RPCResponse) {
		return 500, &RPCResponse{
			JSONRpc: "2.0",
			ID:      rpcReq.ID,
			Error: &RPCError{
				Message: "pop",
			},
		}
	})
	rb.requestCounter = 12345
	defer done()

	var txCount pldtypes.HexUint64
	err := rb.CallRPC(ctx, &txCount, "eth_getTransactionCount", pldtypes.MustEthAddress("0xfb075bb99f2aa4c49955bf703509a227d7a12248"), "pending")
	assert.Regexp(t, "pop", err)
}

func TestSyncRPCCallBadJSONResponse(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{!!!!`))
	}))
	defer server.Close()

	c, err := NewHTTPClient(context.Background(), &pldconf.HTTPClientConfig{
		URL: server.URL,
	})
	assert.NoError(t, err)

	var txCount pldtypes.HexUint64
	rpcErr := c.CallRPC(context.Background(), &txCount, "eth_getTransactionCount", pldtypes.MustEthAddress("0xfb075bb99f2aa4c49955bf703509a227d7a12248"), "pending")
	assert.Regexp(t, "PD020502", rpcErr)
}

func TestSyncRPCCallFailParseJSONResponse(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"result":"not an object"}`))
	}))
	defer server.Close()

	c, err := NewHTTPClient(context.Background(), &pldconf.HTTPClientConfig{
		URL: server.URL,
	})
	assert.NoError(t, err)

	var mapResult map[string]interface{}
	rpcErr := c.CallRPC(context.Background(), &mapResult, "eth_getTransactionCount", pldtypes.MustEthAddress("0xfb075bb99f2aa4c49955bf703509a227d7a12248"), "pending")
	assert.Regexp(t, "PD020504", rpcErr)
}

func TestSyncRPCCallErrorBadInput(t *testing.T) {

	ctx, rb, done := newTestServer(t, func(rpcReq *RPCRequest) (status int, rpcRes *RPCResponse) { return 500, nil })
	defer done()

	var txCount pldtypes.HexUint64
	err := rb.CallRPC(ctx, &txCount, "test-bad-params", map[bool]bool{false: true})
	assert.Regexp(t, "PD020505", err)
}

func TestSyncRPCCallServerDown(t *testing.T) {

	ctx, rb, done := newTestServer(t, func(rpcReq *RPCRequest) (status int, rpcRes *RPCResponse) { return 500, nil })
	done()

	var txCount pldtypes.HexUint64
	err := rb.CallRPC(ctx, &txCount, "net_version")
	assert.Regexp(t, "PD020502", err)
}

func TestSafeMessageGetter(t *testing.T) {

	assert.Empty(t, (&RPCResponse{}).Message())
}

func TestRPCErrorResponse(t *testing.T) {
	rpcRes := NewRPCErrorResponse(fmt.Errorf("pop"), pldtypes.RawJSON(`"1"`), RPCCodeInternalError)
	assert.Equal(t, &RPCResponse{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(`"1"`),
		Error: &RPCError{
			Code:    -32603,
			Message: "pop",
		},
	}, rpcRes)
}

func TestWrapRPCError(t *testing.T) {
	require.Regexp(t, "pop", WrapRPCError(RPCCodeInternalError, fmt.Errorf("pop")))
}
