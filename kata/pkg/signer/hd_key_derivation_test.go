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
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
	"github.com/stretchr/testify/assert"
	"github.com/tyler-smith/go-bip39"
)

func TestHDSigningStaticExample(t *testing.T) {

	ctx := context.Background()
	mnemonic := "extra monster happy tone improve slight duck equal sponsor fruit sister rate very bulb reopen mammal venture pull just motion faculty grab tenant kind"
	sm, err := NewSigningModule(ctx, &api.Config{
		KeyDerivation: api.KeyDerivationConfig{
			Type:                  api.KeyDerivationTypeBIP32,
			BIP44Prefix:           confutil.P(" m / 44' / 60' / 0' / 0 "), // we allow friendly spaces here
			BIP44HardenedSegments: confutil.P(0),
		},
		KeyStore: api.StoreConfig{
			Type: api.KeyStoreTypeStatic,
			Static: api.StaticKeyStorageConfig{
				Keys: map[string]api.StaticKeyEntryConfig{
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
		Algorithms: []string{api.Algorithm_ECDSA_SECP256K1_PLAINBYTES},
		Name:       "key1",
		Index:      0,
	})
	assert.NoError(t, err)
	assert.Equal(t, "m/44'/60'/0'/0/0", res.KeyHandle)
	assert.Equal(t, "0x6331ccb948aaf903a69d6054fd718062bd0d535c", res.Identifiers[0].Identifier)

	resSign, err := sm.Sign(ctx, &proto.SignRequest{
		KeyHandle: res.KeyHandle,
		Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
		Payload:   ([]byte)("some data"),
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, resSign.Payload)

}

func TestHDSigningDirectResNoPrefix(t *testing.T) {

	ctx := context.Background()
	sm, err := NewSigningModule(ctx, &api.Config{
		KeyDerivation: api.KeyDerivationConfig{
			Type:                  api.KeyDerivationTypeBIP32,
			BIP44Prefix:           confutil.P("m"),
			BIP44HardenedSegments: confutil.P(0),
			BIP44DirectResolution: true,
		},
		KeyStore: api.StoreConfig{
			Type:       api.KeyStoreTypeFilesystem,
			FileSystem: api.FileSystemConfig{Path: confutil.P(t.TempDir())},
		},
	})
	assert.NoError(t, err)

	res, err := sm.Resolve(ctx, &proto.ResolveKeyRequest{
		Algorithms: []string{api.Algorithm_ECDSA_SECP256K1_PLAINBYTES},
		Name:       "50'",
		Index:      0,
		Path: []*proto.ResolveKeyPathSegment{
			{
				Name:  "10'",
				Index: 0,
			},
			{
				Name:  "20'",
				Index: 0,
			},
			{
				Name:  "30",
				Index: 0,
			},
			{
				Name:  "40",
				Index: 0,
			},
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "m/10'/20'/30/40/50'", res.KeyHandle)

	_, err = sm.Resolve(ctx, &proto.ResolveKeyRequest{
		Algorithms: []string{api.Algorithm_ECDSA_SECP256K1_PLAINBYTES},
		Name:       "key1",
		Index:      0,
	})
	assert.Regexp(t, "PD011413", err)

	_, err = sm.Resolve(ctx, &proto.ResolveKeyRequest{
		Algorithms: []string{api.Algorithm_ECDSA_SECP256K1_PLAINBYTES},
		Name:       "2147483648", // too big
		Index:      0,
	})
	assert.Regexp(t, "PD011414", err)

	_, err = sm.(*signingModule).hd.signHDWalletKey(ctx, &proto.SignRequest{
		KeyHandle: "m/wrong",
	})
	assert.Regexp(t, "PD011413", err)

	_, err = sm.(*signingModule).hd.loadHDWalletPrivateKey(ctx, "")
	assert.Regexp(t, "PD011413", err)

}

func TestHDSigningDefaultBehaviorOK(t *testing.T) {

	ctx := context.Background()
	entropy, err := bip39.NewEntropy(256)
	assert.NoError(t, err)

	mnemonic, err := bip39.NewMnemonic(entropy)
	assert.NoError(t, err)

	sm, err := NewSigningModule(ctx, &api.Config{
		KeyDerivation: api.KeyDerivationConfig{
			Type: api.KeyDerivationTypeBIP32,
			SeedKeyPath: api.ConfigKeyEntry{
				Name:  "seed",
				Index: 0,
				Path: []api.ConfigKeyPathEntry{
					{Name: "custom", Index: 0},
				},
			},
		},
		KeyStore: api.StoreConfig{
			Type: api.KeyStoreTypeStatic,
			Static: api.StaticKeyStorageConfig{
				Keys: map[string]api.StaticKeyEntryConfig{
					"custom/seed": {
						Encoding: "none",
						Inline:   mnemonic,
					},
				},
			},
		},
	})
	assert.NoError(t, err)

	res, err := sm.Resolve(ctx, &proto.ResolveKeyRequest{
		Algorithms: []string{api.Algorithm_ECDSA_SECP256K1_PLAINBYTES},
		Name:       "E82D5A3F-D154-4C5B-A297-F8D49528DA73",
		Index:      0x7FFFFFFF, // largest possible - not in hardened range
		Path: []*proto.ResolveKeyPathSegment{
			{
				Name:  "bob",
				Index: 0x7FFFFFFF, // largest possible - will be pushed to hardened range (default config)
			},
			{
				Name:  "single-use",
				Index: 3,
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
	testKeyPair := secp256k1.KeyPairFromBytes(keyBytes[:])
	assert.Equal(t, testKeyPair.Address.String(), res.Identifiers[0].Identifier)

	resSign, err := sm.Sign(ctx, &proto.SignRequest{
		KeyHandle: res.KeyHandle,
		Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
		Payload:   ([]byte)("some data"),
	})
	assert.NoError(t, err)

	testSign, err := testKeyPair.SignDirect(([]byte)("some data"))
	assert.NoError(t, err)
	assert.Equal(t, testSign.CompactRSV(), resSign.Payload)
	sig, err := secp256k1.DecodeCompactRSV(ctx, resSign.Payload)
	assert.NoError(t, err)
	assert.Equal(t, testSign, sig)

}

func TestHDSigningInitFailDisabled(t *testing.T) {

	ctx := context.Background()
	_, err := NewSigningModule(ctx, &api.Config{
		KeyDerivation: api.KeyDerivationConfig{
			Type: api.KeyDerivationTypeBIP32,
		},
		KeyStore: api.StoreConfig{
			DisableKeyLoading: true,
			Type:              api.KeyStoreTypeStatic,
		},
	})
	assert.Regexp(t, "PD011408", err)

}

func TestHDSigningInitFailBadMnemonic(t *testing.T) {

	ctx := context.Background()
	_, err := NewSigningModule(ctx, &api.Config{
		KeyDerivation: api.KeyDerivationConfig{
			Type: api.KeyDerivationTypeBIP32,
		},
		KeyStore: api.StoreConfig{
			Type: api.KeyStoreTypeStatic,
			Static: api.StaticKeyStorageConfig{
				Keys: map[string]api.StaticKeyEntryConfig{
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

func TestHDInitBadSeed(t *testing.T) {

	ctx := context.Background()
	entropy, err := bip39.NewEntropy(256)
	assert.NoError(t, err)

	mnemonic, err := bip39.NewMnemonic(entropy)
	assert.NoError(t, err)

	_, err = NewSigningModule(ctx, &api.Config{
		KeyDerivation: api.KeyDerivationConfig{
			Type: api.KeyDerivationTypeBIP32,
			SeedKeyPath: api.ConfigKeyEntry{
				Name: "missing",
			},
		},
		KeyStore: api.StoreConfig{
			Type: api.KeyStoreTypeStatic,
			Static: api.StaticKeyStorageConfig{
				Keys: map[string]api.StaticKeyEntryConfig{
					"seed": {
						Encoding: "none",
						Inline:   mnemonic,
					},
				},
			},
		},
	})
	assert.Regexp(t, "PD011418", err)

}

func TestHDInitGenSeed(t *testing.T) {

	ctx := context.Background()

	sm, err := NewSigningModule(ctx, &api.Config{
		KeyDerivation: api.KeyDerivationConfig{
			Type: api.KeyDerivationTypeBIP32,
			SeedKeyPath: api.ConfigKeyEntry{
				Name: "seed",
				Path: []api.ConfigKeyPathEntry{{Name: "generate"}},
			},
		},
		KeyStore: api.StoreConfig{
			Type: api.KeyStoreTypeFilesystem,
			FileSystem: api.FileSystemConfig{
				Path: confutil.P(t.TempDir()),
			},
		},
	})
	assert.NoError(t, err)

	generatedSeed, err := sm.(*signingModule).keyStore.LoadKeyMaterial(ctx, "generate/seed")
	assert.NoError(t, err)
	assert.Len(t, generatedSeed, 32)
	assert.NotEqual(t, make([]byte, 32), generatedSeed) // not zero
}
