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
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
)

// Domain manager is the boundary between the paladin core / testbed and the domains
type DomainManager interface {
	plugins.DomainRegistration
	GetDomainByName(ctx context.Context, name string) (DomainActions, error)
}

func NewDomainManager(bgCtx context.Context, conf *DomainManagerConfig, stateStore statestore.StateStore, chainID int64) DomainManager {
	return &domainManager{
		bgCtx:         bgCtx,
		stateStore:    stateStore,
		chainID:       chainID,
		conf:          conf,
		domainsByID:   make(map[uuid.UUID]*domain),
		domainsByName: make(map[string]*domain),
	}
}

type domainManager struct {
	bgCtx         context.Context
	mux           sync.Mutex
	conf          *DomainManagerConfig
	domainsByID   map[uuid.UUID]*domain
	domainsByName map[string]*domain
	stateStore    statestore.StateStore
	chainID       int64
}

func (dm *domainManager) ConfiguredDomains() map[string]*plugins.PluginConfig {
	pluginConf := make(map[string]*plugins.PluginConfig)
	for name, conf := range dm.conf.Domains {
		pluginConf[name] = &conf.Plugin
	}
	return pluginConf
}

func (dm *domainManager) DomainRegistered(name string, id uuid.UUID, toDomain plugintk.DomainAPI) (fromDomain plugintk.DomainCallbacks, err error) {
	dm.mux.Lock()
	defer dm.mux.Unlock()

	// Get the config for this domain
	conf := dm.conf.Domains[name]
	if conf == nil {
		// Shouldn't be possible
		return nil, i18n.NewError(dm.bgCtx, msgs.MsgDomainNotFound, name)
	}

	// Replaces any previously registered instance
	if existing := dm.domainsByID[id]; existing != nil {
		existing.close()
	}

	// Initialize
	d := dm.newDomain(id, name, conf, toDomain)
	dm.domainsByID[id] = d
	dm.domainsByName[name] = d
	go d.init()
	return d, nil
}

func (dm *domainManager) GetDomainByName(ctx context.Context, name string) (DomainActions, error) {
	dm.mux.Lock()
	defer dm.mux.Unlock()
	d := dm.domainsByName[name]
	if d == nil {
		return nil, i18n.NewError(ctx, msgs.MsgDomainNotFound, name)
	}
	return d, nil
}
