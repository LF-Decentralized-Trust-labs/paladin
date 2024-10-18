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

package txmgr

import (
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/stretchr/testify/assert"
)

func TestGetTransactionByIDFullFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectBegin()
		mc.db.ExpectQuery("SELECT.*transactions").WillReturnError(fmt.Errorf("pop"))
		mc.db.ExpectRollback()
	})
	defer done()

	_, err := txm.GetTransactionByIDFull(ctx, uuid.New())
	assert.Regexp(t, "pop", err)
}

func TestGetTransactionByIDFullPublicFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectBegin()
		mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
		mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
		mc.db.ExpectRollback()
	}, mockQueryPublicTxForTransactions(func(ids []uuid.UUID, jq *query.QueryJSON) (map[uuid.UUID][]*pldapi.PublicTx, error) {
		return nil, fmt.Errorf("pop")
	}))
	defer done()

	_, err := txm.GetTransactionByIDFull(ctx, uuid.New())
	assert.Regexp(t, "pop", err)
}

func TestGetTransactionByIDFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*transactions").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	_, err := txm.GetTransactionByID(ctx, uuid.New())
	assert.Regexp(t, "pop", err)
}

func TestGetTransactionDependenciesFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	_, err := txm.GetTransactionDependencies(ctx, uuid.New())
	assert.Regexp(t, "pop", err)
}
