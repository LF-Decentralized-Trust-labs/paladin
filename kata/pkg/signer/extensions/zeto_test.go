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

package extensions

import (
	"context"
	"encoding/hex"
	"os"
	"path"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/key"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/types"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
	"github.com/kaleido-io/paladin/kata/pkg/signer/keystore"
)

func newTestFilesystemStore(t *testing.T) (context.Context, api.KeyStore, string) {
	ctx := context.Background()

	dirPath := t.TempDir()
	store, err := keystore.NewFilesystemStore(ctx, &keystore.FileSystemConfig{
		Path: confutil.P(dirPath),
	})
	assert.NoError(t, err)

	return ctx, store, dirPath
}

func newTestStaticStore(t *testing.T, keys map[string]keystore.StaticKeyEntryConfig) (context.Context, api.KeyStore) {
	ctx := context.Background()

	store, err := keystore.NewStaticKeyStore(ctx, &keystore.StaticKeyStorageConfig{
		Keys: keys,
	})
	assert.NoError(t, err)

	return ctx, store
}

func TestZetoKeystoreExtension(t *testing.T) {
	zke := NewZetoSignerExtension()
	ks, err := zke.KeyStore("")
	assert.NoError(t, err)
	_, _, err = ks.FindOrCreateLoadableKey(context.Background(), &proto.ResolveKeyRequest{}, nil)
	assert.NoError(t, err)
	_, err = ks.LoadKeyMaterial(context.Background(), "")
	assert.NoError(t, err)
}

func TestFileSystemStoreCreateBJJ(t *testing.T) {
	ctx, fs, _ := newTestFilesystemStore(t)

	key0 := babyjub.NewRandPrivKey()

	keyBytes, keyHandle, err := fs.FindOrCreateLoadableKey(ctx, &proto.ResolveKeyRequest{
		Path: []*proto.KeyPathSegment{
			{Name: "bob"},
			{Name: "blue"},
			{Name: "42"},
		},
	}, func() ([]byte, error) { return key0[:], nil })
	assert.NoError(t, err)

	assert.Equal(t, keyBytes, key0[:])
	assert.Equal(t, "bob/blue/42", keyHandle)

	keyBytes, err = fs.LoadKeyMaterial(ctx, keyHandle)
	assert.NoError(t, err)
	assert.Equal(t, keyBytes, key0[:])

	privKeyBytes := [32]byte{}
	copy(privKeyBytes[:], keyBytes)
	keyEntry := key.NewKeyEntryFromPrivateKeyBytes(privKeyBytes)
	assert.NotZero(t, keyEntry.PrivateKey)
	assert.NotZero(t, keyEntry.PublicKey)
	assert.NotZero(t, keyEntry.PrivateKeyForZkp)
}

func TestStaticStoreFileFileWithTrim(t *testing.T) {
	keyData := types.RandHex(32)
	keyFile := path.Join(t.TempDir(), "my.key")
	err := os.WriteFile(keyFile, []byte(keyData+"\n"), 0644)
	assert.NoError(t, err)

	ctx, store := newTestStaticStore(t, map[string]keystore.StaticKeyEntryConfig{
		"myKey": {
			Encoding: "none",
			Filename: keyFile,
			Trim:     true,
		},
	})

	loadedKey, err := store.LoadKeyMaterial(ctx, "myKey")
	assert.NoError(t, err)
	assert.Equal(t, ([]byte)(keyData), loadedKey)

	var keyBytes [32]byte
	copy(keyBytes[:], keyData)
	keyEntry := key.NewKeyEntryFromPrivateKeyBytes(keyBytes)
	assert.NotZero(t, keyEntry.PrivateKey)
	assert.NotZero(t, keyEntry.PublicKey)
	assert.NotZero(t, keyEntry.PrivateKeyForZkp)
}

func TestZKPSigningModuleUsingFileSystemStore(t *testing.T) {
	ctx, fs, dir := newTestFilesystemStore(t)

	// // create a BJJ key in the filesystem store
	keyHex := "627d15ca47363fb118997679bc8941d1ae16a034dc8ae96c938e3997e3d6ca98"
	keyBytes, _ := hex.DecodeString(keyHex)
	privKeyBytes := [32]byte{}
	copy(privKeyBytes[:], keyBytes)
	key0 := key.NewKeyEntryFromPrivateKeyBytes(privKeyBytes)

	_, _, err := fs.FindOrCreateLoadableKey(ctx, &proto.ResolveKeyRequest{
		Path: []*proto.KeyPathSegment{
			{Name: "bob"},
			{Name: "blue"},
		},
	}, func() ([]byte, error) { return key0.PrivateKey[:], nil })
	assert.NoError(t, err)

	sm, err := signer.NewSigningModule(ctx, &signer.Config{
		KeyStore: signer.StoreConfig{
			Type:       signer.KeyStoreTypeFilesystem,
			FileSystem: keystore.FileSystemConfig{Path: confutil.P(dir)},
		},
	})
	assert.NoError(t, err)
	assert.NotZero(t, sm)

	resp, err := sm.Resolve(ctx, &proto.ResolveKeyRequest{
		Algorithms: []string{signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES, signer.Algorithm_ZKP_BABYJUBJUB_PLAINBYTES},
		Path: []*proto.KeyPathSegment{
			{Name: "bob"},
			{Name: "blue"},
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, len(resp.Identifiers))
	assert.Equal(t, "0xa08be87002f2548dfb055eb4f6ef0508751e56bc", resp.Identifiers[0].Identifier)
	assert.Equal(t, "0feb0bde75701adb03d08ee386f44722c7504c1e2e6f158f70582e0d0b2d67a4", resp.Identifiers[1].Identifier)
}
