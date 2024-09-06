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
	"math/big"
	"testing"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/core/pkg/proto"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveKeyFail(t *testing.T) {
	ctx, ecf, done := newTestClientAndServer(t, &mockEth{})
	defer done()

	ec := ecf.HTTPClient().(*ethClient)

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
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_call: func(ctx context.Context, t ethsigner.Transaction, s string) (tktypes.HexBytes, error) {
			return nil, fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.HTTPClient().CallContract(ctx, confutil.P("wrong"), &ethsigner.Transaction{}, "latest")
	assert.Regexp(t, "pop", err)

}

func TestGetTransactionCountFailForBuildRawTx(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionCount: func(ctx context.Context, ah ethtypes.Address0xHex, s string) (ethtypes.HexUint64, error) {
			return 0, fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.HTTPClient().BuildRawTransaction(ctx, EIP1559, "key1", &ethsigner.Transaction{})
	assert.Regexp(t, "pop", err)

}

func TestGetBalance(t *testing.T) {
	balanceHexInt := ethtypes.NewHexInteger(big.NewInt(200000))
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getBalance: func(ctx context.Context, ah ethtypes.Address0xHex, s string) (ethtypes.HexInteger, error) {
			return *balanceHexInt, nil
		},
	})
	defer done()

	balance, err := ec.HTTPClient().GetBalance(ctx, "0x1d0cD5b99d2E2a380e52b4000377Dd507c6df754", "latest")
	require.NoError(t, err)
	assert.Equal(t, balanceHexInt, balance)

}

func TestGetBalanceFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getBalance: func(ctx context.Context, ah ethtypes.Address0xHex, s string) (ethtypes.HexInteger, error) {
			return ethtypes.HexInteger{}, fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.HTTPClient().GetBalance(ctx, "0x1d0cD5b99d2E2a380e52b4000377Dd507c6df754", "latest")
	assert.Regexp(t, "pop", err)

}

func TestGasPrice(t *testing.T) {
	gasPriceHexInt := ethtypes.NewHexInteger(big.NewInt(200000))
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_gasPrice: func(ctx context.Context) (ethtypes.HexInteger, error) {
			return *gasPriceHexInt, nil
		},
	})
	defer done()

	gasPrice, err := ec.HTTPClient().GasPrice(ctx)
	require.NoError(t, err)
	assert.Equal(t, gasPriceHexInt, gasPrice)

}

func TestGasPriceFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_gasPrice: func(ctx context.Context) (ethtypes.HexInteger, error) {
			return ethtypes.HexInteger{}, fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.HTTPClient().GasPrice(ctx)
	assert.Regexp(t, "pop", err)

}

func TestGasEstimate(t *testing.T) {
	gasEstimateHexInt := ethtypes.NewHexInteger(big.NewInt(200000))
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_estimateGas: func(ctx context.Context, tx ethsigner.Transaction) (ethtypes.HexInteger, error) {
			return *gasEstimateHexInt, nil
		},
	})
	defer done()

	gasLimit, err := ec.HTTPClient().GasEstimate(ctx, &ethsigner.Transaction{})
	require.NoError(t, err)
	assert.Equal(t, gasEstimateHexInt, gasLimit)

}

func TestGasEstimateFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_estimateGas: func(ctx context.Context, tx ethsigner.Transaction) (ethtypes.HexInteger, error) {
			return ethtypes.HexInteger{}, fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.HTTPClient().GasEstimate(ctx, &ethsigner.Transaction{})
	assert.Regexp(t, "pop", err)
}

func TestGetTransactionCount(t *testing.T) {
	txCountHexUint := (ethtypes.HexUint64)(200000)
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionCount: func(ctx context.Context, addr ethtypes.Address0xHex, block string) (ethtypes.HexUint64, error) {
			return txCountHexUint, nil
		},
	})
	defer done()

	txCount, err := ec.HTTPClient().GetTransactionCount(ctx, "0x1d0cD5b99d2E2a380e52b4000377Dd507c6df754")
	require.NoError(t, err)
	assert.Equal(t, txCountHexUint, *txCount)

}

func TestGetTransactionCountFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionCount: func(ctx context.Context, addr ethtypes.Address0xHex, block string) (ethtypes.HexUint64, error) {
			return (ethtypes.HexUint64)(0), fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.HTTPClient().GetTransactionCount(ctx, "0x1d0cD5b99d2E2a380e52b4000377Dd507c6df754")
	assert.Regexp(t, "pop", err)
}

func TestEstimateGasFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionCount: func(ctx context.Context, ah ethtypes.Address0xHex, s string) (ethtypes.HexUint64, error) {
			return 0, nil
		},
		eth_estimateGas: func(ctx context.Context, t ethsigner.Transaction) (ethtypes.HexInteger, error) {
			return *ethtypes.NewHexInteger64(0), fmt.Errorf("pop")
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
	})
	assert.Regexp(t, "PD011505.*wrong", err)

}

func TestSignFail(t *testing.T) {
	ctx, ecf, done := newTestClientAndServer(t, &mockEth{})
	defer done()

	ec := ecf.HTTPClient().(*ethClient)
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
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_sendRawTransaction: func(ctx context.Context, hbp tktypes.HexBytes) (tktypes.HexBytes, error) {
			return nil, fmt.Errorf("pop")
		},
	})
	defer done()

	rawTx, err := ec.HTTPClient().BuildRawTransaction(ctx, EIP1559, "key1", &ethsigner.Transaction{
		Nonce:    ethtypes.NewHexInteger64(0),
		GasLimit: ethtypes.NewHexInteger64(100000),
	})
	require.NoError(t, err)

	_, err = ec.HTTPClient().SendRawTransaction(ctx, rawTx)
	assert.Regexp(t, "pop", err)

	_, err = ec.HTTPClient().SendRawTransaction(ctx, ([]byte)("not RLP"))
	assert.Regexp(t, "pop", err)

}

const testTxHash = "0x7d48ae971faf089878b57e3c28e3035540d34f38af395958d2c73c36c57c83a2"

func TestGetReceiptOkSuccess(t *testing.T) {
	sampleJSONRPCReceipt := &txReceiptJSONRPC{
		BlockNumber:      ethtypes.NewHexInteger64(1988),
		TransactionIndex: ethtypes.NewHexInteger64(30),
		Status:           ethtypes.NewHexInteger64(1),
		ContractAddress:  ethtypes.MustNewAddress("0x87ae94ab290932c4e6269648bb47c86978af4436"),
	}
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionReceipt: func(context.Context, ethtypes.HexBytes0xPrefix) (*txReceiptJSONRPC, error) {
			return sampleJSONRPCReceipt, nil
		},
	})
	defer done()

	receipt, err := ec.HTTPClient().GetTransactionReceipt(ctx, testTxHash)
	require.NoError(t, err)

	assert.True(t, receipt.Success)
	assert.Equal(t, int64(1988), receipt.BlockNumber.Int64())
	assert.Equal(t, int64(30), receipt.TransactionIndex.Int64())
}

func TestGetReceiptOkFailed(t *testing.T) {
	revertReasonTooSmallHex := ethtypes.MustNewHexBytes0xPrefix("0x08c379a00000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000001d5468652073746f7265642076616c756520697320746f6f20736d616c6c000000")
	sampleJSONRPCReceipt := &txReceiptJSONRPC{
		BlockNumber:      ethtypes.NewHexInteger64(1988),
		TransactionIndex: ethtypes.NewHexInteger64(30),
		Status:           ethtypes.NewHexInteger64(0),
		ContractAddress:  ethtypes.MustNewAddress("0x87ae94ab290932c4e6269648bb47c86978af4436"),
		RevertReason:     &revertReasonTooSmallHex,
	}
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionReceipt: func(context.Context, ethtypes.HexBytes0xPrefix) (*txReceiptJSONRPC, error) {
			return sampleJSONRPCReceipt, nil
		},
	})
	defer done()

	receipt, err := ec.HTTPClient().GetTransactionReceipt(ctx, testTxHash)
	require.NoError(t, err)

	assert.False(t, receipt.Success)
	assert.Contains(t, receipt.ExtraInfo.String(), "The stored value is too small")
}

func TestGetReceiptOkFailedMissingReason(t *testing.T) {
	sampleJSONRPCReceipt := &txReceiptJSONRPC{
		BlockNumber:      ethtypes.NewHexInteger64(1988),
		TransactionIndex: ethtypes.NewHexInteger64(30),
		Status:           ethtypes.NewHexInteger64(0),
		ContractAddress:  ethtypes.MustNewAddress("0x87ae94ab290932c4e6269648bb47c86978af4436"),
	}
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionReceipt: func(context.Context, ethtypes.HexBytes0xPrefix) (*txReceiptJSONRPC, error) {
			return sampleJSONRPCReceipt, nil
		},
	})
	defer done()

	receipt, err := ec.HTTPClient().GetTransactionReceipt(ctx, testTxHash)
	require.NoError(t, err)

	assert.False(t, receipt.Success)
	assert.Contains(t, receipt.ExtraInfo.String(), "PD011913")
}

func TestGetReceiptOkFailedCustomReason(t *testing.T) {
	revertCustomHex := ethtypes.MustNewHexBytes0xPrefix("0x08c379a0000000000000000000000000000000000000000000000000000000000000003e000000000000000000000000000000000000000000000000000000000000001d5468652073746f7265642076616c756520697320746f6f20736d616c6c000000")

	sampleJSONRPCReceipt := &txReceiptJSONRPC{
		BlockNumber:      ethtypes.NewHexInteger64(1988),
		TransactionIndex: ethtypes.NewHexInteger64(30),
		Status:           ethtypes.NewHexInteger64(0),
		ContractAddress:  ethtypes.MustNewAddress("0x87ae94ab290932c4e6269648bb47c86978af4436"),
		RevertReason:     &revertCustomHex,
	}
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionReceipt: func(context.Context, ethtypes.HexBytes0xPrefix) (*txReceiptJSONRPC, error) {
			return sampleJSONRPCReceipt, nil
		},
	})
	defer done()

	receipt, err := ec.HTTPClient().GetTransactionReceipt(ctx, testTxHash)
	require.NoError(t, err)

	assert.False(t, receipt.Success)
	assert.Contains(t, receipt.ExtraInfo.String(), revertCustomHex.String())
}

func TestGetReceiptError(t *testing.T) {

	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionReceipt: func(context.Context, ethtypes.HexBytes0xPrefix) (*txReceiptJSONRPC, error) {
			return nil, fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.HTTPClient().GetTransactionReceipt(ctx, testTxHash)
	assert.Regexp(t, "pop", err)
}

func TestGetReceiptNotFound(t *testing.T) {

	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionReceipt: func(context.Context, ethtypes.HexBytes0xPrefix) (*txReceiptJSONRPC, error) {
			return nil, nil
		},
	})
	defer done()

	_, err := ec.HTTPClient().GetTransactionReceipt(ctx, testTxHash)
	assert.Regexp(t, "PD011914", err)

}
func TestProtocolIDForReceipt(t *testing.T) {
	assert.Equal(t, "000000012345/000042", ProtocolIDForReceipt(fftypes.NewFFBigInt(12345), fftypes.NewFFBigInt(42)))
	assert.Equal(t, "", ProtocolIDForReceipt(nil, nil))
}
