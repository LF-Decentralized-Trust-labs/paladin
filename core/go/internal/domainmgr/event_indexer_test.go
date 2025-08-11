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
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/blockindexer"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

	td, done := newTestDomain(t, true /* real DB */, goodDomainConf())
	defer done()
	ctx := td.ctx
	tp := td.tp
	dm := td.dm

	deployTX := uuid.New()
	contractAddr := pldtypes.EthAddress(pldtypes.RandBytes(20))

	// Index an event indicating deployment of a new smart contract instance
	var batchTxs txCompletionsOrdered
	var unprocessedEvents []*pldapi.EventWithData
	err := dm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		unprocessedEvents, batchTxs, err = dm.registrationIndexer(ctx, dbTX, &blockindexer.EventDeliveryBatch{
			StreamID:   uuid.New(),
			StreamName: "name_given_by_component_mgr",
			BatchID:    uuid.New(),
			Events: []*pldapi.EventWithData{
				{
					SoliditySignature: eventSolSig_PaladinRegisterSmartContract_V0,
					Address:           (pldtypes.EthAddress)(*tp.d.RegistryAddress()),
					IndexedEvent: &pldapi.IndexedEvent{
						BlockNumber:      12345,
						TransactionIndex: 0,
						LogIndex:         0,
						TransactionHash:  pldtypes.NewBytes32FromSlice(pldtypes.RandBytes(32)),
						Signature:        eventSig_PaladinRegisterSmartContract_V0,
					},
					Data: pldtypes.RawJSON(`{
						 "txId": "` + pldtypes.Bytes32UUIDFirst16(deployTX).String() + `",
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

	tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{
			Valid: true,
			ContractConfig: &prototk.ContractConfig{
				ContractConfigJson:   `{}`,
				CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
				SubmitterSelection:   prototk.ContractConfig_SUBMITTER_SENDER,
			},
		}, nil
	}

	// Lookup the instance against the domain
	psc, err := dm.GetSmartContractByAddress(ctx, td.c.dbTX, contractAddr)
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
	assert.Equal(t, "0xfeedbeef", psc.(*domainContract).info.ConfigBytes.String())

	// Get cached
	psc2, err := dm.GetSmartContractByAddress(ctx, td.c.dbTX, contractAddr)
	require.NoError(t, err)
	assert.Equal(t, psc, psc2)
}

func TestEventIndexingBadEvent(t *testing.T) {

	td, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
		mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, nil)
		mc.db.ExpectBegin()
		mc.db.ExpectCommit()
		mc.db.ExpectBegin()
		mc.db.ExpectCommit()
	})
	defer done()

	err := td.dm.persistence.Transaction(td.ctx, func(ctx context.Context, tx persistence.DBTX) error {
		_, _, err := td.dm.registrationIndexer(td.ctx, tx, &blockindexer.EventDeliveryBatch{
			StreamID:   uuid.New(),
			StreamName: "name_given_by_component_mgr",
			BatchID:    uuid.New(),
			Events: []*pldapi.EventWithData{
				{
					Address:           *td.d.registryAddress,
					SoliditySignature: eventSolSig_PaladinRegisterSmartContract_V0,
					Data: pldtypes.RawJSON(`{
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

	td, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
		mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, nil)
		mc.db.ExpectBegin()
		mc.db.ExpectCommit()
		mc.db.ExpectBegin()
		mc.db.ExpectExec("INSERT").WillReturnError(fmt.Errorf("pop"))
		mc.db.ExpectRollback()
	})
	defer done()

	contractAddr := pldtypes.EthAddress(pldtypes.RandBytes(20))
	deployTX := uuid.New()
	err := td.dm.persistence.Transaction(td.ctx, func(ctx context.Context, tx persistence.DBTX) error {
		_, _, err := td.dm.registrationIndexer(td.ctx, tx, &blockindexer.EventDeliveryBatch{
			StreamID:   uuid.New(),
			StreamName: "name_given_by_component_mgr",
			BatchID:    uuid.New(),
			Events: []*pldapi.EventWithData{
				{
					SoliditySignature: eventSolSig_PaladinRegisterSmartContract_V0,
					Address:           *td.tp.d.RegistryAddress(),
					IndexedEvent: &pldapi.IndexedEvent{
						BlockNumber:      12345,
						TransactionIndex: 0,
						LogIndex:         0,
						TransactionHash:  pldtypes.NewBytes32FromSlice(pldtypes.RandBytes(32)),
						Signature:        eventSig_PaladinRegisterSmartContract_V0,
					},
					Data: pldtypes.RawJSON(`{
						 "txId": "` + pldtypes.Bytes32UUIDFirst16(deployTX).String() + `",
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
	txIDBytes32 := pldtypes.Bytes32UUIDFirst16(txID)
	contract1 := pldtypes.RandAddress()
	contract2 := pldtypes.RandAddress()
	stateSpent := pldtypes.RandHex(32)
	stateRead := pldtypes.RandHex(32)
	stateConfirmed := pldtypes.RandHex(32)
	stateInfo := pldtypes.RandHex(32)
	fakeHash1 := pldtypes.RandHex(32)
	fakeSchema := pldtypes.RandBytes32()
	event1 := &pldapi.EventWithData{
		Address: *contract1,
		IndexedEvent: &pldapi.IndexedEvent{
			BlockNumber:      1000,
			TransactionIndex: 20,
			LogIndex:         30,
			TransactionHash:  pldtypes.MustParseBytes32(pldtypes.RandHex(32)),
			Signature:        pldtypes.MustParseBytes32(pldtypes.RandHex(32)),
		},
		SoliditySignature: "some event signature 1",
		Data:              pldtypes.RawJSON(`{"result": "success"}`),
	}
	event2 := &pldapi.EventWithData{
		Address: *contract2,
		IndexedEvent: &pldapi.IndexedEvent{
			BlockNumber:      2000,
			TransactionIndex: 30,
			LogIndex:         40,
			TransactionHash:  pldtypes.MustParseBytes32(pldtypes.RandHex(32)),
			Signature:        pldtypes.MustParseBytes32(pldtypes.RandHex(32)),
		},
		SoliditySignature: "some event signature 2",
		Data:              pldtypes.RawJSON(`{"result": "success"}`),
	}

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {

		mc.stateStore.On("WriteStateFinalizations", mock.Anything, mock.Anything, []*pldapi.StateSpendRecord{
			{DomainName: "test1", State: pldtypes.MustParseHexBytes(stateSpent), Transaction: txID}, // the SpentStates StateUpdate
		}, []*pldapi.StateReadRecord{
			{DomainName: "test1", State: pldtypes.MustParseHexBytes(stateRead), Transaction: txID}, // the ReadStates StateUpdate
		}, []*pldapi.StateConfirmRecord{
			{DomainName: "test1", State: pldtypes.MustParseHexBytes(stateConfirmed), Transaction: txID}, // the ConfirmedStates StateUpdate
			{DomainName: "test1", State: pldtypes.MustParseHexBytes(fakeHash1), Transaction: txID},      // the implicit confirm from the NewConfirmedState
		}, []*pldapi.StateInfoRecord{
			{DomainName: "test1", State: pldtypes.MustParseHexBytes(stateInfo), Transaction: txID}, // the InfoStates StateUpdate
		}).Return(nil, nil)

		mc.stateStore.On("WritePreVerifiedStates", mock.Anything, mock.Anything, "test1", []*components.StateUpsertOutsideContext{
			{
				ID:              pldtypes.MustParseHexBytes(fakeHash1),
				Data:            pldtypes.RawJSON(`{"color": "blue"}`),
				ContractAddress: contract2,
				SchemaID:        fakeSchema,
			},
		}).Return(nil, nil)

		mc.txManager.On("FinalizeTransactions", mock.Anything, mock.Anything, mock.MatchedBy(func(receipts []*components.ReceiptInput) bool {
			// Note first contract is unrecognized, second is recognized
			require.Len(t, receipts, 1)
			r := receipts[0]
			expectedEvent := event2
			assert.Equal(t, txID, r.TransactionID)
			assert.Equal(t, pldtypes.OnChainEvent, r.OnChain.Type)
			assert.Equal(t, expectedEvent.TransactionHash.String(), r.OnChain.TransactionHash.String())
			assert.Equal(t, expectedEvent.BlockNumber, r.OnChain.BlockNumber)
			assert.Equal(t, expectedEvent.TransactionIndex, r.OnChain.TransactionIndex)
			assert.Equal(t, expectedEvent.LogIndex, r.OnChain.LogIndex)
			return true
		})).Return(nil)

		mc.privateTxManager.On("PrivateTransactionConfirmed", mock.Anything, mock.Anything).Return()

		mc.txManager.On("SendTransactions", mock.Anything, mock.Anything, mock.Anything).Return([]uuid.UUID{txID}, nil)

	})
	defer done()
	d := td.d
	ctx := td.ctx

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	// First contract is unrecognized, second is recognized
	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{},
	))
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract2, d.registryAddress))
	mp.Mock.ExpectCommit()

	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		assert.Equal(t, batchID.String(), req.BatchId)
		assert.Equal(t, contract2.String(), req.ContractInfo.ContractAddress)
		assert.Equal(t, `{"result": "success"}`, req.Events[0].DataJson)

		// Can call send TX in this flow
		_, err := td.d.SendTransaction(ctx, &prototk.SendTransactionRequest{
			StateQueryContext: req.StateQueryContext,
			Transaction: &prototk.TransactionInput{
				ContractAddress: "0x05d936207F04D81a85881b72A0D17854Ee8BE45A",
				FunctionAbiJson: `{}`,
				ParamsJson:      `{}`,
			},
		})
		require.NoError(t, err)

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
			ReadStates: []*prototk.StateUpdate{
				{
					Id:            stateRead,
					TransactionId: txIDBytes32.String(),
				},
			},
			ConfirmedStates: []*prototk.StateUpdate{
				{
					Id:            stateConfirmed,
					TransactionId: txIDBytes32.String(),
				},
			},
			InfoStates: []*prototk.StateUpdate{
				{
					Id:            stateInfo,
					TransactionId: txIDBytes32.String(),
				},
			},
			NewStates: []*prototk.NewConfirmedState{
				{
					Id:            &fakeHash1,
					StateDataJson: `{"color": "blue"}`,
					SchemaId:      fakeSchema.String(),
					TransactionId: txIDBytes32.String(),
				},
			},
		}, nil
	}
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{Valid: true, ContractConfig: &prototk.ContractConfig{}}, nil
	}

	req := d.dm.privateTxWaiter.AddInflight(ctx, txID)
	err = mp.P.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return d.handleEventBatch(ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events:  []*pldapi.EventWithData{event1, event2},
		})
	})
	require.NoError(t, err)

	_, err = req.Wait()
	require.NoError(t, err)
}

func TestHandleEventBatchFinalizeFail(t *testing.T) {
	batchID := uuid.New()

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.db.ExpectBegin()
		mc.db.ExpectExec(`INSERT.*private_smart_contracts`).WillReturnResult(driver.ResultNoRows)

		mc.txManager.On("FinalizeTransactions", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	})
	defer done()

	err := td.dm.persistence.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address: *td.d.registryAddress,
					IndexedEvent: &pldapi.IndexedEvent{
						BlockNumber:      1000,
						TransactionIndex: 20,
						LogIndex:         30,
						TransactionHash:  pldtypes.MustParseBytes32(pldtypes.RandHex(32)),
						Signature:        eventSig_PaladinRegisterSmartContract_V0,
					},
					SoliditySignature: eventSolSig_PaladinRegisterSmartContract_V0,
					Data:              pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	assert.Regexp(t, "pop", err)

}

func TestHandleEventIgnoreUnknownDomain(t *testing.T) {
	batchID := uuid.New()

	td, done := newTestDomain(t, false, goodDomainConf(), mockBegin, mockSchemas())
	defer done()

	err := td.dm.persistence.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address: *pldtypes.RandAddress(),
					IndexedEvent: &pldapi.IndexedEvent{
						BlockNumber:      1000,
						TransactionIndex: 20,
						LogIndex:         30,
						TransactionHash:  pldtypes.MustParseBytes32(pldtypes.RandHex(32)),
						Signature:        eventSig_PaladinRegisterSmartContract_V0,
					},
					SoliditySignature: eventSolSig_PaladinRegisterSmartContract_V0,
					Data:              pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	require.NoError(t, err)

}

func TestHandleEventBatchContractLookupFail(t *testing.T) {
	batchID := uuid.New()
	contract1 := pldtypes.RandAddress()

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnError(fmt.Errorf("pop"))

	err = mp.P.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address: *contract1,
					Data:    pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	assert.EqualError(t, err, "pop")
}

func TestHandleEventBatchRegistrationError(t *testing.T) {
	batchID := uuid.New()

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectExec("INSERT.*private_smart_contracts").WillReturnError(fmt.Errorf("pop"))

	registrationData := &event_PaladinRegisterSmartContract_V0{
		TXId: pldtypes.RandBytes32(),
	}
	registrationDataJSON, err := json.Marshal(registrationData)
	require.NoError(t, err)

	err = mp.P.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address:           *td.d.registryAddress,
					IndexedEvent:      &pldapi.IndexedEvent{},
					SoliditySignature: eventSolSig_PaladinRegisterSmartContract_V0,
					Data:              registrationDataJSON,
				},
			},
		})
	})
	assert.EqualError(t, err, "pop")
}

func TestHandleEventBatchDomainError(t *testing.T) {
	batchID := uuid.New()
	contract1 := pldtypes.RandAddress()

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, td.d.registryAddress))

	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return nil, fmt.Errorf("pop")
	}
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{Valid: true, ContractConfig: &prototk.ContractConfig{}}, nil
	}

	err = mp.P.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address:      *td.d.registryAddress,
					IndexedEvent: &pldapi.IndexedEvent{},
					Data:         pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	assert.EqualError(t, err, "pop")
}

func TestHandleEventBatchSpentBadTransactionID(t *testing.T) {
	batchID := uuid.New()
	contract1 := pldtypes.RandAddress()
	stateSpent := pldtypes.RandHex(32)

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, td.d.registryAddress))

	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			SpentStates: []*prototk.StateUpdate{
				{
					Id:            stateSpent,
					TransactionId: "badnotgood",
				},
			},
		}, nil
	}
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{Valid: true, ContractConfig: &prototk.ContractConfig{}}, nil
	}

	err = mp.P.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address:      *td.d.registryAddress,
					IndexedEvent: &pldapi.IndexedEvent{},
					Data:         pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	assert.ErrorContains(t, err, "PD020007")
}

func TestHandleEventBatchReadBadTransactionID(t *testing.T) {
	batchID := uuid.New()
	contract1 := pldtypes.RandAddress()
	stateSpent := pldtypes.RandHex(32)

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, td.d.registryAddress))

	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			ReadStates: []*prototk.StateUpdate{
				{
					Id:            stateSpent,
					TransactionId: "badnotgood",
				},
			},
		}, nil
	}
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{Valid: true, ContractConfig: &prototk.ContractConfig{}}, nil
	}

	err = mp.P.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address:      *td.d.registryAddress,
					IndexedEvent: &pldapi.IndexedEvent{},
					Data:         pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	assert.ErrorContains(t, err, "PD020007")
}

func TestHandleEventBatchConfirmBadTransactionID(t *testing.T) {
	batchID := uuid.New()
	contract1 := pldtypes.RandAddress()
	stateSpent := pldtypes.RandHex(32)

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, td.d.registryAddress))

	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			ConfirmedStates: []*prototk.StateUpdate{
				{
					Id:            stateSpent,
					TransactionId: "badnotgood",
				},
			},
		}, nil
	}
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{Valid: true, ContractConfig: &prototk.ContractConfig{}}, nil
	}

	err = mp.P.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address:      *td.d.registryAddress,
					IndexedEvent: &pldapi.IndexedEvent{},
					Data:         pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	assert.ErrorContains(t, err, "PD020007")
}

func TestHandleEventBatchInfoBadTransactionID(t *testing.T) {
	batchID := uuid.New()
	contract1 := pldtypes.RandAddress()
	stateSpent := pldtypes.RandHex(32)

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, td.d.registryAddress))

	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			InfoStates: []*prototk.StateUpdate{
				{
					Id:            stateSpent,
					TransactionId: "badnotgood",
				},
			},
		}, nil
	}
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{Valid: true, ContractConfig: &prototk.ContractConfig{}}, nil
	}

	err = mp.P.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address:      *td.d.registryAddress,
					IndexedEvent: &pldapi.IndexedEvent{},
					Data:         pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	assert.ErrorContains(t, err, "PD020007")
}

func TestHandleEventBatchSpentBadSchemaID(t *testing.T) {
	batchID := uuid.New()
	contract1 := pldtypes.RandAddress()

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, td.d.registryAddress))

	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			SpentStates: []*prototk.StateUpdate{
				{
					Id:            "bad",
					TransactionId: pldtypes.RandHex(32),
				},
			},
		}, nil
	}
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{Valid: true, ContractConfig: &prototk.ContractConfig{}}, nil
	}

	err = mp.P.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address:      *td.d.registryAddress,
					IndexedEvent: &pldapi.IndexedEvent{},
					Data:         pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	assert.ErrorContains(t, err, "PD011650")
}

func TestHandleEventBatchReadBadSchemaID(t *testing.T) {
	batchID := uuid.New()
	contract1 := pldtypes.RandAddress()

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, td.d.registryAddress))

	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			ReadStates: []*prototk.StateUpdate{
				{
					Id:            "bad",
					TransactionId: pldtypes.RandHex(32),
				},
			},
		}, nil
	}
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{Valid: true, ContractConfig: &prototk.ContractConfig{}}, nil
	}

	err = mp.P.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address:      *td.d.registryAddress,
					IndexedEvent: &pldapi.IndexedEvent{},
					Data:         pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	assert.ErrorContains(t, err, "PD011650")
}

func TestHandleEventBatchConfirmBadSchemaID(t *testing.T) {
	batchID := uuid.New()
	contract1 := pldtypes.RandAddress()

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, td.d.registryAddress))

	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			ConfirmedStates: []*prototk.StateUpdate{
				{
					Id:            "bad",
					TransactionId: pldtypes.RandHex(32),
				},
			},
		}, nil
	}
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{Valid: true, ContractConfig: &prototk.ContractConfig{}}, nil
	}

	err = mp.P.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address:      *td.d.registryAddress,
					IndexedEvent: &pldapi.IndexedEvent{},
					Data:         pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	assert.ErrorContains(t, err, "PD011650")
}

func TestHandleEventBatchNewBadTransactionID(t *testing.T) {
	batchID := uuid.New()
	contract1 := pldtypes.RandAddress()

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, td.d.registryAddress))

	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			NewStates: []*prototk.NewConfirmedState{
				{
					TransactionId: "badnotgood",
				},
			},
		}, nil
	}
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{Valid: true, ContractConfig: &prototk.ContractConfig{}}, nil
	}

	err = mp.P.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address:      *td.d.registryAddress,
					IndexedEvent: &pldapi.IndexedEvent{},
					Data:         pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	assert.ErrorContains(t, err, "PD020007")
}

func TestHandleEventBatchNewBadSchemaID(t *testing.T) {
	batchID := uuid.New()
	contract1 := pldtypes.RandAddress()

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, td.d.registryAddress))

	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			NewStates: []*prototk.NewConfirmedState{
				{
					SchemaId:      "badnotgood",
					TransactionId: pldtypes.RandHex(32),
				},
			},
		}, nil
	}
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{Valid: true, ContractConfig: &prototk.ContractConfig{}}, nil
	}

	err = mp.P.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address:      *td.d.registryAddress,
					IndexedEvent: &pldapi.IndexedEvent{},
					Data:         pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	assert.ErrorContains(t, err, "PD011641")
}

func TestHandleEventBatchNewBadStateID(t *testing.T) {
	batchID := uuid.New()
	contract1 := pldtypes.RandAddress()

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, td.d.registryAddress))

	stateID := "badnotgood"
	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			NewStates: []*prototk.NewConfirmedState{
				{
					Id: &stateID,
				},
			},
		}, nil
	}
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{Valid: true, ContractConfig: &prototk.ContractConfig{}}, nil
	}

	err = mp.P.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address:      *td.d.registryAddress,
					IndexedEvent: &pldapi.IndexedEvent{},
					Data:         pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	assert.ErrorContains(t, err, "PD011650")
}

func TestHandleEventBatchBadTransactionID(t *testing.T) {
	batchID := uuid.New()
	contract1 := pldtypes.RandAddress()

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, td.d.registryAddress))

	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
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
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{Valid: true, ContractConfig: &prototk.ContractConfig{}}, nil
	}

	err = mp.P.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address:      *td.d.registryAddress,
					IndexedEvent: &pldapi.IndexedEvent{},
					Data:         pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	assert.ErrorContains(t, err, "PD020008")
}

func TestHandleEventBatchMarkConfirmedFail(t *testing.T) {
	batchID := uuid.New()
	txID := uuid.New()
	txIDBytes32 := pldtypes.Bytes32UUIDFirst16(txID)
	contract1 := pldtypes.RandAddress()
	stateConfirmed := pldtypes.RandHex(32)

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.stateStore.On("WriteStateFinalizations", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(fmt.Errorf("pop"))
	})
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, td.d.registryAddress))

	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			ConfirmedStates: []*prototk.StateUpdate{
				{
					Id:            stateConfirmed,
					TransactionId: txIDBytes32.String(),
				},
			},
		}, nil
	}
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{Valid: true, ContractConfig: &prototk.ContractConfig{}}, nil
	}

	err = mp.P.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address:      *td.d.registryAddress,
					IndexedEvent: &pldapi.IndexedEvent{},
					Data:         pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	assert.EqualError(t, err, "pop")
}

func TestHandleEventBatchUpsertStateFail(t *testing.T) {
	batchID := uuid.New()
	contract1 := pldtypes.RandAddress()

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.stateStore.On("WritePreVerifiedStates", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, td.d.registryAddress))

	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			NewStates: []*prototk.NewConfirmedState{
				{
					SchemaId:      pldtypes.RandHex(32),
					StateDataJson: `{"color": "blue"}`,
					TransactionId: pldtypes.RandHex(32),
				},
			},
		}, nil
	}
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{Valid: true, ContractConfig: &prototk.ContractConfig{}}, nil
	}

	err = mp.P.Transaction(context.Background(), func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(td.ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: batchID,
			Events: []*pldapi.EventWithData{
				{
					Address:      *td.d.registryAddress,
					IndexedEvent: &pldapi.IndexedEvent{},
					Data:         pldtypes.RawJSON(`{"result": "success"}`),
				},
			},
		})
	})
	assert.EqualError(t, err, "pop")
}

func TestReceiptSorting(t *testing.T) {
	// Note the detail of the sorting code is in pldtypes.OnChainLocation
	receiptList := txCompletionsOrdered{
		{ReceiptInput: components.ReceiptInput{OnChain: pldtypes.OnChainLocation{Type: pldtypes.OnChainEvent, BlockNumber: 1100}}},
		{ReceiptInput: components.ReceiptInput{OnChain: pldtypes.OnChainLocation{ /* not onchain */ }}},
		{ReceiptInput: components.ReceiptInput{OnChain: pldtypes.OnChainLocation{Type: pldtypes.OnChainEvent, BlockNumber: 1000}}},
	}
	sort.Sort(receiptList)
	assert.Equal(t, txCompletionsOrdered{
		{ReceiptInput: components.ReceiptInput{OnChain: pldtypes.OnChainLocation{Type: pldtypes.NotOnChain}}},
		{ReceiptInput: components.ReceiptInput{OnChain: pldtypes.OnChainLocation{Type: pldtypes.OnChainEvent, BlockNumber: 1000}}},
		{ReceiptInput: components.ReceiptInput{OnChain: pldtypes.OnChainLocation{Type: pldtypes.OnChainEvent, BlockNumber: 1100}}},
	}, receiptList)
}
