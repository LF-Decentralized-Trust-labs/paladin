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

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
)

type registryManager struct {
	bgCtx context.Context
	mux   sync.Mutex

	conf *RegistryManagerConfig

	registriesByID   map[uuid.UUID]*registry
	registriesByName map[string]*registry
}

func NewRegistryManager(bgCtx context.Context, conf *RegistryManagerConfig) components.RegistryManager {
	return &registryManager{
		bgCtx:            bgCtx,
		conf:             conf,
		registriesByID:   make(map[uuid.UUID]*registry),
		registriesByName: make(map[string]*registry),
	}
}

func (rm *registryManager) Init(pic components.PreInitComponents) (*components.ManagerInitResult, error) {
	// RegistryManager does not rely on any other components during the pre-init phase (at the moment)
	// for QoS we may need persistence in the future, and this will be the plug point for the registry
	// when we have it

	return &components.ManagerInitResult{}, nil
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

func (rm *registryManager) ConfiguredRegistries() map[string]*plugins.PluginConfig {
	pluginConf := make(map[string]*plugins.PluginConfig)
	for name, conf := range rm.conf.Registries {
		pluginConf[name] = &conf.Plugin
	}
	return pluginConf
}

func (rm *registryManager) RegistryRegistered(name string, id uuid.UUID, toRegistry plugins.RegistryManagerToRegistry) (fromRegistry plugintk.RegistryCallbacks, err error) {
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
	return t, nil
}

func (rm *registryManager) GetNodeTransports(ctx context.Context, node string) ([]*components.RegistryNodeTransportEntry, error) {
	// Scroll through all the configured registries to see if one of them knows about this node
	var transports []*components.RegistryNodeTransportEntry
	for _, r := range rm.registriesByID {
		transports = append(transports, r.getNodeTransports(node)...)
	}
	if len(transports) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgRegistryNodeEntiresNotFound, node)
	}
	return transports, nil
}
