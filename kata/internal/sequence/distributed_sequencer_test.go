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
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
	"github.com/kaleido-io/paladin/kata/mocks/commsbusmocks"
	"github.com/kaleido-io/paladin/kata/mocks/sequencemocks"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto/sequence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Test cases to assert the emergent behaviour when multiple concurrent copies of the sequencer are running
// as in a distributed system.
func TestDistributedSequencer(t *testing.T) {
	// when 2 transactions attempt to claim the same state id, then
	// both sequencers agree on which one emerges as the winner
	// and which one is reassembled

	ctx := context.Background()
	node1ID := uuid.New()
	node1Sequencer, commsBusBrokerMock1, persistenceMock1 := newSequencerForTesting(t, node1ID)
	node2ID := uuid.New()
	node2Sequencer, commsBusBrokerMock2, persistenceMock2 := newSequencerForTesting(t, node2ID)

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
		ID:     txn1ID,
		NodeID: node1ID,
	}
	txn2 := transactionstore.Transaction{
		ID:     txn2ID,
		NodeID: node2ID,
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

func newSequencerForTesting(t *testing.T, nodeID uuid.UUID) (Sequencer, *commsbusmocks.Broker, *sequencemocks.Persistence) {

	brokerMock := commsbusmocks.NewBroker(t)
	commsBusMock := commsbusmocks.NewCommsBus(t)
	commsBusMock.On("Broker").Return(brokerMock)
	persistenceMock := sequencemocks.NewPersistence(t)

	return &sequencer{
		nodeID:      nodeID,
		persistence: persistenceMock,
		commsBus:    commsBusMock,
	}, brokerMock, persistenceMock
}
