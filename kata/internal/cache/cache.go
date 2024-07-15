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
	"github.com/Code-Hex/go-generics-cache/policy/lru"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
)

type Config struct {
	Capacity *int `yaml:"capacity"`
}

type Cache[K comparable, V any] interface {
	Get(key K) (V, bool)
	Set(key K, val V)
	Delete(key K)
}

type cache[K comparable, V any] struct {
	cache *lru.Cache[K, V]
}

func NewCache[K comparable, V any](conf *Config, defs *Config) Cache[K, V] {
	c := &cache[K, V]{
		cache: lru.NewCache[K, V](
			lru.WithCapacity(confutil.Int(conf.Capacity, *defs.Capacity)),
		),
	}
	return c
}

func (c *cache[K, V]) Get(key K) (V, bool) {
	return c.cache.Get(key)
}

func (c *cache[K, V]) Set(key K, val V) {
	c.cache.Set(key, val)
}

func (c *cache[K, V]) Delete(key K) {
	c.cache.Delete(key)
}
