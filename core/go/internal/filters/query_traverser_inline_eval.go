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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
)

type ValueSet interface {
	// Implementation can choose whether it holds the SQL final value, or does the resolve on demand.
	// Result needs to be exactly as it would have been if passed through the resolver.
	// This includes handling of SQL NULL
	GetValue(ctx context.Context, fieldName string, resolver FieldResolver) (driver.Value, error)
}

type ResolvingValueSet map[string]pldtypes.RawJSON

func (vs ResolvingValueSet) GetValue(ctx context.Context, fieldName string, resolver FieldResolver) (driver.Value, error) {
	val, err := resolver.SQLValue(ctx, vs[fieldName])
	if err != nil {
		return nil, err
	}
	return val, nil
}

type PassthroughValueSet map[string]driver.Value

func (vs PassthroughValueSet) GetValue(ctx context.Context, fieldName string, resolver FieldResolver) (driver.Value, error) {
	return vs[fieldName], nil
}

func EvalQuery(ctx context.Context, qj *query.QueryJSON, fieldSet FieldSet, valueSet ValueSet) (bool, error) {
	eval := &inlineEval{
		inlineEvalRoot: &inlineEvalRoot{
			ctx:      ctx,
			valueSet: valueSet,
			convertLike: func(s string, caseInsensitive bool) (*regexp.Regexp, error) {
				// Use \ escaped LIKE conversion
				return sqlLikeToRegexp(s, caseInsensitive, '\\')
			},
		},
		matches: true,
	}
	qt := &queryTraverser[*inlineEval]{
		ctx:        ctx,
		jsonFilter: qj,
		fieldSet:   fieldSet,
	}
	res := qt.traverse(eval).T()
	return res.matches, res.err
}

type inlineEvalRoot struct {
	ctx         context.Context
	valueSet    ValueSet
	convertLike func(s string, caseInsensitive bool) (*regexp.Regexp, error)
}

type inlineEval struct {
	*inlineEvalRoot
	matches bool
	err     error
}

func (t *inlineEval) NewRoot() Traverser[*inlineEval] {
	return &inlineEval{inlineEvalRoot: t.inlineEvalRoot, matches: true}
}

func (t *inlineEval) T() *inlineEval {
	return t
}

func (t *inlineEval) Error() error {
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

func (t *inlineEval) BuildOr(ot ...*inlineEval) Traverser[*inlineEval] {
	orMatches := false
	for _, o := range ot {
		if o.err != nil {
			return t.WithError(o.err)
		}
		orMatches = orMatches || o.matches
	}
	t.matches = t.matches && orMatches
	return t
}

func (t *inlineEval) doCompare(e *query.Op, fieldName string, field FieldResolver, testValue driver.Value,
	compareStrings func(caseInsensitive bool, s1, s2 string) bool,
	compareInt64 func(s1, s2 int64) bool,
) *inlineEval {
	// Get the actual value to compare against
	actualValue, err := t.valueSet.GetValue(t.ctx, fieldName, field)
	if err != nil {
		return t.withError(err)
	}
	var valMatches bool
	if actualValue == nil {
		// Nil does not match any value - there's a separate nil check operation for that
		valMatches = false
	} else {
		// Get the value for the test value, to know what type test to perform
		// REMEMBER - if you update this function you must update BuildQueryCompareLessFunc() too
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
	}
	if t.err == nil {
		if e.Not {
			valMatches = !valMatches
		}
		t.matches = t.matches && valMatches
	}
	return t
}

func (t *inlineEval) IsEqual(e *query.OpSingleVal, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*inlineEval] {
	return t.isEqual(&e.Op, fieldName, field, testValue)
}

func (t *inlineEval) isEqual(e *query.Op, fieldName string, field FieldResolver, testValue driver.Value) *inlineEval {
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

func (t *inlineEval) IsLike(e *query.OpSingleVal, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*inlineEval] {
	return t.doCompare(&e.Op, fieldName, field, testValue,
		func(caseInsensitive bool, s1, s2 string) bool {
			re, err := t.convertLike(s2, caseInsensitive)
			if err != nil {
				// Unexpected as we should handle all cases
				_ = t.WithError(i18n.NewError(t.ctx, msgs.MsgFiltersLikeConversionToRegexpFail, s2, err))
				return false
			}
			return re.MatchString(s1)
		},
		t.int64LikeNotSupported,
	)
}

func (t *inlineEval) int64LikeNotSupported(s1, s2 int64) bool {
	_ = t.WithError(i18n.NewError(t.ctx, msgs.MsgFiltersLikeNotSupportedForIntValue))
	return false
}

func (t *inlineEval) IsNull(e *query.Op, fieldName string, field FieldResolver) Traverser[*inlineEval] {
	var valMatches bool
	actualValue, err := t.valueSet.GetValue(t.ctx, fieldName, field)
	if err != nil {
		return t.withError(err)
	}
	if e.Not {
		valMatches = actualValue != nil
	} else {
		valMatches = actualValue == nil
	}
	t.matches = t.matches && valMatches
	return t
}

func (t *inlineEval) IsLessThan(e *query.OpSingleVal, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*inlineEval] {
	return t.doCompare(&e.Op, fieldName, field, testValue,
		func(caseInsensitive bool, s1, s2 string) bool {
			return strings.Compare(s1, s2) < 0
		},
		func(s1, s2 int64) bool {
			return s1 < s2
		},
	)
}

func (t *inlineEval) IsLessThanOrEqual(e *query.OpSingleVal, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*inlineEval] {
	return t.doCompare(&e.Op, fieldName, field, testValue,
		func(caseInsensitive bool, s1, s2 string) bool {
			return strings.Compare(s1, s2) <= 0
		},
		func(s1, s2 int64) bool {
			return s1 <= s2
		},
	)
}

func (t *inlineEval) IsGreaterThan(e *query.OpSingleVal, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*inlineEval] {
	return t.doCompare(&e.Op, fieldName, field, testValue,
		func(caseInsensitive bool, s1, s2 string) bool {
			return strings.Compare(s1, s2) > 0
		},
		func(s1, s2 int64) bool {
			return s1 > s2
		},
	)
}

func (t *inlineEval) IsGreaterThanOrEqual(e *query.OpSingleVal, fieldName string, field FieldResolver, testValue driver.Value) Traverser[*inlineEval] {
	return t.doCompare(&e.Op, fieldName, field, testValue,
		func(caseInsensitive bool, s1, s2 string) bool {
			return strings.Compare(s1, s2) >= 0
		},
		func(s1, s2 int64) bool {
			return s1 >= s2
		},
	)
}

func (t *inlineEval) IsIn(e *query.OpMultiVal, fieldName string, field FieldResolver, testValues []driver.Value) Traverser[*inlineEval] {
	// Do not negate the check in the individual compares
	withoutNegate := e.Op
	withoutNegate.Not = false

	isIn := false
	for _, v := range testValues {
		comp := t.NewRoot().T().isEqual(&withoutNegate, fieldName, field, v)
		if comp.err != nil {
			return t.withError(comp.T().err)
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
