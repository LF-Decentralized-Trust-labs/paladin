// Copyright © 2025 Kaleido, Inc.
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

package i18n

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewError(t *testing.T) {
	err := NewError(context.Background(), TestError1)
	assert.Error(t, err)
}

func TestNewErrorTruncate(t *testing.T) {
	err := NewError(context.Background(), TestError3, "field", strings.Repeat("x", 3000))
	assert.Error(t, err)
	var ffe PDError
	assert.Implements(t, &ffe, err)
	assert.Equal(t, 400, interface{}(err).(PDError).HTTPStatus())
	assert.Equal(t, TestError3, interface{}(err).(PDError).MessageKey())
}

func TestWrapError(t *testing.T) {
	err := WrapError(context.Background(), fmt.Errorf("some error"), TestError1)
	assert.Error(t, err)
	var ffe PDError
	assert.Implements(t, &ffe, err)
	assert.Equal(t, 500, interface{}(err).(PDError).HTTPStatus())
	assert.Equal(t, TestError1, interface{}(err).(PDError).MessageKey())
	stackString := interface{}(err).(PDError).StackTrace()
	assert.NotEmpty(t, stackString)
}

func TestSafeStackFail(t *testing.T) {
	stackString := (&pdError{}).StackTrace()
	assert.Empty(t, stackString)
}

func TestWrapNilError(t *testing.T) {
	err := WrapError(context.Background(), nil, TestError1)
	assert.Error(t, err)
}

func TestStackWithDebug(t *testing.T) {
	err := WrapError(context.Background(), nil, TestError1)
	assert.Error(t, err)
}
