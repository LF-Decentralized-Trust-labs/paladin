/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package pldclient

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

type BlockIndex interface {
	RPCModule
}

// This is necessary because there's no way to introspect function parameter names via reflection
var blockIndexInfo = &rpcModuleInfo{
	group: "bidx",
	methodInfo: map[string]RPCMethodInfo{
		"bidx_getBlockByNumber": {
			Inputs: []string{"blockNumber"},
			Output: "block",
		},
		"bidx_getTransactionByHash": {
			Inputs: []string{"blockHash"},
			Output: "transaction",
		},
		"bidx_getTransactionByNonce": {
			Inputs: []string{"from", "nonce"},
			Output: "transaction",
		},
		"bidx_getBlockTransactionsByNumber": {
			Inputs: []string{"blockNumber"},
			Output: "transactions",
		},
		"bidx_getTransactionEventsByHash": {
			Inputs: []string{"transactionHash"},
			Output: "events",
		},
		"bidx_queryIndexedBlocks": {
			Inputs: []string{"query"},
			Output: "blocks",
		},
		"bidx_queryIndexedTransactions": {
			Inputs: []string{"query"},
			Output: "transactions",
		},
		"bidx_queryIndexedEvents": {
			Inputs: []string{"query"},
			Output: "events",
		},
		"bidx_getConfirmedBlockHeight": {
			Inputs: []string{},
			Output: "blockHeight",
		},
		"bidx_decodeTransactionEvents": {
			Inputs: []string{"transactionHash", "abi", "resultFormat"},
			Output: "events",
		},
	},
}

type blockIndex struct {
	*rpcModuleInfo
	c *paladinClient
}

func (c *paladinClient) BlockIndex() BlockIndex {
	return &blockIndex{rpcModuleInfo: blockIndexInfo, c: c}
}

func (r *blockIndex) GetBlockByNumber(ctx context.Context, blockNumber pldtypes.HexUint64) (block *pldapi.IndexedBlock, err error) {
	err = r.c.CallRPC(ctx, &blockNumber, "bidx_getBlockByNumber", blockNumber)
	return
}

func (r *blockIndex) GetTransactionByHash(ctx context.Context, transactionHash pldtypes.Bytes32) (transaction *pldapi.IndexedTransaction, err error) {
	err = r.c.CallRPC(ctx, &transaction, "bidx_getTransactionByHash", transactionHash)
	return
}

func (r *blockIndex) GetTransactionByNonce(ctx context.Context, from pldtypes.EthAddress, nonce pldtypes.HexUint64) (transaction *pldapi.IndexedTransaction, err error) {
	err = r.c.CallRPC(ctx, &transaction, "bidx_getTransactionByNonce", from, nonce)
	return
}

func (r *blockIndex) GetBlockTransactionsByNumber(ctx context.Context, blockNumber pldtypes.HexUint64) (transactions []*pldapi.IndexedTransaction, err error) {
	err = r.c.CallRPC(ctx, &transactions, "bidx_getBlockTransactionsByNumber", blockNumber)
	return
}

func (r *blockIndex) GetTransactionEventsByHash(ctx context.Context, transactionHash pldtypes.Bytes32) (transactions []*pldapi.IndexedEvent, err error) {
	err = r.c.CallRPC(ctx, &transactions, "bidx_getTransactionEventsByHash", transactionHash)
	return
}

func (r *blockIndex) QueryIndexedBlocks(ctx context.Context, query *query.QueryJSON) (blocks []*pldapi.IndexedBlock, err error) {
	err = r.c.CallRPC(ctx, &blocks, "bidx_queryIndexedBlocks", query)
	return
}

func (r *blockIndex) QueryIndexedTransactions(ctx context.Context, query *query.QueryJSON) (transactions []*pldapi.IndexedTransaction, err error) {
	err = r.c.CallRPC(ctx, &transactions, "bidx_queryIndexedTransactions", query)
	return
}

func (r *blockIndex) QueryIndexedEvents(ctx context.Context, query *query.QueryJSON) (events []*pldapi.IndexedEvent, err error) {
	err = r.c.CallRPC(ctx, &events, "bidx_queryIndexedEvents", query)
	return
}

func (r *blockIndex) GetConfirmedBlockHeight(ctx context.Context) (blockHeight pldtypes.HexUint64, err error) {
	err = r.c.CallRPC(ctx, &blockHeight, "bidx_getConfirmedBlockHeight")
	return
}

func (r *blockIndex) DecodeTransactionEvents(ctx context.Context, transactionHash pldtypes.Bytes32, abi abi.ABI, resultFormat pldtypes.JSONFormatOptions) (events []*pldapi.EventWithData, err error) {
	err = r.c.CallRPC(ctx, &events, "bidx_decodeTransactionEvents", transactionHash, abi, resultFormat)
	return
}
