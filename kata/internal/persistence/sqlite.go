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

	migratedb "github.com/golang-migrate/migrate/v4/database"
	migratesqlite3 "github.com/golang-migrate/migrate/v4/database/sqlite3"
	gormSQLite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type sqliteProvider struct{}

func newSQLiteProvider(ctx context.Context, conf *Config) (p *provider, err error) {
	return newSQLProvider(ctx, &sqliteProvider{}, &conf.SQLite.SQLDBConfig, &SQLDBConfigDefaults{
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxIdleTime: 0,
		ConnMaxLifetime: 0,
	})
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
