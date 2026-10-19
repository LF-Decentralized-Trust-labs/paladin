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
	"slices"
	"sort"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/pkg/blockindexer"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSolidityEventSignatures(t *testing.T) {
	// We don't expect these to change without our knowledge.
	// We tolerate them changing between versions of firefly-signer (used only in memory), but it's important we understand why if it does.
	//
	// We don't store them as a constant because we're reliant on us and blockindexer calculating it identically (we use the same lib).
	//
	// The standard solidity signature is insufficient, as it doesn't include variable names, or the indexed-ness of fields
	assert.Equal(t, "event PaladinRegisterSmartContract_V0(bytes32 indexed txId, address indexed instance, bytes config)", eventSolSig_PaladinRegisterSmartContract_V0)
	assert.Equal(t, "event PaladinUpgradeSmartContractConfig_V0(bytes config)", eventSolSig_PaladinUpgradeSmartContractConfig_V0)
}

func registrationEvent(registry pldtypes.EthAddress, deployTX uuid.UUID, contractAddr pldtypes.EthAddress, config string, logIndex int64) *pldapi.EventWithData {
	return &pldapi.EventWithData{
		SoliditySignature: eventSolSig_PaladinRegisterSmartContract_V0,
		Address:           registry,
		IndexedEvent: &pldapi.IndexedEvent{
			BlockNumber:      12345,
			TransactionIndex: 0,
			LogIndex:         logIndex,
			TransactionHash:  pldtypes.RandBytes32(),
			Signature:        eventSig_PaladinRegisterSmartContract_V0,
		},
		Data: pldtypes.RawJSON(`{
			"txId": "` + pldtypes.Bytes32UUIDFirst16(deployTX).String() + `",
			"instance": "` + contractAddr.String() + `",
			"config": "` + config + `"
		}`),
	}
}

func registerTestSmartContract(t *testing.T, td *testDomainContext) (deployTX uuid.UUID, contractAddr pldtypes.EthAddress) {
	// Index an event indicating deployment of a new smart contract instance
	deployTX = uuid.New()
	contractAddr = pldtypes.EthAddress(pldtypes.RandBytes(20))
	var batchTxs txCompletionsOrdered
	var unprocessedEvents []*pldapi.EventWithData
	err := td.dm.persistence.Transaction(td.ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		unprocessedEvents, _, batchTxs, err = td.dm.registrationIndexer(ctx, dbTX, &blockindexer.EventDeliveryBatch{
			StreamID:   uuid.New(),
			StreamName: "name_given_by_component_mgr",
			BatchID:    uuid.New(),
			Events: []*pldapi.EventWithData{
				registrationEvent(*td.d.registryAddress, deployTX, contractAddr, "0xfeedbeef", 0),
			},
		})
		return err
	})
	require.NoError(t, err)
	assert.Len(t, batchTxs, 1)
	assert.Empty(t, unprocessedEvents) // we consumed all the events there were

	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{
			Valid: true,
			ContractConfig: &prototk.ContractConfig{
				ContractConfigJson:   `{}`,
				CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
				SubmitterSelection:   prototk.ContractConfig_SUBMITTER_SENDER,
			},
		}, nil
	}

	return deployTX, contractAddr
}

func TestEventIndexingWithDB(t *testing.T) {

	td, done := newTestDomain(t, true /* real DB */, goodDomainConf())
	defer done()
	ctx := td.ctx
	tp := td.tp
	dm := td.dm

	// Index an event indicating deployment of a new smart contract instance
	deployTX, contractAddr := registerTestSmartContract(t, td)

	// Lookup the instance against the domain
	psc, err := dm.GetSmartContractByAddress(ctx, td.c.dbTX, contractAddr)
	require.NoError(t, err)
	dc := psc.(*domainContract)

	assert.Equal(t, deployTX, dc.info.DeployTX)
	assert.Equal(t, *tp.d.RegistryAddress(), dc.info.RegistryAddress)
	assert.Equal(t, contractAddr, dc.info.Address)
	assert.Equal(t, pldtypes.HexBytes{0xfe, 0xed, 0xbe, 0xef}, dc.info.ConfigBytes)
	assert.NotEqual(t, pldtypes.Timestamp(0), dc.info.Created) // Ensure Created field is populated

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
		mc.db.ExpectRollback()
	})
	defer done()

	err := td.dm.persistence.Transaction(td.ctx, func(ctx context.Context, tx persistence.DBTX) error {
		_, _, _, err := td.dm.registrationIndexer(td.ctx, tx, &blockindexer.EventDeliveryBatch{
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
	assert.Regexp(t, "PD011668", err)

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
		_, _, _, err := td.dm.registrationIndexer(td.ctx, tx, &blockindexer.EventDeliveryBatch{
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
	eventTx2Hash := pldtypes.RandHex(32)
	sequencerNotified := make(chan struct{}, 1)

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
			TransactionHash:  pldtypes.MustParseBytes32(eventTx2Hash),
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

		mc.sequencerManager.On("PrivateTransactionsConfirmed", mock.Anything, mock.Anything).Run(func(mock.Arguments) {
			sequencerNotified <- struct{}{}
		}).Return(nil)

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

	<-sequencerNotified
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
					IndexedEvent: &pldapi.IndexedEvent{},
					Address:      *contract1,
					Data:         pldtypes.RawJSON(`{"result": "success"}`),
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

func TestHandleEventBatchPendingPrivateStateData(t *testing.T) {
	batchID := uuid.New()
	txID := uuid.New()
	txIDBytes32 := pldtypes.Bytes32UUIDFirst16(txID)
	txHash := pldtypes.RandBytes32()
	contract1 := pldtypes.RandAddress()
	stateConfirmedHex := pldtypes.RandHex(32)

	domainConf := goodDomainConf()
	domainConf.FullStateAvailablityRequired = true

	sequencerNotified := make(chan struct{}, 1)
	td, done := newTestDomain(t, false, domainConf, mockSchemas(), func(mc *mockComponents) {
		mc.stateStore.On("WriteStateFinalizations", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mc.stateStore.On("WritePendingPrivateStateDataBatch", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil)
		mc.txManager.On("FinalizeTransactions", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mc.sequencerManager.On("PrivateTransactionsConfirmed", mock.Anything, mock.Anything).Run(func(mock.Arguments) {
			sequencerNotified <- struct{}{}
		}).Return(nil)
	})
	defer done()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectBegin()
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, td.d.registryAddress))
	mp.Mock.ExpectCommit()

	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			ConfirmedStates: []*prototk.StateUpdate{
				{Id: stateConfirmedHex, TransactionId: txIDBytes32.String()},
			},
			TransactionsComplete: []*prototk.CompletedTransaction{
				{
					TransactionId: txIDBytes32.String(),
					Location: &prototk.OnChainEventLocation{
						BlockNumber:     100,
						TransactionHash: txHash.String(),
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
				{Address: *td.d.registryAddress, IndexedEvent: &pldapi.IndexedEvent{}, Data: pldtypes.RawJSON(`{"result": "success"}`)},
			},
		})
	})
	require.NoError(t, err)

	<-sequencerNotified
}

func TestHandleEventBatchPendingPrivateStateDataWriteError(t *testing.T) {
	batchID := uuid.New()
	txID := uuid.New()
	txIDBytes32 := pldtypes.Bytes32UUIDFirst16(txID)
	txHash := pldtypes.RandBytes32()
	contract1 := pldtypes.RandAddress()
	stateConfirmedHex := pldtypes.RandHex(32)

	domainConf := goodDomainConf()
	domainConf.FullStateAvailablityRequired = true

	td, done := newTestDomain(t, false, domainConf, mockSchemas(), func(mc *mockComponents) {
		mc.stateStore.On("WriteStateFinalizations", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mc.stateStore.On("WritePendingPrivateStateDataBatch", mock.Anything, mock.Anything, "test1", mock.Anything).Return(fmt.Errorf("pop"))
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
				{Id: stateConfirmedHex, TransactionId: txIDBytes32.String()},
			},
			TransactionsComplete: []*prototk.CompletedTransaction{
				{
					TransactionId: txIDBytes32.String(),
					Location: &prototk.OnChainEventLocation{
						BlockNumber:     100,
						TransactionHash: txHash.String(),
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
				{Address: *td.d.registryAddress, IndexedEvent: &pldapi.IndexedEvent{}, Data: pldtypes.RawJSON(`{"result": "success"}`)},
			},
		})
	})
	assert.EqualError(t, err, "pop")
}

func TestHandleEventBatchPendingPrivateStateDataBadTransactionID(t *testing.T) {
	batchID := uuid.New()
	txID := uuid.New()
	txIDBytes32 := pldtypes.Bytes32UUIDFirst16(txID)
	contract1 := pldtypes.RandAddress()
	stateConfirmedHex := pldtypes.RandHex(32)

	domainConf := goodDomainConf()
	domainConf.FullStateAvailablityRequired = true

	td, done := newTestDomain(t, false, domainConf, mockSchemas(), func(mc *mockComponents) {
		mc.stateStore.On("WriteStateFinalizations", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
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
				{Id: stateConfirmedHex, TransactionId: txIDBytes32.String()},
			},
			TransactionsComplete: []*prototk.CompletedTransaction{
				// Empty TransactionId triggers recoverTransactionID error
				{TransactionId: ""},
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
				{Address: *td.d.registryAddress, IndexedEvent: &pldapi.IndexedEvent{}, Data: pldtypes.RawJSON(`{"result": "success"}`)},
			},
		})
	})
	assert.ErrorContains(t, err, "PD020008")
}

func upgradeEvent(source pldtypes.EthAddress, config string, logIndex int64) *pldapi.EventWithData {
	return &pldapi.EventWithData{
		SoliditySignature: eventSolSig_PaladinUpgradeSmartContractConfig_V0,
		Address:           source,
		IndexedEvent: &pldapi.IndexedEvent{
			BlockNumber:      12346,
			TransactionIndex: 0,
			LogIndex:         logIndex,
			TransactionHash:  pldtypes.RandBytes32(),
			Signature:        eventSig_PaladinUpgradeSmartContractConfig_V0,
		},
		Data: pldtypes.RawJSON(`{"config": "` + config + `"}`),
	}
}

func domainEvent(source pldtypes.EthAddress, logIndex int64) *pldapi.EventWithData {
	return &pldapi.EventWithData{
		SoliditySignature: "some event signature",
		Address:           source,
		IndexedEvent: &pldapi.IndexedEvent{
			BlockNumber:      12346,
			TransactionIndex: 0,
			LogIndex:         logIndex,
			TransactionHash:  pldtypes.RandBytes32(),
			Signature:        pldtypes.RandBytes32(),
		},
		Data: pldtypes.RawJSON(`{"result": "success"}`),
	}
}

// upgradeTestDomain is a real-DB domain whose sequencer manager mock relays what the completion loop hands it
type upgradeTestDomain struct {
	*testDomainContext
	deployTX      uuid.UUID
	confirmed     chan []*components.TxCompletion
	configChanged chan pldtypes.EthAddress
	initConfigs   []string
}

func newUpgradeTestDomain(t *testing.T, upgradeFails bool) (*upgradeTestDomain, pldtypes.EthAddress, func()) {
	utd := &upgradeTestDomain{
		confirmed:     make(chan []*components.TxCompletion, 1),
		configChanged: make(chan pldtypes.EthAddress, 10),
	}
	td, done := newTestDomain(t, true, goodDomainConf(), func(mc *mockComponents) {
		if !upgradeFails {
			mc.sequencerManager.On("PrivateTransactionsConfirmed", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				utd.confirmed <- args[1].([]*components.TxCompletion)
			}).Return()
		}
		mc.sequencerManager.On("HandleContractConfigChanged", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			utd.configChanged <- args[1].(pldtypes.EthAddress)
		}).Return().Maybe()
		mc.txManager.On("FinalizeTransactions", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	})
	utd.testDomainContext = td
	deployTX, contractAddr := registerTestSmartContract(t, td)
	utd.deployTX = deployTX

	td.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{}, nil
	}

	// The domain reports each configuration it is initialized with, so the test can see which bytes reached it
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		config := pldtypes.HexBytes(icr.ContractConfig).String()
		utd.initConfigs = append(utd.initConfigs, config)
		return &prototk.InitContractResponse{
			Valid: config != "0xbad0",
			ContractConfig: &prototk.ContractConfig{
				ContractConfigJson: `{"config":"` + config + `"}`,
			},
		}, nil
	}
	return utd, contractAddr, done
}

func (utd *upgradeTestDomain) handleBatch(t *testing.T, events ...*pldapi.EventWithData) {
	err := utd.dm.persistence.Transaction(utd.ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return utd.d.handleEventBatch(ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: uuid.New(),
			Events:  events,
		})
	})
	require.NoError(t, err)
}

func (utd *upgradeTestDomain) storedContract(t *testing.T, addr pldtypes.EthAddress) *PrivateSmartContract {
	var contracts []*PrivateSmartContract
	err := utd.dm.persistence.NOTX().DB(utd.ctx).Table("private_smart_contracts").Where("address = ?", addr).Find(&contracts).Error
	require.NoError(t, err)
	require.Len(t, contracts, 1)
	return contracts[0]
}

func (utd *upgradeTestDomain) storedConfig(t *testing.T, addr pldtypes.EthAddress) string {
	return utd.storedContract(t, addr).ConfigBytes.String()
}

func (utd *upgradeTestDomain) storedIndexedConfig(t *testing.T, addr pldtypes.EthAddress) string {
	return utd.storedContract(t, addr).IndexedConfigBytes.String()
}

func (utd *upgradeTestDomain) liveContract(t *testing.T, addr pldtypes.EthAddress) components.DomainSmartContract {
	psc, err := utd.dm.GetSmartContractByAddress(utd.ctx, utd.dm.persistence.NOTX(), addr)
	require.NoError(t, err)
	return psc
}

func (utd *upgradeTestDomain) liveContractConfig(t *testing.T, addr pldtypes.EthAddress) string {
	return utd.liveContract(t, addr).ContractConfig().ContractConfigJson
}

func (utd *upgradeTestDomain) recordBatchConfigs() *[]string {
	var batchConfigs []string
	utd.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		batchConfigs = append(batchConfigs, req.ContractInfo.ContractConfigJson)
		return &prototk.HandleEventBatchResponse{}, nil
	}
	return &batchConfigs
}

func TestUpgradeEventFromInstanceReplacesConfig(t *testing.T) {
	utd, contractAddr, done := newUpgradeTestDomain(t, false)
	defer done()
	ctx := utd.ctx

	before, err := utd.dm.GetSmartContractByAddress(ctx, utd.dm.persistence.NOTX(), contractAddr)
	require.NoError(t, err)

	utd.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return nil, fmt.Errorf("registry events are not delivered to the domain")
	}

	utd.handleBatch(t, upgradeEvent(contractAddr, "0xcafe", 0))

	assert.Empty(t, <-utd.confirmed)
	assert.Equal(t, contractAddr, <-utd.configChanged)

	assert.Equal(t, "0xcafe", utd.storedConfig(t, contractAddr))

	// The next load initializes the domain with the new configuration
	after, err := utd.dm.GetSmartContractByAddress(ctx, utd.dm.persistence.NOTX(), contractAddr)
	require.NoError(t, err)
	assert.NotSame(t, before, after)
	assert.Equal(t, "0xcafe", after.(*domainContract).info.ConfigBytes.String())
	assert.Equal(t, `{"config":"0xcafe"}`, after.ContractConfig().ContractConfigJson)
	assert.Equal(t, []string{"0xfeedbeef", "0xcafe"}, utd.initConfigs)
}

func TestUpgradeEventFromUnregisteredAddressIgnored(t *testing.T) {
	utd, contractAddr, done := newUpgradeTestDomain(t, false)
	defer done()

	utd.handleBatch(t, upgradeEvent(*pldtypes.RandAddress(), "0xcafe", 0))

	assert.Empty(t, <-utd.confirmed)
	assert.Empty(t, utd.configChanged)
	assert.Equal(t, "0xfeedbeef", utd.storedConfig(t, contractAddr))
	assert.Empty(t, utd.initConfigs)
}

func TestUpgradeEventForInstanceOfAnotherRegistryIgnored(t *testing.T) {
	utd, _, done := newUpgradeTestDomain(t, false)
	defer done()

	otherInstance := *pldtypes.RandAddress()
	err := utd.dm.persistence.NOTX().DB(utd.ctx).Table("private_smart_contracts").Create(&PrivateSmartContract{
		DeployTX:           uuid.New(),
		RegistryAddress:    *pldtypes.RandAddress(),
		Address:            otherInstance,
		ConfigBytes:        pldtypes.HexBytes{0xfe, 0xed},
		IndexedConfigBytes: pldtypes.HexBytes{0xfe, 0xed},
	}).Error
	require.NoError(t, err)

	utd.handleBatch(t, upgradeEvent(otherInstance, "0xcafe", 0))

	assert.Empty(t, <-utd.confirmed)
	assert.Empty(t, utd.configChanged)
	assert.Equal(t, "0xfeed", utd.storedConfig(t, otherInstance))
	assert.Empty(t, utd.initConfigs)
}

func TestUpgradeEventBadDataFails(t *testing.T) {
	utd, contractAddr, done := newUpgradeTestDomain(t, true)
	defer done()

	ev := upgradeEvent(contractAddr, "0xcafe", 0)
	ev.Data = pldtypes.RawJSON(`{"config": "cannot parse this"}`)
	err := utd.dm.persistence.Transaction(utd.ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return utd.d.handleEventBatch(ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: uuid.New(),
			Events:  []*pldapi.EventWithData{ev},
		})
	})
	assert.Regexp(t, "PD011668", err)

	assert.Empty(t, utd.configChanged)
	assert.Equal(t, "0xfeedbeef", utd.storedConfig(t, contractAddr))
}

func TestHandleEventBatchSegmentedAtUpgrade(t *testing.T) {
	utd, contractAddr, done := newUpgradeTestDomain(t, false)
	defer done()

	var batchConfigs []string
	var batchEventCounts []int
	utd.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		assert.Equal(t, contractAddr.String(), req.ContractInfo.ContractAddress)
		batchConfigs = append(batchConfigs, req.ContractInfo.ContractConfigJson)
		batchEventCounts = append(batchEventCounts, len(req.Events))
		return &prototk.HandleEventBatchResponse{}, nil
	}

	utd.handleBatch(t,
		domainEvent(contractAddr, 0),
		domainEvent(contractAddr, 1),
		upgradeEvent(contractAddr, "0xcafe", 2),
		domainEvent(contractAddr, 3),
	)

	assert.Empty(t, <-utd.confirmed)
	assert.Equal(t, contractAddr, <-utd.configChanged)
	assert.Equal(t, []string{`{"config":"0xfeedbeef"}`, `{"config":"0xcafe"}`}, batchConfigs)
	assert.Equal(t, []int{2, 1}, batchEventCounts)
}

func TestUpgradeEventInvalidConfigDiscardsLaterEvents(t *testing.T) {
	utd, contractAddr, done := newUpgradeTestDomain(t, false)
	defer done()

	utd.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return nil, fmt.Errorf("must not be called with a configuration the domain rejected")
	}

	utd.handleBatch(t,
		upgradeEvent(contractAddr, "0xbad0", 0),
		domainEvent(contractAddr, 1),
	)

	assert.Empty(t, <-utd.confirmed)
	assert.Equal(t, contractAddr, <-utd.configChanged)
	assert.Equal(t, "0xbad0", utd.storedConfig(t, contractAddr))

	// The stored configuration is now one the domain rejects, so the contract can no longer be loaded
	_, err := utd.dm.GetSmartContractByAddress(utd.ctx, utd.dm.persistence.NOTX(), contractAddr)
	assert.Regexp(t, "PD011610", err)
}

func TestUpgradeEventTwiceInBatchNotifiesOnce(t *testing.T) {
	utd, contractAddr, done := newUpgradeTestDomain(t, false)
	defer done()

	utd.handleBatch(t,
		upgradeEvent(contractAddr, "0xcafe", 0),
		upgradeEvent(contractAddr, "0xf00d", 1),
	)

	assert.Empty(t, <-utd.confirmed)
	assert.Equal(t, contractAddr, <-utd.configChanged)
	assert.Equal(t, "0xf00d", utd.storedConfig(t, contractAddr))
	assert.Empty(t, utd.configChanged)
}

func TestUpgradeEventUpdateError(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.db.ExpectBegin()
		mc.db.ExpectExec("UPDATE.*private_smart_contracts").WillReturnError(fmt.Errorf("pop"))
		mc.db.ExpectRollback()
	})
	defer done()

	contractAddr := *pldtypes.RandAddress()
	err := td.dm.persistence.Transaction(td.ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: uuid.New(),
			Events:  []*pldapi.EventWithData{upgradeEvent(contractAddr, "0xcafe", 0)},
		})
	})
	assert.Regexp(t, "pop", err)
}

func TestUpgradeEventLiveConfigUpdateError(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.db.ExpectBegin()
		mc.db.ExpectExec("UPDATE.*private_smart_contracts.*indexed_config_bytes").WillReturnResult(sqlmock.NewResult(0, 1))
		mc.db.ExpectExec("UPDATE.*private_smart_contracts.*config_bytes").WillReturnError(fmt.Errorf("pop"))
		mc.db.ExpectRollback()
	})
	defer done()

	contractAddr := *pldtypes.RandAddress()
	err := td.dm.persistence.Transaction(td.ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return td.d.handleEventBatch(ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: uuid.New(),
			Events:  []*pldapi.EventWithData{upgradeEvent(contractAddr, "0xcafe", 0)},
		})
	})
	assert.Regexp(t, "pop", err)
}

func TestEventForInstanceOfUnconfiguredRegistryDiscarded(t *testing.T) {
	utd, _, done := newUpgradeTestDomain(t, false)
	defer done()

	otherInstance := *pldtypes.RandAddress()
	err := utd.dm.persistence.NOTX().DB(utd.ctx).Table("private_smart_contracts").Create(&PrivateSmartContract{
		DeployTX:           uuid.New(),
		RegistryAddress:    *pldtypes.RandAddress(),
		Address:            otherInstance,
		ConfigBytes:        pldtypes.HexBytes{0xfe, 0xed},
		IndexedConfigBytes: pldtypes.HexBytes{0xfe, 0xed},
	}).Error
	require.NoError(t, err)

	utd.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return nil, fmt.Errorf("events of a contract whose registry is not configured must not be dispatched")
	}

	utd.handleBatch(t, domainEvent(otherInstance, 0))
	assert.Empty(t, <-utd.confirmed)
	assert.Empty(t, utd.initConfigs)
}

func TestReplayedStreamDispatchesHistoricalConfigAndKeepsLiveConfig(t *testing.T) {
	utd, contractAddr, done := newUpgradeTestDomain(t, false)
	defer done()

	var batchConfigs []string
	var batchEventCounts []int
	utd.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		batchConfigs = append(batchConfigs, req.ContractInfo.ContractConfigJson)
		batchEventCounts = append(batchEventCounts, len(req.Events))
		return &prototk.HandleEventBatchResponse{}, nil
	}

	registration := registrationEvent(*utd.d.registryAddress, utd.deployTX, contractAddr, "0xfeedbeef", 0)
	beforeUpgrades := []*pldapi.EventWithData{domainEvent(contractAddr, 1), domainEvent(contractAddr, 2)}
	afterFirstUpgrade := []*pldapi.EventWithData{upgradeEvent(contractAddr, "0xcafe", 3), domainEvent(contractAddr, 4), domainEvent(contractAddr, 5)}
	afterSecondUpgrade := []*pldapi.EventWithData{upgradeEvent(contractAddr, "0xf00d", 6), domainEvent(contractAddr, 7), domainEvent(contractAddr, 8)}

	utd.handleBatch(t, slices.Concat(beforeUpgrades, afterFirstUpgrade, afterSecondUpgrade)...)
	assert.Empty(t, <-utd.confirmed)
	assert.Equal(t, contractAddr, <-utd.configChanged)
	assert.Equal(t, "0xf00d", utd.storedConfig(t, contractAddr))

	batchConfigs, batchEventCounts = nil, nil
	for i, replayed := range [][]*pldapi.EventWithData{
		append([]*pldapi.EventWithData{registration}, beforeUpgrades...),
		afterFirstUpgrade,
		afterSecondUpgrade,
	} {
		utd.handleBatch(t, replayed...)
		if i == 0 {
			assert.Len(t, <-utd.confirmed, 1)
		} else {
			assert.Empty(t, <-utd.confirmed)
		}
		assert.Equal(t, "0xf00d", utd.storedConfig(t, contractAddr))
		assert.Equal(t, `{"config":"0xf00d"}`, utd.liveContractConfig(t, contractAddr))
	}

	assert.Empty(t, utd.configChanged)
	assert.Equal(t, []string{`{"config":"0xfeedbeef"}`, `{"config":"0xcafe"}`, `{"config":"0xf00d"}`}, batchConfigs)
	assert.Equal(t, []int{2, 2, 2}, batchEventCounts)
	assert.Equal(t, "0xf00d", utd.storedIndexedConfig(t, contractAddr))
}

func TestReplayedRegistrationMovesIndexedConfigBack(t *testing.T) {
	utd, contractAddr, done := newUpgradeTestDomain(t, false)
	defer done()

	upgrade := upgradeEvent(contractAddr, "0xcafe", 0)
	utd.handleBatch(t, upgrade)
	assert.Empty(t, <-utd.confirmed)
	assert.Equal(t, contractAddr, <-utd.configChanged)
	live := utd.liveContract(t, contractAddr)
	assert.Equal(t, `{"config":"0xcafe"}`, live.ContractConfig().ContractConfigJson)

	utd.handleBatch(t, registrationEvent(*utd.d.registryAddress, utd.deployTX, contractAddr, "0xfeedbeef", 0))
	assert.Len(t, <-utd.confirmed, 1)
	assert.Empty(t, utd.configChanged)
	assert.Equal(t, "0xcafe", utd.storedConfig(t, contractAddr))
	assert.Equal(t, configLocation(upgrade), utd.storedContract(t, contractAddr).ConfigLocation)
	assert.Equal(t, "0xfeedbeef", utd.storedIndexedConfig(t, contractAddr))
	assert.Same(t, live, utd.liveContract(t, contractAddr))
}

func TestIndexedContractLoadedAfterReplayedRegistrationIsCached(t *testing.T) {
	utd, contractAddr, done := newUpgradeTestDomain(t, false)
	defer done()

	utd.handleBatch(t, upgradeEvent(contractAddr, "0xcafe", 0))
	assert.Empty(t, <-utd.confirmed)
	assert.Equal(t, contractAddr, <-utd.configChanged)
	live := utd.liveContract(t, contractAddr)

	utd.handleBatch(t, registrationEvent(*utd.d.registryAddress, utd.deployTX, contractAddr, "0xfeedbeef", 0))
	assert.Len(t, <-utd.confirmed, 1)

	batchConfigs := utd.recordBatchConfigs()
	utd.initConfigs = nil
	utd.handleBatch(t, domainEvent(contractAddr, 1))
	assert.Empty(t, <-utd.confirmed)
	utd.handleBatch(t, domainEvent(contractAddr, 2))
	assert.Empty(t, <-utd.confirmed)

	assert.Equal(t, []string{`{"config":"0xfeedbeef"}`, `{"config":"0xfeedbeef"}`}, *batchConfigs)
	assert.Equal(t, []string{"0xfeedbeef"}, utd.initConfigs)
	assert.Same(t, live, utd.liveContract(t, contractAddr))
}

func TestRegistrationOfDifferentDeploymentDoesNotResetConfig(t *testing.T) {
	utd, contractAddr, done := newUpgradeTestDomain(t, false)
	defer done()

	utd.handleBatch(t, registrationEvent(*utd.d.registryAddress, uuid.New(), contractAddr, "0xcafe", 0))

	assert.Len(t, <-utd.confirmed, 1)
	assert.Empty(t, utd.configChanged)
	assert.Equal(t, "0xfeedbeef", utd.storedConfig(t, contractAddr))
	assert.Equal(t, "0xfeedbeef", utd.storedIndexedConfig(t, contractAddr))
}

func TestRegistrationForInstanceOfAnotherRegistryDoesNotResetConfig(t *testing.T) {
	utd, _, done := newUpgradeTestDomain(t, false)
	defer done()

	deployTX := uuid.New()
	otherInstance := *pldtypes.RandAddress()
	err := utd.dm.persistence.NOTX().DB(utd.ctx).Table("private_smart_contracts").Create(&PrivateSmartContract{
		DeployTX:           deployTX,
		RegistryAddress:    *pldtypes.RandAddress(),
		Address:            otherInstance,
		ConfigBytes:        pldtypes.HexBytes{0xfe, 0xed},
		IndexedConfigBytes: pldtypes.HexBytes{0xfe, 0xed},
	}).Error
	require.NoError(t, err)

	utd.handleBatch(t, registrationEvent(*utd.d.registryAddress, deployTX, otherInstance, "0xcafe", 0))

	assert.Len(t, <-utd.confirmed, 1)
	assert.Empty(t, utd.configChanged)
	assert.Equal(t, "0xfeed", utd.storedConfig(t, otherInstance))
	assert.Equal(t, "0xfeed", utd.storedIndexedConfig(t, otherInstance))
}

func TestUpgradeInRolledBackBatchNotCached(t *testing.T) {
	utd, contractAddr, done := newUpgradeTestDomain(t, false)
	defer done()

	utd.tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	err := utd.dm.persistence.Transaction(utd.ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return utd.d.handleEventBatch(ctx, dbTX, &blockindexer.EventDeliveryBatch{
			BatchID: uuid.New(),
			Events:  []*pldapi.EventWithData{upgradeEvent(contractAddr, "0xcafe", 0), domainEvent(contractAddr, 1)},
		})
	})
	assert.Regexp(t, "pop", err)

	assert.Empty(t, utd.configChanged)
	assert.Equal(t, `{"config":"0xfeedbeef"}`, utd.liveContractConfig(t, contractAddr))

	batchConfigs := utd.recordBatchConfigs()
	utd.handleBatch(t, domainEvent(contractAddr, 2))
	assert.Empty(t, <-utd.confirmed)
	assert.Equal(t, []string{`{"config":"0xfeedbeef"}`}, *batchConfigs)
}
