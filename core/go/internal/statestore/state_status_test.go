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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/core/pkg/types"
	"github.com/stretchr/testify/assert"
)

const widgetABI = `{
	"type": "tuple",
	"internalType": "struct Widget",
	"components": [
		{
			"name": "salt",
			"type": "bytes32"
		},
		{
			"name": "size",
			"type": "int64"
		},
		{
			"name": "color",
			"type": "string",
			"indexed": true
		},
		{
			"name": "price",
			"type": "uint256",
			"indexed": true
		}
	]
}`

func makeWidgets(t *testing.T, ctx context.Context, ss *stateStore, domainID, schemaID string, withoutSalt []string) []*StateWithLabels {
	states := make([]*StateWithLabels, len(withoutSalt))
	for i, w := range withoutSalt {
		var ij map[string]interface{}
		err := json.Unmarshal([]byte(w), &ij)
		assert.NoError(t, err)
		ij["salt"] = types.RandHex(32)
		withSalt, err := json.Marshal(ij)
		assert.NoError(t, err)
		states[i], err = ss.PersistState(ctx, domainID, schemaID, withSalt)
		assert.NoError(t, err)
		fmt.Printf("widget[%d]: %s\n", i, states[i].Data)
	}
	return states
}

func toQuery(t *testing.T, queryString string) *filters.QueryJSON {
	var q filters.QueryJSON
	err := json.Unmarshal([]byte(queryString), &q)
	assert.NoError(t, err)
	return &q
}

func TestStateLockingQuery(t *testing.T) {

	ctx, ss, done := newDBTestStateStore(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, widgetABI))
	assert.NoError(t, err)
	err = ss.PersistSchema(ctx, schema)
	assert.NoError(t, err)
	schemaID := schema.IDString()

	widgets := makeWidgets(t, ctx, ss, "domain1", schemaID, []string{
		`{"size": 11111, "color": "red",  "price": 100}`,
		`{"size": 22222, "color": "red",  "price": 150}`,
		`{"size": 33333, "color": "blue", "price": 199}`,
		`{"size": 44444, "color": "pink", "price": 199}`,
		`{"size": 55555, "color": "blue", "price": 500}`,
	})

	checkQuery := func(query string, status StateStatusQualifier, expected ...int) {
		states, err := ss.FindStates(ctx, "domain1", schemaID, toQuery(t, query), status)
		assert.NoError(t, err)
		assert.Len(t, states, len(expected))
		for _, wIndex := range expected {
			found := false
			for _, state := range states {
				if state.ID == widgets[wIndex].ID {
					assert.False(t, found)
					found = true
					break
				}
			}
			assert.True(t, found, fmt.Sprintf("Widget %d missing", wIndex))
		}
	}

	seqID := uuid.New()
	seqQual := StateStatusQualifier(seqID.String())

	checkQuery(`{}`, StateStatusAll, 0, 1, 2, 3, 4)
	checkQuery(`{}`, StateStatusAvailable)
	checkQuery(`{}`, StateStatusLocked)
	checkQuery(`{}`, StateStatusConfirmed)
	checkQuery(`{}`, StateStatusUnconfirmed, 0, 1, 2, 3, 4)
	checkQuery(`{}`, StateStatusSpent)
	checkQuery(`{}`, seqQual)

	// Mark them all confirmed apart from one
	for i, w := range widgets {
		if i != 3 {
			err = ss.MarkConfirmed(ctx, "domain1", w.ID.String(), uuid.New())
			assert.NoError(t, err)
		}
	}

	checkQuery(`{}`, StateStatusAll, 0, 1, 2, 3, 4)    // unchanged
	checkQuery(`{}`, StateStatusAvailable, 0, 1, 2, 4) // added all but 3
	checkQuery(`{}`, StateStatusLocked)                // unchanged
	checkQuery(`{}`, StateStatusConfirmed, 0, 1, 2, 4) // added all but 3
	checkQuery(`{}`, StateStatusUnconfirmed, 3)        // added 3
	checkQuery(`{}`, StateStatusSpent)                 // unchanged
	checkQuery(`{}`, seqQual)                          // unchanged

	// Mark one spent
	err = ss.MarkSpent(ctx, "domain1", widgets[0].ID.String(), uuid.New())
	assert.NoError(t, err)

	checkQuery(`{}`, StateStatusAll, 0, 1, 2, 3, 4) // unchanged
	checkQuery(`{}`, StateStatusAvailable, 1, 2, 4) // removed 0
	checkQuery(`{}`, StateStatusLocked)             // unchanged
	checkQuery(`{}`, StateStatusConfirmed, 1, 2, 4) // removed 0
	checkQuery(`{}`, StateStatusUnconfirmed, 3)     // unchanged
	checkQuery(`{}`, StateStatusSpent, 0)           // added 0
	checkQuery(`{}`, seqQual)                       // unchanged

	// lock a confirmed one for spending
	err = ss.MarkLocked(ctx, "domain1", widgets[1].ID.String(), seqID, false, true)
	assert.NoError(t, err)

	checkQuery(`{}`, StateStatusAll, 0, 1, 2, 3, 4) // unchanged
	checkQuery(`{}`, StateStatusAvailable, 2, 4)    // removed 1
	checkQuery(`{}`, StateStatusLocked, 1)          // added 1
	checkQuery(`{}`, StateStatusConfirmed, 1, 2, 4) // unchanged
	checkQuery(`{}`, StateStatusUnconfirmed, 3)     // unchanged
	checkQuery(`{}`, StateStatusSpent, 0)           // added 0
	checkQuery(`{}`, seqQual, 1)                    // added 1

	// lock the unconfirmed one for spending
	err = ss.MarkLocked(ctx, "domain1", widgets[3].ID.String(), seqID, false, true)
	assert.NoError(t, err)

	checkQuery(`{}`, StateStatusAll, 0, 1, 2, 3, 4) // unchanged
	checkQuery(`{}`, StateStatusAvailable, 2, 4)    // unchanged
	checkQuery(`{}`, StateStatusLocked, 1, 3)       // added 3
	checkQuery(`{}`, StateStatusConfirmed, 1, 2, 4) // unchanged
	checkQuery(`{}`, StateStatusUnconfirmed, 3)     // unchanged
	checkQuery(`{}`, StateStatusSpent, 0)           // unchanged
	checkQuery(`{}`, seqQual, 1, 3)                 // added 3

	// check a sub-select
	checkQuery(`{"eq":[{"field":"color","value":"pink"}]}`, seqQual, 3)
	checkQuery(`{"eq":[{"field":"color","value":"pink"}]}`, StateStatusAvailable)

	// clear the transaction locks
	err = ss.ResetTransaction(ctx, "domain1", seqID)
	assert.NoError(t, err)

	checkQuery(`{}`, StateStatusAll, 0, 1, 2, 3, 4) // unchanged
	checkQuery(`{}`, StateStatusAvailable, 1, 2, 4) // added 1
	checkQuery(`{}`, StateStatusLocked)             // removed 1, 3
	checkQuery(`{}`, StateStatusConfirmed, 1, 2, 4) // unchanged
	checkQuery(`{}`, StateStatusUnconfirmed, 3)     // unchanged
	checkQuery(`{}`, StateStatusSpent, 0)           // unchanged
	checkQuery(`{}`, seqQual)                       // removed 1, 3
}

func TestStateStatusQualifierJSON(t *testing.T) {
	var q StateStatusQualifier
	err := json.Unmarshal(([]byte)(`"wrong"`), &q)
	assert.Regexp(t, "PD010117", err)

	u := uuid.New().String()
	err = json.Unmarshal(types.JSONString(u), &q)
	assert.NoError(t, err)
	assert.Equal(t, u, (string)(q))
}
