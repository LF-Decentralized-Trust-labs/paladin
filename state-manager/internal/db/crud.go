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
	// *** REMEMBER to add to UTDeleteAllData() below too ***
	States() StatesCRUD

	RunTransaction(ctx context.Context, fn func(ctx context.Context) error) error
	UTDeleteAllData(ctx context.Context) error
}

type crudDeleteMany interface {
	DeleteMany(ctx context.Context, filter ffapi.Filter, hooks ...dbsql.PostCompletionHook) (err error)
}

// Efficient cleanup function to speed up tests - do not use for any other purposes
func (p *persistence) UTDeleteAllData(ctx context.Context) error {
	return p.RunTransaction(ctx, func(ctx context.Context) (err error) {
		err = p.deleteChain(ctx, err, p.States())
		return err
	})
}

func (p *persistence) deleteChain(ctx context.Context, lastErr error, db crudDeleteMany) error {
	fb := (&ffapi.QueryFields{}).NewFilter(ctx).And()
	if lastErr == nil {
		lastErr = db.DeleteMany(ctx, fb)
	}
	return lastErr
}

type persistence struct {
	db *dbsql.Database
}

func InitConfig(conf config.Section) *Postgres {
	newPsql := &Postgres{}
	newPsql.InitConfig(conf)

	return newPsql
}

func NewPersistencePSQL(newPsql *Postgres) Persistence {
	return newPersistence(&newPsql.Database)
}

func newPersistence(db *dbsql.Database) Persistence {
	p := &persistence{
		db: db,
	}
	return p
}

func (p *persistence) RunTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return p.db.RunAsGroup(ctx, fn)
}

// For tests
func UTNewPersistenceDB(db *dbsql.Database) Persistence {
	return newPersistence(db)
}
