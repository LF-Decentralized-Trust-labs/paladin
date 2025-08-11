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
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/statemgr"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/blockindexermocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/ethclientmocks"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockComponents struct {
	db               sqlmock.Sqlmock
	c                *componentsmocks.AllComponents
	ethClient        *ethclientmocks.EthClient
	ethClientFactory *ethclientmocks.EthClientFactory
	stateStore       *componentsmocks.StateManager
	blockIndexer     *blockindexermocks.BlockIndexer
	keyManager       *componentsmocks.KeyManager
	txManager        *componentsmocks.TXManager
	privateTxManager *componentsmocks.PrivateTxManager
	transportMgr     *componentsmocks.TransportManager
}

func newTestDomainManager(t *testing.T, realDB bool, conf *pldconf.DomainManagerConfig, extraSetup ...func(mc *mockComponents)) (context.Context, *domainManager, *mockComponents, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	allComponents := componentsmocks.NewAllComponents(t)
	mc := &mockComponents{
		c:                allComponents,
		blockIndexer:     blockindexermocks.NewBlockIndexer(t),
		stateStore:       componentsmocks.NewStateManager(t),
		ethClientFactory: ethclientmocks.NewEthClientFactory(t),
		keyManager:       componentsmocks.NewKeyManager(t),
		txManager:        componentsmocks.NewTXManager(t),
		privateTxManager: componentsmocks.NewPrivateTxManager(t),
		transportMgr:     componentsmocks.NewTransportManager(t),
	}

	// Blockchain stuff is always mocked
	allComponents.On("EthClientFactory").Return(mc.ethClientFactory)
	mc.ethClientFactory.On("ChainID").Return(int64(12345)).Maybe()
	mc.ethClientFactory.On("HTTPClient").Return(mc.ethClient).Maybe()
	mc.ethClientFactory.On("WSClient").Return(mc.ethClient).Maybe()
	allComponents.On("BlockIndexer").Return(mc.blockIndexer)
	mc.keyManager.On("AddInMemorySigner", "domain", mock.Anything).Return().Maybe()
	allComponents.On("KeyManager").Return(mc.keyManager)
	allComponents.On("TxManager").Return(mc.txManager)
	allComponents.On("PrivateTxManager").Return(mc.privateTxManager)
	allComponents.On("TransportManager").Return(mc.transportMgr)
	mc.transportMgr.On("LocalNodeName").Return("node1").Maybe()

	var p persistence.Persistence
	var err error
	var realStateManager components.StateManager
	var pDone func()
	if realDB {
		p, pDone, err = persistence.NewUnitTestPersistence(ctx, "domainmgr")
		require.NoError(t, err)
		realStateManager = statemgr.NewStateManager(ctx, &pldconf.StateStoreConfig{}, p)
		allComponents.On("StateManager").Return(realStateManager)
		_, _ = realStateManager.PreInit(allComponents)
	} else {
		mp, err := mockpersistence.NewSQLMockProvider()
		require.NoError(t, err)
		p = mp.P
		mc.db = mp.Mock
		pDone = func() {
			require.NoError(t, mp.Mock.ExpectationsWereMet())
		}
		allComponents.On("StateManager").Return(mc.stateStore)
	}
	allComponents.On("Persistence").Return(p)

	for _, fn := range extraSetup {
		fn(mc)
	}

	dm := NewDomainManager(ctx, conf)

	_, err = dm.PreInit(allComponents)
	require.NoError(t, err)
	err = dm.PostInit(allComponents)
	require.NoError(t, err)

	if realDB {
		allComponents.On("DomainManager").Return(dm)
		_ = realStateManager.PostInit(allComponents)
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
					Type:    string(pldtypes.LibraryTypeCShared),
					Library: "some/where",
				},
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	assert.Equal(t, map[string]*pldconf.PluginConfig{
		"test1": {
			Type:    string(pldtypes.LibraryTypeCShared),
			Library: "some/where",
		},
	}, dm.ConfiguredDomains())
}

func TestDomainRegisteredNotFound(t *testing.T) {
	_, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: pldtypes.RandHex(20),
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
					Type:    string(pldtypes.LibraryTypeCShared),
					Library: "some/where",
				},
			},
		},
	}

	mc := &mockComponents{
		blockIndexer:     blockindexermocks.NewBlockIndexer(t),
		stateStore:       componentsmocks.NewStateManager(t),
		ethClientFactory: ethclientmocks.NewEthClientFactory(t),
		keyManager:       componentsmocks.NewKeyManager(t),
		txManager:        componentsmocks.NewTXManager(t),
		privateTxManager: componentsmocks.NewPrivateTxManager(t),
		transportMgr:     componentsmocks.NewTransportManager(t),
	}
	componentsmocks := componentsmocks.NewAllComponents(t)
	componentsmocks.On("EthClientFactory").Return(mc.ethClientFactory)
	mc.ethClientFactory.On("ChainID").Return(int64(12345)).Maybe()
	mc.ethClientFactory.On("HTTPClient").Return(mc.ethClient).Maybe()
	mc.ethClientFactory.On("WSClient").Return(mc.ethClient).Maybe()
	componentsmocks.On("BlockIndexer").Return(mc.blockIndexer)
	mc.keyManager.On("AddInMemorySigner", "domain", mock.Anything).Return().Maybe()
	componentsmocks.On("KeyManager").Return(mc.keyManager)
	componentsmocks.On("TxManager").Return(mc.txManager)
	componentsmocks.On("PrivateTxManager").Return(mc.privateTxManager)
	componentsmocks.On("TransportManager").Return(mc.transportMgr)

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	componentsmocks.On("StateManager").Return(mc.stateStore)
	componentsmocks.On("Persistence").Return(mp.P)
	dm := NewDomainManager(context.Background(), config)
	_, err = dm.PreInit(componentsmocks)
	require.NoError(t, err)
	err = dm.PostInit(componentsmocks)
	assert.Regexp(t, "PD011606", err)
}

func TestGetDomainNotFound(t *testing.T) {
	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	_, err := dm.GetDomainByName(ctx, "wrong")
	assert.Regexp(t, "PD011600", err)

	_, err = dm.getDomainByAddress(ctx, pldtypes.MustEthAddress(pldtypes.RandHex(20)))
	assert.Regexp(t, "PD011600", err)

	dc := dm.getDomainByAddressOrNil(pldtypes.MustEthAddress(pldtypes.RandHex(20)))
	assert.Nil(t, dc)
}

func TestGetDomainNotInit(t *testing.T) {
	td, done := newTestDomain(t, false, &prototk.DomainConfig{
		AbiStateSchemasJson: []string{`{!!! bad`},
	})
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
	domainAddr := pldtypes.RandAddress()
	contractAddr := pldtypes.RandAddress()

	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"domain1": {
				RegistryAddress: pldtypes.RandHex(20),
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
		assert.Regexp(t, "PD011654", err)
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
				RegistryAddress: pldtypes.RandHex(20),
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
				RegistryAddress: pldtypes.RandHex(20),
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
				RegistryAddress: pldtypes.RandHex(20),
			},
		},
	})
	defer done()

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	err := dm.ExecAndWaitTransaction(cancelled, uuid.New(), func() error { return nil })
	assert.Regexp(t, "PD020100", err)
}
