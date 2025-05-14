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

package blockindexer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/query"
	"github.com/kaleido-io/paladin/sdk/go/pkg/retry"
	"github.com/kaleido-io/paladin/sdk/go/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/inflight"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcserver"
)

type BlockIndexer interface {
	Start(...*InternalEventStream) error
	Stop()
	AddEventStream(ctx context.Context, dbTX persistence.DBTX, stream *InternalEventStream) (*EventStream, error)
	RemoveEventStream(ctx context.Context, id uuid.UUID) error
	QueryEventStreamDefinitions(ctx context.Context, dbTX persistence.DBTX, esType pldtypes.Enum[EventStreamType], jq *query.QueryJSON) ([]*EventStream, error)
	StartEventStream(ctx context.Context, id uuid.UUID) error
	StopEventStream(ctx context.Context, id uuid.UUID) error
	GetIndexedBlockByNumber(ctx context.Context, number uint64) (*pldapi.IndexedBlock, error)
	GetIndexedTransactionByHash(ctx context.Context, hash pldtypes.Bytes32) (*pldapi.IndexedTransaction, error)
	GetIndexedTransactionByNonce(ctx context.Context, from pldtypes.EthAddress, nonce uint64) (*pldapi.IndexedTransaction, error)
	GetBlockTransactionsByNumber(ctx context.Context, blockNumber int64) ([]*pldapi.IndexedTransaction, error)
	GetTransactionEventsByHash(ctx context.Context, hash pldtypes.Bytes32) ([]*pldapi.IndexedEvent, error)
	QueryIndexedBlocks(ctx context.Context, jq *query.QueryJSON) ([]*pldapi.IndexedBlock, error)
	QueryIndexedEvents(ctx context.Context, jq *query.QueryJSON) ([]*pldapi.IndexedEvent, error)
	QueryIndexedTransactions(ctx context.Context, jq *query.QueryJSON) ([]*pldapi.IndexedTransaction, error)
	ListTransactionEvents(ctx context.Context, lastBlock int64, lastIndex, limit int) ([]*pldapi.IndexedEvent, error)
	DecodeTransactionEvents(ctx context.Context, hash pldtypes.Bytes32, abi abi.ABI, resultFormat pldtypes.JSONFormatOptions) ([]*pldapi.EventWithData, error)
	WaitForTransactionSuccess(ctx context.Context, hash pldtypes.Bytes32, errorABI abi.ABI) (*pldapi.IndexedTransaction, error)
	WaitForTransactionAnyResult(ctx context.Context, hash pldtypes.Bytes32) (*pldapi.IndexedTransaction, error)
	GetBlockListenerHeight(ctx context.Context) (highest uint64, err error)
	GetConfirmedBlockHeight(ctx context.Context) (confirmed pldtypes.HexUint64, err error)
	GetEventStreamStatus(ctx context.Context, id uuid.UUID) (*EventStreamStatus, error)
	RPCModule() *rpcserver.RPCModule
}

// Processes blocks from a configure baseline block (0 for example), up until it
// reaches the head of the chain. Then processes blocks that come from the listener
// against the required number of confirmations.
//
// Note that this builds upon the block listener, which likely itself has detailed handling
// of re-orgs at the front of the chain
//
// This implementation is thus deliberately simple assuming that when instability is found
// in the notifications it can simply wipe out its view and start again.
type blockIndexer struct {
	parentCtxForReset          context.Context
	cancelFunc                 func()
	persistence                persistence.Persistence
	blockListener              *blockListener
	wsConn                     rpcclient.WSClient
	stateLock                  sync.Mutex
	fromBlock                  *ethtypes.HexUint64
	nextBlock                  *ethtypes.HexUint64 // nil in the special case of "latest" and no block received yet
	highestConfirmedBlock      atomic.Int64        // set after we persist blocks
	blocksSinceCheckpoint      []*BlockInfoJSONRPC
	newHeadToAdd               []*BlockInfoJSONRPC // used by the notification routine when there are new blocks that add directly onto the end of the blocksSinceCheckpoint
	requiredConfirmations      int
	retry                      *retry.Retry
	batchSize                  int
	batchTimeout               time.Duration
	txWaiters                  *inflight.InflightManager[pldtypes.Bytes32, *pldapi.IndexedTransaction]
	preCommitHandlers          []PreCommitHandler
	eventStreams               map[uuid.UUID]*eventStream
	eventStreamsHeadSet        map[uuid.UUID]*eventStream
	eventStreamsLock           sync.Mutex
	esBlockDispatchQueueLength int
	esCatchUpQueryPageSize     int
	started                    bool
	dispatcherTap              chan struct{}
	processorDone              chan struct{}
	dispatcherDone             chan struct{}
	rpcModule                  *rpcserver.RPCModule
}

func NewBlockIndexer(ctx context.Context, config *pldconf.BlockIndexerConfig, wsConfig *pldconf.WSClientConfig, persistence persistence.Persistence) (_ BlockIndexer, err error) {

	blockListener, err := newBlockListener(ctx, config, wsConfig)
	if err != nil {
		return nil, err
	}

	return newBlockIndexer(ctx, config, persistence, blockListener)
}

func newBlockIndexer(ctx context.Context, conf *pldconf.BlockIndexerConfig, persistence persistence.Persistence, blockListener *blockListener) (bi *blockIndexer, err error) {
	bi = &blockIndexer{
		parentCtxForReset:          ctx, // stored for startOrResetProcessing
		persistence:                persistence,
		wsConn:                     blockListener.wsConn,
		blockListener:              blockListener,
		requiredConfirmations:      confutil.IntMin(conf.RequiredConfirmations, 0, *pldconf.BlockIndexerDefaults.RequiredConfirmations),
		retry:                      blockListener.retry,
		batchSize:                  confutil.IntMin(conf.CommitBatchSize, 1, *pldconf.BlockIndexerDefaults.CommitBatchSize),
		batchTimeout:               confutil.DurationMin(conf.CommitBatchTimeout, 0, *pldconf.BlockIndexerDefaults.CommitBatchTimeout),
		txWaiters:                  inflight.NewInflightManager[pldtypes.Bytes32, *pldapi.IndexedTransaction](pldtypes.ParseBytes32),
		eventStreams:               make(map[uuid.UUID]*eventStream),
		eventStreamsHeadSet:        make(map[uuid.UUID]*eventStream),
		esBlockDispatchQueueLength: confutil.IntMin(conf.EventStreams.BlockDispatchQueueLength, 0, *pldconf.EventStreamDefaults.BlockDispatchQueueLength),
		esCatchUpQueryPageSize:     confutil.IntMin(conf.EventStreams.CatchUpQueryPageSize, 0, *pldconf.EventStreamDefaults.CatchUpQueryPageSize),
		dispatcherTap:              make(chan struct{}, 1),
	}
	bi.highestConfirmedBlock.Store(-1)
	bi.fromBlock, err = bi.getFromBlock(ctx, conf.FromBlock, pldconf.BlockIndexerDefaults.FromBlock)
	if err != nil {
		return nil, err
	}
	if err := bi.loadEventStreams(ctx); err != nil {
		return nil, err
	}
	bi.initRPC()
	return bi, nil
}

func (bi *blockIndexer) Start(internalStreams ...*InternalEventStream) error {
	// Internal event streams can be instated before we start the listener itself
	// (so even on first startup they function as if they were there before the indexer loads)
	for _, ies := range internalStreams {
		switch ies.Type {
		case IESTypeEventStreamDBTX:
			if _, err := bi.upsertInternalEventStream(bi.parentCtxForReset, bi.persistence.NOTX(), ies); err != nil {
				return err
			}
		case IESTypePreCommitHandler:
			bi.preCommitHandlers = append(bi.preCommitHandlers, ies.PreCommitHandler)
		}
	}
	bi.blockListener.start()
	bi.startOrReset()
	bi.startEventStreams()
	return nil
}

func (bi *blockIndexer) startOrReset() {
	bi.Stop()

	// Restore any checkpoint
	runCtx, cancelFunc := context.WithCancel(log.WithLogField(bi.parentCtxForReset, "role", "block_indexer"))
	if err := bi.retry.Do(runCtx, func(attempt int) (retryable bool, err error) {
		return true, bi.restoreCheckpoint()
	}); err != nil {
		cancelFunc()
		return
	}

	// kick things off
	bi.stateLock.Lock()
	bi.blocksSinceCheckpoint = nil
	bi.newHeadToAdd = nil
	bi.processorDone = make(chan struct{})
	bi.dispatcherDone = make(chan struct{})
	bi.cancelFunc = cancelFunc
	bi.started = true
	bi.stateLock.Unlock()

	go bi.startup(runCtx)

}

func (bi *blockIndexer) startup(runCtx context.Context) {

	// Do we need the highest block height to start?
	if bi.nextBlock == nil {
		highestBlock, err := bi.blockListener.getHighestBlock(runCtx)
		if err != nil {
			close(bi.dispatcherDone)
			close(bi.processorDone)
			return
		}
		log.L(bi.parentCtxForReset).Infof("Block indexer queried 'latest' starting block from chain nextBlock=%d", highestBlock)
		bi.stateLock.Lock()
		bi.nextBlock = (*ethtypes.HexUint64)(&highestBlock)
		bi.stateLock.Unlock()
	}

	go bi.dispatcher(runCtx)
	go bi.notificationProcessor(runCtx)

}

func (bi *blockIndexer) Stop() {
	bi.stateLock.Lock()
	wasStarted := bi.started
	processorDone := bi.processorDone
	dispatcherDone := bi.dispatcherDone
	cancelCtx := bi.cancelFunc
	bi.started = false
	bi.stateLock.Unlock()

	if wasStarted {
		bi.eventStreamsLock.Lock()
		for _, es := range bi.eventStreams {
			// no possibility of error if not updating DB
			_ = es.stop(false)
		}
		bi.eventStreamsLock.Unlock()

		if cancelCtx != nil {
			cancelCtx()
		}
		if processorDone != nil {
			<-processorDone
		}
		if dispatcherDone != nil {
			<-dispatcherDone
		}
	}
}

func (bi *blockIndexer) GetConfirmedBlockHeight(ctx context.Context) (highest pldtypes.HexUint64, err error) {
	highestConfirmedBlock := bi.highestConfirmedBlock.Load()
	if highestConfirmedBlock < 0 {
		return 0, i18n.NewError(ctx, msgs.MsgBlockIndexerNoBlocksIndexed)
	}
	return pldtypes.HexUint64(highestConfirmedBlock), nil
}

func (bi *blockIndexer) GetBlockListenerHeight(ctx context.Context) (confirmed uint64, err error) {
	return bi.blockListener.getHighestBlock(ctx)
}

func (bi *blockIndexer) getFromBlock(ctx context.Context, fromBlock json.RawMessage, defaultValue json.RawMessage) (*ethtypes.HexUint64, error) {
	var vUntyped interface{}
	if fromBlock == nil {
		fromBlock = defaultValue
	}
	dec := json.NewDecoder(bytes.NewReader(fromBlock))
	dec.UseNumber()
	if err := dec.Decode(&vUntyped); err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgBlockIndexerInvalidFromBlock, fromBlock)
	}
	switch vTyped := vUntyped.(type) {
	case string:
		return bi.getFromBlockStr(ctx, vTyped)
	case json.Number:
		return bi.getFromBlockStr(ctx, vTyped.String())
	default:
		return nil, i18n.NewError(ctx, msgs.MsgBlockIndexerInvalidFromBlock, fromBlock)
	}
}

func (bi *blockIndexer) getFromBlockStr(ctx context.Context, fromBlock string) (*ethtypes.HexUint64, error) {
	log.L(ctx).Infof("From block: %s", fromBlock)
	if strings.EqualFold(fromBlock, "latest") {
		return nil, nil
	}
	uint64Val, err := strconv.ParseUint(fromBlock, 0, 64)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgBlockIndexerInvalidFromBlock, fromBlock)
	}
	return (*ethtypes.HexUint64)(&uint64Val), nil
}

func (bi *blockIndexer) restoreCheckpoint() error {

	// Outcomes possible:
	// 1) We found a block written to the DB - one after this is our expected next block
	// 2) We have a non-nil fromBlock - that is our checkpoint
	// 3) We have a nil ("latest") fromBlock - we just need to wait for the next block
	var blocks []*pldapi.IndexedBlock
	err := bi.persistence.DB().
		Table("indexed_blocks").
		Order("number DESC").
		Limit(1).
		Find(&blocks).
		Error
	if err != nil {
		return err
	}
	switch {
	case len(blocks) > 0:
		log.L(bi.parentCtxForReset).Infof("Block indexer restarting from checkpoint fromBlock=%s", bi.fromBlock)
		nextBlock := ethtypes.HexUint64(blocks[0].Number + 1)
		bi.nextBlock = &nextBlock
		bi.highestConfirmedBlock.Store(blocks[0].Number)
	default:
		bi.nextBlock = bi.fromBlock
	}
	return nil
}

// The notificationProcessor processes all notification immediately from the head of the chain
// regardless of how far back in the chain the dispatcher is.
func (bi *blockIndexer) notificationProcessor(ctx context.Context) {
	defer close(bi.processorDone)
	for {
		select {
		case block := <-bi.blockListener.channel():
			bi.processBlockNotification(ctx, block)
		case <-ctx.Done():
			log.L(ctx).Debugf("Confirmed block listener stopping")
			return
		}
	}
}

// Whenever we get a new block we try and reconcile it into our current view of the
// canonical chain ahead of the last checkpoint.
// Then we update the state the dispatcher uses to walk forwards from and see what
// is confirmed and ready to dispatch
func (bi *blockIndexer) processBlockNotification(ctx context.Context, block *BlockInfoJSONRPC) {
	bi.stateLock.Lock()
	defer bi.stateLock.Unlock()

	log.L(ctx).Tracef("<processBlockNotification> block.Number:%d, bi.nextBlock:%d, len(bi.newHeadToAdd):%d, len(bi.blocksSinceCheckpoint):%d",
		block.Number, *bi.nextBlock, len(bi.newHeadToAdd), len(bi.blocksSinceCheckpoint))

	// If the block is before our checkpoint, we ignore it completely
	if block.Number < *bi.nextBlock {
		log.L(ctx).Debugf("Notification of block %d/%s <= next block %d", block.Number, block.Hash, *bi.nextBlock)
		return
	}

	// If the block immediate adds onto the set of blocks being processed, then we just attach it there
	// and notify the dispatcher to process it directly. No need for the other routine to query again.
	// When we're in steady state listening to the stable head of the chain, this should be the most common case.
	var dispatchHead *BlockInfoJSONRPC
	if len(bi.newHeadToAdd) > 0 {
		// we've snuck in multiple notifications while the dispatcher is busy... don't add indefinitely to this list
		if len(bi.newHeadToAdd) > 10 /* not considered worth adding/explaining a tuning property for this */ {
			log.L(ctx).Infof("Block listener fell behind head of chain")
			bi.newHeadToAdd = nil
		} else {
			dispatchHead = bi.newHeadToAdd[len(bi.newHeadToAdd)-1]
		}
	}
	if dispatchHead == nil && len(bi.blocksSinceCheckpoint) > 0 {
		log.L(ctx).Debugf("Setting dispatch head to %d blocks since checkpoint", len(bi.blocksSinceCheckpoint))
		dispatchHead = bi.blocksSinceCheckpoint[len(bi.blocksSinceCheckpoint)-1]
	}
	if dispatchHead == nil {
		log.L(ctx).Trace("<processBlockNotification> dispatchHead is nil")
	} else {
		log.L(ctx).Tracef("<processBlockNotification> dispatchHead.Number:%d, dispatchHead.Hash:%s, block.ParentHash:%s",
			dispatchHead.Number, dispatchHead.Hash, block.ParentHash)
	}

	switch {
	case dispatchHead != nil && block.Number == dispatchHead.Number+1 && block.ParentHash.Equals(dispatchHead.Hash):
		// Ok - we just need to pop it onto the list, and ensure we wake the dispatcher routine
		log.L(ctx).Debugf("Directly passing block %d/%s to dispatcher after block %d/%s", block.Number, block.Hash, dispatchHead.Number, dispatchHead.Hash)
		bi.newHeadToAdd = append(bi.newHeadToAdd, block)
	case dispatchHead == nil && (block.Number == *bi.nextBlock):
		// This is the next block the dispatcher needs, to wake it up with this.
		log.L(ctx).Debugf("Directly passing block %d/%s to dispatcher as no blocks pending", block.Number, block.Hash)
		bi.newHeadToAdd = append(bi.newHeadToAdd, block)
	default:
		// Otherwise see if it's a conflicting fork to any of our existing blocks
		for idx, existingBlock := range bi.blocksSinceCheckpoint {
			if existingBlock.Number == block.Number {
				// Must discard up to this point
				bi.blocksSinceCheckpoint = bi.blocksSinceCheckpoint[0:idx]
				bi.newHeadToAdd = nil
				// This block fits, slot it into this point in the chain
				if idx == 0 || block.ParentHash.Equals(bi.blocksSinceCheckpoint[idx-1].Hash) {
					log.L(ctx).Debugf("Notification of re-org %d/%s replacing block %d/%s", block.Number, block.Hash, existingBlock.Number, existingBlock.Hash)
					bi.blocksSinceCheckpoint = append(bi.blocksSinceCheckpoint[0:idx], block)
				} else {
					log.L(ctx).Debugf("Notification of block %d/%s conflicting with previous block %d/%s", block.Number, block.Hash, existingBlock.Number, existingBlock.Hash)
				}
				break
			}
		}
	}

	// There's something for the dispatcher to process
	log.L(ctx).Debugf("Notification for %d/%s tapping dispatcher", block.Number, block.Hash)
	bi.tapDispatcher()

}

func (bi *blockIndexer) tapDispatcher() {
	select {
	case bi.dispatcherTap <- struct{}{}:
	default:
	}
}

type blockWriterBatch struct {
	wg             sync.WaitGroup
	lock           sync.Mutex
	opened         time.Time
	blocks         []*BlockInfoJSONRPC
	summaries      []string
	receipts       [][]*TXReceiptJSONRPC
	receiptResults []error
	timeoutContext context.Context
	timeoutCancel  func()
}

func (bi *blockIndexer) dispatchEnrich(ctx context.Context, batch *blockWriterBatch, toDispatch *BlockInfoJSONRPC) {
	batch.lock.Lock()
	defer batch.lock.Unlock()
	blockIndex := len(batch.blocks)
	batch.blocks = append(batch.blocks, toDispatch)
	batch.summaries = append(batch.summaries, fmt.Sprintf("%s/%d", toDispatch.Hash.String(), toDispatch.Number))
	batch.receiptResults = append(batch.receiptResults, nil)
	if len(toDispatch.Transactions) > 0 {
		batch.receipts = append(batch.receipts, nil)
		batch.wg.Add(1) // we need to wait for this to return
		go bi.hydrateBlock(ctx, batch, blockIndex)
	} else {
		// No need to call get receipts for empty blocks
		batch.receipts = append(batch.receipts, []*TXReceiptJSONRPC{})
	}
}

func (bi *blockIndexer) dispatcher(ctx context.Context) {
	defer close(bi.dispatcherDone)

	var batch *blockWriterBatch
	var timedOut bool

	timeoutContext := ctx
	lastFromNotification := false

	for {
		var pendingDispatch *BlockInfoJSONRPC

		found := bi.readNextBlock(ctx, &lastFromNotification)
		if found {
			pendingDispatch = bi.getNextConfirmed(ctx)
		}

		if pendingDispatch != nil {
			if batch == nil {
				batch = &blockWriterBatch{opened: time.Now()}
				batch.timeoutContext, batch.timeoutCancel = context.WithTimeout(ctx, bi.batchTimeout)
			}
			timeoutContext = batch.timeoutContext
			log.L(ctx).Tracef("Dispatching enrich for block %d/%s", pendingDispatch.Number, pendingDispatch.Hash)
			bi.dispatchEnrich(ctx, batch, pendingDispatch)
		}

		if batch != nil && (timedOut || (len(batch.blocks) >= bi.batchSize)) {
			batch.timeoutCancel()
			// Wait for all the hydrations in the batch to complete (for good or bad)
			log.L(ctx).Debugf("Flushing block indexing batch: %s", batch.summaries)
			batch.wg.Wait()
			// Check we got all the results, or we have to reset
			for i, receiptError := range batch.receiptResults {
				if receiptError != nil {
					log.L(ctx).Errorf("Block indexer requires reset after failing to query receipts for block %s in batch of %d blocks: %s", batch.blocks[i].Hash, len(batch.blocks), receiptError)
					go bi.startOrReset()
					return // We know we need to exit
				}
			}
			bi.writeBatch(ctx, batch)
			// Write the batch
			batch = nil
		}

		timedOut = false
		if !found {
			lastFromNotification = false
			select {
			case <-bi.dispatcherTap:
			case <-timeoutContext.Done():
				timedOut = true
				timeoutContext = ctx
				select {
				case <-ctx.Done():
					log.L(ctx).Debugf("Confirmed block dispatcher stopping")
					return
				default:
				}
			}
		}

	}
}

func (bi *blockIndexer) hydrateBlock(ctx context.Context, batch *blockWriterBatch, blockIndex int) {
	defer batch.wg.Done()
	err := bi.retry.Do(ctx, func(attempt int) (bool, error) {
		// We use eth_getBlockReceipts, which takes either a number or a hash (supported by Besu and go-ethereum)
		rpcErr := bi.wsConn.CallRPC(ctx, &batch.receipts[blockIndex], "eth_getBlockReceipts", batch.blocks[blockIndex].Hash)
		if rpcErr != nil || batch.receipts[blockIndex] == nil {
			var err error = rpcErr
			retry := true
			log.L(ctx).Errorf("Failed to query block %s: %v", batch.summaries[blockIndex], rpcErr)
			if err == nil {
				// TODO: We've seen this with Besu instead of an error, and need to diagnose
				// Convert to a not found, but DO retry here.
				log.L(ctx).Warnf("Blockchain node returned null from eth_getBlockReceipts")
				err = i18n.NewError(ctx, msgs.MsgBlockIndexerConfirmedBlockNotFound, batch.blocks[blockIndex].Hash, batch.blocks[blockIndex].Number)
			} else if isNotFound(err) {
				// If we get a not-found, that's an indication the confirmations are not set correctly,
				// but there's no point in continuing to retry as a confirmed block should be available
				// on our connection.
				// This nil entry in batch.receipts[blockIndex] triggers a reset.
				retry = false
				err = i18n.WrapError(ctx, rpcErr, msgs.MsgBlockIndexerConfirmedBlockNotFound, batch.blocks[blockIndex].Hash, batch.blocks[blockIndex].Number)
			}
			return retry, err
		}
		return false, nil
	})
	batch.receiptResults[blockIndex] = err
}

func (bi *blockIndexer) logToIndexedEvent(l *LogJSONRPC) *pldapi.IndexedEvent {
	var topic0 pldtypes.Bytes32
	if len(l.Topics) > 0 {
		topic0 = pldtypes.NewBytes32FromSlice(l.Topics[0])
	}
	return &pldapi.IndexedEvent{
		Signature:        topic0,
		TransactionHash:  pldtypes.NewBytes32FromSlice(l.TransactionHash),
		BlockNumber:      int64(l.BlockNumber),
		TransactionIndex: int64(l.TransactionIndex),
		LogIndex:         int64(l.LogIndex),
	}
}

func (bi *blockIndexer) blockInfoToIndexedBlock(block *BlockInfoJSONRPC) *pldapi.IndexedBlock {
	return &pldapi.IndexedBlock{
		Timestamp: pldtypes.Timestamp(block.Timestamp),
		Number:    int64(block.Number),
		Hash:      pldtypes.NewBytes32FromSlice(block.Hash),
	}
}

func (bi *blockIndexer) writeBatch(ctx context.Context, batch *blockWriterBatch) {

	var blocks []*pldapi.IndexedBlock
	var notifyTransactions []*IndexedTransactionNotify
	var transactions []*pldapi.IndexedTransaction
	var events []*pldapi.IndexedEvent
	newHighestBlock := int64(-1)

	for i, block := range batch.blocks {
		newHighestBlock = int64(block.Number)
		blocks = append(blocks, bi.blockInfoToIndexedBlock(block))
		for txIndex, r := range batch.receipts[i] {
			result := pldapi.TXResult_FAILURE.Enum()
			if r.Status.BigInt().Int64() == 1 {
				result = pldapi.TXResult_SUCCESS.Enum()
			}
			txn := IndexedTransactionNotify{
				IndexedTransaction: pldapi.IndexedTransaction{
					Hash:             pldtypes.NewBytes32FromSlice(r.TransactionHash),
					BlockNumber:      int64(r.BlockNumber),
					TransactionIndex: int64(txIndex),
					From:             (*pldtypes.EthAddress)(r.From),
					To:               (*pldtypes.EthAddress)(r.To),
					Nonce:            uint64(block.Transactions[txIndex].Nonce),
					ContractAddress:  (*pldtypes.EthAddress)(r.ContractAddress),
					Result:           result,
				},
				RevertReason: pldtypes.HexBytes(r.RevertReason),
			}
			notifyTransactions = append(notifyTransactions, &txn)
			transactions = append(transactions, &txn.IndexedTransaction)
			for _, l := range r.Logs {
				events = append(events, bi.logToIndexedEvent(l))
			}
		}
	}

	err := bi.retry.Do(ctx, func(attempt int) (retryable bool, err error) {
		err = bi.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
			for _, preCommitHandler := range bi.preCommitHandlers {
				if err == nil {
					err = preCommitHandler(ctx, dbTX, blocks, notifyTransactions)
				}
			}
			if err == nil && len(blocks) > 0 {
				err = dbTX.DB().
					WithContext(ctx).
					Table("indexed_blocks").
					Create(blocks).
					Error
			}
			if err == nil && len(transactions) > 0 {
				err = dbTX.DB().
					WithContext(ctx).
					Table("indexed_transactions").
					Create(transactions).
					Error
			}
			if err == nil && len(events) > 0 {
				err = dbTX.DB().
					WithContext(ctx).
					Table("indexed_events").
					Omit("Transaction").
					Omit("Event").
					Create(events).
					Error
			}
			return err
		})
		return true, err
	})
	if err == nil {
		// Context was cancelled exiting retry - no notification in that case
		bi.notifyEventStreams(ctx, batch)
	}
	if newHighestBlock >= 0 {
		bi.highestConfirmedBlock.Store(newHighestBlock)
	}
	if err == nil {
		for _, t := range transactions {
			if inflight := bi.txWaiters.GetInflight(t.Hash); inflight != nil {
				inflight.Complete(t)
			}
		}
	}
}

func (bi *blockIndexer) notifyEventStreams(ctx context.Context, batch *blockWriterBatch) {
	// Every event stream gets notified about every block, but only the
	// logs that are applicable to it
	bi.eventStreamsLock.Lock()
	defer bi.eventStreamsLock.Unlock()
	for _, es := range bi.eventStreams {
		for iBlk, blk := range batch.blocks {
			blockNotification := &eventStreamBlock{
				block: blk,
			}
			for _, r := range batch.receipts[iBlk] {
				for _, l := range r.Logs {
					if len(l.Topics) > 0 {
						logSig := l.Topics[0].String()
						if _, isMatch := es.signatures[logSig]; isMatch {
							// This is a log the event stream needs
							blockNotification.events = append(blockNotification.events, l)
						}
					}
				}
			}
			// Best effort dispatch here - it's the Event Stream's responsibility
			// to keep up to date. If it falls behind, it will catch back up
			// using the database and the node.
			select {
			case es.blocks <- blockNotification:
				log.L(ctx).Debugf("dispatched block %d (%d signature matched events) to ES %s",
					blockNotification.block.Number, len(blockNotification.events), es.definition.ID)
			default:
			}
		}
	}
}

// MUST be called under lock
func (bi *blockIndexer) popDispatchedIfAvailable(lastFromNotification *bool) (blockNumberToFetch ethtypes.HexUint64, found bool) {

	if len(bi.newHeadToAdd) > 0 {
		// If we find one in the lock, it must be ready for us to append
		nextBlock := bi.newHeadToAdd[0]
		bi.newHeadToAdd = append([]*BlockInfoJSONRPC{}, bi.newHeadToAdd[1:]...)
		bi.blocksSinceCheckpoint = append(bi.blocksSinceCheckpoint, nextBlock)

		// We track that we've done this, so we know if we run out going round the loop later,
		// there's no point in doing a get-by-number
		*lastFromNotification = true
		return 0, true
	}

	blockNumberToFetch = *bi.nextBlock
	if len(bi.blocksSinceCheckpoint) > 0 {
		blockNumberToFetch = bi.blocksSinceCheckpoint[len(bi.blocksSinceCheckpoint)-1].Number + 1
	}
	return blockNumberToFetch, false
}

func (bi *blockIndexer) readNextBlock(ctx context.Context, lastFromNotification *bool) (found bool) {

	var nextBlock *BlockInfoJSONRPC
	var blockNumberToFetch ethtypes.HexUint64
	var dispatchedPopped bool
	err := bi.retry.Do(ctx, func(_ int) (retry bool, err error) {
		// If the notifier has lined up a block for us grab it before
		bi.stateLock.Lock()
		blockNumberToFetch, dispatchedPopped = bi.popDispatchedIfAvailable(lastFromNotification)
		bi.stateLock.Unlock()
		if dispatchedPopped || *lastFromNotification {
			// We processed a dispatch this time, or last time.
			// Either way we're tracking at the head and there's no point doing a query
			// we expect to return nothing - as we should get another notification.
			return false, nil
		}

		// Get the next block
		nextBlock, err = bi.blockListener.getBlockInfoByNumber(ctx, blockNumberToFetch)
		return true, err
	})
	if nextBlock == nil || err != nil {
		// We either got a block dispatched, or did not find a block ourselves.
		return dispatchedPopped
	}

	// In the lock append it to our list, checking it's valid to append to what we have
	bi.stateLock.Lock()
	defer bi.stateLock.Unlock()

	// We have to check because we unlocked, that we weren't beaten to the punch while we queried
	// by the dispatcher.
	if _, dispatchedPopped = bi.popDispatchedIfAvailable(lastFromNotification); !dispatchedPopped {

		// It's possible that while we were off at the node querying this, a notification came in
		// that affected our state. We need to check this still matches, or go round again
		if len(bi.blocksSinceCheckpoint) > 0 {
			headBlock := bi.blocksSinceCheckpoint[len(bi.blocksSinceCheckpoint)-1]
			if !headBlock.Hash.Equals(nextBlock.ParentHash) {
				// This doesn't attach to the end of our list. Trim it off and try again.
				bi.blocksSinceCheckpoint = bi.blocksSinceCheckpoint[0 : len(bi.blocksSinceCheckpoint)-1]
				log.L(ctx).Debugf("Block %d / %s does not fit in our view of the canonical chain - trimming (parentHash=%s != head[%d]=%s, new blocksSinceCheckpoint=%d)",
					nextBlock.Number, nextBlock.Hash, nextBlock.ParentHash, len(bi.blocksSinceCheckpoint)-1, headBlock.Hash, len(bi.blocksSinceCheckpoint))
				return true
			}
		}

		// We successfully attached it
		bi.blocksSinceCheckpoint = append(bi.blocksSinceCheckpoint, nextBlock)
		log.L(ctx).Debugf("Added read block %d / %s to list (new blocksSinceCheckpoint=%d)", nextBlock.Number, nextBlock.Hash, len(bi.blocksSinceCheckpoint))
	}
	return true

}

func (bi *blockIndexer) getNextConfirmed(ctx context.Context) (toDispatch *BlockInfoJSONRPC) {
	bi.stateLock.Lock()
	defer bi.stateLock.Unlock()
	if len(bi.blocksSinceCheckpoint) > bi.requiredConfirmations {
		toDispatch = bi.blocksSinceCheckpoint[0]
		// don't want memory to grow indefinitely by shifting right, so we create a new slice here
		bi.blocksSinceCheckpoint = append([]*BlockInfoJSONRPC{}, bi.blocksSinceCheckpoint[1:]...)
		newCheckpoint := toDispatch.Number + 1
		bi.nextBlock = &newCheckpoint
		log.L(ctx).Debugf("Confirmed block popped for dispatch %d / %s (new blocksSinceCheckpoint=%d)", toDispatch.Number, toDispatch.Hash, len(bi.blocksSinceCheckpoint))
	}
	return toDispatch
}

func (bi *blockIndexer) WaitForTransactionAnyResult(ctx context.Context, hash pldtypes.Bytes32) (*pldapi.IndexedTransaction, error) {
	inflight := bi.txWaiters.AddInflight(ctx, hash)
	defer inflight.Cancel()

	tx, err := bi.GetIndexedTransactionByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	if tx != nil {
		log.L(ctx).Infof("TX %s already in DB", hash)
		return tx, nil
	}
	return inflight.Wait()
}

func (bi *blockIndexer) WaitForTransactionSuccess(ctx context.Context, hash pldtypes.Bytes32, errorABI abi.ABI) (*pldapi.IndexedTransaction, error) {
	rtx, err := bi.WaitForTransactionAnyResult(ctx, hash)
	if err != nil {
		return nil, err
	}
	if rtx.Result.V() == pldapi.TXResult_SUCCESS {
		return rtx, nil
	}
	return nil, bi.getReceiptRevertError(ctx, hash, errorABI)
}

func (bi *blockIndexer) getReceiptRevertError(ctx context.Context, hash pldtypes.Bytes32, errorABI abi.ABI) error {
	// See if we can decode the error from the receipt
	receipt, err := bi.getConfirmedTransactionReceipt(ctx, hash[:])
	if err != nil {
		return err
	}
	if errorABI == nil {
		errorABI = abi.ABI{}
	}
	// Note only Besu when configured with --revert-reason-enabled and in full sync mode will have the revert reason to decode
	errString, _ := errorABI.ErrorStringCtx(ctx, receipt.RevertReason)
	if errString == "" {
		errString = ethtypes.HexBytes0xPrefix(receipt.RevertReason).String()
	}
	return i18n.NewError(ctx, msgs.MsgBlockIndexerTransactionReverted, errString)
}

func (bi *blockIndexer) GetIndexedBlockByNumber(ctx context.Context, number uint64) (*pldapi.IndexedBlock, error) {
	var blocks []*pldapi.IndexedBlock
	db := bi.persistence.DB()
	err := db.
		WithContext(ctx).
		Table("indexed_blocks").
		Where("number = ?", number).
		Find(&blocks).
		Error
	if err != nil || len(blocks) < 1 {
		return nil, err
	}
	return blocks[0], nil
}

func (bi *blockIndexer) GetIndexedTransactionByHash(ctx context.Context, hash pldtypes.Bytes32) (*pldapi.IndexedTransaction, error) {
	return bi.getIndexedTransactionByHash(ctx, hash)
}

func (bi *blockIndexer) getIndexedTransactionByHash(ctx context.Context, hashID pldtypes.Bytes32) (*pldapi.IndexedTransaction, error) {
	var txns []*pldapi.IndexedTransaction
	db := bi.persistence.DB()
	err := db.
		WithContext(ctx).
		Table("indexed_transactions").
		Where("hash = ?", hashID).
		Find(&txns).
		Error
	if err != nil || len(txns) < 1 {
		return nil, err
	}
	return txns[0], nil
}

func (bi *blockIndexer) GetIndexedTransactionByNonce(ctx context.Context, from pldtypes.EthAddress, nonce uint64) (*pldapi.IndexedTransaction, error) {
	var txns []*pldapi.IndexedTransaction
	db := bi.persistence.DB()
	err := db.
		WithContext(ctx).
		Table("indexed_transactions").
		Where(`"from" = ?`, from).
		Where("nonce = ?", nonce).
		Find(&txns).
		Error
	if err != nil || len(txns) < 1 {
		return nil, err
	}
	return txns[0], nil
}

func (bi *blockIndexer) GetBlockTransactionsByNumber(ctx context.Context, blockNumber int64) ([]*pldapi.IndexedTransaction, error) {
	var txns []*pldapi.IndexedTransaction
	db := bi.persistence.DB()
	err := db.
		WithContext(ctx).
		Table("indexed_transactions").
		Order("block_number").
		Order("transaction_index").
		Where("block_number = ?", blockNumber).
		Find(&txns).
		Error
	return txns, err
}

func (bi *blockIndexer) GetTransactionEventsByHash(ctx context.Context, hash pldtypes.Bytes32) ([]*pldapi.IndexedEvent, error) {
	var events []*pldapi.IndexedEvent
	db := bi.persistence.DB()
	err := db.
		WithContext(ctx).
		Table("indexed_events").
		Where("transaction_hash = ?", hash).
		Order("log_index").
		Find(&events).
		Error
	return events, err
}

func (bi *blockIndexer) ListTransactionEvents(ctx context.Context, lastBlock int64, lastIndex, limit int) ([]*pldapi.IndexedEvent, error) {
	var events []*pldapi.IndexedEvent
	db := bi.persistence.DB()
	q := db.
		WithContext(ctx).
		Table("indexed_events").
		Joins("Block").
		Where("indexed_events.block_number > ?", lastBlock).
		Or(db.Where("indexed_events.block_number = ?", lastBlock).Where("indexed_events.log_index > ?", lastIndex)).
		Order("indexed_events.block_number").
		Order("indexed_events.transaction_index").
		Order("indexed_events.log_index").
		Limit(limit)
	err := q.Find(&events).Error
	return events, err
}

func (bi *blockIndexer) DecodeTransactionEvents(ctx context.Context, hash pldtypes.Bytes32, a abi.ABI, resultFormat pldtypes.JSONFormatOptions) ([]*pldapi.EventWithData, error) {
	var serailizer *abi.Serializer
	events, err := bi.GetTransactionEventsByHash(ctx, hash)
	if err == nil {
		serailizer, err = resultFormat.GetABISerializer(ctx)
	}
	if err != nil {
		return nil, err
	}
	decoded := make([]*pldapi.EventWithData, len(events))
	for i, event := range events {
		decoded[i] = &pldapi.EventWithData{IndexedEvent: event}
	}
	err = bi.enrichTransactionEvents(ctx, a, nil, hash, decoded, serailizer, false /* no retry */)
	return decoded, err
}

func (bi *blockIndexer) getConfirmedTransactionReceipt(ctx context.Context, tx ethtypes.HexBytes0xPrefix) (*TXReceiptJSONRPC, error) {
	var receipt *TXReceiptJSONRPC
	rpcErr := bi.wsConn.CallRPC(ctx, &receipt, "eth_getTransactionReceipt", tx)
	if rpcErr != nil {
		return nil, rpcErr
	}
	if receipt == nil {
		return nil, i18n.NewError(ctx, msgs.MsgBlockIndexerConfirmedReceiptNotFound, tx)
	}
	return receipt, nil
}

func (bi *blockIndexer) enrichTransactionEvents(ctx context.Context, abi abi.ABI, source *pldtypes.EthAddress, tx pldtypes.Bytes32, events []*pldapi.EventWithData, serializer *abi.Serializer, indefiniteRetry bool) error {
	// Get the TX receipt with all the logs
	var receipt *TXReceiptJSONRPC
	err := bi.retry.Do(ctx, func(attempt int) (_ bool, err error) {
		receipt, err = bi.getConfirmedTransactionReceipt(ctx, tx[:])
		return indefiniteRetry, err
	})
	if err != nil {
		return err
	}

	// Spin through the logs to find the corresponding result entries
	for _, l := range receipt.Logs {
		for _, e := range events {
			if ethtypes.HexUint64(e.LogIndex) == l.LogIndex {
				// This the the log for this event - try and enrich the .Data field
				_ = bi.matchLog(ctx, abi, l, e, source, serializer)
				break
			}
		}
	}
	return nil
}

func (bi *blockIndexer) matchLog(ctx context.Context, abi abi.ABI, in *LogJSONRPC, out *pldapi.EventWithData, source *pldtypes.EthAddress, serializer *abi.Serializer) bool {
	if !source.IsZero() && !source.Equals((*pldtypes.EthAddress)(in.Address)) {
		log.L(ctx).Debugf("Event %d/%d/%d does not match source=%s (tx=%s,address=%s)", in.BlockNumber, in.TransactionIndex, in.LogIndex, source, in.TransactionHash, in.Address)
		return false
	}
	// This is one that matches our signature, but we need to check it against our ABI list.
	// We stop at the first entry that parses it, and it's perfectly fine and expected that
	// none will (because Eth signatures are not precise enough to distinguish events -
	// particularly the "indexed" settings on parameters)
	for _, abiEntry := range abi {
		cv, err := abiEntry.DecodeEventDataCtx(ctx, in.Topics, in.Data)
		if err == nil {
			out.SoliditySignature = abiEntry.SolString() // uniquely identifies this ABI entry for the event stream consumer
			out.Data, err = serializer.SerializeJSONCtx(ctx, cv)
		}
		if err == nil {
			log.L(ctx).Debugf("Event %d/%d/%d matches ABI event %s matchSource=%v (tx=%s,address=%s)", in.BlockNumber, in.TransactionIndex, in.LogIndex, abiEntry, source, in.TransactionHash, in.Address)
			if in.Address != nil {
				out.Address = pldtypes.EthAddress(*in.Address)
			}
			return true
		} else {
			log.L(ctx).Tracef("Event %d/%d/%d does not match ABI event %s matchSource=%v (tx=%s,address=%s): %s", in.BlockNumber, in.TransactionIndex, in.LogIndex, abiEntry, source, in.TransactionHash, in.Address, err)
		}
	}
	return false
}

func (bi *blockIndexer) QueryIndexedBlocks(ctx context.Context, jq *query.QueryJSON) ([]*pldapi.IndexedBlock, error) {

	if jq.Limit == nil || *jq.Limit == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgBlockIndexerLimitRequired)
	}
	db := bi.persistence.DB()
	q := db.Table("indexed_blocks").WithContext(ctx)
	if jq != nil {
		q = filters.BuildGORM(ctx, jq, q, IndexedBlockFilters)
	}
	var results []*pldapi.IndexedBlock
	err := q.Find(&results).Error
	return results, err
}

func (bi *blockIndexer) QueryIndexedTransactions(ctx context.Context, jq *query.QueryJSON) ([]*pldapi.IndexedTransaction, error) {

	if jq.Limit == nil || *jq.Limit == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgBlockIndexerLimitRequired)
	}
	db := bi.persistence.DB()
	q := db.Table("indexed_transactions").Joins("Block").WithContext(ctx)
	if jq != nil {
		q = filters.BuildGORM(ctx, jq, q, IndexedTransactionFilters)
	}
	var results []*pldapi.IndexedTransaction
	err := q.Find(&results).Error
	return results, err
}

func (bi *blockIndexer) QueryIndexedEvents(ctx context.Context, jq *query.QueryJSON) ([]*pldapi.IndexedEvent, error) {

	if jq.Limit == nil || *jq.Limit == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgBlockIndexerLimitRequired)
	}
	db := bi.persistence.DB()
	q := db.Table("indexed_events").Joins("Block").WithContext(ctx)
	if jq != nil {
		q = filters.BuildGORM(ctx, jq, q, IndexedEventFilters)
	}
	var results []*pldapi.IndexedEvent
	err := q.Find(&results).Error
	return results, err
}
