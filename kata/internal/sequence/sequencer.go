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
	"github.com/kaleido-io/paladin/kata/internal/statestore"
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

type Persistence interface {
	// this is some temporary scaffolding around the core logic of the algorithm to allow us to make progress on the alogrithm and its functional tests
	// before figuring how to integrate it into the main codebase and frameworks of Orchestrator, StageController, CommsBus etc...
	// most likely will be replaced with some utility of the StageController or direct access to state store and transaction store etc..
	GetStateByHash(context.Context, string) (statestore.State, error)
	UpdateState(context.Context, statestore.State) error

	GetTransactionByID(context.Context, uuid.UUID) (transactionstore.Transaction, error)
	//return the transaction that has minted, or is assembled to mint the state with the given hash
	GetMintingTransactionByStateHash(context.Context, string) (transactionstore.Transaction, error)
}

type Sequencer interface {
	OnStateClaimEvent(ctx context.Context, event *pb.StateClaimEvent) error
	OnTransactionAssembled(ctx context.Context, event *pb.TransactionAssembledEvent) error
	OnTransactionEndorsed(ctx context.Context, event *pb.TransactionEndorsedEvent) error
}

type sequencer struct {
	nodeID      uuid.UUID
	persistence Persistence
	commsBus    commsbus.CommsBus
	resolver    ContentionResolver
	dispatcher  Dispatcher
	graph       Graph
}

func NewSequencer(eventSync EventSync, persistence Persistence) Sequencer {
	return &sequencer{
		persistence: persistence,
	}
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
func (s *sequencer) OnTransactionAssembled(ctx context.Context, event *pb.TransactionAssembledEvent) error {
	log.L(ctx).Infof("Received transaction assembled event: %s", event.String())

	err := s.graph.AddTransaction(ctx, event.TransactionId, event.InputStateHash, event.OutputStateHash)
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
func (s *sequencer) OnTransactionEndorsed(ctx context.Context, event *pb.TransactionEndorsedEvent) error {

	log.L(ctx).Infof("Received transaction endorsed event: %s", event.String())

	err := s.graph.RecordEndorsement(ctx, event.TransactionId)
	if err != nil {
		log.L(ctx).Errorf("Error recording endorsement: %s", err)
		return err
	}

	s.evaluateGraph(ctx)
	if err != nil {
		log.L(ctx).Errorf("Error evaluating graph: %s", err)
		return err
	}
	return nil
}

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
			if currentClaimer.NodeID == s.nodeID {

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
