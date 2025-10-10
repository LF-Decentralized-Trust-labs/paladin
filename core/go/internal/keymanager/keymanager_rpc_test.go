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

package keymanager

import (
	"context"
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRPCLocalDetails(t *testing.T) {
	ctx, km, _, done := newTestDBKeyManagerWithWallets(t, hdWalletConfig("hdwallet1", ""))
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

	/*
		Keys used for testing
		|---------------------------------|
		| Segment | Is key | Has children |
		|---------|--------|--------------|
		| a       | No     | Yes          |
		| b       | Yes    | Yes          |
		| c       | Yes    | No           |
		|---------|--------|--------------|
	*/

	var key *pldapi.KeyMappingAndVerifier
	err = rpc.CallRPC(ctx, &key, "keymgr_resolveKey", "a.b", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	err = rpc.CallRPC(ctx, &key, "keymgr_resolveKey", "a.b.c", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	var queryEntries []*pldapi.KeyQueryEntry

	// Query keys at the root level
	err = rpc.CallRPC(ctx, &queryEntries, "keymgr_queryKeys", query.NewQueryBuilder().Equal("parent", "").Sort("index").Limit(10).Query())
	require.NoError(t, err)

	// Result must contain 2 entries
	assert.Equal(t, len(queryEntries), 2)

	// Verify result is sorted by index
	assert.Equal(t, queryEntries[0].Index, int64(1))
	assert.Equal(t, queryEntries[1].Index, int64(2))

	// Check all parents and paths
	assert.Equal(t, queryEntries[0].Parent, "")
	assert.Equal(t, queryEntries[1].Parent, "")
	assert.Equal(t, queryEntries[0].Path, "my")
	assert.Equal(t, queryEntries[1].Path, "a")

	// Entries "my" and "a" are not keys and have children
	assert.Equal(t, queryEntries[0].IsKey, false)
	assert.Equal(t, queryEntries[0].HasChildren, true)
	assert.Equal(t, queryEntries[1].IsKey, false)
	assert.Equal(t, queryEntries[1].HasChildren, true)

	// Query keys with parent "a"
	err = rpc.CallRPC(ctx, &queryEntries, "keymgr_queryKeys", query.NewQueryBuilder().Equal("parent", "a").Limit(10).Query())
	require.NoError(t, err)

	// Result must contain 1 entry
	assert.Equal(t, len(queryEntries), 1)

	// Check parent and path
	assert.Equal(t, queryEntries[0].Parent, "a")
	assert.Equal(t, queryEntries[0].Path, "a.b")

	// Check entry is key and has children
	assert.Equal(t, queryEntries[0].IsKey, true)
	assert.Equal(t, queryEntries[0].HasChildren, true)

	// Check verifier
	assert.Equal(t, len(queryEntries[0].Verifiers), 1)
	assert.Equal(t, queryEntries[0].Verifiers[0].Type, verifiers.ETH_ADDRESS)
	assert.Equal(t, queryEntries[0].Verifiers[0].Algorithm, algorithms.ECDSA_SECP256K1)
	assert.NotNil(t, queryEntries[0].Verifiers[0].Verifier)

	// Query keys with parent "a.b"
	err = rpc.CallRPC(ctx, &queryEntries, "keymgr_queryKeys", query.NewQueryBuilder().Equal("parent", "a.b").Limit(10).Query())
	require.NoError(t, err)

	// Result must contain 1 entry
	assert.Equal(t, len(queryEntries), 1)

	// Check parent and path
	assert.Equal(t, queryEntries[0].Parent, "a.b")
	assert.Equal(t, queryEntries[0].Path, "a.b.c")

	// Check entry is key and has no children
	assert.Equal(t, queryEntries[0].IsKey, true)
	assert.Equal(t, queryEntries[0].HasChildren, false)

	var ethAddress *pldtypes.EthAddress
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
