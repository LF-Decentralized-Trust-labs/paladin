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

package types

import (
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

var NotoABI = abi.ABI{
	{
		Name: "mint",
		Type: abi.Function,
		Inputs: abi.ParameterArray{
			{Name: "to", Type: "string"},
			{Name: "amount", Type: "uint256"},
		},
	},
	{
		Name: "transfer",
		Type: abi.Function,
		Inputs: abi.ParameterArray{
			{Name: "to", Type: "string"},
			{Name: "amount", Type: "uint256"},
		},
	},
	{
		Name: "approveTransfer",
		Type: abi.Function,
		Inputs: abi.ParameterArray{
			{
				Name:         "inputs",
				Type:         "tuple[]",
				InternalType: "struct FullState[]",
				Components: abi.ParameterArray{
					{Name: "id", Type: "bytes"},
					{Name: "schema", Type: "bytes32"},
					{Name: "data", Type: "bytes"},
				},
			},
			{
				Name:         "outputs",
				Type:         "tuple[]",
				InternalType: "struct FullState[]",
				Components: abi.ParameterArray{
					{Name: "id", Type: "bytes"},
					{Name: "schema", Type: "bytes32"},
					{Name: "data", Type: "bytes"},
				},
			},
			{Name: "data", Type: "bytes"},
			{Name: "delegate", Type: "address"},
		},
	},
}

type ConstructorParams struct {
	Notary         string              `json:"notary"`
	GuardAddress   *tktypes.EthAddress `json:"guardAddress,omitempty"`
	Implementation string              `json:"implementation"`
}

type MintParams struct {
	To     string              `json:"to"`
	Amount *tktypes.HexUint256 `json:"amount"`
}

type TransferParams struct {
	To     string              `json:"to"`
	Amount *tktypes.HexUint256 `json:"amount"`
}

type ApproveParams struct {
	Inputs   []*tktypes.FullState `json:"inputs"`
	Outputs  []*tktypes.FullState `json:"outputs"`
	Data     tktypes.HexBytes     `json:"data"`
	Delegate *tktypes.EthAddress  `json:"delegate"`
}
