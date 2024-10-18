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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func genWidget(t *testing.T, schemaID tktypes.Bytes32, txID *uuid.UUID, withoutSalt string) *components.StateUpsert {
	var ij map[string]interface{}
	err := json.Unmarshal([]byte(withoutSalt), &ij)
	require.NoError(t, err)
	ij["salt"] = tktypes.RandHex(32)
	withSalt, err := json.Marshal(ij)
	require.NoError(t, err)
	return &components.StateUpsert{
		SchemaID:  schemaID,
		Data:      withSalt,
		CreatedBy: txID,
	}
}

func makeWidgets(t *testing.T, ctx context.Context, ss *stateManager, domainName string, contractAddress tktypes.EthAddress, schemaID tktypes.Bytes32, withoutSalt []string) []*components.State {
	states := make([]*components.State, len(withoutSalt))
	for i, w := range withoutSalt {
		withSalt := genWidget(t, schemaID, nil, w)
		newStates, err := ss.WritePreVerifiedStates(ctx, ss.p.DB(), domainName, []*components.StateUpsertOutsideContext{
			{
				ContractAddress: contractAddress,
				SchemaID:        schemaID,
				Data:            withSalt.Data,
			},
		})
		require.NoError(t, err)
		states[i] = newStates[0]
		fmt.Printf("widget[%d]: %s\n", i, states[i].Data)
	}
	return states
}

func syncFlushContext(t *testing.T, dc components.DomainContext) {
	flushed := make(chan error)
	err := dc.InitiateFlush(func(err error) { flushed <- err })
	require.NoError(t, err)
	err = <-flushed
	require.NoError(t, err)
}

func newTestDomainContext(t *testing.T, ctx context.Context, ss *stateManager, name string, customHashFunction bool) (tktypes.EthAddress, *domainContext) {
	md := componentmocks.NewDomain(t)
	md.On("Name").Return(name)
	md.On("CustomHashFunction").Return(customHashFunction)
	contractAddress := tktypes.RandAddress()
	dc := ss.NewDomainContext(ctx, md, *contractAddress, ss.p.DB())
	return *contractAddress, dc.(*domainContext)
}

func TestStateLockingQuery(t *testing.T) {

	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	_ = mockDomain(t, m, "domain1", false)

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, widgetABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.DB(), []*components.SchemaPersisted{schema.SchemaPersisted})
	require.NoError(t, err)
	schemaID := schema.ID()

	contractAddress, dc := newTestDomainContext(t, ctx, ss, "domain1", false)
	defer dc.Close()

	widgets := makeWidgets(t, ctx, ss, "domain1", contractAddress, schemaID, []string{
		`{"size": 11111, "color": "red",  "price": 100}`,
		`{"size": 22222, "color": "red",  "price": 150}`,
		`{"size": 33333, "color": "blue", "price": 199}`,
		`{"size": 44444, "color": "pink", "price": 199}`,
		`{"size": 55555, "color": "blue", "price": 500}`,
	})

	checkQuery := func(jq *query.QueryJSON, status StateStatusQualifier, expected ...int) {
		states, err := ss.FindStates(ctx, ss.p.DB(), "domain1", contractAddress, schemaID, jq, status)
		require.NoError(t, err)
		assert.Len(t, states, len(expected))
		for _, wIndex := range expected {
			found := false
			for _, state := range states {
				if state.ID.Equals(widgets[wIndex].ID) {
					assert.False(t, found)
					found = true
					break
				}
			}
			assert.True(t, found, fmt.Sprintf("Widget %d missing", wIndex))
		}
	}

	seqQual := StateStatusQualifier(dc.Info().ID.String())
	all := query.NewQueryBuilder().Query()

	checkQuery(all, StateStatusAll, 0, 1, 2, 3, 4)
	checkQuery(all, StateStatusAvailable)
	checkQuery(all, StateStatusConfirmed)
	checkQuery(all, StateStatusUnconfirmed, 0, 1, 2, 3, 4)
	checkQuery(all, StateStatusSpent)
	checkQuery(all, seqQual)

	// Mark them all confirmed apart from one
	for i, w := range widgets {
		if i != 3 {
			err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.DB(), []*components.StateSpend{},
				[]*components.StateConfirm{
					{DomainName: "domain1", State: w.ID, Transaction: uuid.New()},
				})
			require.NoError(t, err)
		}
	}

	checkQuery(all, StateStatusAll, 0, 1, 2, 3, 4)    // unchanged
	checkQuery(all, StateStatusAvailable, 0, 1, 2, 4) // added all but 3
	checkQuery(all, StateStatusConfirmed, 0, 1, 2, 4) // added all but 3
	checkQuery(all, StateStatusUnconfirmed, 3)        // added 3
	checkQuery(all, StateStatusSpent)                 // unchanged
	checkQuery(all, seqQual, 0, 1, 2, 4)              // added all but 3

	// Mark one spent
	err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.DB(),
		[]*components.StateSpend{
			{DomainName: "domain1", State: widgets[0].ID, Transaction: uuid.New()},
		}, []*components.StateConfirm{})
	require.NoError(t, err)

	checkQuery(all, StateStatusAll, 0, 1, 2, 3, 4) // unchanged
	checkQuery(all, StateStatusAvailable, 1, 2, 4) // removed 0
	checkQuery(all, StateStatusConfirmed, 1, 2, 4) // removed 0
	checkQuery(all, StateStatusUnconfirmed, 3)     // unchanged
	checkQuery(all, StateStatusSpent, 0)           // added 0
	checkQuery(all, seqQual, 1, 2, 4)              // unchanged

	// add a new state only within the domain context
	txID1 := uuid.New()
	contextStates, err := dc.UpsertStates(
		genWidget(t, schemaID, &txID1, `{"size": 66666, "color": "blue", "price": 600}`))
	require.NoError(t, err)
	widgets = append(widgets, contextStates...)
	syncFlushContext(t, dc)

	checkQuery(all, StateStatusAll, 0, 1, 2, 3, 4, 5) // added 5
	checkQuery(all, StateStatusAvailable, 1, 2, 4)    // unchanged
	checkQuery(all, StateStatusConfirmed, 1, 2, 4)    // unchanged
	checkQuery(all, StateStatusUnconfirmed, 3, 5)     // added 5
	checkQuery(all, StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 4, 5)              // added 5

	// lock the unconfirmed one for spending
	txID2 := uuid.New()
	err = dc.AddStateLocks(&components.StateLock{
		Type:        components.StateLockTypeSpend.Enum(),
		Transaction: txID2,
		State:       widgets[5].ID,
	})
	require.NoError(t, err)

	checkQuery(all, StateStatusAll, 0, 1, 2, 3, 4, 5) // unchanged
	checkQuery(all, StateStatusAvailable, 1, 2, 4)    // unchanged
	checkQuery(all, StateStatusConfirmed, 1, 2, 4)    // unchanged
	checkQuery(all, StateStatusUnconfirmed, 3, 5)     // unchanged
	checkQuery(all, StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 4)                 // removed 5

	// cancel that spend lock
	dc.ResetTransactions(txID2)

	checkQuery(all, StateStatusAll, 0, 1, 2, 3, 4, 5) // unchanged
	checkQuery(all, StateStatusAvailable, 1, 2, 4)    // unchanged
	checkQuery(all, StateStatusConfirmed, 1, 2, 4)    // unchanged
	checkQuery(all, StateStatusUnconfirmed, 3, 5)     // unchanged
	checkQuery(all, StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 4, 5)              // added 5 back

	// Mark that new state confirmed
	err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.DB(),
		[]*components.StateSpend{},
		[]*components.StateConfirm{
			{DomainName: "domain1", State: widgets[5].ID, Transaction: uuid.New()},
		})
	require.NoError(t, err)

	// reset the domain context - does not matter now
	dc.Reset()

	checkQuery(all, StateStatusAll, 0, 1, 2, 3, 4, 5) // unchanged
	checkQuery(all, StateStatusAvailable, 1, 2, 4, 5) // added 5
	checkQuery(all, StateStatusConfirmed, 1, 2, 4, 5) // added 5
	checkQuery(all, StateStatusUnconfirmed, 3)        // removed 5
	checkQuery(all, StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 4, 5)              // unchanged

	// Add 3 only as confirmed by a TX only within the domain context
	// Note we have to re-supply the data here, so that the domain context can
	// have it in memory for queries
	txID13 := uuid.New()
	_, err = dc.UpsertStates(&components.StateUpsert{
		ID:        widgets[3].ID,
		SchemaID:  widgets[3].Schema,
		Data:      widgets[3].Data,
		CreatedBy: &txID13,
	})
	require.NoError(t, err)

	checkQuery(all, StateStatusAll, 0, 1, 2, 3, 4, 5) // unchanged
	checkQuery(all, StateStatusAvailable, 1, 2, 4, 5) // unchanged
	checkQuery(all, StateStatusConfirmed, 1, 2, 4, 5) // unchanged
	checkQuery(all, StateStatusUnconfirmed, 3)        // unchanged
	checkQuery(all, StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 3, 4, 5)           // added 3

	// check a sub-select
	checkQuery(query.NewQueryBuilder().Equal("color", "pink").Query(), seqQual, 3)
	checkQuery(query.NewQueryBuilder().Equal("color", "pink").Query(), StateStatusAvailable)

}

func TestStateStatusQualifierJSON(t *testing.T) {
	var q StateStatusQualifier
	err := json.Unmarshal(([]byte)(`"wrong"`), &q)
	assert.Regexp(t, "PD010117", err)

	u := uuid.New().String()
	err = json.Unmarshal(tktypes.JSONString(u), &q)
	require.NoError(t, err)
	assert.Equal(t, u, (string)(q))
}
