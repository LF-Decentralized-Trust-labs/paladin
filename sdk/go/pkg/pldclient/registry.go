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

package pldclient

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
)

type Registry interface {
	RPCModule

	Registries(ctx context.Context) (registryNames []string, err error)
	QueryEntries(ctx context.Context, registryName string, jq query.QueryJSON, activeFilter pldtypes.Enum[pldapi.ActiveFilter]) (entries []*pldapi.RegistryEntry, err error)
	QueryEntriesWithProps(ctx context.Context, registryName string, jq query.QueryJSON, activeFilter pldtypes.Enum[pldapi.ActiveFilter]) (entries []*pldapi.RegistryEntryWithProperties, err error)
	GetEntryProperties(ctx context.Context, registryName string, entryID pldtypes.HexBytes, activeFilter pldtypes.Enum[pldapi.ActiveFilter]) (entries []*pldapi.RegistryProperty, err error)
}

// This is necessary because there's no way to introspect function parameter names via reflection
var registryInfo = &rpcModuleInfo{
	group: "reg",
	methodInfo: map[string]RPCMethodInfo{
		"reg_registries": {
			Inputs: []string{},
			Output: "registryNames",
		},
		"reg_queryEntries": {
			Inputs: []string{"registryName", "query", "activeFilter"},
			Output: "entries",
		},
		"reg_queryEntriesWithProps": {
			Inputs: []string{"registryName", "query", "activeFilter"},
			Output: "entries",
		},
		"reg_getEntryProperties": {
			Inputs: []string{"registryName", "entryId", "activeFilter"},
			Output: "properties",
		},
	},
}

type registry struct {
	*rpcModuleInfo
	c *paladinClient
}

func (c *paladinClient) Registry() Registry {
	return &registry{rpcModuleInfo: registryInfo, c: c}
}

func (r *registry) Registries(ctx context.Context) (registryNames []string, err error) {
	err = r.c.CallRPC(ctx, &registryNames, "reg_registries")
	return
}

func (r *registry) QueryEntries(ctx context.Context, registryName string, jq query.QueryJSON, activeFilter pldtypes.Enum[pldapi.ActiveFilter]) (entries []*pldapi.RegistryEntry, err error) {
	err = r.c.CallRPC(ctx, &entries, "reg_queryEntries", registryName, jq, activeFilter)
	return
}

func (r *registry) QueryEntriesWithProps(ctx context.Context, registryName string, jq query.QueryJSON, activeFilter pldtypes.Enum[pldapi.ActiveFilter]) (entries []*pldapi.RegistryEntryWithProperties, err error) {
	err = r.c.CallRPC(ctx, &entries, "reg_queryEntriesWithProps", registryName, jq, activeFilter)
	return
}

func (r *registry) GetEntryProperties(ctx context.Context, registryName string, entryID pldtypes.HexBytes, activeFilter pldtypes.Enum[pldapi.ActiveFilter]) (properties []*pldapi.RegistryProperty, err error) {
	err = r.c.CallRPC(ctx, &properties, "reg_getEntryProperties", registryName, entryID, activeFilter)
	return
}
