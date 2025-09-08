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

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/google/uuid"
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

func genWidget(t *testing.T, schemaID pldtypes.Bytes32, txID *uuid.UUID, withoutSalt string) *components.StateUpsert {
	var ij map[string]interface{}
	err := json.Unmarshal([]byte(withoutSalt), &ij)
	require.NoError(t, err)
	ij["salt"] = pldtypes.RandHex(32)
	withSalt, err := json.Marshal(ij)
	require.NoError(t, err)
	return &components.StateUpsert{
		Schema:    schemaID,
		Data:      withSalt,
		CreatedBy: txID,
	}
}

func makeWidgets(t *testing.T, ctx context.Context, ss *stateManager, domainName string, contractAddress *pldtypes.EthAddress, schemaID pldtypes.Bytes32, withoutSalt []string) []*pldapi.State {
	states := make([]*pldapi.State, len(withoutSalt))
	for i, w := range withoutSalt {
		withSalt := genWidget(t, schemaID, nil, w)
		var newStates []*pldapi.State
		err := ss.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
			newStates, err = ss.WritePreVerifiedStates(ctx, dbTX, domainName, []*components.StateUpsertOutsideContext{
				{
					ContractAddress: contractAddress,
					SchemaID:        schemaID,
					Data:            withSalt.Data,
				},
			})
			return err
		})
		require.NoError(t, err)
		states[i] = newStates[0]
		fmt.Printf("widget[%d]: %s\n", i, states[i].Data)
	}
	return states
}

func syncFlushContext(t *testing.T, dc components.DomainContext) {
	ss := dc.(*domainContext).ss
	err := ss.p.Transaction(dc.Ctx(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return dc.Flush(dbTX)
	})
	require.NoError(t, err)
}

func newTestDomainContext(t *testing.T, ctx context.Context, ss *stateManager, name string, customHashFunction bool) (*pldtypes.EthAddress, *domainContext) {
	md := componentsmocks.NewDomain(t)
	md.On("Name").Return(name)
	md.On("CustomHashFunction").Return(customHashFunction)
	contractAddress := pldtypes.RandAddress()
	dc := ss.NewDomainContext(ctx, md, *contractAddress)
	return contractAddress, dc.(*domainContext)
}

func TestStateLockingQuery(t *testing.T) {

	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	_ = mockDomain(t, m, "domain1", false)
	mockStateCallback(m)

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, widgetABI))
	require.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema.Schema})
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

	checkQuery := func(jq *query.QueryJSON, status pldapi.StateStatusQualifier, expected ...int) {
		states, err := ss.FindContractStates(ctx, ss.p.NOTX(), "domain1", contractAddress, schemaID, jq, status)
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

	seqQual := pldapi.StateStatusQualifier(dc.Info().ID.String())
	all := query.NewQueryBuilder().Query()

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4)
	checkQuery(all, pldapi.StateStatusAvailable)
	checkQuery(all, pldapi.StateStatusConfirmed)
	checkQuery(all, pldapi.StateStatusUnconfirmed, 0, 1, 2, 3, 4)
	checkQuery(all, pldapi.StateStatusSpent)
	checkQuery(all, seqQual)

	// Mark them all confirmed apart from one
	for i, w := range widgets {
		if i != 3 {
			err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.NOTX(), []*pldapi.StateSpendRecord{}, []*pldapi.StateReadRecord{},
				[]*pldapi.StateConfirmRecord{
					{DomainName: "domain1", State: w.ID, Transaction: uuid.New()},
				}, []*pldapi.StateInfoRecord{})
			require.NoError(t, err)
		}
	}

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusAvailable, 0, 1, 2, 4) // added all but 3
	checkQuery(all, pldapi.StateStatusConfirmed, 0, 1, 2, 4) // added all but 3
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3)        // added 3
	checkQuery(all, pldapi.StateStatusSpent)                 // unchanged
	checkQuery(all, seqQual, 0, 1, 2, 4)                     // added all but 3

	// Mark one spent
	err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.NOTX(),
		[]*pldapi.StateSpendRecord{
			{DomainName: "domain1", State: widgets[0].ID, Transaction: uuid.New()},
		}, []*pldapi.StateReadRecord{}, []*pldapi.StateConfirmRecord{}, []*pldapi.StateInfoRecord{})
	require.NoError(t, err)

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4) // unchanged
	checkQuery(all, pldapi.StateStatusAvailable, 1, 2, 4) // removed 0
	checkQuery(all, pldapi.StateStatusConfirmed, 1, 2, 4) // removed 0
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3)     // unchanged
	checkQuery(all, pldapi.StateStatusSpent, 0)           // added 0
	checkQuery(all, seqQual, 1, 2, 4)                     // unchanged

	// add a new state only within the domain context
	txID1 := uuid.New()
	contextStates, err := dc.UpsertStates(ss.p.NOTX(),
		genWidget(t, schemaID, &txID1, `{"size": 66666, "color": "blue", "price": 600}`))
	require.NoError(t, err)
	widgets = append(widgets, contextStates...)
	syncFlushContext(t, dc)

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4, 5) // added 5
	checkQuery(all, pldapi.StateStatusAvailable, 1, 2, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusConfirmed, 1, 2, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3, 5)     // added 5
	checkQuery(all, pldapi.StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 4, 5)                     // added 5

	// lock the unconfirmed one for spending
	txID2 := uuid.New()
	err = dc.AddStateLocks(&pldapi.StateLock{
		Type:        pldapi.StateLockTypeSpend.Enum(),
		Transaction: txID2,
		StateID:     widgets[5].ID,
	})
	require.NoError(t, err)

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4, 5) // unchanged
	checkQuery(all, pldapi.StateStatusAvailable, 1, 2, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusConfirmed, 1, 2, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3, 5)     // unchanged
	checkQuery(all, pldapi.StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 4)                        // removed 5

	// cancel that spend lock
	dc.ResetTransactions(txID2)

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4, 5) // unchanged
	checkQuery(all, pldapi.StateStatusAvailable, 1, 2, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusConfirmed, 1, 2, 4)    // unchanged
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3, 5)     // unchanged
	checkQuery(all, pldapi.StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 4, 5)                     // added 5 back

	// Mark that new state confirmed
	err = ss.WriteStateFinalizations(ss.bgCtx, ss.p.NOTX(),
		[]*pldapi.StateSpendRecord{},
		[]*pldapi.StateReadRecord{
			{DomainName: "domain1", State: widgets[1].ID, Transaction: uuid.New()}, // this is inert
		},
		[]*pldapi.StateConfirmRecord{
			{DomainName: "domain1", State: widgets[5].ID, Transaction: uuid.New()},
		}, []*pldapi.StateInfoRecord{})
	require.NoError(t, err)

	// reset the domain context - does not matter now
	dc.Reset()

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4, 5) // unchanged
	checkQuery(all, pldapi.StateStatusAvailable, 1, 2, 4, 5) // added 5
	checkQuery(all, pldapi.StateStatusConfirmed, 1, 2, 4, 5) // added 5
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3)        // removed 5
	checkQuery(all, pldapi.StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 4, 5)                     // unchanged

	// Add 3 only as confirmed by a TX only within the domain context
	// Note we have to re-supply the data here, so that the domain context can
	// have it in memory for queries
	txID13 := uuid.New()
	_, err = dc.UpsertStates(ss.p.NOTX(), &components.StateUpsert{
		ID:        widgets[3].ID,
		Schema:    widgets[3].Schema,
		Data:      widgets[3].Data,
		CreatedBy: &txID13,
	})
	require.NoError(t, err)

	checkQuery(all, pldapi.StateStatusAll, 0, 1, 2, 3, 4, 5) // unchanged
	checkQuery(all, pldapi.StateStatusAvailable, 1, 2, 4, 5) // unchanged
	checkQuery(all, pldapi.StateStatusConfirmed, 1, 2, 4, 5) // unchanged
	checkQuery(all, pldapi.StateStatusUnconfirmed, 3)        // unchanged
	checkQuery(all, pldapi.StateStatusSpent, 0)              // unchanged
	checkQuery(all, seqQual, 1, 2, 3, 4, 5)                  // added 3

	// check a sub-select
	checkQuery(query.NewQueryBuilder().Equal("color", "pink").Query(), seqQual, 3)
	checkQuery(query.NewQueryBuilder().Equal("color", "pink").Query(), pldapi.StateStatusAvailable)

}
