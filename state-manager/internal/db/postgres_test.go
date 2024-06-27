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
	"database/sql"
	"fmt"
	"path"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/golang-migrate/migrate/v4"
	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/dbsql"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/stretchr/testify/assert"
)

// This must match the database container that is started for local testing
const PSQL_URL = "postgres://postgres:my-secret@localhost:5432/%s?sslmode=disable"

func initTestPSQL(t *testing.T) (context.Context, *Postgres, *migrate.Migrate, func()) {

	ctx, cancelCtx := context.WithCancel(context.Background())
	dbconf := config.RootSection("utdb")
	InitConfig(dbconf)

	dbURL := func(dbname string) string {
		return fmt.Sprintf(PSQL_URL, dbname)
	}
	utdbName := "ut_" + fftypes.NewUUID().String()

	// First create the database - using the super user
	adminDB, err := sql.Open("postgres", dbURL("postgres"))
	assert.NoError(t, err)
	_, err = adminDB.Exec(fmt.Sprintf(`CREATE DATABASE "%s";`, utdbName))
	assert.NoError(t, err)
	err = adminDB.Close()
	assert.NoError(t, err)

	dbconf.Set(dbsql.SQLConfDatasourceURL, dbURL(utdbName))
	dbconf.Set(dbsql.SQLConfMigrationsDirectory, path.Join("..", "..", "db", "migrations", "postgres"))

	psql := InitConfig(dbconf)
	err = psql.Init(ctx, dbconf)
	assert.NoError(t, err)

	driver, err := psql.GetMigrationDriver(psql.DB())
	assert.NoError(t, err)
	m, err := migrate.NewWithDatabaseInstance(
		"file://../../db/migrations/postgres",
		utdbName,
		driver,
	)
	assert.NoError(t, err)

	err = m.Up()
	assert.NoError(t, err)

	return ctx, psql, m, func() {
		cancelCtx()
		psql.Close()
		err := m.Drop()
		assert.NoError(t, err)
		psql = nil
	}
}

func TestPSQLProvider(t *testing.T) {

	ctx, psql, m, done := initTestPSQL(t)
	defer done()

	// Test locking
	txctx, tx, ac, err := psql.BeginOrUseTx(ctx)
	assert.NoError(t, err)
	err = psql.AcquireLockTx(txctx, "mylock", tx)
	assert.NoError(t, err)
	psql.RollbackTx(ctx, tx, ac)

	// Test insert query optimizations
	_, ok := psql.ApplyInsertQueryCustomizations(sq.Insert(""), true)
	assert.True(t, ok)

	// test down migration (up migration is in initTestPSQL)
	err = m.Down()
	assert.NoError(t, err)

}
