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
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/domainmgr"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/kaleido-io/paladin/kata/internal/registrymgr"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/internal/transportmgr"
	"github.com/kaleido-io/paladin/kata/pkg/blockindexer"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/kata/pkg/persistence"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

type ComponentManager interface {
	components.PreInitComponentsAndManagers
	Init() error
	StartComponents() error
	StartManagers() error
	CompleteStart() error
	Stop()
}

type componentManager struct {
	grpcTarget   string
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
	// managers
	domainManager    components.DomainManager
	transportManager components.TransportManager
	registryManager  components.RegistryManager
	pluginManager    components.PluginManager
	// engine
	engine components.Engine
	// init to start tracking
	initResults          map[string]*components.ManagerInitResult
	internalEventStreams []*blockindexer.InternalEventStream
	// keep track of everything we started
	started map[string]stoppable
	opened  map[string]closeable
}

// things that have a running component that is active in the background and hence "stops"
type stoppable interface {
	Stop()
}

// things that are services used in various places, but need to cleanly disconnect all connections and hence "close"
type closeable interface {
	Close()
}

func NewComponentManager(bgCtx context.Context, grpcTarget string, instanceUUID uuid.UUID, conf *Config, engine components.Engine) ComponentManager {
	log.InitConfig(&conf.Log)
	return &componentManager{
		grpcTarget:   grpcTarget, // default is a UDS path, can use tcp:127.0.0.1:12345 strings too (or tcp4:/tcp6:)
		instanceUUID: instanceUUID,
		bgCtx:        bgCtx,
		conf:         conf,
		engine:       engine,
		initResults:  make(map[string]*components.ManagerInitResult),
		started:      make(map[string]stoppable),
		opened:       make(map[string]closeable),
	}
}

func (cm *componentManager) Init() (err error) {
	// pre-init components
	cm.keyManager, err = ethclient.NewSimpleTestKeyManager(cm.bgCtx, &cm.conf.Signer)
	err = cm.addIfOpened("key_manager", cm.keyManager, err, msgs.MsgComponentKeyManagerInitError)
	if err == nil {
		cm.ethClientFactory, err = ethclient.NewEthClientFactory(cm.bgCtx, cm.keyManager, &cm.conf.Blockchain)
		err = cm.wrapIfErr(err, msgs.MsgComponentEthClientInitError)
	}
	if err == nil {
		cm.persistence, err = persistence.NewPersistence(cm.bgCtx, &cm.conf.DB)
		err = cm.addIfOpened("database", cm.persistence, err, msgs.MsgComponentDBInitError)
	}
	if err == nil {
		cm.stateStore = statestore.NewStateStore(cm.bgCtx, &cm.conf.StateStore, cm.persistence)
		err = cm.addIfOpened("state_store", cm.stateStore, err, msgs.MsgComponentStateStoreInitError)
	}
	if err == nil {
		cm.blockIndexer, err = blockindexer.NewBlockIndexer(cm.bgCtx, &cm.conf.BlockIndexer, &cm.conf.Blockchain.WS, cm.persistence)
		err = cm.wrapIfErr(err, msgs.MsgComponentBlockIndexerInitError)
	}
	if err == nil {
		cm.rpcServer, err = rpcserver.NewRPCServer(cm.bgCtx, &cm.conf.RPCServer)
		err = cm.wrapIfErr(err, msgs.MsgComponentRPCServerInitError)
	}

	// pre-init managers
	if err == nil {
		cm.domainManager = domainmgr.NewDomainManager(cm.bgCtx, &cm.conf.DomainManagerConfig)
		cm.initResults["domain_mgr"], err = cm.domainManager.PreInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentDomainInitError)
	}

	if err == nil {
		cm.transportManager = transportmgr.NewTransportManager(cm.bgCtx, &cm.conf.TransportManagerConfig)
		cm.initResults["transports_mgr"], err = cm.transportManager.PreInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentTransportInitError)
	}

	if err == nil {
		cm.registryManager = registrymgr.NewRegistryManager(cm.bgCtx, &cm.conf.RegistryManagerConfig)
		cm.initResults["registry_mgr"], err = cm.registryManager.PreInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentRegistryInitError)
	}

	if err == nil {
		cm.pluginManager = plugins.NewPluginManager(cm.bgCtx, cm.grpcTarget, cm.instanceUUID, &cm.conf.PluginManagerConfig)
		cm.initResults["plugin_mgr"], err = cm.pluginManager.PreInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentPluginInitError)
	}

	// init engine
	if err == nil {
		cm.initResults[cm.engine.EngineName()], err = cm.engine.Init(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentEngineInitError)
	}

	// post-init the managers
	if err == nil {
		err = cm.domainManager.PostInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentDomainInitError)
	}

	if err == nil {
		err = cm.transportManager.PostInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentTransportInitError)
	}

	if err == nil {
		err = cm.transportManager.PostInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentRegistryInitError)
	}

	if err == nil {
		err = cm.pluginManager.PostInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentPluginInitError)
	}

	return err
}

func (cm *componentManager) StartComponents() (err error) {

	// start the eth client
	err = cm.ethClientFactory.Start()
	err = cm.addIfStarted("eth_client", cm.ethClientFactory, err, msgs.MsgComponentEthClientStartError)

	// start the block indexer
	if err == nil {
		cm.internalEventStreams, err = cm.buildInternalEventStreams()
	}
	if err == nil {
		err = cm.blockIndexer.Start(cm.internalEventStreams...)
		err = cm.addIfStarted("block_indexer", cm.blockIndexer, err, msgs.MsgComponentBlockIndexerStartError)
	}
	if err == nil {
		// we wait until the block indexer has connected and established the block height
		// this is for the edge case that on first start, when using "latest" for listeners,
		// we can't possibly submit any transactions before the block height is known
		_, err = cm.blockIndexer.GetBlockListenerHeight(cm.bgCtx)
		err = cm.wrapIfErr(err, msgs.MsgComponentBlockIndexerStartError)
	}
	return err
}

func (cm *componentManager) StartManagers() (err error) {

	// start the managers
	err = cm.domainManager.Start()
	err = cm.addIfStarted("domain_manager", cm.domainManager, err, msgs.MsgComponentDomainStartError)

	if err == nil {
		err = cm.transportManager.Start()
		err = cm.addIfStarted("transport_manager", cm.transportManager, err, msgs.MsgComponentTransportStartError)
	}

	if err == nil {
		err = cm.registryManager.Start()
		err = cm.addIfStarted("registry_manager", cm.registryManager, err, msgs.MsgComponentRegistryStartError)
	}

	if err == nil {
		err = cm.pluginManager.Start()
		err = cm.addIfStarted("plugin_manager", cm.pluginManager, err, msgs.MsgComponentPluginStartError)
	}
	return err
}

func (cm *componentManager) CompleteStart() error {
	// Wait for the plugins to all start
	err := cm.pluginManager.WaitForInit(cm.bgCtx)
	err = cm.wrapIfErr(err, msgs.MsgComponentWaitPluginStartError)

	// start the engine
	if err == nil {
		err = cm.engine.Start()
		err = cm.addIfStarted(cm.engine.EngineName(), cm.engine, err, msgs.MsgComponentEngineStartError)
	}

	// start the RPC server last
	if err == nil {
		cm.registerRPCModules()
		err = cm.rpcServer.Start()
		err = cm.addIfStarted("rpc_server", cm.rpcServer, err, msgs.MsgComponentRPCServerStartError)
	}
	if err == nil {
		httpEndpoint := "disabled"
		if cm.rpcServer.HTTPAddr() != nil {
			httpEndpoint = cm.rpcServer.HTTPAddr().String()
		}
		wsEndpoint := "disabled"
		if cm.rpcServer.WSAddr() != nil {
			httpEndpoint = cm.rpcServer.WSAddr().String()
		}
		log.L(cm.bgCtx).Infof("Startup complete. RPC endpoints http=%s ws=%s", httpEndpoint, wsEndpoint)
	}

	return err
}

func (cm *componentManager) wrapIfErr(err error, failMsg i18n.ErrorMessageKey) error {
	if err != nil {
		return i18n.WrapError(cm.bgCtx, err, failMsg)
	}
	return nil
}

func (cm *componentManager) addIfStarted(desc string, c stoppable, err error, failMsg i18n.ErrorMessageKey) error {
	if err != nil {
		return i18n.WrapError(cm.bgCtx, err, failMsg)
	}
	cm.started[desc] = c
	return nil
}

func (cm *componentManager) addIfOpened(desc string, c closeable, err error, failMsg i18n.ErrorMessageKey) error {
	if err != nil {
		return i18n.WrapError(cm.bgCtx, err, failMsg)
	}
	cm.opened[desc] = c
	return nil
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
			streamName := fmt.Sprintf("i_%s_%s", shortName, streamHash)
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
	log.L(cm.bgCtx).Info("Stopping")
	// stop all the stoppable things we started
	for name, c := range cm.started {
		log.L(cm.bgCtx).Infof("Stopping %s", name)
		c.Stop()
		log.L(cm.bgCtx).Debugf("Stopped %s", name)
	}
	// close all the closable things we opened
	for name, c := range cm.opened {
		log.L(cm.bgCtx).Infof("Stopping %s", name)
		c.Close()
		log.L(cm.bgCtx).Debugf("Stopped %s", name)
	}
	log.L(cm.bgCtx).Debug("Stopped")
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

func (cm *componentManager) TransportManager() components.TransportManager {
	return cm.transportManager
}

func (cm *componentManager) RegistryManager() components.RegistryManager {
	return cm.registryManager
}

func (cm *componentManager) PluginManager() components.PluginManager {
	return cm.pluginManager
}

func (cm *componentManager) Engine() components.Engine {
	return cm.engine
}
