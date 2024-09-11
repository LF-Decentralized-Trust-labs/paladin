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
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/rpcclient"
	"github.com/kaleido-io/paladin/core/pkg/persistence"

	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/inflight"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
)

type BlockIndexer interface {
	Start(internalStreams ...*InternalEventStream) error
	Stop()
	GetIndexedBlockByNumber(ctx context.Context, number uint64) (*IndexedBlock, error)
	GetIndexedTransactionByHash(ctx context.Context, hash tktypes.Bytes32) (*IndexedTransaction, error)
	GetBlockTransactionsByNumber(ctx context.Context, blockNumber int64) ([]*IndexedTransaction, error)
	GetTransactionEventsByHash(ctx context.Context, hash tktypes.Bytes32) ([]*IndexedEvent, error)
	ListTransactionEvents(ctx context.Context, lastBlock int64, lastIndex, limit int, withTransaction, withBlock bool) ([]*IndexedEvent, error)
	DecodeTransactionEvents(ctx context.Context, hash tktypes.Bytes32, abi abi.ABI) ([]*EventWithData, error)
	WaitForTransaction(ctx context.Context, hash tktypes.Bytes32) (*IndexedTransaction, error)
	GetBlockListenerHeight(ctx context.Context) (highest uint64, err error)
	GetConfirmedBlockHeight(ctx context.Context) (confirmed uint64, err error)
}

// Processes blocks from a configure baseline block (0 for example), up until it
// reaches the head of the chain. Then processes blocks that come from the listener
// against he required number of confirmations.
//
// Note that this builds upon the lock listener, which likely itself has detailed handling
// of re-orgs at the front of the chian
//
// This implementation is thus deliberately simple assuming that when instability is found
// in the notifications it can simply wipe out its view and start again.
type blockIndexer struct {
	parentCtxForReset          context.Context
	cancelFunc                 func()
	persistence                persistence.Persistence
	blockListener              *blockListener
	wsConn                     rpcbackend.WebSocketRPCClient
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
	txWaiters                  *inflight.InflightManager[tktypes.Bytes32, *IndexedTransaction]
	eventStreams               map[uuid.UUID]*eventStream
	eventStreamsHeadSet        map[uuid.UUID]*eventStream
	eventStreamsLock           sync.Mutex
	esBlockDispatchQueueLength int
	esCatchUpQueryPageSize     int
	dispatcherTap              chan struct{}
	processorDone              chan struct{}
	dispatcherDone             chan struct{}
	utBatchNotify              chan *blockWriterBatch
}

func NewBlockIndexer(ctx context.Context, config *Config, wsConfig *rpcclient.WSConfig, persistence persistence.Persistence) (_ BlockIndexer, err error) {

	blockListener, err := newBlockListener(ctx, config, wsConfig)
	if err != nil {
		return nil, err
	}

	return newBlockIndexer(ctx, config, persistence, blockListener)
}

func newBlockIndexer(ctx context.Context, config *Config, persistence persistence.Persistence, blockListener *blockListener) (bi *blockIndexer, err error) {
	bi = &blockIndexer{
		parentCtxForReset:          ctx, // stored for startOrResetProcessing
		persistence:                persistence,
		wsConn:                     blockListener.wsConn,
		blockListener:              blockListener,
		requiredConfirmations:      confutil.IntMin(config.RequiredConfirmations, 0, *DefaultConfig.RequiredConfirmations),
		retry:                      blockListener.retry,
		batchSize:                  confutil.IntMin(config.CommitBatchSize, 1, *DefaultConfig.CommitBatchSize),
		batchTimeout:               confutil.DurationMin(config.CommitBatchTimeout, 0, *DefaultConfig.CommitBatchTimeout),
		txWaiters:                  inflight.NewInflightManager[tktypes.Bytes32, *IndexedTransaction](tktypes.ParseBytes32),
		eventStreams:               make(map[uuid.UUID]*eventStream),
		eventStreamsHeadSet:        make(map[uuid.UUID]*eventStream),
		esBlockDispatchQueueLength: confutil.IntMin(config.EventStreams.BlockDispatchQueueLength, 0, *DefaultEventStreamsConfig.BlockDispatchQueueLength),
		esCatchUpQueryPageSize:     confutil.IntMin(config.EventStreams.CatchUpQueryPageSize, 0, *DefaultEventStreamsConfig.CatchUpQueryPageSize),
		dispatcherTap:              make(chan struct{}, 1),
	}
	bi.highestConfirmedBlock.Store(-1)
	if err := bi.setFromBlock(ctx, config); err != nil {
		return nil, err
	}
	if err := bi.loadEventStreams(ctx); err != nil {
		return nil, err
	}
	return bi, nil
}

func (bi *blockIndexer) Start(internalStreams ...*InternalEventStream) error {
	// Internal event streams can be instated before we start the listener itself
	// (so even on first startup they function as if they were there before the indexer loads)
	for _, ies := range internalStreams {
		if _, err := bi.upsertInternalEventStream(bi.parentCtxForReset, ies); err != nil {
			return err
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
		bi.stateLock.Lock()
		bi.nextBlock = (*ethtypes.HexUint64)(&highestBlock)
		bi.stateLock.Unlock()
	}

	go bi.dispatcher(runCtx)
	go bi.notificationProcessor(runCtx)

}

func (bi *blockIndexer) Stop() {
	bi.stateLock.Lock()
	processorDone := bi.processorDone
	dispatcherDone := bi.dispatcherDone
	cancelCtx := bi.cancelFunc
	bi.stateLock.Unlock()

	bi.eventStreamsLock.Lock()
	for _, es := range bi.eventStreams {
		es.stop()
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

func (bi *blockIndexer) GetConfirmedBlockHeight(ctx context.Context) (highest uint64, err error) {
	highestConfirmedBlock := bi.highestConfirmedBlock.Load()
	if highestConfirmedBlock < 0 {
		return 0, i18n.NewError(ctx, msgs.MsgBlockIndexerNoBlocksIndexed)
	}
	return uint64(highestConfirmedBlock), nil
}

func (bi *blockIndexer) GetBlockListenerHeight(ctx context.Context) (confirmed uint64, err error) {
	return bi.blockListener.getHighestBlock(ctx)
}

func (bi *blockIndexer) setFromBlock(ctx context.Context, conf *Config) error {
	var vUntyped interface{}
	if conf.FromBlock == nil {
		vUntyped = "latest"
	} else {
		if err := json.Unmarshal(conf.FromBlock, &vUntyped); err != nil {
			return i18n.WrapError(ctx, err, msgs.MsgBlockIndexerInvalidFromBlock, conf.FromBlock)
		}
	}
	switch vTyped := vUntyped.(type) {
	case string:
		if strings.EqualFold(vTyped, "latest") {
			bi.fromBlock = nil
			return nil
		}
		uint64Val, err := strconv.ParseUint(vTyped, 0, 64)
		if err != nil {
			return i18n.WrapError(ctx, err, msgs.MsgBlockIndexerInvalidFromBlock, conf.FromBlock)
		}
		bi.fromBlock = (*ethtypes.HexUint64)(&uint64Val)
		return nil
	case float64:
		uint64Val := uint64(vTyped)
		bi.fromBlock = (*ethtypes.HexUint64)(&uint64Val)
		return nil
	case nil:
		bi.fromBlock = nil // same as "latest"
		return nil
	default:
		return i18n.NewError(ctx, msgs.MsgBlockIndexerInvalidFromBlock, conf.FromBlock)
	}
}

func (bi *blockIndexer) restoreCheckpoint() error {

	// Outcomes possible:
	// 1) We found a block written to the DB - one after this is our expected next block
	// 2) We have a non-nil fromBlock - that is our checkpoint
	// 3) We have a nil ("latest") fromBlock - we just need to wait for the next block
	var blocks []*IndexedBlock
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
		dispatchHead = bi.blocksSinceCheckpoint[len(bi.blocksSinceCheckpoint)-1]
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
	timeoutContext context.Context
	timeoutCancel  func()
}

func (bi *blockIndexer) dispatcher(ctx context.Context) {
	defer close(bi.dispatcherDone)

	var batch *blockWriterBatch
	var pendingDispatch []*BlockInfoJSONRPC
	var timedOut bool
	for {
		timeoutContext := ctx

		if len(pendingDispatch) == 0 {
			pendingDispatch = nil // ensure we clear the memory if we just looped through a set with pendingDispatch[1:] below

			// spin getting blocks until we it looks like we need to wait for a notification
			lastFromNotification := false
			for bi.readNextBlock(ctx, &lastFromNotification) {
				toDispatch := bi.getNextConfirmed()
				if toDispatch != nil {
					pendingDispatch = append(pendingDispatch, toDispatch)
				}
			}
		}
		if len(pendingDispatch) > 0 {
			toDispatch := pendingDispatch[0]
			pendingDispatch = pendingDispatch[1:]
			if batch == nil {
				batch = &blockWriterBatch{opened: time.Now()}
				batch.timeoutContext, batch.timeoutCancel = context.WithTimeout(ctx, bi.batchTimeout)
			}
			timeoutContext = batch.timeoutContext
			batch.lock.Lock()
			blockIndex := len(batch.blocks)
			batch.blocks = append(batch.blocks, toDispatch)
			batch.receipts = append(batch.receipts, nil)
			batch.summaries = append(batch.summaries, fmt.Sprintf("%s/%d", toDispatch.Hash.String(), toDispatch.Number))
			batch.wg.Add(1)
			batch.lock.Unlock()
			go bi.hydrateBlock(ctx, batch, blockIndex)
		}

		if batch != nil && (timedOut || (len(batch.blocks) >= bi.batchSize)) {
			batch.timeoutCancel()
			// Wait for all the hydrations in the batch to complete (for good or bad)
			log.L(ctx).Debugf("Flushing block indexing batch: %s", batch.summaries)
			batch.wg.Wait()
			// Check we got all the results, or we have to reset
			for _, r := range batch.receipts {
				if r == nil {
					log.L(ctx).Errorf("Block indexer requires reset after failing to query blocks: %s", batch.summaries)
					go bi.startOrReset()
					return // We know we need to exit
				}
			}
			bi.writeBatch(ctx, batch)
			// Write the batch
			batch = nil
		}

		timedOut = false
		if len(pendingDispatch) == 0 {
			select {
			case <-bi.dispatcherTap:
			case <-timeoutContext.Done():
				timedOut = true
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
	_ = bi.retry.Do(ctx, func(attempt int) (retryable bool, err error) {
		// We use eth_getBlockReceipts, which takes either a number or a hash (supported by Besu and go-ethereum)
		rpcErr := bi.wsConn.CallRPC(ctx, &batch.receipts[blockIndex], "eth_getBlockReceipts", batch.blocks[blockIndex].Hash)
		if rpcErr != nil {
			log.L(ctx).Errorf("Failed to query block %s: %s", batch.summaries[blockIndex], err)
			// If we get a not-found, that's an indication the confirmations are not set correctly,
			// but there's no point in continuing to retry as a confirmed block should be available
			// on our connection.
			// This nil entry in batch.receipts[blockIndex] triggers a reset.
			return !isNotFound(rpcErr), rpcErr.Error()
		}
		return false, nil
	})
}

func (bi *blockIndexer) logToIndexedEvent(l *LogJSONRPC) *IndexedEvent {
	var topic0 tktypes.Bytes32
	if len(l.Topics) > 0 {
		topic0 = tktypes.NewBytes32FromSlice(l.Topics[0])
	}
	return &IndexedEvent{
		Signature:        topic0,
		TransactionHash:  tktypes.NewBytes32FromSlice(l.TransactionHash),
		BlockNumber:      int64(l.BlockNumber),
		TransactionIndex: int64(l.TransactionIndex),
		LogIndex:         int64(l.LogIndex),
	}
}

func (bi *blockIndexer) writeBatch(ctx context.Context, batch *blockWriterBatch) {

	var blocks []*IndexedBlock
	var transactions []*IndexedTransaction
	var events []*IndexedEvent
	newHighestBlock := int64(-1)

	for i, block := range batch.blocks {
		newHighestBlock = int64(block.Number)
		blocks = append(blocks, &IndexedBlock{
			Number: int64(block.Number),
			Hash:   tktypes.NewBytes32FromSlice(block.Hash),
		})
		for txIndex, r := range batch.receipts[i] {
			result := TXResult_FAILURE.Enum()
			if r.Status.BigInt().Int64() == 1 {
				result = TXResult_SUCCESS.Enum()
			}
			transactions = append(transactions, &IndexedTransaction{
				Hash:             tktypes.NewBytes32FromSlice(r.TransactionHash),
				BlockNumber:      int64(r.BlockNumber),
				TransactionIndex: int64(txIndex),
				From:             (*tktypes.EthAddress)(r.From),
				To:               (*tktypes.EthAddress)(r.To),
				Nonce:            uint64(block.Transactions[txIndex].Nonce),
				ContractAddress:  (*tktypes.EthAddress)(r.ContractAddress),
				Result:           result,
			})
			for _, l := range r.Logs {
				events = append(events, bi.logToIndexedEvent(l))
			}
		}
	}

	err := bi.retry.Do(ctx, func(attempt int) (retryable bool, err error) {
		err = bi.persistence.DB().Transaction(func(tx *gorm.DB) error {
			if len(blocks) > 0 {
				err = tx.
					Table("indexed_blocks").
					Create(blocks).
					Error
			}
			if err == nil && len(transactions) > 0 {
				err = tx.
					Table("indexed_transactions").
					Create(transactions).
					Error
			}
			if err == nil && len(events) > 0 {
				err = tx.
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
		if bi.utBatchNotify != nil {
			bi.utBatchNotify <- batch
		}
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
				blockNumber: blk.Number.Uint64(),
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
					blockNotification.blockNumber, len(blockNotification.events), es.definition.ID)
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
			if !bi.blocksSinceCheckpoint[len(bi.blocksSinceCheckpoint)-1].Hash.Equals(nextBlock.ParentHash) {
				// This doesn't attach to the end of our list. Trim it off and try again.
				bi.blocksSinceCheckpoint = bi.blocksSinceCheckpoint[0 : len(bi.blocksSinceCheckpoint)-1]
				return true
			}
		}

		// We successfully attached it
		bi.blocksSinceCheckpoint = append(bi.blocksSinceCheckpoint, nextBlock)
	}
	return true

}

func (bi *blockIndexer) getNextConfirmed() (toDispatch *BlockInfoJSONRPC) {
	bi.stateLock.Lock()
	defer bi.stateLock.Unlock()
	if len(bi.blocksSinceCheckpoint) > bi.requiredConfirmations {
		toDispatch = bi.blocksSinceCheckpoint[0]
		// don't want memory to grow indefinitely by shifting right, so we create a new slice here
		bi.blocksSinceCheckpoint = append([]*BlockInfoJSONRPC{}, bi.blocksSinceCheckpoint[1:]...)
		newCheckpoint := toDispatch.Number + 1
		bi.nextBlock = &newCheckpoint
	}
	return toDispatch
}

func (bi *blockIndexer) WaitForTransaction(ctx context.Context, hash tktypes.Bytes32) (*IndexedTransaction, error) {

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

func (bi *blockIndexer) GetIndexedBlockByNumber(ctx context.Context, number uint64) (*IndexedBlock, error) {
	var blocks []*IndexedBlock
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

func (bi *blockIndexer) GetIndexedTransactionByHash(ctx context.Context, hash tktypes.Bytes32) (*IndexedTransaction, error) {
	return bi.getIndexedTransactionByHash(ctx, hash)
}

func (bi *blockIndexer) getIndexedTransactionByHash(ctx context.Context, hashID tktypes.Bytes32) (*IndexedTransaction, error) {
	var txns []*IndexedTransaction
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

func (bi *blockIndexer) GetBlockTransactionsByNumber(ctx context.Context, blockNumber int64) ([]*IndexedTransaction, error) {
	var txns []*IndexedTransaction
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

func (bi *blockIndexer) GetTransactionEventsByHash(ctx context.Context, hash tktypes.Bytes32) ([]*IndexedEvent, error) {
	var events []*IndexedEvent
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

func (bi *blockIndexer) ListTransactionEvents(ctx context.Context, lastBlock int64, lastIndex, limit int, withTransaction, withBlock bool) ([]*IndexedEvent, error) {
	var events []*IndexedEvent
	db := bi.persistence.DB()
	q := db.
		WithContext(ctx).
		Table("indexed_events").
		Where("indexed_events.block_number > ?", lastBlock).
		Or(db.Where("indexed_events.block_number = ?", lastBlock).Where("indexed_events.log_index > ?", lastIndex)).
		Order("indexed_events.block_number").
		Order("indexed_events.transaction_index").
		Order("indexed_events.log_index").
		Limit(limit)
	if withTransaction {
		q = q.Joins("Transaction")
	}
	if withBlock {
		q = q.Joins("Block")
	}
	err := q.Find(&events).Error
	return events, err
}

func (bi *blockIndexer) DecodeTransactionEvents(ctx context.Context, hash tktypes.Bytes32, abi abi.ABI) ([]*EventWithData, error) {
	events, err := bi.GetTransactionEventsByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	decoded := make([]*EventWithData, len(events))
	for i, event := range events {
		decoded[i] = &EventWithData{IndexedEvent: event}
	}
	err = bi.queryTransactionEvents(ctx, abi, hash[:], decoded)
	return decoded, err
}

func (bi *blockIndexer) queryTransactionEvents(ctx context.Context, abi abi.ABI, tx ethtypes.HexBytes0xPrefix, events []*EventWithData) error {
	// Get the TX receipt with all the logs
	var receipt *TXReceiptJSONRPC
	err := bi.retry.Do(ctx, func(attempt int) (retryable bool, err error) {
		log.L(ctx).Debugf("Fetching transaction receipt by hash %s", tx)
		rpcErr := bi.wsConn.CallRPC(ctx, &receipt, "eth_getTransactionReceipt", tx)
		if rpcErr != nil {
			return true, rpcErr.Error()
		}
		if receipt == nil {
			return true, i18n.NewError(ctx, msgs.MsgBlockIndexerConfirmedReceiptNotFound, tx)
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	// Spin through the logs to find the corresponding result entries
	for _, l := range receipt.Logs {
		for _, e := range events {
			if ethtypes.HexUint64(e.LogIndex) == l.LogIndex {
				bi.matchLog(ctx, abi, l, e, nil)
			}
		}
	}
	return nil
}

func (bi *blockIndexer) matchLog(ctx context.Context, abi abi.ABI, in *LogJSONRPC, out *EventWithData, source *tktypes.EthAddress) {
	if in.Address != nil && !source.IsZero() && !source.Equals((*tktypes.EthAddress)(in.Address)) {
		return
	}
	// This is one that matches our signature, but we need to check it against our ABI list.
	// We stop at the first entry that parses it, and it's perfectly fine and expected that
	// none will (because Eth signatures are not precise enough to distinguish events -
	// particularly the "indexed" settings on parameters)
	for _, abiEntry := range abi {
		cv, err := abiEntry.DecodeEventDataCtx(ctx, in.Topics, in.Data)
		if err == nil {
			out.SoliditySignature = abiEntry.SolString() // uniquely identifies this ABI entry for the event stream consumer
			out.Data, err = tktypes.StandardABISerializer().SerializeJSONCtx(ctx, cv)
		}
		if err == nil {
			log.L(ctx).Debugf("Event %d/%d/%d matches ABI event %s (tx=%s,address=%s)", in.BlockNumber, in.TransactionIndex, in.LogIndex, abiEntry, in.TransactionHash, in.Address)
			if in.Address != nil {
				out.Address = tktypes.EthAddress(*in.Address)
			}
			return
		} else {
			log.L(ctx).Debugf("Event %d/%d/%d does not match ABI event %s (tx=%s,address=%s): %s", in.BlockNumber, in.TransactionIndex, in.LogIndex, abiEntry, in.TransactionHash, in.Address, err)
		}
	}
}
