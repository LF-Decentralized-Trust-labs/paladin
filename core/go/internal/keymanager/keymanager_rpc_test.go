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

package keymanager

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRPCLocalDetails(t *testing.T) {
	ctx, km, _, done := newTestKeyManagerHDWallet(t)
	defer done()

	rpc, rpcDone := newTestRPCServer(t, ctx, km)
	defer rpcDone()

	var wallets []*pldapi.WalletInfo
	err := rpc.CallRPC(ctx, &wallets, "keymgr_wallets")
	require.NoError(t, err)
	assert.Equal(t, []*pldapi.WalletInfo{{Name: "hdwallet1", KeySelector: ".*"}}, wallets)

	var resolvedKey *pldapi.KeyMappingAndVerifier
	err = rpc.CallRPC(ctx, &resolvedKey, "keymgr_resolveKey", "my.key.1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	assert.Equal(t, "m/44'/60'/1'/0/0", resolvedKey.KeyHandle)

	var ethAddress *tktypes.EthAddress
	err = rpc.CallRPC(ctx, &ethAddress, "keymgr_resolveEthAddress", "my.key.1")
	require.NoError(t, err)
	assert.Equal(t, resolvedKey.Verifier.Verifier, ethAddress.String())

	var reverseLookedUp *pldapi.KeyMappingAndVerifier
	err = rpc.CallRPC(ctx, &reverseLookedUp, "keymgr_reverseKeyLookup", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, ethAddress)
	require.NoError(t, err)
	assert.Equal(t, resolvedKey, reverseLookedUp)

}

func newTestRPCServer(t *testing.T, ctx context.Context, km *keyManager) (rpcclient.Client, func()) {

	s, err := rpcserver.NewRPCServer(ctx, &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{
			HTTPServerConfig: pldconf.HTTPServerConfig{Address: confutil.P("127.0.0.1"), Port: confutil.P(0)},
		},
		WS: pldconf.RPCServerConfigWS{Disabled: true},
	})
	require.NoError(t, err)
	err = s.Start()
	require.NoError(t, err)

	s.Register(km.RPCModule())

	c := rpcclient.WrapRestyClient(resty.New().SetBaseURL(fmt.Sprintf("http://%s", s.HTTPAddr())))

	return c, s.Stop

}
