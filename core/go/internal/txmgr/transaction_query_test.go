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

	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
)

func TestGetTransactionByIDFullFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.GetTransactionByIDFull(ctx, uuid.New())
	assert.Regexp(t, "pop", err)
}

func TestGetTransactionByIDFullPublicFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
		}, mockQueryPublicTxForTransactions(func(ids []uuid.UUID, jq *query.QueryJSON) (map[uuid.UUID][]*pldapi.PublicTx, error) {
			return nil, fmt.Errorf("pop")
		}))
	defer done()

	_, err := txm.GetTransactionByIDFull(ctx, uuid.New())
	assert.Regexp(t, "pop", err)
}

func TestGetTransactionByIDFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.GetTransactionByID(ctx, uuid.New())
	assert.Regexp(t, "pop", err)
}

func TestGetTransactionDependenciesFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.GetTransactionDependencies(ctx, uuid.New())
	assert.Regexp(t, "pop", err)
}

func TestGetResolvedTransactionByIDFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.GetResolvedTransactionByID(ctx, uuid.New())
	assert.Regexp(t, "pop", err)
}

func TestResolveABIReferencesAndCacheFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*abis").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.resolveABIReferencesAndCache(ctx, txm.p.DB(), []*components.ResolvedTransaction{
		{Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				ABIReference: confutil.P((tktypes.Bytes32)(tktypes.RandBytes(32))),
			},
		}},
	})
	assert.Regexp(t, "pop", err)
}

func TestResolveABIReferencesAndCacheBadFunc(t *testing.T) {
	var abiHash = (tktypes.Bytes32)(tktypes.RandBytes(32))
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*abis").WillReturnRows(mc.db.NewRows([]string{"hash", "abi"}).AddRow(
				abiHash.String(), `[]`,
			))
		})
	defer done()

	_, err := txm.resolveABIReferencesAndCache(ctx, txm.p.DB(), []*components.ResolvedTransaction{
		{Transaction: &pldapi.Transaction{
			ID: confutil.P(uuid.New()),
			TransactionBase: pldapi.TransactionBase{
				Function:     "doStuff()",
				To:           tktypes.RandAddress(),
				ABIReference: confutil.P(abiHash),
			},
		}},
	})
	assert.Regexp(t, "PD012206", err)
}
