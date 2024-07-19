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

package rpcserver

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/iotest"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-signer/mocks/ethsignermocks"
	"github.com/hyperledger/firefly-signer/mocks/rpcbackendmocks"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSignAndSendTransactionWithNonce(t *testing.T) {

	url, s, done := newTestServer(t)
	defer done()
	s.chainID = 1

	w := s.wallet.(*ethsignermocks.Wallet)
	w.On("Initialize", mock.Anything).Return(nil)
	w.On("Sign", mock.Anything, mock.MatchedBy(func(txn *ethsigner.Transaction) bool {
		return txn.Nonce.BigInt().Int64() == 0x24
	}), int64(1) /* chain ID */).Return([]byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // fake signed payload
	}, nil)

	bm := s.backend.(*rpcbackendmocks.Backend)
	bm.On("SyncRequest", mock.Anything, mock.MatchedBy(func(rpcReq *rpcbackend.RPCRequest) bool {
		return rpcReq.Method == "eth_sendRawTransaction" && rpcReq.ID.String() == `234`
	})).Return(&rpcbackend.RPCResponse{
		JSONRpc: "2.0",
		ID:      fftypes.JSONAnyPtr(`234`),
		Result:  fftypes.JSONAnyPtr(`"0x61ca9c99c1d752fb3bda568b8566edf33ba93585c64a970566e6dfb540a5cbc1"`),
	}, nil)

	err := s.Start()
	assert.NoError(t, err)

	res, err := http.Post(url, "application/json", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": 234,
		"method": "eth_sendTransaction",
		"params": [{
			"from": "0xfb075bb99f2aa4c49955bf703509a227d7a12248",
			"gas": "0x2b13d",
			"data": "0xa0712d680000000000000000000000000000000000000000000000000000000000000001",
			"maxFeePerGas": "0x4e58be5c3c",
			"maxPriorityFeePerGas": "0x59682f00",
			"nonce": "0x24",
			"to": "0x3c99f2a4b366d46bcf2277639a135a6d1288eceb",
			"value": "0x8e1bc9bf040000"
		}]
	}`)))
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)

	b, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, string(b), `{
		"jsonrpc": "2.0",
		"id": 234,
		"result": "0x61ca9c99c1d752fb3bda568b8566edf33ba93585c64a970566e6dfb540a5cbc1"
	}`)

	w.AssertExpectations(t)
	bm.AssertExpectations(t)

}

func TestSignAndSendTransactionWithoutNonce(t *testing.T) {

	url, s, done := newTestServer(t)
	defer done()
	s.chainID = 1

	w := s.wallet.(*ethsignermocks.Wallet)
	w.On("Initialize", mock.Anything).Return(nil)
	w.On("Sign", mock.Anything, mock.MatchedBy(func(txn *ethsigner.Transaction) bool {
		return txn.Nonce.BigInt().Int64() == 12345
	}), int64(1) /* chain ID */).Return([]byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // fake signed payload
	}, nil)

	bm := s.backend.(*rpcbackendmocks.Backend)
	bm.On("CallRPC", mock.Anything, mock.Anything, "eth_getTransactionCount", mock.MatchedBy(
		func(a *ethtypes.Address0xHex) bool {
			return a.String() == "0xfb075bb99f2aa4c49955bf703509a227d7a12248"
		},
	), "pending").Run(func(args mock.Arguments) {
		hi := args[1].(**ethtypes.HexInteger)
		*hi = (*ethtypes.HexInteger)(big.NewInt(12345))
	}).Return(nil)
	bm.On("SyncRequest", mock.Anything, mock.MatchedBy(func(rpcReq *rpcbackend.RPCRequest) bool {
		return rpcReq.Method == "eth_sendRawTransaction" && rpcReq.ID.String() == `234`
	})).Return(&rpcbackend.RPCResponse{
		JSONRpc: "2.0",
		ID:      fftypes.JSONAnyPtr(`234`),
		Result:  fftypes.JSONAnyPtr(`"0x61ca9c99c1d752fb3bda568b8566edf33ba93585c64a970566e6dfb540a5cbc1"`),
	}, nil)

	err := s.Start()
	assert.NoError(t, err)

	res, err := http.Post(url, "application/json", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": 234,
		"method": "eth_sendTransaction",
		"params": [{
			"from": "0xfb075bb99f2aa4c49955bf703509a227d7a12248",
			"gas": "0x2b13d",
			"data": "0xa0712d680000000000000000000000000000000000000000000000000000000000000001",
			"to": "0x3c99f2a4b366d46bcf2277639a135a6d1288eceb"
		}]
	}`)))
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)

	b, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, string(b), `{
		"jsonrpc": "2.0",
		"id": 234,
		"result": "0x61ca9c99c1d752fb3bda568b8566edf33ba93585c64a970566e6dfb540a5cbc1"
	}`)

	w.AssertExpectations(t)
	bm.AssertExpectations(t)

}

func TestServeJSONRPCFail(t *testing.T) {

	url, s, done := newTestServer(t)
	defer done()
	s.chainID = 1

	w := s.wallet.(*ethsignermocks.Wallet)
	w.On("Initialize", mock.Anything).Return(nil)

	bm := s.backend.(*rpcbackendmocks.Backend)
	bm.On("SyncRequest", mock.Anything, mock.MatchedBy(func(rpcReq *rpcbackend.RPCRequest) bool {
		return rpcReq.Method == "eth_rpc1" && rpcReq.ID.String() == `1`
	})).Return(&rpcbackend.RPCResponse{
		JSONRpc: "2.0",
		ID:      fftypes.JSONAnyPtr(`1`),
		Error: &rpcbackend.RPCError{
			Code:    int64(rpcbackend.RPCCodeInternalError),
			Message: "error 1",
		},
	}, fmt.Errorf("pop"))

	err := s.Start()
	assert.NoError(t, err)

	res, err := http.Post(url, "application/json", bytes.NewReader([]byte(`
		{
			"jsonrpc": "2.0",
			"id": 1,
			"method": "eth_rpc1"
		}
	`)))
	assert.NoError(t, err)
	assert.Equal(t, 500, res.StatusCode)

	b, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, string(b), `
		{
			"jsonrpc": "2.0",
			"id": 1,
			"error": {
				"code": -32603,
				"message": "error 1"	
			}
		}
	`)

	bm.AssertExpectations(t)

}

func TestServeJSONRPCBatchOK(t *testing.T) {

	url, s, done := newTestServer(t)
	defer done()
	s.chainID = 1

	w := s.wallet.(*ethsignermocks.Wallet)
	w.On("Initialize", mock.Anything).Return(nil)

	bm := s.backend.(*rpcbackendmocks.Backend)
	bm.On("SyncRequest", mock.Anything, mock.MatchedBy(func(rpcReq *rpcbackend.RPCRequest) bool {
		return rpcReq.Method == "eth_rpc1" && rpcReq.ID.String() == `1`
	})).Return(&rpcbackend.RPCResponse{
		JSONRpc: "2.0",
		ID:      fftypes.JSONAnyPtr(`1`),
		Result:  fftypes.JSONAnyPtr(`"result 1"`),
	}, nil)
	bm.On("SyncRequest", mock.Anything, mock.MatchedBy(func(rpcReq *rpcbackend.RPCRequest) bool {
		return rpcReq.Method == "eth_rpc2" && rpcReq.ID.String() == `2`
	})).Return(&rpcbackend.RPCResponse{
		JSONRpc: "2.0",
		ID:      fftypes.JSONAnyPtr(`2`),
		Result:  fftypes.JSONAnyPtr(`"result 2"`),
	}, nil)
	bm.On("SyncRequest", mock.Anything, mock.MatchedBy(func(rpcReq *rpcbackend.RPCRequest) bool {
		return rpcReq.Method == "eth_rpc3" && rpcReq.ID.String() == `3`
	})).Return(&rpcbackend.RPCResponse{
		JSONRpc: "2.0",
		ID:      fftypes.JSONAnyPtr(`3`),
		Result:  fftypes.JSONAnyPtr(`"result 3"`),
	}, nil)

	err := s.Start()
	assert.NoError(t, err)

	res, err := http.Post(url, "application/json", bytes.NewReader([]byte(`[
		{
			"jsonrpc": "2.0",
			"id": 1,
			"method": "eth_rpc1"
		},
		{
			"jsonrpc": "2.0",
			"id": 2,
			"method": "eth_rpc2"
		},
		{
			"jsonrpc": "2.0",
			"id": 3,
			"method": "eth_rpc3"
		}
	]`)))
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)

	b, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, string(b), `[
		{
			"jsonrpc": "2.0",
			"id": 1,
			"result": "result 1"
		},
		{
			"jsonrpc": "2.0",
			"id": 2,
			"result": "result 2"
		},
		{
			"jsonrpc": "2.0",
			"id": 3,
			"result": "result 3"
		}
	]`)

	bm.AssertExpectations(t)

}

func TestServeJSONRPCBatchOneFailed(t *testing.T) {

	url, s, done := newTestServer(t)
	defer done()
	s.chainID = 1

	w := s.wallet.(*ethsignermocks.Wallet)
	w.On("Initialize", mock.Anything).Return(nil)

	bm := s.backend.(*rpcbackendmocks.Backend)
	bm.On("SyncRequest", mock.Anything, mock.MatchedBy(func(rpcReq *rpcbackend.RPCRequest) bool {
		return rpcReq.Method == "eth_rpc1" && rpcReq.ID.String() == `1`
	})).Return(&rpcbackend.RPCResponse{
		JSONRpc: "2.0",
		ID:      fftypes.JSONAnyPtr(`1`),
		Result:  fftypes.JSONAnyPtr(`"result 1"`),
	}, nil)
	bm.On("SyncRequest", mock.Anything, mock.MatchedBy(func(rpcReq *rpcbackend.RPCRequest) bool {
		return rpcReq.Method == "eth_rpc2" && rpcReq.ID.String() == `2`
	})).Return(&rpcbackend.RPCResponse{
		JSONRpc: "2.0",
		ID:      fftypes.JSONAnyPtr(`2`),
		Error: &rpcbackend.RPCError{
			Code:    int64(rpcbackend.RPCCodeInternalError),
			Message: "error 2",
		},
	}, fmt.Errorf("pop"))

	err := s.Start()
	assert.NoError(t, err)

	res, err := http.Post(url, "application/json", bytes.NewReader([]byte(`[
		{
			"jsonrpc": "2.0",
			"id": 1,
			"method": "eth_rpc1"
		},
		{
			"jsonrpc": "2.0",
			"id": 2,
			"method": "eth_rpc2"
		}
	]`)))
	assert.NoError(t, err)
	assert.Equal(t, 500, res.StatusCode)

	b, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, string(b), `[
		{
			"jsonrpc": "2.0",
			"id": 1,
			"result": "result 1"
		},
		{
			"jsonrpc": "2.0",
			"id": 2,
			"error": {
				"code": -32603,
				"message": "error 2"	
			}
		}
	]`)

	bm.AssertExpectations(t)

}

func TestServeJSONRPCBatchBadArray(t *testing.T) {

	url, s, done := newTestServer(t)
	defer done()
	s.chainID = 1

	w := s.wallet.(*ethsignermocks.Wallet)
	w.On("Initialize", mock.Anything).Return(nil)

	err := s.Start()
	assert.NoError(t, err)

	res, err := http.Post(url, "application/json", bytes.NewReader([]byte(`[`)))
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)

	b, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, string(b), `
		{
			"jsonrpc": "2.0",
			"id": 1,
			"error": {
				"code": -32600,
				"message": "FF22018: Invalid request data"
			}
		}
	`)

}

func TestServeJSONRPCBatchEmptyData(t *testing.T) {

	url, s, done := newTestServer(t)
	defer done()
	s.chainID = 1

	w := s.wallet.(*ethsignermocks.Wallet)
	w.On("Initialize", mock.Anything).Return(nil)

	err := s.Start()
	assert.NoError(t, err)

	res, err := http.Post(url, "application/json", bytes.NewReader([]byte(``)))
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)

	b, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, string(b), `
		{
			"jsonrpc": "2.0",
			"id": 1,
			"error": {
				"code": -32600,
				"message": "FF22018: Invalid request data"	
			}
		}
	`)

}

func TestServeJSONRPCBatchBadJSON(t *testing.T) {

	url, s, done := newTestServer(t)
	defer done()
	s.chainID = 1

	w := s.wallet.(*ethsignermocks.Wallet)
	w.On("Initialize", mock.Anything).Return(nil)

	err := s.Start()
	assert.NoError(t, err)

	res, err := http.Post(url, "application/json", bytes.NewReader([]byte(``)))
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)

	b, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, string(b), `
		{
			"jsonrpc": "2.0",
			"id": 1,
			"error": {
				"code": -32600,
				"message": "FF22018: Invalid request data"	
			}
		}
	`)

}
func TestRPCHandlerReadFail(t *testing.T) {

	_, s, done := newTestServer(t)
	defer done()

	w := httptest.NewRecorder()
	s.rpcHandler(w, httptest.NewRequest(http.MethodPost, "/", iotest.ErrReader(fmt.Errorf("pop"))))

	assert.Equal(t, 400, w.Result().StatusCode)

}
