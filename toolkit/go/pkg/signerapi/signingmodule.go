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

package signerapi

import (
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type ResolveKeyRequest struct {
	// a name assured to be unique at this path
	Name string `json:"name,omitempty"`

	// a unique index managed by the key manager assured to be unique at this path. Used for key derivation (BIP32). Should not be used for direct mapping.
	Index uint64 `json:"index,omitempty"`

	// Attributes passed to the signing module during key resolution
	Attributes map[string]string `json:"attributes,omitempty"`

	// Hierarchical path to the key split into segments (optional)
	Path []*ResolveKeyPathSegment `json:"path,omitempty"`

	// Required identifiers for the resolved key (optional)
	RequiredIdentifiers []*PublicKeyIdentifierType `json:"requiredIdentifiers,omitempty"`
}

type ResolveKeyResponse struct {
	// Maps this internal key representation down to the key material
	KeyHandle string `json:"keyHandle,omitempty"`

	// Resolved public key information
	Identifiers []*PublicKeyIdentifier `json:"identifiers,omitempty"`
}

type SignRequest struct {
	// the key handle as returned by a previoius Resolve call (potentially a very long time ago)
	KeyHandle string `json:"keyHandle,omitempty"`

	// identifier for the signing engine and algorithm to use in signing. Examples: "ecdsa:secp256k1" or "domain:zeto:circuit1"
	Algorithm string `json:"algorithm,omitempty"`

	// describes the input and output payload combination to the signer. Example: "opaque:rsv" or "groth16:zeto"
	PayloadType string `json:"payloadType,omitempty"`

	// the input payload to process according to the algorithm
	Payload tktypes.HexBytes `json:"payload,omitempty"`
}

type SignResponse struct {
	// an set of bytes appropriate to the Paladin signing algorithm spec used
	Payload tktypes.HexBytes `json:"payload,omitempty"`
}

type ListKeysRequest struct {
	// the maximum number of records to return
	Limit int `json:"limit,omitempty"`

	// the "next" string from a previous call, or empty
	Continue string `json:"continue,omitempty"`
}

type ListKeysResponse struct {
	// any length less than the limit will cause the caller to assume there might be more records
	Items []*ListKeyEntry `json:"items,omitempty"`

	// non empty string to support pagination when the are potentially more records
	Next string `json:"next,omitempty"`
}

type ResolveKeyPathSegment struct {
	// the name of the path segment (folder)
	Name string `json:"name,omitempty"`

	// a unique index managed by the key manager assured to be unique at this level in the path. Used for key derivation (BIP32). Should not be used for direct mapping.
	Index uint64 `json:"index,omitempty"`
}

type ListKeyEntry struct {
	// The part of the key identifier representing this key
	Name string `json:"name,omitempty"`

	// Maps this internal key representation down to the key material
	KeyHandle string `json:"keyHandle,omitempty"`

	// Attributes passed to the signing module during key resolution
	Attributes map[string]string `json:"attributes,omitempty"`

	// Hierarchical path to the key split into segments
	Path []*ListKeyPathSegment `json:"path,omitempty"`

	// Public key information
	Identifiers []*PublicKeyIdentifier `json:"identifiers,omitempty"`
}

// The only discoverable item for a path segment is the name, but it is an object for future extensibility
type ListKeyPathSegment struct {
	Name string `json:"name,omitempty"`
}

type PublicKeyIdentifierType struct {
	// The curve on which the key material has been generated (for predefined constants refer to the toolkit/go/pkg/algorithms package)
	Algorithm string `json:"algorithm,omitempty"`

	// The representation to which the public key material is encoded (for predefined constants refer to the toolkit/go/pkg/verifiers package)
	VerifierType string `json:"verifierType,omitempty"`
}

type PublicKeyIdentifier struct {
	// The curve on which the key material has been generated (for predefined constants refer to the toolkit/go/pkg/algorithms package)
	Algorithm string `json:"algorithm,omitempty"`

	// The representation to which the public key material is encoded (for predefined constants refer to the toolkit/go/pkg/verifiers package)
	VerifierType string `json:"verifierType,omitempty"`

	// The public key encoded in the form of the Verifier type (for example, a 0x address)
	Verifier string `json:"verifier,omitempty"`
}
