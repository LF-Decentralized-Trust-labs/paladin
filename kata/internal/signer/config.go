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

type KeyStoreType string

const (
	KeyStoreTypeFilesystem KeyStoreType = "filesystem"
)

type Config struct {
	KeyStore      StoreConfig         `yaml:"keyStore"`
	KeyDerivation KeyDerivationConfig `yaml:"keyDerivation"`
}

type StoreConfig struct {
	Type              KeyStoreType     `yaml:"type"`
	DisableKeyLoading bool             `yaml:"disableKeyLoading"` // must be false for HD Wallet or ZKP based signing
	FileSystem        FileSystemConfig `yaml:"filesystem"`
}

type KeyDerivationType string

const (
	// Direct uses a unique piece of key material in the storage for each key, for this signing module
	KeyDerivationTypeDirect KeyDerivationType = "direct"
	// Hierarchical uses a single BIP39 seed mnemonic in the storage, combined with a BIP32 wallet to derive keys
	KeyDerivationTypeHierarchical KeyDerivationType = "hierarchical"
)

type ConfigKeyPathEntry struct {
	Name       string            `yaml:"name"`
	Index      uint32            `yaml:"index"`
	Attributes map[string]string `yaml:"attributes"`
}

type KeyDerivationConfig struct {
	Type        KeyDerivationType    `yaml:"type"`
	SeedKeyPath []ConfigKeyPathEntry `yaml:"seedKeyHandle"`
}

var KeyDerivationDefaults = &KeyDerivationConfig{
	SeedKeyPath: []ConfigKeyPathEntry{
		{Name: "seed", Index: 0},
	},
}
