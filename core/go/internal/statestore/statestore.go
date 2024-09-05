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

package statestore

import (
	"context"
	"sync"

	"github.com/kaleido-io/paladin/core/internal/cache"
	"github.com/kaleido-io/paladin/core/internal/rpcserver"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
)

type Config struct {
	SchemaCache cache.Config      `yaml:"schemaCache"`
	StateWriter StateWriterConfig `yaml:"stateWriter"`
}

type StateWriterConfig struct {
	WorkerCount  *int    `yaml:"workerCount"`
	BatchTimeout *string `yaml:"batchTimeout"`
	BatchMaxSize *int    `yaml:"batchMaxSize"`
}

var StateWriterConfigDefaults = StateWriterConfig{
	WorkerCount:  confutil.P(10),
	BatchTimeout: confutil.P("25ms"),
	BatchMaxSize: confutil.P(100),
}

type StateStore interface {
	RPCModule() *rpcserver.RPCModule
	RunInDomainContext(domainID string, fn DomainContextFunction) error
	RunInDomainContextFlush(domainID string, fn DomainContextFunction) error
	Close()
}

type stateStore struct {
	p              persistence.Persistence
	bgCtx          context.Context
	cancelCtx      context.CancelFunc
	writer         *stateWriter
	abiSchemaCache cache.Cache[string, Schema]
	rpcModule      *rpcserver.RPCModule
	domainLock     sync.Mutex
	domainContexts map[string]*domainContext
}

var SchemaCacheDefaults = &cache.Config{
	Capacity: confutil.P(1000),
}

func NewStateStore(ctx context.Context, conf *Config, p persistence.Persistence) StateStore {
	ss := &stateStore{
		p:              p,
		abiSchemaCache: cache.NewCache[string, Schema](&conf.SchemaCache, SchemaCacheDefaults),
		domainContexts: make(map[string]*domainContext),
	}
	ss.bgCtx, ss.cancelCtx = context.WithCancel(ctx)
	ss.writer = newStateWriter(ctx, ss, &conf.StateWriter)
	ss.initRPC()
	return ss
}

func (ss *stateStore) Close() {
	ss.writer.stop()
	ss.cancelCtx()
}
