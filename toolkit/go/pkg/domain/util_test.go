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

package domain

import (
	"testing"

	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

func TestFindVerifier(t *testing.T) {
	tests := []struct {
		name         string
		lookup       string
		algorithm    string
		verifierType string
		verifiers    []*pb.ResolvedVerifier
		expected     *pb.ResolvedVerifier
	}{
		{
			name:         "find existing verifier",
			lookup:       "alice",
			algorithm:    "ecdsa-secp256k1",
			verifierType: "eth_address",
			verifiers: []*pb.ResolvedVerifier{
				{
					Lookup:       "alice",
					Algorithm:    "ecdsa-secp256k1",
					VerifierType: "eth_address",
					Verifier:     "0x1234567890abcdef",
				},
				{
					Lookup:       "bob",
					Algorithm:    "ecdsa-secp256k1",
					VerifierType: "eth_address",
					Verifier:     "0xfedcba0987654321",
				},
			},
			expected: &pb.ResolvedVerifier{
				Lookup:       "alice",
				Algorithm:    "ecdsa-secp256k1",
				VerifierType: "eth_address",
				Verifier:     "0x1234567890abcdef",
			},
		},
		{
			name:         "verifier not found - different lookup",
			lookup:       "charlie",
			algorithm:    "ecdsa-secp256k1",
			verifierType: "eth_address",
			verifiers: []*pb.ResolvedVerifier{
				{
					Lookup:       "alice",
					Algorithm:    "ecdsa-secp256k1",
					VerifierType: "eth_address",
					Verifier:     "0x1234567890abcdef",
				},
				{
					Lookup:       "bob",
					Algorithm:    "ecdsa-secp256k1",
					VerifierType: "eth_address",
					Verifier:     "0xfedcba0987654321",
				},
			},
			expected: nil,
		},
		{
			name:         "verifier not found - different algorithm",
			lookup:       "alice",
			algorithm:    "ed25519",
			verifierType: "eth_address",
			verifiers: []*pb.ResolvedVerifier{
				{
					Lookup:       "alice",
					Algorithm:    "ecdsa-secp256k1",
					VerifierType: "eth_address",
					Verifier:     "0x1234567890abcdef",
				},
			},
			expected: nil,
		},
		{
			name:         "verifier not found - different verifier type",
			lookup:       "alice",
			algorithm:    "ecdsa-secp256k1",
			verifierType: "public_key",
			verifiers: []*pb.ResolvedVerifier{
				{
					Lookup:       "alice",
					Algorithm:    "ecdsa-secp256k1",
					VerifierType: "eth_address",
					Verifier:     "0x1234567890abcdef",
				},
			},
			expected: nil,
		},
		{
			name:         "empty verifiers slice",
			lookup:       "alice",
			algorithm:    "ecdsa-secp256k1",
			verifierType: "eth_address",
			verifiers:    []*pb.ResolvedVerifier{},
			expected:     nil,
		},
		{
			name:         "nil verifiers slice",
			lookup:       "alice",
			algorithm:    "ecdsa-secp256k1",
			verifierType: "eth_address",
			verifiers:    nil,
			expected:     nil,
		},
		{
			name:         "case sensitive matching",
			lookup:       "Alice",
			algorithm:    "ECDSA-SECP256K1",
			verifierType: "ETH_ADDRESS",
			verifiers: []*pb.ResolvedVerifier{
				{
					Lookup:       "alice",
					Algorithm:    "ecdsa-secp256k1",
					VerifierType: "eth_address",
					Verifier:     "0x1234567890abcdef",
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FindVerifier(tt.lookup, tt.algorithm, tt.verifierType, tt.verifiers)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindAttestation(t *testing.T) {
	tests := []struct {
		name            string
		attestationName string
		attestations    []*pb.AttestationResult
		expected        *pb.AttestationResult
	}{
		{
			name:            "find existing attestation",
			attestationName: "sender_signature",
			attestations: []*pb.AttestationResult{
				{
					Name:            "sender_signature",
					AttestationType: pb.AttestationType_SIGN,
					Verifier: &pb.ResolvedVerifier{
						Lookup:       "alice",
						Algorithm:    "ecdsa-secp256k1",
						VerifierType: "eth_address",
						Verifier:     "0x1234567890abcdef",
					},
					Payload: []byte("signature_data"),
				},
				{
					Name:            "notary_endorsement",
					AttestationType: pb.AttestationType_ENDORSE,
					Verifier: &pb.ResolvedVerifier{
						Lookup:       "notary",
						Algorithm:    "ecdsa-secp256k1",
						VerifierType: "eth_address",
						Verifier:     "0xfedcba0987654321",
					},
				},
			},
			expected: &pb.AttestationResult{
				Name:            "sender_signature",
				AttestationType: pb.AttestationType_SIGN,
				Verifier: &pb.ResolvedVerifier{
					Lookup:       "alice",
					Algorithm:    "ecdsa-secp256k1",
					VerifierType: "eth_address",
					Verifier:     "0x1234567890abcdef",
				},
				Payload: []byte("signature_data"),
			},
		},
		{
			name:            "attestation not found",
			attestationName: "missing_attestation",
			attestations: []*pb.AttestationResult{
				{
					Name:            "sender_signature",
					AttestationType: pb.AttestationType_SIGN,
					Verifier: &pb.ResolvedVerifier{
						Lookup:       "alice",
						Algorithm:    "ecdsa-secp256k1",
						VerifierType: "eth_address",
						Verifier:     "0x1234567890abcdef",
					},
				},
			},
			expected: nil,
		},
		{
			name:            "empty attestations slice",
			attestationName: "sender_signature",
			attestations:    []*pb.AttestationResult{},
			expected:        nil,
		},
		{
			name:            "nil attestations slice",
			attestationName: "sender_signature",
			attestations:    nil,
			expected:        nil,
		},
		{
			name:            "case sensitive matching",
			attestationName: "SENDER_SIGNATURE",
			attestations: []*pb.AttestationResult{
				{
					Name:            "sender_signature",
					AttestationType: pb.AttestationType_SIGN,
					Verifier: &pb.ResolvedVerifier{
						Lookup:       "alice",
						Algorithm:    "ecdsa-secp256k1",
						VerifierType: "eth_address",
						Verifier:     "0x1234567890abcdef",
					},
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FindAttestation(tt.attestationName, tt.attestations)
			assert.Equal(t, tt.expected, result)
		})
	}
}
