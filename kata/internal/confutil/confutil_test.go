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

package confutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInt(t *testing.T) {
	assert.Equal(t, 12345, Int(nil, 12345))
	assert.Equal(t, 23456, Int(P(23456), 12345))
	assert.Equal(t, 10, IntMin(P(0), 1, 10))
	assert.Equal(t, 5, IntMin(P(5), 1, 10))
}

func TestInt64(t *testing.T) {
	assert.Equal(t, int64(12345), Int64(nil, 12345))
	assert.Equal(t, int64(23456), Int64(P(int64(23456)), 12345))
	assert.Equal(t, int64(10), Int64Min(P(int64(0)), 1, 10))
	assert.Equal(t, int64(5), Int64Min(P(int64(5)), 1, 10))
}

func TestBool(t *testing.T) {
	assert.True(t, Bool(nil, true))
	assert.False(t, Bool(nil, false))
	assert.True(t, Bool(P(true), false))
}

func TestDuration(t *testing.T) {
	assert.Equal(t, 50*time.Second, Duration(nil, "50s"))
	assert.Equal(t, 50*time.Second, Duration(P("wrong"), "50s"))
	assert.Equal(t, 100*time.Millisecond, Duration(P("100ms"), "50s"))
}

func TestDurationToBeMerged(t *testing.T) {
	assert.Equal(t, 50*time.Second, *DurationToBeMerged(nil, P(50*time.Second)))
	assert.Equal(t, 100*time.Millisecond, *DurationToBeMerged(P(100*time.Millisecond), P(50*time.Second)))
}
