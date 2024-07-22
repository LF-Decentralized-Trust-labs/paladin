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

	"gorm.io/gorm"
)

func (qj *QueryJSON) BuildGORM(ctx context.Context, db *gorm.DB, fieldSet FieldSet) *gorm.DB {
	gt := &gormTraverser{
		// We can't assume anything about the db passed in - if it's a clone (internal concept
		// in GORM I can't work out how to detect), then it will aggregate WHERE clauses
		// and cause us to do all kinds of wonky nesting.
		// So use this function to get a clean session to do our nested db.Where() clauses against.
		rootDB: db.Session(&gorm.Session{SkipDefaultTransaction: true}),
		db:     db,
	}
	qt := &queryTraverser[*gormTraverser]{
		ctx:        ctx,
		jsonFilter: qj,
		fieldSet:   fieldSet,
	}
	return qt.traverse(gt).Result().db
}

type gormTraverser struct {
	rootDB *gorm.DB
	db     *gorm.DB
}

func (t *gormTraverser) NewRoot() Traverser[*gormTraverser] {
	return &gormTraverser{rootDB: t.rootDB, db: t.rootDB}
}

func (t *gormTraverser) Result() *gormTraverser {
	return t
}

func (t *gormTraverser) Error() error {
	return t.db.Error
}

func (t *gormTraverser) WithError(err error) Traverser[*gormTraverser] {
	_ = t.db.AddError(err)
	return t
}

func (t *gormTraverser) Limit(l int) Traverser[*gormTraverser] {
	t.db = t.db.Limit(l)
	return t
}

func (t *gormTraverser) Order(order string) Traverser[*gormTraverser] {
	t.db = t.db.Order(order)
	return t
}

func (t *gormTraverser) And(ot *gormTraverser) Traverser[*gormTraverser] {
	t.db = t.db.Where(ot.db)
	return t
}

func (t *gormTraverser) Or(ot *gormTraverser) Traverser[*gormTraverser] {
	t.db = t.db.Or(ot.db)
	return t
}

func (t *gormTraverser) IsEqual(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*gormTraverser] {
	if e.CaseInsensitive {
		if e.Not {
			t.db = t.db.Where(fmt.Sprintf("LOWER(%s) != LOWER(?)", field.SQLColumn()), testValue)
		} else {
			t.db = t.db.Where(fmt.Sprintf("LOWER(%s) = LOWER(?)", field.SQLColumn()), testValue)
		}
	} else {
		if e.Not {
			t.db = t.db.Where(fmt.Sprintf("%s != ?", field.SQLColumn()), testValue)
		} else {
			t.db = t.db.Where(fmt.Sprintf("%s = ?", field.SQLColumn()), testValue)
		}
	}
	return t
}

func (t *gormTraverser) IsLike(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*gormTraverser] {
	if e.CaseInsensitive {
		if e.Not {
			t.db = t.db.Where(fmt.Sprintf("%s NOT ILIKE ?", field.SQLColumn()), testValue)
		} else {
			t.db = t.db.Where(fmt.Sprintf("%s ILIKE ?", field.SQLColumn()), testValue)
		}
	} else {
		if e.Not {
			t.db = t.db.Where(fmt.Sprintf("%s NOT LIKE ?", field.SQLColumn()), testValue)
		} else {
			t.db = t.db.Where(fmt.Sprintf("%s LIKE ?", field.SQLColumn()), testValue)
		}
	}
	return t
}

func (t *gormTraverser) IsNull(e *FilterJSONBase, fieldName string, field FieldResolver) Traverser[*gormTraverser] {
	if e.Not {
		t.db = t.db.Where(fmt.Sprintf("%s IS NOT NULL", field.SQLColumn()))
	} else {
		t.db = t.db.Where(fmt.Sprintf("%s IS NULL", field.SQLColumn()))
	}
	return t
}

func (t *gormTraverser) IsLessThan(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*gormTraverser] {
	t.db = t.db.Where(fmt.Sprintf("%s < ?", field.SQLColumn()), testValue)
	return t
}

func (t *gormTraverser) IsLessThanOrEqual(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*gormTraverser] {
	t.db = t.db.Where(fmt.Sprintf("%s <= ?", field.SQLColumn()), testValue)
	return t
}

func (t *gormTraverser) IsGreaterThan(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*gormTraverser] {
	t.db = t.db.Where(fmt.Sprintf("%s > ?", field.SQLColumn()), testValue)
	return t
}

func (t *gormTraverser) IsGreaterThanOrEqual(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*gormTraverser] {
	t.db = t.db.Where(fmt.Sprintf("%s >= ?", field.SQLColumn()), testValue)
	return t
}

func (t *gormTraverser) IsIn(e *FilterJSONKeyValues, fieldName string, field FieldResolver, testValues []driver.Value) Traverser[*gormTraverser] {
	if e.Not {
		t.db = t.db.Where(fmt.Sprintf("%s NOT IN (?)", field.SQLColumn()), testValues)
	} else {
		t.db = t.db.Where(fmt.Sprintf("%s IN (?)", field.SQLColumn()), testValues)
	}
	return t
}
