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
	"net/http"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/domainmgr"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/groupmgr"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/identityresolver"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/keymanager"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/metrics"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/plugins"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/publictxmgr"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/registrymgr"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/statemgr"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/transportmgr"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/txmgr"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/blockindexer"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/ethclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/retry"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/httpserver"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/metricsserver"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
	"github.com/google/uuid"
)

type ComponentManager interface {
	components.AllComponents
	Init() error
	StartManagers() error
	CompleteStart() error
	Stop()
}

type componentManager struct {
	grpcTarget   string
	instanceUUID uuid.UUID
	bgCtx        context.Context
	// config
	conf *pldconf.PaladinConfig
	// debug server
	debugServer httpserver.Server
	// pre-init
	keyManager       components.KeyManager
	ethClientFactory ethclient.EthClientFactory
	persistence      persistence.Persistence
	blockIndexer     blockindexer.BlockIndexer
	rpcServer        rpcserver.RPCServer
	metricsServer    metricsserver.MetricsServer
	metricsManager   metrics.Metrics

	// managers
	stateManager     components.StateManager
	domainManager    components.DomainManager
	transportManager components.TransportManager
	registryManager  components.RegistryManager
	pluginManager    components.PluginManager
	publicTxManager  components.PublicTxManager
	privateTxManager components.PrivateTxManager
	txManager        components.TXManager
	identityResolver components.IdentityResolver
	groupManager     components.GroupManager
	// managers that are not a core part of the engine, but allow Paladin to operate in an extended mode - the testbed is an example.
	// these cannot be queried by other components (no AdditionalManagers() function on AllComponents)
	additionalManagers []components.AdditionalManager
	// init to start tracking
	initResults          map[string]*components.ManagerInitResult
	internalEventStreams []*blockindexer.InternalEventStream
	// keep track of everything we started
	started map[string]stoppable
	opened  map[string]closeable
	// limited startup retry for connecting to blockchain
	ethClientStartupRetry *retry.Retry
}

// things that have a running component that is active in the background and hence "stops"
type stoppable interface {
	Stop()
}

// things that are services used in various places, but need to cleanly disconnect all connections and hence "close"
type closeable interface {
	Close()
}

func NewComponentManager(bgCtx context.Context, grpcTarget string, instanceUUID uuid.UUID, conf *pldconf.PaladinConfig,
	// the testbed registers itself into the lifecycle as an additional manager
	additionalManagers ...components.AdditionalManager,
) ComponentManager {
	log.InitConfig(&conf.Log)
	return &componentManager{
		grpcTarget:            grpcTarget, // default is a UDS path, can use tcp:127.0.0.1:12345 strings too (or tcp4:/tcp6:)
		instanceUUID:          instanceUUID,
		bgCtx:                 bgCtx,
		conf:                  conf,
		additionalManagers:    additionalManagers,
		initResults:           make(map[string]*components.ManagerInitResult),
		started:               make(map[string]stoppable),
		opened:                make(map[string]closeable),
		ethClientStartupRetry: retry.NewRetryLimited(&conf.Startup.BlockchainConnectRetry, &pldconf.StartupConfigDefaults.BlockchainConnectRetry),
	}
}

func (cm *componentManager) javaDump(res http.ResponseWriter, req *http.Request) {
	cm.pluginManager.SendSystemCommandToLoader(prototk.PluginLoad_THREAD_DUMP)
	res.WriteHeader(202)
}

func (cm *componentManager) startDebugServer() (httpserver.Server, error) {
	cm.conf.DebugServer.Port = confutil.P(confutil.Int(cm.conf.DebugServer.Port, 0)) // if enabled with no port, we allocate one
	server, err := httpserver.NewDebugServer(cm.bgCtx, &cm.conf.DebugServer.HTTPServerConfig)
	if err == nil {
		server.Router().PathPrefix("/debug/javadump").HandlerFunc(http.HandlerFunc(cm.javaDump))
		err = server.Start()
	}
	return server, err
}

func (cm *componentManager) Init() (err error) {
	// start the debug server as early as possible
	if confutil.Bool(cm.conf.DebugServer.Enabled, *pldconf.DebugServerDefaults.Enabled) {
		cm.debugServer, err = cm.startDebugServer()
		err = cm.addIfStarted("debugServer", cm.debugServer, err, msgs.MsgComponentDebugServerStartError)
	}

	if err == nil {
		cm.ethClientFactory, err = ethclient.NewEthClientFactory(cm.bgCtx, &cm.conf.Blockchain)
		err = cm.wrapIfErr(err, msgs.MsgComponentEthClientInitError)
	}

	if err == nil {
		cm.persistence, err = persistence.NewPersistence(cm.bgCtx, &cm.conf.DB)
		err = cm.addIfOpened("database", cm.persistence, err, msgs.MsgComponentDBInitError)
	}
	if err == nil {
		cm.blockIndexer, err = blockindexer.NewBlockIndexer(cm.bgCtx, &cm.conf.BlockIndexer, &cm.conf.Blockchain.WS, cm.persistence)
		err = cm.wrapIfErr(err, msgs.MsgComponentBlockIndexerInitError)
	}
	if err == nil {
		cm.rpcServer, err = rpcserver.NewRPCServer(cm.bgCtx, &cm.conf.RPCServer)
		err = cm.wrapIfErr(err, msgs.MsgComponentRPCServerInitError)
	}
	if err == nil {
		cm.metricsManager = metrics.NewMetricsManager(cm.bgCtx)
		err = cm.wrapIfErr(err, msgs.MsgComponentMetricsManagerInitError)
	}
	if err == nil {
		if confutil.Bool(cm.conf.MetricsServer.Enabled, *pldconf.MetricsServerDefaults.Enabled) {
			cm.metricsServer, err = metricsserver.NewMetricsServer(cm.bgCtx, cm.metricsManager.Registry(), &cm.conf.MetricsServer)
			err = cm.wrapIfErr(err, msgs.MsgComponentMetricsServerInitError)
		}
	}

	// pre-init managers
	if err == nil {
		cm.keyManager = keymanager.NewKeyManager(cm.bgCtx, &cm.conf.KeyManagerConfig)
		cm.initResults["key_manager"], err = cm.keyManager.PreInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentKeyManagerInitError)
	}
	if err == nil {
		cm.stateManager = statemgr.NewStateManager(cm.bgCtx, &cm.conf.StateStore, cm.persistence)
		cm.initResults["state_manager"], err = cm.stateManager.PreInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentStateManagerInitError)
	}
	if err == nil {
		cm.domainManager = domainmgr.NewDomainManager(cm.bgCtx, &cm.conf.DomainManagerConfig)
		cm.initResults["domain_manager"], err = cm.domainManager.PreInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentDomainInitError)
	}

	if err == nil {
		cm.transportManager = transportmgr.NewTransportManager(cm.bgCtx, &cm.conf.TransportManagerConfig)
		cm.initResults["transports_manager"], err = cm.transportManager.PreInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentTransportInitError)
	}

	if err == nil {
		cm.registryManager = registrymgr.NewRegistryManager(cm.bgCtx, &cm.conf.RegistryManagerConfig)
		cm.initResults["registry_manager"], err = cm.registryManager.PreInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentRegistryInitError)
	}

	if err == nil {
		cm.pluginManager = plugins.NewPluginManager(cm.bgCtx, cm.grpcTarget, cm.instanceUUID, &cm.conf.PluginManagerConfig)
		cm.initResults["plugin_manager"], err = cm.pluginManager.PreInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentPluginInitError)
	}

	if err == nil {
		cm.publicTxManager = publictxmgr.NewPublicTransactionManager(cm.bgCtx, &cm.conf.PublicTxManager)
		cm.initResults["public_tx_manager"], err = cm.publicTxManager.PreInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentPublicTxnManagerInitError)
	}

	if err == nil {
		cm.privateTxManager = privatetxnmgr.NewPrivateTransactionMgr(cm.bgCtx, &cm.conf.PrivateTxManager)
		cm.initResults["private_tx_manager"], err = cm.privateTxManager.PreInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentPrivateTxManagerInitError)
	}

	if err == nil {
		cm.txManager = txmgr.NewTXManager(cm.bgCtx, &cm.conf.TxManager)
		cm.initResults["tx_manager"], err = cm.txManager.PreInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentTxManagerInitError)
	}

	if err == nil {
		cm.groupManager = groupmgr.NewGroupManager(cm.bgCtx, &cm.conf.GroupManager)
		cm.initResults["group_manager"], err = cm.groupManager.PreInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentGroupManagerInitError)
	}

	if err == nil {
		cm.identityResolver = identityresolver.NewIdentityResolver(cm.bgCtx, &cm.conf.IdentityResolver)
		cm.initResults["identity_resolver"], err = cm.identityResolver.PreInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentIdentityResolverInitError)
	}

	for _, am := range cm.additionalManagers {
		if err == nil {
			cm.initResults[am.Name()], err = am.PreInit(cm)
			err = cm.wrapIfErr(err, msgs.MsgComponentAdditionalMgrInitError, am.Name())
		}

	}

	// post-init the managers
	if err == nil {
		err = cm.keyManager.PostInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentKeyManagerInitError)
	}

	if err == nil {
		err = cm.stateManager.PostInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentStateManagerInitError)
	}

	if err == nil {
		err = cm.domainManager.PostInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentDomainInitError)
	}

	if err == nil {
		err = cm.transportManager.PostInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentTransportInitError)
	}

	if err == nil {
		err = cm.registryManager.PostInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentRegistryInitError)
	}

	if err == nil {
		err = cm.pluginManager.PostInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentPluginInitError)
	}

	if err == nil {
		err = cm.publicTxManager.PostInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentPublicTxnManagerInitError)
	}

	if err == nil {
		err = cm.privateTxManager.PostInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentPrivateTxManagerInitError)
	}

	if err == nil {
		err = cm.txManager.PostInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentTxManagerInitError)
	}

	if err == nil {
		err = cm.groupManager.PostInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentGroupManagerInitError)
	}

	if err == nil {
		err = cm.identityResolver.PostInit(cm)
		err = cm.wrapIfErr(err, msgs.MsgComponentIdentityResolverInitError)
	}

	for _, am := range cm.additionalManagers {
		if err == nil {
			err = am.PostInit(cm)
			err = cm.wrapIfErr(err, msgs.MsgComponentAdditionalMgrInitError, am.Name())
		}
	}

	return err
}

func (cm *componentManager) startBlockIndexer() (err error) {
	// start the block indexer
	cm.internalEventStreams, err = cm.buildInternalEventStreams()
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
	if err == nil {
		err = cm.txManager.LoadBlockchainEventListeners()
	}
	return err
}

func (cm *componentManager) startEthClient() error {
	return cm.ethClientStartupRetry.Do(cm.bgCtx, func(attempt int) (retryable bool, err error) {
		return true, cm.ethClientFactory.Start()
	})
}

func (cm *componentManager) StartManagers() (err error) {

	// start the eth client before any managers - this connects the WebSocket, and gathers the ChainID
	// We have special handling here to allow for concurrent startup of the blockchain node and Paladin
	err = cm.startEthClient()
	err = cm.addIfStarted("eth_client", cm.ethClientFactory, err, msgs.MsgComponentEthClientStartError)

	// start the managers
	if err == nil {
		err = cm.pluginManager.Start()
		err = cm.addIfStarted("plugin_manager", cm.pluginManager, err, msgs.MsgComponentPluginStartError)
	}

	// Wait for signing module plugins to all start before starting the key manager
	if err == nil {
		err = cm.pluginManager.WaitForInit(cm.bgCtx, prototk.PluginInfo_SIGNING_MODULE)
		err = cm.wrapIfErr(err, msgs.MsgComponentWaitPluginStartError)
	}

	if err == nil {
		err = cm.keyManager.Start()
		err = cm.addIfStarted("key_manager", cm.keyManager, err, msgs.MsgComponentKeyManagerStartError)
	}

	if err == nil {
		err = cm.stateManager.Start()
		err = cm.addIfStarted("state_manager", cm.stateManager, err, msgs.MsgComponentStateManagerStartError)
	}

	if err == nil {
		err = cm.domainManager.Start()
		err = cm.addIfStarted("domain_manager", cm.domainManager, err, msgs.MsgComponentDomainStartError)
	}

	if err == nil {
		err = cm.transportManager.Start()
		err = cm.addIfStarted("transport_manager", cm.transportManager, err, msgs.MsgComponentTransportStartError)
	}

	if err == nil {
		err = cm.registryManager.Start()
		err = cm.addIfStarted("registry_manager", cm.registryManager, err, msgs.MsgComponentRegistryStartError)
	}

	if err == nil {
		err = cm.publicTxManager.Start()
		err = cm.addIfStarted("public_tx_manager", cm.publicTxManager, err, msgs.MsgComponentPublicTxManagerStartError)
	}

	if err == nil {
		err = cm.privateTxManager.Start()
		err = cm.addIfStarted("private_tx_manager", cm.privateTxManager, err, msgs.MsgComponentPrivateTxManagerStartError)
	}

	if err == nil {
		err = cm.txManager.Start()
		err = cm.addIfStarted("tx_manager", cm.txManager, err, msgs.MsgComponentTxManagerStartError)
	}

	if err == nil {
		err = cm.groupManager.Start()
		err = cm.addIfStarted("group_manager", cm.groupManager, err, msgs.MsgComponentGroupManagerStartError)
	}

	for _, am := range cm.additionalManagers {
		if err == nil {
			err = am.Start()
			err = cm.addIfStarted(am.Name(), am, err, msgs.MsgComponentAdditionalMgrStartError, am.Name())
		}
	}

	return err
}

func (cm *componentManager) CompleteStart() error {
	// Wait for the domain plugins to all start
	err := cm.pluginManager.WaitForInit(cm.bgCtx, prototk.PluginInfo_DOMAIN)
	err = cm.wrapIfErr(err, msgs.MsgComponentWaitPluginStartError)

	// then start the block indexer
	if err == nil {
		err = cm.startBlockIndexer()
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
		log.L(cm.bgCtx).Infof("RPC endpoints http=%s ws=%s", httpEndpoint, wsEndpoint)
	}

	if cm.metricsServer != nil {
		err = cm.metricsServer.Start()
		err = cm.addIfStarted("metrics_server", cm.metricsServer, err, msgs.MsgComponentMetricsServerStartError)
	}

	log.L(cm.bgCtx).Infof("Startup complete")

	return err
}

func (cm *componentManager) wrapIfErr(err error, failMsg i18n.ErrorMessageKey, inserts ...any) error {
	if err != nil {
		return i18n.WrapError(cm.bgCtx, err, failMsg, inserts...)
	}
	return nil
}

func (cm *componentManager) addIfStarted(desc string, c stoppable, err error, failMsg i18n.ErrorMessageKey, inserts ...any) error {
	if err != nil {
		return i18n.WrapError(cm.bgCtx, err, failMsg, inserts...)
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
	for _, initResult := range cm.initResults {
		if initResult.PreCommitHandler != nil {
			streams = append(streams, &blockindexer.InternalEventStream{
				Type:             blockindexer.IESTypePreCommitHandler,
				PreCommitHandler: initResult.PreCommitHandler,
			})
		}
	}
	return streams, nil
}

func (cm *componentManager) registerRPCModules() {
	// Manager/engine modules
	for _, initResult := range cm.initResults {
		for _, rpcMod := range initResult.RPCModules {
			cm.rpcServer.Register(rpcMod)
		}
	}
	// We handle block indexer separately (doesn't fit the internal ManagerLifecycle model
	// as it's currently a standalone re-usable component)
	cm.rpcServer.Register(cm.BlockIndexer().RPCModule())
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

func (cm *componentManager) KeyManager() components.KeyManager {
	return cm.keyManager
}

func (cm *componentManager) EthClientFactory() ethclient.EthClientFactory {
	return cm.ethClientFactory
}

func (cm *componentManager) Persistence() persistence.Persistence {
	return cm.persistence
}

func (cm *componentManager) StateManager() components.StateManager {
	return cm.stateManager
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

func (cm *componentManager) PublicTxManager() components.PublicTxManager {
	return cm.publicTxManager
}

func (cm *componentManager) PrivateTxManager() components.PrivateTxManager {
	return cm.privateTxManager
}

func (cm *componentManager) TxManager() components.TXManager {
	return cm.txManager
}

func (cm *componentManager) GroupManager() components.GroupManager {
	return cm.groupManager
}

func (cm *componentManager) IdentityResolver() components.IdentityResolver {
	return cm.identityResolver
}

func (cm *componentManager) MetricsManager() metrics.Metrics {
	return cm.metricsManager
}
