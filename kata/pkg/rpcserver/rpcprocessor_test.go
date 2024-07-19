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
	"fmt"
	"testing"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-signer/mocks/ethsignermocks"
	"github.com/hyperledger/firefly-signer/mocks/rpcbackendmocks"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestEthAccountsOK(t *testing.T) {

	_, s, done := newTestServer(t)
	defer done()

	w := s.wallet.(*ethsignermocks.Wallet)
	w.On("GetAccounts", mock.Anything).Return([]*ethtypes.Address0xHex{
		ethtypes.MustNewAddress("0xFB075BB99F2AA4C49955BF703509A227D7A12248"),
	}, nil)

	rpcRes, err := s.processRPC(s.ctx, &rpcbackend.RPCRequest{
		ID:     fftypes.JSONAnyPtr("1"),
		Method: "eth_accounts",
	})
	assert.NoError(t, err)

	assert.Equal(t, `["0xfb075bb99f2aa4c49955bf703509a227d7a12248"]`, rpcRes.Result.String())

}

func TestMissingID(t *testing.T) {

	_, s, done := newTestServer(t)
	defer done()

	_, err := s.processRPC(s.ctx, &rpcbackend.RPCRequest{
		Method: "net_version",
	})
	assert.Regexp(t, "FF22024", err)

}

func TestPersonalAccountsFail(t *testing.T) {

	_, s, done := newTestServer(t)
	defer done()

	w := s.wallet.(*ethsignermocks.Wallet)
	w.On("GetAccounts", mock.Anything).Return(nil, fmt.Errorf("pop"))

	_, err := s.processRPC(s.ctx, &rpcbackend.RPCRequest{
		ID:     fftypes.JSONAnyPtr("1"),
		Method: "personal_accounts",
	})
	assert.Regexp(t, "pop", err)

}

func TestPassthrough(t *testing.T) {

	_, s, done := newTestServer(t)
	defer done()

	bm := s.backend.(*rpcbackendmocks.Backend)
	bm.On("SyncRequest", mock.Anything, mock.MatchedBy(func(rpcReq *rpcbackend.RPCRequest) bool {
		return rpcReq.Method == "net_version"
	})).Return(&rpcbackend.RPCResponse{
		Result: fftypes.JSONAnyPtr(`"0x12345"`),
	}, nil)

	rpcRes, err := s.processRPC(s.ctx, &rpcbackend.RPCRequest{
		ID:     fftypes.JSONAnyPtr("1"),
		Method: "net_version",
	})
	assert.NoError(t, err)

	assert.Equal(t, `"0x12345"`, rpcRes.Result.String())

}

func TestSignMissingParam(t *testing.T) {

	_, s, done := newTestServer(t)
	defer done()

	_, err := s.processRPC(s.ctx, &rpcbackend.RPCRequest{
		ID:     fftypes.JSONAnyPtr("1"),
		Method: "eth_sendTransaction",
	})
	assert.Regexp(t, "FF22019", err)

}

func TestSignBadTX(t *testing.T) {

	_, s, done := newTestServer(t)
	defer done()

	_, err := s.processRPC(s.ctx, &rpcbackend.RPCRequest{
		ID:     fftypes.JSONAnyPtr("1"),
		Method: "eth_sendTransaction",
		Params: []*fftypes.JSONAny{
			fftypes.JSONAnyPtr(`"not an object"`),
		},
	})
	assert.Regexp(t, "FF22023", err)

}

func TestSignMissingFrom(t *testing.T) {

	_, s, done := newTestServer(t)
	defer done()

	_, err := s.processRPC(s.ctx, &rpcbackend.RPCRequest{
		ID:     fftypes.JSONAnyPtr("1"),
		Method: "eth_sendTransaction",
		Params: []*fftypes.JSONAny{
			fftypes.JSONAnyPtr(`{}`),
		},
	})
	assert.Regexp(t, "FF22020", err)

}

func TestSignGetNonceBadAddress(t *testing.T) {

	_, s, done := newTestServer(t)
	defer done()

	bm := s.backend.(*rpcbackendmocks.Backend)
	bm.On("CallRPC", mock.Anything, mock.Anything, "eth_getTransactionCount", mock.Anything, "pending").Return(fmt.Errorf("pop"))

	_, err := s.processRPC(s.ctx, &rpcbackend.RPCRequest{
		ID:     fftypes.JSONAnyPtr("1"),
		Method: "eth_sendTransaction",
		Params: []*fftypes.JSONAny{
			fftypes.JSONAnyPtr(`{
				"from": "bad address"
			}`),
		},
	})
	assert.Regexp(t, "bad address", err)

}

func TestSignGetNonceFail(t *testing.T) {

	_, s, done := newTestServer(t)
	defer done()

	bm := s.backend.(*rpcbackendmocks.Backend)
	bm.On("CallRPC", mock.Anything, mock.Anything, "eth_getTransactionCount", mock.Anything, "pending").Return(&rpcbackend.RPCError{Message: "pop"})

	_, err := s.processRPC(s.ctx, &rpcbackend.RPCRequest{
		ID:     fftypes.JSONAnyPtr("1"),
		Method: "eth_sendTransaction",
		Params: []*fftypes.JSONAny{
			fftypes.JSONAnyPtr(`{
				"from": "0xfb075bb99f2aa4c49955bf703509a227d7a12248"
			}`),
		},
	})
	assert.Regexp(t, "pop", err)

}

func TestSignSignFail(t *testing.T) {

	_, s, done := newTestServer(t)
	defer done()

	w := s.wallet.(*ethsignermocks.Wallet)
	w.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))

	_, err := s.processRPC(s.ctx, &rpcbackend.RPCRequest{
		ID:     fftypes.JSONAnyPtr("1"),
		Method: "eth_sendTransaction",
		Params: []*fftypes.JSONAny{
			fftypes.JSONAnyPtr(`{
				"from": "0xfb075bb99f2aa4c49955bf703509a227d7a12248",
				"nonce": "0x123"
			}`),
		},
	})
	assert.Regexp(t, "pop", err)

}
