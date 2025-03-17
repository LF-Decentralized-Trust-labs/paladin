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
	"github.com/hyperledger/firefly-signer/pkg/abi"
	commontypes "github.com/kaleido-io/paladin/common/go/pkg/types"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
)

type MintHookParams struct {
	Sender   *commontypes.EthAddress `json:"sender"`
	To       *commontypes.EthAddress `json:"to"`
	Amount   *commontypes.HexUint256 `json:"amount"`
	Data     commontypes.HexBytes    `json:"data"`
	Prepared PreparedTransaction     `json:"prepared"`
}

type TransferHookParams struct {
	Sender   *commontypes.EthAddress `json:"sender"`
	From     *commontypes.EthAddress `json:"from"`
	To       *commontypes.EthAddress `json:"to"`
	Amount   *commontypes.HexUint256 `json:"amount"`
	Data     commontypes.HexBytes    `json:"data"`
	Prepared PreparedTransaction     `json:"prepared"`
}

type BurnHookParams struct {
	Sender   *commontypes.EthAddress `json:"sender"`
	From     *commontypes.EthAddress `json:"from"`
	Amount   *commontypes.HexUint256 `json:"amount"`
	Data     commontypes.HexBytes    `json:"data"`
	Prepared PreparedTransaction     `json:"prepared"`
}

type ApproveTransferHookParams struct {
	Sender   *commontypes.EthAddress `json:"sender"`
	From     *commontypes.EthAddress `json:"from"`
	Delegate *commontypes.EthAddress `json:"delegate"`
	Data     commontypes.HexBytes    `json:"data"`
	Prepared PreparedTransaction     `json:"prepared"`
}

type LockHookParams struct {
	Sender   *commontypes.EthAddress `json:"sender"`
	LockID   commontypes.Bytes32     `json:"lockId"`
	From     *commontypes.EthAddress `json:"from"`
	Amount   *commontypes.HexUint256 `json:"amount"`
	Data     commontypes.HexBytes    `json:"data"`
	Prepared PreparedTransaction     `json:"prepared"`
}

type UnlockHookParams struct {
	Sender     *commontypes.EthAddress    `json:"sender"`
	LockID     commontypes.Bytes32        `json:"lockId"`
	Recipients []*ResolvedUnlockRecipient `json:"recipients"`
	Data       commontypes.HexBytes       `json:"data"`
	Prepared   PreparedTransaction        `json:"prepared"`
}

type ApproveUnlockHookParams struct {
	Sender   *commontypes.EthAddress `json:"sender"`
	LockID   commontypes.Bytes32     `json:"lockId"`
	Delegate *commontypes.EthAddress `json:"delegate"`
	Data     commontypes.HexBytes    `json:"data"`
	Prepared PreparedTransaction     `json:"prepared"`
}

type DelegateUnlockHookParams struct {
	Sender     *commontypes.EthAddress    `json:"sender"`
	LockID     commontypes.Bytes32        `json:"lockId"`
	Recipients []*ResolvedUnlockRecipient `json:"recipients"`
	Data       commontypes.HexBytes       `json:"data"`
}

type PreparedTransaction struct {
	ContractAddress *commontypes.EthAddress `json:"contractAddress"`
	EncodedCall     commontypes.HexBytes    `json:"encodedCall"`
}

type ResolvedUnlockRecipient struct {
	To     *commontypes.EthAddress `json:"to"`
	Amount *commontypes.HexUint256 `json:"amount"`
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
	To     *commontypes.EthAddress  `json:"to"`
	Inputs any                      `json:"inputs"`
}
