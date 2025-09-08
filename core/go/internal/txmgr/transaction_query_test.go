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

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(txID))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*transaction_history").WillReturnRows(sqlmock.NewRows([]string{"id", "tx_id"}).AddRow(uuid.New(), txID))
		}, mockQueryPublicTxForTransactions(func(ids []uuid.UUID, jq *query.QueryJSON) (map[uuid.UUID][]*pldapi.PublicTx, error) {
			return nil, fmt.Errorf("pop")
		}))
	defer done()

	_, err := txm.GetTransactionByIDFull(ctx, txID)
	assert.Regexp(t, "pop", err)
}

func TestGetTransactionByIDFullPublicHistoryFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*transaction_history").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.GetTransactionByIDFull(ctx, uuid.New())
	assert.Regexp(t, "pop", err)
}

func TestGetTransactionByIDFullPublicHistory(t *testing.T) {
	txID := uuid.New()
	to1 := pldtypes.RandAddress()
	to2 := pldtypes.RandAddress()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(txID))
			mc.db.ExpectQuery("SELECT.*transaction_deps").WillReturnRows(sqlmock.NewRows([]string{}))
			rows := sqlmock.NewRows([]string{"id", "tx_id", "to"}).
				AddRow(uuid.New(), txID, to1).
				AddRow(uuid.New(), txID, to2)
			mc.db.ExpectQuery("SELECT.*transaction_history").WillReturnRows(rows)
		}, mockQueryPublicTxForTransactions(func(ids []uuid.UUID, jq *query.QueryJSON) (map[uuid.UUID][]*pldapi.PublicTx, error) {
			pubTX := map[uuid.UUID][]*pldapi.PublicTx{
				txID: {{
					To: to2,
				}},
			}
			return pubTX, nil
		}))
	defer done()

	tx, err := txm.GetTransactionByIDFull(ctx, txID)
	require.NoError(t, err)
	require.Equal(t, 2, len(tx.History))
	assert.Equal(t, to1, tx.History[0].To)
	assert.Equal(t, to2, tx.History[1].To)
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

	_, err := txm.resolveABIReferencesAndCache(ctx, txm.p.NOTX(), []*components.ResolvedTransaction{
		{Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				ABIReference: confutil.P((pldtypes.Bytes32)(pldtypes.RandBytes(32))),
			},
		}},
	})
	assert.Regexp(t, "pop", err)
}

func TestResolveABIReferencesAndCacheBadFunc(t *testing.T) {
	var abiHash = (pldtypes.Bytes32)(pldtypes.RandBytes(32))
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*abis").WillReturnRows(mc.db.NewRows([]string{"hash", "abi"}).AddRow(
				abiHash.String(), `[]`,
			))
		})
	defer done()

	_, err := txm.resolveABIReferencesAndCache(ctx, txm.p.NOTX(), []*components.ResolvedTransaction{
		{Transaction: &pldapi.Transaction{
			ID: confutil.P(uuid.New()),
			TransactionBase: pldapi.TransactionBase{
				Function:     "doStuff()",
				To:           pldtypes.RandAddress(),
				ABIReference: confutil.P(abiHash),
			},
		}},
	})
	assert.Regexp(t, "PD012206", err)
}
