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
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSchemaNotFoundNil(t *testing.T) {
	ctx, ss, mdb, done := newDBMockStateStore(t)
	defer done()

	mdb.ExpectQuery("SELECT.*schemas").WillReturnRows(sqlmock.NewRows([]string{}))

	s, err := ss.GetSchema(ctx, "domain1", tktypes.Bytes32Keccak(([]byte)("test")).String(), false)
	require.NoError(t, err)
	assert.Nil(t, s)
}

func TestGetSchemaNotFoundError(t *testing.T) {
	ctx, ss, mdb, done := newDBMockStateStore(t)
	defer done()

	mdb.ExpectQuery("SELECT.*schemas").WillReturnRows(sqlmock.NewRows([]string{}))

	_, err := ss.GetSchema(ctx, "domain1", tktypes.Bytes32Keccak(([]byte)("test")).String(), true)
	assert.Regexp(t, "PD010106", err)
}

func TestGetSchemaInvalidType(t *testing.T) {
	ctx, ss, mdb, done := newDBMockStateStore(t)
	defer done()

	mdb.ExpectQuery("SELECT.*schemas").WillReturnRows(sqlmock.NewRows(
		[]string{"type"},
	).AddRow("wrong"))

	_, err := ss.GetSchema(ctx, "domain1", tktypes.Bytes32Keccak(([]byte)("test")).String(), true)
	assert.Regexp(t, "PD010103.*wrong", err)
}

func TestGetSchemaInvalidID(t *testing.T) {
	ctx, ss, _, done := newDBMockStateStore(t)
	defer done()

	_, err := ss.GetSchema(ctx, "domain1", "wrong", true)
	assert.Regexp(t, "PD020007", err)
}

func TestListSchemasListIDsFail(t *testing.T) {
	ctx, ss, mdb, done := newDBMockStateStore(t)
	defer done()

	mdb.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("pop"))

	_, err := ss.ListSchemas(ctx, "domain1")
	assert.Regexp(t, "pop", err)
}

func TestListSchemasGetFullSchemaFail(t *testing.T) {
	ctx, ss, mdb, done := newDBMockStateStore(t)
	defer done()

	id := tktypes.Bytes32Keccak(([]byte)("test"))
	mdb.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(
		id.String(),
	))
	mdb.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("pop"))

	_, err := ss.ListSchemas(ctx, "domain1")
	assert.Regexp(t, "pop", err)
}
