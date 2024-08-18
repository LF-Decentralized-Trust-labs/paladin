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

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto/sequence"
)

type EventSync interface {
	// most likely will be replaced with either the comms bus or some utility of the StageController framework
	Publish(event *commsbus.Event)
}

// an ordered list of transactions that are handed over to the dispatcher to be submitted to the base ledger in that order
type Sequence []*transactionstore.Transaction

type Dispatcher interface {
	// Dispatcher is the component that takes responsibility for submitting the transactions in the sequence to the base ledger in the correct order
	// most likely will be replaced with (or become an integration to) either the comms bus or some utility of the StageController framework
	Dispatch(context.Context, []uuid.UUID) error
}

type EndorsementRequest struct {
	transactionID string
	inputStates   []string
}
type Sequencer interface {
	OnTransactionAssembled(ctx context.Context, event *pb.TransactionAssembledEvent) error
	OnTransactionEndorsed(ctx context.Context, event *pb.TransactionEndorsedEvent) error
	OnTransactionConfirmed(ctx context.Context, event *pb.TransactionConfirmedEvent) error
	/* OnTransationReverted is emitted whenever a transaction has been rejected by any of the validation
	steps on any nodes or the base leddger contract. The transaction may or may not be reassembled after this
	event is emitted.
	*/
	OnTransactionReverted(ctx context.Context, event *pb.TransactionRevertedEvent) error
	ApproveEndorsement(ctx context.Context, endorsementRequest EndorsementRequest) (bool, error)
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
	stateHash            string
	mintingTransactionID string
}

type unconfirmedTransaction struct {
	transactionID    string
	sequencingNodeID string
	assemblerNodeID  string
	outputStates     []string
	inputStates      []string
}

type sequencer struct {
	nodeID                      uuid.UUID
	commsBus                    commsbus.CommsBus
	resolver                    ContentionResolver
	dispatcher                  Dispatcher
	graph                       Graph
	blockedTransactions         []*blockedTransaction // naive implementation of a list of blocked transaction TODO may need to make this a graph so that we can analyise knock on effects of unblocking a transaction but this simple list will do for now to prove out functional behaviour
	unconfirmedStatesByHash     map[string]*unconfirmedState
	unconfirmedTransactionsByID map[string]*unconfirmedTransaction
	stateSpenders               map[string]string /// map of state hash to our recognised spender of that state
}

func NewSequencer(eventSync EventSync) Sequencer {
	return &sequencer{}
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

	err = s.dispatcher.Dispatch(ctx, transactionUUIDs)
	if err != nil {
		log.L(ctx).Errorf("Error dispatching transaction: %s", err)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, err)
	}

	return nil
}

func (s *sequencer) publishStateClaimLostEvent(ctx context.Context, stateHash, transactionID string) {
	err := s.commsBus.Broker().PublishEvent(ctx, commsbus.Event{
		Body: &pb.StateClaimLostEvent{
			StateHash:     stateHash,
			TransactionId: transactionID,
		},
	})
	if err != nil {
		// TODO - what should we do here?  Should we retry?  Should we log and ignore?
		log.L(ctx).Errorf("Error publishing state claim lost event: %s", err)
	}
}

func (s *sequencer) sendReassembleMessage(ctx context.Context, transactionID string) {
	err := s.commsBus.Broker().SendMessage(ctx, commsbus.Message{
		Body: &pb.ReassembleRequest{
			TransactionId: transactionID,
		},
	})
	if err != nil {
		//TODO - what should we do here?  Should we retry?  Should we log and ignore?
		log.L(ctx).Errorf("Error sending reassemble message: %s", err)
	}
}

func (s *sequencer) getUnconfirmedDependencies(ctx context.Context, event *pb.TransactionAssembledEvent) ([]*unconfirmedTransaction, error) {
	mintingTransactions := make([]*unconfirmedTransaction, 0, len(event.InputStateHash))
	for _, stateHash := range event.InputStateHash {
		mintingTransactionID := s.unconfirmedStatesByHash[stateHash].mintingTransactionID
		mintingTransaction := s.unconfirmedTransactionsByID[mintingTransactionID]

		if mintingTransaction != nil {
			mintingTransactions = append(mintingTransactions, mintingTransaction)
		}
	}
	return mintingTransactions, nil
}

func (s *sequencer) delegate(ctx context.Context, transactionId string, delegateNodeID string) error {
	err := s.commsBus.Broker().SendMessage(ctx, commsbus.Message{
		Body: &pb.DelegateTransaction{
			TransactionId:    transactionId,
			DelegatingNodeId: s.nodeID.String(),
			DelegateNodeId:   delegateNodeID,
		},
	})
	if err != nil {
		log.L(ctx).Errorf("Error sending delegate transaction message: %s", err)
		return err
	}
	return nil
}

func (s *sequencer) blockTransaction(ctx context.Context, transactionId string, blockedBy []blockingTransaction) error {
	s.blockedTransactions = append(s.blockedTransactions, &blockedTransaction{
		transactionID: transactionId,
		blockedBy:     blockedBy,
	})
	err := s.commsBus.Broker().PublishEvent(ctx, commsbus.Event{
		Body: &pb.TransactionBlockedEvent{
			TransactionId: transactionId,
		},
	})
	if err != nil {
		log.L(ctx).Errorf("Error sending delegate transaction message: %s", err)
		return err
	}
	return nil
}

func (s *sequencer) delegateIfAppropriate(ctx context.Context, event *pb.TransactionAssembledEvent) (bool, error) {
	//if the transaction has any dependencies on transactions that are being managed by other nodes,
	//then we need to delegate this one to that remote node too
	unconfirmedDependencies, err := s.getUnconfirmedDependencies(ctx, event)
	if err != nil {
		log.L(ctx).Errorf("Error getting unconfirmed dependencies: %s", err)
		return false, err
	}

	blockingNodeIDs := make(map[string]bool)
	blockedBy := make([]blockingTransaction, 0, len(unconfirmedDependencies))

	for _, dependency := range unconfirmedDependencies {
		blockingNodeIDs[dependency.sequencingNodeID] = true
		blockedBy = append(blockedBy, blockingTransaction{
			transactionID: dependency.transactionID,
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
		err := s.blockTransaction(ctx, event.TransactionId, blockedBy)

		if err != nil {
			log.L(ctx).Errorf("Error blocking transaction: %s", err)
			return false, err
		}
		return true, nil
	}
	if len(keys) == 1 && keys[0] != s.nodeID.String() {
		// we are dependent on one other node so we can delegate
		err := s.delegate(ctx, event.TransactionId, keys[0])
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

func (s *sequencer) OnTransactionAssembled(ctx context.Context, event *pb.TransactionAssembledEvent) error {
	log.L(ctx).Infof("Received transaction assembled event: %s", event.String())
	//Record the new transaction
	s.unconfirmedTransactionsByID[event.TransactionId] = &unconfirmedTransaction{
		transactionID:    event.TransactionId,
		sequencingNodeID: event.NodeId, // assume it goes to its local sequencer until we hear otherwise
		assemblerNodeID:  event.NodeId,
		outputStates:     event.OutputStateHash,
		inputStates:      event.InputStateHash,
	}
	for _, unconfirmedStateHash := range event.OutputStateHash {
		s.unconfirmedStatesByHash[unconfirmedStateHash] = &unconfirmedState{
			stateHash:            unconfirmedStateHash,
			mintingTransactionID: event.TransactionId,
		}
	}

	//if the transaction was assembled on the local node, then we add it into the graph
	//if it was assembled on another node, then we remember it in case we end up getting some dependencies on it
	if event.NodeId == s.nodeID.String() {
		delegated, err := s.delegateIfAppropriate(ctx, event)
		if err != nil {
			log.L(ctx).Errorf("Error delegating transaction: %s", err)
			return err
		}
		if delegated {
			return nil
		}

		err = s.graph.AddTransaction(ctx, event.TransactionId, event.InputStateHash, event.OutputStateHash)
		if err != nil {
			log.L(ctx).Errorf("Error adding transaction to graph: %s", err)
			return err
		}
		err = s.evaluateGraph(ctx)
		if err != nil {
			log.L(ctx).Errorf("Error evaluating graph: %s", err)
			return err
		}
	} else {
		//TODO this could be a dependency on a transaction that we have already added to our graph but
		// we didn't know about it when we added the dependant transaction
	}

	return nil
}

func (s *sequencer) OnTransactionEndorsed(ctx context.Context, event *pb.TransactionEndorsedEvent) error {

	log.L(ctx).Infof("Received transaction endorsed event: %s", event.String())

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

/*
func (s *sequencer) OnStateClaimEvent(ctx context.Context, event *pb.StateClaimEvent) error {
	log.L(ctx).Infof("Received state claim event: %s", event.String())
	state, err := s.persistence.GetStateByHash(ctx, event.StateHash)
	if err != nil {
		log.L(ctx).Errorf("Error getting state by ID: %s", err)
		return err
	}
	if state.ClaimedBy != nil {
		//we have a contention
		log.L(ctx).Debug("Contention")
		// if this is the only state being claimed by both transactions, then we can resolve the contention immediately
		// if either of the transactions is claiming more than one state, then the is no guarantee that we can predict the final outcome
		// so all we can do is add both transactions to a sequence and let that sequencer
		resolvedClaimer, err := s.resolver.Resolve(state.Hash.String(), state.ClaimedBy.String(), event.TransactionId)
		if err != nil {
			log.L(ctx).Errorf("Error resolving contention: %s", err)
			return err
		}
		if resolvedClaimer == event.TransactionId {
			// the current claimer has lost its claim
			currentClaimer, err := s.persistence.GetTransactionByID(ctx, *state.ClaimedBy)
			if err != nil {
				log.L(ctx).Errorf("Error getting transaction by ID: %s", err)
				return err
			}
			if currentClaimer.AssemblingNodeID == s.nodeID {

				// if the loser is assembled by the current node, then send a message to the assembler to reassemble
				// TODO - not sure this is exactly how the orchestrator expects us to deal with this.
				//Should we be sending a message to the orchestrator to update the transaction state and let the assemble stage notice that on its next cycle?

				//Before sending that message, we need to ensure that the DB is updated with the new claimer otherwise the assembler will attempt to claim this new state again
				resolvedClaimerUUID, err := uuid.Parse(resolvedClaimer)
				if err != nil {
					log.L(ctx).Errorf("failed to parse resolved claimer as uuid: %s", resolvedClaimer)
					return err
				}
				state.ClaimedBy = &resolvedClaimerUUID
				err = s.persistence.UpdateState(ctx, state)
				if err != nil {
					log.L(ctx).Errorf("Error updating state: %s", err)
					return err
				}
				s.sendReassembleMessage(ctx, currentClaimer.ID.String())
			}
		}
		s.publishStateClaimLostEvent(ctx, state.Hash.String(), state.ClaimedBy.String())
	} else {
		log.L(ctx).Debug("No contention")
		err = s.persistence.UpdateState(ctx, state)
		if err != nil {
			log.L(ctx).Errorf("Error updating state: %s", err)
			return err
		}
	}
	return nil
}
*/

func (s *sequencer) OnTransactionConfirmed(ctx context.Context, event *pb.TransactionConfirmedEvent) error {
	log.L(ctx).Infof("Received transaction confirmed event: %s", event.String())
	outputStateHashes := s.unconfirmedTransactionsByID[event.TransactionId].outputStates
	for _, outputStateHash := range outputStateHashes {
		s.unconfirmedStatesByHash[outputStateHash] = nil
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

func (s *sequencer) OnTransactionReverted(ctx context.Context, event *pb.TransactionRevertedEvent) error {
	//release the transaction's claim on any states
	for state, spender := range s.stateSpenders {
		if spender == event.TransactionId {
			delete(s.stateSpenders, state)
		}
	}

	return nil
}

func (s *sequencer) ApproveEndorsement(ctx context.Context, endorsementRequst EndorsementRequest) (bool, error) {
	contentionFound := false
	for _, stateHash := range endorsementRequst.inputStates {
		if stateSpender, ok := s.stateSpenders[stateHash]; ok {
			if stateSpender != endorsementRequst.transactionID {
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
	for _, stateHash := range endorsementRequst.inputStates {
		s.stateSpenders[stateHash] = endorsementRequst.transactionID
	}
	return true, nil
}
