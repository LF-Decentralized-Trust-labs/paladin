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

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/blockindexermocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/ethclientmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/rpcservermocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/blockindexer"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence/mockpersistence"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
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
		TransportManagerConfig: pldconf.TransportManagerConfig{
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
		KeyManagerConfig: pldconf.KeyManagerConfig{
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
	assert.NotNil(t, cm.PrivateTxManager())
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

	mockPrivateTxManager := componentsmocks.NewPrivateTxManager(t)
	mockPrivateTxManager.On("Start").Return(nil)
	mockPrivateTxManager.On("Stop").Return()

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
	cm.privateTxManager = mockPrivateTxManager
	cm.txManager = mockTxManager
	cm.groupManager = mockGroupManager
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
