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

	"github.com/go-resty/resty/v2"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockIndexRPCCalls(t *testing.T) {

	ctx, rpcBlock, bi, biDone := newBlockIndexerWithOneBlock(t)
	defer biDone()

	rpc, rpcDone := newTestRPCServer(t, ctx, bi)
	defer rpcDone()

	var idxBlock *pldapi.IndexedBlock
	err := rpc.CallRPC(ctx, &idxBlock, "bidx_getBlockByNumber", rpcBlock.Number)
	require.NoError(t, err)
	assert.Equal(t, rpcBlock.Hash.String(), idxBlock.Hash.String())

	var idxTxn *pldapi.IndexedTransaction
	err = rpc.CallRPC(ctx, &idxTxn, "bidx_getTransactionByHash", rpcBlock.Transactions[0].Hash)
	require.NoError(t, err)
	assert.Equal(t, rpcBlock.Transactions[0].Hash.String(), idxTxn.Hash.String())

	err = rpc.CallRPC(ctx, &idxTxn, "bidx_getTransactionByNonce", rpcBlock.Transactions[0].From, rpcBlock.Transactions[0].Nonce)
	require.NoError(t, err)
	assert.Equal(t, rpcBlock.Transactions[0].Hash.String(), idxTxn.Hash.String())

	var idxTxns []*pldapi.IndexedTransaction
	err = rpc.CallRPC(ctx, &idxTxns, "bidx_getBlockTransactionsByNumber", rpcBlock.Number)
	require.NoError(t, err)
	assert.Equal(t, rpcBlock.Transactions[0].Hash.String(), idxTxns[0].Hash.String())

	var idxEvents []*pldapi.IndexedEvent
	err = rpc.CallRPC(ctx, &idxEvents, "bidx_getTransactionEventsByHash", rpcBlock.Transactions[0].Hash)
	require.NoError(t, err)
	assert.Equal(t, rpcBlock.Transactions[0].Hash.String(), idxEvents[0].TransactionHash.String())

	var idxBlocks []*pldapi.IndexedBlock
	err = rpc.CallRPC(ctx, &idxBlocks, "bidx_queryIndexedBlocks", query.NewQueryBuilder().Equal("hash", rpcBlock.Hash).Limit(1).Query())
	require.NoError(t, err)
	assert.Equal(t, rpcBlock.Hash.String(), idxBlocks[0].Hash.String())

	err = rpc.CallRPC(ctx, &idxTxns, "bidx_queryIndexedTransactions", query.NewQueryBuilder().Equal("hash", rpcBlock.Transactions[0].Hash).Limit(1).Query())
	require.NoError(t, err)
	assert.Equal(t, rpcBlock.Transactions[0].Hash.String(), idxTxns[0].Hash.String())

	err = rpc.CallRPC(ctx, &idxEvents, "bidx_queryIndexedEvents", query.NewQueryBuilder().
		Equal("blockNumber", rpcBlock.Number).
		Equal("transactionIndex", 0).
		Equal("logIndex", 2).
		Limit(1).Query())
	require.NoError(t, err)
	assert.Equal(t, rpcBlock.Transactions[0].Hash.String(), idxEvents[0].TransactionHash.String())
	assert.Equal(t, int64(2), idxEvents[0].LogIndex)

	var blockHeight int64
	err = rpc.CallRPC(ctx, &blockHeight, "bidx_getConfirmedBlockHeight")
	require.NoError(t, err)
	assert.Equal(t, int64(0), blockHeight)
}

func newBlockIndexerWithOneBlock(t *testing.T) (context.Context, *BlockInfoJSONRPC, *blockIndexer, func()) {
	ctx, bi, mRPC, done := newTestBlockIndexer(t)

	blocks, receipts := testBlockArray(t, 1)
	mockBlocksRPCCalls(mRPC, blocks, receipts)

	utBatchNotify := make(chan []*pldapi.IndexedBlock)
	addBlockPostCommit(bi, func(blocks []*pldapi.IndexedBlock) { utBatchNotify <- blocks })

	bi.startOrReset() // do not start block listener
	<-utBatchNotify

	return ctx, blocks[0], bi, done

}

func newTestRPCServer(t *testing.T, ctx context.Context, bi *blockIndexer) (rpcclient.Client, func()) {

	s, err := rpcserver.NewRPCServer(ctx, &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{
			HTTPServerConfig: pldconf.HTTPServerConfig{Address: confutil.P("127.0.0.1"), Port: confutil.P(0)},
		},
		WS: pldconf.RPCServerConfigWS{Disabled: true},
	})
	require.NoError(t, err)
	err = s.Start()
	require.NoError(t, err)

	s.Register(bi.RPCModule())

	c := rpcclient.WrapRestyClient(resty.New().SetBaseURL(fmt.Sprintf("http://%s", s.HTTPAddr())))

	return c, s.Stop

}
