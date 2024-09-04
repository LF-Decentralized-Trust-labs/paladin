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
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
)

type registry struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	conf *RegistryConfig
	tm   *registryManager
	id   uuid.UUID
	name string
	api  plugins.RegistryManagerToRegistry

	// TODO: Replace with a cache-backed DB system
	stateLock           sync.Mutex
	inMemoryPlaceholder map[string][]*components.RegistryNodeTransportEntry

	initialized atomic.Bool
	initRetry   *retry.Retry

	initError atomic.Pointer[error]
	initDone  chan struct{}
}

func (tm *registryManager) newRegistry(id uuid.UUID, name string, conf *RegistryConfig, toRegistry plugins.RegistryManagerToRegistry) *registry {
	t := &registry{
		tm:                  tm,
		conf:                conf,
		initRetry:           retry.NewRetryIndefinite(&conf.Init.Retry),
		name:                name,
		id:                  id,
		api:                 toRegistry,
		inMemoryPlaceholder: make(map[string][]*components.RegistryNodeTransportEntry),
		initDone:            make(chan struct{}),
	}
	t.ctx, t.cancelCtx = context.WithCancel(log.WithLogField(tm.bgCtx, "registry", t.name))
	return t
}

func (t *registry) init() {
	defer close(t.initDone)

	// We block retrying each part of init until we succeed, or are cancelled
	// (which the plugin manager will do if the registry disconnects)
	err := t.initRetry.Do(t.ctx, func(attempt int) (bool, error) {
		// Send the configuration to the registry for processing
		confJSON, _ := json.Marshal(&t.conf.Config)
		_, err := t.api.ConfigureRegistry(t.ctx, &prototk.ConfigureRegistryRequest{
			Name:       t.name,
			ConfigJson: string(confJSON),
		})
		return true, err
	})
	if err != nil {
		log.L(t.ctx).Debugf("registry initialization cancelled before completion: %s", err)
		t.initError.Store(&err)
	} else {
		log.L(t.ctx).Debugf("registry initialization complete")
		t.initialized.Store(true)
		// Inform the plugin manager callback
		t.api.Initialized()
	}
}

func (t *registry) getNodeTransports(node string) []*components.RegistryNodeTransportEntry {
	t.stateLock.Lock()
	defer t.stateLock.Unlock()
	return t.inMemoryPlaceholder[node]
}

// Registry callback to the registry manager when new entries are available to upsert & cache
// (can be called during and after initialization asynchronously as pre-configured and updated information becomes known)
func (t *registry) UpsertTransportDetails(ctx context.Context, req *prototk.UpsertTransportDetails) (*prototk.UpsertTransportDetailsResponse, error) {

	t.stateLock.Lock()
	defer t.stateLock.Unlock()

	if req.Node == "" || req.Transport == "" {
		return nil, i18n.NewError(ctx, msgs.MsgRegistryInvalidEntry)
	}

	existingEntries := t.inMemoryPlaceholder[req.Node]
	deDuped := make([]*components.RegistryNodeTransportEntry, 0, len(existingEntries))
	for _, existing := range existingEntries {
		if existing.Node != req.Node || existing.Transport != req.Transport {
			deDuped = append(deDuped, existing)
		}
	}
	t.inMemoryPlaceholder[req.Node] = append(deDuped, &components.RegistryNodeTransportEntry{
		Node:             req.Node,
		Transport:        req.Transport,
		TransportDetails: req.TransportDetails,
	})

	return &prototk.UpsertTransportDetailsResponse{}, nil
}

func (t *registry) close() {
	t.cancelCtx()
	<-t.initDone
}
