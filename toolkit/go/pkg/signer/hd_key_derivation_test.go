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

package signer

import (
	"context"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signpayloads"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tyler-smith/go-bip39"
)

func TestHDSigningStaticExample(t *testing.T) {

	ctx := context.Background()
	mnemonic := "extra monster happy tone improve slight duck equal sponsor fruit sister rate very bulb reopen mammal venture pull just motion faculty grab tenant kind"
	sm, err := NewSigningModule(ctx, &signerapi.ConfigNoExt{
		KeyDerivation: pldconf.KeyDerivationConfig{
			Type:                  pldconf.KeyDerivationTypeBIP32,
			BIP44Prefix:           confutil.P(" m / 44' / 60' / 0' / 0 "), // we allow friendly spaces here
			BIP44HardenedSegments: confutil.P(0),
		},
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
			Static: pldconf.StaticKeyStoreConfig{
				Keys: map[string]pldconf.StaticKeyEntryConfig{
					"seed": {
						Encoding: "none",
						Inline:   mnemonic,
					},
				},
			},
		},
	})
	require.NoError(t, err)

	res, err := sm.Resolve(ctx, &prototk.ResolveKeyRequest{
		RequiredIdentifiers: []*prototk.PublicKeyIdentifierType{{Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS}},
		Name:                "key1",
		Index:               0,
	})
	require.NoError(t, err)
	assert.Equal(t, "m/44'/60'/0'/0/0", res.KeyHandle)
	assert.Equal(t, "0x6331ccb948aaf903a69d6054fd718062bd0d535c", res.Identifiers[0].Verifier)

	resSign, err := sm.Sign(ctx, &prototk.SignWithKeyRequest{
		KeyHandle:   res.KeyHandle,
		Algorithm:   algorithms.ECDSA_SECP256K1,
		PayloadType: signpayloads.OPAQUE_TO_RSV,
		Payload:     ([]byte)("some data"),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resSign.Payload)

}

func TestHDSigningStaticExamplePreResolved(t *testing.T) {

	ctx := context.Background()
	sm, err := NewSigningModule(ctx, &signerapi.ConfigNoExt{
		KeyDerivation: pldconf.KeyDerivationConfig{
			Type: pldconf.KeyDerivationTypeBIP32,
			SeedKeyPath: pldconf.StaticKeyReference{
				KeyHandle: "directly.resolved",
			},
		},
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
			Static: pldconf.StaticKeyStoreConfig{
				Keys: map[string]pldconf.StaticKeyEntryConfig{
					"directly.resolved": {
						Encoding: "hex",
						Inline:   pldtypes.RandHex(32),
					},
				},
			},
		},
	})
	require.NoError(t, err)

	res, err := sm.Resolve(ctx, &prototk.ResolveKeyRequest{
		RequiredIdentifiers: []*prototk.PublicKeyIdentifierType{{Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS}},
		Name:                "key1",
		Index:               0,
	})
	require.NoError(t, err)
	assert.Equal(t, "m/44'/60'/0'", res.KeyHandle)

	resSign, err := sm.Sign(ctx, &prototk.SignWithKeyRequest{
		KeyHandle:   res.KeyHandle,
		Algorithm:   algorithms.ECDSA_SECP256K1,
		PayloadType: signpayloads.OPAQUE_TO_RSV,
		Payload:     ([]byte)("some data"),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resSign.Payload)

}

func TestHDSigningDirectResNoPrefix(t *testing.T) {

	ctx := context.Background()
	sm, err := NewSigningModule(ctx, &signerapi.ConfigNoExt{
		KeyDerivation: pldconf.KeyDerivationConfig{
			Type:                  pldconf.KeyDerivationTypeBIP32,
			BIP44Prefix:           confutil.P("m"),
			BIP44HardenedSegments: confutil.P(0),
			BIP44DirectResolution: true,
		},
		KeyStore: pldconf.KeyStoreConfig{
			Type:       pldconf.KeyStoreTypeFilesystem,
			FileSystem: pldconf.FileSystemKeyStoreConfig{Path: confutil.P(t.TempDir())},
		},
	})
	require.NoError(t, err)

	res, err := sm.Resolve(ctx, &prototk.ResolveKeyRequest{
		RequiredIdentifiers: []*prototk.PublicKeyIdentifierType{{Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS}},
		Name:                "50'",
		Index:               0,
		Path: []*prototk.ResolveKeyPathSegment{
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
	require.NoError(t, err)
	assert.Equal(t, "m/10'/20'/30/40/50'", res.KeyHandle)

	_, err = sm.Resolve(ctx, &prototk.ResolveKeyRequest{
		RequiredIdentifiers: []*prototk.PublicKeyIdentifierType{{Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS}},
		Name:                "key1",
		Index:               0,
	})
	assert.Regexp(t, "PD020813", err)

	_, err = sm.Resolve(ctx, &prototk.ResolveKeyRequest{
		RequiredIdentifiers: []*prototk.PublicKeyIdentifierType{{Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS}},
		Name:                "2147483648", // too big
		Index:               0,
	})
	assert.Regexp(t, "PD020814", err)

	_, err = sm.(*signingModule[*signerapi.ConfigNoExt]).hd.signHDWalletKey(ctx, &prototk.SignWithKeyRequest{
		KeyHandle: "m/wrong",
	})
	assert.Regexp(t, "PD020813", err)

	_, err = sm.(*signingModule[*signerapi.ConfigNoExt]).hd.loadHDWalletPrivateKey(ctx, "")
	assert.Regexp(t, "PD020813", err)

}

func TestHDSigningDefaultBehaviorOK(t *testing.T) {

	ctx := context.Background()
	entropy, err := bip39.NewEntropy(256)
	require.NoError(t, err)

	mnemonic, err := bip39.NewMnemonic(entropy)
	require.NoError(t, err)

	sm, err := NewSigningModule(ctx, &signerapi.ConfigNoExt{
		KeyDerivation: pldconf.KeyDerivationConfig{
			Type: pldconf.KeyDerivationTypeBIP32,
			SeedKeyPath: pldconf.StaticKeyReference{
				Name:  "seed",
				Index: 0,
				Path: []pldconf.ConfigKeyPathEntry{
					{Name: "custom", Index: 0},
				},
			},
		},
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
			Static: pldconf.StaticKeyStoreConfig{
				Keys: map[string]pldconf.StaticKeyEntryConfig{
					"custom.seed": {
						Encoding: "none",
						Inline:   mnemonic,
					},
				},
			},
		},
	})
	require.NoError(t, err)

	res, err := sm.Resolve(ctx, &prototk.ResolveKeyRequest{
		RequiredIdentifiers: []*prototk.PublicKeyIdentifierType{{Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS}},
		Name:                "E82D5A3F-D154-4C5B-A297-F8D49528DA73",
		Index:               0x7FFFFFFF, // largest possible - not in hardened range
		Path: []*prototk.ResolveKeyPathSegment{
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
	require.NoError(t, err)
	assert.Equal(t, "m/44'/60'/2147483647'/3/2147483647", res.KeyHandle)

	seed, err := bip39.NewSeedWithErrorChecking(string(mnemonic), "")
	require.NoError(t, err)
	tk, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	require.NoError(t, err)
	tk, err = tk.Derive(0x80000000 + 44)
	require.NoError(t, err)
	tk, err = tk.Derive(0x80000000 + 60)
	require.NoError(t, err)
	tk, err = tk.Derive(0x80000000 + 0x7FFFFFFF)
	require.NoError(t, err)
	tk, err = tk.Derive(3)
	require.NoError(t, err)
	tk, err = tk.Derive(0x7FFFFFFF)
	require.NoError(t, err)

	expectedKey, err := tk.ECPrivKey()
	require.NoError(t, err)
	keyBytes := expectedKey.Key.Bytes()
	testKeyPair := secp256k1.KeyPairFromBytes(keyBytes[:])
	assert.Equal(t, testKeyPair.Address.String(), res.Identifiers[0].Verifier)

	resSign, err := sm.Sign(ctx, &prototk.SignWithKeyRequest{
		KeyHandle:   res.KeyHandle,
		Algorithm:   algorithms.ECDSA_SECP256K1,
		PayloadType: signpayloads.OPAQUE_TO_RSV,
		Payload:     ([]byte)("some data"),
	})
	require.NoError(t, err)

	testSign, err := testKeyPair.SignDirect(([]byte)("some data"))
	require.NoError(t, err)
	assert.Equal(t, testSign.CompactRSV(), resSign.Payload)
	sig, err := secp256k1.DecodeCompactRSV(ctx, resSign.Payload)
	require.NoError(t, err)
	assert.Equal(t, testSign, sig)

}

func TestHDSigningInitFailDisabled(t *testing.T) {

	te := &signerapi.Extensions[*signerapi.ConfigNoExt]{
		KeyStoreFactories: map[string]signerapi.KeyStoreFactory[*signerapi.ConfigNoExt]{
			"ext-store": &testKeyStoreAllFactory{keyStore: &testKeyStoreAll{}},
		},
	}

	ctx := context.Background()
	_, err := NewSigningModule(ctx, &signerapi.ConfigNoExt{
		KeyDerivation: pldconf.KeyDerivationConfig{
			Type: pldconf.KeyDerivationTypeBIP32,
		},
		KeyStore: pldconf.KeyStoreConfig{
			KeyStoreSigning: true,
			Type:            "ext-store",
		},
	}, te)
	assert.Regexp(t, "PD020808", err)

}

func TestHDSigningInitFailBadMnemonic(t *testing.T) {

	ctx := context.Background()
	_, err := NewSigningModule(ctx, &signerapi.ConfigNoExt{
		KeyDerivation: pldconf.KeyDerivationConfig{
			Type: pldconf.KeyDerivationTypeBIP32,
		},
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
			Static: pldconf.StaticKeyStoreConfig{
				Keys: map[string]pldconf.StaticKeyEntryConfig{
					"seed": {
						Encoding: "none",
						Inline:   "wrong",
					},
				},
			},
		},
	})
	assert.Regexp(t, "PD020812", err)

}

func TestHDInitBadSeed(t *testing.T) {

	ctx := context.Background()
	entropy, err := bip39.NewEntropy(256)
	require.NoError(t, err)

	mnemonic, err := bip39.NewMnemonic(entropy)
	require.NoError(t, err)

	_, err = NewSigningModule(ctx, &signerapi.ConfigNoExt{
		KeyDerivation: pldconf.KeyDerivationConfig{
			Type: pldconf.KeyDerivationTypeBIP32,
			SeedKeyPath: pldconf.StaticKeyReference{
				Name: "missing",
			},
		},
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
			Static: pldconf.StaticKeyStoreConfig{
				Keys: map[string]pldconf.StaticKeyEntryConfig{
					"seed": {
						Encoding: "none",
						Inline:   mnemonic,
					},
				},
			},
		},
	})
	assert.Regexp(t, "PD020818", err)

}

func TestHDInitGenSeed(t *testing.T) {

	ctx := context.Background()

	sm, err := NewSigningModule(ctx, &signerapi.ConfigNoExt{
		KeyDerivation: pldconf.KeyDerivationConfig{
			Type: pldconf.KeyDerivationTypeBIP32,
			SeedKeyPath: pldconf.StaticKeyReference{
				Name: "seed",
				Path: []pldconf.ConfigKeyPathEntry{{Name: "generate"}},
			},
		},
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeFilesystem,
			FileSystem: pldconf.FileSystemKeyStoreConfig{
				Path: confutil.P(t.TempDir()),
			},
		},
	})
	require.NoError(t, err)

	generatedSeed, err := sm.(*signingModule[*signerapi.ConfigNoExt]).keyStore.LoadKeyMaterial(ctx, "generate/seed")
	require.NoError(t, err)
	assert.Len(t, generatedSeed, 32)
	assert.NotEqual(t, make([]byte, 32), generatedSeed) // not zero
}
