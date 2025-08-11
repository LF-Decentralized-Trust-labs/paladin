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
	"sync"
	"sync/atomic"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/txmgr/metrics"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/blockindexer"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/ethclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/retry"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/cache"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
)

func NewTXManager(ctx context.Context, conf *pldconf.TxManagerConfig) components.TXManager {
	tm := &txManager{
		bgCtx:    ctx,
		conf:     conf,
		abiCache: cache.NewCache[pldtypes.Bytes32, *pldapi.StoredABI](&conf.ABI.Cache, &pldconf.TxManagerDefaults.ABI.Cache),
		txCache:  cache.NewCache[uuid.UUID, *components.ResolvedTransaction](&conf.Transactions.Cache, &pldconf.TxManagerDefaults.Transactions.Cache),
	}
	tm.receiptsInit()
	tm.blockchainEventsInit()
	tm.rpcEventStreams = newRPCEventStreams(tm)
	return tm
}

type txManager struct {
	bgCtx               context.Context
	conf                *pldconf.TxManagerConfig
	p                   persistence.Persistence
	localNodeName       string
	ethClientFactory    ethclient.EthClientFactory
	keyManager          components.KeyManager
	publicTxMgr         components.PublicTxManager
	privateTxMgr        components.PrivateTxManager
	domainMgr           components.DomainManager
	stateMgr            components.StateManager
	identityResolver    components.IdentityResolver
	blockIndexer        blockindexer.BlockIndexer
	rpcEventStreams     *rpcEventStreams
	txCache             cache.Cache[uuid.UUID, *components.ResolvedTransaction]
	abiCache            cache.Cache[pldtypes.Bytes32, *pldapi.StoredABI]
	rpcModule           *rpcserver.RPCModule
	debugRpcModule      *rpcserver.RPCModule
	lastStateUpdateTime atomic.Int64

	receiptsRetry                *retry.Retry
	receiptsReadPageSize         int
	receiptsStateGapCheckTime    time.Duration
	receiptListenersLoadPageSize int
	receiptListenerLock          sync.Mutex
	receiptListeners             map[string]*receiptListener

	blockchainEventListenerLock          sync.Mutex
	blockchainEventListeners             map[string]*blockchainEventListener
	blockchainEventListenersLoadPageSize int
	metrics                              metrics.TransactionManagerMetrics
}

func (tm *txManager) PreInit(c components.PreInitComponents) (*components.ManagerInitResult, error) {
	tm.metrics = metrics.InitMetrics(tm.bgCtx, c.MetricsManager().Registry())
	tm.buildRPCModule()
	return &components.ManagerInitResult{
		RPCModules:       []*rpcserver.RPCModule{tm.rpcModule, tm.debugRpcModule},
		PreCommitHandler: tm.blockIndexerPreCommit,
	}, nil
}

func (tm *txManager) PostInit(c components.AllComponents) error {
	tm.p = c.Persistence()
	tm.ethClientFactory = c.EthClientFactory()
	tm.keyManager = c.KeyManager()
	tm.publicTxMgr = c.PublicTxManager()
	tm.privateTxMgr = c.PrivateTxManager()
	tm.domainMgr = c.DomainManager()
	tm.stateMgr = c.StateManager()
	tm.identityResolver = c.IdentityResolver()
	tm.blockIndexer = c.BlockIndexer()
	tm.localNodeName = c.TransportManager().LocalNodeName()

	err := tm.loadReceiptListeners()
	return err
}

func (tm *txManager) Start() error {
	tm.startReceiptListeners()
	return nil
}

func (tm *txManager) Stop() {
	tm.rpcEventStreams.stop()
	tm.stopReceiptListeners()
	tm.stopBlockchainEventListeners()
}
