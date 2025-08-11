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
	"sort"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
)

type WithValueSet interface {
	ValueSet() ValueSet
}

type ValueSetSorter[T WithValueSet] struct {
	Values   []T
	LessFunc func(i, j T) bool
	Error    error
}

func (s *ValueSetSorter[T]) Len() int           { return len(s.Values) }
func (s *ValueSetSorter[T]) Swap(i, j int)      { s.Values[i], s.Values[j] = s.Values[j], s.Values[i] }
func (s *ValueSetSorter[T]) Less(i, j int) bool { return s.LessFunc(s.Values[i], s.Values[j]) }
func (s *ValueSetSorter[T]) SetError(err error) { s.Error = err }

func SortedValueSetCopy[T WithValueSet](ctx context.Context, fieldSet FieldSet, values []T, sortInstructions ...string) ([]T, error) {
	valuesCopy := make([]T, len(values))
	copy(valuesCopy, values)

	sorter, err := NewValueSetSorter(ctx, fieldSet, valuesCopy, sortInstructions...)
	if err != nil {
		return nil, err
	}

	sort.Sort(sorter)
	err = sorter.Error
	if err != nil {
		return nil, err
	}
	return valuesCopy, nil
}

func SortValueSetInPlace[T WithValueSet](ctx context.Context, fieldSet FieldSet, values []T, sortInstructions ...string) error {
	sorter, err := NewValueSetSorter(ctx, fieldSet, values, sortInstructions...)
	if err != nil {
		return err
	}

	sort.Sort(sorter)
	return sorter.Error
}

func NewValueSetSorter[T WithValueSet](ctx context.Context, fieldSet FieldSet, values []T, sortInstructions ...string) (*ValueSetSorter[T], error) {

	if len(sortInstructions) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgFiltersMissingSortField)
	}
	sortFields := make([]*sortField, len(sortInstructions))
	for i, s := range sortInstructions {
		sortField, err := resolveSortField(ctx, fieldSet, s)
		if err != nil {
			return nil, err
		}
		sortFields[i] = sortField
	}

	// We can only log errors during compare
	sorter := &ValueSetSorter[T]{Values: values}
	sorter.LessFunc = func(i, j T) bool {
		compare := int64(0)
		for _, sortField := range sortFields {
			vI, err := i.ValueSet().GetValue(ctx, sortField.fieldName, sortField.field)
			if err != nil {
				sorter.SetError(err)
			}

			vJ, err := j.ValueSet().GetValue(ctx, sortField.fieldName, sortField.field)
			if err != nil {
				sorter.SetError(err)
			}

			if sortField.direction == directionDescending {
				vI, vJ = vJ, vI
			}

			// Handle either value being nil at this point
			if vI == nil && vJ != nil {
				compare = -1
			} else if vI != nil && vJ == nil {
				compare = 1
			} else {
				switch vtI := vI.(type) {
				case string:
					vtJ, ok := vJ.(string)
					if !ok {
						sorter.SetError(i18n.NewError(ctx, msgs.MsgFiltersTypeErrorDuringCompare, vI, vJ))
						compare = -1
					} else {
						compare = (int64)(strings.Compare(vtI, vtJ))
					}
				case int64:
					vtJ, ok := vJ.(int64)
					if !ok {
						sorter.SetError(i18n.NewError(ctx, msgs.MsgFiltersTypeErrorDuringCompare, vI, vJ))
						compare = -1
					} else if vtI > vtJ {
						compare = 1
					} else if vtI < vtJ {
						compare = -1
					}
				default:
					// We only support a limited number of types from field resolvers as above
					sorter.SetError(i18n.NewError(ctx, msgs.MsgFiltersTypeErrorDuringCompare, vI, vJ))
					compare = -1
				}
			}
			// If we have a winner, return the less() result, rather than moving onto next field
			if compare != 0 {
				return compare < 0
			}
		}

		// If we have a draw after all the fields, return false from less() function
		return false
	}

	return sorter, nil

}
