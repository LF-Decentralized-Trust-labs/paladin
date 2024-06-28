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

package manager

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/config"
	smconfig "github.com/kaleido-io/paladin-state-manager/internal/config"
	"github.com/kaleido-io/paladin-state-manager/internal/db"
)

type Config struct {
	Database config.Section
}

type PaladinStateManager interface {
}

type stateManagerService struct {
	config      Config
	persistence db.Persistence
}

func NewStateManagerService(ctx context.Context) (PaladinStateManager, error) {
	config := Config{
		Database: smconfig.DatabaseSection,
	}

	persistence, err := initDBConf(ctx, config.Database)
	if err != nil {
		return nil, err
	}
	return &stateManagerService{
		config:      config,
		persistence: persistence,
	}, nil
}

func initDBConf(ctx context.Context, databaseConfig config.Section) (db.Persistence, error) {
	// Get the postgres config
	postgresConfig := databaseConfig.SubSection(smconfig.ConfigDatabasePostgres)
	psql := db.InitConfig(postgresConfig)

	// Init the db connection
	if err := psql.Init(ctx, postgresConfig); err != nil {
		return nil, err
	}

	return db.NewPersistencePSQL(psql), nil
}
