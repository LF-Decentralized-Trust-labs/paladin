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

package smt

import (
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
)

func TestPackageInit(t *testing.T) {
	assert.Equal(t, SMT_HEIGHT_UTXO, len(Empty_Proof_Utxos.Nodes))
	assert.Equal(t, "0", Empty_Proof_Utxos.Nodes[0])
	assert.Equal(t, "0", Empty_Proof_Utxos.Nodes[SMT_HEIGHT_UTXO-1])
	assert.Equal(t, SMT_HEIGHT_KYC, len(Empty_Proof_kyc.Nodes))
	assert.Equal(t, "0", Empty_Proof_kyc.Nodes[0])
	assert.Equal(t, "0", Empty_Proof_kyc.Nodes[SMT_HEIGHT_KYC-1])
}

func TestMerkleTreeName(t *testing.T) {
	address, _ := pldtypes.ParseEthAddress("0xe12c416382988005ace9b2e2f9a8a904d8be961c")
	assert.Equal(t, "smt_test1_0xe12c416382988005ace9b2e2f9a8a904d8be961c", MerkleTreeName("test1", address))
}
