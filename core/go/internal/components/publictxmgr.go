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

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/filters"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/blockindexer"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/google/uuid"
)

var PublicTxFilterFields filters.FieldSet = filters.FieldMap{
	"localId":         filters.Int64Field(`"public_txns"."pub_txn_id"`),
	"from":            filters.HexBytesField(`"from"`),
	"nonce":           filters.Int64Field("nonce"),
	"created":         filters.Int64Field("created"),
	"completedAt":     filters.Int64Field(`"Completed"."created"`),
	"transactionHash": filters.Int64Field(`"Completed"."tx_hash"`),
	"success":         filters.BooleanField(`"Completed"."success"`),
	"revertData":      filters.HexBytesField(`"Completed"."revert_data"`),
}

type PublicTxSubmission struct {
	Bindings             []*PaladinTXReference
	pldapi.PublicTxInput // the request to create the transaction
}

type PaladinTXReference struct {
	TransactionID              uuid.UUID
	TransactionType            pldtypes.Enum[pldapi.TransactionType]
	TransactionSender          string
	TransactionContractAddress string
}

type PublicTxMatch struct {
	PaladinTXReference
	*blockindexer.IndexedTransactionNotify
}

type PublicTxManager interface {
	ManagerLifecycle

	// Synchronous functions that are executed on the callers thread
	QueryPublicTxForTransactions(ctx context.Context, dbTX persistence.DBTX, boundToTxns []uuid.UUID, jq *query.QueryJSON) (map[uuid.UUID][]*pldapi.PublicTx, error)
	QueryPublicTxWithBindings(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.PublicTxWithBinding, error)
	GetPublicTransactionForHash(ctx context.Context, dbTX persistence.DBTX, hash pldtypes.Bytes32) (*pldapi.PublicTxWithBinding, error)

	// Perform (potentially expensive) transaction level validation, such as gas estimation. Call before starting a DB transaction
	ValidateTransaction(ctx context.Context, dbTX persistence.DBTX, transaction *PublicTxSubmission) error
	// Write a set of validated transactions to the public TX mgr database, notifying the relevant orchestrator(s) to wake, assign nonces, and start the submission process
	WriteNewTransactions(ctx context.Context, dbTX persistence.DBTX, transactions []*PublicTxSubmission) ([]*pldapi.PublicTx, error)
	// Convenience function that does ValidateTransaction+WriteNewTransactions for a single Tx
	SingleTransactionSubmit(ctx context.Context, transaction *PublicTxSubmission) (*pldapi.PublicTx, error)

	MatchUpdateConfirmedTransactions(ctx context.Context, dbTX persistence.DBTX, itxs []*blockindexer.IndexedTransactionNotify) ([]*PublicTxMatch, error)
	NotifyConfirmPersisted(ctx context.Context, confirms []*PublicTxMatch)

	UpdateTransaction(ctx context.Context, id uuid.UUID, pubTXID uint64, from *pldtypes.EthAddress, tx *pldapi.TransactionInput, publicTxData []byte, txmgrDBUpdate func(dbTX persistence.DBTX) error) error
}
