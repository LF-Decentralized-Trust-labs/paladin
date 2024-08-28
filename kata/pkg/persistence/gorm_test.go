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
	"testing"

	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/stretchr/testify/assert"
)

func TestGormInitFail(t *testing.T) {

	// We can make SQLite fail by pointing it at a directory
	_, err := newSQLiteProvider(context.Background(), &Config{
		Type: "sqlite",
		SQLite: SQLiteConfig{
			SQLDBConfig: SQLDBConfig{
				URI: "file://" + t.TempDir(),
			},
		},
	})
	assert.Regexp(t, "PD010202", err)

}

func TestGormMigrationMissingDir(t *testing.T) {

	// We can make migration fail by pointing it at a file
	tempFile := t.TempDir() + "/wrong"
	err := os.WriteFile(tempFile, []byte{}, 0664)
	assert.NoError(t, err)
	_, err = newSQLiteProvider(context.Background(), &Config{
		Type: "sqlite",
		SQLite: SQLiteConfig{
			SQLDBConfig: SQLDBConfig{
				URI:           ":memory:",
				AutoMigrate:   confutil.P(true),
				MigrationsDir: tempFile,
			},
		},
	})
	assert.Regexp(t, "PD010203", err)

}

func TestGormMigrationFail(t *testing.T) {

	// We can make migration fail by pointing it at a file
	tempFile := t.TempDir() + "/wrong"
	err := os.WriteFile(tempFile, []byte{}, 0664)
	assert.NoError(t, err)
	_, err = newSQLiteProvider(context.Background(), &Config{
		Type: "sqlite",
		SQLite: SQLiteConfig{
			SQLDBConfig: SQLDBConfig{
				URI:         ":memory:",
				AutoMigrate: confutil.P(true),
			},
		},
	})
	assert.Regexp(t, "PD010203", err)

}
