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
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/mocks/rpcbackendmocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

func mockBlockListenerNil(mRPC *rpcbackendmocks.WebSocketRPCClient) {
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
	bi.utBatchNotify = nil // we only care about the blocks

	// Mock up the block calls to the blockchain for 15 blocks
	blocks, receipts := testBlockArray(t, 15)
	mockBlocksRPCCalls(mRPC, blocks, receipts)
	mockBlockListenerNil(mRPC)

	eventCollector := make(chan *EventWithData)

	// Do a full start now with an internal block listener
	var esID string
	err := bi.Start(func(ctx context.Context, tx *gorm.DB, batch *EventDeliveryBatch) error {
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
	}, &EventStream{
		Name: "unit_test",
		Config: &EventStreamConfig{
			BatchSize:    confutil.P(3),
			BatchTimeout: confutil.P("5ms"),
		},
		// Listen to two out of three event types
		ABI: abi.ABI{
			testABI[1],
			testABI[2],
		},
	})
	assert.NoError(t, err)

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

	// Check the checkpoint is where we expect
	es := bi.eventStreams[uuid.MustParse(esID)]
	cp, err := es.processCheckpoint()
	assert.NoError(t, err)
	assert.Equal(t, int64(14), cp)

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
	handler := func(ctx context.Context, tx *gorm.DB, batch *EventDeliveryBatch) error {
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
	err := bi.Start(handler)
	assert.NoError(t, err)
	for i := 0; i < len(blocks); i++ {
		b := <-bi.utBatchNotify
		assert.Len(t, b.blocks, 1)
		assert.Equal(t, blocks[i], b.blocks[0])
	}

	// Add a listener
	es, err := bi.upsertInternalEventStream(ctx, &EventStream{
		Name: "unit_test",
		Config: &EventStreamConfig{
			BatchSize:    confutil.P(3),
			BatchTimeout: confutil.P("5ms"),
		},
		// Listen to two out of three event types
		ABI: abi.ABI{
			testABI[1],
			testABI[2],
		},
	})
	assert.NoError(t, err)

	// And start it
	es.start()

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

	// Check the checkpoint is where we expect
	cp, err := es.processCheckpoint()
	assert.NoError(t, err)
	assert.Equal(t, int64(14), cp)
}
