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

package statemgr

import (
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSchemaNotFoundNil(t *testing.T) {
	ctx, ss, mdb, _, done := newDBMockStateManager(t)
	defer done()

	mdb.ExpectQuery("SELECT.*schemas").WillReturnRows(sqlmock.NewRows([]string{}))

	s, err := ss.GetSchemaByID(ctx, ss.p.NOTX(), "domain1", pldtypes.Bytes32Keccak(([]byte)("test")), false)
	require.NoError(t, err)
	assert.Nil(t, s)
}

func mockGetSchemaOK(mdb sqlmock.Sqlmock) {
	mdb.ExpectQuery("SELECT.*schemas").WillReturnRows(sqlmock.NewRows([]string{
		"id",
		"type",
		"domain",
		"definition",
	}).AddRow(
		pldtypes.RandBytes32(),
		pldapi.SchemaTypeABI.Enum(),
		"domain1",
		`{
		  "type": "tuple",
		  "internalType": "struct MyType;",
		  "components": []
		}`,
	))
}

func TestGetSchemaK(t *testing.T) {
	ctx, ss, mdb, _, done := newDBMockStateManager(t)
	defer done()

	mockGetSchemaOK(mdb)

	s, err := ss.GetSchemaByID(ctx, ss.p.NOTX(), "domain1", pldtypes.Bytes32Keccak(([]byte)("test")), false)
	require.NoError(t, err)
	assert.NotNil(t, s)
}

func TestGetSchemaNotFoundError(t *testing.T) {
	ctx, ss, mdb, _, done := newDBMockStateManager(t)
	defer done()

	mdb.ExpectQuery("SELECT.*schemas").WillReturnRows(sqlmock.NewRows([]string{}))

	_, err := ss.GetSchemaByID(ctx, ss.p.NOTX(), "domain1", pldtypes.Bytes32Keccak(([]byte)("test")), true)
	assert.Regexp(t, "PD010106", err)
}

func TestGetSchemaInvalidType(t *testing.T) {
	ctx, ss, _, _, done := newDBMockStateManager(t)
	defer done()

	_, err := ss.restoreSchema(ctx, &pldapi.Schema{Type: pldtypes.Enum[pldapi.SchemaType]("wrong")})
	assert.Regexp(t, "PD010103.*wrong", err)
}

func TestListSchemasListIDsFail(t *testing.T) {
	ctx, ss, mdb, _, done := newDBMockStateManager(t)
	defer done()

	mdb.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("pop"))

	_, err := ss.ListSchemas(ctx, ss.p.NOTX(), "domain1")
	assert.Regexp(t, "pop", err)
}

func TestListSchemasGetFullSchemaFail(t *testing.T) {
	ctx, ss, mdb, _, done := newDBMockStateManager(t)
	defer done()

	id := pldtypes.Bytes32Keccak(([]byte)("test"))
	mdb.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(
		id.String(),
	))
	mdb.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("pop"))

	_, err := ss.ListSchemas(ctx, ss.p.NOTX(), "domain1")
	assert.Regexp(t, "pop", err)
}
