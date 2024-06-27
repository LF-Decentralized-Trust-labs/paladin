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

package db

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hyperledger/firefly-common/pkg/dbsql"
	"github.com/hyperledger/firefly-common/pkg/ffapi"
	"github.com/kaleido-io/paladin-state-manager/pkg/apitypes"
	"github.com/stretchr/testify/assert"
)

func newTestDatabase(t *testing.T) (context.Context, *persistence, func()) {
	ctx, psql, _, done := initTestPSQL(t)
	return ctx, NewPersistencePSQL(psql).(*persistence), done
}

func ptrTo[T any](v T) *T {
	return &v
}

// TestE2E:
// 1: Validates the migrations against a real DB
// 2: Creates one of each object to check the DB tables function
func TestCRUDWithRealDB(t *testing.T) {
	ctx, p, done := newTestDatabase(t)
	defer done()
	tCtx := &crudTestActivity{Context: ctx, t: t, crud: p}

	testStoreRetrieve(tCtx, p.States(), &apitypes.State{
		ID:    ptrTo("state1"),
		State: &apitypes.StateProposed,
	})

	testUTDeleteAllData(tCtx)
}

// counter is the un-typed bits of the CRUD interface that let us count the DB entries
type counter interface {
	Count(ctx context.Context, filter ffapi.Filter) (count int64, err error)
	NewFilterBuilder(ctx context.Context) ffapi.FilterBuilder
}

type crudTestActivity struct {
	t *testing.T
	context.Context
	crud     *persistence
	names    []string
	counters []counter
}

func testStoreRetrieve[T dbsql.Resource](tCtx *crudTestActivity, coll dbsql.CRUD[T], obj T, tweaks ...func(a, b T)) {
	created, err := coll.Upsert(tCtx, obj, dbsql.UpsertOptimizationDB)
	assert.NoError(tCtx.t, err)
	assert.True(tCtx.t, created)
	rObj, err := coll.GetByID(tCtx, obj.GetID(), dbsql.FailIfNotFound)
	assert.NoError(tCtx.t, err)
	for _, fn := range tweaks {
		fn(obj, rObj)
	}
	compareJSON(tCtx.t, obj, rObj)
	objName := fmt.Sprintf("%T", obj)
	tCtx.names = append(tCtx.names, objName)
	tCtx.counters = append(tCtx.counters, coll)
}

func testUTDeleteAllData(tCtx *crudTestActivity) {
	assert.NoError(tCtx.t, tCtx.crud.UTDeleteAllData(tCtx))
	for i, counter := range tCtx.counters {
		count, err := counter.Count(tCtx, counter.NewFilterBuilder(tCtx).And())
		assert.NoError(tCtx.t, err)
		assert.Zero(tCtx.t, count, fmt.Sprintf("Maybe forgot to add table to UTDeleteAllData for '%s'?", tCtx.names[i]))
	}
}

func compareJSON(t *testing.T, expected, actual interface{}) {
	ej, err := json.Marshal(expected)
	assert.NoError(t, err)
	aj, err := json.Marshal(actual)
	assert.NoError(t, err)
	assert.JSONEq(t, string(ej), string(aj))
}

func TestUTNewPersistenceDB(t *testing.T) {
	assert.NotNil(t, UTNewPersistenceDB(nil))
}
