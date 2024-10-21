// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pldapi

import (
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type EthTransactionResult string

const (
	TXResult_FAILURE EthTransactionResult = "failure"
	TXResult_SUCCESS EthTransactionResult = "success"
)

func (lt EthTransactionResult) Enum() tktypes.Enum[EthTransactionResult] {
	return tktypes.Enum[EthTransactionResult](lt)
}

func (pl EthTransactionResult) Options() []string {
	return []string{
		string(TXResult_FAILURE),
		string(TXResult_SUCCESS),
	}
}

type IndexedBlock struct {
	Number int64           `docstruct:"IndexedBlock" json:"number"`
	Hash   tktypes.Bytes32 `docstruct:"IndexedBlock" json:"hash"           gorm:"primaryKey"`
}

type IndexedTransaction struct {
	Hash             tktypes.Bytes32                    `docstruct:"IndexedTransaction" json:"hash"               gorm:"primaryKey"`
	BlockNumber      int64                              `docstruct:"IndexedTransaction" json:"blockNumber"`
	TransactionIndex int64                              `docstruct:"IndexedTransaction" json:"transactionIndex"`
	From             *tktypes.EthAddress                `docstruct:"IndexedTransaction" json:"from"`
	To               *tktypes.EthAddress                `docstruct:"IndexedTransaction" json:"to,omitempty"`
	Nonce            uint64                             `docstruct:"IndexedTransaction" json:"nonce"`
	ContractAddress  *tktypes.EthAddress                `docstruct:"IndexedTransaction" json:"contractAddress,omitempty"`
	Result           tktypes.Enum[EthTransactionResult] `docstruct:"IndexedTransaction" json:"result,omitempty"`
}

type IndexedEvent struct {
	BlockNumber      int64               `docstruct:"IndexedEvent" json:"blockNumber"            gorm:"primaryKey"`
	TransactionIndex int64               `docstruct:"IndexedEvent" json:"transactionIndex"       gorm:"primaryKey"`
	LogIndex         int64               `docstruct:"IndexedEvent" json:"logIndex"               gorm:"primaryKey"`
	TransactionHash  tktypes.Bytes32     `docstruct:"IndexedEvent" json:"transactionHash"`
	Signature        tktypes.Bytes32     `docstruct:"IndexedEvent" json:"signature"`
	Transaction      *IndexedTransaction `docstruct:"IndexedEvent" json:"transaction,omitempty"  gorm:"foreignKey:block_number,transaction_index;references:block_number,transaction_index"`
	Block            *IndexedBlock       `docstruct:"IndexedEvent" json:"block,omitempty"        gorm:"foreignKey:number;references:block_number"`
}
