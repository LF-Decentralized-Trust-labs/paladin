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

package db

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/dbsql"
	"github.com/hyperledger/firefly-common/pkg/ffapi"
)

type Persistence interface {
	Transactions() TransactionsCollection
	RunTransaction(ctx context.Context, fn func(ctx context.Context) error) error

	UTDeleteAllData(ctx context.Context) error
}

// Efficient cleanup function to speed up tests - do not use for any other purposes
func (p *persistence) UTDeleteAllData(ctx context.Context) error {
	return p.RunTransaction(ctx, func(ctx context.Context) (err error) {
		err = p.deleteChain(ctx, err, p.Transactions())
		return err
	})
}

type crudDeleteMany interface {
	DeleteMany(ctx context.Context, filter ffapi.Filter, hooks ...dbsql.PostCompletionHook) (err error)
}

func (p *persistence) deleteChain(ctx context.Context, lastErr error, db crudDeleteMany) error {
	fb := (&ffapi.QueryFields{}).NewFilter(ctx).And()
	if lastErr == nil {
		lastErr = db.DeleteMany(ctx, fb)
	}
	return lastErr
}

var psql *Postgres
var initialized bool = false

type persistence struct {
	db *dbsql.Database
}

// InitConfig gets called after config reset to initialize the config structure
func InitConfig(conf config.Section) {
	psql = &Postgres{}
	psql.InitConfig(conf)
}

// Init gets called once to initialize the database connection itself
func Init(ctx context.Context, conf config.Section) error {
	if !initialized {
		initialized = true
		return psql.Init(ctx, conf)
	}
	return nil
}

func (p *persistence) RunTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return p.db.RunAsGroup(ctx, fn)
}

// NewPersistence can be called as often as required (future versions might
// parameterize this with scoping information)
func NewPersistence() Persistence {
	return &persistence{
		db: &psql.Database,
	}
}
