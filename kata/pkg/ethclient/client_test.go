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
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/stretchr/testify/assert"
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
		eth_call: func(ctx context.Context, t ethsigner.Transaction, s string) (types.HexBytes, error) {
			return nil, fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.HTTPClient().CallContract(ctx, confutil.P("wrong"), &ethsigner.Transaction{}, "latest")
	assert.Regexp(t, "pop", err)

}

func TestGetTransactionCountFail(t *testing.T) {
	ctx, ec, done := newTestClientAndServer(t, &mockEth{
		eth_getTransactionCount: func(ctx context.Context, ah ethtypes.Address0xHex, s string) (ethtypes.HexUint64, error) {
			return 0, fmt.Errorf("pop")
		},
	})
	defer done()

	_, err := ec.HTTPClient().BuildRawTransaction(ctx, EIP1559, "key1", &ethsigner.Transaction{})
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
		eth_sendRawTransaction: func(ctx context.Context, hbp types.HexBytes) (types.HexBytes, error) {
			return nil, fmt.Errorf("pop")
		},
	})
	defer done()

	rawTx, err := ec.HTTPClient().BuildRawTransaction(ctx, EIP1559, "key1", &ethsigner.Transaction{
		Nonce:    ethtypes.NewHexInteger64(0),
		GasLimit: ethtypes.NewHexInteger64(100000),
	})
	assert.NoError(t, err)

	_, err = ec.HTTPClient().SendRawTransaction(ctx, rawTx)
	assert.Regexp(t, "pop", err)

	_, err = ec.HTTPClient().SendRawTransaction(ctx, ([]byte)("not RLP"))
	assert.Regexp(t, "pop", err)

}
