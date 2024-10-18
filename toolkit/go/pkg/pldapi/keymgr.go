// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pldapi

type WalletInfo struct {
	Name        string `json:"name"`
	KeySelector string `json:"keySelector"`
}

type KeyMapping struct {
	Identifier string `json:"identifier"` // the full identifier used to look up this key (including "." separators)
	Wallet     string `json:"wallet"`     // the name of the wallet containing this key
	KeyHandle  string `json:"keyHandle"`  // the handle within the wallet containing the key
}

type KeyMappingWithPath struct {
	*KeyMapping `json:",inline"`
	Path        []*KeyPathSegment `json:"path"` // the full path including the leaf that is the identifier
}

type KeyMappingAndVerifier struct {
	*KeyMappingWithPath `json:",inline"`
	Verifier            *KeyVerifier `json:"verifier"`
}

type KeyVerifierWithKeyRef struct {
	KeyIdentifier string `json:"keyIdentifier"`
	*KeyVerifier  `json:",inline"`
}

type KeyVerifier struct {
	Verifier  string `json:"verifier"`
	Type      string `json:"type"`
	Algorithm string `json:"algorithm"`
}

type KeyPathSegment struct {
	Name  string `json:"name"`
	Index int64  `json:"index"`
}
