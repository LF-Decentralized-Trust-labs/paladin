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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
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

type ReceiptInput struct {
	ReceiptType     ReceiptType             // required
	TransactionID   uuid.UUID               // required
	OnChain         tktypes.OnChainLocation // OnChain.Type must be set for an on-chain transaction/event
	ContractAddress *tktypes.EthAddress     // the contract address - deployments only
	FailureMessage  string                  // set for RT_FailedWithMessage
	RevertData      tktypes.HexBytes        // set for RT_FailedOnChainWithRevertData
}

type TXManager interface {
	ManagerLifecycle
	MatchAndFinalizeTransactions(ctx context.Context, dbTX *gorm.DB, info []*ReceiptInput) ([]uuid.UUID, error) // returns which transactions were known
	FinalizeTransactions(ctx context.Context, dbTX *gorm.DB, info []*ReceiptInput) error                        // requires all transactions to be known
	CalculateRevertError(ctx context.Context, dbTX *gorm.DB, revertData tktypes.HexBytes) error
	SendTransaction(ctx context.Context, tx *pldapi.TransactionInput) (*uuid.UUID, error)
	SendTransactions(ctx context.Context, txs []*pldapi.TransactionInput) (txIDs []uuid.UUID, err error)
	GetTransactionByID(ctx context.Context, id uuid.UUID) (*pldapi.Transaction, error)
	GetTransactionByIDFull(ctx context.Context, id uuid.UUID) (result *pldapi.TransactionFull, err error)
	GetTransactionDependencies(ctx context.Context, id uuid.UUID) (*pldapi.TransactionDependencies, error)
	GetPublicTransactionByNonce(ctx context.Context, from tktypes.EthAddress, nonce tktypes.HexUint64) (*pldapi.PublicTxWithBinding, error)
	GetPublicTransactionByHash(ctx context.Context, hash tktypes.Bytes32) (*pldapi.PublicTxWithBinding, error)
	QueryTransactions(ctx context.Context, jq *query.QueryJSON, pending bool) ([]*pldapi.Transaction, error)
	QueryTransactionsFull(ctx context.Context, jq *query.QueryJSON, pending bool) (results []*pldapi.TransactionFull, err error)
	QueryTransactionsFullTx(ctx context.Context, jq *query.QueryJSON, dbTX *gorm.DB, pending bool) ([]*pldapi.TransactionFull, error)
	QueryTransactionReceipts(ctx context.Context, jq *query.QueryJSON) ([]*pldapi.TransactionReceipt, error)
	GetTransactionReceiptByID(ctx context.Context, id uuid.UUID) (*pldapi.TransactionReceipt, error)
	CallTransaction(ctx context.Context, result any, tx *pldapi.TransactionInput) (err error)
}
