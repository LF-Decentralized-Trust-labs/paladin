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

package components

import (
	"context"

	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
)

// Special mapped type of record used by the transport plugin to route to nodes.
// Configuration in the registry manager (which can handle any type of record) defines how to
// map certain records from certain registries to node transport entries.
type RegistryNodeTransportEntry struct {
	Node      string
	Registry  string
	Transport string
	Details   string
}

type RegistryManagerToRegistry interface {
	plugintk.RegistryAPI
	Initialized()
}

type RegistryManager interface {
	ManagerLifecycle
	ConfiguredRegistries() map[string]*pldconf.PluginConfig
	RegistryRegistered(name string, id uuid.UUID, toRegistry RegistryManagerToRegistry) (fromRegistry plugintk.RegistryCallbacks, err error)
	GetNodeTransports(ctx context.Context, node string) ([]*RegistryNodeTransportEntry, error)
	GetRegistry(ctx context.Context, name string) (Registry, error)
}

type Registry interface {
	QueryEntries(ctx context.Context, dbTX persistence.DBTX, fActive pldapi.ActiveFilter, jq *query.QueryJSON) ([]*pldapi.RegistryEntry, error)
	QueryEntriesWithProps(ctx context.Context, dbTX persistence.DBTX, fActive pldapi.ActiveFilter, jq *query.QueryJSON) ([]*pldapi.RegistryEntryWithProperties, error)
	GetEntryProperties(ctx context.Context, dbTX persistence.DBTX, fActive pldapi.ActiveFilter, entityIDs ...pldtypes.HexBytes) ([]*pldapi.RegistryProperty, error)
}
