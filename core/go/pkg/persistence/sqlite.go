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

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	migratedb "github.com/golang-migrate/migrate/v4/database"
	migratesqlite3 "github.com/golang-migrate/migrate/v4/database/sqlite3"
	gormSQLite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type sqliteProvider struct{}

var SQLiteDefaults = &pldconf.SQLDBConfig{
	MaxOpenConns:    confutil.P(1),
	MaxIdleConns:    confutil.P(1),
	ConnMaxIdleTime: confutil.P("0"),
	ConnMaxLifetime: confutil.P("0"),
	StatementCache:  confutil.P(false),
}

func newSQLiteProvider(ctx context.Context, conf *pldconf.DBConfig) (p Persistence, err error) {
	return NewSQLProvider(ctx, &sqliteProvider{}, &conf.SQLite.SQLDBConfig, SQLiteDefaults)
}

func (p *sqliteProvider) DBName() string {
	return "sqlite"
}

func (p *sqliteProvider) Open(uri string) gorm.Dialector {
	return gormSQLite.Open(uri)
}

func (p *sqliteProvider) GetMigrationDriver(db *sql.DB) (migratedb.Driver, error) {
	return migratesqlite3.WithInstance(db, &migratesqlite3.Config{})
}

func (p *sqliteProvider) TakeNamedLock(ctx context.Context, dbTX DBTX, lockName string) error {
	return nil
}
