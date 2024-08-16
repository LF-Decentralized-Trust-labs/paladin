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

	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/httpserver"
	"github.com/kaleido-io/paladin/kata/internal/rpcclient"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer"
	"github.com/stretchr/testify/assert"
)

type mockEth struct {
	eth_chainId             func(context.Context) (ethtypes.HexUint64, error)
	eth_getTransactionCount func(context.Context, ethtypes.Address0xHex, string) (ethtypes.HexUint64, error)
	eth_estimateGas         func(context.Context, ethsigner.Transaction) (ethtypes.HexInteger, error)
	eth_sendRawTransaction  func(context.Context, ethtypes.HexBytes0xPrefix) (ethtypes.HexBytes0xPrefix, error)
	eth_call                func(context.Context, ethsigner.Transaction, string) (ethtypes.HexBytes0xPrefix, error)
}

func newTestServer(t *testing.T, isWS bool, mEth *mockEth) (ctx context.Context, rpcServer rpcserver.Server, done func()) {
	ctx = context.Background()

	var rpcServerConf *rpcserver.Config
	if isWS {
		rpcServerConf = &rpcserver.Config{
			HTTP: rpcserver.HTTPEndpointConfig{
				Disabled: true,
			},
			WS: rpcserver.WSEndpointConfig{
				Config: httpserver.Config{
					Port: confutil.P(0),
				},
			},
		}
	} else {
		rpcServerConf = &rpcserver.Config{
			HTTP: rpcserver.HTTPEndpointConfig{
				Config: httpserver.Config{
					Port: confutil.P(0),
				},
			},
			WS: rpcserver.WSEndpointConfig{
				Disabled: true,
			},
		}
	}

	rpcServer, err := rpcserver.NewServer(ctx, rpcServerConf)
	assert.NoError(t, err)

	if mEth.eth_chainId == nil {
		mEth.eth_chainId = func(ctx context.Context) (ethtypes.HexUint64, error) {
			return 12345, nil
		}
	}

	rpcServer.Register(rpcserver.NewRPCModule("eth").
		Add("eth_chainId", rpcserver.RPCMethod0(mEth.eth_chainId)).
		Add("eth_getTransactionCount", rpcserver.RPCMethod2(mEth.eth_getTransactionCount)).
		Add("eth_estimateGas", rpcserver.RPCMethod1(mEth.eth_estimateGas)).
		Add("eth_sendRawTransaction", rpcserver.RPCMethod1(mEth.eth_sendRawTransaction)).
		Add("eth_call", rpcserver.RPCMethod2(mEth.eth_call)),
	)

	err = rpcServer.Start()
	assert.NoError(t, err)

	return ctx, rpcServer, func() {
		rpcServer.Stop()
	}
}

func newTestClientAndServer(t *testing.T, isWS bool, mEth *mockEth) (ctx context.Context, ec *ethClient, done func()) {
	ctx, rpcServer, serverDone := newTestServer(t, isWS, mEth)

	kmgr := newTestHDWalletKeyManager(t)

	if isWS {
		iec, err := NewEthClient(ctx, kmgr, &Config{
			WS: rpcclient.WSConfig{
				HTTPConfig: rpcclient.HTTPConfig{
					URL: fmt.Sprintf("ws://%s", rpcServer.WSAddr().String()),
				},
			},
		})
		assert.NoError(t, err)
		ec = iec.(*ethClient)
	} else {
		iec, err := NewEthClient(ctx, kmgr, &Config{
			HTTP: rpcclient.HTTPConfig{
				URL: fmt.Sprintf("http://%s", rpcServer.HTTPAddr().String()),
			},
		})
		assert.NoError(t, err)
		ec = iec.(*ethClient)
	}
	assert.Equal(t, int64(12345), ec.ChainID())

	return ctx, ec, func() {
		serverDone()
		ec.Close()
	}

}

func TestNewEthClientBadConfig(t *testing.T) {
	kmgr, err := NewSimpleTestKeyManager(context.Background(), &signer.Config{
		KeyStore: signer.StoreConfig{Type: signer.KeyStoreTypeStatic},
	})
	assert.NoError(t, err)
	_, err = NewEthClient(context.Background(), kmgr, &Config{})
	assert.Regexp(t, "PD011301", err)
}

func TestNewEthClientChainIDFail(t *testing.T) {
	ctx, rpcServer, done := newTestServer(t, false, &mockEth{
		eth_chainId: func(ctx context.Context) (ethtypes.HexUint64, error) { return 0, fmt.Errorf("pop") },
	})
	defer done()

	_, err := NewEthClient(ctx, newTestHDWalletKeyManager(t), &Config{
		HTTP: rpcclient.HTTPConfig{
			URL: fmt.Sprintf("http://%s", rpcServer.HTTPAddr().String()),
		},
	})
	assert.Regexp(t, "PD011508.*pop", err)

}

func TestResolveKeyFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, false, &mockEth{})
	defer done()

	ec.keymgr = &mockKeyManager{
		resolveKey: func(ctx context.Context, identifier, algorithm string) (keyHandle string, verifier string, err error) {
			return "", "", fmt.Errorf("pop")
		},
	}

	_, err := ec.CallContract(ctx, confutil.P("wrong"), &ethsigner.Transaction{}, "latest")
	assert.Regexp(t, "pop", err)

	_, err = ec.BuildRawTransaction(ctx, EIP1559, "wrong", &ethsigner.Transaction{})
	assert.Regexp(t, "pop", err)

}

func TestCallFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, false, &mockEth{
		eth_call: func(ctx context.Context, t ethsigner.Transaction, s string) (ethtypes.HexBytes0xPrefix, error) {
			return nil, fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.CallContract(ctx, confutil.P("wrong"), &ethsigner.Transaction{}, "latest")
	assert.Regexp(t, "pop", err)

}

func TestGetTransactionCountFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, false, &mockEth{
		eth_getTransactionCount: func(ctx context.Context, ah ethtypes.Address0xHex, s string) (ethtypes.HexUint64, error) {
			return 0, fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.BuildRawTransaction(ctx, EIP1559, "key1", &ethsigner.Transaction{})
	assert.Regexp(t, "pop", err)

}

func TestEstimateGasFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, false, &mockEth{
		eth_getTransactionCount: func(ctx context.Context, ah ethtypes.Address0xHex, s string) (ethtypes.HexUint64, error) {
			return 0, nil
		},
		eth_estimateGas: func(ctx context.Context, t ethsigner.Transaction) (ethtypes.HexInteger, error) {
			return *ethtypes.NewHexInteger64(0), fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.BuildRawTransaction(ctx, EIP1559, "key1", &ethsigner.Transaction{})
	assert.Regexp(t, "pop", err)

}

func TestBadTXVersion(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, false, &mockEth{})
	defer done()

	_, err := ec.BuildRawTransaction(ctx, EthTXVersion("wrong"), "key1", &ethsigner.Transaction{
		Nonce:    ethtypes.NewHexInteger64(0),
		GasLimit: ethtypes.NewHexInteger64(100000),
	})
	assert.Regexp(t, "PD011505.*wrong", err)

}

func TestSignFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, false, &mockEth{})
	defer done()

	ec.keymgr = &mockKeyManager{
		resolveKey: func(ctx context.Context, identifier, algorithm string) (keyHandle string, verifier string, err error) {
			return "kh1", "0x1d0cD5b99d2E2a380e52b4000377Dd507c6df754", nil
		},
		sign: func(ctx context.Context, req *proto.SignRequest) (*proto.SignResponse, error) {
			return nil, fmt.Errorf("pop")
		},
	}

	_, err := ec.BuildRawTransaction(ctx, EIP1559, "key1", &ethsigner.Transaction{
		Nonce:    ethtypes.NewHexInteger64(0),
		GasLimit: ethtypes.NewHexInteger64(100000),
	})
	assert.Regexp(t, "pop", err)

}

func TestSendRawFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, false, &mockEth{
		eth_sendRawTransaction: func(ctx context.Context, hbp ethtypes.HexBytes0xPrefix) (ethtypes.HexBytes0xPrefix, error) {
			return nil, fmt.Errorf("pop")
		},
	})
	defer done()

	rawTx, err := ec.BuildRawTransaction(ctx, EIP1559, "key1", &ethsigner.Transaction{
		Nonce:    ethtypes.NewHexInteger64(0),
		GasLimit: ethtypes.NewHexInteger64(100000),
	})
	assert.NoError(t, err)

	_, err = ec.SendRawTransaction(ctx, rawTx)
	assert.Regexp(t, "pop", err)

	_, err = ec.SendRawTransaction(ctx, ([]byte)("not RLP"))
	assert.Regexp(t, "pop", err)

}
