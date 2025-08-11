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
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

type NotoDomainReceipt struct {
	States    ReceiptStates      `json:"states"`
	Transfers []*ReceiptTransfer `json:"transfers,omitempty"`
	LockInfo  *ReceiptLockInfo   `json:"lockInfo,omitempty"`
	Data      pldtypes.HexBytes  `json:"data,omitempty"`
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
	LockID       pldtypes.Bytes32     `json:"lockId"`
	Delegate     *pldtypes.EthAddress `json:"delegate,omitempty"`     // only set for delegateLock
	UnlockParams *UnlockPublicParams  `json:"unlockParams,omitempty"` // only set for prepareUnlock
	UnlockCall   pldtypes.HexBytes    `json:"unlockCall,omitempty"`   // only set for prepareUnlock
}

type ReceiptState struct {
	ID     pldtypes.HexBytes `json:"id"`
	Schema pldtypes.Bytes32  `json:"schema"`
	Data   pldtypes.RawJSON  `json:"data"`
}

type ReceiptTransfer struct {
	From   *pldtypes.EthAddress `json:"from,omitempty"`
	To     *pldtypes.EthAddress `json:"to,omitempty"`
	Amount *pldtypes.HexUint256 `json:"amount"`
}

type NotoCoinState struct {
	ID              pldtypes.Bytes32    `json:"id"`
	Created         pldtypes.Timestamp  `json:"created"`
	ContractAddress pldtypes.EthAddress `json:"contractAddress"`
	Data            NotoCoin            `json:"data"`
}

type NotoCoin struct {
	Salt   pldtypes.Bytes32     `json:"salt"`
	Owner  *pldtypes.EthAddress `json:"owner"`
	Amount *pldtypes.HexUint256 `json:"amount"`
}

var NotoCoinABI = &abi.Parameter{
	Name:         "NotoCoin",
	Type:         "tuple",
	InternalType: "struct NotoCoin",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "bytes32"},
		{Name: "owner", Type: "string", Indexed: true},
		{Name: "amount", Type: "uint256", Indexed: true},
	},
}

type NotoLockedCoinState struct {
	ID              pldtypes.Bytes32    `json:"id"`
	Created         pldtypes.Timestamp  `json:"created"`
	ContractAddress pldtypes.EthAddress `json:"contractAddress"`
	Data            NotoLockedCoin      `json:"data"`
}

type NotoLockedCoin struct {
	Salt   pldtypes.Bytes32     `json:"salt"`
	LockID pldtypes.Bytes32     `json:"lockId"`
	Owner  *pldtypes.EthAddress `json:"owner"`
	Amount *pldtypes.HexUint256 `json:"amount"`
}

var NotoLockedCoinABI = &abi.Parameter{
	Name:         "NotoLockedCoin",
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
	Salt     pldtypes.Bytes32     `json:"salt"`
	LockID   pldtypes.Bytes32     `json:"lockId"`
	Owner    *pldtypes.EthAddress `json:"owner"`
	Delegate *pldtypes.EthAddress `json:"delegate"`
}

var NotoLockInfoABI = &abi.Parameter{
	Name:         "NotoLockInfo",
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
	Salt string            `json:"salt"`
	Data pldtypes.HexBytes `json:"data"`
}

var TransactionDataABI = &abi.Parameter{
	Name:         "TransactionData",
	Type:         "tuple",
	InternalType: "struct TransactionData",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "bytes32"},
		{Name: "data", Type: "bytes"},
	},
}
