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
	"database/sql"

	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	// Import pq driver
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	migratedb "github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/database/postgres"
)

var PostgresDefaults = &pldconf.SQLDBConfig{
	MaxOpenConns:    confutil.P(100),
	MaxIdleConns:    confutil.P(100),
	ConnMaxIdleTime: confutil.P("60s"),
	ConnMaxLifetime: confutil.P("0"),
	StatementCache:  confutil.P(true),
}

type postgresProvider struct{}

func newPostgresProvider(ctx context.Context, conf *pldconf.DBConfig) (p Persistence, err error) {
	return NewSQLProvider(ctx, &postgresProvider{}, &conf.Postgres.SQLDBConfig, PostgresDefaults)
}

func (p *postgresProvider) DBName() string {
	return "postgres"
}

func (p *postgresProvider) Open(dsn string) gorm.Dialector {
	return gormPostgres.Open(dsn)
}

func (p *postgresProvider) GetMigrationDriver(db *sql.DB) (migratedb.Driver, error) {
	return postgres.WithInstance(db, &postgres.Config{})
}

func (p *postgresProvider) TakeNamedLock(ctx context.Context, dbTX DBTX, lockName string) error {
	return dbTX.DB().Exec(`SELECT pg_advisory_xact_lock( ? )`, hashCode(lockName)).Error
}
