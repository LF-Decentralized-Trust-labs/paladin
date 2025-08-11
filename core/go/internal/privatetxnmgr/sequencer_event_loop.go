/*
 * Copyright © 2024 Kaleido, Inc.
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

package privatetxnmgr

import (
	"context"
	"fmt"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr/ptmgrtypes"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
)

func (s *Sequencer) evaluationLoop() {
	// Select from the event channel and process each event on a single thread
	// individual event handlers are responsible for kicking off another go routine if they have
	// long running tasks that are not dependant on the order of events
	// TODO if the channel ever fills up, then we need to be aware that we have potentially missed some events
	// and we need to poll the database for any events that we missed

	ctx := log.WithLogField(s.ctx, "role", fmt.Sprintf("pctm-loop-%s", s.contractAddress))
	log.L(ctx).Infof("Sequencer for contract address %s started evaluation loop based on interval %s", s.contractAddress, s.evalInterval)

	defer close(s.sequencerLoopDone)

	ticker := time.NewTicker(s.evalInterval)
	for {
		// an InFlight
		select {
		case blockHeight := <-s.newBlockEvents:
			//TODO should we use this is as the metronome to periodically trigger any inflight transactions to re-evaluate their state?
			s.environment.blockHeight = blockHeight
		case pendingEvent := <-s.pendingTransactionEvents:
			s.handleTransactionEvent(ctx, pendingEvent)
		case <-s.orchestrationEvalRequestChan:
		case <-ticker.C:
		case <-ctx.Done():
			log.L(ctx).Infof("Sequencer loop exit due to canceled context, it processed %d transaction during its lifetime.", s.totalCompleted)
			return
		case <-s.stopProcess:
			log.L(ctx).Infof("Sequencer loop process stopped, it processed %d transaction during its lifetime.", s.totalCompleted)
			s.state = SequencerStateStopped
			s.stateEntryTime = time.Now()
			// TODO: trigger parent loop for removal
			return
		}
		// TODO while we have woken up, iterate through all transactions in memory and check if any are stale or completed and query the database for any in flight transactions that need to be brought into memory
	}
}

func (s *Sequencer) handleTransactionEvent(ctx context.Context, event ptmgrtypes.PrivateTransactionEvent) {
	//For any event that is specific to a single transaction,
	// find (or create) the transaction processor for that transaction
	// and pass the event to it
	transactionID := event.GetTransactionID()
	log.L(ctx).Debugf("Sequencer handling event %T for transaction %s", event, transactionID)

	transactionProcessor := s.getTransactionProcessor(transactionID)
	if transactionProcessor == nil {
		//What has happened here is either:
		// a) we don't know about this transaction yet, which is possible if we have just restarted
		// and not yet completed the initialization from the database but in the interim, we have received
		// a transport message from another node concerning this transaction
		// b) the transaction has been completed and removed from memory
		// in case of (a) we ignore it and rely on the fact that we will send out request for data that we need once
		// we have completed the initialization from the database
		// in case of (b) we ignore it because an event for a completed transaction is redundant.
		// most likely it is a tardy response for something we timed out waiting for and failed or retried successfully
		log.L(ctx).Warnf("Received an event for a transaction that is not in flight %s", transactionID)
		return
	}

	validationError := event.Validate(ctx)
	if validationError != nil {
		log.L(ctx).Errorf("Error validating %T event: %s ", event, validationError.Error())
		//we can't handle this event.  If that leaves a transaction in an incomplete state, then it will eventually resend requests for the data it needs
		return
	}

	/*
		Apply the event to the transaction processor's in memory record of the transaction
		this is expected to be a simple in memory data mapping and therefore is not expected to return any errors
		it must either
			- decide to ignore the event altogether
			- completely apply the the event
	*/
	transactionProcessor.ApplyEvent(ctx, event)

	/*
		 	After applying the event to the transaction, we can either a) clean up that transaction ( if we have just learned, from the event that the transaction is complete and needs no further actions)
			or b) perform any necessary actions (e.g. sending requests for signatures, endorsements etc.)
	*/
	if transactionProcessor.IsComplete(ctx) {

		s.graph.RemoveTransaction(ctx, transactionID)
		s.removeTransactionProcessor(transactionID)
	} else {

		/*
			Action will perform any necessary actions based on the current state of the transaction.
			This may include sending asynchronous requests for data and/or synchronous analysis of the in memory record of the transaction.
			Action is retry safe and idempotent.
		*/
		transactionProcessor.Action(ctx)
	}

	if transactionProcessor.CoordinatingLocally(ctx) && transactionProcessor.ReadyForSequencing(ctx) && !transactionProcessor.Dispatched(ctx) {
		// we are responsible for coordinating the endorsement flow for this transaction, ensure that it has been added it to the graph
		// NOTE: AddTransaction is idempotent so we don't need to check whether we have already added it
		s.graph.AddTransaction(ctx, transactionProcessor)
	} else {
		// incase the transaction was previously added to the graph but is no longer coordinating locally or is no longer ready for sequencing
		// then we need to remove it from the graph
		// this is a no-op if the transaction was not previously added to the graph

		//TODO - this should really be a method on the graph itself ( similar to GetDispatchableTransactions) to find all transactions ( and there dependents)
		// that are no longer ready for sequencing and remove them from the graph
		s.graph.RemoveTransaction(ctx, transactionID)
	}

	//analyze the graph to see if we can dispatch any transactions
	dispatchableTransactions, err := s.graph.GetDispatchableTransactions(ctx)
	if err != nil {
		//If the graph can't give us an answer without an error then we have no confidence that we are in possession of a valid
		// graph of transactions that can successfully be dispatched so only option here is to abandon everything we have in-memory for this contract
		// and start again
		log.L(ctx).Errorf("Error getting dispatchable transactions: %s", err)
		s.abort(err)
		return
	}
	if len(dispatchableTransactions) == 0 {
		log.L(ctx).Debug("No dispatchable transactions")
		return
	}
	err = s.DispatchTransactions(ctx, dispatchableTransactions)
	if err != nil {
		log.L(ctx).Errorf("Error dispatching transaction: %s", err)
		// assuming this is a transient error with e.g. network or the DB, then we will try again next time round the loop
		return
	}

	//DispatchTransactions is a persistence point so we can remove the transactions from our graph now that they are dispatched
	s.graph.RemoveTransactions(ctx, dispatchableTransactions.IDs(ctx))

}
