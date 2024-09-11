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
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
)

var NotoABI = abi.ABI{
	{
		Type: abi.Constructor,
		Inputs: abi.ParameterArray{
			{Name: "notary", Type: "string"},
			{Name: "implementation", Type: "string"}, // optional
		},
	},
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
		Name: "approvedTransfer",
		Type: abi.Function,
		Inputs: abi.ParameterArray{
			{Name: "to", Type: "string"},
			{Name: "amount", Type: "uint256"},
		},
	},
	{
		Name: "approve",
		Type: abi.Function,
		Inputs: abi.ParameterArray{
			{Name: "delegate", Type: "address"},
			{Name: "call", Type: "bytes"}, // assumed to be an encoded "approvedTransfer"
		},
	},
}

type ConstructorParams struct {
	Notary         string `json:"notary"`
	Implementation string `json:"implementation"`
}

type MintParams struct {
	To     string               `json:"to"`
	Amount *ethtypes.HexInteger `json:"amount"`
}

type TransferParams struct {
	To     string               `json:"to"`
	Amount *ethtypes.HexInteger `json:"amount"`
}

type ApproveParams struct {
	Delegate ethtypes.Address0xHex     `json:"delegate"`
	Call     ethtypes.HexBytes0xPrefix `json:"call"`
}
