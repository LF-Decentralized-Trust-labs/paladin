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

package transportmgr

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
)

type transportManager struct {
	bgCtx context.Context
	mux   sync.Mutex

	conf *TransportManagerConfig

	transportsByID   map[uuid.UUID]*transport
	transportsByName map[string]*transport

	recvMessages map[string]chan components.TransportMessage
}

func NewTransportManager(bgCtx context.Context, conf *TransportManagerConfig) components.TransportManager {
	return &transportManager{
		bgCtx: bgCtx,
		conf:  conf,
	}
}

func (tm *transportManager) Init(pic components.PreInitComponents) (*components.ManagerInitResult, error) {
	// TransportManager does not rely on any other components during the pre-init phase (at the moment)
	// for QoS we may need persistence in the future, and this will be the plug point for the registry
	// when we have it
	return nil, nil
}

func (tm *transportManager) Start() error { return nil }

func (tm *transportManager) Stop() {
	tm.mux.Lock()
	var allTransports []*transport
	for _, t := range tm.transportsByID {
		allTransports = append(allTransports, t)
	}
	tm.mux.Unlock()
	for _, t := range allTransports {
		tm.cleanupTransport(t)
	}

}

func (tm *transportManager) cleanupTransport(t *transport) {
	// must not hold the transport lock when running this
	t.close()
	delete(tm.transportsByID, t.id)
	delete(tm.transportsByName, t.name)
}

func (tm *transportManager) ConfiguredTransports() map[string]*plugins.PluginConfig {
	pluginConf := make(map[string]*plugins.PluginConfig)
	for name, conf := range tm.conf.Transports {
		pluginConf[name] = &conf.Plugin
	}
	return pluginConf
}

func (tm *transportManager) TransportRegistered(name string, id uuid.UUID, toTransport plugins.TransportManagerToTransport) (fromTransport plugintk.TransportCallbacks, err error) {
	tm.mux.Lock()
	defer tm.mux.Unlock()

	// Replaces any previously registered instance
	existing := tm.transportsByName[name]
	for existing != nil {
		// Can't hold the lock in cleanup, hence the loop
		tm.mux.Unlock()
		tm.cleanupTransport(existing)
		tm.mux.Lock()
		existing = tm.transportsByName[name]
	}

	// Get the config for this transport
	conf := tm.conf.Transports[name]
	if conf == nil {
		// Shouldn't be possible
		return nil, i18n.NewError(tm.bgCtx, msgs.MsgDomainNotFound, name)
	}

	// Initialize
	t := tm.newTransport(id, name, conf, toTransport)
	tm.transportsByID[id] = t
	tm.transportsByName[name] = t
	go t.init()
	return t, nil
}

func (tm *transportManager) GetTransportByName(ctx context.Context, name string) (components.Transport, error) {
	tm.mux.Lock()
	defer tm.mux.Unlock()
	t := tm.transportsByName[name]
	if t == nil {
		return nil, i18n.NewError(ctx, msgs.MsgDomainNotFound, name)
	}
	return t, nil
}

// Internal callback method from the transports to the manager
func (tm *transportManager) recieveExternalMessage(component string, msg components.TransportMessage) {
	if tm.recvMessages[component] == nil {
		tm.recvMessages[component] = make(chan components.TransportMessage)
	}

	tm.recvMessages[component] <- msg
}

// Send implements TransportManager
func (tm *transportManager) Send(ctx context.Context, message components.TransportMessage, identity string, component string) error {
	// TODO: Plug point for calling through to the registry
	// TODO: Plugin determination

	knownPlugin := "grpc"
	transport, err := tm.GetTransportByName(ctx, knownPlugin)

	serializedMessage, err := json.Marshal(message)
	if err != nil {
		return err
	}

	// TODO: Transport Details
	err = transport.Send(ctx, string(serializedMessage), "", component)
	if err != nil {
		return err
	}

	return nil
}

// Send implements TransportManager
func (tm *transportManager) Recieve(component string, onMessage func(ctx context.Context, message components.TransportMessage) error) error {
	if tm.recvMessages[component] == nil {
		tm.recvMessages[component] = make(chan components.TransportMessage)
	}

	go func() {
		for {
			select {
			case <-tm.bgCtx.Done():
				return
			case message := <-tm.recvMessages[component]:
				{
					onMessage(tm.bgCtx, message)
				}
			}
		}
	}()

	return nil
} 