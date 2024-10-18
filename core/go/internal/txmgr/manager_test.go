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
	"github.com/alecthomas/assert/v2"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/mocks/ethclientmocks"

	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/core/pkg/persistence/mockpersistence"
	"github.com/stretchr/testify/require"
)

type mockComponents struct {
	db               sqlmock.Sqlmock
	ethClientFactory *ethclientmocks.EthClientFactory
	domainManager    *componentmocks.DomainManager
	blockIndexer     *componentmocks.BlockIndexer
	keyManager       *componentmocks.KeyManager
	publicTxMgr      *componentmocks.PublicTxManager
	privateTxMgr     *componentmocks.PrivateTxManager
	identityResolver *componentmocks.IdentityResolver
}

func newTestTransactionManager(t *testing.T, realDB bool, init ...func(conf *pldconf.TxManagerConfig, mc *mockComponents)) (context.Context, *txManager, func()) {

	// log.SetLevel("debug")
	ctx := context.Background()

	conf := &pldconf.TxManagerConfig{}
	mc := &mockComponents{
		blockIndexer:     componentmocks.NewBlockIndexer(t),
		ethClientFactory: ethclientmocks.NewEthClientFactory(t),
		keyManager:       componentmocks.NewKeyManager(t),
		domainManager:    componentmocks.NewDomainManager(t),
		publicTxMgr:      componentmocks.NewPublicTxManager(t),
		privateTxMgr:     componentmocks.NewPrivateTxManager(t),
		identityResolver: componentmocks.NewIdentityResolver(t),
	}

	componentMocks := componentmocks.NewAllComponents(t)
	componentMocks.On("BlockIndexer").Return(mc.blockIndexer).Maybe()
	componentMocks.On("DomainManager").Return(mc.domainManager).Maybe()
	componentMocks.On("KeyManager").Return(mc.keyManager).Maybe()
	componentMocks.On("PublicTxManager").Return(mc.publicTxMgr).Maybe()
	componentMocks.On("PrivateTxManager").Return(mc.privateTxMgr).Maybe()
	componentMocks.On("IdentityResolver").Return(mc.identityResolver).Maybe()
	componentMocks.On("EthClientFactory").Return(mc.ethClientFactory).Maybe()

	var p persistence.Persistence
	var err error
	var pDone func()
	if realDB {
		p, pDone, err = persistence.NewUnitTestPersistence(ctx)
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
	componentMocks.On("Persistence").Return(p)

	for _, fn := range init {
		fn(conf, mc)
	}

	txm := NewTXManager(ctx, conf).(*txManager)

	ic, err := txm.PreInit(componentMocks)
	require.NoError(t, err)
	assert.Equal(t, txm.rpcModule, ic.RPCModules[0])

	err = txm.PostInit(componentMocks)
	require.NoError(t, err)

	err = txm.Start()
	require.NoError(t, err)

	return ctx, txm, func() {
		pDone()
		txm.Stop()
	}

}
