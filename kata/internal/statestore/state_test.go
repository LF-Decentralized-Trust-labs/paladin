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

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/kata/internal/filters"
	"github.com/kaleido-io/paladin/kata/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestPersistStateMissingSchema(t *testing.T) {
	ctx, ss, db, done := newDBMockStateStore(t)
	defer done()

	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))

	_, err := ss.PersistState(ctx, "domain1", HashIDKeccak(([]byte)("test")).String(), nil)
	assert.Regexp(t, "PD010106", err)
}

func TestPersistStateInvalidState(t *testing.T) {
	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	schemaHash := HashIDKeccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaHash)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		definition: &abi.Parameter{},
	})

	_, err := ss.PersistState(ctx, "domain1", schemaHash.String(), nil)
	assert.Regexp(t, "FF22025", err)
}

func TestGetStateMissing(t *testing.T) {
	ctx, ss, db, done := newDBMockStateStore(t)
	defer done()

	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))

	_, err := ss.GetState(ctx, "domain1", HashIDKeccak(([]byte)("state1")), true, false)
	assert.Regexp(t, "PD010112", err)
}

func TestFindStatesMissingSchema(t *testing.T) {
	ctx, ss, db, done := newDBMockStateStore(t)
	defer done()

	db.ExpectQuery("SELECT").WillReturnRows(db.NewRows([]string{}))

	_, err := ss.FindStates(ctx, "domain1", HashIDKeccak(([]byte)("schema1")).String(), &filters.QueryJSON{}, "all")
	assert.Regexp(t, "PD010106", err)
}

func TestFindStatesBadQuery(t *testing.T) {
	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	schemaHash := HashIDKeccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaHash)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		definition: &abi.Parameter{},
	})

	_, err := ss.FindStates(ctx, "domain1", schemaHash.String(), &filters.QueryJSON{
		FilterJSON: filters.FilterJSON{
			FilterJSONOps: filters.FilterJSONOps{
				Equal: []*filters.FilterJSONKeyValue{
					{FilterJSONBase: filters.FilterJSONBase{Field: "wrong"}},
				},
			},
		},
	}, "all")
	assert.Regexp(t, "PD010600.*wrong", err)

}

func TestFindStatesFail(t *testing.T) {
	ctx, ss, db, done := newDBMockStateStore(t)
	defer done()

	schemaHash := HashIDKeccak(([]byte)("schema1"))
	cacheKey := schemaCacheKey("domain1", schemaHash)
	ss.abiSchemaCache.Set(cacheKey, &abiSchema{
		SchemaEntity: &SchemaEntity{Hash: *schemaHash},
		definition:   &abi.Parameter{},
	})

	db.ExpectQuery("SELECT.*created_at").WillReturnError(fmt.Errorf("pop"))

	_, err := ss.FindStates(ctx, "domain1", schemaHash.String(), &filters.QueryJSON{
		FilterJSON: filters.FilterJSON{
			FilterJSONOps: filters.FilterJSONOps{
				GreaterThan: []*filters.FilterJSONKeyValue{
					{FilterJSONBase: filters.FilterJSONBase{
						Field: ".created",
					}, Value: types.RawJSON(fmt.Sprintf("%d", time.Now().UnixNano()))},
				},
			},
		},
	}, "all")
	assert.Regexp(t, "pop", err)

}
