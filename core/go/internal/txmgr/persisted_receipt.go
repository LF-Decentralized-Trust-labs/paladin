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

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type transactionReceipt struct {
	TransactionID    uuid.UUID           `gorm:"column:transaction"`
	Indexed          tktypes.Timestamp   `gorm:"column:indexed"`
	Success          bool                `gorm:"column:success"`
	TransactionHash  *tktypes.Bytes32    `gorm:"column:tx_hash"`
	BlockNumber      *int64              `gorm:"column:block_number"`
	TransactionIndex *int64              `gorm:"column:tx_index"`
	LogIndex         *int64              `gorm:"column:log_index"`
	Source           *tktypes.EthAddress `gorm:"column:source"`
	FailureMessage   *string             `gorm:"column:failure_message"`
	RevertData       tktypes.HexBytes    `gorm:"column:revert_data"`
	ContractAddress  *tktypes.EthAddress `gorm:"column:contract_address"`
}

func mapPersistedReceipt(receipt *transactionReceipt) *pldapi.TransactionReceiptData {
	r := &pldapi.TransactionReceiptData{
		Success:         receipt.Success,
		FailureMessage:  stringOrEmpty(receipt.FailureMessage),
		RevertData:      receipt.RevertData,
		ContractAddress: receipt.ContractAddress,
	}
	if receipt.TransactionHash != nil {
		r.TransactionReceiptDataOnchain = &pldapi.TransactionReceiptDataOnchain{
			TransactionHash:  receipt.TransactionHash,
			BlockNumber:      int64OrZero(receipt.BlockNumber),
			TransactionIndex: int64OrZero(receipt.TransactionIndex),
		}
	}
	if receipt.Source != nil {
		r.TransactionReceiptDataOnchainEvent = &pldapi.TransactionReceiptDataOnchainEvent{
			LogIndex: int64OrZero(receipt.LogIndex),
			Source:   *receipt.Source,
		}
	}

	return r
}

var transactionReceiptFilters = filters.FieldMap{
	"id":              filters.UUIDField(`"transaction"`),
	"indexed":         filters.TimestampField("indexed"),
	"success":         filters.BooleanField("success"),
	"transactionHash": filters.StringField("tx_hash"),
	"blockNumber":     filters.Int64Field("block_number"),
}

func (tm *txManager) MatchAndFinalizeTransactions(ctx context.Context, dbTX *gorm.DB, info []*components.ReceiptInput) ([]uuid.UUID, error) {
	// It's possible for transactions to be deleted out of band, and we don't place a responsibility
	// on the caller to know that. So we take the hit of querying for the existence of these transactions
	// and only marking completion on those that exist.
	// The batching should make this acceptably efficient.
	allIDs := make([]uuid.UUID, len(info))
	for i, ri := range info {
		allIDs[i] = ri.TransactionID
	}
	var existingTXs []uuid.UUID
	err := dbTX.Table("transactions").
		Where("id IN (?)", allIDs).
		Pluck("id", &existingTXs).
		Error
	if err != nil {
		return nil, err
	}
	confirmedInfo := make([]*components.ReceiptInput, 0, len(info))
	for _, ri := range info {
		exists := false
		for _, existing := range existingTXs {
			if ri.TransactionID == existing {
				exists = true
				break
			}
		}
		if !exists {
			log.L(ctx).Warnf("Receipt notification for untracked transaction %s: %+v", ri.TransactionID, tktypes.JSONString(ri))
		} else {
			confirmedInfo = append(confirmedInfo, ri)
		}
	}
	return existingTXs, tm.FinalizeTransactions(ctx, dbTX, confirmedInfo)
}

// FinalizeTransactions is called by the block indexing routine, but also can be called
// by the private transaction manager if transactions fail without making it to the blockchain
func (tm *txManager) FinalizeTransactions(ctx context.Context, dbTX *gorm.DB, info []*components.ReceiptInput) error {

	if len(info) == 0 {
		return nil
	}

	receiptsToInsert := make([]*transactionReceipt, 0, len(info))
	for _, ri := range info {
		receipt := &transactionReceipt{
			TransactionID:   ri.TransactionID,
			Indexed:         tktypes.TimestampNow(),
			ContractAddress: ri.ContractAddress,
		}
		if ri.OnChain.Type != tktypes.NotOnChain {
			receipt.TransactionHash = &ri.OnChain.TransactionHash
			receipt.BlockNumber = &ri.OnChain.BlockNumber
			receipt.TransactionIndex = &ri.OnChain.TransactionIndex
			receipt.LogIndex = &ri.OnChain.LogIndex
			receipt.Source = ri.OnChain.Source
		}
		// Process each type, checking for coding errors in the calling component
		var failureMsg string
		switch ri.ReceiptType {
		case components.RT_Success:
			if ri.FailureMessage != "" || ri.RevertData != nil {
				return i18n.NewError(ctx, msgs.MsgTxMgrInvalidReceiptNotification, tktypes.JSONString(ri))
			}
			receipt.Success = true
		case components.RT_FailedWithMessage:
			if ri.FailureMessage == "" || ri.RevertData != nil {
				return i18n.NewError(ctx, msgs.MsgTxMgrInvalidReceiptNotification, tktypes.JSONString(ri))
			}
			receipt.Success = false
			failureMsg = ri.FailureMessage
			receipt.FailureMessage = &ri.FailureMessage
		case components.RT_FailedOnChainWithRevertData:
			if ri.FailureMessage != "" {
				return i18n.NewError(ctx, msgs.MsgTxMgrInvalidReceiptNotification, tktypes.JSONString(ri))
			}
			receipt.Success = false
			receipt.RevertData = ri.RevertData
			// We calculate the failure message - all errors handled mapped internally here
			failureMsg = tm.CalculateRevertError(ctx, dbTX, ri.RevertData).Error()
			receipt.FailureMessage = &failureMsg
		default:
			return i18n.NewError(ctx, msgs.MsgTxMgrInvalidReceiptNotification, tktypes.JSONString(ri))
		}
		log.L(ctx).Infof("Inserting receipt txId=%s success=%t failure=%s txHash=%v", receipt.TransactionID, receipt.Success, failureMsg, receipt.TransactionHash)
		receiptsToInsert = append(receiptsToInsert, receipt)
	}

	if len(receiptsToInsert) > 0 {
		err := dbTX.Table("transaction_receipts").
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "transaction"}},
				DoNothing: true, // once inserted, the receipt is immutable
			}).
			Create(receiptsToInsert).
			Error
		if err != nil {
			return err
		}
	}

	// TODO: Need to create an guaranteed increasing event table for these receipts, as applications
	//       must be able to efficiently and reliably listen for them as they are written (good or bad)

	return nil
}

func (tm *txManager) CalculateRevertError(ctx context.Context, dbTX *gorm.DB, revertData tktypes.HexBytes) error {

	if len(revertData) < 4 {
		return i18n.NewError(ctx, msgs.MsgTxMgrRevertedNoData)
	}
	selector := tktypes.HexBytes(revertData[0:4])

	// There is potential with a 4 byte selector for clashes, so we do a distinct on the full hash
	var errorDefs []*PersistedABIError
	err := dbTX.Table("abi_errors").
		Where("selector = ?", selector).
		Distinct("full_hash", "definition").
		Find(&errorDefs).
		Error
	if err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgTxMgrRevertedDataNotDecoded)
	}

	// Turn this into an ABI that we pass to the handy utility (which also includes
	// the default revert error) to decode for us.
	virtualABI := abi.ABI{}
	for _, def := range errorDefs {
		var e abi.Entry
		err := json.Unmarshal(def.Definition, &e)
		if err == nil {
			virtualABI = append(virtualABI, &e)
		}
	}
	decodedErrString, ok := virtualABI.ErrorStringCtx(ctx, revertData)
	if ok {
		return i18n.NewError(ctx, msgs.MsgTxMgrRevertedDecodedData, decodedErrString)
	}
	return i18n.NewError(ctx, msgs.MsgTxMgrRevertedDataNotDecoded)
}

func (tm *txManager) QueryTransactionReceipts(ctx context.Context, jq *query.QueryJSON) ([]*pldapi.TransactionReceipt, error) {
	qw := &queryWrapper[transactionReceipt, pldapi.TransactionReceipt]{
		p:           tm.p,
		table:       "transaction_receipts",
		defaultSort: "-indexed",
		filters:     transactionReceiptFilters,
		query:       jq,
		mapResult: func(pt *transactionReceipt) (*pldapi.TransactionReceipt, error) {
			return &pldapi.TransactionReceipt{
				ID:                     pt.TransactionID,
				TransactionReceiptData: *mapPersistedReceipt(pt),
			}, nil
		},
	}
	return qw.run(ctx, nil)
}

func (tm *txManager) GetTransactionReceiptByID(ctx context.Context, id uuid.UUID) (*pldapi.TransactionReceipt, error) {
	prs, err := tm.QueryTransactionReceipts(ctx, query.NewQueryBuilder().Limit(1).Equal("id", id).Query())
	if len(prs) == 0 || err != nil {
		return nil, err
	}
	return prs[0], nil
}
