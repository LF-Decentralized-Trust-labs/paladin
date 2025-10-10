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

package noto

import (
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/noto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

type MintHookParams struct {
	Sender   *pldtypes.EthAddress `json:"sender"`
	To       *pldtypes.EthAddress `json:"to"`
	Amount   *pldtypes.HexUint256 `json:"amount"`
	Data     pldtypes.HexBytes    `json:"data"`
	Prepared PreparedTransaction  `json:"prepared"`
}

type TransferHookParams struct {
	Sender   *pldtypes.EthAddress `json:"sender"`
	From     *pldtypes.EthAddress `json:"from"`
	To       *pldtypes.EthAddress `json:"to"`
	Amount   *pldtypes.HexUint256 `json:"amount"`
	Data     pldtypes.HexBytes    `json:"data"`
	Prepared PreparedTransaction  `json:"prepared"`
}

type BurnHookParams struct {
	Sender   *pldtypes.EthAddress `json:"sender"`
	From     *pldtypes.EthAddress `json:"from"`
	Amount   *pldtypes.HexUint256 `json:"amount"`
	Data     pldtypes.HexBytes    `json:"data"`
	Prepared PreparedTransaction  `json:"prepared"`
}

type ApproveTransferHookParams struct {
	Sender   *pldtypes.EthAddress `json:"sender"`
	From     *pldtypes.EthAddress `json:"from"`
	Delegate *pldtypes.EthAddress `json:"delegate"`
	Data     pldtypes.HexBytes    `json:"data"`
	Prepared PreparedTransaction  `json:"prepared"`
}

type LockHookParams struct {
	Sender   *pldtypes.EthAddress `json:"sender"`
	LockID   pldtypes.Bytes32     `json:"lockId"`
	From     *pldtypes.EthAddress `json:"from"`
	Amount   *pldtypes.HexUint256 `json:"amount"`
	Data     pldtypes.HexBytes    `json:"data"`
	Prepared PreparedTransaction  `json:"prepared"`
}

type UnlockHookParams struct {
	Sender     *pldtypes.EthAddress       `json:"sender"`
	LockID     pldtypes.Bytes32           `json:"lockId"`
	Recipients []*ResolvedUnlockRecipient `json:"recipients"`
	Data       pldtypes.HexBytes          `json:"data"`
	Prepared   PreparedTransaction        `json:"prepared"`
}

type ApproveUnlockHookParams struct {
	Sender   *pldtypes.EthAddress `json:"sender"`
	LockID   pldtypes.Bytes32     `json:"lockId"`
	Delegate *pldtypes.EthAddress `json:"delegate"`
	Data     pldtypes.HexBytes    `json:"data"`
	Prepared PreparedTransaction  `json:"prepared"`
}

type DelegateUnlockHookParams struct {
	Sender     *pldtypes.EthAddress       `json:"sender"`
	LockID     pldtypes.Bytes32           `json:"lockId"`
	Recipients []*ResolvedUnlockRecipient `json:"recipients"`
	Data       pldtypes.HexBytes          `json:"data"`
}

type PreparedTransaction struct {
	ContractAddress *pldtypes.EthAddress `json:"contractAddress"`
	EncodedCall     pldtypes.HexBytes    `json:"encodedCall"`
}

type ResolvedUnlockRecipient struct {
	To     *pldtypes.EthAddress `json:"to"`
	Amount *pldtypes.HexUint256 `json:"amount"`
}

func penteInvokeABI(name string, inputs abi.ParameterArray) *abi.Entry {
	return &abi.Entry{
		Name: name,
		Type: "function",
		Inputs: abi.ParameterArray{
			{
				Name:         "group",
				Type:         "tuple",
				InternalType: "struct Group",
				Components: abi.ParameterArray{
					{Name: "salt", Type: "bytes32"},
					{Name: "members", Type: "string[]"},
				},
			},
			{Name: "to", Type: "address"},
			{
				Name:         "inputs",
				Type:         "tuple",
				InternalType: "struct PrivateInvokeInputs",
				Components:   inputs,
			},
		},
		Outputs: abi.ParameterArray{},
	}
}

type PenteInvokeParams struct {
	Group  *types.PentePrivateGroup `json:"group"`
	To     *pldtypes.EthAddress     `json:"to"`
	Inputs any                      `json:"inputs"`
}
