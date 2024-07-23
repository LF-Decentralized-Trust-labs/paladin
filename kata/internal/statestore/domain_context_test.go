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

		// Check the final state is what we expect
		states, err = dsi.FindAvailableStates(schemaHash, toQuery(t, `{
			"sort": [ "owner", "amount" ]
		}`))
		assert.NoError(t, err)
		assert.Len(t, states, 3)
		assert.Equal(t, int64(50), parseFakeCoin(t, states[0]).Amount.Int64())
		assert.Equal(t, int64(35), parseFakeCoin(t, states[1]).Amount.Int64())
		assert.Equal(t, int64(100), parseFakeCoin(t, states[2]).Amount.Int64())

		return nil
	})
	assert.NoError(t, err)

}
