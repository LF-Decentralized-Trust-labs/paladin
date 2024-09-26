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

package txmgr

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"gorm.io/gorm"
)

type queryWrapper[PT, T any] struct {
	p           persistence.Persistence
	table       string
	defaultSort string
	filters     filters.FieldMap
	query       *query.QueryJSON
	finalize    func(db *gorm.DB) *gorm.DB
	mapResult   func(*PT) (*T, error)
}

func stringOrEmpty(ps *string) string {
	if ps == nil {
		return ""
	}
	return *ps
}

func notEmptyOrNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func int64OrZero(pi *int64) int64 {
	if pi == nil {
		return 0
	}
	return *pi
}

func checkLimitSet(ctx context.Context, jq *query.QueryJSON) error {
	if jq.Limit == nil || *jq.Limit <= 0 {
		return i18n.NewError(ctx, msgs.MsgTxMgrQueryLimitRequired)
	}
	return nil
}

func (qw *queryWrapper[PT, T]) run(ctx context.Context, dbTX *gorm.DB) ([]*T, error) {
	if err := checkLimitSet(ctx, qw.query); err != nil {
		return nil, err
	}
	if len(qw.query.Sort) == 0 {
		// By default return the newest in descending order
		qw.query.Sort = []string{qw.defaultSort}
	}

	// Build the query
	var dbResults []*PT
	if dbTX == nil {
		dbTX = qw.p.DB()
	}
	q := filters.BuildGORM(ctx,
		qw.query,
		dbTX.Table(qw.table).WithContext(ctx),
		qw.filters)
	if qw.finalize != nil {
		q = qw.finalize(q)
	}
	err := q.Find(&dbResults).Error
	if err != nil {
		return nil, err
	}

	finalResults := make([]*T, len(dbResults))
	for i, r := range dbResults {
		if finalResults[i], err = qw.mapResult(r); err != nil {
			return nil, err
		}
	}
	return finalResults, nil
}
