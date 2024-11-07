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
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/statemgr"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
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

func writeStates(t *testing.T, txm *txManager, testSchemaID tktypes.Bytes32, fakeContractAddr tktypes.EthAddress, count int) ([]*pldapi.StateBase, []tktypes.HexBytes) {
	stateInputs := make([]*components.StateUpsertOutsideContext, count)
	for i := range stateInputs {
		stateInputs[i] = &components.StateUpsertOutsideContext{
			SchemaID:        tktypes.Bytes32(testSchemaID),
			ContractAddress: fakeContractAddr,
			Data: tktypes.JSONString(map[string]any{
				"salt":   tktypes.RandHex(32),
				"owner":  tktypes.RandAddress(),
				"amount": 10000 + i,
			}),
		}
	}
	written, err := txm.stateMgr.WriteReceivedStates(context.Background(), txm.p.DB(), "domain1", stateInputs)
	require.NoError(t, err)
	states := make([]*pldapi.StateBase, len(written))
	stateIDs := make([]tktypes.HexBytes, len(written))
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

	var stateMgr components.StateManager
	ctx, txm, done := newTestTransactionManager(t, true, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		stateMgr = newRealStateManager(t, mc)
		md := componentmocks.NewDomain(t)
		md.On("Name").Return("domain1")
		md.On("CustomHashFunction").Return(false)
		mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(md, nil)
	})
	defer done()

	txm.stateMgr = stateMgr
	defer txm.stateMgr.Stop()

	schemas, err := txm.stateMgr.EnsureABISchemas(ctx, txm.p.DB(), "domain1", []*abi.Parameter{testStateSchema})
	require.NoError(t, err)

	contractAddressDomain1 := *tktypes.RandAddress()
	testSchemaID := schemas[0].ID()

	// Create the parent TX
	parentTx, err := txm.resolveNewTransaction(ctx, txm.p.DB(), &pldapi.TransactionInput{
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
	_, err = txm.insertTransactions(ctx, txm.p.DB(), []*components.ValidatedTransaction{parentTx}, false)
	require.NoError(t, err)

	// Mimic some states that it produced
	spent, spentIDs := writeStates(t, txm, testSchemaID, contractAddressDomain1, 3)
	read, readIDs := writeStates(t, txm, testSchemaID, contractAddressDomain1, 2)
	confirm, confirmIDs := writeStates(t, txm, testSchemaID, contractAddressDomain1, 5)
	info, infoIDs := writeStates(t, txm, testSchemaID, contractAddressDomain1, 1)

	childFnABI := abi.ABI{{Type: abi.Function, Name: "doThing2"}}
	contractAddressDomain2 := tktypes.RandAddress()
	ptInsert := &components.PrepareTransactionWithRefs{
		ID:     *parentTx.Transaction.ID,
		Domain: parentTx.Transaction.Domain,
		To:     &contractAddressDomain1,
		Transaction: &pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				From:           "me@node1",
				IdempotencyKey: "child_txn",
				Type:           pldapi.TransactionTypePrivate.Enum(),
				Domain:         "domain2",
				To:             contractAddressDomain2,
				Function:       "doThing2",
			},
			ABI: childFnABI,
		},
		States: components.TransactionStateRefs{
			Spent:     spentIDs,
			Read:      readIDs,
			Confirmed: confirmIDs,
			Info:      infoIDs,
		},
		Metadata: tktypes.RawJSON(`{"some":"data"}`),
	}

	storedABI, err := txm.UpsertABI(ctx, txm.p.DB(), childFnABI)
	require.NoError(t, err)

	// Write the prepared TX it results in
	err = txm.WritePreparedTransactions(ctx, txm.p.DB(), []*components.PrepareTransactionWithRefs{ptInsert})
	require.NoError(t, err)

	// Query it back
	pt, err := txm.GetPreparedTransactionByID(ctx, txm.p.DB(), *parentTx.Transaction.ID)
	require.NoError(t, err)
	require.Equal(t, &pldapi.PreparedTransaction{
		ID:     *parentTx.Transaction.ID,
		Domain: "domain1",
		To:     &contractAddressDomain1,
		Transaction: pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				From:           "me@node1",
				IdempotencyKey: "child_txn",
				Type:           pldapi.TransactionTypePrivate.Enum(),
				Domain:         "domain2",
				To:             contractAddressDomain2,
				Function:       "doThing2()",    // now fully qualified
				ABIReference:   &storedABI.Hash, // now resolved
			},
		},
		States: pldapi.TransactionStates{
			Spent:     spent,
			Read:      read,
			Confirmed: confirm,
			Info:      info,
		},
		Metadata: tktypes.RawJSON(`{"some":"data"}`),
	}, pt)

}

func TestWritePreparedTransactionsBadTX(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false)
	defer done()

	err := txm.WritePreparedTransactions(ctx, txm.p.DB(), []*components.PrepareTransactionWithRefs{{
		Transaction: &pldapi.TransactionInput{},
	}})
	assert.Regexp(t, "PD012211", err)

}

func TestQueryPreparedTransactionFailNoLimit(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false)
	defer done()

	_, err := txm.QueryPreparedTransactions(ctx, txm.p.DB(), query.NewQueryBuilder().Query())
	assert.Regexp(t, "PD012200", err)

}

func TestQueryPreparedTransactionFailStates(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*prepared_txns").WillReturnRows(
			sqlmock.NewRows([]string{"id", "transaction"}).
				AddRow(txID, `{}`))
		mc.db.ExpectQuery("SELECT.*prepared_txn_states").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	_, err := txm.QueryPreparedTransactions(ctx, txm.p.DB(), query.NewQueryBuilder().Limit(1).Query())
	assert.Regexp(t, "pop", err)

}
