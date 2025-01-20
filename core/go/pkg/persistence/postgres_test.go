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

	gormPostgres "gorm.io/driver/postgres"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPostgresProvider(t *testing.T) {
	p := &postgresProvider{}
	assert.Equal(t, "postgres", p.DBName())
	assert.Equal(t, "*postgres.Dialector", reflect.TypeOf(p.Open("")).String())
	db, mdb, _ := sqlmock.New()
	_, err := p.GetMigrationDriver(db)
	assert.Error(t, err)

	mdb.ExpectBegin()
	mdb.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(driver.ResultNoRows)
	mdb.ExpectCommit()

	gdb, err := gorm.Open(gormPostgres.New(gormPostgres.Config{Conn: db}), &gorm.Config{})
	require.NoError(t, err)
	err = gdb.Transaction(func(dbTX *gorm.DB) error {
		return p.TakeNamedLock(context.Background(), dbTX, "any")
	})
	require.NoError(t, err)
}
