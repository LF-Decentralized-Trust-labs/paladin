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
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/kata/internal/filters"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestPersistStateMissingSchema(t *testing.T) {
	ctx, ss, db, done := newDBMockStateStore(t)
	defer done()

	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))

	_, err := ss.PersistState(ctx, "domain1", types.Bytes32Keccak(([]byte)("test")).String(), nil)
	assert.Regexp(t, "PD010106", err)
}

func TestPersistStateInvalidState(t *testing.T) {
	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	schemaID := types.Bytes32Keccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaID)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		definition: &abi.Parameter{},
	})

	_, err := ss.PersistState(ctx, "domain1", schemaID.String(), nil)
	assert.Regexp(t, "PD010116", err)
}

func TestGetStateMissing(t *testing.T) {
	ctx, ss, db, done := newDBMockStateStore(t)
	defer done()

	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))

	_, err := ss.GetState(ctx, "domain1", types.Bytes32Keccak(([]byte)("state1")).String(), true, false)
	assert.Regexp(t, "PD010112", err)
}

func TestGetStateBadID(t *testing.T) {
	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	_, err := ss.GetState(ctx, "domain1", "bad id", true, false)
	assert.Regexp(t, "PD010100", err)
}

func TestMarkConfirmedBadID(t *testing.T) {
	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	err := ss.MarkConfirmed(ctx, "domain1", "bad id", uuid.New())
	assert.Regexp(t, "PD010100", err)
}

func TestMarkSpentBadID(t *testing.T) {
	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	err := ss.MarkSpent(ctx, "domain1", "bad id", uuid.New())
	assert.Regexp(t, "PD010100", err)
}

func TestMarkLockedBadID(t *testing.T) {
	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	err := ss.MarkLocked(ctx, "domain1", "bad id", uuid.New(), false, false)
	assert.Regexp(t, "PD010100", err)
}

func TestFindStatesMissingSchema(t *testing.T) {
	ctx, ss, db, done := newDBMockStateStore(t)
	defer done()

	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))

	_, err := ss.FindStates(ctx, "domain1", types.Bytes32Keccak(([]byte)("schema1")).String(), &filters.QueryJSON{}, "all")
	assert.Regexp(t, "PD010106", err)
}

func TestFindStatesBadQuery(t *testing.T) {
	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	schemaID := types.Bytes32Keccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaID)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		definition: &abi.Parameter{},
	})

	_, err := ss.FindStates(ctx, "domain1", schemaID.String(), &filters.QueryJSON{
		Statements: filters.Statements{
			Ops: filters.Ops{
				Equal: []*filters.OpSingleVal{
					{Op: filters.Op{Field: "wrong"}},
				},
			},
		},
	}, "all")
	assert.Regexp(t, "PD010700.*wrong", err)

}

func TestFindStatesFail(t *testing.T) {
	ctx, ss, db, done := newDBMockStateStore(t)
	defer done()

	schemaID := types.Bytes32Keccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaID)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		SchemaPersisted: &SchemaPersisted{ID: *schemaID},
		definition:      &abi.Parameter{},
	})

	db.ExpectQuery("SELECT.*created_at").WillReturnError(fmt.Errorf("pop"))

	_, err := ss.FindStates(ctx, "domain1", schemaID.String(), &filters.QueryJSON{
		Statements: filters.Statements{
			Ops: filters.Ops{
				GreaterThan: []*filters.OpSingleVal{
					{Op: filters.Op{
						Field: ".created",
					}, Value: types.RawJSON(fmt.Sprintf("%d", time.Now().UnixNano()))},
				},
			},
		},
	}, "all")
	assert.Regexp(t, "pop", err)

}
