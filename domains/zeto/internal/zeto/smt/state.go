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
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type MerkleTreeRoot struct {
	SmtName   string          `json:"smtName"`
	RootIndex tktypes.Bytes32 `json:"rootIndex"`
}

var MerkleTreeRootABI = &abi.Parameter{
	Type:         "tuple",
	InternalType: "struct MerkleTreeRoot",
	Components: abi.ParameterArray{
		{Name: "smtName", Type: "string", Indexed: true},
		{Name: "rootIndex", Type: "bytes32"},
	},
}

type MerkleTreeNode struct {
	RefKey     tktypes.Bytes32  `json:"refKey"`
	Index      tktypes.Bytes32  `json:"index"`
	Type       tktypes.HexBytes `json:"type"`
	LeftChild  tktypes.Bytes32  `json:"leftChild"`
	RightChild tktypes.Bytes32  `json:"rightChild"`
}

var MerkleTreeNodeABI = &abi.Parameter{
	Type:         "tuple",
	InternalType: "struct MerkleTreeNode",
	Components: abi.ParameterArray{
		{Name: "refKey", Type: "bytes32", Indexed: true},
		{Name: "index", Type: "bytes32"},
		{Name: "type", Type: "bytes1"},
		{Name: "leftChild", Type: "bytes32"},
		{Name: "rightChild", Type: "bytes32"},
	},
}
