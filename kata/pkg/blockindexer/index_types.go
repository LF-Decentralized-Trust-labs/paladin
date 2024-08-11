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
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/kata/pkg/types"
)

type IndexedBlock struct {
	Number int64        `json:"number"`
	Hash   types.HashID `json:"hash"                                     gorm:"primaryKey;embedded;embeddedPrefix:hash_;"`
}

type IndexedTransaction struct {
	Hash            types.HashID      `json:"hash"                       gorm:"primaryKey;embedded;embeddedPrefix:hash_;"`
	BlockNumber     int64             `json:"blockNumber"`
	TXIndex         int64             `json:"transactionIndex"`
	From            *types.EthAddress `json:"from"`
	To              *types.EthAddress `json:"to,omitempty"`
	ContractAddress *types.EthAddress `json:"contractAddress,omitempty"`
}

type IndexedEvent struct {
	TransactionHash types.HashID        `json:"transactionHash"          gorm:"embedded;embeddedPrefix:transaction_;"`
	BlockNumber     int64               `json:"blockNumber"`
	TXIndex         int64               `json:"transactionIndex"`
	EventIndex      int64               `json:"eventIndex"`
	Signature       types.HashID        `json:"signature"                gorm:"primaryKey;embedded;embeddedPrefix:signature_;"`
	Transaction     *IndexedTransaction `json:"transaction,omitempty"    gorm:"foreignKey:hash_l,hash_h;references:transaction_l,transaction_h;"`
	Block           *IndexedBlock       `json:"block,omitempty"          gorm:"foreignKey:number;references:block_number;"`
}

type EventStream struct {
	ID  uuid.UUID            `json:"id"                                  gorm:"primaryKey"`
	ABI types.JSONP[abi.ABI] `json:"abi,omitempty"`
}

type EventStreamCheckpoint struct {
	ID          uuid.UUID `json:"id"                                     gorm:"primaryKey"`
	BlockNumber int64     `json:"blockNumber"                            gorm:"primaryKey"`
}

type EventStreamSignature struct {
	Stream    uuid.UUID    `json:"stream"                                gorm:"primaryKey"`
	Signature types.HashID `json:"signature"                             gorm:"primaryKey;embedded;embeddedPrefix:signature_;"`
}

type EventWithData struct {
	Stream uuid.UUID `json:"stream"`
	*IndexedEvent
	Address types.EthAddress `json:"address"`
	Data    types.RawJSON    `json:"data"`
}
