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

package keystore

import (
	"math/big"
	"os"
	"path"
	"testing"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/key"
	"github.com/iden3/go-iden3-crypto/babyjub"
	pb "github.com/kaleido-io/paladin/core/pkg/proto"
	"github.com/kaleido-io/paladin/core/pkg/signer/api"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type User struct {
	PrivateKey       *babyjub.PrivateKey
	PublicKey        *babyjub.PublicKey
	PrivateKeyBigInt *big.Int
}

func NewKeypair() *User {
	babyJubjubPrivKey := babyjub.NewRandPrivKey()
	babyJubjubPubKey := babyJubjubPrivKey.Public()
	// convert the private key to big.Int for use inside circuits
	privKeyBigInt := babyjub.SkToBigInt(&babyJubjubPrivKey)

	return &User{
		PrivateKey:       &babyJubjubPrivKey,
		PublicKey:        babyJubjubPubKey,
		PrivateKeyBigInt: privKeyBigInt,
	}
}

func TestFileSystemStoreCreateBJJ(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	key0 := babyjub.NewRandPrivKey()

	keyBytes, keyHandle, err := fs.FindOrCreateLoadableKey(ctx, &pb.ResolveKeyRequest{
		Name: "42",
		Path: []*pb.ResolveKeyPathSegment{
			{Name: "bob"},
			{Name: "blue"},
		},
	}, func() ([]byte, error) { return key0[:], nil })
	require.NoError(t, err)

	assert.Equal(t, keyBytes, key0[:])
	assert.Equal(t, "bob/blue/42", keyHandle)

	keyBytes, err = fs.LoadKeyMaterial(ctx, keyHandle)
	require.NoError(t, err)
	assert.Equal(t, keyBytes, key0[:])

	privKeyBytes := [32]byte{}
	copy(privKeyBytes[:], keyBytes)
	keyEntry := key.NewKeyEntryFromPrivateKeyBytes(privKeyBytes)
	assert.NotZero(t, keyEntry.PrivateKey)
	assert.NotZero(t, keyEntry.PublicKey)
	assert.NotZero(t, keyEntry.PrivateKeyForZkp)
}

func TestStaticStoreFileFileWithTrimForBJJ(t *testing.T) {
	keyData := tktypes.RandHex(32)
	keyFile := path.Join(t.TempDir(), "my.key")
	err := os.WriteFile(keyFile, []byte(keyData+"\n"), 0644)
	require.NoError(t, err)

	ctx, store := newTestStaticStore(t, map[string]api.StaticKeyEntryConfig{
		"myKey": {
			Encoding: "none",
			Filename: keyFile,
			Trim:     true,
		},
	})

	loadedKey, err := store.LoadKeyMaterial(ctx, "myKey")
	require.NoError(t, err)
	assert.Equal(t, ([]byte)(keyData), loadedKey)

	var keyBytes [32]byte
	copy(keyBytes[:], keyData)
	keyEntry := key.NewKeyEntryFromPrivateKeyBytes(keyBytes)
	assert.NotZero(t, keyEntry.PrivateKey)
	assert.NotZero(t, keyEntry.PublicKey)
	assert.NotZero(t, keyEntry.PrivateKeyForZkp)
}
