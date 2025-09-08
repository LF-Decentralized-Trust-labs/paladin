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
	"html/template"
	"os"
	"runtime/debug"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/golang-migrate/migrate/v4"
	migratedb "github.com/golang-migrate/migrate/v4/database"

	"gorm.io/gorm"
	// Import migrate file source
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

type provider struct {
	p    SQLDBProvider
	gdb  *gorm.DB
	db   *sql.DB
	conf *pldconf.SQLDBConfig
}

type SQLDBProvider interface {
	DBName() string
	Open(uri string) gorm.Dialector
	GetMigrationDriver(*sql.DB) (migratedb.Driver, error)
	TakeNamedLock(ctx context.Context, dbTX DBTX, lockName string) error
}

func NewSQLProvider(ctx context.Context, p SQLDBProvider, conf *pldconf.SQLDBConfig, defs *pldconf.SQLDBConfig) (_ Persistence, err error) {
	if conf.DSN == "" {
		return nil, i18n.WrapError(ctx, err, msgs.MsgPersistenceMissingDSN)
	}
	dsn := conf.DSN

	if len(conf.DSNParams) > 0 {
		if dsn, err = templatedDSN(ctx, conf); err != nil {
			return nil, err
		}
	}

	var gp *provider
	gdb, err := gorm.Open(p.Open(dsn), &gorm.Config{
		SkipDefaultTransaction: true,
		PrepareStmt:            confutil.Bool(conf.StatementCache, *defs.StatementCache),
	})
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
	if conf.DebugQueries {
		gp.gdb = gp.gdb.Debug()
	}
	gp.db.SetMaxOpenConns(confutil.IntMin(conf.MaxOpenConns, 1, *defs.MaxOpenConns))
	gp.db.SetMaxIdleConns(confutil.Int(conf.MaxIdleConns, *defs.MaxIdleConns))
	gp.db.SetConnMaxIdleTime(confutil.DurationMin(conf.ConnMaxIdleTime, 0, *defs.ConnMaxIdleTime))
	gp.db.SetConnMaxLifetime(confutil.DurationMin(conf.ConnMaxLifetime, 0, *defs.ConnMaxLifetime))

	if confutil.Bool(conf.AutoMigrate, false) {
		if err = gp.runMigration(ctx, func(m *migrate.Migrate) error { return m.Up() }); err != nil {
			return nil, err
		}
	}
	return gp, nil
}

func templatedDSN(ctx context.Context, conf *pldconf.SQLDBConfig) (string, error) {

	tmpl, err := template.New("dsn").Option("missingkey=error").Parse(conf.DSN)
	if err != nil {
		return "", i18n.WrapError(ctx, err, msgs.MsgPersistenceInvalidDSNTemplate)
	}

	// Load each of the params - from a kubernetes secret for example
	values := map[string]any{}
	for paramName, param := range conf.DSNParams {
		switch {
		case param.File != "":
			valueBytes, err := os.ReadFile(param.File)
			if err != nil {
				return "", i18n.WrapError(ctx, err, msgs.MsgPersistenceDSNParamLoadFile, paramName, param.File)
			}
			values[paramName] = strings.TrimSpace(string(valueBytes))
		}
	}

	out := new(strings.Builder)
	if err := tmpl.Execute(out, values); err != nil {
		return "", i18n.WrapError(ctx, err, msgs.MsgPersistenceInvalidDSNTemplate)
	}
	log.L(ctx).Warnf("REMOVE: DSN='%s'", out.String())
	return out.String(), nil

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

func (gp *provider) Close() {
	err := gp.db.Close()
	log.L(context.Background()).Infof("DB closed (err=%v)", err)
}

func (gp *provider) TakeNamedLock(ctx context.Context, dbTX DBTX, lockName string) error {
	return gp.p.TakeNamedLock(ctx, dbTX, lockName)
}

// Run a transaction with preCommit, postCommit and finalizer support to propagate between components in a simple and consistent way.
func (gp *provider) Transaction(parentCtx context.Context, fn func(ctx context.Context, tx DBTX) error) (err error) {

	completed := false
	tx := &transaction{txCtx: log.WithLogField(parentCtx, "dbtx", pldtypes.ShortID())}
	defer func() {
		if !completed {
			panicData := recover()
			log.L(tx.txCtx).Errorf("Panic within database transaction: %v\n%s", panicData, debug.Stack())
			if err == nil {
				err = i18n.NewError(tx.txCtx, msgs.MsgPersistenceErrorInDBTransaction, panicData)
			}
		}
		for _, fn := range tx.finalizers {
			// Finalizers are called with success or failure
			fn(tx.txCtx, err)
		}
		if err == nil {
			for _, fn := range tx.postCommits {
				fn(tx.txCtx)
			}
		}
		if !completed {
			panic(err) // having logged this, we continue to panic rather than switching to normal error handling
		}
	}()

	// Run the database transaction itself
	err = gp.gdb.Transaction(func(gormTX *gorm.DB) error {
		tx.gdb = gormTX.WithContext(tx.txCtx)
		innerErr := fn(tx.txCtx, tx)
		for _, fn := range tx.preCommits {
			if innerErr == nil {
				innerErr = fn(tx.txCtx, tx)
			}
		}
		return innerErr
	})

	if err != nil {
		for _, fn := range tx.postRollbacks {
			err = fn(tx.txCtx, err)
		}
	}

	completed = true
	return err // important that this is the function var used in the defer processing

}

func (gp *provider) NOTX() DBTX {
	return newNOTX(gp.gdb)
}
