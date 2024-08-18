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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/stretchr/testify/assert"
)

func newTestFilesystemStore(t *testing.T) (context.Context, *filesystemStore) {
	ctx := context.Background()

	store, err := NewFilesystemStore(ctx, api.FileSystemConfig{
		Path: confutil.P(t.TempDir()),
	})
	assert.NoError(t, err)

	return ctx, store.(*filesystemStore)
}

func TestFileSystemStoreBadDir(t *testing.T) {

	badPath := path.Join(t.TempDir(), "wrong")

	_, err := NewFilesystemStore(context.Background(), api.FileSystemConfig{
		Path: confutil.P(badPath),
	})
	assert.Regexp(t, "PD011400", err)

	err = os.WriteFile(badPath, []byte{}, 0644)
	assert.NoError(t, err)

	_, err = NewFilesystemStore(context.Background(), api.FileSystemConfig{
		Path: confutil.P(badPath),
	})
	assert.Regexp(t, "PD011400", err)
}

func TestFileSystemStoreCreateSecp256k1(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	key0, err := secp256k1.GenerateSecp256k1KeyPair()
	assert.NoError(t, err)

	keyBytes, keyHandle, err := fs.FindOrCreateLoadableKey(ctx, &proto.ResolveKeyRequest{
		Name: "42",
		Path: []*proto.ResolveKeyPathSegment{{Name: "bob"}, {Name: "blue"}},
	}, func() ([]byte, error) { return key0.PrivateKeyBytes(), nil })
	assert.NoError(t, err)

	assert.Equal(t, keyBytes, key0.PrivateKeyBytes())
	assert.Equal(t, "bob/blue/42", keyHandle)
	cached, _ := fs.cache.Get(keyHandle)
	assert.NotNil(t, cached)

	keyBytes, err = fs.LoadKeyMaterial(ctx, keyHandle)
	assert.NoError(t, err)
	assert.Equal(t, keyBytes, key0.PrivateKeyBytes())

	fs.cache.Delete(keyHandle)

	keyBytes, err = fs.LoadKeyMaterial(ctx, keyHandle)
	assert.NoError(t, err)
	assert.Equal(t, keyBytes, key0.PrivateKeyBytes())

	// Check the JSON doesn't contain an address
	var jsonWallet map[string]interface{}
	b, err := os.ReadFile(path.Join(fs.path, "_bob", "_blue", "-42.key"))
	assert.NoError(t, err)
	err = json.Unmarshal(b, &jsonWallet)
	assert.NoError(t, err)
	_, hasAddressProperty := jsonWallet["address"]
	assert.False(t, hasAddressProperty)

}

func TestFileSystemStoreCreateBabyjubjub(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	privKey := babyjub.NewRandPrivKey()

	keyBytes, keyHandle, err := fs.FindOrCreateLoadableKey(ctx, &proto.ResolveKeyRequest{
		Name: "42",
		Path: []*proto.ResolveKeyPathSegment{
			{Name: "bob"},
			{Name: "blue"},
		},
	}, func() ([]byte, error) { return privKey[:], nil })
	assert.NoError(t, err)

	assert.Equal(t, keyBytes, privKey[:])
	assert.Equal(t, "bob/blue/42", keyHandle)
	cached, _ := fs.cache.Get(keyHandle)
	assert.NotNil(t, cached)

	keyBytes, err = fs.LoadKeyMaterial(ctx, keyHandle)
	assert.NoError(t, err)
	assert.Equal(t, keyBytes, privKey[:])

	fs.cache.Delete(keyHandle)

	keyBytes, err = fs.LoadKeyMaterial(ctx, keyHandle)
	assert.NoError(t, err)
	assert.Equal(t, keyBytes, privKey[:])

}

func TestFileSystemStoreCreateReloadMnemonic(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	phrase := []byte("fame point uphold pumpkin april violin orphan cat bid upper meadow family")

	keyBytes, keyHandle, err := fs.FindOrCreateLoadableKey(ctx, &proto.ResolveKeyRequest{
		Name: "sally",
	}, func() ([]byte, error) { return phrase, nil })
	assert.NoError(t, err)

	assert.Equal(t, phrase, keyBytes)
	assert.Equal(t, "sally", keyHandle)
	cached, _ := fs.cache.Get(keyHandle)
	assert.NotNil(t, cached)

	keyBytes, err = fs.LoadKeyMaterial(ctx, keyHandle)
	assert.NoError(t, err)
	assert.Equal(t, phrase, keyBytes)

	fs.cache.Delete(keyHandle)

	keyBytes, err = fs.LoadKeyMaterial(ctx, keyHandle)
	assert.NoError(t, err)
	assert.Equal(t, phrase, keyBytes)

}

func TestFileSystemStoreBadSegments(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	_, _, err := fs.FindOrCreateLoadableKey(ctx, &proto.ResolveKeyRequest{}, nil)
	assert.Regexp(t, "PD011403", err)

	_, _, err = fs.FindOrCreateLoadableKey(ctx, &proto.ResolveKeyRequest{
		Path: []*proto.ResolveKeyPathSegment{
			{},
		},
	}, nil)
	assert.Regexp(t, "PD011403", err)
}

func TestFileSystemClashes(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	err := os.MkdirAll(path.Join(fs.path, "-clash"), fs.dirMode)
	assert.NoError(t, err)

	_, _, err = fs.FindOrCreateLoadableKey(ctx, &proto.ResolveKeyRequest{
		Name: "clash",
	}, func() ([]byte, error) { return []byte("key1"), nil })
	assert.Regexp(t, "PD011405", err)

}

func TestCreateWalletFileFail(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	err := os.MkdirAll(path.Join(fs.path, "clash.key"), fs.dirMode)
	assert.NoError(t, err)

	_, err = fs.createWalletFile(ctx, path.Join(fs.path, "clash.key"), path.Join(fs.path, "clash.pwd"),
		func() ([]byte, error) { return []byte{}, nil })
	assert.Regexp(t, "PD011404", err)

	_, err = fs.createWalletFile(ctx, path.Join(fs.path, "ok.key"), path.Join(fs.path, "ok.pwd"),
		func() ([]byte, error) { return nil, fmt.Errorf("pop") })
	assert.Regexp(t, "pop", err)

}

func TestReadWalletFileFail(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	err := os.MkdirAll(path.Join(fs.path, "dir.key"), fs.dirMode)
	assert.NoError(t, err)

	_, err = fs.readWalletFile(ctx, path.Join(fs.path, "dir"), "")
	assert.Regexp(t, "PD011401", err)

}

func TestReadPassFileFail(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	keyFilePath, passwordFilePath := path.Join(fs.path, "ok.key"), path.Join(fs.path, "fail.pass")

	_, err := fs.createWalletFile(ctx, keyFilePath, passwordFilePath,
		func() ([]byte, error) { return []byte{0x01}, nil })
	assert.NoError(t, err)

	err = os.Remove(passwordFilePath)
	assert.NoError(t, err)

	_, err = fs.readWalletFile(ctx, keyFilePath, passwordFilePath)
	assert.Regexp(t, "PD011402", err)
}

func TestLoadKeyFail(t *testing.T) {
	ctx, fs := newTestFilesystemStore(t)

	_, err := fs.LoadKeyMaterial(ctx, "wrong")
	assert.Regexp(t, "PD011406", err)
}
