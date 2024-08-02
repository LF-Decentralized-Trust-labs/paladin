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

package signer

import (
	"context"
	"testing"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/tyler-smith/go-bip39"
)

func TestHDSigningStaticExample(t *testing.T) {

	ctx := context.Background()
	mnemonic := "extra monster happy tone improve slight duck equal sponsor fruit sister rate very bulb reopen mammal venture pull just motion faculty grab tenant kind"
	sm, err := NewSigningModule(ctx, &Config{
		KeyDerivation: KeyDerivationConfig{
			Type:                  KeyDerivationTypeBIP32,
			BIP44Prefix:           confutil.P("m/44'/60'/0'/0"),
			BIP44HardenedSegments: confutil.P(0),
		},
		KeyStore: StoreConfig{
			Type: KeyStoreTypeStatic,
			Static: StaticKeyStorageConfig{
				Keys: map[string]StaticKeyEntryConfig{
					"seed": {
						Encoding: "none",
						Inline:   mnemonic,
					},
				},
			},
		},
	})
	assert.NoError(t, err)

	res, err := sm.Resolve(ctx, &proto.ResolveKeyRequest{
		Algorithms: []string{Algorithm_ECDSA_SECP256K1},
		Path: []*proto.KeyPathSegment{
			{
				Name:  "key1",
				Index: 0,
			},
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "m/44'/60'/0'/0/0", res.KeyHandle)
	assert.Equal(t, "0x6331ccb948aaf903a69d6054fd718062bd0d535c", res.Identifiers[0].Identifier)

}

func TestHDSigningDefaultBehaviorOK(t *testing.T) {

	ctx := context.Background()
	entropy, err := bip39.NewEntropy(256)
	assert.NoError(t, err)

	mnemonic, err := bip39.NewMnemonic(entropy)
	assert.NoError(t, err)

	sm, err := NewSigningModule(ctx, &Config{
		KeyDerivation: KeyDerivationConfig{
			Type: KeyDerivationTypeBIP32,
		},
		KeyStore: StoreConfig{
			Type: KeyStoreTypeStatic,
			Static: StaticKeyStorageConfig{
				Keys: map[string]StaticKeyEntryConfig{
					"seed": {
						Encoding: "none",
						Inline:   mnemonic,
					},
				},
			},
		},
	})
	assert.NoError(t, err)

	res, err := sm.Resolve(ctx, &proto.ResolveKeyRequest{
		Algorithms: []string{Algorithm_ECDSA_SECP256K1},
		Path: []*proto.KeyPathSegment{
			{
				Name:  "bob",
				Index: 0x7FFFFFFF, // largest possible - will be pushed to hardened range (default config)
			},
			{
				Name:  "single-use",
				Index: 3,
			},
			{
				Name:  "E82D5A3F-D154-4C5B-A297-F8D49528DA73",
				Index: 0x7FFFFFFF, // largest possible - not in hardened range
			},
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "m/44'/60'/2147483647'/3/2147483647", res.KeyHandle)

	seed, err := bip39.NewSeedWithErrorChecking(string(mnemonic), "")
	assert.NoError(t, err)
	tk, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	assert.NoError(t, err)
	tk, err = tk.Derive(0x80000000 + 44)
	assert.NoError(t, err)
	tk, err = tk.Derive(0x80000000 + 60)
	assert.NoError(t, err)
	tk, err = tk.Derive(0x80000000 + 0x7FFFFFFF)
	assert.NoError(t, err)
	tk, err = tk.Derive(3)
	assert.NoError(t, err)
	tk, err = tk.Derive(0x7FFFFFFF)
	assert.NoError(t, err)

	expectedKey, err := tk.ECPrivKey()
	assert.NoError(t, err)
	keyBytes := expectedKey.Key.Bytes()
	addressable, err := secp256k1.NewSecp256k1KeyPair(keyBytes[:])
	assert.NoError(t, err)
	assert.Equal(t, addressable.Address.String(), res.Identifiers[0].Identifier)

}

func TestHDSigningInitFailDisabled(t *testing.T) {

	ctx := context.Background()
	_, err := NewSigningModule(ctx, &Config{
		KeyDerivation: KeyDerivationConfig{
			Type: KeyDerivationTypeBIP32,
		},
		KeyStore: StoreConfig{
			DisableKeyLoading: true,
			Type:              KeyStoreTypeStatic,
		},
	})
	assert.Regexp(t, "PD011408", err)

}

func TestHDSigningInitFailBadMnemonic(t *testing.T) {

	ctx := context.Background()
	_, err := NewSigningModule(ctx, &Config{
		KeyDerivation: KeyDerivationConfig{
			Type: KeyDerivationTypeBIP32,
		},
		KeyStore: StoreConfig{
			Type: KeyStoreTypeStatic,
			Static: StaticKeyStorageConfig{
				Keys: map[string]StaticKeyEntryConfig{
					"seed": {
						Encoding: "none",
						Inline:   "wrong",
					},
				},
			},
		},
	})
	assert.Regexp(t, "PD011412", err)

}
