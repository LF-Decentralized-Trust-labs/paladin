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

type NotoCoinState struct {
	ID              tktypes.Bytes32    `json:"id"`
	Created         tktypes.Timestamp  `json:"created"`
	ContractAddress tktypes.EthAddress `json:"contractAddress"`
	Data            NotoCoin           `json:"data"`
}

type NotoCoin struct {
	Salt   string              `json:"salt"`
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

type TransactionData struct {
	Data tktypes.HexBytes `json:"data"`
}

var TransactionDataABI = &abi.Parameter{
	Type:         "tuple",
	InternalType: "struct TransactionData",
	Components: abi.ParameterArray{
		{Name: "data", Type: "bytes"},
	},
}
