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

package main

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"
	"github.com/kaleido-io/paladin/kata/pkg/persistence"
	"github.com/kaleido-io/paladin/kata/pkg/persistence/mockpersistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gopkg.in/yaml.v3"
)

type mockComponents struct {
	db                   sqlmock.Sqlmock
	ethClient            *componentmocks.EthClient
	ethClientFactory     *componentmocks.EthClientFactory
	stateStore           *componentmocks.StateStore
	domainStateInterface *componentmocks.DomainStateInterface
	blockIndexer         *componentmocks.BlockIndexer
}

func newTestDomainManager(t *testing.T, realDB bool, conf *DomainManagerConfig, extraSetup ...func(mc *mockComponents)) (context.Context, *domainManager, *mockComponents, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	mc := &mockComponents{
		blockIndexer:         componentmocks.NewBlockIndexer(t),
		stateStore:           componentmocks.NewStateStore(t),
		domainStateInterface: componentmocks.NewDomainStateInterface(t),
		ethClientFactory:     componentmocks.NewEthClientFactory(t),
	}

	// Blockchain stuff is always mocked
	preMocks := componentmocks.NewPreInitComponents(t)
	preMocks.On("EthClientFactory").Return(mc.ethClientFactory)
	mc.ethClientFactory.On("ChainID").Return(int64(12345))
	mc.ethClientFactory.On("HTTPClient").Return(mc.ethClient).Maybe()
	mc.ethClientFactory.On("WSClient").Return(mc.ethClient).Maybe()
	postMocks := componentmocks.NewPostInitComponents(t)
	postMocks.On("BlockIndexer").Return(mc.blockIndexer)

	var p persistence.Persistence
	var err error
	var pDone func()
	if realDB {
		p, pDone, err = persistence.NewUnitTestPersistence(ctx)
		assert.NoError(t, err)
		realStateStore := statestore.NewStateStore(ctx, &statestore.Config{}, p)
		preMocks.On("StateStore").Return(realStateStore)
	} else {
		mp, err := mockpersistence.NewSQLMockProvider()
		assert.NoError(t, err)
		p = mp.P
		mc.db = mp.Mock
		pDone = func() {
			assert.NoError(t, mp.Mock.ExpectationsWereMet())
		}
		preMocks.On("StateStore").Return(mc.stateStore)
		mridc := mc.stateStore.On("RunInDomainContext", mock.Anything, mock.Anything)
		mridc.Run(func(args mock.Arguments) {
			mridc.Return((args[1].(statestore.DomainContextFunction))(
				ctx, mc.domainStateInterface,
			))
		}).Maybe()
		mridcf := mc.stateStore.On("RunInDomainContextFlush", mock.Anything, mock.Anything)
		mridcf.Run(func(args mock.Arguments) {
			mridcf.Return((args[1].(statestore.DomainContextFunction))(
				ctx, mc.domainStateInterface,
			))
		}).Maybe()
	}
	preMocks.On("Persistence").Return(p)

	for _, fn := range extraSetup {
		fn(mc)
	}

	dm := NewDomainManager(ctx, conf)
	initInstructions, err := dm.PreInit(preMocks)
	assert.NoError(t, err)
	assert.Len(t, initInstructions.EventStreams, 1)
	err = dm.PostInit(postMocks)
	assert.NoError(t, err)

	err = dm.Start()
	assert.NoError(t, err)

	return ctx, dm.(*domainManager), mc, func() {
		cancelCtx()
		pDone()
		dm.Stop()
	}
}

func yamlNode(t *testing.T, s string) (n yaml.Node) {
	err := yaml.Unmarshal([]byte(s), &n)
	assert.NoError(t, err)
	return
}
