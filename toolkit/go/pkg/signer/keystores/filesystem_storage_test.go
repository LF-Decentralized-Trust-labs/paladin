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

package keystores

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestFilesystemStore(t *testing.T) (context.Context, *filesystemStore) {
	ctx := context.Background()

	sf := NewFilesystemStoreFactory[*signerapi.ConfigNoExt]()
	store, err := sf.NewKeyStore(ctx, &signerapi.ConfigNoExt{
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeFilesystem,
			FileSystem: pldconf.FileSystemKeyStoreConfig{
				Path: confutil.P(t.TempDir()),
			},
		},
	})
	require.NoError(t, err)

	store.Close()

	return ctx, store.(*filesystemStore)
}

func TestFileSystemStoreBadDir(t *testing.T) {

	badPath := path.Join(t.TempDir(), "wrong")

	sf := NewFilesystemStoreFactory[*signerapi.ConfigNoExt]()
	_, err := sf.NewKeyStore(context.Background(), &signerapi.ConfigNoExt{
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeFilesystem,
			FileSystem: pldconf.FileSystemKeyStoreConfig{
				Path: confutil.P(badPath),
			},
		},
	})
	assert.Regexp(t, "PD020800", err)

	err = os.WriteFile(badPath, []byte{}, 0644)
	require.NoError(t, err)

	_, err = sf.NewKeyStore(context.Background(), &signerapi.ConfigNoExt{
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeFilesystem,
			FileSystem: pldconf.FileSystemKeyStoreConfig{
				Path: confutil.P(badPath),
			},
		},
	})
	assert.Regexp(t, "PD020800", err)
}

func TestFileSystemStoreCreate(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	key0, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)

	keyBytes, keyHandle, err := fs.FindOrCreateLoadableKey(ctx, &prototk.ResolveKeyRequest{
		Name: "42",
		Path: []*prototk.ResolveKeyPathSegment{{Name: "bob"}, {Name: "blue"}},
	}, func() ([]byte, error) { return key0.PrivateKeyBytes(), nil })
	require.NoError(t, err)

	assert.Equal(t, keyBytes, key0.PrivateKeyBytes())
	assert.Equal(t, "bob/blue/42", keyHandle)
	cached, _ := fs.cache.Get(keyHandle)
	assert.NotNil(t, cached)

	keyBytes, err = fs.LoadKeyMaterial(ctx, keyHandle)
	require.NoError(t, err)
	assert.Equal(t, keyBytes, key0.PrivateKeyBytes())

	fs.cache.Delete(keyHandle)

	keyBytes, err = fs.LoadKeyMaterial(ctx, keyHandle)
	require.NoError(t, err)
	assert.Equal(t, keyBytes, key0.PrivateKeyBytes())

	// Check the JSON doesn't contain an address
	var jsonWallet map[string]interface{}
	b, err := os.ReadFile(path.Join(fs.path, "_bob", "_blue", "-42.key"))
	require.NoError(t, err)
	err = json.Unmarshal(b, &jsonWallet)
	require.NoError(t, err)
	_, hasAddressProperty := jsonWallet["address"]
	assert.False(t, hasAddressProperty)

}

func TestFileSystemStoreCreateReloadMnemonic(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	phrase := []byte("fame point uphold pumpkin april violin orphan cat bid upper meadow family")

	keyBytes, keyHandle, err := fs.FindOrCreateLoadableKey(ctx, &prototk.ResolveKeyRequest{
		Name: "sally",
	}, func() ([]byte, error) { return phrase, nil })
	require.NoError(t, err)

	assert.Equal(t, phrase, keyBytes)
	assert.Equal(t, "sally", keyHandle)
	cached, _ := fs.cache.Get(keyHandle)
	assert.NotNil(t, cached)

	keyBytes, err = fs.LoadKeyMaterial(ctx, keyHandle)
	require.NoError(t, err)
	assert.Equal(t, phrase, keyBytes)

	fs.cache.Delete(keyHandle)

	keyBytes, err = fs.LoadKeyMaterial(ctx, keyHandle)
	require.NoError(t, err)
	assert.Equal(t, phrase, keyBytes)

}

func TestFileSystemStoreBadSegments(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	_, _, err := fs.FindOrCreateLoadableKey(ctx, &prototk.ResolveKeyRequest{}, nil)
	assert.Regexp(t, "PD020803", err)

	_, _, err = fs.FindOrCreateLoadableKey(ctx, &prototk.ResolveKeyRequest{
		Path: []*prototk.ResolveKeyPathSegment{
			{},
		},
	}, nil)
	assert.Regexp(t, "PD020803", err)
}

func TestFileSystemClashes(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	err := os.MkdirAll(path.Join(fs.path, "-clash"), fs.dirMode)
	require.NoError(t, err)

	_, _, err = fs.FindOrCreateLoadableKey(ctx, &prototk.ResolveKeyRequest{
		Name: "clash",
	}, func() ([]byte, error) { return []byte("key1"), nil })
	assert.Regexp(t, "PD020805", err)

}

func TestCreateWalletFileFail(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	err := os.MkdirAll(path.Join(fs.path, "clash.key"), fs.dirMode)
	require.NoError(t, err)

	_, err = fs.createWalletFile(ctx, path.Join(fs.path, "clash.key"), path.Join(fs.path, "clash.pwd"),
		func() ([]byte, error) { return []byte{}, nil })
	assert.Regexp(t, "PD020804", err)

	_, err = fs.createWalletFile(ctx, path.Join(fs.path, "ok.key"), path.Join(fs.path, "ok.pwd"),
		func() ([]byte, error) { return nil, fmt.Errorf("pop") })
	assert.Regexp(t, "pop", err)

}

func TestReadWalletFileFail(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	err := os.MkdirAll(path.Join(fs.path, "dir.key"), fs.dirMode)
	require.NoError(t, err)

	_, err = fs.readWalletFile(ctx, path.Join(fs.path, "dir"), "")
	assert.Regexp(t, "PD020801", err)

}

func TestReadPassFileFail(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	keyFilePath, passwordFilePath := path.Join(fs.path, "ok.key"), path.Join(fs.path, "fail.pass")

	_, err := fs.createWalletFile(ctx, keyFilePath, passwordFilePath,
		func() ([]byte, error) { return []byte{0x01}, nil })
	require.NoError(t, err)

	err = os.Remove(passwordFilePath)
	require.NoError(t, err)

	_, err = fs.readWalletFile(ctx, keyFilePath, passwordFilePath)
	assert.Regexp(t, "PD020802", err)
}

func TestLoadKeyFail(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	_, err := fs.LoadKeyMaterial(ctx, "wrong")
	assert.Regexp(t, "PD020806", err)
}
