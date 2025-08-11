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

package txmgr

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/metrics"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/blockindexermocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/ethclientmocks"
	"github.com/stretchr/testify/assert"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence/mockpersistence"
	"github.com/stretchr/testify/require"
)

type mockComponents struct {
	c                *componentsmocks.AllComponents
	db               sqlmock.Sqlmock
	ethClientFactory *ethclientmocks.EthClientFactory
	domainManager    *componentsmocks.DomainManager
	blockIndexer     *blockindexermocks.BlockIndexer
	keyManager       *componentsmocks.KeyManager
	publicTxMgr      *componentsmocks.PublicTxManager
	privateTxMgr     *componentsmocks.PrivateTxManager
	stateMgr         *componentsmocks.StateManager
	identityResolver *componentsmocks.IdentityResolver
	transportManager *componentsmocks.TransportManager
}

func newTestTransactionManager(t *testing.T, realDB bool, init ...func(conf *pldconf.TxManagerConfig, mc *mockComponents)) (context.Context, *txManager, func()) {

	// log.SetLevel("debug")
	ctx := context.Background()

	conf := &pldconf.TxManagerConfig{
		ReceiptListeners: pldconf.ReceiptListeners{
			StateGapCheckInterval: confutil.P("100ms"),
		},
	}
	mc := &mockComponents{
		c:                componentsmocks.NewAllComponents(t),
		blockIndexer:     blockindexermocks.NewBlockIndexer(t),
		ethClientFactory: ethclientmocks.NewEthClientFactory(t),
		keyManager:       componentsmocks.NewKeyManager(t),
		domainManager:    componentsmocks.NewDomainManager(t),
		publicTxMgr:      componentsmocks.NewPublicTxManager(t),
		privateTxMgr:     componentsmocks.NewPrivateTxManager(t),
		stateMgr:         componentsmocks.NewStateManager(t),
		identityResolver: componentsmocks.NewIdentityResolver(t),
		transportManager: componentsmocks.NewTransportManager(t),
	}

	txm := NewTXManager(ctx, conf).(*txManager)
	mm := metrics.NewMetricsManager(ctx)

	componentsmocks := mc.c
	componentsmocks.On("TxManager").Return(txm).Maybe()
	componentsmocks.On("BlockIndexer").Return(mc.blockIndexer).Maybe()
	componentsmocks.On("DomainManager").Return(mc.domainManager).Maybe()
	componentsmocks.On("KeyManager").Return(mc.keyManager).Maybe()
	componentsmocks.On("PublicTxManager").Return(mc.publicTxMgr).Maybe()
	componentsmocks.On("PrivateTxManager").Return(mc.privateTxMgr).Maybe()
	componentsmocks.On("StateManager").Return(mc.stateMgr).Maybe()
	componentsmocks.On("IdentityResolver").Return(mc.identityResolver).Maybe()
	componentsmocks.On("EthClientFactory").Return(mc.ethClientFactory).Maybe()
	componentsmocks.On("TransportManager").Return(mc.transportManager).Maybe()
	componentsmocks.On("MetricsManager").Return(mm).Maybe()
	mc.transportManager.On("LocalNodeName").Return("node1").Maybe()
	var p persistence.Persistence
	var err error
	var pDone func()
	if realDB {
		p, pDone, err = persistence.NewUnitTestPersistence(ctx, "txmgr")
		require.NoError(t, err)
	} else {
		mp, err := mockpersistence.NewSQLMockProvider()
		require.NoError(t, err)
		p = mp.P
		mc.db = mp.Mock
		pDone = func() {
			require.NoError(t, mp.Mock.ExpectationsWereMet())
		}
	}
	componentsmocks.On("Persistence").Return(p)

	for _, fn := range init {
		fn(conf, mc)
	}

	ic, err := txm.PreInit(componentsmocks)
	require.NoError(t, err)
	assert.Equal(t, txm.rpcModule, ic.RPCModules[0])

	err = txm.PostInit(componentsmocks)
	require.NoError(t, err)

	err = txm.Start()
	require.NoError(t, err)

	return ctx, txm, func() {
		pDone()
		txm.Stop()
	}

}
