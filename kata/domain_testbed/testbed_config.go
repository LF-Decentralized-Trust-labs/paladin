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

package main

import (
	"github.com/kaleido-io/paladin/kata/internal/blockindexer"
	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/internal/persistence"
	"github.com/kaleido-io/paladin/kata/internal/rpcclient"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
)

type TestbedBlockchainConfig struct {
	WS rpcclient.WSConfig `yaml:"ws"`
}

type TestbedDestinationsConfig struct {
	ToDomain   *string `yaml:"toDomain"`
	FromDomain *string `yaml:"fromDomain"`
}

type TestBedConfig struct {
	Blockchain   TestbedBlockchainConfig   `yaml:"blockchain"`
	CommsBus     commsbus.Config           `yaml:"bus"`
	DB           persistence.Config        `yaml:"db"`
	RPCServer    rpcserver.Config          `yaml:"rpcServer"`
	StateStore   statestore.Config         `yaml:"statestore"`
	BlockIndexer blockindexer.Config       `yaml:"blockIndexer"`
	TempDir      *string                   `yaml:"tempDir"`
	Destinations TestbedDestinationsConfig `yaml:"destinations"`
}
