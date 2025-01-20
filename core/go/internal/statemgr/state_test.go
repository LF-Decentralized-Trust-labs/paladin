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
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPersistStateMissingSchema(t *testing.T) {
	ctx, ss, db, m, done := newDBMockStateManager(t)
	defer done()

	_ = mockDomain(t, m, "domain1", false)

	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))
	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))

	upserts := []*components.StateUpsertOutsideContext{
		{
			ContractAddress: *tktypes.RandAddress(),
			SchemaID:        tktypes.Bytes32Keccak(([]byte)("test")),
		},
	}

	_, _, err := ss.WritePreVerifiedStates(ctx, ss.p.DB(), "domain1", upserts)
	assert.Regexp(t, "PD010106", err)

	_, _, err = ss.WriteReceivedStates(ctx, ss.p.DB(), "domain1", upserts)
	assert.Regexp(t, "PD010106", err)
}

func TestPersistStateInvalidState(t *testing.T) {
	ctx, ss, _, m, done := newDBMockStateManager(t)
	defer done()

	_ = mockDomain(t, m, "domain1", false)

	schemaID := tktypes.Bytes32Keccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaID)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		definition: &abi.Parameter{},
	})

	upserts := []*components.StateUpsertOutsideContext{
		{
			ContractAddress: *tktypes.RandAddress(),
			SchemaID:        schemaID,
		},
	}

	_, _, err := ss.WritePreVerifiedStates(ctx, ss.p.DB(), "domain1", upserts)
	assert.Regexp(t, "PD010116", err)

	_, _, err = ss.WriteReceivedStates(ctx, ss.p.DB(), "domain1", upserts)
	assert.Regexp(t, "PD010116", err)
}

func TestGetStateMissing(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))

	contractAddress := tktypes.RandAddress()
	_, err := ss.GetState(ctx, ss.p.DB(), "domain1", *contractAddress, tktypes.Bytes32Keccak(([]byte)("state1")).Bytes(), true, false)
	assert.Regexp(t, "PD010112", err)
}

func TestFindStatesMissingSchema(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))

	contractAddress := tktypes.RandAddress()
	_, err := ss.FindContractStates(ctx, ss.p.DB(), "domain1", *contractAddress, tktypes.Bytes32Keccak(([]byte)("schema1")), &query.QueryJSON{}, "all")
	assert.Regexp(t, "PD010106", err)
}

func TestFindStatesBadQuery(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schemaID := tktypes.Bytes32Keccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaID)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		definition: &abi.Parameter{},
	})

	contractAddress := tktypes.RandAddress()
	_, err := ss.FindContractStates(ctx, ss.p.DB(), "domain1", *contractAddress, schemaID, &query.QueryJSON{
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

	schemaID := tktypes.Bytes32Keccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaID)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		Schema:     &pldapi.Schema{ID: schemaID},
		definition: &abi.Parameter{},
	})

	db.ExpectQuery("SELECT.*created").WillReturnError(fmt.Errorf("pop"))

	contractAddress := tktypes.RandAddress()
	_, err := ss.FindContractStates(ctx, ss.p.DB(), "domain1", *contractAddress, schemaID, &query.QueryJSON{
		Statements: query.Statements{
			Ops: query.Ops{
				GreaterThan: []*query.OpSingleVal{
					{Op: query.Op{
						Field: ".created",
					}, Value: tktypes.RawJSON(fmt.Sprintf("%d", time.Now().UnixNano()))},
				},
			},
		},
	}, "all")
	assert.Regexp(t, "pop", err)

}

func TestFindStatesUnknownContext(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schemaID := tktypes.Bytes32Keccak(([]byte)("schema1"))
	contractAddress := tktypes.RandAddress()
	_, err := ss.FindContractStates(ctx, ss.p.DB(), "domain1", *contractAddress, schemaID, &query.QueryJSON{
		Statements: query.Statements{
			Ops: query.Ops{
				GreaterThan: []*query.OpSingleVal{
					{Op: query.Op{
						Field: ".created",
					}, Value: tktypes.RawJSON(fmt.Sprintf("%d", time.Now().UnixNano()))},
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

	_, _, err := ss.WritePreVerifiedStates(ctx, ss.p.DB(), "domain1", []*components.StateUpsertOutsideContext{})
	assert.Regexp(t, "not found", err)

	_, _, err = ss.WriteReceivedStates(ctx, ss.p.DB(), "domain1", []*components.StateUpsertOutsideContext{})
	assert.Regexp(t, "not found", err)

}

func TestWriteReceivedStatesValidateHashFail(t *testing.T) {
	ctx, ss, _, m, done := newDBMockStateManager(t)
	defer done()

	md := mockDomain(t, m, "domain1", true)
	md.On("ValidateStateHashes", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))

	_, _, err := ss.WriteReceivedStates(ctx, ss.p.DB(), "domain1", []*components.StateUpsertOutsideContext{
		{ID: tktypes.RandBytes(32), SchemaID: tktypes.Bytes32(tktypes.RandBytes(32)),
			Data: tktypes.RawJSON(fmt.Sprintf(
				`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
				tktypes.RandHex(32)))},
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
	stateID1 := tktypes.RandBytes(32)
	md.On("ValidateStateHashes", mock.Anything, mock.Anything).Return([]tktypes.HexBytes{stateID1}, nil)

	_, _, err = ss.WriteReceivedStates(ctx, ss.p.DB(), "domain1", []*components.StateUpsertOutsideContext{
		{SchemaID: schema1.ID(), Data: tktypes.RawJSON(fmt.Sprintf(
			`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
			tktypes.RandHex(32)))},
	})
	assert.Regexp(t, "pop", err)

}

func TestWriteNullifiersForReceivedStatesOkRealDB(t *testing.T) {
	ctx, ss, m, done := newDBTestStateManager(t)
	defer done()

	md := componentmocks.NewDomain(t)
	md.On("Name").Return("domain1")
	m.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(md, nil)

	err := ss.WriteNullifiersForReceivedStates(ctx, ss.p.DB(), "domain1", []*components.NullifierUpsert{
		{
			ID:    tktypes.HexBytes(tktypes.RandHex(32)),
			State: tktypes.HexBytes(tktypes.RandHex(32)),
		},
		{
			ID:    tktypes.HexBytes(tktypes.RandHex(32)),
			State: tktypes.HexBytes(tktypes.RandHex(32)),
		},
	})
	require.NoError(t, err)

}

func TestWriteNullifiersForReceivedStatesBadDomain(t *testing.T) {
	ctx, ss, _, m, done := newDBMockStateManager(t)
	defer done()

	m.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(nil, fmt.Errorf("not found"))

	err := ss.WriteNullifiersForReceivedStates(ctx, ss.p.DB(), "domain1", []*components.NullifierUpsert{
		{
			ID:    tktypes.HexBytes(tktypes.RandHex(32)),
			State: tktypes.HexBytes(tktypes.RandHex(32)),
		},
	})
	assert.Regexp(t, "not found", err)

}

func TestFindNullifiersInContext(t *testing.T) {
	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectQuery("SELECT.*states").WillReturnRows(sqlmock.NewRows([]string{}))

	schemaID := tktypes.Bytes32Keccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaID)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		definition: &abi.Parameter{},
		Schema:     &pldapi.Schema{},
	})

	td := componentmocks.NewDomain(t)
	td.On("Name").Return("domain1")
	td.On("CustomHashFunction").Return(false)

	dCtx := ss.NewDomainContext(ctx, td, *tktypes.RandAddress())
	defer dCtx.Close()

	contractAddress := tktypes.RandAddress()
	results, err := ss.FindContractNullifiers(ctx, ss.p.DB(), "domain1", *contractAddress, schemaID,
		query.NewQueryBuilder().Limit(1).Query(), pldapi.StateStatusQualifier(dCtx.Info().ID.String()))
	require.NoError(t, err)
	require.Empty(t, results)

}

func TestFindNullifiersUnknownContext(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	schemaID := tktypes.Bytes32Keccak(([]byte)("schema1"))
	contractAddress := tktypes.RandAddress()
	_, err := ss.FindContractNullifiers(ctx, ss.p.DB(), "domain1", *contractAddress, schemaID, &query.QueryJSON{
		Statements: query.Statements{
			Ops: query.Ops{
				GreaterThan: []*query.OpSingleVal{
					{Op: query.Op{
						Field: ".created",
					}, Value: tktypes.RawJSON(fmt.Sprintf("%d", time.Now().UnixNano()))},
				},
			},
		},
	}, pldapi.StateStatusQualifier(uuid.NewString()))
	assert.Regexp(t, "PD010123", err)

}
