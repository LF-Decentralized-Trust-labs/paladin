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

func testQueryWrapper(txm *txManager, jq *query.QueryJSON) *queryWrapper[persistedTransaction, pldapi.Transaction] {
	return &queryWrapper[persistedTransaction, pldapi.Transaction]{
		p:           txm.p,
		table:       "transactions",
		defaultSort: "-created",
		filters:     transactionFilters,
		query:       jq,
	}
}

func TestQueryWrapperLimitRequired(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false)
	defer done()

	_, err := testQueryWrapper(txm, query.NewQueryBuilder().Query()).run(ctx, txm.p.DB())
	assert.Regexp(t, "PD012200", err)
}

func TestQueryWrapperMapFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	})
	defer done()

	qw := testQueryWrapper(txm, query.NewQueryBuilder().Limit(1).Query())
	qw.mapResult = func(pt *persistedTransaction) (*pldapi.Transaction, error) {
		return nil, fmt.Errorf("pop")
	}
	_, err := qw.run(ctx, txm.p.DB())
	assert.Regexp(t, "pop", err)
}

func TestInt64OrZero(t *testing.T) {
	assert.Equal(t, int64(0), int64OrZero(nil))
}
