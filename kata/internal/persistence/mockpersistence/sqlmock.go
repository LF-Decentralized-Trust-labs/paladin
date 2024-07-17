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

package mockpersistence

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/DATA-DOG/go-sqlmock"
	migratedb "github.com/golang-migrate/migrate/v4/database"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/persistence"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var SQLMockDefaults = &persistence.SQLDBConfig{
	MaxOpenConns:    confutil.P(1),
	MaxIdleConns:    confutil.P(1),
	ConnMaxIdleTime: confutil.P("0"),
	ConnMaxLifetime: confutil.P("0"),
}

type SQLMockProvider struct {
	DB   *sql.DB
	Mock sqlmock.Sqlmock
	P    persistence.Persistence
}

func NewSQLMockProvider() (p *SQLMockProvider, err error) {
	mp := &SQLMockProvider{}
	mp.DB, mp.Mock, err = sqlmock.New()
	if err == nil {
		mp.P, err = persistence.NewSQLProvider(context.Background(), mp, &persistence.SQLDBConfig{
			URI: "mocked",
		}, SQLMockDefaults)
	}
	return mp, err
}

func (p *SQLMockProvider) DBName() string {
	return "sqlmock"
}

func (p *SQLMockProvider) Open(uri string) gorm.Dialector {
	return mysql.New(mysql.Config{
		Conn:                      p.DB,
		SkipInitializeWithVersion: true,
	})
}

func (p *SQLMockProvider) GetMigrationDriver(db *sql.DB) (migratedb.Driver, error) {
	return nil, fmt.Errorf("not supported")
}
