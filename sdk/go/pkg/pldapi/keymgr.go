// Copyright © 2025 Kaleido, Inc.
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
	Name                    string `docstruct:"WalletInfo" json:"name"`
	KeySelector             string `docstruct:"WalletInfo" json:"keySelector"`
	KeySelectorMustNotMatch bool   `docstruct:"WalletInfo" json:"keySelectorMustNotMatch"`
}

type KeyMapping struct {
	Identifier string `docstruct:"KeyMapping" json:"identifier"` // the full identifier used to look up this key (including "." separators)
	Wallet     string `docstruct:"KeyMapping" json:"wallet"`     // the name of the wallet containing this key
	KeyHandle  string `docstruct:"KeyMapping" json:"keyHandle"`  // the handle within the wallet containing the key
}

type KeyMappingWithPath struct {
	*KeyMapping `json:",inline"`
	Path        []*KeyPathSegment `docstruct:"KeyMappingWithPath" json:"path"` // the full path including the leaf that is the identifier
}

type KeyMappingAndVerifier struct {
	*KeyMappingWithPath `json:",inline"`
	Verifier            *KeyVerifier `docstruct:"KeyMappingAndVerifier" json:"verifier"`
}

type KeyVerifierWithKeyRef struct {
	KeyIdentifier string `docstruct:"KeyVerifierWithKeyRef" json:"keyIdentifier"`
	*KeyVerifier  `json:",inline"`
}

type KeyVerifier struct {
	Verifier  string `docstruct:"KeyVerifier" json:"verifier"`
	Type      string `docstruct:"KeyVerifier" json:"type"`
	Algorithm string `docstruct:"KeyVerifier" json:"algorithm"`
}

type KeyPathSegment struct {
	Name  string `docstruct:"KeyPathSegment" json:"name"`
	Index int64  `docstruct:"KeyPathSegment" json:"index"`
}

type KeyQueryEntry struct {
	IsKey       bool           `docstruct:"KeyListEntry" json:"isKey"`
	HasChildren bool           `docstruct:"KeyListEntry" json:"hasChildren"`
	Parent      string         `docstruct:"KeyListEntry" json:"parent"`
	Path        string         `docstruct:"KeyListEntry" json:"path"`
	Name        string         `docstruct:"KeyListEntry" json:"name"`
	Index       int64          `docstruct:"KeyListEntry" json:"index"`
	Wallet      string         `docstruct:"KeyListEntry" json:"wallet"`
	KeyHandle   string         `docstruct:"KeyListEntry" json:"keyHandle"`
	Verifiers   []*KeyVerifier `docstruct:"KeyListEntry" json:"verifiers" gorm:"-"`
}
