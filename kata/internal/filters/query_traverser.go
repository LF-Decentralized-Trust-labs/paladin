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

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/types"
)

type Traverser[T any] interface {
	Result() T
	NewRoot() Traverser[T]
	HasError() error
	WithError(err error) Traverser[T]
	Limit(l int) Traverser[T]
	Order(f string) Traverser[T]
	And(ot T) Traverser[T]
	Or(ot T) Traverser[T]
	IsEqual(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[T]
	IsLike(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[T]
	IsNull(e *FilterJSONBase, fieldName string, field FieldResolver) Traverser[T]
	IsLessThan(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[T]
	IsLessThanOrEqual(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[T]
	IsGreaterThan(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[T]
	IsGreaterThanOrEqual(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[T]
	IsIn(e *FilterJSONKeyValues, fieldName string, field FieldResolver, testValues []driver.Value) Traverser[T]
}

var allMods = []string{"not", "caseInsensitive"}
var justCaseInsensitive = []string{"caseInsensitive"}

type FieldResolver interface {
	SQLColumn() string
	SQLValue(ctx context.Context, v types.RawJSON) (driver.Value, error)
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

type queryBuilder[T any] struct {
	ctx        context.Context
	jsonFilter *QueryJSON
	fieldSet   FieldSet
}

func (qb *queryBuilder[T]) build(t Traverser[T]) Traverser[T] {
	jf := qb.jsonFilter
	if jf.Limit != nil && *jf.Limit > 0 {
		t = t.Limit(*jf.Limit)
	}
	for _, s := range jf.Sort {
		tSortField, err := qb.resolveSortField(s)
		if err != nil {
			return t.WithError(err)
		}
		t = t.Order(tSortField)
	}
	return qb.BuildAndFilter(t, &jf.FilterJSON)
}

func (qb *queryBuilder[T]) resolveSortField(fieldName string) (string, error) {
	direction := "asc"
	startEnd := strings.SplitN(fieldName, " ", 2)
	fieldName, isNegated := strings.CutPrefix(startEnd[0], "-")
	if isNegated || (len(startEnd) == 2 && strings.EqualFold(startEnd[1], "desc")) {
		direction = "desc"
	}
	field, err := resolveField(qb.ctx, qb.fieldSet, fieldName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %s", field.SQLColumn(), direction), nil
}

func resolveField(ctx context.Context, fieldSet FieldSet, fieldName string) (FieldResolver, error) {
	field := fieldSet.ResolverFor(fieldName)
	if field != nil {
		return field, nil
	}
	return nil, i18n.NewError(ctx, msgs.MsgFiltersUnknownField, fieldName)
}

func resolveValue(ctx context.Context, fieldName string, field FieldResolver, jsonValue types.RawJSON) (driver.Value, error) {
	if len(jsonValue) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgFiltersValueMissing, fieldName)
	}
	value, err := field.SQLValue(ctx, jsonValue)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgFiltersJSONValueParseError, fieldName, field, err)
	}
	return value, nil
}

func resolveFieldAndValue(ctx context.Context, fieldSet FieldSet, fieldName string, jsonValue types.RawJSON) (FieldResolver, driver.Value, error) {
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

func resolveFieldAndValues(ctx context.Context, fieldSet FieldSet, fieldName string, jsonValues []types.RawJSON) (FieldResolver, []driver.Value, error) {
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

func (qb *queryBuilder[T]) addSimpleFilters(t Traverser[T], jf *FilterJSON) Traverser[T] {
	for _, e := range joinShortNames(jf.Equal, jf.Eq, jf.NEq) {
		field, testValue, err := resolveFieldAndValue(qb.ctx, qb.fieldSet, e.Field, e.Value)
		if err != nil {
			return t.WithError(err)
		}
		t = t.IsEqual(e, e.Field, field, testValue)
	}
	for _, e := range jf.Like {
		field, testValue, err := resolveFieldAndValue(qb.ctx, qb.fieldSet, e.Field, e.Value)
		if err != nil {
			return t.WithError(err)
		}
		t = t.IsLike(e, e.Field, field, testValue)
	}
	for _, e := range jf.Null {
		field, err := resolveField(qb.ctx, qb.fieldSet, e.Field)
		if err != nil {
			return t.WithError(err)
		}
		t = t.IsNull(e, e.Field, field)
	}
	return t
}

func joinShortNames(long, short, negated []*FilterJSONKeyValue) []*FilterJSONKeyValue {
	res := make([]*FilterJSONKeyValue, len(long)+len(short)+len(negated))
	copy(res, long)
	copy(res[len(long):], short)
	negs := res[len(short)+len(long):]
	copy(negs, negated)
	for _, n := range negs {
		n.Not = true
	}
	return res
}

func joinInAndNin(in, nin []*FilterJSONKeyValues) []*FilterJSONKeyValues {
	res := make([]*FilterJSONKeyValues, len(in)+len(nin))
	copy(res, in)
	negs := res[len(in):]
	copy(negs, nin)
	for _, n := range negs {
		n.Not = true
	}
	return res
}

func (qb *queryBuilder[T]) BuildAndFilter(t Traverser[T], jf *FilterJSON) Traverser[T] {
	t = t.NewRoot()
	t = qb.addSimpleFilters(t, jf)
	if t.HasError() != nil {
		return t
	}
	for _, e := range joinShortNames(jf.LessThan, jf.LT, nil) {
		field, testValue, err := resolveFieldAndValue(qb.ctx, qb.fieldSet, e.Field, e.Value)
		if err != nil {
			return t.WithError(err)
		}
		if e.CaseInsensitive || e.Not {
			return t.WithError(i18n.NewError(qb.ctx, msgs.MsgFiltersJSONQueryOpUnsupportedMod, "lessThan", allMods))
		}
		t = t.IsLessThan(e, e.Field, field, testValue)
	}
	for _, e := range joinShortNames(jf.LessThanOrEqual, jf.LTE, nil) {
		field, testValue, err := resolveFieldAndValue(qb.ctx, qb.fieldSet, e.Field, e.Value)
		if err != nil {
			return t.WithError(err)
		}
		if e.CaseInsensitive || e.Not {
			return t.WithError(i18n.NewError(qb.ctx, msgs.MsgFiltersJSONQueryOpUnsupportedMod, "lessThanOrEqual", allMods))
		}
		t = t.IsLessThanOrEqual(e, e.Field, field, testValue)
	}
	for _, e := range joinShortNames(jf.GreaterThan, jf.GT, nil) {
		field, testValue, err := resolveFieldAndValue(qb.ctx, qb.fieldSet, e.Field, e.Value)
		if err != nil {
			return t.WithError(err)
		}
		if e.CaseInsensitive || e.Not {
			return t.WithError(i18n.NewError(qb.ctx, msgs.MsgFiltersJSONQueryOpUnsupportedMod, "greaterThan", allMods))
		}
		t = t.IsGreaterThan(e, e.Field, field, testValue)
	}
	for _, e := range joinShortNames(jf.GreaterThanOrEqual, jf.GTE, nil) {
		field, testValue, err := resolveFieldAndValue(qb.ctx, qb.fieldSet, e.Field, e.Value)
		if err != nil {
			return t.WithError(err)
		}
		if e.CaseInsensitive || e.Not {
			return t.WithError(i18n.NewError(qb.ctx, msgs.MsgFiltersJSONQueryOpUnsupportedMod, "greaterThanOrEqual", allMods))
		}
		t = t.IsGreaterThanOrEqual(e, e.Field, field, testValue)
	}
	for _, e := range joinInAndNin(jf.In, jf.NIn) {
		field, testValues, err := resolveFieldAndValues(qb.ctx, qb.fieldSet, e.Field, e.Values)
		if err != nil {
			return t.WithError(err)
		}
		if e.CaseInsensitive {
			return t.WithError(i18n.NewError(qb.ctx, msgs.MsgFiltersJSONQueryOpUnsupportedMod, "in", justCaseInsensitive))
		}
		t = t.IsIn(e, e.Field, field, testValues)
	}
	if len(jf.Or) > 0 {
		ors := t.NewRoot()
		for _, child := range jf.Or {
			sub := qb.BuildAndFilter(t, child)
			if sub.HasError() != nil {
				return sub
			}
			ors = ors.Or(sub.Result())
		}
		t = t.And(ors.Result())
	}
	return t
}
