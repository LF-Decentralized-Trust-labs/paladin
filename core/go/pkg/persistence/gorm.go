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
	"reflect"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/golang-migrate/migrate/v4"
	migratedb "github.com/golang-migrate/migrate/v4/database"
	"github.com/lib/pq"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

	// If supported by the dialect, use the ANY clause for IN statements to avoid parameter limits
	// This is a PostgreSQL specific feature
	UseAny(gp.gdb)
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

var (
	whereClause     = clause.Where{}.Name()
	postgresDialect = postgres.Dialector{}.Name()
)

// ANY is a custom implementation of the clause.IN or clause.Expr which binds array of Values directly to a single variable
// In the clause.Expr, it replaces the IN clause with ANY, which is a PostgreSQL-specific feature.
// it has been implemented to address the limitation of "protocol limited to 65535 parameters".
type ANY struct {
	IN   *clause.IN
	Expr *clause.Expr
}

// UseAny configures the DB to use the ANY type for IN clauses to resolve parameter limitations.
func UseAny(db *gorm.DB) {
	currentDialect := db.Name()
	if currentDialect != postgresDialect {
		log.L(db.Statement.Context).Errorf("ANY clause not supported with %q dialect", currentDialect)
		return
	}

	db.ClauseBuilders[whereClause] = func(c clause.Clause, builder clause.Builder) {
		where := c.Expression.(clause.Where)
		// Recursively process all expressions to handle nested conditions
		processExpressions(where.Exprs)
		c.Build(builder)
	}
}

// processExpressions recursively processes expressions to find and replace IN clauses with ANY
func processExpressions(exprs []clause.Expression) {
	for i, expr := range exprs {
		switch e := expr.(type) {
		case clause.IN:
			// Replace IN clause with ANY
			exprs[i] = ANY{IN: &e}
		case clause.Expr:
			// Check if this expression contains IN (but not NOT IN)
			strExpr := e.SQL
			if strings.Contains(strExpr, " IN ") && !strings.Contains(strExpr, " NOT ") {
				exprs[i] = ANY{Expr: &e}
			}
		case clause.Where:
			// Recursively process nested WHERE clauses
			processExpressions(e.Exprs)
		case clause.OrConditions:
			// Recursively process OR conditions
			processExpressions(e.Exprs)
		case clause.AndConditions:
			// Recursively process AND conditions
			processExpressions(e.Exprs)
		}
	}
}

// Build constructs the postgres ANY clause, used to make queries with large value lists work
func (c ANY) Build(builder clause.Builder) {

	if c.IN != nil {
		// Only replace clause.IN with ANY for value lists, not subqueries
		hasNonValue := slices.ContainsFunc(c.IN.Values, func(v any) bool {
			switch v.(type) {
			case sql.NamedArg, clause.Column, clause.Table, clause.Interface, clause.Expression, []any, *gorm.DB:
				return true
			}
			return false
		})

		// use clause.IN as default
		if hasNonValue || len(c.IN.Values) <= 1 {
			c.IN.Build(builder)
			return
		}

		builder.WriteQuoted(c.IN.Column)
		stmt := builder.(*gorm.Statement)

		// actual binding of the array
		// replacing `IN ($1, $2, $3)` with `= ANY ($1)`
		// which then translates to `= ANY([element, element2, element3, ...])`
		_, _ = builder.WriteString(" = ANY (")
		addBulk(stmt, c.IN.Values)
		_, _ = builder.WriteString(")")
	}

	if c.Expr != nil {
		// Only replace clause.Expr with ANY for value lists, not subqueries
		hasNonValue := slices.ContainsFunc(c.Expr.Vars, func(v any) bool {
			switch v.(type) {
			case sql.NamedArg, clause.Column, clause.Table, clause.Interface, clause.Expression, []any, *gorm.DB:
				return true
			}
			return false
		})
		if hasNonValue || len(c.Expr.Vars) == 0 {
			c.Expr.Build(builder)
			return
		}

		values := interfaceSlice(c.Expr.Vars[0])
		if values == nil {
			// Not an array of values, so use clause.Expr
			c.Expr.Build(builder)
			return
		}

		if len(values) <= 1 {
			// Only one value so use clause.Expr
			c.Expr.Build(builder)
			return
		}

		strExpr := c.Expr.SQL
		colName := strings.TrimSpace(strings.Split(strExpr, "IN")[0])

		builder.WriteQuoted(colName)
		stmt := builder.(*gorm.Statement)

		// actual binding of the array
		// replacing `IN ($1, $2, $3)` with `= ANY ($1)`
		// which then translates to `= ANY([element, element2, element3, ...])`
		_, _ = builder.WriteString(" = ANY (")
		addBulk(stmt, pq.Array(values))
		_, _ = builder.WriteString(")")
	}
}

// addBulk integrates a list of values into the query, leveraging postgres's array binding support
func addBulk(stmt *gorm.Statement, v any) {
	stmt.Vars = append(stmt.Vars, v)
	stmt.BindVarTo(stmt, stmt, v)
}

// Util function to convert a slice of any type to a slice of interface{}
func interfaceSlice(slice interface{}) []interface{} {
	s := reflect.ValueOf(slice)
	if s.Kind() != reflect.Slice {
		panic("InterfaceSlice() given a non-slice type")
	}

	// Keep the distinction between nil and empty slice input
	if s.IsNil() {
		return nil
	}

	ret := make([]interface{}, s.Len())

	for i := 0; i < s.Len(); i++ {
		ret[i] = s.Index(i).Interface()
	}

	return ret
}
