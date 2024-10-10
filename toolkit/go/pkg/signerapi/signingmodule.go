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
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/signpayloads"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

type ResolveKeyRequest struct {
	// a name assured to be unique at this path
	Name string `json:"name"`

	// a unique index managed by the key manager assured to be unique at this path. Used for key derivation (BIP32). Should not be used for direct mapping.
	Index               uint64                     `json:"index"`
	Attributes          map[string]string          `json:"attributes"`
	Path                []*ResolveKeyPathSegment   `json:"path"`
	RequiredIdentifiers []*PublicKeyIdentifierType `json:"requiredIdentifiers"`
}

type ResolveKeyResponse struct {
	KeyHandle   string                 `json:"keyHandle"`
	Identifiers []*PublicKeyIdentifier `json:"identifiers"`
}

type SignRequest struct {
	// the key handle as returned by a previoius Resolve call (potentially a very long time ago)
	KeyHandle string `json:"keyHandle"`

	// identifier for the signing engine and algorithm to use in signing. Examples: "ecdsa:secp256k1" or "domain:zeto:circuit1"
	Algorithm algorithms.Algorithm `json:"algorithm"`

	// describes the input and output payload combination to the signer. Example: "opaque:rsv" or "groth16:zeto"
	PayloadType signpayloads.SignPayloadType `json:"payloadType"`

	// the input payload to process according to the algorithm
	Payload tktypes.HexBytes `json:"payload"`
}

type SignResponse struct {
	// an set of bytes appropriate to the Paladin signing algorithm spec used
	Payload tktypes.HexBytes `json:"payload"`
}

type ListKeysRequest struct {
	// the maximum number of records to return
	Limit int `json:"limit"`

	// the "next" string from a previous call, or empty
	Continue string `json:"continue"`
}

type ListKeysResponse struct {
	// any length less than the limit will cause the caller to assume there might be more records
	Items []*ListKeyEntry `json:"items"`

	// non empty string to support pagination when the are potentially more records
	Next string `json:"next"`
}

type ResolveKeyPathSegment struct {
	// the name of the path segment (folder)
	Name string `json:"name"`

	// a unique index managed by the key manager assured to be unique at this level in the path. Used for key derivation (BIP32). Should not be used for direct mapping.
	Index uint64 `json:"index"`
}

type ListKeyEntry struct {
	Name        string                 `json:"name"`
	KeyHandle   string                 `json:"keyHandle"`
	Attributes  map[string]string      `json:"attributes"`
	Path        []*ListKeyPathSegment  `json:"path"`
	Identifiers []*PublicKeyIdentifier `json:"identifiers"`
}

// The only discoverable item for a path segment is the name, but it is an object for future extensibility
type ListKeyPathSegment struct {
	Name string `json:"name"`
}

type PublicKeyIdentifierType struct {
	Algorithm    algorithms.Algorithm   `json:"algorithm"`
	VerifierType verifiers.VerifierType `json:"verifierType"`
}

type PublicKeyIdentifier struct {
	Algorithm    algorithms.Algorithm   `json:"algorithm"`
	VerifierType verifiers.VerifierType `json:"verifierType"`
	Verifier     string                 `json:"verifier"`
}
