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

package pldconf

import (
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
)

const (
	KeyStoreTypeFilesystem = "filesystem" // keystorev3 based filesystem storage
	KeyStoreTypeStatic     = "static"     // unencrypted keys in-line in the config
)

// Config can be directly embedded to provide ExtensibleConfig implementation
// (as in Paladin's core configuration) or you can create your own structure
// with more options as long as you provide a way to get hold of the base
// KeyStoreConfig / KeyDerivationConfig that the core module requires
// (by embedding those somewhere in your config hierarchy).
type SignerConfig struct {
	KeyStore      KeyStoreConfig      `json:"keyStore"`
	KeyDerivation KeyDerivationConfig `json:"keyDerivation"`
}

type KeyStoreConfig struct {
	Type              string                   `json:"type"`
	DisableKeyListing bool                     `json:"disableKeyListing"`
	KeyStoreSigning   bool                     `json:"keyStoreSigning"` // if HD Wallet or ZKP based signing is required, in-memory keys are required (so this needs to be false)
	FileSystem        FileSystemKeyStoreConfig `json:"filesystem"`
	Static            StaticKeyStoreConfig     `json:"static"`
}

type KeyDerivationType string

const (
	// Direct uses a unique piece of key material in the storage for each key, for this signing module
	KeyDerivationTypeDirect KeyDerivationType = "direct"
	// Hierarchical uses a single BIP39 seed mnemonic in the storage, combined with a BIP32 wallet to derive keys
	KeyDerivationTypeBIP32 KeyDerivationType = "bip32"
)

type ConfigKeyPathEntry struct {
	Name  string `json:"name"`
	Index uint64 `json:"index"`
}

type StaticKeyReference struct {
	KeyHandle  string               `json:"keyHandle,omitempty"` // causes resolution to be bypassed, similarly to if a key-mapping already exists in the DB for runtime resolution
	Name       string               `json:"name"`
	Index      uint64               `json:"index"`
	Attributes map[string]string    `json:"attributes"`
	Path       []ConfigKeyPathEntry `json:"path"`
}

type KeyDerivationConfig struct {
	Type                  KeyDerivationType  `json:"type"`
	SeedKeyPath           StaticKeyReference `json:"seedKey"`
	BIP44DirectResolution bool               `json:"bip44DirectResolution"`
	BIP44Prefix           *string            `json:"bip44Prefix"`
	BIP44HardenedSegments *int               `json:"bip44HardenedSegments"`
}

var KeyDerivationDefaults = &KeyDerivationConfig{
	BIP44Prefix:           confutil.P("m/44'/60'"),
	BIP44HardenedSegments: confutil.P(1), // in addition to the prefix, so `m/44'/60'/0'/0/0` for example with 3 segments, on top of the prefix
	SeedKeyPath:           StaticKeyReference{Name: "seed", Index: 0},
}

type StaticKeyEntryEncoding string

const (
	StaticKeyEntryEncodingNONE   StaticKeyEntryEncoding = "none"
	StaticKeyEntryEncodingHEX    StaticKeyEntryEncoding = "hex"
	StaticKeyEntryEncodingBase64 StaticKeyEntryEncoding = "base64"
)

type StaticKeyEntryConfig struct {
	Encoding StaticKeyEntryEncoding `json:"encoding"`
	Filename string                 `json:"filename"`
	Trim     bool                   `json:"trim"`
	Inline   string                 `json:"inline"`
}

type StaticKeyStoreConfig struct {
	File string                          `json:"file,omitempty"` // whole file to use as a store
	Keys map[string]StaticKeyEntryConfig `json:"keys"`           // individual key entries in the config
}

type FileSystemKeyStoreConfig struct {
	Path     *string     `json:"path"`
	Cache    CacheConfig `json:"cache"`
	FileMode *string     `json:"fileMode"`
	DirMode  *string     `json:"dirMode"`
}

var FileSystemDefaults = &FileSystemKeyStoreConfig{
	Path:     confutil.P("keystore"),
	FileMode: confutil.P("0600"),
	DirMode:  confutil.P("0700"),
	Cache: CacheConfig{
		Capacity: confutil.P(100),
	},
}
