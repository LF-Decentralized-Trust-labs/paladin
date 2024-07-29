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

import "github.com/kaleido-io/paladin/kata/internal/types"

type IndexedBlock struct {
	Number int64        `json:"number"`
	Hash   types.HashID `json:"hash"                gorm:"primaryKey;embedded;embeddedPrefix:hash_;"`
}

type IndexedTransaction struct {
	Hash        types.HashID `json:"hash"                gorm:"primaryKey;embedded;embeddedPrefix:hash_;"`
	BlockNumber int64        `json:"blockNumber"`
	Index       int64        `json:"index"`
}

type IndexedEvent struct {
	TransactionHash types.HashID `json:"transactionHash"      gorm:"embedded;embeddedPrefix:transaction_;"`
	BlockNumber     int64        `json:"blockNumber"`
	Index           int64        `json:"index"`
	Signature       types.HashID `json:"signature"            gorm:"primaryKey;embedded;embeddedPrefix:signature_;"`
}
