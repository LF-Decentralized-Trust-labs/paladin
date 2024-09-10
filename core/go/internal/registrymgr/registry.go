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
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
)

type registry struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	conf *RegistryConfig
	rm   *registryManager
	id   uuid.UUID
	name string
	api  components.RegistryManagerToRegistry

	// TODO: Replace with a cache-backed DB system
	stateLock           sync.Mutex
	inMemoryPlaceholder map[string][]*components.RegistryNodeTransportEntry

	initialized atomic.Bool
	initRetry   *retry.Retry

	initError atomic.Pointer[error]
	initDone  chan struct{}
}

func (rm *registryManager) newRegistry(id uuid.UUID, name string, conf *RegistryConfig, toRegistry components.RegistryManagerToRegistry) *registry {
	r := &registry{
		rm:                  rm,
		conf:                conf,
		initRetry:           retry.NewRetryIndefinite(&conf.Init.Retry),
		name:                name,
		id:                  id,
		api:                 toRegistry,
		inMemoryPlaceholder: make(map[string][]*components.RegistryNodeTransportEntry),
		initDone:            make(chan struct{}),
	}
	r.ctx, r.cancelCtx = context.WithCancel(log.WithLogField(rm.bgCtx, "registry", r.name))
	return r
}

func (r *registry) init() {
	defer close(r.initDone)

	// We block retrying each part of init until we succeed, or are cancelled
	// (which the plugin manager will do if the registry disconnects)
	err := r.initRetry.Do(r.ctx, func(attempt int) (bool, error) {
		// Send the configuration to the registry for processing
		confJSON, _ := json.Marshal(&r.conf.Config)
		_, err := r.api.ConfigureRegistry(r.ctx, &prototk.ConfigureRegistryRequest{
			Name:       r.name,
			ConfigJson: string(confJSON),
		})
		return true, err
	})
	if err != nil {
		log.L(r.ctx).Debugf("registry initialization cancelled before completion: %s", err)
		r.initError.Store(&err)
	} else {
		log.L(r.ctx).Debugf("registry initialization complete")
		r.initialized.Store(true)
		// Inform the plugin manager callback
		r.api.Initialized()
	}
}

func (r *registry) getNodeTransports(node string) []*components.RegistryNodeTransportEntry {
	r.stateLock.Lock()
	defer r.stateLock.Unlock()
	return r.inMemoryPlaceholder[node]
}

// Registry callback to the registry manager when new entries are available to upsert & cache
// (can be called during and after initialization asynchronously as pre-configured and updated information becomes known)
func (r *registry) UpsertTransportDetails(ctx context.Context, req *prototk.UpsertTransportDetails) (*prototk.UpsertTransportDetailsResponse, error) {

	r.stateLock.Lock()
	defer r.stateLock.Unlock()

	if req.Node == "" || req.Transport == "" {
		return nil, i18n.NewError(ctx, msgs.MsgRegistryInvalidEntry)
	}

	existingEntries := r.inMemoryPlaceholder[req.Node]
	deDuped := make([]*components.RegistryNodeTransportEntry, 0, len(existingEntries))
	for _, existing := range existingEntries {
		if existing.Node != req.Node || existing.Transport != req.Transport {
			deDuped = append(deDuped, existing)
		}
	}
	r.inMemoryPlaceholder[req.Node] = append(deDuped, &components.RegistryNodeTransportEntry{
		Node:             req.Node,
		Transport:        req.Transport,
		TransportDetails: req.TransportDetails,
	})

	return &prototk.UpsertTransportDetailsResponse{}, nil
}

func (r *registry) close() {
	r.cancelCtx()
	<-r.initDone
}
