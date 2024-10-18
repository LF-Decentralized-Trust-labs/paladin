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
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/kaleido-io/paladin/core/mocks/privatetxnmgrmocks"

	pb "github.com/kaleido-io/paladin/core/pkg/proto/sequence"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type FakeEngine interface {
	Invoke(ctx context.Context) error
	HandleTransactionAssembledEvent(ctx context.Context, event *pb.TransactionAssembledEvent)
	HandleTransactionEndorsedEvent(ctx context.Context, event *pb.TransactionEndorsedEvent) error
	HandleTransactionConfirmedEvent(ctx context.Context, event *pb.TransactionConfirmedEvent) error
	HandleTransactionRevertedEvent(ctx context.Context, event *pb.TransactionRevertedEvent) error
	ApproveEndorsement(ctx context.Context, endorsementRequest ptmgrtypes.EndorsementRequest) (bool, error)
	DelegateTransaction(ctx context.Context, message *pb.DelegateTransaction)
}

type fakeEngine struct {
	nodeID         string
	sequencer      ptmgrtypes.Sequencer
	currentState   string
	transportLayer *fakeTransportLayer
	t              *testing.T
}

func newFakeEngine(t *testing.T, nodeID string, sequencer ptmgrtypes.Sequencer, seedState string, transportLayer *fakeTransportLayer) *fakeEngine {
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
	newState := tktypes.NewBytes32FromSlice(tktypes.RandBytes(32))
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
	f.sequencer.AssignTransaction(ctx, txnID.String())

	//initiate an endorsement flow
	//TODO - for more complex scenarios, may need to revert rather than endorse some transactions but for now, we just tell all sequencers that the transaction is endorsed
	err = f.transportLayer.PublishEvent(ctx, &pb.TransactionEndorsedEvent{
		TransactionId: txnID.String(),
	})

	return err
}

func (f *fakeEngine) HandleTransactionAssembledEvent(ctx context.Context, event *pb.TransactionAssembledEvent) {
	ctx = log.WithLogField(ctx, "node", f.nodeID)

	f.currentState = event.OutputStateId[0]
	f.sequencer.HandleTransactionAssembledEvent(ctx, event)

}

func (f *fakeEngine) HandleTransactionConfirmedEvent(ctx context.Context, event *pb.TransactionConfirmedEvent) error {
	ctx = log.WithLogField(ctx, "node", f.nodeID)

	return f.sequencer.HandleTransactionConfirmedEvent(ctx, event)
}

func (f *fakeEngine) HandleTransactionEndorsedEvent(ctx context.Context, event *pb.TransactionEndorsedEvent) error {
	ctx = log.WithLogField(ctx, "node", f.nodeID)

	return f.sequencer.HandleTransactionEndorsedEvent(ctx, event)
}

func (f *fakeEngine) HandleTransactionRevertedEvent(ctx context.Context, event *pb.TransactionRevertedEvent) error {
	ctx = log.WithLogField(ctx, "node", f.nodeID)

	return f.sequencer.HandleTransactionRevertedEvent(ctx, event)
}

func (f *fakeEngine) ApproveEndorsement(ctx context.Context, endorsementRequest ptmgrtypes.EndorsementRequest) (bool, error) {
	ctx = log.WithLogField(ctx, "node", f.nodeID)

	// Implement the logic for approving an endorsement request.
	return f.sequencer.ApproveEndorsement(ctx, endorsementRequest)
}

func (f *fakeEngine) DelegateTransaction(ctx context.Context, message *pb.DelegateTransaction) {
	ctx = log.WithLogField(ctx, "node", f.nodeID)

	f.sequencer.AssignTransaction(ctx, message.TransactionId)
}

// Test cases to assert the emergent behaviour when multiple concurrent copies of the sequencer are running
// as in a distributed system.
type fakeTransportLayer struct {
	//map of sequencer to nodeID
	engines map[string]FakeEngine
	t       *testing.T
}

// PublishEvent implements ptmgrtypes.Publisher.
func (f *fakeTransportLayer) PublishEvent(ctx context.Context, event interface{}) error {
	log.L(ctx).Info("PublishEvent")

	switch event := event.(type) {
	case *pb.TransactionBlockedEvent:
	case *pb.TransactionAssembledEvent:
		for _, engine := range f.engines {
			engine.HandleTransactionAssembledEvent(ctx, event)
		}
	case *pb.TransactionEndorsedEvent:
		for _, engine := range f.engines {
			err := engine.HandleTransactionEndorsedEvent(ctx, event)
			require.NoError(f.t, err)
		}
	default:
		panic("unimplemented event type ")
	}
	return nil
}

func NewFakeTransportLayer(t *testing.T) *fakeTransportLayer {
	return &fakeTransportLayer{
		engines: make(map[string]FakeEngine),
		t:       t,
	}
}

type fakeDelegator struct {
	//map of sequencer to nodeID
	engines map[string]FakeEngine
	t       *testing.T
	nodeID  string
}

// Delegate implements ptmgrtypes.Delegator.
func (f *fakeDelegator) Delegate(ctx context.Context, txnID, delegate string) error {

	log.L(ctx).Info("DelegateTransaction")
	engine := f.engines[delegate]
	engine.DelegateTransaction(ctx, &pb.DelegateTransaction{
		TransactionId:    txnID,
		DelegatingNodeId: f.nodeID,
		DelegateNodeId:   delegate,
	})
	return nil
}

func NewFakeDelegator(t *testing.T, nodeID string) *fakeDelegator {
	return &fakeDelegator{
		engines: make(map[string]FakeEngine),
		t:       t,
		nodeID:  nodeID,
	}
}

func TestConcurrentSequencing(t *testing.T) {
	t.Skip("This test needs to be fixed since recent code changes but it is very complex and I am not sure if it gives us any value now that we have the engine component test.")
	// 3 nodes, concurrently assemble transactions that consume the output of each other
	// check that all transactions are dispatched to the same node
	// unless there is a break in the chain long enough for the previous transactions to be confirmed
	ctx := context.Background()
	// log.SetLevel("debug")

	node1ID := uuid.New().String()
	node2ID := uuid.New().String()
	node3ID := uuid.New().String()
	log.L(ctx).Infof("node1ID %s", node1ID)
	log.L(ctx).Infof("node2ID %s", node2ID)
	log.L(ctx).Infof("node3ID %s", node3ID)

	dispatcher1Mock := privatetxnmgrmocks.NewDispatcher(t)
	dispatcher2Mock := privatetxnmgrmocks.NewDispatcher(t)
	dispatcher3Mock := privatetxnmgrmocks.NewDispatcher(t)

	publisher1Mock := privatetxnmgrmocks.NewPublisher(t)
	publisher2Mock := privatetxnmgrmocks.NewPublisher(t)
	publisher3Mock := privatetxnmgrmocks.NewPublisher(t)

	transportWriterMock := privatetxnmgrmocks.NewTransportWriter(t)
	internodeTransportLayer := NewFakeTransportLayer(t)

	seedState := tktypes.NewBytes32FromSlice(tktypes.RandBytes(32))

	node1Sequencer := NewSequencer(node1ID,
		publisher1Mock,
		transportWriterMock,
	)
	node1Sequencer.SetDispatcher(dispatcher1Mock)
	node1Engine := newFakeEngine(t, node1ID, node1Sequencer, seedState.String(), internodeTransportLayer)

	node2Sequencer := NewSequencer(node2ID,
		publisher2Mock,
		transportWriterMock,
	)
	node2Sequencer.SetDispatcher(dispatcher2Mock)
	node2Engine := newFakeEngine(t, node2ID, node2Sequencer, seedState.String(), internodeTransportLayer)

	node3Sequencer := NewSequencer(node3ID,
		publisher3Mock,
		transportWriterMock,
	)
	node3Sequencer.SetDispatcher(dispatcher3Mock)
	node3Engine := newFakeEngine(t, node3ID, node3Sequencer, seedState.String(), internodeTransportLayer)

	transportWriterMock.On("SendDelegateTransactionMessage", mock.Anything, mock.Anything, node1ID).Run(func(args mock.Arguments) {
		transactionID := args.Get(1).(string)
		node1Sequencer.AssignTransaction(ctx, transactionID)
	}).Return(nil).Maybe()

	transportWriterMock.On("SendDelegateTransactionMessage", mock.Anything, mock.Anything, node2ID).Run(func(args mock.Arguments) {
		transactionID := args.Get(1).(string)
		node2Sequencer.AssignTransaction(ctx, transactionID)
	}).Return(nil).Maybe()

	transportWriterMock.On("SendDelegateTransactionMessage", mock.Anything, mock.Anything, node3ID).Run(func(args mock.Arguments) {
		transactionID := args.Get(1).(string)
		node3Sequencer.AssignTransaction(ctx, transactionID)
	}).Return(nil).Maybe()

	testTransactionInvoker1 := newtestTransactionInvoker(node1ID, "testTransactionInvoker1", node1Engine)
	testTransactionInvoker2 := newtestTransactionInvoker(node2ID, "testTransactionInvoker2", node2Engine)
	testTransactionInvoker3 := newtestTransactionInvoker(node3ID, "testTransactionInvoker3", node3Engine)

	targetNumberOfTransactions := 10
	dispatcher1Mock.On("DispatchTransactions", mock.Anything, mock.Anything).Return(nil).Times(targetNumberOfTransactions)
	dispatcher2Mock.On("DispatchTransactions", mock.Anything, mock.Anything).Return(errors.New("should not dispatch to node 2")).Maybe()
	dispatcher3Mock.On("DispatchTransactions", mock.Anything, mock.Anything).Return(errors.New("should not dispatch to node 3")).Maybe()

	var wg sync.WaitGroup

	wg.Add(3)
	invoked := make(chan bool, targetNumberOfTransactions)

	go func() {
		testTransactionInvoker1.run(t, func() {
			invoked <- true
		})
		wg.Done()
	}()

	go func() {
		testTransactionInvoker2.run(t, func() {
			invoked <- true
		})
		wg.Done()
	}()

	go func() {
		testTransactionInvoker3.run(t, func() {
			invoked <- true

		})
		wg.Done()
	}()

	testTransactionInvoker1.next <- true

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

				testTransactionInvoker1.next <- true
			case 1:
				log.L(ctx).Info("passing baton to 2")

				testTransactionInvoker2.next <- true
			case 2:
				log.L(ctx).Info("passing baton to 3")

				testTransactionInvoker3.next <- true

			}
		}
	}

	testTransactionInvoker1.stop(t)
	testTransactionInvoker2.stop(t)
	testTransactionInvoker3.stop(t)

	dispatcher1Mock.AssertExpectations(t)

}

type testTransactionInvoker struct {
	name    string
	nodeID  string
	engine  FakeEngine
	next    chan bool
	stopMsg chan bool
}

func (a *testTransactionInvoker) stop(_ *testing.T) {
	a.stopMsg <- true
}
func (a *testTransactionInvoker) run(t *testing.T, postInvoke func()) {
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

func newtestTransactionInvoker(nodeId string, name string, engine FakeEngine) *testTransactionInvoker {
	return &testTransactionInvoker{
		name:    name,
		nodeID:  nodeId,
		engine:  engine,
		next:    make(chan bool, 1),
		stopMsg: make(chan bool, 1),
	}
}
