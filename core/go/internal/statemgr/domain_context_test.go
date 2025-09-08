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

package statemgr

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/filters"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fakeCoinABI = `{
	"type": "tuple",
	"internalType": "struct FakeCoin",
	"components": [
		{
			"name": "salt",
			"type": "bytes32"
		},
		{
			"name": "owner",
			"type": "address",
			"indexed": true
		},
		{
			"name": "amount",
			"type": "uint256",
			"indexed": true
		}
	]
}`

const fakeCoinABI2 = `{
	"type": "tuple",
	"internalType": "struct FakeCoin2",
	"components": [
		{
			"name": "salt",
			"type": "bytes32"
		},
		{
			"name": "owner",
			"type": "address",
			"indexed": true
		},
		{
			"name": "tokenUri",
			"type": "bytes32",
			"indexed": true
		}
	]
}`

type FakeCoin struct {
	Amount ethtypes.HexInteger       `json:"amount"`
	Salt   ethtypes.HexBytes0xPrefix `json:"salt"`
}

func parseFakeCoin(t *testing.T, s *pldapi.State) *FakeCoin {
	var c FakeCoin
	err := json.Unmarshal(s.Data, &c)
	require.NoError(t, err)
	return &c
}

func TestListDomainContexts(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	// Three contexts on different address, two on one domain, one on another
	_, dc1 := newTestDomainContext(t, ctx, ss, "domainA", false)
	_, dc2 := newTestDomainContext(t, ctx, ss, "domainA", false)
	_, dc3 := newTestDomainContext(t, ctx, ss, "domainB", false)

	dcList := ss.ListDomainContexts()
	assert.Len(t, dcList, 3)
	assert.Contains(t, dcList, dc1.Info())
	assert.Contains(t, dcList, dc2.Info())
	assert.Contains(t, dcList, dc3.Info())

	dc1.Close()
	dcList = ss.ListDomainContexts()
	assert.Len(t, dcList, 2)
	assert.NotContains(t, dcList, dc1.Info())

	dc2.Close()
	dc3.Close()
	assert.Empty(t, ss.ListDomainContexts())

}

func TestStateFlushNoWork(t *testing.T) {

	ctx, ss, mdb, _, done := newDBMockStateManager(t)
	defer done()

	_, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	mdb.ExpectBegin()
	mdb.ExpectCommit()

	err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return dc.Flush(dbTX)
	})
	require.NoError(t, err)

	// There was nothing to flush, we we're flushed even before callback
	require.Nil(t, dc.flushing)

}

func TestUpsertSchemaEmptyList(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schemas, err := ss.EnsureABISchemas(ctx, ss.p.NOTX(), "domain1", []*abi.Parameter{})
	require.NoError(t, err)
	require.Len(t, schemas, 0)

}

func TestUpsertSchemaAndStates(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	schemas, err := ss.EnsureABISchemas(ctx, ss.p.NOTX(), "domain1", []*abi.Parameter{testABIParam(t, fakeCoinABI)})
	require.NoError(t, err)
	require.Len(t, schemas, 1)
	schemaID := schemas[0].ID()
	fakeHash1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	fakeHash2 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	_, dc := newTestDomainContext(t, ctx, ss, "domain1", true)
	defer dc.Close()

	upsert1 := &components.StateUpsert{
		ID:     fakeHash1,
		Schema: schemaID,
		Data:   pldtypes.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0x1eDfD974fE6828dE81a1a762df680111870B7cDD", "salt": "%s"}`, pldtypes.RandHex(32))),
	}
	states, err := dc.UpsertStates(ss.p.NOTX(),
		upsert1,
		&components.StateUpsert{
			ID:     fakeHash2,
			Schema: schemaID,
			Data:   pldtypes.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0x1eDfD974fE6828dE81a1a762df680111870B7cDD", "salt": "%s"}`, pldtypes.RandHex(32))),
		},
	)
	require.NoError(t, err)
	require.Len(t, states, 2)
	assert.NotEmpty(t, states[0].ID)
	assert.Equal(t, fakeHash2, states[1].ID)

	// Check the DB is happy with us double-writing states so we don't de-dup anything in the unFlushed list
	_, err = dc.UpsertStates(ss.p.NOTX(), upsert1)
	require.NoError(t, err)
	require.Len(t, dc.unFlushed.states, 3)

	syncFlushContext(t, dc)

}

func TestStateLockErrorsTransaction(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	schemas, err := ss.EnsureABISchemas(ctx, ss.p.NOTX(), "domain1", []*abi.Parameter{testABIParam(t, fakeCoinABI)})
	require.NoError(t, err)
	require.Len(t, schemas, 1)

	_, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	zeroTxn := uuid.UUID{}
	_, err = dc.UpsertStates(ss.p.NOTX(),
		&components.StateUpsert{
			Schema:    schemas[0].ID(),
			Data:      pldtypes.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0x1eDfD974fE6828dE81a1a762df680111870B7cDD", "salt": "%s"}`, pldtypes.RandHex(32))),
			CreatedBy: &zeroTxn,
		},
	)
	require.Regexp(t, "PD010124", err) // zero/missing txn

	err = dc.AddStateLocks(&pldapi.StateLock{
		Type: pldapi.StateLockTypeSpend.Enum(),
	})
	require.Regexp(t, "PD010124", err) // zero/missing txn

	err = dc.AddStateLocks(&pldapi.StateLock{
		Type: pldtypes.Enum[pldapi.StateLockType]("wrong"),
	})
	require.Regexp(t, "PD020003", err) // bad type

	txn1 := uuid.New()
	err = dc.AddStateLocks(&pldapi.StateLock{
		Type:        pldapi.StateLockTypeSpend.Enum(),
		Transaction: txn1,
	})
	require.Regexp(t, "PD010125", err) // missing state

	err = dc.AddStateLocks(&pldapi.StateLock{
		Type:        pldapi.StateLockTypeCreate.Enum(),
		StateID:     pldtypes.RandBytes(32),
		Transaction: txn1,
	})
	require.Regexp(t, "PD010118", err) // create lock for state not in context
}

func TestStateContextMintSpendMint(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	transactionID1 := uuid.New()

	schemas, err := ss.EnsureABISchemas(ctx, ss.p.NOTX(), "domain1", []*abi.Parameter{
		testABIParam(t, fakeCoinABI), // Pop in our widget ABI
		{Type: "tuple", InternalType: "struct TXInfo", Components: abi.ParameterArray{ // and an info state schema
			{Name: "info", Type: "string"},
			{Name: "salt", Type: "bytes32"},
		}},
	})
	require.NoError(t, err)
	assert.Len(t, schemas, 2)
	schemaID := schemas[0].ID()
	infoSchema := schemas[1].ID()

	_, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	// Store some states
	tx1states, err := dc.UpsertStates(ss.p.NOTX(),
		&components.StateUpsert{Schema: schemaID, Data: pldtypes.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, pldtypes.RandHex(32))), CreatedBy: &transactionID1},
		&components.StateUpsert{Schema: schemaID, Data: pldtypes.RawJSON(fmt.Sprintf(`{"amount": 10,  "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, pldtypes.RandHex(32))), CreatedBy: &transactionID1},
		&components.StateUpsert{Schema: schemaID, Data: pldtypes.RawJSON(fmt.Sprintf(`{"amount": 75,  "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, pldtypes.RandHex(32))), CreatedBy: &transactionID1},
		&components.StateUpsert{Schema: infoSchema, Data: pldtypes.RawJSON(fmt.Sprintf(`{"info": "some info", "salt": "%s"}`, pldtypes.RandHex(32)))},
	)
	require.NoError(t, err)
	assert.Len(t, tx1states, 4)

	// Mark an in-memory read - doesn't affect it's availability
	transactionID2 := uuid.New()
	err = dc.AddStateLocks(
		&pldapi.StateLock{Type: pldapi.StateLockTypeRead.Enum(), StateID: tx1states[1].ID, Transaction: transactionID2},
	)
	require.NoError(t, err)

	// Check we can query the current state of locks.
	// The map is by TXID, and within the map the locks are in order created by TX
	lockView := dc.StateLocksByTransaction()
	assert.Equal(t, map[uuid.UUID][]pldapi.StateLock{
		transactionID1: {
			{Type: pldapi.StateLockTypeCreate.Enum(), StateID: tx1states[0].ID, Transaction: transactionID1},
			{Type: pldapi.StateLockTypeCreate.Enum(), StateID: tx1states[1].ID, Transaction: transactionID1},
			{Type: pldapi.StateLockTypeCreate.Enum(), StateID: tx1states[2].ID, Transaction: transactionID1},
		},
		transactionID2: {
			{Type: pldapi.StateLockTypeRead.Enum(), StateID: tx1states[1].ID, Transaction: transactionID2},
		},
	}, lockView)

	// Query the states, and notice we find the ones that are still in the process of creating
	// even though they've not yet been written to the DB
	_, states, err := dc.FindAvailableStates(ss.p.NOTX(), schemaID, query.NewQueryBuilder().Sort("amount").Query())
	require.NoError(t, err)
	assert.Len(t, states, 3)

	// The values should be sorted according to the requested order
	assert.Equal(t, int64(10), parseFakeCoin(t, states[0]).Amount.Int64())
	assert.Equal(t, int64(75), parseFakeCoin(t, states[1]).Amount.Int64())
	assert.Equal(t, int64(100), parseFakeCoin(t, states[2]).Amount.Int64())
	require.Len(t, states[0].Locks, 2)
	assert.Equal(t, pldapi.StateLockTypeCreate, states[0].Locks[0].Type.V()) // should be marked creating
	assert.Equal(t, transactionID1, states[0].Locks[0].Transaction)          // for the transaction we specified
	assert.Equal(t, pldapi.StateLockTypeRead, states[0].Locks[1].Type.V())   // should be marked read
	assert.Equal(t, transactionID2, states[0].Locks[1].Transaction)          // for the transaction we specified

	// Simulate a transaction where we spend two states, and create 2 new ones
	transactionID3 := uuid.New()
	err = dc.AddStateLocks(
		&pldapi.StateLock{Type: pldapi.StateLockTypeSpend.Enum(), StateID: tx1states[1].ID, Transaction: transactionID3}, // 10 +
		&pldapi.StateLock{Type: pldapi.StateLockTypeSpend.Enum(), StateID: tx1states[2].ID, Transaction: transactionID3}, // 75 +
	)
	require.NoError(t, err)

	// Do a quick check on upsert semantics with un-flushed updates, to make sure the unflushed list doesn't dup
	tx3states, err := dc.UpsertStates(ss.p.NOTX(),
		&components.StateUpsert{Schema: schemaID, Data: pldtypes.RawJSON(fmt.Sprintf(`{"amount": 35, "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, pldtypes.RandHex(32))), CreatedBy: &transactionID3},
		&components.StateUpsert{Schema: schemaID, Data: pldtypes.RawJSON(fmt.Sprintf(`{"amount": 50, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, pldtypes.RandHex(32))), CreatedBy: &transactionID3},
	)
	require.NoError(t, err)
	assert.Len(t, tx3states, 2)
	assert.Equal(t, len(dc.unFlushed.states), 6)
	assert.Equal(t, len(dc.txLocks), 8)

	// Query the states on the first address
	_, states, err = dc.FindAvailableStates(ss.p.NOTX(), schemaID, query.NewQueryBuilder().
		Equal("owner", "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180").Sort("-amount").Query())
	require.NoError(t, err)
	assert.Len(t, states, 2)
	assert.Equal(t, int64(100), parseFakeCoin(t, states[0]).Amount.Int64())
	assert.Equal(t, int64(35), parseFakeCoin(t, states[1]).Amount.Int64())

	// Query the states on the other address
	_, states, err = dc.FindAvailableStates(ss.p.NOTX(), schemaID, query.NewQueryBuilder().
		Equal("owner", "0x615dD09124271D8008225054d85Ffe720E7a447A").Sort("-amount").Query())
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, int64(50), parseFakeCoin(t, states[0]).Amount.Int64())

	// Flush the states to the database
	syncFlushContext(t, dc)

	// Check the DB persisted state is what we expect
	_, states, err = dc.FindAvailableStates(ss.p.NOTX(), schemaID, query.NewQueryBuilder().Sort("owner", "amount").Query())
	require.NoError(t, err)
	assert.Len(t, states, 3)
	assert.Equal(t, int64(50), parseFakeCoin(t, states[0]).Amount.Int64())
	assert.Equal(t, int64(35), parseFakeCoin(t, states[1]).Amount.Int64())
	assert.Equal(t, int64(100), parseFakeCoin(t, states[2]).Amount.Int64())

	// Write another transaction that splits a coin to two
	transactionID4 := uuid.New()
	err = dc.AddStateLocks(
		&pldapi.StateLock{Type: pldapi.StateLockTypeSpend.Enum(), StateID: tx3states[1].ID, Transaction: transactionID4}, // 50
		&pldapi.StateLock{Type: pldapi.StateLockTypeRead.Enum(), StateID: tx1states[0].ID, Transaction: transactionID4},  // 100
	)
	require.NoError(t, err)
	tx4states, err := dc.UpsertStates(ss.p.NOTX(),
		&components.StateUpsert{Schema: schemaID, Data: pldtypes.RawJSON(fmt.Sprintf(`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, pldtypes.RandHex(32))), CreatedBy: &transactionID4},
		&components.StateUpsert{Schema: schemaID, Data: pldtypes.RawJSON(fmt.Sprintf(`{"amount": 30, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, pldtypes.RandHex(32))), CreatedBy: &transactionID4},
	)
	require.NoError(t, err)
	assert.Len(t, tx4states, 2)

	// Now check that we merge the DB and in-memory state
	_, states, err = dc.FindAvailableStates(ss.p.NOTX(), schemaID, query.NewQueryBuilder().Sort("owner", "amount").Query())
	require.NoError(t, err)
	assert.Len(t, states, 4)
	assert.Equal(t, int64(20), parseFakeCoin(t, states[0]).Amount.Int64())
	assert.Equal(t, int64(30), parseFakeCoin(t, states[1]).Amount.Int64())
	assert.Equal(t, int64(35), parseFakeCoin(t, states[2]).Amount.Int64())
	assert.Equal(t, int64(100), parseFakeCoin(t, states[3]).Amount.Int64())

	// Check the limit works too across this
	_, states, err = dc.FindAvailableStates(ss.p.NOTX(), schemaID, query.NewQueryBuilder().Sort("owner", "amount").Limit(1).Query())
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, int64(20), parseFakeCoin(t, states[0]).Amount.Int64())

	// Query states by ID - including unflushed, and consumed
	_, statesByID, err := dc.GetStatesByID(ss.p.NOTX(), schemaID, []string{tx3states[1].ID.String(), tx4states[0].ID.String()})
	require.NoError(t, err)
	assert.Len(t, statesByID, 2)
	assert.Equal(t, int64(50), parseFakeCoin(t, statesByID[0]).Amount.Int64())
	assert.Equal(t, int64(20), parseFakeCoin(t, statesByID[1]).Amount.Int64())

	syncFlushContext(t, dc)

	// Write confirmations for all the things that happened above
	var spends []*pldapi.StateSpendRecord
	var reads []*pldapi.StateReadRecord
	var confirms []*pldapi.StateConfirmRecord
	for _, lock := range dc.txLocks {
		switch lock.Type.V() {
		case pldapi.StateLockTypeSpend:
			spends = append(spends, &pldapi.StateSpendRecord{DomainName: "domain1", State: lock.StateID, Transaction: lock.Transaction})
		case pldapi.StateLockTypeRead:
			reads = append(reads, &pldapi.StateReadRecord{DomainName: "domain1", State: lock.StateID, Transaction: lock.Transaction})
		case pldapi.StateLockTypeCreate:
			confirms = append(confirms, &pldapi.StateConfirmRecord{DomainName: "domain1", State: lock.StateID, Transaction: lock.Transaction})
		}
	}
	// We add one extra spend that simulates something happening outside of this context
	transactionID5 := uuid.New()
	spends = append(spends, &pldapi.StateSpendRecord{DomainName: "domain1", State: states[0].ID, Transaction: transactionID5}) //20
	err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.NOTX(), spends, reads, confirms, []*pldapi.StateInfoRecord{
		{DomainName: "domain1", State: tx1states[3].ID, Transaction: transactionID1}, // Add an info record for TX 1
	})
	require.NoError(t, err)

	// So in the domain context, this states will still be visible - because we don't have transactionID5
	// that spends the state, but we have transactionID4 that created the state in our in-memory.
	// So the right thing that would happen in practice, is we would clear transactionID4 when the confirmation
	// was notified to us.
	dc.ResetTransactions(transactionID1)
	dc.ResetTransactions(transactionID3)
	dc.ResetTransactions(transactionID4)

	// We left the read
	assert.Len(t, dc.txLocks, 1)
	assert.Equal(t, pldapi.StateLockTypeRead, dc.txLocks[0].Type.V()) // should be marked read
	assert.Equal(t, transactionID2, dc.txLocks[0].Transaction)        // for the transaction we specified

	// Check the remaining states
	_, states, err = dc.FindAvailableStates(ss.p.NOTX(), schemaID, query.NewQueryBuilder().Sort("owner", "amount").Query())
	require.NoError(t, err)
	assert.Len(t, states, 3)
	assert.Equal(t, int64(30), parseFakeCoin(t, states[0]).Amount.Int64())
	assert.Equal(t, int64(35), parseFakeCoin(t, states[1]).Amount.Int64())
	assert.Equal(t, int64(100), parseFakeCoin(t, states[2]).Amount.Int64())

	// Check the post-commit lookups for each set
	checkPostCommit(t, ss, transactionID1, // transaction one
		[]pldtypes.HexBytes{},
		[]pldtypes.HexBytes{},
		[]pldtypes.HexBytes{tx1states[0].ID, tx1states[2].ID, tx1states[2].ID}, // mints these three ...
		[]pldtypes.HexBytes{tx1states[3].ID},                                   // and has this info
	)
	checkPostCommit(t, ss, transactionID2, // transaction one
		[]pldtypes.HexBytes{},
		[]pldtypes.HexBytes{tx1states[1].ID}, // just reads this one
		[]pldtypes.HexBytes{},
		[]pldtypes.HexBytes{},
	)
	checkPostCommit(t, ss, transactionID3, // transaction three
		[]pldtypes.HexBytes{tx1states[1].ID, tx1states[2].ID}, // spends these two ..
		[]pldtypes.HexBytes{},
		[]pldtypes.HexBytes{tx3states[0].ID, tx3states[1].ID}, // and mints these two
		[]pldtypes.HexBytes{},
	)
	checkPostCommit(t, ss, transactionID4, // transaction four,
		[]pldtypes.HexBytes{tx3states[1].ID},                  // spends this one ...
		[]pldtypes.HexBytes{tx1states[0].ID},                  // reads this one ...
		[]pldtypes.HexBytes{tx4states[0].ID, tx4states[1].ID}, // and mints these two
		[]pldtypes.HexBytes{},
	)

}

func checkPostCommit(t *testing.T, ss *stateManager, txID uuid.UUID, expectedSpent, expectedRead, expectedConfirmed, expectedInfo []pldtypes.HexBytes) {

	txStates, err := ss.GetTransactionStates(ss.bgCtx, ss.p.NOTX(), txID)
	require.NoError(t, err)

	require.Nil(t, txStates.Unavailable)

	toMap := func(states []*pldapi.StateBase) map[string]bool {
		m := make(map[string]bool)
		for _, s := range states {
			m[s.ID.String()] = true
		}
		return m
	}

	spentIDs := toMap(txStates.Spent)
	require.Equal(t, len(expectedSpent), len(spentIDs), "unique spent ID counts match")
	for _, sID := range expectedSpent {
		require.Contains(t, spentIDs, sID.String())
	}

	readIDs := toMap(txStates.Read)
	require.Equal(t, len(expectedRead), len(readIDs), "unique read ID counts match")
	for _, sID := range expectedRead {
		require.Contains(t, readIDs, sID.String())
	}

	confirmedIDs := toMap(txStates.Confirmed)
	require.Equal(t, len(expectedConfirmed), len(confirmedIDs), "unique confirmed ID counts match")
	for _, sID := range expectedConfirmed {
		require.Contains(t, confirmedIDs, sID.String())
	}

	infoIDs := toMap(txStates.Info)
	require.Equal(t, len(expectedInfo), len(infoIDs), "unique confirmed ID counts match")
	for _, sID := range expectedInfo {
		require.Contains(t, infoIDs, sID.String())
	}

}

func TestStateContextMintSpendWithNullifier(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	transactionID1 := uuid.New()

	schemas, err := ss.EnsureABISchemas(ctx, ss.p.NOTX(), "domain1", []*abi.Parameter{testABIParam(t, fakeCoinABI)})
	require.NoError(t, err)
	assert.Len(t, schemas, 1)
	schemaID := schemas[0].ID()
	stateID1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	stateID2 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	nullifier1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	nullifier2 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	data1 := pldtypes.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, pldtypes.RandHex(32)))
	data2 := pldtypes.RawJSON(fmt.Sprintf(`{"amount": 10,  "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, pldtypes.RandHex(32)))

	_, dc := newTestDomainContext(t, ctx, ss, "domain1", true)
	defer dc.Close()

	// Start with 2 states
	tx1states, err := dc.UpsertStates(ss.p.NOTX(),
		&components.StateUpsert{ID: stateID1, Schema: schemaID, Data: data1, CreatedBy: &transactionID1},
		&components.StateUpsert{ID: stateID2, Schema: schemaID, Data: data2, CreatedBy: &transactionID1},
	)
	require.NoError(t, err)
	assert.Len(t, tx1states, 2)

	_, states, err := dc.FindAvailableStates(ss.p.NOTX(), schemaID, query.NewQueryBuilder().Query())
	require.NoError(t, err)
	assert.Len(t, states, 2)
	_, states, err = dc.FindAvailableNullifiers(ss.p.NOTX(), schemaID, query.NewQueryBuilder().Query())
	require.NoError(t, err)
	assert.Len(t, states, 0)

	// Attach a nullifier to the first state
	err = dc.UpsertNullifiers(
		&components.NullifierUpsert{State: stateID1, ID: nullifier1},
	)
	require.NoError(t, err)

	// Cannot attach another nullifier without a reset
	err = dc.UpsertNullifiers(
		&components.NullifierUpsert{State: stateID1, ID: nullifier2},
	)
	assert.Regexp(t, "PD010127", err)

	_, states, err = dc.FindAvailableNullifiers(ss.p.NOTX(), schemaID, query.NewQueryBuilder().Query())
	require.NoError(t, err)
	require.Len(t, states, 1)
	require.NotNil(t, states[0].Nullifier)
	assert.Equal(t, nullifier1, states[0].Nullifier.ID)

	// Flush the states to the database
	syncFlushContext(t, dc)

	// Confirm still 2 states and 1 nullifier
	_, states, err = dc.FindAvailableStates(ss.p.NOTX(), schemaID, query.NewQueryBuilder().Query())
	require.NoError(t, err)
	assert.Len(t, states, 2)
	_, states, err = dc.FindAvailableNullifiers(ss.p.NOTX(), schemaID, query.NewQueryBuilder().Query())
	require.NoError(t, err)
	assert.Len(t, states, 1)
	require.NotNil(t, states[0].Nullifier)
	assert.Equal(t, nullifier1, states[0].Nullifier.ID)

	syncFlushContext(t, dc)

	// Mark both states confirmed
	err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.NOTX(), []*pldapi.StateSpendRecord{}, []*pldapi.StateReadRecord{},
		[]*pldapi.StateConfirmRecord{
			{DomainName: "domain1", State: stateID1, Transaction: transactionID1},
			{DomainName: "domain1", State: stateID2, Transaction: transactionID1},
		}, []*pldapi.StateInfoRecord{})
	require.NoError(t, err)

	// Mark the first state as "spending"
	transactionID2 := uuid.New()
	err = dc.AddStateLocks(
		&pldapi.StateLock{Type: pldapi.StateLockTypeSpend.Enum(), StateID: stateID1, Transaction: transactionID2},
	)
	assert.NoError(t, err)

	// Confirm no more nullifiers available
	_, states, err = dc.FindAvailableNullifiers(ss.p.NOTX(), schemaID, query.NewQueryBuilder().Query())
	require.NoError(t, err)
	assert.Len(t, states, 0)

	// Reset transaction to unlock
	dc.ResetTransactions(transactionID2)
	_, states, err = dc.FindAvailableNullifiers(ss.p.NOTX(), schemaID, query.NewQueryBuilder().Query())
	require.NoError(t, err)
	assert.Len(t, states, 1)

	syncFlushContext(t, dc)

	// Spend the state associated with nullifier
	transactionID3 := uuid.New()
	err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.NOTX(),
		[]*pldapi.StateSpendRecord{
			{DomainName: "domain1", State: nullifier1, Transaction: transactionID3},
		}, []*pldapi.StateReadRecord{}, []*pldapi.StateConfirmRecord{}, []*pldapi.StateInfoRecord{})
	require.NoError(t, err)

	// reset the domain context so we're working from the db
	dc.Reset()

	// Confirm no more nullifiers available
	_, states, err = dc.FindAvailableNullifiers(ss.p.NOTX(), schemaID, query.NewQueryBuilder().Query())
	require.NoError(t, err)
	assert.Len(t, states, 0)

	// Attach a nullifier to the second state
	// Note - this is only allowed when the state is loaded into the context for creation, as otherwise:
	// - queries within the context before the nullifier is flushed would not return the nullifier
	// - the creation of the nullifier in the DB might fail due to the state not existing
	err = dc.UpsertNullifiers(&components.NullifierUpsert{State: stateID2, ID: nullifier2})
	assert.Regexp(t, "PD010126", err)
	_, err = dc.UpsertStates(ss.p.NOTX(), &components.StateUpsert{ID: stateID2, Schema: schemaID, Data: data2, CreatedBy: &transactionID1})
	require.NoError(t, err)
	err = dc.UpsertNullifiers(&components.NullifierUpsert{State: stateID2, ID: nullifier2})
	require.NoError(t, err)

	_, states, err = dc.FindAvailableNullifiers(ss.p.NOTX(), schemaID, query.NewQueryBuilder().Query())
	require.NoError(t, err)
	require.Len(t, states, 1)
	require.NotNil(t, states[0].Nullifier)
	assert.Equal(t, nullifier2, states[0].Nullifier.ID)

}

func TestBadSchema(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	_, err := ss.EnsureABISchemas(ctx, ss.p.NOTX(), "domain1", []*abi.Parameter{{}})
	assert.Regexp(t, "PD010114", err)

}

func TestDomainContextFlushErrorCapture(t *testing.T) {

	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectExec("INSERT.*schemas").WillReturnResult(driver.ResultNoRows)
	db.ExpectBegin()
	db.ExpectExec("INSERT").WillReturnError(fmt.Errorf("pop"))

	schemas, err := ss.EnsureABISchemas(ctx, ss.p.NOTX(), "domain1", []*abi.Parameter{testABIParam(t, fakeCoinABI)})
	require.NoError(t, err)

	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schemas[0].ID()), schemas[0])

	_, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	data1 := fmt.Sprintf(`{"amount": 100, "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, pldtypes.RandHex(32))
	tx1 := uuid.New()
	_, err = dc.UpsertStates(ss.p.NOTX(), genWidget(t, schemas[0].ID(), &tx1, data1))
	require.NoError(t, err)

	// Sync error
	err = ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return dc.Flush(dbTX)
	})
	require.Regexp(t, "pop", err)

	_, _, err = dc.FindAvailableStates(ss.p.NOTX(), schemas[0].ID(), nil)
	assert.Regexp(t, "PD010119.*pop", err) // needs resetp

	_, _, err = dc.FindAvailableNullifiers(ss.p.NOTX(), schemas[0].ID(), nil)
	assert.Regexp(t, "PD010119.*pop", err) // needs reset

	_, err = dc.mergeUnFlushedApplyLocks(schemas[0], nil, nil, true, false)
	assert.Regexp(t, "PD010119.*pop", err) // needs reset

	_, err = dc.UpsertStates(ss.p.NOTX(), genWidget(t, schemas[0].ID(), &tx1, data1))
	assert.Regexp(t, "PD010119.*pop", err) // needs reset

	err = dc.UpsertNullifiers()
	assert.Regexp(t, "PD010119.*pop", err) // needs reset

	err = dc.AddStateLocks(&pldapi.StateLock{})
	assert.Regexp(t, "PD010119.*pop", err) // needs reset

	_, err = dc.ExportSnapshot()
	assert.Regexp(t, "PD010119.*pop", err) // needs reset

	err = dc.ImportSnapshot([]byte("{}"))
	assert.Regexp(t, "PD010119.*pop", err) // needs reset

	err = dc.AddStateLocks(&pldapi.StateLock{})
	assert.Regexp(t, "PD010119.*pop", err) // needs reset

	db.ExpectBegin()
	err = ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return dc.Flush(dbTX)
	})
	assert.Regexp(t, "pop", err) // the original error as it's a flush

	dc.Reset()

	err = dc.checkResetInitUnFlushed()
	require.NoError(t, err)

	// Now check for an async error in the commit itself for some reason outside our control
	_, err = dc.UpsertStates(ss.p.NOTX(), genWidget(t, schemas[0].ID(), &tx1, data1))
	require.NoError(t, err)
	_, err = dc.UpsertStates(ss.p.NOTX(), genWidget(t, schemas[0].ID(), &tx1, data1))
	require.NoError(t, err)

	db.ExpectBegin()
	db.ExpectExec("INSERT.*states").WillReturnResult(driver.ResultNoRows)
	db.ExpectExec("INSERT.*state_labels").WillReturnResult(driver.ResultNoRows)
	db.ExpectCommit()
	err = ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		err := dc.Flush(dbTX)
		require.NoError(t, err)
		err = dc.Flush(dbTX)
		assert.Regexp(t, "PD010131", err) // cannot flush again until we call the callback
		return nil
	})
	require.NoError(t, err)

	dc.flushing = dc.newPendingStateWrites()
	dc.finalizer(ctx, fmt.Errorf("crackle"))

	err = dc.AddStateLocks(&pldapi.StateLock{})
	assert.Regexp(t, "PD010119.*crackle", err) // needs reset

	dc.Close()
	err = dc.UpsertNullifiers()
	assert.Regexp(t, "PD010122", err) // closed

}

func TestDCMergeUnFlushedWhileFlushing(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dc.customHashFunction)
	require.NoError(t, err)
	tx1 := uuid.New()
	_, err = dc.UpsertStates(ss.p.NOTX(), &components.StateUpsert{ID: s1.ID, Schema: schema.ID(), Data: s1.Data, CreatedBy: &tx1})
	require.NoError(t, err)

	// Fake a flush transition
	dc.flushing = dc.unFlushed

	// We'll merge in creating
	states, err := dc.mergeUnFlushedApplyLocks(schema, []*pldapi.State{}, &query.QueryJSON{
		Sort: []string{".created"},
	}, true /* exclude locked */, false /* no nullifier required */)
	require.NoError(t, err)
	assert.Len(t, states, 1)

	// Unless we require a nullifier
	states, err = dc.mergeUnFlushedApplyLocks(schema, []*pldapi.State{}, &query.QueryJSON{
		Sort: []string{".created"},
	}, true /* exclude locked */, true /* nullifier required */)
	require.NoError(t, err)
	assert.Len(t, states, 0)

	// But we can have an unflushed nullifier
	err = dc.UpsertNullifiers(&components.NullifierUpsert{ID: pldtypes.RandBytes(32), State: s1.ID})
	require.NoError(t, err)

	// Fake a flush transition
	dc.flushing.stateNullifiers = append(dc.flushing.stateNullifiers, dc.unFlushed.stateNullifiers...)

	// And then it will return the state
	states, err = dc.mergeUnFlushedApplyLocks(schema, []*pldapi.State{}, &query.QueryJSON{
		Sort: []string{".created"},
	}, true /* exclude locked */, true /* nullifier required */)
	require.NoError(t, err)
	assert.Len(t, states, 1)

}

func TestDSIMergeUnFlushedMultipleSchemas(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema1, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema1.ID()), schema1)

	schema2, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI2))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema2.ID()), schema2)

	contractAddress, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	s1, err := schema1.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dc.customHashFunction)
	require.NoError(t, err)
	s2, err := schema2.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"tokenUri": "%s", "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32), pldtypes.RandHex(32))), nil, dc.customHashFunction)
	require.NoError(t, err)

	dc.creatingStates[s1.ID.String()] = s1
	dc.creatingStates[s2.ID.String()] = s2

	states, err := dc.mergeUnFlushedApplyLocks(schema1, []*pldapi.State{},
		query.NewQueryBuilder().Sort(".created").Query(), true, false)
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, s1.State, states[0])

}

func TestDSIMergeUnFlushedBadDBRecord(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema1, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema1.ID()), schema1)

	contractAddress, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	s1, err := schema1.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dc.customHashFunction)
	require.NoError(t, err)

	dc.creatingStates[s1.ID.String()] = s1

	_, err = dc.mergeUnFlushedApplyLocks(schema1, []*pldapi.State{
		{StateBase: pldapi.StateBase{ID: pldtypes.RandBytes(32), Data: pldtypes.RawJSON("wrong")}},
	}, query.NewQueryBuilder().Sort(".created").Query(), true, false)
	assert.Regexp(t, "PD010116", err)

}

func TestDCMergeUnFlushedWhileFlushingDedup(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	// Add a first state that will be included in the query
	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 10, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dc.customHashFunction)
	require.NoError(t, err)
	tx1 := uuid.New()
	_, err = dc.UpsertStates(ss.p.NOTX(), &components.StateUpsert{ID: s1.ID, Schema: schema.ID(), Data: s1.Data, CreatedBy: &tx1})
	require.NoError(t, err)

	// We add a second state, that will be excluded from the query due to a spending lock
	s2, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dc.customHashFunction)
	require.NoError(t, err)
	_, err = dc.UpsertStates(ss.p.NOTX(), &components.StateUpsert{ID: s2.ID, Schema: schema.ID(), Data: s2.Data, CreatedBy: &tx1})
	require.NoError(t, err)
	err = dc.AddStateLocks(&pldapi.StateLock{Type: pldapi.StateLockTypeSpend.Enum(), StateID: s2.ID, Transaction: tx1})
	require.NoError(t, err)

	// Fake a flush transition
	dc.flushing = dc.unFlushed

	spending, _, _, err := dc.getUnFlushedSpends()
	require.NoError(t, err)
	assert.Len(t, spending, 1)

	// Simulate the DB having returned us the same state we ask for
	dc.stateLock.Lock()
	inTheFlush := dc.flushing.states[0]
	assert.Equal(t, s1.ID, inTheFlush.State.ID)
	dc.stateLock.Unlock()

	states, err := dc.mergeUnFlushedApplyLocks(schema, []*pldapi.State{
		inTheFlush.State,
	}, &query.QueryJSON{
		Sort: []string{".created"},
	}, true, false)
	require.NoError(t, err)
	assert.Len(t, states, 1)

}

func TestDCMergeUnFlushedEvalError(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dc.customHashFunction)
	require.NoError(t, err)
	tx1 := uuid.New()
	_, err = dc.UpsertStates(ss.p.NOTX(), &components.StateUpsert{ID: s1.ID, Schema: schema.ID(), Data: s1.Data, CreatedBy: &tx1})
	require.NoError(t, err)

	_, err = dc.mergeUnFlushedApplyLocks(schema, []*pldapi.State{},
		query.NewQueryBuilder().Equal("wrong", "any").Query(), true, false)
	assert.Regexp(t, "PD010700", err)

}

func TestDCMergedInMemoryMatchesRecoverLabelsFail(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dc.customHashFunction)
	require.NoError(t, err)
	s1.Data = pldtypes.RawJSON(`! wrong `)

	// Insert broken state into our unflushed state list
	dc.flushing = dc.newPendingStateWrites()
	dc.flushing.states = append(dc.flushing.states, s1)

	_, err = dc.mergeInMemoryMatches(schema, []*pldapi.State{
		s1.State,
	}, []*components.StateWithLabels{}, nil)
	assert.Regexp(t, "PD010116", err)

}

func TestDCMergedInMemoryMatchesSortFail(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	contractAddress, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	s1, err := schema.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dc.customHashFunction)
	require.NoError(t, err)

	// Insert state into our unflushed state list
	dc.flushing = dc.newPendingStateWrites()
	dc.flushing.states = append(dc.flushing.states, s1)

	_, err = dc.mergeInMemoryMatches(schema, []*pldapi.State{
		s1.State,
	}, []*components.StateWithLabels{}, query.NewQueryBuilder().Sort("wrong").Query())
	assert.Regexp(t, "PD010700", err)
}

func TestDCFindBadQueryAndInsertBadValue(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema.ID()), schema)

	_, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	schemaID := schema.ID()
	assert.Equal(t, "type=FakeCoin(bytes32 salt,address owner,uint256 amount),labels=[owner,amount]", schema.Signature())

	_, _, err = dc.FindAvailableStates(ss.p.NOTX(), schemaID, query.NewQueryBuilder().Sort("wrong").Query())
	assert.Regexp(t, "PD010700", err)

	_, _, err = dc.FindAvailableNullifiers(ss.p.NOTX(), schemaID, query.NewQueryBuilder().Sort("wrong").Query())
	assert.Regexp(t, "PD010700", err)

	_, err = dc.UpsertStates(ss.p.NOTX(), &components.StateUpsert{
		Schema: schemaID, Data: pldtypes.RawJSON(`"wrong"`),
	})
	assert.Regexp(t, "FF22038", err)

}

func TestDCUpsertStatesFailSchemaLookup(t *testing.T) {

	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectQuery("SELECT.*schema").WillReturnError(fmt.Errorf("pop"))

	_, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	_, err := dc.UpsertStates(ss.p.NOTX(), &components.StateUpsert{
		ID:     pldtypes.RandBytes(32),
		Schema: pldtypes.Bytes32(pldtypes.RandBytes(32)),
	})
	assert.Regexp(t, "pop", err)

}

func TestDCResetWithMixedTxns(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	_, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	state1 := pldtypes.HexBytes("state1")
	transactionID1 := uuid.New()
	err := dc.AddStateLocks(
		&pldapi.StateLock{StateID: state1, Type: pldapi.StateLockTypeRead.Enum(), Transaction: transactionID1})
	require.NoError(t, err)

	state2 := pldtypes.HexBytes("state2")
	transactionID2 := uuid.New()
	err = dc.AddStateLocks(
		&pldapi.StateLock{StateID: state2, Type: pldapi.StateLockTypeSpend.Enum(), Transaction: transactionID2})
	require.NoError(t, err)

	dc.ResetTransactions(transactionID1)

	assert.Len(t, dc.txLocks, 1)
	assert.Equal(t, dc.txLocks[0].StateID, state2)

}

func TestCheckEvalGTTimestamp(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	_, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	jq := query.NewQueryBuilder().GreaterThan(".created", 1726545933211347000).Limit(10).Sort(".created").Query()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	labelSet := dc.ss.labelSetFor(schema)

	ls := filters.PassthroughValueSet{}

	stateID := pldtypes.MustParseHexBytes("2eaf4727b7c7e9b3728b1344ac38ea6d8698603dc3b41d9458d7c011c20ce672")

	// create time is equal - no match
	created := pldtypes.TimestampFromUnix(1726545933211347000)
	addStateBaseLabels(ls, stateID, created)
	match, err := filters.EvalQuery(ctx, jq, labelSet, ls)
	assert.NoError(t, err)
	assert.False(t, match)

	// create time is greater - match
	created = pldtypes.TimestampFromUnix(1726545933211347001)
	addStateBaseLabels(ls, stateID, created)
	match, err = filters.EvalQuery(ctx, jq, labelSet, ls)
	assert.NoError(t, err)
	assert.True(t, match)

	// create time is less - no match
	created = pldtypes.TimestampFromUnix(1726545933211346999)
	addStateBaseLabels(ls, stateID, created)
	match, err = filters.EvalQuery(ctx, jq, labelSet, ls)
	assert.NoError(t, err)
	assert.False(t, match)

}

func TestExportSnapshot(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schema1, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema1.ID()), schema1)

	schema2, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI2))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema2.ID()), schema2)

	contractAddress, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	s1, err := schema1.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dc.customHashFunction)
	require.NoError(t, err)

	s2, err := schema2.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"tokenUri": "%s", "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32), pldtypes.RandHex(32))), nil, dc.customHashFunction)
	require.NoError(t, err)

	transactionID1 := uuid.New()
	transactionID2 := uuid.New()

	_, err = dc.UpsertStates(
		ss.p.NOTX(),
		&components.StateUpsert{
			ID:        s1.ID,
			Schema:    schema1.ID(),
			Data:      s1.Data,
			CreatedBy: &transactionID1,
		},
	)
	require.NoError(t, err)

	err = dc.AddStateLocks(
		&pldapi.StateLock{
			Type:        pldapi.StateLockTypeSpend.Enum(),
			StateID:     s2.ID,
			Transaction: transactionID2,
		},
	)
	assert.NoError(t, err)

	json, err := dc.ExportSnapshot()
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"locks": [
			{
				"stateId":"`+s1.ID.String()+`",
				"transaction":"`+transactionID1.String()+`",
				"type":"create"
			},
			{
				"stateId":"`+s2.ID.String()+`",
				"transaction":"`+transactionID2.String()+`",
				"type":"spend"
			}
		],
		"states": [
		    {
			   "id": "`+s1.ID.String()+`",
			   "schema": "`+s1.Schema.String()+`",
		       "data": `+s1.Data.String()+`
		    }
		]
	}`, string(json),
	)
}

func TestImportSnapshot(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	schema1, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema1.ID()), schema1)

	contractAddress, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	s1, err := schema1.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dc.customHashFunction)
	require.NoError(t, err)

	s2, err := schema1.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dc.customHashFunction)
	require.NoError(t, err)

	s3ID := pldtypes.RandHex(32)

	s4, err := schema1.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dc.customHashFunction)
	require.NoError(t, err)

	s5, err := schema1.ProcessState(ctx, contractAddress, pldtypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		pldtypes.RandHex(32))), nil, dc.customHashFunction)
	require.NoError(t, err)

	transactionID1 := uuid.New()
	transactionID2 := uuid.New()
	transactionID3 := uuid.New()

	stateUpserts := []*components.StateUpsert{
		{
			ID:     s1.ID,
			Schema: schema1.ID(),
			Data:   s1.Data,
		},
		{
			ID:     s2.ID,
			Schema: schema1.ID(),
			Data:   s2.Data,
		},
		{
			ID:     s4.ID,
			Schema: schema1.ID(),
			Data:   s4.Data,
		},
		{
			ID:     s5.ID,
			Schema: schema1.ID(),
			Data:   s5.Data,
		},
	}
	_, err = dc.UpsertStates(ss.p.NOTX(), stateUpserts...)
	require.NoError(t, err)

	//imported locks include
	// - state1 created by transaction1 for which we have the data
	// - state2 created by transaction2 for which we have the data but has been spent by transaction3
	// - state3 created by transaction3 for which we do not have the data
	// - state4 created by transaction3 for which we do have the data
	// and does not include state5 even though we do have the data for that
	// so after all that, the only available states should be state1 and state 4
	jsonToImport := fmt.Sprintf(`{
		"locks": [
			{
				"stateID":"%s",
				"transaction":"%s",
				"type":"create"
			},
			{
				"stateID":"%s",
				"transaction":"%s",
				"type":"create"
			},
			{
				"stateID":"%s",
				"transaction":"%s",
				"type":"create"
			},
			{
				"stateID":"%s",
				"transaction":"%s",
				"type":"create"
			},
			{
				"stateID":"%s",
				"transaction":"%s",
				"type":"spend"
			}
		],
		"states": `+pldtypes.JSONString(stateUpserts).Pretty()+`
	}`,
		s1.ID.String(), transactionID1.String(),
		s2.ID.String(), transactionID2.String(),
		s3ID, transactionID3.String(),
		s4.ID.String(), transactionID3.String(),
		s2.ID.String(), transactionID3.String(),
	)

	err = dc.ImportSnapshot([]byte(jsonToImport))
	require.NoError(t, err)
	_, states, err := dc.FindAvailableStates(ss.p.NOTX(), schema1.ID(), query.NewQueryBuilder().Query())
	require.NoError(t, err)
	require.Len(t, states, 2)
	assert.Equal(t, s1.ID, states[0].ID)
	assert.Equal(t, s4.ID, states[1].ID)

}

func TestImportSnapshotBadStates(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	_, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	err := dc.ImportSnapshot([]byte(`{
		"states": [
			{
				"id": "` + pldtypes.RandHex(32) + `",
				"schema": "` + pldtypes.RandHex(32) + `",
				"data": {}
			}
		]
	}`))
	require.Regexp(t, "PD010133.*PD010106" /* unknown state schema */, err)

}

func TestImportSnapshotJSONError(t *testing.T) {

	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()
	_, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()
	//valid JSON but wrong type for stateID
	jsonToImport := `
		[
			{
				"stateID":true
			}
		]`

	err := dc.ImportSnapshot([]byte(jsonToImport))
	assert.Error(t, err)
	assert.Regexp(t, "PD010132", err)

}

func TestGetStatesByIDFail(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()
	_, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	db.ExpectQuery("SELECT.*schemas").WillReturnError(fmt.Errorf("pop"))

	_, _, err := dc.GetStatesByID(dc.ss.p.NOTX(), pldtypes.Bytes32(pldtypes.RandBytes(32)), []string{pldtypes.RandHex(32)})
	assert.Regexp(t, "pop", err)
}
