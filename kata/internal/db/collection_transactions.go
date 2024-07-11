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

package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/hyperledger/firefly-common/pkg/dbsql"
)

type TransactionsCollection = dbsql.CRUD[*Transaction]

func (c *persistence) Transactions() TransactionsCollection {
	collection := &dbsql.CrudBase[*Transaction]{
		DB:    c.db,
		Table: "transactions",
		Columns: []string{
			dbsql.ColumnID,
			dbsql.ColumnCreated,
			dbsql.ColumnUpdated,
			"idempotency_key",
			"status",
			"status_message",
			"pre_req_txs",
			"tx_from",
			"tx_contract_address",
			"tx_payload",
			// "assembled_pre_req_txs",
			// "assembled_payload",
			// "assembled_input_states",
			// "assembled_output_states",
			// "confirmation_tracking_id",
		},
		FilterFieldMap: map[string]string{
			"idempotencykey":  "idempotency_key",
			"contractaddress": "tx_contract_address",
			"from":            "tx_from",
		},
		IDValidator:  func(ctx context.Context, idStr string) error { return nil },
		NilValue:     func() *Transaction { return nil },
		NewInstance:  func() *Transaction { return &Transaction{} },
		ScopedFilter: func() sq.Eq { return sq.Eq{ /* no scoping */ } },
		EventHandler: nil, // set below
		GetFieldPtr: func(inst *Transaction, col string) interface{} {
			switch col {
			case dbsql.ColumnID:
				return &inst.ID
			case dbsql.ColumnCreated:
				return &inst.Created
			case dbsql.ColumnUpdated:
				return &inst.Updated
			case "idempotency_key":
				return &inst.IdempotencyKey
			case "status":
				return &inst.Status
			case "status_message":
				return &inst.StatusMessage
			case "pre_req_txs":
				return &inst.PreReqTxs
			case "tx_from":
				return &inst.From
			case "tx_contract_address":
				return &inst.ContractAddress
			case "tx_payload":
				return &inst.Payload
			case "assembled_pre_req_txs":
				return &inst.AssembledPreReqTxs
			case "assembled_payload":
				return &inst.AssembledPayload
			case "assembled_input_states":
				return &inst.AssembledInputStates
			case "assembled_output_states":
				return &inst.AssembledOutputStates
			case "confirmation_tracking_id":
				return &inst.ConfirmationTrackingId
			}

			return nil
		},
	}
	collection.Validate()
	return collection
}
