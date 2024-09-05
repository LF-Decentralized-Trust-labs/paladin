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

package componentmgr

import (
	"context"
	"os"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/domainmgr"
	"github.com/kaleido-io/paladin/core/internal/plugins"
	"github.com/kaleido-io/paladin/core/internal/registrymgr"
	"github.com/kaleido-io/paladin/core/internal/rpcserver"
	"github.com/kaleido-io/paladin/core/internal/statestore"
	"github.com/kaleido-io/paladin/core/internal/transportmgr"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/core/pkg/signer/api"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tkmsgs"
	"gopkg.in/yaml.v2"
)

type Config struct {
	domainmgr.DomainManagerConfig       `yaml:",inline"`
	plugins.PluginManagerConfig         `yaml:",inline"`
	transportmgr.TransportManagerConfig `yaml:",inline"`
	registrymgr.RegistryManagerConfig   `yaml:",inline"`
	Log                                 log.Config          `yaml:"log"`
	Blockchain                          ethclient.Config    `yaml:"blockchain"`
	DB                                  persistence.Config  `yaml:"db"`
	RPCServer                           rpcserver.Config    `yaml:"rpcServer"`
	StateStore                          statestore.Config   `yaml:"statestore"`
	BlockIndexer                        blockindexer.Config `yaml:"blockIndexer"`
	Signer                              api.Config          `yaml:"signer"`
	TempDir                             *string             `yaml:"tempDir"`
}

func ReadAndParseYAMLFile(ctx context.Context, filePath string, config interface{}) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.L(ctx).Errorf("file not found: %s", filePath)
		return i18n.NewError(ctx, tkmsgs.MsgConfigFileMissing, filePath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		log.L(ctx).Errorf("failed to read file: %v", err)
		return i18n.NewError(ctx, tkmsgs.MsgConfigFileReadError, filePath, err.Error())
	}

	err = yaml.Unmarshal(data, config)
	if err != nil {
		log.L(ctx).Errorf("failed to parse file: %v", err)
		return i18n.NewError(ctx, tkmsgs.MsgConfigFileParseError, err.Error())
	}

	return nil
}
