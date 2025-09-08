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
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPersistStateMissingSchema(t *testing.T) {
	ctx, ss, db, m, done := newDBMockStateManager(t)
	defer done()

	_ = mockDomain(t, m, "domain1", false)

	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))
	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))

	upserts := []*components.StateUpsertOutsideContext{
		{
			ContractAddress: pldtypes.RandAddress(),
			SchemaID:        pldtypes.Bytes32Keccak(([]byte)("test")),
		},
	}

	_, err := ss.WritePreVerifiedStates(ctx, ss.p.NOTX(), "domain1", upserts)
	assert.Regexp(t, "PD010106", err)

	_, err = ss.WriteReceivedStates(ctx, ss.p.NOTX(), "domain1", upserts)
	assert.Regexp(t, "PD010106", err)
}

func TestPersistStateInvalidState(t *testing.T) {
	ctx, ss, _, m, done := newDBMockStateManager(t)
	defer done()

	_ = mockDomain(t, m, "domain1", false)

	schemaID := pldtypes.Bytes32Keccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaID)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		definition: &abi.Parameter{},
	})

	upserts := []*components.StateUpsertOutsideContext{
		{
			ContractAddress: pldtypes.RandAddress(),
			SchemaID:        schemaID,
		},
	}

	_, err := ss.WritePreVerifiedStates(ctx, ss.p.NOTX(), "domain1", upserts)
	assert.Regexp(t, "PD010116", err)

	_, err = ss.WriteReceivedStates(ctx, ss.p.NOTX(), "domain1", upserts)
	assert.Regexp(t, "PD010116", err)
}

func TestGetStateMissing(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))

	stateID := pldtypes.Bytes32Keccak(([]byte)("state1")).Bytes()
	_, err := ss.GetStatesByID(ctx, ss.p.NOTX(), "domain1", nil, []pldtypes.HexBytes{stateID}, true, false)
	assert.Regexp(t, "PD010112", err)
}

func TestFindStatesMissingSchema(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))

	contractAddress := pldtypes.RandAddress()
	_, err := ss.FindContractStates(ctx, ss.p.NOTX(), "domain1", contractAddress, pldtypes.Bytes32Keccak(([]byte)("schema1")), &query.QueryJSON{}, "all")
	assert.Regexp(t, "PD010106", err)
}

func TestFindStatesBadQuery(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schemaID := pldtypes.Bytes32Keccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaID)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		definition: &abi.Parameter{},
	})

	contractAddress := pldtypes.RandAddress()
	_, err := ss.FindContractStates(ctx, ss.p.NOTX(), "domain1", contractAddress, schemaID, &query.QueryJSON{
		Statements: query.Statements{
			Ops: query.Ops{
				Equal: []*query.OpSingleVal{
					{Op: query.Op{Field: "wrong"}},
				},
			},
		},
	}, "all")
	assert.Regexp(t, "PD010700.*wrong", err)

}

func TestFindStatesFail(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	schemaID := pldtypes.Bytes32Keccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaID)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		Schema:     &pldapi.Schema{ID: schemaID},
		definition: &abi.Parameter{},
	})

	db.ExpectQuery("SELECT.*created").WillReturnError(fmt.Errorf("pop"))

	contractAddress := pldtypes.RandAddress()
	_, err := ss.FindContractStates(ctx, ss.p.NOTX(), "domain1", contractAddress, schemaID, &query.QueryJSON{
		Statements: query.Statements{
			Ops: query.Ops{
				GreaterThan: []*query.OpSingleVal{
					{Op: query.Op{
						Field: ".created",
					}, Value: pldtypes.RawJSON(fmt.Sprintf("%d", time.Now().UnixNano()))},
				},
			},
		},
	}, "all")
	assert.Regexp(t, "pop", err)

}

func TestFindStatesUnknownContext(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schemaID := pldtypes.Bytes32Keccak(([]byte)("schema1"))
	contractAddress := pldtypes.RandAddress()
	_, err := ss.FindContractStates(ctx, ss.p.NOTX(), "domain1", contractAddress, schemaID, &query.QueryJSON{
		Statements: query.Statements{
			Ops: query.Ops{
				GreaterThan: []*query.OpSingleVal{
					{Op: query.Op{
						Field: ".created",
					}, Value: pldtypes.RawJSON(fmt.Sprintf("%d", time.Now().UnixNano()))},
				},
			},
		},
	}, pldapi.StateStatusQualifier(uuid.NewString()))
	assert.Regexp(t, "PD010123", err)

}

func TestWritePreVerifiedStateInvalidDomain(t *testing.T) {
	ctx, ss, _, m, done := newDBMockStateManager(t)
	defer done()

	m.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(nil, fmt.Errorf("not found"))

	_, err := ss.WritePreVerifiedStates(ctx, ss.p.NOTX(), "domain1", []*components.StateUpsertOutsideContext{})
	assert.Regexp(t, "not found", err)

	_, err = ss.WriteReceivedStates(ctx, ss.p.NOTX(), "domain1", []*components.StateUpsertOutsideContext{})
	assert.Regexp(t, "not found", err)

}

func TestWriteReceivedStatesValidateHashFail(t *testing.T) {
	ctx, ss, _, m, done := newDBMockStateManager(t)
	defer done()

	md := mockDomain(t, m, "domain1", true)
	md.On("ValidateStateHashes", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))

	_, err := ss.WriteReceivedStates(ctx, ss.p.NOTX(), "domain1", []*components.StateUpsertOutsideContext{
		{ID: pldtypes.RandBytes(32), SchemaID: pldtypes.RandBytes32(),
			Data: pldtypes.RawJSON(fmt.Sprintf(
				`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
				pldtypes.RandHex(32)))},
	})
	assert.Regexp(t, "pop", err)

}
func TestWriteReceivedStatesValidateHashOkInsertFail(t *testing.T) {
	ctx, ss, db, m, done := newDBMockStateManager(t)
	defer done()

	db.ExpectExec("INSERT.*states").WillReturnError(fmt.Errorf("pop"))

	schema1, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	ss.abiSchemaCache.Set(schemaCacheKey("domain1", schema1.ID()), schema1)

	md := mockDomain(t, m, "domain1", true)
	stateID1 := pldtypes.RandBytes(32)
	md.On("ValidateStateHashes", mock.Anything, mock.Anything).Return([]pldtypes.HexBytes{stateID1}, nil)

	_, err = ss.WriteReceivedStates(ctx, ss.p.NOTX(), "domain1", []*components.StateUpsertOutsideContext{
		{SchemaID: schema1.ID(), Data: pldtypes.RawJSON(fmt.Sprintf(
			`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
			pldtypes.RandHex(32)))},
	})
	assert.Regexp(t, "pop", err)

}

func TestWriteNullifiersForReceivedStatesOkRealDB(t *testing.T) {
	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	md := componentsmocks.NewDomain(t)
	md.On("Name").Return("domain1")
	m.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(md, nil)

	err := ss.WriteNullifiersForReceivedStates(ctx, ss.p.NOTX(), "domain1", []*components.NullifierUpsert{
		{
			ID:    pldtypes.HexBytes(pldtypes.RandHex(32)),
			State: pldtypes.HexBytes(pldtypes.RandHex(32)),
		},
		{
			ID:    pldtypes.HexBytes(pldtypes.RandHex(32)),
			State: pldtypes.HexBytes(pldtypes.RandHex(32)),
		},
	})
	require.NoError(t, err)

}

func TestWriteNullifiersForReceivedStatesBadDomain(t *testing.T) {
	ctx, ss, _, m, done := newDBMockStateManager(t)
	defer done()

	m.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(nil, fmt.Errorf("not found"))

	err := ss.WriteNullifiersForReceivedStates(ctx, ss.p.NOTX(), "domain1", []*components.NullifierUpsert{
		{
			ID:    pldtypes.HexBytes(pldtypes.RandHex(32)),
			State: pldtypes.HexBytes(pldtypes.RandHex(32)),
		},
	})
	assert.Regexp(t, "not found", err)

}

func TestFindNullifiersInContext(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectQuery("SELECT.*states").WillReturnRows(sqlmock.NewRows([]string{}))

	schemaID := pldtypes.Bytes32Keccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaID)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		definition: &abi.Parameter{},
		Schema:     &pldapi.Schema{},
	})

	td := componentsmocks.NewDomain(t)
	td.On("Name").Return("domain1")
	td.On("CustomHashFunction").Return(false)

	dCtx := ss.NewDomainContext(ctx, td, *pldtypes.RandAddress())
	defer dCtx.Close()

	contractAddress := pldtypes.RandAddress()
	results, err := ss.FindContractNullifiers(ctx, ss.p.NOTX(), "domain1", *contractAddress, schemaID,
		query.NewQueryBuilder().Limit(1).Query(), pldapi.StateStatusQualifier(dCtx.Info().ID.String()))
	require.NoError(t, err)
	require.Empty(t, results)

}

func TestFindNullifiersUnknownContext(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schemaID := pldtypes.Bytes32Keccak(([]byte)("schema1"))
	contractAddress := pldtypes.RandAddress()
	_, err := ss.FindContractNullifiers(ctx, ss.p.NOTX(), "domain1", *contractAddress, schemaID, &query.QueryJSON{
		Statements: query.Statements{
			Ops: query.Ops{
				GreaterThan: []*query.OpSingleVal{
					{Op: query.Op{
						Field: ".created",
					}, Value: pldtypes.RawJSON(fmt.Sprintf("%d", time.Now().UnixNano()))},
				},
			},
		},
	}, pldapi.StateStatusQualifier(uuid.NewString()))
	assert.Regexp(t, "PD010123", err)

}

func TestFindStatesWithAdvancedDBQueryModifier(t *testing.T) {
	ctx, ss, mdb, _, done := newDBMockStateManager(t)
	defer done()

	mockGetSchemaOK(mdb)
	mdb.ExpectQuery(`SELECT.*FROM "states".*LEFT JOIN "another_table".*"j"."state_id" IS NOT NULL`).
		WillReturnError(fmt.Errorf("called"))

	_, err := ss.FindStates(ctx, ss.p.NOTX(), "domain1", pldtypes.RandBytes32(), query.NewQueryBuilder().Query(), &components.StateQueryOptions{
		QueryModifier: func(db persistence.DBTX, query *gorm.DB) *gorm.DB {
			return query.
				Joins(`LEFT JOIN "another_table" AS "j" WHERE "j"."state_id" = "states"."id"`).
				Where(`"j"."state_id" IS NOT NULL`)
		},
	})
	assert.Regexp(t, "called", err)

}

func TestFindStatesWithNilOptions(t *testing.T) {
	ctx, ss, mdb, _, done := newDBMockStateManager(t)
	defer done()

	mockGetSchemaOK(mdb)
	mdb.ExpectQuery(`SELECT.*FROM`).WillReturnError(fmt.Errorf("called"))

	_, err := ss.FindStates(ctx, ss.p.NOTX(), "domain1", pldtypes.RandBytes32(), query.NewQueryBuilder().Query(), nil)
	assert.Regexp(t, "called", err)

}
