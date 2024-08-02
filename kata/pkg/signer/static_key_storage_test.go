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
	"encoding/base64"
	"encoding/hex"
	"os"
	"path"
	"testing"

	"github.com/kaleido-io/paladin/kata/internal/types"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/stretchr/testify/assert"
)

func newTestStaticStore(t *testing.T, keys map[string]StaticKeyEntryConfig) (context.Context, *staticStore) {
	ctx := context.Background()

	fs, err := newStaticKeyStore(ctx, &StaticKeyStorageConfig{
		Keys: keys,
	})
	assert.NoError(t, err)

	return ctx, fs.(*staticStore)
}

func TestStaticStoreFileFileWithTrim(t *testing.T) {

	keyData := types.RandHex(32)
	keyFile := path.Join(t.TempDir(), "my.key")
	err := os.WriteFile(keyFile, []byte(keyData+"\n"), 0644)
	assert.NoError(t, err)

	ctx, store := newTestStaticStore(t, map[string]StaticKeyEntryConfig{
		"myKey": {
			Encoding: "none",
			Filename: keyFile,
			Trim:     true,
		},
	})

	loadedKey, err := store.LoadKeyMaterial(ctx, "myKey")
	assert.NoError(t, err)
	assert.Equal(t, ([]byte)(keyData), loadedKey)

}

func TestStaticStoreHexLoadFile(t *testing.T) {

	keyData := types.RandHex(32)
	keyFile := path.Join(t.TempDir(), "my.key")
	err := os.WriteFile(keyFile, []byte(keyData), 0644)
	assert.NoError(t, err)

	ctx, store := newTestStaticStore(t, map[string]StaticKeyEntryConfig{
		"myKey": {
			Encoding: "hex",
			Filename: keyFile,
			Trim:     true,
		},
	})

	loadedKey, err := store.LoadKeyMaterial(ctx, "myKey")
	assert.NoError(t, err)
	keyDataDecoded, err := hex.DecodeString(keyData)
	assert.NoError(t, err)
	assert.Equal(t, keyDataDecoded, loadedKey)

}

func TestStaticStoreBase64InConf(t *testing.T) {

	keyData, err := hex.DecodeString(types.RandHex(32))
	assert.NoError(t, err)
	b64KeyData := base64.StdEncoding.EncodeToString(keyData)

	ctx, store := newTestStaticStore(t, map[string]StaticKeyEntryConfig{
		"myKey": {
			Encoding: "base64",
			Inline:   b64KeyData,
		},
	})

	loadedKey, err := store.LoadKeyMaterial(ctx, "myKey")
	assert.NoError(t, err)
	assert.Equal(t, keyData, loadedKey)

}

func TestStaticStoreLoadFileFail(t *testing.T) {

	_, err := newStaticKeyStore(context.Background(), &StaticKeyStorageConfig{
		Keys: map[string]StaticKeyEntryConfig{
			"myKey": {
				Encoding: "none",
				Filename: t.TempDir(),
				Trim:     true,
			},
		},
	})
	assert.Regexp(t, "PD011415", err)

}

func TestStaticStoreBadHEX(t *testing.T) {

	_, err := newStaticKeyStore(context.Background(), &StaticKeyStorageConfig{
		Keys: map[string]StaticKeyEntryConfig{
			"myKey": {
				Encoding: "hex",
				Inline:   "not hex",
			},
		},
	})
	assert.Regexp(t, "PD011415", err)

}

func TestStaticStoreBadBase64(t *testing.T) {

	_, err := newStaticKeyStore(context.Background(), &StaticKeyStorageConfig{
		Keys: map[string]StaticKeyEntryConfig{
			"myKey": {
				Encoding: "base64",
				Inline:   "!$$**~~",
			},
		},
	})
	assert.Regexp(t, "PD011415", err)

}

func TestStaticStoreEmpty(t *testing.T) {

	_, err := newStaticKeyStore(context.Background(), &StaticKeyStorageConfig{
		Keys: map[string]StaticKeyEntryConfig{
			"myKey": {
				Encoding: "none",
				Trim:     true,
				Inline:   "     ",
			},
		},
	})
	assert.Regexp(t, "PD011415", err)

}

func TestStaticStoreBadEncType(t *testing.T) {

	_, err := newStaticKeyStore(context.Background(), &StaticKeyStorageConfig{
		Keys: map[string]StaticKeyEntryConfig{
			"myKey": {
				Encoding: "",
				Inline:   "anything",
			},
		},
	})
	assert.Regexp(t, "PD011416", err)

}

func TestStaticStoreResolveOK(t *testing.T) {

	ctx, store := newTestStaticStore(t, map[string]StaticKeyEntryConfig{
		"my/shiny/key%20ten": {
			Encoding: "none",
			Inline:   "my key",
		},
	})

	keyData, keyHandle, err := store.FindOrCreateLoadableKey(ctx, &proto.ResolveKeyRequest{
		Path: []*proto.KeyPathSegment{
			{Name: "my"},
			{Name: "shiny"},
			{Name: "key ten"},
		},
	}, nil)
	assert.NoError(t, err)
	assert.Equal(t, "my/shiny/key%20ten", keyHandle)
	assert.Equal(t, ([]byte)("my key"), keyData)

}

func TestStaticStoreResolveBadPath(t *testing.T) {

	ctx, store := newTestStaticStore(t, map[string]StaticKeyEntryConfig{
		"my/shiny/key%20ten": {
			Encoding: "none",
			Inline:   "my key",
		},
	})

	_, _, err := store.FindOrCreateLoadableKey(ctx, &proto.ResolveKeyRequest{
		Path: []*proto.KeyPathSegment{},
	}, nil)
	assert.Regexp(t, "PD011403", err)

	_, _, err = store.FindOrCreateLoadableKey(ctx, &proto.ResolveKeyRequest{
		Path: []*proto.KeyPathSegment{
			{Name: ""},
		},
	}, nil)
	assert.Regexp(t, "PD011403", err)

}

func TestStaticStoreResolveNotFound(t *testing.T) {

	ctx, store := newTestStaticStore(t, map[string]StaticKeyEntryConfig{
		"my/shiny/key%20ten": {
			Encoding: "none",
			Inline:   "my key",
		},
	})

	_, _, err := store.FindOrCreateLoadableKey(ctx, &proto.ResolveKeyRequest{
		Path: []*proto.KeyPathSegment{
			{Name: "my"},
			{Name: "shiny"},
			{Name: "key eleven"},
		},
	}, nil)
	assert.Regexp(t, "PD011417", err)

}
