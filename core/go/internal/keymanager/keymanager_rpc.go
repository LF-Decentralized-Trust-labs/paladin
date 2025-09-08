// Copyright © 2024 Kaleido, Inc.
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

package keymanager

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
)

func (km *keyManager) RPCModule() *rpcserver.RPCModule {
	return km.rpcModule
}

func (km *keyManager) initRPC() {
	km.rpcModule = rpcserver.NewRPCModule("keymgr").
		Add("keymgr_wallets", km.rpcWallets()).
		Add("keymgr_resolveKey", km.rpcResolveKey()).
		Add("keymgr_resolveEthAddress", km.rpcResolveEthAddress()).
		Add("keymgr_reverseKeyLookup", km.rpcReverseKeyLookup()).
		Add("keymgr_queryKeys", km.rpcQueryKeys())

}

func (km *keyManager) rpcWallets() rpcserver.RPCHandler {
	return rpcserver.RPCMethod0(func(ctx context.Context,
	) ([]*pldapi.WalletInfo, error) {
		return km.getWalletList(), nil
	})
}

func (km *keyManager) rpcResolveKey() rpcserver.RPCHandler {
	return rpcserver.RPCMethod3(func(ctx context.Context,
		identifier string,
		algorithm string,
		verifierType string,
	) (*pldapi.KeyMappingAndVerifier, error) {
		return km.ResolveKeyNewDatabaseTX(ctx, identifier, algorithm, verifierType)
	})
}

func (km *keyManager) rpcResolveEthAddress() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		identifier string,
	) (*pldtypes.EthAddress, error) {
		return km.ResolveEthAddressNewDatabaseTX(ctx, identifier)
	})
}

func (km *keyManager) rpcReverseKeyLookup() rpcserver.RPCHandler {
	return rpcserver.RPCMethod3(func(ctx context.Context,
		algorithm string,
		verifierType string,
		verifier string,
	) (*pldapi.KeyMappingAndVerifier, error) {
		return km.ReverseKeyLookup(ctx, km.p.NOTX(), algorithm, verifierType, verifier)
	})
}

func (km *keyManager) rpcQueryKeys() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		jq query.QueryJSON,
	) ([]*pldapi.KeyQueryEntry, error) {
		return km.QueryKeys(ctx, km.p.DB(), &jq)
	})
}
