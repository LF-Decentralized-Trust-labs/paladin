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

package query

import (
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

type addOns func(op *Op)

var (
	// AddOns is a map of add-ons to be used in the query
	Not             addOns = func(op *Op) { op.Not = true }
	CaseInsensitive addOns = func(op *Op) { op.CaseInsensitive = true }
	CaseSensitive   addOns = func(op *Op) { op.CaseInsensitive = false }
)

// QueryBuilder defines an interface for building queries
type QueryBuilder interface {

	// Limit sets the limit of the query
	Limit(limit int) QueryBuilder

	// Sort adds a sort filter to the query
	Sort(fields ...string) QueryBuilder

	// Equal adds an equal filter to the query
	Equal(field string, value any, adds ...addOns) QueryBuilder

	// NotEqual adds a not equal filter to the query
	NotEqual(field string, value any, adds ...addOns) QueryBuilder

	// GreaterThan adds a greater than filter to the query
	GreaterThan(field string, value any) QueryBuilder

	// GreaterThanOrEqual adds a greater than or equal filter to the query
	GreaterThanOrEqual(field string, value any) QueryBuilder

	// LessThan adds a less than filter to the query
	LessThan(field string, value any) QueryBuilder

	// LessThanOrEqual adds a less than or equal filter to the query
	LessThanOrEqual(field string, value any) QueryBuilder

	// In adds an in filter to the query
	In(field string, values []any, adds ...addOns) QueryBuilder

	// NotIn adds a not in filter to the query
	NotIn(field string, values []any, adds ...addOns) QueryBuilder

	// Null adds an is null filter to the query
	Null(field string) QueryBuilder

	// NotNull adds an is not null filter to the query
	NotNull(field string) QueryBuilder

	// Like adds a like filter to the query
	Like(field string, value any) QueryBuilder

	// NotLike adds a not like filter to the query
	NotLike(field string, value any) QueryBuilder

	// Or creates an OR condition between multiple queries
	Or(...QueryBuilder) QueryBuilder

	// Query returns the query
	Query() *QueryJSON
}

// Ensure queryBuilderImpl implements QueryBuilder
var _ QueryBuilder = &queryBuilderImpl{}

type queryBuilderImpl struct {
	rootQuery  *QueryJSON
	statements *Statements
}

func NewQueryBuilder() QueryBuilder {
	qj := &QueryJSON{}
	return qj.ToBuilder()
}

// Limit sets the limit of the query
func (qb *queryBuilderImpl) Limit(limit int) QueryBuilder {
	qb.rootQuery.Limit = &limit
	return qb
}

// Sort adds a sort filter to the query
func (qb *queryBuilderImpl) Sort(fields ...string) QueryBuilder {
	qb.rootQuery.Sort = append(qb.rootQuery.Sort, fields...)
	return qb
}

func buildOp(field string, adds ...addOns) *Op {
	op := &Op{
		Field: field,
	}
	for _, add := range adds {
		add(op)
	}
	return op
}

func buildSingleValueOp(field string, value any, adds ...addOns) *OpSingleVal {
	return &OpSingleVal{
		Op:    *buildOp(field, adds...),
		Value: pldtypes.JSONString(value),
	}
}

func buildMultiValueOp(field string, values []any, adds ...addOns) *OpMultiVal {
	omv := &OpMultiVal{
		Op: *buildOp(field, adds...),
	}
	for _, value := range values {
		omv.Values = append(omv.Values, pldtypes.JSONString(value))
	}
	return omv
}

// Equal adds an equal filter to the query
func (qb *queryBuilderImpl) Equal(field string, value any, adds ...addOns) QueryBuilder {
	qb.statements.Eq = append(qb.statements.Eq, buildSingleValueOp(field, value, adds...))
	return qb
}

// NotEqual adds a not equal filter to the query
func (qb *queryBuilderImpl) NotEqual(field string, value any, adds ...addOns) QueryBuilder {
	qb.statements.NEq = append(qb.statements.NEq, buildSingleValueOp(field, value, adds...))
	return qb
}

// GreaterThan adds a greater than filter to the query
func (qb *queryBuilderImpl) GreaterThan(field string, value any) QueryBuilder {
	qb.statements.GT = append(qb.statements.GT, buildSingleValueOp(field, value))
	return qb
}

// GreaterThanOrEqual adds a greater than or equal filter to the query
func (qb *queryBuilderImpl) GreaterThanOrEqual(field string, value any) QueryBuilder {
	qb.statements.GTE = append(qb.statements.GTE, buildSingleValueOp(field, value))
	return qb
}

// LessThan adds a less than filter to the query
func (qb *queryBuilderImpl) LessThan(field string, value any) QueryBuilder {
	qb.statements.LT = append(qb.statements.LT, buildSingleValueOp(field, value))
	return qb
}

// LessThanOrEqual adds a less than or equal filter to the query
func (qb *queryBuilderImpl) LessThanOrEqual(field string, value any) QueryBuilder {
	qb.statements.LTE = append(qb.statements.LTE, buildSingleValueOp(field, value))
	return qb
}

// In adds an in filter to the query
func (qb *queryBuilderImpl) In(field string, values []any, adds ...addOns) QueryBuilder {
	qb.statements.In = append(qb.statements.In, buildMultiValueOp(field, values, adds...))
	return qb
}

// NotIn adds a not in filter to the query
func (qb *queryBuilderImpl) NotIn(field string, values []any, adds ...addOns) QueryBuilder {
	qb.statements.NIn = append(qb.statements.NIn, buildMultiValueOp(field, values, adds...))
	return qb
}

// Null adds an is null filter to the query
func (qb *queryBuilderImpl) Null(field string) QueryBuilder {
	qb.statements.Null = append(qb.statements.Null, buildOp(field))
	return qb
}

// NotNull adds an is not null filter to the query
func (qb *queryBuilderImpl) NotNull(field string) QueryBuilder {
	qb.statements.Null = append(qb.statements.Null, buildOp(field, Not))
	return qb
}

// Like adds a like filter to the query
func (qb *queryBuilderImpl) Like(field string, value any) QueryBuilder {
	qb.statements.Like = append(qb.statements.Like, buildSingleValueOp(field, value))
	return qb
}

// NotLike adds a not like filter to the query
func (qb *queryBuilderImpl) NotLike(field string, value any) QueryBuilder {
	qb.statements.Like = append(qb.statements.Like, buildSingleValueOp(field, value, Not))
	return qb
}

// Or creates an OR condition between multiple queries
func (qb *queryBuilderImpl) Or(q ...QueryBuilder) QueryBuilder {
	for _, child := range q {
		qb.statements.Or = append(qb.statements.Or, child.(*queryBuilderImpl).statements)
	}
	return qb
}

// Query returns the query
func (qb *queryBuilderImpl) Query() *QueryJSON {
	return qb.rootQuery
}
