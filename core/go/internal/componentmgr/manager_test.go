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
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/mocks/blockindexermocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/ethclientmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/rpcservermocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/blockindexer"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/google/uuid"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/rpcserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInitOK(t *testing.T) {

	l, err := net.Listen("tcp4", ":0")
	require.NoError(t, err)
	debugPort := l.Addr().(*net.TCPAddr).Port
	metricsPort := 6100
	require.NoError(t, l.Close())

	// We build a config that allows us to get through init successfully, as should be possible
	// (anything that can't do this should have a separate Start() phase).
	testConfig := &pldconf.PaladinConfig{
		TransportManagerInlineConfig: pldconf.TransportManagerInlineConfig{
			NodeName: "node1",
		},
		DB: pldconf.DBConfig{
			Type: "sqlite",
			SQLite: pldconf.SQLiteConfig{
				SQLDBConfig: pldconf.SQLDBConfig{
					DSN:           ":memory:",
					AutoMigrate:   confutil.P(true),
					MigrationsDir: "../../db/migrations/sqlite",
				},
			},
		},
		Blockchain: pldconf.EthClientConfig{
			HTTP: pldconf.HTTPClientConfig{
				URL: "http://localhost:8545", // we won't actually connect this test, just check the config
			},
		},
		KeyManagerInlineConfig: pldconf.KeyManagerInlineConfig{
			Wallets: []*pldconf.WalletConfig{
				{
					Name: "wallet1",
					Signer: &pldconf.SignerConfig{
						KeyDerivation: pldconf.KeyDerivationConfig{
							Type: pldconf.KeyDerivationTypeBIP32,
						},
						KeyStore: pldconf.KeyStoreConfig{
							Type: "static",
							Static: pldconf.StaticKeyStoreConfig{
								Keys: map[string]pldconf.StaticKeyEntryConfig{
									"seed": {
										Encoding: "hex",
										Inline:   pldtypes.RandHex(32),
									},
								},
							},
						},
					},
				},
			},
		},
		RPCServer: pldconf.RPCServerConfig{
			HTTP: pldconf.RPCServerConfigHTTP{Disabled: true},
			WS:   pldconf.RPCServerConfigWS{Disabled: true},
		},
		DebugServer: pldconf.DebugServerConfig{
			Enabled: confutil.P(true),
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Port: confutil.P(debugPort),
			},
		},
		MetricsServer: pldconf.MetricsServerConfig{
			Enabled: confutil.P(true),
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Port: confutil.P(metricsPort),
			},
		},
	}

	mockExtraManager := componentsmocks.NewAdditionalManager(t)
	mockExtraManager.On("Name").Return("unittest_manager")
	mockExtraManager.On("PreInit", mock.Anything).Return(&components.ManagerInitResult{}, nil)
	mockExtraManager.On("PostInit", mock.Anything).Return(nil)
	cm := NewComponentManager(context.Background(), tempSocketFile(t), uuid.New(), testConfig, mockExtraManager).(*componentManager)
	err = cm.Init()
	require.NoError(t, err)

	assert.NotNil(t, cm.KeyManager())
	assert.NotNil(t, cm.EthClientFactory())
	assert.NotNil(t, cm.Persistence())
	assert.NotNil(t, cm.StateManager())
	assert.NotNil(t, cm.RPCServer())
	assert.NotNil(t, cm.BlockIndexer())
	assert.NotNil(t, cm.DomainManager())
	assert.NotNil(t, cm.TransportManager())
	assert.NotNil(t, cm.RegistryManager())
	assert.NotNil(t, cm.PluginManager())
	assert.NotNil(t, cm.SequencerManager())
	assert.NotNil(t, cm.PublicTxManager())
	assert.NotNil(t, cm.TxManager())
	assert.NotNil(t, cm.GroupManager())
	assert.NotNil(t, cm.IdentityResolver())

	// Check we can send a request for a javadump - even just after init (not start)
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/debug/javadump", debugPort))
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	cm.Stop()

}

func tempSocketFile(t *testing.T) (fileName string) {
	f, err := os.CreateTemp("", "p.*.sock")
	if err == nil {
		fileName = f.Name()
	}
	if err == nil {
		err = f.Close()
	}
	if err == nil {
		err = os.Remove(fileName)
	}
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Remove(fileName)
	})
	return
}

func TestStartOK(t *testing.T) {

	mockEthClientFactory := ethclientmocks.NewEthClientFactory(t)
	mockEthClientFactory.On("Start").Return(nil)
	mockEthClientFactory.On("Stop").Return()

	mockBlockIndexer := blockindexermocks.NewBlockIndexer(t)
	mockBlockIndexer.On("Start").Return(nil)
	mockBlockIndexer.On("GetBlockListenerHeight", mock.Anything).Return(uint64(12345), nil)
	mockBlockIndexer.On("RPCModule").Return(nil)
	mockBlockIndexer.On("Stop").Return()

	mockPluginManager := componentsmocks.NewPluginManager(t)
	mockPluginManager.On("Start").Return(nil)
	mockPluginManager.On("WaitForInit", mock.Anything, prototk.PluginInfo_SIGNING_MODULE).Return(nil)
	mockPluginManager.On("WaitForInit", mock.Anything, prototk.PluginInfo_DOMAIN).Return(nil)
	mockPluginManager.On("Stop").Return()

	mockKeyManager := componentsmocks.NewKeyManager(t)
	mockKeyManager.On("Start").Return(nil)
	mockKeyManager.On("Stop").Return()

	mockDomainManager := componentsmocks.NewDomainManager(t)
	mockDomainManager.On("Start").Return(nil)
	mockDomainManager.On("Stop").Return()

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockTransportManager.On("Start").Return(nil)
	mockTransportManager.On("Stop").Return()

	mockRegistryManager := componentsmocks.NewRegistryManager(t)
	mockRegistryManager.On("Start").Return(nil)
	mockRegistryManager.On("Stop").Return()

	mockPublicTxManager := componentsmocks.NewPublicTxManager(t)
	mockPublicTxManager.On("Start").Return(nil)
	mockPublicTxManager.On("Stop").Return()

	mockSequencerManager := componentsmocks.NewSequencerManager(t)
	mockSequencerManager.On("Start").Return(nil)
	mockSequencerManager.On("Stop").Return()

	mockTxManager := componentsmocks.NewTXManager(t)
	mockTxManager.On("Start").Return(nil)
	mockTxManager.On("Stop").Return()
	mockTxManager.On("LoadBlockchainEventListeners").Return(nil)

	mockGroupManager := componentsmocks.NewGroupManager(t)
	mockGroupManager.On("Start").Return(nil)
	mockGroupManager.On("Stop").Return()

	mockRPCAuthManager := componentsmocks.NewRPCAuthManager(t)
	mockRPCAuthManager.On("Start").Return(nil)
	mockRPCAuthManager.On("Stop").Return()

	mockStateManager := componentsmocks.NewStateManager(t)
	mockStateManager.On("Start").Return(nil)
	mockStateManager.On("Stop").Return()

	mockRPCServer := rpcservermocks.NewRPCServer(t)
	mockRPCServer.On("Start").Return(nil)
	mockRPCServer.On("Register", mock.AnythingOfType("*rpcserver.RPCModule")).Return()
	mockRPCServer.On("Stop").Return()
	mockRPCServer.On("HTTPAddr").Return(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8545})
	mockRPCServer.On("WSAddr").Return(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8546})

	mockExtraManager := componentsmocks.NewAdditionalManager(t)
	mockExtraManager.On("Start").Return(nil)
	mockExtraManager.On("Name").Return("unittest_manager")
	mockExtraManager.On("Stop").Return()

	cm := NewComponentManager(context.Background(), tempSocketFile(t), uuid.New(), &pldconf.PaladinConfig{}, mockExtraManager).(*componentManager)
	cm.ethClientFactory = mockEthClientFactory
	cm.initResults = map[string]*components.ManagerInitResult{
		"utengine": {
			RPCModules: []*rpcserver.RPCModule{
				rpcserver.NewRPCModule("ut"),
			},
		},
	}
	cm.blockIndexer = mockBlockIndexer
	cm.pluginManager = mockPluginManager
	cm.keyManager = mockKeyManager
	cm.domainManager = mockDomainManager
	cm.transportManager = mockTransportManager
	cm.registryManager = mockRegistryManager
	cm.stateManager = mockStateManager
	cm.rpcServer = mockRPCServer
	cm.publicTxManager = mockPublicTxManager
	cm.sequencerManager = mockSequencerManager
	cm.txManager = mockTxManager
	cm.groupManager = mockGroupManager
	cm.rpcAuthManager = mockRPCAuthManager
	cm.additionalManagers = append(cm.additionalManagers, mockExtraManager)

	err := cm.StartManagers()
	require.NoError(t, err)
	err = cm.CompleteStart()
	require.NoError(t, err)

	cm.Stop()
	require.NoError(t, err)
}

func TestBuildInternalEventStreamsPreCommitPostCommit(t *testing.T) {
	cm := NewComponentManager(context.Background(), tempSocketFile(t), uuid.New(), &pldconf.PaladinConfig{}, nil).(*componentManager)
	handler := func(ctx context.Context, dbTX persistence.DBTX, blocks []*pldapi.IndexedBlock, transactions []*blockindexer.IndexedTransactionNotify) error {
		return nil
	}
	cm.initResults = map[string]*components.ManagerInitResult{
		"utengine": {
			PreCommitHandler: handler,
		},
	}

	streams, err := cm.buildInternalEventStreams()
	assert.NoError(t, err)
	assert.Len(t, streams, 1)
	assert.Equal(t, blockindexer.IESTypePreCommitHandler, streams[0].Type)
	assert.NotNil(t, streams[0].PreCommitHandler)

}

func TestErrorWrapping(t *testing.T) {
	cm := NewComponentManager(context.Background(), tempSocketFile(t), uuid.New(), &pldconf.PaladinConfig{}, nil).(*componentManager)

	mockPersistence, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	mockEthClientFactory := ethclientmocks.NewEthClientFactory(t)

	assert.Regexp(t, "PD010000.*pop", cm.addIfOpened("p", mockPersistence.P, errors.New("pop"), msgs.MsgComponentKeyManagerInitError))
	assert.Regexp(t, "PD010002.*pop", cm.addIfStarted("eth_client", mockEthClientFactory, errors.New("pop"), msgs.MsgComponentEthClientInitError))
	assert.Regexp(t, "PD010008.*pop", cm.wrapIfErr(errors.New("pop"), msgs.MsgComponentBlockIndexerInitError))

}

func TestCompleteStart_MultipleAuthorizers(t *testing.T) {
	mockEthClientFactory := ethclientmocks.NewEthClientFactory(t)
	mockEthClientFactory.On("Start").Return(nil)
	mockEthClientFactory.On("Stop").Return()

	mockPluginManager := componentsmocks.NewPluginManager(t)
	mockPluginManager.On("Start").Return(nil)
	mockPluginManager.On("WaitForInit", mock.Anything, prototk.PluginInfo_SIGNING_MODULE).Return(nil)
	mockPluginManager.On("WaitForInit", mock.Anything, prototk.PluginInfo_DOMAIN).Return(nil)
	mockPluginManager.On("WaitForInit", mock.Anything, prototk.PluginInfo_RPC_AUTH).Return(nil)
	mockPluginManager.On("Stop").Return()

	mockBlockIndexer := blockindexermocks.NewBlockIndexer(t)
	mockBlockIndexer.On("Start", mock.Anything).Return(nil)
	mockBlockIndexer.On("GetBlockListenerHeight", mock.Anything).Return(uint64(0), nil)
	mockBlockIndexer.On("RPCModule").Return(nil)
	mockBlockIndexer.On("Stop").Return()

	mockRPCAuthManager := componentsmocks.NewRPCAuthManager(t)
	mockRPCAuthManager.On("Start").Return(nil)
	mockRPCAuthManager.On("Stop").Return()

	mockKeyManager := componentsmocks.NewKeyManager(t)
	mockKeyManager.On("Start").Return(nil)
	mockKeyManager.On("Stop").Return()

	mockDomainManager := componentsmocks.NewDomainManager(t)
	mockDomainManager.On("Start").Return(nil)
	mockDomainManager.On("Stop").Return()

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockTransportManager.On("Start").Return(nil)
	mockTransportManager.On("Stop").Return()

	mockRegistryManager := componentsmocks.NewRegistryManager(t)
	mockRegistryManager.On("Start").Return(nil)
	mockRegistryManager.On("Stop").Return()

	mockPublicTxManager := componentsmocks.NewPublicTxManager(t)
	mockPublicTxManager.On("Start").Return(nil)
	mockPublicTxManager.On("Stop").Return()

	mockSequencerManager := componentsmocks.NewSequencerManager(t)
	mockSequencerManager.On("Start").Return(nil)
	mockSequencerManager.On("Stop").Return()

	mockTxManager := componentsmocks.NewTXManager(t)
	mockTxManager.On("Start").Return(nil)
	mockTxManager.On("Stop").Return()
	mockTxManager.On("LoadBlockchainEventListeners").Return(nil)

	mockGroupManager := componentsmocks.NewGroupManager(t)
	mockGroupManager.On("Start").Return(nil)
	mockGroupManager.On("Stop").Return()

	mockStateManager := componentsmocks.NewStateManager(t)
	mockStateManager.On("Start").Return(nil)
	mockStateManager.On("Stop").Return()

	// Create mock authorizers (simple struct implementing Authorizer interface)
	mockAuthorizer1 := rpcservermocks.NewAuthorizer(t)
	mockAuthorizer2 := rpcservermocks.NewAuthorizer(t)

	// Expect GetRPCAuthorizer to be called twice
	mockRPCAuthManager.On("GetRPCAuthorizer", "auth1").Return(mockAuthorizer1)
	mockRPCAuthManager.On("GetRPCAuthorizer", "auth2").Return(mockAuthorizer2)

	mockRPCServer := rpcservermocks.NewRPCServer(t)
	mockRPCServer.On("Start").Return(nil)
	mockRPCServer.On("Register", mock.Anything).Return()
	mockRPCServer.On("SetAuthorizers", mock.MatchedBy(func(auths []rpcserver.Authorizer) bool {
		return len(auths) == 2 && auths[0] == mockAuthorizer1 && auths[1] == mockAuthorizer2
	})).Return()
	mockRPCServer.On("Stop").Return()
	mockRPCServer.On("HTTPAddr").Return(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8545})
	mockRPCServer.On("WSAddr").Return(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8546})

	testConfig := &pldconf.PaladinConfig{
		RPCAuthManagerConfig: pldconf.RPCAuthManagerConfig{
			RPCAuthorizers: map[string]*pldconf.RPCAuthorizerConfig{
				"auth1": {
					Plugin: pldconf.PluginConfig{
						Type:    "c-shared",
						Library: "/tmp/auth1.so",
					},
					Config: `{}`,
				},
				"auth2": {
					Plugin: pldconf.PluginConfig{
						Type:    "c-shared",
						Library: "/tmp/auth2.so",
					},
					Config: `{}`,
				},
			},
		},
		RPCServer: pldconf.RPCServerConfig{
			Authorizers: []string{"auth1", "auth2"},
		},
	}

	cm := NewComponentManager(context.Background(), tempSocketFile(t), uuid.New(), testConfig).(*componentManager)
	cm.ethClientFactory = mockEthClientFactory
	cm.keyManager = mockKeyManager
	cm.domainManager = mockDomainManager
	cm.transportManager = mockTransportManager
	cm.registryManager = mockRegistryManager
	cm.publicTxManager = mockPublicTxManager
	cm.sequencerManager = mockSequencerManager
	cm.txManager = mockTxManager
	cm.groupManager = mockGroupManager
	cm.stateManager = mockStateManager
	cm.pluginManager = mockPluginManager
	cm.blockIndexer = mockBlockIndexer
	cm.rpcAuthManager = mockRPCAuthManager
	cm.rpcServer = mockRPCServer
	cm.initResults = map[string]*components.ManagerInitResult{
		"utengine": {
			RPCModules: []*rpcserver.RPCModule{
				rpcserver.NewRPCModule("ut"),
			},
		},
	}

	err := cm.StartManagers()
	require.NoError(t, err)
	err = cm.CompleteStart()
	require.NoError(t, err)

	cm.Stop()

	mockRPCServer.AssertExpectations(t)
	mockRPCAuthManager.AssertExpectations(t)
}

func TestCompleteStart_SingleAuthorizer(t *testing.T) {
	mockEthClientFactory := ethclientmocks.NewEthClientFactory(t)
	mockEthClientFactory.On("Start").Return(nil)
	mockEthClientFactory.On("Stop").Return()

	mockKeyManager := componentsmocks.NewKeyManager(t)
	mockKeyManager.On("Start").Return(nil)
	mockKeyManager.On("Stop").Return()

	mockDomainManager := componentsmocks.NewDomainManager(t)
	mockDomainManager.On("Start").Return(nil)
	mockDomainManager.On("Stop").Return()

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockTransportManager.On("Start").Return(nil)
	mockTransportManager.On("Stop").Return()

	mockRegistryManager := componentsmocks.NewRegistryManager(t)
	mockRegistryManager.On("Start").Return(nil)
	mockRegistryManager.On("Stop").Return()

	mockPublicTxManager := componentsmocks.NewPublicTxManager(t)
	mockPublicTxManager.On("Start").Return(nil)
	mockPublicTxManager.On("Stop").Return()

	mockSequencerManager := componentsmocks.NewSequencerManager(t)
	mockSequencerManager.On("Start").Return(nil)
	mockSequencerManager.On("Stop").Return()

	mockTxManager := componentsmocks.NewTXManager(t)
	mockTxManager.On("Start").Return(nil)
	mockTxManager.On("Stop").Return()
	mockTxManager.On("LoadBlockchainEventListeners").Return(nil)

	mockGroupManager := componentsmocks.NewGroupManager(t)
	mockGroupManager.On("Start").Return(nil)
	mockGroupManager.On("Stop").Return()

	mockStateManager := componentsmocks.NewStateManager(t)
	mockStateManager.On("Start").Return(nil)
	mockStateManager.On("Stop").Return()

	mockPluginManager := componentsmocks.NewPluginManager(t)
	mockPluginManager.On("Start").Return(nil)
	mockPluginManager.On("WaitForInit", mock.Anything, prototk.PluginInfo_SIGNING_MODULE).Return(nil)
	mockPluginManager.On("WaitForInit", mock.Anything, prototk.PluginInfo_DOMAIN).Return(nil)
	mockPluginManager.On("WaitForInit", mock.Anything, prototk.PluginInfo_RPC_AUTH).Return(nil)
	mockPluginManager.On("Stop").Return()

	mockBlockIndexer := blockindexermocks.NewBlockIndexer(t)
	mockBlockIndexer.On("Start", mock.Anything).Return(nil)
	mockBlockIndexer.On("GetBlockListenerHeight", mock.Anything).Return(uint64(0), nil)
	mockBlockIndexer.On("RPCModule").Return(nil)
	mockBlockIndexer.On("Stop").Return()

	mockRPCAuthManager := componentsmocks.NewRPCAuthManager(t)
	mockRPCAuthManager.On("Start").Return(nil)
	mockRPCAuthManager.On("Stop").Return()

	mockAuthorizer := rpcservermocks.NewAuthorizer(t)

	// Expect GetRPCAuthorizer to be called once
	mockRPCAuthManager.On("GetRPCAuthorizer", "auth1").Return(mockAuthorizer)

	mockRPCServer := rpcservermocks.NewRPCServer(t)
	mockRPCServer.On("Start").Return(nil)
	mockRPCServer.On("Register", mock.Anything).Return()
	mockRPCServer.On("SetAuthorizers", mock.MatchedBy(func(auths []rpcserver.Authorizer) bool {
		return len(auths) == 1 && auths[0] == mockAuthorizer
	})).Return()
	mockRPCServer.On("Stop").Return()
	mockRPCServer.On("HTTPAddr").Return(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8545})
	mockRPCServer.On("WSAddr").Return(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8546})

	testConfig := &pldconf.PaladinConfig{
		RPCAuthManagerConfig: pldconf.RPCAuthManagerConfig{
			RPCAuthorizers: map[string]*pldconf.RPCAuthorizerConfig{
				"auth1": {
					Plugin: pldconf.PluginConfig{
						Type:    "c-shared",
						Library: "/tmp/auth1.so",
					},
					Config: `{}`,
				},
			},
		},
		RPCServer: pldconf.RPCServerConfig{
			Authorizers: []string{"auth1"},
		},
	}

	cm := NewComponentManager(context.Background(), tempSocketFile(t), uuid.New(), testConfig).(*componentManager)
	cm.ethClientFactory = mockEthClientFactory
	cm.keyManager = mockKeyManager
	cm.domainManager = mockDomainManager
	cm.transportManager = mockTransportManager
	cm.registryManager = mockRegistryManager
	cm.publicTxManager = mockPublicTxManager
	cm.sequencerManager = mockSequencerManager
	cm.txManager = mockTxManager
	cm.groupManager = mockGroupManager
	cm.stateManager = mockStateManager
	cm.pluginManager = mockPluginManager
	cm.blockIndexer = mockBlockIndexer
	cm.rpcAuthManager = mockRPCAuthManager
	cm.rpcServer = mockRPCServer
	cm.initResults = map[string]*components.ManagerInitResult{
		"utengine": {
			RPCModules: []*rpcserver.RPCModule{
				rpcserver.NewRPCModule("ut"),
			},
		},
	}

	err := cm.StartManagers()
	require.NoError(t, err)
	err = cm.CompleteStart()
	require.NoError(t, err)

	cm.Stop()

	mockRPCServer.AssertExpectations(t)
	mockRPCAuthManager.AssertExpectations(t)
}

func TestCompleteStart_AuthorizerNotFound(t *testing.T) {
	mockEthClientFactory := ethclientmocks.NewEthClientFactory(t)
	mockEthClientFactory.On("Start").Return(nil)
	mockEthClientFactory.On("Stop").Return().Maybe()

	mockPluginManager := componentsmocks.NewPluginManager(t)
	mockPluginManager.On("Start").Return(nil)
	mockPluginManager.On("WaitForInit", mock.Anything, prototk.PluginInfo_SIGNING_MODULE).Return(nil)
	mockPluginManager.On("WaitForInit", mock.Anything, prototk.PluginInfo_DOMAIN).Return(nil)
	mockPluginManager.On("Stop").Return().Maybe()

	mockBlockIndexer := blockindexermocks.NewBlockIndexer(t)
	mockBlockIndexer.On("Start", mock.Anything).Return(nil).Maybe()
	mockBlockIndexer.On("GetBlockListenerHeight", mock.Anything).Return(uint64(0), nil).Maybe()
	mockBlockIndexer.On("RPCModule").Return(nil)
	mockBlockIndexer.On("Stop").Return().Maybe()

	mockRPCAuthManager := componentsmocks.NewRPCAuthManager(t)
	mockRPCAuthManager.On("Start").Return(nil)
	mockRPCAuthManager.On("Stop").Return().Maybe()

	mockKeyManager := componentsmocks.NewKeyManager(t)
	mockKeyManager.On("Start").Return(nil)
	mockKeyManager.On("Stop").Return().Maybe()

	mockDomainManager := componentsmocks.NewDomainManager(t)
	mockDomainManager.On("Start").Return(nil)
	mockDomainManager.On("Stop").Return().Maybe()

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockTransportManager.On("Start").Return(nil)
	mockTransportManager.On("Stop").Return().Maybe()

	mockRegistryManager := componentsmocks.NewRegistryManager(t)
	mockRegistryManager.On("Start").Return(nil)
	mockRegistryManager.On("Stop").Return().Maybe()

	mockPublicTxManager := componentsmocks.NewPublicTxManager(t)
	mockPublicTxManager.On("Start").Return(nil)
	mockPublicTxManager.On("Stop").Return().Maybe()

	mockSequencerManager := componentsmocks.NewSequencerManager(t)
	mockSequencerManager.On("Start").Return(nil)
	mockSequencerManager.On("Stop").Return()

	mockTxManager := componentsmocks.NewTXManager(t)
	mockTxManager.On("Start").Return(nil)
	mockTxManager.On("Stop").Return().Maybe()
	mockTxManager.On("LoadBlockchainEventListeners").Return(nil).Maybe()

	mockGroupManager := componentsmocks.NewGroupManager(t)
	mockGroupManager.On("Start").Return(nil)
	mockGroupManager.On("Stop").Return().Maybe()

	mockStateManager := componentsmocks.NewStateManager(t)
	mockStateManager.On("Start").Return(nil)
	mockStateManager.On("Stop").Return().Maybe()

	// Expect GetRPCAuthorizer to return nil (not found)
	mockRPCAuthManager.On("GetRPCAuthorizer", "nonexistent").Return(nil)

	mockRPCServer := rpcservermocks.NewRPCServer(t)
	// Register is called even if CompleteStart fails early
	mockRPCServer.On("Register", mock.Anything).Return().Maybe()
	// Start and Stop are NOT called if CompleteStart fails early, so make them optional
	mockRPCServer.On("Start").Return(nil).Maybe()
	mockRPCServer.On("Stop").Return().Maybe()
	mockRPCServer.On("HTTPAddr").Maybe().Return(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8545})
	mockRPCServer.On("WSAddr").Maybe().Return(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8546})

	testConfig := &pldconf.PaladinConfig{
		RPCAuthManagerConfig: pldconf.RPCAuthManagerConfig{
			RPCAuthorizers: map[string]*pldconf.RPCAuthorizerConfig{}, // Empty - test is for "not found" error, not "missing from array"
		},
		RPCServer: pldconf.RPCServerConfig{
			Authorizers: []string{"nonexistent"},
		},
	}

	cm := NewComponentManager(context.Background(), tempSocketFile(t), uuid.New(), testConfig).(*componentManager)
	cm.ethClientFactory = mockEthClientFactory
	cm.keyManager = mockKeyManager
	cm.domainManager = mockDomainManager
	cm.transportManager = mockTransportManager
	cm.registryManager = mockRegistryManager
	cm.publicTxManager = mockPublicTxManager
	cm.sequencerManager = mockSequencerManager
	cm.txManager = mockTxManager
	cm.groupManager = mockGroupManager
	cm.stateManager = mockStateManager
	cm.pluginManager = mockPluginManager
	cm.blockIndexer = mockBlockIndexer
	cm.rpcAuthManager = mockRPCAuthManager
	cm.rpcServer = mockRPCServer
	cm.initResults = map[string]*components.ManagerInitResult{
		"utengine": {
			RPCModules: []*rpcserver.RPCModule{
				rpcserver.NewRPCModule("ut"),
			},
		},
	}

	err := cm.StartManagers()
	require.NoError(t, err)
	err = cm.CompleteStart()
	require.Error(t, err)
	assert.Regexp(t, "PD.*nonexistent", err.Error())

	cm.Stop()

	// Only assert the specific expectations we care about for this test
	mockRPCAuthManager.AssertExpectations(t)
	mockRPCServer.AssertExpectations(t)
	// Note: Other managers' Stop() expectations may not be met if CompleteStart()
	// failed before they were started, so we don't assert them here
}

func TestCompleteStart_AuthorizerMissingFromArray(t *testing.T) {
	mockEthClientFactory := ethclientmocks.NewEthClientFactory(t)
	mockEthClientFactory.On("Start").Return(nil)
	mockEthClientFactory.On("Stop").Return().Maybe()

	mockPluginManager := componentsmocks.NewPluginManager(t)
	mockPluginManager.On("Start").Return(nil)
	mockPluginManager.On("WaitForInit", mock.Anything, prototk.PluginInfo_SIGNING_MODULE).Return(nil)
	mockPluginManager.On("WaitForInit", mock.Anything, prototk.PluginInfo_DOMAIN).Return(nil)
	mockPluginManager.On("WaitForInit", mock.Anything, prototk.PluginInfo_RPC_AUTH).Return(nil)
	mockPluginManager.On("Stop").Return().Maybe()

	mockBlockIndexer := blockindexermocks.NewBlockIndexer(t)
	mockBlockIndexer.On("Start", mock.Anything).Return(nil).Maybe()
	mockBlockIndexer.On("GetBlockListenerHeight", mock.Anything).Return(uint64(0), nil).Maybe()
	mockBlockIndexer.On("RPCModule").Return(nil)
	mockBlockIndexer.On("Stop").Return().Maybe()

	mockRPCAuthManager := componentsmocks.NewRPCAuthManager(t)
	mockRPCAuthManager.On("Start").Return(nil)
	mockRPCAuthManager.On("Stop").Return().Maybe()

	mockKeyManager := componentsmocks.NewKeyManager(t)
	mockKeyManager.On("Start").Return(nil)
	mockKeyManager.On("Stop").Return().Maybe()

	mockDomainManager := componentsmocks.NewDomainManager(t)
	mockDomainManager.On("Start").Return(nil)
	mockDomainManager.On("Stop").Return().Maybe()

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockTransportManager.On("Start").Return(nil)
	mockTransportManager.On("Stop").Return().Maybe()

	mockRegistryManager := componentsmocks.NewRegistryManager(t)
	mockRegistryManager.On("Start").Return(nil)
	mockRegistryManager.On("Stop").Return().Maybe()

	mockPublicTxManager := componentsmocks.NewPublicTxManager(t)
	mockPublicTxManager.On("Start").Return(nil)
	mockPublicTxManager.On("Stop").Return().Maybe()

	mockSequencerManager := componentsmocks.NewSequencerManager(t)
	mockSequencerManager.On("Start").Return(nil)
	mockSequencerManager.On("Stop").Return()

	mockTxManager := componentsmocks.NewTXManager(t)
	mockTxManager.On("Start").Return(nil)
	mockTxManager.On("Stop").Return().Maybe()
	mockTxManager.On("LoadBlockchainEventListeners").Return(nil).Maybe()

	mockGroupManager := componentsmocks.NewGroupManager(t)
	mockGroupManager.On("Start").Return(nil)
	mockGroupManager.On("Stop").Return().Maybe()

	mockStateManager := componentsmocks.NewStateManager(t)
	mockStateManager.On("Start").Return(nil)
	mockStateManager.On("Stop").Return().Maybe()

	mockRPCServer := rpcservermocks.NewRPCServer(t)
	// Register is called even if CompleteStart fails early
	mockRPCServer.On("Register", mock.Anything).Return().Maybe()
	// Start and Stop are NOT called if CompleteStart fails early, so make them optional
	mockRPCServer.On("Start").Return(nil).Maybe()
	mockRPCServer.On("Stop").Return().Maybe()
	mockRPCServer.On("HTTPAddr").Maybe().Return(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8545})
	mockRPCServer.On("WSAddr").Maybe().Return(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8546})

	// Configure two authorizers but only include one in the authorizers array
	testConfig := &pldconf.PaladinConfig{
		RPCAuthManagerConfig: pldconf.RPCAuthManagerConfig{
			RPCAuthorizers: map[string]*pldconf.RPCAuthorizerConfig{
				"auth1": {
					Plugin: pldconf.PluginConfig{
						Type:    "c-shared",
						Library: "/tmp/auth1.so",
					},
					Config: `{}`,
				},
				"auth2": {
					Plugin: pldconf.PluginConfig{
						Type:    "c-shared",
						Library: "/tmp/auth2.so",
					},
					Config: `{}`,
				},
			},
		},
		RPCServer: pldconf.RPCServerConfig{
			Authorizers: []string{"auth1"}, // auth2 is missing
		},
	}

	cm := NewComponentManager(context.Background(), tempSocketFile(t), uuid.New(), testConfig).(*componentManager)
	cm.ethClientFactory = mockEthClientFactory
	cm.keyManager = mockKeyManager
	cm.domainManager = mockDomainManager
	cm.transportManager = mockTransportManager
	cm.registryManager = mockRegistryManager
	cm.publicTxManager = mockPublicTxManager
	cm.sequencerManager = mockSequencerManager
	cm.txManager = mockTxManager
	cm.groupManager = mockGroupManager
	cm.stateManager = mockStateManager
	cm.pluginManager = mockPluginManager
	cm.blockIndexer = mockBlockIndexer
	cm.rpcAuthManager = mockRPCAuthManager
	cm.rpcServer = mockRPCServer
	cm.initResults = map[string]*components.ManagerInitResult{
		"utengine": {
			RPCModules: []*rpcserver.RPCModule{
				rpcserver.NewRPCModule("ut"),
			},
		},
	}

	err := cm.StartManagers()
	require.NoError(t, err)
	err = cm.CompleteStart()
	require.Error(t, err)
	assert.Regexp(t, "PD.*auth2", err.Error())

	cm.Stop()

	// Only assert the specific expectations we care about for this test
	mockRPCAuthManager.AssertExpectations(t)
	mockRPCServer.AssertExpectations(t)
}

func TestCompleteStart_AuthorizersArrayEmptyButConfigured(t *testing.T) {
	mockEthClientFactory := ethclientmocks.NewEthClientFactory(t)
	mockEthClientFactory.On("Start").Return(nil)
	mockEthClientFactory.On("Stop").Return().Maybe()

	mockPluginManager := componentsmocks.NewPluginManager(t)
	mockPluginManager.On("Start").Return(nil)
	mockPluginManager.On("WaitForInit", mock.Anything, prototk.PluginInfo_SIGNING_MODULE).Return(nil)
	mockPluginManager.On("WaitForInit", mock.Anything, prototk.PluginInfo_DOMAIN).Return(nil)
	mockPluginManager.On("WaitForInit", mock.Anything, prototk.PluginInfo_RPC_AUTH).Return(nil)
	mockPluginManager.On("Stop").Return().Maybe()

	mockBlockIndexer := blockindexermocks.NewBlockIndexer(t)
	mockBlockIndexer.On("Start", mock.Anything).Return(nil).Maybe()
	mockBlockIndexer.On("GetBlockListenerHeight", mock.Anything).Return(uint64(0), nil).Maybe()
	mockBlockIndexer.On("RPCModule").Return(nil)
	mockBlockIndexer.On("Stop").Return().Maybe()

	mockRPCAuthManager := componentsmocks.NewRPCAuthManager(t)
	mockRPCAuthManager.On("Start").Return(nil)
	mockRPCAuthManager.On("Stop").Return().Maybe()

	mockKeyManager := componentsmocks.NewKeyManager(t)
	mockKeyManager.On("Start").Return(nil)
	mockKeyManager.On("Stop").Return().Maybe()

	mockDomainManager := componentsmocks.NewDomainManager(t)
	mockDomainManager.On("Start").Return(nil)
	mockDomainManager.On("Stop").Return().Maybe()

	mockTransportManager := componentsmocks.NewTransportManager(t)
	mockTransportManager.On("Start").Return(nil)
	mockTransportManager.On("Stop").Return().Maybe()

	mockRegistryManager := componentsmocks.NewRegistryManager(t)
	mockRegistryManager.On("Start").Return(nil)
	mockRegistryManager.On("Stop").Return().Maybe()

	mockPublicTxManager := componentsmocks.NewPublicTxManager(t)
	mockPublicTxManager.On("Start").Return(nil)
	mockPublicTxManager.On("Stop").Return().Maybe()

	mockSequencerManager := componentsmocks.NewSequencerManager(t)
	mockSequencerManager.On("Start").Return(nil)
	mockSequencerManager.On("Stop").Return()

	mockTxManager := componentsmocks.NewTXManager(t)
	mockTxManager.On("Start").Return(nil)
	mockTxManager.On("Stop").Return().Maybe()
	mockTxManager.On("LoadBlockchainEventListeners").Return(nil).Maybe()

	mockGroupManager := componentsmocks.NewGroupManager(t)
	mockGroupManager.On("Start").Return(nil)
	mockGroupManager.On("Stop").Return().Maybe()

	mockStateManager := componentsmocks.NewStateManager(t)
	mockStateManager.On("Start").Return(nil)
	mockStateManager.On("Stop").Return().Maybe()

	mockRPCServer := rpcservermocks.NewRPCServer(t)
	// Register is called even if CompleteStart fails early
	mockRPCServer.On("Register", mock.Anything).Return().Maybe()
	// Start and Stop are NOT called if CompleteStart fails early, so make them optional
	mockRPCServer.On("Start").Return(nil).Maybe()
	mockRPCServer.On("Stop").Return().Maybe()
	mockRPCServer.On("HTTPAddr").Maybe().Return(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8545})
	mockRPCServer.On("WSAddr").Maybe().Return(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8546})

	// Configure authorizers but authorizers array is empty
	testConfig := &pldconf.PaladinConfig{
		RPCAuthManagerConfig: pldconf.RPCAuthManagerConfig{
			RPCAuthorizers: map[string]*pldconf.RPCAuthorizerConfig{
				"auth1": {
					Plugin: pldconf.PluginConfig{
						Type:    "c-shared",
						Library: "/tmp/auth1.so",
					},
					Config: `{}`,
				},
			},
		},
		RPCServer: pldconf.RPCServerConfig{
			Authorizers: []string{}, // Empty array
		},
	}

	cm := NewComponentManager(context.Background(), tempSocketFile(t), uuid.New(), testConfig).(*componentManager)
	cm.ethClientFactory = mockEthClientFactory
	cm.keyManager = mockKeyManager
	cm.domainManager = mockDomainManager
	cm.transportManager = mockTransportManager
	cm.registryManager = mockRegistryManager
	cm.publicTxManager = mockPublicTxManager
	cm.sequencerManager = mockSequencerManager
	cm.txManager = mockTxManager
	cm.groupManager = mockGroupManager
	cm.stateManager = mockStateManager
	cm.pluginManager = mockPluginManager
	cm.blockIndexer = mockBlockIndexer
	cm.rpcAuthManager = mockRPCAuthManager
	cm.rpcServer = mockRPCServer
	cm.initResults = map[string]*components.ManagerInitResult{
		"utengine": {
			RPCModules: []*rpcserver.RPCModule{
				rpcserver.NewRPCModule("ut"),
			},
		},
	}

	err := cm.StartManagers()
	require.NoError(t, err)
	err = cm.CompleteStart()
	require.Error(t, err)
	assert.Regexp(t, "PD.*auth1", err.Error())

	cm.Stop()

	// Only assert the specific expectations we care about for this test
	mockRPCAuthManager.AssertExpectations(t)
	mockRPCServer.AssertExpectations(t)
}
