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
	"reflect"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func TestSQLiteProvider(t *testing.T) {
	p := &sqliteProvider{}
	assert.Equal(t, "sqlite", p.DBName())
	assert.Equal(t, "*sqlite.Dialector", reflect.TypeOf(p.Open("")).String())
	db, _, _ := sqlmock.New()
	_, err := p.GetMigrationDriver(db)
	assert.Error(t, err)
}
