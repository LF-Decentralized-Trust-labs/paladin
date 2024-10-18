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

import "github.com/kaleido-io/paladin/config/pkg/confutil"

type KeyManagerConfig struct {
	KeyManagerManagerConfig `json:"keyManager"`
	Wallets                 []*WalletConfig `json:"wallets"` // ordered list
}

type KeyManagerManagerConfig struct {
	IdentifierCache CacheConfig `json:"identifierCache"`
	VerifierCache   CacheConfig `json:"verifierCache"`
}

type WalletConfig struct {
	Name        string        `json:"name"`
	KeySelector string        `json:"keySelector"`
	SignerType  string        `json:"signerType"`
	Signer      *SignerConfig `json:"signer"` // embedded only
}

const (
	WalletSignerTypeEmbedded string = "embedded"
)

var WalletDefaults = &WalletConfig{
	KeySelector: `.*`,                     // catch-all
	SignerType:  WalletSignerTypeEmbedded, // uses the embedded signing module running in the Paladin process
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
