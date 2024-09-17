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
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/internal/msgs"

	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type eventStream struct {
	ctx            context.Context
	cancelCtx      context.CancelFunc
	bi             *blockIndexer
	definition     *EventStream
	signatures     map[string]bool
	signatureList  []tktypes.Bytes32
	eventABIs      []*abi.Entry
	batchSize      int
	batchTimeout   time.Duration
	blocks         chan *eventStreamBlock
	dispatch       chan *eventDispatch
	handlerLock    sync.Mutex
	waitForHandler chan struct{}
	handler        InternalStreamCallback
	detectorDone   chan struct{}
	dispatcherDone chan struct{}
}

type eventBatch struct {
	EventDeliveryBatch
	checkpointAfterBatch int64
	opened               time.Time
	timeoutContext       context.Context
	timeoutCancel        context.CancelFunc
}

type eventDispatch struct {
	event       *EventWithData
	lastInBlock bool
}

// event streams get notified of every confirmed block to process the data in that block,
// or simply update their checkpoint. They might fall behind and need to to query the
// database to catch up.
type eventStreamBlock struct {
	blockNumber uint64
	events      []*LogJSONRPC // only the ones that match signatures we've registered an interest in due to our ABI
}

func (bi *blockIndexer) loadEventStreams(ctx context.Context) error {

	// Paladin is optimized for a relatively small number of event streams
	// We hold all event streams in memory, as we process all of them against every block.
	var eventStreams []*EventStream
	err := bi.persistence.DB().
		Table("event_streams").
		WithContext(ctx).
		Find(&eventStreams).
		Error
	if err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgBlockIndexerESInitFail)
	}

	for _, esDefinition := range eventStreams {
		bi.initEventStream(esDefinition)
	}
	return nil
}

func (bi *blockIndexer) upsertInternalEventStream(ctx context.Context, ies *InternalEventStream) (*eventStream, error) {

	// Defensive coding against panics
	def := ies.Definition
	if def == nil {
		def = &EventStream{}
	}

	// This will need to open up when we have more externally consumable event streams
	def.Type = EventStreamTypeInternal.Enum()

	// Validate the name
	if err := tktypes.ValidateSafeCharsStartEndAlphaNum(ctx, def.Name, tktypes.DefaultNameMaxLen, "name"); err != nil {
		return nil, err
	}

	// Find if one exists - as we need to check it matches, and get its uuid
	var existing []*EventStream
	err := bi.persistence.DB().
		Table("event_streams").
		Where("type = ?", def.Type).
		Where("name = ?", def.Name).
		WithContext(ctx).
		Find(&existing).
		Error
	if err != nil {
		return nil, err
	}

	if len(existing) > 0 {
		// The event definitions in both events must be identical
		// We do not support changing the ABI after creation
		if err := tktypes.ABIsMustMatch(ctx, existing[0].ABI, def.ABI); err != nil {
			return nil, err
		}
		if !existing[0].Source.Equals(def.Source) {
			return nil, i18n.NewError(ctx, msgs.MsgBlockIndexerESSourceError)
		}
		def.ID = existing[0].ID
		// Update in the DB so we store the latest config
		// only the config can be updated. In particular the
		// "Source" is immutable after creation
		err := bi.persistence.DB().
			Table("event_streams").
			Where("type = ?", def.Type).
			Where("name = ?", def.Name).
			WithContext(ctx).
			Updates(&EventStream{Config: def.Config}).
			Error
		if err != nil {
			return nil, err
		}
	} else {
		// Otherwise we're just creating
		def.ID = uuid.New()
		err := bi.persistence.DB().
			Table("event_streams").
			WithContext(ctx).
			Create(def).
			Error
		if err != nil {
			return nil, err
		}
	}

	// We call init here
	// TODO: Full stop/start lifecycle
	es := bi.initEventStream(def)

	// Register the internal handler against the new or existing stream
	es.attachHandler(ies.Handler)

	return es, nil
}

func (es *eventStream) attachHandler(handler InternalStreamCallback) {
	es.handlerLock.Lock()
	prevHandler := es.handler
	es.handler = handler
	if prevHandler == nil {
		close(es.waitForHandler)
	}
	es.handlerLock.Unlock()
}

func (bi *blockIndexer) initEventStream(definition *EventStream) *eventStream {
	bi.eventStreamsLock.Lock()
	defer bi.eventStreamsLock.Unlock()

	es := bi.eventStreams[definition.ID]
	batchSize := confutil.IntMin(definition.Config.BatchSize, 1, *EventStreamDefaults.BatchSize)
	if es != nil {
		// If we're already initialized, the only thing that can be changed is the config.
		// Caller is responsible for ensuring we're stopped at this point
		es.definition.Config = definition.Config
	} else {
		es = &eventStream{
			bi:             bi,
			definition:     definition,
			eventABIs:      []*abi.Entry{},
			signatures:     make(map[string]bool),
			blocks:         make(chan *eventStreamBlock, bi.esBlockDispatchQueueLength),
			dispatch:       make(chan *eventDispatch, batchSize),
			waitForHandler: make(chan struct{}),
		}
	}

	// Set the batch config
	es.batchSize = batchSize
	es.batchTimeout = confutil.DurationMin(definition.Config.BatchTimeout, 0, *EventStreamDefaults.BatchTimeout)

	// Calculate all the signatures we require
	for _, abiEntry := range definition.ABI {
		if abiEntry.Type == abi.Event {
			sig := tktypes.NewBytes32FromSlice(abiEntry.SignatureHashBytes())
			sigStr := sig.String()
			es.eventABIs = append(es.eventABIs, abiEntry)
			if _, dup := es.signatures[sigStr]; !dup {
				es.signatures[sigStr] = true
				es.signatureList = append(es.signatureList, sig)
			}
		}
	}

	// ok - all looks good, put ourselves in the blockindexer list
	bi.eventStreams[definition.ID] = es
	return es
}

func (bi *blockIndexer) startEventStreams() {
	bi.eventStreamsLock.Lock()
	defer bi.eventStreamsLock.Unlock()
	for _, es := range bi.eventStreams {
		es.start()
	}
}

func (es *eventStream) start() {
	if es.detectorDone == nil && es.dispatcherDone == nil {
		es.ctx, es.cancelCtx = context.WithCancel(log.WithLogField(es.bi.parentCtxForReset, "eventstream", es.definition.ID.String()))
		es.detectorDone = make(chan struct{})
		es.dispatcherDone = make(chan struct{})
		es.run()
	}
}

func (es *eventStream) run() {

	select {
	case <-es.waitForHandler:
	case <-es.ctx.Done():
		log.L(es.ctx).Debugf("stopping before event handler registered")
		close(es.detectorDone)
		close(es.dispatcherDone)
		return
	}

	go es.detector()
	go es.dispatcher()

}

func (es *eventStream) stop() {
	if es.cancelCtx != nil {
		es.cancelCtx()
	}
	if es.detectorDone != nil {
		<-es.detectorDone
	}
	if es.dispatcherDone != nil {
		<-es.dispatcherDone
	}
}

func (es *eventStream) processCheckpoint() (int64, error) {
	var checkpoints []*EventStreamCheckpoint
	err := es.bi.retry.Do(es.ctx, func(attempt int) (retryable bool, err error) {
		return true, es.bi.persistence.DB().
			Table("event_stream_checkpoints").
			Where("stream = ?", es.definition.ID).
			WithContext(es.ctx).
			Find(&checkpoints).
			Error
	})
	if err != nil {
		return -1, err
	}
	baseBlock := int64(-1)
	if len(checkpoints) > 0 {
		baseBlock = checkpoints[0].BlockNumber
		log.L(es.ctx).Infof("starting from checkpoint block %d", baseBlock)
	}
	return baseBlock, nil
}

func (bi *blockIndexer) getHighestIndexedBlock(ctx context.Context) (*int64, error) {
	var blocks []*IndexedBlock
	err := bi.retry.Do(ctx, func(attempt int) (retryable bool, err error) {
		return true, bi.persistence.DB().
			Table("indexed_blocks").
			Order("number DESC").
			Limit(1).
			WithContext(ctx).
			Find(&blocks).
			Error
	})
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return nil, nil
	}
	highestIndexedBlock := blocks[0].Number
	return &highestIndexedBlock, nil
}

func (es *eventStream) detector() {
	defer close(es.detectorDone)

	// This routine reads the checkpoint on startup, and maintains its view in memory,
	// but never writes it back.
	// The checkpoint is updated on the dispatcher after each batch is confirmed downstream.
	checkpointBlock, err := es.processCheckpoint()
	if err != nil {
		log.L(es.ctx).Debugf("exiting before retrieving checkpoint")
		return
	}

	// Find the highest block of the chain that's been persisted so far on startup,
	// so we don't need to wait for a block to be mined (which feasibly might be
	// mine-on-demand) to kick off our catchup.
	// Note startupBlock might be nil, and that's fine
	startupBlock, err := es.bi.getHighestIndexedBlock(es.ctx)
	if err != nil {
		log.L(es.ctx).Debugf("exiting before retrieving highest block")
		return
	}

	var catchUpToBlock *eventStreamBlock
	for {
		// we wait to be told about a block from the chain, to see whether that is a block that
		// slots directly after our checkpoint. Under normal operation when we're caught up
		// this should be the case.
		// It's only if we fall more than the channel length behind the head that we
		// need to enter catchup mode until we make it back again
		if startupBlock == nil && catchUpToBlock == nil {
			select {
			case block := <-es.blocks:
				if int64(block.blockNumber) <= checkpointBlock {
					log.L(es.ctx).Debugf("notified of block %d at or behind checkpoint %d", block.blockNumber, checkpointBlock)
					continue
				}
				if block.blockNumber == uint64(checkpointBlock+1) {
					// Happy place
					checkpointBlock = int64(block.blockNumber)
					es.processNotifiedBlock(block, true)
				} else {
					// Entering catchup - defer processing of this block until catchup complete,
					// and we won't pick up anything else off the channel until then
					catchUpToBlock = block
				}
			case <-es.ctx.Done():
				log.L(es.ctx).Debugf("exiting")
				return
			}
		} else {
			// Get a page of events from the DB
			var catchUpToBlockNumber int64
			if startupBlock != nil {
				catchUpToBlockNumber = *startupBlock + 1
			} else {
				catchUpToBlockNumber = int64(catchUpToBlock.blockNumber)
			}
			caughtUp, err := es.processCatchupEventPage(checkpointBlock, catchUpToBlockNumber)
			if err != nil {
				log.L(es.ctx).Debugf("exiting during catchup phase")
				return
			}
			if caughtUp {
				if startupBlock == nil {
					// Process the deferred notified block, and back to normal operation
					es.processNotifiedBlock(catchUpToBlock, true)
					checkpointBlock = int64(catchUpToBlock.blockNumber)
					catchUpToBlock = nil
				} else {
					// We've now started
					checkpointBlock = *startupBlock
					startupBlock = nil
				}
			}
		}
	}
}

func (es *eventStream) processNotifiedBlock(block *eventStreamBlock, fullBlock bool) {
	for i, l := range block.events {
		event := &EventWithData{
			IndexedEvent: es.bi.logToIndexedEvent(l),
		}
		es.matchLog(l, event)
		// Only dispatch events that were completed by the validation against our ABI
		if event.Data != nil {
			es.sendToDispatcher(event,
				// Can only move checkpoint past this block once we know we've processed the last one
				fullBlock && i == (len(block.events)-1))
		}
	}
}

func (es *eventStream) sendToDispatcher(event *EventWithData, lastInBlock bool) {
	log.L(es.ctx).Debugf("passing event to dispatcher %d/%d/%d (tx=%s,address=%s)", event.BlockNumber, event.TransactionIndex, event.LogIndex, event.TransactionHash, &event.Address)
	select {
	case es.dispatch <- &eventDispatch{event, lastInBlock}:
	case <-es.ctx.Done():
	}
}

func (es *eventStream) dispatcher() {
	defer close(es.dispatcherDone)

	l := log.L(es.ctx)
	var batch *eventBatch
	for {
		var timeoutContext context.Context
		var timedOut bool
		if batch != nil {
			timeoutContext = batch.timeoutContext
		} else {
			timeoutContext = es.ctx
		}
		select {
		case d := <-es.dispatch:
			if batch == nil {
				batch = &eventBatch{
					EventDeliveryBatch: EventDeliveryBatch{
						StreamID:   es.definition.ID,
						StreamName: es.definition.Name,
						BatchID:    uuid.New(),
					},
					opened: time.Now(),
				}
				batch.timeoutContext, batch.timeoutCancel = context.WithTimeout(es.ctx, es.batchTimeout)
			}
			event := d.event
			if d.lastInBlock {
				// We know we can move our checkpoint now to this block, as we've processed the last event in it
				batch.checkpointAfterBatch = d.event.BlockNumber
			} else if d.event.BlockNumber > 0 {
				// Otherwise we have to set our checkpoint one behind
				batch.checkpointAfterBatch = d.event.BlockNumber - 1
			}
			batch.Events = append(batch.Events, event)
			l.Debugf("Added event %d/%d/%d to batch %s (len=%d)", event.BlockNumber, event.TransactionIndex, event.LogIndex, batch.BatchID, len(batch.Events))
		case <-timeoutContext.Done():
			timedOut = true
			select {
			case <-es.ctx.Done():
				l.Debugf("event stream dispatcher ending")
				return
			default:
			}
		}

		if batch != nil && (timedOut || (len(batch.Events) >= es.batchSize)) {
			batch.timeoutCancel()
			l.Debugf("Running batch %s (len=%d,timeout=%t,age=%dms)", batch.BatchID, len(batch.Events), timedOut, time.Since(batch.opened).Milliseconds())
			if err := es.runBatch(batch); err != nil {
				l.Debugf("event stream dispatcher ending (during dispatch)")
				return
			}
			batch = nil
		}

	}
}

func (es *eventStream) runBatch(batch *eventBatch) error {

	// We start a database transaction, run the callback function
	return es.bi.retry.Do(es.ctx, func(attempt int) (retryable bool, err error) {
		var postCommit PostCommit
		err = es.bi.persistence.DB().Transaction(func(tx *gorm.DB) (err error) {

			es.handlerLock.Lock()
			handler := es.handler
			es.handlerLock.Unlock()

			if handler == nil {
				return i18n.NewError(es.ctx, msgs.MsgBlockMissingHandler)
			}
			postCommit, err = handler(es.ctx, tx, &batch.EventDeliveryBatch)
			if err != nil {
				return err
			}
			// commit the checkpoint
			return tx.
				Table("event_stream_checkpoints").
				Clauses(clause.OnConflict{
					Columns: []clause.Column{{Name: "stream"}},
					DoUpdates: clause.AssignmentColumns([]string{
						"block_number",
					}),
				}).
				Create(&EventStreamCheckpoint{
					Stream:      es.definition.ID,
					BlockNumber: int64(batch.checkpointAfterBatch),
				}).
				WithContext(es.ctx).
				Error
		})
		if err == nil && postCommit != nil {
			postCommit()
		}
		return true, err
	})

}

func (es *eventStream) processCatchupEventPage(checkpointBlock int64, catchUpToBlockNumber int64) (caughtUp bool, err error) {

	// We query up to the head of the chain as currently indexed, with a limit on the events
	// we return for enrichment/processing.
	//
	// The steady state is we find nothing, as the events we need are in the direct memory buffer
	// between the main blockindexer and this event stream.
	//
	// We're only interested in the signatures in our ABI, but we'll still have to check
	// they match as signatures in ethereum are not precise (due to the "indexed" flag not being included)
	// That also means we can do an efficient IN query on the sig H/L
	pageSize := es.bi.esCatchUpQueryPageSize
	var page []*IndexedEvent
	err = es.bi.retry.Do(es.ctx, func(attempt int) (retryable bool, err error) {
		return true, es.bi.persistence.DB().
			Table("indexed_events").
			Where("signature IN (?)", es.signatureList).
			Where("block_number > ?", checkpointBlock).
			Where("block_number < ?", catchUpToBlockNumber).
			Order("block_number").Order("transaction_index").Order("log_index").
			Limit(pageSize).
			Find(&page).
			Error
	})
	if err != nil {
		// context cancelled
		return false, err
	}
	if len(page) == 0 {
		// nothing to report - we're caught up
		return true, nil
	}
	caughtUp = (len(page) < pageSize)

	// Because we're in catch up here, we have to query the chain ourselves for the receipts.
	// That's done by transaction (not by event) - so we've got to group
	byTxID := make(map[string][]*EventWithData)
	for _, event := range page {
		byTxID[event.TransactionHash.String()] = append(byTxID[event.TransactionHash.String()], &EventWithData{
			IndexedEvent: event,
			// Leave Address and Data as that's what we'll fill in, if it works
		})
	}

	// Parallel query for the TXs - note we require the transactions to exist here, because they have been
	// confirmed on the blockchain. So we go into a retry loop if any are not found.
	//
	// If the blockchain is compromised beyond the confirmations length, then the block index needs
	// to be rebuilt. This would be a very significant event in a production network.
	//
	// In early phase dev, it's just about consistently resetting both your chain and your index.
	enrichments := make(chan error)
	for txStr, _events := range byTxID {
		events := _events // not safe to pass loop pointer
		go es.queryTransactionEvents(tktypes.MustParseBytes32(txStr), events, enrichments)
	}
	// Collect all the results
	for range byTxID {
		txErr := <-enrichments
		if txErr != nil && err == nil {
			err = txErr
		}
	}
	if err != nil {
		// context cancelled
		return false, err
	}

	// Now reconstruct the final set in the original order, but only where we
	// successfully extracted the event data
	for iPage, origEntry := range page {
		eventsForTX := byTxID[origEntry.TransactionHash.String()]
		for iEvent, event := range eventsForTX {
			if event.LogIndex == origEntry.LogIndex && event.Data != nil {
				// can only update our checkpoint to the block itself (vs. one before)
				// when we are caught up, and dispatching the last block
				lastInBlock := caughtUp && (iPage == len(page)-1) && (iEvent == len(eventsForTX)-1)
				// Dispatch this event.
				es.sendToDispatcher(event, lastInBlock)
			}
		}
	}
	return caughtUp, nil

}

func (es *eventStream) queryTransactionEvents(tx tktypes.Bytes32, events []*EventWithData, done chan error) {
	done <- es.bi.enrichTransactionEvents(es.ctx, es.eventABIs, tx, events, true /* retry indefinitely */)
}

func (es *eventStream) matchLog(in *LogJSONRPC, out *EventWithData) {
	es.bi.matchLog(es.ctx, es.eventABIs, in, out, es.definition.Source)
}
