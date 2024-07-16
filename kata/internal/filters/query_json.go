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

package filters

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"
)

type QueryJSON struct {
	FilterJSON
	Limit *int     `json:"limit,omitempty"`
	Sort  []string `json:"sort,omitempty"`
}

// Note if ItemsResultTyped below might be preferred for new APIs (if you are able to adopt always-return {items:[]} style)
type FilterResultsWithCount struct {
	Count int64       `json:"count"`
	Total *int64      `json:"total,omitempty"` // omitted if a count was not calculated (AlwaysPaginate enabled, and count not specified)
	Items interface{} `json:"items"`
}

type ItemsResultTyped[T any] struct {
	Count int    `ffstruct:"CollectionResults" json:"count"`
	Total *int64 `ffstruct:"CollectionResults" json:"total,omitempty"` // omitted if a count was not calculated (AlwaysPaginate enabled, and count not specified)
	Items []T    `ffstruct:"CollectionResults" json:"items"`
}

type FilterJSONBase struct {
	Not             bool   `json:"not,omitempty"`
	CaseInsensitive bool   `json:"caseInsensitive,omitempty"`
	Field           string `json:"field,omitempty"`
}

type FilterJSONKeyValue struct {
	FilterJSONBase
	Value json.RawMessage `json:"value,omitempty"`
}

type FilterJSONKeyValues struct {
	FilterJSONBase
	Values []json.RawMessage `json:"values,omitempty"`
}

type FilterJSON struct {
	Or []*FilterJSON `json:"or,omitempty"`
	FilterJSONOps
}

type FilterJSONOps struct {
	Equal              []*FilterJSONKeyValue  `json:"equal,omitempty"`
	Eq                 []*FilterJSONKeyValue  `json:"eq,omitempty"`  // short name
	NEq                []*FilterJSONKeyValue  `json:"neq,omitempty"` // negated short name
	Like               []*FilterJSONKeyValue  `json:"like,omitempty"`
	LessThan           []*FilterJSONKeyValue  `json:"lessThan,omitempty"`
	LT                 []*FilterJSONKeyValue  `json:"lt,omitempty"` // short name
	LessThanOrEqual    []*FilterJSONKeyValue  `json:"lessThanOrEqual,omitempty"`
	LTE                []*FilterJSONKeyValue  `json:"lte,omitempty"` // short name
	GreaterThan        []*FilterJSONKeyValue  `json:"greaterThan,omitempty"`
	GT                 []*FilterJSONKeyValue  `json:"gt,omitempty"` // short name
	GreaterThanOrEqual []*FilterJSONKeyValue  `json:"greaterThanOrEqual,omitempty"`
	GTE                []*FilterJSONKeyValue  `json:"gte,omitempty"` // short name
	In                 []*FilterJSONKeyValues `json:"in,omitempty"`
	NIn                []*FilterJSONKeyValues `json:"nin,omitempty"` // negated short name
	Null               []*FilterJSONBase      `json:"null,omitempty"`
}

func (qj *QueryJSON) Build(ctx context.Context, db *gorm.DB, fieldList FieldList) *gorm.DB {
	qb := &queryBuilder{
		ctx: ctx,
		// We can't assume anything about the db passed in - if it's a clone (internal concept
		// in GORM I can't work out how to detect), then it will aggregate WHERE clauses
		// and cause us to do all kinds of wonky nesting.
		// So use this function to get a clean session to do our nested db.Where() clauses against.
		rootDB:     db.Session(&gorm.Session{DryRun: true, SkipDefaultTransaction: true}),
		jsonFilter: qj,
		fieldList:  fieldList,
	}
	return qb.build(db)
}
