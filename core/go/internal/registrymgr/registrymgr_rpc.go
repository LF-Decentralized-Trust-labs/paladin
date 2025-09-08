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

package registrymgr

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
)

func (rm *registryManager) RPCModule() *rpcserver.RPCModule {
	return rm.rpcModule
}

func (rm *registryManager) initRPC() {
	rm.rpcModule = rpcserver.NewRPCModule("reg").
		Add("reg_registries", rm.rpcListRegistries()).
		Add("reg_queryEntries", rm.rpcQueryEntries()).
		Add("reg_queryEntriesWithProps", rm.rpcQueryEntriesWithProps()).
		Add("reg_getEntryProperties", rm.rpcGetEntryProperties())
}

func (rm *registryManager) rpcListRegistries() rpcserver.RPCHandler {
	return rpcserver.RPCMethod0(func(ctx context.Context,
	) ([]string, error) {
		return rm.getRegistryNames(), nil
	})
}

func withRegistry[RET any](ctx context.Context, rm *registryManager, registryName string, fn func(r components.Registry) (RET, error)) (RET, error) {
	r, err := rm.GetRegistry(ctx, registryName)
	if err != nil {
		return *new(RET), err
	}
	return fn(r)
}

func (rm *registryManager) rpcQueryEntries() rpcserver.RPCHandler {
	return rpcserver.RPCMethod3(func(ctx context.Context,
		registryName string,
		jq query.QueryJSON,
		activeFilter pldtypes.Enum[pldapi.ActiveFilter],
	) ([]*pldapi.RegistryEntry, error) {
		return withRegistry(ctx, rm, registryName,
			func(r components.Registry) ([]*pldapi.RegistryEntry, error) {
				return r.QueryEntries(ctx, rm.p.NOTX(), activeFilter.V(), &jq)
			},
		)
	})
}

func (rm *registryManager) rpcQueryEntriesWithProps() rpcserver.RPCHandler {
	return rpcserver.RPCMethod3(func(ctx context.Context,
		registryName string,
		jq query.QueryJSON,
		activeFilter pldtypes.Enum[pldapi.ActiveFilter],
	) ([]*pldapi.RegistryEntryWithProperties, error) {
		return withRegistry(ctx, rm, registryName,
			func(r components.Registry) ([]*pldapi.RegistryEntryWithProperties, error) {
				return r.QueryEntriesWithProps(ctx, rm.p.NOTX(), activeFilter.V(), &jq)
			},
		)
	})
}

func (rm *registryManager) rpcGetEntryProperties() rpcserver.RPCHandler {
	return rpcserver.RPCMethod3(func(ctx context.Context,
		registryName string,
		entryID pldtypes.HexBytes,
		activeFilter pldtypes.Enum[pldapi.ActiveFilter],
	) ([]*pldapi.RegistryProperty, error) {
		return withRegistry(ctx, rm, registryName,
			func(r components.Registry) ([]*pldapi.RegistryProperty, error) {
				return r.GetEntryProperties(ctx, rm.p.NOTX(), activeFilter.V(), entryID)
			},
		)
	})
}
