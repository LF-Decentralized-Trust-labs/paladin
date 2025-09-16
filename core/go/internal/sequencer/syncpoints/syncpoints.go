// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package syncpoints

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/flushwriter"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"gorm.io/gorm"
)

var WriterConfigDefaults = pldconf.FlushWriterConfig{
	WorkerCount:  confutil.P(10),
	BatchTimeout: confutil.P("25ms"),
	BatchMaxSize: confutil.P(100),
}

type PublicTransactionsSubmit func(tx *gorm.DB) (publicTxID []string, err error)

// SyncPoints is the interface for all private transaction manager's integration with persistent resources
// this includes writing to the database tables that private transaction manager owns as well as
// calling syncpoint APIs on other components like the TxManager and PublicTxManager
// All of the persistence here is offloaded to worker threads for performance reasons
type SyncPoints interface {
	Start()

	// PersistDispatchSequence takes a DispatchBatch and for each sequence in the batch, it integrates
	// with the PublicTxManager to record the transactions in its persistence store and also writes the dispatch
	// to the PrivateTxnManager's persistence store in the same database transaction
	// Although the actual persistence is offloaded to the flushwriter, this method is synchronous and will block until the
	// dispatch sequence is written to the database
	PersistDispatchBatch(dCtx components.DomainContext, contractAddress pldtypes.EthAddress, dispatchBatch *DispatchBatch, stateDistributions []*components.StateDistribution, preparedTxnDistributions []*components.PreparedTransactionWithRefs) error

	// Deploy is a special case of dispatch batch, where there are no private states, so no domain context is required
	PersistDeployDispatchBatch(ctx context.Context, dispatchBatch *DispatchBatch) error

	// QueueTransactionFinalize integrates with TxManager to mark a transaction as finalized with the given formatter revert reason
	// this is an async operation so it can safely be called from the sequencer event loop thread
	// the onCommit and onRollback callbacks are called, on a separate goroutine when the transaction is committed or rolled back
	QueueTransactionFinalize(ctx context.Context, domain string, contractAddress pldtypes.EthAddress, originator string, transactionID uuid.UUID, failureMessage string, onCommit func(context.Context), onRollback func(context.Context, error))

	// This is a recursive callback between syncpoints when flushing receipts, and FinalizeTransactions on txMgr
	WriteOrDistributeReceipts(ctx context.Context, dbTX persistence.DBTX, receipts []*components.ReceiptInputWithOriginator) error

	Close()
}

type syncPoints struct {
	started      bool
	writer       flushwriter.Writer[*syncPointOperation, *noResult]
	txMgr        components.TXManager
	pubTxMgr     components.PublicTxManager
	transportMgr components.TransportManager
}

func NewSyncPoints(ctx context.Context, conf *pldconf.FlushWriterConfig, p persistence.Persistence, txMgr components.TXManager, pubTxMgr components.PublicTxManager, transportMgr components.TransportManager) SyncPoints {
	s := &syncPoints{
		txMgr:        txMgr,
		pubTxMgr:     pubTxMgr,
		transportMgr: transportMgr,
	}
	s.writer = flushwriter.NewWriter(ctx, s.runBatch, p, conf, &WriterConfigDefaults)
	return s
}

func (s *syncPoints) Start() {
	if !s.started {
		s.writer.Start()
	}
}

func (s *syncPoints) Close() {
	s.writer.Shutdown()
}
