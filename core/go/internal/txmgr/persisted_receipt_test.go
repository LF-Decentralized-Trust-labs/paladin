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
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/ptxapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFinalizeTransactionsNoOp(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false)
	defer done()

	err := txm.FinalizeTransactions(ctx, txm.p.DB(), nil)
	assert.NoError(t, err)

}

func TestFinalizeTransactionsLookupFail(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*transactions").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	txID := uuid.New()
	_, err := txm.MatchAndFinalizeTransactions(ctx, txm.p.DB(), []*components.ReceiptInput{
		{TransactionID: txID, ReceiptType: components.RT_Success},
	})
	assert.Regexp(t, "pop", err)

}

func TestFinalizeTransactionsSuccessWithFailure(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(txID))
	})
	defer done()

	_, err := txm.MatchAndFinalizeTransactions(ctx, txm.p.DB(), []*components.ReceiptInput{
		{TransactionID: txID, ReceiptType: components.RT_Success,
			FailureMessage: "not empty"},
	})
	assert.Regexp(t, "PD012213", err)
}

func TestFinalizeTransactionsBadType(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(txID))
	})
	defer done()

	_, err := txm.MatchAndFinalizeTransactions(ctx, txm.p.DB(), []*components.ReceiptInput{
		{TransactionID: txID, ReceiptType: components.ReceiptType(42)},
	})
	assert.Regexp(t, "PD012213", err)

}

func TestFinalizeTransactionsFailedWithMessageNoMessage(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(txID))
	})
	defer done()

	_, err := txm.MatchAndFinalizeTransactions(ctx, txm.p.DB(), []*components.ReceiptInput{
		{TransactionID: txID, ReceiptType: components.RT_FailedWithMessage},
	})
	assert.Regexp(t, "PD012213", err)

}

func TestFinalizeTransactionsFailedWithRevertDataWithMessage(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(txID))
	})
	defer done()

	_, err := txm.MatchAndFinalizeTransactions(ctx, txm.p.DB(), []*components.ReceiptInput{
		{TransactionID: txID, ReceiptType: components.RT_FailedOnChainWithRevertData,
			FailureMessage: "not empty"},
	})
	assert.Regexp(t, "PD012213", err)

}

func TestFinalizeTransactionsInsertFail(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectBegin()
		mc.db.ExpectExec("INSERT.*transaction_receipts").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	err := txm.p.DB().Transaction(func(tx *gorm.DB) error {
		return txm.FinalizeTransactions(ctx, tx, []*components.ReceiptInput{
			{TransactionID: txID, ReceiptType: components.RT_FailedWithMessage,
				FailureMessage: "something went wrong"},
		})
	})
	assert.Regexp(t, "pop", err)

}

func TestFinalizeTransactionsInsertOkOffChain(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, true, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.privateTxMgr.On("HandleNewTx", mock.Anything, mock.Anything).Return(nil)
	})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	txID, err := txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type:     ptxapi.TransactionTypePrivate.Enum(),
			Domain:   "domain1",
			Function: "doIt",
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)

	err = txm.p.DB().Transaction(func(tx *gorm.DB) error {
		return txm.FinalizeTransactions(ctx, tx, []*components.ReceiptInput{
			{
				TransactionID: *txID,
				ReceiptType:   components.RT_FailedOnChainWithRevertData,
			},
		})
	})
	require.NoError(t, err)

	receipt, err := txm.getTransactionReceiptByID(ctx, *txID)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.JSONEq(t, fmt.Sprintf(`{
		"id":"%s",
		"failureMessage":"PD012214: Transaction reverted (no revert data)"
	}`, txID), string(tktypes.JSONString(receipt)))

}

func TestFinalizeTransactionsInsertOkEvent(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, true, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.privateTxMgr.On("HandleNewTx", mock.Anything, mock.Anything).Return(nil)
	})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	txID, err := txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type:     ptxapi.TransactionTypePrivate.Enum(),
			Domain:   "domain1",
			Function: "doIt",
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)

	err = txm.p.DB().Transaction(func(tx *gorm.DB) error {
		return txm.FinalizeTransactions(ctx, tx, []*components.ReceiptInput{
			{
				TransactionID: *txID,
				ReceiptType:   components.RT_Success,
				OnChain: tktypes.OnChainLocation{
					Type:             tktypes.OnChainEvent,
					TransactionHash:  tktypes.MustParseBytes32("d0561b310b77e47bc16fb3c40d48b72255b1748efeecf7452373dfce8045af30"),
					BlockNumber:      12345,
					TransactionIndex: 10,
					LogIndex:         5,
					Source:           tktypes.MustEthAddress("0x3f9f796ff55589dd2358c458f185bbed357c0b6e"),
				},
			},
		})
	})
	require.NoError(t, err)

	receipt, err := txm.getTransactionReceiptByID(ctx, *txID)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.JSONEq(t, fmt.Sprintf(`{
		"id":"%s",
		"blockNumber":12345, 
		"logIndex":5,
	 	"source":"0x3f9f796ff55589dd2358c458f185bbed357c0b6e",
	  	"success":true, 
	  	"transactionHash":"0xd0561b310b77e47bc16fb3c40d48b72255b1748efeecf7452373dfce8045af30", 
		"transactionIndex":10
	}`, txID), string(tktypes.JSONString(receipt)))

}

func TestFinalizeTransactionsIgnoreUnknown(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	})
	defer done()

	ids, err := txm.MatchAndFinalizeTransactions(ctx, txm.p.DB(), []*components.ReceiptInput{
		{TransactionID: uuid.New(), ReceiptType: components.RT_FailedOnChainWithRevertData,
			FailureMessage: "will be ignored"},
	})
	assert.NoError(t, err)
	assert.Empty(t, ids)

}

func TestCalculateRevertErrorNoData(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false)
	defer done()

	err := txm.CalculateRevertError(ctx, nil, nil)
	assert.Regexp(t, "PD012214", err)

}

func TestCalculateRevertErrorQueryFail(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*abi_errors").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	err := txm.CalculateRevertError(ctx, txm.p.DB(), []byte("any data"))
	assert.Regexp(t, "PD012215.*pop", err)

}

func TestCalculateRevertErrorDecodeFail(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*abi_errors").WillReturnRows(sqlmock.NewRows([]string{"definition"}).AddRow(`{}`))
	})
	defer done()

	err := txm.CalculateRevertError(ctx, txm.p.DB(), []byte("any data"))
	assert.Regexp(t, "PD012215", err)

}

func TestGetTransactionReceiptNoResult(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{}))
	})
	defer done()

	res, err := txm.getTransactionReceiptByID(ctx, uuid.New())
	assert.NoError(t, err)
	assert.Nil(t, res)

}
