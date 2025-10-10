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

//go:build testdbpostgres
// +build testdbpostgres

package persistence

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
)

const utDBPrefix = "paladin_ut_"
const migrationsDirRelative = "../../db/migrations/postgres"

var reversedDropTables []string = nil // built once
var utDBLock sync.Mutex

func requireNoError(err error) {
	if err != nil {
		panic(err)
	}
}

func buildReversedTableListFromMigrations() []string {
	migrationFiles, err := os.ReadDir(migrationsDirRelative)
	requireNoError(err)

	fileNames := make([]string, len(migrationFiles))
	for i, f := range migrationFiles {
		fileNames[i] = f.Name()
	}
	sort.Strings(fileNames)

	createTableRegex := regexp.MustCompile(`CREATE\s+TABLE\s+"?([^\s";]+)"?`)
	dropTableRegex := regexp.MustCompile(`DROP\s+TABLE\s+(IF\sEXISTS\s+)?"?([^\s";]+)+"?`)

	createTables := map[string]string{}
	dropTables := map[string]string{}
	var dropList []string
	for _, migrationFile := range fileNames {
		fileData, err := os.ReadFile(path.Join(migrationsDirRelative, migrationFile))
		requireNoError(err)
		scanner := bufio.NewScanner(bytes.NewReader(fileData))
		switch {
		case strings.HasSuffix(migrationFile, ".up.sql"):
			for scanner.Scan() {
				createTableMatch := createTableRegex.FindStringSubmatch(scanner.Text())
				dropTableMatch := dropTableRegex.FindStringSubmatch(scanner.Text())
				if len(createTableMatch) == 2 {
					createTables[createTableMatch[1]] = migrationFile
				} else if len(dropTableMatch) == 3 {
					// Remove from create & drop list - as it's been superseded in a .up migration
					delete(dropTables, dropTableMatch[2])
					delete(createTables, dropTableMatch[2])
					newDropList := make([]string, 0, len(dropList))
					for _, t := range dropList {
						if t != dropTableMatch[2] {
							newDropList = append(newDropList, t)
						}
					}
					dropList = newDropList
				}
			}
		case strings.HasSuffix(migrationFile, ".down.sql"):
			for scanner.Scan() {
				dropTableMatch := dropTableRegex.FindStringSubmatch(scanner.Text())
				if len(dropTableMatch) == 3 {
					dropTables[dropTableMatch[2]] = migrationFile
					dropList = append(dropList, dropTableMatch[2])
				}
			}
		default:
			panic(fmt.Errorf("invalid migration file %s", migrationFile))
		}
	}

	// Check the lists match
	for tableName, migrationFile := range createTables {
		if _, isDropped := dropTables[tableName]; !isDropped {
			panic(fmt.Errorf(`table "%s" created in migration file %s is not dropped`, tableName, migrationFile))
		}
	}

	reversedDropTables = make([]string, len(dropList))
	for i := 0; i < len(dropList); i++ {
		reversedDropTables[len(dropList)-i-1] = dropList[i]
	}
	return reversedDropTables
}

func clearAllData(utDBName string) {
	dbConn, err := sql.Open("postgres", dbDSN(utDBName))
	requireNoError(err)
	defer dbConn.Close()

	for _, table := range reversedDropTables {
		_, err := dbConn.Exec(fmt.Sprintf(`DELETE FROM "%s"`, table))
		requireNoError(err)
	}
}

func dbDSN(dbname string) string {
	return fmt.Sprintf("postgres://postgres:my-secret@localhost:5432/%s?sslmode=disable", dbname)
}

// Used for unit tests throughout the project that want to test against a real DB
// - This version uses PostgreSQL
// - This version validates our migrations contain the same list of CREATE TABLE as DROP TABLE
// - This version clears the tables between runs, rather than dropping the DB
func NewUnitTestPersistence(ctx context.Context, suite string) (p Persistence, cleanup func(), err error) {

	utDBName := utDBPrefix + suite

	log.L(ctx).Infof("Unit test Postgres DB: %s", dbDSN(utDBName))

	autoMigrate := false
	if len(reversedDropTables) == 0 {
		autoMigrate = true

		// All subsequent calls in this Go process will just use the DB we create in this go
		reversedDropTables = buildReversedTableListFromMigrations()

		// Create the database - using the super user
		adminDB, err := sql.Open("postgres", dbDSN("postgres"))
		requireNoError(err)
		_, err = adminDB.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH(FORCE)`, utDBName))
		requireNoError(err)
		_, err = adminDB.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, utDBName))
		requireNoError(err)
		err = adminDB.Close()
		requireNoError(err)
	}
	p, err = newPostgresProvider(ctx, &pldconf.DBConfig{
		Type: "postgres",
		Postgres: pldconf.PostgresConfig{
			SQLDBConfig: pldconf.SQLDBConfig{
				DSN:           dbDSN(utDBName),
				MigrationsDir: migrationsDirRelative,
				AutoMigrate:   &autoMigrate,
				DebugQueries:  true,
			},
		},
	})
	requireNoError(err)
	return p, func() {
		if recovered := recover(); recovered != nil {
			fmt.Fprintf(os.Stderr, "not cleaning up DB '%s' due to panic: %s\n", utDBName, err)
			panic(recovered)
		}
		clearAllData(utDBName)
	}, err
}
