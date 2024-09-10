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
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
)

type BlockInfoJSONRPC struct {
	Number       ethtypes.HexUint64        `json:"number"`
	Hash         ethtypes.HexBytes0xPrefix `json:"hash"`
	ParentHash   ethtypes.HexBytes0xPrefix `json:"parentHash"`
	Timestamp    ethtypes.HexUint64        `json:"timestamp"`
	Transactions []*PartialTransactionInfo `json:"transactions"`
}

type PartialTransactionInfo struct {
	Hash  ethtypes.HexBytes0xPrefix `json:"hash"`
	Nonce ethtypes.HexUint64        `json:"nonce"`
}

type TXReceiptJSONRPC struct {
	BlockHash         ethtypes.HexBytes0xPrefix `json:"blockHash"`
	BlockNumber       ethtypes.HexUint64        `json:"blockNumber"`
	ContractAddress   *ethtypes.Address0xHex    `json:"contractAddress"`
	CumulativeGasUsed *ethtypes.HexInteger      `json:"cumulativeGasUsed"`
	From              *ethtypes.Address0xHex    `json:"from"`
	GasUsed           *ethtypes.HexInteger      `json:"gasUsed"`
	Logs              []*LogJSONRPC             `json:"logs"`
	Status            *ethtypes.HexInteger      `json:"status"`
	To                *ethtypes.Address0xHex    `json:"to"`
	TransactionHash   ethtypes.HexBytes0xPrefix `json:"transactionHash"`
	TransactionIndex  *ethtypes.HexInteger      `json:"transactionIndex"`
}

type LogJSONRPC struct {
	Removed          bool                        `json:"removed"`
	LogIndex         ethtypes.HexUint64          `json:"logIndex"`
	TransactionIndex ethtypes.HexUint64          `json:"transactionIndex"`
	BlockNumber      ethtypes.HexUint64          `json:"blockNumber"`
	TransactionHash  ethtypes.HexBytes0xPrefix   `json:"transactionHash"`
	BlockHash        ethtypes.HexBytes0xPrefix   `json:"blockHash"`
	Address          *ethtypes.Address0xHex      `json:"address"`
	Data             ethtypes.HexBytes0xPrefix   `json:"data"`
	Topics           []ethtypes.HexBytes0xPrefix `json:"topics"`
}
