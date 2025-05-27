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
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/query"
	"github.com/kaleido-io/paladin/sdk/go/pkg/rpcclient"
)

type PTX interface {
	RPCModule

	SendTransaction(ctx context.Context, tx *pldapi.TransactionInput) (txID *uuid.UUID, err error)
	SendTransactions(ctx context.Context, txs []*pldapi.TransactionInput) (txIDs []uuid.UUID, err error)
	PrepareTransaction(ctx context.Context, tx *pldapi.TransactionInput) (txID *uuid.UUID, err error)
	PrepareTransactions(ctx context.Context, txs []*pldapi.TransactionInput) (txIDs []uuid.UUID, err error)
	UpdateTransaction(ctx context.Context, id uuid.UUID, tx *pldapi.TransactionInput) (txID *uuid.UUID, err error)
	Call(ctx context.Context, tx *pldapi.TransactionCall) (data pldtypes.RawJSON, err error)

	GetTransaction(ctx context.Context, txID uuid.UUID) (receipt *pldapi.Transaction, err error)
	GetTransactionFull(ctx context.Context, txID uuid.UUID) (receipt *pldapi.TransactionFull, err error)
	GetTransactionByIdempotencyKey(ctx context.Context, idempotencyKey string) (tx *pldapi.Transaction, err error)
	QueryTransactions(ctx context.Context, jq *query.QueryJSON) (txs []*pldapi.Transaction, err error)
	QueryTransactionsFull(ctx context.Context, jq *query.QueryJSON) (txs []*pldapi.TransactionFull, err error)

	GetTransactionReceipt(ctx context.Context, txID uuid.UUID) (receipt *pldapi.TransactionReceipt, err error)
	GetTransactionReceiptFull(ctx context.Context, txID uuid.UUID) (receipt *pldapi.TransactionReceiptFull, err error)
	GetDomainReceipt(ctx context.Context, domain string, txID uuid.UUID) (domainReceipt pldtypes.RawJSON, err error)
	GetStateReceipt(ctx context.Context, txID uuid.UUID) (stateReceipt *pldapi.TransactionStates, err error)
	QueryTransactionReceipts(ctx context.Context, jq *query.QueryJSON) (receipts []*pldapi.TransactionReceipt, err error)
	GetPreparedTransaction(ctx context.Context, txID uuid.UUID) (preparedTransaction *pldapi.PreparedTransaction, err error)
	QueryPreparedTransactions(ctx context.Context, jq *query.QueryJSON) (preparedTransactions []*pldapi.PreparedTransaction, err error)
	DecodeError(ctx context.Context, revertData pldtypes.HexBytes, dataFormat pldtypes.JSONFormatOptions) (decodedError *pldapi.ABIDecodedData, err error)
	DecodeCall(ctx context.Context, callData pldtypes.HexBytes, dataFormat pldtypes.JSONFormatOptions) (decodedCall *pldapi.ABIDecodedData, err error)
	DecodeEvent(ctx context.Context, topics []pldtypes.Bytes32, eventData pldtypes.HexBytes, dataFormat pldtypes.JSONFormatOptions) (decodedEvent *pldapi.ABIDecodedData, err error)

	StoreABI(ctx context.Context, abi abi.ABI) (storedABI *pldapi.StoredABI, err error)
	GetStoredABI(ctx context.Context, hashRef pldtypes.Bytes32) (storedABI *pldapi.StoredABI, err error)
	QueryStoredABIs(ctx context.Context, jq *query.QueryJSON) (storedABIs []*pldapi.StoredABI, err error)

	ResolveVerifier(ctx context.Context, keyIdentifier string, algorithm string, verifierType string) (verifier string, err error)

	CreateReceiptListener(ctx context.Context, listener *pldapi.TransactionReceiptListener) (success bool, err error)
	QueryReceiptListeners(ctx context.Context, jq *query.QueryJSON) (listeners []*pldapi.TransactionReceiptListener, err error)
	GetReceiptListener(ctx context.Context, listenerName string) (listener *pldapi.TransactionReceiptListener, err error)
	StartReceiptListener(ctx context.Context, listenerName string) (success bool, err error)
	StopReceiptListener(ctx context.Context, listenerName string) (success bool, err error)
	DeleteReceiptListener(ctx context.Context, listenerName string) (success bool, err error)

	CreateBlockchainEventListener(ctx context.Context, listener *pldapi.BlockchainEventListener) (success bool, err error)
	QueryBlockchainEventListeners(ctx context.Context, jq *query.QueryJSON) (listeners []*pldapi.BlockchainEventListener, err error)
	GetBlockchainEventListener(ctx context.Context, listenerName string) (listener *pldapi.BlockchainEventListener, err error)
	StartBlockchainEventListener(ctx context.Context, listenerName string) (success bool, err error)
	StopBlockchainEventListener(ctx context.Context, listenerName string) (success bool, err error)
	DeleteBlockchainEventListener(ctx context.Context, listenerName string) (success bool, err error)
	GetBlockchainEventListenerStatus(ctx context.Context, name string) (*pldapi.BlockchainEventListenerStatus, error)

	SubscribeReceipts(ctx context.Context, listenerName string) (sub rpcclient.Subscription, err error)
	SubscribeBlockchainEvents(ctx context.Context, listenerName string) (sub rpcclient.Subscription, err error)
}

var ptxSubscriptionConfig = rpcclient.SubscriptionConfig{
	SubscribeMethod:    "ptx_subscribe",
	UnsubscribeMethod:  "ptx_unsubscribe",
	NotificationMethod: "ptx_subscription",
	AckMethod:          "ptx_ack",
	NackMethod:         "ptx_nack",
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
		"ptx_prepareTransaction": {
			Inputs: []string{"transaction"},
			Output: "transactionId",
		},
		"ptx_prepareTransactions": {
			Inputs: []string{"transactions"},
			Output: "transactionIds",
		},
		"ptx_updateTransaction": {
			Inputs: []string{"transactionId", "transaction"},
			Output: "transactionId",
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
		"ptx_getTransactionReceiptFull": {
			Inputs: []string{"transactionId"},
			Output: "receipt",
		},
		"ptx_getPreparedTransaction": {
			Inputs: []string{"transactionId"},
			Output: "preparedTransaction",
		},
		"ptx_getDomainReceipt": {
			Inputs: []string{"domain", "transactionId"},
			Output: "domainReceipt",
		},
		"ptx_getStateReceipt": {
			Inputs: []string{"transactionId"},
			Output: "stateReceipt",
		},
		"ptx_queryTransactionReceipts": {
			Inputs: []string{"query"},
			Output: "receipts",
		},
		"ptx_queryPreparedTransactions": {
			Inputs: []string{"query"},
			Output: "preparedTransactions",
		},
		"ptx_storeABI": {
			Inputs: []string{"abi"},
			Output: "storedABI",
		},
		"ptx_getStoredABI": {
			Inputs: []string{"hashRef"},
			Output: "storedABI",
		},
		"ptx_queryStoredABIs": {
			Inputs: []string{"query"},
			Output: "storedABIs",
		},
		"ptx_decodeError": {
			Inputs: []string{"revertData", "dataFormat"},
			Output: "decodedError",
		},
		"ptx_decodeCall": {
			Inputs: []string{"callData", "dataFormat"},
			Output: "decodedCall",
		},
		"ptx_decodeEvent": {
			Inputs: []string{"topics", "data", "dataFormat"},
			Output: "decodedEvent",
		},
		"ptx_resolveVerifier": {
			Inputs: []string{"keyIdentifier", "algorithm", "verifierType"},
			Output: "verifier",
		},
		"ptx_createReceiptListener": {
			Inputs: []string{"listener"},
			Output: "success",
		},
		"ptx_queryReceiptListeners": {
			Inputs: []string{"query"},
			Output: "listeners",
		},
		"ptx_getReceiptListener": {
			Inputs: []string{"listenerName"},
			Output: "listener",
		},
		"ptx_startReceiptListener": {
			Inputs: []string{"listenerName"},
			Output: "success",
		},
		"ptx_stopReceiptListener": {
			Inputs: []string{"listenerName"},
			Output: "success",
		},
		"ptx_deleteReceiptListener": {
			Inputs: []string{"listenerName"},
			Output: "success",
		},
		"ptx_createBlockchainEventListener": {
			Inputs: []string{"listener"},
			Output: "success",
		},
		"ptx_queryBlockchainEventListeners": {
			Inputs: []string{"query"},
			Output: "listeners",
		},
		"ptx_getBlockchainEventListener": {
			Inputs: []string{"listenerName"},
			Output: "listener",
		},
		"ptx_startBlockchainEventListener": {
			Inputs: []string{"listenerName"},
			Output: "success",
		},
		"ptx_stopBlockchainEventListener": {
			Inputs: []string{"listenerName"},
			Output: "success",
		},
		"ptx_deleteBlockchainEventListener": {
			Inputs: []string{"listenerName"},
			Output: "success",
		},
		"ptx_getBlockchainEventListenerStatus": {
			Inputs: []string{"listenerName"},
			Output: "listenerStatus",
		},
	},
	subscriptions: []RPCSubscriptionInfo{
		{
			SubscriptionConfig: ptxSubscriptionConfig,
			FixedInputs:        []string{"receipts"},
			Inputs:             []string{"listenerName"},
		},
		{
			SubscriptionConfig: ptxSubscriptionConfig,
			FixedInputs:        []string{"blockchainevents"},
			Inputs:             []string{"listenerName"},
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

func (p *ptx) PrepareTransaction(ctx context.Context, tx *pldapi.TransactionInput) (txID *uuid.UUID, err error) {
	err = p.c.CallRPC(ctx, &txID, "ptx_prepareTransaction", tx)
	return
}

func (p *ptx) PrepareTransactions(ctx context.Context, txs []*pldapi.TransactionInput) (txIDs []uuid.UUID, err error) {
	err = p.c.CallRPC(ctx, &txIDs, "ptx_prepareTransactions", txs)
	return
}

func (p *ptx) UpdateTransaction(ctx context.Context, id uuid.UUID, tx *pldapi.TransactionInput) (txID *uuid.UUID, err error) {
	err = p.c.CallRPC(ctx, &txID, "ptx_updateTransaction", id, tx)
	return
}

func (p *ptx) Call(ctx context.Context, tx *pldapi.TransactionCall) (data pldtypes.RawJSON, err error) {
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

func (p *ptx) GetTransactionReceiptFull(ctx context.Context, txID uuid.UUID) (receipt *pldapi.TransactionReceiptFull, err error) {
	err = p.c.CallRPC(ctx, &receipt, "ptx_getTransactionReceiptFull", txID)
	return
}

func (p *ptx) GetPreparedTransaction(ctx context.Context, txID uuid.UUID) (preparedTransaction *pldapi.PreparedTransaction, err error) {
	err = p.c.CallRPC(ctx, &preparedTransaction, "ptx_getPreparedTransaction", txID)
	return
}

func (p *ptx) GetDomainReceipt(ctx context.Context, domain string, txID uuid.UUID) (domainReceipt pldtypes.RawJSON, err error) {
	err = p.c.CallRPC(ctx, &domainReceipt, "ptx_getDomainReceipt", domain, txID)
	return
}

func (p *ptx) GetStateReceipt(ctx context.Context, txID uuid.UUID) (stateReceipt *pldapi.TransactionStates, err error) {
	err = p.c.CallRPC(ctx, &stateReceipt, "ptx_getStateReceipt", txID)
	return
}

func (p *ptx) QueryTransactionReceipts(ctx context.Context, jq *query.QueryJSON) (receipts []*pldapi.TransactionReceipt, err error) {
	err = p.c.CallRPC(ctx, &receipts, "ptx_queryTransactionReceipts", jq)
	return
}

func (p *ptx) QueryPreparedTransactions(ctx context.Context, jq *query.QueryJSON) (preparedTransactions []*pldapi.PreparedTransaction, err error) {
	err = p.c.CallRPC(ctx, &preparedTransactions, "ptx_queryPreparedTransactions", jq)
	return
}

func (p *ptx) StoreABI(ctx context.Context, abi abi.ABI) (storedABI *pldapi.StoredABI, err error) {
	err = p.c.CallRPC(ctx, &storedABI, "ptx_storeABI", abi)
	return
}

func (p *ptx) GetStoredABI(ctx context.Context, hashRef pldtypes.Bytes32) (storedABI *pldapi.StoredABI, err error) {
	err = p.c.CallRPC(ctx, &storedABI, "ptx_getStoredABI", hashRef)
	return
}

func (p *ptx) QueryStoredABIs(ctx context.Context, jq *query.QueryJSON) (storedABIs []*pldapi.StoredABI, err error) {
	err = p.c.CallRPC(ctx, &storedABIs, "ptx_queryStoredABIs", jq)
	return
}

func (p *ptx) DecodeError(ctx context.Context, revertData pldtypes.HexBytes, dataFormat pldtypes.JSONFormatOptions) (decodedError *pldapi.ABIDecodedData, err error) {
	err = p.c.CallRPC(ctx, &decodedError, "ptx_decodeError", revertData, dataFormat)
	return
}

func (p *ptx) DecodeCall(ctx context.Context, callData pldtypes.HexBytes, dataFormat pldtypes.JSONFormatOptions) (decodedCall *pldapi.ABIDecodedData, err error) {
	err = p.c.CallRPC(ctx, &decodedCall, "ptx_decodeCall", callData, dataFormat)
	return
}

func (p *ptx) DecodeEvent(ctx context.Context, topics []pldtypes.Bytes32, data pldtypes.HexBytes, dataFormat pldtypes.JSONFormatOptions) (decodedEvent *pldapi.ABIDecodedData, err error) {
	err = p.c.CallRPC(ctx, &decodedEvent, "ptx_decodeEvent", topics, data, dataFormat)
	return
}

func (p *ptx) ResolveVerifier(ctx context.Context, keyIdentifier string, algorithm string, verifierType string) (verifier string, err error) {
	err = p.c.CallRPC(ctx, &verifier, "ptx_resolveVerifier", keyIdentifier, algorithm, verifierType)
	return
}

func (p *ptx) CreateReceiptListener(ctx context.Context, listener *pldapi.TransactionReceiptListener) (success bool, err error) {
	err = p.c.CallRPC(ctx, &success, "ptx_createReceiptListener", listener)
	return
}

func (p *ptx) QueryReceiptListeners(ctx context.Context, jq *query.QueryJSON) (listeners []*pldapi.TransactionReceiptListener, err error) {
	err = p.c.CallRPC(ctx, &listeners, "ptx_queryReceiptListeners", jq)
	return
}

func (p *ptx) GetReceiptListener(ctx context.Context, listenerName string) (listener *pldapi.TransactionReceiptListener, err error) {
	err = p.c.CallRPC(ctx, &listener, "ptx_getReceiptListener", listenerName)
	return
}

func (p *ptx) StartReceiptListener(ctx context.Context, listenerName string) (success bool, err error) {
	err = p.c.CallRPC(ctx, &success, "ptx_startReceiptListener", listenerName)
	return
}

func (p *ptx) StopReceiptListener(ctx context.Context, listenerName string) (success bool, err error) {
	err = p.c.CallRPC(ctx, &success, "ptx_stopReceiptListener", listenerName)
	return
}

func (p *ptx) DeleteReceiptListener(ctx context.Context, listenerName string) (success bool, err error) {
	err = p.c.CallRPC(ctx, &success, "ptx_deleteReceiptListener", listenerName)
	return
}

func (p *ptx) CreateBlockchainEventListener(ctx context.Context, listener *pldapi.BlockchainEventListener) (success bool, err error) {
	err = p.c.CallRPC(ctx, &success, "ptx_createBlockchainEventListener", listener)
	return
}

func (p *ptx) QueryBlockchainEventListeners(ctx context.Context, jq *query.QueryJSON) (listeners []*pldapi.BlockchainEventListener, err error) {
	err = p.c.CallRPC(ctx, &listeners, "ptx_queryBlockchainEventListeners", jq)
	return
}

func (p *ptx) GetBlockchainEventListener(ctx context.Context, listenerName string) (listener *pldapi.BlockchainEventListener, err error) {
	err = p.c.CallRPC(ctx, &listener, "ptx_getBlockchainEventListener", listenerName)
	return
}

func (p *ptx) StartBlockchainEventListener(ctx context.Context, listenerName string) (success bool, err error) {
	err = p.c.CallRPC(ctx, &success, "ptx_startBlockchainEventListener", listenerName)
	return
}

func (p *ptx) StopBlockchainEventListener(ctx context.Context, listenerName string) (success bool, err error) {
	err = p.c.CallRPC(ctx, &success, "ptx_stopBlockchainEventListener", listenerName)
	return
}

func (p *ptx) DeleteBlockchainEventListener(ctx context.Context, listenerName string) (success bool, err error) {
	err = p.c.CallRPC(ctx, &success, "ptx_deleteBlockchainEventListener", listenerName)
	return
}

func (p *ptx) GetBlockchainEventListenerStatus(ctx context.Context, listenerName string) (listener *pldapi.BlockchainEventListenerStatus, err error) {
	err = p.c.CallRPC(ctx, &listener, "ptx_getBlockchainEventListenerStatus", listenerName)
	return
}

func (p *ptx) SubscribeReceipts(ctx context.Context, listenerName string) (sub rpcclient.Subscription, err error) {
	ws, err := p.c.WSClient(ctx)
	if err != nil {
		return nil, err
	}
	return ws.Subscribe(ctx, ptxSubscriptionConfig, "receipts", listenerName)
}

func (p *ptx) SubscribeBlockchainEvents(ctx context.Context, listenerName string) (sub rpcclient.Subscription, err error) {
	ws, err := p.c.WSClient(ctx)
	if err != nil {
		return nil, err
	}
	return ws.Subscribe(ctx, ptxSubscriptionConfig, "blockchainevents", listenerName)
}
