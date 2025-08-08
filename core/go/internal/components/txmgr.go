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

package components

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

type ReceiptType int

const (
	// Success should come with a transaction hash - nothing more
	RT_Success ReceiptType = iota
	// Asks the Transaction Manager to use the error decoding dictionary to decode an revert data and build the message
	RT_FailedOnChainWithRevertData
	// The provided pre-translated message states that any failure, and should be written directly
	RT_FailedWithMessage
)

type ReceiptInputWithOriginator struct {
	Originator string
	ReceiptInput
}

type ReceiptInput struct {
	Domain          string                   // set when the receipt is from a domain
	ReceiptType     ReceiptType              // required
	TransactionID   uuid.UUID                // required
	OnChain         pldtypes.OnChainLocation // OnChain.Type must be set for an on-chain transaction/event
	ContractAddress *pldtypes.EthAddress     // the contract address - deployments only
	FailureMessage  string                   // set for RT_FailedWithMessage
	RevertData      pldtypes.HexBytes        // set for RT_FailedOnChainWithRevertData
}

type TxCompletion struct {
	ReceiptInput
	PSC DomainSmartContract
}

type ResolvedTransaction struct {
	Transaction *pldapi.Transaction `json:"transaction"`
	DependsOn   []uuid.UUID         `json:"dependsOn"`
	Function    *ResolvedFunction   `json:"function"`
}

// This is a transaction read for insertion into the Paladin database with all pre-verification completed.
type ValidatedTransaction struct {
	ResolvedTransaction
	LocalFrom    string
	PublicTxData []byte
}

type ChainedPrivateTransaction struct {
	OriginalSenderLocator string    // the original sender fully qualified identity
	OriginalTransaction   uuid.UUID // the original transaction that chained this transaction
	OriginalDomain        string    // the original domain of the upstream transaction
	NewTransaction        *ValidatedTransaction
}

// A resolved function on the ABI
type ResolvedFunction struct {
	// ABI          abi.ABI          `json:"abi"`
	ABIReference *pldtypes.Bytes32 `json:"abiReference"`
	Definition   *abi.Entry        `json:"definition"`
	Signature    string            `json:"signature"`
}

type ReceiptReceiver interface {
	DeliverReceiptBatch(ctx context.Context, batchID uint64, receipts []*pldapi.TransactionReceiptFull) error
}

type BlockchainEventReceiver interface {
	DeliverBlockchainEventBatch(ctx context.Context, batchID uuid.UUID, events []*pldapi.EventWithData) error
}

type ReceiverCloser interface {
	Close()
}

type TXManager interface {
	ManagerLifecycle

	// These are the general purpose functions exposed also as JSON/RPC APIs on the TX Manager

	FinalizeTransactions(ctx context.Context, dbTX persistence.DBTX, info []*ReceiptInput) error // requires all transactions to be known
	CalculateRevertError(ctx context.Context, dbTX persistence.DBTX, revertData pldtypes.HexBytes) error
	DecodeRevertError(ctx context.Context, dbTX persistence.DBTX, revertData pldtypes.HexBytes, dataFormat pldtypes.JSONFormatOptions) (*pldapi.ABIDecodedData, error)
	DecodeCall(ctx context.Context, dbTX persistence.DBTX, callData pldtypes.HexBytes, dataFormat pldtypes.JSONFormatOptions) (*pldapi.ABIDecodedData, error)
	DecodeEvent(ctx context.Context, dbTX persistence.DBTX, topics []pldtypes.Bytes32, eventData pldtypes.HexBytes, dataFormat pldtypes.JSONFormatOptions) (*pldapi.ABIDecodedData, error)
	SendTransactions(ctx context.Context, dbTX persistence.DBTX, txs ...*pldapi.TransactionInput) (txIDs []uuid.UUID, err error)
	ResolveTransactionInputs(ctx context.Context, dbTX persistence.DBTX, tx *pldapi.TransactionInput) (*ResolvedFunction, *abi.ComponentValue, pldtypes.RawJSON, error)
	PrepareTransactions(ctx context.Context, dbTX persistence.DBTX, txs ...*pldapi.TransactionInput) (txIDs []uuid.UUID, err error)
	GetTransactionByID(ctx context.Context, id uuid.UUID) (*pldapi.Transaction, error)
	GetResolvedTransactionByID(ctx context.Context, id uuid.UUID) (*ResolvedTransaction, error) // cache optimized
	GetTransactionByIDFull(ctx context.Context, id uuid.UUID) (result *pldapi.TransactionFull, err error)
	GetTransactionDependencies(ctx context.Context, id uuid.UUID) (*pldapi.TransactionDependencies, error)
	GetPublicTransactionByNonce(ctx context.Context, from pldtypes.EthAddress, nonce pldtypes.HexUint64) (*pldapi.PublicTxWithBinding, error)
	GetPublicTransactionByHash(ctx context.Context, hash pldtypes.Bytes32) (*pldapi.PublicTxWithBinding, error)
	QueryTransactions(ctx context.Context, jq *query.QueryJSON, dbTX persistence.DBTX, pending bool) ([]*pldapi.Transaction, error)
	QueryTransactionsResolved(ctx context.Context, jq *query.QueryJSON, dbTX persistence.DBTX, pending bool) ([]*ResolvedTransaction, error)
	QueryTransactionsFull(ctx context.Context, jq *query.QueryJSON, dbTX persistence.DBTX, pending bool) (results []*pldapi.TransactionFull, err error)
	QueryTransactionsFullTx(ctx context.Context, jq *query.QueryJSON, dbTX persistence.DBTX, pending bool) ([]*pldapi.TransactionFull, error)
	QueryTransactionReceipts(ctx context.Context, jq *query.QueryJSON) ([]*pldapi.TransactionReceipt, error)
	GetTransactionReceiptByID(ctx context.Context, id uuid.UUID) (*pldapi.TransactionReceipt, error)
	GetPreparedTransactionByID(ctx context.Context, dbTX persistence.DBTX, id uuid.UUID) (*pldapi.PreparedTransaction, error)
	GetPreparedTransactionWithRefsByID(ctx context.Context, dbTX persistence.DBTX, id uuid.UUID) (*PreparedTransactionWithRefs, error)
	QueryPreparedTransactions(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.PreparedTransaction, error)
	QueryPreparedTransactionsWithRefs(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*PreparedTransactionWithRefs, error)
	CallTransaction(ctx context.Context, dbTX persistence.DBTX, result any, tx *pldapi.TransactionCall) (err error)
	UpsertABI(ctx context.Context, dbTX persistence.DBTX, a abi.ABI) (*pldapi.StoredABI, error)
	CreateReceiptListener(ctx context.Context, spec *pldapi.TransactionReceiptListener) error
	GetReceiptListener(ctx context.Context, name string) *pldapi.TransactionReceiptListener
	QueryReceiptListeners(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.TransactionReceiptListener, error)
	StartReceiptListener(ctx context.Context, name string) error
	StopReceiptListener(ctx context.Context, name string) error
	DeleteReceiptListener(ctx context.Context, name string) error
	AddReceiptReceiver(ctx context.Context, name string, r ReceiptReceiver) (ReceiverCloser, error)

	// These functions for use of other components

	LoadBlockchainEventListeners() error
	NotifyStatesDBChanged(ctx context.Context) // called by state manager after committing DB TXs writing new states that might fill in gaps
	PrepareChainedPrivateTransaction(ctx context.Context, dbTX persistence.DBTX, origSender string, origTxID uuid.UUID, origDomain string, txToChain *pldapi.TransactionInput, submitMode pldapi.SubmitMode) (*ChainedPrivateTransaction, error)
	ChainPrivateTransactions(ctx context.Context, dbTX persistence.DBTX, txis []*ChainedPrivateTransaction) error
	WritePreparedTransactions(ctx context.Context, dbTX persistence.DBTX, prepared []*PreparedTransactionWithRefs) error
}
