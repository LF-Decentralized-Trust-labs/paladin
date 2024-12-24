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
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"gorm.io/gorm"

	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type transportManager struct {
	bgCtx context.Context
	mux   sync.Mutex

	rpcModule       *rpcserver.RPCModule
	conf            *pldconf.TransportManagerConfig
	localNodeName   string
	registryManager components.RegistryManager

	transportsByID   map[uuid.UUID]*transport
	transportsByName map[string]*transport

	destinations      map[string]components.TransportClient
	destinationsFixed bool
	destinationsMux   sync.RWMutex

	peersLock sync.RWMutex
	peers     map[string]*peer

	senderBufferLen int
}

func NewTransportManager(bgCtx context.Context, conf *pldconf.TransportManagerConfig) components.TransportManager {
	return &transportManager{
		bgCtx:            bgCtx,
		conf:             conf,
		localNodeName:    conf.NodeName,
		transportsByID:   make(map[uuid.UUID]*transport),
		transportsByName: make(map[string]*transport),
		destinations:     make(map[string]components.TransportClient),
		senderBufferLen:  confutil.IntMin(conf.SendQueueLen, 0, *pldconf.TransportManagerDefaults.SendQueueLen),
	}
}

func (tm *transportManager) PreInit(pic components.PreInitComponents) (*components.ManagerInitResult, error) {
	if tm.localNodeName == "" {
		return nil, i18n.NewError(tm.bgCtx, msgs.MsgTransportNodeNameNotConfigured)
	}
	tm.initRPC()
	return &components.ManagerInitResult{
		RPCModules: []*rpcserver.RPCModule{tm.rpcModule},
	}, nil
}

func (tm *transportManager) PostInit(c components.AllComponents) error {
	// Asserted to be thread safe to do initialization here without lock, as it's before the
	// plugin manager starts, and thus before any domain would have started any go-routine
	// that could have cached a nil value in memory.
	tm.registryManager = c.RegistryManager()
	return nil
}

func (tm *transportManager) Start() error {
	tm.destinationsMux.Lock()
	defer tm.destinationsMux.Unlock()
	// All destinations must be registered as part of the startup sequence
	tm.destinationsFixed = true
	return nil
}

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

func (tm *transportManager) RegisterClient(ctx context.Context, client components.TransportClient) error {
	tm.destinationsMux.Lock()
	defer tm.destinationsMux.Unlock()
	if tm.destinationsFixed {
		return i18n.NewError(tm.bgCtx, msgs.MsgTransportClientRegisterAfterStartup, client.Destination())
	}
	if _, found := tm.destinations[client.Destination()]; found {
		log.L(ctx).Errorf("Client already registered for destination %s", client.Destination())
		return i18n.NewError(tm.bgCtx, msgs.MsgTransportClientAlreadyRegistered, client.Destination())
	}
	tm.destinations[client.Destination()] = client
	return nil

}

func (tm *transportManager) cleanupTransport(t *transport) {
	// must not hold the transport lock when running this
	t.close()
	delete(tm.transportsByID, t.id)
	delete(tm.transportsByName, t.name)
}

func (tm *transportManager) ConfiguredTransports() map[string]*pldconf.PluginConfig {
	pluginConf := make(map[string]*pldconf.PluginConfig)
	for name, conf := range tm.conf.Transports {
		pluginConf[name] = &conf.Plugin
	}
	return pluginConf
}

func (tm *transportManager) getTransportNames() []string {
	tm.mux.Lock()
	defer tm.mux.Unlock()

	transportNames := make([]string, 0, len(tm.transportsByName))
	for transportName := range tm.transportsByName {
		transportNames = append(transportNames, transportName)
	}
	return transportNames
}

func (tm *transportManager) getTransportByName(ctx context.Context, transportName string) (*transport, error) {
	tm.mux.Lock()
	defer tm.mux.Unlock()

	t := tm.transportsByName[transportName]
	if t == nil {
		return nil, i18n.NewError(ctx, msgs.MsgTransportNotFound, transportName)
	}
	return t, nil
}

func (tm *transportManager) getLocalTransportDetails(ctx context.Context, transportName string) (string, error) {
	t, err := tm.getTransportByName(ctx, transportName)
	if err != nil {
		return "", err
	}
	return t.getLocalDetails(ctx)
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

func (tm *transportManager) prepareNewMessage(ctx context.Context, msg *components.TransportMessage) (*peer, error) {
	msg.Created = tktypes.TimestampNow()
	msg.MessageID = uuid.New()

	// Check the message is valid
	if len(msg.MessageType) == 0 ||
		len(msg.Payload) == 0 {
		log.L(ctx).Errorf("Invalid message send request %+v", msg)
		return nil, i18n.NewError(ctx, msgs.MsgTransportInvalidMessage)
	}

	if msg.ReplyTo == "" {
		msg.ReplyTo = tm.localNodeName
	}

	// Use or establish a peer connection for the send
	peer, err := tm.getPeer(ctx, msg.Node)
	if err != nil {
		return nil, err
	}

	return peer, nil
}

// See docs in components package
func (tm *transportManager) Send(ctx context.Context, msg *components.TransportMessage) error {

	peer, err := tm.prepareNewMessage(ctx, msg)
	if err != nil {
		return err
	}

	// Push the send to the peer - this is a best effort interaction.
	// There is some retry in the Paladin layer, and some transports provide resilience.
	// However, the send is at-most-once, and the higher level message protocols that
	// use this "send" must be fault tolerant to message loss.
	return peer.send(ctx, msg)

}

// See docs in components package
func (tm *transportManager) SendReliable(ctx context.Context, dbTX *gorm.DB, msg *components.TransportMessage) (preCommit func(), err error) {

	peer, err := tm.prepareNewMessage(ctx, msg)
	if err != nil {
		return nil, err
	}

	err = dbTX.
		WithContext(ctx).
		Create(msg).
		Error
	if err != nil {
		return nil, err
	}

	return peer.notifyPersistedMsgAvailable, nil

}
