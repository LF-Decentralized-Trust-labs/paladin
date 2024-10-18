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

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"

	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/cache"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

func NewTXManager(ctx context.Context, conf *pldconf.TxManagerConfig) components.TXManager {
	return &txManager{
		abiCache: cache.NewCache[tktypes.Bytes32, *pldapi.StoredABI](&conf.ABI.Cache, &pldconf.TxManagerDefaults.ABI.Cache),
	}
}

type txManager struct {
	p                persistence.Persistence
	ethClientFactory ethclient.EthClientFactory
	keyManager       components.KeyManager
	publicTxMgr      components.PublicTxManager
	privateTxMgr     components.PrivateTxManager
	identityResolver components.IdentityResolver
	abiCache         cache.Cache[tktypes.Bytes32, *pldapi.StoredABI]
	rpcModule        *rpcserver.RPCModule
	debugRpcModule   *rpcserver.RPCModule
}

func (tm *txManager) PostInit(c components.AllComponents) error {
	tm.p = c.Persistence()
	tm.ethClientFactory = c.EthClientFactory()
	tm.keyManager = c.KeyManager()
	tm.publicTxMgr = c.PublicTxManager()
	tm.privateTxMgr = c.PrivateTxManager()
	tm.identityResolver = c.IdentityResolver()
	return nil
}

func (tm *txManager) PreInit(c components.PreInitComponents) (*components.ManagerInitResult, error) {
	tm.buildRPCModule()
	return &components.ManagerInitResult{
		RPCModules:       []*rpcserver.RPCModule{tm.rpcModule, tm.debugRpcModule},
		PreCommitHandler: tm.blockIndexerPreCommit,
	}, nil
}

func (tm *txManager) Start() error { return nil }

func (tm *txManager) Stop() {}
