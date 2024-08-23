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

// import (
// 	"context"
// 	"errors"
// 	"testing"

// 	"github.com/google/uuid"
// 	"github.com/hyperledger/firefly-signer/pkg/abi"
// 	"github.com/kaleido-io/paladin/kata/internal/components"
// 	"github.com/kaleido-io/paladin/kata/internal/msgs"
// 	"github.com/kaleido-io/paladin/kata/internal/rpcclient"
// 	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
// 	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"
// 	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
// 	"github.com/kaleido-io/paladin/kata/pkg/persistence"
// 	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
// 	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/mock"
// )

// func TestInitOK(t *testing.T) {

// 	// We build a config that allows us to get through init successfully, as should be possible
// 	// (anything that can't do this should have a separate Start() phase).
// 	testConfig := &Config{
// 		DB: persistence.Config{
// 			Type: "sqlite",
// 			SQLite: persistence.SQLiteConfig{
// 				SQLDBConfig: persistence.SQLDBConfig{
// 					URI:           ":memory:",
// 					AutoMigrate:   confutil.P(true),
// 					MigrationsDir: "../../db/migrations/sqlite",
// 				},
// 			},
// 		},
// 		Blockchain: ethclient.Config{
// 			HTTP: rpcclient.HTTPConfig{
// 				URL: "http://localhost:8545", // we won't actually connect this test, just check the config
// 			},
// 		},
// 		Signer: api.Config{
// 			KeyDerivation: api.KeyDerivationConfig{
// 				Type: api.KeyDerivationTypeBIP32,
// 			},
// 			KeyStore: api.StoreConfig{
// 				Type: "static",
// 				Static: api.StaticKeyStorageConfig{
// 					Keys: map[string]api.StaticKeyEntryConfig{
// 						"seed": {
// 							Encoding: "hex",
// 							Inline:   "dfaf68b749c53672e5fa8e0b41514f9efd033ba6aa3add3b8b07f92e66f0e64a",
// 						},
// 					},
// 				},
// 			},
// 		},
// 		RPCServer: rpcserver.Config{
// 			HTTP: rpcserver.HTTPEndpointConfig{Disabled: true},
// 			WS:   rpcserver.WSEndpointConfig{Disabled: true},
// 		},
// 	}

// 	mockEngine := componentmocks.NewEngine(t)
// 	mockEngine.On("EngineName").Return("utengine")
// 	mockEngine.On("Init", mock.Anything).Return(&components.ManagerInitResult{}, nil)
// 	cm := NewComponentManager(context.Background(), uuid.New(), testConfig, mockEngine).(*componentManager)
// 	err := cm.Init()
// 	assert.NoError(t, err)

// 	assert.NotNil(t, cm.KeyManager())
// 	assert.NotNil(t, cm.EthClientFactory())
// 	assert.NotNil(t, cm.Persistence())
// 	assert.NotNil(t, cm.StateStore())
// 	assert.NotNil(t, cm.RPCServer())
// 	assert.NotNil(t, cm.BlockIndexer())
// 	assert.NotNil(t, cm.DomainManager())
// 	assert.NotNil(t, cm.DomainRegistration())
// 	assert.NotNil(t, cm.PluginController())
// 	assert.NotNil(t, cm.Engine())

// 	cm.Stop()

// }

// func TestStartOK(t *testing.T) {

// 	mockEthClientFactory := componentmocks.NewEthClientFactory(t)
// 	mockEthClientFactory.On("Start").Return(nil)
// 	mockEthClientFactory.On("Stop").Return()

// 	mockBlockIndexer := componentmocks.NewBlockIndexer(t)
// 	mockBlockIndexer.On("Start", mock.AnythingOfType("*blockindexer.InternalEventStream")).Return(nil)
// 	mockBlockIndexer.On("GetBlockListenerHeight", mock.Anything).Return(uint64(12345), nil)
// 	mockBlockIndexer.On("Stop").Return()

// 	mockPluginController := componentmocks.NewPluginController(t)
// 	mockPluginController.On("Start").Return(nil)
// 	mockPluginController.On("WaitForInit", mock.Anything).Return(nil)
// 	mockPluginController.On("Stop").Return()

// 	mockDomainManager := componentmocks.NewDomainManager(t)
// 	mockDomainManager.On("Start").Return(nil)
// 	mockDomainManager.On("Stop").Return()

// 	mockStateStore := componentmocks.NewStateStore(t)
// 	mockStateStore.On("RPCModule").Return(rpcserver.NewRPCModule("utss"))

// 	mockRPCServer := componentmocks.NewRPCServer(t)
// 	mockRPCServer.On("Start").Return(nil)
// 	mockRPCServer.On("Register", mock.AnythingOfType("*rpcserver.RPCModule")).Return()
// 	mockRPCServer.On("Stop").Return()

// 	mockEngine := componentmocks.NewEngine(t)
// 	mockEngine.On("Start").Return(nil)
// 	mockEngine.On("Stop").Return()

// 	cm := NewComponentManager(context.Background(), uuid.New(), &Config{}, mockEngine).(*componentManager)
// 	cm.ethClientFactory = mockEthClientFactory
// 	cm.initResults = map[string]*components.ManagerInitResult{
// 		"utengine": {
// 			EventStreams: []*components.ManagerEventStream{
// 				{ABI: abi.ABI{}},
// 			},
// 			RPCModules: []*rpcserver.RPCModule{
// 				rpcserver.NewRPCModule("ut"),
// 			},
// 		},
// 	}
// 	cm.blockIndexer = mockBlockIndexer
// 	cm.pluginController = mockPluginController
// 	cm.domainManager = mockDomainManager
// 	cm.stateStore = mockStateStore
// 	cm.rpcServer = mockRPCServer
// 	cm.engine = mockEngine

// 	err := cm.StartComponents()
// 	assert.NoError(t, err)
// 	err = cm.CompleteStart()
// 	assert.NoError(t, err)

// 	cm.Stop()
// 	assert.NoError(t, err)
// }

// func TestBuildInternalEventStreamsError(t *testing.T) {
// 	cm := NewComponentManager(context.Background(), uuid.New(), &Config{}, nil).(*componentManager)
// 	cm.initResults = map[string]*components.ManagerInitResult{
// 		"utengine": {
// 			EventStreams: []*components.ManagerEventStream{
// 				{ABI: abi.ABI{
// 					{Type: "event", Inputs: abi.ParameterArray{{Type: "wrong"}}},
// 				}},
// 			},
// 		},
// 	}

// 	_, err := cm.buildInternalEventStreams()
// 	assert.Regexp(t, "FF22025", err)

// }

// func TestErrorWrapping(t *testing.T) {
// 	cm := NewComponentManager(context.Background(), uuid.New(), &Config{}, nil).(*componentManager)

// 	mockKeyManager := componentmocks.NewKeyManager(t)
// 	mockEthClientFactory := componentmocks.NewEthClientFactory(t)

// 	assert.Regexp(t, "PD010000.*pop", cm.addIfOpened(mockKeyManager, errors.New("pop"), msgs.MsgComponentKeyManagerInitError))
// 	assert.Regexp(t, "PD010017.*pop", cm.addIfStarted(mockEthClientFactory, errors.New("pop"), msgs.MsgComponentEngineInitError))
// 	assert.Regexp(t, "PD010008.*pop", cm.wrapIfErr(errors.New("pop"), msgs.MsgComponentBlockIndexerInitError))

// }
