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
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/signermocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewWalletConfigErrors(t *testing.T) {
	ctx, km, _, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{}, nil)
	defer done()

	_, err := km.newWallet(ctx, &pldconf.WalletConfig{
		Name:        "wallet1",
		KeySelector: "((((((!!!",
	})
	assert.Regexp(t, "PD010510", err)

	_, err = km.newWallet(ctx, &pldconf.WalletConfig{
		Name:       "wallet1",
		SignerType: "wrong",
	})
	assert.Regexp(t, "PD010506", err)

	_, err = km.newWallet(ctx, &pldconf.WalletConfig{
		Name: "wallet1",
		Signer: &pldconf.SignerConfig{
			KeyStore: pldconf.KeyStoreConfig{
				Type: "wrong",
			},
		},
	})
	assert.Regexp(t, "PD010507", err)

	_, err = km.newWallet(ctx, &pldconf.WalletConfig{
		Name:       "wallet1",
		SignerType: pldconf.WalletSignerTypePlugin,
	})
	assert.Regexp(t, "PD010516", err)

	_, err = km.newWallet(ctx, &pldconf.WalletConfig{
		Name:             "wallet1",
		SignerType:       pldconf.WalletSignerTypePlugin,
		SignerPluginName: "test1",
	})
	assert.Regexp(t, "PD010517", err)

	_, err = km.selectWallet(ctx, "anything")
	assert.Regexp(t, "PD010501", err)
}

func TestResolveKeyAndVerifierErr(t *testing.T) {

	ctx, km, _, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	w, err := km.getWalletByName(ctx, "hdwallet1")
	require.NoError(t, err)

	_, err = w.resolveKeyAndVerifier(ctx, &pldapi.KeyMappingWithPath{
		KeyMapping: &pldapi.KeyMapping{
			KeyHandle:  "another",
			Identifier: "key",
		},
		Path: []*pldapi.KeyPathSegment{{Name: "a"}},
	}, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	assert.Regexp(t, "PD010504.*another", err)

}

func TestResolveBadSignerResponse(t *testing.T) {

	ctx, km, _, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	w, err := km.getWalletByName(ctx, "hdwallet1")
	require.NoError(t, err)

	ms := signermocks.NewSigningModule(t)
	ms.On("Resolve", mock.Anything, mock.Anything).Return(&prototk.ResolveKeyResponse{
		KeyHandle: "some.handle",
	}, nil)
	w.signingModule = ms

	_, err = w.resolveKeyAndVerifier(ctx, &pldapi.KeyMappingWithPath{
		KeyMapping: &pldapi.KeyMapping{
			Identifier: "key",
		},
		Path: []*pldapi.KeyPathSegment{{Name: "a"}},
	}, "test:algo", "requested-type")
	assert.Regexp(t, "PD010505", err)
}

func TestSignError(t *testing.T) {

	ctx, km, _, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	w, err := km.getWalletByName(ctx, "hdwallet1")
	require.NoError(t, err)

	ms := signermocks.NewSigningModule(t)
	ms.On("Sign", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	w.signingModule = ms

	_, err = w.sign(ctx, &pldapi.KeyMappingAndVerifier{
		KeyMappingWithPath: &pldapi.KeyMappingWithPath{
			KeyMapping: &pldapi.KeyMapping{
				Identifier: "any",
			},
		},
		Verifier: &pldapi.KeyVerifier{},
	}, "any", []byte("payload"))
	assert.Regexp(t, "pop", err)

}
