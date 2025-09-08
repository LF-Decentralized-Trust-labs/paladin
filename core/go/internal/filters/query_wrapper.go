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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"gorm.io/gorm"
)

type QueryWrapper[PT, T any] struct {
	P           persistence.Persistence
	Table       string
	DefaultSort string
	Filters     FieldMap
	Query       *query.QueryJSON
	Finalize    func(db *gorm.DB) *gorm.DB
	MapResult   func(*PT) (*T, error)
}

func CheckLimitSet(ctx context.Context, jq *query.QueryJSON) error {
	if jq.Limit == nil || *jq.Limit <= 0 {
		return i18n.NewError(ctx, msgs.MsgFiltersQueryLimitRequired)
	}
	return nil
}

func (qw *QueryWrapper[PT, T]) Run(ctx context.Context, dbTX persistence.DBTX) ([]*T, error) {
	if err := CheckLimitSet(ctx, qw.Query); err != nil {
		return nil, err
	}
	if len(qw.Query.Sort) == 0 {
		// By default return the newest in descending order
		qw.Query.Sort = []string{qw.DefaultSort}
	}

	// Build the query
	var dbResults []*PT
	if dbTX == nil {
		dbTX = qw.P.NOTX()
	}
	q := dbTX.DB().WithContext(ctx)
	if qw.Table != "" {
		q = q.Table(qw.Table)
	}
	q = BuildGORM(ctx, qw.Query, q, qw.Filters)
	if qw.Finalize != nil {
		q = qw.Finalize(q)
	}
	err := q.Find(&dbResults).Error
	if err != nil {
		return nil, err
	}

	finalResults := make([]*T, len(dbResults))
	for i, r := range dbResults {
		if finalResults[i], err = qw.MapResult(r); err != nil {
			return nil, err
		}
	}
	return finalResults, nil
}
