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

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/types"
)

type eventStream struct {
	ctx         context.Context
	cancelCtx   context.CancelFunc
	bi          *blockIndexer
	definition  *EventStream
	signatures  []types.HashID
	signaturesH []uuid.UUID
	signaturesL []uuid.UUID
	blocks      chan *eventStreamBlock
	done        chan struct{}
}

const (
	eventStreamQueryPageSize = 100
	eventStreamQueueLength   = 100
)

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
		Find(&eventStreams).
		Error
	if err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgBlockIndexerESInitFail)
	}

	for _, esDefinition := range eventStreams {
		if err = bi.initEventStream(ctx, esDefinition); err != nil {
			return err
		}
	}
	return nil
}

func (bi *blockIndexer) initEventStream(ctx context.Context, definition *EventStream) error {
	bi.eventStreamsLock.Lock()
	defer bi.eventStreamsLock.Unlock()

	if _, alreadyInit := bi.eventStreams[definition.ID]; alreadyInit {
		return i18n.NewError(ctx, msgs.MsgBlockIndexerESAlreadyInit)
	}

	es := &eventStream{
		bi:         bi,
		definition: definition,
		signatures: []types.HashID{},
		blocks:     make(chan *eventStreamBlock, eventStreamQueueLength),
		done:       make(chan struct{}),
	}

	// Calculate all the signatures we require
	for _, abiEntry := range definition.ABI.V() {
		if abiEntry.Type == abi.Event {
			sigStr, err := abiEntry.SignatureCtx(ctx)
			if err != nil {
				return err
			}
			sig := types.MustParseHashID(sigStr)
			dup := false
			for _, existing := range es.signatures {
				if existing.Equals(sig) {
					dup = true
					break
				}
			}
			if !dup {
				es.signatures = append(es.signatures, *sig)
				es.signaturesL = append(es.signaturesL, sig.L)
				es.signaturesH = append(es.signaturesH, sig.H)
			}
		}
	}

	// ok - all looks good, put ourselves in the blockindexer list
	bi.eventStreams[definition.ID] = es
	// and register ourselves against all the signatures we care about
	for _, s := range es.signatures {
		bi.eventStreamSignatures[s.String()] = append(bi.eventStreamSignatures[s.String()], es)
	}

	// start our main routine
	es.ctx, es.cancelCtx = context.WithCancel(log.WithLogField(bi.parentCtxForReset, "eventstream", definition.ID.String()))
	go es.run()
	return nil
}

func (es *eventStream) processCheckpoint() (int64, error) {
	var checkpoints []*EventStreamCheckpoint
	err := es.bi.retry.Do(es.ctx, func(attempt int) (retryable bool, err error) {
		return true, es.bi.persistence.DB().
			Table("event_stream_checkpoints").
			Where("id = ?", es.definition.ID).
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

func (es *eventStream) run() {
	defer close(es.done)

	checkpointBlock, err := es.processCheckpoint()
	if err != nil {
		log.L(es.ctx).Debugf("exiting before retrieving checkpoint")
		return
	}

	lastChainHeadView := int64(-1)
	atHead := false
	for {
		if !atHead {
			// Get a page of events from the DB
		}

	}
}

func (es *eventStream) getCatchupEventPage(baseBlock int64, lastChainHeadView int64) (caughtUp bool, _ []*EventStreamData, err error) {

	// We query up to the head of the chain as currently indexed, with a limit on the events
	// we return for enrichment/processing.
	//
	// The steady state is we find nothing, as the events we need are in the direct memory buffer
	// between the main blockindexer and this event stream.
	//
	// We're only interested in the signatures in our ABI, but we'll still have to check
	// they match as signatures in ethereum are not precise (due to the "indexed" flag not being included)
	// That also means we can do an efficient IN query on the sig H/L
	var page []*IndexedEvent
	err = es.bi.retry.Do(es.ctx, func(attempt int) (retryable bool, err error) {
		query := es.bi.persistence.DB().
			Table("indexed_events").
			Where("signature_l IN (?)", es.signaturesL).
			Where("signature_h IN (?)", es.signaturesH).
			Where("block_number > ?", baseBlock).
			Limit(eventStreamQueueLength)
		if lastChainHeadView >= 0 {
			query = query.Where("block_number <= ?", lastChainHeadView)
		}
		return true, query.Find(&page).Error
	})
	if err != nil || len(page) == 0 {
		// context cancelled, or nothing to report - we're caught up
		return true, nil, err
	}

	// Because we're in catch up here, we have to query the chain ourselves for the receipts.
	// That's done by transaction (not by event) - so we've got to group
	var byTxID map[string][]*EventStreamData
	for _, event := range page {
		byTxID[event.TransactionHash.String()] = append(byTxID[event.TransactionHash.String()], &EventStreamData{
			Stream:      es.definition.ID,
			BlockNumber: event.BlockNumber,
			TXIndex:     event.TXIndex,
			EventIndex:  event.EventIndex,
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
	wg := new(sync.WaitGroup)
	for tx, _events := range byTxID {
		wg.Add(1)
		events := _events // not safe to pass loop pointer
		go es.queryTransactionEvents(tx, events)
	}
	wg.Wait()

}

func (es *eventStream) queryTransactionEvents(tx string, events []*EventStreamData) {

	var receipt *TXReceiptJSONRPC
	es.bi.retry.Do(es.ctx, func(attempt int) (retryable bool, err error) {
		log.L(es.ctx).Debugf("Fetching transaction receipt by hash %s", tx)
		rpcErr := es.bi.wsConn.CallRPC(es.ctx, &receipt, "eth_getTransactionReceipt", tx)
		if rpcErr != nil {
			return true, rpcErr.Error()
		}
		if receipt == nil {
			return true, i18n.NewError(es.ctx, msgs.MsgBlockIndexerConfirmedReceiptNotFound, tx)
		}
		return false, nil
	})

}
