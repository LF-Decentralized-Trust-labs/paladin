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
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/stretchr/testify/assert"
)

func TestMigrateUpDown(t *testing.T) {

	ctx := context.Background()

	// Up runs as part of the init
	p, done, err := NewUnitTestPersistence(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, p.DB())
	defer done()

	// Get the migration drive directly using the internal function, to run Down()
	err = p.(*provider).runMigration(ctx, func(m *migrate.Migrate) error { return m.Down() })
	assert.NoError(t, err)

}

func TestPersistenceTypes(t *testing.T) {
	ctx := context.Background()

	_, err := NewPersistence(ctx, &Config{})
	assert.Regexp(t, "PD010201", err)

	_, err = NewPersistence(ctx, &Config{Type: "sqlite"})
	assert.Regexp(t, "PD010201", err)

	_, err = NewPersistence(ctx, &Config{Type: "postgres"})
	assert.Regexp(t, "PD010201", err)

	// Different error for wrong case
	_, err = NewPersistence(ctx, &Config{Type: "wrong"})
	assert.Regexp(t, "PD010200.*wrong", err)

}
