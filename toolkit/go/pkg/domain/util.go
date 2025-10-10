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

package domain

import (
	pb "github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
)

func FindVerifier(lookup, algorithm, verifierType string, verifiers []*pb.ResolvedVerifier) *pb.ResolvedVerifier {
	for _, verifier := range verifiers {
		if verifier.Lookup == lookup && verifier.Algorithm == algorithm && verifier.VerifierType == verifierType {
			return verifier
		}
	}
	return nil
}

func FindAttestation(name string, attestations []*pb.AttestationResult) *pb.AttestationResult {
	for _, attestation := range attestations {
		if attestation.Name == name {
			return attestation
		}
	}
	return nil
}
