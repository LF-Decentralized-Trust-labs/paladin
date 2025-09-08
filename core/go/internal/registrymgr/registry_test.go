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

package registrymgr

import (
	"context"
	"crypto/rand"
	"database/sql/driver"
	"fmt"
	"math/big"
	"sync/atomic"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/blockindexer"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var registryID uuid.UUID

type testPlugin struct {
	plugintk.RegistryAPIBase
	initialized  atomic.Bool
	r            *registry
	sendMessages chan *prototk.SendMessageRequest
}

func (tp *testPlugin) Initialized() {
	tp.initialized.Store(true)
}

func newTestPlugin(registryFuncs *plugintk.RegistryAPIFunctions) *testPlugin {
	return &testPlugin{
		RegistryAPIBase: plugintk.RegistryAPIBase{
			Functions: registryFuncs,
		},
		sendMessages: make(chan *prototk.SendMessageRequest, 1),
	}
}

func newTestRegistry(t *testing.T, realDB bool, extraSetup ...func(mc *mockComponents, conf *pldconf.RegistryManagerConfig, regConf *prototk.RegistryConfig)) (context.Context, *registryManager, *testPlugin, *mockComponents, func()) {
	conf := &pldconf.RegistryManagerConfig{
		Registries: map[string]*pldconf.RegistryConfig{
			"test1": {
				Config: map[string]any{"some": "conf"},
			},
		},
	}
	regConf := &prototk.RegistryConfig{}

	ctx, rm, mc, done := newTestRegistryManager(t, realDB, conf, func(mc *mockComponents) {
		for _, fn := range extraSetup {
			fn(mc, conf, regConf)
		}
	})

	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.RegistryAPIFunctions{
		ConfigureRegistry: func(ctx context.Context, ctr *prototk.ConfigureRegistryRequest) (*prototk.ConfigureRegistryResponse, error) {
			assert.Equal(t, "test1", ctr.Name)
			assert.JSONEq(t, `{"some":"conf"}`, ctr.ConfigJson)
			return &prototk.ConfigureRegistryResponse{
				RegistryConfig: regConf,
			}, nil
		},
	}

	registerTestRegistry(t, rm, tp)
	return ctx, rm, tp, mc, done
}

func registerTestRegistry(t *testing.T, rm *registryManager, tp *testPlugin) {
	registryID = uuid.New()
	_, err := rm.RegistryRegistered("test1", registryID, tp)
	require.NoError(t, err)

	ra := rm.registriesByName["test1"]
	assert.NotNil(t, ra)
	tp.r = ra
	tp.r.initRetry.UTSetMaxAttempts(1)
	<-tp.r.initDone
}

func TestDoubleRegisterReplaces(t *testing.T) {

	_, rm, tp0, _, done := newTestRegistry(t, false)
	defer done()
	assert.Nil(t, tp0.r.initError.Load())
	assert.True(t, tp0.initialized.Load())

	// Register again
	tp1 := newTestPlugin(nil)
	tp1.Functions = tp0.Functions
	registerTestRegistry(t, rm, tp1)
	assert.Nil(t, tp1.r.initError.Load())
	assert.True(t, tp1.initialized.Load())

	// Check we get the second from all the maps
	byName := rm.registriesByName[tp1.r.name]
	assert.Same(t, tp1.r, byName)
	byUUID := rm.registriesByID[tp1.r.id]
	assert.Same(t, tp1.r, byUUID)

}

func randID() string { return pldtypes.RandHex(32) }

func randInt() int64 {
	i, _ := rand.Int(rand.Reader, big.NewInt(10^9))
	return i.Int64()
}

func randChainInfo() *prototk.OnChainEventLocation {
	return &prototk.OnChainEventLocation{
		TransactionHash: pldtypes.RandHex(32),
		BlockNumber:     randInt(), TransactionIndex: randInt(), LogIndex: randInt(),
	}
}

func randPropFor(id string) *prototk.RegistryProperty {
	return &prototk.RegistryProperty{
		EntryId:  id,
		Name:     fmt.Sprintf("prop_%s", pldtypes.RandHex(5)),
		Value:    fmt.Sprintf("val_%s", pldtypes.RandHex(5)),
		Active:   true,
		Location: randChainInfo(),
	}
}

func newPropFor(id, name, value string) *prototk.RegistryProperty {
	return &prototk.RegistryProperty{
		EntryId:  id,
		Name:     name,
		Value:    value,
		Active:   true,
		Location: randChainInfo(),
	}
}

func newSystemPropFor(id, name, value string) *prototk.RegistryProperty {
	prop := newPropFor(id, name, value)
	prop.PluginReserved = true
	return prop
}

func TestUpsertRegistryRecordsRealDBok(t *testing.T) {
	ctx, rm, tp, _, done := newTestRegistry(t, true)
	defer done()

	r, err := rm.GetRegistry(ctx, "test1")
	require.NoError(t, err)

	// Insert a root entry
	rootEntry1 := &prototk.RegistryEntry{Id: randID(), Name: "entry1", Location: randChainInfo(), Active: true}
	rootEntry1SysProp := newSystemPropFor(rootEntry1.Id, "$owner", pldtypes.RandAddress().String())
	rootEntry1Props1 := randPropFor(rootEntry1.Id)
	rootEntry2 := &prototk.RegistryEntry{Id: randID(), Name: "entry2", Location: randChainInfo(), Active: true}
	rootEntry2Props1 := randPropFor(rootEntry2.Id)
	rootEntry2Props2 := randPropFor(rootEntry2.Id)
	upsert1 := &prototk.UpsertRegistryRecordsRequest{
		Entries:    []*prototk.RegistryEntry{rootEntry1, rootEntry2},
		Properties: []*prototk.RegistryProperty{rootEntry1Props1, rootEntry2Props1, rootEntry2Props2, rootEntry1SysProp},
	}

	// Upsert first entry
	res, err := tp.r.UpsertRegistryRecords(ctx, upsert1)
	require.NoError(t, err)
	assert.NotNil(t, res)

	// Test getting all the entries with props
	entries, err := r.QueryEntriesWithProps(ctx, rm.p.NOTX(), "active", query.NewQueryBuilder().Limit(100).Query())
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, rootEntry1.Id, entries[0].ID.HexString())
	require.Len(t, entries[0].Properties, 2)
	require.Equal(t, rootEntry1SysProp.Value, entries[0].Properties[rootEntry1SysProp.Name])
	require.Equal(t, rootEntry1Props1.Value, entries[0].Properties[rootEntry1Props1.Name])
	assert.Equal(t, rootEntry2.Id, entries[1].ID.HexString())
	require.Len(t, entries[1].Properties, 2)
	require.Equal(t, rootEntry2Props1.Value, entries[1].Properties[rootEntry2Props1.Name])
	require.Equal(t, rootEntry2Props2.Value, entries[1].Properties[rootEntry2Props2.Name])

	// Test on a non-null field
	entries, err = r.QueryEntriesWithProps(ctx, rm.p.NOTX(), "active",
		query.NewQueryBuilder().NotNull(rootEntry2Props2.Name).Limit(100).Query(),
	)
	require.NoError(t, err)
	assert.Equal(t, rootEntry2.Id, entries[0].ID.HexString())
	require.Len(t, entries, 1)
	require.Len(t, entries[0].Properties, 2)
	require.Equal(t, rootEntry2Props1.Value, entries[0].Properties[rootEntry2Props1.Name])
	require.Equal(t, rootEntry2Props2.Value, entries[0].Properties[rootEntry2Props2.Name])

	// Test on an equal field
	entries, err = r.QueryEntriesWithProps(ctx, rm.p.NOTX(), "active",
		query.NewQueryBuilder().Equal(rootEntry1Props1.Name, rootEntry1Props1.Value).Limit(100).Query(),
	)
	require.NoError(t, err)
	assert.Equal(t, rootEntry1.Id, entries[0].ID.HexString())
	require.Len(t, entries, 1)
	require.Len(t, entries[0].Properties, 2)
	require.Equal(t, rootEntry1SysProp.Value, entries[0].Properties[rootEntry1SysProp.Name])
	require.Equal(t, rootEntry1Props1.Value, entries[0].Properties[rootEntry1Props1.Name])

	// Search on the system prop
	entries, err = r.QueryEntriesWithProps(ctx, rm.p.NOTX(), "active",
		query.NewQueryBuilder().Equal("$owner", rootEntry1SysProp.Value).Limit(100).Query(),
	)
	require.NoError(t, err)
	assert.Equal(t, rootEntry1.Id, entries[0].ID.HexString())

	// Add a child entry - checking it's allowed to have the same name without replacing
	root1ChildEntry1 := &prototk.RegistryEntry{Id: randID(), Name: "entry1", ParentId: rootEntry1.Id, Location: randChainInfo(), Active: true}
	res, err = tp.r.UpsertRegistryRecords(ctx, &prototk.UpsertRegistryRecordsRequest{
		Entries: []*prototk.RegistryEntry{root1ChildEntry1},
	})
	require.NoError(t, err)
	assert.NotNil(t, res)

	// Find children and check sorting fields
	children, err := r.QueryEntries(ctx, rm.p.NOTX(), "active", query.NewQueryBuilder().Equal(
		".parentId", rootEntry1.Id,
	).Sort("-.created", "-.updated").Limit(100).Query())
	require.NoError(t, err)
	require.Len(t, children, 1)
	require.Equal(t, root1ChildEntry1.Id, children[0].ID.HexString())

	// Make an entry inactive - this does NOT affect child entries (responsibility
	// is on the registry plugin to do this if it wishes).
	rootEntry2.Active = false                      // make entry inactive
	rootEntry2Props2.Active = false                // make one prop inactive
	rootEntry2Props3 := randPropFor(rootEntry2.Id) // add prop as active
	res, err = tp.r.UpsertRegistryRecords(ctx, &prototk.UpsertRegistryRecordsRequest{
		Entries:    []*prototk.RegistryEntry{rootEntry2},
		Properties: []*prototk.RegistryProperty{rootEntry2Props2, rootEntry2Props3},
	})
	require.NoError(t, err)
	assert.NotNil(t, res)

	// Check not returned from normal query
	entries, err = r.QueryEntriesWithProps(ctx, rm.p.NOTX(), "active",
		query.NewQueryBuilder().Null(".parentId").Limit(100).Query())
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, rootEntry1.Id, entries[0].ID.HexString())

	// Check returned from cherry pick with any
	entries, err = r.QueryEntriesWithProps(ctx, rm.p.NOTX(), "any",
		query.NewQueryBuilder().Equal(".name", rootEntry2.Name).Limit(100).Query())
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, rootEntry2.Id, entries[0].ID.HexString())
	assert.False(t, entries[0].Active)
	// ... but here the props are the active props still (prop 2 excluded)
	require.Len(t, entries[0].Properties, 2)
	require.Equal(t, rootEntry2Props1.Value, entries[0].Properties[rootEntry2Props1.Name])
	require.Equal(t, rootEntry2Props3.Value, entries[0].Properties[rootEntry2Props3.Name])

	// Check returned from cherry pick with inactive
	entries, err = r.QueryEntriesWithProps(ctx, rm.p.NOTX(), "inactive",
		query.NewQueryBuilder().Equal(".id", rootEntry2.Id).Limit(100).Query())
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, rootEntry2.Id, entries[0].ID.HexString())

	// Can get the complete prop set
	allProps, err := r.GetEntryProperties(ctx, rm.p.NOTX(), "any", pldtypes.MustParseHexBytes(rootEntry2.Id))
	require.NoError(t, err)
	propsMap := filteredPropsMap(allProps, pldtypes.MustParseHexBytes(rootEntry2.Id))
	require.Len(t, propsMap, 3)
	require.Equal(t, rootEntry2Props1.Value, propsMap[rootEntry2Props1.Name])
	require.Equal(t, rootEntry2Props2.Value, propsMap[rootEntry2Props2.Name])
	require.Equal(t, rootEntry2Props3.Value, propsMap[rootEntry2Props3.Name])

	// Can get just the inactive props set
	allProps, err = r.GetEntryProperties(ctx, rm.p.NOTX(), "inactive", pldtypes.MustParseHexBytes(rootEntry2.Id))
	require.NoError(t, err)
	propsMap = filteredPropsMap(allProps, pldtypes.MustParseHexBytes(rootEntry2.Id))
	require.Len(t, propsMap, 1)
	require.Equal(t, rootEntry2Props2.Value, propsMap[rootEntry2Props2.Name])
}

func TestUpsertRegistryRecordsRealDBNameIsUniqueScopedToParentId(t *testing.T) {
	ctx, _, tp, _, done := newTestRegistry(t, true)
	defer done()

	// Insert a root entry
	rootId1 := randID()
	rootId2 := randID()
	parentId := randID()
	parent1 := &prototk.RegistryEntry{Id: parentId, Name: "parent1", Location: randChainInfo(), Active: true}
	rootEntry1 := &prototk.RegistryEntry{Id: rootId1, Name: "entry1", Location: randChainInfo(), Active: true, ParentId: parentId}
	rootEntry2 := &prototk.RegistryEntry{Id: rootId2, Name: "entry1", Location: randChainInfo(), Active: true, ParentId: parentId}

	upsert := &prototk.UpsertRegistryRecordsRequest{
		Entries:    []*prototk.RegistryEntry{parent1, rootEntry1, rootEntry2},
		Properties: []*prototk.RegistryProperty{},
	}

	// Upsert first entry
	res, err := tp.r.UpsertRegistryRecords(ctx, upsert)
	require.Error(t, err)
	assert.Nil(t, res)
	require.Error(t, err)
	//Observed error messages:
	//Postgres: "ERROR: duplicate key value violates unique constraint "reg_entries_name" (SQLSTATE 23505)"
	//          "ERROR: insert or update on table \"reg_entries\" violates foreign key constraint \"reg_entries_registry_parent_id_fkey\"
	//SQLite: "UNIQUE constraint failed: index 'reg_entries_name"
	//pass as long as it mentions the table and a constraint
	assert.Regexp(t, ".*constraint.*", err)
	assert.Regexp(t, ".*reg_entries.*", err)
}

func TestUpsertRegistryRecordsRealDBSameNameAllowedForDifferentParents(t *testing.T) {
	ctx, _, tp, _, done := newTestRegistry(t, true)
	defer done()

	// r, err := rm.GetRegistry(ctx, "test1")
	// require.NoError(t, err)

	// Insert a root entry
	rootId1 := randID()
	rootId2 := randID()
	parentId := randID()
	parentId2 := randID()
	entry1 := &prototk.RegistryEntry{Id: rootId1, Name: "entry1", Location: randChainInfo(), Active: true, ParentId: parentId}
	entry2 := &prototk.RegistryEntry{Id: rootId2, Name: "entry1", Location: randChainInfo(), Active: true, ParentId: parentId2}
	parent1 := &prototk.RegistryEntry{Id: parentId, Name: "parent1", Location: randChainInfo(), Active: true}
	parent2 := &prototk.RegistryEntry{Id: parentId2, Name: "parent2", Location: randChainInfo(), Active: true}

	upsert := &prototk.UpsertRegistryRecordsRequest{
		Entries:    []*prototk.RegistryEntry{parent1, parent2, entry1, entry2},
		Properties: []*prototk.RegistryProperty{},
	}

	// Upsert first entry
	_, err := tp.r.UpsertRegistryRecords(ctx, upsert)
	require.NoError(t, err)

}

func TestUpsertRegistryRecordsRealDBNameSameParentDifferentNameAllowed(t *testing.T) {
	ctx, _, tp, _, done := newTestRegistry(t, true)
	defer done()

	// Insert a root entry
	rootId1 := randID()
	rootId2 := randID()
	parentId := randID()
	parent1 := &prototk.RegistryEntry{Id: parentId, Name: "parent1", Location: randChainInfo(), Active: true}
	rootEntry1 := &prototk.RegistryEntry{Id: rootId1, Name: "entry1", Location: randChainInfo(), Active: true, ParentId: parentId}
	rootEntry2 := &prototk.RegistryEntry{Id: rootId2, Name: "entry2", Location: randChainInfo(), Active: true, ParentId: parentId}
	upsert1 := &prototk.UpsertRegistryRecordsRequest{
		Entries:    []*prototk.RegistryEntry{parent1, rootEntry1, rootEntry2},
		Properties: []*prototk.RegistryProperty{},
	}

	// Upsert first entry
	_, err := tp.r.UpsertRegistryRecords(ctx, upsert1)
	require.NoError(t, err)

}

func TestUpsertRegistryRecordsRealDBpreventsTwoRootEntries(t *testing.T) {
	ctx, _, tp, _, done := newTestRegistry(t, true)
	defer done()

	// Insert a root entry
	rootId1 := randID()
	rootId2 := randID()
	rootEntry1 := &prototk.RegistryEntry{Id: rootId1, Name: "entry1", Location: randChainInfo(), Active: true}
	rootEntry2 := &prototk.RegistryEntry{Id: rootId2, Name: "entry1", Location: randChainInfo(), Active: true}
	upsert1 := &prototk.UpsertRegistryRecordsRequest{
		Entries:    []*prototk.RegistryEntry{rootEntry1, rootEntry2},
		Properties: []*prototk.RegistryProperty{},
	}

	// Upsert first entry
	res, err := tp.r.UpsertRegistryRecords(ctx, upsert1)
	require.Error(t, err)
	assert.Nil(t, res)
	require.Error(t, err)
	//Observed error messages:
	//Postgres: "ERROR: duplicate key value violates unique constraint "reg_entries_name" (SQLSTATE 23505)"
	//SQLite: "UNIQUE constraint failed: index 'reg_entries_name"
	assert.Regexp(t, ".*constraint.*reg_entries_name.*", err)
}

func TestUpsertRegistryRecordsInsertBadID(t *testing.T) {
	ctx, _, tp, m, done := newTestRegistry(t, false)
	defer done()

	m.db.ExpectBegin()

	entry1 := &prototk.RegistryEntry{Id: "not hex", Name: "entry1", Active: true}
	_, err := tp.r.UpsertRegistryRecords(ctx, &prototk.UpsertRegistryRecordsRequest{
		Entries: []*prototk.RegistryEntry{entry1},
	})
	assert.Regexp(t, "PD012103.*not hex", err)
}

func TestUpsertRegistryRecordsInsertBadParentID(t *testing.T) {
	ctx, _, tp, m, done := newTestRegistry(t, false)
	defer done()

	m.db.ExpectBegin()

	entry1 := &prototk.RegistryEntry{Id: randID(), ParentId: "not hex", Name: "entry1", Active: true}
	_, err := tp.r.UpsertRegistryRecords(ctx, &prototk.UpsertRegistryRecordsRequest{
		Entries: []*prototk.RegistryEntry{entry1},
	})
	assert.Regexp(t, "PD012106.*not hex", err)
}

func TestUpsertRegistryRecordsInsertBadName(t *testing.T) {
	ctx, _, tp, m, done := newTestRegistry(t, false)
	defer done()

	m.db.ExpectBegin()

	entry1 := &prototk.RegistryEntry{Id: randID(), Name: "not valid", Active: true}
	_, err := tp.r.UpsertRegistryRecords(ctx, &prototk.UpsertRegistryRecordsRequest{
		Entries: []*prototk.RegistryEntry{entry1},
	})
	assert.Regexp(t, "PD012104.*not valid", err)
}

func TestUpsertRegistryRecordsInsertPropBadEntryID(t *testing.T) {
	ctx, _, tp, m, done := newTestRegistry(t, false)
	defer done()

	m.db.ExpectBegin()

	_, err := tp.r.UpsertRegistryRecords(ctx, &prototk.UpsertRegistryRecordsRequest{
		Properties: []*prototk.RegistryProperty{
			{EntryId: "not valid"},
		},
	})
	assert.Regexp(t, "PD012103.*not valid", err)
}

func TestUpsertRegistryRecordsInsertPropBadName(t *testing.T) {
	ctx, _, tp, m, done := newTestRegistry(t, false)
	defer done()

	m.db.ExpectBegin()

	_, err := tp.r.UpsertRegistryRecords(ctx, &prototk.UpsertRegistryRecordsRequest{
		Properties: []*prototk.RegistryProperty{
			{EntryId: randID(), Name: "not valid"},
		},
	})
	assert.Regexp(t, "PD012105.*not valid", err)
}

func TestUpsertRegistryRecordsInsertPropBadPrefixNonReserved(t *testing.T) {
	ctx, _, tp, m, done := newTestRegistry(t, false)
	defer done()

	m.db.ExpectBegin()

	_, err := tp.r.UpsertRegistryRecords(ctx, &prototk.UpsertRegistryRecordsRequest{
		Properties: []*prototk.RegistryProperty{
			{EntryId: randID(), Name: "$anything", PluginReserved: false},
		},
	})
	assert.Regexp(t, "PD012109.*\\$anything", err)
}

func TestUpsertRegistryRecordsInsertEntryFail(t *testing.T) {
	ctx, _, tp, m, done := newTestRegistry(t, false)
	defer done()

	m.db.ExpectBegin()
	m.db.ExpectExec("INSERT.*reg_entries").WillReturnError(fmt.Errorf("pop"))

	entry1 := &prototk.RegistryEntry{Id: randID(), Name: "entry1", Active: true}
	_, err := tp.r.UpsertRegistryRecords(ctx, &prototk.UpsertRegistryRecordsRequest{
		Entries: []*prototk.RegistryEntry{entry1},
	})
	assert.Regexp(t, "pop", err)
}

func TestUpsertRegistryRecordsInsertPropFail(t *testing.T) {
	ctx, _, tp, m, done := newTestRegistry(t, false)
	defer done()

	m.db.ExpectBegin()
	m.db.ExpectExec("INSERT.*reg_props").WillReturnError(fmt.Errorf("pop"))

	_, err := tp.r.UpsertRegistryRecords(ctx, &prototk.UpsertRegistryRecordsRequest{
		Properties: []*prototk.RegistryProperty{randPropFor(randID())},
	})
	assert.Regexp(t, "pop", err)
}

func TestQueryEntriesQueryNoLimit(t *testing.T) {
	ctx, _, tp, _, done := newTestRegistry(t, false)
	defer done()

	_, err := tp.r.QueryEntriesWithProps(ctx, tp.r.rm.p.NOTX(), "active", query.NewQueryBuilder().Query())
	assert.Regexp(t, "PD012107", err)
}

func TestQueryEntriesQueryFail(t *testing.T) {
	ctx, _, tp, m, done := newTestRegistry(t, false)
	defer done()

	m.db.ExpectQuery("SELECT.*reg_entries").WillReturnError(fmt.Errorf("pop"))

	_, err := tp.r.QueryEntries(ctx, tp.r.rm.p.NOTX(), "active", query.NewQueryBuilder().Limit(100).Query())
	assert.Regexp(t, "pop", err)
}

func TestGetEntryPropertiesQueryFail(t *testing.T) {
	ctx, _, tp, m, done := newTestRegistry(t, false)
	defer done()

	m.db.ExpectQuery("SELECT.*reg_entries").WillReturnRows(sqlmock.
		NewRows([]string{"id"}).
		AddRow(pldtypes.HexBytes(pldtypes.RandBytes(32))))
	m.db.ExpectQuery("SELECT.*reg_props").WillReturnError(fmt.Errorf("pop"))

	_, err := tp.r.QueryEntriesWithProps(ctx, tp.r.rm.p.NOTX(), "active", query.NewQueryBuilder().Limit(100).Query())
	assert.Regexp(t, "pop", err)
}

func TestRegistryWithEventStreams(t *testing.T) {
	es := &blockindexer.EventStream{ID: uuid.New()}

	_, _, tp, _, done := newTestRegistry(t, false, func(mc *mockComponents, conf *pldconf.RegistryManagerConfig, regConf *prototk.RegistryConfig) {
		a := abi.ABI{
			{
				Type: abi.Event,
				Name: "Registered",
				Inputs: abi.ParameterArray{
					{Name: "node", Type: "string"},
					{Name: "details", Type: "string"},
				},
			},
		}
		addr := pldtypes.RandAddress()

		mc.blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.MatchedBy(func(ies *blockindexer.InternalEventStream) bool {
			require.Len(t, ies.Definition.Sources, 1)
			assert.JSONEq(t, pldtypes.JSONString(a).String(), pldtypes.JSONString(ies.Definition.Sources[0].ABI).String())
			assert.Equal(t, addr, ies.Definition.Sources[0].Address)
			return true
		})).Return(es, nil)

		regConf.EventSources = []*prototk.RegistryEventSource{
			{
				ContractAddress: addr.String(),
				AbiEventsJson:   pldtypes.JSONString(a).Pretty(),
			},
		}
	})
	defer done()

	assert.Equal(t, es, tp.r.eventStream)

}

func TestConfigureEventStreamBadEventABI(t *testing.T) {
	ctx, _, tp, _, done := newTestRegistry(t, false)
	defer done()

	tp.r.config = &prototk.RegistryConfig{
		EventSources: []*prototk.RegistryEventSource{
			{
				AbiEventsJson: `{!!! wrong `,
			},
		},
	}
	err := tp.r.configureEventStream(ctx, tp.r.rm.p.NOTX())
	assert.Regexp(t, "PD012102", err)

}

func TestConfigureEventStreamBadEventContractAddr(t *testing.T) {
	ctx, _, tp, _, done := newTestRegistry(t, false)
	defer done()

	tp.r.config = &prototk.RegistryConfig{
		EventSources: []*prototk.RegistryEventSource{
			{
				ContractAddress: "wrong",
			},
		},
	}
	err := tp.r.configureEventStream(ctx, tp.r.rm.p.NOTX())
	assert.Regexp(t, "PD012102", err)

}

func TestConfigureEventStreamBadEventABITypes(t *testing.T) {
	ctx, _, tp, _, done := newTestRegistry(t, false)
	defer done()

	tp.r.config = &prototk.RegistryConfig{
		EventSources: []*prototk.RegistryEventSource{
			{
				AbiEventsJson: `[{"type":"event","inputs":[{"type":"badness"}]}]`,
			},
		},
	}
	err := tp.r.configureEventStream(ctx, tp.r.rm.p.NOTX())
	assert.Regexp(t, "FF22025", err)

}

func TestHandleEventBatchOk(t *testing.T) {

	ctx, _, tp, _, done := newTestRegistry(t, false, func(mc *mockComponents, conf *pldconf.RegistryManagerConfig, regConf *prototk.RegistryConfig) {
		mc.db.ExpectBegin()
		mc.db.ExpectExec("INSERT.*reg_entries").WillReturnResult(driver.ResultNoRows)
		mc.db.ExpectCommit()
	})
	defer done()

	batch := &blockindexer.EventDeliveryBatch{
		StreamID:   uuid.New(),
		StreamName: "registry_1",
		BatchID:    uuid.New(),
		Events: []*pldapi.EventWithData{
			{
				IndexedEvent: &pldapi.IndexedEvent{
					BlockNumber:      12345,
					TransactionIndex: 10,
					LogIndex:         20,
					TransactionHash:  pldtypes.RandBytes32(),
					Signature:        pldtypes.RandBytes32(),
				},
				SoliditySignature: "event1()",
				Address:           *pldtypes.RandAddress(),
				Data:              []byte("some data"),
			},
		},
	}

	tp.Functions.HandleRegistryEvents = func(ctx context.Context, rebr *prototk.HandleRegistryEventsRequest) (*prototk.HandleRegistryEventsResponse, error) {
		assert.Equal(t, batch.BatchID.String(), rebr.BatchId)
		assert.Equal(t, "event1()", rebr.Events[0].SoliditySignature)
		return &prototk.HandleRegistryEventsResponse{
			Entries: []*prototk.RegistryEntry{
				{
					Id:   randID(),
					Name: "node1",
				},
			},
		}, nil
	}

	err := tp.r.rm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return tp.r.handleEventBatch(ctx, dbTX, batch)
	})
	require.NoError(t, err)

}

func TestHandleEventBatchError(t *testing.T) {

	ctx, _, tp, _, done := newTestRegistry(t, false, func(mc *mockComponents, conf *pldconf.RegistryManagerConfig, regConf *prototk.RegistryConfig) {
		mc.db.ExpectBegin()
	})
	defer done()

	batch := &blockindexer.EventDeliveryBatch{
		BatchID: uuid.New(),
		Events: []*pldapi.EventWithData{{
			IndexedEvent: &pldapi.IndexedEvent{
				BlockNumber:      12345,
				TransactionIndex: 10,
				LogIndex:         20,
				TransactionHash:  pldtypes.RandBytes32(),
				Signature:        pldtypes.RandBytes32(),
			},
			SoliditySignature: "event1()",
			Address:           *pldtypes.RandAddress(),
			Data:              []byte("some data"),
		}},
	}

	tp.Functions.HandleRegistryEvents = func(ctx context.Context, rebr *prototk.HandleRegistryEventsRequest) (*prototk.HandleRegistryEventsResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	err := tp.r.rm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return tp.r.handleEventBatch(ctx, dbTX, batch)
	})
	require.Regexp(t, "pop", err)

}
