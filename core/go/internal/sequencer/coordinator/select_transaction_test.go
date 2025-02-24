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
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSelectTransaction_PreserveOrderWithinSender(t *testing.T) {
	//transactions from a given sender are assembled, then dispatched in the order they were received
	ctx := context.Background()
	testSender := "alice@node1"

	coordinator, mocks := NewCoordinatorForUnitTest(t, ctx)

	// send a significant number of transactions from the same sender so that we don't luckily get the right order
	txns := newPrivateTransactionsForTesting(coordinator.contractAddress, 5)

	err := coordinator.addToDelegatedTransactions(ctx, testSender, txns)
	assert.NoError(t, err)

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

	//Assert that transactions are assembled in the correct order
	for i := 0; i < 5; i++ {

		mocks.stateIntegration.On(
			"WriteLockAndDistributeStatesForTransaction",
			mock.Anything, // ctx
			mock.MatchedBy(privateTransactionMatcher(txns[i].ID)), // transaction
		).Return(nil)
		err = coordinator.selectNextTransaction(ctx)
		assert.NoError(t, err)

		//Send a success
		assembleResponseEvent := &transaction.AssembleSuccessEvent{}
		assembleResponseEvent.TransactionID = assemblingTxnID
		assembleResponseEvent.RequestID = assembleRequestID
		assembleResponseEvent.PostAssembly = &components.TransactionPostAssembly{
			//TODO use a builder
		}
		coordinator.propagateEventToTransaction(ctx, assembleResponseEvent)

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

	coordinator, mocks := NewCoordinatorForUnitTest(t, ctx)

	// first transaction from sender B fails to assemble. It should be placed at the end of the slow queue.
	// so we get through the second transaction from other senders before coming back that transaction
	//NOTE: this test is not sensitive to which order the transactions are selected in, only that the slow queue is processed after the fast queue
	txnsA := newPrivateTransactionsForTesting(coordinator.contractAddress, 2)
	txnsB := newPrivateTransactionsForTesting(coordinator.contractAddress, 2)
	txnsC := newPrivateTransactionsForTesting(coordinator.contractAddress, 2)
	txnsD := newPrivateTransactionsForTesting(coordinator.contractAddress, 2)

	err := coordinator.addToDelegatedTransactions(ctx, testSenderA, txnsA)
	assert.NoError(t, err)

	err = coordinator.addToDelegatedTransactions(ctx, testSenderB, txnsB)
	assert.NoError(t, err)

	err = coordinator.addToDelegatedTransactions(ctx, testSenderC, txnsC)
	assert.NoError(t, err)

	err = coordinator.addToDelegatedTransactions(ctx, testSenderD, txnsD)
	assert.NoError(t, err)

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

	mocks.stateIntegration.On(
		"WriteLockAndDistributeStatesForTransaction",
		mock.Anything, // ctx
		mock.MatchedBy(privateTransactionMatcher(txnsA[0].ID, txnsC[0].ID, txnsD[0].ID, txnsA[1].ID, txnsC[1].ID, txnsD[1].ID)), //match both transactions from A, C and D
	).Return(nil)

	for i := 0; i < 7; i++ {
		err = coordinator.selectNextTransaction(ctx)
		assert.NoError(t, err)

		if assemblingTxnID == txnsB[0].ID {
			//Send an error
			assembleResponseEvent := &transaction.AssembleRevertResponseEvent{}
			assembleResponseEvent.TransactionID = assemblingTxnID
			assembleResponseEvent.RequestID = assembleRequestID
			assembleResponseEvent.PostAssembly = &components.TransactionPostAssembly{
				RevertReason: ptrTo("some revert reason"),
			}
			//TODO should this be calling coordinator.HandleEvent to be more black box?
			coordinator.propagateEventToTransaction(ctx, assembleResponseEvent)

		} else {
			//Send a success
			assembleResponseEvent := &transaction.AssembleSuccessEvent{}
			assembleResponseEvent.TransactionID = assemblingTxnID
			assembleResponseEvent.RequestID = assembleRequestID
			assembleResponseEvent.PostAssembly = &components.TransactionPostAssembly{
				//TODO use a builder
			}
			coordinator.propagateEventToTransaction(ctx, assembleResponseEvent)
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

}

func TestSelectTransaction_FairnessAcrossSenders(t *testing.T) {
	t.Skip()
	// When there are multiple transactions from multiple senders arriving at roughly the same time, ensure that all senders get a fair look in
	//TODO this test also tests the slow queue.  Remove that behavior as it is covered in a separate test now
	ctx := context.Background()
	testSenderA := "alice@node1"
	testSenderB := "bob@node2"
	testSenderC := "carol@node3"
	testSenderD := "dave@node4"

	coordinator, mocks := NewCoordinatorForUnitTest(t, ctx)

	// first transaction from sender A fails to assemble. It should be placed at the end of the slow queue.
	// so we get through the second transaction from other senders before coming back that transaction
	//NOTE: this test is not sensitive to which order the transactions are selected in, only that the slow queue is processed after the fast queue
	txnsA := newPrivateTransactionsForTesting(coordinator.contractAddress, 2)
	txnsB := newPrivateTransactionsForTesting(coordinator.contractAddress, 2)
	txnsC := newPrivateTransactionsForTesting(coordinator.contractAddress, 2)
	txnsD := newPrivateTransactionsForTesting(coordinator.contractAddress, 2)

	err := coordinator.addToDelegatedTransactions(ctx, testSenderA, txnsA)
	assert.NoError(t, err)

	err = coordinator.addToDelegatedTransactions(ctx, testSenderB, txnsB)
	assert.NoError(t, err)

	err = coordinator.addToDelegatedTransactions(ctx, testSenderC, txnsC)
	assert.NoError(t, err)

	err = coordinator.addToDelegatedTransactions(ctx, testSenderD, txnsD)
	assert.NoError(t, err)

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

	mocks.stateIntegration.On(
		"WriteLockAndDistributeStatesForTransaction",
		mock.Anything, // ctx
		mock.MatchedBy(privateTransactionMatcher(txnsB[0].ID, txnsC[0].ID, txnsD[0].ID)), //match the first transaction from B, C and D
	).Return(nil)

	for i := 0; i < 4; i++ {
		err = coordinator.selectNextTransaction(ctx)
		assert.NoError(t, err)

		if assemblingTxnID == txnsA[0].ID {
			//Send an error
			assembleResponseEvent := &transaction.AssembleRevertResponseEvent{}
			assembleResponseEvent.TransactionID = assemblingTxnID
			assembleResponseEvent.RequestID = assembleRequestID
			assembleResponseEvent.PostAssembly = &components.TransactionPostAssembly{
				RevertReason: ptrTo("some revert reason"),
			}
			//TODO should this be calling coordinator.HandleEvent to be more black box?
			coordinator.propagateEventToTransaction(ctx, assembleResponseEvent)

		} else {
			//Send a success
			assembleResponseEvent := &transaction.AssembleSuccessEvent{}
			assembleResponseEvent.TransactionID = assemblingTxnID
			assembleResponseEvent.RequestID = assembleRequestID
			assembleResponseEvent.PostAssembly = &components.TransactionPostAssembly{
				//TODO use a builder
			}
			coordinator.propagateEventToTransaction(ctx, assembleResponseEvent)
		}
	}

	//After the first round, we should have B0, C0 and D0 in endorsing state while A0 is in pooled state
	transactionsInEndorsing := coordinator.getTransactionsInStates(ctx, []transaction.State{transaction.State_Endorsement_Gathering})
	endorsingTransactionIDs := make([]uuid.UUID, len(transactionsInEndorsing))
	for i, txn := range transactionsInEndorsing {
		endorsingTransactionIDs[i] = txn.ID
	}

	assert.Len(t, transactionsInEndorsing, 3)
	assert.Contains(t, endorsingTransactionIDs, txnsB[0].ID)
	assert.Contains(t, endorsingTransactionIDs, txnsC[0].ID)
	assert.Contains(t, endorsingTransactionIDs, txnsD[0].ID)

	//do another round of 3 transactions and ensure we do not select A0
	mocks.stateIntegration.On(
		"WriteLockAndDistributeStatesForTransaction",
		mock.Anything, // ctx
		mock.MatchedBy(privateTransactionMatcher(txnsB[1].ID, txnsC[1].ID, txnsD[1].ID)), //match the second transactions from B, C and D
	).Return(nil)

	for i := 0; i < 3; i++ {
		err = coordinator.selectNextTransaction(ctx)
		assert.NoError(t, err)

		require.NotEqual(t, txnsA[0].ID, assemblingTxnID)
		//Send a success
		assembleResponseEvent := &transaction.AssembleSuccessEvent{}
		assembleResponseEvent.TransactionID = assemblingTxnID
		assembleResponseEvent.RequestID = assembleRequestID
		assembleResponseEvent.PostAssembly = &components.TransactionPostAssembly{
			//TODO use a builder
		}
		coordinator.propagateEventToTransaction(ctx, assembleResponseEvent)
	}

	transactionsInEndorsing = coordinator.getTransactionsInStates(ctx, []transaction.State{transaction.State_Endorsement_Gathering})
	endorsingTransactionIDs = make([]uuid.UUID, len(transactionsInEndorsing))
	for i, txn := range transactionsInEndorsing {
		endorsingTransactionIDs[i] = txn.ID
	}

	assert.Len(t, transactionsInEndorsing, 3)
	assert.Contains(t, endorsingTransactionIDs, txnsB[1].ID)
	assert.Contains(t, endorsingTransactionIDs, txnsC[1].ID)
	assert.Contains(t, endorsingTransactionIDs, txnsD[1].ID)

}

//TODO test that, in spite of the fairness across senders, that if one particular sender has delegated a transaction a long time ago then a whole bunch of senders come in with new transactions, the old transaction should be selected first
