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
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"gopkg.in/yaml.v3"

	grpctransport "github.com/kaleido-io/paladin/kata/internal/plugins/grpctransport/plugin"
)

type transportManager struct {
	bgCtx context.Context
	mux   sync.Mutex

	conf *TransportManagerConfig

	transportsByID   map[uuid.UUID]*transport
	transportsByName map[string]*transport

	recvMessages chan components.TransportMessage
}

func NewTransportManager(bgCtx context.Context, conf *TransportManagerConfig) components.TransportManager {
	return &transportManager{
		bgCtx:            bgCtx,
		conf:             conf,
		recvMessages:     make(chan components.TransportMessage, 1),
		transportsByID:   make(map[uuid.UUID]*transport),
		transportsByName: make(map[string]*transport),
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
func (tm *transportManager) recieveExternalMessage(msg components.TransportMessage) {
	tm.recvMessages <- msg
}

// Send implements TransportManager
func (tm *transportManager) Send(ctx context.Context, message components.TransportMessage, identity string) error {
	// TODO: Plug point for calling through to the registry
	// TODO: Plugin determination

	transportDetails := ""
	if identity == "test" {
		deets := grpctransport.TransportDetails{
			Address: ":8081",
			CaCertificate: `-----BEGIN CERTIFICATE-----
			MIIDuzCCAqOgAwIBAgIUPTw5vaIfHg8yLutcS+IKqHAEWiwwDQYJKoZIhvcNAQEL
			BQAwbTELMAkGA1UEBhMCVVMxDjAMBgNVBAgMBVN0YXRlMREwDwYDVQQHDAhMb2Nh
			bGl0eTEVMBMGA1UECgwMT3JnYW5pemF0aW9uMRAwDgYDVQQLDAdPcmdVbml0MRIw
			EAYDVQQDDAlsb2NhbGhvc3QwHhcNMjQwODA4MTAzNTMwWhcNMzQwODA2MTAzNTMw
			WjBtMQswCQYDVQQGEwJVUzEOMAwGA1UECAwFU3RhdGUxETAPBgNVBAcMCExvY2Fs
			aXR5MRUwEwYDVQQKDAxPcmdhbml6YXRpb24xEDAOBgNVBAsMB09yZ1VuaXQxEjAQ
			BgNVBAMMCWxvY2FsaG9zdDCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEB
			AOsKJKuyMysGsmW0X9oYSd3NJgzS6X3o8FqJWuC0vM6tmJMNORLJKcgE7bzKS2J9
			pHEG9qU0VADy4cfkj2Jaf0nXiptGZWGF5M1TV3gA6K/ZQt1SwS8Y4LZNo13Ek4pm
			znav4HWP8hGjW1Ym70M2Ru9vAvh14pv1VPaDq0eQY7de/Wpt0NPfcrXv5dw+wZQh
			OhxczE4QW1hJVF+7uyTzqBVXnUuIpWEYH3WIO/VyQIJERN8ynApnndtglbHXoNhj
			xZcZV1gfrOMHXQURhy04KigIvx7lxYqz5MNkFgfFxCHrkkmKH6CTw2ALmHBlXF6X
			+qE1jyWYClh014v/yFik82cCAwEAAaNTMFEwHQYDVR0OBBYEFKzheOJklxwLUrx7
			qAi/wOKzRd7FMB8GA1UdIwQYMBaAFKzheOJklxwLUrx7qAi/wOKzRd7FMA8GA1Ud
			EwEB/wQFMAMBAf8wDQYJKoZIhvcNAQELBQADggEBAAcQOJhQ9NhBjjvFJAfbF9S1
			+E1DrP+zjOm8vGWEvVi4NlGVqd4KJVBeHX7IWewMSvBQasdOAFP25VOBqoPFVhNS
			XrnBnErCwQyx3NzHQCv50tRDI6e3ms5xh+4bnP7q4fye7QdFJtY7P6CQQMJq46dp
			r4aQhKExbB4TgECsYvFLrEpqHI375nghkEKAZD2wmLWCPb7mi1jommXBzxsIyl8u
			dlHsczoHgXf2K90p0iqCAluHMB4WgOVZX39ljHN/2o3mQgPQZtDHAL0jCaXKN9io
			o4+luzQ1J0UWAGpVThWlEcC5IRrmo5+4+KqyE/wTYJF4dlG/noA8XxkNqM15kY0=
			-----END CERTIFICATE-----
			`,
		}

		bytes, _ := yaml.Marshal(deets)
		transportDetails = string(bytes)
	}

	knownPlugin := "grpc"
	transport, err := tm.GetTransportByName(ctx, knownPlugin)
	if err != nil {
		return err
	}

	serializedMessage, err := json.Marshal(message)
	if err != nil {
		return err
	}

	// TODO: Transport Details
	err = transport.Send(ctx, string(serializedMessage), transportDetails)
	if err != nil {
		return err
	}

	return nil
}

// Send implements TransportManager
func (tm *transportManager) RegisterReceiver(onMessage func(ctx context.Context, message components.TransportMessage) error) error {
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
