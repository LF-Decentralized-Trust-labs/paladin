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

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBuildQueryJSONNestedAndOr(t *testing.T) {

	var qf query.QueryJSON
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
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf, tx.Table("test"), FieldMap{
			"tag":      StringField("tag"),
			"sequence": Int64Field("sequence"),
			"masked":   Int64BoolField("masked"),
			"cid":      Int256Field("correl_id"),
		}).Count(&count)
		require.NoError(t, db.Error)
		return db
	})

	assert.Equal(t, "SELECT count(*) FROM \"test\" WHERE tag = 'a' AND masked = 1 AND sequence != 999 AND correl_id IS NULL AND sequence > 10 AND ((masked = 1 AND \"tag\" = ANY ('{\"a\",\"b\",\"c\"}') AND tag NOT IN ('x','y') AND tag NOT IN ('z')) OR masked = 0) LIMIT 10", generatedSQL)
}

func TestBuildQuerySingleNestedOr(t *testing.T) {

	var qf query.QueryJSON
	err := json.Unmarshal([]byte(`{
	    "limit": 10,
		"sort": ["tag"],
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
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf, tx.Table("test"), FieldMap{
			"tag": StringField("tag"),
		}).Count(&count)
		require.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM \"test\" WHERE tag = 'a' LIMIT 10", generatedSQL)
}

func TestBuildQuerySingleNestedWithResolverErrorTag(t *testing.T) {

	var qf query.QueryJSON
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
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	_ = p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf, tx.Table("test"), FieldMap{}).Count(&count)
		assert.Regexp(t, "PD010700.*tag", db.Error)
		return db
	})
}

func TestBuildQuerySingleNestedWithResolverErrorValue(t *testing.T) {

	var qf query.QueryJSON
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
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	_ = p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf, tx.Table("test"), FieldMap{
			"tag": StringField("tag"),
		}).Count(&count)
		assert.Regexp(t, "PD010710.*tag.*PD010705", db.Error)
		return db
	})
}

func TestBuildQueryResolverErrorMissing(t *testing.T) {

	var qf query.QueryJSON
	err := json.Unmarshal([]byte(`{
		"eq": [
			{
				"field": "tag"
			}
		]
	}`), &qf)
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	_ = p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf, tx.Table("test"), FieldMap{
			"tag": StringField("tag"),
		}).Count(&count)
		assert.Regexp(t, "PD010708.*tag", db.Error)
		return db
	})
}

func TestBuildQueryJSONEqual(t *testing.T) {

	var qf query.QueryJSON
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
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf, tx.Table("test"), FieldMap{
			"tag":      StringField("tag"),
			"sequence": Int64Field("sequence"),
			"masked":   Int64BoolField("masked"),
			"cid":      Int256Field("correl_id"),
			"created":  TimestampField("created"),
		}).Count(&count)
		require.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM \"test\" WHERE created = 981173106000000000 AND tag != 'abc' AND LOWER(tag) = LOWER('ABC') AND LOWER(tag) != LOWER('abc') AND correl_id IS NOT NULL LIMIT 10", generatedSQL)
}

func TestBuildQueryJSONLike(t *testing.T) {

	var qf query.QueryJSON
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
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf, tx.Table("test"), FieldMap{
			"tag":      StringField("tag"),
			"sequence": Int64Field("sequence"),
			"masked":   Int64BoolField("masked"),
			"cid":      Int256Field("correl_id"),
			"created":  Int64Field("created"),
		}).Count(&count)
		require.NoError(t, db.Error)
		return db
	})

	assert.Equal(t, "SELECT count(*) FROM \"test\" WHERE tag LIKE '%%stuff%%' AND tag NOT LIKE 'abc' AND tag ILIKE 'ABC' AND tag NOT ILIKE 'abc' LIMIT 10", generatedSQL)
}

func TestBuildQueryJSONGreaterThan(t *testing.T) {

	var qf query.QueryJSON
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
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf, tx.Table("test"), FieldMap{
			"sequence": Int64Field("sequence"),
		}).Count(&count)
		require.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM \"test\" WHERE sequence > 0 LIMIT 10", generatedSQL)
}

func TestBuildQueryJSONLessThan(t *testing.T) {

	var qf query.QueryJSON
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
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf, tx.Table("test"), FieldMap{
			"amount": Uint256Field("amount"),
			"delta":  Int256Field("delta"),
		}).Count(&count)
		require.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM \"test\" WHERE amount < '0000000000000000000000000000000000000000000000000000000000003039' AND delta < '0ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff9c' LIMIT 10", generatedSQL)
}

func TestBuildQueryJSONGreaterThanOrEqual(t *testing.T) {

	var qf query.QueryJSON
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
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf, tx.Table("test"), FieldMap{
			"sequence": Int64Field("sequence"),
			"delta":    Int256Field("delta"),
		}).Count(&count)
		require.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM \"test\" WHERE sequence >= 0 AND delta >= '10000000000000000000000000000000000000000000000000000000000000064' LIMIT 10", generatedSQL)
}

func TestBuildQueryJSONLessThanOrEqual(t *testing.T) {

	var qf query.QueryJSON
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
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf, tx.Table("test"), FieldMap{
			"sequence": Int64Field("sequence"),
		}).Count(&count)
		require.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM \"test\" WHERE sequence <= 12345 LIMIT 10", generatedSQL)
}

func TestBuildQueryJSONIn(t *testing.T) {

	var qf query.QueryJSON
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
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf, tx.Table("test"), FieldMap{
			"tag": StringField("tag"),
		}).Count(&count)
		require.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM \"test\" WHERE \"tag\" = ANY ('{\"a\",\"b\",\"c\"}') AND tag NOT IN ('x','y','z') LIMIT 10", generatedSQL)
}

func TestBuildQueryJSONNestedAndOrWithANY(t *testing.T) {
	// This test verifies that nested expressions properly use ANY clauses instead of IN
	// when the UseAny function is applied to PostgreSQL queries
	var qf query.QueryJSON
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
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	// Apply the UseAny function to enable ANY clause replacement
	// Only need to do this because it's the mock provider
	persistence.UseAny(p.P.DB())

	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf, tx.Table("test"), FieldMap{
			"tag":      StringField("tag"),
			"sequence": Int64Field("sequence"),
			"masked":   Int64BoolField("masked"),
			"cid":      Int256Field("correl_id"),
		}).Count(&count)
		require.NoError(t, db.Error)

		return db
	})

	// Assert the complete SQL statement to verify ANY clause replacement
	// The expected SQL should use ANY clauses instead of IN for better PostgreSQL performance
	expectedSQL := `SELECT count(*) FROM "test" WHERE tag = 'a' AND masked = 1 AND sequence != 999 AND correl_id IS NULL AND sequence > 10 AND ((masked = 1 AND "tag" = ANY ('{"a","b","c"}') AND tag NOT IN ('x','y') AND tag NOT IN ('z')) OR masked = 0) LIMIT 10`
	assert.Equal(t, expectedSQL, generatedSQL)
}

func TestBuildQueryJSONComplexNestedWithANY(t *testing.T) {
	// This test covers deeply nested expressions with multiple levels of OR/AND combinations
	var qf query.QueryJSON
	err := json.Unmarshal([]byte(`{
		"limit": 50,
		"sort": ["created", "-priority"],
		"equal": [
			{
				"field": "status",
				"value": "active"
			}
		],
		"or": [
			{
				"and": [
					{
						"equal": [
							{
								"field": "category",
								"value": "urgent"
							}
						],
						"in": [
							{
								"field": "assignee",
								"values": ["alice", "bob", "charlie"]
							}
						],
						"greaterThan": [
							{
								"field": "priority",
								"value": 5
							}
						]
					}
				]
			},
			{
				"or": [
					{
						"equal": [
							{
								"field": "category",
								"value": "normal"
							}
						],
						"in": [
							{
								"field": "department",
								"values": ["engineering", "product", "design"]
							}
						],
						"nin": [
							{
								"field": "tags",
								"values": ["deprecated", "archived"]
							}
						]
					},
					{
						"and": [
							{
								"equal": [
									{
										"field": "category",
										"value": "low"
									}
								],
								"in": [
									{
										"field": "region",
										"values": ["us-east", "us-west", "eu-central"]
									}
								],
								"lessThan": [
									{
										"field": "age_days",
										"value": 30
									}
								]
							}
						]
					}
				]
			},
			{
				"equal": [
					{
						"field": "category",
						"value": "critical"
					}
				],
				"in": [
					{
						"field": "owner",
						"values": ["admin", "manager"]
					}
				],
				"null": [
					{
						"field": "archived_at"
					}
				]
			}
		]
	}`), &qf)
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	// Apply the UseAny function to enable ANY clause replacement
	// Only need to do this because it's the mock provider, on by default for the real provider
	persistence.UseAny(p.P.DB())

	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf, tx.Table("tasks"), FieldMap{
			"status":      StringField("status"),
			"category":    StringField("category"),
			"assignee":    StringField("assignee"),
			"priority":    Int64Field("priority"),
			"department":  StringField("department"),
			"tags":        StringField("tags"),
			"region":      StringField("region"),
			"age_days":    Int64Field("age_days"),
			"owner":       StringField("owner"),
			"archived_at": TimestampField("archived_at"),
			"created":     TimestampField("created"),
		}).Count(&count)
		require.NoError(t, db.Error)
		return db
	})

	// Assert the complete SQL statement to verify ANY clause replacement across all nested levels
	// The expected SQL should use ANY clauses instead of IN for better PostgreSQL performance
	expectedSQL := `SELECT count(*) FROM "tasks" WHERE status = 'active' AND ((category = 'normal' AND "department" = ANY ('{"engineering","product","design"}') AND tags NOT IN ('deprecated','archived')) OR (category = 'critical' AND archived_at IS NULL AND "owner" = ANY ('{"admin","manager"}'))) LIMIT 50`
	assert.Equal(t, expectedSQL, generatedSQL)

}

func TestBuildQueryJSONBadModifiers(t *testing.T) {

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	testJSON := func(j string) error {
		var qf query.QueryJSON
		err := json.Unmarshal([]byte(j), &qf)
		require.NoError(t, err)
		var count int64
		db := BuildGORM(context.Background(), &qf, p.P.DB().Table("test"), FieldMap{
			"tag": StringField("tag"),
		}).Count(&count)
		return db.Error
	}

	err = testJSON(`{"lessThan": [{"not": true, "field": "tag", "value": ""}]}`)
	assert.Regexp(t, "PD010702", err)

	err = testJSON(`{"lessThanOrEqual": [{"not": true, "field": "tag", "value": ""}]}`)
	assert.Regexp(t, "PD010702", err)

	err = testJSON(`{"greaterThan": [{"not": true, "field": "tag", "value": ""}]}`)
	assert.Regexp(t, "PD010702", err)

	err = testJSON(`{"greaterThanOrEqual": [{"not": true, "field": "tag", "value": ""}]}`)
	assert.Regexp(t, "PD010702", err)

	err = testJSON(`{"in": [{"caseInsensitive": true, "field": "tag", "value": ""}]}`)
	assert.Regexp(t, "PD010702", err)

	err = testJSON(`{"or": [{"in": [{"caseInsensitive": true, "field": "tag", "value": ""}]}] }`)
	assert.Regexp(t, "PD010702", err)

}

func TestBuildQueryJSONBadFields(t *testing.T) {

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	testJSON := func(j string) error {
		var qf query.QueryJSON
		err := json.Unmarshal([]byte(j), &qf)
		require.NoError(t, err)
		var count int64
		db := BuildGORM(context.Background(), &qf, p.P.DB().Table("test"), FieldMap{
			"tag": StringField("tag"),
		}).Count(&count)
		return db.Error
	}

	err = testJSON(`{"sort": ["-wrong"]}`)
	assert.Regexp(t, "PD010700", err)

	err = testJSON(`{"equal": [{"field": "wrong"}]}`)
	assert.Regexp(t, "PD010700", err)

	err = testJSON(`{"like": [{"field": "wrong"}]}`)
	assert.Regexp(t, "PD010700", err)

	err = testJSON(`{"lessThan": [{"field": "wrong"}]}`)
	assert.Regexp(t, "PD010700", err)

	err = testJSON(`{"lessThanOrEqual": [{"field": "wrong"}]}`)
	assert.Regexp(t, "PD010700", err)

	err = testJSON(`{"greaterThan": [{"field": "wrong"}]}`)
	assert.Regexp(t, "PD010700", err)

	err = testJSON(`{"greaterThanOrEqual": [{"field": "wrong"}]}`)
	assert.Regexp(t, "PD010700", err)

	err = testJSON(`{"in": [{"field": "wrong"}]}`)
	assert.Regexp(t, "PD010700", err)

	err = testJSON(`{"null": [{"field": "wrong"}]}`)
	assert.Regexp(t, "PD010700", err)
}

func TestBuildQueryJSONContainsShortNames(t *testing.T) {

	var qf1 query.QueryJSON
	err := json.Unmarshal([]byte(`{
		"eq": [
			{
				"field": "sequence",
				"value": "12345"
			}
		]
	}`), &qf1)
	require.NoError(t, err)

	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	generatedSQL := p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf1, tx.Table("test"), FieldMap{
			"sequence": Int64Field("sequence"),
		}).Count(&count)
		require.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM \"test\" WHERE sequence = 12345", generatedSQL)

	var qf2 query.QueryJSON
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
	require.NoError(t, err)

	generatedSQL = p.P.DB().ToSQL(func(tx *gorm.DB) *gorm.DB {
		var count int64
		db := BuildGORM(context.Background(), &qf2, tx.Table("test"), FieldMap{
			"sequence": Int64Field("sequence"),
		}).Count(&count)
		require.NoError(t, db.Error)
		return db
	})
	assert.Equal(t, "SELECT count(*) FROM \"test\" WHERE sequence <= 12345 AND sequence > 12345", generatedSQL)
}
