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

package blockindexer

import (
	"github.com/kaleido-io/paladin/core/pkg/types"
)

type IndexedBlock struct {
	Number int64         `json:"number"`
	Hash   types.Bytes32 `json:"hash"                                     gorm:"primaryKey"`
}

type EthTransactionResult string

const (
	TXResult_FAILURE EthTransactionResult = "failure"
	TXResult_SUCCESS EthTransactionResult = "success"
)

func (lt EthTransactionResult) Enum() types.Enum[EthTransactionResult] {
	return types.Enum[EthTransactionResult](lt)
}

func (pl EthTransactionResult) Options() []string {
	return []string{
		string(TXResult_FAILURE),
		string(TXResult_SUCCESS),
	}
}

type IndexedTransaction struct {
	Hash             types.Bytes32                    `json:"hash"                      gorm:"primaryKey"`
	BlockNumber      int64                            `json:"blockNumber"`
	TransactionIndex int64                            `json:"transactionIndex"`
	From             *types.EthAddress                `json:"from"`
	To               *types.EthAddress                `json:"to,omitempty"`
	ContractAddress  *types.EthAddress                `json:"contractAddress,omitempty"`
	Result           types.Enum[EthTransactionResult] `json:"result,omitempty"`
}

type IndexedEvent struct {
	BlockNumber      int64               `json:"blockNumber"             gorm:"primaryKey"`
	TransactionIndex int64               `json:"transactionIndex"        gorm:"primaryKey"`
	LogIndex         int64               `json:"logIndex"                gorm:"primaryKey"`
	TransactionHash  types.Bytes32       `json:"transactionHash"`
	Signature        types.Bytes32       `json:"signature"`
	Transaction      *IndexedTransaction `json:"transaction,omitempty"   gorm:"foreignKey:block_number,transaction_index;references:block_number,transaction_index"`
	Block            *IndexedBlock       `json:"block,omitempty"         gorm:"foreignKey:number;references:block_number"`
}
