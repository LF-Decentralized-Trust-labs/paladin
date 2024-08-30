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

// import (
// 	"context"
// 	"testing"

// 	"github.com/google/uuid"
// 	"github.com/kaleido-io/paladin/kata/internal/plugins"
// 	"github.com/stretchr/testify/assert"
// 	"gopkg.in/yaml.v3"
// )

// type mockComponents struct {
// }

// func newTestTransportManager(t *testing.T, realDB bool, conf *TransportManagerConfig, extraSetup ...func(mc *mockComponents)) (context.Context, *transportManager, *mockComponents, func()) {
// 	ctx, cancelCtx := context.WithCancel(context.Background())

// 	mc := &mockComponents{}

// 	// Blockchain stuff is always mocked
// 	// preMocks := componentmocks.NewPreInitComponents(t)
// 	// preMocks.On("EthClientFactory").Return(mc.ethClientFactory)
// 	// mc.ethClientFactory.On("ChainID").Return(int64(12345)).Maybe()
// 	// mc.ethClientFactory.On("HTTPClient").Return(mc.ethClient).Maybe()
// 	// mc.ethClientFactory.On("WSClient").Return(mc.ethClient).Maybe()
// 	// preMocks.On("BlockIndexer").Return(mc.blockIndexer)

// 	// var p persistence.Persistence
// 	// var err error
// 	// var pDone func()
// 	// if realDB {
// 	// 	p, pDone, err = persistence.NewUnitTestPersistence(ctx)
// 	// 	assert.NoError(t, err)
// 	// 	realStateStore := statestore.NewStateStore(ctx, &statestore.Config{}, p)
// 	// 	preMocks.On("StateStore").Return(realStateStore)
// 	// } else {
// 	// 	mp, err := mockpersistence.NewSQLMockProvider()
// 	// 	assert.NoError(t, err)
// 	// 	p = mp.P
// 	// 	mc.db = mp.Mock
// 	// 	pDone = func() {
// 	// 		assert.NoError(t, mp.Mock.ExpectationsWereMet())
// 	// 	}
// 	// 	preMocks.On("StateStore").Return(mc.stateStore)
// 	// 	mridc := mc.stateStore.On("RunInTransportContext", mock.Anything, mock.Anything)
// 	// 	mridc.Run(func(args mock.Arguments) {
// 	// 		mridc.Return((args[1].(statestore.TransportContextFunction))(
// 	// 			ctx, mc.transportStateInterface,
// 	// 		))
// 	// 	}).Maybe()
// 	// 	mridcf := mc.stateStore.On("RunInTransportContextFlush", mock.Anything, mock.Anything)
// 	// 	mridcf.Run(func(args mock.Arguments) {
// 	// 		mridcf.Return((args[1].(statestore.TransportContextFunction))(
// 	// 			ctx, mc.transportStateInterface,
// 	// 		))
// 	// 	}).Maybe()
// 	// }
// 	// preMocks.On("Persistence").Return(p)

// 	// for _, fn := range extraSetup {
// 	// 	fn(mc)
// 	// }

// 	dm := NewTransportManager(ctx, conf)
// 	initInstructions, err := dm.Init(nil)
// 	assert.NoError(t, err)
// 	assert.Len(t, initInstructions.EventStreams, 1)

// 	err = dm.Start()
// 	assert.NoError(t, err)

// 	return ctx, dm.(*transportManager), mc, func() {
// 		cancelCtx()
// 		// pDone()
// 		dm.Stop()
// 	}
// }

// func yamlNode(t *testing.T, s string) (n yaml.Node) {
// 	err := yaml.Unmarshal([]byte(s), &n)
// 	assert.NoError(t, err)
// 	return
// }

// func TestConfiguredTransports(t *testing.T) {
// 	_, dm, _, done := newTestTransportManager(t, false, &TransportManagerConfig{
// 		Transports: map[string]*TransportConfig{
// 			"test1": {
// 				Plugin: plugins.PluginConfig{
// 					Type:     plugins.LibraryTypeCShared.Enum(),
// 					Location: "some/where",
// 				},
// 			},
// 		},
// 	})
// 	defer done()

// 	assert.Equal(t, map[string]*plugins.PluginConfig{
// 		"test1": {
// 			Type:     plugins.LibraryTypeCShared.Enum(),
// 			Location: "some/where",
// 		},
// 	}, dm.ConfiguredTransports())
// }

// func TestTransportRegisteredNotFound(t *testing.T) {
// 	_, dm, _, done := newTestTransportManager(t, false, &TransportManagerConfig{
// 		Transports: map[string]*TransportConfig{},
// 	})
// 	defer done()

// 	_, err := dm.TransportRegistered("unknown", uuid.New(), nil)
// 	assert.Regexp(t, "PD011600", err)
// }

// func TestGetTransportNotFound(t *testing.T) {
// 	ctx, dm, _, done := newTestTransportManager(t, false, &TransportManagerConfig{
// 		Transports: map[string]*TransportConfig{},
// 	})
// 	defer done()

// 	_, err := dm.GetTransportByName(ctx, "wrong")
// 	assert.Regexp(t, "PD011600", err)
// }
