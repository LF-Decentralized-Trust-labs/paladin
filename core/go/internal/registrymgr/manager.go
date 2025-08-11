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

package registrymgr

import (
	"context"
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/registrymgr/metrics"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/blockindexer"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/cache"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
)

type registryManager struct {
	bgCtx context.Context
	mux   sync.Mutex

	conf *pldconf.RegistryManagerConfig

	p            persistence.Persistence
	blockIndexer blockindexer.BlockIndexer
	rpcModule    *rpcserver.RPCModule

	// We provide a high level of customization of how the nodes are looked up in the registry
	registryTransportLookups map[string]*transportLookup

	// Due to the high frequency of calls to the registry for node details, we maintain
	// a cache of resolved nodes by name - which is a global index, across all registries.
	transportDetailsCache cache.Cache[string, []*components.RegistryNodeTransportEntry]

	registriesByID   map[uuid.UUID]*registry
	registriesByName map[string]*registry
	metrics          metrics.RegistryManagerMetrics
}

func NewRegistryManager(bgCtx context.Context, conf *pldconf.RegistryManagerConfig) components.RegistryManager {
	return &registryManager{
		bgCtx:                    bgCtx,
		conf:                     conf,
		registriesByID:           make(map[uuid.UUID]*registry),
		registriesByName:         make(map[string]*registry),
		registryTransportLookups: make(map[string]*transportLookup),
		transportDetailsCache:    cache.NewCache[string, []*components.RegistryNodeTransportEntry](&conf.RegistryManager.RegistryCache, pldconf.RegistryCacheDefaults),
	}
}

func (rm *registryManager) PreInit(pic components.PreInitComponents) (_ *components.ManagerInitResult, err error) {
	rm.p = pic.Persistence()
	rm.metrics = metrics.InitMetrics(rm.bgCtx, pic.MetricsManager().Registry())

	// For each of the registries, parse the transport lookup semantics
	for regName, regConf := range rm.conf.Registries {
		if confutil.Bool(regConf.Transports.Enabled, *pldconf.RegistryTransportsDefaults.Enabled) {
			if rm.registryTransportLookups[regName], err = newTransportLookup(rm.bgCtx, regName, &regConf.Transports); err != nil {
				return nil, err
			}
			log.L(rm.bgCtx).Infof("Transport lookups enabled for registry '%s' with matcher '%s'", regName, rm.registryTransportLookups[regName].propertyRegexp)
		}
	}
	rm.initRPC()
	return &components.ManagerInitResult{
		RPCModules: []*rpcserver.RPCModule{rm.rpcModule},
	}, nil
}

func (rm *registryManager) PostInit(c components.AllComponents) error {
	rm.blockIndexer = c.BlockIndexer()
	return nil
}

func (rm *registryManager) Start() error { return nil }

func (rm *registryManager) Stop() {
	rm.mux.Lock()
	var allRegistries []*registry
	for _, t := range rm.registriesByID {
		allRegistries = append(allRegistries, t)
	}
	rm.mux.Unlock()
	for _, t := range allRegistries {
		rm.cleanupRegistry(t)
	}

}

func (rm *registryManager) cleanupRegistry(t *registry) {
	// must not hold the registry lock when running this
	t.close()
	delete(rm.registriesByID, t.id)
	delete(rm.registriesByName, t.name)
}

func (rm *registryManager) ConfiguredRegistries() map[string]*pldconf.PluginConfig {
	pluginConf := make(map[string]*pldconf.PluginConfig)
	for name, conf := range rm.conf.Registries {
		pluginConf[name] = &conf.Plugin
	}
	return pluginConf
}

func (rm *registryManager) RegistryRegistered(name string, id uuid.UUID, toRegistry components.RegistryManagerToRegistry) (fromRegistry plugintk.RegistryCallbacks, err error) {
	rm.mux.Lock()
	defer rm.mux.Unlock()

	// Replaces any previously registered instance
	existing := rm.registriesByName[name]
	for existing != nil {
		// Can't hold the lock in cleanup, hence the loop
		rm.mux.Unlock()
		rm.cleanupRegistry(existing)
		rm.mux.Lock()
		existing = rm.registriesByName[name]
	}

	// Get the config for this registry
	conf := rm.conf.Registries[name]
	if conf == nil {
		// Shouldn't be possible
		return nil, i18n.NewError(rm.bgCtx, msgs.MsgRegistryNotFound, name)
	}

	// Initialize
	t := rm.newRegistry(id, name, conf, toRegistry)
	rm.registriesByID[id] = t
	rm.registriesByName[name] = t
	go t.init()
	rm.metrics.IncRegistries()
	return t, nil
}

func (rm *registryManager) GetRegistry(ctx context.Context, name string) (components.Registry, error) {
	rm.mux.Lock()
	defer rm.mux.Unlock()

	r := rm.registriesByName[name]
	if r == nil {
		return nil, i18n.NewError(ctx, msgs.MsgRegistryNotFound, name)
	}
	return r, nil
}

func (rm *registryManager) getRegistryNames() []string {
	rm.mux.Lock()
	defer rm.mux.Unlock()

	registryNames := make([]string, 0, len(rm.registriesByName))
	for registryName := range rm.registriesByName {
		registryNames = append(registryNames, registryName)
	}
	return registryNames
}

func (rm *registryManager) GetNodeTransports(ctx context.Context, node string) ([]*components.RegistryNodeTransportEntry, error) {
	// Check cache
	transports, present := rm.transportDetailsCache.Get(node)
	if present {
		return transports, nil
	}

	regLookupsChecked := 0
	for regName, r := range rm.registriesByName {
		tl := rm.registryTransportLookups[regName]
		if tl != nil {
			regLookupsChecked++
			regTransports, err := tl.getNodeTransports(ctx, rm.p.NOTX() /* no TX needed */, r, node)
			if err != nil {
				return nil, err
			}
			// we only return entries from a single registry (we do not merge transports across registries)
			// the requiredPrefix allows node partitioning across registries.
			if len(regTransports) > 0 {
				log.L(ctx).Infof("Node '%s' matched to %d transports in registry '%s'", node, len(regTransports), regName)
				rm.transportDetailsCache.Set(node, regTransports)
				return regTransports, nil
			}
		}
	}
	log.L(ctx).Infof("No transports found for node '%s' after checking %d registries configured with transports lookups", node, regLookupsChecked)

	return nil, i18n.NewError(ctx, msgs.MsgRegistryNodeEntiresNotFound, node)
}
