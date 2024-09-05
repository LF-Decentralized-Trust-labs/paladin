// Copyright 2019 Kaleido

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/kata/internal/rpcclient"
	"github.com/kaleido-io/paladin/kata/mocks/rpcbackendmocks"
	"github.com/kaleido-io/paladin/kata/pkg/persistence"
	"github.com/kaleido-io/paladin/kata/pkg/persistence/mockpersistence"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/tlsconf"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var testEventABIJSON = ([]byte)(`[
    {
      "type": "event",
      "name": "EventA",
      "inputs": []
    },
    {
      "type": "event",
      "name": "EventB",
      "inputs": [
        {
          "name": "intParam1",
          "type": "uint256"
        },
        {
          "name": "strParam2",
          "type": "string"
        }
      ]
    },
    {
      "type": "event",
      "name": "EventC",
      "inputs": [
        {
          "name": "structParam1",
          "type": "tuple",
          "internalType": "struct Test.Struct1",
          "components": [
            {
              "name": "strField",
              "type": "string"
            },
            {
              "name": "intArrayField",
              "type": "int64[]"
            }
          ]
        }
      ]
    }
]`)

var testABI = testParseABI(testEventABIJSON)

var (
	topicA = testABI[0].SignatureHashBytes()
	topicB = testABI[1].SignatureHashBytes()
	topicC = testABI[2].SignatureHashBytes()
)

func testParseABI(abiJSON []byte) abi.ABI {
	var a abi.ABI
	err := json.Unmarshal(abiJSON, &a)
	if err != nil {
		panic(err)
	}
	return a
}

func newTestBlockIndexer(t *testing.T) (context.Context, *blockIndexer, *rpcbackendmocks.WebSocketRPCClient, func()) {
	return newTestBlockIndexerConf(t, &Config{
		CommitBatchSize: confutil.P(1), // makes testing simpler
		FromBlock:       types.RawJSON(`0`),
	})
}

func newTestBlockIndexerConf(t *testing.T, config *Config) (context.Context, *blockIndexer, *rpcbackendmocks.WebSocketRPCClient, func()) {
	logrus.SetLevel(logrus.DebugLevel)

	ctx, cancelCtx := context.WithCancel(context.Background())

	p, pDone, err := persistence.NewUnitTestPersistence(ctx)
	assert.NoError(t, err)

	blockListener, mRPC := newTestBlockListenerConf(t, ctx, config)
	bi, err := newBlockIndexer(ctx, config, p, blockListener)
	assert.NoError(t, err)
	bi.utBatchNotify = make(chan *blockWriterBatch)
	return ctx, bi, mRPC, func() {
		r := recover()
		if r != nil {
			panic(r)
		}
		bi.Stop()
		cancelCtx()
		pDone()
	}
}

func newMockBlockIndexer(t *testing.T, config *Config) (context.Context, *blockIndexer, *rpcbackendmocks.WebSocketRPCClient, *mockpersistence.SQLMockProvider, func()) {
	ctx, bl, mRPC, done := newTestBlockListener(t)

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows([]string{}))

	bi, err := newBlockIndexer(ctx, config, p.P, bl)
	assert.NoError(t, err)

	return ctx, bi, mRPC, p, done

}

func testBlockArray(t *testing.T, l int) ([]*BlockInfoJSONRPC, map[string][]*TXReceiptJSONRPC) {
	blocks := make([]*BlockInfoJSONRPC, l)
	receipts := make(map[string][]*TXReceiptJSONRPC, l)
	for i := 0; i < l; i++ {
		var contractAddress, to *ethtypes.Address0xHex
		if i == 0 {
			contractAddress = ethtypes.MustNewAddress(types.RandHex(20))
		} else {
			to = ethtypes.MustNewAddress(types.RandHex(20))
		}
		blocks[i] = &BlockInfoJSONRPC{
			Number: ethtypes.HexUint64(i),
			Hash:   ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32)),
		}
		txHash := ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
		eventBData, err := testABI[1].Inputs.EncodeABIDataValues(map[string]interface{}{
			"intParam1": i + 1000000,
			"strParam2": fmt.Sprintf("event_b_in_block_%d", i),
		})
		assert.NoError(t, err)
		eventCData, err := testABI[2].Inputs.EncodeABIDataValues(map[string]interface{}{
			"structParam1": map[string]interface{}{
				"strField":      fmt.Sprintf("event_c_in_block_%d", i),
				"intArrayField": []int{i + 1000, i + 2000, i + 3000, i + 4000, i + 5000},
			},
		})
		assert.NoError(t, err)
		receipts[blocks[i].Hash.String()] = []*TXReceiptJSONRPC{
			{
				TransactionHash: txHash,
				From:            ethtypes.MustNewAddress(types.RandHex(20)),
				To:              to,
				ContractAddress: contractAddress,
				BlockNumber:     blocks[i].Number,
				BlockHash:       blocks[i].Hash,
				Status:          ethtypes.NewHexInteger64(1),
				Logs: []*LogJSONRPC{
					{Address: ethtypes.MustNewAddress(types.RandHex(20)), BlockNumber: blocks[i].Number, LogIndex: 0, TransactionHash: txHash, Topics: []ethtypes.HexBytes0xPrefix{topicA, ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))}},
					{Address: ethtypes.MustNewAddress(types.RandHex(20)), BlockNumber: blocks[i].Number, LogIndex: 1, TransactionHash: txHash, Topics: []ethtypes.HexBytes0xPrefix{topicB, ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))}, Data: eventBData},
					{Address: ethtypes.MustNewAddress(types.RandHex(20)), BlockNumber: blocks[i].Number, LogIndex: 2, TransactionHash: txHash, Topics: []ethtypes.HexBytes0xPrefix{topicC, ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))}, Data: eventCData},
				},
			},
		}
		if i == 0 {
			blocks[i].ParentHash = ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32))
		} else {
			blocks[i].ParentHash = blocks[i-1].Hash
		}
	}
	return blocks, receipts
}

func mockBlocksRPCCalls(mRPC *rpcbackendmocks.WebSocketRPCClient, blocks []*BlockInfoJSONRPC, receipts map[string][]*TXReceiptJSONRPC) {
	mockBlocksRPCCallsDynamic(mRPC, func(args mock.Arguments) ([]*BlockInfoJSONRPC, map[string][]*TXReceiptJSONRPC) {
		return blocks, receipts
	})
}

func mockBlocksRPCCallsDynamic(mRPC *rpcbackendmocks.WebSocketRPCClient, dynamic func(args mock.Arguments) ([]*BlockInfoJSONRPC, map[string][]*TXReceiptJSONRPC)) {
	byBlock := mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockByNumber", mock.Anything, false).Maybe()
	byBlock.Run(func(args mock.Arguments) {
		blocks, _ := dynamic(args)
		blockReturn := args[1].(**BlockInfoJSONRPC)
		blockNumber := int(args[3].(ethtypes.HexUint64))
		if blockNumber >= len(blocks) {
			byBlock.Return(&rpcbackend.RPCError{Message: "not found"})
		} else {
			*blockReturn = blocks[blockNumber]
			byBlock.Return(nil)
		}
	})

	blockReceipts := mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getBlockReceipts", mock.Anything).Maybe()
	blockReceipts.Run(func(args mock.Arguments) {
		_, receipts := dynamic(args)
		blockReturn := args[1].(*[]*TXReceiptJSONRPC)
		blockHash := args[3].(ethtypes.HexBytes0xPrefix)
		*blockReturn = receipts[blockHash.String()]
		if *blockReturn == nil {
			blockReceipts.Return(&rpcbackend.RPCError{Message: "not found"})
		} else {
			blockReceipts.Return(nil)
		}
	})

	txReceipt := mRPC.On("CallRPC", mock.Anything, mock.Anything, "eth_getTransactionReceipt", mock.Anything).Maybe()
	txReceipt.Run(func(args mock.Arguments) {
		_, receipts := dynamic(args)
		blockReturn := args[1].(**TXReceiptJSONRPC)
		txHash := args[3].(ethtypes.HexBytes0xPrefix)
		for _, receipts := range receipts {
			for _, r := range receipts {
				if txHash.String() == r.TransactionHash.String() {
					*blockReturn = r
					break
				}
			}
		}
		if *blockReturn == nil {
			txReceipt.Return(&rpcbackend.RPCError{Message: "not found"})
		} else {
			txReceipt.Return(nil)
		}
	})
}

func TestNewBlockIndexerBadTLS(t *testing.T) {
	_, err := NewBlockIndexer(context.Background(), &Config{}, &rpcclient.WSConfig{
		HTTPConfig: rpcclient.HTTPConfig{
			URL: "wss://localhost:8546",
			TLS: tlsconf.Config{
				CAFile: t.TempDir(),
			},
		},
	}, nil)
	assert.Regexp(t, "PD010901", err)
}

func TestNewBlockIndexerRestoreCheckpointFail(t *testing.T) {
	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows([]string{}))

	wsConf := &rpcclient.WSConfig{HTTPConfig: rpcclient.HTTPConfig{URL: "ws://localhost:8546"}}

	cancelledCtx, cancelCtx := context.WithCancel(context.Background())
	bi, err := NewBlockIndexer(cancelledCtx, &Config{}, wsConf, p.P)
	assert.NoError(t, err)
	cancelCtx()

	// Start will get error, but return due to cancelled context
	err = bi.Start()
	assert.NoError(t, err)
	assert.Nil(t, bi.(*blockIndexer).processorDone)

	assert.NoError(t, p.Mock.ExpectationsWereMet())
}

func TestBlockIndexerCatchUpToHeadFromZeroNoConfirmations(t *testing.T) {
	_, bi, mRPC, blDone := newTestBlockIndexer(t)
	defer blDone()

	blocks, receipts := testBlockArray(t, 10)
	mockBlocksRPCCalls(mRPC, blocks, receipts)

	bi.requiredConfirmations = 0
	bi.startOrReset() // do not start block listener

	for i := 0; i < len(blocks); i++ {
		b := <-bi.utBatchNotify
		assert.Len(t, b.blocks, 1) // We should get one block per batch
		assert.Equal(t, blocks[i], b.blocks[0])
	}
}

func TestBlockIndexerBatchTimeoutOne(t *testing.T) {
	_, bi, mRPC, blDone := newTestBlockIndexer(t)
	defer blDone()

	bi.batchTimeout = 1 * time.Microsecond
	bi.batchSize = 100

	blocks, receipts := testBlockArray(t, 1)
	mockBlocksRPCCalls(mRPC, blocks, receipts)

	bi.requiredConfirmations = 0
	bi.startOrReset() // do not start block listener

	for i := 0; i < len(blocks); i++ {
		b := <-bi.utBatchNotify
		assert.Len(t, b.blocks, 1) // We should get one block per batch
		assert.Equal(t, blocks[i], b.blocks[0])
	}
}

func TestBlockIndexerCatchUpToHeadFromZeroWithConfirmations(t *testing.T) {
	ctx, bi, mRPC, blDone := newTestBlockIndexer(t)
	defer blDone()

	blocks, receipts := testBlockArray(t, 15)
	mockBlocksRPCCalls(mRPC, blocks, receipts)

	bi.requiredConfirmations = 5
	bi.startOrReset() // do not start block listener

	for i := 0; i < len(blocks)-bi.requiredConfirmations; i++ {
		b := <-bi.utBatchNotify
		assert.Len(t, b.blocks, 1) // We should get one block per batch
		assert.Equal(t, blocks[i], b.blocks[0])

		// Get the block
		indexedBlock, err := bi.GetIndexedBlockByNumber(ctx, blocks[i].Number.Uint64())
		assert.NoError(t, err)
		assert.Equal(t, blocks[i].Hash.String(), indexedBlock.Hash.String())

		// Get the transaction
		indexedTX, err := bi.GetIndexedTransactionByHash(ctx, types.Bytes32(receipts[blocks[i].Hash.String()][0].TransactionHash))
		assert.NoError(t, err)
		assert.Equal(t, receipts[blocks[i].Hash.String()][0].TransactionHash.String(), indexedTX.Hash.String())

		// Get the events
		tx0 := receipts[blocks[i].Hash.String()][0]
		txEvents, err := bi.GetTransactionEventsByHash(ctx, types.Bytes32(tx0.TransactionHash))
		assert.NoError(t, err)
		assert.Len(t, txEvents, 3)
		assert.Equal(t, topicA.String(), txEvents[0].Signature.String())
		assert.Equal(t, topicB.String(), txEvents[1].Signature.String())
		assert.Equal(t, topicC.String(), txEvents[2].Signature.String())
		if i == 0 {
			assert.Nil(t, tx0.To)
			assert.NotNil(t, tx0.ContractAddress)
			assert.NotEqual(t, types.EthAddress{}, *tx0.ContractAddress)
		} else {
			assert.Nil(t, tx0.ContractAddress)
			assert.NotNil(t, tx0.To)
			assert.NotEqual(t, types.EthAddress{}, *tx0.To)
		}
		assert.NotNil(t, tx0.From)
		assert.NotEqual(t, types.EthAddress{}, *tx0.From)

		// Get the transactions per block
		indexedTXs, err := bi.GetBlockTransactionsByNumber(ctx, int64(blocks[i].Number))
		assert.NoError(t, err)
		assert.Len(t, indexedTXs, 1)
		assert.Equal(t, receipts[blocks[i].Hash.String()][0].TransactionHash.String(), indexedTXs[0].Hash.String())
	}

	// Get the first unconfirmed block
	indexedBlock, err := bi.GetIndexedBlockByNumber(ctx, blocks[len(blocks)-bi.requiredConfirmations+1].Number.Uint64())
	assert.NoError(t, err)
	assert.Nil(t, indexedBlock)

	// Use small pages to list all the events
	lastBlock := int64(-1)
	lastIndex := -1
	for i := 0; i < 15; i += 5 {
		page, err := bi.ListTransactionEvents(ctx, lastBlock, lastIndex, 5, true, true)
		assert.NoError(t, err)
		assert.Len(t, page, 5)
		for i2 := 0; i2 < 5; i2++ {
			// There's one transaction per block, and 3 events per transaction
			expectedBlock := (i + i2) / 3
			expectedIndex := (i + i2) - (expectedBlock * 3)
			expectedReceipt := receipts[blocks[expectedBlock].Hash.String()]
			assert.Equal(t, expectedReceipt[0].BlockHash.String(), page[i2].Block.Hash.String())
			assert.Equal(t, int64(expectedReceipt[0].BlockNumber), page[i2].Block.Number)
			assert.Equal(t, expectedReceipt[0].Logs[expectedIndex].Topics[0].String(), page[i2].Signature.String())

			lastBlock = page[i2].BlockNumber
			lastIndex = int(page[i2].LogIndex)
		}
	}

}

func TestBlockIndexerListenFromCurrentBlock(t *testing.T) {
	ctx, bi, mRPC, blDone := newTestBlockIndexer(t)
	defer blDone()

	blocks, receipts := testBlockArray(t, 15)
	mockBlocksRPCCalls(mRPC, blocks, receipts)

	bi.fromBlock = nil
	bi.nextBlock = nil
	bi.requiredConfirmations = 5

	// simulate the highest block being known
	bi.blockListener.highestBlock = 5
	close(bi.blockListener.initialBlockHeightObtained)

	_, err := bi.GetConfirmedBlockHeight(ctx)
	assert.Regexp(t, "PD011308", err)

	// do not start block listener
	bi.startOrReset()

	bh, err := bi.GetBlockListenerHeight(ctx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(5), bh)

	// Notify starting at block 5
	for i := 5; i < len(blocks); i++ {
		bi.blockListener.notifyBlock(blocks[i])
	}

	// Randomly notify below that too, which will be ignored
	bi.blockListener.notifyBlock(blocks[1])

	for i := 5; i < len(blocks)-bi.requiredConfirmations; i++ {
		b := <-bi.utBatchNotify
		assert.Len(t, b.blocks, 1) // We should get one block per batch
		assert.Equal(t, blocks[i], b.blocks[0])
	}

	ch, err := bi.GetConfirmedBlockHeight(ctx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(9), ch)
}

func TestBlockIndexerCancelledBeforeCurrentBlock(t *testing.T) {
	_, bi, _, blDone := newTestBlockIndexer(t)
	defer blDone()

	bi.nextBlock = nil
	bi.dispatcherDone = make(chan struct{})
	bi.processorDone = make(chan struct{})

	// close the block listener ctx
	closed, close := context.WithCancel(context.Background())
	close()
	bi.startup(closed)

	<-bi.dispatcherDone
	<-bi.processorDone

}

func TestBatching(t *testing.T) {
	_, bi, mRPC, blDone := newTestBlockIndexer(t)
	defer blDone()

	blocks, receipts := testBlockArray(t, 10)
	mockBlocksRPCCalls(mRPC, blocks, receipts)

	bi.batchSize = 5

	bi.startOrReset() // do not start block listener

	// Notify starting at block 5
	for i := 5; i < len(blocks); i++ {
		bi.blockListener.notifyBlock(blocks[i])
	}

	// Randomly notify below that too, which will be ignored
	bi.blockListener.notifyBlock(blocks[1])

	for i := 0; i < len(blocks)-bi.requiredConfirmations; i += 5 {
		batch := <-bi.utBatchNotify
		assert.Len(t, batch.blocks, 5)
		for i2, b := range batch.blocks {
			assert.Equal(t, blocks[i+i2], b)
		}
	}
}

func TestBlockIndexerListenFromCurrentUsingCheckpointBlock(t *testing.T) {
	_, bi, mRPC, blDone := newTestBlockIndexer(t)
	defer blDone()

	blocks, receipts := testBlockArray(t, 15)
	mockBlocksRPCCalls(mRPC, blocks, receipts)

	bi.persistence.DB().Table("indexed_blocks").Create(&IndexedBlock{
		Number: 12345,
		Hash:   types.MustParseBytes32(types.RandHex(32)),
	})

	bi.startOrReset() // do not start block listener

	assert.Equal(t, ethtypes.HexUint64(12346), *bi.nextBlock)
}

func TestBlockIndexerHandleReorgInConfirmationWindow1(t *testing.T) {
	// test where the reorg happens at the edge of the confirmation window
	testBlockIndexerHandleReorgInConfirmationWindow(t,
		10, // blocks in chain before re-org
		5,  // blocks that remain from original chain after re-org
		5,  // required confirmations
	)
}

func TestBlockIndexerHandleReorgInConfirmationWindow2(t *testing.T) {
	// test where the reorg happens replacing some blocks
	// WE ALREADY CONFIRMED - meaning we dispatched them incorrectly
	// because the confirmations were not tuned correctly
	testBlockIndexerHandleReorgInConfirmationWindow(t,
		10, // blocks in chain before re-org
		0,  // blocks that remain from original chain after re-org
		5,  // required confirmations
	)
}

func TestBlockIndexerHandleReorgInConfirmationWindow3(t *testing.T) {
	// test without confirmations, so everything is a problem
	testBlockIndexerHandleReorgInConfirmationWindow(t,
		10, // blocks in chain before re-org
		0,  // blocks that remain from original chain after re-org
		0,  // required confirmations
	)
}

func TestBlockIndexerHandleReorgInConfirmationWindow4(t *testing.T) {
	// test of a re-org of one
	testBlockIndexerHandleReorgInConfirmationWindow(t,
		5, // blocks in chain before re-org
		4, // blocks that remain from original chain after re-org
		4, // required confirmations
	)
}

func checkBlocksSequential(t *testing.T, desc string, blocks []*BlockInfoJSONRPC, receipts map[string][]*TXReceiptJSONRPC) {
	blockSummaries := make([]string, len(blocks))
	var lastBlock *BlockInfoJSONRPC
	invalid := false
	for i, b := range blocks {
		assert.NotEmpty(t, b.Hash)
		blockSummaries[i] = fmt.Sprintf("%d/%s [rok=%t]", b.Number, b.Hash, receipts[b.Hash.String()] != nil)
		if i == 0 {
			assert.NotEmpty(t, b.ParentHash)
		} else if lastBlock.Hash.String() != b.ParentHash.String() {
			invalid = true
		}
		lastBlock = b
	}
	fmt.Printf("%s: %s\n", desc, strings.Join(blockSummaries, ",\n"))
	if invalid {
		panic("wrong sequence") // aid to writing tests that build sequences
	}
}

func testBlockIndexerHandleReorgInConfirmationWindow(t *testing.T, blockLenBeforeReorg, overlap, reqConf int) {
	_, bi, mRPC, blDone := newTestBlockIndexer(t)
	defer blDone()

	bi.requiredConfirmations = reqConf

	blocksBeforeReorg, receipts := testBlockArray(t, blockLenBeforeReorg)
	blocksAfterReorg, receiptsAfterReorg := testBlockArray(t, blockLenBeforeReorg+overlap)
	dangerArea := len(blocksAfterReorg) - overlap
	for i := 0; i < len(blocksAfterReorg); i++ {
		receipts[blocksAfterReorg[i].Hash.String()] = receiptsAfterReorg[blocksAfterReorg[i].Hash.String()]
		if i < overlap {
			b := blocksBeforeReorg[i]
			// Copy the blocks over from the before-reorg chain
			blockCopy := *b
			blocksAfterReorg[i] = &blockCopy
		}
	}
	if overlap > 0 {
		// Re-wire the first forked block
		blocksAfterReorg[overlap].ParentHash = blocksAfterReorg[overlap-1].Hash
	}
	checkBlocksSequential(t, "before", blocksBeforeReorg, receipts)
	checkBlocksSequential(t, "after ", blocksAfterReorg, receipts)

	var isAfterReorg atomic.Bool
	notificationsDone := make(chan struct{})
	mockBlocksRPCCallsDynamic(mRPC, func(args mock.Arguments) ([]*BlockInfoJSONRPC, map[string][]*TXReceiptJSONRPC) {
		blockNumber := -1
		if args[2].(string) == "eth_getBlockByNumber" {
			blockNumber = int(args[3].(ethtypes.HexUint64))
		}
		if isAfterReorg.Load() {
			return blocksAfterReorg, receipts
		} else {
			// we instigate the re-org when we've returned all the blocks
			if blockNumber >= len(blocksBeforeReorg) {
				isAfterReorg.Store(true)
				go func() {
					defer close(notificationsDone)
					// Simulate the modified blocks only coming in with delays
					for i := overlap; i < len(blocksAfterReorg); i++ {
						time.Sleep(100 * time.Microsecond)
						bi.blockListener.notifyBlock(blocksAfterReorg[i])
					}
				}()
			}
			return blocksBeforeReorg, receipts
		}
	})

	bi.startOrReset() // do not start block listener

	for i := 0; i < len(blocksAfterReorg)-bi.requiredConfirmations; i++ {
		b := <-bi.utBatchNotify
		assert.Len(t, b.blocks, 1) // We should get one block per batch
		if i >= overlap && i < (dangerArea-reqConf) {
			// This would be a bad situation in reality, where a reorg crossed the confirmations
			// boundary. An indication someone incorrectly configured their confirmations
			assert.Equal(t, b.blocks[0].Hash.String(), blocksBeforeReorg[i].Hash.String())
			assert.Equal(t, b.blocks[0].Number, blocksBeforeReorg[i].Number)
		} else {
			assert.Equal(t, b.blocks[0].Hash.String(), blocksAfterReorg[i].Hash.String())
			assert.Equal(t, b.blocks[0].Number, blocksAfterReorg[i].Number)
		}
	}
	// Wait for the notifications to go through
	<-notificationsDone

}

func TestBlockIndexerHandleRandomConflictingBlockNotification(t *testing.T) {
	_, bi, mRPC, blDone := newTestBlockIndexer(t)
	defer blDone()

	bi.requiredConfirmations = 5

	blocks, receipts := testBlockArray(t, 50)

	randBlock := &BlockInfoJSONRPC{
		Number:     3,
		Hash:       ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32)),
		ParentHash: ethtypes.MustNewHexBytes0xPrefix(types.RandHex(32)),
	}

	sentRandom := false
	mockBlocksRPCCallsDynamic(mRPC, func(args mock.Arguments) ([]*BlockInfoJSONRPC, map[string][]*TXReceiptJSONRPC) {
		if !sentRandom && args[3].(ethtypes.HexUint64) == 4 {
			sentRandom = true
			bi.blockListener.notifyBlock(randBlock)
			// Give notification handler likelihood to run before we continue the by-number getting
			time.Sleep(1 * time.Millisecond)
		}
		return blocks, receipts
	})

	bi.startOrReset() // do not start block listener

	for i := 0; i < len(blocks)-bi.requiredConfirmations; i++ {
		b := <-bi.utBatchNotify
		assert.Len(t, b.blocks, 1) // We should get one block per batch
		assert.Equal(t, blocks[i], b.blocks[0])
	}
}

func TestBlockIndexerResetsAfterHashLookupFail(t *testing.T) {
	_, bi, mRPC, blDone := newTestBlockIndexer(t)
	defer blDone()

	blocks, receipts := testBlockArray(t, 5)

	sentFail := false
	mockBlocksRPCCallsDynamic(mRPC, func(args mock.Arguments) ([]*BlockInfoJSONRPC, map[string][]*TXReceiptJSONRPC) {
		if !sentFail &&
			args[2].(string) == "eth_getBlockReceipts" &&
			args[3].(ethtypes.HexBytes0xPrefix).Equals(blocks[2].Hash) {
			sentFail = true
			// Send back a not found, to send us round the reset loop
			return []*BlockInfoJSONRPC{}, map[string][]*TXReceiptJSONRPC{}
		}
		return blocks, receipts
	})

	bi.startOrReset() // do not start block listener

	for i := 0; i < len(blocks); i++ {
		b := <-bi.utBatchNotify
		assert.Len(t, b.blocks, 1) // We should get one block per batch
		assert.Equal(t, blocks[i], b.blocks[0])
	}

	assert.True(t, sentFail)
}

func TestBlockIndexerDispatcherFallsBehindHead(t *testing.T) {
	_, bi, mRPC, blDone := newTestBlockIndexer(t)
	defer blDone()

	bi.requiredConfirmations = 5

	blocks, receipts := testBlockArray(t, 30)
	mockBlocksRPCCalls(mRPC, blocks, receipts)

	bi.startOrReset() // do not start block listener

	// Notify all the blocks before we process any
	assert.True(t, bi.blockListener.unstableHeadLength > len(blocks))
	for _, b := range blocks {
		bi.blockListener.notifyBlock(b)
	}

	// The dispatches should have been added, until it got too far ahead
	// and then set to nil.
	for bi.newHeadToAdd != nil {
		time.Sleep(1 * time.Millisecond)
	}

	for i := 0; i < len(blocks)-bi.requiredConfirmations; i++ {
		b := <-bi.utBatchNotify
		assert.Len(t, b.blocks, 1) // We should get one block per batch
		assert.Equal(t, blocks[i], b.blocks[0])
	}

}

func TestBlockIndexerStartFromBlock(t *testing.T) {
	ctx, bl, _, done := newTestBlockListener(t)
	defer done()

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows([]string{}))
	_, err = newBlockIndexer(ctx, &Config{
		FromBlock: types.RawJSON(`"pending"`),
	}, p.P, bl)
	assert.Regexp(t, "PD011300.*pending", err)

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows([]string{}))
	bi, err := newBlockIndexer(ctx, &Config{
		FromBlock: types.RawJSON(`"latest"`),
	}, p.P, bl)
	assert.NoError(t, err)
	assert.Nil(t, bi.fromBlock)

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows([]string{}))
	bi, err = newBlockIndexer(ctx, &Config{
		FromBlock: types.RawJSON(`null`),
	}, p.P, bl)
	assert.NoError(t, err)
	assert.Nil(t, bi.fromBlock)

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows([]string{}))
	bi, err = newBlockIndexer(ctx, &Config{}, p.P, bl)
	assert.NoError(t, err)
	assert.Nil(t, bi.fromBlock)

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows([]string{}))
	bi, err = newBlockIndexer(ctx, &Config{
		FromBlock: types.RawJSON(`123`),
	}, p.P, bl)
	assert.NoError(t, err)
	assert.Equal(t, ethtypes.HexUint64(123), *bi.fromBlock)

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows([]string{}))
	bi, err = newBlockIndexer(ctx, &Config{
		FromBlock: types.RawJSON(`"0x7b"`),
	}, p.P, bl)
	assert.NoError(t, err)
	assert.Equal(t, ethtypes.HexUint64(123), *bi.fromBlock)

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows([]string{}))
	_, err = newBlockIndexer(ctx, &Config{
		FromBlock: types.RawJSON(`!!! bad JSON`),
	}, p.P, bl)
	assert.Regexp(t, "PD011300", err)

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows([]string{}))
	_, err = newBlockIndexer(ctx, &Config{
		FromBlock: types.RawJSON(`false`),
	}, p.P, bl)
	assert.Regexp(t, "PD011300", err)
}

func TestBlockIndexerBadStream(t *testing.T) {
	ctx, bl, _, done := newTestBlockListener(t)
	defer done()

	p, err := mockpersistence.NewSQLMockProvider()
	assert.NoError(t, err)

	p.Mock.ExpectQuery("SELECT.*event_streams").WillReturnRows(sqlmock.NewRows([]string{
		"id", "abi",
	}).AddRow(
		uuid.New().String(), `!!!bad JSON`,
	))
	_, err = newBlockIndexer(ctx, &Config{}, p.P, bl)
	assert.Regexp(t, "PD011303", err)
}

func TestGetIndexedTransactionByHashErrors(t *testing.T) {

	ctx, bi, _, p, done := newMockBlockIndexer(t, &Config{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*indexed_transactions").WillReturnRows(sqlmock.NewRows([]string{}))

	res, err := bi.GetIndexedTransactionByHash(ctx, types.Bytes32(types.RandBytes(32)))
	assert.NoError(t, err)
	assert.Nil(t, res)

	p.Mock.ExpectQuery("SELECT.*indexed_transactions").WillReturnError(fmt.Errorf("pop"))

	_, err = bi.GetIndexedTransactionByHash(ctx, types.Bytes32(types.RandBytes(32)))
	assert.Regexp(t, "pop", err)

}

func TestBlockIndexerWaitForTransaction(t *testing.T) {
	ctx, bi, mRPC, blDone := newTestBlockIndexer(t)
	defer blDone()

	blocks, receipts := testBlockArray(t, 5)
	mockBlocksRPCCalls(mRPC, blocks, receipts)

	txHash := types.Bytes32(receipts[blocks[2].Hash.String()][0].TransactionHash)
	gotTX := make(chan struct{})
	go func() {
		defer close(gotTX)
		tx, err := bi.WaitForTransaction(ctx, txHash)
		assert.NoError(t, err)
		assert.Equal(t, ethtypes.HexUint64(tx.BlockNumber), blocks[2].Number)
		assert.Equal(t, txHash, tx.Hash)
	}()

	// Wait for initial query to fail
	for bi.txWaiters.InFlightCount() == 0 {
		time.Sleep(1 * time.Millisecond)
	}

	bi.startOrReset() // do not start block listener

	for i := 0; i < len(blocks); i++ {
		b := <-bi.utBatchNotify
		assert.Len(t, b.blocks, 1) // We should get one block per batch
		assert.Equal(t, blocks[i], b.blocks[0])
	}

	<-gotTX

	tx, err := bi.WaitForTransaction(ctx, txHash)
	assert.NoError(t, err)
	assert.Equal(t, TXResult_SUCCESS, tx.Result.V())
	assert.Equal(t, ethtypes.HexUint64(tx.BlockNumber), blocks[2].Number)
	assert.Equal(t, txHash, tx.Hash)
}

func TestBlockIndexerWaitForTransactionRevert(t *testing.T) {
	ctx, bi, mRPC, blDone := newTestBlockIndexer(t)
	defer blDone()

	blocks, receipts := testBlockArray(t, 5)
	mockBlocksRPCCalls(mRPC, blocks, receipts)

	receipt := receipts[blocks[2].Hash.String()][0]
	receipt.Status = ethtypes.NewHexInteger64(0) // reverted
	txHash := types.Bytes32(receipt.TransactionHash)
	gotTX := make(chan struct{})
	go func() {
		defer close(gotTX)
		tx, err := bi.WaitForTransaction(ctx, txHash)
		assert.NoError(t, err)
		assert.Equal(t, ethtypes.HexUint64(tx.BlockNumber), blocks[2].Number)
		assert.Equal(t, txHash, tx.Hash)
	}()

	// Wait for initial query to fail
	for bi.txWaiters.InFlightCount() == 0 {
		time.Sleep(1 * time.Millisecond)
	}

	bi.startOrReset() // do not start block listener

	for i := 0; i < len(blocks); i++ {
		b := <-bi.utBatchNotify
		assert.Len(t, b.blocks, 1) // We should get one block per batch
		assert.Equal(t, blocks[i], b.blocks[0])
	}

	<-gotTX

	tx, err := bi.WaitForTransaction(ctx, txHash)
	assert.NoError(t, err)
	assert.Equal(t, TXResult_FAILURE, tx.Result.V())
	assert.Equal(t, ethtypes.HexUint64(tx.BlockNumber), blocks[2].Number)
	assert.Equal(t, txHash, tx.Hash)
}

func TestWaitForTransactionErrorCases(t *testing.T) {

	ctx, bi, _, p, done := newMockBlockIndexer(t, &Config{})
	defer done()

	p.Mock.ExpectQuery("SELECT.*indexed_transactions").WillReturnError(fmt.Errorf("pop"))

	_, err := bi.WaitForTransaction(ctx, types.Bytes32(types.RandBytes(32)))
	assert.Regexp(t, "pop", err)

}
