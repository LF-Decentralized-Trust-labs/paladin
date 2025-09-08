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
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/statemgr"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var testStateSchema = &abi.Parameter{
	Type:         "tuple",
	InternalType: "struct FakeCoin",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "bytes32"},
		{Name: "owner", Type: "string", Indexed: true},
		{Name: "amount", Type: "uint256", Indexed: true},
	},
}

func writeStates(t *testing.T, txm *txManager, dbTX persistence.DBTX, testSchemaID pldtypes.Bytes32, fakeContractAddr pldtypes.EthAddress, count int) ([]*pldapi.StateBase, []pldtypes.HexBytes) {
	stateInputs := make([]*components.StateUpsertOutsideContext, count)
	for i := range stateInputs {
		stateInputs[i] = &components.StateUpsertOutsideContext{
			SchemaID:        pldtypes.Bytes32(testSchemaID),
			ContractAddress: &fakeContractAddr,
			Data: pldtypes.JSONString(map[string]any{
				"salt":   pldtypes.RandHex(32),
				"owner":  pldtypes.RandAddress(),
				"amount": 10000 + i,
			}),
		}
	}
	written, err := txm.stateMgr.WriteReceivedStates(context.Background(), dbTX, "domain1", stateInputs)
	require.NoError(t, err)

	states := make([]*pldapi.StateBase, len(written))
	stateIDs := make([]pldtypes.HexBytes, len(written))
	for i, s := range written {
		states[i] = &s.StateBase
		stateIDs[i] = s.ID
	}
	return states, stateIDs
}

func newRealStateManager(t *testing.T, mc *mockComponents) components.StateManager {
	stateMgr := statemgr.NewStateManager(context.Background(), &pldconf.StateStoreConfig{}, mc.c.Persistence())
	_, err := stateMgr.PreInit(mc.c)
	require.NoError(t, err)
	err = stateMgr.PostInit(mc.c)
	require.NoError(t, err)
	err = stateMgr.Start()
	require.NoError(t, err)
	return stateMgr
}

func TestPreparedTransactionRealDB(t *testing.T) {

	contractAddressDomain1 := *pldtypes.RandAddress()
	contractAddressDomain2 := *pldtypes.RandAddress()

	var stateMgr components.StateManager
	ctx, txm, done := newTestTransactionManager(t, true,
		mockDomainContractResolve(t, "domain1", contractAddressDomain1),
		mockDomainContractResolve(t, "domain2", contractAddressDomain2),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			stateMgr = newRealStateManager(t, mc)
			md := componentsmocks.NewDomain(t)
			md.On("Name").Return("domain1")
			md.On("CustomHashFunction").Return(false)
			mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(md, nil)
		})
	defer done()

	txm.stateMgr = stateMgr
	defer txm.stateMgr.Stop()

	var testSchemaID pldtypes.Bytes32
	var parentTx *components.ValidatedTransaction
	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {

		schemas, err := txm.stateMgr.EnsureABISchemas(ctx, dbTX, "domain1", []*abi.Parameter{testStateSchema})
		require.NoError(t, err)

		testSchemaID = schemas[0].ID()

		// Create the parent TX
		parentTx, err = txm.resolveNewTransaction(ctx, dbTX, &pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				From:           "me",
				IdempotencyKey: "parent_txn",
				Type:           pldapi.TransactionTypePrivate.Enum(),
				Domain:         "domain1",
				To:             &contractAddressDomain1,
				Function:       "doThing1",
			},
			ABI: abi.ABI{{Type: abi.Function, Name: "doThing1"}},
		}, pldapi.SubmitModeAuto)
		require.NoError(t, err)
		_, err = txm.insertTransactions(ctx, dbTX, []*components.ValidatedTransaction{parentTx}, false)
		return err
	})
	require.NoError(t, err)

	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		// Mimic some states that it produced
		spent, spentIDs := writeStates(t, txm, dbTX, testSchemaID, contractAddressDomain1, 3)
		read, readIDs := writeStates(t, txm, dbTX, testSchemaID, contractAddressDomain1, 2)
		confirm, confirmIDs := writeStates(t, txm, dbTX, testSchemaID, contractAddressDomain1, 5)
		info, infoIDs := writeStates(t, txm, dbTX, testSchemaID, contractAddressDomain1, 1)

		childFnABI := abi.ABI{{Type: abi.Function, Name: "doThing2"}}
		ptInsert := &components.PreparedTransactionWithRefs{
			PreparedTransactionBase: &pldapi.PreparedTransactionBase{
				ID:       *parentTx.Transaction.ID,
				Domain:   parentTx.Transaction.Domain,
				To:       &contractAddressDomain1,
				Metadata: pldtypes.RawJSON(`{"some":"data"}`),
				Transaction: pldapi.TransactionInput{
					TransactionBase: pldapi.TransactionBase{
						From:           "me@node1",
						IdempotencyKey: "child_txn",
						Type:           pldapi.TransactionTypePrivate.Enum(),
						Domain:         "domain2",
						To:             &contractAddressDomain2,
						Function:       "doThing2",
					},
					ABI: childFnABI,
				},
			},
			StateRefs: components.TransactionStateRefs{
				Spent:     spentIDs,
				Read:      readIDs,
				Confirmed: confirmIDs,
				Info:      infoIDs,
			},
		}

		storedABI, err := txm.UpsertABI(ctx, dbTX, childFnABI)
		require.NoError(t, err)

		// Write the prepared TX it results in
		err = txm.WritePreparedTransactions(ctx, dbTX, []*components.PreparedTransactionWithRefs{ptInsert})
		require.NoError(t, err)

		expectedPBT := &pldapi.PreparedTransactionBase{
			ID:     *parentTx.Transaction.ID,
			Domain: "domain1",
			To:     &contractAddressDomain1,
			Transaction: pldapi.TransactionInput{
				TransactionBase: pldapi.TransactionBase{
					From:           "me@node1",
					IdempotencyKey: "child_txn",
					Type:           pldapi.TransactionTypePrivate.Enum(),
					Domain:         "domain2",
					To:             &contractAddressDomain2,
					Function:       "doThing2()",           // now fully qualified
					ABIReference:   &storedABI.Hash,        // now resolved
					Data:           pldtypes.RawJSON(`{}`), // normalized
				},
			},
			Metadata: pldtypes.RawJSON(`{"some":"data"}`),
		}

		// Query it back
		pt, err := txm.GetPreparedTransactionByID(ctx, dbTX, *parentTx.Transaction.ID)
		require.NoError(t, err)
		require.Equal(t, &pldapi.PreparedTransaction{
			PreparedTransactionBase: expectedPBT,
			States: pldapi.TransactionStates{
				Spent:     spent,
				Read:      read,
				Confirmed: confirm,
				Info:      info,
			},
		}, pt)

		// Query it back
		ptr, err := txm.GetPreparedTransactionWithRefsByID(ctx, dbTX, *parentTx.Transaction.ID)
		require.NoError(t, err)
		require.Equal(t, &components.PreparedTransactionWithRefs{
			PreparedTransactionBase: expectedPBT,
			StateRefs: components.TransactionStateRefs{
				Spent:     stateIDs(spent),
				Read:      stateIDs(read),
				Confirmed: stateIDs(confirm),
				Info:      stateIDs(info),
			},
		}, ptr)

		return nil
	})
	require.NoError(t, err)

}

func stateIDs(states []*pldapi.StateBase) []pldtypes.HexBytes {
	stateIDs := make([]pldtypes.HexBytes, len(states))
	for i, s := range states {
		stateIDs[i] = s.ID
	}
	return stateIDs
}

func TestWritePreparedTransactionsBadTX(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	err := txm.WritePreparedTransactions(ctx, txm.p.NOTX(), []*components.PreparedTransactionWithRefs{{
		PreparedTransactionBase: &pldapi.PreparedTransactionBase{},
	}})
	assert.Regexp(t, "PD012211", err)

}

func TestQueryPreparedTransactionFailNoLimit(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	_, err := txm.QueryPreparedTransactions(ctx, txm.p.NOTX(), query.NewQueryBuilder().Query())
	assert.Regexp(t, "PD010721", err)

}

func TestQueryPreparedTransactionWithRefsFailNoLimit(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	_, err := txm.QueryPreparedTransactionsWithRefs(ctx, txm.p.NOTX(), query.NewQueryBuilder().Query())
	assert.Regexp(t, "PD010721", err)

}

func TestQueryPreparedTransactionFailStates(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*prepared_txns").WillReturnRows(
				sqlmock.NewRows([]string{"id", "transaction"}).
					AddRow(txID, `{}`))
			mc.db.ExpectQuery("SELECT.*prepared_txn_states").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.QueryPreparedTransactions(ctx, txm.p.NOTX(), query.NewQueryBuilder().Limit(1).Query())
	assert.Regexp(t, "pop", err)

}

func TestQueryPreparedTransactionWithRefsFailStates(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*prepared_txns").WillReturnRows(
				sqlmock.NewRows([]string{"id", "transaction"}).
					AddRow(txID, `{}`))
			mc.db.ExpectQuery("SELECT.*prepared_txn_states").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.QueryPreparedTransactionsWithRefs(ctx, txm.p.NOTX(), query.NewQueryBuilder().Limit(1).Query())
	assert.Regexp(t, "pop", err)

}

func TestGetPreparedTransactionWithRefsByIDNotFound(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*prepared_txns").WillReturnRows(
				sqlmock.NewRows([]string{"id", "transaction"}))
		})
	defer done()

	pt, err := txm.GetPreparedTransactionWithRefsByID(ctx, txm.p.NOTX(), uuid.New())
	require.NoError(t, err)
	assert.Nil(t, pt)

}
