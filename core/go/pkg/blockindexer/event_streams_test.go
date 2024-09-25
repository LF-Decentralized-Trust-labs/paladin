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
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/core/mocks/rpcclientmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func mockBlockListenerNil(mRPC *rpcclientmocks.WSClient) {
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_blockNumber").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*ethtypes.HexUint64)
		*hbh = ethtypes.HexUint64(0)
	}).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_newBlockFilter").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*string)
		*hbh = "filter_id1"
	}).Maybe()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getFilterChanges", "filter_id1").Return(nil).Run(func(args mock.Arguments) {
		hbh := args[1].(*[]ethtypes.HexBytes0xPrefix)
		*hbh = []ethtypes.HexBytes0xPrefix{}
	}).Maybe()
}

func TestInternalEventStreamDeliveryAtHead(t *testing.T) {

	// This test uses a real DB, includes the full block indexer, but simulates the blockchain.
	_, bi, mRPC, blDone := newTestBlockIndexer(t)
	defer blDone()

	// Mock up the block calls to the blockchain for 15 blocks
	blocks, receipts := testBlockArray(t, 15)
	mockBlocksRPCCalls(mRPC, blocks, receipts)
	mockBlockListenerNil(mRPC)

	eventCollector := make(chan *EventWithData)

	// Do a full start now with an internal event listener
	var esID string
	calledPostCommit := false
	err := bi.Start(&InternalEventStream{
		Handler: func(ctx context.Context, tx *gorm.DB, batch *EventDeliveryBatch) (PostCommit, error) {
			if esID == "" {
				esID = batch.StreamID.String()
			} else {
				assert.Equal(t, esID, batch.StreamID.String())
			}
			assert.Equal(t, "unit_test", batch.StreamName)
			assert.Greater(t, len(batch.Events), 0)
			assert.LessOrEqual(t, len(batch.Events), 3)
			for _, e := range batch.Events {
				select {
				case eventCollector <- e:
				case <-ctx.Done():
				}
			}
			return func() { calledPostCommit = true }, nil
		},
		Definition: &EventStream{
			Name: "unit_test",
			Config: EventStreamConfig{
				BatchSize:    confutil.P(3),
				BatchTimeout: confutil.P("5ms"),
			},
			// Listen to two out of three event types
			ABI: abi.ABI{
				testABI[1],
				testABI[2],
			},
		},
	})
	require.NoError(t, err)

	// Expect to get 15 * 2 events (1 TX x 3 Events per block, but we only listen to two)
	for i := 0; i < len(blocks)*2; i++ {
		e := <-eventCollector
		blockNumber := i / 2
		if i%2 == 0 {
			assert.JSONEq(t, fmt.Sprintf(`{
				"intParam1": "%d",
				"strParam2": "event_b_in_block_%d"
			}`, 1000000+blockNumber, blockNumber), string(e.Data))
		} else {
			assert.JSONEq(t, fmt.Sprintf(`{
				"structParam1": {
					"intArrayField": [ "%d", "%d", "%d", "%d", "%d" ],
					"strField": "event_c_in_block_%d"
				}
			}`, 1000+blockNumber, 2000+blockNumber, 3000+blockNumber, 4000+blockNumber, 5000+blockNumber,
				blockNumber), string(e.Data))
		}
	}
	assert.True(t, calledPostCommit)

}

func TestInternalEventStreamDeliveryAtHeadWithSourceAddress(t *testing.T) {

	// This test uses a real DB, includes the full block indexer, but simulates the blockchain.
	_, bi, mRPC, blDone := newTestBlockIndexer(t)
	defer blDone()

	sourceContractAddress := tktypes.MustEthAddress(tktypes.RandHex(20))

	// Mock up the block calls to the blockchain for 15 blocks
	blocks, receipts := testBlockArray(t, 15, *sourceContractAddress.Address0xHex())
	mockBlocksRPCCalls(mRPC, blocks, receipts)
	mockBlockListenerNil(mRPC)

	eventCollector := make(chan *EventWithData)

	definition := &EventStream{
		Name: "unit_test",
		Config: EventStreamConfig{
			BatchSize:    confutil.P(3),
			BatchTimeout: confutil.P("5ms"),
		},
		// Listen to two out of three event types
		ABI: abi.ABI{
			testABI[1],
			testABI[2],
		},
		Source: sourceContractAddress,
	}

	// Do a full start now with an internal event listener
	var esID string
	calledPostCommit := false
	err := bi.Start(&InternalEventStream{
		Handler: func(ctx context.Context, tx *gorm.DB, batch *EventDeliveryBatch) (PostCommit, error) {
			if esID == "" {
				esID = batch.StreamID.String()
			} else {
				assert.Equal(t, esID, batch.StreamID.String())
			}
			assert.Equal(t, "unit_test", batch.StreamName)
			assert.Greater(t, len(batch.Events), 0)
			assert.LessOrEqual(t, len(batch.Events), 3)
			for _, e := range batch.Events {
				select {
				case eventCollector <- e:
				case <-ctx.Done():
				}
			}
			return func() { calledPostCommit = true }, nil
		},
		Definition: definition,
	})
	require.NoError(t, err)

	// Expect to get 15 events. 1 TX x 3 Events per block, but we only have
	// one event matching the expected source address
	for i := 0; i < len(blocks); i++ {
		e := <-eventCollector
		blockNumber := i
		assert.JSONEq(t, fmt.Sprintf(`{
			"intParam1": "%d",
			"strParam2": "event_b_in_block_%d"
		}`, 1000000+blockNumber, blockNumber), string(e.Data))
	}
	assert.True(t, calledPostCommit)

}

func TestInternalEventStreamDeliveryCatchUp(t *testing.T) {

	// This test uses a real DB, includes the full block indexer, but simulates the blockchain.
	ctx, bi, mRPC, done := newTestBlockIndexer(t)
	defer done()

	// Mock up the block calls to the blockchain for 15 blocks
	blocks, receipts := testBlockArray(t, 15)
	mockBlocksRPCCalls(mRPC, blocks, receipts)
	mockBlockListenerNil(mRPC)

	// Set up our handler, even though it won't be driven with anything yet
	eventCollector := make(chan *EventWithData)
	var esID string
	handler := func(ctx context.Context, tx *gorm.DB, batch *EventDeliveryBatch) (PostCommit, error) {
		if esID == "" {
			esID = batch.StreamID.String()
		} else {
			assert.Equal(t, esID, batch.StreamID.String())
		}
		assert.Equal(t, "unit_test", batch.StreamName)
		assert.Greater(t, len(batch.Events), 0)
		assert.LessOrEqual(t, len(batch.Events), 3)
		for _, e := range batch.Events {
			select {
			case eventCollector <- e:
			case <-ctx.Done():
			}
		}
		return nil, nil
	}

	// Do a full start now without a block listener, and wait for the ut notification of all the blocks
	utBatchNotify := make(chan []*IndexedBlock)
	preCommitCount := 0
	err := bi.Start(&InternalEventStream{
		Type: IESTypePreCommitHandler,
		PreCommitHandler: func(ctx context.Context, dbTX *gorm.DB, blocks []*IndexedBlock, transactions []*IndexedTransactionNotify) (PostCommit, error) {
			// Return an error once to drive a retry
			preCommitCount++
			if preCommitCount == 0 {
				return nil, fmt.Errorf("pop")
			}
			return func() {
				utBatchNotify <- blocks
			}, nil
		},
	})
	require.NoError(t, err)
	for i := 0; i < len(blocks); i++ {
		notifyBlocks := <-utBatchNotify
		assert.Len(t, notifyBlocks, 1)
		checkIndexedBlockEqual(t, blocks[i], notifyBlocks[0])
	}

	// Add a listener
	internalESConfig := &EventStream{
		Name: "unit_test",
		Config: EventStreamConfig{
			BatchSize:    confutil.P(3),
			BatchTimeout: confutil.P("5ms"),
		},
		// Listen to two out of three event types
		ABI: abi.ABI{
			testABI[1],
			testABI[2],
		},
	}
	_, err = bi.AddEventStream(ctx, &InternalEventStream{
		Definition: internalESConfig,
		Handler:    handler,
	})
	require.NoError(t, err)

	// Expect to get 15 * 2 events (1 TX x 3 Events per block, but we only listen to two)
	for i := 0; i < len(blocks)*2; i++ {
		e := <-eventCollector
		blockNumber := i / 2
		if i%2 == 0 {
			assert.JSONEq(t, fmt.Sprintf(`{
					"intParam1": "%d",
					"strParam2": "event_b_in_block_%d"
				}`, 1000000+blockNumber, blockNumber), string(e.Data))
		} else {
			assert.JSONEq(t, fmt.Sprintf(`{
					"structParam1": {
						"intArrayField": [ "%d", "%d", "%d", "%d", "%d" ],
						"strField": "event_c_in_block_%d"
					}
				}`, 1000+blockNumber, 2000+blockNumber, 3000+blockNumber, 4000+blockNumber, 5000+blockNumber,
				blockNumber), string(e.Data))
		}
	}

	// Stop and restart
	bi.Stop()

	bi, err = newBlockIndexer(ctx, &Config{
		CommitBatchSize: confutil.P(1),
		FromBlock:       tktypes.RawJSON(`0`),
	}, bi.persistence, bi.blockListener)
	require.NoError(t, err)
	err = bi.Start(&InternalEventStream{
		Definition: internalESConfig,
		Handler:    handler,
	})
	require.NoError(t, err)

	// Check it's back to the checkpoint we expect
	es := bi.eventStreams[uuid.MustParse(esID)]
	cp, err := es.processCheckpoint()
	require.NoError(t, err)
	assert.Equal(t, int64(14), cp)

	// And check we don't get any events
	select {
	case <-eventCollector:
		panic("redelivery")
	case <-time.After(5 * time.Millisecond):
	}
}

func TestNoMatchingEvents(t *testing.T) {

	// This test uses a real DB, includes the full block indexer, but simulates the blockchain.
	_, bi, mRPC, blDone := newTestBlockIndexer(t)
	defer blDone()

	// Mock up the block calls to the blockchain for 15 blocks
	blocks, receipts := testBlockArray(t, 15)
	mockBlocksRPCCalls(mRPC, blocks, receipts)
	mockBlockListenerNil(mRPC)

	// Create a matcher that only mismatched on indexed - so same signature
	testABICopy := testParseABI(testEventABIJSON)
	testABICopy[1].Inputs[0].Indexed = !testABICopy[1].Inputs[0].Indexed

	// Do a full start now with an internal event listener
	utBatchNotify := make(chan []*IndexedBlock)
	err := bi.Start(&InternalEventStream{
		Type: IESTypePreCommitHandler,
		PreCommitHandler: func(ctx context.Context, dbTX *gorm.DB, blocks []*IndexedBlock, transactions []*IndexedTransactionNotify) (PostCommit, error) {
			return func() {
				utBatchNotify <- blocks
			}, nil
		},
	}, &InternalEventStream{
		Handler: func(ctx context.Context, tx *gorm.DB, batch *EventDeliveryBatch) (PostCommit, error) {
			require.Fail(t, "should not be called")
			return nil, nil
		},
		Definition: &EventStream{
			Name: "unit_test",
			Config: EventStreamConfig{
				BatchSize:    confutil.P(1),
				BatchTimeout: confutil.P("5ms"),
			},
			// Listen to two out of three event types
			ABI: abi.ABI{
				// Mismatched only on index
				testABICopy[1],
			},
		},
	})
	require.NoError(t, err)

	for i := 0; i < 15; i++ {
		<-utBatchNotify
	}

}

func TestStartBadInternalEventStream(t *testing.T) {

	// This test uses a real DB, includes the full block indexer, but simulates the blockchain.
	_, bi, _, blDone := newTestBlockIndexer(t)
	defer blDone()

	err := bi.Start(&InternalEventStream{})
	assert.Regexp(t, "PD020005", err)

}

func TestTestNotifyEventStreamDoesNotBlock(t *testing.T) {

	// This test uses a real DB, includes the full block indexer, but simulates the blockchain.
	ctx, bi, _, blDone := newTestBlockIndexer(t)
	defer blDone()

	bi.eventStreams[uuid.New()] = &eventStream{
		signatures: map[string]bool{
			topicA.String(): true,
		},
		blocks: make(chan *eventStreamBlock),
	}

	blockHash := ethtypes.MustNewHexBytes0xPrefix(tktypes.RandHex(32))
	txHash := ethtypes.MustNewHexBytes0xPrefix(tktypes.RandHex(32))
	bi.notifyEventStreams(ctx, &blockWriterBatch{
		blocks: []*BlockInfoJSONRPC{
			{
				Number: 12345,
				Hash:   blockHash,
			},
		},
		receipts: [][]*TXReceiptJSONRPC{
			{
				{
					Logs: []*LogJSONRPC{
						{
							BlockNumber:      12345,
							BlockHash:        blockHash,
							TransactionHash:  txHash,
							TransactionIndex: 0,
							Topics:           []ethtypes.HexBytes0xPrefix{topicA},
						},
					},
				},
			},
		},
	})

}

func TestUpsertInternalEventQueryExistingStreamFail(t *testing.T) {
	_, bi, _, p, done := newMockBlockIndexer(t, &Config{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnError(fmt.Errorf("pop"))

	err := bi.Start(&InternalEventStream{
		Definition: &EventStream{
			Name: "testing",
		},
	})
	assert.Regexp(t, "pop", err)
}

func TestUpsertInternalEventStreamMismatchExistingABI(t *testing.T) {
	_, bi, _, p, done := newMockBlockIndexer(t, &Config{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows(
		[]string{"id", "abi"},
	).AddRow(uuid.New().String(), testEventABIJSON))

	err := bi.Start(&InternalEventStream{
		Definition: &EventStream{
			Name: "testing",
		},
	})
	assert.Regexp(t, "PD020004", err)

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestUpsertInternalEventStreamMismatchExistingSource(t *testing.T) {
	_, bi, _, p, done := newMockBlockIndexer(t, &Config{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows(
		[]string{"id", "abi"},
	).AddRow(uuid.New().String(), testEventABIJSON))

	var a abi.ABI
	err := json.Unmarshal(testEventABIJSON, &a)
	assert.NoError(t, err)

	err = bi.Start(&InternalEventStream{
		Definition: &EventStream{
			Name:   "testing",
			ABI:    a,
			Source: tktypes.MustEthAddress(tktypes.RandHex(20)),
		},
	})
	assert.Regexp(t, "PD011302", err)

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestUpsertInternalEventStreamUpdateFail(t *testing.T) {
	_, bi, _, p, done := newMockBlockIndexer(t, &Config{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows(
		[]string{"id", "abi"},
	).AddRow(uuid.New().String(), testEventABIJSON))
	p.Mock.ExpectExec("UPDATE.*config").WillReturnError(fmt.Errorf("pop"))

	err := bi.Start(&InternalEventStream{
		Definition: &EventStream{
			Name: "testing",
			ABI:  testParseABI(testEventABIJSON),
			Config: EventStreamConfig{
				BatchSize: confutil.P(12345),
			},
		},
	})
	assert.Regexp(t, "pop", err)

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestUpsertInternalEventStreamCreateFail(t *testing.T) {
	_, bi, _, p, done := newMockBlockIndexer(t, &Config{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows(
		[]string{"id", "abi"},
	))
	p.Mock.ExpectExec("INSERT.*config").WillReturnError(fmt.Errorf("pop"))

	err := bi.Start(&InternalEventStream{
		Definition: &EventStream{
			Name: "testing",
			ABI:  testParseABI(testEventABIJSON),
		},
	})
	assert.Regexp(t, "pop", err)

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestProcessCheckpointFail(t *testing.T) {
	ctx, bi, _, p, done := newMockBlockIndexer(t, &Config{})
	defer done()

	bi.retry.UTSetMaxAttempts(1)
	p.Mock.ExpectQuery("SELECT.*event_stream_checkpoints").WillReturnError(fmt.Errorf("pop"))

	es := &eventStream{
		bi:           bi,
		ctx:          ctx,
		definition:   &EventStream{ID: uuid.New()},
		detectorDone: make(chan struct{}),
	}
	es.detector()

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestGetHighestIndexedBlockFail(t *testing.T) {
	ctx, bi, _, p, done := newMockBlockIndexer(t, &Config{})
	defer done()

	bi.retry.UTSetMaxAttempts(1)
	p.Mock.ExpectQuery("SELECT.*event_stream_checkpoints").WillReturnRows(p.Mock.NewRows([]string{}))
	p.Mock.ExpectQuery("SELECT.*indexed_blocks").WillReturnError(fmt.Errorf("pop"))

	es := &eventStream{
		bi:           bi,
		ctx:          ctx,
		definition:   &EventStream{ID: uuid.New()},
		detectorDone: make(chan struct{}),
	}
	es.detector()

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestReturnToCatchupAfterStartHead(t *testing.T) {
	testReturnToCatchupAfterStart(t, 0)
}

func TestReturnToCatchupAfterStartHeadBlock5(t *testing.T) {
	testReturnToCatchupAfterStart(t, 5)
}

func testReturnToCatchupAfterStart(t *testing.T, headBlock int64) {
	ctx, bi, _, p, done := newMockBlockIndexer(t, &Config{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*event_stream_checkpoints").WillReturnRows(p.Mock.NewRows([]string{}))
	// Start on block as specified
	rows := p.Mock.NewRows([]string{"number"})
	if headBlock > 0 {
		rows.AddRow(5)
	}
	p.Mock.ExpectQuery("SELECT.*indexed_blocks").WillReturnRows(rows)
	// We'll query from 5
	p.Mock.ExpectQuery("SELECT.*indexed_events").WillReturnRows(p.Mock.NewRows([]string{}))
	// Then after notify notify go back to get block 10, causing us to hunt the gap
	p.Mock.ExpectQuery("SELECT.*indexed_events").WillReturnRows(p.Mock.NewRows([]string{}))

	cancellableCtx, cancelCtx := context.WithCancel(ctx)
	es := &eventStream{
		bi:  bi,
		ctx: cancellableCtx,
		definition: &EventStream{
			ID:  uuid.New(),
			ABI: testABI,
		},
		eventABIs:    testABI,
		blocks:       make(chan *eventStreamBlock),
		dispatch:     make(chan *eventDispatch),
		detectorDone: make(chan struct{}),
	}
	go func() {
		assert.NotPanics(t, func() { es.detector() })
	}()

	// This will be ignored as behind our head
	es.blocks <- &eventStreamBlock{blockNumber: 5}

	// notify block ten
	es.blocks <- &eventStreamBlock{
		blockNumber: 10,
		events: []*LogJSONRPC{
			{
				BlockHash:        ethtypes.MustNewHexBytes0xPrefix(tktypes.RandHex(32)),
				TransactionHash:  ethtypes.MustNewHexBytes0xPrefix(tktypes.RandHex(32)),
				BlockNumber:      10,
				TransactionIndex: 0,
				LogIndex:         0,
				Topics:           []ethtypes.HexBytes0xPrefix{topicA /* this one has no data */},
			},
		},
	}
	d := <-es.dispatch
	assert.Equal(t, int64(10), d.event.BlockNumber)

	cancelCtx()
	<-es.detectorDone

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestExitInCatchupPhase(t *testing.T) {
	ctx, bi, _, p, done := newMockBlockIndexer(t, &Config{})
	defer done()

	bi.retry.UTSetMaxAttempts(1)
	p.Mock.ExpectQuery("SELECT.*event_stream_checkpoints").WillReturnRows(p.Mock.NewRows([]string{}))
	p.Mock.ExpectQuery("SELECT.*indexed_blocks").WillReturnRows(p.Mock.
		NewRows([]string{"number"}).AddRow(5))
	p.Mock.ExpectQuery("SELECT.*indexed_events").WillReturnError(fmt.Errorf("pop"))

	es := &eventStream{
		bi:  bi,
		ctx: ctx,
		definition: &EventStream{
			ID:  uuid.New(),
			ABI: testABI,
		},
		eventABIs:    testABI,
		blocks:       make(chan *eventStreamBlock),
		detectorDone: make(chan struct{}),
	}
	go func() {
		assert.NotPanics(t, func() { es.detector() })
	}()
	<-es.detectorDone

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestSendToDispatcherClosedNoBlock(t *testing.T) {
	ctx, bi, _, _, done := newMockBlockIndexer(t, &Config{})
	done()

	es := &eventStream{
		bi:       bi,
		ctx:      ctx,
		dispatch: make(chan *eventDispatch),
	}
	es.sendToDispatcher(&EventWithData{
		IndexedEvent: &IndexedEvent{},
	}, false)
}

func TestDispatcherDispatchClosed(t *testing.T) {
	ctx, bi, _, p, done := newMockBlockIndexer(t, &Config{})
	defer done()

	p.Mock.ExpectBegin()
	p.Mock.ExpectRollback()

	called := false

	bi.retry.UTSetMaxAttempts(1)
	es := &eventStream{
		bi:  bi,
		ctx: ctx,
		definition: &EventStream{
			ID:   uuid.New(),
			Type: EventStreamTypeInternal.Enum(),
			ABI:  testABI,
		},
		eventABIs:      testABI,
		batchSize:      2,                    // aim for two
		batchTimeout:   1 * time.Microsecond, // but not going to wait
		dispatch:       make(chan *eventDispatch),
		dispatcherDone: make(chan struct{}),
		handler: func(ctx context.Context, tx *gorm.DB, batch *EventDeliveryBatch) (PostCommit, error) {
			called = true
			return nil, fmt.Errorf("pop")
		},
	}
	go func() {
		assert.NotPanics(t, func() { es.dispatcher() })
	}()

	es.dispatch <- &eventDispatch{
		event: &EventWithData{
			IndexedEvent: &IndexedEvent{},
		},
	}

	<-es.dispatcherDone

	assert.True(t, called)
}

func TestProcessCatchupEventPageFailRPC(t *testing.T) {
	ctx, bi, mRPC, p, done := newMockBlockIndexer(t, &Config{})
	defer done()

	txHash := tktypes.MustParseBytes32(tktypes.RandHex(32))

	bi.retry.UTSetMaxAttempts(2)
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getTransactionReceipt", ethtypes.MustNewHexBytes0xPrefix(txHash.String())).
		Return(rpcclient.WrapErrorRPC(rpcclient.RPCCodeInternalError, fmt.Errorf("pop"))).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getTransactionReceipt", ethtypes.MustNewHexBytes0xPrefix(txHash.String())).
		Return(nil) // but still not found

	p.Mock.ExpectQuery("SELECT.*indexed_events").WillReturnRows(
		sqlmock.NewRows([]string{
			"transaction_hash",
		}).AddRow(txHash),
	)

	es := &eventStream{
		bi:         bi,
		ctx:        ctx,
		definition: &EventStream{ID: uuid.New(), ABI: testABI},
		eventABIs:  testABI,
	}

	_, err := es.processCatchupEventPage(0, 10000)
	assert.Regexp(t, "PD011305", err)
}
