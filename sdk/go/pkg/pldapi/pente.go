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

import "github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"

type PenteDomainReceipt struct {
	Transaction *PrivateEVMTransaction `json:"transaction"`
	Receipt     *PrivateEVMReceipt     `json:"receipt"`
}

type PrivateEVMTransaction struct {
	From  pldtypes.EthAddress  `json:"from"`
	To    *pldtypes.EthAddress `json:"to"`
	Nonce pldtypes.HexUint64   `json:"nonce"`
	Gas   pldtypes.HexUint64   `json:"gas,omitempty"`
	Value *pldtypes.HexUint256 `json:"value,omitempty"`
	Data  pldtypes.HexBytes    `json:"data"`
}

type PrivateEVMReceipt struct {
	From            pldtypes.EthAddress  `json:"from"`
	To              *pldtypes.EthAddress `json:"to"`
	GasUsed         pldtypes.HexUint64   `json:"gasUsed"`
	ContractAddress *pldtypes.EthAddress `json:"contractAddress"`
	Logs            []*PrivateEVMLog     `json:"logs"`
}

type PrivateEVMLog struct {
	Address pldtypes.EthAddress `json:"address"`
	Topics  []pldtypes.Bytes32  `json:"topics"`
	Data    pldtypes.HexBytes   `json:"data"`
}
