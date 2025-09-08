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
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/core"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/smt"
)

const SMT_HEIGHT_UTXO = 64
const SMT_HEIGHT_KYC = 10

var Empty_Proof_Utxos proto.MerkleProof
var Empty_Proof_kyc proto.MerkleProof

func init() {
	var nodes []string
	for i := 0; i < SMT_HEIGHT_UTXO; i++ {
		nodes = append(nodes, "0")
	}
	Empty_Proof_Utxos = proto.MerkleProof{
		Nodes: nodes,
	}
	var kycNodes []string
	for i := 0; i < SMT_HEIGHT_KYC; i++ {
		kycNodes = append(kycNodes, "0")
	}
	Empty_Proof_kyc = proto.MerkleProof{
		Nodes: kycNodes,
	}
}

func NewSmt(storage StatesStorage, levels int) (core.SparseMerkleTree, error) {
	mt, err := smt.NewMerkleTree(storage, levels)
	return mt, err
}

func MerkleTreeName(tokenName string, domainInstanceContract *pldtypes.EthAddress) string {
	return "smt_" + tokenName + "_" + domainInstanceContract.String()
}

func MerkleTreeNameForLockedStates(tokenName string, domainInstanceContract *pldtypes.EthAddress) string {
	return "smtLocked_" + tokenName + "_" + domainInstanceContract.String()
}

func MerkleTreeNameForKycStates(tokenName string, domainInstanceContract *pldtypes.EthAddress) string {
	return "smtKyc_" + tokenName + "_" + domainInstanceContract.String()
}
