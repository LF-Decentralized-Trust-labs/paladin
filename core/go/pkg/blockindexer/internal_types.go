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
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/filters"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

var IndexedBlockFilters filters.FieldSet = filters.FieldMap{
	"hash":   filters.HexBytesField(`"hash"`),
	"number": filters.Int64Field("number"),
}

var IndexedTransactionFilters filters.FieldSet = filters.FieldMap{
	"hash":             filters.HexBytesField(`"indexed_transactions"."hash"`),
	"blockNumber":      filters.Int64Field("block_number"),
	"transactionIndex": filters.Int64Field("transaction_index"),
	"from":             filters.HexBytesField(`"from"`),
	"to":               filters.HexBytesField("to"),
	"nonce":            filters.Int64Field("nonce"),
	"contractAddress":  filters.HexBytesField("contract_address"),
	"result":           filters.StringField("result"),
}

var IndexedEventFilters filters.FieldSet = filters.FieldMap{
	"blockNumber":      filters.Int64Field("block_number"),
	"transactionIndex": filters.Int64Field("transaction_index"),
	"logIndex":         filters.Int64Field("log_index"),
	"signature":        filters.HexBytesField("signature"),
}

var EventStreamFilters filters.FieldSet = filters.FieldMap{
	"name":    filters.StringField("name"),
	"created": filters.TimestampField("created"),
	"started": filters.BooleanField("started"),
	"type":    filters.StringField("type"),
}

// Contains additional data that the block indexer does not persist, but allows other code to process
// and persist during PreCommitHandlers and PostCommitHandlers (no JSON serialization for these)
type IndexedTransactionNotify struct {
	pldapi.IndexedTransaction
	RevertReason pldtypes.HexBytes
}
