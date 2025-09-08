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

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

type QueryJSON struct {
	Statements
	Limit *int     `docstruct:"QueryJSON" json:"limit,omitempty"`
	Sort  []string `docstruct:"QueryJSON" json:"sort,omitempty"`
}

// Note if ItemsResultTyped below might be preferred for new APIs (if you are able to adopt always-return {items:[]} style)
type FilterResultsWithCount struct {
	Count int64       `docstruct:"FilterResultsWithCount" json:"count"`
	Total *int64      `docstruct:"FilterResultsWithCount" json:"total,omitempty"` // omitted if a count was not calculated (AlwaysPaginate enabled, and count not specified)
	Items interface{} `docstruct:"FilterResultsWithCount" json:"items"`
}

type ItemsResultTyped[T any] struct {
	Count int    `docstruct:"ItemsResultTyped" json:"count"`
	Total *int64 `docstruct:"ItemsResultTyped" json:"total,omitempty"` // omitted if a count was not calculated (AlwaysPaginate enabled, and count not specified)
	Items []T    `docstruct:"ItemsResultTyped" json:"items"`
}

type Op struct {
	Not             bool   `docstruct:"Op" json:"not,omitempty"`
	CaseInsensitive bool   `docstruct:"Op" json:"caseInsensitive,omitempty"`
	Field           string `docstruct:"Op" json:"field,omitempty"`
}

type OpSingleVal struct {
	Op
	Value pldtypes.RawJSON `docstruct:"OpSingleVal" json:"value,omitempty"`
}

type OpMultiVal struct {
	Op
	Values []pldtypes.RawJSON `docstruct:"OpMultiVal" json:"values,omitempty"`
}

type Statements struct {
	Or []*Statements `docstruct:"Statements" json:"or,omitempty"`
	Ops
}

type Ops struct {
	Equal              []*OpSingleVal `docstruct:"Ops" json:"equal,omitempty"`
	Eq                 []*OpSingleVal `docstruct:"Ops" json:"eq,omitempty"`  // short name
	NEq                []*OpSingleVal `docstruct:"Ops" json:"neq,omitempty"` // negated short name
	Like               []*OpSingleVal `docstruct:"Ops" json:"like,omitempty"`
	LessThan           []*OpSingleVal `docstruct:"Ops" json:"lessThan,omitempty"`
	LT                 []*OpSingleVal `docstruct:"Ops" json:"lt,omitempty"` // short name
	LessThanOrEqual    []*OpSingleVal `docstruct:"Ops" json:"lessThanOrEqual,omitempty"`
	LTE                []*OpSingleVal `docstruct:"Ops" json:"lte,omitempty"` // short name
	GreaterThan        []*OpSingleVal `docstruct:"Ops" json:"greaterThan,omitempty"`
	GT                 []*OpSingleVal `docstruct:"Ops" json:"gt,omitempty"` // short name
	GreaterThanOrEqual []*OpSingleVal `docstruct:"Ops" json:"greaterThanOrEqual,omitempty"`
	GTE                []*OpSingleVal `docstruct:"Ops" json:"gte,omitempty"` // short name
	In                 []*OpMultiVal  `docstruct:"Ops" json:"in,omitempty"`
	NIn                []*OpMultiVal  `docstruct:"Ops" json:"nin,omitempty"` // negated short name
	Null               []*Op          `docstruct:"Ops" json:"null,omitempty"`
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
