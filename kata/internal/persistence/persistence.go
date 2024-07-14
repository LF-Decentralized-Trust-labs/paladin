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

package persistence

import (
	"context"

	// Import pq driver
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	_ "github.com/lib/pq"
)

type Persistence interface{}

const (
	TypePostgres = "postgres"
	TypeSQLite   = "sqlite"
)

type Config struct {
	Type     string         `yaml:"type"`
	Postgres PostgresConfig `yaml:"postgres"`
	SQLite   SQLiteConfig   `yaml:"sqlite"`
}

type PostgresConfig struct {
	SQLDBConfig
}

type SQLiteConfig struct {
	SQLDBConfig
}

func NewPersistence(ctx context.Context, conf *Config) (Persistence, error) {
	switch conf.Type {
	case "", TypeSQLite: // default
		return newSQLitePersistence(ctx, conf)
	case TypePostgres:
		return newPostgresPersistence(ctx, conf)
	default:
		return nil, i18n.NewError(ctx, msgs.MsgPersistenceInvalidType, conf.Type)
	}
}
