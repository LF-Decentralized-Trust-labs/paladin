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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
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
