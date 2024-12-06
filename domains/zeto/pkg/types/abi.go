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

var ZetoABI = abi.ABI{
	{
		Type: abi.Constructor,
		Inputs: abi.ParameterArray{
			{
				Name: "tokenName",
				Type: "string",
			},
		},
	},
	{
		Name: "mint",
		Type: abi.Function,
		Inputs: abi.ParameterArray{
			{
				Name: "mints",
				Type: "tuple[]",
				Components: abi.ParameterArray{
					{Name: "to", Type: "string"},
					{Name: "amount", Type: "uint256"},
				},
			},
		},
	},
	{
		Name: "transfer",
		Type: abi.Function,
		Inputs: abi.ParameterArray{
			{
				Name: "transfers",
				Type: "tuple[]",
				Components: abi.ParameterArray{
					{Name: "to", Type: "string"},
					{Name: "amount", Type: "uint256"},
				},
			},
		},
	},
	{
		Name: "lockProof",
		Type: abi.Function,
		Inputs: abi.ParameterArray{
			{Name: "delegate", Type: "address"},
			{Name: "call", Type: "bytes"}, // assumed to be an encoded "transfer"
		},
	},
	{
		Name: "setERC20",
		Type: abi.Function,
		Inputs: abi.ParameterArray{
			{Name: "_erc20", Type: "address"},
		},
	},
	{
		Name: "deposit",
		Type: abi.Function,
		Inputs: abi.ParameterArray{
			{Name: "amount", Type: "uint256"},
		},
	},
	{
		Name: "withdraw",
		Type: abi.Function,
		Inputs: abi.ParameterArray{
			{Name: "amount", Type: "uint256"},
		},
	},
}

type InitializerParams struct {
	TokenName string `json:"tokenName"`
	// InitialOwner string `json:"initialOwner"` // TODO: allow the initial owner to be specified by the deploy request
}

type DeployParams struct {
	TransactionID string           `json:"transactionId"`
	Data          tktypes.HexBytes `json:"data"`
	TokenName     string           `json:"tokenName"`
	InitialOwner  string           `json:"initialOwner"`
}

type MintParams struct {
	Mints []*TransferParamEntry `json:"mints"`
}

type TransferParams struct {
	Transfers []*TransferParamEntry `json:"transfers"`
}

type TransferParamEntry struct {
	To     string              `json:"to"`
	Amount *tktypes.HexUint256 `json:"amount"`
}

type LockParams struct {
	Delegate *tktypes.EthAddress `json:"delegate"`
	Call     tktypes.HexBytes    `json:"call"`
}

type DepositParams struct {
	Amount *tktypes.HexUint256 `json:"amount"`
}

type WithdrawParams struct {
	Amount *tktypes.HexUint256 `json:"amount"`
}
