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
				"value": true
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
		db := qf.Build(context.Background(), tx.Table("test"), FieldList{
			"tag":      StringField("tag"),
			"sequence": Int64Field("sequence"),
			"masked":   BoolField("masked"),
			"cid":      StringField("correl_id"),
		}).Count(&count)
		assert.NoError(t, db.Error)
		return db
	})

	assert.Equal(t, "SELECT count(*) FROM `test` WHERE tag = 'a' AND masked = true AND sequence != 999 AND correl_id IS NULL AND sequence > 10 AND ((masked = true AND tag IN ('a','b','c') AND tag NOT IN ('x','y') AND tag NOT IN ('z')) OR masked = false) LIMIT 10", generatedSQL)
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
		db := qf.Build(context.Background(), tx.Table("test"), FieldList{
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
		db := qf.Build(context.Background(), tx.Table("test"), FieldList{}).Count(&count)
		assert.Regexp(t, "PD010300.*tag", db.Error)
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
		db := qf.Build(context.Background(), tx.Table("test"), FieldList{
			"tag": StringField("tag"),
		}).Count(&count)
		assert.Regexp(t, "PD010306.*tag", db.Error)
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
				"value": 0
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
		db := qf.Build(context.Background(), tx.Table("test"), FieldList{
			"tag":      StringField("tag"),
			"sequence": Int64Field("sequence"),
			"masked":   BoolField("masked"),
			"cid":      StringField("correl_id"),
			"created":  Int64Field("created_at"),
		}).Count(&count)
		assert.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM `test` WHERE created_at = 0 AND tag != 'abc' AND LOWER(tag) = LOWER('ABC') AND LOWER(tag) != LOWER('abc') AND correl_id IS NOT NULL LIMIT 10", generatedSQL)
}

// func TestBuildQueryJSONContains(t *testing.T) {

// 	var qf QueryJSON
// 	err := json.Unmarshal([]byte(`{
// 		"skip": 5,
// 		"limit": 10,
// 		"contains": [
// 			{
// 				"field": "tag",
// 				"value": 0
// 			},
// 			{
// 				"not": true,
// 				"field": "tag",
// 				"value": "abc"
// 			},
// 			{
// 				"caseInsensitive": true,
// 				"field": "tag",
// 				"value": "ABC"
// 			},
// 			{
// 				"caseInsensitive": true,
// 				"not": true,
// 				"field": "tag",
// 				"value": "abc"
// 			}
// 		]
// 	}`), &qf)
// 	assert.NoError(t, err)

// 	filter, err := qf.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.NoError(t, err)

// 	fi, err := filter.Finalize()
// 	assert.NoError(t, err)

// 	assert.Equal(t, "( tag %= '0' ) && ( tag !% 'abc' ) && ( tag :% 'ABC' ) && ( tag ;% 'abc' ) skip=5 limit=10", fi.String())
// }

// func TestBuildQueryJSONStartsWith(t *testing.T) {

// 	var qf QueryJSON
// 	err := json.Unmarshal([]byte(`{
// 		"skip": 5,
// 		"limit": 10,
// 		"startsWith": [
// 			{
// 				"field": "tag",
// 				"value": 0
// 			},
// 			{
// 				"not": true,
// 				"field": "tag",
// 				"value": "abc"
// 			},
// 			{
// 				"caseInsensitive": true,
// 				"field": "tag",
// 				"value": "ABC"
// 			},
// 			{
// 				"caseInsensitive": true,
// 				"not": true,
// 				"field": "tag",
// 				"value": true
// 			}
// 		]
// 	}`), &qf)
// 	assert.NoError(t, err)

// 	filter, err := qf.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.NoError(t, err)

// 	fi, err := filter.Finalize()
// 	assert.NoError(t, err)

// 	assert.Equal(t, "( tag ^= '0' ) && ( tag !^ 'abc' ) && ( tag :^ 'ABC' ) && ( tag ;^ 'true' ) skip=5 limit=10", fi.String())
// }

// func TestBuildQueryJSONGreaterThan(t *testing.T) {

// 	var qf QueryJSON
// 	err := json.Unmarshal([]byte(`{
// 		"skip": 5,
// 		"limit": 10,
// 		"greaterThan": [
// 			{
// 				"field": "sequence",
// 				"value": 0
// 			}
// 		]
// 	}`), &qf)
// 	assert.NoError(t, err)

// 	filter, err := qf.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.NoError(t, err)

// 	fi, err := filter.Finalize()
// 	assert.NoError(t, err)

// 	assert.Equal(t, "sequence >> 0 skip=5 limit=10", fi.String())
// }

// func TestBuildQueryJSONLessThan(t *testing.T) {

// 	var qf QueryJSON
// 	err := json.Unmarshal([]byte(`{
// 		"skip": 5,
// 		"limit": 10,
// 		"lessThan": [
// 			{
// 				"field": "sequence",
// 				"value": "12345"
// 			}
// 		]
// 	}`), &qf)
// 	assert.NoError(t, err)

// 	filter, err := qf.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.NoError(t, err)

// 	fi, err := filter.Finalize()
// 	assert.NoError(t, err)

// 	assert.Equal(t, "sequence << 12345 skip=5 limit=10", fi.String())
// }

// func TestBuildQueryJSONGreaterThanOrEqual(t *testing.T) {

// 	var qf QueryJSON
// 	err := json.Unmarshal([]byte(`{
// 		"skip": 5,
// 		"limit": 10,
// 		"greaterThanOrEqual": [
// 			{
// 				"field": "sequence",
// 				"value": 0
// 			}
// 		]
// 	}`), &qf)
// 	assert.NoError(t, err)

// 	filter, err := qf.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.NoError(t, err)

// 	fi, err := filter.Finalize()
// 	assert.NoError(t, err)

// 	assert.Equal(t, "sequence >= 0 skip=5 limit=10", fi.String())
// }

// func TestBuildQueryJSONLessThanOrEqual(t *testing.T) {

// 	var qf QueryJSON
// 	err := json.Unmarshal([]byte(`{
// 		"skip": 5,
// 		"limit": 10,
// 		"lessThanOrEqual": [
// 			{
// 				"field": "sequence",
// 				"value": "12345"
// 			}
// 		]
// 	}`), &qf)
// 	assert.NoError(t, err)

// 	filter, err := qf.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.NoError(t, err)

// 	fi, err := filter.Finalize()
// 	assert.NoError(t, err)

// 	assert.Equal(t, "sequence <= 12345 skip=5 limit=10", fi.String())
// }

// func TestBuildQueryJSONIn(t *testing.T) {

// 	var qf QueryJSON
// 	err := json.Unmarshal([]byte(`{
// 		"skip": 5,
// 		"limit": 10,
// 		"in": [
// 			{
// 				"field": "tag",
// 				"values": ["a","b","c"]
// 			},
// 			{
// 				"not": true,
// 				"field": "tag",
// 				"values": ["x","y","z"]
// 			}
// 		]
// 	}`), &qf)
// 	assert.NoError(t, err)

// 	filter, err := qf.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.NoError(t, err)

// 	fi, err := filter.Finalize()
// 	assert.NoError(t, err)

// 	assert.Equal(t, "( tag IN ['a','b','c'] ) && ( tag NI ['x','y','z'] ) skip=5 limit=10", fi.String())
// }

// func TestBuildQueryJSONBadModifiers(t *testing.T) {

// 	var qf1 QueryJSON
// 	err := json.Unmarshal([]byte(`{"lessThan": [{"not": true, "field": "tag"}]}`), &qf1)
// 	assert.NoError(t, err)
// 	_, err = qf1.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.Regexp(t, "FF00240", err)

// 	var qf2 QueryJSON
// 	err = json.Unmarshal([]byte(`{"lessThanOrEqual": [{"not": true, "field": "tag"}]}`), &qf2)
// 	assert.NoError(t, err)
// 	_, err = qf2.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.Regexp(t, "FF00240", err)

// 	var qf3 QueryJSON
// 	err = json.Unmarshal([]byte(`{"greaterThan": [{"not": true, "field": "tag"}]}`), &qf3)
// 	assert.NoError(t, err)
// 	_, err = qf3.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.Regexp(t, "FF00240", err)

// 	var qf4 QueryJSON
// 	err = json.Unmarshal([]byte(`{"greaterThanOrEqual": [{"not": true, "field": "tag"}]}`), &qf4)
// 	assert.NoError(t, err)
// 	_, err = qf4.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.Regexp(t, "FF00240", err)

// 	var qf5 QueryJSON
// 	err = json.Unmarshal([]byte(`{"in": [{"caseInsensitive": true, "field": "tag"}]}`), &qf5)
// 	assert.NoError(t, err)
// 	_, err = qf5.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.Regexp(t, "FF00240", err)

// 	var qf6 QueryJSON
// 	err = json.Unmarshal([]byte(`{"or": [{"in": [{"caseInsensitive": true, "field": "tag"}]}] }`), &qf6)
// 	assert.NoError(t, err)
// 	_, err = qf6.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.Regexp(t, "FF00240", err)

// }

// func TestStringableParseFail(t *testing.T) {

// 	var js SimpleFilterValue
// 	err := js.UnmarshalJSON([]byte(`{!!! not json`))
// 	assert.Error(t, err)

// 	err = js.UnmarshalJSON([]byte(`{"this": "is an object"}`))
// 	assert.Error(t, err)

// }

// func TestBuildQueryJSONBadFields(t *testing.T) {

// 	var qf1 QueryJSON
// 	err := json.Unmarshal([]byte(`{"equal": [{"field": "wrong"}]}`), &qf1)
// 	assert.NoError(t, err)
// 	_, err = qf1.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.Regexp(t, "FF00142", err)

// 	var qf2 QueryJSON
// 	err = json.Unmarshal([]byte(`{"contains": [{"field": "wrong"}]}`), &qf2)
// 	assert.NoError(t, err)
// 	_, err = qf2.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.Regexp(t, "FF00142", err)

// 	var qf3 QueryJSON
// 	err = json.Unmarshal([]byte(`{"startsWith": [{"field": "wrong"}]}`), &qf3)
// 	assert.NoError(t, err)
// 	_, err = qf3.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.Regexp(t, "FF00142", err)

// 	var qf4 QueryJSON
// 	err = json.Unmarshal([]byte(`{"lessThan": [{"field": "wrong"}]}`), &qf4)
// 	assert.NoError(t, err)
// 	_, err = qf4.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.Regexp(t, "FF00142", err)

// 	var qf5 QueryJSON
// 	err = json.Unmarshal([]byte(`{"lessThanOrEqual": [{"field": "wrong"}]}`), &qf5)
// 	assert.NoError(t, err)
// 	_, err = qf5.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.Regexp(t, "FF00142", err)

// 	var qf6 QueryJSON
// 	err = json.Unmarshal([]byte(`{"greaterThan": [{"field": "wrong"}]}`), &qf6)
// 	assert.NoError(t, err)
// 	_, err = qf6.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.Regexp(t, "FF00142", err)

// 	var qf7 QueryJSON
// 	err = json.Unmarshal([]byte(`{"greaterThanOrEqual": [{"field": "wrong"}]}`), &qf7)
// 	assert.NoError(t, err)
// 	_, err = qf7.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.Regexp(t, "FF00142", err)

// 	var qf8 QueryJSON
// 	err = json.Unmarshal([]byte(`{"in": [{"field": "wrong"}]}`), &qf8)
// 	assert.NoError(t, err)
// 	_, err = qf8.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.Regexp(t, "FF00142", err)

// 	var qf9 QueryJSON
// 	err = json.Unmarshal([]byte(`{"null": [{"field": "wrong"}]}`), &qf9)
// 	assert.NoError(t, err)
// 	_, err = qf9.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.Regexp(t, "FF00142", err)
// }

// func TestBuildQueryJSONDocumented(t *testing.T) {
// 	CheckObjectDocumented(&QueryJSON{})
// }

// func TestBuildQueryJSONContainsShortNames(t *testing.T) {

// 	var qf1 QueryJSON
// 	err := json.Unmarshal([]byte(`{
// 		"eq": [
// 			{
// 				"field": "sequence",
// 				"value": "12345"
// 			}
// 		]
// 	}`), &qf1)
// 	assert.NoError(t, err)

// 	filter, err := qf1.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.NoError(t, err)

// 	fi, err := filter.Finalize()
// 	assert.NoError(t, err)

// 	assert.Equal(t, "sequence == 12345", fi.String())

// 	var qf2 QueryJSON
// 	err = json.Unmarshal([]byte(`{
// 		"gt": [
// 			{
// 				"field": "sequence",
// 				"value": "12345"
// 			}
// 		],
// 		"lte": [
// 			{
// 				"field": "sequence",
// 				"value": "12345"
// 			}
// 		]
// 	}`), &qf2)
// 	assert.NoError(t, err)

// 	filter, err = qf2.BuildFilter(context.Background(), TestQueryFactory)
// 	assert.NoError(t, err)

// 	fi, err = filter.Finalize()
// 	assert.NoError(t, err)

// 	assert.Equal(t, "( sequence <= 12345 ) && ( sequence >> 12345 )", fi.String())
// }
