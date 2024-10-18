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
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"

	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newTestConfirm(revertReason ...[]byte) *blockindexer.IndexedTransactionNotify {
	txi := &blockindexer.IndexedTransactionNotify{
		IndexedTransaction: pldapi.IndexedTransaction{
			Hash:             tktypes.Bytes32(tktypes.RandBytes(32)),
			BlockNumber:      12345,
			TransactionIndex: 0,
			From:             tktypes.MustEthAddress(tktypes.RandHex(20)),
			Nonce:            1000,
			To:               nil,
			ContractAddress:  tktypes.MustEthAddress(tktypes.RandHex(20)),
			Result:           pldapi.TXResult_SUCCESS.Enum(),
		},
	}
	if len(revertReason) > 0 {
		txi.Result = pldapi.TXResult_FAILURE.Enum()
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

	ctx, txm, done := newTestTransactionManager(t, true, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
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
						TransactionType: pldapi.TransactionTypePublic.Enum(),
					},
					IndexedTransactionNotify: txi,
				},
			}, nil)
		})

		mc.publicTxMgr.On("NotifyConfirmPersisted", mock.Anything, mock.MatchedBy(func(matches []*components.PublicTxMatch) bool {
			return len(matches) == 1 && matches[0].TransactionID == *txID
		}))

		mc.keyManager.On("ResolveEthAddressBatchNewDatabaseTX", mock.Anything, []string{"sender1"}).
			Return([]*tktypes.EthAddress{tktypes.RandAddress()}, nil)
	})
	defer done()

	abiRef, err := txm.storeABI(ctx, testABI)
	require.NoError(t, err)

	txID, err = txm.SendTransaction(ctx, &pldapi.TransactionInput{
		Transaction: pldapi.Transaction{
			Type:         pldapi.TransactionTypePublic.Enum(),
			ABIReference: abiRef,
			From:         "sender1",
			To:           tktypes.MustEthAddress(tktypes.RandHex(20)),
		},
	})
	require.NoError(t, err)

	postCommit, err := txm.blockIndexerPreCommit(ctx, txm.p.DB(), []*pldapi.IndexedBlock{},
		[]*blockindexer.IndexedTransactionNotify{txi})
	require.NoError(t, err)
	postCommit()

	// Check we can query the receipt
	receipt, err := txm.GetTransactionReceiptByID(ctx, *txID)
	require.NoError(t, err)
	assert.False(t, receipt.Success)
	assert.Equal(t, `PD012216: Transaction reverted ErrorNum("12345")`, receipt.FailureMessage)
}

func TestPublicConfirmMatch(t *testing.T) {

	txi := newTestConfirm()
	txID := uuid.New()
	txi.ContractAddress = tktypes.RandAddress()

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.publicTxMgr.On("MatchUpdateConfirmedTransactions", mock.Anything, mock.Anything, []*blockindexer.IndexedTransactionNotify{txi}).
			Return([]*components.PublicTxMatch{
				{
					PaladinTXReference: components.PaladinTXReference{
						TransactionID:   txID,
						TransactionType: pldapi.TransactionTypePublic.Enum(),
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

	postCommit, err := txm.blockIndexerPreCommit(ctx, txm.p.DB(), []*pldapi.IndexedBlock{},
		[]*blockindexer.IndexedTransactionNotify{txi})
	require.NoError(t, err)
	postCommit()
}

func TestPrivateConfirmMatchPrivateFailures(t *testing.T) {

	testABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{}},
		{Type: abi.Error, Name: "ErrorNum", Inputs: abi.ParameterArray{{Type: "uint256"}}},
	}
	revertData, err := testABI.Errors()["ErrorNum"].EncodeCallDataJSON([]byte(`[12345]`))
	require.NoError(t, err)

	txiOk1 := newTestConfirm() // one succeeded
	txID1 := uuid.New()
	txiFail2 := newTestConfirm(revertData) // one failed
	txID2 := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.publicTxMgr.On("MatchUpdateConfirmedTransactions", mock.Anything, mock.Anything,
			[]*blockindexer.IndexedTransactionNotify{txiOk1, txiFail2}).
			Return([]*components.PublicTxMatch{
				{
					PaladinTXReference: components.PaladinTXReference{
						TransactionID:   txID1,
						TransactionType: pldapi.TransactionTypePrivate.Enum(),
					},
					IndexedTransactionNotify: txiOk1,
				},
				{
					PaladinTXReference: components.PaladinTXReference{
						TransactionID:   txID2,
						TransactionType: pldapi.TransactionTypePrivate.Enum(),
					},
					IndexedTransactionNotify: txiFail2,
				},
			}, nil)

		mc.privateTxMgr.On("NotifyFailedPublicTx", mock.Anything, mock.Anything, mock.MatchedBy(func(matches []*components.PublicTxMatch) bool {
			return len(matches) == 1 &&
				matches[0].TransactionID == txID2
		})).Return(nil)

		mc.publicTxMgr.On("NotifyConfirmPersisted", mock.Anything, mock.MatchedBy(func(matches []*components.PublicTxMatch) bool {
			return len(matches) == 2 &&
				matches[0].TransactionID == txID1 &&
				matches[1].TransactionID == txID2
		}))
	})
	defer done()

	postCommit, err := txm.blockIndexerPreCommit(ctx, txm.p.DB(), []*pldapi.IndexedBlock{},
		[]*blockindexer.IndexedTransactionNotify{txiOk1, txiFail2})
	require.NoError(t, err)
	postCommit()
}

func TestNoConfirmMatch(t *testing.T) {

	txi := newTestConfirm()

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.publicTxMgr.On("MatchUpdateConfirmedTransactions", mock.Anything, mock.Anything, []*blockindexer.IndexedTransactionNotify{txi}).
			Return(nil, nil)
	})
	defer done()

	postCommit, err := txm.blockIndexerPreCommit(ctx, txm.p.DB(), []*pldapi.IndexedBlock{},
		[]*blockindexer.IndexedTransactionNotify{txi})
	require.NoError(t, err)
	postCommit()
}

func TestConfirmMatchFAil(t *testing.T) {

	txi := newTestConfirm()

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.publicTxMgr.On("MatchUpdateConfirmedTransactions", mock.Anything, mock.Anything, []*blockindexer.IndexedTransactionNotify{txi}).
			Return(nil, fmt.Errorf("pop"))
	})
	defer done()

	_, err := txm.blockIndexerPreCommit(ctx, txm.p.DB(), []*pldapi.IndexedBlock{},
		[]*blockindexer.IndexedTransactionNotify{txi})
	assert.Regexp(t, "pop", err)
}

func TestPrivateConfirmError(t *testing.T) {

	txi := newTestConfirm([]byte("revert data"))
	txID := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.publicTxMgr.On("MatchUpdateConfirmedTransactions", mock.Anything, mock.Anything, []*blockindexer.IndexedTransactionNotify{txi}).
			Return([]*components.PublicTxMatch{
				{
					PaladinTXReference: components.PaladinTXReference{
						TransactionID:   txID,
						TransactionType: pldapi.TransactionTypePrivate.Enum(),
					},
					IndexedTransactionNotify: txi,
				},
			}, nil)
		mc.privateTxMgr.On("NotifyFailedPublicTx", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	})
	defer done()

	_, err := txm.blockIndexerPreCommit(ctx, txm.p.DB(), []*pldapi.IndexedBlock{},
		[]*blockindexer.IndexedTransactionNotify{txi})
	assert.Regexp(t, "pop", err)
}

func TestConfirmInsertError(t *testing.T) {

	txi := newTestConfirm()
	txID := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.publicTxMgr.On("MatchUpdateConfirmedTransactions", mock.Anything, mock.Anything, []*blockindexer.IndexedTransactionNotify{txi}).
			Return([]*components.PublicTxMatch{
				{
					PaladinTXReference: components.PaladinTXReference{
						TransactionID:   txID,
						TransactionType: pldapi.TransactionTypePublic.Enum(),
					},
					IndexedTransactionNotify: txi,
				},
			}, nil)

		mc.db.ExpectExec("INSERT.*transaction_receipts").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	_, err := txm.blockIndexerPreCommit(ctx, txm.p.DB(), []*pldapi.IndexedBlock{},
		[]*blockindexer.IndexedTransactionNotify{txi})
	assert.Regexp(t, "pop", err)
}
