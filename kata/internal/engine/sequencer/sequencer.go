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
	"sync"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto/sequence"
)

// an ordered list of transactions that are handed over to the dispatcher to be submitted to the base ledger in that order
type Sequence []*transactionstore.Transaction

func NewSequencer(
	nodeID uuid.UUID,

	publisher types.Publisher,
	delegator types.Delegator,

	/*
		dispatcher is the reciever of the sequenced transactions and will be responsible for submitting them to the base ledger in the correct order
	*/
	dispatcher types.Dispatcher,

) types.Sequencer {
	return &sequencer{
		publisher:                   publisher,
		dispatcher:                  dispatcher,
		nodeID:                      nodeID,
		resolver:                    NewContentionResolver(),
		graph:                       NewGraph(),
		unconfirmedStatesByID:       make(map[string]*unconfirmedState),
		unconfirmedTransactionsByID: make(map[string]*transaction),
		stateSpenders:               make(map[string]string),
		delegator:                   delegator,
	}
}

type blockingTransaction struct {
	transactionID string
	nodeID        string
}

type blockedTransaction struct {
	transactionID string
	blockedBy     []blockingTransaction
}

// a delegatable transaction is one that has only one dependency on a transaction that is owned by another node
type delegatableTransaction struct {
	transactionID string
	nodeId        string
}

type unconfirmedState struct {
	stateID              string
	mintingTransactionID string
}

type transaction struct {
	id               string
	sequencingNodeID string
	assemblerNodeID  string
	endorsed         bool
	inputStates      []string
	outputStates     []string
}

type sequencer struct {
	nodeID                      uuid.UUID
	publisher                   types.Publisher
	delegator                   types.Delegator
	resolver                    ContentionResolver
	dispatcher                  types.Dispatcher
	graph                       Graph
	blockedTransactions         []*blockedTransaction // naive implementation of a list of blocked transaction TODO may need to make this a graph so that we can analyise knock on effects of unblocking a transaction but this simple list will do for now to prove out functional behaviour
	unconfirmedStatesByID       map[string]*unconfirmedState
	unconfirmedTransactionsByID map[string]*transaction
	stateSpenders               map[string]string /// map of state hash to our recognised spender of that state
	lock                        sync.Mutex        //put one massive mutex around the whole sequencer for now.  We can optimise this later

}

func (s *sequencer) evaluateGraph(ctx context.Context) error {

	dispatchableTransactions, err := s.graph.GetDispatchableTransactions(ctx)
	if err != nil {
		log.L(ctx).Errorf("Error getting dispatchable transactions: %s", err)
		return err
	}
	if len(dispatchableTransactions) == 0 {
		return nil
	}

	transactionUUIDs := make([]uuid.UUID, len(dispatchableTransactions))
	for i, txID := range dispatchableTransactions {
		transactionUUID, err := uuid.Parse(txID)
		if err != nil {
			log.L(ctx).Errorf("failed to parse transaction ID as uuid: %s", txID)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, txID)
		}
		transactionUUIDs[i] = transactionUUID
	}

	log.L(ctx).Debugf("Dispatching transactions: %v", transactionUUIDs)
	err = s.dispatcher.Dispatch(ctx, transactionUUIDs)
	if err != nil {
		log.L(ctx).Errorf("Error dispatching transaction: %s", err)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, err)
	}
	err = s.graph.RemoveTransactions(ctx, dispatchableTransactions)
	if err != nil {
		//TODO this is bad.  What can we do?
		// probably need to add more precise error reporting to RemoveTransactions function
		log.L(ctx).Errorf("Error dispatching transaction: %s", err)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, err)
	}
	return nil
}

func (s *sequencer) getUnconfirmedDependencies(ctx context.Context, txn transaction) ([]*transaction, error) {
	mintingTransactions := make([]*transaction, 0, len(txn.inputStates))
	for _, stateID := range txn.inputStates {
		unconfirmedState, ok := s.unconfirmedStatesByID[stateID]
		if !ok {
			//this state is already confirmed
			//TODO should we verify this is the case and not just the case that we have not learned about it yet?
			log.L(ctx).Debugf("State %s is already confirmed", stateID)
			continue
		}
		mintingTransactionID := unconfirmedState.mintingTransactionID
		mintingTransaction := s.unconfirmedTransactionsByID[mintingTransactionID]

		if mintingTransaction != nil {
			log.L(ctx).Debugf("Transaction %s is dependant on transaction %s on node %s", txn.id, mintingTransactionID, mintingTransaction.sequencingNodeID)
			mintingTransactions = append(mintingTransactions, mintingTransaction)
		}
	}
	return mintingTransactions, nil
}

func (s *sequencer) delegate(ctx context.Context, transactionId string, delegateNodeID string) error {
	err := s.delegator.Delegate(ctx, transactionId, delegateNodeID)
	if err != nil {
		log.L(ctx).Errorf("Error sending delegate transaction message: %s", err)
		return err
	}
	//update our local state to reflect that this transaction is now delegated
	txn, ok := s.unconfirmedTransactionsByID[transactionId]
	if !ok {
		//TODO could we recover from this error by adding the transaction to the map now?
		log.L(ctx).Errorf("failed to find minting transaction %s", transactionId)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, transactionId)
	}
	txn.sequencingNodeID = delegateNodeID
	return nil
}

func (s *sequencer) blockTransaction(ctx context.Context, transactionId string, blockedBy []blockingTransaction) error {
	s.blockedTransactions = append(s.blockedTransactions, &blockedTransaction{
		transactionID: transactionId,
		blockedBy:     blockedBy,
	})
	err := s.publisher.PublishEvent(ctx, &pb.TransactionBlockedEvent{
		TransactionId: transactionId,
	},
	)
	if err != nil {
		log.L(ctx).Errorf("Error sending delegate transaction message: %s", err)
		return err
	}
	return nil
}

func (s *sequencer) delegateIfAppropriate(ctx context.Context, transaction *transaction) (bool, error) {
	//if the transaction has any dependencies on transactions that are being managed by other nodes,
	//then we need to delegate this one to that remote node too
	unconfirmedDependencies, err := s.getUnconfirmedDependencies(ctx, *transaction)
	if err != nil {
		log.L(ctx).Errorf("Error getting unconfirmed dependencies: %s", err)
		return false, err
	}

	blockingNodeIDs := make(map[string]bool)
	blockedBy := make([]blockingTransaction, 0, len(unconfirmedDependencies))

	for _, dependency := range unconfirmedDependencies {
		blockingNodeIDs[dependency.sequencingNodeID] = true
		blockedBy = append(blockedBy, blockingTransaction{
			transactionID: dependency.id,
			nodeID:        dependency.sequencingNodeID,
		})
	}
	keys := make([]string, 0, len(blockingNodeIDs))
	for k := range blockingNodeIDs {
		keys = append(keys, k)
	}
	if len(keys) > 1 {

		// we have a dependency on transactions from multiple nodes
		// we can't delegate this transaction to multiple nodes, so we need to wait for the dependencies to be resolved
		log.L(ctx).Debugf("Transaction %s is blocked by transactions from multiple nodes %v", transaction.id, keys)
		err := s.blockTransaction(ctx, transaction.id, blockedBy)

		if err != nil {
			log.L(ctx).Errorf("Error blocking transaction: %s", err)
			return false, err
		}
		return true, nil
	}
	if len(keys) == 1 && keys[0] != s.nodeID.String() {
		// we are dependent on one other node so we can delegate
		log.L(ctx).Debugf("Transaction %s is dependant on transaction(s). Delagating to node %s", transaction.id, keys[0])
		err := s.delegate(ctx, transaction.id, keys[0])
		if err != nil {
			log.L(ctx).Errorf("Error delegating: %s", err)
			return false, err
		}

		return true, nil

	}
	//otherwise there are no dependencies ( or they are all on the local node) so we can just add the transaction to the graph

	return false, nil
}

func (s *sequencer) updateBlockedTransactions(ctx context.Context, event *pb.TransactionConfirmedEvent) {
	for _, blockedTransaction := range s.blockedTransactions {
		for i, dependency := range blockedTransaction.blockedBy {
			if dependency.transactionID == event.TransactionId {
				//TODO assuming the dependency transaction is only in the array once.  Can we assert this?
				blockedTransaction.blockedBy = append(blockedTransaction.blockedBy[:i], blockedTransaction.blockedBy[i+1:]...)
				continue
			}
		}
	}
}

func (s *sequencer) findDelegatableTransactions(ctx context.Context) []delegatableTransaction {
	delegatableTransactions := make([]delegatableTransaction, 0, len(s.blockedTransactions))
	//if I have any transactions in blocked that are dependant on this confirmed transaction, then I need to re-evaluate them

	for _, blockedTransaction := range s.blockedTransactions {
		blockingNodeIDs := make(map[string]bool)

		for _, dependency := range blockedTransaction.blockedBy {
			blockingNodeIDs[dependency.nodeID] = true
		}
		keys := make([]string, 0, len(blockingNodeIDs))
		for k := range blockingNodeIDs {
			keys = append(keys, k)
		}
		if len(keys) > 1 {
			// we still have a dependency on transactions from multiple nodes
			// we can't delegate this transaction to multiple nodes, so we need to wait for the dependencies to be resolved
			continue
		}
		if len(keys) == 1 && keys[0] != s.nodeID.String() {
			// we are dependent on one other node so we can delegate
			delegatableTransactions = append(delegatableTransactions, delegatableTransaction{
				transactionID: blockedTransaction.transactionID,
				nodeId:        keys[0],
			})
			continue

		}
		//otherwise there are no dependencies ( or they are all on the local node) so we can just add the transaction to the graph
		//TODO - is there any scenario ( including timing conditions) where the number of blockedBy could be zero? and we just dispatch it ourselves rather than delegating it?
	}
	return delegatableTransactions
}
func (s *sequencer) acceptTransaction(ctx context.Context, transaction *transaction) error {
	transaction.sequencingNodeID = s.nodeID.String()

	delegated, err := s.delegateIfAppropriate(ctx, transaction)
	if err != nil {
		log.L(ctx).Errorf("Error delegating transaction: %s", err)
		return err
	}
	if delegated {
		return nil
	}

	err = s.graph.AddTransaction(ctx, transaction.id, transaction.inputStates, transaction.outputStates)
	if err != nil {
		log.L(ctx).Errorf("Error adding transaction to graph: %s", err)
		return err
	}
	err = s.evaluateGraph(ctx)
	if err != nil {
		log.L(ctx).Errorf("Error evaluating graph: %s", err)
		return err
	}
	return nil
}

func (s *sequencer) HandleTransactionAssembledEvent(ctx context.Context, event *pb.TransactionAssembledEvent) error {
	log.L(ctx).Infof("Received transaction assembled event: %s", event.String())
	s.lock.Lock()
	defer s.lock.Unlock()
	//Record the new transaction
	s.unconfirmedTransactionsByID[event.TransactionId] = &transaction{
		id:               event.TransactionId,
		sequencingNodeID: event.NodeId, // assume it goes to its local sequencer until we hear otherwise
		assemblerNodeID:  event.NodeId,
		outputStates:     event.OutputStateId,
		inputStates:      event.InputStateId,
	}
	for _, unconfirmedStateID := range event.OutputStateId {
		s.unconfirmedStatesByID[unconfirmedStateID] = &unconfirmedState{
			stateID:              unconfirmedStateID,
			mintingTransactionID: event.TransactionId,
		}
	}

	//TODO this could be a dependency on a transaction that we have already added to our graph but
	// we didn't know about it when we added the dependant transaction

	return nil
}

func (s *sequencer) HandleTransactionEndorsedEvent(ctx context.Context, event *pb.TransactionEndorsedEvent) error {

	log.L(ctx).Infof("Received transaction endorsed event: %s", event.String())
	s.lock.Lock()
	defer s.lock.Unlock()
	if !s.graph.IncludesTransaction(event.TransactionId) {
		log.L(ctx).Debugf("Transaction %s does not exist locally", event.TransactionId)
		return nil
	}

	err := s.graph.RecordEndorsement(ctx, event.TransactionId)
	if err != nil {
		log.L(ctx).Errorf("Error recording endorsement: %s", err)
		return err
	}

	err = s.evaluateGraph(ctx)
	if err != nil {
		log.L(ctx).Errorf("Error evaluating graph: %s", err)
		return err
	}
	return nil
}

func (s *sequencer) HandleTransactionConfirmedEvent(ctx context.Context, event *pb.TransactionConfirmedEvent) error {
	log.L(ctx).Infof("Received transaction confirmed event: %s", event.String())
	s.lock.Lock()
	defer s.lock.Unlock()
	outputStateIDes := s.unconfirmedTransactionsByID[event.TransactionId].outputStates
	for _, outputStateID := range outputStateIDes {
		s.unconfirmedStatesByID[outputStateID] = nil
	}
	s.unconfirmedTransactionsByID[event.TransactionId] = nil

	s.updateBlockedTransactions(ctx, event)
	delegatableTransactions := s.findDelegatableTransactions(ctx)
	for _, delegatableTransaction := range delegatableTransactions {
		err := s.delegate(ctx, delegatableTransaction.transactionID, delegatableTransaction.nodeId)
		if err != nil {
			log.L(ctx).Errorf("Error delegating: %s", err)
			return err
		}
	}

	return nil
}

func (s *sequencer) HandleTransactionRevertedEvent(ctx context.Context, event *pb.TransactionRevertedEvent) error {
	//release the transaction's claim on any states
	s.lock.Lock()
	defer s.lock.Unlock()
	for state, spender := range s.stateSpenders {
		if spender == event.TransactionId {
			delete(s.stateSpenders, state)
		}
	}
	return nil
}

func (s *sequencer) HandleTransactionDelegatedEvent(ctx context.Context, event *pb.TransactionDelegatedEvent) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	transaction := s.unconfirmedTransactionsByID[event.TransactionId]
	if transaction == nil {
		log.L(ctx).Errorf("Transaction %s does not exist", event.TransactionId)
		//TODO - should we do something here?  Should we add the transaction to our map?
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, event.TransactionId)
	}
	log.L(ctx).Infof("HandleTransactionDelegatedEvent transaction %s delegated from %s to %s", event.TransactionId, event.DelegatingNodeId, event.DelegateNodeId)
	if transaction.sequencingNodeID != event.DelegatingNodeId {
		log.L(ctx).Debugf("local info about transaction %s out of date current sequening node is thought to be  %s", event.TransactionId, transaction.sequencingNodeID)
	}

	transaction.sequencingNodeID = event.DelegateNodeId
	return nil
}

func (s *sequencer) AssignTransaction(ctx context.Context, txnID string) error {
	log.L(ctx).Infof("AssignTransaction: %s", txnID)
	s.lock.Lock()
	defer s.lock.Unlock()

	//TODO we assume that the AssignTransaction message always comes _after_ the transactionAssembled event.  Is this safe to assume?  Should we pass the full transaction details on the delegateTransaction message? Or should we wait for the transactionAssembled event before actioning the delgation?
	txn, ok := s.unconfirmedTransactionsByID[txnID]
	if !ok {
		log.L(ctx).Errorf("Transaction %s does not exist", txnID)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, txnID)
	}

	// txn is passed as a pointer so that it can be updated in the acceptTransaction method
	// this is not thread safe but we assume that the sequencer is single threaded
	return s.acceptTransaction(ctx, txn)
}

func (s *sequencer) ApproveEndorsement(ctx context.Context, endorsementRequst types.EndorsementRequest) (bool, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	contentionFound := false
	for _, stateID := range endorsementRequst.InputStates {
		if stateSpender, ok := s.stateSpenders[stateID]; ok {
			if stateSpender != endorsementRequst.TransactionID {
				//another transaction is already recognised as the spender of this state
				contentionFound = true
				break
			}
		}
	}
	if contentionFound {
		return false, nil
	}
	//register this transaction as the spender of all the states
	for _, stateID := range endorsementRequst.InputStates {
		s.stateSpenders[stateID] = endorsementRequst.TransactionID
	}
	return true, nil
}
