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

package manager

import (
	"context"
	"database/sql"
	"fmt"
	"path"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/dbsql"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/log"
	smconfig "github.com/kaleido-io/paladin-state-manager/internal/config"
	"github.com/kaleido-io/paladin-state-manager/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// This must match the database container that is started for local testing
const PSQL_URL = "postgres://postgres:my-secret@localhost:5432/%s?sslmode=disable"

// The intention of this test is to get as close to an E2E as possible, while still running
// efficiently in-line with VSCode development.
//
// - Uses a real PostgreSQL database
// - Only uses the public interface of the service
// - Keeps mocking to a minimum, with hopefully clear justifications in cases it's necessary

type E2ESuite struct {
	suite.Suite
	ctx         context.Context
	utdbName    string
	persistence db.Persistence
	suiteDone   func()
	testDone    func()
	mgr         *stateManagerService
}

func TestE2E(t *testing.T) {
	suite.Run(t, new(E2ESuite))
}

func (e *E2ESuite) BeforeTest(suiteName, testName string) {
	log.L(e.ctx).Infof("START: %s/%s", suiteName, testName)
}

func (e *E2ESuite) AfterTest(suiteName, testName string) {
	log.L(e.ctx).Infof("END: %s/%s", suiteName, testName)
	e.testDone()
}

func (e *E2ESuite) SetupSuite() {
	t := e.T()
	setupConfig(t, baseConfig)
	e.utdbName = "ut_" + fftypes.NewUUID().String()
	e.ctx, e.persistence, e.suiteDone = initTestPSQL(t, e.utdbName)
	config.Set(config.LogLevel, "debug")
	config.SetupLogging(e.ctx)
}

func (e *E2ESuite) TearDownSuite() {
	e.suiteDone()
}

func (e *E2ESuite) SetupTest() {
	ctx := context.Background()
	t := e.T()

	mgr, err := NewStateManagerService(ctx)
	assert.NoError(t, err)

	e.mgr = mgr.(*stateManagerService)

	e.testDone = func() {
		err := e.persistence.UTDeleteAllData(e.ctx)
		assert.NoError(e.T(), err)
	}
}

func initTestPSQL(t *testing.T, utdbName string) (context.Context, db.Persistence, func()) {

	ctx, cancelCtx := context.WithCancel(context.Background())
	dbURL := func(dbname string) string {
		return fmt.Sprintf(PSQL_URL, dbname)
	}

	// First create the database - using the super user
	adminDB, err := sql.Open("postgres", dbURL("postgres"))
	assert.NoError(t, err)
	_, err = adminDB.Exec(fmt.Sprintf(`CREATE DATABASE "%s";`, utdbName))
	assert.NoError(t, err)
	err = adminDB.Close()
	assert.NoError(t, err)

	// Get the database entry
	databaseConfig := smconfig.DatabaseSection
	postgresConfig := databaseConfig.SubSection("postgres")

	postgresConfig.Set(dbsql.SQLConfDatasourceURL, dbURL(utdbName))
	postgresConfig.AddKnownKey(dbsql.SQLConfMigrationsDirectory, path.Join("..", "..", "db", "migrations", "postgres"))

	psql := db.InitConfig(postgresConfig)
	err = psql.Init(ctx, postgresConfig)
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

	return ctx, db.UTNewPersistenceDB(&psql.Database), func() {
		cancelCtx()
		psql.Close()
		err := m.Drop()
		assert.NoError(t, err)
	}
}

func ptrTo[T any](v T) *T {
	return &v
}
