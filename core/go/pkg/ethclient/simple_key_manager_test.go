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

package ethclient

import (
	"context"
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockKeyManager struct {
	resolveKey func(ctx context.Context, identifier, algorithm, verifierType string) (keyHandle, verifier string, err error)
	sign       func(ctx context.Context, req *prototk.SignWithKeyRequest) (*prototk.SignWithKeyResponse, error)
}

// AddInMemorySigner implements KeyManager.
func (mkm *mockKeyManager) AddInMemorySigner(prefix string, signer signerapi.InMemorySigner) {}

func (mkm *mockKeyManager) ResolveKey(ctx context.Context, identifier, algorithm, verifierType string) (keyHandle, verifier string, err error) {
	return mkm.resolveKey(ctx, identifier, algorithm, verifierType)
}

func (mkm *mockKeyManager) Sign(ctx context.Context, req *prototk.SignWithKeyRequest) (*prototk.SignWithKeyResponse, error) {
	return mkm.sign(ctx, req)
}

func (mkm *mockKeyManager) Close() {

}

type mockSigner struct {
	getMinimumKeyLen func(ctx context.Context, algorithm string) (int, error)
	getVerifier      func(ctx context.Context, algorithm string, verifierType string, privateKey []byte) (string, error)
	sign             func(ctx context.Context, algorithm string, payloadType string, privateKey []byte, payload []byte) ([]byte, error)
}

func (m *mockSigner) GetMinimumKeyLen(ctx context.Context, algorithm string) (int, error) {
	return m.getMinimumKeyLen(ctx, algorithm)
}

func (m *mockSigner) GetVerifier(ctx context.Context, algorithm string, verifierType string, privateKey []byte) (string, error) {
	return m.getVerifier(ctx, algorithm, verifierType, privateKey)
}

func (m *mockSigner) Sign(ctx context.Context, algorithm string, payloadType string, privateKey []byte, payload []byte) ([]byte, error) {
	return m.sign(ctx, algorithm, payloadType, privateKey, payload)
}

func newTestHDWalletKeyManager(t *testing.T) (*simpleKeyManager, func()) {
	kmgr, err := NewSimpleTestKeyManager(context.Background(), &signerapi.ConfigNoExt{
		KeyDerivation: pldconf.KeyDerivationConfig{
			Type: pldconf.KeyDerivationTypeBIP32,
		},
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
			Static: pldconf.StaticKeyStoreConfig{
				Keys: map[string]pldconf.StaticKeyEntryConfig{
					"seed": {
						Encoding: "hex",
						Inline:   pldtypes.RandHex(32),
					},
				},
			},
		},
	})
	require.NoError(t, err)
	return kmgr.(*simpleKeyManager), kmgr.Close
}

func TestSimpleKeyManagerInitFail(t *testing.T) {
	_, err := NewSimpleTestKeyManager(context.Background(), &signerapi.ConfigNoExt{
		KeyDerivation: pldconf.KeyDerivationConfig{
			Type: pldconf.KeyDerivationTypeBIP32,
		},
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
		},
	})
	assert.Regexp(t, "PD020818", err)
}

func TestSimpleKeyManagerPassThoroughInMemSigner(t *testing.T) {
	sm, err := NewSimpleTestKeyManager(context.Background(), &signerapi.ConfigNoExt{
		KeyDerivation: pldconf.KeyDerivationConfig{
			Type: pldconf.KeyDerivationTypeBIP32,
		},
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
			Static: pldconf.StaticKeyStoreConfig{
				Keys: map[string]pldconf.StaticKeyEntryConfig{
					"seed": {
						Encoding: "hex",
						Inline:   pldtypes.RandHex(32),
					},
				},
			},
		},
	})
	require.NoError(t, err)

	sm.AddInMemorySigner("bad", &mockSigner{
		getVerifier: func(ctx context.Context, algorithm, verifierType string, privateKey []byte) (string, error) {
			return "", fmt.Errorf("pop")
		},
	})
	_, _, err = sm.ResolveKey(context.Background(), "any", "bad:test", verifiers.ETH_ADDRESS)
	assert.Regexp(t, "pop", err)
}

func TestGenerateIndexes(t *testing.T) {
	kmgr, done := newTestHDWalletKeyManager(t)
	defer done()
	for iFolder := 0; iFolder < 10; iFolder++ {
		for iKey := 0; iKey < 10; iKey++ {
			keyHandle, addr, err := kmgr.ResolveKey(context.Background(), fmt.Sprintf("my/one-use-set-%d/%s", iFolder, uuid.New()), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
			require.NoError(t, err)
			assert.NotEmpty(t, ethtypes.MustNewAddress(addr))
			assert.Equal(t, fmt.Sprintf("m/44'/60'/0'/%d/%d", iFolder, iKey), keyHandle)
		}
	}
}

func TestKeyManagerResolveFail(t *testing.T) {

	kmgr, err := NewSimpleTestKeyManager(context.Background(), &signerapi.ConfigNoExt{
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
		},
	})
	require.NoError(t, err)

	_, _, err = kmgr.ResolveKey(context.Background(), "does not exist", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	assert.Regexp(t, "PD020818", err)
}

func TestKeyManagerResolveConflict(t *testing.T) {

	kmgr, done := newTestHDWalletKeyManager(t)
	defer done()

	kmgr.rootFolder.Keys = map[string]*keyMapping{
		"key1": {
			Name:        "key1",
			KeyHandle:   "existing",
			Identifiers: map[string]string{},
		},
	}

	_, _, err := kmgr.ResolveKey(context.Background(), "key1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	assert.Regexp(t, "PD011509", err)
}

func TestKeyManagerResolveSameKeyTwoVerifierTypes(t *testing.T) {

	kmgr, done := newTestHDWalletKeyManager(t)
	defer done()

	kmgr.rootFolder.Keys = map[string]*keyMapping{}

	keyHandle1, verifier1, err := kmgr.ResolveKey(context.Background(), "key1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)

	keyHandle2, verifier2, err := kmgr.ResolveKey(context.Background(), "key1", algorithms.ECDSA_SECP256K1, verifiers.HEX_ECDSA_PUBKEY_UNCOMPRESSED_0X)
	require.NoError(t, err)

	assert.Equal(t, keyHandle1, keyHandle2)
	assert.NotEqual(t, verifier1, verifier2)
}
