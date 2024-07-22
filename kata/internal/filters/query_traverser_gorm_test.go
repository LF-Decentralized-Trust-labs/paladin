// Copyright © 2021 Kaleido, Inc.
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
	"testing"

	"github.com/kaleido-io/paladin/kata/internal/persistence/mockpersistence"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestBuildQueryJSONNestedAndOr(t *testing.T) {

	var qf QueryJSON
	err := json.Unmarshal([]byte(`{
		"skip": 5,
		"limit": 10,
		"sort": [
			"tag",
			"-sequence"
		],
		"equal": [
			{
				"field": "tag",
				"value": "a"
			}
		],
		"eq": [
			{
				"field": "masked",
				"value": "true"
			}
		],
		"neq": [
			{
				"field": "sequence",
				"value": 999
			}
		],
		"null": [
			{
				"field": "cid"
			}
		],
		"greaterThan": [
			{
				"field": "sequence",
				"value": 10
			}
		],
		"or": [
			{
				"equal": [
					{
						"field": "masked",
						"value": true
					}
				],
				"in": [
					{
						"field": "tag",
						"values": ["a","b","c"]
					}
				],
				"nin": [
					{
						"field": "tag",
						"values": ["x","y"]
					},
					{
						"field": "tag",
						"values": ["z"]
					}
				]
			},
			{
				"equal": [
					{
						"field": "masked",
						"value": false
					}
				]
			}
		]
	}`), &qf)
	assert.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := qf.BuildGORM(context.Background(), tx.Table("test"), FieldMap{
			"tag":      StringField("tag"),
			"sequence": Int64Field("sequence"),
			"masked":   Int64BoolField("masked"),
			"cid":      Int256Field("correl_id"),
		}).Count(&count)
		assert.NoError(t, db.Error)
		return db
	})

	assert.Equal(t, "SELECT count(*) FROM `test` WHERE tag = 'a' AND masked = 1 AND sequence != 999 AND correl_id IS NULL AND sequence > 10 AND ((masked = 1 AND tag IN ('a','b','c') AND tag NOT IN ('x','y') AND tag NOT IN ('z')) OR masked = 0) LIMIT 10", generatedSQL)
}

func TestBuildQuerySingleNestedOr(t *testing.T) {

	var qf QueryJSON
	err := json.Unmarshal([]byte(`{
		"or": [
			{
				"equal": [
					{
						"field": "tag",
						"value": "a"
					}
				]
			}
		]
	}`), &qf)
	assert.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := qf.BuildGORM(context.Background(), tx.Table("test"), FieldMap{
			"tag": StringField("tag"),
		}).Count(&count)
		assert.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM `test` WHERE tag = 'a'", generatedSQL)
}

func TestBuildQuerySingleNestedWithResolverErrorTag(t *testing.T) {

	var qf QueryJSON
	err := json.Unmarshal([]byte(`{
		"or": [
			{
				"in": [
					{
						"field": "tag",
						"values": ["a"]
					}
				]
			}
		]
	}`), &qf)
	assert.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)
	_ = p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := qf.BuildGORM(context.Background(), tx.Table("test"), FieldMap{}).Count(&count)
		assert.Regexp(t, "PD010600.*tag", db.Error)
		return db
	})
}

func TestBuildQuerySingleNestedWithResolverErrorValue(t *testing.T) {

	var qf QueryJSON
	err := json.Unmarshal([]byte(`{
		"or": [
			{
				"in": [
					{
						"field": "tag",
						"values": [false]
					}
				]
			}
		]
	}`), &qf)
	assert.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)
	_ = p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := qf.BuildGORM(context.Background(), tx.Table("test"), FieldMap{
			"tag": StringField("tag"),
		}).Count(&count)
		assert.Regexp(t, "PD010610.*tag.*PD010605", db.Error)
		return db
	})
}

func TestBuildQueryResolverErrorMissing(t *testing.T) {

	var qf QueryJSON
	err := json.Unmarshal([]byte(`{
		"eq": [
			{
				"field": "tag"
			}
		]
	}`), &qf)
	assert.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)
	_ = p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := qf.BuildGORM(context.Background(), tx.Table("test"), FieldMap{
			"tag": StringField("tag"),
		}).Count(&count)
		assert.Regexp(t, "PD010608.*tag", db.Error)
		return db
	})
}

func TestBuildQueryJSONEqual(t *testing.T) {

	var qf QueryJSON
	err := json.Unmarshal([]byte(`{
		"skip": 5,
		"limit": 10,
		"count": true,
		"sort": [
			"tag",
			"sequence"
		],
		"equal": [
			{
				"field": "created",
				"value": "2001-02-03T04:05:06.000Z"
			},
			{
				"not": true,
				"field": "tag",
				"value": "abc"
			},
			{
				"caseInsensitive": true,
				"field": "tag",
				"value": "ABC"
			},
			{
				"caseInsensitive": true,
				"not": true,
				"field": "tag",
				"value": "abc"
			}
		],
		"null": [
			{
				"not": true,
				"field": "cid"
			}
		]
	}`), &qf)
	assert.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := qf.BuildGORM(context.Background(), tx.Table("test"), FieldMap{
			"tag":      StringField("tag"),
			"sequence": Int64Field("sequence"),
			"masked":   Int64BoolField("masked"),
			"cid":      Int256Field("correl_id"),
			"created":  TimestampField("created_at"),
		}).Count(&count)
		assert.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM `test` WHERE created_at = 981173106000000000 AND tag != 'abc' AND LOWER(tag) = LOWER('ABC') AND LOWER(tag) != LOWER('abc') AND correl_id IS NOT NULL LIMIT 10", generatedSQL)
}

func TestBuildQueryJSONLike(t *testing.T) {

	var qf QueryJSON
	err := json.Unmarshal([]byte(`{
		"skip": 5,
		"limit": 10,
		"like": [
			{
				"field": "tag",
				"value": "%%stuff%%"
			},
			{
				"not": true,
				"field": "tag",
				"value": "abc"
			},
			{
				"caseInsensitive": true,
				"field": "tag",
				"value": "ABC"
			},
			{
				"caseInsensitive": true,
				"not": true,
				"field": "tag",
				"value": "abc"
			}
		]
	}`), &qf)
	assert.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := qf.BuildGORM(context.Background(), tx.Table("test"), FieldMap{
			"tag":      StringField("tag"),
			"sequence": Int64Field("sequence"),
			"masked":   Int64BoolField("masked"),
			"cid":      Int256Field("correl_id"),
			"created":  Int64Field("created_at"),
		}).Count(&count)
		assert.NoError(t, db.Error)
		return db
	})

	assert.Equal(t, "SELECT count(*) FROM `test` WHERE tag LIKE '%%stuff%%' AND tag NOT LIKE 'abc' AND tag ILIKE 'ABC' AND tag NOT ILIKE 'abc' LIMIT 10", generatedSQL)
}

func TestBuildQueryJSONGreaterThan(t *testing.T) {

	var qf QueryJSON
	err := json.Unmarshal([]byte(`{
		"skip": 5,
		"limit": 10,
		"greaterThan": [
			{
				"field": "sequence",
				"value": 0
			}
		]
	}`), &qf)
	assert.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := qf.BuildGORM(context.Background(), tx.Table("test"), FieldMap{
			"sequence": Int64Field("sequence"),
		}).Count(&count)
		assert.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM `test` WHERE sequence > 0 LIMIT 10", generatedSQL)
}

func TestBuildQueryJSONLessThan(t *testing.T) {

	var qf QueryJSON
	err := json.Unmarshal([]byte(`{
		"skip": 5,
		"limit": 10,
		"lessThan": [
			{
				"field": "amount",
				"value": "12345"
			},
			{
				"field": "delta",
				"value": "-100"
			}
		]
	}`), &qf)
	assert.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := qf.BuildGORM(context.Background(), tx.Table("test"), FieldMap{
			"amount": Uint256Field("amount"),
			"delta":  Int256Field("delta"),
		}).Count(&count)
		assert.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM `test` WHERE amount < '0000000000000000000000000000000000000000000000000000000000003039' AND delta < '0ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff9c' LIMIT 10", generatedSQL)
}

func TestBuildQueryJSONGreaterThanOrEqual(t *testing.T) {

	var qf QueryJSON
	err := json.Unmarshal([]byte(`{
		"skip": 5,
		"limit": 10,
		"greaterThanOrEqual": [
			{
				"field": "sequence",
				"value": 0
			},
			{
				"field": "delta",
				"value": 100
			}
		]
	}`), &qf)
	assert.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := qf.BuildGORM(context.Background(), tx.Table("test"), FieldMap{
			"sequence": Int64Field("sequence"),
			"delta":    Int256Field("delta"),
		}).Count(&count)
		assert.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM `test` WHERE sequence >= 0 AND delta >= '10000000000000000000000000000000000000000000000000000000000000064' LIMIT 10", generatedSQL)
}

func TestBuildQueryJSONLessThanOrEqual(t *testing.T) {

	var qf QueryJSON
	err := json.Unmarshal([]byte(`{
		"skip": 5,
		"limit": 10,
		"lessThanOrEqual": [
			{
				"field": "sequence",
				"value": "12345"
			}
		]
	}`), &qf)
	assert.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := qf.BuildGORM(context.Background(), tx.Table("test"), FieldMap{
			"sequence": Int64Field("sequence"),
		}).Count(&count)
		assert.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM `test` WHERE sequence <= 12345 LIMIT 10", generatedSQL)
}

func TestBuildQueryJSONIn(t *testing.T) {

	var qf QueryJSON
	err := json.Unmarshal([]byte(`{
		"skip": 5,
		"limit": 10,
		"in": [
			{
				"field": "tag",
				"values": ["a","b","c"]
			},
			{
				"not": true,
				"field": "tag",
				"values": ["x","y","z"]
			}
		]
	}`), &qf)
	assert.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := qf.BuildGORM(context.Background(), tx.Table("test"), FieldMap{
			"tag": StringField("tag"),
		}).Count(&count)
		assert.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM `test` WHERE tag IN ('a','b','c') AND tag NOT IN ('x','y','z') LIMIT 10", generatedSQL)
}

func TestBuildQueryJSONBadModifiers(t *testing.T) {

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)

	testJSON := func(j string) error {
		var qf QueryJSON
		err := json.Unmarshal([]byte(j), &qf)
		assert.NoError(t, err)
		var count int64
		db := qf.BuildGORM(context.Background(), p.P.DB().Table("test"), FieldMap{
			"tag": StringField("tag"),
		}).Count(&count)
		return db.Error
	}

	err = testJSON(`{"lessThan": [{"not": true, "field": "tag", "value": ""}]}`)
	assert.Regexp(t, "PD010602", err)

	err = testJSON(`{"lessThanOrEqual": [{"not": true, "field": "tag", "value": ""}]}`)
	assert.Regexp(t, "PD010602", err)

	err = testJSON(`{"greaterThan": [{"not": true, "field": "tag", "value": ""}]}`)
	assert.Regexp(t, "PD010602", err)

	err = testJSON(`{"greaterThanOrEqual": [{"not": true, "field": "tag", "value": ""}]}`)
	assert.Regexp(t, "PD010602", err)

	err = testJSON(`{"in": [{"caseInsensitive": true, "field": "tag", "value": ""}]}`)
	assert.Regexp(t, "PD010602", err)

	err = testJSON(`{"or": [{"in": [{"caseInsensitive": true, "field": "tag", "value": ""}]}] }`)
	assert.Regexp(t, "PD010602", err)

}

func TestBuildQueryJSONBadFields(t *testing.T) {

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)

	testJSON := func(j string) error {
		var qf QueryJSON
		err := json.Unmarshal([]byte(j), &qf)
		assert.NoError(t, err)
		var count int64
		db := qf.BuildGORM(context.Background(), p.P.DB().Table("test"), FieldMap{
			"tag": StringField("tag"),
		}).Count(&count)
		return db.Error
	}

	err = testJSON(`{"sort": ["-wrong"]}`)
	assert.Regexp(t, "PD010600", err)

	err = testJSON(`{"equal": [{"field": "wrong"}]}`)
	assert.Regexp(t, "PD010600", err)

	err = testJSON(`{"like": [{"field": "wrong"}]}`)
	assert.Regexp(t, "PD010600", err)

	err = testJSON(`{"lessThan": [{"field": "wrong"}]}`)
	assert.Regexp(t, "PD010600", err)

	err = testJSON(`{"lessThanOrEqual": [{"field": "wrong"}]}`)
	assert.Regexp(t, "PD010600", err)

	err = testJSON(`{"greaterThan": [{"field": "wrong"}]}`)
	assert.Regexp(t, "PD010600", err)

	err = testJSON(`{"greaterThanOrEqual": [{"field": "wrong"}]}`)
	assert.Regexp(t, "PD010600", err)

	err = testJSON(`{"in": [{"field": "wrong"}]}`)
	assert.Regexp(t, "PD010600", err)

	err = testJSON(`{"null": [{"field": "wrong"}]}`)
	assert.Regexp(t, "PD010600", err)
}

func TestBuildQueryJSONContainsShortNames(t *testing.T) {

	var qf1 QueryJSON
	err := json.Unmarshal([]byte(`{
		"eq": [
			{
				"field": "sequence",
				"value": "12345"
			}
		]
	}`), &qf1)
	assert.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := qf1.BuildGORM(context.Background(), tx.Table("test"), FieldMap{
			"sequence": Int64Field("sequence"),
		}).Count(&count)
		assert.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM `test` WHERE sequence = 12345", generatedSQL)

	var qf2 QueryJSON
	err = json.Unmarshal([]byte(`{
		"gt": [
			{
				"field": "sequence",
				"value": "12345"
			}
		],
		"lte": [
			{
				"field": "sequence",
				"value": "12345"
			}
		]
	}`), &qf2)
	assert.NoError(t, err)

	generatedSQL = p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := qf2.BuildGORM(context.Background(), tx.Table("test"), FieldMap{
			"sequence": Int64Field("sequence"),
		}).Count(&count)
		assert.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM `test` WHERE sequence <= 12345 AND sequence > 12345", generatedSQL)
}
