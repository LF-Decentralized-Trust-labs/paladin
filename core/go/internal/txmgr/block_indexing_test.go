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
	"database/sql/driver"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/toolkit/pkg/ptxapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newTestConfirm(revertReason ...[]byte) *blockindexer.IndexedTransactionNotify {
	txi := &blockindexer.IndexedTransactionNotify{
		IndexedTransaction: blockindexer.IndexedTransaction{
			Hash:             tktypes.Bytes32(tktypes.RandBytes(32)),
			BlockNumber:      12345,
			TransactionIndex: 0,
			From:             tktypes.MustEthAddress(tktypes.RandHex(20)),
			Nonce:            1000,
			To:               nil,
			ContractAddress:  tktypes.MustEthAddress(tktypes.RandHex(20)),
			Result:           blockindexer.TXResult_SUCCESS.Enum(),
		},
	}
	if len(revertReason) > 0 {
		txi.Result = blockindexer.TXResult_FAILURE.Enum()
		txi.RevertReason = revertReason[0]
	}
	return txi
}

func TestPublicConfirmWithErrorDecodeRealDB(t *testing.T) {

	testABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{}},
		{Type: abi.Error, Name: "ErrorNum", Inputs: abi.ParameterArray{{Type: "uint256"}}},
	}
	revertData, err := testABI.Errors()["ErrorNum"].EncodeCallDataJSON([]byte(`[12345]`))
	require.NoError(t, err)

	txi := newTestConfirm(revertData)
	var txID *uuid.UUID

	ctx, txm, done := newTestTransactionManager(t, true, func(conf *Config, mc *mockComponents) {
		mockSubmissionBatch := componentmocks.NewPublicTxBatch(t)
		mockSubmissionBatch.On("Rejected").Return([]components.PublicTxRejected{})
		mockSubmissionBatch.On("Submit", mock.Anything, mock.Anything).Return(nil)
		mockSubmissionBatch.On("Completed", mock.Anything, true).Return(nil)
		mc.publicTxMgr.On("PrepareSubmissionBatch", mock.Anything, mock.Anything).Return(mockSubmissionBatch, nil)

		mut := mc.publicTxMgr.On("MatchUpdateConfirmedTransactions", mock.Anything, mock.Anything, []*blockindexer.IndexedTransactionNotify{txi})
		mut.Run(func(args mock.Arguments) {
			mut.Return([]*components.PublicTxMatch{
				{
					PaladinTXReference: components.PaladinTXReference{
						TransactionID:   *txID, // Transaction ID resolved by this point
						TransactionType: ptxapi.TransactionTypePublic.Enum(),
					},
					IndexedTransactionNotify: txi,
				},
			}, nil)
		})

		mc.publicTxMgr.On("NotifyConfirmPersisted", mock.Anything, mock.MatchedBy(func(matches []*components.PublicTxMatch) bool {
			return len(matches) == 1 && matches[0].TransactionID == *txID
		}))
	})
	defer done()

	abiRef, err := txm.storeABI(ctx, testABI)
	require.NoError(t, err)

	txID, err = txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type:         ptxapi.TransactionTypePublic.Enum(),
			ABIReference: abiRef,
			To:           tktypes.MustEthAddress(tktypes.RandHex(20)),
		},
	})
	require.NoError(t, err)

	postCommit, err := txm.blockIndexerPreCommit(ctx, txm.p.DB(), []*blockindexer.IndexedBlock{},
		[]*blockindexer.IndexedTransactionNotify{txi})
	require.NoError(t, err)
	postCommit()

	// Check we can query the receipt
	receipt, err := txm.getTransactionReceiptByID(ctx, *txID)
	require.NoError(t, err)
	assert.False(t, receipt.Success)
	assert.Equal(t, `PD012216: Transaction reverted ErrorNum("12345")`, receipt.FailureMessage)
}

func TestPublicConfirmMatch(t *testing.T) {

	txi := newTestConfirm()
	txID := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *Config, mc *mockComponents) {
		mc.publicTxMgr.On("MatchUpdateConfirmedTransactions", mock.Anything, mock.Anything, []*blockindexer.IndexedTransactionNotify{txi}).
			Return([]*components.PublicTxMatch{
				{
					PaladinTXReference: components.PaladinTXReference{
						TransactionID:   txID,
						TransactionType: ptxapi.TransactionTypePublic.Enum(),
					},
					IndexedTransactionNotify: txi,
				},
			}, nil)

		mc.db.ExpectExec("INSERT.*transaction_receipts").WillReturnResult(driver.ResultNoRows)

		mc.publicTxMgr.On("NotifyConfirmPersisted", mock.Anything, mock.MatchedBy(func(matches []*components.PublicTxMatch) bool {
			return len(matches) == 1 && matches[0].TransactionID == txID
		}))
	})
	defer done()

	postCommit, err := txm.blockIndexerPreCommit(ctx, txm.p.DB(), []*blockindexer.IndexedBlock{},
		[]*blockindexer.IndexedTransactionNotify{txi})
	require.NoError(t, err)
	postCommit()
}

func TestPrivateConfirmMatch(t *testing.T) {

	txi := newTestConfirm()
	txID := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *Config, mc *mockComponents) {
		mc.publicTxMgr.On("MatchUpdateConfirmedTransactions", mock.Anything, mock.Anything, []*blockindexer.IndexedTransactionNotify{txi}).
			Return([]*components.PublicTxMatch{
				{
					PaladinTXReference: components.PaladinTXReference{
						TransactionID:   txID,
						TransactionType: ptxapi.TransactionTypePrivate.Enum(),
					},
					IndexedTransactionNotify: txi,
				},
			}, nil)

		mc.db.ExpectExec("INSERT.*transaction_receipts").WillReturnResult(driver.ResultNoRows)

		mnc := mc.privateTxMgr.On("NotifyConfirmed", mock.Anything, mock.Anything)
		mnc.Run(func(args mock.Arguments) {
			completed := make(map[uuid.UUID]bool)
			confirms := args[1].([]*components.PublicTxMatch)
			for _, c := range confirms {
				completed[c.TransactionID] = true
			}
			mnc.Return(completed, nil)
		})

		mc.publicTxMgr.On("NotifyConfirmPersisted", mock.Anything, mock.MatchedBy(func(matches []*components.PublicTxMatch) bool {
			return len(matches) == 1 && matches[0].TransactionID == txID
		}))
	})
	defer done()

	postCommit, err := txm.blockIndexerPreCommit(ctx, txm.p.DB(), []*blockindexer.IndexedBlock{},
		[]*blockindexer.IndexedTransactionNotify{txi})
	require.NoError(t, err)
	postCommit()
}

func TestNoConfirmMatch(t *testing.T) {

	txi := newTestConfirm()

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *Config, mc *mockComponents) {
		mc.publicTxMgr.On("MatchUpdateConfirmedTransactions", mock.Anything, mock.Anything, []*blockindexer.IndexedTransactionNotify{txi}).
			Return(nil, nil)
	})
	defer done()

	postCommit, err := txm.blockIndexerPreCommit(ctx, txm.p.DB(), []*blockindexer.IndexedBlock{},
		[]*blockindexer.IndexedTransactionNotify{txi})
	require.NoError(t, err)
	postCommit()
}

func TestConfirmMatchFAil(t *testing.T) {

	txi := newTestConfirm()

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *Config, mc *mockComponents) {
		mc.publicTxMgr.On("MatchUpdateConfirmedTransactions", mock.Anything, mock.Anything, []*blockindexer.IndexedTransactionNotify{txi}).
			Return(nil, fmt.Errorf("pop"))
	})
	defer done()

	_, err := txm.blockIndexerPreCommit(ctx, txm.p.DB(), []*blockindexer.IndexedBlock{},
		[]*blockindexer.IndexedTransactionNotify{txi})
	assert.Regexp(t, "pop", err)
}

func TestPrivateConfirmError(t *testing.T) {

	txi := newTestConfirm()
	txID := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *Config, mc *mockComponents) {
		mc.publicTxMgr.On("MatchUpdateConfirmedTransactions", mock.Anything, mock.Anything, []*blockindexer.IndexedTransactionNotify{txi}).
			Return([]*components.PublicTxMatch{
				{
					PaladinTXReference: components.PaladinTXReference{
						TransactionID:   txID,
						TransactionType: ptxapi.TransactionTypePrivate.Enum(),
					},
					IndexedTransactionNotify: txi,
				},
			}, nil)
		mc.privateTxMgr.On("NotifyConfirmed", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()

	_, err := txm.blockIndexerPreCommit(ctx, txm.p.DB(), []*blockindexer.IndexedBlock{},
		[]*blockindexer.IndexedTransactionNotify{txi})
	assert.Regexp(t, "pop", err)
}

func TestConfirmInsertError(t *testing.T) {

	txi := newTestConfirm()
	txID := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *Config, mc *mockComponents) {
		mc.publicTxMgr.On("MatchUpdateConfirmedTransactions", mock.Anything, mock.Anything, []*blockindexer.IndexedTransactionNotify{txi}).
			Return([]*components.PublicTxMatch{
				{
					PaladinTXReference: components.PaladinTXReference{
						TransactionID:   txID,
						TransactionType: ptxapi.TransactionTypePublic.Enum(),
					},
					IndexedTransactionNotify: txi,
				},
			}, nil)

		mc.db.ExpectExec("INSERT.*transaction_receipts").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	_, err := txm.blockIndexerPreCommit(ctx, txm.p.DB(), []*blockindexer.IndexedBlock{},
		[]*blockindexer.IndexedTransactionNotify{txi})
	assert.Regexp(t, "pop", err)
}
