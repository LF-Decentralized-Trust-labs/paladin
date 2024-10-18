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

	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type KeyManager interface {
	Wallets(ctx context.Context) ([]string, error)
	ResolveKey(ctx context.Context, identifier, algorithm, verifierType string) (mapping *pldapi.KeyMappingAndVerifier, err error)
	ResolveEthAddress(ctx context.Context, identifier string) (addr *tktypes.EthAddress, err error)
	ReverseKeyLookup(ctx context.Context, algorithm, verifierType, verifier string) (mapping *pldapi.KeyMappingAndVerifier, err error)
}

type kmgr struct{ *paladinClient }

func (c *paladinClient) KeyManager() KeyManager {
	return &kmgr{paladinClient: c}
}

func (k *kmgr) Wallets(ctx context.Context) (wallets []string, err error) {
	err = k.CallRPC(ctx, &wallets, "keymgr_wallets")
	return wallets, err
}

func (k *kmgr) ResolveKey(ctx context.Context, identifier, algorithm, verifierType string) (mapping *pldapi.KeyMappingAndVerifier, err error) {
	err = k.CallRPC(ctx, &mapping, "keymgr_resolveKey", identifier, algorithm, verifierType)
	return mapping, err
}

func (k *kmgr) ResolveEthAddress(ctx context.Context, identifier string) (addr *tktypes.EthAddress, err error) {
	err = k.CallRPC(ctx, &addr, "keymgr_resolveEthAddress", identifier)
	return addr, err
}

func (k *kmgr) ReverseKeyLookup(ctx context.Context, algorithm, verifierType, verifier string) (mapping *pldapi.KeyMappingAndVerifier, err error) {
	err = k.CallRPC(ctx, &mapping, "keymgr_reverseKeyLookup", algorithm, verifierType, verifier)
	return mapping, err
}
