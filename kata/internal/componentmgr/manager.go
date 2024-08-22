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

package componentmgr

import (
	"context"
	"fmt"

	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/domainmgr"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/pkg/blockindexer"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/kata/pkg/persistence"
	"github.com/kaleido-io/paladin/kata/pkg/types"
)

type ComponentManager interface {
	components.AllComponents
	Start() error
	Stop()
	Engine() components.Engine
}

type componentManager struct {
	bgCtx context.Context
	// config
	conf *Config
	// pre-init
	keyManager       ethclient.KeyManager
	ethClientFactory ethclient.EthClientFactory
	persistence      persistence.Persistence
	stateStore       statestore.StateStore
	// post-init
	blockIndexer     blockindexer.BlockIndexer
	pluginController plugins.PluginController
	rpcServer        rpcserver.Server
	// managers
	domainManager components.DomainManager
	// engine
	engine components.Engine
	// keep track of everything we started
	started []stoppable
	opened  []closeable
}

type stoppable interface {
	Stop()
}

type closeable interface {
	Close()
}

func NewComponentManager(bgCtx context.Context, conf *Config, engine components.Engine) ComponentManager {
	return newComponentManager(bgCtx, conf, engine)
}

// For unit tests we create a component manager from an existing persistence layer
func NewComponentManagerWithPersistence(bgCtx context.Context, conf *Config, persistence persistence.Persistence, engine components.Engine) ComponentManager {
	cm := newComponentManager(bgCtx, conf, engine)
	cm.persistence = persistence
	return cm
}

func newComponentManager(bgCtx context.Context, conf *Config, engine components.Engine) *componentManager {
	return &componentManager{
		bgCtx:  bgCtx,
		conf:   conf,
		engine: engine,
	}
}

func (cm *componentManager) Start() (err error) {

	// pre-init components
	cm.keyManager, err = ethclient.NewSimpleTestKeyManager(cm.bgCtx, &cm.conf.Signer)
	if err == nil {
		cm.ethClientFactory, err = ethclient.NewEthClientFactory(cm.bgCtx, cm.keyManager, &cm.conf.Blockchain)
		cm.addIfOpened(cm.ethClientFactory, err)
	}
	if err == nil && cm.persistence == nil {
		cm.persistence, err = persistence.NewPersistence(cm.bgCtx, &cm.conf.DB)
		cm.addIfOpened(cm.persistence, err)
	}
	if err == nil {
		cm.stateStore = statestore.NewStateStore(cm.bgCtx, &cm.conf.StateStore, cm.persistence)
		cm.addIfOpened(cm.stateStore, err)
	}

	// pre-init managers
	initResults := map[string]*components.ManagerInitResult{}
	if err == nil {
		cm.domainManager = domainmgr.NewDomainManager(cm.bgCtx, &cm.conf.DomainManagerConfig)
		initResults["domain_mgr"], err = cm.domainManager.PreInit(cm)
	}

	// pre-init engine
	if err == nil {
		initResults[cm.engine.Name()], err = cm.engine.PreInit(cm)
	}

	// using pre-init of managers, for init of post-init components
	if err == nil {
		cm.blockIndexer, err = blockindexer.NewBlockIndexer(cm.bgCtx, &cm.conf.BlockIndexer, &cm.conf.Blockchain.WS, cm.persistence)
	}
	var internalEventStreams []*blockindexer.InternalEventStream
	if err == nil {
		internalEventStreams, err = cm.buildInternalEventStreams(initResults)
	}
	if err == nil {
		err = cm.blockIndexer.Start(internalEventStreams...)
		cm.addIfStarted(cm.blockIndexer, err)
	}
	if err == nil {
		// note the RPC server is the last thing to actually start, only after we're fully initialized
		cm.rpcServer, err = rpcserver.NewServer(cm.bgCtx, &cm.conf.RPCServer)
		cm.registerRPCModules(initResults)
	}

	// post-init managers
	if err == nil {
		err = cm.domainManager.PostInit(cm)
	}

	// post-init the engine
	if err == nil {
		err = cm.engine.PostInit(cm)
	}

	// start the managers
	if err == nil {
		err = cm.domainManager.Start()
		cm.addIfStarted(cm.domainManager, err)
	}

	// start the engine
	if err == nil {
		err = cm.engine.Start()
		cm.addIfStarted(cm.engine, err)
	}

	// start the RPC server
	if err == nil {
		err = cm.rpcServer.Start()
		cm.addIfStarted(cm.rpcServer, err)
	}

	// ... and we're done
	return err
}

func (cm *componentManager) addIfStarted(c stoppable, err error) {
	if err == nil {
		cm.started = append(cm.started, c)
	}
}

func (cm *componentManager) addIfOpened(c closeable, err error) {
	if err == nil {
		cm.opened = append(cm.opened, c)
	}
}

func (cm *componentManager) buildInternalEventStreams(initResults map[string]*components.ManagerInitResult) ([]*blockindexer.InternalEventStream, error) {
	var streams []*blockindexer.InternalEventStream
	for shortName, initResult := range initResults {
		for _, initStream := range initResult.EventStreams {
			// We build a stream name in a way assured to result in a new stream if the ABI changes,
			// TODO... and in the future with a logical way to clean up defunct streams
			streamHash, err := types.ABISolDefinitionHash(cm.bgCtx, initStream.ABI)
			if err != nil {
				return nil, err
			}
			streamName := fmt.Sprintf(".internal/%s/%s", shortName, streamHash)
			streams = append(streams, &blockindexer.InternalEventStream{
				Definition: &blockindexer.EventStream{
					Name: streamName,
					Type: blockindexer.EventStreamTypeInternal.Enum(),
					ABI:  initStream.ABI,
				},
				Handler: initStream.Handler,
			})
		}

	}
	return streams, nil
}

func (cm *componentManager) registerRPCModules(initResults map[string]*components.ManagerInitResult) {
	for _, initResult := range initResults {
		for _, rpcMod := range initResult.RPCModules {
			cm.rpcServer.Register(rpcMod)
		}
	}
}

func (cm *componentManager) Stop() {
	// stop all the stoppable things we started
	for _, c := range cm.started {
		c.Stop()
	}
	// close all the closable things we opened
	for _, c := range cm.opened {
		c.Close()
	}
}

func (cm *componentManager) KeyManager() ethclient.KeyManager {
	return cm.keyManager
}

func (cm *componentManager) EthClientFactory() ethclient.EthClientFactory {
	return cm.ethClientFactory
}

func (cm *componentManager) Persistence() persistence.Persistence {
	return cm.persistence
}

func (cm *componentManager) StateStore() statestore.StateStore {
	return cm.stateStore
}

func (cm *componentManager) RPCServer() rpcserver.Server {
	return cm.rpcServer
}

func (cm *componentManager) BlockIndexer() blockindexer.BlockIndexer {
	return cm.blockIndexer
}

func (cm *componentManager) DomainManager() components.DomainManager {
	return cm.domainManager
}

func (cm *componentManager) Engine() components.Engine {
	return cm.engine
}

func (cm *componentManager) PluginController() plugins.PluginController {
	return cm.pluginController
}
