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
	"testing"

	"github.com/kaleido-io/paladin/kata/internal/types"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer"
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

func newTestHDWalletKeyManager(t *testing.T) KeyManager {
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
	return kmgr

}
