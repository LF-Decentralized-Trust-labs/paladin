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
	"time"

	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	// Import pq driver
	migratedb "github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/database/postgres"
)

type postgresProvider struct{}

func newPostgresProvider(ctx context.Context, conf *Config) (p *provider, err error) {
	return newSQLProvider(ctx, &postgresProvider{}, &conf.Postgres.SQLDBConfig, &SQLDBConfigDefaults{
		MaxOpenConns:    100,
		MaxIdleConns:    100,
		ConnMaxIdleTime: 60 * time.Second,
		ConnMaxLifetime: 0,
	})
}

func (p *postgresProvider) DBName() string {
	return "postgres"
}

func (p *postgresProvider) Open(uri string) gorm.Dialector {
	return gormPostgres.Open(uri)
}

func (p *postgresProvider) GetMigrationDriver(db *sql.DB) (migratedb.Driver, error) {
	return postgres.WithInstance(db, &postgres.Config{})
}
