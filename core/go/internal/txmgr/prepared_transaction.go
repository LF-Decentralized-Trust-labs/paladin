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
	"encoding/json"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/filters"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

// DB persisted record for a prepared transaction
type preparedTransaction struct {
	ID          uuid.UUID            `gorm:"column:id"`
	Domain      string               `gorm:"column:domain"`
	To          *pldtypes.EthAddress `gorm:"column:to"`
	Created     pldtypes.Timestamp   `gorm:"column:created"`
	Transaction pldtypes.RawJSON     `gorm:"column:transaction"`
	Metadata    pldtypes.RawJSON     `gorm:"column:metadata"`
}

func (preparedTransaction) TableName() string {
	return "prepared_txns"
}

type preparedStateType string

const (
	preparedSpend   preparedStateType = "spend"
	preparedRead    preparedStateType = "read"
	preparedConfirm preparedStateType = "confirm"
	preparedInfo    preparedStateType = "info"
)

type preparedTransactionState struct {
	Transaction uuid.UUID         `gorm:"column:transaction"`
	DomainName  string            `gorm:"column:domain_name"`
	StateID     pldtypes.HexBytes `gorm:"column:state"`
	StateIdx    int               `gorm:"column:state_idx"`
	Type        preparedStateType `gorm:"column:type"`
	State       *pldapi.StateBase `gorm:"foreignKey:state;references:id;"`
}

func (preparedTransactionState) TableName() string {
	return "prepared_txn_states"
}

var preparedTransactionFilters = filters.FieldMap{
	"id":      filters.UUIDField(`"id"`),
	"created": filters.TimestampField("created"),
}

func (tm *txManager) WritePreparedTransactions(ctx context.Context, dbTX persistence.DBTX, prepared []*components.PreparedTransactionWithRefs) error {

	var preparedTxInserts []*preparedTransaction
	var preparedTxStateInserts []*preparedTransactionState
	for _, p := range prepared {
		dbPreparedTx := &preparedTransaction{
			ID:       p.ID,
			Domain:   p.Domain,
			To:       p.To,
			Metadata: p.Metadata,
		}
		// We do the work for the ABI validation etc. before we insert the TX
		resolved, err := tm.resolveNewTransaction(ctx, dbTX, &p.Transaction, pldapi.SubmitModePrepare)
		if err == nil {
			p.Transaction.ABI = nil // move to the reference
			p.Transaction.ABIReference = resolved.Function.ABIReference
			p.Transaction.Function = resolved.Function.Definition.String()
			dbPreparedTx.Transaction, err = json.Marshal(p.Transaction)
		}
		if err != nil {
			return err
		}
		preparedTxInserts = append(preparedTxInserts, dbPreparedTx)
		for i, stateID := range p.StateRefs.Spent {
			preparedTxStateInserts = append(preparedTxStateInserts, &preparedTransactionState{
				Transaction: p.ID,
				Type:        preparedSpend,
				DomainName:  p.Domain,
				StateID:     stateID,
				StateIdx:    i,
			})
		}
		for i, stateID := range p.StateRefs.Read {
			preparedTxStateInserts = append(preparedTxStateInserts, &preparedTransactionState{
				Transaction: p.ID,
				Type:        preparedRead,
				DomainName:  p.Domain,
				StateID:     stateID,
				StateIdx:    i,
			})
		}
		for i, stateID := range p.StateRefs.Confirmed {
			preparedTxStateInserts = append(preparedTxStateInserts, &preparedTransactionState{
				Transaction: p.ID,
				Type:        preparedConfirm,
				DomainName:  p.Domain,
				StateID:     stateID,
				StateIdx:    i,
			})
		}
		for i, stateID := range p.StateRefs.Info {
			preparedTxStateInserts = append(preparedTxStateInserts, &preparedTransactionState{
				Transaction: p.ID,
				Type:        preparedInfo,
				DomainName:  p.Domain,
				StateID:     stateID,
				StateIdx:    i,
			})
		}
		log.L(ctx).Infof("Inserting prepared %s transaction for transaction %s with spent=%d read=%d confirmed=%d info=%d",
			p.Transaction.Type, p.ID, len(p.StateRefs.Spent), len(p.StateRefs.Read), len(p.StateRefs.Confirmed), len(p.StateRefs.Info))
	}

	var err error
	if len(preparedTxInserts) > 0 {
		err = dbTX.DB().WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true /* immutable */}).
			Create(preparedTxInserts).
			Error
	}

	if err == nil && len(preparedTxStateInserts) > 0 {
		err = dbTX.DB().WithContext(ctx).
			Omit("State").
			Clauses(clause.OnConflict{DoNothing: true /* immutable */}).
			Create(preparedTxStateInserts).
			Error
	}

	return err

}

func (tm *txManager) QueryPreparedTransactions(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.PreparedTransaction, error) {
	bpts, err := tm.queryPreparedTransactionsBase(ctx, dbTX, jq)
	if err != nil {
		return nil, err
	}
	return tm.enrichPreparedTransactionsFull(ctx, dbTX, bpts)
}

func (tm *txManager) QueryPreparedTransactionsWithRefs(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*components.PreparedTransactionWithRefs, error) {
	bpts, err := tm.queryPreparedTransactionsBase(ctx, dbTX, jq)
	if err != nil {
		return nil, err
	}
	return tm.enrichPreparedTransactionsRefs(ctx, dbTX, bpts)
}

func (tm *txManager) queryPreparedTransactionsBase(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.PreparedTransactionBase, error) {
	qw := &filters.QueryWrapper[preparedTransaction, pldapi.PreparedTransactionBase]{
		P:           tm.p,
		Table:       "prepared_txns",
		DefaultSort: "-created",
		Filters:     preparedTransactionFilters,
		Query:       jq,
		MapResult: func(pt *preparedTransaction) (*pldapi.PreparedTransactionBase, error) {
			preparedTx := &pldapi.PreparedTransactionBase{
				ID:       pt.ID,
				Domain:   pt.Domain,
				To:       pt.To,
				Metadata: pt.Metadata,
			}
			return preparedTx, json.Unmarshal(pt.Transaction, &preparedTx.Transaction)
		},
	}
	return qw.Run(ctx, dbTX)
}

func (tm *txManager) enrichPreparedTransactionsFull(ctx context.Context, dbTX persistence.DBTX, basePTs []*pldapi.PreparedTransactionBase) ([]*pldapi.PreparedTransaction, error) {
	preparedTransactions := make([]*pldapi.PreparedTransaction, len(basePTs))
	for i, bpt := range basePTs {
		preparedTransactions[i] = &pldapi.PreparedTransaction{
			PreparedTransactionBase: bpt,
		}
	}
	if len(preparedTransactions) > 0 {
		transactionIDs := make([]uuid.UUID, len(preparedTransactions))
		for i, pt := range preparedTransactions {
			transactionIDs[i] = pt.ID
		}
		var preparedStates []*preparedTransactionState
		err := dbTX.DB().WithContext(ctx).
			Where(`"transaction" IN (?)`, transactionIDs).
			Order(`"transaction"`).
			Order(`"type"`).
			Order(`"state_idx"`).
			Joins("State").
			Find(&preparedStates).
			Error
		if err != nil {
			return nil, err
		}
		for _, ps := range preparedStates {
			for _, pt := range preparedTransactions {
				if ps.Transaction == pt.ID {
					switch ps.Type {
					case preparedSpend:
						pt.States.Spent = append(pt.States.Spent, ps.State)
					case preparedRead:
						pt.States.Read = append(pt.States.Read, ps.State)
					case preparedConfirm:
						pt.States.Confirmed = append(pt.States.Confirmed, ps.State)
					case preparedInfo:
						pt.States.Info = append(pt.States.Info, ps.State)
					}
				}
			}
		}

	}
	return preparedTransactions, nil
}

func (tm *txManager) enrichPreparedTransactionsRefs(ctx context.Context, dbTX persistence.DBTX, basePTs []*pldapi.PreparedTransactionBase) ([]*components.PreparedTransactionWithRefs, error) {
	preparedTransactions := make([]*components.PreparedTransactionWithRefs, len(basePTs))
	for i, bpt := range basePTs {
		preparedTransactions[i] = &components.PreparedTransactionWithRefs{
			PreparedTransactionBase: bpt,
		}
	}
	if len(preparedTransactions) > 0 {
		transactionIDs := make([]uuid.UUID, len(preparedTransactions))
		for i, pt := range preparedTransactions {
			transactionIDs[i] = pt.ID
		}
		var preparedStates []*preparedTransactionState
		err := dbTX.DB().WithContext(ctx).
			Where(`"transaction" IN (?)`, transactionIDs).
			Order(`"transaction"`).
			Order(`"type"`).
			Order(`"state_idx"`).
			Find(&preparedStates).
			Error
		if err != nil {
			return nil, err
		}
		for _, ps := range preparedStates {
			for _, pt := range preparedTransactions {
				if ps.Transaction == pt.ID {
					switch ps.Type {
					case preparedSpend:
						pt.StateRefs.Spent = append(pt.StateRefs.Spent, ps.StateID)
					case preparedRead:
						pt.StateRefs.Read = append(pt.StateRefs.Read, ps.StateID)
					case preparedConfirm:
						pt.StateRefs.Confirmed = append(pt.StateRefs.Confirmed, ps.StateID)
					case preparedInfo:
						pt.StateRefs.Info = append(pt.StateRefs.Info, ps.StateID)
					}
				}
			}
		}

	}
	return preparedTransactions, nil
}

func (tm *txManager) GetPreparedTransactionByID(ctx context.Context, dbTX persistence.DBTX, id uuid.UUID) (*pldapi.PreparedTransaction, error) {
	pts, err := tm.QueryPreparedTransactions(ctx, dbTX, query.NewQueryBuilder().Limit(1).Equal("id", id).Query())
	if len(pts) == 0 || err != nil {
		return nil, err
	}
	return pts[0], nil
}

func (tm *txManager) GetPreparedTransactionWithRefsByID(ctx context.Context, dbTX persistence.DBTX, id uuid.UUID) (*components.PreparedTransactionWithRefs, error) {
	pts, err := tm.QueryPreparedTransactionsWithRefs(ctx, dbTX, query.NewQueryBuilder().Limit(1).Equal("id", id).Query())
	if len(pts) == 0 || err != nil {
		return nil, err
	}
	return pts[0], nil
}
