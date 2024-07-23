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
	"github.com/kaleido-io/paladin/kata/internal/filters"
	"github.com/kaleido-io/paladin/kata/internal/types"
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

	schemaHashReceiver := make(chan string)

	// Run one handler that ends in a flush, of a schema that won't be available unless we flush
	err := ss.RunInDomainContext("domain1", func(ctx context.Context, dsi DomainStateInterface) error {
		schemas, err := dsi.EnsureABISchemas([]*abi.Parameter{testABIParam(t, fakeCoinABI)})
		assert.NoError(t, err)
		assert.Len(t, schemas, 1)
		schemaHash := schemas[0].Hash.String()
		return dsi.Flush(func(ctx context.Context, dsi DomainStateInterface) error {
			schemaHashReceiver <- schemaHash
			return nil
		})
	})
	assert.NoError(t, err)

	var schemaHash string
	select {
	case schemaHash = <-schemaHashReceiver:
	case <-time.After(5 * time.Second):
		assert.Fail(t, "timed out")
	}

	// Run a 2nd handler that depends on that schema being available
	err = ss.RunInDomainContext("domain1", func(ctx context.Context, dsi DomainStateInterface) error {
		states, err := dsi.WriteNewStates(uuid.New(), schemaHash, []types.RawJSON{
			types.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0x1eDfD974fE6828dE81a1a762df680111870B7cDD", "salt": "%s"}`, types.RandHex(32))),
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

	sequenceIDs := []uuid.UUID{uuid.New()}
	var schemaHash string

	err := ss.RunInDomainContext("domain1", func(ctx context.Context, dsi DomainStateInterface) error {
		// Pop in our widget ABI
		schemas, err := dsi.EnsureABISchemas([]*abi.Parameter{testABIParam(t, fakeCoinABI)})
		assert.NoError(t, err)
		assert.Len(t, schemas, 1)
		schemaHash = schemas[0].Hash.String()

		// Flush as ABI schemas only available after a flush
		err = dsi.UnitTestFlushSync()
		assert.NoError(t, err)

		// Store some states
		tx1states, err := dsi.WriteNewStates(sequenceIDs[0], schemaHash, []types.RawJSON{
			types.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, types.RandHex(32))),
			types.RawJSON(fmt.Sprintf(`{"amount": 10,  "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, types.RandHex(32))),
			types.RawJSON(fmt.Sprintf(`{"amount": 75,  "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, types.RandHex(32))),
		})
		assert.NoError(t, err)
		assert.Len(t, tx1states, 3)

		// Query the states, and notice we find the ones that are still in the process of minting
		// even though they've not yet been written to the DB
		states, err := dsi.FindAvailableStates(schemaHash, toQuery(t, `{
			"sort": [ "amount" ]
		}`))
		assert.NoError(t, err)
		assert.Len(t, states, 3)

		// The values should be sorted according to the requested order
		assert.Equal(t, int64(10), parseFakeCoin(t, states[0]).Amount.Int64())
		assert.Equal(t, int64(75), parseFakeCoin(t, states[1]).Amount.Int64())
		assert.Equal(t, int64(100), parseFakeCoin(t, states[2]).Amount.Int64())
		assert.True(t, states[0].Locked.Minting)                   // should be marked minting
		assert.Equal(t, sequenceIDs[0], states[0].Locked.Sequence) // for the sequence we specified

		// Simulate a transaction where we spend two states, and create 2 new ones
		err = dsi.MarkStatesSpending(sequenceIDs[0], schemaHash, []string{
			states[0].Hash.String(), // 10 +
			states[1].Hash.String(), // 75
		})
		assert.NoError(t, err)
		tx2states, err := dsi.WriteNewStates(sequenceIDs[0], schemaHash, []types.RawJSON{
			types.RawJSON(fmt.Sprintf(`{"amount": 35, "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, types.RandHex(32))),
			types.RawJSON(fmt.Sprintf(`{"amount": 50, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, types.RandHex(32))),
		})
		assert.NoError(t, err)
		assert.Len(t, tx2states, 2)

		// Query the states on the first address
		states, err = dsi.FindAvailableStates(schemaHash, toQuery(t, `{
			"sort": [ "-amount" ],
			"eq": [{"field": "owner", "value": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180"}]
		}`))
		assert.NoError(t, err)
		assert.Len(t, states, 2)
		assert.Equal(t, int64(100), parseFakeCoin(t, states[0]).Amount.Int64())
		assert.Equal(t, int64(35), parseFakeCoin(t, states[1]).Amount.Int64())

		// Query the states on the other address
		states, err = dsi.FindAvailableStates(schemaHash, toQuery(t, `{
					"sort": [ "-amount" ],
					"eq": [{"field": "owner", "value": "0x615dD09124271D8008225054d85Ffe720E7a447A"}]
				}`))
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, int64(50), parseFakeCoin(t, states[0]).Amount.Int64())

		// Flush the states to the database
		err = dsi.UnitTestFlushSync()
		assert.NoError(t, err)

		// Check the DB persisted state is what we expect
		states, err = dsi.FindAvailableStates(schemaHash, toQuery(t, `{
			"sort": [ "owner", "amount" ]
		}`))
		assert.NoError(t, err)
		assert.Len(t, states, 3)
		assert.Equal(t, int64(50), parseFakeCoin(t, states[0]).Amount.Int64())
		assert.Equal(t, int64(35), parseFakeCoin(t, states[1]).Amount.Int64())
		assert.Equal(t, int64(100), parseFakeCoin(t, states[2]).Amount.Int64())

		// Write another transaction that splits a coin to two
		err = dsi.MarkStatesSpending(sequenceIDs[0], schemaHash, []string{
			states[0].Hash.String(), // 50
		})
		assert.NoError(t, err)
		tx3states, err := dsi.WriteNewStates(sequenceIDs[0], schemaHash, []types.RawJSON{
			types.RawJSON(fmt.Sprintf(`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, types.RandHex(32))),
			types.RawJSON(fmt.Sprintf(`{"amount": 30, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, types.RandHex(32))),
		})
		assert.NoError(t, err)
		assert.Len(t, tx3states, 2)

		// Now check that we merge the DB and in-memory state
		states, err = dsi.FindAvailableStates(schemaHash, toQuery(t, `{
					"sort": [ "owner", "amount" ]
				}`))
		assert.NoError(t, err)
		assert.Len(t, states, 4)
		assert.Equal(t, int64(20), parseFakeCoin(t, states[0]).Amount.Int64())
		assert.Equal(t, int64(30), parseFakeCoin(t, states[1]).Amount.Int64())
		assert.Equal(t, int64(35), parseFakeCoin(t, states[2]).Amount.Int64())
		assert.Equal(t, int64(100), parseFakeCoin(t, states[3]).Amount.Int64())

		// Check the limit works too across this
		states, err = dsi.FindAvailableStates(schemaHash, toQuery(t, `{
			"limit": 1,
			"sort": [ "owner", "amount" ]
		}`))
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, int64(20), parseFakeCoin(t, states[0]).Amount.Int64())

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
	assert.Regexp(t, "FF00154", err)

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
		dc.flushed = make(chan error, 1)
		dc.flushed <- fmt.Errorf("pop")
	}

	_ = ss.RunInDomainContext("domain1", func(ctx context.Context, dsi DomainStateInterface) error {

		schemas, err := dsi.EnsureABISchemas([]*abi.Parameter{testABIParam(t, fakeCoinABI)})
		assert.NoError(t, err)
		schemaHash := schemas[0].Hash.String()
		err = dsi.UnitTestFlushSync()
		assert.NoError(t, err)

		dc := dsi.(*domainContext)

		fakeFlushError(dc)
		_, err = dsi.EnsureABISchemas(nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		_, err = dsi.FindAvailableStates("", nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		schema, err := ss.getSchemaByHash(ctx, "domain1", &schemas[0].Hash, true)
		assert.NoError(t, err)
		_, err = dc.mergedUnFlushed(schema, nil, nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		_, err = dsi.WriteNewStates(uuid.New(), schemaHash, nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		err = dsi.MarkStatesRead(uuid.New(), schemaHash, nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		err = dsi.MarkStatesSpending(uuid.New(), schemaHash, nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		err = dsi.ResetSequence(uuid.New())
		assert.Regexp(t, "pop", err)

		return nil

	})

}

func TestDSIMergedUnFlushedWhileFlushing(t *testing.T) {

	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	assert.NoError(t, err)

	dc := ss.getDomainContext("domain1")

	s1, err := schema.ProcessState(ctx, types.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		types.RandHex(32))))
	assert.NoError(t, err)
	s1.Locked = &StateLock{State: s1.Hash, Sequence: uuid.New(), Minting: true}

	dc.flushing = &writeOperation{
		states: []*StateWithLabels{s1},
		stateLocks: []*StateLock{
			s1.Locked,
			{State: *HashIDKeccak(([]byte)("another")), Spending: true},
		},
	}

	spending, err := dc.getUnFlushedSpending()
	assert.NoError(t, err)
	assert.Len(t, spending, 1)

	states, err := dc.mergedUnFlushed(schema, []*State{}, &filters.QueryJSON{})
	assert.NoError(t, err)
	assert.Len(t, states, 1)

}

func TestDSIMergedUnFlushedEvalError(t *testing.T) {

	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	assert.NoError(t, err)

	dc := ss.getDomainContext("domain1")

	s1, err := schema.ProcessState(ctx, types.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		types.RandHex(32))))
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

	s1, err := schema.ProcessState(ctx, types.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		types.RandHex(32))))
	assert.NoError(t, err)
	s1.Data = types.RawJSON(`! wrong `)

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

	s1, err := schema.ProcessState(ctx, types.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		types.RandHex(32))))
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
