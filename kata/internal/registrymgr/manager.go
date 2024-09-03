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
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type registryManager struct {
	bgCtx context.Context
	mux   sync.Mutex

	conf      *RegistryManagerConfig
	localNode string

	registriesByID   map[uuid.UUID]*registry
	registriesByName map[string]*registry

	recvMessages chan components.RegistryMessage
}

func NewRegistryManager(bgCtx context.Context, conf *RegistryManagerConfig) components.RegistryManager {
	return &registryManager{
		bgCtx:            bgCtx,
		conf:             conf,
		recvMessages:     make(chan components.RegistryMessage, 1),
		registriesByID:   make(map[uuid.UUID]*registry),
		registriesByName: make(map[string]*registry),
	}
}

func (tm *registryManager) Init(pic components.PreInitComponents) (*components.ManagerInitResult, error) {
	// RegistryManager does not rely on any other components during the pre-init phase (at the moment)
	// for QoS we may need persistence in the future, and this will be the plug point for the registry
	// when we have it

	return &components.ManagerInitResult{}, nil
}

func (tm *registryManager) Start() error { return nil }

func (tm *registryManager) Stop() {
	tm.mux.Lock()
	var allRegistries []*registry
	for _, t := range tm.registriesByID {
		allRegistries = append(allRegistries, t)
	}
	tm.mux.Unlock()
	for _, t := range allRegistries {
		tm.cleanupRegistry(t)
	}

}

func (tm *registryManager) cleanupRegistry(t *registry) {
	// must not hold the registry lock when running this
	t.close()
	delete(tm.registriesByID, t.id)
	delete(tm.registriesByName, t.name)
}

func (tm *registryManager) ConfiguredRegistries() map[string]*plugins.PluginConfig {
	pluginConf := make(map[string]*plugins.PluginConfig)
	for name, conf := range tm.conf.Registries {
		pluginConf[name] = &conf.Plugin
	}
	return pluginConf
}

func (tm *registryManager) RegistryRegistered(name string, id uuid.UUID, toRegistry plugins.RegistryManagerToRegistry) (fromRegistry plugintk.RegistryCallbacks, err error) {
	tm.mux.Lock()
	defer tm.mux.Unlock()

	// Replaces any previously registered instance
	existing := tm.registriesByName[name]
	for existing != nil {
		// Can't hold the lock in cleanup, hence the loop
		tm.mux.Unlock()
		tm.cleanupRegistry(existing)
		tm.mux.Lock()
		existing = tm.registriesByName[name]
	}

	// Get the config for this registry
	conf := tm.conf.Registries[name]
	if conf == nil {
		// Shouldn't be possible
		return nil, i18n.NewError(tm.bgCtx, msgs.MsgDomainNotFound, name)
	}

	// Initialize
	t := tm.newRegistry(id, name, conf, toRegistry)
	tm.registriesByID[id] = t
	tm.registriesByName[name] = t
	go t.init()
	return t, nil
}

func (tm *registryManager) getRegistryByName(ctx context.Context, name string) (*registry, error) {
	tm.mux.Lock()
	defer tm.mux.Unlock()
	t := tm.registriesByName[name]
	if t == nil {
		return nil, i18n.NewError(ctx, msgs.MsgDomainNotFound, name)
	}
	return t, nil
}

// Internal callback method from the registries to the manager
func (tm *registryManager) receiveExternalMessage(msg components.RegistryMessage) {
	tm.recvMessages <- msg
}

// Send implements RegistryManager
func (tm *registryManager) Send(ctx context.Context, msgInput *components.RegistryMessageInput) error {
	// TODO: Plug point for calling through to the registry
	// TODO: Plugin determination

	knownPlugin := "grpc"
	registry, err := tm.getRegistryByName(ctx, knownPlugin)
	if err != nil {
		return err
	}

	if len(msgInput.Destination.Node) == 0 ||
		len(msgInput.Destination.Identity) == 0 ||
		len(msgInput.ReplyToIdentity) == 0 ||
		len(msgInput.Payload) == 0 {
		log.L(ctx).Errorf("Invalid message send request %+v", msgInput)
		return i18n.NewError(ctx, msgs.MsgRegistryInvalidMessage)
	}

	err = registry.Send(ctx, message)
	if err != nil {
		return err
	}

	return nil
}

func (tm *registryManager) GetRegistryDetails(ctx context.Context, req *prototk.GetRegistryDetailsRequest) (*prototk.GetRegistryDetailsResponse, error) {
	if req.Node != "test" {
		panic("unimplemented")
	}
	return &prototk.GetRegistryDetailsResponse{RegistryDetails: `
		{
			"address": ":8081",
			"caCertificate": "-----BEGIN CERTIFICATE-----\n` +
		`MIIDuzCCAqOgAwIBAgIUPTw5vaIfHg8yLutcS+IKqHAEWiwwDQYJKoZIhvcNAQEL\n` +
		`BQAwbTELMAkGA1UEBhMCVVMxDjAMBgNVBAgMBVN0YXRlMREwDwYDVQQHDAhMb2Nh\n` +
		`bGl0eTEVMBMGA1UECgwMT3JnYW5pemF0aW9uMRAwDgYDVQQLDAdPcmdVbml0MRIw\n` +
		`EAYDVQQDDAlsb2NhbGhvc3QwHhcNMjQwODA4MTAzNTMwWhcNMzQwODA2MTAzNTMw\n` +
		`WjBtMQswCQYDVQQGEwJVUzEOMAwGA1UECAwFU3RhdGUxETAPBgNVBAcMCExvY2Fs\n` +
		`aXR5MRUwEwYDVQQKDAxPcmdhbml6YXRpb24xEDAOBgNVBAsMB09yZ1VuaXQxEjAQ\n` +
		`BgNVBAMMCWxvY2FsaG9zdDCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEB\n` +
		`AOsKJKuyMysGsmW0X9oYSd3NJgzS6X3o8FqJWuC0vM6tmJMNORLJKcgE7bzKS2J9\n` +
		`pHEG9qU0VADy4cfkj2Jaf0nXiptGZWGF5M1TV3gA6K/ZQt1SwS8Y4LZNo13Ek4pm\n` +
		`znav4HWP8hGjW1Ym70M2Ru9vAvh14pv1VPaDq0eQY7de/Wpt0NPfcrXv5dw+wZQh\n` +
		`OhxczE4QW1hJVF+7uyTzqBVXnUuIpWEYH3WIO/VyQIJERN8ynApnndtglbHXoNhj\n` +
		`xZcZV1gfrOMHXQURhy04KigIvx7lxYqz5MNkFgfFxCHrkkmKH6CTw2ALmHBlXF6X\n` +
		`+qE1jyWYClh014v/yFik82cCAwEAAaNTMFEwHQYDVR0OBBYEFKzheOJklxwLUrx7\n` +
		`qAi/wOKzRd7FMB8GA1UdIwQYMBaAFKzheOJklxwLUrx7qAi/wOKzRd7FMA8GA1Ud\n` +
		`EwEB/wQFMAMBAf8wDQYJKoZIhvcNAQELBQADggEBAAcQOJhQ9NhBjjvFJAfbF9S1\n` +
		`+E1DrP+zjOm8vGWEvVi4NlGVqd4KJVBeHX7IWewMSvBQasdOAFP25VOBqoPFVhNS\n` +
		`XrnBnErCwQyx3NzHQCv50tRDI6e3ms5xh+4bnP7q4fye7QdFJtY7P6CQQMJq46dp\n` +
		`r4aQhKExbB4TgECsYvFLrEpqHI375nghkEKAZD2wmLWCPb7mi1jommXBzxsIyl8u\n` +
		`dlHsczoHgXf2K90p0iqCAluHMB4WgOVZX39ljHN/2o3mQgPQZtDHAL0jCaXKN9io\n` +
		`o4+luzQ1J0UWAGpVThWlEcC5IRrmo5+4+KqyE/wTYJF4dlG/noA8XxkNqM15kY0=\n` +
		`-----END CERTIFICATE-----\n",
		}`}, nil
}

// Send implements RegistryManager
func (tm *registryManager) RegisterReceiver(onMessage func(ctx context.Context, message components.RegistryMessage) error) error {
	go func() {
		for {
			select {
			case <-tm.bgCtx.Done():
				return
			case message := <-tm.recvMessages:
				{
					err := onMessage(tm.bgCtx, message)
					if err != nil {
						log.L(tm.bgCtx).Errorf("error from receiver when processing new message: %v", err)
					}
				}
			}
		}
	}()

	return nil
}
