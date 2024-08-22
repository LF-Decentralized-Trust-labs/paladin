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
	"github.com/kaleido-io/paladin/kata/internal/domainmgr"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/internal/transportmgr"
	"github.com/kaleido-io/paladin/kata/pkg/blockindexer"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/kata/pkg/persistence"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
)

type Config struct {
	domainmgr.DomainManagerConfig
	transportmgr.TransportManagerConfig
	plugins.PluginControllerConfig
	Blockchain   ethclient.Config    `yaml:"blockchain"`
	DB           persistence.Config  `yaml:"db"`
	RPCServer    rpcserver.Config    `yaml:"rpcServer"`
	StateStore   statestore.Config   `yaml:"statestore"`
	BlockIndexer blockindexer.Config `yaml:"blockIndexer"`
	Signer       api.Config          `yaml:"signer"`
	TempDir      *string             `yaml:"tempDir"`
}
