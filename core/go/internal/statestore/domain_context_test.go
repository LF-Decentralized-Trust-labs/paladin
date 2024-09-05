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

package statestore

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
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

type FakeCoin struct {
	Amount ethtypes.HexInteger       `json:"amount"`
	Salt   ethtypes.HexBytes0xPrefix `json:"salt"`
}

func parseFakeCoin(t *testing.T, s *State) *FakeCoin {
	var c FakeCoin
	err := json.Unmarshal(s.Data, &c)
	assert.NoError(t, err)
	return &c
}

func TestStateFlushAsync(t *testing.T) {

	_, ss, done := newDBTestStateStore(t)
	defer done()

	schemaIDReceiver := make(chan string)

	// Run one handler that ends in a flush, of a schema that won't be available unless we flush
	err := ss.RunInDomainContext("domain1", func(ctx context.Context, dsi DomainStateInterface) error {
		schemas, err := dsi.EnsureABISchemas([]*abi.Parameter{testABIParam(t, fakeCoinABI)})
		assert.NoError(t, err)
		assert.Len(t, schemas, 1)
		schemaID := schemas[0].IDString()
		return dsi.Flush(func(ctx context.Context, dsi DomainStateInterface) error {
			schemaIDReceiver <- schemaID
			return nil
		})
	})
	assert.NoError(t, err)

	var schemaID string
	select {
	case schemaID = <-schemaIDReceiver:
	case <-time.After(5 * time.Second):
		assert.Fail(t, "timed out")
	}

	// Run a 2nd handler that depends on that schema being available
	err = ss.RunInDomainContext("domain1", func(ctx context.Context, dsi DomainStateInterface) error {
		states, err := dsi.UpsertStates(nil, []*StateUpsert{
			{
				SchemaID: schemaID,
				Data:     tktypes.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0x1eDfD974fE6828dE81a1a762df680111870B7cDD", "salt": "%s"}`, tktypes.RandHex(32))),
			},
		})
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		return nil
	})
	assert.NoError(t, err)

}

func TestStateContextMintSpendMint(t *testing.T) {

	_, ss, done := newDBTestStateStore(t)
	defer done()

	transactionID := uuid.New()
	var schemaID string

	err := ss.RunInDomainContextFlush("domain1", func(ctx context.Context, dsi DomainStateInterface) error {
		// Pop in our widget ABI
		schemas, err := dsi.EnsureABISchemas([]*abi.Parameter{testABIParam(t, fakeCoinABI)})
		assert.NoError(t, err)
		assert.Len(t, schemas, 1)
		schemaID = schemas[0].IDString()

		// Need to flush for the schemas to be available
		return nil
	})
	assert.NoError(t, err)

	err = ss.RunInDomainContextFlush("domain1", func(ctx context.Context, dsi DomainStateInterface) error {

		// Store some states
		tx1states, err := dsi.UpsertStates(&transactionID, []*StateUpsert{
			{SchemaID: schemaID, Data: tktypes.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, tktypes.RandHex(32))), Creating: true},
			{SchemaID: schemaID, Data: tktypes.RawJSON(fmt.Sprintf(`{"amount": 10,  "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, tktypes.RandHex(32))), Creating: true},
			{SchemaID: schemaID, Data: tktypes.RawJSON(fmt.Sprintf(`{"amount": 75,  "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, tktypes.RandHex(32))), Creating: true},
		})
		assert.NoError(t, err)
		assert.Len(t, tx1states, 3)

		// Mark an in-memory read - doesn't affect it's availability, but will be locked to that transaction
		err = dsi.MarkStatesRead(transactionID, []string{tx1states[0].ID.String()})
		assert.NoError(t, err)

		// We can't arbitrarily move it to another transaction (would need to reset the first transaction)
		err = dsi.MarkStatesRead(uuid.New(), []string{tx1states[0].ID.String()})
		assert.Regexp(t, "PD010118", err)
		err = dsi.MarkStatesSpending(uuid.New(), []string{tx1states[0].ID.String()})
		assert.Regexp(t, "PD010118", err)

		// Query the states, and notice we find the ones that are still in the process of creating
		// even though they've not yet been written to the DB
		states, err := dsi.FindAvailableStates(schemaID, toQuery(t, `{
			"sort": [ "amount" ]
		}`))
		assert.NoError(t, err)
		assert.Len(t, states, 3)

		// The values should be sorted according to the requested order
		assert.Equal(t, int64(10), parseFakeCoin(t, states[0]).Amount.Int64())
		assert.Equal(t, int64(75), parseFakeCoin(t, states[1]).Amount.Int64())
		assert.Equal(t, int64(100), parseFakeCoin(t, states[2]).Amount.Int64())
		assert.True(t, states[0].Locked.Creating)                    // should be marked creating
		assert.Equal(t, transactionID, states[0].Locked.Transaction) // for the transaction we specified

		// Simulate a transaction where we spend two states, and create 2 new ones
		err = dsi.MarkStatesSpending(transactionID, []string{
			states[0].ID.String(), // 10 +
			states[1].ID.String(), // 75
		})
		assert.NoError(t, err)

		// Do a quick check on upsert semantics with un-flushed updates, to make sure the unflushed list doesn't dup
		tx2Salts := []string{tktypes.RandHex(32), tktypes.RandHex(32)}
		for dup := 0; dup < 2; dup++ {
			tx2states, err := dsi.UpsertStates(&transactionID, []*StateUpsert{
				{SchemaID: schemaID, Data: tktypes.RawJSON(fmt.Sprintf(`{"amount": 35, "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, tx2Salts[0])), Creating: true},
				{SchemaID: schemaID, Data: tktypes.RawJSON(fmt.Sprintf(`{"amount": 50, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, tx2Salts[1])), Creating: true},
			})
			assert.NoError(t, err)
			assert.Len(t, tx2states, 2)
			assert.Equal(t, len(dsi.(*domainContext).unFlushed.states), 5)
			assert.Equal(t, len(dsi.(*domainContext).unFlushed.stateLocks), 5)
		}

		// Query the states on the first address
		states, err = dsi.FindAvailableStates(schemaID, toQuery(t, `{
			"sort": [ "-amount" ],
			"eq": [{"field": "owner", "value": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180"}]
		}`))
		assert.NoError(t, err)
		assert.Len(t, states, 2)
		assert.Equal(t, int64(100), parseFakeCoin(t, states[0]).Amount.Int64())
		assert.Equal(t, int64(35), parseFakeCoin(t, states[1]).Amount.Int64())

		// Query the states on the other address
		states, err = dsi.FindAvailableStates(schemaID, toQuery(t, `{
					"sort": [ "-amount" ],
					"eq": [{"field": "owner", "value": "0x615dD09124271D8008225054d85Ffe720E7a447A"}]
				}`))
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, int64(50), parseFakeCoin(t, states[0]).Amount.Int64())

		// Flush the states to the database
		return nil
	})
	assert.NoError(t, err)

	err = ss.RunInDomainContextFlush("domain1", func(ctx context.Context, dsi DomainStateInterface) error {
		// Check the DB persisted state is what we expect
		states, err := dsi.FindAvailableStates(schemaID, toQuery(t, `{
			"sort": [ "owner", "amount" ]
		}`))
		assert.NoError(t, err)
		assert.Len(t, states, 3)
		assert.Equal(t, int64(50), parseFakeCoin(t, states[0]).Amount.Int64())
		assert.Equal(t, int64(35), parseFakeCoin(t, states[1]).Amount.Int64())
		assert.Equal(t, int64(100), parseFakeCoin(t, states[2]).Amount.Int64())

		// Mark a persisted one read - doesn't affect it's availability, but will be locked to that transaction
		err = dsi.MarkStatesRead(transactionID, []string{
			states[1].ID.String(),
		})
		assert.NoError(t, err)

		// Write another transaction that splits a coin to two
		err = dsi.MarkStatesSpending(transactionID, []string{
			states[0].ID.String(), // 50
		})
		assert.NoError(t, err)
		tx3states, err := dsi.UpsertStates(&transactionID, []*StateUpsert{
			{SchemaID: schemaID, Data: tktypes.RawJSON(fmt.Sprintf(`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, tktypes.RandHex(32))), Creating: true},
			{SchemaID: schemaID, Data: tktypes.RawJSON(fmt.Sprintf(`{"amount": 30, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, tktypes.RandHex(32))), Creating: true},
		})
		assert.NoError(t, err)
		assert.Len(t, tx3states, 2)

		// Now check that we merge the DB and in-memory state
		states, err = dsi.FindAvailableStates(schemaID, toQuery(t, `{
					"sort": [ "owner", "amount" ]
				}`))
		assert.NoError(t, err)
		assert.Len(t, states, 4)
		assert.Equal(t, int64(20), parseFakeCoin(t, states[0]).Amount.Int64())
		assert.Equal(t, int64(30), parseFakeCoin(t, states[1]).Amount.Int64())
		assert.Equal(t, int64(35), parseFakeCoin(t, states[2]).Amount.Int64())
		assert.Equal(t, int64(100), parseFakeCoin(t, states[3]).Amount.Int64())

		// Check the limit works too across this
		states, err = dsi.FindAvailableStates(schemaID, toQuery(t, `{
			"limit": 1,
			"sort": [ "owner", "amount" ]
		}`))
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, int64(20), parseFakeCoin(t, states[0]).Amount.Int64())

		// Reset the transaction - this will clear the in-memory state,
		// and remove the locks from the DB. It will not remove the states
		// themselves
		err = dsi.ResetTransaction(transactionID)
		assert.NoError(t, err)

		// None of the states will be returned to available after the flush
		// - but before then the DB ones will be
		return nil
	})
	assert.NoError(t, err)

	err = ss.RunInDomainContextFlush("domain1", func(ctx context.Context, dsi DomainStateInterface) error {

		// Confirm
		states, err := dsi.FindAvailableStates(schemaID, toQuery(t, `{}`))
		assert.NoError(t, err)
		assert.Empty(t, states)

		return nil
	})
	assert.NoError(t, err)

}

func TestDSILatch(t *testing.T) {

	_, ss, done := newDBTestStateStore(t)

	dsi := ss.getDomainContext("domain1")
	err := dsi.takeLatch()
	assert.NoError(t, err)

	done()
	err = dsi.run(func(ctx context.Context, dsi DomainStateInterface) error { return nil })
	assert.Regexp(t, "PD010301", err)

}

func TestDSIBadSchema(t *testing.T) {

	_, ss, _, done := newDBMockStateStore(t)
	defer done()

	err := ss.RunInDomainContext("domain1", func(ctx context.Context, dsi DomainStateInterface) error {
		_, err := dsi.EnsureABISchemas([]*abi.Parameter{{}})
		return err
	})
	assert.Regexp(t, "PD010114", err)

}

func TestDSIFlushErrorCapture(t *testing.T) {

	_, ss, done := newDBTestStateStore(t)
	defer done()

	fakeFlushError := func(dc *domainContext) {
		dc.flushing = &writeOperation{}
		dc.flushResult = make(chan error, 1)
		dc.flushResult <- fmt.Errorf("pop")
	}

	var schemas []Schema
	err := ss.RunInDomainContextFlush("domain1", func(ctx context.Context, dsi DomainStateInterface) (err error) {
		schemas, err = dsi.EnsureABISchemas([]*abi.Parameter{testABIParam(t, fakeCoinABI)})
		assert.NoError(t, err)
		return nil
	})
	assert.NoError(t, err)

	err = ss.RunInDomainContextFlush("domain1", func(ctx context.Context, dsi DomainStateInterface) error {

		dc := dsi.(*domainContext)

		fakeFlushError(dc)
		_, err = dsi.EnsureABISchemas(nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		_, err = dsi.FindAvailableStates("", nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		schema, err := ss.getSchemaByID(ctx, "domain1", tktypes.MustParseBytes32(schemas[0].IDString()), true)
		assert.NoError(t, err)
		_, err = dc.mergedUnFlushed(schema, nil, nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		_, err = dsi.UpsertStates(nil, nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		err = dsi.MarkStatesRead(uuid.New(), nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		err = dsi.MarkStatesSpending(uuid.New(), nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		err = dsi.ResetTransaction(uuid.New())
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		err = dsi.Flush()
		assert.Regexp(t, "pop", err)

		return nil

	})
	assert.NoError(t, err)

}

func TestDSIMergedUnFlushedWhileFlushing(t *testing.T) {

	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	assert.NoError(t, err)

	dc := ss.getDomainContext("domain1")

	s1, err := schema.ProcessState(ctx, tktypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		tktypes.RandHex(32))))
	assert.NoError(t, err)
	s1.Locked = &StateLock{State: s1.ID, Transaction: uuid.New(), Creating: true}

	dc.flushing = &writeOperation{
		states: []*StateWithLabels{s1},
		stateLocks: []*StateLock{
			s1.Locked,
			{State: tktypes.Bytes32Keccak(([]byte)("another")), Spending: true},
		},
	}

	spending, err := dc.getUnFlushedSpending()
	assert.NoError(t, err)
	assert.Len(t, spending, 1)

	states, err := dc.mergedUnFlushed(schema, []*State{}, &filters.QueryJSON{
		Sort: []string{".created"},
	})
	assert.NoError(t, err)
	assert.Len(t, states, 1)

}

func TestDSIMergedUnFlushedEvalError(t *testing.T) {

	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	assert.NoError(t, err)

	dc := ss.getDomainContext("domain1")

	s1, err := schema.ProcessState(ctx, tktypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		tktypes.RandHex(32))))
	assert.NoError(t, err)

	dc.flushing = &writeOperation{
		states: []*StateWithLabels{s1},
	}

	_, err = dc.mergedUnFlushed(schema, []*State{}, toQuery(t,
		`{"eq": [{ "field": "wrong", "value": "any" }]}`,
	))
	assert.Regexp(t, "PD010700", err)

}

func TestDSIMergedInMemoryMatchesRecoverLabelsFail(t *testing.T) {

	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	assert.NoError(t, err)

	dc := ss.getDomainContext("domain1")

	s1, err := schema.ProcessState(ctx, tktypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		tktypes.RandHex(32))))
	assert.NoError(t, err)
	s1.Data = tktypes.RawJSON(`! wrong `)

	dc.flushing = &writeOperation{
		states: []*StateWithLabels{s1},
	}

	_, err = dc.mergeInMemoryMatches(schema, []*State{
		s1.State,
	}, []*StateWithLabels{}, nil)
	assert.Regexp(t, "PD010116", err)

}

func TestDSIMergedInMemoryMatchesSortFail(t *testing.T) {

	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	assert.NoError(t, err)

	dc := ss.getDomainContext("domain1")

	s1, err := schema.ProcessState(ctx, tktypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		tktypes.RandHex(32))))
	assert.NoError(t, err)

	dc.flushing = &writeOperation{
		states: []*StateWithLabels{s1},
	}

	_, err = dc.mergeInMemoryMatches(schema, []*State{
		s1.State,
	}, []*StateWithLabels{}, toQuery(t,
		`{"sort": ["wrong"]}`,
	))
	assert.Regexp(t, "PD010700", err)
}

func TestDSIFindBadQueryAndInsert(t *testing.T) {

	_, ss, done := newDBTestStateStore(t)
	defer done()

	var schemas []Schema
	var schemaID string
	err := ss.RunInDomainContextFlush("domain1", func(ctx context.Context, dsi DomainStateInterface) (err error) {

		schemas, err = dsi.EnsureABISchemas([]*abi.Parameter{testABIParam(t, fakeCoinABI)})
		assert.NoError(t, err)
		schemaID = schemas[0].IDString()
		assert.Equal(t, "type=FakeCoin(bytes32 salt,address owner,uint256 amount),labels=[owner,amount]", schemas[0].Signature())
		return nil
	})
	assert.NoError(t, err)

	err = ss.RunInDomainContextFlush("domain1", func(ctx context.Context, dsi DomainStateInterface) error {
		_, err = dsi.FindAvailableStates(schemaID, toQuery(t,
			`{"sort":["wrong"]}`))
		assert.Regexp(t, "PD010700", err)

		_, err = dsi.UpsertStates(nil, []*StateUpsert{
			{SchemaID: schemaID, Data: tktypes.RawJSON(`"wrong"`)},
		})
		assert.Regexp(t, "FF22038", err)

		return nil
	})
	assert.NoError(t, err)

}

func TestDSIBadIDs(t *testing.T) {

	_, ss, _, done := newDBMockStateStore(t)
	defer done()

	_ = ss.RunInDomainContext("domain1", func(ctx context.Context, dsi DomainStateInterface) error {

		_, err := dsi.UpsertStates(nil, []*StateUpsert{
			{SchemaID: "wrong"},
		})
		assert.Regexp(t, "PD020007", err)

		err = dsi.MarkStatesRead(uuid.New(), []string{"wrong"})
		assert.Regexp(t, "PD020007", err)

		err = dsi.MarkStatesSpending(uuid.New(), []string{"wrong"})
		assert.Regexp(t, "PD020007", err)

		return nil
	})

}

func TestDSIResetWithMixed(t *testing.T) {

	_, ss, _, done := newDBMockStateStore(t)
	defer done()

	dc := ss.getDomainContext("domain1")

	state1 := tktypes.Bytes32Keccak(([]byte)("state1"))
	transactionID1 := uuid.New()
	err := dc.MarkStatesRead(transactionID1, []string{state1.String()})
	assert.NoError(t, err)

	state2 := tktypes.Bytes32Keccak(([]byte)("state2"))
	transactionID2 := uuid.New()
	err = dc.MarkStatesSpending(transactionID2, []string{state2.String()})
	assert.NoError(t, err)

	err = dc.ResetTransaction(transactionID1)
	assert.NoError(t, err)

	assert.Len(t, dc.unFlushed.stateLocks, 1)
	assert.Equal(t, dc.unFlushed.stateLocks[0].State, state2)

}
