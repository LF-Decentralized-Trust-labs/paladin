// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sequence

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/mocks/commsbusmocks"
	"github.com/kaleido-io/paladin/kata/mocks/sequencemocks"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto/sequence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSequencerGraphOfOne(t *testing.T) {
	// Transactions that are added to a sequencer's graph and have no dependencies on other transactions
	// are immediately moved to the dispatch stage on the current node as soon as they are endorsed
	ctx := context.Background()
	node1ID := uuid.New()
	txn1ID := uuid.New()
	node1Sequencer, node1SequencerMockDependencies := newSequencerForTesting(t, node1ID, false)
	err := node1Sequencer.OnTransactionAssembled(ctx, &pb.TransactionAssembledEvent{
		NodeId:        node1ID.String(),
		TransactionId: txn1ID.String(),
	})
	assert.NoError(t, err)

	node1SequencerMockDependencies.dispatcherMock.On("Dispatch", ctx, []uuid.UUID{txn1ID}).Return(nil).Once()
	err = node1Sequencer.OnTransactionEndorsed(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txn1ID.String(),
	})
	assert.NoError(t, err)
	node1SequencerMockDependencies.dispatcherMock.AssertExpectations(t)
}

func TestSequencerLocalDependency(t *testing.T) {
	// Transactions that are added to a sequencer's graph and have dependencies on other transactions that are also
	// managed by the same sequencer, will be moved to the dispatch stage as soon as they are endorsed and
	// all of their dependencies are also endorsed and dispatched
	ctx := context.Background()
	node1ID := uuid.New()
	txn1ID := uuid.New()
	txn2ID := uuid.New()
	stateHash := statestore.HashID{
		L: uuid.New(),
		H: uuid.New(),
	}
	node1Sequencer, node1SequencerMockDependencies := newSequencerForTesting(t, node1ID, false)
	err := node1Sequencer.OnTransactionAssembled(ctx, &pb.TransactionAssembledEvent{
		NodeId:          node1ID.String(),
		TransactionId:   txn1ID.String(),
		OutputStateHash: []string{stateHash.String()},
	})
	assert.NoError(t, err)

	err = node1Sequencer.OnTransactionAssembled(ctx, &pb.TransactionAssembledEvent{
		NodeId:         node1ID.String(),
		TransactionId:  txn2ID.String(),
		InputStateHash: []string{stateHash.String()},
	})
	assert.NoError(t, err)

	err = node1Sequencer.OnTransactionEndorsed(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txn2ID.String(),
	})
	assert.NoError(t, err)

	// add the mock for dispatch now because we need to assert that it is called but not before txn1 is endorsed
	node1SequencerMockDependencies.dispatcherMock.On("Dispatch", ctx, []uuid.UUID{txn1ID, txn2ID}).Return(nil).Once()

	//now endorse txn1 and expect that both txn1 and txn2 are dispatched
	err = node1Sequencer.OnTransactionEndorsed(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txn1ID.String(),
	})
	assert.NoError(t, err)

	node1SequencerMockDependencies.dispatcherMock.AssertExpectations(t)
}

func TestSequencerRemoteDependency(t *testing.T) {
	// Transactions that are added to a sequencer's graph and have dependencies on another transaction that is
	// managed by another sequencer, will be moved to that other sequencer as soon as they are assembled
	ctx := context.Background()
	localNodeId := uuid.New()
	remoteNodeId := uuid.New()

	txn1ID := uuid.New()
	txn2ID := uuid.New()

	stateHash := statestore.HashID{
		L: uuid.New(),
		H: uuid.New(),
	}

	//create a sequencer for the local node
	node1Sequencer, node1SequencerMockDependencies := newSequencerForTesting(t, localNodeId, false)
	commsBusBrokerMock1 := node1SequencerMockDependencies.brokerMock

	// First transaction (the minter of a given state) is assembled on the remote node
	err := node1Sequencer.OnTransactionAssembled(ctx, &pb.TransactionAssembledEvent{
		TransactionId:   txn1ID.String(),
		NodeId:          remoteNodeId.String(),
		OutputStateHash: []string{stateHash.String()},
	})
	assert.NoError(t, err)

	// Should see an event relinquishing ownership of this this transaction
	commsBusBrokerMock1.On("SendMessage", ctx, mock.Anything).Run(func(args mock.Arguments) {
		delegateTransactionMessage := args.Get(1).(commsbus.Message).Body.(*pb.DelegateTransaction)
		assert.Equal(t, txn2ID.String(), delegateTransactionMessage.TransactionId)
		assert.Equal(t, localNodeId.String(), delegateTransactionMessage.DelegatingNodeId)
		assert.Equal(t, remoteNodeId.String(), delegateTransactionMessage.DelegateNodeId)
	}).Return(nil)

	//Second transaction (the spender of that state) is assembled on the local node
	err = node1Sequencer.OnTransactionAssembled(ctx, &pb.TransactionAssembledEvent{
		TransactionId:  txn2ID.String(),
		NodeId:         localNodeId.String(),
		InputStateHash: []string{stateHash.String()},
	})
	assert.NoError(t, err)

	commsBusBrokerMock1.AssertExpectations(t)

	//We shouldn't see any dispatch, from the local sequencer, even when both transactions are endorsed
	err = node1Sequencer.OnTransactionEndorsed(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txn1ID.String(),
	})
	assert.NoError(t, err)

	err = node1Sequencer.OnTransactionEndorsed(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txn2ID.String(),
	})
	assert.NoError(t, err)

}

func TestSequencerMultipleRemoteDependencies(t *testing.T) {
	// Transactions that are added to a sequencer's graph and have dependencies on multiple other transactions that are
	// managed by multiple other sequencers, will be moved to the blocked stage until all bar one of their dependencies are
	// committed
	ctx := context.Background()
	localNodeId := uuid.New()
	remoteNode1Id := uuid.New()
	remoteNode2Id := uuid.New()

	newTransactionID := uuid.New()
	dependency1TransactionID := uuid.New()
	dependency2TransactionID := uuid.New()

	stateHash1 := statestore.HashID{
		L: uuid.New(),
		H: uuid.New(),
	}

	stateHash2 := statestore.HashID{
		L: uuid.New(),
		H: uuid.New(),
	}

	//create a sequencer for the local node
	localNodeSequencer, localNodeSequencerMockDependencies := newSequencerForTesting(t, localNodeId, false)
	commsBusBrokerMock1 := localNodeSequencerMockDependencies.brokerMock

	// First transaction (the minter of a given state) is assembled on the remote node
	err := localNodeSequencer.OnTransactionAssembled(ctx, &pb.TransactionAssembledEvent{
		TransactionId:   dependency1TransactionID.String(),
		NodeId:          remoteNode1Id.String(),
		OutputStateHash: []string{stateHash1.String()},
	})
	assert.NoError(t, err)

	// Second transaction (the minter of the other state) is assembled on a different remote node
	err = localNodeSequencer.OnTransactionAssembled(ctx, &pb.TransactionAssembledEvent{
		TransactionId:   dependency2TransactionID.String(),
		NodeId:          remoteNode2Id.String(),
		OutputStateHash: []string{stateHash2.String()},
	})
	assert.NoError(t, err)

	// Should see an event moving this transaction to the blocked stage
	commsBusBrokerMock1.On("PublishEvent", ctx, mock.Anything).Run(func(args mock.Arguments) {
		transactionBlockedEvent := args.Get(1).(commsbus.Event).Body.(*pb.TransactionBlockedEvent)
		assert.Equal(t, newTransactionID.String(), transactionBlockedEvent.TransactionId)
	}).Return(nil)

	// new transaction (the spender of that states) is assembled on the local node
	err = localNodeSequencer.OnTransactionAssembled(ctx, &pb.TransactionAssembledEvent{
		TransactionId:  newTransactionID.String(),
		NodeId:         localNodeId.String(),
		InputStateHash: []string{stateHash1.String(), stateHash2.String()},
	})
	assert.NoError(t, err)

	commsBusBrokerMock1.AssertExpectations(t)

	//We shouldn't see any dispatch, from the local sequencer, even when both transactions are endorsed
	err = localNodeSequencer.OnTransactionEndorsed(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: dependency1TransactionID.String(),
	})
	assert.NoError(t, err)

	err = localNodeSequencer.OnTransactionEndorsed(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: dependency2TransactionID.String(),
	})
	assert.NoError(t, err)

	err = localNodeSequencer.OnTransactionEndorsed(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: newTransactionID.String(),
	})
	assert.NoError(t, err)

	// once all bar one transaction is confirmed, should see the dependant transaction being delegated
	// Should see an event relinquishing ownership of this this transaction
	commsBusBrokerMock1.On("SendMessage", ctx, mock.Anything).Run(func(args mock.Arguments) {
		delegateTransactionMessage := args.Get(1).(commsbus.Message).Body.(*pb.DelegateTransaction)
		assert.Equal(t, newTransactionID.String(), delegateTransactionMessage.TransactionId)
		assert.Equal(t, localNodeId.String(), delegateTransactionMessage.DelegatingNodeId)
		assert.Equal(t, remoteNode1Id.String(), delegateTransactionMessage.DelegateNodeId)
	}).Return(nil)

	err = localNodeSequencer.OnTransactionConfirmed(ctx, &pb.TransactionConfirmedEvent{
		TransactionId: dependency2TransactionID.String(),
	})
	assert.NoError(t, err)
	// TODO I know I have a bug where I am leaving the transaction in the `blockedTransactions` array even after it has been delegated.  I need to fix that but first I need to write a test that proves it is wrong.
}

// TODO mode complex variations of TestSequencerMultipleRemoteDependencies where there are still multiple remainign dependency transactions but they are all on the same remote node and/or they are all on the local node
// timing conditions where the remote transactions themselves get delegated to another node or even get delegated to this local node

//Endorsement tests
// before endorsement is confirmed, the sequencer on the node local to the endorser is invoked to
//  a) record the endorsement
//  b) assert that the endorsement does not result in any contention with any transaction that we have already endorsed
//  make no assumption about what endorsement mode we are in ( e.g. we might be the notary, we might just one of a privacy group in a 100% endorsement)
//  all we can be sure is that we do not endorse 2 conflicting transactions
//  whichever one we see first is the winner in our eyes.
//  that might turn out to be wrong later.  The contention resolution alorithm might decide that the other one is the winner
//  and the first transaction will be re-assembled so in that case, we must retract our endorsement for it
//  and endorse the other one instead

func TestSequencerApproveEndorsement(t *testing.T) {

	ctx := context.Background()
	nodeID := uuid.New()
	txn1ID := uuid.New()
	stateHash := statestore.HashID{
		L: uuid.New(),
		H: uuid.New(),
	}
	node1Sequencer, _ := newSequencerForTesting(t, nodeID, false)

	//with no other information, a sequencer should have no reason not to approve endorsement
	approved, err := node1Sequencer.ApproveEndorsement(ctx, EndorsementRequest{
		transactionID: txn1ID.String(),
		inputStates:   []string{stateHash.String()},
	})
	assert.NoError(t, err)
	assert.True(t, approved)
}

func TestSequencerApproveEndorsementForRemoteTransaction(t *testing.T) {

	//in this test, we do the check after we have seen the assembled event
	ctx := context.Background()
	nodeID := uuid.New()
	remoteNodeID := uuid.New()

	txn1ID := uuid.New()
	stateHash := statestore.HashID{
		L: uuid.New(),
		H: uuid.New(),
	}

	node1Sequencer, _ := newSequencerForTesting(t, nodeID, false)
	err := node1Sequencer.OnTransactionAssembled(ctx, &pb.TransactionAssembledEvent{
		NodeId:         remoteNodeID.String(),
		TransactionId:  txn1ID.String(),
		InputStateHash: []string{stateHash.String()},
	})
	assert.NoError(t, err)

	approved, err := node1Sequencer.ApproveEndorsement(ctx, EndorsementRequest{
		transactionID: txn1ID.String(),
		inputStates:   []string{stateHash.String()},
	})
	assert.NoError(t, err)
	assert.True(t, approved)
}

func TestSequencerApproveEndorsementDoubleSpendAvoidance(t *testing.T) {

	ctx := context.Background()
	nodeID := uuid.New()
	stateHash := statestore.HashID{
		L: uuid.New(),
		H: uuid.New(),
	}

	txn1ID := uuid.New()
	txn2ID := uuid.New()
	node1Sequencer, _ := newSequencerForTesting(t, nodeID, false)

	approved, err := node1Sequencer.ApproveEndorsement(ctx, EndorsementRequest{
		transactionID: txn1ID.String(),
		inputStates:   []string{stateHash.String()},
	})
	assert.NoError(t, err)
	assert.True(t, approved)

	approved, err = node1Sequencer.ApproveEndorsement(ctx, EndorsementRequest{
		transactionID: txn2ID.String(),
		inputStates:   []string{stateHash.String()},
	})
	assert.NoError(t, err)
	assert.False(t, approved)
}

func TestSequencerApproveEndorsementReleaseStateOnRevert(t *testing.T) {

	ctx := context.Background()
	nodeID := uuid.New()
	remoteNodeID := uuid.New()
	stateHash := statestore.HashID{
		L: uuid.New(),
		H: uuid.New(),
	}

	txn1ID := uuid.New()
	txn2ID := uuid.New()
	node1Sequencer, _ := newSequencerForTesting(t, nodeID, false)
	err := node1Sequencer.OnTransactionAssembled(ctx, &pb.TransactionAssembledEvent{
		NodeId:         remoteNodeID.String(),
		TransactionId:  txn1ID.String(),
		InputStateHash: []string{stateHash.String()},
	})
	assert.NoError(t, err)

	//with no other information, a sequencer should have no reason not to approve endorsement
	approved, err := node1Sequencer.ApproveEndorsement(ctx, EndorsementRequest{
		transactionID: txn1ID.String(),
		inputStates:   []string{stateHash.String()},
	})
	assert.NoError(t, err)
	assert.True(t, approved)

	err = node1Sequencer.OnTransactionReverted(ctx, &pb.TransactionRevertedEvent{
		TransactionId: txn1ID.String(),
	})
	assert.NoError(t, err)

	approved, err = node1Sequencer.ApproveEndorsement(ctx, EndorsementRequest{
		transactionID: txn2ID.String(),
		inputStates:   []string{stateHash.String()},
	})
	assert.NoError(t, err)
	assert.True(t, approved)
}

// Test cases to assert the emergent behaviour when multiple concurrent copies of the sequencer are running
// as in a distributed system.
/*func TestSequencer(t *testing.T) {
	// when 2 transactions attempt to claim the same state id, then
	// both sequencers agree on which one emerges as the winner
	// and which one is reassembled

	ctx := context.Background()
	node1ID := uuid.New()
	node1Sequencer, node1SequencerMockDependencies := newSequencerForTesting(t, node1ID, false)
	commsBusBrokerMock1 := node1SequencerMockDependencies.brokerMock
	persistenceMock1 := node1SequencerMockDependencies.persistenceMock

	node2ID := uuid.New()
	node2Sequencer, node2SequencerMockDependencies := newSequencerForTesting(t, node2ID, false)
	commsBusBrokerMock2 := node2SequencerMockDependencies.brokerMock
	persistenceMock2 := node2SequencerMockDependencies.persistenceMock

	txn1ID := uuid.New()
	txn2ID := uuid.New()

	stateHash := statestore.HashID{
		L: uuid.New(),
		H: uuid.New(),
	}
	stateKnownByNode1 := statestore.State{
		Hash:      stateHash,
		ClaimedBy: &txn1ID,
	}

	stateKnownByNode2 := statestore.State{
		Hash:      stateHash,
		ClaimedBy: &txn2ID,
	}

	txn1 := transactionstore.Transaction{
		ID:               txn1ID,
		AssemblingNodeID: node1ID,
	}
	txn2 := transactionstore.Transaction{
		ID:               txn2ID,
		AssemblingNodeID: node2ID,
	}

	node1PersistedState := stateKnownByNode1
	node2PersistedState := stateKnownByNode2

	persistenceMock1.On("GetTransactionByID", ctx, txn1ID).Return(txn1, nil).Maybe()
	persistenceMock1.On("GetTransactionByID", ctx, txn2ID).Return(txn2, nil).Maybe()
	persistenceMock2.On("GetTransactionByID", ctx, txn1ID).Return(txn1, nil).Maybe()
	persistenceMock2.On("GetTransactionByID", ctx, txn2ID).Return(txn2, nil).Maybe()

	persistenceMock1.On("GetStateByHash", ctx, stateHash.String()).Return(stateKnownByNode1, nil).Maybe()
	persistenceMock2.On("GetStateByHash", ctx, stateHash.String()).Return(stateKnownByNode2, nil).Maybe()

	persistenceMock1.On("UpdateState", ctx, mock.Anything).Run(func(args mock.Arguments) {
		node1PersistedState = args.Get(1).(statestore.State)
	}).Return(nil).Maybe()
	persistenceMock2.On("UpdateState", ctx, mock.Anything).Run(func(args mock.Arguments) {
		node2PersistedState = args.Get(1).(statestore.State)
	}).Return(nil).Maybe()

	var node1StateClaimLostEvent *pb.StateClaimLostEvent = nil
	var node2StateClaimLostEvent *pb.StateClaimLostEvent = nil
	commsBusBrokerMock1.On("PublishEvent", ctx, mock.Anything).Run(func(args mock.Arguments) {
		node1StateClaimLostEvent = args.Get(1).(commsbus.Event).Body.(*pb.StateClaimLostEvent)
	}).Return(nil)
	commsBusBrokerMock2.On("PublishEvent", ctx, mock.Anything).Run(func(args mock.Arguments) {
		node2StateClaimLostEvent = args.Get(1).(commsbus.Event).Body.(*pb.StateClaimLostEvent)
	}).Return(nil).Maybe()

	isReassembleMessage := func(msg commsbus.Message) bool {
		_, ok := msg.Body.(*pb.ReassembleRequest)
		return ok
	}
	numberOfReassembleMessages := 0
	recordReassembleMessage := func(args mock.Arguments) {
		numberOfReassembleMessages++
	}

	commsBusBrokerMock1.On("SendMessage", ctx, mock.MatchedBy(isReassembleMessage)).Run(recordReassembleMessage).Return(nil).Maybe()
	commsBusBrokerMock2.On("SendMessage", ctx, mock.MatchedBy(isReassembleMessage)).Run(recordReassembleMessage).Return(nil).Maybe()

	// txn1 claims the state
	stateClaimEvent1 := &pb.StateClaimEvent{
		StateHash:     stateHash.String(),
		TransactionId: txn1ID.String(),
	}

	// node2 claims the state
	stateClaimEvent2 := &pb.StateClaimEvent{
		StateHash:     stateHash.String(),
		TransactionId: txn2ID.String(),
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		err := node1Sequencer.OnStateClaimEvent(ctx, stateClaimEvent2)
		assert.NoError(t, err)
	}()
	go func() {
		defer wg.Done()
		err := node2Sequencer.OnStateClaimEvent(ctx, stateClaimEvent1)
		assert.NoError(t, err)
	}()
	wg.Wait()

	//Assert that both nodes have eneded up with the same claimedBy recorded on the state
	assert.Equal(t, node1PersistedState.ClaimedBy, node2PersistedState.ClaimedBy)

	//assert that they both published the same loser
	assert.Equal(t, node1StateClaimLostEvent.TransactionId, node2StateClaimLostEvent.TransactionId)

	//assert that one of them send a re-assemble message
	assert.Equal(t, 1, numberOfReassembleMessages)
}

func TestSequencerLoopDetection(t *testing.T) {
	// When 2 transactions attempt to claim the set of states and one transaction wins the contention
	// resolution for one state, the other transaction wins the contention resolution for the other state
	// then we avoid the situation of an endless loop where both transactions are re-assembled

	//easiest way to define a behaviour that will not result in an endless loop of both transactions
	// is to define a behaviour that will not result in a single loop of both transactions
	// so we define a behaviour where one transaction wins the contention resolution for both states

	// given that the contention resolution is deterministic based on the UUID of the transactions and
	// the hash of the states and the desired behaviour is related to how we deal with content resolution
	// resulting in 2 different answers when 2 different state hashes are contested by the same 2 transaction uuids
	// there are 3 ways we could test this behaviour
	// 1. we could attempt to generate state hashes that we can predict will result in different outcomes of
	//    the contention resolution
	// 2. we could generate state hashes randomly but re-run the test enough times to get confidence that
	//    the probability of replicating those conditions is is so high, we can assume the test is valid
	// 3. we could mock the contention resolver to return different results based on the state hash
	// 1. is not feasible without some very complex code in the test case.  2. would increase the time taken to
	// run the test so maybe would be more approriate for system testing and 3. would be the easiest to implement
	// and would be the most reliable way to test the behaviour and given that we have goo unit test coverage of the
	// contention resolver, and good coverage of its integration with the sequencer in other tests, it is ok to mock that here

	ctx := context.Background()

	node1ID := uuid.New()
	node1Sequencer, node1SequencerMockDependencies := newSequencerForTesting(t, node1ID, false)
	commsBusBrokerMock1 := node1SequencerMockDependencies.brokerMock
	persistenceMock1 := node1SequencerMockDependencies.persistenceMock
	resolverMock1 := node1SequencerMockDependencies.resolverMock

	node2ID := uuid.New()
	node2Sequencer, node2SequencerMockDependencies := newSequencerForTesting(t, node2ID, false)
	commsBusBrokerMock2 := node2SequencerMockDependencies.brokerMock
	persistenceMock2 := node2SequencerMockDependencies.persistenceMock
	resolverMock2 := node2SequencerMockDependencies.resolverMock

	txn1ID := uuid.New()
	txn2ID := uuid.New()

	txn1 := transactionstore.Transaction{
		ID:               txn1ID,
		AssemblingNodeID: node1ID,
	}
	txn2 := transactionstore.Transaction{
		ID:               txn2ID,
		AssemblingNodeID: node2ID,
	}

	stateAHash := statestore.HashID{
		L: uuid.New(),
		H: uuid.New(),
	}
	stateAKnownByNode1 := statestore.State{
		Hash:      stateAHash,
		ClaimedBy: &txn1ID,
	}

	stateAKnownByNode2 := statestore.State{
		Hash:      stateAHash,
		ClaimedBy: &txn2ID,
	}

	stateBHash := statestore.HashID{
		L: uuid.New(),
		H: uuid.New(),
	}
	stateBKnownByNode1 := statestore.State{
		Hash:      stateBHash,
		ClaimedBy: &txn1ID,
	}

	stateBKnownByNode2 := statestore.State{
		Hash:      stateBHash,
		ClaimedBy: &txn2ID,
	}

	persistenceMock1.On("GetTransactionByID", ctx, txn1ID).Return(txn1, nil).Maybe()
	persistenceMock1.On("GetTransactionByID", ctx, txn2ID).Return(txn2, nil).Maybe()
	persistenceMock2.On("GetTransactionByID", ctx, txn1ID).Return(txn1, nil).Maybe()
	persistenceMock2.On("GetTransactionByID", ctx, txn2ID).Return(txn2, nil).Maybe()

	persistenceMock1.On("GetStateByHash", ctx, stateAHash.String()).Return(stateAKnownByNode1, nil).Maybe()
	persistenceMock2.On("GetStateByHash", ctx, stateAHash.String()).Return(stateAKnownByNode2, nil).Maybe()

	persistenceMock1.On("GetStateByHash", ctx, stateBHash.String()).Return(stateBKnownByNode1, nil).Maybe()
	persistenceMock2.On("GetStateByHash", ctx, stateBHash.String()).Return(stateBKnownByNode2, nil).Maybe()

	persistenceMock1.On("UpdateState", ctx, mock.Anything).Return(nil).Maybe()
	persistenceMock2.On("UpdateState", ctx, mock.Anything).Return(nil).Maybe()

	//T1 wins A on both nodes
	resolverMock1.On("Resolve", stateAHash.String(), mock.Anything, mock.Anything).Return(txn1ID.String(), nil)
	resolverMock2.On("Resolve", stateAHash.String(), mock.Anything, mock.Anything).Return(txn1ID.String(), nil)

	//T2 wins B on both nodes
	resolverMock1.On("Resolve", stateBHash.String(), mock.Anything, mock.Anything).Return(txn1ID.String(), nil)
	resolverMock2.On("Resolve", stateBHash.String(), mock.Anything, mock.Anything).Return(txn1ID.String(), nil)

	var node1StateClaimLostEvent *pb.StateClaimLostEvent = nil
	var node2StateClaimLostEvent *pb.StateClaimLostEvent = nil
	commsBusBrokerMock1.On("PublishEvent", ctx, mock.Anything).Run(func(args mock.Arguments) {
		node1StateClaimLostEvent = args.Get(1).(commsbus.Event).Body.(*pb.StateClaimLostEvent)
	}).Return(nil)
	commsBusBrokerMock2.On("PublishEvent", ctx, mock.Anything).Run(func(args mock.Arguments) {
		node2StateClaimLostEvent = args.Get(1).(commsbus.Event).Body.(*pb.StateClaimLostEvent)
	}).Return(nil).Maybe()

	isReassembleMessage := func(msg commsbus.Message) bool {
		_, ok := msg.Body.(*pb.ReassembleRequest)
		return ok
	}
	numberOfReassembleMessages := 0
	recordReassembleMessage := func(args mock.Arguments) {
		numberOfReassembleMessages++
	}

	commsBusBrokerMock1.On("SendMessage", ctx, mock.MatchedBy(isReassembleMessage)).Run(recordReassembleMessage).Return(nil).Maybe()
	commsBusBrokerMock2.On("SendMessage", ctx, mock.MatchedBy(isReassembleMessage)).Run(recordReassembleMessage).Return(nil).Maybe()

	stateClaimEvent1A := &pb.StateClaimEvent{
		StateHash:     stateAHash.String(),
		TransactionId: txn1ID.String(),
	}
	stateClaimEvent1B := &pb.StateClaimEvent{
		StateHash:     stateBHash.String(),
		TransactionId: txn1ID.String(),
	}
	stateClaimEvent2A := &pb.StateClaimEvent{
		StateHash:     stateAHash.String(),
		TransactionId: txn2ID.String(),
	}
	stateClaimEvent2B := &pb.StateClaimEvent{
		StateHash:     stateBHash.String(),
		TransactionId: txn2ID.String(),
	}

	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		err := node1Sequencer.OnStateClaimEvent(ctx, stateClaimEvent1A)
		assert.NoError(t, err)
	}()
	go func() {
		defer wg.Done()
		err := node2Sequencer.OnStateClaimEvent(ctx, stateClaimEvent2A)
		assert.NoError(t, err)
	}()
	go func() {
		defer wg.Done()
		err := node1Sequencer.OnStateClaimEvent(ctx, stateClaimEvent1B)
		assert.NoError(t, err)
	}()
	go func() {
		defer wg.Done()
		err := node2Sequencer.OnStateClaimEvent(ctx, stateClaimEvent2B)
		assert.NoError(t, err)
	}()
	wg.Wait()

	//Assert that both nodes have eneded up with the same claimedBy recorded on the state
	//assert.Equal(t, node1PersistedState.ClaimedBy, node2PersistedState.ClaimedBy)

	//assert that they both published the same loser
	assert.Equal(t, node1StateClaimLostEvent.TransactionId, node2StateClaimLostEvent.TransactionId)

	//assert that one of them send a re-assemble message
	assert.Equal(t, 1, numberOfReassembleMessages)

}
*/
type sequencerMockDependencies struct {
	brokerMock     *commsbusmocks.Broker
	resolverMock   *sequencemocks.ContentionResolver
	dispatcherMock *sequencemocks.Dispatcher
}

func newSequencerForTesting(t *testing.T, nodeID uuid.UUID, mockResolver bool) (Sequencer, sequencerMockDependencies) {

	brokerMock := commsbusmocks.NewBroker(t)
	commsBusMock := commsbusmocks.NewCommsBus(t)
	commsBusMock.On("Broker").Return(brokerMock).Maybe()
	dispatcherMock := sequencemocks.NewDispatcher(t)
	var resolverMock *sequencemocks.ContentionResolver = nil
	var resolver ContentionResolver
	if mockResolver {
		resolverMock = sequencemocks.NewContentionResolver(t)
		resolver = resolverMock
	} else {
		resolver = NewContentionResolver()
	}

	return &sequencer{
			nodeID:                      nodeID,
			commsBus:                    commsBusMock,
			resolver:                    resolver,
			dispatcher:                  dispatcherMock,
			graph:                       NewGraph(),
			unconfirmedStatesByHash:     make(map[string]*unconfirmedState),
			unconfirmedTransactionsByID: make(map[string]*unconfirmedTransaction),
			stateSpenders:               make(map[string]string),
		},
		sequencerMockDependencies{
			brokerMock,
			resolverMock,
			dispatcherMock,
		}

}

//TODO test that the right thing happens when I deletegate a dependent transaction to a node but that node (unbeknown to me yet) has already delegated the depencency elsewhere
