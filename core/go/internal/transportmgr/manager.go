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
	"sync"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type transportManager struct {
	bgCtx context.Context
	mux   sync.Mutex

	conf            *TransportManagerConfig
	localNodeName   string
	registryManager components.RegistryManager
	engine          components.Engine

	transportsByID   map[uuid.UUID]*transport
	transportsByName map[string]*transport
}

func NewTransportManager(bgCtx context.Context, conf *TransportManagerConfig) components.TransportManager {
	return &transportManager{
		bgCtx:            bgCtx,
		conf:             conf,
		localNodeName:    conf.NodeName,
		transportsByID:   make(map[uuid.UUID]*transport),
		transportsByName: make(map[string]*transport),
	}
}

func (tm *transportManager) PreInit(pic components.PreInitComponents) (*components.ManagerInitResult, error) {
	if tm.localNodeName == "" {
		return nil, i18n.NewError(tm.bgCtx, msgs.MsgTransportNodeNameNotConfigured)
	}
	return &components.ManagerInitResult{}, nil
}

func (tm *transportManager) PostInit(c components.AllComponents) error {
	// Asserted to be thread safe to do initialization here without lock, as it's before the
	// plugin manager starts, and thus before any domain would have started any go-routine
	// that could have cached a nil value in memory.
	tm.registryManager = c.RegistryManager()
	tm.engine = c.Engine()
	return nil
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

func (tm *transportManager) ConfiguredTransports() map[string]*components.PluginConfig {
	pluginConf := make(map[string]*components.PluginConfig)
	for name, conf := range tm.conf.Transports {
		pluginConf[name] = &conf.Plugin
	}
	return pluginConf
}

func (tm *transportManager) TransportRegistered(name string, id uuid.UUID, toTransport components.TransportManagerToTransport) (fromTransport plugintk.TransportCallbacks, err error) {
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
		return nil, i18n.NewError(tm.bgCtx, msgs.MsgTransportNotFound, name)
	}

	// Initialize
	t := tm.newTransport(id, name, conf, toTransport)
	tm.transportsByID[id] = t
	tm.transportsByName[name] = t
	go t.init()
	return t, nil
}

func (tm *transportManager) LocalNodeName() string {
	return tm.localNodeName
}

// See docs in components package
func (tm *transportManager) Send(ctx context.Context, msg *components.TransportMessage) error {

	// Check the message is valid
	if len(msg.MessageType) == 0 ||
		len(msg.Payload) == 0 {
		log.L(ctx).Errorf("Invalid message send request %+v", msg)
		return i18n.NewError(ctx, msgs.MsgTransportInvalidMessage)
	}

	targetNode, err := msg.Destination.Node(ctx, false)
	if err != nil || targetNode == tm.localNodeName {
		return i18n.WrapError(ctx, err, msgs.MsgTransportInvalidDestinationSend, tm.localNodeName, msg.Destination)
	}

	msg.ReplyTo, err = msg.ReplyTo.FullyQualified(ctx, tm.localNodeName)
	if err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgTransportInvalidReplyToReceived, msg.ReplyTo)
	}

	// Note the registry is responsible for caching to make this call as efficient as if
	// we maintained the transport details in-memory ourselves.
	registeredTransportDetails, err := tm.registryManager.GetNodeTransports(ctx, targetNode)
	if err != nil {
		return err
	}

	// See if any of the transports registered by the node, are configured on this local node
	// Note: We just pick the first one if multiple are available, and there is no retry to
	//       fallback to a secondary one currently.
	var transport *transport
	for _, rtd := range registeredTransportDetails {
		transport = tm.transportsByName[rtd.Transport]
	}
	if transport == nil {
		// If we didn't find one, then feedback to the caller which transports were registered
		registeredTransportNames := []string{}
		for _, rtd := range registeredTransportDetails {
			registeredTransportNames = append(registeredTransportNames, rtd.Transport)
		}
		return i18n.NewError(ctx, msgs.MsgTransportNoTransportsConfiguredForNode, targetNode, registeredTransportNames)
	}

	// Call the selected transport to send
	// Note: We do not push the transport details down to the plugin on every send, as they are very large
	//       (KBs of certificates and other data).
	//       The transport plugin uses GetTransportDetails to request them back from us, and then caches
	//       these internally through use of a long lived connection / connection-pool.
	var correlID *string
	if msg.CorrelationID != nil {
		correlID = confutil.P(msg.CorrelationID.String())
	}
	err = transport.send(ctx, &prototk.Message{
		MessageId:     uuid.New().String(),
		CorrelationId: correlID,
		Destination:   msg.Destination.String(),
		ReplyTo:       msg.ReplyTo.String(),
		Payload:       msg.Payload,
	})
	if err != nil {
		return err
	}

	return nil
}
