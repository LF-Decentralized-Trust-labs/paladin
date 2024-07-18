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

package statestore

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func TestGetSchemaNotFoundNil(t *testing.T) {
	ctx, ss, mdb, done := newDBMockStateStore(t)
	defer done()

	mdb.ExpectQuery("SELECT.*schemas").WillReturnRows(sqlmock.NewRows([]string{}))

	s, err := ss.GetSchema(ctx, "domain1", HashIDKeccak(([]byte)("test")), false)
	assert.NoError(t, err)
	assert.Nil(t, s)
}

func TestGetSchemaNotFoundError(t *testing.T) {
	ctx, ss, mdb, done := newDBMockStateStore(t)
	defer done()

	mdb.ExpectQuery("SELECT.*schemas").WillReturnRows(sqlmock.NewRows([]string{}))

	_, err := ss.GetSchema(ctx, "domain1", HashIDKeccak(([]byte)("test")), true)
	assert.Regexp(t, "PD010106", err)
}

func TestGetSchemaInvalidType(t *testing.T) {
	ctx, ss, mdb, done := newDBMockStateStore(t)
	defer done()

	mdb.ExpectQuery("SELECT.*schemas").WillReturnRows(sqlmock.NewRows(
		[]string{"type"},
	).AddRow("wrong"))

	_, err := ss.GetSchema(ctx, "domain1", HashIDKeccak(([]byte)("test")), true)
	assert.Regexp(t, "PD010103.*wrong", err)
}
