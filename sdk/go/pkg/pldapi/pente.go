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

package pldapi

import "github.com/kaleido-io/paladin/common/go/pkg/types"

type PenteDomainReceipt struct {
	Transaction *PrivateEVMTransaction `json:"transaction"`
	Receipt     *PrivateEVMReceipt     `json:"receipt"`
}

type PrivateEVMTransaction struct {
	From  types.EthAddress  `json:"from"`
	To    *types.EthAddress `json:"to"`
	Nonce types.HexUint64   `json:"nonce"`
	Gas   types.HexUint64   `json:"gas,omitempty"`
	Value *types.HexUint256 `json:"value,omitempty"`
	Data  types.HexBytes    `json:"data"`
}

type PrivateEVMReceipt struct {
	From            types.EthAddress  `json:"from"`
	To              *types.EthAddress `json:"to"`
	GasUsed         types.HexUint64   `json:"gasUsed"`
	ContractAddress *types.EthAddress `json:"contractAddress"`
	Logs            []*PrivateEVMLog  `json:"logs"`
}

type PrivateEVMLog struct {
	Address types.EthAddress `json:"address"`
	Topics  []types.Bytes32  `json:"topics"`
	Data    types.HexBytes   `json:"data"`
}
