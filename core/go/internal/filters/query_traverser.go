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
	"database/sql/driver"
	"fmt"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
)

type Traverser[T any] interface {
	T() T
	NewRoot() Traverser[T]
	Error() error
	WithError(err error) Traverser[T]
	Limit(l int) Traverser[T]
	Order(f string) Traverser[T]
	And(ot T) Traverser[T]
	BuildOr(ot ...T) Traverser[T]
	IsEqual(e *query.OpSingleVal, fieldName string, field FieldResolver, testValue driver.Value) Traverser[T]
	IsLike(e *query.OpSingleVal, fieldName string, field FieldResolver, testValue driver.Value) Traverser[T]
	IsNull(e *query.Op, fieldName string, field FieldResolver) Traverser[T]
	IsLessThan(e *query.OpSingleVal, fieldName string, field FieldResolver, testValue driver.Value) Traverser[T]
	IsLessThanOrEqual(e *query.OpSingleVal, fieldName string, field FieldResolver, testValue driver.Value) Traverser[T]
	IsGreaterThan(e *query.OpSingleVal, fieldName string, field FieldResolver, testValue driver.Value) Traverser[T]
	IsGreaterThanOrEqual(e *query.OpSingleVal, fieldName string, field FieldResolver, testValue driver.Value) Traverser[T]
	IsIn(e *query.OpMultiVal, fieldName string, field FieldResolver, testValues []driver.Value) Traverser[T]
}

var allMods = []string{"not", "caseInsensitive"}
var justCaseInsensitive = []string{"caseInsensitive"}

type FieldResolver interface {
	SupportsLIKE() bool
	SQLColumn() string
	SQLValue(ctx context.Context, v pldtypes.RawJSON) (driver.Value, error)
}

// FieldSet is an interface (rather than a simple map) as the function
// provides a way for consumers to know which fields from the total
// possible set are being referenced in a query.
type FieldSet interface {
	ResolverFor(fieldName string) FieldResolver // nil for not found
}

// Simple implementation of FieldSet
type FieldMap map[string]FieldResolver

func (fm FieldMap) ResolverFor(fieldName string) FieldResolver {
	return fm[fieldName]
}

type queryTraverser[T any] struct {
	ctx        context.Context
	jsonFilter *query.QueryJSON
	fieldSet   FieldSet
}

type sortDirection string

const (
	directionAscending  sortDirection = "ASC"
	directionDescending sortDirection = "DESC"
)

type sortField struct {
	fieldName string
	field     FieldResolver
	direction sortDirection
}

func (sf *sortField) sql() string {
	return fmt.Sprintf("%s %s", sf.field.SQLColumn(), sf.direction)
}

func (qt *queryTraverser[T]) traverse(t Traverser[T]) Traverser[T] {
	jf := qt.jsonFilter
	t = qt.BuildAndFilter(t, &jf.Statements)
	if jf.Limit != nil && *jf.Limit > 0 {
		t = t.Limit(*jf.Limit)
	}
	for _, s := range jf.Sort {
		tSortField, err := resolveSortField(qt.ctx, qt.fieldSet, s)
		if err != nil {
			return t.WithError(err)
		}
		t = t.Order(tSortField.sql())
	}
	return t
}

func resolveSortField(ctx context.Context, fieldSet FieldSet, fieldName string) (*sortField, error) {
	direction := directionAscending
	startEnd := strings.SplitN(fieldName, " ", 2)
	fieldName, isNegated := strings.CutPrefix(startEnd[0], "-")
	if isNegated || (len(startEnd) == 2 && strings.EqualFold(startEnd[1], "desc")) {
		direction = directionDescending
	}
	field, err := resolveField(ctx, fieldSet, fieldName)
	if err != nil {
		return nil, err
	}
	return &sortField{
		fieldName: fieldName,
		field:     field,
		direction: direction,
	}, nil
}

func resolveField(ctx context.Context, fieldSet FieldSet, fieldName string) (FieldResolver, error) {
	field := fieldSet.ResolverFor(fieldName)
	if field != nil {
		return field, nil
	}
	return nil, i18n.NewError(ctx, msgs.MsgFiltersUnknownField, fieldName)
}

func resolveValue(ctx context.Context, fieldName string, field FieldResolver, jsonValue pldtypes.RawJSON) (driver.Value, error) {
	if len(jsonValue) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgFiltersValueMissing, fieldName)
	}
	value, err := field.SQLValue(ctx, jsonValue)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgFiltersJSONValueParseError, fieldName, field, err)
	}
	return value, nil
}

func resolveFieldAndValue(ctx context.Context, fieldSet FieldSet, fieldName string, jsonValue pldtypes.RawJSON) (FieldResolver, driver.Value, error) {
	field, err := resolveField(ctx, fieldSet, fieldName)
	if err != nil {
		return nil, nil, err
	}
	value, err := resolveValue(ctx, fieldName, field, jsonValue)
	if err != nil {
		return nil, nil, err
	}
	return field, value, nil
}

func resolveFieldAndValues(ctx context.Context, fieldSet FieldSet, fieldName string, jsonValues []pldtypes.RawJSON) (FieldResolver, []driver.Value, error) {
	field, err := resolveField(ctx, fieldSet, fieldName)
	if err != nil {
		return nil, nil, err
	}
	values := make([]driver.Value, len(jsonValues))
	for i, jsonValue := range jsonValues {
		if values[i], err = resolveValue(ctx, fieldName, field, jsonValue); err != nil {
			return nil, nil, err
		}
	}
	return field, values, nil
}

func (qt *queryTraverser[T]) addSimpleFilters(t Traverser[T], jf *query.Statements) Traverser[T] {
	for _, e := range joinShortNames(jf.Equal, jf.Eq, jf.NEq) {
		field, testValue, err := resolveFieldAndValue(qt.ctx, qt.fieldSet, e.Field, e.Value)
		if err != nil {
			return t.WithError(err)
		}
		t = t.IsEqual(e, e.Field, field, testValue)
	}
	for _, e := range jf.Like {
		field, testValue, err := resolveFieldAndValue(qt.ctx, qt.fieldSet, e.Field, e.Value)
		if err != nil {
			return t.WithError(err)
		}
		if !field.SupportsLIKE() {
			return t.WithError(i18n.NewError(qt.ctx, msgs.MsgFiltersFieldTypeDoesNotSupportLike, field))
		}
		t = t.IsLike(e, e.Field, field, testValue)
	}
	for _, e := range jf.Null {
		field, err := resolveField(qt.ctx, qt.fieldSet, e.Field)
		if err != nil {
			return t.WithError(err)
		}
		t = t.IsNull(e, e.Field, field)
	}
	return t
}

func joinShortNames(long, short, negated []*query.OpSingleVal) []*query.OpSingleVal {
	res := make([]*query.OpSingleVal, len(long)+len(short)+len(negated))
	copy(res, long)
	copy(res[len(long):], short)
	negs := res[len(short)+len(long):]
	copy(negs, negated)
	for _, n := range negs {
		n.Not = true
	}
	return res
}

func joinInAndNin(in, nin []*query.OpMultiVal) []*query.OpMultiVal {
	res := make([]*query.OpMultiVal, len(in)+len(nin))
	copy(res, in)
	negs := res[len(in):]
	copy(negs, nin)
	for _, n := range negs {
		n.Not = true
	}
	return res
}

func (qt *queryTraverser[T]) BuildAndFilter(t Traverser[T], jf *query.Statements) Traverser[T] {
	t = t.NewRoot()
	t = qt.addSimpleFilters(t, jf)
	if t.Error() != nil {
		return t
	}
	for _, e := range joinShortNames(jf.LessThan, jf.LT, nil) {
		field, testValue, err := resolveFieldAndValue(qt.ctx, qt.fieldSet, e.Field, e.Value)
		if err != nil {
			return t.WithError(err)
		}
		if e.CaseInsensitive || e.Not {
			return t.WithError(i18n.NewError(qt.ctx, msgs.MsgFiltersJSONQueryOpUnsupportedMod, "lessThan", allMods))
		}
		t = t.IsLessThan(e, e.Field, field, testValue)
	}
	for _, e := range joinShortNames(jf.LessThanOrEqual, jf.LTE, nil) {
		field, testValue, err := resolveFieldAndValue(qt.ctx, qt.fieldSet, e.Field, e.Value)
		if err != nil {
			return t.WithError(err)
		}
		if e.CaseInsensitive || e.Not {
			return t.WithError(i18n.NewError(qt.ctx, msgs.MsgFiltersJSONQueryOpUnsupportedMod, "lessThanOrEqual", allMods))
		}
		t = t.IsLessThanOrEqual(e, e.Field, field, testValue)
	}
	for _, e := range joinShortNames(jf.GreaterThan, jf.GT, nil) {
		field, testValue, err := resolveFieldAndValue(qt.ctx, qt.fieldSet, e.Field, e.Value)
		if err != nil {
			return t.WithError(err)
		}
		if e.CaseInsensitive || e.Not {
			return t.WithError(i18n.NewError(qt.ctx, msgs.MsgFiltersJSONQueryOpUnsupportedMod, "greaterThan", allMods))
		}
		t = t.IsGreaterThan(e, e.Field, field, testValue)
	}
	for _, e := range joinShortNames(jf.GreaterThanOrEqual, jf.GTE, nil) {
		field, testValue, err := resolveFieldAndValue(qt.ctx, qt.fieldSet, e.Field, e.Value)
		if err != nil {
			return t.WithError(err)
		}
		if e.CaseInsensitive || e.Not {
			return t.WithError(i18n.NewError(qt.ctx, msgs.MsgFiltersJSONQueryOpUnsupportedMod, "greaterThanOrEqual", allMods))
		}
		t = t.IsGreaterThanOrEqual(e, e.Field, field, testValue)
	}
	for _, e := range joinInAndNin(jf.In, jf.NIn) {
		field, testValues, err := resolveFieldAndValues(qt.ctx, qt.fieldSet, e.Field, e.Values)
		if err != nil {
			return t.WithError(err)
		}
		if e.CaseInsensitive {
			return t.WithError(i18n.NewError(qt.ctx, msgs.MsgFiltersJSONQueryOpUnsupportedMod, "in", justCaseInsensitive))
		}
		t = t.IsIn(e, e.Field, field, testValues)
	}
	if len(jf.Or) > 0 {
		var ors []T
		for _, child := range jf.Or {
			sub := qt.BuildAndFilter(t, child)
			if sub.Error() != nil {
				return sub
			}
			ors = append(ors, sub.T())
		}
		t = t.And(t.BuildOr(ors...).T())
	}
	return t
}
