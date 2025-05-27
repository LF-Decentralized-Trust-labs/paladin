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
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/mocks/rpcclientmocks"
	"github.com/kaleido-io/paladin/core/pkg/persistence"

	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/query"
	"github.com/kaleido-io/paladin/sdk/go/pkg/rpcclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

	eventCollector := make(chan *pldapi.EventWithData)

	// Do a full start now with an internal event listener
	var esID string
	calledPostCommit := false
	err := bi.Start(&InternalEventStream{
		HandlerDBTX: func(ctx context.Context, dbTX persistence.DBTX, batch *EventDeliveryBatch) error {
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
			dbTX.AddPostCommit(func(ctx context.Context) { calledPostCommit = true })
			return nil
		},
		Definition: &EventStream{
			Name: "unit_test",
			Config: EventStreamConfig{
				BatchSize:    confutil.P(3),
				BatchTimeout: confutil.P("5ms"),
			},
			// Listen to two out of three event types
			Sources: []EventStreamSource{{
				ABI: abi.ABI{
					testABI[1],
					testABI[2],
				},
			}},
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

	sourceContractAddress := pldtypes.MustEthAddress(pldtypes.RandHex(20))

	// Mock up the block calls to the blockchain for 15 blocks
	blocks, receipts := testBlockArray(t, 15, *sourceContractAddress.Address0xHex())
	mockBlocksRPCCalls(mRPC, blocks, receipts)
	mockBlockListenerNil(mRPC)

	eventCollector := make(chan *pldapi.EventWithData)

	definition := &EventStream{
		Name: "unit_test",
		Config: EventStreamConfig{
			BatchSize:    confutil.P(3),
			BatchTimeout: confutil.P("5ms"),
		},
		// Listen to two out of three event types
		Sources: []EventStreamSource{{
			ABI: abi.ABI{
				testABI[1],
				testABI[2],
			},
			Address: sourceContractAddress,
		}},
	}

	// Do a full start now with an internal event listener
	var esID string
	calledPostCommit := false
	err := bi.Start(&InternalEventStream{
		HandlerDBTX: func(ctx context.Context, dbTX persistence.DBTX, batch *EventDeliveryBatch) error {
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
			dbTX.AddPostCommit(func(ctx context.Context) { calledPostCommit = true })
			return nil
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
	eventCollector := make(chan *pldapi.EventWithData)
	var esID string
	handler := func(ctx context.Context, dbTX persistence.DBTX, batch *EventDeliveryBatch) error {
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
		return nil
	}

	// Do a full start now without a block listener, and wait for the ut notification of all the blocks
	utBatchNotify := make(chan []*pldapi.IndexedBlock)
	preCommitCount := 0
	err := bi.Start(&InternalEventStream{
		Type: IESTypePreCommitHandler,
		PreCommitHandler: func(ctx context.Context, dbTX persistence.DBTX, blocks []*pldapi.IndexedBlock, transactions []*IndexedTransactionNotify) error {
			// Return an error once to drive a retry
			preCommitCount++
			if preCommitCount == 0 {
				return fmt.Errorf("pop")
			}
			dbTX.AddPostCommit(func(ctx context.Context) {
				utBatchNotify <- blocks
			})
			return nil
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
		Sources: []EventStreamSource{{
			ABI: abi.ABI{
				testABI[1],
				testABI[2],
			},
		}},
	}
	_, err = bi.AddEventStream(ctx, bi.persistence.NOTX(), &InternalEventStream{
		Definition:  internalESConfig,
		HandlerDBTX: handler,
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

	// Wait for checkpoint
	for {
		es := bi.eventStreams[uuid.MustParse(esID)]
		baseBlock, err := es.readDBCheckpoint()
		require.NoError(t, err)
		if baseBlock != nil && *baseBlock >= 14 {
			break
		}
		require.False(t, t.Failed())
		time.Sleep(10 * time.Millisecond)
	}

	// Stop and restart
	bi.Stop()

	bi, err = newBlockIndexer(ctx, &pldconf.BlockIndexerConfig{
		CommitBatchSize: confutil.P(1),
		FromBlock:       json.RawMessage(`0`),
	}, bi.persistence, bi.blockListener)
	require.NoError(t, err)
	err = bi.Start(&InternalEventStream{
		Definition:  internalESConfig,
		HandlerDBTX: handler,
	})
	require.NoError(t, err)

	// Check it's back to the checkpoint we expect
	es := bi.eventStreams[uuid.MustParse(esID)]
	cp, err := es.processCheckpoint()
	require.NoError(t, err)
	require.NotNil(t, cp)
	assert.Equal(t, int64(14), *cp)

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
	utBatchNotify := make(chan []*pldapi.IndexedBlock)
	err := bi.Start(&InternalEventStream{
		Type: IESTypePreCommitHandler,
		PreCommitHandler: func(ctx context.Context, dbTX persistence.DBTX, blocks []*pldapi.IndexedBlock, transactions []*IndexedTransactionNotify) error {
			dbTX.AddPostCommit(func(ctx context.Context) {
				utBatchNotify <- blocks
			})
			return nil
		},
	}, &InternalEventStream{
		HandlerDBTX: func(ctx context.Context, dbTX persistence.DBTX, batch *EventDeliveryBatch) error {
			require.Fail(t, "should not be called")
			return nil
		},
		Definition: &EventStream{
			Name: "unit_test",
			Config: EventStreamConfig{
				BatchSize:    confutil.P(1),
				BatchTimeout: confutil.P("5ms"),
			},
			Sources: []EventStreamSource{{
				ABI: abi.ABI{
					// Mismatched only on index
					testABICopy[1],
				},
			}},
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

	blockHash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
	txHash := ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32))
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

func TestAddEventStreamBadName(t *testing.T) {
	ctx, bi, _, mp, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()

	_, err := bi.AddEventStream(ctx, mp.P.NOTX(), &InternalEventStream{})
	assert.Regexp(t, "PD020005", err)
}

func TestAddEventStreamBadFromBlockConfiguration(t *testing.T) {
	ctx, bi, _, mp, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()

	_, err := bi.AddEventStream(ctx, mp.P.NOTX(), &InternalEventStream{
		Definition: &EventStream{
			Name: "testing",
			Config: EventStreamConfig{
				FromBlock: json.RawMessage(`"one"`),
			},
		},
	})
	assert.Regexp(t, "PD011300", err)
}

func TestUpsertInternalEventQueryExistingStreamFail(t *testing.T) {
	_, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnError(fmt.Errorf("pop"))

	err := bi.Start(&InternalEventStream{
		Definition: &EventStream{
			Name: "testing",
		},
	})
	assert.Regexp(t, "pop", err)
}

func TestUpsertInternalEventStreamMismatchExistingSourceABI(t *testing.T) {
	_, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows(
		[]string{"id", "sources"},
	).AddRow(uuid.New().String(), testEventSourcesJSON))

	err := bi.Start(&InternalEventStream{
		Definition: &EventStream{
			Name:    "testing",
			Sources: []EventStreamSource{{}},
		},
	})
	assert.Regexp(t, "PD020004", err)

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestUpsertInternalEventStreamMismatchExistingSourceAddress(t *testing.T) {
	_, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows(
		[]string{"id", "sources"},
	).AddRow(uuid.New().String(), testEventSourcesJSON))

	var a abi.ABI
	err := json.Unmarshal(testEventABIJSON, &a)
	assert.NoError(t, err)

	err = bi.Start(&InternalEventStream{
		Definition: &EventStream{
			Name: "testing",
			Sources: []EventStreamSource{{
				ABI:     a,
				Address: pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			}},
		},
	})
	assert.Regexp(t, "PD011302", err)

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestUpsertInternalEventStreamMismatchExistingSourceLength(t *testing.T) {
	_, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows(
		[]string{"id", "sources"},
	).AddRow(uuid.New().String(), testEventSourcesJSON))

	var a abi.ABI
	err := json.Unmarshal(testEventABIJSON, &a)
	assert.NoError(t, err)

	err = bi.Start(&InternalEventStream{
		Definition: &EventStream{
			Name:    "testing",
			Sources: []EventStreamSource{},
		},
	})
	assert.Regexp(t, "PD011302", err)

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestUpsertInternalEventStreamUpdateFail(t *testing.T) {
	_, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows(
		[]string{"id", "sources"},
	).AddRow(uuid.New().String(), testEventSourcesJSON))
	p.Mock.ExpectExec("UPDATE.*config").WillReturnError(fmt.Errorf("pop"))

	err := bi.Start(&InternalEventStream{
		Definition: &EventStream{
			Name: "testing",
			Sources: []EventStreamSource{{
				ABI: testParseABI(testEventABIJSON),
			}},
			Config: EventStreamConfig{
				BatchSize: confutil.P(12345),
			},
		},
	})
	assert.Regexp(t, "pop", err)

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestUpsertInternalEventStreamCreateFail(t *testing.T) {
	_, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows(
		[]string{"id", "abi"},
	))
	p.Mock.ExpectExec("INSERT.*config").WillReturnError(fmt.Errorf("pop"))

	err := bi.Start(&InternalEventStream{
		Definition: &EventStream{
			Name: "testing",
			Sources: []EventStreamSource{{
				ABI: testParseABI(testEventABIJSON),
			}},
		},
	})
	assert.Regexp(t, "pop", err)

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestRemoveEventStream(t *testing.T) {
	ctx, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()
	esID := uuid.New()
	eventStream := &eventStream{
		definition: &EventStream{
			ID: esID,
		},
	}

	// doesn't exist
	err := bi.RemoveEventStream(ctx, esID)
	require.ErrorContains(t, err, "PD011312")

	// error calling delete
	bi.eventStreams[esID] = eventStream
	bi.eventStreamsHeadSet[esID] = eventStream

	p.Mock.ExpectExec("DELETE.*event_streams").WillReturnError(fmt.Errorf("pop"))
	err = bi.RemoveEventStream(ctx, esID)
	require.ErrorContains(t, err, "pop")

	// success
	p.Mock.ExpectExec("DELETE.*event_streams").WithArgs(esID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = bi.RemoveEventStream(ctx, esID)
	require.NoError(t, err)
	assert.NotContains(t, bi.eventStreams, esID)
	assert.NotContains(t, bi.eventStreamsHeadSet, esID)

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestQueryEventStreamDefinitions(t *testing.T) {
	ctx, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()

	q := query.NewQueryBuilder().
		Equal("name", "test-es").
		Limit(10).
		Query()

	// limit errors
	_, err := bi.QueryEventStreamDefinitions(ctx, bi.persistence.NOTX(), EventStreamTypePTXBlockchainEventListener.Enum(), nil)
	assert.ErrorContains(t, err, "PD011311")
	_, err = bi.QueryEventStreamDefinitions(ctx, bi.persistence.NOTX(), EventStreamTypePTXBlockchainEventListener.Enum(), query.NewQueryBuilder().Query())
	assert.ErrorContains(t, err, "PD011311")
	_, err = bi.QueryEventStreamDefinitions(ctx, bi.persistence.NOTX(), EventStreamTypePTXBlockchainEventListener.Enum(), query.NewQueryBuilder().Limit(0).Query())
	assert.ErrorContains(t, err, "PD011311")

	// error
	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnError(fmt.Errorf("pop"))
	_, err = bi.QueryEventStreamDefinitions(ctx, bi.persistence.NOTX(), EventStreamTypePTXBlockchainEventListener.Enum(), q)
	assert.ErrorContains(t, err, "pop")

	//success
	p.Mock.ExpectQuery("SELECT.*event_streams").WithArgs(EventStreamTypePTXBlockchainEventListener.Enum(), "test-es", 10).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type"},
		).AddRow(uuid.New().String(), "test-es", EventStreamTypePTXBlockchainEventListener.Enum()))
	result, err := bi.QueryEventStreamDefinitions(ctx, bi.persistence.NOTX(), EventStreamTypePTXBlockchainEventListener.Enum(), q)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "test-es", result[0].Name)
	assert.Equal(t, EventStreamTypePTXBlockchainEventListener.Enum(), result[0].Type)

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestStartStopEventStream(t *testing.T) {
	ctx, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()
	esID := uuid.New()

	// doesn't exist
	err := bi.StartEventStream(ctx, esID)
	require.ErrorContains(t, err, "PD011312")

	err = bi.StopEventStream(ctx, esID)
	require.ErrorContains(t, err, "PD011312")

	// these are the DB calls that will be made when starting the the event stream
	p.Mock.ExpectExec("UPDATE.*event_streams").WillReturnError(errors.New("pop"))
	p.Mock.ExpectExec("UPDATE.*event_streams").WillReturnResult(sqlmock.NewResult(1, 1))

	// these are the DB calls that the detector go routine might call
	p.Mock.ExpectQuery("SELECT.*event_stream_checkpoints").WillReturnRows(sqlmock.NewRows([]string{}))
	p.Mock.ExpectQuery("SELECT.*indexed_blocks").WillReturnRows(sqlmock.NewRows([]string{}))

	// these are the DB calls that will be made when stopping the the event stream
	p.Mock.ExpectExec("UPDATE.*event_streams").WillReturnError(errors.New("pop"))
	p.Mock.ExpectExec("UPDATE.*event_streams").WillReturnResult(sqlmock.NewResult(1, 1))

	eventStream := &eventStream{
		definition: &EventStream{
			ID: esID,
		},
		handlerDBTX: func(ctx context.Context, dbTX persistence.DBTX, batch *EventDeliveryBatch) error {
			return nil
		},
		bi:        bi,
		fromBlock: confutil.P(ethtypes.HexUint64(0)),
	}

	bi.eventStreams[esID] = eventStream

	err = bi.StartEventStream(ctx, esID)
	require.ErrorContains(t, err, "pop")

	err = bi.StartEventStream(ctx, esID)
	require.NoError(t, err)

	assert.NotNil(t, eventStream.detectorDone)
	assert.NotNil(t, eventStream.dispatcherDone)

	// wait because the detector DB mocks run in a different thread and we can't guarantee the ordering otherwise
	time.Sleep(10 * time.Millisecond)

	err = bi.StopEventStream(ctx, esID)
	require.ErrorContains(t, err, "pop")

	err = bi.StopEventStream(ctx, esID)
	require.NoError(t, err)

	assert.Nil(t, eventStream.detectorDone)
	assert.Nil(t, eventStream.dispatcherDone)
}

func TestGetEventStreamStatus(t *testing.T) {
	ctx, bi, _, _, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()
	esID := uuid.New()

	// doesn't exist
	_, err := bi.GetEventStreamStatus(ctx, esID)
	require.ErrorContains(t, err, "PD011312")

	eventStream := &eventStream{
		definition: &EventStream{
			ID: esID,
		},
		handlerDBTX: func(ctx context.Context, dbTX persistence.DBTX, batch *EventDeliveryBatch) error {
			return nil
		},
		bi: bi,
	}
	eventStream.checkpoint.Store(25)
	eventStream.catchup.Store(true)
	bi.eventStreams[esID] = eventStream

	// success
	status, err := bi.GetEventStreamStatus(ctx, esID)
	require.NoError(t, err)
	assert.Equal(t, int64(25), status.CheckpointBlock)
	assert.True(t, status.Catchup)
}

func TestProcessCheckpointFail(t *testing.T) {
	ctx, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()

	bi.retry.UTSetMaxAttempts(1)
	p.Mock.ExpectQuery("SELECT.*event_stream_checkpoints").WillReturnError(fmt.Errorf("pop"))

	es := &eventStream{
		bi:           bi,
		ctx:          ctx,
		definition:   &EventStream{ID: uuid.New()},
		detectorDone: make(chan struct{}),
		fromBlock:    confutil.P(ethtypes.HexUint64(0)),
	}
	es.detector()

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestGetHighestIndexedBlockFail(t *testing.T) {
	ctx, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()

	bi.retry.UTSetMaxAttempts(1)
	p.Mock.ExpectQuery("SELECT.*event_stream_checkpoints").WillReturnRows(p.Mock.NewRows([]string{}))
	p.Mock.ExpectQuery("SELECT.*indexed_blocks").WillReturnError(fmt.Errorf("pop"))

	es := &eventStream{
		bi:           bi,
		ctx:          ctx,
		definition:   &EventStream{ID: uuid.New()},
		detectorDone: make(chan struct{}),
		fromBlock:    confutil.P(ethtypes.HexUint64(0)),
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
	ctx, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
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
			ID: uuid.New(),
			Sources: []EventStreamSource{{
				ABI: testABI,
			}},
		},
		blocks:       make(chan *eventStreamBlock),
		dispatch:     make(chan *eventDispatch),
		detectorDone: make(chan struct{}),
		serializer:   pldtypes.JSONFormatOptions("").GetABISerializerIgnoreErrors(ctx),
		fromBlock:    confutil.P(ethtypes.HexUint64(0)),
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
				BlockHash:        ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32)),
				TransactionHash:  ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32)),
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
	ctx, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
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
			ID: uuid.New(),
			Sources: []EventStreamSource{{
				ABI: testABI,
			}},
		},
		blocks:       make(chan *eventStreamBlock),
		detectorDone: make(chan struct{}),
		fromBlock:    confutil.P(ethtypes.HexUint64(0)),
	}
	go func() {
		assert.NotPanics(t, func() { es.detector() })
	}()
	<-es.detectorDone

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestStartFromLatest(t *testing.T) {
	ctx, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*event_stream_checkpoints").WillReturnRows(sqlmock.NewRows([]string{}))

	cancellableCtx, cancelCtx := context.WithCancel(ctx)
	es := &eventStream{
		bi:  bi,
		ctx: cancellableCtx,
		definition: &EventStream{
			ID: uuid.New(),
			Sources: []EventStreamSource{{
				ABI: testABI,
			}},
		},
		blocks:       make(chan *eventStreamBlock),
		dispatch:     make(chan *eventDispatch),
		detectorDone: make(chan struct{}),
		serializer:   pldtypes.JSONFormatOptions("").GetABISerializerIgnoreErrors(ctx),
	}
	go func() {
		assert.NotPanics(t, func() { es.detector() })
	}()

	es.blocks <- &eventStreamBlock{
		blockNumber: 5,
		events: []*LogJSONRPC{
			{
				BlockHash:        ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32)),
				TransactionHash:  ethtypes.MustNewHexBytes0xPrefix(pldtypes.RandHex(32)),
				BlockNumber:      5,
				TransactionIndex: 0,
				LogIndex:         0,
			},
		}}
	d := <-es.dispatch
	assert.Equal(t, int64(5), d.event.BlockNumber)

	cancelCtx()
	<-es.detectorDone

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestExitStartFromLatest(t *testing.T) {
	ctx, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*event_stream_checkpoints").WillReturnRows(sqlmock.NewRows([]string{}))

	cancellableCtx, cancelCtx := context.WithCancel(ctx)
	es := &eventStream{
		bi:  bi,
		ctx: cancellableCtx,
		definition: &EventStream{
			ID: uuid.New(),
			Sources: []EventStreamSource{{
				ABI: testABI,
			}},
		},
		blocks:       make(chan *eventStreamBlock),
		dispatch:     make(chan *eventDispatch),
		detectorDone: make(chan struct{}),
		serializer:   pldtypes.JSONFormatOptions("").GetABISerializerIgnoreErrors(ctx),
	}
	go func() {
		assert.NotPanics(t, func() { es.detector() })
	}()

	// brief pause otherwise the detector may not start
	time.Sleep(10 * time.Millisecond)

	cancelCtx()
	<-es.detectorDone

	require.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestSendToDispatcherClosedNoBlock(t *testing.T) {
	ctx, bi, _, _, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	done()

	es := &eventStream{
		bi:       bi,
		ctx:      ctx,
		dispatch: make(chan *eventDispatch),
	}
	es.sendToDispatcher(&pldapi.EventWithData{
		IndexedEvent: &pldapi.IndexedEvent{},
	}, false)
}

func TestDispatcherDispatchClosed(t *testing.T) {
	ctx, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
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
			Sources: []EventStreamSource{{
				ABI: testABI,
			}},
		},
		batchSize:      2,                    // aim for two
		batchTimeout:   1 * time.Microsecond, // but not going to wait
		dispatch:       make(chan *eventDispatch),
		dispatcherDone: make(chan struct{}),
		handlerDBTX: func(ctx context.Context, dbTX persistence.DBTX, batch *EventDeliveryBatch) error {
			called = true
			return fmt.Errorf("pop")
		},
	}
	go func() {
		assert.NotPanics(t, func() { es.dispatcher() })
	}()

	es.dispatch <- &eventDispatch{
		event: &pldapi.EventWithData{
			IndexedEvent: &pldapi.IndexedEvent{},
		},
	}

	<-es.dispatcherDone

	assert.True(t, called)
}

func TestProcessCatchupEventPageFailRPC(t *testing.T) {
	ctx, bi, mRPC, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()

	txHash := pldtypes.MustParseBytes32(pldtypes.RandHex(32))

	bi.retry.UTSetMaxAttempts(2)
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getTransactionReceipt", ethtypes.MustNewHexBytes0xPrefix(txHash.String())).
		Return(rpcclient.WrapRPCError(rpcclient.RPCCodeInternalError, fmt.Errorf("pop"))).Once()
	mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getTransactionReceipt", ethtypes.MustNewHexBytes0xPrefix(txHash.String())).
		Return(nil) // but still not found

	p.Mock.ExpectQuery("SELECT.*indexed_events").WillReturnRows(
		sqlmock.NewRows([]string{
			"transaction_hash",
		}).AddRow(txHash),
	)

	es := &eventStream{
		bi:  bi,
		ctx: ctx,
		definition: &EventStream{
			ID: uuid.New(),
			Sources: []EventStreamSource{{
				ABI: testABI,
			}},
		},
	}

	_, _, err := es.processCatchupEventPage(nil, 0, 10000)
	assert.Regexp(t, "PD011305", err)
}

func TestProcessCatchupEventMultiPageRealDB(t *testing.T) {
	ctx, bi, mRPC, done := newTestBlockIndexer(t)
	defer done()

	eventSig := pldtypes.Bytes32(testABI.Events()["EventA"].SignatureHashBytes())

	allBlocks := []*pldapi.IndexedBlock{}
	allTransactions := []*pldapi.IndexedTransaction{}
	allEvents := []*pldapi.IndexedEvent{}
	for b := 1; b < 14; b++ {
		blockHash := pldtypes.RandBytes32()
		allBlocks = append(allBlocks, &pldapi.IndexedBlock{
			Number: int64(b),
			Hash:   blockHash,
		})
		for tx := 0; tx < 8; tx++ {
			txHash := pldtypes.RandBytes32()
			allTransactions = append(allTransactions, &pldapi.IndexedTransaction{
				Hash:             txHash,
				BlockNumber:      int64(b),
				TransactionIndex: int64(tx),
				From:             pldtypes.RandAddress(),
				To:               pldtypes.RandAddress(),
				Nonce:            0,
				Result:           pldapi.TXResult_SUCCESS.Enum(),
			})
			txReceipt := &TXReceiptJSONRPC{
				BlockHash:   blockHash[:],
				BlockNumber: ethtypes.HexUint64(b),
			}
			for li := 0; li < 9; li++ {
				allEvents = append(allEvents, &pldapi.IndexedEvent{
					BlockNumber:      int64(b),
					TransactionIndex: int64(tx),
					LogIndex:         int64(li),
					TransactionHash:  txHash,
					Signature:        eventSig,
				})
				txReceipt.Logs = append(txReceipt.Logs, &LogJSONRPC{
					TransactionHash:  txHash[:],
					BlockNumber:      ethtypes.HexUint64(b),
					TransactionIndex: ethtypes.HexUint64(tx),
					LogIndex:         ethtypes.HexUint64(li),
					Topics:           []ethtypes.HexBytes0xPrefix{eventSig[:]},
					Data:             []byte{}, // "EventA" has no data
				})
			}
			mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getTransactionReceipt", ethtypes.HexBytes0xPrefix(txHash[:])).
				Run(func(args mock.Arguments) {
					pTxReceipt := args[1].(**TXReceiptJSONRPC)
					*pTxReceipt = txReceipt
				}).
				Return(nil)
		}
	}
	err := bi.persistence.DB().Table("indexed_blocks").Create(allBlocks).Error
	require.NoError(t, err)
	err = bi.persistence.DB().Table("indexed_transactions").Create(allTransactions).Error
	require.NoError(t, err)
	err = bi.persistence.DB().Table("indexed_events").Create(allEvents).Error
	require.NoError(t, err)

	es := &eventStream{
		bi:            bi,
		ctx:           ctx,
		signatureList: []pldtypes.Bytes32{eventSig},
		dispatch:      make(chan *eventDispatch, len(allEvents)),
		definition: &EventStream{
			ID: uuid.New(),
			Sources: []EventStreamSource{{
				ABI: testABI,
			}},
		},
		serializer: pldtypes.JSONFormatOptions("").GetABISerializerIgnoreErrors(ctx),
	}

	go func() {
		var caughtUp bool
		var lastEvent *pldapi.IndexedEvent
		var err error
		for !caughtUp {
			caughtUp, lastEvent, err = es.processCatchupEventPage(lastEvent, 0, 100000000)
			require.NoError(t, err)
		}
	}()

	for i := 0; i < len(allEvents); i++ {
		d := <-es.dispatch
		require.Equal(t, allEvents[i].BlockNumber, d.event.BlockNumber)
		require.Equal(t, allEvents[i].TransactionIndex, d.event.TransactionIndex)
		require.Equal(t, allEvents[i].LogIndex, d.event.LogIndex)
	}
}

func TestEventSourcesHashing(t *testing.T) {

	abiEventIndexed := &abi.Entry{
		Type: abi.Event,
		Name: "Purple",
		Inputs: abi.ParameterArray{
			{
				Name:    "maybeIndexed",
				Type:    "uint256",
				Indexed: true, //only diff to other purple
			},
		},
	}
	abiEventUnindexed := &abi.Entry{
		Type: abi.Event,
		Name: "Purple",
		Inputs: abi.ParameterArray{
			{
				Name: "maybeIndexed",
				Type: "uint256",
			},
		},
	}
	abiFunction := &abi.Entry{
		Type:   abi.Function,
		Name:   "goPurple",
		Inputs: abi.ParameterArray{},
	}
	address1 := pldtypes.RandAddress()
	address2 := pldtypes.RandAddress()

	mustHash := func(ess EventSources) string {
		hash, err := ess.Hash(context.Background())
		require.NoError(t, err)
		return hash.String()
	}

	// order and ancillary entries do not matter
	assert.Equal(t,
		mustHash(EventSources{{ABI: abi.ABI{abiEventIndexed, abiEventUnindexed, abiFunction}}}),
		mustHash(EventSources{{ABI: abi.ABI{abiEventUnindexed, abiEventIndexed}}}),
	)

	// address or not does matter
	assert.NotEqual(t,
		mustHash(EventSources{{ABI: abi.ABI{abiEventIndexed}, Address: address1}}),
		mustHash(EventSources{{ABI: abi.ABI{abiEventIndexed}}}),
	)

	// addresses matter
	assert.NotEqual(t,
		mustHash(EventSources{{ABI: abi.ABI{abiEventIndexed}, Address: address1}}),
		mustHash(EventSources{{ABI: abi.ABI{abiEventIndexed}, Address: address2}}),
	)

	// error case
	ess := EventSources{{ABI: abi.ABI{{Type: abi.Event, Inputs: abi.ParameterArray{{Type: "wrong"}}}}}}
	_, err := ess.Hash(context.Background())
	assert.Regexp(t, "FF22025", err)

}

func TestNOTXHandler(t *testing.T) {
	ctx, bi, _, p, done := newMockBlockIndexer(t, &pldconf.BlockIndexerConfig{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(p.Mock.NewRows([]string{}))
	p.Mock.ExpectExec("INSERT.*event_streams").WillReturnResult(sqlmock.NewResult(1, 1))
	p.Mock.ExpectExec("INSERT.*event_stream_checkpoints").WillReturnResult(driver.ResultNoRows)
	returnErr := true

	definition, err := bi.AddEventStream(ctx, bi.persistence.NOTX(), &InternalEventStream{
		Type: IESTypeEventStreamNOTX,
		Definition: &EventStream{
			ID:   uuid.New(),
			Name: "es",
			Sources: []EventStreamSource{{
				ABI: testABI,
			}},
		},
		HandlerNOTX: func(_ context.Context, _ *EventDeliveryBatch) error {
			if returnErr {
				returnErr = false
				return errors.New("pop")
			}
			return nil
		},
	})

	require.NoError(t, err)

	es := bi.eventStreams[definition.ID]
	es.ctx = ctx

	err = es.runBatch(&eventBatch{})
	assert.NoError(t, err)
	assert.False(t, returnErr)
}
