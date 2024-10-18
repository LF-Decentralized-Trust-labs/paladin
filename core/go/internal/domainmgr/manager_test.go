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
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/statemgr"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/mocks/ethclientmocks"

	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/core/pkg/persistence/mockpersistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockComponents struct {
	db               sqlmock.Sqlmock
	ethClient        *ethclientmocks.EthClient
	ethClientFactory *ethclientmocks.EthClientFactory
	stateStore       *componentmocks.StateManager
	blockIndexer     *componentmocks.BlockIndexer
	keyManager       *componentmocks.KeyManager
	txManager        *componentmocks.TXManager
	privateTxManager *componentmocks.PrivateTxManager
}

func newTestDomainManager(t *testing.T, realDB bool, conf *pldconf.DomainManagerConfig, extraSetup ...func(mc *mockComponents)) (context.Context, *domainManager, *mockComponents, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	mc := &mockComponents{
		blockIndexer:     componentmocks.NewBlockIndexer(t),
		stateStore:       componentmocks.NewStateManager(t),
		ethClientFactory: ethclientmocks.NewEthClientFactory(t),
		keyManager:       componentmocks.NewKeyManager(t),
		txManager:        componentmocks.NewTXManager(t),
		privateTxManager: componentmocks.NewPrivateTxManager(t),
	}

	// Blockchain stuff is always mocked
	componentMocks := componentmocks.NewAllComponents(t)
	componentMocks.On("EthClientFactory").Return(mc.ethClientFactory)
	mc.ethClientFactory.On("ChainID").Return(int64(12345)).Maybe()
	mc.ethClientFactory.On("HTTPClient").Return(mc.ethClient).Maybe()
	mc.ethClientFactory.On("WSClient").Return(mc.ethClient).Maybe()
	componentMocks.On("BlockIndexer").Return(mc.blockIndexer)
	mc.keyManager.On("AddInMemorySigner", "domain", mock.Anything).Return().Maybe()
	componentMocks.On("KeyManager").Return(mc.keyManager)
	componentMocks.On("TxManager").Return(mc.txManager)
	componentMocks.On("PrivateTxManager").Return(mc.privateTxManager)

	var p persistence.Persistence
	var err error
	var realStateManager components.StateManager
	var pDone func()
	if realDB {
		p, pDone, err = persistence.NewUnitTestPersistence(ctx)
		require.NoError(t, err)
		realStateManager = statemgr.NewStateManager(ctx, &pldconf.StateStoreConfig{}, p)
		componentMocks.On("StateManager").Return(realStateManager)
		_, _ = realStateManager.PreInit(componentMocks)
	} else {
		mp, err := mockpersistence.NewSQLMockProvider()
		require.NoError(t, err)
		p = mp.P
		mc.db = mp.Mock
		pDone = func() {
			require.NoError(t, mp.Mock.ExpectationsWereMet())
		}
		componentMocks.On("StateManager").Return(mc.stateStore)
	}
	componentMocks.On("Persistence").Return(p)

	for _, fn := range extraSetup {
		fn(mc)
	}

	dm := NewDomainManager(ctx, conf)
	_, err = dm.PreInit(componentMocks)
	require.NoError(t, err)
	err = dm.PostInit(componentMocks)
	require.NoError(t, err)

	if realDB {
		componentMocks.On("DomainManager").Return(dm)
		_ = realStateManager.PostInit(componentMocks)
		_ = realStateManager.Start()
	}

	err = dm.Start()
	require.NoError(t, err)

	return ctx, dm.(*domainManager), mc, func() {
		cancelCtx()
		pDone()
		dm.Stop()
	}
}

func TestConfiguredDomains(t *testing.T) {
	_, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				Plugin: pldconf.PluginConfig{
					Type:    string(tktypes.LibraryTypeCShared),
					Library: "some/where",
				},
				RegistryAddress: tktypes.RandHex(20),
			},
		},
	})
	defer done()

	assert.Equal(t, map[string]*pldconf.PluginConfig{
		"test1": {
			Type:    string(tktypes.LibraryTypeCShared),
			Library: "some/where",
		},
	}, dm.ConfiguredDomains())
}

func TestDomainRegisteredNotFound(t *testing.T) {
	_, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: tktypes.RandHex(20),
			},
		},
	})
	defer done()

	_, err := dm.DomainRegistered("unknown", nil)
	assert.Regexp(t, "PD011600", err)
}

func TestDomainMissingRegistryAddress(t *testing.T) {
	config := &pldconf.DomainManagerConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				Plugin: pldconf.PluginConfig{
					Type:    string(tktypes.LibraryTypeCShared),
					Library: "some/where",
				},
			},
		},
	}

	mc := &mockComponents{
		blockIndexer:     componentmocks.NewBlockIndexer(t),
		stateStore:       componentmocks.NewStateManager(t),
		ethClientFactory: ethclientmocks.NewEthClientFactory(t),
		keyManager:       componentmocks.NewKeyManager(t),
		txManager:        componentmocks.NewTXManager(t),
		privateTxManager: componentmocks.NewPrivateTxManager(t),
	}
	componentMocks := componentmocks.NewAllComponents(t)
	componentMocks.On("EthClientFactory").Return(mc.ethClientFactory)
	mc.ethClientFactory.On("ChainID").Return(int64(12345)).Maybe()
	mc.ethClientFactory.On("HTTPClient").Return(mc.ethClient).Maybe()
	mc.ethClientFactory.On("WSClient").Return(mc.ethClient).Maybe()
	componentMocks.On("BlockIndexer").Return(mc.blockIndexer)
	mc.keyManager.On("AddInMemorySigner", "domain", mock.Anything).Return().Maybe()
	componentMocks.On("KeyManager").Return(mc.keyManager)
	componentMocks.On("TxManager").Return(mc.txManager)
	componentMocks.On("PrivateTxManager").Return(mc.privateTxManager)

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	componentMocks.On("StateManager").Return(mc.stateStore)
	componentMocks.On("Persistence").Return(mp.P)
	dm := NewDomainManager(context.Background(), config)
	_, err = dm.PreInit(componentMocks)
	require.NoError(t, err)
	err = dm.PostInit(componentMocks)
	assert.Regexp(t, "PD011606", err)
}

func TestGetDomainNotFound(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: tktypes.RandHex(20),
			},
		},
	})
	defer done()

	_, err := dm.GetDomainByName(ctx, "wrong")
	assert.Regexp(t, "PD011600", err)

	_, err = dm.getDomainByAddress(ctx, tktypes.MustEthAddress(tktypes.RandHex(20)), false)
	assert.Regexp(t, "PD011600", err)

	dc, err := dm.getDomainByAddress(ctx, tktypes.MustEthAddress(tktypes.RandHex(20)), true)
	assert.NoError(t, err)
	assert.Nil(t, dc)
}

func TestGetDomainNotInit(t *testing.T) {
	td, done := newTestDomain(t, false, &prototk.DomainConfig{})
	defer done()

	_, err := td.dm.GetDomainByName(td.ctx, td.d.name)
	assert.Regexp(t, "PD011601", err)
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

func TestWaitForDeployDomainNotFound(t *testing.T) {
	reqID := uuid.New()
	domainAddr := tktypes.RandAddress()
	contractAddr := tktypes.RandAddress()

	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: tktypes.RandHex(20),
			},
		},
	}, func(mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows([]string{
			"deploy_tx", "domain_address", "address", "config_bytes",
		}).AddRow(
			reqID, domainAddr, contractAddr, "",
		))
	})
	defer done()

	received := make(chan struct{})
	go func() {
		_, err := dm.ExecDeployAndWait(ctx, reqID, func() error {
			// We simulate this on the main test routine below
			return nil
		})
		assert.Regexp(t, "PD011609", err)
		close(received)
	}()

	for dm.privateTxWaiter.InFlightCount() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	dm.privateTxWaiter.GetInflight(reqID).Complete(&components.ReceiptInput{
		ContractAddress: contractAddr,
	})

	<-received

}

func TestWaitForDeployNotADeploy(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: tktypes.RandHex(20),
			},
		},
	})
	defer done()

	reqID := uuid.New()

	received := make(chan struct{})
	go func() {
		_, err := dm.ExecDeployAndWait(ctx, reqID, func() error {
			// We simulate this on the main test routine below
			return nil
		})
		assert.Regexp(t, "PD011648", err)
		close(received)
	}()

	for dm.privateTxWaiter.InFlightCount() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	dm.privateTxWaiter.GetInflight(reqID).Complete(&components.ReceiptInput{
		ContractAddress: nil, // we complete without a contract address
	})

	<-received

}

func TestWaitForDeployTimeout(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: tktypes.RandHex(20),
			},
		},
	})
	defer done()

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err := dm.waitForDeploy(cancelled, dm.privateTxWaiter.AddInflight(cancelled, uuid.New()))
	assert.Regexp(t, "PD020100", err)
}

func TestWaitForTransactionTimeout(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: tktypes.RandHex(20),
			},
		},
	})
	defer done()

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	err := dm.ExecAndWaitTransaction(cancelled, uuid.New(), func() error { return nil })
	assert.Regexp(t, "PD020100", err)
}
