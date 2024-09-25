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
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/toolkit/pkg/ptxapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
)

type PublicTxAccepted interface {
	Bindings() []*PaladinTXReference
	PublicTx() *ptxapi.PublicTx // the nonce can only be read after Submit() on the batch succeeds
}

type PublicTxRejected interface {
	Bindings() []*PaladinTXReference
	RejectedError() error         // non-nil if the transaction was rejected during prepare (estimate gas error), so cannot be submitted
	RevertData() tktypes.HexBytes // if revert data is available for error decoding
}

type PublicTxBatch interface {
	Submit(ctx context.Context, dbTX *gorm.DB) error
	Accepted() []PublicTxAccepted
	Rejected() []PublicTxRejected
	Completed(ctx context.Context, committed bool) // caller must ensure this is called on all code paths, and only with true after DB TX has committed
}

var PublicTxFilterFields filters.FieldSet = filters.FieldMap{
	"from":            filters.HexBytesField("from"),
	"nonce":           filters.Int64Field("nonce"),
	"created":         filters.Int64Field("created"),
	"completedAt":     filters.Int64Field(`"Completed"."created"`),
	"transactionHash": filters.Int64Field(`"Completed"."tx_hash"`),
	"success":         filters.BooleanField(`"Completed"."success"`),
	"revertData":      filters.HexBytesField(`"Completed"."revert_data"`),
}

type PublicTxSubmission struct {
	Bindings             []*PaladinTXReference
	ptxapi.PublicTxInput // the request to create the transaction
}

type PaladinTXReference struct {
	TransactionID   uuid.UUID
	TransactionType tktypes.Enum[ptxapi.TransactionType]
}

type PublicTxMatch struct {
	PaladinTXReference
	*blockindexer.IndexedTransactionNotify
}

// Database record used for efficiency in both public and Paladin transaction managers as part of a JOIN
// PublicTxMgr owns insertion of these records at creation time of the public Txn (inside the batch)
type PublicTxnBinding struct {
	Sequence        uint64                               `gorm:"column:sequence;autoIncrement"` // unique identifier for this record
	SignerNonce     string                               `gorm:"column:signer_nonce"`
	Transaction     uuid.UUID                            `gorm:"column:transaction"`
	TransactionType tktypes.Enum[ptxapi.TransactionType] `gorm:"column:tx_type"`
}

type PublicTxManager interface {
	ManagerLifecycle

	// Synchronous functions that are executed on the callers thread
	QueryTransactions(ctx context.Context, dbTX *gorm.DB, scopeToTxn *uuid.UUID, jq *query.QueryJSON) ([]*ptxapi.PublicTx, error)
	PrepareSubmissionBatch(ctx context.Context, transactions []*PublicTxSubmission) (batch PublicTxBatch, err error)
	MatchUpdateConfirmedTransactions(ctx context.Context, dbTX *gorm.DB, itxs []*blockindexer.IndexedTransactionNotify) ([]*PublicTxMatch, error)
	NotifyConfirmPersisted(ctx context.Context, confirms []*PublicTxMatch)
}
