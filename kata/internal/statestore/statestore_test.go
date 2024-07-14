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

// import (
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"testing"

// 	"github.com/hyperledger/firefly-common/pkg/dbsql"
// 	"github.com/hyperledger/firefly-common/pkg/fftypes"
// 	"github.com/sirupsen/logrus"
// 	"github.com/stretchr/testify/assert"
// )

// func newTestCollections(t *testing.T) (context.Context, *persistence, func()) {
// 	ctx, _, done := initTestPSQL(t, true)
// 	return ctx, NewPersistence().(*persistence), done
// }

// // TestE2E:
// // 1: Validates the migrations against a real DB
// // 2: Creates one of each object to check the DB tables function
// func TestE2E(t *testing.T) {
// 	logrus.SetLevel(logrus.TraceLevel)

// 	ctx, crud, done := newTestCollections(t)
// 	defer done()
// 	tCtx := &crudTestActivity{Context: ctx, t: t, crud: crud, uniquePrefixes: map[string]string{}}
// 	fakeString := "fake_string"
// 	pendingStatus := TransactionPending

// 	t1 := &Transaction{
// 		ID:              fftypes.NewUUID(),
// 		Created:         fftypes.Now(),
// 		Updated:         fftypes.Now(),
// 		IdempotencyKey:  &fakeString,
// 		Status:          &pendingStatus,
// 		From:            &fakeString,
// 		ContractAddress: &fakeString,
// 		Payload:         &fakeString,
// 	}
// 	testStoreRetrieve(tCtx, t1)
// 	assert.NoError(t, crud.UTDeleteAllData(ctx))

// }

// type crudTestActivity struct {
// 	t *testing.T
// 	context.Context
// 	crud           *persistence
// 	uniquePrefixes map[string]string
// 	names          []string
// }

// func testStoreRetrieve[T dbsql.Resource](tCtx *crudTestActivity, obj T, generators ...func(obj, rObj T)) {
// 	result := tCtx.crud.db.Create(obj)
// 	assert.NoError(tCtx.t, result.Error)
// 	var rObj T
// 	result = tCtx.crud.db.Model(&Transaction{ID: fftypes.MustParseUUID(obj.GetID())}).First(&rObj)
// 	assert.NoError(tCtx.t, result.Error)
// 	for _, g := range generators {
// 		g(obj, rObj)
// 	}
// 	compareJSON(tCtx.t, obj, rObj)
// 	objName := fmt.Sprintf("%T", obj)
// 	tCtx.names = append(tCtx.names, objName)
// }

// func compareJSON(t *testing.T, expected, actual interface{}) {
// 	ej, err := json.Marshal(expected)
// 	assert.NoError(t, err)
// 	aj, err := json.Marshal(actual)
// 	assert.NoError(t, err)
// 	assert.JSONEq(t, string(ej), string(aj))
// }
