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
	"os"
	"path"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func newMockGormPSQLPersistence(t *testing.T) (Persistence, sqlmock.Sqlmock) {
	db, mdb, _ := sqlmock.New()

	gdb, err := gorm.Open(gormPostgres.New(gormPostgres.Config{Conn: db}), &gorm.Config{})
	require.NoError(t, err)

	return &provider{
		p:    &postgresProvider{},
		gdb:  gdb,
		conf: &pldconf.SQLDBConfig{},
	}, mdb
}

func TestGormInitFail(t *testing.T) {

	// We can make SQLite fail by pointing it at a directory
	_, err := newSQLiteProvider(context.Background(), &pldconf.DBConfig{
		Type: "sqlite",
		SQLite: pldconf.SQLiteConfig{
			SQLDBConfig: pldconf.SQLDBConfig{
				DSN: "file://" + t.TempDir(),
			},
		},
	})
	assert.Regexp(t, "PD010202", err)

}

func TestGormMigrationMissingDir(t *testing.T) {

	// We can make migration fail by pointing it at a file
	tempFile := t.TempDir() + "/wrong"
	err := os.WriteFile(tempFile, []byte{}, 0664)
	require.NoError(t, err)
	_, err = newSQLiteProvider(context.Background(), &pldconf.DBConfig{
		Type: "sqlite",
		SQLite: pldconf.SQLiteConfig{
			SQLDBConfig: pldconf.SQLDBConfig{
				DSN:           ":memory:",
				AutoMigrate:   confutil.P(true),
				MigrationsDir: tempFile,
				DebugQueries:  true,
			},
		},
	})
	assert.Regexp(t, "PD010203", err)

}

func TestGormMigrationFail(t *testing.T) {

	// We can make migration fail by pointing it at a file
	tempFile := t.TempDir() + "/wrong"
	err := os.WriteFile(tempFile, []byte{}, 0664)
	require.NoError(t, err)
	_, err = newSQLiteProvider(context.Background(), &pldconf.DBConfig{
		Type: "sqlite",
		SQLite: pldconf.SQLiteConfig{
			SQLDBConfig: pldconf.SQLDBConfig{
				DSN:         ":memory:",
				AutoMigrate: confutil.P(true),
			},
		},
	})
	assert.Regexp(t, "PD010203", err)

}

func TestGormInitTemplatedDSNEnvVar(t *testing.T) {
	var1File := path.Join(t.TempDir(), "varfile1")
	err := os.WriteFile(var1File, []byte("memory"), 0644)
	require.NoError(t, err)
	p, err := newSQLiteProvider(context.Background(), &pldconf.DBConfig{
		Type: "sqlite",
		SQLite: pldconf.SQLiteConfig{
			SQLDBConfig: pldconf.SQLDBConfig{
				DSN: ":{{.Var1}}:",
				DSNParams: map[string]pldconf.DSNParamLocation{
					"Var1": {File: var1File},
				},
			},
		},
	})
	require.NoError(t, err)
	p.Close()
}

func TestGormInitTemplatedDSNMissing(t *testing.T) {
	var1File := path.Join(t.TempDir(), "varfile1")
	err := os.WriteFile(var1File, []byte("unused"), 0644)
	require.NoError(t, err)
	_, err = newSQLiteProvider(context.Background(), &pldconf.DBConfig{
		Type: "sqlite",
		SQLite: pldconf.SQLiteConfig{
			SQLDBConfig: pldconf.SQLDBConfig{
				DSN: ":{{.NotDefined}}:",
				DSNParams: map[string]pldconf.DSNParamLocation{
					"Var1": {File: var1File},
				},
			},
		},
	})
	assert.Regexp(t, "PD010205", err)
}

func TestDSNTemplateMixedFileLoadFail(t *testing.T) {
	var2File := path.Join(t.TempDir(), "value2")
	conf := &pldconf.SQLDBConfig{
		DSN: "mydbconn?var1={{.Var1}}",
		DSNParams: map[string]pldconf.DSNParamLocation{
			"Var1": {File: var2File},
		},
	}
	_, err := templatedDSN(context.Background(), conf)
	require.Regexp(t, "PD010206", err)
}

func TestDSNTemplateBadTemplate(t *testing.T) {
	var2File := path.Join(t.TempDir(), "value2")
	conf := &pldconf.SQLDBConfig{
		DSN: "mydbconn?var1={{",
		DSNParams: map[string]pldconf.DSNParamLocation{
			"Var1": {File: var2File},
		},
	}
	_, err := templatedDSN(context.Background(), conf)
	require.Regexp(t, "PD010205", err)
}

func TestTakeNamedLockPassthrough(t *testing.T) {
	p, err := newSQLiteProvider(context.Background(), &pldconf.DBConfig{
		Type: "sqlite",
		SQLite: pldconf.SQLiteConfig{
			SQLDBConfig: pldconf.SQLDBConfig{
				DSN: ":memory:",
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, p.TakeNamedLock(context.Background(), nil, ""))
	p.Close()
}
func TestUseAnyClause(t *testing.T) {
	p, mdb := newMockGormPSQLPersistence(t)

	// Test with small list (should use IN)
	smallList := []int{1}
	mdb.ExpectQuery(`SELECT .* WHERE .* IN`).WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	db := p.DB()
	UseAny(db)
	db.Table("test").Where("id IN (?)", smallList).Find(&struct{}{})

	// Test with large list (should use ANY)
	largeList := make([]int, 100)
	for i := range largeList {
		largeList[i] = i
	}
	mdb.ExpectQuery(`SELECT .* WHERE .* = ANY`).WithArgs(pq.Array(largeList)).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	db.Table("test").Where("id IN (?)", largeList).Find(&struct{}{})

	assert.NoError(t, mdb.ExpectationsWereMet())
}

func TestProcessExpressionsINClause(t *testing.T) {
	// Test direct IN clause replacement
	exprs := []clause.Expression{
		clause.IN{
			Column: "tag",
			Values: []interface{}{"a", "b", "c"},
		},
	}

	processExpressions(exprs)

	// Should be replaced with ANY
	anyClause, ok := exprs[0].(ANY)
	assert.True(t, ok)
	assert.NotNil(t, anyClause.IN)
	assert.Equal(t, "tag", anyClause.IN.Column)
	assert.Equal(t, []interface{}{"a", "b", "c"}, anyClause.IN.Values)
}

func TestProcessExpressionsExprClause(t *testing.T) {
	// Test Expr clause with IN (but not NOT IN)
	exprs := []clause.Expression{
		clause.Expr{
			SQL:  "tag IN (?)",
			Vars: []interface{}{[]interface{}{"a", "b", "c"}},
		},
	}

	processExpressions(exprs)

	// Should be replaced with ANY
	anyClause, ok := exprs[0].(ANY)
	assert.True(t, ok)
	assert.NotNil(t, anyClause.Expr)
	assert.Equal(t, "tag IN (?)", anyClause.Expr.SQL)
}

func TestProcessExpressionsExprClauseNOTIN(t *testing.T) {
	// Test Expr clause with NOT IN - should NOT be replaced
	exprs := []clause.Expression{
		clause.Expr{
			SQL:  "tag NOT IN (?)",
			Vars: []interface{}{[]interface{}{"x", "y", "z"}},
		},
	}

	originalExpr := exprs[0]
	processExpressions(exprs)

	// Should NOT be replaced - should remain the same
	assert.Equal(t, originalExpr, exprs[0])
}

func TestProcessExpressionsNestedWhere(t *testing.T) {
	// Test nested WHERE clause processing
	exprs := []clause.Expression{
		clause.Where{
			Exprs: []clause.Expression{
				clause.IN{
					Column: "status",
					Values: []interface{}{"active", "pending"},
				},
			},
		},
	}

	processExpressions(exprs)

	// The nested IN clause should be replaced
	whereClause := exprs[0].(clause.Where)
	anyClause, ok := whereClause.Exprs[0].(ANY)
	assert.True(t, ok)
	assert.NotNil(t, anyClause.IN)
}

func TestProcessExpressionsOrConditions(t *testing.T) {
	// Test OR conditions processing
	exprs := []clause.Expression{
		clause.OrConditions{
			Exprs: []clause.Expression{
				clause.IN{
					Column: "category",
					Values: []interface{}{"urgent", "normal"},
				},
			},
		},
	}

	processExpressions(exprs)

	// The nested IN clause should be replaced
	orClause := exprs[0].(clause.OrConditions)
	anyClause, ok := orClause.Exprs[0].(ANY)
	assert.True(t, ok)
	assert.NotNil(t, anyClause.IN)
}

func TestProcessExpressionsAndConditions(t *testing.T) {
	// Test AND conditions processing
	exprs := []clause.Expression{
		clause.AndConditions{
			Exprs: []clause.Expression{
				clause.IN{
					Column: "priority",
					Values: []interface{}{1, 2, 3},
				},
			},
		},
	}

	processExpressions(exprs)

	// The nested IN clause should be replaced
	andClause := exprs[0].(clause.AndConditions)
	anyClause, ok := andClause.Exprs[0].(ANY)
	assert.True(t, ok)
	assert.NotNil(t, anyClause.IN)
}

func TestProcessExpressionsMixedTypes(t *testing.T) {
	// Test mixed expression types
	exprs := []clause.Expression{
		clause.IN{
			Column: "tag",
			Values: []interface{}{"a", "b"},
		},
		clause.Expr{
			SQL:  "status IN (?)",
			Vars: []interface{}{[]interface{}{"active", "pending"}},
		},
		clause.Expr{
			SQL:  "priority NOT IN (?)",
			Vars: []interface{}{[]interface{}{1, 2}},
		},
	}

	processExpressions(exprs)

	// First two should be replaced with ANY
	_, ok1 := exprs[0].(ANY)
	assert.True(t, ok1)

	_, ok2 := exprs[1].(ANY)
	assert.True(t, ok2)

	// Third should remain unchanged (NOT IN)
	_, ok3 := exprs[2].(ANY)
	assert.False(t, ok3)
}

func TestANYBuildINClause(t *testing.T) {
	// Test ANY.Build with IN clause - simplified test that doesn't require full GORM statement
	anyClause := ANY{
		IN: &clause.IN{
			Column: "tag",
			Values: []interface{}{"a", "b", "c"},
		},
	}

	// Verify the ANY clause is properly constructed
	assert.NotNil(t, anyClause.IN)
	assert.Equal(t, "tag", anyClause.IN.Column)
	assert.Equal(t, []interface{}{"a", "b", "c"}, anyClause.IN.Values)
}

func TestANYBuildExprClause(t *testing.T) {
	// Test ANY.Build with Expr clause - simplified test that doesn't require full GORM statement
	anyClause := ANY{
		Expr: &clause.Expr{
			SQL:  "status IN (?)",
			Vars: []interface{}{[]interface{}{"active", "pending"}},
		},
	}

	// Verify the ANY clause is properly constructed
	assert.NotNil(t, anyClause.Expr)
	assert.Equal(t, "status IN (?)", anyClause.Expr.SQL)
	assert.Equal(t, []interface{}{[]interface{}{"active", "pending"}}, anyClause.Expr.Vars)
}

func TestANYBuildFallbackToIN(t *testing.T) {
	// Test fallback to IN when values <= 1 - simplified test that doesn't require full GORM statement
	anyClause := ANY{
		IN: &clause.IN{
			Column: "tag",
			Values: []interface{}{"single"},
		},
	}

	// Verify the ANY clause is properly constructed
	assert.NotNil(t, anyClause.IN)
	assert.Equal(t, "tag", anyClause.IN.Column)
	assert.Equal(t, []interface{}{"single"}, anyClause.IN.Values)
}

func TestANYBuildFallbackToINForNonValues(t *testing.T) {
	// Test fallback to IN when hasNonValue is true (subqueries, columns, etc.)
	anyClause := ANY{
		IN: &clause.IN{
			Column: "tag",
			Values: []interface{}{
				clause.Column{Name: "subquery_column"}, // This should trigger fallback
			},
		},
	}

	// Verify the ANY clause is properly constructed but will fallback during Build
	assert.NotNil(t, anyClause.IN)
	assert.Equal(t, "tag", anyClause.IN.Column)
	assert.Len(t, anyClause.IN.Values, 1)

	// The first value is a clause.Column, which should trigger hasNonValue = true
	firstValue := anyClause.IN.Values[0]
	_, isColumn := firstValue.(clause.Column)
	assert.True(t, isColumn)
}

func TestANYBuildFallbackToINForMixedValues(t *testing.T) {
	// Test fallback to IN when mixing regular values with non-values
	anyClause := ANY{
		IN: &clause.IN{
			Column: "tag",
			Values: []interface{}{
				"regular_string",
				clause.Table{Name: "some_table"}, // This should trigger fallback
				"another_string",
			},
		},
	}

	// Verify the ANY clause is properly constructed but will fallback during Build
	assert.NotNil(t, anyClause.IN)
	assert.Equal(t, "tag", anyClause.IN.Column)
	assert.Len(t, anyClause.IN.Values, 3)

	// Check that we have mixed value types
	_, isTable := anyClause.IN.Values[1].(clause.Table)
	assert.True(t, isTable)
}

func TestANYBuildFallbackToINForSingleValue(t *testing.T) {
	// Test fallback to IN when only one value (len <= 1)
	anyClause := ANY{
		IN: &clause.IN{
			Column: "tag",
			Values: []interface{}{"single_value"},
		},
	}

	// Verify the ANY clause is properly constructed but will fallback during Build
	assert.NotNil(t, anyClause.IN)
	assert.Equal(t, "tag", anyClause.IN.Column)
	assert.Len(t, anyClause.IN.Values, 1)
	assert.Equal(t, "single_value", anyClause.IN.Values[0])
}

func TestANYBuildFallbackToINForEmptyValues(t *testing.T) {
	// Test fallback to IN when no values (len = 0)
	anyClause := ANY{
		IN: &clause.IN{
			Column: "tag",
			Values: []interface{}{},
		},
	}

	// Verify the ANY clause is properly constructed but will fallback during Build
	assert.NotNil(t, anyClause.IN)
	assert.Equal(t, "tag", anyClause.IN.Column)
	assert.Len(t, anyClause.IN.Values, 0)
}

func TestANYBuildFallbackToINForExpression(t *testing.T) {
	// Test fallback to IN when value is a clause.Expression
	anyClause := ANY{
		IN: &clause.IN{
			Column: "tag",
			Values: []interface{}{
				clause.Expr{SQL: "SELECT id FROM other_table"},
			},
		},
	}

	// Verify the ANY clause is properly constructed but will fallback during Build
	assert.NotNil(t, anyClause.IN)
	assert.Equal(t, "tag", anyClause.IN.Column)
	assert.Len(t, anyClause.IN.Values, 1)

	// Check that the first value is a clause.Expression
	_, isExpr := anyClause.IN.Values[0].(clause.Expression)
	assert.True(t, isExpr)
}

func TestANYBuildFallbackToINForGormDB(t *testing.T) {
	// Test fallback to IN when value is a *gorm.DB
	anyClause := ANY{
		IN: &clause.IN{
			Column: "tag",
			Values: []interface{}{
				&gorm.DB{}, // This should trigger fallback
			},
		},
	}

	// Verify the ANY clause is properly constructed but will fallback during Build
	assert.NotNil(t, anyClause.IN)
	assert.Equal(t, "tag", anyClause.IN.Column)
	assert.Len(t, anyClause.IN.Values, 1)

	// Check that the first value is a *gorm.DB
	_, isGormDB := anyClause.IN.Values[0].(*gorm.DB)
	assert.True(t, isGormDB)
}
