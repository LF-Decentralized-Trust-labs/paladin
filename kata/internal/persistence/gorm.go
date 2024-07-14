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

	"github.com/golang-migrate/migrate/v4"
	migratedb "github.com/golang-migrate/migrate/v4/database"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/msgs"

	"gorm.io/gorm"
	// Import migrate file source
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

type provider struct {
	p    SQLDBProvider
	gdb  *gorm.DB
	db   *sql.DB
	conf *SQLDBConfig
}

type SQLDBProvider interface {
	DBName() string
	Open(uri string) gorm.Dialector
	GetMigrationDriver(*sql.DB) (migratedb.Driver, error)
}

type SQLDBConfig struct {
	URI             string  `yaml:"uri"`
	MaxOpenConns    *int    `yaml:"maxOpenConns"`
	MaxIdleConns    *int    `yaml:"maxIdleConns"`
	ConnMaxIdleTime *string `yaml:"connMaxIdleTime"`
	ConnMaxLifetime *string `yaml:"connMaxLifetime"`
	AutoMigrate     *bool   `yaml:"autoMigrate"`
	MigrationsDir   string  `yaml:"migrationsDir"`
}

type SQLDBConfigDefaults struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxIdleTime time.Duration
	ConnMaxLifetime time.Duration
}

func newSQLProvider(ctx context.Context, p SQLDBProvider, conf *SQLDBConfig, defs *SQLDBConfigDefaults) (gp *provider, err error) {
	if conf.URI == "" {
		return nil, i18n.WrapError(ctx, err, msgs.MsgPersistenceMissingURI)
	}

	gdb, err := gorm.Open(p.Open(conf.URI), &gorm.Config{})
	if err == nil {
		gp = &provider{
			p:    p,
			gdb:  gdb,
			conf: conf,
		}
		gp.db, err = gdb.DB()
	}
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgPersistenceInitFailed)
	}
	gp.db.SetMaxOpenConns(confutil.IntMin(conf.MaxOpenConns, 1, defs.MaxOpenConns))
	gp.db.SetMaxIdleConns(confutil.Int(conf.MaxIdleConns, defs.MaxIdleConns))
	gp.db.SetConnMaxIdleTime(confutil.Duration(conf.ConnMaxIdleTime, defs.ConnMaxIdleTime))
	gp.db.SetConnMaxLifetime(confutil.Duration(conf.ConnMaxLifetime, defs.ConnMaxLifetime))

	if confutil.Bool(conf.AutoMigrate, false) {
		if err = gp.runMigration(ctx, func(m *migrate.Migrate) error { return m.Up() }); err != nil {
			return nil, err
		}
	}
	return gp, nil
}

func (gp *provider) runMigration(ctx context.Context, mig func(m *migrate.Migrate) error) error {
	m, err := gp.getMigrate(ctx)
	if err == nil {
		err = mig(m)
	}
	if err != nil && err != migrate.ErrNoChange {
		return i18n.WrapError(ctx, err, msgs.MsgPersistenceMigrationFailed)
	}
	version, dirty, _ := m.Version()
	log.L(ctx).Infof("Migrations now at: v=%d dirty=%t", version, dirty)
	return nil
}

func (gp *provider) getMigrate(ctx context.Context) (m *migrate.Migrate, err error) {
	if gp.conf.MigrationsDir == "" {
		return nil, i18n.NewError(ctx, msgs.MsgPersistenceMissingMigrationDir)
	}
	driver, err := gp.p.GetMigrationDriver(gp.db)
	if err == nil {
		fileURL := "file://" + gp.conf.MigrationsDir
		log.L(ctx).Infof("Running migrations in: %s", fileURL)
		m, err = migrate.NewWithDatabaseInstance(fileURL, gp.p.DBName(), driver)
	}
	return m, err
}

func (gp *provider) DB() *gorm.DB {
	return gp.gdb
}
