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
	"io/fs"
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

func TestFloat64(t *testing.T) {
	assert.Equal(t, float64(10), Float64Min(P(float64(0)), 1, 10))
	assert.Equal(t, float64(5), Float64Min(P(float64(5)), 1, 10))
}

func TestUnixFilePerm(t *testing.T) {
	assert.Equal(t, fs.FileMode(0644), UnixFileMode(nil, "0644"))
	assert.Equal(t, fs.FileMode(0644), UnixFileMode(P(""), "0644"))
	assert.Equal(t, fs.FileMode(0000), UnixFileMode(P("0"), "0644"))
	assert.Equal(t, fs.FileMode(0600), UnixFileMode(P("0600"), "0644"))
	assert.Equal(t, fs.FileMode(0777), UnixFileMode(P("777"), "0644"))
	assert.Equal(t, fs.FileMode(0644), UnixFileMode(P("0778"), "0644"))
}

func TestBool(t *testing.T) {
	assert.True(t, Bool(nil, true))
	assert.False(t, Bool(nil, false))
	assert.True(t, Bool(P(true), false))
}

func TestStringNotEmpty(t *testing.T) {
	assert.Equal(t, "def", StringNotEmpty(nil, "def"))
	assert.Equal(t, "def", StringNotEmpty(P(""), "def"))
	assert.Equal(t, "val", StringNotEmpty(P("val"), "def"))
}

func TestStringOrEmpty(t *testing.T) {
	assert.Equal(t, "def", StringOrEmpty(nil, "def"))
	assert.Equal(t, "", StringOrEmpty(P(""), "def"))
	assert.Equal(t, "val", StringOrEmpty(P("val"), "def"))
}

func TestStringSlice(t *testing.T) {
	assert.Equal(t, []string{"def"}, StringSlice(nil, []string{"def"}))
	assert.Equal(t, []string{"set"}, StringSlice([]string{"set"}, []string{"def"}))
}

func TestDuration(t *testing.T) {
	assert.Equal(t, 50*time.Second, DurationMin(nil, 0, "50s"))
	assert.Equal(t, 50*time.Second, DurationMin(P("wrong"), 0, "50s"))
	assert.Equal(t, 100*time.Millisecond, DurationMin(P("100ms"), 0, "50s"))

	assert.Equal(t, int64(1000000000), DurationSeconds(P("1000000000000ms"), 0, "0s"))
	assert.Equal(t, int64(1000000001), DurationSeconds(P("1000000001000ms"), 0, "0s"))
}

func TestByteSize(t *testing.T) {
	assert.Equal(t, int64(1024*1024), ByteSize(nil, 0, "1Mb"))
	assert.Equal(t, int64(16*1024), ByteSize(P("16Kb"), 0, "1Mb"))
}
