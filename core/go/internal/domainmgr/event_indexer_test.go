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

package domainmgr

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/persistence/mockpersistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSolidityEventSignatures(t *testing.T) {
	// We don't expect this to change without our knowledge.
	// We tolerate it changing between versions of firefly-signer (used only in memory), but it's important we understand why if it does.
	//
	// We don't store it as a constant because we're reliant on us and blockindexer calculating it identically (we use the same lib).
	//
	// The standard solidity signature is insufficient, as it doesn't include variable names, or the indexed-ness of fields
	assert.Equal(t, "event PaladinRegisterSmartContract_V0(bytes32 indexed txId, address indexed instance, bytes config)", eventSolSig_PaladinRegisterSmartContract_V0)
}

func TestEventIndexingWithDB(t *testing.T) {

	ctx, dm, tp, done := newTestDomain(t, true /* real DB */, goodDomainConf())
	defer done()

	deployTX := uuid.New()
	contractAddr := tktypes.EthAddress(tktypes.RandBytes(20))

	txNotified := make(chan struct{})
	go func() {
		defer close(txNotified)
		sc, err := dm.WaitForDeploy(ctx, deployTX)
		require.NoError(t, err)
		assert.Equal(t, contractAddr, sc.Address())
	}()

	// Index an event indicating deployment of a new smart contract instance
	var batchTxs []*components.ReceiptInput
	var unprocessedEvents []*blockindexer.EventWithData
	err := dm.persistence.DB().Transaction(func(tx *gorm.DB) (err error) {
		unprocessedEvents, batchTxs, err = dm.registrationIndexer(ctx, tx, &blockindexer.EventDeliveryBatch{
			StreamID:   uuid.New(),
			StreamName: "name_given_by_component_mgr",
			BatchID:    uuid.New(),
			Events: []*blockindexer.EventWithData{
				{
					SoliditySignature: eventSolSig_PaladinRegisterSmartContract_V0,
					Address:           (tktypes.EthAddress)(*tp.d.RegistryAddress()),
					IndexedEvent: &blockindexer.IndexedEvent{
						BlockNumber:      12345,
						TransactionIndex: 0,
						LogIndex:         0,
						TransactionHash:  tktypes.NewBytes32FromSlice(tktypes.RandBytes(32)),
						Signature:        eventSig_PaladinRegisterSmartContract_V0,
					},
					Data: tktypes.RawJSON(`{
						"txId": "` + tktypes.Bytes32UUIDFirst16(deployTX).String() + `",
						"instance": "` + contractAddr.String() + `",
						"config": "0xfeedbeef"
					}`),
				},
			},
		})
		return err
	})
	require.NoError(t, err)
	assert.Len(t, batchTxs, 1)
	assert.Empty(t, unprocessedEvents) // we consumed all the events there were

	// Lookup the instance against the domain
	psc, err := dm.GetSmartContractByAddress(ctx, contractAddr)
	require.NoError(t, err)
	dc := psc.(*domainContract)
	assert.Equal(t, &PrivateSmartContract{
		DeployTX:        deployTX,
		RegistryAddress: *tp.d.RegistryAddress(),
		Address:         contractAddr,
		ConfigBytes:     []byte{0xfe, 0xed, 0xbe, 0xef},
	}, dc.info)
	assert.Equal(t, contractAddr, psc.Address())
	assert.Equal(t, "test1", psc.Domain().Name())
	assert.Equal(t, "0xfeedbeef", psc.ConfigBytes().String())

	// Get cached
	psc2, err := dm.GetSmartContractByAddress(ctx, contractAddr)
	require.NoError(t, err)
	assert.Equal(t, psc, psc2)

	<-txNotified
}

func TestEventIndexingBadEvent(t *testing.T) {

	ctx, dm, _, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
		mc.stateStore.On("EnsureABISchemas", mock.Anything, "test1", mock.Anything).Return(nil, nil)
		mc.db.ExpectBegin()
		mc.db.ExpectCommit()
	})
	defer done()

	err := dm.persistence.DB().Transaction(func(tx *gorm.DB) error {
		_, _, err := dm.registrationIndexer(ctx, tx, &blockindexer.EventDeliveryBatch{
			StreamID:   uuid.New(),
			StreamName: "name_given_by_component_mgr",
			BatchID:    uuid.New(),
			Events: []*blockindexer.EventWithData{
				{
					SoliditySignature: eventSolSig_PaladinRegisterSmartContract_V0,
					Data: tktypes.RawJSON(`{
						 "config": "cannot parse this"
					 }`),
				},
			},
		})
		return err
	})
	require.NoError(t, err)

}

func TestEventIndexingInsertError(t *testing.T) {

	ctx, dm, tp, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
		mc.stateStore.On("EnsureABISchemas", mock.Anything, "test1", mock.Anything).Return(nil, nil)
		mc.db.ExpectBegin()
		mc.db.ExpectExec("INSERT").WillReturnError(fmt.Errorf("pop"))
		mc.db.ExpectRollback()
	})
	defer done()

	contractAddr := tktypes.EthAddress(tktypes.RandBytes(20))
	deployTX := uuid.New()
	err := dm.persistence.DB().Transaction(func(tx *gorm.DB) error {
		_, _, err := dm.registrationIndexer(ctx, tx, &blockindexer.EventDeliveryBatch{
			StreamID:   uuid.New(),
			StreamName: "name_given_by_component_mgr",
			BatchID:    uuid.New(),
			Events: []*blockindexer.EventWithData{
				{
					SoliditySignature: eventSolSig_PaladinRegisterSmartContract_V0,
					Address:           *tp.d.RegistryAddress(),
					IndexedEvent: &blockindexer.IndexedEvent{
						BlockNumber:      12345,
						TransactionIndex: 0,
						LogIndex:         0,
						TransactionHash:  tktypes.NewBytes32FromSlice(tktypes.RandBytes(32)),
						Signature:        eventSig_PaladinRegisterSmartContract_V0,
					},
					Data: tktypes.RawJSON(`{
						"txId": "` + tktypes.Bytes32UUIDFirst16(deployTX).String() + `",
						"domain": "` + contractAddr.String() + `",
						"data": "0xfeedbeef"
					}`),
				},
			},
		})
		return err
	})
	assert.Regexp(t, "pop", err)

}

func TestHandleEventBatch(t *testing.T) {
	batchID := uuid.New()
	txID := uuid.New()
	txIDBytes32 := tktypes.Bytes32UUIDFirst16(txID)
	contract1 := tktypes.RandAddress()
	contract2 := tktypes.RandAddress()
	stateSpent := tktypes.RandHex(32)
	stateConfirmed := tktypes.RandHex(32)
	fakeHash1 := tktypes.RandHex(32)
	event1 := &blockindexer.EventWithData{
		Address: *contract1,
		IndexedEvent: &blockindexer.IndexedEvent{
			BlockNumber:      1000,
			TransactionIndex: 20,
			LogIndex:         30,
			TransactionHash:  tktypes.MustParseBytes32(tktypes.RandHex(32)),
			Signature:        tktypes.MustParseBytes32(tktypes.RandHex(32)),
		},
		SoliditySignature: "some event signature 1",
		Data:              tktypes.RawJSON(`{"result": "success"}`),
	}
	event2 := &blockindexer.EventWithData{
		Address: *contract2,
		IndexedEvent: &blockindexer.IndexedEvent{
			BlockNumber:      2000,
			TransactionIndex: 30,
			LogIndex:         40,
			TransactionHash:  tktypes.MustParseBytes32(tktypes.RandHex(32)),
			Signature:        tktypes.MustParseBytes32(tktypes.RandHex(32)),
		},
		SoliditySignature: "some event signature 2",
		Data:              tktypes.RawJSON(`{"result": "success"}`),
	}

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.domainStateInterface.On("MarkStatesSpent", txID, []string{stateSpent}).Return(nil)
		mc.domainStateInterface.On("MarkStatesConfirmed", txID, []string{stateConfirmed}).Return(nil)
		mc.domainStateInterface.On("UpsertStates", &txID, mock.Anything).Return(nil, nil)
		mc.txManager.On("MatchAndFinalizeTransactions", mock.Anything, mock.Anything, mock.MatchedBy(func(receipts []*components.ReceiptInput) bool {
			// Note first contract is unrecognized, second is recognized
			require.Len(t, receipts, 1)
			r := receipts[0]
			expectedEvent := event2
			assert.Equal(t, txID, r.TransactionID)
			assert.Equal(t, tktypes.OnChainEvent, r.OnChain.Type)
			assert.Equal(t, expectedEvent.TransactionHash.String(), r.OnChain.TransactionHash.String())
			assert.Equal(t, expectedEvent.BlockNumber, r.OnChain.BlockNumber)
			assert.Equal(t, expectedEvent.TransactionIndex, r.OnChain.TransactionIndex)
			assert.Equal(t, expectedEvent.LogIndex, r.OnChain.LogIndex)
			return true
		})).Return([]uuid.UUID{txID}, nil)
	})
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	// First contract is unrecognized, second is recognized
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{},
	))
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract2, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		assert.Equal(t, batchID.String(), req.BatchId)
		assert.Equal(t, contract2.String(), req.ContractInfo.ContractAddress)
		assert.Equal(t, `{"result": "success"}`, req.Events[0].DataJson)
		return &prototk.HandleEventBatchResponse{
			TransactionsComplete: []*prototk.CompletedTransaction{
				{
					TransactionId: txIDBytes32.String(),
					Location:      req.Events[0].Location,
				},
			},
			SpentStates: []*prototk.StateUpdate{
				{
					Id:            stateSpent,
					TransactionId: txIDBytes32.String(),
				},
			},
			ConfirmedStates: []*prototk.StateUpdate{
				{
					Id:            stateConfirmed,
					TransactionId: txIDBytes32.String(),
				},
			},
			NewStates: []*prototk.NewLocalState{
				{
					Id:            &fakeHash1,
					StateDataJson: `{"color": "blue"}`,
					TransactionId: txIDBytes32.String(),
				},
			},
		}, nil
	}

	cb, err := d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events:  []*blockindexer.EventWithData{event1, event2},
	})
	assert.NoError(t, err)

	req := d.dm.privateTxWaiter.AddInflight(ctx, txID)
	cb()
	_, err = req.Wait()
	assert.NoError(t, err)
}

func TestHandleEventBatchFinalizeFail(t *testing.T) {
	batchID := uuid.New()

	ctx, dm, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.db.ExpectExec(`INSERT.*private_smart_contracts`).WillReturnResult(driver.ResultNoRows)

		mc.txManager.On("MatchAndFinalizeTransactions", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()
	d := tp.d

	_, err := d.handleEventBatch(ctx, dm.persistence.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				Address: *tktypes.RandAddress(),
				IndexedEvent: &blockindexer.IndexedEvent{
					BlockNumber:      1000,
					TransactionIndex: 20,
					LogIndex:         30,
					TransactionHash:  tktypes.MustParseBytes32(tktypes.RandHex(32)),
					Signature:        eventSig_PaladinRegisterSmartContract_V0,
				},
				SoliditySignature: eventSolSig_PaladinRegisterSmartContract_V0,
				Data:              tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.Regexp(t, "pop", err)

}

func TestHandleEventBatchContractLookupFail(t *testing.T) {
	batchID := uuid.New()
	contract1 := tktypes.RandAddress()

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnError(fmt.Errorf("pop"))

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				Address: *contract1,
				Data:    tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.EqualError(t, err, "pop")
}

func TestHandleEventBatchRegistrationError(t *testing.T) {
	batchID := uuid.New()

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectExec("INSERT.*private_smart_contracts").WillReturnError(fmt.Errorf("pop"))

	registrationData := &event_PaladinRegisterSmartContract_V0{
		TXId: tktypes.Bytes32(tktypes.RandBytes(32)),
	}
	registrationDataJSON, err := json.Marshal(registrationData)
	require.NoError(t, err)

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				IndexedEvent:      &blockindexer.IndexedEvent{},
				SoliditySignature: eventSolSig_PaladinRegisterSmartContract_V0,
				Data:              registrationDataJSON,
			},
		},
	})
	assert.EqualError(t, err, "pop")
}

func TestHandleEventBatchDomainError(t *testing.T) {
	batchID := uuid.New()
	contract1 := tktypes.RandAddress()

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				IndexedEvent: &blockindexer.IndexedEvent{},
				Address:      *contract1,
				Data:         tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.EqualError(t, err, "pop")
}

func TestHandleEventBatchSpentBadTransactionID(t *testing.T) {
	batchID := uuid.New()
	contract1 := tktypes.RandAddress()
	stateSpent := tktypes.RandHex(32)

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			SpentStates: []*prototk.StateUpdate{
				{
					Id:            stateSpent,
					TransactionId: "badnotgood",
				},
			},
		}, nil
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				IndexedEvent: &blockindexer.IndexedEvent{},
				Address:      *contract1,
				Data:         tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.ErrorContains(t, err, "PD020007")
}

func TestHandleEventBatchConfirmBadTransactionID(t *testing.T) {
	batchID := uuid.New()
	contract1 := tktypes.RandAddress()
	stateSpent := tktypes.RandHex(32)

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			ConfirmedStates: []*prototk.StateUpdate{
				{
					Id:            stateSpent,
					TransactionId: "badnotgood",
				},
			},
		}, nil
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				IndexedEvent: &blockindexer.IndexedEvent{},
				Address:      *contract1,
				Data:         tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.ErrorContains(t, err, "PD020007")
}

func TestHandleEventBatchNewBadTransactionID(t *testing.T) {
	batchID := uuid.New()
	contract1 := tktypes.RandAddress()

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			NewStates: []*prototk.NewLocalState{
				{
					TransactionId: "badnotgood",
				},
			},
		}, nil
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				IndexedEvent: &blockindexer.IndexedEvent{},
				Address:      *contract1,
				Data:         tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.ErrorContains(t, err, "PD020007")
}

func TestHandleEventBatchNewBadStateID(t *testing.T) {
	batchID := uuid.New()
	contract1 := tktypes.RandAddress()

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	stateID := "badnotgood"
	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			NewStates: []*prototk.NewLocalState{
				{
					TransactionId: tktypes.RandHex(32),
					Id:            &stateID,
				},
			},
		}, nil
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				IndexedEvent: &blockindexer.IndexedEvent{},
				Address:      *contract1,
				Data:         tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.ErrorContains(t, err, "PD020007")
}

func TestHandleEventBatchBadTransactionID(t *testing.T) {
	batchID := uuid.New()
	contract1 := tktypes.RandAddress()

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			TransactionsComplete: []*prototk.CompletedTransaction{
				{
					Location: &prototk.OnChainEventLocation{
						TransactionHash: "badnotgood",
					},
				},
			},
		}, nil
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				IndexedEvent: &blockindexer.IndexedEvent{},
				Address:      *contract1,
				Data:         tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.ErrorContains(t, err, "PD020008")
}

func TestHandleEventBatchMarkSpentFail(t *testing.T) {
	batchID := uuid.New()
	txID := uuid.New()
	txIDBytes32 := tktypes.Bytes32UUIDFirst16(txID)
	contract1 := tktypes.RandAddress()
	stateSpent := tktypes.RandHex(32)

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.domainStateInterface.On("MarkStatesSpent", txID, []string{stateSpent}).Return(fmt.Errorf("pop"))
	})
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			SpentStates: []*prototk.StateUpdate{
				{
					Id:            stateSpent,
					TransactionId: txIDBytes32.String(),
				},
			},
		}, nil
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				IndexedEvent: &blockindexer.IndexedEvent{},
				Address:      *contract1,
				Data:         tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.EqualError(t, err, "pop")
}

func TestHandleEventBatchMarkConfirmedFail(t *testing.T) {
	batchID := uuid.New()
	txID := uuid.New()
	txIDBytes32 := tktypes.Bytes32UUIDFirst16(txID)
	contract1 := tktypes.RandAddress()
	stateConfirmed := tktypes.RandHex(32)

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.domainStateInterface.On("MarkStatesConfirmed", txID, []string{stateConfirmed}).Return(fmt.Errorf("pop"))
	})
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			ConfirmedStates: []*prototk.StateUpdate{
				{
					Id:            stateConfirmed,
					TransactionId: txIDBytes32.String(),
				},
			},
		}, nil
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				IndexedEvent: &blockindexer.IndexedEvent{},
				Address:      *contract1,
				Data:         tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.EqualError(t, err, "pop")
}

func TestHandleEventBatchUpsertStateFail(t *testing.T) {
	batchID := uuid.New()
	txID := uuid.New()
	txIDBytes32 := tktypes.Bytes32UUIDFirst16(txID)
	contract1 := tktypes.RandAddress()

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.domainStateInterface.On("UpsertStates", &txID, mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			NewStates: []*prototk.NewLocalState{
				{
					StateDataJson: `{"color": "blue"}`,
					TransactionId: txIDBytes32.String(),
				},
			},
		}, nil
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				IndexedEvent: &blockindexer.IndexedEvent{},
				Address:      *contract1,
				Data:         tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.EqualError(t, err, "pop")
}

func TestReceiptSorting(t *testing.T) {
	// Note the detail of the sorting code is in tktypes.OnChainLocation
	receiptList := receiptsByOnChainOrder{
		{OnChain: tktypes.OnChainLocation{Type: tktypes.OnChainEvent, BlockNumber: 1100}},
		{OnChain: tktypes.OnChainLocation{ /* not onchain */ }},
		{OnChain: tktypes.OnChainLocation{Type: tktypes.OnChainEvent, BlockNumber: 1000}},
	}
	sort.Sort(receiptList)
	assert.Equal(t, receiptsByOnChainOrder{
		{OnChain: tktypes.OnChainLocation{Type: tktypes.NotOnChain}},
		{OnChain: tktypes.OnChainLocation{Type: tktypes.OnChainEvent, BlockNumber: 1000}},
		{OnChain: tktypes.OnChainLocation{Type: tktypes.OnChainEvent, BlockNumber: 1100}},
	}, receiptList)
}
