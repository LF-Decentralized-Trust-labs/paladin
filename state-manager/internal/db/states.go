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
	"github.com/hyperledger/firefly-common/pkg/dbsql"
	"github.com/kaleido-io/paladin-state-manager/pkg/apitypes"
)

type StatesCRUD interface {
	dbsql.CRUD[*apitypes.State]
}

func (p *persistence) States() StatesCRUD {
	crud := &dbsql.CrudBase[*apitypes.State]{
		DB:             p.db,
		Table:          "states",
		ReadTableAlias: "states",
		Columns: []string{
			dbsql.ColumnID,
			dbsql.ColumnCreated,
			dbsql.ColumnUpdated,
			"state",
		},
		NilValue:     func() *apitypes.State { return nil },
		NewInstance:  func() *apitypes.State { return &apitypes.State{} },
		QueryFactory: apitypes.StateFilters,
		GetFieldPtr: func(inst *apitypes.State, col string) interface{} {
			switch col {
			case dbsql.ColumnID:
				return &inst.ID
			case dbsql.ColumnCreated:
				return &inst.Created
			case dbsql.ColumnUpdated:
				return &inst.Updated
			case "state":
				return &inst.State
			}
			return nil
		},
	}
	return crud
}
