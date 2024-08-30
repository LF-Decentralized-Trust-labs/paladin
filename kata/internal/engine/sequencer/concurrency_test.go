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

package sequencer

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/mocks/enginemocks"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto/sequence"
	ptypes "github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type Engine interface {
	Invoke(ctx context.Context) error
	OnTransactionAssembled(ctx context.Context, event *pb.TransactionAssembledEvent) error
	OnTransactionEndorsed(ctx context.Context, event *pb.TransactionEndorsedEvent) error
	OnTransactionConfirmed(ctx context.Context, event *pb.TransactionConfirmedEvent) error
	OnTransactionReverted(ctx context.Context, event *pb.TransactionRevertedEvent) error
	ApproveEndorsement(ctx context.Context, endorsementRequest types.EndorsementRequest) (bool, error)
	DelegateTransaction(ctx context.Context, message *pb.DelegateTransaction) error
}

type fakeEngine struct {
	nodeID         string
	sequencer      types.Sequencer
	currentState   string
	transportLayer *fakeTransportLayer
	t              *testing.T
}

func newFakeEngine(t *testing.T, nodeID string, sequencer types.Sequencer, seedState string, transportLayer *fakeTransportLayer) *fakeEngine {
	return &fakeEngine{
		nodeID:         nodeID,
		sequencer:      sequencer,
		currentState:   seedState,
		transportLayer: transportLayer,
		t:              t,
	}
}

func (f *fakeEngine) Invoke(ctx context.Context) error {

	ctx = log.WithLogField(ctx, "node", f.nodeID)
	// Assemble a transaction
	newState := ptypes.NewBytes32FromSlice(ptypes.RandBytes(32))
	txnID := uuid.New()
	log.L(ctx).Infof("Assembling transaction %s", txnID)

	// Tell all nodes about the transaction
	err := f.transportLayer.PublishEvent(ctx, &pb.TransactionAssembledEvent{
		TransactionId: txnID.String(),
		NodeId:        f.nodeID,
		InputStateId:  []string{f.currentState},
		OutputStateId: []string{newState.String()},
	})
	require.NoError(f.t, err)

	// Assign to the local sequencer
	err = f.sequencer.AssignTransaction(ctx, txnID.String())
	require.NoError(f.t, err)

	//initiate an endorsement flow
	//TODO - for more complex scenarios, may need to revert rather than endorse some transactions but for now, we just tell all sequencers that the transaction is endorsed
	err = f.transportLayer.PublishEvent(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txnID.String(),
	})

	return err
}

func (f *fakeEngine) OnTransactionAssembled(ctx context.Context, event *pb.TransactionAssembledEvent) error {
	ctx = log.WithLogField(ctx, "node", f.nodeID)

	f.currentState = event.OutputStateId[0]
	return f.sequencer.OnTransactionAssembled(ctx, event)

}

func (f *fakeEngine) OnTransactionConfirmed(ctx context.Context, event *pb.TransactionConfirmedEvent) error {
	ctx = log.WithLogField(ctx, "node", f.nodeID)

	return f.sequencer.OnTransactionConfirmed(ctx, event)
}

func (f *fakeEngine) OnTransactionEndorsed(ctx context.Context, event *pb.TransactionEndorsedEvent) error {
	ctx = log.WithLogField(ctx, "node", f.nodeID)

	return f.sequencer.OnTransactionEndorsed(ctx, event)
}

func (f *fakeEngine) OnTransactionReverted(ctx context.Context, event *pb.TransactionRevertedEvent) error {
	ctx = log.WithLogField(ctx, "node", f.nodeID)

	return f.sequencer.OnTransactionReverted(ctx, event)
}

func (f *fakeEngine) ApproveEndorsement(ctx context.Context, endorsementRequest types.EndorsementRequest) (bool, error) {
	ctx = log.WithLogField(ctx, "node", f.nodeID)

	// Implement the logic for approving an endorsement request.
	return f.sequencer.ApproveEndorsement(ctx, endorsementRequest)
}

func (f *fakeEngine) DelegateTransaction(ctx context.Context, message *pb.DelegateTransaction) error {
	ctx = log.WithLogField(ctx, "node", f.nodeID)

	return f.sequencer.AssignTransaction(ctx, message.TransactionId)
}

// Test cases to assert the emergent behaviour when multiple concurrent copies of the sequencer are running
// as in a distributed system.
type fakeTransportLayer struct {
	//map of sequencer to nodeID
	engines map[string]Engine
	t       *testing.T
}

func (f *fakeTransportLayer) addEngine(nodeID uuid.UUID, engine Engine) {
	f.engines[nodeID.String()] = engine
}

// PublishEvent implements Publisher.
func (f *fakeTransportLayer) PublishEvent(ctx context.Context, event proto.Message) error {
	log.L(ctx).Info("PublishEvent")

	switch event := event.(type) {
	case *pb.TransactionBlockedEvent:
	case *pb.TransactionAssembledEvent:
		for _, engine := range f.engines {
			err := engine.OnTransactionAssembled(ctx, event)
			require.NoError(f.t, err)
		}
	case *pb.TransactionEndorsedEvent:
		for _, engine := range f.engines {
			err := engine.OnTransactionEndorsed(ctx, event)
			require.NoError(f.t, err)
		}
	default:
		panic("unimplemented event type ")
	}
	return nil
}

func (f *fakeTransportLayer) PublishStageEvent(ctx context.Context, stageEvent *types.StageEvent) error {
	panic("unimplemented")
}

func NewFakeTransportLayer(t *testing.T) *fakeTransportLayer {
	return &fakeTransportLayer{
		engines: make(map[string]Engine),
		t:       t,
	}
}

type fakeDelegator struct {
	//map of sequencer to nodeID
	engines map[string]Engine
	t       *testing.T
	nodeID  string
}

func (f *fakeDelegator) addEngine(nodeID uuid.UUID, engine Engine) {
	f.engines[nodeID.String()] = engine
}

// Delegate implements Delegator.
func (f *fakeDelegator) Delegate(ctx context.Context, txnID, delegate string) error {

	log.L(ctx).Info("DelegateTransaction")
	engine := f.engines[delegate]
	err := engine.DelegateTransaction(ctx, &pb.DelegateTransaction{
		TransactionId:    txnID,
		DelegatingNodeId: f.nodeID,
		DelegateNodeId:   delegate,
	})
	require.NoError(f.t, err)
	return nil
}

func NewFakeDelegator(t *testing.T, nodeID string) *fakeDelegator {
	return &fakeDelegator{
		engines: make(map[string]Engine),
		t:       t,
		nodeID:  nodeID,
	}
}

func TestConcurrentSequencing(t *testing.T) {
	// 3 nodes, concurrently assemble transactions that consume the output of each other
	// check that all transactions are dispatched to the same node
	// unless there is a break in the chain long enough for the previous transactions to be confirmed
	ctx := context.Background()
	log.SetLevel("debug")

	node1ID := uuid.New()
	node2ID := uuid.New()
	node3ID := uuid.New()
	log.L(ctx).Infof("node1ID %s", node1ID)
	log.L(ctx).Infof("node2ID %s", node2ID)
	log.L(ctx).Infof("node3ID %s", node3ID)

	dispatcher1Mock := enginemocks.NewDispatcher(t)
	dispatcher2Mock := enginemocks.NewDispatcher(t)
	dispatcher3Mock := enginemocks.NewDispatcher(t)

	internodeTransportLayer := NewFakeTransportLayer(t)

	seedState := ptypes.NewBytes32FromSlice(ptypes.RandBytes(32))

	delegatorMock1 := NewFakeDelegator(t, node1ID.String())

	node1Sequencer := NewSequencer(node1ID,
		internodeTransportLayer,
		delegatorMock1,
		dispatcher1Mock,
	)
	node1Engine := newFakeEngine(t, node1ID.String(), node1Sequencer, seedState.String(), internodeTransportLayer)

	delegatorMock2 := NewFakeDelegator(t, node2ID.String())
	node2Sequencer := NewSequencer(node2ID,
		internodeTransportLayer,
		delegatorMock2,
		dispatcher2Mock,
	)
	node2Engine := newFakeEngine(t, node2ID.String(), node2Sequencer, seedState.String(), internodeTransportLayer)

	delegatorMock3 := NewFakeDelegator(t, node3ID.String())
	node3Sequencer := NewSequencer(node3ID,
		internodeTransportLayer,
		delegatorMock3,
		dispatcher3Mock,
	)
	node3Engine := newFakeEngine(t, node3ID.String(), node3Sequencer, seedState.String(), internodeTransportLayer)

	internodeTransportLayer.addEngine(node1ID, node1Engine)
	internodeTransportLayer.addEngine(node2ID, node2Engine)
	internodeTransportLayer.addEngine(node3ID, node3Engine)

	delegatorMock1.addEngine(node2ID, node2Engine)
	delegatorMock1.addEngine(node3ID, node3Engine)

	delegatorMock2.addEngine(node1ID, node1Engine)
	delegatorMock2.addEngine(node3ID, node3Engine)

	delegatorMock3.addEngine(node1ID, node1Engine)
	delegatorMock3.addEngine(node2ID, node2Engine)

	transactionInvoker1 := newTransactionInvoker(node1ID, "transactionInvoker1", node1Engine)
	transactionInvoker2 := newTransactionInvoker(node2ID, "transactionInvoker2", node2Engine)
	transactionInvoker3 := newTransactionInvoker(node3ID, "transactionInvoker3", node3Engine)

	targetNumberOfTransactions := 10
	dispatcher1Mock.On("Dispatch", mock.Anything, mock.Anything).Return(nil).Times(targetNumberOfTransactions)
	dispatcher2Mock.On("Dispatch", mock.Anything, mock.Anything).Return(errors.New("should not dispatch to node 2")).Maybe()
	dispatcher3Mock.On("Dispatch", mock.Anything, mock.Anything).Return(errors.New("should not dispatch to node 3")).Maybe()

	var wg sync.WaitGroup

	wg.Add(3)
	invoked := make(chan bool, targetNumberOfTransactions)

	go func() {
		transactionInvoker1.run(t, func() {
			invoked <- true
		})
		wg.Done()
	}()

	go func() {
		transactionInvoker2.run(t, func() {
			invoked <- true
		})
		wg.Done()
	}()

	go func() {
		transactionInvoker3.run(t, func() {
			invoked <- true

		})
		wg.Done()
	}()

	transactionInvoker1.next <- true

	numInvoked := 0

	// handle the signal that each routing emites to the channel after invoke
	for range invoked {

		numInvoked++
		log.L(ctx).Infof("%d invoked", numInvoked)

		if numInvoked == targetNumberOfTransactions {
			log.L(ctx).Info("Done")

			break
		} else {
			switch numInvoked % 3 {

			case 0:
				log.L(ctx).Info("passing baton to 1")

				transactionInvoker1.next <- true
			case 1:
				log.L(ctx).Info("passing baton to 2")

				transactionInvoker2.next <- true
			case 2:
				log.L(ctx).Info("passing baton to 3")

				transactionInvoker3.next <- true

			}
		}
	}

	transactionInvoker1.stop(t)
	transactionInvoker2.stop(t)
	transactionInvoker3.stop(t)

	dispatcher1Mock.AssertExpectations(t)

}

type transactionInvoker struct {
	name    string
	nodeID  uuid.UUID
	engine  Engine
	next    chan bool
	stopMsg chan bool
}

func (a *transactionInvoker) stop(_ *testing.T) {
	a.stopMsg <- true
}
func (a *transactionInvoker) run(t *testing.T, postInvoke func()) {
	ctx := log.WithLogField(context.Background(), "invoker", a.name)
	for {
		select {
		case <-a.next:
			log.L(ctx).Info("Invoking transaction")

			err := a.engine.Invoke(ctx)
			require.NoError(t, err)
			postInvoke()
		case <-a.stopMsg:
			return
		}
	}

}

func newTransactionInvoker(nodeId uuid.UUID, name string, engine Engine) *transactionInvoker {
	return &transactionInvoker{
		name:    name,
		nodeID:  nodeId,
		engine:  engine,
		next:    make(chan bool, 1),
		stopMsg: make(chan bool, 1),
	}
}
