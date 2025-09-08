/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package filters

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testFilters = FieldMap{
	"id":      UUIDField("id"),
	"created": TimestampField("created"),
	"name":    StringField("name"),
}

type testQueryObject struct {
	ID      uuid.UUID          `gorm:"column:id;primaryKey"`
	Created pldtypes.Timestamp `gorm:"column:created"`
	Name    string             `gorm:"column:name"`
	Parent  *testQueryObject   `gorm:"foreignKey:Parent;references:ID"`
}

type testOutputObject struct {
	ID      uuid.UUID          `json:"id"`
	Created pldtypes.Timestamp `json:"created"`
	Name    string             `json:"name"`
}

func (tq testQueryObject) TableName() string {
	return "test_object"
}

func TestQueryWrapperLimitRequired(t *testing.T) {
	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	qw := &QueryWrapper[testQueryObject, testOutputObject]{
		P:           p.P,
		DefaultSort: "-created",
		Filters:     testFilters,
		Query:       query.NewQueryBuilder().Query(),
	}

	_, err = qw.Run(context.Background(), nil)
	assert.Regexp(t, "PD010721", err)
}

func TestQueryWrapperMapFail(t *testing.T) {
	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	qw := &QueryWrapper[testQueryObject, testOutputObject]{
		P:           p.P,
		DefaultSort: "-created",
		Filters:     testFilters,
		Query:       query.NewQueryBuilder().Limit(1).Query(),
	}
	qw.MapResult = func(pt *testQueryObject) (*testOutputObject, error) {
		return nil, fmt.Errorf("pop")
	}

	p.Mock.ExpectQuery("SELECT.*test_object").WillReturnRows(
		sqlmock.NewRows([]string{"id", "created", "name"}).
			AddRow(uuid.New(), pldtypes.TimestampNow(), "sally"),
	)

	_, err = qw.Run(context.Background(), nil)
	assert.Regexp(t, "pop", err)
}

func TestQueryWrapperFinalizeOK(t *testing.T) {
	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	qw := &QueryWrapper[testQueryObject, testOutputObject]{
		P:           p.P,
		DefaultSort: "-created",
		Filters:     testFilters,
		Query:       query.NewQueryBuilder().Limit(1).Query(),
		Finalize: func(db *gorm.DB) *gorm.DB {
			return db.Joins("Parent")
		},
		Table: "test_object_1",
	}
	qw.MapResult = func(pt *testQueryObject) (*testOutputObject, error) {
		return &testOutputObject{
			ID:      pt.ID,
			Created: pt.Created,
			Name:    pt.Name,
		}, nil
	}

	p.Mock.ExpectQuery("SELECT.*test_object_1.*LEFT JOIN.*test_object_1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "created", "name"}).
			AddRow(uuid.New(), pldtypes.TimestampNow(), "sally"),
	)

	objs, err := qw.Run(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, objs, 1)
	require.NotNil(t, objs[0])
}

func TestQueryWrapperQueryFail(t *testing.T) {
	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	qw := &QueryWrapper[testQueryObject, testOutputObject]{
		P:           p.P,
		DefaultSort: "-created",
		Filters:     testFilters,
		Query:       query.NewQueryBuilder().Limit(1).Query(),
	}

	p.Mock.ExpectQuery("SELECT.*test_object").WillReturnError(fmt.Errorf("pop"))

	_, err = qw.Run(context.Background(), nil)
	assert.Regexp(t, "pop", err)

}
