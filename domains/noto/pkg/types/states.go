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

type NotoDomainReceipt struct {
	States    ReceiptStates      `json:"states"`
	Transfers []*ReceiptTransfer `json:"transfers,omitempty"`
	LockInfo  *ReceiptLockInfo   `json:"lockInfo,omitempty"`
	Data      tktypes.HexBytes   `json:"data,omitempty"`
}

type ReceiptStates struct {
	Inputs                []*ReceiptState `json:"inputs,omitempty"`
	LockedInputs          []*ReceiptState `json:"lockedInputs,omitempty"`
	Outputs               []*ReceiptState `json:"outputs,omitempty"`
	LockedOutputs         []*ReceiptState `json:"lockedOutputs,omitempty"`
	ReadInputs            []*ReceiptState `json:"readInputs,omitempty"`
	ReadLockedInputs      []*ReceiptState `json:"readLockedInputs,omitempty"`
	PreparedOutputs       []*ReceiptState `json:"preparedOutputs,omitempty"`
	PreparedLockedOutputs []*ReceiptState `json:"preparedLockedOutputs,omitempty"`
}

type ReceiptLockInfo struct {
	LockID   tktypes.Bytes32     `json:"lockId"`
	Delegate *tktypes.EthAddress `json:"delegate,omitempty"` // only set for delegateLock
	Unlock   tktypes.HexBytes    `json:"unlock,omitempty"`   // only set for prepareUnlock
}

type ReceiptState struct {
	ID   tktypes.HexBytes `json:"id"`
	Data tktypes.RawJSON  `json:"data"`
}

type ReceiptTransfer struct {
	From   *tktypes.EthAddress `json:"from,omitempty"`
	To     *tktypes.EthAddress `json:"to,omitempty"`
	Amount *tktypes.HexUint256 `json:"amount"`
}

type NotoCoinState struct {
	ID              tktypes.Bytes32    `json:"id"`
	Created         tktypes.Timestamp  `json:"created"`
	ContractAddress tktypes.EthAddress `json:"contractAddress"`
	Data            NotoCoin           `json:"data"`
}

type NotoCoin struct {
	Salt   tktypes.Bytes32     `json:"salt"`
	Owner  *tktypes.EthAddress `json:"owner"`
	Amount *tktypes.HexUint256 `json:"amount"`
}

var NotoCoinABI = &abi.Parameter{
	Type:         "tuple",
	InternalType: "struct NotoCoin",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "bytes32"},
		{Name: "owner", Type: "string", Indexed: true},
		{Name: "amount", Type: "uint256", Indexed: true},
	},
}

type NotoLockedCoin struct {
	Salt   tktypes.Bytes32     `json:"salt"`
	LockID tktypes.Bytes32     `json:"lockId"`
	Owner  *tktypes.EthAddress `json:"owner"`
	Amount *tktypes.HexUint256 `json:"amount"`
}

var NotoLockedCoinABI = &abi.Parameter{
	Type:         "tuple",
	InternalType: "struct NotoLockedCoin",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "bytes32"},
		{Name: "lockId", Type: "bytes32", Indexed: true},
		{Name: "owner", Type: "string", Indexed: true},
		{Name: "amount", Type: "uint256"},
	},
}

type NotoLockInfo struct {
	Salt     tktypes.Bytes32     `json:"salt"`
	LockID   tktypes.Bytes32     `json:"lockId"`
	Owner    *tktypes.EthAddress `json:"owner"`
	Delegate *tktypes.EthAddress `json:"delegate"`
}

var NotoLockInfoABI = &abi.Parameter{
	Type:         "tuple",
	InternalType: "struct NotoLockInfo",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "bytes32"},
		{Name: "lockId", Type: "bytes32"},
		{Name: "owner", Type: "address"},
		{Name: "delegate", Type: "address"},
	},
}

type TransactionData struct {
	Salt string           `json:"salt"`
	Data tktypes.HexBytes `json:"data"`
}

var TransactionDataABI = &abi.Parameter{
	Type:         "tuple",
	InternalType: "struct TransactionData",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "bytes32"},
		{Name: "data", Type: "bytes"},
	},
}
