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

	"github.com/google/uuid"
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
	Init() error
	Start() error
	Stop()
	Engine() components.Engine
}

type componentManager struct {
	instanceUUID uuid.UUID
	bgCtx        context.Context
	// config
	conf *Config
	// pre-init
	keyManager       ethclient.KeyManager
	ethClientFactory ethclient.EthClientFactory
	persistence      persistence.Persistence
	stateStore       statestore.StateStore
	blockIndexer     blockindexer.BlockIndexer
	rpcServer        rpcserver.RPCServer
	// post-init
	pluginController plugins.PluginController
	// managers
	domainManager components.DomainManager
	// engine
	engine components.Engine
	// init to start tracking
	initResults          map[string]*components.ManagerInitResult
	internalEventStreams []*blockindexer.InternalEventStream
	// keep track of everything we started
	started []stoppable
	opened  []closeable
}

// things that have a running component that is active in the background and hence "stops"
type stoppable interface {
	Stop()
}

// things that are services used in various places, but need to cleanly disconnect all connections and hence "close"
type closeable interface {
	Close()
}

func NewComponentManager(bgCtx context.Context, instanceUUID uuid.UUID, conf *Config, engine components.Engine) ComponentManager {
	return &componentManager{
		instanceUUID: instanceUUID,
		bgCtx:        bgCtx,
		conf:         conf,
		engine:       engine,
		initResults:  make(map[string]*components.ManagerInitResult),
	}
}

func (cm *componentManager) Init() (err error) {

	// pre-init components
	cm.keyManager, err = ethclient.NewSimpleTestKeyManager(cm.bgCtx, &cm.conf.Signer)
	cm.addIfOpened(cm.keyManager, err)
	if err == nil {
		cm.ethClientFactory, err = ethclient.NewEthClientFactory(cm.bgCtx, cm.keyManager, &cm.conf.Blockchain)
	}
	if err == nil {
		cm.persistence, err = persistence.NewPersistence(cm.bgCtx, &cm.conf.DB)
		cm.addIfOpened(cm.persistence, err)
	}
	if err == nil {
		cm.stateStore = statestore.NewStateStore(cm.bgCtx, &cm.conf.StateStore, cm.persistence)
		cm.addIfOpened(cm.stateStore, err)
	}
	if err == nil {
		cm.blockIndexer, err = blockindexer.NewBlockIndexer(cm.bgCtx, &cm.conf.BlockIndexer, &cm.conf.Blockchain.WS, cm.persistence)
	}
	if err == nil {
		cm.rpcServer, err = rpcserver.NewRPCServer(cm.bgCtx, &cm.conf.RPCServer)
	}

	// init managers
	if err == nil {
		cm.domainManager = domainmgr.NewDomainManager(cm.bgCtx, &cm.conf.DomainManagerConfig)
		cm.initResults["domain_mgr"], err = cm.domainManager.Init(cm)
	}

	// using init of managers, for post-init components
	if err == nil {
		cm.pluginController, err = plugins.NewPluginController(cm.bgCtx, cm.instanceUUID, cm, &cm.conf.PluginControllerConfig)
	}

	// init engine
	if err == nil {
		cm.initResults[cm.engine.EngineName()], err = cm.engine.Init(cm)
	}
	return err
}

func (cm *componentManager) Start() (err error) {

	// start the eth client
	err = cm.ethClientFactory.Start()
	cm.addIfStarted(cm.ethClientFactory, err)

	// start the block indexer
	if err == nil {
		cm.internalEventStreams, err = cm.buildInternalEventStreams()
	}
	if err == nil {
		err = cm.blockIndexer.Start(cm.internalEventStreams...)
		cm.addIfStarted(cm.blockIndexer, err)
	}
	if err == nil {
		// we wait until the block indexer has connected and established the block height
		// this is for the edge case that on first start, when using "latest" for listeners,
		// we can't possibly submit any transactions before the block height is known
		_, err = cm.blockIndexer.GetBlockListenerHeight(cm.bgCtx)
	}

	// start the managers
	if err == nil {
		err = cm.domainManager.Start()
		cm.addIfStarted(cm.domainManager, err)
	}

	// start the plugin controller
	if err == nil {
		err = cm.pluginController.Start()
		cm.addIfStarted(cm.pluginController, err)
	}

	// Wait for the plugins to all start
	if err == nil {
		err = cm.pluginController.WaitForInit(cm.bgCtx)
	}

	// start the engine
	if err == nil {
		err = cm.engine.Start()
		cm.addIfStarted(cm.engine, err)
	}

	// start the RPC server last
	if err == nil {
		cm.registerRPCModules()
		err = cm.rpcServer.Start()
		cm.addIfStarted(cm.rpcServer, err)
	}

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

func (cm *componentManager) buildInternalEventStreams() ([]*blockindexer.InternalEventStream, error) {
	var streams []*blockindexer.InternalEventStream
	for shortName, initResult := range cm.initResults {
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

func (cm *componentManager) registerRPCModules() {
	// Component modules
	cm.rpcServer.Register(cm.stateStore.RPCModule())
	// Manager/engine modules
	for _, initResult := range cm.initResults {
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

func (cm *componentManager) RPCServer() rpcserver.RPCServer {
	return cm.rpcServer
}

func (cm *componentManager) BlockIndexer() blockindexer.BlockIndexer {
	return cm.blockIndexer
}

func (cm *componentManager) DomainManager() components.DomainManager {
	return cm.domainManager
}

func (cm *componentManager) DomainRegistration() plugins.DomainRegistration {
	return cm.domainManager
}

func (cm *componentManager) PluginController() plugins.PluginController {
	return cm.pluginController
}

func (cm *componentManager) Engine() components.Engine {
	return cm.engine
}
