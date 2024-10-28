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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type PTX interface {
	RPCModule

	SendTransaction(ctx context.Context, tx *pldapi.TransactionInput) (txID *uuid.UUID, err error)
	SendTransactions(ctx context.Context, txs []*pldapi.TransactionInput) (txIDs []uuid.UUID, err error)
	Call(ctx context.Context, tx *pldapi.TransactionCall) (data tktypes.RawJSON, err error)

	GetTransaction(ctx context.Context, txID uuid.UUID) (receipt *pldapi.Transaction, err error)
	GetTransactionFull(ctx context.Context, txID uuid.UUID) (receipt *pldapi.TransactionFull, err error)
	GetTransactionByIdempotencyKey(ctx context.Context, idempotencyKey string) (tx *pldapi.Transaction, err error)
	QueryTransactions(ctx context.Context, jq *query.QueryJSON) (txs []*pldapi.Transaction, err error)
	QueryTransactionsFull(ctx context.Context, jq *query.QueryJSON) (txs []*pldapi.TransactionFull, err error)

	GetTransactionReceipt(ctx context.Context, txID uuid.UUID) (receipt *pldapi.TransactionReceipt, err error)
	QueryTransactionReceipts(ctx context.Context, jq *query.QueryJSON) (receipts []*pldapi.TransactionReceipt, err error)
	DecodeError(ctx context.Context, revertData tktypes.HexBytes, dataFormat tktypes.JSONFormatOptions) (decodedError *pldapi.DecodedError, err error)

	ResolveVerifier(ctx context.Context, keyIdentifier string, algorithm string, verifierType string) (verifier string, err error)
}

// This is necessary because there's no way to introspect function parameter names via reflection
var ptxInfo = &rpcModuleInfo{
	group: "ptx",
	methodInfo: map[string]RPCMethodInfo{
		"ptx_sendTransaction": {
			Inputs: []string{"transaction"},
			Output: "transactionId",
		},
		"ptx_sendTransactions": {
			Inputs: []string{"transactions"},
			Output: "transactionIds",
		},
		"ptx_call": {
			Inputs: []string{"transaction"},
			Output: "result",
		},
		"ptx_getTransaction": {
			Inputs: []string{"transactionId"},
			Output: "transaction",
		},
		"ptx_getTransactionFull": {
			Inputs: []string{"transactionId"},
			Output: "transaction",
		},
		"ptx_getTransactionByIdempotencyKey": {
			Inputs: []string{"idempotencyKey"},
			Output: "transaction",
		},
		"ptx_queryTransactions": {
			Inputs: []string{"query"},
			Output: "transactions",
		},
		"ptx_queryTransactionsFull": {
			Inputs: []string{"query"},
			Output: "transactions",
		},
		"ptx_getTransactionReceipt": {
			Inputs: []string{"transactionId"},
			Output: "receipt",
		},
		"ptx_queryTransactionReceipts": {
			Inputs: []string{"query"},
			Output: "receipts",
		},
		"ptx_decodeError": {
			Inputs: []string{"revertData", "dataFormat"},
			Output: "decodedError",
		},
		"ptx_resolveVerifier": {
			Inputs: []string{"keyIdentifier", "algorithm", "verifierType"},
			Output: "receipts",
		},
	},
}

type ptx struct {
	*rpcModuleInfo
	c *paladinClient
}

func (c *paladinClient) PTX() PTX {
	return &ptx{rpcModuleInfo: ptxInfo, c: c}
}

func (p *ptx) SendTransaction(ctx context.Context, tx *pldapi.TransactionInput) (txID *uuid.UUID, err error) {
	err = p.c.CallRPC(ctx, &txID, "ptx_sendTransaction", tx)
	return
}

func (p *ptx) SendTransactions(ctx context.Context, txs []*pldapi.TransactionInput) (txIDs []uuid.UUID, err error) {
	err = p.c.CallRPC(ctx, &txIDs, "ptx_sendTransactions", txs)
	return
}

func (p *ptx) Call(ctx context.Context, tx *pldapi.TransactionCall) (data tktypes.RawJSON, err error) {
	err = p.c.CallRPC(ctx, &data, "ptx_call", tx)
	return
}

func (p *ptx) GetTransaction(ctx context.Context, txID uuid.UUID) (tx *pldapi.Transaction, err error) {
	err = p.c.CallRPC(ctx, &tx, "ptx_getTransaction", txID)
	return
}

func (p *ptx) GetTransactionFull(ctx context.Context, txID uuid.UUID) (tx *pldapi.TransactionFull, err error) {
	err = p.c.CallRPC(ctx, &tx, "ptx_getTransactionFull", txID)
	return
}

func (p *ptx) GetTransactionByIdempotencyKey(ctx context.Context, idempotencyKey string) (tx *pldapi.Transaction, err error) {
	err = p.c.CallRPC(ctx, &tx, "ptx_getTransactionByIdempotencyKey", idempotencyKey)
	return
}

func (p *ptx) QueryTransactions(ctx context.Context, jq *query.QueryJSON) (txs []*pldapi.Transaction, err error) {
	err = p.c.CallRPC(ctx, &txs, "ptx_queryTransactions", jq)
	return
}

func (p *ptx) QueryTransactionsFull(ctx context.Context, jq *query.QueryJSON) (txs []*pldapi.TransactionFull, err error) {
	err = p.c.CallRPC(ctx, &txs, "ptx_queryTransactionsFull", jq)
	return
}

func (p *ptx) GetTransactionReceipt(ctx context.Context, txID uuid.UUID) (receipt *pldapi.TransactionReceipt, err error) {
	err = p.c.CallRPC(ctx, &receipt, "ptx_getTransactionReceipt", txID)
	return
}

func (p *ptx) QueryTransactionReceipts(ctx context.Context, jq *query.QueryJSON) (receipts []*pldapi.TransactionReceipt, err error) {
	err = p.c.CallRPC(ctx, &receipts, "ptx_queryTransactionReceipts", jq)
	return
}

func (p *ptx) DecodeError(ctx context.Context, revertData tktypes.HexBytes, dataFormat tktypes.JSONFormatOptions) (decodedError *pldapi.DecodedError, err error) {
	err = p.c.CallRPC(ctx, &decodedError, "ptx_decodeError", revertData, dataFormat)
	return
}

func (p *ptx) ResolveVerifier(ctx context.Context, keyIdentifier string, algorithm string, verifierType string) (verifier string, err error) {
	err = p.c.CallRPC(ctx, &verifier, "ptx_resolveVerifier", keyIdentifier, algorithm, verifierType)
	return
}
