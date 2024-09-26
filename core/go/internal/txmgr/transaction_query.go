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

package txmgr

import (
	"context"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/toolkit/pkg/ptxapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
)

var transactionFilters = filters.FieldMap{
	"id":           filters.UUIDField("id"),
	"created":      filters.TimestampField("created"),
	"abiReference": filters.TimestampField("abi_ref"),
	"functionName": filters.StringField("fn_name"),
	"domain":       filters.StringField("domain"),
	"from":         filters.StringField("from"),
	"to":           filters.HexBytesField("to"),
}

func mapPersistedTXBase(pt *persistedTransaction) *ptxapi.Transaction {
	res := &ptxapi.Transaction{
		ID:             pt.ID,
		Created:        pt.Created,
		IdempotencyKey: stringOrEmpty(pt.IdempotencyKey),
		Type:           pt.Type,
		Domain:         stringOrEmpty(pt.Domain),
		Function:       stringOrEmpty(pt.Function),
		ABIReference:   pt.ABIReference,
		From:           pt.From,
		To:             pt.To,
		Data:           pt.Data,
	}
	return res
}

func (tm *txManager) mapPersistedTXFull(pt *persistedTransaction) *ptxapi.TransactionFull {
	res := &ptxapi.TransactionFull{
		Transaction: mapPersistedTXBase(pt),
	}
	receipt := pt.TransactionReceipt
	if receipt != nil {
		res.Receipt = mapPersistedReceipt(receipt)
	}
	for _, dep := range pt.TransactionDeps {
		res.DependsOn = append(res.DependsOn, dep.DependsOn)
	}
	return res
}

func (tm *txManager) queryTransactions(ctx context.Context, jq *query.QueryJSON, pending bool) ([]*ptxapi.Transaction, error) {
	qw := &queryWrapper[persistedTransaction, ptxapi.Transaction]{
		p:           tm.p,
		table:       "transactions",
		defaultSort: "-created",
		filters:     transactionFilters,
		query:       jq,
		finalize: func(q *gorm.DB) *gorm.DB {
			if pending {
				q = q.Joins("TransactionReceipt").
					Where(`"TransactionReceipt"."transaction" IS NULL`)
			}
			return q
		},
		mapResult: func(pt *persistedTransaction) (*ptxapi.Transaction, error) {
			return mapPersistedTXBase(pt), nil
		},
	}
	return qw.run(ctx, nil)
}

func (tm *txManager) queryTransactionsFull(ctx context.Context, jq *query.QueryJSON, pending bool) (results []*ptxapi.TransactionFull, err error) {
	err = tm.p.DB().Transaction(func(dbTX *gorm.DB) error {
		results, err = tm.queryTransactionsFullTx(ctx, jq, dbTX, pending)
		return err
	})
	return
}

func (tm *txManager) queryTransactionsFullTx(ctx context.Context, jq *query.QueryJSON, dbTX *gorm.DB, pending bool) ([]*ptxapi.TransactionFull, error) {
	qw := &queryWrapper[persistedTransaction, ptxapi.TransactionFull]{
		p:           tm.p,
		table:       "transactions",
		defaultSort: "-created",
		filters:     transactionFilters,
		query:       jq,
		finalize: func(q *gorm.DB) *gorm.DB {
			q = q.
				Preload("TransactionDeps").
				Joins("TransactionReceipt")
			if pending {
				q = q.Where(`"TransactionReceipt"."transaction" IS NULL`)
			}
			return q
		},
		mapResult: func(pt *persistedTransaction) (*ptxapi.TransactionFull, error) {
			return tm.mapPersistedTXFull(pt), nil
		},
	}
	ptxs, err := qw.run(ctx, dbTX)
	if err != nil {
		return nil, err
	}
	return tm.mergePublicTransactions(ctx, dbTX, ptxs)
}

func (tm *txManager) mergePublicTransactions(ctx context.Context, dbTX *gorm.DB, txs []*ptxapi.TransactionFull) ([]*ptxapi.TransactionFull, error) {
	txIDs := make([]uuid.UUID, len(txs))
	for i, tx := range txs {
		txIDs[i] = tx.ID
	}
	pubTxByTX, err := tm.publicTxMgr.QueryPublicTxForTransactions(ctx, dbTX, txIDs, nil)
	if err != nil {
		return nil, err
	}
	for _, tx := range txs {
		tx.Public = pubTxByTX[tx.ID]
	}
	return txs, nil

}

func (tm *txManager) getTransactionByIDFull(ctx context.Context, id uuid.UUID) (result *ptxapi.TransactionFull, err error) {
	ptxs, err := tm.queryTransactionsFull(ctx, query.NewQueryBuilder().Limit(1).Equal("id", id).Query(), false)
	if len(ptxs) == 0 || err != nil {
		return nil, err
	}
	return ptxs[0], nil
}

func (tm *txManager) getTransactionByID(ctx context.Context, id uuid.UUID) (*ptxapi.Transaction, error) {
	ptxs, err := tm.queryTransactions(ctx, query.NewQueryBuilder().Limit(1).Equal("id", id).Query(), false)
	if len(ptxs) == 0 || err != nil {
		return nil, err
	}
	return ptxs[0], nil
}

func (tm *txManager) getTransactionDependencies(ctx context.Context, id uuid.UUID) (*ptxapi.TransactionDependencies, error) {
	var persistedDeps []*transactionDep
	err := tm.p.DB().
		WithContext(ctx).
		Table(`transaction_deps`).
		Where(`"transaction" = ?`, id).
		Or("depends_on = ?", id).
		Find(&persistedDeps).
		Error
	if err != nil {
		return nil, err
	}
	res := &ptxapi.TransactionDependencies{
		DependsOn: make([]uuid.UUID, 0, len(persistedDeps)),
		PrereqOf:  make([]uuid.UUID, 0, len(persistedDeps)),
	}
	for _, td := range persistedDeps {
		if td.Transaction == id {
			res.DependsOn = append(res.DependsOn, td.DependsOn)
		} else {
			res.PrereqOf = append(res.PrereqOf, td.Transaction)
		}
	}
	return res, nil
}

func (tm *txManager) queryPublicTransactions(ctx context.Context, jq *query.QueryJSON) ([]*ptxapi.PublicTxWithBinding, error) {
	if err := checkLimitSet(ctx, jq); err != nil {
		return nil, err
	}
	return tm.publicTxMgr.QueryPublicTxWithBindings(ctx, tm.p.DB(), jq)
}

func (tm *txManager) getPublicTransactionByNonce(ctx context.Context, from tktypes.EthAddress, nonce tktypes.HexUint64) (*ptxapi.PublicTxWithBinding, error) {
	prs, err := tm.publicTxMgr.QueryPublicTxWithBindings(ctx, tm.p.DB(),
		query.NewQueryBuilder().Limit(1).
			Equal("from", from).
			Equal("nonce", nonce).
			Query())
	if len(prs) == 0 || err != nil {
		return nil, err
	}
	return prs[0], nil
}

func (tm *txManager) getPublicTransactionByHash(ctx context.Context, hash tktypes.Bytes32) (*ptxapi.PublicTxWithBinding, error) {
	return tm.publicTxMgr.GetPublicTransactionForHash(ctx, tm.p.DB(), hash)
}
