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

package privatetxnmgr

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/kaleido-io/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/kaleido-io/paladin/core/mocks/privatetxnmgrmocks"
	pb "github.com/kaleido-io/paladin/core/pkg/proto/sequence"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSequencerGraphOfOne(t *testing.T) {
	// Transactions that are added to a sequencer's graph and have no dependencies on other transactions
	// are immediately moved to the dispatch stage on the current node as soon as they are endorsed
	ctx := context.Background()
	node1ID := uuid.New().String()
	txn1ID := uuid.New()
	node1Sequencer, node1SequencerMockDependencies := newSequencerForTesting(t, node1ID, false)
	node1Sequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		NodeId:        node1ID,
		TransactionId: txn1ID.String(),
	})

	node1Sequencer.AssignTransaction(ctx, txn1ID.String())

	testSigner := tktypes.RandHex(32)

	node1SequencerMockDependencies.dispatcherMock.On("DispatchTransactions", ctx, ptmgrtypes.DispatchableTransactions{testSigner: []string{txn1ID.String()}}).Return(nil).Once()

	err := node1Sequencer.HandleTransactionDispatchResolvedEvent(ctx, &pb.TransactionDispatchResolvedEvent{
		TransactionId: txn1ID.String(),
		Signer:        testSigner,
	})
	require.NoError(t, err)
	err = node1Sequencer.HandleTransactionEndorsedEvent(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txn1ID.String(),
	})
	require.NoError(t, err)
	node1SequencerMockDependencies.dispatcherMock.AssertExpectations(t)
}

func TestSequencerTwoGraphsOfOne(t *testing.T) {
	// Transactions that are added to a sequencer's graph and have no dependencies on other transactions
	// are immediately moved to the dispatch stage on the current node as soon as they are endorsed
	// further transactions that are dependant on dispatched transactions are also dispatched
	ctx := context.Background()
	node1ID := uuid.New().String()
	txn1ID := uuid.New()
	txn2ID := uuid.New()
	stateID := tktypes.NewBytes32FromSlice(tktypes.RandBytes(32))
	node1Sequencer, node1SequencerMockDependencies := newSequencerForTesting(t, node1ID, false)
	node1Sequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		NodeId:        node1ID,
		TransactionId: txn1ID.String(),
		OutputStateId: []string{stateID.String()},
	})

	node1Sequencer.AssignTransaction(ctx, txn1ID.String())

	testSigner := tktypes.RandHex(32)

	node1SequencerMockDependencies.dispatcherMock.On("DispatchTransactions", ctx, ptmgrtypes.DispatchableTransactions{testSigner: []string{txn1ID.String()}}).Return(nil).Once()
	err := node1Sequencer.HandleTransactionDispatchResolvedEvent(ctx, &pb.TransactionDispatchResolvedEvent{
		TransactionId: txn1ID.String(),
		Signer:        testSigner,
	})
	require.NoError(t, err)
	err = node1Sequencer.HandleTransactionEndorsedEvent(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txn1ID.String(),
	})
	require.NoError(t, err)

	//now add a second transaction that is dependant on the first (before the first is confirmed)
	node1Sequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		NodeId:        node1ID,
		TransactionId: txn2ID.String(),
		InputStateId:  []string{stateID.String()},
	})

	node1Sequencer.AssignTransaction(ctx, txn2ID.String())
	require.NoError(t, err)

	node1SequencerMockDependencies.dispatcherMock.On("DispatchTransactions", ctx, ptmgrtypes.DispatchableTransactions{testSigner: []string{txn2ID.String()}}).Return(nil).Once()

	err = node1Sequencer.HandleTransactionDispatchResolvedEvent(ctx, &pb.TransactionDispatchResolvedEvent{
		TransactionId: txn2ID.String(),
		Signer:        testSigner,
	})
	require.NoError(t, err)
	err = node1Sequencer.HandleTransactionEndorsedEvent(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txn2ID.String(),
	})
	require.NoError(t, err)
}

func TestSequencerLocalUnendorsedDependency(t *testing.T) {
	// Transactions that are added to a sequencer's graph and have dependencies on other transactions that are also
	// managed by the same sequencer, will be moved to the dispatch stage as soon as they are endorsed and
	// all of their dependencies are also endorsed and dispatched
	ctx := context.Background()
	node1ID := uuid.New().String()
	txn1ID := uuid.New()
	txn2ID := uuid.New()
	stateID := tktypes.NewBytes32FromSlice(tktypes.RandBytes(32))
	node1Sequencer, node1SequencerMockDependencies := newSequencerForTesting(t, node1ID, false)

	node1Sequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		NodeId:        node1ID,
		TransactionId: txn1ID.String(),
		OutputStateId: []string{stateID.String()},
	})

	node1Sequencer.AssignTransaction(ctx, txn1ID.String())

	node1Sequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		NodeId:        node1ID,
		TransactionId: txn2ID.String(),
		InputStateId:  []string{stateID.String()},
	})

	node1Sequencer.AssignTransaction(ctx, txn2ID.String())

	testSigner := tktypes.RandHex(32)
	err := node1Sequencer.HandleTransactionDispatchResolvedEvent(ctx, &pb.TransactionDispatchResolvedEvent{
		TransactionId: txn2ID.String(),
		Signer:        testSigner,
	})
	require.NoError(t, err)
	err = node1Sequencer.HandleTransactionEndorsedEvent(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txn2ID.String(),
	})
	require.NoError(t, err)
	// add the mock for dispatch now because we need to assert that it is called but not before txn1 is endorsed
	node1SequencerMockDependencies.dispatcherMock.On("DispatchTransactions", ctx, ptmgrtypes.DispatchableTransactions{testSigner: []string{txn1ID.String(), txn2ID.String()}}).Return(nil).Once()

	err = node1Sequencer.HandleTransactionDispatchResolvedEvent(ctx, &pb.TransactionDispatchResolvedEvent{
		TransactionId: txn1ID.String(),
		Signer:        testSigner,
	})
	require.NoError(t, err)

	//now endorse txn1 and expect that both txn1 and txn2 are dispatched
	err = node1Sequencer.HandleTransactionEndorsedEvent(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txn1ID.String(),
	})
	require.NoError(t, err)

	node1SequencerMockDependencies.dispatcherMock.AssertExpectations(t)
}

func TestSequencerRemoteDependency(t *testing.T) {
	// Transactions that are added to a sequencer's graph and have dependencies on another transaction that is
	// managed by another sequencer, will be moved to that other sequencer as soon as they are assembled
	ctx := context.Background()
	localNodeId := uuid.New().String()
	remoteNodeId := uuid.New().String()

	txn1ID := uuid.New()
	txn2ID := uuid.New()

	stateID := tktypes.NewBytes32FromSlice(tktypes.RandBytes(32))

	//create a sequencer for the local node
	node1Sequencer, node1SequencerMockDependencies := newSequencerForTesting(t, localNodeId, false)

	// First transaction (the minter of a given state) is assembled on the remote node
	node1Sequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		TransactionId: txn1ID.String(),
		NodeId:        remoteNodeId,
		OutputStateId: []string{stateID.String()},
	})

	//TODO assert that it is the correct node that is delegated to
	node1SequencerMockDependencies.transportWriterMock.On("SendDelegateTransactionMessage", mock.Anything, txn2ID.String(), remoteNodeId).Return(nil)

	node1Sequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		TransactionId: txn2ID.String(),
		NodeId:        localNodeId,
		InputStateId:  []string{stateID.String()},
	})

	//Second transaction (the spender of that state) is assembled on the local node
	node1Sequencer.AssignTransaction(ctx, txn2ID.String())

	//We shouldn't see any dispatch, from the local sequencer, even when both transactions are endorsed
	err := node1Sequencer.HandleTransactionEndorsedEvent(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txn1ID.String(),
	})
	require.NoError(t, err)

	err = node1Sequencer.HandleTransactionEndorsedEvent(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txn2ID.String(),
	})
	require.NoError(t, err)

}

func TestSequencerTransitiveRemoteDependency(t *testing.T) {
	// Transactions that are added to a sequencer's graph and have dependencies on another transaction that is
	// managed by another sequencer, will be moved to that other sequencer as soon as they are assembled
	// even when the dependency is itself dependent on another transaction and has already been delegated too
	// i.e. make sure that we don't assume that the assemblyNodeID is the same as the delegatingNodeID
	ctx := context.Background()
	localNodeId := uuid.New().String()
	remotenode1ID := uuid.New().String()
	remoteNode2Id := uuid.New().String()

	txn1ID := uuid.New()
	txn2ID := uuid.New()
	txn3ID := uuid.New()

	stateIDA := tktypes.NewBytes32FromSlice(tktypes.RandBytes(32))

	stateIDB := tktypes.NewBytes32FromSlice(tktypes.RandBytes(32))

	//create a sequencer for the local node
	node1Sequencer, node1SequencerMockDependencies := newSequencerForTesting(t, localNodeId, false)

	// First transaction (the minter of a given state) is assembled on the remote node
	node1Sequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		TransactionId: txn1ID.String(),
		NodeId:        remotenode1ID,
		OutputStateId: []string{stateIDA.String()},
	})

	// Second transaction (the spender of that state and minter of a new state) is assembled on another remote node
	node1Sequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		TransactionId: txn2ID.String(),
		NodeId:        remoteNode2Id,
		InputStateId:  []string{stateIDA.String()},
		OutputStateId: []string{stateIDB.String()},
	})

	err := node1Sequencer.HandleTransactionDelegatedEvent(ctx, &pb.TransactionDelegatedEvent{
		TransactionId:    txn2ID.String(),
		DelegatingNodeId: remoteNode2Id,
		DelegateNodeId:   remotenode1ID,
	})
	assert.NoError(t, err)

	//Third transaction (the spender of the output of the second transaction) is assembled on the local node
	node1Sequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		TransactionId: txn3ID.String(),
		NodeId:        localNodeId,
		InputStateId:  []string{stateIDB.String()},
	})

	// Should see an event relinquishing ownership of this this transaction to the first remote node
	// even though the transaction's direct dependency is on the second remote node
	//TODO assert that it is the correct node that is delegated to
	node1SequencerMockDependencies.transportWriterMock.On("SendDelegateTransactionMessage", mock.Anything, txn3ID.String(), remotenode1ID).Return(nil)

	node1Sequencer.AssignTransaction(ctx, txn3ID.String())

	//We shouldn't see any dispatch, from the local sequencer, even when both transactions are endorsed
	err = node1Sequencer.HandleTransactionEndorsedEvent(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txn1ID.String(),
	})
	require.NoError(t, err)

	err = node1Sequencer.HandleTransactionEndorsedEvent(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txn2ID.String(),
	})
	require.NoError(t, err)
}

func TestSequencerTransitiveRemoteDependencyTiming(t *testing.T) {
	//TODO test the following case
	// txn1 is assembled on node1
	// txn2 is assembled on node2 and has a dependency on txn1 and is delegated to remoteNode1
	// txn3 is assembled on node3 and has a dependency on txn2 but the node3 node hasn't yet recieved the event to notify it that txn2 has been delegated to node1
	// It is valid for node3 to delegate to node2 and it is node 2's responsibiilty to onward delegate to node1

	ctx := context.Background()
	localNodeId := uuid.New().String()
	remotenode1ID := uuid.New().String()
	remoteNode2Id := uuid.New().String()

	txn1ID := uuid.New()
	txn2ID := uuid.New()
	txn3ID := uuid.New()

	stateIDA := tktypes.NewBytes32FromSlice(tktypes.RandBytes(32))

	stateIDB := tktypes.NewBytes32FromSlice(tktypes.RandBytes(32))

	//create a sequencer for the local node
	node1Sequencer, node1SequencerMockDependencies := newSequencerForTesting(t, localNodeId, false)

	// First transaction (the minter of a given state) is assembled on the remote node
	node1Sequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		TransactionId: txn1ID.String(),
		NodeId:        remotenode1ID,
		OutputStateId: []string{stateIDA.String()},
	})

	// Second transaction (the spender of that state and minter of a new state) is assembled on another remote node
	node1Sequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		TransactionId: txn2ID.String(),
		NodeId:        remoteNode2Id,
		InputStateId:  []string{stateIDA.String()},
		OutputStateId: []string{stateIDB.String()},
	})

	//Third transaction (the spender of the output of the second transaction) is assembled on the local node
	//but the local node has not been notified that txn2 has been delegated to remoteNode1 yet
	node1Sequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		TransactionId: txn3ID.String(),
		NodeId:        localNodeId,
		InputStateId:  []string{stateIDB.String()},
	})

	// Should see an event relinquishing ownership of this this transaction to the second remote node
	//TODO assert that it is the correct node that is delegated to
	node1SequencerMockDependencies.transportWriterMock.On("SendDelegateTransactionMessage", mock.Anything, txn3ID.String(), remoteNode2Id).Return(nil)

	node1Sequencer.AssignTransaction(ctx, txn3ID.String())

	//We shouldn't see any dispatch, from the local sequencer, even when both transactions are endorsed
	err := node1Sequencer.HandleTransactionEndorsedEvent(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txn1ID.String(),
	})
	assert.NoError(t, err)

	err = node1Sequencer.HandleTransactionEndorsedEvent(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txn2ID.String(),
	})
	assert.NoError(t, err)

	//Now a 4th node comes along and delegates txn4 (which has a dependency on txn3)
	// this 4th node (remoteNode3) is in the same possition as the local node was.
	// it has not been notified that txn3 has been delegated to remoteNode2
	// and so it delegates txn4 to localNode
	// local node is now in the possition that remoteNode2 was in above
	// i.e. we expect localnode to forward the delegation to remoteNode2
	remoteNode3Id := uuid.New()

	txn4ID := uuid.New()

	stateIDC := tktypes.NewBytes32FromSlice(tktypes.RandBytes(32))

	node1Sequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		TransactionId: txn4ID.String(),
		NodeId:        remoteNode3Id.String(),
		InputStateId:  []string{stateIDB.String()},
		OutputStateId: []string{stateIDC.String()},
	})

	node1SequencerMockDependencies.transportWriterMock.On("SendDelegateTransactionMessage", ctx, txn4ID.String(), remoteNode2Id).Return(nil)

	node1Sequencer.AssignTransaction(ctx, txn4ID.String())

}

func TestSequencerMultipleRemoteDependencies(t *testing.T) {
	// Transactions that are added to a sequencer's graph and have dependencies on multiple other transactions that are
	// managed by multiple other sequencers, will be moved to the blocked stage until all bar one of their dependencies are
	// committed
	ctx := context.Background()
	localNodeId := uuid.New().String()
	remotenode1ID := uuid.New().String()
	remoteNode2Id := uuid.New().String()

	newTransactionID := uuid.New()
	dependency1TransactionID := uuid.New()
	dependency2TransactionID := uuid.New()

	stateID1 := tktypes.NewBytes32FromSlice(tktypes.RandBytes(32))

	stateID2 := tktypes.NewBytes32FromSlice(tktypes.RandBytes(32))

	//create a sequencer for the local node
	localNodeSequencer, localNodeSequencerMockDependencies := newSequencerForTesting(t, localNodeId, false)
	publisherMock1 := localNodeSequencerMockDependencies.publisherMock

	// First transaction (the minter of a given state) is assembled on the remote node
	localNodeSequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		TransactionId: dependency1TransactionID.String(),
		NodeId:        remotenode1ID,
		OutputStateId: []string{stateID1.String()},
	})

	// Second transaction (the minter of the other state) is assembled on a different remote node
	localNodeSequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		TransactionId: dependency2TransactionID.String(),
		NodeId:        remoteNode2Id,
		OutputStateId: []string{stateID2.String()},
	})

	// Should see an event moving this transaction to the blocked stage
	publisherMock1.On("PublishTransactionBlockedEvent", ctx, newTransactionID.String()).Return(nil)

	// new transaction (the spender of that states) is assembled on the local node

	localNodeSequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		TransactionId: newTransactionID.String(),
		NodeId:        localNodeId,
		InputStateId:  []string{stateID1.String(), stateID2.String()},
	})

	localNodeSequencer.AssignTransaction(ctx, newTransactionID.String())

	publisherMock1.AssertExpectations(t)

	//We shouldn't see any dispatch, from the local sequencer, even when both transactions are endorsed
	err := localNodeSequencer.HandleTransactionEndorsedEvent(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: dependency1TransactionID.String(),
	})
	require.NoError(t, err)

	err = localNodeSequencer.HandleTransactionEndorsedEvent(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: dependency2TransactionID.String(),
	})
	require.NoError(t, err)

	err = localNodeSequencer.HandleTransactionEndorsedEvent(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: newTransactionID.String(),
	})
	require.NoError(t, err)

	// once all bar one transaction is confirmed, should see the dependant transaction being delegated
	// Should see an event relinquishing ownership of this this transaction
	localNodeSequencerMockDependencies.transportWriterMock.On("SendDelegateTransactionMessage", mock.Anything, newTransactionID.String(), remotenode1ID).Return(nil)

	err = localNodeSequencer.HandleTransactionConfirmedEvent(ctx, &pb.TransactionConfirmedEvent{
		TransactionId: dependency2TransactionID.String(),
	})
	require.NoError(t, err)
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
	nodeID := uuid.New().String()
	txn1ID := uuid.New()
	stateID := tktypes.NewBytes32FromSlice(tktypes.RandBytes(32))
	node1Sequencer, _ := newSequencerForTesting(t, nodeID, false)

	//with no other information, a sequencer should have no reason not to approve endorsement
	approved, err := node1Sequencer.ApproveEndorsement(ctx, ptmgrtypes.EndorsementRequest{
		TransactionID: txn1ID.String(),
		InputStates:   []string{stateID.String()},
	})
	require.NoError(t, err)
	assert.True(t, approved)
}

func TestSequencerApproveEndorsementForRemoteTransaction(t *testing.T) {

	//in this test, we do the check after we have seen the assembled event
	ctx := context.Background()
	nodeID := uuid.New().String()
	remoteNodeID := uuid.New()

	txn1ID := uuid.New()
	stateID := tktypes.NewBytes32FromSlice(tktypes.RandBytes(32))

	node1Sequencer, _ := newSequencerForTesting(t, nodeID, false)
	node1Sequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		NodeId:        remoteNodeID.String(),
		TransactionId: txn1ID.String(),
		InputStateId:  []string{stateID.String()},
	})

	approved, err := node1Sequencer.ApproveEndorsement(ctx, ptmgrtypes.EndorsementRequest{
		TransactionID: txn1ID.String(),
		InputStates:   []string{stateID.String()},
	})
	require.NoError(t, err)
	assert.True(t, approved)
}

func TestSequencerApproveEndorsementDoubleSpendAvoidance(t *testing.T) {

	ctx := context.Background()
	nodeID := uuid.New().String()
	stateID := tktypes.NewBytes32FromSlice(tktypes.RandBytes(32))

	txn1ID := uuid.New()
	txn2ID := uuid.New()
	node1Sequencer, _ := newSequencerForTesting(t, nodeID, false)

	approved, err := node1Sequencer.ApproveEndorsement(ctx, ptmgrtypes.EndorsementRequest{
		TransactionID: txn1ID.String(),
		InputStates:   []string{stateID.String()},
	})
	require.NoError(t, err)
	assert.True(t, approved)

	approved, err = node1Sequencer.ApproveEndorsement(ctx, ptmgrtypes.EndorsementRequest{
		TransactionID: txn2ID.String(),
		InputStates:   []string{stateID.String()},
	})
	require.NoError(t, err)
	assert.False(t, approved)
}

func TestSequencerApproveEndorsementReleaseStateOnRevert(t *testing.T) {

	ctx := context.Background()
	nodeID := uuid.New().String()
	remoteNodeID := uuid.New()
	stateID := tktypes.NewBytes32FromSlice(tktypes.RandBytes(32))

	txn1ID := uuid.New()
	txn2ID := uuid.New()
	node1Sequencer, _ := newSequencerForTesting(t, nodeID, false)
	node1Sequencer.HandleTransactionAssembledEvent(ctx, &pb.TransactionAssembledEvent{
		NodeId:        remoteNodeID.String(),
		TransactionId: txn1ID.String(),
		InputStateId:  []string{stateID.String()},
	})

	//with no other information, a sequencer should have no reason not to approve endorsement
	approved, err := node1Sequencer.ApproveEndorsement(ctx, ptmgrtypes.EndorsementRequest{
		TransactionID: txn1ID.String(),
		InputStates:   []string{stateID.String()},
	})
	require.NoError(t, err)
	assert.True(t, approved)

	err = node1Sequencer.HandleTransactionRevertedEvent(ctx, &pb.TransactionRevertedEvent{
		TransactionId: txn1ID.String(),
	})
	require.NoError(t, err)

	approved, err = node1Sequencer.ApproveEndorsement(ctx, ptmgrtypes.EndorsementRequest{
		TransactionID: txn2ID.String(),
		InputStates:   []string{stateID.String()},
	})
	require.NoError(t, err)
	assert.True(t, approved)
}

type sequencerMockDependencies struct {
	resolverMock        *privatetxnmgrmocks.ContentionResolver
	dispatcherMock      *privatetxnmgrmocks.Dispatcher
	transportWriterMock *privatetxnmgrmocks.TransportWriter
	publisherMock       *privatetxnmgrmocks.Publisher
}

func newSequencerForTesting(t *testing.T, nodeID string, mockResolver bool) (ptmgrtypes.Sequencer, sequencerMockDependencies) {

	publisherMock := privatetxnmgrmocks.NewPublisher(t)
	dispatcherMock := privatetxnmgrmocks.NewDispatcher(t)
	transportWriterMock := privatetxnmgrmocks.NewTransportWriter(t)
	if mockResolver {
		resolverMock := privatetxnmgrmocks.NewContentionResolver(t)
		return &sequencer{
				nodeID:                      nodeID,
				resolver:                    resolverMock,
				dispatcher:                  dispatcherMock,
				graph:                       NewGraph(),
				unconfirmedStatesByID:       make(map[string]*unconfirmedState),
				unconfirmedTransactionsByID: make(map[string]*transaction),
				stateSpenders:               make(map[string]string),
				transportWriter:             transportWriterMock,
			},
			sequencerMockDependencies{
				resolverMock,
				dispatcherMock,
				transportWriterMock,
				publisherMock,
			}
	} else {
		newSequencer := NewSequencer(
			nodeID,
			publisherMock,
			transportWriterMock,
		)
		newSequencer.SetDispatcher(dispatcherMock)
		return newSequencer,
			sequencerMockDependencies{
				nil,
				dispatcherMock,
				transportWriterMock,
				publisherMock,
			}
	}

}

//TODO test that the right thing happens when I delegate a dependent transaction to a node but that node (unbeknown to me yet) has already delegated the depencency elsewhere
