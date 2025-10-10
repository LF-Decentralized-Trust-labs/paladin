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

package pldconf

import "github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"

type KeyManagerConfig struct {
	KeyManagerManagerConfig `json:"keyManager"`
	SigningModules          map[string]*SigningModuleConfig `json:"signingModules"`
	Wallets                 []*WalletConfig                 `json:"wallets"` // ordered list
}

type KeyManagerManagerConfig struct {
	IdentifierCache CacheConfig `json:"identifierCache"`
	VerifierCache   CacheConfig `json:"verifierCache"`
}

type SigningModuleConfig struct {
	Init   SigningModuleInitConfig `json:"init"`
	Plugin PluginConfig            `json:"plugin"`
	Config map[string]any          `json:"config"`
}

type SigningModuleInitConfig struct {
	Retry RetryConfig `json:"retry"`
}

type WalletConfig struct {
	Name                    string        `json:"name"`
	KeySelector             string        `json:"keySelector"`             // Regex pattern conforming to https://golang.org/s/re2syntax
	KeySelectorMustNotMatch bool          `json:"keySelectorMustNotMatch"` // To allow for specifying a non-matching regex i.e. all keys that aren't this pattern
	Signer                  *SignerConfig `json:"signer"`                  // embedded only
	SignerPluginName        string        `json:"signerPluginName"`
	SignerType              string        `json:"signerType"`
}

const (
	WalletSignerTypeEmbedded string = "embedded"
	WalletSignerTypePlugin   string = "plugin"
)

var WalletDefaults = &WalletConfig{
	KeySelector:             `.*`, // catch-all
	KeySelectorMustNotMatch: false,
	SignerType:              WalletSignerTypeEmbedded, // uses the embedded signing module running in the Paladin process
}

var KeyManagerDefaults = &KeyManagerConfig{
	KeyManagerManagerConfig: KeyManagerManagerConfig{
		IdentifierCache: CacheConfig{
			Capacity: confutil.P(1000),
		},
		VerifierCache: CacheConfig{
			Capacity: confutil.P(1000),
		},
	},
}
