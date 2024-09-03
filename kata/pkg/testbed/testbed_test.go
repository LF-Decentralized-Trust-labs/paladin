/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package testbed

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/kaleido-io/paladin/kata/internal/componentmgr"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

func writeTestConfig(t *testing.T) (configFile string) {
	ctx := context.Background()
	log.SetLevel("debug")

	var conf *componentmgr.Config
	err := componentmgr.ReadAndParseYAMLFile(ctx, "../../test/config/sqlite.memory.config.yaml", &conf)
	assert.NoError(t, err)
	// For running in this unit test the dirs are different to the sample config
	conf.DB.SQLite.MigrationsDir = "../../db/migrations/sqlite"
	conf.DB.Postgres.MigrationsDir = "../../db/migrations/postgres"
	configFile = path.Join(t.TempDir(), "test.config.yaml")
	f, err := os.Create(configFile)
	assert.NoError(t, err)
	defer f.Close()
	err = yaml.NewEncoder(f).Encode(conf)
	assert.NoError(t, err)

	return configFile
}
