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

package ethclient

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/stretchr/testify/assert"
)

type mockKeyManager struct {
	resolveKey func(ctx context.Context, identifier string, algorithm string) (keyHandle, verifier string, err error)
	sign       func(ctx context.Context, req *proto.SignRequest) (*proto.SignResponse, error)
}

func (mkm *mockKeyManager) ResolveKey(ctx context.Context, identifier string, algorithm string) (keyHandle, verifier string, err error) {
	return mkm.resolveKey(ctx, identifier, algorithm)
}

func (mkm *mockKeyManager) Sign(ctx context.Context, req *proto.SignRequest) (*proto.SignResponse, error) {
	return mkm.sign(ctx, req)
}

func newTestHDWalletKeyManager(t *testing.T) *simpleKeyManager {
	kmgr, err := NewSimpleTestKeyManager(context.Background(), &signer.Config{
		KeyDerivation: signer.KeyDerivationConfig{
			Type: signer.KeyDerivationTypeBIP32,
		},
		KeyStore: signer.StoreConfig{
			Type: signer.KeyStoreTypeStatic,
			Static: signer.StaticKeyStorageConfig{
				Keys: map[string]signer.StaticKeyEntryConfig{
					"seed": {
						Encoding: "hex",
						Inline:   types.RandHex(32),
					},
				},
			},
		},
	})
	assert.NoError(t, err)
	return kmgr.(*simpleKeyManager)
}

func TestSimpleKeyManagerInitFail(t *testing.T) {
	_, err := NewSimpleTestKeyManager(context.Background(), &signer.Config{
		KeyDerivation: signer.KeyDerivationConfig{
			Type: signer.KeyDerivationTypeBIP32,
		},
		KeyStore: signer.StoreConfig{
			Type: signer.KeyStoreTypeStatic,
		},
	})
	assert.Regexp(t, "PD011418", err)

}

func TestGenerateIndexes(t *testing.T) {
	kmgr := newTestHDWalletKeyManager(t)
	for iFolder := 0; iFolder < 10; iFolder++ {
		for iKey := 0; iKey < 10; iKey++ {
			keyHandle, addr, err := kmgr.ResolveKey(context.Background(), fmt.Sprintf("my/one-use-set-%d/%s", iFolder, uuid.New()), signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES)
			assert.NoError(t, err)
			assert.NotEmpty(t, ethtypes.MustNewAddress(addr))
			assert.Equal(t, fmt.Sprintf("m/44'/60'/0'/%d/%d", iFolder, iKey), keyHandle)
		}
	}
}

func TestKeyManagerResolveFail(t *testing.T) {

	kmgr, err := NewSimpleTestKeyManager(context.Background(), &signer.Config{
		KeyStore: signer.StoreConfig{
			Type: signer.KeyStoreTypeStatic,
		},
	})
	assert.NoError(t, err)

	_, _, err = kmgr.ResolveKey(context.Background(), "does not exist", signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES)
	assert.Regexp(t, "PD011418", err)
}

func TestKeyManagerResolveConflict(t *testing.T) {

	kmgr := newTestHDWalletKeyManager(t)

	kmgr.rootFolder.Keys = map[string]*keyMapping{
		"key1": {
			Name:        "key1",
			KeyHandle:   "existing",
			Identifiers: map[string]string{},
		},
	}

	_, _, err := kmgr.ResolveKey(context.Background(), "key1", signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES)
	assert.Regexp(t, "PD011509", err)
}
