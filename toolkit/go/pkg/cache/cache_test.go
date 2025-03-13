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

package cache

import (
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/stretchr/testify/assert"
)

func TestCache(t *testing.T) {

	c := NewCache[string, string](&pldconf.CacheConfig{}, &pldconf.CacheConfig{Capacity: confutil.P(1)})

	c.Set("key1", "val1")
	v, ok := c.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "val1", v)

	c.Set("key2", "val2")
	v, ok = c.Get("key2")
	assert.True(t, ok)
	assert.Equal(t, "val2", v)

	_, ok = c.Get("key1")
	assert.False(t, ok)

	c.Delete("key2")
	_, ok = c.Get("key2")
	assert.False(t, ok)

	assert.Equal(t, 1, c.Capacity())
}
