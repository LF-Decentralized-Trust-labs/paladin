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
	"regexp"
	"strings"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/types"
)

type ValueSet interface {
	GetValue(fieldName string) types.RawJSON
}

type inlineEval struct {
	ctx      context.Context
	valueSet ValueSet
	matches  bool
	err      error
}

func (t *inlineEval) NewRoot() Traverser[*inlineEval] {
	return &inlineEval{ctx: t.ctx, valueSet: t.valueSet, matches: true}
}

func (t *inlineEval) Result() *inlineEval {
	return t
}

func (t *inlineEval) HasError() error {
	return t.err
}

func (t *inlineEval) WithError(err error) Traverser[*inlineEval] {
	return t.withError(err)
}

func (t *inlineEval) withError(err error) *inlineEval {
	if t.err == nil {
		t.err = err
	}
	t.matches = false
	return t
}

func (t *inlineEval) Limit(l int) Traverser[*inlineEval] {
	// N/A
	return t
}

func (t *inlineEval) Order(order string) Traverser[*inlineEval] {
	// N/A
	return t
}

func (t *inlineEval) And(ot *inlineEval) Traverser[*inlineEval] {
	t.matches = t.matches && ot.matches
	return t
}

func (t *inlineEval) Or(ot *inlineEval) Traverser[*inlineEval] {
	t.matches = t.matches || ot.matches
	return t
}

func (t *inlineEval) doCompare(e *FilterJSONBase, fieldName string, field FieldResolver, testValue driver.Value,
	compareStrings func(caseInsensitive bool, s1, s2 string) bool,
	compareInt64 func(s1, s2 int64) bool,
) *inlineEval {
	// Get the actual value to compare against
	actualValue, err := field.SQLValue(t.ctx, t.valueSet.GetValue(fieldName))
	if err != nil {
		return t.withError(err)
	}
	// Get the value for the test value, to know what type test to perform
	var valMatches bool
	switch testValueTyped := testValue.(type) {
	case string:
		strValue, ok := actualValue.(string)
		if !ok {
			return t.withError(i18n.NewError(t.ctx, msgs.MsgFiltersUnexpectedResolvedValueType, actualValue, testValue))
		}
		valMatches = compareStrings(e.CaseInsensitive, strValue, testValueTyped)
	case int64:
		int64Value, ok := actualValue.(int64)
		if !ok {
			return t.withError(i18n.NewError(t.ctx, msgs.MsgFiltersUnexpectedResolvedValueType, actualValue, testValue))
		}
		valMatches = compareInt64(int64Value, testValueTyped)
	default:
		// We only support a limited number of types from field resolvers as above
		return t.withError(i18n.NewError(t.ctx, msgs.MsgFiltersUnexpectedFieldResolverType, testValue, field))
	}
	if t.err == nil {
		if e.Not {
			valMatches = !valMatches
		}
		t.matches = t.matches && valMatches
	}
	return t
}

func (t *inlineEval) IsEqual(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*inlineEval] {
	return t.isEqual(&e.FilterJSONBase, fieldName, field, testValue)
}

func (t *inlineEval) isEqual(e *FilterJSONBase, fieldName string, field FieldResolver, testValue driver.Value) *inlineEval {
	return t.doCompare(e, fieldName, field, testValue,
		func(caseInsensitive bool, s1, s2 string) bool {
			if caseInsensitive {
				return strings.EqualFold(s1, s2)
			}
			return s1 == s2
		},
		func(s1, s2 int64) bool {
			return s1 == s2
		},
	)
}

// We need to support LIKE - simple function to convert a
// \ escaped SQL LIKE function into a Go regexp for evaluation
func sqlLikeToRegexp(likeStr string) (*regexp.Regexp, error) {
	buff := new(strings.Builder)
	lastChar := rune(0)
	buff.WriteRune('^')
	for _, c := range likeStr {
		switch c {
		case '\\': // escape not currently configurable
			if lastChar == '\\' {
				// Double escape to get an escape char in the output
				buff.WriteRune('\\')
				buff.WriteRune('\\')
			}
		case '.', '^', '$', '*', '+', '-', '?', '(', ')', '[', ']', '{', '}', '|':
			// Escape this char in the regexp
			buff.WriteRune('\\')
			buff.WriteRune(c)
		case '_':
			if lastChar == '\\' {
				// This was escaped in the source
				buff.WriteRune('_')
			} else {
				// Match a single character
				buff.WriteRune('.')
			}
		case '%':
			if lastChar == '\\' {
				// This was escaped in the source
				buff.WriteRune('%')
			} else {
				// Do a lazy match
				buff.WriteString(".*?")
			}
		default:
			// Plain old character
			buff.WriteRune(c)
		}
		lastChar = c
	}
	buff.WriteRune('$')
	return regexp.Compile(likeStr)
}

func (t *inlineEval) IsLike(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*inlineEval] {
	return t.doCompare(&e.FilterJSONBase, fieldName, field, testValue,
		func(caseInsensitive bool, s1, s2 string) bool {
			re, err := sqlLikeToRegexp(s2)
			if err != nil {
				_ = t.WithError(i18n.NewError(t.ctx, msgs.MsgFiltersLikeConversionToRegexpFail, s2, err))
				return false
			}
			return re.MatchString(s1)
		},
		func(s1, s2 int64) bool {
			_ = t.WithError(i18n.NewError(t.ctx, msgs.MsgFiltersLikeNotSupportedForIntValue))
			return false
		},
	)
}

func (t *inlineEval) IsNull(e *FilterJSONBase, fieldName string, field FieldResolver) Traverser[*inlineEval] {
	var valMatches bool
	if e.Not {
		valMatches = t.valueSet.GetValue(fieldName) != nil
	} else {
		valMatches = t.valueSet.GetValue(fieldName) == nil
	}
	t.matches = t.matches && valMatches
	return t
}

func (t *inlineEval) IsLessThan(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*inlineEval] {
	return t.doCompare(&e.FilterJSONBase, fieldName, field, testValue,
		func(caseInsensitive bool, s1, s2 string) bool {
			return strings.Compare(s1, s2) < 0
		},
		func(s1, s2 int64) bool {
			return (s1 - s2) < 0
		},
	)
}

func (t *inlineEval) IsLessThanOrEqual(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*inlineEval] {
	return t.doCompare(&e.FilterJSONBase, fieldName, field, testValue,
		func(caseInsensitive bool, s1, s2 string) bool {
			return strings.Compare(s1, s2) <= 0
		},
		func(s1, s2 int64) bool {
			return (s1 - s2) <= 0
		},
	)
}

func (t *inlineEval) IsGreaterThan(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*inlineEval] {
	return t.doCompare(&e.FilterJSONBase, fieldName, field, testValue,
		func(caseInsensitive bool, s1, s2 string) bool {
			return strings.Compare(s1, s2) > 0
		},
		func(s1, s2 int64) bool {
			return (s1 - s2) > 0
		},
	)
}

func (t *inlineEval) IsGreaterThanOrEqual(e *FilterJSONKeyValue, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*inlineEval] {
	return t.doCompare(&e.FilterJSONBase, fieldName, field, testValue,
		func(caseInsensitive bool, s1, s2 string) bool {
			return strings.Compare(s1, s2) >= 0
		},
		func(s1, s2 int64) bool {
			return (s1 - s2) >= 0
		},
	)
}

func (t *inlineEval) IsIn(e *FilterJSONKeyValues, fieldName string, field FieldResolver, testValues []driver.Value) Traverser[*inlineEval] {
	isIn := true
	for _, v := range testValues {
		comp := t.NewRoot().Result().isEqual(&e.FilterJSONBase, fieldName, field, v)
		if comp.err != nil {
			return t.withError(comp.Result().err)
		}
		if comp.matches {
			isIn = true
			break
		}
	}
	if e.Not {
		isIn = !isIn
	}
	t.matches = t.matches && isIn
	return t
}
