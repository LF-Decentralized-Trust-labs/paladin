/*
 * Copyright © 2025 Kaleido, Inc.
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
	"math/big"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveKeyFail(t *testing.T) {
	ctx, ecf, done := newTestClientAndServer(t, &mockEth{})
	defer done()

	ec := ecf.HTTPClient().(*ethClient)

	ec.keymgr = &mockKeyManager{
		resolveKey: func(ctx context.Context, identifier, algorithm, verifierType string) (keyHandle string, verifier string, err error) {
			return "", "", fmt.Errorf("pop")
		},
	}

	_, err := ec.EstimateGas(ctx, confutil.P("wrong"), &ethsigner.Transaction{})
	assert.Regexp(t, "pop", err)

	_, err = ec.CallContract(ctx, confutil.P("wrong"), &ethsigner.Transaction{}, "latest")
	assert.Regexp(t, "pop", err)

	_, err = ec.BuildRawTransaction(ctx, EIP1559, "wrong", &ethsigner.Transaction{}, nil)
	assert.Regexp(t, "pop", err)

}

func TestCallFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_call: func(ctx context.Context, t ethsigner.Transaction, s string) (pldtypes.HexBytes, error) {
			return nil, fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.HTTPClient().CallContract(ctx, confutil.P("wrong"), &ethsigner.Transaction{}, "latest")
	assert.Regexp(t, "pop", err)

}

func TestGetTransactionCountFailForBuildRawTx(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionCount: func(ctx context.Context, ah pldtypes.EthAddress, s string) (pldtypes.HexUint64, error) {
			return 0, fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.HTTPClient().BuildRawTransaction(ctx, EIP1559, "key1", &ethsigner.Transaction{})
	assert.Regexp(t, "pop", err)

}

func TestGetBalance(t *testing.T) {
	balanceHexInt := (*pldtypes.HexUint256)(big.NewInt(200000))
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getBalance: func(ctx context.Context, ah pldtypes.EthAddress, s string) (*pldtypes.HexUint256, error) {
			return balanceHexInt, nil
		},
	})
	defer done()

	balance, err := ec.HTTPClient().GetBalance(ctx, *pldtypes.MustEthAddress("0x1d0cD5b99d2E2a380e52b4000377Dd507c6df754"), "latest")
	require.NoError(t, err)
	assert.Equal(t, balanceHexInt, balance)

}

func TestGetBalanceFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getBalance: func(ctx context.Context, ah pldtypes.EthAddress, s string) (*pldtypes.HexUint256, error) {
			return nil, fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.HTTPClient().GetBalance(ctx, *pldtypes.MustEthAddress("0x1d0cD5b99d2E2a380e52b4000377Dd507c6df754"), "latest")
	assert.Regexp(t, "pop", err)

}

func TestGasPrice(t *testing.T) {
	gasPriceHexInt := (*pldtypes.HexUint256)(big.NewInt(200000))
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_gasPrice: func(ctx context.Context) (*pldtypes.HexUint256, error) {
			return gasPriceHexInt, nil
		},
	})
	defer done()

	gasPrice, err := ec.HTTPClient().GasPrice(ctx)
	require.NoError(t, err)
	assert.Equal(t, gasPriceHexInt, gasPrice)

}

func TestGasPriceFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_gasPrice: func(ctx context.Context) (*pldtypes.HexUint256, error) {
			return nil, fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.HTTPClient().GasPrice(ctx)
	assert.Regexp(t, "pop", err)

}

func TestEstimateGas(t *testing.T) {
	gasEstimateHexInt := pldtypes.HexUint64(200000)
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_estimateGas: func(ctx context.Context, tx ethsigner.Transaction) (pldtypes.HexUint64, error) {
			return gasEstimateHexInt, nil
		},
	})
	defer done()

	res, err := ec.HTTPClient().EstimateGas(ctx, nil, &ethsigner.Transaction{}, nil)
	require.NoError(t, err)
	assert.Equal(t, gasEstimateHexInt, pldtypes.HexUint64(res.GasLimit.Uint64()))

}

func TestEstimateGasFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_estimateGas: func(ctx context.Context, tx ethsigner.Transaction) (pldtypes.HexUint64, error) {
			return 0, fmt.Errorf("pop1")
		},
		eth_call: func(ctx context.Context, t ethsigner.Transaction, s string) (pldtypes.HexBytes, error) {
			return nil, fmt.Errorf("pop2")
		},
	})
	defer done()

	_, err := ec.HTTPClient().EstimateGas(ctx, nil, &ethsigner.Transaction{})
	assert.Regexp(t, "pop2", err)
}

func TestGetTransactionCount(t *testing.T) {
	txCountHexUint := (pldtypes.HexUint64)(200000)
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionCount: func(ctx context.Context, addr pldtypes.EthAddress, block string) (pldtypes.HexUint64, error) {
			return txCountHexUint, nil
		},
	})
	defer done()

	txCount, err := ec.HTTPClient().GetTransactionCount(ctx, *pldtypes.MustEthAddress("0x1d0cD5b99d2E2a380e52b4000377Dd507c6df754"))
	require.NoError(t, err)
	assert.Equal(t, txCountHexUint, *txCount)

}

func TestGetTransactionCountFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionCount: func(ctx context.Context, addr pldtypes.EthAddress, block string) (pldtypes.HexUint64, error) {
			return (pldtypes.HexUint64)(0), fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.HTTPClient().GetTransactionCount(ctx, *pldtypes.MustEthAddress("0x1d0cD5b99d2E2a380e52b4000377Dd507c6df754"))
	assert.Regexp(t, "pop", err)
}

func TestBuildRawTransactionEstimateGasFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionCount: func(ctx context.Context, ah pldtypes.EthAddress, s string) (pldtypes.HexUint64, error) {
			return 0, nil
		},
		eth_estimateGas: func(ctx context.Context, t ethsigner.Transaction) (pldtypes.HexUint64, error) {
			return 0, fmt.Errorf("pop1")
		},
		eth_call: func(ctx context.Context, t ethsigner.Transaction, s string) (pldtypes.HexBytes, error) {
			return nil, fmt.Errorf("pop2")
		},
	})
	defer done()

	_, err := ec.HTTPClient().BuildRawTransaction(ctx, EIP1559, "key1", &ethsigner.Transaction{})
	assert.Regexp(t, "pop", err)

}

func TestBadTXVersion(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{})
	defer done()

	_, err := ec.HTTPClient().BuildRawTransaction(ctx, EthTXVersion("wrong"), "key1", &ethsigner.Transaction{
		Nonce:    ethtypes.NewHexInteger64(0),
		GasLimit: ethtypes.NewHexInteger64(100000),
	}, nil)
	assert.Regexp(t, "PD011505.*wrong", err)

}

func TestSignFail(t *testing.T) {
	ctx, ecf, done := newTestClientAndServer(t, &mockEth{})
	defer done()

	ec := ecf.HTTPClient().(*ethClient)
	ec.keymgr = &mockKeyManager{
		resolveKey: func(ctx context.Context, identifier, algorithm, verifierType string) (keyHandle string, verifier string, err error) {
			return "kh1", "0x1d0cD5b99d2E2a380e52b4000377Dd507c6df754", nil
		},
		sign: func(ctx context.Context, req *prototk.SignWithKeyRequest) (*prototk.SignWithKeyResponse, error) {
			return nil, fmt.Errorf("pop")
		},
	}

	_, err := ec.BuildRawTransaction(ctx, EIP1559, "key1", &ethsigner.Transaction{
		Nonce:    ethtypes.NewHexInteger64(0),
		GasLimit: ethtypes.NewHexInteger64(100000),
	}, nil)
	assert.Regexp(t, "pop", err)

}

func TestSendRawFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_sendRawTransaction: func(ctx context.Context, hbp pldtypes.HexBytes) (pldtypes.HexBytes, error) {
			return nil, fmt.Errorf("pop")
		},
	})
	defer done()

	rawTx, err := ec.HTTPClient().BuildRawTransaction(ctx, EIP1559, "key1", &ethsigner.Transaction{
		Nonce:    ethtypes.NewHexInteger64(0),
		GasLimit: ethtypes.NewHexInteger64(100000),
	}, nil)
	require.NoError(t, err)

	_, err = ec.HTTPClient().SendRawTransaction(ctx, rawTx)
	assert.Regexp(t, "pop", err)

	_, err = ec.HTTPClient().SendRawTransaction(ctx, ([]byte)("not RLP"))
	assert.Regexp(t, "pop", err)

}

func TestUnconnectedRPCClient(t *testing.T) {
	ctx := context.Background()
	ec := NewUnconnectedRPCClient(ctx, &pldconf.EthClientConfig{}, 0)
	_, err := ec.GetBalance(ctx, *pldtypes.RandAddress(), "latest")
	assert.Regexp(t, "PD011517", err)
}
