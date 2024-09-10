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

package domainmgr

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/statestore"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/core/pkg/persistence/mockpersistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
	componentMocks := componentmocks.NewAllComponents(t)
	componentMocks.On("EthClientFactory").Return(mc.ethClientFactory)
	mc.ethClientFactory.On("ChainID").Return(int64(12345)).Maybe()
	mc.ethClientFactory.On("HTTPClient").Return(mc.ethClient).Maybe()
	mc.ethClientFactory.On("WSClient").Return(mc.ethClient).Maybe()
	componentMocks.On("BlockIndexer").Return(mc.blockIndexer)

	var p persistence.Persistence
	var err error
	var pDone func()
	if realDB {
		p, pDone, err = persistence.NewUnitTestPersistence(ctx)
		require.NoError(t, err)
		realStateStore := statestore.NewStateStore(ctx, &statestore.Config{}, p)
		componentMocks.On("StateStore").Return(realStateStore)
	} else {
		mp, err := mockpersistence.NewSQLMockProvider()
		require.NoError(t, err)
		p = mp.P
		mc.db = mp.Mock
		pDone = func() {
			require.NoError(t, mp.Mock.ExpectationsWereMet())
		}
		componentMocks.On("StateStore").Return(mc.stateStore)
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
	componentMocks.On("Persistence").Return(p)

	for _, fn := range extraSetup {
		fn(mc)
	}

	dm := NewDomainManager(ctx, conf)
	initInstructions, err := dm.PreInit(componentMocks)
	require.NoError(t, err)
	err = dm.PostInit(componentMocks)
	require.NoError(t, err)
	assert.Len(t, initInstructions.EventStreams, 1)

	err = dm.Start()
	require.NoError(t, err)

	return ctx, dm.(*domainManager), mc, func() {
		cancelCtx()
		pDone()
		dm.Stop()
	}
}

func TestConfiguredDomains(t *testing.T) {
	_, dm, _, done := newTestDomainManager(t, false, &DomainManagerConfig{
		Domains: map[string]*DomainConfig{
			"test1": {
				Plugin: components.PluginConfig{
					Type:    components.LibraryTypeCShared.Enum(),
					Library: "some/where",
				},
			},
		},
	})
	defer done()

	assert.Equal(t, map[string]*components.PluginConfig{
		"test1": {
			Type:    components.LibraryTypeCShared.Enum(),
			Library: "some/where",
		},
	}, dm.ConfiguredDomains())
}

func TestDomainRegisteredNotFound(t *testing.T) {
	_, dm, _, done := newTestDomainManager(t, false, &DomainManagerConfig{
		Domains: map[string]*DomainConfig{},
	})
	defer done()

	_, err := dm.DomainRegistered("unknown", uuid.New(), nil)
	assert.Regexp(t, "PD011600", err)
}

func TestGetDomainNotFound(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &DomainManagerConfig{
		Domains: map[string]*DomainConfig{},
	})
	defer done()

	_, err := dm.GetDomainByName(ctx, "wrong")
	assert.Regexp(t, "PD011600", err)

	_, err = dm.getDomainByAddress(ctx, tktypes.MustEthAddress(tktypes.RandHex(20)))
	assert.Regexp(t, "PD011600", err)
}

func TestMustParseLoaders(t *testing.T) {
	assert.Panics(t, func() {
		_ = mustParseEmbeddedBuildABI([]byte(`{!wrong`))
	})
	assert.Panics(t, func() {
		_ = mustParseEventSoliditySignature(abi.ABI{}, "nope")
	})
	assert.Panics(t, func() {
		_ = mustParseEventSignatureHash(abi.ABI{}, "nope")
	})
	badEvent := &abi.Entry{
		Type:   abi.Event,
		Name:   "broken",
		Inputs: abi.ParameterArray{{Type: "wrong"}},
	}
	assert.Panics(t, func() {
		_ = mustParseEventSoliditySignature(abi.ABI{badEvent}, "broken")
	})
	assert.Panics(t, func() {
		_ = mustParseEventSignatureHash(abi.ABI{badEvent}, "broken")
	})
}

func TestWaitForDeployQueryError(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &DomainManagerConfig{
		Domains: map[string]*DomainConfig{},
	}, func(mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	_, err := dm.WaitForDeploy(ctx, uuid.New())
	assert.Regexp(t, "pop", err)
}

func TestWaitForDeployDomainNotFound(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &DomainManagerConfig{
		Domains: map[string]*DomainConfig{},
	}, func(mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows([]string{}))
	})
	defer done()

	reqID := uuid.New()

	received := make(chan struct{})
	go func() {
		_, err := dm.WaitForDeploy(ctx, reqID)
		assert.Regexp(t, "PD011600", err)
		close(received)
	}()

	for dm.contractWaiter.InFlightCount() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	dm.contractWaiter.GetInflight(reqID).Complete(&PrivateSmartContract{})

	<-received

}

func TestWaitForDeployTimeout(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &DomainManagerConfig{
		Domains: map[string]*DomainConfig{},
	})
	defer done()

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err := dm.waitAndEnrich(cancelled, dm.contractWaiter.AddInflight(cancelled, uuid.New()))
	assert.Regexp(t, "PD020100", err)
}
