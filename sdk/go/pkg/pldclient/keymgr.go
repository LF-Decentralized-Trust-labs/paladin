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

package pldclient

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

type KeyManager interface {
	RPCModule

	Wallets(ctx context.Context) ([]string, error)
	ResolveKey(ctx context.Context, keyIdentifier, algorithm, verifierType string) (mapping *pldapi.KeyMappingAndVerifier, err error)
	ResolveEthAddress(ctx context.Context, keyIdentifier string) (ethAddress *pldtypes.EthAddress, err error)
	ReverseKeyLookup(ctx context.Context, algorithm, verifierType, verifier string) (mapping *pldapi.KeyMappingAndVerifier, err error)
}

// This is necessary because there's no way to introspect function parameter names via reflection
var keymgrInfo = &rpcModuleInfo{
	group: "keymgr",
	methodInfo: map[string]RPCMethodInfo{
		"keymgr_wallets": {
			Inputs: []string{},
			Output: "wallets",
		},
		"keymgr_resolveKey": {
			Inputs: []string{"keyIdentifier", "algorithm", "verifierType"},
			Output: "mapping",
		},
		"keymgr_resolveEthAddress": {
			Inputs: []string{"keyIdentifier"},
			Output: "ethAddress",
		},
		"keymgr_reverseKeyLookup": {
			Inputs: []string{"algorithm", "verifierType", "verifier"},
			Output: "mapping",
		},
	},
}

type keymgr struct {
	*rpcModuleInfo
	c *paladinClient
}

func (c *paladinClient) KeyManager() KeyManager {
	return &keymgr{rpcModuleInfo: keymgrInfo, c: c}
}

func (k *keymgr) Wallets(ctx context.Context) (wallets []string, err error) {
	err = k.c.CallRPC(ctx, &wallets, "keymgr_wallets")
	return
}

func (k *keymgr) ResolveKey(ctx context.Context, keyIdentifier, algorithm, verifierType string) (mapping *pldapi.KeyMappingAndVerifier, err error) {
	err = k.c.CallRPC(ctx, &mapping, "keymgr_resolveKey", keyIdentifier, algorithm, verifierType)
	return
}

func (k *keymgr) ResolveEthAddress(ctx context.Context, keyIdentifier string) (ethAddress *pldtypes.EthAddress, err error) {
	err = k.c.CallRPC(ctx, &ethAddress, "keymgr_resolveEthAddress", keyIdentifier)
	return
}

func (k *keymgr) ReverseKeyLookup(ctx context.Context, algorithm, verifierType, verifier string) (mapping *pldapi.KeyMappingAndVerifier, err error) {
	err = k.c.CallRPC(ctx, &mapping, "keymgr_reverseKeyLookup", algorithm, verifierType, verifier)
	return
}
