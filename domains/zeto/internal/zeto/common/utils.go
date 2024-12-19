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

package common

import (
	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
)

func IsNullifiersCircuit(circuitId string) bool {
	nullifierCircuits := []string{
		constants.CIRCUIT_ANON_NULLIFIER,
		constants.CIRCUIT_ANON_NULLIFIER_BATCH,
		constants.CIRCUIT_WITHDRAW_NULLIFIER,
		constants.CIRCUIT_WITHDRAW_NULLIFIER_BATCH,
	}
	for _, c := range nullifierCircuits {
		if circuitId == c {
			return true
		}
	}
	return false
}

func IsEncryptionCircuit(circuitId string) bool {
	encryptionCircuits := []string{
		constants.CIRCUIT_ANON_ENC,
		constants.CIRCUIT_ANON_ENC_BATCH,
	}
	for _, c := range encryptionCircuits {
		if circuitId == c {
			return true
		}
	}
	return false
}

func IsBatchCircuit(sizeOfEndorsableStates int) bool {
	if sizeOfEndorsableStates <= 2 {
		return false
	}
	return true
}

func IsNullifiersToken(tokenName string) bool {
	return tokenName == constants.TOKEN_ANON_NULLIFIER
}

func IsEncryptionToken(tokenName string) bool {
	return tokenName == constants.TOKEN_ANON_ENC
}

// the Zeto implementations support two input/output sizes for the circuits: 2 and 10,
// if the input or output size is larger than 2, then the batch circuit is used with
// input/output size 10
func GetInputSize(sizeOfEndorsableStates int) int {
	if sizeOfEndorsableStates <= 2 {
		return 2
	}
	return 10
}
