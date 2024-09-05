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
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

// Used for unit tests throughout the project that want to test against a real DB
// - This version uses PostgreSQL
func NewUnitTestPersistence(ctx context.Context) (p Persistence, cleanup func(), err error) {
	dbURL := func(dbname string) string {
		return fmt.Sprintf("postgres://postgres:my-secret@localhost:5432/%s?sslmode=disable", dbname)
	}
	utdbName := "ut_" + uuid.New().String()
	log.L(ctx).Infof("Unit test Postgres DB: %s", utdbName)

	// First create the database - using the super user
	adminDB, err := sql.Open("postgres", dbURL("postgres"))
	if err == nil {
		_, err = adminDB.Exec(fmt.Sprintf(`CREATE DATABASE "%s";`, utdbName))
	}
	if err == nil {
		err = adminDB.Close()
	}
	if err == nil {
		p, err = newPostgresProvider(ctx, &Config{
			Type: "postgres",
			Postgres: PostgresConfig{
				SQLDBConfig: SQLDBConfig{
					URI:           dbURL(utdbName),
					MigrationsDir: "../../db/migrations/postgres",
					AutoMigrate:   confutil.P(true),
					DebugQueries:  true,
				},
			},
		})
	}
	return p, func() {
		adminDB, err := sql.Open("postgres", dbURL("postgres"))
		if err == nil {
			_, _ = adminDB.Exec(fmt.Sprintf(`DROP DATABASE "%s" WITH(FORCE);`, utdbName))
			adminDB.Close()
		}
	}, err
}
