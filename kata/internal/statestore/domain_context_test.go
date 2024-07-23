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

func TestStateContextEventStyleMintSpendMint(t *testing.T) {

	_, ss, done := newDBTestStateStore(t)
	defer done()

	allDone := make(chan struct{})

	sequenceIDs := []uuid.UUID{uuid.New()}
	var schemaHash string

	var simulatedEventHandlers []DomainContextFunction
	simulatedEventHandlers = []DomainContextFunction{
		func(ctx context.Context, dsi DomainStateInterface) error {
			// Pop in our widget ABI
			schemas, err := dsi.EnsureABISchemas([]*abi.Parameter{testABIParam(t, fakeCoinABI)})
			assert.NoError(t, err)
			assert.Len(t, schemas, 1)
			schemaHash = schemas[0].Hash.String()

			// Flush as ABI schemas only available after a flush
			return dsi.Flush(simulatedEventHandlers[1])
		},
		func(ctx context.Context, dsi DomainStateInterface) error {
			// Store some states
			states, err := dsi.WriteNewStates(sequenceIDs[0], schemaHash, []types.RawJSON{
				types.RawJSON(fmt.Sprintf(`{"amount": 100, "salt": "%s"}`, types.RandHex(32))),
				types.RawJSON(fmt.Sprintf(`{"amount": 10,  "salt": "%s"}`, types.RandHex(32))),
				types.RawJSON(fmt.Sprintf(`{"amount": 75,  "salt": "%s"}`, types.RandHex(32))),
			})
			assert.NoError(t, err)
			assert.Len(t, states, 3)

			// Do NOT flush in this case, move onto to next simulated step
			go ss.RunInDomainContext("domain1", simulatedEventHandlers[2])
			return nil
		},
		func(ctx context.Context, dsi DomainStateInterface) error {
			// Query the states, and notice we find the ones that are still in the process of minting
			// even though they've not yet been written to the DB
			states, err := dsi.FindAvailableStates(schemaHash, toQuery(t, `{
				"sort": [ "amount" ]
			}`))
			assert.NoError(t, err)
			assert.Len(t, states, 3)
			// TODO: SORTING
			assert.Equal(t, int64(100), parseFakeCoin(t, states[0]).Amount.Int64())
			assert.Equal(t, int64(10), parseFakeCoin(t, states[1]).Amount.Int64())
			assert.Equal(t, int64(75), parseFakeCoin(t, states[2]).Amount.Int64())

			close(allDone)
			return nil
		},
	}

	err := ss.RunInDomainContext("domain1", simulatedEventHandlers[0])
	assert.NoError(t, err)

	select {
	case <-allDone:
	case <-time.After(5 * time.Second):
		assert.Fail(t, "timed out")
	}

}
