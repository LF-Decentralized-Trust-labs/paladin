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
	"database/sql/driver"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentsmocks"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type testReceiptReceiver struct {
	err       error
	callCount int
	called    chan struct{}
	receipts  chan *pldapi.TransactionReceiptFull
}

func (trr *testReceiptReceiver) DeliverReceiptBatch(ctx context.Context, batchID uint64, receipts []*pldapi.TransactionReceiptFull) error {
	if trr.callCount == 0 {
		close(trr.called)
	}
	trr.callCount++
	if trr.err != nil {
		return trr.err
	}
	for _, r := range receipts {
		trr.receipts <- r
	}
	return nil
}

func newTestReceiptReceiver(err error) *testReceiptReceiver {
	return &testReceiptReceiver{
		err:      err,
		called:   make(chan struct{}),
		receipts: make(chan *pldapi.TransactionReceiptFull, 1),
	}
}

var defaultErrorABI = &abi.Entry{
	Type: abi.Error,
	Name: "Error",
	Inputs: abi.ParameterArray{
		{Name: "message", Type: "string"},
	},
}

func mockTxStatesAllAvailable(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, mock.Anything).
		Return(&pldapi.TransactionStates{
			Spent:     []*pldapi.StateBase{},
			Read:      []*pldapi.StateBase{},
			Confirmed: []*pldapi.StateBase{},
			Info:      []*pldapi.StateBase{},
		}, nil)
}

func mockDomainStateCompletion(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	md := componentsmocks.NewDomain(mc.t)
	mc.domainManager.On("GetDomainByName", mock.Anything, mock.Anything).Return(md, nil)
	md.On("CheckStateCompletion", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(func(ctx context.Context, dbTX persistence.DBTX, txID uuid.UUID, txStates *pldapi.TransactionStates) (bool, error) {
			return txStates.Unavailable == nil, nil
		})
}

func mockDomainStateCompletionAndReceipt(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	md := componentsmocks.NewDomain(mc.t)
	mc.domainManager.On("GetDomainByName", mock.Anything, mock.Anything).Return(md, nil)
	md.On("CheckStateCompletion", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(func(ctx context.Context, dbTX persistence.DBTX, txID uuid.UUID, txStates *pldapi.TransactionStates) (bool, error) {
			return txStates.Unavailable == nil, nil
		})
	md.On("BuildDomainReceipt", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(pldtypes.RawJSON{}, nil)
}

func TestE2EReceiptListenerDeliveryLateAttach(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, true, mockTxStatesAllAvailable, mockDomainStateCompletion)
	defer done()

	// Create listener (started)
	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
	})
	require.NoError(t, err)

	// Write some receipts (before we attach to the listener to consume events)
	contractAddr1 := pldtypes.RandAddress()
	contractAddr2 := pldtypes.RandAddress()
	crackleData, err := defaultErrorABI.EncodeCallDataJSON([]byte(`{
	    "message": "crackle"
	}`))
	require.NoError(t, err)
	popData, err := defaultErrorABI.EncodeCallDataJSON([]byte(`{
	    "message": "pop"
	}`))
	require.NoError(t, err)
	receiptInputs := []*components.ReceiptInput{
		{
			ReceiptType:    components.RT_FailedWithMessage,
			Domain:         "", // public, failed without making it to chain
			TransactionID:  uuid.New(),
			FailureMessage: "snap",
		},
		{
			ReceiptType:   components.RT_FailedOnChainWithRevertData,
			Domain:        "domain1", // private, failed on-chain
			TransactionID: uuid.New(),
			RevertData:    crackleData,
			OnChain: pldtypes.OnChainLocation{
				Type:             pldtypes.OnChainEvent,
				TransactionHash:  pldtypes.Bytes32(pldtypes.RandBytes(32)),
				BlockNumber:      12345,
				TransactionIndex: 20,
				LogIndex:         10,
				Source:           contractAddr1,
			},
		},
		{
			ReceiptType:   components.RT_FailedOnChainWithRevertData,
			Domain:        "", // public, failed on-chain
			TransactionID: uuid.New(),
			RevertData:    popData,
			OnChain: pldtypes.OnChainLocation{
				Type:             pldtypes.OnChainTransaction,
				TransactionHash:  pldtypes.Bytes32(pldtypes.RandBytes(32)),
				BlockNumber:      12345,
				TransactionIndex: 10,
				Source:           contractAddr2,
			},
		},
	}
	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, receiptInputs)
	})
	require.NoError(t, err)

	// Create a receiver and check we get everything delivered
	receipts := newTestReceiptReceiver(nil)
	closeReceiver, err := txm.AddReceiptReceiver(ctx, "listener1", receipts)
	require.NoError(t, err)
	defer closeReceiver.Close()

	// These will have all ended up in a single batch, as they were committed together
	r := <-receipts.receipts
	assert.Equal(t, r.ID, receiptInputs[0].TransactionID)
	assert.Regexp(t, "snap", r.FailureMessage)
	r = <-receipts.receipts
	assert.Equal(t, r.ID, receiptInputs[1].TransactionID)
	assert.Regexp(t, "crackle", r.FailureMessage)
	r = <-receipts.receipts
	assert.Equal(t, r.ID, receiptInputs[2].TransactionID)
	assert.Regexp(t, "pop", r.FailureMessage)

	// Now send another - we make this one successful
	receiptInputs2 := []*components.ReceiptInput{
		{
			ReceiptType:   components.RT_Success,
			Domain:        "", // public, success
			TransactionID: uuid.New(),
			OnChain: pldtypes.OnChainLocation{
				Type:             pldtypes.OnChainTransaction,
				TransactionHash:  pldtypes.Bytes32(pldtypes.RandBytes(32)),
				BlockNumber:      23456,
				TransactionIndex: 30,
				Source:           contractAddr2,
			},
		},
	}
	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, receiptInputs2)
	})
	require.NoError(t, err)

	// This one is assured to be in a new batch
	r = <-receipts.receipts
	assert.Equal(t, r.ID, receiptInputs2[0].TransactionID)
	assert.Empty(t, r.FailureMessage)

}

func randOnChain(addr *pldtypes.EthAddress) pldtypes.OnChainLocation {
	return pldtypes.OnChainLocation{
		Type:             pldtypes.OnChainTransaction,
		TransactionHash:  pldtypes.Bytes32(pldtypes.RandBytes(32)),
		BlockNumber:      23456,
		TransactionIndex: 30,
		Source:           addr,
	}
}

func TestLoadListenersMultiPageFilters(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, true, mockTxStatesAllAvailable, mockDomainStateCompletion)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
		Filters: pldapi.TransactionReceiptFilters{
			Type:   confutil.P(pldapi.TransactionTypePrivate.Enum()),
			Domain: "domain1",
		},
	})
	require.NoError(t, err)
	err = txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener2",
		Started: confutil.P(false),
		Filters: pldapi.TransactionReceiptFilters{
			Type: confutil.P(pldapi.TransactionTypePrivate.Enum()),
		},
	})
	require.NoError(t, err)
	err = txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener3",
		Started: confutil.P(false),
		Filters: pldapi.TransactionReceiptFilters{
			Type: confutil.P(pldapi.TransactionTypePublic.Enum()),
		},
	})
	require.NoError(t, err)
	err = txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener4",
		Started: confutil.P(false),
		Filters: pldapi.TransactionReceiptFilters{
			SequenceAbove: confutil.P(uint64(100000)), // will ignore all, even though no other filters
		},
	})
	require.NoError(t, err)

	txm.receiptsInit()
	txm.receiptListenersLoadPageSize = 1

	err = txm.loadReceiptListeners()
	require.NoError(t, err)

	require.Len(t, txm.receiptListeners, 4)

	// for variation we register before start here
	r1 := newTestReceiptReceiver(nil)
	close1, err := txm.AddReceiptReceiver(ctx, "listener1", r1)
	require.NoError(t, err)
	defer close1.Close()
	r2 := newTestReceiptReceiver(nil)
	close2, err := txm.AddReceiptReceiver(ctx, "listener2", r2)
	require.NoError(t, err)
	defer close2.Close()
	r3 := newTestReceiptReceiver(nil)
	close3, err := txm.AddReceiptReceiver(ctx, "listener3", r3)
	require.NoError(t, err)
	defer close3.Close()
	r4 := newTestReceiptReceiver(nil)
	close4, err := txm.AddReceiptReceiver(ctx, "listener4", r4)
	require.NoError(t, err)
	defer close4.Close()

	// Now start them all
	err = txm.StartReceiptListener(ctx, "listener1")
	require.NoError(t, err)
	err = txm.StartReceiptListener(ctx, "listener2")
	require.NoError(t, err)
	err = txm.StartReceiptListener(ctx, "listener3")
	require.NoError(t, err)
	err = txm.StartReceiptListener(ctx, "listener4")
	require.NoError(t, err)

	tx1 := uuid.New()
	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		// Private domain2 to listener2 only
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				ReceiptType:   components.RT_Success,
				Domain:        "domain2",
				TransactionID: tx1,
				OnChain:       randOnChain(pldtypes.RandAddress()),
			},
		})
	})
	require.NoError(t, err)
	require.Equal(t, tx1, (<-r2.receipts).ID)

	// Private domain1 to listener 1&2
	tx2 := uuid.New()
	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				ReceiptType:   components.RT_Success,
				Domain:        "domain1",
				TransactionID: tx2,
				OnChain:       randOnChain(pldtypes.RandAddress()),
			},
		})
	})
	require.NoError(t, err)
	require.Equal(t, tx2, (<-r1.receipts).ID)
	require.Equal(t, tx2, (<-r2.receipts).ID)

	// Public to listener3
	tx3 := uuid.New()
	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				ReceiptType:   components.RT_Success,
				Domain:        "",
				TransactionID: tx3,
				OnChain:       randOnChain(pldtypes.RandAddress()),
			},
		})
	})
	require.NoError(t, err)
	require.Equal(t, tx3, (<-r3.receipts).ID)

	// Nothing should have gone to 4
	select {
	case <-r4.receipts:
		require.Fail(t, "received receipt")
	default:
	}

	// Wait for checkpoint
	for txm.receiptListeners["listener3"].checkpoint == nil {
		time.Sleep(10 * time.Millisecond)
	}

	// restart and deliver next
	txm.stopReceiptListeners()
	txm.startReceiptListeners()

	// Public to listener3 again
	tx4 := uuid.New()
	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				ReceiptType:   components.RT_Success,
				Domain:        "",
				TransactionID: tx4,
				OnChain:       randOnChain(pldtypes.RandAddress()),
			},
		})
	})
	require.NoError(t, err)
	require.Equal(t, tx4, (<-r3.receipts).ID)

}

func TestGapsDomainsForNonAvailableReceipts(t *testing.T) {
	testGapsDomainsForNonAvailableReceipts(t, 100)
}

func TestGapsDomainsForNonAvailableReceiptsForcingPagination(t *testing.T) {
	testGapsDomainsForNonAvailableReceipts(t, 1)
}

func testGapsDomainsForNonAvailableReceipts(t *testing.T, pageSize int) {
	txID1 := uuid.New()
	txID2 := uuid.New()
	txID3 := uuid.New()
	txID4 := uuid.New()
	txID5 := uuid.New()
	txID6 := uuid.New()
	missingStateID1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	missingStateID2 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	ctx, txm, done := newTestTransactionManager(t, true,
		mockDomainStateCompletion,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			// Mock TX2 being unavailable when first attempted, so it will block TX3
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID2).
				Return(&pldapi.TransactionStates{
					Unavailable: &pldapi.UnavailableStates{
						Confirmed: []pldtypes.HexBytes{missingStateID1},
					},
				}, nil).
				Once()
			// Mock TX3 being unavailable when first attempted, so it will block TX5
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID3).
				Return(&pldapi.TransactionStates{
					Unavailable: &pldapi.UnavailableStates{
						Spent: []pldtypes.HexBytes{missingStateID2},
					},
				}, nil).
				Once()
			// Other calls return ok for when we unblock
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, mock.Anything).Return(&pldapi.TransactionStates{}, nil)
		},
	)
	defer done()

	txm.receiptsReadPageSize = pageSize

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
		Filters: pldapi.TransactionReceiptFilters{
			Type:   confutil.P(pldapi.TransactionTypePrivate.Enum()),
			Domain: "domain1",
		},
		Options: pldapi.TransactionReceiptListenerOptions{
			IncompleteStateReceiptBehavior: pldapi.IncompleteStateReceiptBehaviorBlockContract.Enum(),
		},
	})
	require.NoError(t, err)

	r1 := newTestReceiptReceiver(nil)
	close1, err := txm.AddReceiptReceiver(ctx, "listener1", r1)
	require.NoError(t, err)
	defer close1.Close()

	contract1 := pldtypes.RandAddress()
	contract2 := pldtypes.RandAddress()
	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				ReceiptType:   components.RT_Success,
				Domain:        "domain1",
				TransactionID: txID1,
				OnChain:       randOnChain(contract1),
			},
			{
				ReceiptType:   components.RT_Success,
				Domain:        "domain1",
				TransactionID: txID2,
				OnChain:       randOnChain(contract1),
			},
			{
				ReceiptType:   components.RT_Success,
				Domain:        "domain1",
				TransactionID: txID3,
				OnChain:       randOnChain(contract1),
			},
			{
				ReceiptType:   components.RT_Success,
				Domain:        "domain1",
				TransactionID: txID4,
				OnChain:       randOnChain(contract2),
			},
			{
				ReceiptType:   components.RT_Success,
				Domain:        "domain1",
				TransactionID: txID5,
				OnChain:       randOnChain(contract1),
			},
		})
	})
	require.NoError(t, err)

	// We get the first one, before the block
	require.Equal(t, txID1, (<-r1.receipts).ID)
	// .. then we skip to the 4th one, for a different contract address
	require.Equal(t, txID4, (<-r1.receipts).ID)

	// We can get new batches on the unblocked contracts
	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				ReceiptType:   components.RT_Success,
				Domain:        "domain1",
				TransactionID: txID6,
				OnChain:       randOnChain(contract2),
			},
		})
	})
	require.NoError(t, err)
	require.Equal(t, txID6, (<-r1.receipts).ID)

	// Write the state that's missing
	err = txm.p.DB().WithContext(ctx).Exec("INSERT INTO states ( id, created, domain_name, contract_address ) VALUES ( ?, ?, ?, ? )",
		missingStateID1, pldtypes.TimestampNow(), "domain1", contract1,
	).Error
	require.NoError(t, err)

	// Trigger a poll
	txm.NotifyStatesDBChanged(ctx)

	// .. now TX2 is unblocked
	require.Equal(t, txID2, (<-r1.receipts).ID)

	// Write the second state that's missing
	err = txm.p.DB().WithContext(ctx).Exec("INSERT INTO states ( id, created, domain_name, contract_address ) VALUES ( ?, ?, ?, ? )",
		missingStateID2, pldtypes.TimestampNow(), "domain1", contract1,
	).Error
	require.NoError(t, err)

	// Trigger a poll
	txm.NotifyStatesDBChanged(ctx)

	// .. and TX3 is unblocked
	require.Equal(t, txID3, (<-r1.receipts).ID)
	// .. and TX5 is unblocked immediately
	require.Equal(t, txID5, (<-r1.receipts).ID)

}

func TestLoadListenersFailRead(t *testing.T) {
	_, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*receipt_listeners").WillReturnRows(mc.db.NewRows([]string{}))
		// 2nd load fails
		mc.db.ExpectQuery("SELECT.*receipt_listeners").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	txm.receiptsInit()

	err := txm.loadReceiptListeners()
	require.Regexp(t, "pop", err)
}

func TestLoadListenersFailBadListener(t *testing.T) {
	_, txm, done := newTestTransactionManager(t, false, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*receipt_listeners").WillReturnRows(mc.db.NewRows([]string{}))
		// 2nd load gives bad data
		mc.db.ExpectQuery("SELECT.*receipt_listeners").WillReturnRows(mc.db.NewRows([]string{
			"name", "filters", "options",
		}).AddRow(
			"" /* bad name */, "{}", "{}",
		))
	})
	defer done()

	txm.receiptsInit()

	err := txm.loadReceiptListeners()
	require.Regexp(t, "PD020005", err)
}

func TestCreateBadListener(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "badly-behaved",
		Options: pldapi.TransactionReceiptListenerOptions{
			IncompleteStateReceiptBehavior: pldtypes.Enum[pldapi.IncompleteStateReceiptBehavior]("misbehave"),
		},
	})
	require.Regexp(t, "PD020003", err)
}

func TestCreateListenerFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnError(fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "test1",
	})
	require.Regexp(t, "pop", err)
}

func TestAddReceiptReceiverNotFound(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	_, err := txm.AddReceiptReceiver(ctx, "test1", newTestReceiptReceiver(nil))
	require.Regexp(t, "PD012238.*test1", err)
}

func TestStopReceiptListenerNotFound(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	err := txm.StopReceiptListener(ctx, "test1")
	require.Regexp(t, "PD012238.*test1", err)
}

func TestStartReceiptListenerNotFound(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	err := txm.StartReceiptListener(ctx, "test1")
	require.Regexp(t, "PD012238.*test1", err)
}

func TestStartReceiptListenerFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("UPDATE.*receipt_listeners").WillReturnError(fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "test1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	err = txm.StartReceiptListener(ctx, "test1")
	require.Regexp(t, "pop", err)
}

func TestDeleteReceiptListenerNotFound(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	err := txm.DeleteReceiptListener(ctx, "test1")
	require.Regexp(t, "PD012238.*test1", err)
}

func TestDeleteReceiptListenerFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("DELETE.*receipt_listeners").WillReturnError(fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "test1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	_, err = txm.loadReceiptListener(ctx, &persistedReceiptListener{Name: "test1", Filters: pldtypes.RawJSON(`{}`), Options: pldtypes.RawJSON(`{}`)})
	assert.Regexp(t, "PD012235", err)

	err = txm.DeleteReceiptListener(ctx, "test1")
	require.Regexp(t, "pop", err)
}

func TestBuildListenerDBQueryFailBadTypeDomainFilterCombos(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockEmptyReceiptListeners)
	defer done()

	_, err := txm.buildListenerDBQuery(ctx, &pldapi.TransactionReceiptListener{
		Filters: pldapi.TransactionReceiptFilters{
			Domain: "not-private-filtered",
		},
	}, txm.p.DB())
	require.Regexp(t, "PD012236", err)

	_, err = txm.buildListenerDBQuery(ctx, &pldapi.TransactionReceiptListener{
		Filters: pldapi.TransactionReceiptFilters{
			Type:   confutil.P(pldapi.TransactionTypePublic.Enum()),
			Domain: "not-private",
		},
	}, txm.p.DB())
	require.Regexp(t, "PD012236", err)

	_, err = txm.buildListenerDBQuery(ctx, &pldapi.TransactionReceiptListener{
		Filters: pldapi.TransactionReceiptFilters{
			Type: confutil.P(pldapi.TransactionType("badness").Enum()),
		},
	}, txm.p.DB())
	require.Regexp(t, "PD012236", err)
}

func TestCheckMatchBadTypeFalse(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	l := txm.receiptListeners["listener1"]
	l.spec.Filters.Type = confutil.P(pldapi.TransactionType("wrong").Enum())

	require.False(t, l.checkMatch(&transactionReceipt{}))

}

func TestCreateListenerBadOptions(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	_, err := txm.loadReceiptListener(ctx, &persistedReceiptListener{
		Filters: pldtypes.RawJSON(`{ !badness`),
	})
	assert.Regexp(t, "PD012233", err)

	_, err = txm.loadReceiptListener(ctx, &persistedReceiptListener{
		Filters: pldtypes.RawJSON(`{}`),
		Options: pldtypes.RawJSON(`{ !badness`),
	})
	assert.Regexp(t, "PD012234", err)

}

func TestAddReceiverNoBlock(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	r1, err := txm.AddReceiptReceiver(ctx, "listener1", newTestReceiptReceiver(nil))
	require.NoError(t, err)
	defer r1.Close()

	r2, err := txm.AddReceiptReceiver(ctx, "listener1", newTestReceiptReceiver(nil))
	require.NoError(t, err)
	defer r2.Close()
}

func TestNotifyNewReceiptsNoBlock(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	txm.receiptListeners["listener1"].notifyNewReceipts()
	txm.receiptListeners["listener1"].notifyNewReceipts()
}

func TestClosedRetryingLoadingCheckpoint(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("SELECT.*receipt_listener_checkpoints").WillReturnError(fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	txm.receiptsRetry.UTSetMaxAttempts(1)
	l := txm.receiptListeners["listener1"]
	l.initStart()
	l.runListener()
}

func mockPublicReceipts(count int) func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	return func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.MatchExpectationsInOrder(false)
		rows := sqlmock.
			NewRows([]string{
				"transaction",
				"sequence",
				"tx_hash",
			})
		for i := 0; i < count; i++ {
			rows = rows.AddRow(
				uuid.NewString(),
				int64(1000),
				pldtypes.RandHex(32),
			)
		}
		mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(rows)
	}
}

func mockPrivateReceipt(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	mc.db.MatchExpectationsInOrder(false)
	mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(sqlmock.
		NewRows([]string{
			"transaction",
			"sequence",
			"tx_hash",
			"domain",
		}).
		AddRow(
			uuid.NewString(),
			int64(1000),
			pldtypes.RandHex(32),
			"domain1",
		))
}

func TestClosedRetryingBatchDeliver(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockNoGaps,
		mockPublicReceipts(1),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("SELECT.*receipt_listener_checkpoints").WillReturnRows(sqlmock.NewRows([]string{}))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	trr := newTestReceiptReceiver(fmt.Errorf("pop"))
	r, err := txm.AddReceiptReceiver(ctx, "listener1", trr)
	require.NoError(t, err)
	defer r.Close()

	txm.receiptsRetry.UTSetMaxAttempts(1)
	l := txm.receiptListeners["listener1"]
	l.initStart()
	l.runListener()
	<-trr.called
}

func TestClosedRetryingWritingCheckpoint(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockNoGaps,
		mockPublicReceipts(1),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("SELECT.*receipt_listener_checkpoints").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectExec("INSERT.*receipt_listener_checkpoints").WillReturnError(fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	trr := newTestReceiptReceiver(nil)
	r, err := txm.AddReceiptReceiver(ctx, "listener1", trr)
	require.NoError(t, err)
	defer r.Close()

	txm.receiptsRetry.UTSetMaxAttempts(1)
	l := txm.receiptListeners["listener1"]
	l.initStart()
	l.runListener()
	<-trr.receipts
}

func TestClosedRetryingQueryReceiptStates(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockNoGaps,
		mockPrivateReceipt,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("SELECT.*receipt_listener_checkpoints").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	txm.receiptsRetry.UTSetMaxAttempts(1)
	l := txm.receiptListeners["listener1"]
	l.initStart()
	l.runListener()
}

func TestClosedRetryingQueryReceipts(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockNoGaps,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("SELECT.*receipt_listener_checkpoints").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnError(fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	txm.receiptsRetry.UTSetMaxAttempts(1)
	l := txm.receiptListeners["listener1"]
	l.initStart()
	l.runListener()
}

func TestDeliverBatchCancelledCtxNoReceiver(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	l := txm.receiptListeners["listener1"]
	l.ctx, l.cancelCtx = context.WithCancel(ctx)
	l.cancelCtx()
	err = l.deliverBatch(&receiptDeliveryBatch{})
	require.Regexp(t, "PD010301", err)
}

func TestDeliverBatchCancelledCtxNotifyReceiver(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	l := txm.receiptListeners["listener1"]
	l.initStart()

	ready := make(chan struct{})

	go func() {
		<-ready
		receipts := newTestReceiptReceiver(nil)
		closeReceiver, err := txm.AddReceiptReceiver(ctx, "listener1", receipts)
		require.NoError(t, err)
		t.Cleanup(func() { closeReceiver.Close() })
	}()

	close(ready)
	r, err := l.nextReceiver(&receiptDeliveryBatch{})
	require.NoError(t, err)
	require.NotNil(t, r)
	close(l.done)

}

func TestProcessPersistedReceiptPostFilter(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
		Filters: pldapi.TransactionReceiptFilters{
			Type:   confutil.P(pldapi.TransactionTypePrivate.Enum()),
			Domain: "domain1",
		},
	})
	require.NoError(t, err)

	l := txm.receiptListeners["listener1"]
	l.initStart()

	receipt := &transactionReceipt{
		Domain: "domain2",
	}
	batchCtx := l.newReceiptBatchContext()
	err = l.processPersistedReceipt(&receiptDeliveryBatch{}, receipt, batchCtx)
	require.NoError(t, err)
	close(l.done)

}

func mockGap(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	mc.db.MatchExpectationsInOrder(false)
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	mc.db.ExpectQuery("SELECT.*receipt_listener_gap").WillReturnRows(sqlmock.NewRows([]string{
		"listener", "source", "transaction", "sequence", "domain_name", "state",
	}).AddRow(
		"listener1", contractAddr, txID, 12345, "domain1", stateID,
	))
}

func TestProcessStaleGapFailRetryingReadGapPage(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.MatchExpectationsInOrder(false)
			mc.db.ExpectQuery("SELECT.*receipt_listener_gap").WillReturnError(fmt.Errorf("pop"))
		},
	)
	defer done()

	txm.receiptsRetry.UTSetMaxAttempts(1)

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	l := txm.receiptListeners["listener1"]
	l.initStart()

	err = l.processStaleGaps()
	assert.Regexp(t, "pop", err)
	close(l.done)

}

func TestProcessStaleGapFailRetryingReadPage(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockGap,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnError(fmt.Errorf("pop"))
		},
	)
	defer done()

	txm.receiptsRetry.UTSetMaxAttempts(1)

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	l := txm.receiptListeners["listener1"]
	l.initStart()

	err = l.processStaleGaps()
	assert.Regexp(t, "pop", err)
	close(l.done)

}

func TestProcessStaleGapFailRetryingProcessPage(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockGap,
		mockPrivateReceipt,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, mock.Anything).
				Return(nil, fmt.Errorf("pop"))
		},
	)
	defer done()

	txm.receiptsRetry.UTSetMaxAttempts(1)

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	l := txm.receiptListeners["listener1"]
	l.initStart()

	err = l.processStaleGaps()
	assert.Regexp(t, "pop", err)
	close(l.done)

}

func TestProcessStaleGapFailRetryingUpdateGapForPage(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockGap,
		mockPublicReceipts(1),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("UPDATE.*receipt_listener_gap").WillReturnError(fmt.Errorf("pop"))
		},
	)
	defer done()

	txm.receiptsReadPageSize = 1
	txm.receiptsRetry.UTSetMaxAttempts(1)

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	receipts := newTestReceiptReceiver(nil)
	closeReceiver, err := txm.AddReceiptReceiver(ctx, "listener1", receipts)
	require.NoError(t, err)
	defer closeReceiver.Close()

	l := txm.receiptListeners["listener1"]
	l.initStart()

	err = l.processStaleGaps()
	assert.Regexp(t, "pop", err)
	close(l.done)

}

func TestE2EReceiptListenerCompleteOnly(t *testing.T) {
	privateTransactionIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	missingStateIDs := []pldtypes.HexBytes{
		pldtypes.HexBytes(pldtypes.RandBytes(32)),
		pldtypes.HexBytes(pldtypes.RandBytes(32)),
		pldtypes.HexBytes(pldtypes.RandBytes(32)),
	}

	ctx, txm, done := newTestTransactionManager(t, true,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			// Mock state manager to return incomplete states initially for all private transactions
			for i, txID := range privateTransactionIDs {
				mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID).
					Return(&pldapi.TransactionStates{
						Unavailable: &pldapi.UnavailableStates{
							Confirmed: []pldtypes.HexBytes{missingStateIDs[i]},
						},
					}, nil).
					Once()
			}
			// Subsequent calls return complete states for all
			for _, txID := range privateTransactionIDs {
				mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID).
					Return(&pldapi.TransactionStates{
						Confirmed: []*pldapi.StateBase{},
					}, nil)
			}
		},
		mockDomainStateCompletionAndReceipt,
	)
	defer done()

	// Create listener with complete_only behavior
	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "complete_only_listener",
		Options: pldapi.TransactionReceiptListenerOptions{
			IncompleteStateReceiptBehavior: pldapi.IncompleteStateReceiptBehaviorCompleteOnly.Enum(),
			DomainReceipts:                 true,
		},
	})
	require.NoError(t, err)

	// Create a receiver and attach it to the listener
	receipts := newTestReceiptReceiver(nil)
	closeReceiver, err := txm.AddReceiptReceiver(ctx, "complete_only_listener", receipts)
	require.NoError(t, err)
	defer closeReceiver.Close()
	err = txm.StartReceiptListener(ctx, "complete_only_listener")
	require.NoError(t, err)

	// Write multiple receipts - one public (delivered immediately) and three private (should wait for completion)
	contractAddr1 := pldtypes.RandAddress()
	receiptInputs := []*components.ReceiptInput{
		{
			ReceiptType:   components.RT_Success,
			Domain:        "", // public transaction - should be delivered immediately
			TransactionID: uuid.New(),
			OnChain: pldtypes.OnChainLocation{
				Type:             pldtypes.OnChainTransaction,
				TransactionHash:  pldtypes.Bytes32(pldtypes.RandBytes(32)),
				BlockNumber:      12345,
				TransactionIndex: 10,
				Source:           contractAddr1,
			},
		},
	}
	for i, txID := range privateTransactionIDs {
		receiptInputs = append(receiptInputs, &components.ReceiptInput{
			ReceiptType:   components.RT_Success,
			Domain:        "domain1", // private transaction - should wait for completion
			TransactionID: txID,
			OnChain: pldtypes.OnChainLocation{
				Type:             pldtypes.OnChainTransaction,
				TransactionHash:  pldtypes.Bytes32(pldtypes.RandBytes(32)),
				BlockNumber:      12346 + int64(i),
				TransactionIndex: 11 + int64(i),
				Source:           contractAddr1,
			},
		})
	}

	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, receiptInputs)
	})
	require.NoError(t, err)

	// Should receive the public receipt immediately
	r := <-receipts.receipts
	assert.Equal(t, receiptInputs[0].TransactionID, r.ID)
	assert.Equal(t, "", r.Domain) // public transaction

	// The private receipts should be queued as incomplete and not delivered yet
	select {
	case <-receipts.receipts:
		t.Fatal("Should not have received any private receipts yet")
	case <-time.After(100 * time.Millisecond):
		// Expected - no more receipts delivered
	}

	// Simulate state update notification to trigger reprocessing of incomplete receipts
	// This should deliver all three private receipts in a single batched operation
	txm.NotifyStatesDBChanged(ctx)

	receivedTxIDs := make(map[uuid.UUID]bool)
	for range 3 {
		r = <-receipts.receipts
		assert.Equal(t, "domain1", r.Domain) // private transaction
		receivedTxIDs[r.ID] = true
	}

	// Verify we received all expected transaction IDs
	for _, expectedTxID := range privateTransactionIDs {
		assert.True(t, receivedTxIDs[expectedTxID], "Expected to receive transaction %s", expectedTxID)
	}
}

func TestE2EReceiptListenerProcess(t *testing.T) {
	privateTransactionID := uuid.New()
	missingStateID := pldtypes.HexBytes(pldtypes.RandBytes(32))

	ctx, txm, done := newTestTransactionManager(t, true,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			// Mock state manager to return incomplete states for the private transaction
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, privateTransactionID).
				Return(&pldapi.TransactionStates{
					Unavailable: &pldapi.UnavailableStates{
						Confirmed: []pldtypes.HexBytes{missingStateID},
					},
				}, nil)
		},
		mockDomainStateCompletion,
	)
	defer done()

	// Create listener with process behavior (delivers receipts even if states are incomplete)
	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "process_listener",
		Options: pldapi.TransactionReceiptListenerOptions{
			IncompleteStateReceiptBehavior: pldapi.IncompleteStateReceiptBehaviorProcess.Enum(),
			DomainReceipts:                 false,
		},
	})
	require.NoError(t, err)

	// Create a receiver and attach it to the listener
	receipts := newTestReceiptReceiver(nil)
	closeReceiver, err := txm.AddReceiptReceiver(ctx, "process_listener", receipts)
	require.NoError(t, err)
	defer closeReceiver.Close()
	err = txm.StartReceiptListener(ctx, "process_listener")
	require.NoError(t, err)

	// Write receipts - one public and one private with incomplete states
	contractAddr1 := pldtypes.RandAddress()
	receiptInputs := []*components.ReceiptInput{
		{
			ReceiptType:   components.RT_Success,
			Domain:        "", // public transaction - should be delivered immediately
			TransactionID: uuid.New(),
			OnChain: pldtypes.OnChainLocation{
				Type:             pldtypes.OnChainTransaction,
				TransactionHash:  pldtypes.Bytes32(pldtypes.RandBytes(32)),
				BlockNumber:      12345,
				TransactionIndex: 10,
				Source:           contractAddr1,
			},
		},
		{
			ReceiptType:   components.RT_Success,
			Domain:        "domain1", // private transaction with incomplete states
			TransactionID: privateTransactionID,
			OnChain: pldtypes.OnChainLocation{
				Type:             pldtypes.OnChainTransaction,
				TransactionHash:  pldtypes.Bytes32(pldtypes.RandBytes(32)),
				BlockNumber:      12346,
				TransactionIndex: 11,
				Source:           contractAddr1,
			},
		},
	}

	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, receiptInputs)
	})
	require.NoError(t, err)

	// Should receive both receipts immediately, even though the private one has incomplete states
	receivedTxIDs := make(map[uuid.UUID]bool)
	for range 2 {
		r := <-receipts.receipts
		receivedTxIDs[r.ID] = true
	}

	// Verify we received both expected transaction IDs
	assert.True(t, receivedTxIDs[receiptInputs[0].TransactionID], "Expected to receive public transaction")
	assert.True(t, receivedTxIDs[receiptInputs[1].TransactionID], "Expected to receive private transaction with incomplete states")
}

func TestProcessIncompleteReceiptsFailRetryingFetchReceipts(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("SELECT.*receipt_listener_incomplete").WillReturnRows(sqlmock.NewRows([]string{
				"listener", "transaction", "sequence", "domain_name", "created",
			}).AddRow(
				"listener1", uuid.NewString(), 1000, "domain1", pldtypes.TimestampNow(),
			))
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnError(fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
		Options: pldapi.TransactionReceiptListenerOptions{
			IncompleteStateReceiptBehavior: pldapi.IncompleteStateReceiptBehaviorCompleteOnly.Enum(),
		},
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	txm.receiptsRetry.UTSetMaxAttempts(1)
	l := txm.receiptListeners["listener1"]
	l.initStart()

	err = l.processIncompleteReceipts()
	assert.Regexp(t, "pop", err)
	close(l.done)
}

func TestProcessIncompleteReceiptsFailRetryingProcessReceipt(t *testing.T) {
	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("SELECT.*receipt_listener_incomplete").WillReturnRows(sqlmock.NewRows([]string{
				"listener", "transaction", "sequence", "domain_name", "created",
			}).AddRow(
				"listener1", txID.String(), 1000, "domain1", pldtypes.TimestampNow(),
			))
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{
				"transaction", "sequence", "domain",
			}).AddRow(
				txID.String(), 1000, "domain1",
			))
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID).
				Return(nil, fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
		Options: pldapi.TransactionReceiptListenerOptions{
			IncompleteStateReceiptBehavior: pldapi.IncompleteStateReceiptBehaviorCompleteOnly.Enum(),
		},
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	txm.receiptsRetry.UTSetMaxAttempts(1)
	l := txm.receiptListeners["listener1"]
	l.initStart()

	err = l.processIncompleteReceipts()
	assert.Regexp(t, "pop", err)
	close(l.done)
}

func TestProcessIncompleteReceiptsFailRetryingDeliverBatch(t *testing.T) {
	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("SELECT.*receipt_listener_incomplete").WillReturnRows(sqlmock.NewRows([]string{
				"listener", "transaction", "sequence", "domain_name", "created",
			}).AddRow(
				"listener1", txID.String(), 1000, "", pldtypes.TimestampNow(),
			))
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{
				"transaction", "sequence", "domain",
			}).AddRow(
				txID.String(), 1000, "",
			))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
		Options: pldapi.TransactionReceiptListenerOptions{
			IncompleteStateReceiptBehavior: pldapi.IncompleteStateReceiptBehaviorCompleteOnly.Enum(),
		},
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	trr := newTestReceiptReceiver(fmt.Errorf("pop"))
	r, err := txm.AddReceiptReceiver(ctx, "listener1", trr)
	require.NoError(t, err)
	defer r.Close()

	txm.receiptsRetry.UTSetMaxAttempts(1)
	l := txm.receiptListeners["listener1"]
	l.initStart()

	err = l.processIncompleteReceipts()
	assert.Regexp(t, "pop", err)
	close(l.done)
}

func TestProcessIncompleteReceiptsFailRetryingCleanup(t *testing.T) {
	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("SELECT.*receipt_listener_incomplete").WillReturnRows(sqlmock.NewRows([]string{
				"listener", "transaction", "sequence", "domain_name", "created",
			}).AddRow(
				"listener1", txID.String(), 1000, "", pldtypes.TimestampNow(),
			))
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{
				"transaction", "sequence", "domain",
			}).AddRow(
				txID.String(), 1000, "",
			))
			mc.db.ExpectExec("DELETE.*receipt_listener_incomplete").WillReturnError(fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
		Options: pldapi.TransactionReceiptListenerOptions{
			IncompleteStateReceiptBehavior: pldapi.IncompleteStateReceiptBehaviorCompleteOnly.Enum(),
		},
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	trr := newTestReceiptReceiver(nil)
	r, err := txm.AddReceiptReceiver(ctx, "listener1", trr)
	require.NoError(t, err)
	defer r.Close()

	txm.receiptsRetry.UTSetMaxAttempts(1)
	l := txm.receiptListeners["listener1"]
	l.initStart()

	err = l.processIncompleteReceipts()
	assert.Regexp(t, "pop", err)
	close(l.done)
}

func TestProcessIncompleteReceiptsFailRetryingQueryIncomplete(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("SELECT.*receipt_listener_incomplete").WillReturnError(fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
		Options: pldapi.TransactionReceiptListenerOptions{
			IncompleteStateReceiptBehavior: pldapi.IncompleteStateReceiptBehaviorCompleteOnly.Enum(),
		},
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	txm.receiptsRetry.UTSetMaxAttempts(1)
	l := txm.receiptListeners["listener1"]
	l.initStart()

	err = l.processIncompleteReceipts()
	assert.Regexp(t, "pop", err)
	close(l.done)
}

func TestProcessPersistedReceiptFailDomainRetrieval(t *testing.T) {
	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID).
				Return(&pldapi.TransactionStates{}, nil)
			mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").
				Return(nil, fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Options: pldapi.TransactionReceiptListenerOptions{},
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	l := txm.receiptListeners["listener1"]
	l.initStart()

	receipt := &transactionReceipt{
		Domain:           "domain1",
		TransactionID:    txID,
		Source:           pldtypes.RandAddress(),
		TransactionHash:  (*pldtypes.Bytes32)(pldtypes.RandBytes(32)),
		BlockNumber:      confutil.P(int64(12345)),
		TransactionIndex: confutil.P(int64(10)),
		LogIndex:         confutil.P(int64(5)),
	}

	batchCtx := l.newReceiptBatchContext()
	err = l.processPersistedReceipt(&receiptDeliveryBatch{}, receipt, batchCtx)
	assert.Regexp(t, "pop", err)
	close(l.done)
}

func TestProcessPersistedReceiptFailStateCompletionCheck(t *testing.T) {
	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			md := componentsmocks.NewDomain(mc.t)
			mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").
				Return(md, nil)
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID).
				Return(&pldapi.TransactionStates{}, nil)
			md.On("CheckStateCompletion", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(false, fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Options: pldapi.TransactionReceiptListenerOptions{},
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	l := txm.receiptListeners["listener1"]
	l.initStart()

	receipt := &transactionReceipt{
		Domain:           "domain1",
		TransactionID:    txID,
		Source:           pldtypes.RandAddress(),
		TransactionHash:  (*pldtypes.Bytes32)(pldtypes.RandBytes(32)),
		BlockNumber:      confutil.P(int64(12345)),
		TransactionIndex: confutil.P(int64(10)),
		LogIndex:         confutil.P(int64(5)),
	}

	batchCtx := l.newReceiptBatchContext()
	err = l.processPersistedReceipt(&receiptDeliveryBatch{}, receipt, batchCtx)
	assert.Regexp(t, "pop", err)
	close(l.done)
}

func TestProcessIncompleteReceiptsOrphanedReceipt(t *testing.T) {
	orphanedTxID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			// First call returns one orphaned receipt
			mc.db.ExpectQuery("SELECT.*receipt_listener_incomplete").WillReturnRows(sqlmock.NewRows([]string{
				"listener", "transaction", "sequence", "domain_name", "created",
			}).AddRow(
				"listener1", orphanedTxID.String(), 1000, "domain1", pldtypes.TimestampNow(),
			))
			// Return empty result set for receipts query - simulating orphaned receipt
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{
				"transaction", "sequence", "domain",
			}))
			mc.db.ExpectExec("DELETE.*receipt_listener_incomplete").WillReturnResult(driver.ResultNoRows)
			// Second call returns no more incomplete receipts (loop termination)
			mc.db.ExpectQuery("SELECT.*receipt_listener_incomplete").WillReturnRows(sqlmock.NewRows([]string{
				"listener", "transaction", "sequence", "domain_name", "created",
			}))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
		Options: pldapi.TransactionReceiptListenerOptions{
			IncompleteStateReceiptBehavior: pldapi.IncompleteStateReceiptBehaviorCompleteOnly.Enum(),
		},
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	l := txm.receiptListeners["listener1"]
	l.initStart()

	// This should succeed despite the orphaned receipt - it gets cleaned up
	err = l.processIncompleteReceipts()
	require.NoError(t, err)
	close(l.done)
}

func TestRunListenerFailsOnIncompleteReceipts(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("SELECT.*receipt_listener_checkpoints").WillReturnRows(sqlmock.NewRows([]string{}))
			mc.db.ExpectQuery("SELECT.*receipt_listener_gap").WillReturnRows(sqlmock.NewRows([]string{}))
			// First call to processIncompleteReceipts fails
			mc.db.ExpectQuery("SELECT.*receipt_listener_incomplete").WillReturnError(fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
		Options: pldapi.TransactionReceiptListenerOptions{
			IncompleteStateReceiptBehavior: pldapi.IncompleteStateReceiptBehaviorCompleteOnly.Enum(),
		},
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	txm.receiptsRetry.UTSetMaxAttempts(1)
	l := txm.receiptListeners["listener1"]
	l.initStart()
	l.runListener()
}

func TestRunListenerFailsOnStaleGaps(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("SELECT.*receipt_listener_checkpoints").WillReturnRows(sqlmock.NewRows([]string{}))
			// First call to processStaleGaps fails
			mc.db.ExpectQuery("SELECT.*receipt_listener_gap").WillReturnError(fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	txm.receiptsRetry.UTSetMaxAttempts(1)
	l := txm.receiptListeners["listener1"]
	l.initStart()
	l.runListener()
}

func TestBuildFullReceiptGetDomainError(t *testing.T) {
	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*receipt_listeners").WillReturnResult(driver.ResultNoRows)
			mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, txID).Return(&pldapi.TransactionStates{}, nil)
			mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").
				Return(nil, fmt.Errorf("pop"))
		},
	)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name:    "listener1",
		Options: pldapi.TransactionReceiptListenerOptions{},
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	l := txm.receiptListeners["listener1"]
	l.initStart()

	receipt := &pldapi.TransactionReceipt{
		ID: txID,
		TransactionReceiptData: pldapi.TransactionReceiptData{
			Domain: "domain1",
		},
	}

	batchCtx := l.newReceiptBatchContext()
	_, err = batchCtx.buildFullReceipt(receipt, true)
	assert.Regexp(t, "pop", err)

	close(l.done)
}
