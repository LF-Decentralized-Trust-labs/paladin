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
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/blockindexer"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newTestConfirm(revertReason ...[]byte) *blockindexer.IndexedTransactionNotify {
	txi := &blockindexer.IndexedTransactionNotify{
		IndexedTransaction: pldapi.IndexedTransaction{
			Hash:             pldtypes.RandBytes32(),
			BlockNumber:      12345,
			TransactionIndex: 0,
			From:             pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Nonce:            1000,
			To:               nil,
			ContractAddress:  pldtypes.MustEthAddress(pldtypes.RandHex(20)),
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
	var txID uuid.UUID

	ctx, txm, done := newTestTransactionManager(t, true,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mockResolveKey(t, mc, "sender1", pldtypes.RandAddress())

			mc.publicTxMgr.On("ValidateTransaction", mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mc.publicTxMgr.On("WriteNewTransactions", mock.Anything, mock.Anything, mock.Anything).Return(
				[]*pldapi.PublicTx{
					{LocalID: confutil.P(uint64(42))},
				},
				nil,
			)

			mut := mc.publicTxMgr.On("MatchUpdateConfirmedTransactions", mock.Anything, mock.Anything, []*blockindexer.IndexedTransactionNotify{txi})
			mut.Run(func(args mock.Arguments) {
				mut.Return([]*components.PublicTxMatch{
					{
						PaladinTXReference: components.PaladinTXReference{
							TransactionID:   txID, // Transaction ID resolved by this point
							TransactionType: pldapi.TransactionTypePublic.Enum(),
						},
						IndexedTransactionNotify: txi,
					},
				}, nil)
			})

			mc.publicTxMgr.On("NotifyConfirmPersisted", mock.Anything, mock.MatchedBy(func(matches []*components.PublicTxMatch) bool {
				return len(matches) == 1 && matches[0].TransactionID == txID
			}))
		})
	defer done()

	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		abiRef, err := txm.storeABI(ctx, dbTX, testABI)
		require.NoError(t, err)

		txIDs, err := txm.SendTransactions(ctx, dbTX, &pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				Type:         pldapi.TransactionTypePublic.Enum(),
				ABIReference: abiRef,
				From:         "sender1",
				To:           pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			},
		})
		require.NoError(t, err)
		txID = txIDs[0]

		return txm.blockIndexerPreCommit(ctx, dbTX, []*pldapi.IndexedBlock{},
			[]*blockindexer.IndexedTransactionNotify{txi})
	})
	require.NoError(t, err)

	// Check we can query the receipt
	receipt, err := txm.GetTransactionReceiptByID(ctx, txID)
	require.NoError(t, err)
	assert.False(t, receipt.Success)
	assert.Equal(t, `PD012216: Transaction reverted ErrorNum("12345")`, receipt.FailureMessage)
}

func mockEmptyReceiptListeners(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	mc.db.ExpectQuery("SELECT.*receipt_listeners").WillReturnRows(sqlmock.NewRows([]string{}))
}

func mockNoGaps(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	mc.db.MatchExpectationsInOrder(false)
	mc.db.ExpectQuery("SELECT.*receipt_listener_gap").WillReturnRows(sqlmock.NewRows([]string{}))
}

func TestPublicConfirmMatch(t *testing.T) {

	txi := newTestConfirm()
	txID := uuid.New()
	txi.ContractAddress = pldtypes.RandAddress()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
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

			mc.db.ExpectBegin()
			mc.db.ExpectQuery("INSERT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{"sequence"}).AddRow(12345))
			mc.db.ExpectCommit()

			mc.publicTxMgr.On("NotifyConfirmPersisted", mock.Anything, mock.MatchedBy(func(matches []*components.PublicTxMatch) bool {
				return len(matches) == 1 && matches[0].TransactionID == txID
			}))
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		return txm.blockIndexerPreCommit(ctx, dbTX, []*pldapi.IndexedBlock{},
			[]*blockindexer.IndexedTransactionNotify{txi})
	})
	require.NoError(t, err)
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

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
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

			mc.db.ExpectBegin()
			mc.db.ExpectCommit()
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

	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		return txm.blockIndexerPreCommit(ctx, dbTX, []*pldapi.IndexedBlock{},
			[]*blockindexer.IndexedTransactionNotify{txiOk1, txiFail2})
	})
	require.NoError(t, err)
}

func TestNoConfirmMatch(t *testing.T) {

	txi := newTestConfirm()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.publicTxMgr.On("MatchUpdateConfirmedTransactions", mock.Anything, mock.Anything, []*blockindexer.IndexedTransactionNotify{txi}).
				Return(nil, nil)
			mc.db.ExpectCommit()
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		return txm.blockIndexerPreCommit(ctx, dbTX, []*pldapi.IndexedBlock{},
			[]*blockindexer.IndexedTransactionNotify{txi})
	})
	require.NoError(t, err)
}

func TestConfirmMatchFAil(t *testing.T) {

	txi := newTestConfirm()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.publicTxMgr.On("MatchUpdateConfirmedTransactions", mock.Anything, mock.Anything, []*blockindexer.IndexedTransactionNotify{txi}).
				Return(nil, fmt.Errorf("pop"))
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		return txm.blockIndexerPreCommit(ctx, dbTX, []*pldapi.IndexedBlock{},
			[]*blockindexer.IndexedTransactionNotify{txi})
	})
	assert.Regexp(t, "pop", err)
}

func TestPrivateConfirmError(t *testing.T) {

	txi := newTestConfirm([]byte("revert data"))
	txID := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
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

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		return txm.blockIndexerPreCommit(ctx, dbTX, []*pldapi.IndexedBlock{},
			[]*blockindexer.IndexedTransactionNotify{txi})
	})
	assert.Regexp(t, "pop", err)
}

func TestConfirmInsertError(t *testing.T) {

	txi := newTestConfirm()
	txID := uuid.New()

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
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

			mc.db.ExpectBegin()
			mc.db.ExpectQuery("INSERT.*transaction_receipts").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		return txm.blockIndexerPreCommit(ctx, dbTX, []*pldapi.IndexedBlock{},
			[]*blockindexer.IndexedTransactionNotify{txi})
	})
	assert.Regexp(t, "pop", err)
}
