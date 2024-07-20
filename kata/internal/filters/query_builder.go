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
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/types"
	"gorm.io/gorm"
)

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

type queryBuilder struct {
	ctx        context.Context
	rootDB     *gorm.DB
	jsonFilter *QueryJSON
	fieldSet   FieldSet
}

func (qb *queryBuilder) withErr(db *gorm.DB, err error) *gorm.DB {
	log.L(qb.ctx).Errorf("Query build failed: %s", err)
	_ = db.AddError(err)
	return db
}

func (qb *queryBuilder) build(db *gorm.DB) *gorm.DB {
	jf := qb.jsonFilter
	if jf.Limit != nil && *jf.Limit > 0 {
		db = db.Limit(*jf.Limit)
	}
	for _, s := range jf.Sort {
		dbSortField, err := qb.resolveSortField(s)
		if err != nil {
			return qb.withErr(db, err)
		}
		db = db.Order(dbSortField)
	}
	return qb.BuildAndFilter(&jf.FilterJSON)
}

func (qb *queryBuilder) resolveSortField(fieldName string) (string, error) {
	direction := "asc"
	startEnd := strings.SplitN(fieldName, " ", 2)
	fieldName, isNegated := strings.CutPrefix(startEnd[0], "-")
	if isNegated || (len(startEnd) == 2 && strings.EqualFold(startEnd[1], "desc")) {
		direction = "desc"
	}
	field, err := qb.resolveField(fieldName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %s", field.SQLColumn(), direction), nil
}

func (qb *queryBuilder) resolveField(fieldName string) (FieldResolver, error) {
	// case sensitive match wins
	field := qb.fieldSet.ResolverFor(fieldName)
	if field != nil {
		return field, nil
	}
	return nil, i18n.NewError(qb.ctx, msgs.MsgFiltersUnknownField, fieldName)
}

func (qb *queryBuilder) resolveValue(fieldName string, field FieldResolver, jsonValue types.RawJSON) (driver.Value, error) {
	if len(jsonValue) == 0 {
		return nil, i18n.NewError(qb.ctx, msgs.MsgFiltersValueMissing, fieldName)
	}
	value, err := field.SQLValue(qb.ctx, jsonValue)
	if err != nil {
		return nil, i18n.NewError(qb.ctx, msgs.MsgFiltersJSONValueParseError, fieldName, field, err)
	}
	return value, nil
}

func (qb *queryBuilder) resolveFieldAndValue(fieldName string, jsonValue types.RawJSON) (FieldResolver, driver.Value, error) {
	field, err := qb.resolveField(fieldName)
	if err != nil {
		return nil, nil, err
	}
	value, err := qb.resolveValue(fieldName, field, jsonValue)
	if err != nil {
		return nil, nil, err
	}
	return field, value, nil
}

func (qb *queryBuilder) resolveFieldAndValues(fieldName string, jsonValues []types.RawJSON) (FieldResolver, []driver.Value, error) {
	field, err := qb.resolveField(fieldName)
	if err != nil {
		return nil, nil, err
	}
	values := make([]driver.Value, len(jsonValues))
	for i, jsonValue := range jsonValues {
		if values[i], err = qb.resolveValue(fieldName, field, jsonValue); err != nil {
			return nil, nil, err
		}
	}
	return field, values, nil
}

func (qb *queryBuilder) addSimpleFilters(db *gorm.DB, jf *FilterJSON) *gorm.DB {
	for _, e := range joinShortNames(jf.Equal, jf.Eq, jf.NEq) {
		field, sqlValue, err := qb.resolveFieldAndValue(e.Field, e.Value)
		if err != nil {
			return qb.withErr(db, err)
		}
		if e.CaseInsensitive {
			if e.Not {
				db = db.Where(fmt.Sprintf("LOWER(%s) != LOWER(?)", field.SQLColumn()), sqlValue)
			} else {
				db = db.Where(fmt.Sprintf("LOWER(%s) = LOWER(?)", field.SQLColumn()), sqlValue)
			}
		} else {
			if e.Not {
				db = db.Where(fmt.Sprintf("%s != ?", field.SQLColumn()), sqlValue)
			} else {
				db = db.Where(fmt.Sprintf("%s = ?", field.SQLColumn()), sqlValue)
			}
		}
	}
	for _, e := range jf.Like {
		field, sqlValue, err := qb.resolveFieldAndValue(e.Field, e.Value)
		if err != nil {
			return qb.withErr(db, err)
		}
		if e.CaseInsensitive {
			if e.Not {
				db = db.Where(fmt.Sprintf("%s NOT ILIKE ?", field.SQLColumn()), sqlValue)
			} else {
				db = db.Where(fmt.Sprintf("%s ILIKE ?", field.SQLColumn()), sqlValue)
			}
		} else {
			if e.Not {
				db = db.Where(fmt.Sprintf("%s NOT LIKE ?", field.SQLColumn()), sqlValue)
			} else {
				db = db.Where(fmt.Sprintf("%s LIKE ?", field.SQLColumn()), sqlValue)
			}
		}
	}
	for _, e := range jf.Null {
		field, err := qb.resolveField(e.Field)
		if err != nil {
			return qb.withErr(db, err)
		}
		if e.Not {
			db = db.Where(fmt.Sprintf("%s IS NOT NULL", field.SQLColumn()))
		} else {
			db = db.Where(fmt.Sprintf("%s IS NULL", field.SQLColumn()))
		}
	}
	return db
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

func (qb *queryBuilder) BuildAndFilter(jf *FilterJSON) *gorm.DB {
	db := qb.addSimpleFilters(qb.rootDB, jf)
	if db.Error != nil {
		return db
	}
	for _, e := range joinShortNames(jf.LessThan, jf.LT, nil) {
		field, sqlValue, err := qb.resolveFieldAndValue(e.Field, e.Value)
		if err != nil {
			return qb.withErr(db, err)
		}
		if e.CaseInsensitive || e.Not {
			return qb.withErr(db, i18n.NewError(qb.ctx, msgs.MsgFiltersJSONQueryOpUnsupportedMod, "lessThan", allMods))
		}
		db = db.Where(fmt.Sprintf("%s < ?", field.SQLColumn()), sqlValue)
	}
	for _, e := range joinShortNames(jf.LessThanOrEqual, jf.LTE, nil) {
		field, sqlValue, err := qb.resolveFieldAndValue(e.Field, e.Value)
		if err != nil {
			return qb.withErr(db, err)
		}
		if e.CaseInsensitive || e.Not {
			return qb.withErr(db, i18n.NewError(qb.ctx, msgs.MsgFiltersJSONQueryOpUnsupportedMod, "lessThanOrEqual", allMods))
		}
		db = db.Where(fmt.Sprintf("%s <= ?", field.SQLColumn()), sqlValue)
	}
	for _, e := range joinShortNames(jf.GreaterThan, jf.GT, nil) {
		field, sqlValue, err := qb.resolveFieldAndValue(e.Field, e.Value)
		if err != nil {
			return qb.withErr(db, err)
		}
		if e.CaseInsensitive || e.Not {
			return qb.withErr(db, i18n.NewError(qb.ctx, msgs.MsgFiltersJSONQueryOpUnsupportedMod, "greaterThan", allMods))
		}
		db = db.Where(fmt.Sprintf("%s > ?", field.SQLColumn()), sqlValue)
	}
	for _, e := range joinShortNames(jf.GreaterThanOrEqual, jf.GTE, nil) {
		field, sqlValue, err := qb.resolveFieldAndValue(e.Field, e.Value)
		if err != nil {
			return qb.withErr(db, err)
		}
		if e.CaseInsensitive || e.Not {
			return qb.withErr(db, i18n.NewError(qb.ctx, msgs.MsgFiltersJSONQueryOpUnsupportedMod, "greaterThanOrEqual", allMods))
		}
		db = db.Where(fmt.Sprintf("%s >= ?", field.SQLColumn()), sqlValue)
	}
	for _, e := range joinInAndNin(jf.In, jf.NIn) {
		field, values, err := qb.resolveFieldAndValues(e.Field, e.Values)
		if err != nil {
			return qb.withErr(db, err)
		}
		if e.CaseInsensitive {
			return qb.withErr(db, i18n.NewError(qb.ctx, msgs.MsgFiltersJSONQueryOpUnsupportedMod, "in", justCaseInsensitive))
		}
		if e.Not {
			db = db.Where(fmt.Sprintf("%s NOT IN (?)", field.SQLColumn()), values)
		} else {
			db = db.Where(fmt.Sprintf("%s IN (?)", field.SQLColumn()), values)
		}
	}
	if len(jf.Or) > 0 {
		ors := qb.rootDB
		for i, child := range jf.Or {
			subFilter := qb.BuildAndFilter(child)
			if subFilter.Error != nil {
				return subFilter
			}
			if i == 0 {
				ors = ors.Where(subFilter)
			} else {
				ors = ors.Or(subFilter)
			}
		}
		db = db.Where(ors)
	}
	return db
}
