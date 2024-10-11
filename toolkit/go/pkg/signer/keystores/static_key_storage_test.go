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

package keystores

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path"
	"testing"

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/toolkit/pkg/signerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStaticStore(t *testing.T, keys map[string]pldconf.StaticKeyEntryConfig) (context.Context, *staticStore) {
	ctx := context.Background()

	sf := NewStaticStoreFactory[*signerapi.ConfigNoExt]()
	store, err := sf.NewKeyStore(ctx, &signerapi.ConfigNoExt{
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
			Static: pldconf.StaticKeyStoreConfig{
				Keys: keys,
			},
		},
	})
	require.NoError(t, err)

	store.Close() // proving it's a no-op

	return ctx, store.(*staticStore)
}

func TestStaticStoreFileFileWithTrim(t *testing.T) {

	keyData := tktypes.RandHex(32)
	keyFile := path.Join(t.TempDir(), "my.key")
	err := os.WriteFile(keyFile, []byte(keyData+"\n"), 0644)
	require.NoError(t, err)

	ctx, store := newTestStaticStore(t, map[string]pldconf.StaticKeyEntryConfig{
		"myKey": {
			Encoding: "none",
			Filename: keyFile,
			Trim:     true,
		},
	})

	loadedKey, err := store.LoadKeyMaterial(ctx, "myKey")
	require.NoError(t, err)
	assert.Equal(t, ([]byte)(keyData), loadedKey)

}

func TestStaticStoreHexLoadFile(t *testing.T) {

	keyData := tktypes.RandHex(32)
	keyFile := path.Join(t.TempDir(), "my.key")
	err := os.WriteFile(keyFile, []byte(keyData), 0644)
	require.NoError(t, err)

	ctx, store := newTestStaticStore(t, map[string]pldconf.StaticKeyEntryConfig{
		"myKey": {
			Encoding: "hex",
			Filename: keyFile,
			Trim:     true,
		},
	})

	loadedKey, err := store.LoadKeyMaterial(ctx, "myKey")
	require.NoError(t, err)
	keyDataDecoded, err := hex.DecodeString(keyData)
	require.NoError(t, err)
	assert.Equal(t, keyDataDecoded, loadedKey)

}

func TestStaticStoreBase64InConf(t *testing.T) {

	keyData, err := hex.DecodeString(tktypes.RandHex(32))
	require.NoError(t, err)
	b64KeyData := base64.StdEncoding.EncodeToString(keyData)

	ctx, store := newTestStaticStore(t, map[string]pldconf.StaticKeyEntryConfig{
		"myKey": {
			Encoding: "base64",
			Inline:   b64KeyData,
		},
	})

	loadedKey, err := store.LoadKeyMaterial(ctx, "myKey")
	require.NoError(t, err)
	assert.Equal(t, keyData, loadedKey)

}

func TestStaticStoreLoadFileFail(t *testing.T) {

	sf := NewStaticStoreFactory[*signerapi.ConfigNoExt]()
	_, err := sf.NewKeyStore(context.Background(), &signerapi.ConfigNoExt{
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
			Static: pldconf.StaticKeyStoreConfig{
				Keys: map[string]pldconf.StaticKeyEntryConfig{
					"myKey": {
						Encoding: "none",
						Filename: t.TempDir(),
						Trim:     true,
					},
				},
			},
		},
	})
	assert.Regexp(t, "PD020816", err)

}

func TestStaticStoreBadHEX(t *testing.T) {

	sf := NewStaticStoreFactory[*signerapi.ConfigNoExt]()
	_, err := sf.NewKeyStore(context.Background(), &signerapi.ConfigNoExt{
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
			Static: pldconf.StaticKeyStoreConfig{
				Keys: map[string]pldconf.StaticKeyEntryConfig{
					"myKey": {
						Encoding: "hex",
						Inline:   "not hex",
					},
				},
			},
		},
	})
	assert.Regexp(t, "PD020816", err)

}

func TestStaticStoreBadBase64(t *testing.T) {

	sf := NewStaticStoreFactory[*signerapi.ConfigNoExt]()
	_, err := sf.NewKeyStore(context.Background(), &signerapi.ConfigNoExt{
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
			Static: pldconf.StaticKeyStoreConfig{
				Keys: map[string]pldconf.StaticKeyEntryConfig{
					"myKey": {
						Encoding: "base64",
						Inline:   "!$$**~~",
					},
				},
			},
		},
	})
	assert.Regexp(t, "PD020816", err)

}

func TestStaticStoreEmpty(t *testing.T) {

	sf := NewStaticStoreFactory[*signerapi.ConfigNoExt]()
	_, err := sf.NewKeyStore(context.Background(), &signerapi.ConfigNoExt{
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
			Static: pldconf.StaticKeyStoreConfig{
				Keys: map[string]pldconf.StaticKeyEntryConfig{
					"myKey": {
						Encoding: "none",
						Trim:     true,
						Inline:   "     ",
					},
				},
			},
		},
	})
	assert.Regexp(t, "PD020816", err)

}

func TestStaticStoreBadEncType(t *testing.T) {

	sf := NewStaticStoreFactory[*signerapi.ConfigNoExt]()
	_, err := sf.NewKeyStore(context.Background(), &signerapi.ConfigNoExt{
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
			Static: pldconf.StaticKeyStoreConfig{
				Keys: map[string]pldconf.StaticKeyEntryConfig{
					"myKey": {
						Encoding: "",
						Inline:   "anything",
					},
				},
			},
		},
	})
	assert.Regexp(t, "PD020817", err)

}

func TestStaticStoreResolveOK(t *testing.T) {

	ctx, store := newTestStaticStore(t, map[string]pldconf.StaticKeyEntryConfig{
		"my/shiny/key%20ten": {
			Encoding: "none",
			Inline:   "my key",
		},
	})

	keyData, keyHandle, err := store.FindOrCreateLoadableKey(ctx, &signerapi.ResolveKeyRequest{
		Name: "key ten",
		Path: []*signerapi.ResolveKeyPathSegment{
			{Name: "my"},
			{Name: "shiny"},
		},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "my/shiny/key%20ten", keyHandle)
	assert.Equal(t, ([]byte)("my key"), keyData)

}

func TestStaticStoreResolveBadPath(t *testing.T) {

	ctx, store := newTestStaticStore(t, map[string]pldconf.StaticKeyEntryConfig{
		"my/shiny/key%20ten": {
			Encoding: "none",
			Inline:   "my key",
		},
	})

	_, _, err := store.FindOrCreateLoadableKey(ctx, &signerapi.ResolveKeyRequest{}, nil)
	assert.Regexp(t, "PD020803", err)

	_, _, err = store.FindOrCreateLoadableKey(ctx, &signerapi.ResolveKeyRequest{
		Name: "something",
		Path: []*signerapi.ResolveKeyPathSegment{
			{Name: ""},
		},
	}, nil)
	assert.Regexp(t, "PD020803", err)

}

func TestStaticStoreResolveNotFound(t *testing.T) {

	ctx, store := newTestStaticStore(t, map[string]pldconf.StaticKeyEntryConfig{
		"my/shiny/key%20ten": {
			Encoding: "none",
			Inline:   "my key",
		},
	})

	_, _, err := store.FindOrCreateLoadableKey(ctx, &signerapi.ResolveKeyRequest{
		Name: "key eleven",
		Path: []*signerapi.ResolveKeyPathSegment{
			{Name: "my"},
			{Name: "shiny"},
		},
	}, nil)
	assert.Regexp(t, "PD020818", err)

}

func TestStaticStoreWholeStoreInFile(t *testing.T) {

	filePath := path.Join(t.TempDir(), "keystore.yaml")
	err := os.WriteFile(filePath, []byte(`
key1:
  encoding: none
  inline: my key
`), 0644)
	require.NoError(t, err)

	sf := NewStaticStoreFactory[*signerapi.ConfigNoExt]()
	store, err := sf.NewKeyStore(context.Background(), &signerapi.ConfigNoExt{
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
			Static: pldconf.StaticKeyStoreConfig{
				File: filePath,
			},
		},
	})
	require.NoError(t, err)

	key, _, err := store.FindOrCreateLoadableKey(context.Background(), &signerapi.ResolveKeyRequest{
		Name: "key1",
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "my key", string(key))

}

func TestStaticStoreWholeStoreInFileFail(t *testing.T) {

	filePath := path.Join(t.TempDir(), "keystore.yaml")
	err := os.WriteFile(filePath, []byte(`{!!!! not good YAML`), 0644)
	require.NoError(t, err)

	sf := NewStaticStoreFactory[*signerapi.ConfigNoExt]()
	_, err = sf.NewKeyStore(context.Background(), &signerapi.ConfigNoExt{
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
			Static: pldconf.StaticKeyStoreConfig{
				File: filePath,
			},
		},
	})
	require.Regexp(t, "PD020821", err)

}
