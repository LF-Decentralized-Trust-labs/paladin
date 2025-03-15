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

package persistence

import (
	"context"
	"database/sql/driver"
	"reflect"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresProvider(t *testing.T) {
	p := &postgresProvider{}
	assert.Equal(t, "postgres", p.DBName())
	assert.Equal(t, "*postgres.Dialector", reflect.TypeOf(p.Open("")).String())
	db, _, _ := sqlmock.New()
	_, err := p.GetMigrationDriver(db)
	assert.Error(t, err)
}

func TestPosgresNamedLock(t *testing.T) {

	p, mdb := newMockGormPSQLPersistence(t)

	mdb.ExpectBegin()
	mdb.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(driver.ResultNoRows)
	mdb.ExpectCommit()

	err := p.Transaction(context.Background(), func(ctx context.Context, dbTX DBTX) error {
		return p.TakeNamedLock(ctx, dbTX, "any")
	})
	require.NoError(t, err)
}
