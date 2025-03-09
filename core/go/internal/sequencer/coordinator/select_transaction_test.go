/*
 * Copyright © 2025 Kaleido, Inc.
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

package coordinator

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/kaleido-io/paladin/core/internal/sequencer/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSelectTransaction_PreserveOrderWithinSender(t *testing.T) {
	//transactions from a given sender are assembled, then dispatched in the order they were received
	ctx := context.Background()
	testSender := "alice@node1"

	coordinator, mocks := NewCoordinatorForUnitTest(t, ctx, []string{testSender})

	var assembleRequestID uuid.UUID
	mocks.messageSender.On(
		"SendAssembleRequest",
		mock.Anything, // ctx
		mock.Anything, // sender
		mock.Anything, // transaction ID
		mock.Anything, // Idempotency key
		mock.Anything, // transactionPreassembly
	).Return(nil).Run(func(args mock.Arguments) {
		assembleRequestID = args.Get(3).(uuid.UUID)
	})
	mocks.messageSender.On("SendEndorsementRequest",
		mock.Anything, // ctx
		mock.Anything, // idempotency key
		mock.Anything, // party
		mock.Anything, // attestation request
		mock.Anything, // transaction specification
		mock.Anything, // verifiers
		mock.Anything, // signatures
		mock.Anything, // input states
		mock.Anything, // output states
		mock.Anything, // info states
	).Return(nil).Maybe()

	// send a significant number of transactions from the same sender so that we don't luckily get the right order
	builders := testutil.NewPrivateTransactionBuilderListForTesting(5).Address(*coordinator.contractAddress).Sender(testSender)
	txns := builders.BuildSparse()

	mocks.engineIntegration.On(
		"WriteLockAndDistributeStatesForTransaction",
		mock.Anything, // ctx
		mock.MatchedBy(privateTransactionMatcher(txns[0].ID)), // transaction
	).Return(nil)

	// the first delegate event should transition the coordinator into active mode and trigger the first transaction to be selected and assembled
	err := coordinator.HandleEvent(ctx, &TransactionsDelegatedEvent{
		Sender:       testSender,
		Transactions: txns,
	})
	assert.NoError(t, err)

	//Assert that remaining transactions are assembled in the correct order as each previous one gets an assemble response
	for i := 0; i < 5; i++ {

		transactionsInAssembling := coordinator.getTransactionsInStates(ctx, []transaction.State{transaction.State_Assembling})
		assert.Len(t, transactionsInAssembling, 1)
		assert.Equal(t, txns[i].ID, transactionsInAssembling[0].ID)

		mocks.engineIntegration.On(
			"WriteLockAndDistributeStatesForTransaction",
			mock.Anything, // ctx
			mock.MatchedBy(privateTransactionMatcher(txns[i].ID)), // transaction
		).Return(nil)

		//Send a success
		assembleResponseEvent := &transaction.AssembleSuccessEvent{}
		assembleResponseEvent.TransactionID = txns[i].ID
		assembleResponseEvent.RequestID = assembleRequestID
		assembleResponseEvent.PostAssembly = builders[i].BuildPostAssembly()
		err = coordinator.propagateEventToTransaction(ctx, assembleResponseEvent)
		assert.NoError(t, err)

		//After the first round, we should have B0, C0 and D0 in endorsing state while A0 is in pooled state
		transactionsInEndorsing := coordinator.getTransactionsInStates(ctx, []transaction.State{transaction.State_Endorsement_Gathering})
		endorsingTransactionIDs := make([]uuid.UUID, len(transactionsInEndorsing))
		for j, txn := range transactionsInEndorsing {
			endorsingTransactionIDs[j] = txn.ID
		}

		assert.Len(t, transactionsInEndorsing, i+1)
		assert.Contains(t, endorsingTransactionIDs, txns[i].ID)
	}

}

func TestSelectTransaction_SlowQueue(t *testing.T) {
	ctx := context.Background()
	testSenderA := "alice@node1"
	testSenderB := "bob@node2"
	testSenderC := "carol@node3"
	testSenderD := "dave@node4"

	coordinator, mocks := NewCoordinatorForUnitTest(t, ctx, []string{testSenderA, testSenderB, testSenderC, testSenderD})
	mocks.messageSender.On("SendEndorsementRequest",
		mock.Anything, // ctx
		mock.Anything, // idempotency key
		mock.Anything, // party
		mock.Anything, // attestation request
		mock.Anything, // transaction specification
		mock.Anything, // verifiers
		mock.Anything, // signatures
		mock.Anything, // input states
		mock.Anything, // output states
		mock.Anything, // info states
	).Return(nil).Maybe()

	var assemblingTxnID uuid.UUID
	var assembleRequestID uuid.UUID
	mocks.messageSender.On(
		"SendAssembleRequest",
		mock.Anything, // ctx
		mock.Anything, // sender
		mock.Anything, // transaction ID
		mock.Anything, // Idempotency key
		mock.Anything, // transactionPreassembly
	).Return(nil).Run(func(args mock.Arguments) {
		// in this test, we are not concered with the precise order of assemble requests across senders we we take a copy of the transaction ID so that we can send the correct response
		assemblingTxnID = args.Get(2).(uuid.UUID)
		assembleRequestID = args.Get(3).(uuid.UUID)
	})

	// first transaction from sender B times out while assembling. It should be placed at the end of the slow queue.
	// so we get through the second transaction from other senders before coming back that transaction
	//NOTE: this test is not sensitive to which order the transactions are selected in, only that the slow queue is processed after the fast queue
	buildersA := testutil.NewPrivateTransactionBuilderListForTesting(2).Address(*coordinator.contractAddress).Sender(testSenderA)
	buildersB := testutil.NewPrivateTransactionBuilderListForTesting(2).Address(*coordinator.contractAddress).Sender(testSenderB)
	buildersC := testutil.NewPrivateTransactionBuilderListForTesting(2).Address(*coordinator.contractAddress).Sender(testSenderC)
	buildersD := testutil.NewPrivateTransactionBuilderListForTesting(2).Address(*coordinator.contractAddress).Sender(testSenderD)
	txnsA := buildersA.BuildSparse()
	txnsB := buildersB.BuildSparse()
	txnsC := buildersC.BuildSparse()
	txnsD := buildersD.BuildSparse()

	// the first delegate event should transition the coordinator into active mode and trigger the first transaction to be selected and assembled
	err := coordinator.HandleEvent(ctx, &TransactionsDelegatedEvent{
		Sender:       testSenderA,
		Transactions: txnsA,
	})
	assert.NoError(t, err)

	err = coordinator.HandleEvent(ctx, &TransactionsDelegatedEvent{
		Sender:       testSenderB,
		Transactions: txnsB,
	})
	assert.NoError(t, err)

	err = coordinator.HandleEvent(ctx, &TransactionsDelegatedEvent{
		Sender:       testSenderC,
		Transactions: txnsC,
	})
	assert.NoError(t, err)

	err = coordinator.HandleEvent(ctx, &TransactionsDelegatedEvent{
		Sender:       testSenderD,
		Transactions: txnsD,
	})
	assert.NoError(t, err)

	mocks.engineIntegration.On(
		"WriteLockAndDistributeStatesForTransaction",
		mock.Anything, // ctx
		mock.MatchedBy(privateTransactionMatcher(txnsA[0].ID, txnsC[0].ID, txnsD[0].ID, txnsA[1].ID, txnsC[1].ID, txnsD[1].ID)), //match both transactions from A, C and D
	).Return(nil)

	for i := range 7 {
		assert.NotEqual(t, uuid.Nil, assemblingTxnID)

		if assemblingTxnID == txnsB[0].ID {
			assert.LessOrEqual(t, i, 3)
			//Simulate the passage of time then trigger a heartbeat
			mocks.clock.Advance(5001) //because NewCoordinatorForUnitTest sets the assemble timeout to be 5000 TODO use a builder and make this more explicit
			requestTimeoutIntervalEvent := &transaction.RequestTimeoutIntervalEvent{}
			requestTimeoutIntervalEvent.TransactionID = assemblingTxnID
			assemblingTxnID = uuid.Nil
			err = coordinator.HandleEvent(ctx, requestTimeoutIntervalEvent)
			assert.NoError(t, err)

		} else {
			//Send a success
			assembleResponseEvent := &transaction.AssembleSuccessEvent{}
			assembleResponseEvent.TransactionID = assemblingTxnID
			assembleResponseEvent.RequestID = assembleRequestID
			switch assemblingTxnID {
			case txnsA[0].ID:
				assembleResponseEvent.PostAssembly = buildersA[0].BuildPostAssembly()
			case txnsA[1].ID:
				assembleResponseEvent.PostAssembly = buildersA[1].BuildPostAssembly()
			//no case for txnsB[0].ID because we trapped that in the if block above and emulated a timeout
			case txnsB[1].ID:
				assembleResponseEvent.PostAssembly = buildersB[1].BuildPostAssembly()
			case txnsC[0].ID:
				assembleResponseEvent.PostAssembly = buildersC[0].BuildPostAssembly()
			case txnsC[1].ID:
				assembleResponseEvent.PostAssembly = buildersC[1].BuildPostAssembly()
			case txnsD[0].ID:
				assembleResponseEvent.PostAssembly = buildersD[0].BuildPostAssembly()
			case txnsD[1].ID:
				assembleResponseEvent.PostAssembly = buildersD[1].BuildPostAssembly()

			}
			assemblingTxnID = uuid.Nil
			err = coordinator.propagateEventToTransaction(ctx, assembleResponseEvent)
			assert.NoError(t, err)
		}
	}

	//After the first round, we should have B0, C0 and D0 in endorsing state while A0 is in pooled state
	transactionsInEndorsing := coordinator.getTransactionsInStates(ctx, []transaction.State{transaction.State_Endorsement_Gathering})
	endorsingTransactionIDs := make([]uuid.UUID, len(transactionsInEndorsing))
	for i, txn := range transactionsInEndorsing {
		endorsingTransactionIDs[i] = txn.ID
	}

	assert.Len(t, transactionsInEndorsing, 6)
	assert.Contains(t, endorsingTransactionIDs, txnsA[0].ID)
	assert.Contains(t, endorsingTransactionIDs, txnsC[0].ID)
	assert.Contains(t, endorsingTransactionIDs, txnsD[0].ID)
	assert.Contains(t, endorsingTransactionIDs, txnsA[1].ID)
	assert.Contains(t, endorsingTransactionIDs, txnsC[1].ID)
	assert.Contains(t, endorsingTransactionIDs, txnsD[1].ID)

	//slow queue should get a look in occasionally

	transactionsInAssembling := coordinator.getTransactionsInStates(ctx, []transaction.State{transaction.State_Assembling})
	assert.Len(t, transactionsInAssembling, 1)
	assert.Equal(t, txnsB[0].ID, transactionsInAssembling[0].ID)

}

func TestSelectTransaction_FairnessAcrossSenders(t *testing.T) {
	// When there are multiple transactions from multiple senders arriving at roughly the same time, ensure that all senders get a fair look in
	ctx := context.Background()
	testSenderA := "alice@node1"
	testSenderB := "bob@node2"
	testSenderC := "carol@node3"
	testSenderD := "dave@node4"

	coordinator, mocks := NewCoordinatorForUnitTest(t, ctx, []string{testSenderA, testSenderB, testSenderC, testSenderD})

	var assemblingTxnID uuid.UUID
	var assembleRequestID uuid.UUID
	mocks.messageSender.On(
		"SendAssembleRequest",
		mock.Anything, // ctx
		mock.Anything, // sender
		mock.Anything, // transaction ID
		mock.Anything, // Idempotency key
		mock.Anything, // transactionPreassembly
	).Return(nil).Run(func(args mock.Arguments) {
		assemblingTxnID = args.Get(2).(uuid.UUID)
		assembleRequestID = args.Get(3).(uuid.UUID)
	})
	mocks.messageSender.On("SendEndorsementRequest",
		mock.Anything, // ctx
		mock.Anything, // idempotency key
		mock.Anything, // party
		mock.Anything, // attestation request
		mock.Anything, // transaction specification
		mock.Anything, // verifiers
		mock.Anything, // signatures
		mock.Anything, // input states
		mock.Anything, // output states
		mock.Anything, // info states
	).Return(nil).Maybe()

	//NOTE: this test is not sensitive to which order the transactions are selected in, only that we get one transaction from each sender in endorsing state before the next transaction from each sender is selected
	buildersA := testutil.NewPrivateTransactionBuilderListForTesting(2).Address(*coordinator.contractAddress).Sender(testSenderA)
	buildersB := testutil.NewPrivateTransactionBuilderListForTesting(2).Address(*coordinator.contractAddress).Sender(testSenderB)
	buildersC := testutil.NewPrivateTransactionBuilderListForTesting(2).Address(*coordinator.contractAddress).Sender(testSenderC)
	buildersD := testutil.NewPrivateTransactionBuilderListForTesting(2).Address(*coordinator.contractAddress).Sender(testSenderD)
	txnsA := buildersA.BuildSparse()
	txnsB := buildersB.BuildSparse()
	txnsC := buildersC.BuildSparse()
	txnsD := buildersD.BuildSparse()

	err := coordinator.HandleEvent(ctx, &TransactionsDelegatedEvent{
		Sender:       testSenderA,
		Transactions: txnsA,
	})
	assert.NoError(t, err)

	err = coordinator.HandleEvent(ctx, &TransactionsDelegatedEvent{
		Sender:       testSenderB,
		Transactions: txnsB,
	})
	assert.NoError(t, err)

	err = coordinator.HandleEvent(ctx, &TransactionsDelegatedEvent{
		Sender:       testSenderC,
		Transactions: txnsC,
	})
	assert.NoError(t, err)

	err = coordinator.HandleEvent(ctx, &TransactionsDelegatedEvent{
		Sender:       testSenderD,
		Transactions: txnsD,
	})
	assert.NoError(t, err)

	mocks.engineIntegration.On(
		"WriteLockAndDistributeStatesForTransaction",
		mock.Anything, // ctx
		mock.MatchedBy(privateTransactionMatcher(txnsA[0].ID, txnsB[0].ID, txnsC[0].ID, txnsD[0].ID)),
	).Return(nil)

	for range 4 {

		//Send a success
		assembleResponseEvent := &transaction.AssembleSuccessEvent{}
		assembleResponseEvent.TransactionID = assemblingTxnID
		assembleResponseEvent.RequestID = assembleRequestID

		switch assemblingTxnID {
		case txnsA[0].ID:
			assembleResponseEvent.PostAssembly = buildersA[0].BuildPostAssembly()
		case txnsB[0].ID:
			assembleResponseEvent.PostAssembly = buildersB[0].BuildPostAssembly()
		case txnsC[0].ID:
			assembleResponseEvent.PostAssembly = buildersC[0].BuildPostAssembly()
		case txnsD[0].ID:
			assembleResponseEvent.PostAssembly = buildersD[0].BuildPostAssembly()
		default:
			assert.Failf(t, "unexpected assembling transaction ID %s", assemblingTxnID.String())
		}

		assemblingTxnID = uuid.Nil
		err = coordinator.propagateEventToTransaction(ctx, assembleResponseEvent)
		assert.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, assemblingTxnID)

	}

	//After the first round, we should have one transaction from each sender in endorsing state and the remaning transaction from each sender either in assembling or pooled state.  We are not worried about which one is in assembling vs pooled because that can be random
	transactionsInEndorsing := coordinator.getTransactionsInStates(ctx, []transaction.State{transaction.State_Endorsement_Gathering})
	endorsingTransactionIDs := make([]uuid.UUID, len(transactionsInEndorsing))
	for i, txn := range transactionsInEndorsing {
		endorsingTransactionIDs[i] = txn.ID
	}

	assert.Len(t, transactionsInEndorsing, 4)
	assert.Contains(t, endorsingTransactionIDs, txnsA[0].ID)
	assert.Contains(t, endorsingTransactionIDs, txnsB[0].ID)
	assert.Contains(t, endorsingTransactionIDs, txnsC[0].ID)
	assert.Contains(t, endorsingTransactionIDs, txnsD[0].ID)

	//do another round of 4 transactions and ensure we get the next transactions from each sender
	mocks.engineIntegration.On(
		"WriteLockAndDistributeStatesForTransaction",
		mock.Anything, // ctx
		mock.MatchedBy(privateTransactionMatcher(txnsA[1].ID, txnsB[1].ID, txnsC[1].ID, txnsD[1].ID)), //match the second transactions from each sender
	).Return(nil)

	for i := range 4 {

		//Send a success
		assembleResponseEvent := &transaction.AssembleSuccessEvent{}
		assembleResponseEvent.TransactionID = assemblingTxnID
		assembleResponseEvent.RequestID = assembleRequestID

		switch assemblingTxnID {
		case txnsA[1].ID:
			assembleResponseEvent.PostAssembly = buildersA[1].BuildPostAssembly()
		case txnsB[1].ID:
			assembleResponseEvent.PostAssembly = buildersB[1].BuildPostAssembly()
		case txnsC[1].ID:
			assembleResponseEvent.PostAssembly = buildersC[1].BuildPostAssembly()
		case txnsD[1].ID:
			assembleResponseEvent.PostAssembly = buildersD[1].BuildPostAssembly()
		default:
			assert.Failf(t, "unexpected assembling transaction ID %s", assemblingTxnID.String())
		}
		assemblingTxnID = uuid.Nil
		err = coordinator.propagateEventToTransaction(ctx, assembleResponseEvent)
		assert.NoError(t, err)
		// on the last iteration, we don't expect a new transaction to be selected
		if i < 3 {
			assert.NotEqual(t, uuid.Nil, assemblingTxnID)
		}

	}

	transactionsInEndorsing = coordinator.getTransactionsInStates(ctx, []transaction.State{transaction.State_Endorsement_Gathering})
	endorsingTransactionIDs = make([]uuid.UUID, len(transactionsInEndorsing))
	for i, txn := range transactionsInEndorsing {
		endorsingTransactionIDs[i] = txn.ID
	}

	assert.Len(t, transactionsInEndorsing, 8)
	assert.Contains(t, endorsingTransactionIDs, txnsA[1].ID)
	assert.Contains(t, endorsingTransactionIDs, txnsB[1].ID)
	assert.Contains(t, endorsingTransactionIDs, txnsC[1].ID)
	assert.Contains(t, endorsingTransactionIDs, txnsD[1].ID)

}

//TODO test that, in spite of the fairness across senders, that if one particular sender has delegated a transaction a long time ago then a whole bunch of senders come in with new transactions, the old transaction should be selected first
