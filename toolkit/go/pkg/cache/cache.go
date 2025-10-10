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
	"sync/atomic"

	cacheimpl "github.com/Code-Hex/go-generics-cache"
	"github.com/Code-Hex/go-generics-cache/policy/lru"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
)

type Cache[K comparable, V any] interface {
	Get(key K) (V, bool)
	Set(key K, val V)
	Delete(key K)
	Capacity() int
	Clear()
}

type cache[K comparable, V any] struct {
	cache    atomic.Pointer[cacheimpl.Cache[K, V]]
	capacity int
}

func NewCache[K comparable, V any](conf *pldconf.CacheConfig, defs *pldconf.CacheConfig) Cache[K, V] {
	capacity := confutil.Int(conf.Capacity, *defs.Capacity)
	c := &cache[K, V]{
		capacity: capacity,
	}
	// go-generics-cache provides its own thread safety wrapper
	// and janitor for expiry of old records.
	// However, it does not support clear so we do that here
	c.Clear()
	return c
}

func (c *cache[K, V]) Get(key K) (V, bool) {
	return c.cache.Load().Get(key)
}

func (c *cache[K, V]) Set(key K, val V) {
	c.cache.Load().Set(key, val)
}

func (c *cache[K, V]) Delete(key K) {
	c.cache.Load().Delete(key)
}

func (c *cache[K, V]) Clear() {
	newCache := cacheimpl.New[K, V](cacheimpl.AsLRU[K, V](
		lru.WithCapacity(c.capacity),
	))
	c.cache.Store(newCache)
}

func (c *cache[K, V]) Capacity() int {
	return c.capacity
}
