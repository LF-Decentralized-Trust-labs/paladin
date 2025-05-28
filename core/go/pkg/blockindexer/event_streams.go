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
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/sdk/go/pkg/query"

	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"gorm.io/gorm/clause"
)

type eventStream struct {
	ctx               context.Context
	cancelCtx         context.CancelFunc
	bi                *blockIndexer
	definition        *EventStream
	signatures        map[string]bool
	signatureList     []pldtypes.Bytes32
	batchSize         int
	batchTimeout      time.Duration
	blocks            chan *eventStreamBlock
	dispatch          chan *eventDispatch
	useNOTXHandler    bool
	handlerDBTX       InternalStreamCallbackDBTX
	handlerNOTX       InternalStreamCallbackNOTX
	serializer        *abi.Serializer
	detectorDone      chan struct{}
	detectorStarted   chan struct{}
	dispatcherDone    chan struct{}
	dispatcherStarted chan struct{}
	fromBlock         *ethtypes.HexUint64 // nil == latest
	checkpoint        atomic.Int64        // set after we persist checkpoint
	catchup           atomic.Bool
}

type eventBatch struct {
	EventDeliveryBatch
	checkpointAfterBatch int64
	opened               time.Time
	timeoutContext       context.Context
	timeoutCancel        context.CancelFunc
}

type eventDispatch struct {
	event       *pldapi.EventWithData
	lastInBlock bool
}

// event streams get notified of every confirmed block to process the data in that block,
// or simply update their checkpoint. They might fall behind and need to to query the
// database to catch up.
type eventStreamBlock struct {
	block  *BlockInfoJSONRPC
	events []*LogJSONRPC // only the ones that match signatures we've registered an interest in due to our ABI
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
		bi.initEventStream(ctx, esDefinition)
	}
	return nil
}

func (bi *blockIndexer) AddEventStream(ctx context.Context, dbTX persistence.DBTX, stream *InternalEventStream) (*EventStream, error) {
	es, err := bi.upsertInternalEventStream(ctx, dbTX, stream)
	if err != nil {
		return nil, err
	}

	// Can be called before start as managers start before the block indexer
	bi.stateLock.Lock()
	blockIndexerStarted := bi.started
	bi.stateLock.Unlock()

	es.definition.Started = confutil.P(es.definition.Started == nil || *es.definition.Started)

	if blockIndexerStarted && *es.definition.Started {
		// no possibility of error if not updating DB
		_ = bi.startEventStream(es, false)
	}
	return es.definition, nil
}

func (bi *blockIndexer) upsertInternalEventStream(ctx context.Context, dbTX persistence.DBTX, ies *InternalEventStream) (*eventStream, error) {

	// Defensive coding against panics
	def := ies.Definition
	if def == nil {
		def = &EventStream{}
	}

	if def.Type == "" {
		def.Type = EventStreamTypeInternal.Enum()
	}

	// Validate the name
	if err := pldtypes.ValidateSafeCharsStartEndAlphaNum(ctx, def.Name, pldtypes.DefaultNameMaxLen, "name"); err != nil {
		return nil, err
	}

	// Validate the fromBlock
	if _, err := bi.getFromBlock(ctx, def.Config.FromBlock, EventStreamDefaults.FromBlock); err != nil {
		return nil, err
	}

	// Find if one exists - as we need to check it matches, and get its uuid
	var existing []*EventStream
	err := dbTX.DB().
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
		if len(existing[0].Sources) != len(def.Sources) {
			return nil, i18n.NewError(ctx, msgs.MsgBlockIndexerESSourceError)
		}
		for i := range existing[0].Sources {
			if err := pldtypes.ABIsMustMatch(ctx, existing[0].Sources[i].ABI, def.Sources[i].ABI,
				abi.Event, // we only need to compare on the events
			); err != nil {
				return nil, err
			}
			if !existing[0].Sources[i].Address.Equals(def.Sources[i].Address) {
				return nil, i18n.NewError(ctx, msgs.MsgBlockIndexerESSourceError)
			}
		}
		def.ID = existing[0].ID
		// Update in the DB so we store the latest config
		// only the config can be updated. In particular the
		// "Source" is immutable after creation
		err := dbTX.DB().
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
		err := dbTX.DB().
			Table("event_streams").
			WithContext(ctx).
			Create(def).
			Error
		if err != nil {
			return nil, err
		}
	}

	if ies.Type == IESTypeEventStreamDBTX {
		return bi.initEventStreamDBTX(ctx, def, ies.HandlerDBTX), nil
	}
	return bi.initEventStreamNOTX(ctx, def, ies.HandlerNOTX), nil
}

func (bi *blockIndexer) initEventStreamNOTX(ctx context.Context, definition *EventStream, handlerNOTX InternalStreamCallbackNOTX) *eventStream {
	bi.eventStreamsLock.Lock()
	defer bi.eventStreamsLock.Unlock()

	es := bi.initEventStream(ctx, definition)
	es.useNOTXHandler = true
	es.handlerNOTX = handlerNOTX

	return es
}

func (bi *blockIndexer) initEventStreamDBTX(ctx context.Context, definition *EventStream, handlerDBTX InternalStreamCallbackDBTX) *eventStream {
	bi.eventStreamsLock.Lock()
	defer bi.eventStreamsLock.Unlock()

	es := bi.initEventStream(ctx, definition)
	es.handlerDBTX = handlerDBTX

	return es
}

// Note that the event stream must be stopped when this is called
func (bi *blockIndexer) initEventStream(ctx context.Context, definition *EventStream) *eventStream {
	es := bi.eventStreams[definition.ID]
	batchSize := confutil.IntMin(definition.Config.BatchSize, 1, *EventStreamDefaults.BatchSize)
	if es != nil {
		// If we're already initialized, the only thing that can be changed is the pldconf.
		// Caller is responsible for ensuring we're stopped at this point
		es.definition.Config = definition.Config
	} else {
		es = &eventStream{
			bi:         bi,
			definition: definition,
			signatures: make(map[string]bool),
			blocks:     make(chan *eventStreamBlock, bi.esBlockDispatchQueueLength),
			dispatch:   make(chan *eventDispatch, batchSize),
			serializer: definition.Format.GetABISerializerIgnoreErrors(ctx),
		}
	}

	// Set the batch config
	es.batchSize = batchSize
	es.batchTimeout = confutil.DurationMin(definition.Config.BatchTimeout, 0, *EventStreamDefaults.BatchTimeout)
	// The error is already checked before writing to the DB
	es.fromBlock, _ = es.bi.getFromBlock(ctx, definition.Config.FromBlock, EventStreamDefaults.FromBlock)
	es.checkpoint.Store(-1)
	es.catchup.Store(true)

	// Calculate all the signatures we require
	for _, source := range definition.Sources {
		solStrings := []string{}
		location := "*"
		if source.Address != nil {
			location = source.Address.String()
		}

		for _, abiEntry := range source.ABI {
			if abiEntry.Type == abi.Event {
				sig := pldtypes.NewBytes32FromSlice(abiEntry.SignatureHashBytes())
				sigStr := sig.String()
				if _, dup := es.signatures[sigStr]; !dup {
					es.signatures[sigStr] = true
					solStrings = append(solStrings, abiEntry.SolString())
					es.signatureList = append(es.signatureList, sig)
				}
			}
		}

		log.L(ctx).Infof("Event stream %s configured address=%s events=%s", es.definition.ID, location, solStrings)
	}

	// ok - all looks good, put ourselves in the blockindexer list
	bi.eventStreams[definition.ID] = es
	return es
}

func (bi *blockIndexer) RemoveEventStream(ctx context.Context, id uuid.UUID) error {
	bi.eventStreamsLock.Lock()
	defer bi.eventStreamsLock.Unlock()

	es := bi.eventStreams[id]
	if es == nil {
		return i18n.NewError(ctx, msgs.MsgBlockIndexerEventStreamNotFound, id)
	}

	err := bi.persistence.NOTX().DB().
		WithContext(ctx).
		Table("event_streams").
		Where("id = ?", id).
		Delete(&EventStream{}).
		Error
	if err != nil {
		log.L(ctx).Errorf("Failed to delete event stream %s: %s", id, err)
		return err
	}
	// no possibility of error if not updating DB
	_ = es.stop(false)
	delete(bi.eventStreams, id)
	delete(bi.eventStreamsHeadSet, id)
	return nil
}

func (bi *blockIndexer) QueryEventStreamDefinitions(ctx context.Context, dbTX persistence.DBTX, esType pldtypes.Enum[EventStreamType], jq *query.QueryJSON) ([]*EventStream, error) {
	if jq == nil || jq.Limit == nil || *jq.Limit == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgBlockIndexerLimitRequired)
	}
	q := dbTX.DB().
		Table("event_streams").
		WithContext(ctx).
		Where("type = ?", esType)

	q = filters.BuildGORM(ctx, jq, q, EventStreamFilters)

	var results []*EventStream
	err := q.Find(&results).Error
	return results, err
}

func (bi *blockIndexer) StartEventStream(ctx context.Context, id uuid.UUID) error {
	if bi.eventStreams[id] == nil {
		return i18n.NewError(ctx, msgs.MsgBlockIndexerEventStreamNotFound, id)
	}
	return bi.startEventStream(bi.eventStreams[id], true)
}

func (bi *blockIndexer) StopEventStream(ctx context.Context, id uuid.UUID) error {
	bi.eventStreamsLock.Lock()
	defer bi.eventStreamsLock.Unlock()

	if bi.eventStreams[id] == nil {
		return i18n.NewError(ctx, msgs.MsgBlockIndexerEventStreamNotFound, id)
	}
	return bi.eventStreams[id].stop(true)
}

func (bi *blockIndexer) GetEventStreamStatus(ctx context.Context, id uuid.UUID) (*EventStreamStatus, error) {
	bi.eventStreamsLock.Lock()
	defer bi.eventStreamsLock.Unlock()

	if bi.eventStreams[id] == nil {
		return nil, i18n.NewError(ctx, msgs.MsgBlockIndexerEventStreamNotFound, id)
	}
	return &EventStreamStatus{
		CheckpointBlock: bi.eventStreams[id].checkpoint.Load(),
		Catchup:         bi.eventStreams[id].catchup.Load(),
	}, nil
}

func (bi *blockIndexer) getStreamList() []*eventStream {
	bi.eventStreamsLock.Lock()
	defer bi.eventStreamsLock.Unlock()
	streams := make([]*eventStream, 0, len(bi.eventStreams))
	for _, es := range bi.eventStreams {
		streams = append(streams, es)
	}
	return streams
}

func (bi *blockIndexer) startEventStreams() {
	for _, es := range bi.getStreamList() {
		if es.definition.Started == nil || *es.definition.Started {
			// no possibility of error if not updating DB
			_ = es.start(false)
		}
	}
}

func (bi *blockIndexer) startEventStream(es *eventStream, updateDB bool) error {
	bi.eventStreamsLock.Lock()
	defer bi.eventStreamsLock.Unlock()
	return es.start(updateDB)
}

func (es *eventStream) start(updateDB bool) error {
	if (es.handlerDBTX != nil || es.handlerNOTX != nil) && es.detectorDone == nil && es.dispatcherDone == nil {
		es.ctx, es.cancelCtx = context.WithCancel(log.WithLogField(es.bi.parentCtxForReset, "eventstream", es.definition.ID.String()))
		log.L(es.ctx).Infof("Starting event stream %s [%s]", es.definition.Name, es.definition.ID)
		if updateDB {
			err := es.bi.persistence.NOTX().DB().
				WithContext(es.ctx).
				Table("event_streams").
				Where("id = ?", es.definition.ID).
				Update("started", true).
				Error
			if err != nil {
				return err
			}
		}
		es.detectorDone = make(chan struct{})
		es.dispatcherDone = make(chan struct{})
		es.detectorStarted = make(chan struct{})
		es.dispatcherStarted = make(chan struct{})
		go es.detector()
		go es.dispatcher()
	}
	return nil
}

func (es *eventStream) stop(updateDB bool) error {
	if updateDB {
		err := es.bi.persistence.NOTX().DB().
			WithContext(es.ctx).
			Table("event_streams").
			Where("id = ?", es.definition.ID).
			Update("started", false).
			Error
		if err != nil {
			return err
		}
	}
	if es.cancelCtx != nil {
		es.cancelCtx()
		es.cancelCtx = nil
	}
	if es.detectorDone != nil {
		<-es.detectorDone
		es.detectorDone = nil
	}
	if es.dispatcherDone != nil {
		<-es.dispatcherDone
		es.dispatcherDone = nil
	}
	return nil
}

func (es *eventStream) readDBCheckpoint() (*int64, error) {
	var checkpoints []*EventStreamCheckpoint
	err := es.bi.persistence.DB().
		Table("event_stream_checkpoints").
		Where("stream = ?", es.definition.ID).
		WithContext(es.ctx).
		Find(&checkpoints).
		Error
	if err != nil {
		return nil, err
	}

	if len(checkpoints) < 1 {
		// this is the case where we are starting up for the first time so we need to look
		// at the value of fromBlock to decide what to do here
		if es.fromBlock == nil {
			log.L(es.ctx).Info("Using event stream config 'latest' as initial checkpoint")
			return nil, nil
		}
		log.L(es.ctx).Infof("Using event stream config '%d' minus 1 as initial checkpoint", *es.fromBlock)
		return confutil.P(int64(*es.fromBlock) - 1), nil
	}

	baseBlock := checkpoints[0].BlockNumber
	es.checkpoint.Store(baseBlock)
	log.L(es.ctx).Infof("read persisted checkpoint block %d", baseBlock)
	return &baseBlock, nil
}

func (es *eventStream) processCheckpoint() (baseBlock *int64, err error) {
	err = es.bi.retry.Do(es.ctx, func(attempt int) (retryable bool, err error) {
		baseBlock, err = es.readDBCheckpoint()
		return true, err
	})
	return baseBlock, err
}

func (bi *blockIndexer) getHighestIndexedBlock(ctx context.Context) (*int64, error) {
	var blocks []*pldapi.IndexedBlock
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

	log.L(es.ctx).Debugf("Detector started for event stream %s [%s]", es.definition.Name, es.definition.ID)

	// This routine reads the checkpoint on startup, and maintains its view in memory,
	// but never writes it back.
	// The checkpoint is updated on the dispatcher after each batch is confirmed downstream.
	checkpointBlock, err := es.processCheckpoint()
	if err != nil {
		log.L(es.ctx).Debugf("exiting before retrieving checkpoint")
		close(es.detectorStarted)
		return
	}

	// indicate that the detector has started
	close(es.detectorStarted)

	// var checkpointBlock int64
	var startupBlock *int64
	var lastCatchupEvent *pldapi.IndexedEvent
	var catchUpToBlock *eventStreamBlock

	if checkpointBlock == nil {
		// the event stream is starting from the latest block
		// wait here for the first block to be read and processed so that we have a checkpoint to
		// use in later logic for understanding whether we are in catchup mode or not
		es.catchup.Store(false)
		select {
		case block := <-es.blocks:
			checkpointBlock = confutil.P(int64(block.block.Number))
			es.processNotifiedBlock(block, true)
		case <-es.ctx.Done():
			log.L(es.ctx).Debugf("exiting")
			return
		}
	} else {
		// Find the highest block of the chain that's been persisted so far on startup,
		// so we don't need to wait for a block to be mined (which feasibly might be
		// mine-on-demand) to kick off our catchup.
		// Note startupBlock might be nil, and that's fine
		startupBlock, err = es.bi.getHighestIndexedBlock(es.ctx)
		if err != nil {
			log.L(es.ctx).Debugf("exiting before retrieving highest block")
			return
		}
	}

	for {
		// we wait to be told about a block from the chain, to see whether that is a block that
		// slots directly after our checkpoint. Under normal operation when we're caught up
		// this should be the case.
		// It's only if we fall more than the channel length behind the head that we
		// need to enter catchup mode until we make it back again
		if startupBlock == nil && catchUpToBlock == nil {
			select {
			case block := <-es.blocks:
				if int64(block.block.Number) <= *checkpointBlock {
					log.L(es.ctx).Debugf("notified of block %d at or behind checkpoint %d", block.block.Number, checkpointBlock)
					continue
				}
				if block.block.Number == ethtypes.HexUint64(*checkpointBlock+1) {
					// Happy place
					checkpointBlock = confutil.P(int64(block.block.Number))
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
			es.catchup.Store(true)
			// Get a page of events from the DB
			var catchUpToBlockNumber int64
			if startupBlock != nil {
				catchUpToBlockNumber = *startupBlock + 1
			} else {
				catchUpToBlockNumber = int64(catchUpToBlock.block.Number)
			}
			var caughtUp bool
			caughtUp, lastCatchupEvent, err = es.processCatchupEventPage(lastCatchupEvent, *checkpointBlock, catchUpToBlockNumber)
			if err != nil {
				log.L(es.ctx).Debugf("exiting during catchup phase")
				return
			}
			if caughtUp {
				es.catchup.Store(false)
				lastCatchupEvent = nil
				if startupBlock == nil {
					// Process the deferred notified block, and back to normal operation
					es.processNotifiedBlock(catchUpToBlock, true)
					checkpointBlock = confutil.P(int64(catchUpToBlock.block.Number))
					catchUpToBlock = nil
				} else {
					// We've now started
					checkpointBlock = startupBlock
					startupBlock = nil
				}
			}
		}
	}
}

func (es *eventStream) processNotifiedBlock(block *eventStreamBlock, fullBlock bool) {
	for i, l := range block.events {
		indexedEvent := es.bi.logToIndexedEvent(l)
		indexedEvent.Block = es.bi.blockInfoToIndexedBlock(block.block)
		event := &pldapi.EventWithData{
			IndexedEvent: indexedEvent,
		}
		// Only dispatch events that were completed by the validation against our ABI
		for _, source := range es.definition.Sources {
			if es.bi.matchLog(es.ctx, source.ABI, l, event, source.Address, es.serializer) {
				es.sendToDispatcher(event,
					// Can only move checkpoint past this block once we know we've processed the last one
					fullBlock && i == (len(block.events)-1))
				break
			}
		}
	}
}

func (es *eventStream) sendToDispatcher(event *pldapi.EventWithData, lastInBlock bool) {
	log.L(es.ctx).Debugf("passing event to dispatcher %d/%d/%d (tx=%s,address=%s)", event.BlockNumber, event.TransactionIndex, event.LogIndex, event.TransactionHash, &event.Address)
	select {
	case es.dispatch <- &eventDispatch{event, lastInBlock}:
	case <-es.ctx.Done():
	}
}

func (es *eventStream) dispatcher() {
	defer close(es.dispatcherDone)
	close(es.dispatcherStarted)

	log.L(es.ctx).Debugf("Dispatcher started for event stream %s [%s]", es.definition.Name, es.definition.ID)

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

func (es *eventStream) updateCheckpoint(ctx context.Context, dbTX persistence.DBTX, blockNumber int64) error {
	err := dbTX.DB().
		WithContext(ctx).
		Table("event_stream_checkpoints").
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "stream"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"block_number",
			}),
		}).
		Create(&EventStreamCheckpoint{
			Stream:      es.definition.ID,
			BlockNumber: blockNumber,
		}).
		Error
	if err == nil {
		es.checkpoint.Store(blockNumber)
	}
	return err
}

func (es *eventStream) runBatch(batch *eventBatch) error {
	return es.bi.retry.Do(es.ctx, func(attempt int) (retryable bool, err error) {
		if es.useNOTXHandler {
			err = es.handlerNOTX(es.ctx, &batch.EventDeliveryBatch)
			if err == nil {
				err = es.updateCheckpoint(es.ctx, es.bi.persistence.NOTX(), int64(batch.checkpointAfterBatch))
			}
			return true, err
		}
		err = es.bi.persistence.Transaction(es.ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
			err = es.handlerDBTX(ctx, dbTX, &batch.EventDeliveryBatch)
			if err == nil {
				err = es.updateCheckpoint(ctx, dbTX, int64(batch.checkpointAfterBatch))
			}
			return err
		})
		return true, err
	})
}

func (es *eventStream) processCatchupEventPage(lastCatchupEvent *pldapi.IndexedEvent, checkpointBlock int64, catchUpToBlockNumber int64) (caughtUp bool, lastEvent *pldapi.IndexedEvent, err error) {

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
	var page []*pldapi.IndexedEvent
	err = es.bi.retry.Do(es.ctx, func(attempt int) (retryable bool, err error) {
		db := es.bi.persistence.DB()
		q := db.
			Table("indexed_events").
			Joins("Block").
			Where("indexed_events.signature IN (?)", es.signatureList).
			Where("indexed_events.block_number < ?", catchUpToBlockNumber)
		if lastCatchupEvent == nil {
			q = q.Where("indexed_events.block_number > ?", checkpointBlock)
		} else {
			q = q.Where("indexed_events.block_number > ? OR (indexed_events.block_number = ? AND (indexed_events.transaction_index > ? OR (indexed_events.transaction_index = ? AND indexed_events.log_index > ?)))",
				lastCatchupEvent.BlockNumber, lastCatchupEvent.BlockNumber,
				lastCatchupEvent.TransactionIndex, lastCatchupEvent.TransactionIndex,
				lastCatchupEvent.LogIndex,
			)
		}
		return true, q.Order("indexed_events.block_number").Order("indexed_events.transaction_index").Order("indexed_events.log_index").
			Limit(pageSize).
			Find(&page).
			Error
	})
	if err != nil {
		// context cancelled
		return false, nil, err
	}
	if len(page) == 0 {
		// nothing to report - we're caught up
		return true, nil, nil
	}
	caughtUp = (len(page) < pageSize)

	// Because we're in catch up here, we have to query the chain ourselves for the receipts.
	// That's done by transaction (not by event) - so we've got to group
	byTxID := make(map[string][]*pldapi.EventWithData)
	for _, event := range page {
		byTxID[event.TransactionHash.String()] = append(byTxID[event.TransactionHash.String()], &pldapi.EventWithData{
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
		tx := pldtypes.MustParseBytes32(txStr)
		for _, _source := range es.definition.Sources {
			source := _source // not safe to pass loop pointer
			go func() {
				enrichments <- es.bi.enrichTransactionEvents(es.ctx, source.ABI, source.Address, tx, events, es.serializer, true /* retry indefinitely */)
			}()
		}
	}
	// Collect all the results
	for range byTxID {
		for range es.definition.Sources {
			txErr := <-enrichments
			if txErr != nil && err == nil {
				err = txErr
			}
		}
	}
	if err != nil {
		// context cancelled
		return false, nil, err
	}

	// Now reconstruct the final set in the original order, but only where we
	// successfully extracted the event data
	for iPage, origEntry := range page {
		lastEvent = origEntry // need to keep track of the last we saw for the looping round this code
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
	return caughtUp, lastEvent, nil

}
