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
	"github.com/kaleido-io/paladin/kata/internal/types"
	"github.com/kaleido-io/paladin/kata/pkg/signer"
	"github.com/stretchr/testify/assert"
)

type mockEth struct {
	eth_chainId             func(context.Context) (ethtypes.HexUint64, error)
	eth_getTransactionCount func(context.Context, ethtypes.Address0xHex, string) (ethtypes.HexUint64, error)
	eth_estimateGas         func(ctx context.Context, b ethsigner.Transaction) (ethtypes.HexInteger, error)
	eth_sendRawTransaction  func(context.Context, ethtypes.HexBytes0xPrefix) (ethtypes.HexBytes0xPrefix, error)
}

func newTestClientAndServer(t *testing.T, isWS bool, mEth *mockEth) (ctx context.Context, ec *ethClient, done func()) {
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

	rpcServer.Register(rpcserver.NewRPCModule("eth").
		Add("eth_chainId", rpcserver.RPCMethod0(mEth.eth_chainId)).
		Add("eth_getTransactionCount", rpcserver.RPCMethod2(mEth.eth_getTransactionCount)).
		Add("eth_estimateGas", rpcserver.RPCMethod1(mEth.eth_estimateGas)).
		Add("eth_sendRawTransaction", rpcserver.RPCMethod1(mEth.eth_sendRawTransaction)),
	)

	kmgr, err := NewSimpleTestKeyManager(ctx, &signer.Config{
		KeyDerivation: signer.KeyDerivationConfig{
			Type: signer.KeyDerivationTypeBIP32,
		},
		KeyStore: signer.StoreConfig{
			Type: signer.KeyStoreTypeStatic,
			Static: signer.StaticKeyStorageConfig{
				Keys: map[string]signer.StaticKeyEntryConfig{
					"seed": {
						Encoding: "hex",
						Inline:   types.RandHex(32),
					},
				},
			},
		},
	})
	assert.NoError(t, err)

	err = rpcServer.Start()
	assert.NoError(t, err)

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

	return ctx, ec, func() {
		rpcServer.Stop()
	}

}
