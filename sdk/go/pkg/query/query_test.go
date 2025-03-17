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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	// LimitKey is the key for the limit field in the query
	limitKey = "limit"
	// SortKey is the key for the sort field in the query
	sortKey = "sort"
	// EqKey is the key for the eq field in the query
	eqKey = "eq"
	// NqKey is the key for the nq field in the query
	// nqKey = "neq"
	// GtKey is the key for the gt field in the query
	// gtKey = "gt"
	// LtKey is the key for the lt field in the query
	// ltKey = "lt"
	// LeKey is the key for the less than or equal field in the query
	// leKey = "lte"
	// GeKey is the key for the greater than or equal field in the query
	// geKey = "gte"
	// InKey is the key for the in field in the query
	inKey = "in"
	// NinKey is the key for the not in field in the query
	ninKey = "nin"
	// isNullKey is the key for the is null field in the query
	isNullKey = "null"
	// LikeKey is the key for the like field in the query
	likeKey = "like"
	// CaseInsensitiveKey is the key for the case insensitive flag in the query
	// caseInsensitiveKey = "caseInsensitive"
)

func TestQuery(t *testing.T) {
	expectedQuery := `{
        "limit": 10,
        "sort": ["field1 DESC","field2"],
        "eq": [
            { "field": "field1", "value": "value1" },
            { "field": "field12", "value": "value12", "not": true, "caseInsensitive": true }
        ],
        "neq": [
            { "field": "field2", "value": "value2" }
        ],
        "like": [
            { "field": "field3", "value": "some value" }
        ],
        "lt": [
            { "field": "field4", "value": 12345 }
        ],
        "lte": [
            { "field": "field5", "value": 23456 }
        ],
        "gt": [
            { "field": "field6", "value": 34567 }
        ],
        "gte": [
            { "field": "field7", "value": 45678 }
        ],
        "in": [
            { "field": "field8", "values": ["a","b","c"] }
        ],
        "nin": [
            { "field": "field9", "values": ["x","y","z"] }
        ],
        "null": [
            { "field": "field10", "not": true },
            { "field": "field11" }
        ]
    }`

	query := NewQueryBuilder().
		Limit(10).
		Sort("field1 DESC").Sort("field2").
		Equal("field1", "value1", CaseSensitive).
		NotEqual("field2", "value2").
		Like("field3", "some value").
		LessThan("field4", 12345).
		LessThanOrEqual("field5", 23456).
		GreaterThan("field6", 34567).
		GreaterThanOrEqual("field7", 45678).
		In("field8", []any{"a", "b", "c"}).
		NotIn("field9", []any{"x", "y", "z"}).
		NotNull("field10").
		Null("field11").
		Equal("field12", "value12", Not, CaseInsensitive).
		Query()

	jsonQuery, err := query.JSON()
	assert.NoError(t, err)
	assert.JSONEq(t, expectedQuery, string(jsonQuery))

	stringQuery := query.String()
	assert.JSONEq(t, expectedQuery, stringQuery)
}

func TestQuery_StringOr(t *testing.T) {
	expectedQuery := `{
        "or": [
            {
                "eq": [
                    { "field": "field1", "value": "value1" }
                ],
                "neq": [
                    { "field": "field2", "value": "value2" }
                ]
            },
            {
                "eq": [
                    { "field": "field3", "value": "value3" }
                ],
                "neq": [
                    { "field": "field4", "value": "value4" }
                ]
            }
        ]
    }`

	query := NewQueryBuilder().
		Or(
			NewQueryBuilder().
				Equal("field1", "value1").
				NotEqual("field2", "value2"),
		).
		Or(
			NewQueryBuilder().
				Equal("field3", "value3").
				NotEqual("field4", "value4"),
		).
		Query().String()

	assert.JSONEq(t, expectedQuery, query)
}

func assertQueryEqual(t *testing.T, jsonMap map[string]interface{}, jq *QueryJSON) {
	jmb, err := json.Marshal(jsonMap)
	assert.NoError(t, err)
	jqb, err := json.Marshal(jq)
	assert.NoError(t, err)
	assert.JSONEq(t, string(jmb), string(jqb))
}

func TestQueryBuilderImpl_Limit(t *testing.T) {
	tests := []struct {
		name     string
		limit    int
		expected map[string]interface{}
	}{
		{
			name:  "Set positive limit",
			limit: 10,
			expected: map[string]interface{}{
				limitKey: int(10),
			},
		},
		{
			name:  "Set zero limit",
			limit: 0,
			expected: map[string]interface{}{
				limitKey: int(0),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := NewQueryBuilder()
			qb.Limit(tt.limit)
			assertQueryEqual(t, tt.expected, qb.Query())
		})
	}
}
func TestQueryBuilderImpl_Sort(t *testing.T) {
	tests := []struct {
		name     string
		fields   []string
		expected map[string]interface{}
	}{
		{
			name:   "Set single sort field",
			fields: []string{"field1"},
			expected: map[string]interface{}{
				sortKey: []string{"field1"},
			},
		},
		{
			name:   "Set multiple sort fields",
			fields: []string{"field1", "field2"},
			expected: map[string]interface{}{
				sortKey: []string{"field1", "field2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := NewQueryBuilder()
			for _, field := range tt.fields {
				qb.Sort(field)
			}
			assertQueryEqual(t, tt.expected, qb.Query())
		})
	}
}

func TestQueryBuilderImpl_In(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		values   []any
		adds     []addOns
		expected map[string]interface{}
	}{
		{
			name:   "Set single in field",
			field:  "name",
			values: []any{"John", "Doe"},
			expected: map[string]interface{}{
				inKey: []map[string]interface{}{
					{"field": "name", "values": []string{"John", "Doe"}},
				},
			},
		},
		{
			name:   "Set in field with Not add-on",
			field:  "name",
			values: []any{"John", "Doe"},
			adds:   []addOns{Not},
			expected: map[string]interface{}{
				inKey: []map[string]interface{}{
					{"field": "name", "values": []string{"John", "Doe"}, "not": true},
				},
			},
		},
		{
			name:   "Set in field with CaseInsensitive add-on",
			field:  "name",
			values: []any{"John", "Doe"},
			adds:   []addOns{CaseInsensitive},
			expected: map[string]interface{}{
				inKey: []map[string]interface{}{
					{"field": "name", "values": []string{"John", "Doe"}, "caseInsensitive": true},
				},
			},
		},
		{
			name:   "Set in field with multiple add-ons",
			field:  "name",
			values: []any{"John", "Doe"},
			adds:   []addOns{Not, CaseInsensitive},
			expected: map[string]interface{}{
				inKey: []map[string]interface{}{
					{"field": "name", "values": []string{"John", "Doe"}, "not": true, "caseInsensitive": true},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := NewQueryBuilder()
			qb.In(tt.field, tt.values, tt.adds...)
			assertQueryEqual(t, tt.expected, qb.Query())
		})
	}
}
func TestQueryBuilderImpl_NotIn(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		values   []any
		adds     []addOns
		expected map[string]interface{}
	}{
		{
			name:   "Set single not in field",
			field:  "name",
			values: []any{"John", "Doe"},
			expected: map[string]interface{}{
				ninKey: []map[string]interface{}{
					{"field": "name", "values": []string{"John", "Doe"}},
				},
			},
		},
		{
			name:   "Set not in field with Not add-on",
			field:  "name",
			values: []any{"John", "Doe"},
			adds:   []addOns{Not},
			expected: map[string]interface{}{
				ninKey: []map[string]interface{}{
					{"field": "name", "values": []string{"John", "Doe"}, "not": true},
				},
			},
		},
		{
			name:   "Set not in field with CaseInsensitive add-on",
			field:  "name",
			values: []any{"John", "Doe"},
			adds:   []addOns{CaseInsensitive},
			expected: map[string]interface{}{
				ninKey: []map[string]interface{}{
					{"field": "name", "values": []string{"John", "Doe"}, "caseInsensitive": true},
				},
			},
		},
		{
			name:   "Set not in field with multiple add-ons",
			field:  "name",
			values: []any{"John", "Doe"},
			adds:   []addOns{Not, CaseInsensitive},
			expected: map[string]interface{}{
				ninKey: []map[string]interface{}{
					{"field": "name", "values": []string{"John", "Doe"}, "not": true, "caseInsensitive": true},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := NewQueryBuilder()
			qb.NotIn(tt.field, tt.values, tt.adds...)

			assertQueryEqual(t, tt.expected, qb.Query())
		})
	}
}
func TestQueryBuilderImpl_Null(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		expected map[string]interface{}
	}{
		{
			name:  "Set single is null field",
			field: "name",
			expected: map[string]interface{}{
				isNullKey: []map[string]interface{}{
					{"field": "name"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := NewQueryBuilder()
			qb.Null(tt.field)
			assertQueryEqual(t, tt.expected, qb.Query())
		})
	}
}
func TestQueryBuilderImpl_Like(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		value    string
		expected map[string]interface{}
	}{
		{
			name:  "Set single like field",
			field: "name",
			value: "John",
			expected: map[string]interface{}{
				likeKey: []map[string]interface{}{
					{"field": "name", "value": "John"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := NewQueryBuilder()
			qb.Like(tt.field, tt.value)
			assertQueryEqual(t, tt.expected, qb.Query())
		})
	}
}
func TestQueryBuilderImpl_NotLike(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		value    string
		expected map[string]interface{}
	}{
		{
			name:  "Set single not like field",
			field: "name",
			value: "John",
			expected: map[string]interface{}{
				likeKey: []map[string]interface{}{
					{"field": "name", "value": "John", "not": true},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := NewQueryBuilder()
			qb.NotLike(tt.field, tt.value)
			assertQueryEqual(t, tt.expected, qb.Query())
		})
	}
}

func TestQueryBuilderImpl_setField(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		value    interface{}
		adds     []addOns
		expected map[string]interface{}
	}{
		{
			name:  "Set single field",
			field: "name",
			value: "John",
			expected: map[string]interface{}{
				eqKey: []map[string]interface{}{
					{"field": "name", "value": "John"},
				},
			},
		},
		{
			name:  "Set field with Not add-on",
			field: "name",
			value: "John",
			adds:  []addOns{Not},
			expected: map[string]interface{}{
				eqKey: []map[string]interface{}{
					{"field": "name", "value": "John", "not": true},
				},
			},
		},
		{
			name:  "Set field with CaseInsensitive add-on",
			field: "name",
			value: "John",
			adds:  []addOns{CaseInsensitive},
			expected: map[string]interface{}{
				eqKey: []map[string]interface{}{
					{"field": "name", "value": "John", "caseInsensitive": true},
				},
			},
		},
		{
			name:  "Set field with multiple add-ons",
			field: "name",
			value: "John",
			adds:  []addOns{Not, CaseInsensitive},
			expected: map[string]interface{}{
				eqKey: []map[string]interface{}{
					{"field": "name", "value": "John", "not": true, "caseInsensitive": true},
				},
			},
		},
		{
			name:  "Set multiple fields",
			field: "name",
			value: "John",
			expected: map[string]interface{}{
				eqKey: []map[string]interface{}{
					{"field": "name", "value": "John"},
					{"field": "age", "value": "30"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := NewQueryBuilder()
			qb = qb.Equal(tt.field, tt.value, tt.adds...)
			if tt.name == "Set multiple fields" {
				qb = qb.Equal("age", "30", tt.adds...)
			}
			assertQueryEqual(t, tt.expected, qb.Query())
		})
	}
}
func TestQueryBuilderImpl_setFields(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		values   []any
		adds     []addOns
		expected map[string]interface{}
	}{
		{
			name:   "Set single field with values",
			field:  "name",
			values: []any{"John", "Doe"},
			expected: map[string]interface{}{
				inKey: []map[string]interface{}{
					{"field": "name", "values": []string{"John", "Doe"}},
				},
			},
		},
		{
			name:   "Set field with Not add-on",
			field:  "name",
			values: []any{"John", "Doe"},
			adds:   []addOns{Not},
			expected: map[string]interface{}{
				inKey: []map[string]interface{}{
					{"field": "name", "values": []string{"John", "Doe"}, "not": true},
				},
			},
		},
		{
			name:   "Set field with CaseInsensitive add-on",
			field:  "name",
			values: []any{"John", "Doe"},
			adds:   []addOns{CaseInsensitive},
			expected: map[string]interface{}{
				inKey: []map[string]interface{}{
					{"field": "name", "values": []string{"John", "Doe"}, "caseInsensitive": true},
				},
			},
		},
		{
			name:   "Set field with multiple add-ons",
			field:  "name",
			values: []any{"John", "Doe"},
			adds:   []addOns{Not, CaseInsensitive},
			expected: map[string]interface{}{
				inKey: []map[string]interface{}{
					{"field": "name", "values": []string{"John", "Doe"}, "not": true, "caseInsensitive": true},
				},
			},
		},
		{
			name:   "Set multiple fields",
			field:  "name",
			values: []any{"John", "Doe"},
			expected: map[string]interface{}{
				inKey: []map[string]interface{}{
					{"field": "name", "values": []string{"John", "Doe"}},
					{"field": "age", "values": []string{"30", "40"}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := NewQueryBuilder()
			qb = qb.In(tt.field, tt.values, tt.adds...)
			if tt.name == "Set multiple fields" {
				qb = qb.In("age", []any{"30", "40"})
			}
			assertQueryEqual(t, tt.expected, qb.Query())
		})
	}
}

func TestQueryImpl_JSON(t *testing.T) {
	q := NewQueryBuilder().Equal("key1", "value1").Query()
	expectedJSON := `{"eq":[{"field":"key1", "value":"value1"}]}`
	jsonBytes, err := q.JSON()
	assert.NoError(t, err)
	assert.JSONEq(t, expectedJSON, string(jsonBytes))
}

func TestQueryImpl_String(t *testing.T) {
	q := NewQueryBuilder().Equal("key1", "value1").Query()
	expectedString := `{"eq":[{"field":"key1", "value":"value1"}]}`
	assert.JSONEq(t, expectedString, q.String())
}
