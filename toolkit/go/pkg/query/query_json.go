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

package query

import (
	"encoding/json"

	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type QueryJSON struct {
	Statements
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
	Count int    `json:"count"`
	Total *int64 `json:"total,omitempty"` // omitted if a count was not calculated (AlwaysPaginate enabled, and count not specified)
	Items []T    `json:"items"`
}

type Op struct {
	Not             bool   `json:"not,omitempty"`
	CaseInsensitive bool   `json:"caseInsensitive,omitempty"`
	Field           string `json:"field,omitempty"`
}

type OpSingleVal struct {
	Op
	Value tktypes.RawJSON `json:"value,omitempty"`
}

type OpMultiVal struct {
	Op
	Values []tktypes.RawJSON `json:"values,omitempty"`
}

type Statements struct {
	Or []*Statements `json:"or,omitempty"`
	Ops
}

type Ops struct {
	Equal              []*OpSingleVal `json:"equal,omitempty"`
	Eq                 []*OpSingleVal `json:"eq,omitempty"`  // short name
	NEq                []*OpSingleVal `json:"neq,omitempty"` // negated short name
	Like               []*OpSingleVal `json:"like,omitempty"`
	LessThan           []*OpSingleVal `json:"lessThan,omitempty"`
	LT                 []*OpSingleVal `json:"lt,omitempty"` // short name
	LessThanOrEqual    []*OpSingleVal `json:"lessThanOrEqual,omitempty"`
	LTE                []*OpSingleVal `json:"lte,omitempty"` // short name
	GreaterThan        []*OpSingleVal `json:"greaterThan,omitempty"`
	GT                 []*OpSingleVal `json:"gt,omitempty"` // short name
	GreaterThanOrEqual []*OpSingleVal `json:"greaterThanOrEqual,omitempty"`
	GTE                []*OpSingleVal `json:"gte,omitempty"` // short name
	In                 []*OpMultiVal  `json:"in,omitempty"`
	NIn                []*OpMultiVal  `json:"nin,omitempty"` // negated short name
	Null               []*Op          `json:"null,omitempty"`
}

func (jq *QueryJSON) String() string {
	b, _ := jq.JSON()
	return string(b)
}

func (jq *QueryJSON) JSON() ([]byte, error) {
	return json.Marshal(jq)
}

// Converts to a builder - which will add to the underlying query structure (cannot remove existing elements)
func (jq *QueryJSON) ToBuilder() QueryBuilder {
	return &queryBuilderImpl{
		rootQuery:  jq,
		statements: &jq.Statements,
	}
}
