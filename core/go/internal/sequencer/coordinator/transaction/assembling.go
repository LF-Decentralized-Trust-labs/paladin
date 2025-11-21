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
package transaction

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/toolkit/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

func (t *Transaction) applyPostAssembly(ctx context.Context, postAssembly *components.TransactionPostAssembly) error {

	//TODO the response from the assembler actually contains outputStatesPotential so we need to write them to the store and then add the OutputState ids to the index
	t.PostAssembly = postAssembly
	if t.cancelAssembleTimeoutSchedule != nil {
		t.cancelAssembleTimeoutSchedule()
		t.cancelAssembleTimeoutSchedule = nil
	}
	for _, state := range postAssembly.OutputStates {
		err := t.grapher.AddMinter(ctx, state.ID, t)
		if err != nil {
			msg := fmt.Sprintf("Error adding minter for state %s: %s while applying postAssembly", state.ID, err)
			log.L(ctx).Error(msg)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
	}
	return t.calculatePostAssembleDependencies(ctx)
}

func (t *Transaction) sendAssembleRequest(ctx context.Context) error {
	//assemble requests have a short and long timeout
	// the short timeout is for toleration of unreliable networks whereby the action is to retry the request with the same idempotency key
	// the long timeout is to prevent an unavailable transaction sender/assemble from holding up the entire contract / privacy group given that the assemble step is single threaded
	// the action for the long timeout is to return the transaction to the mempool and let another transaction be selected

	//When we first send the request, we start a ticker to emit a requestTimeout event for each tick
	// we and nudge the request every requestTimeout event implement the short retry.
	// the state machine will deal with the long timeout via the guard assembleTimeoutExpired

	t.pendingAssembleRequest = common.NewIdempotentRequest(ctx, t.clock, t.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return t.messageSender.SendAssembleRequest(ctx, t.sender, t.ID, idempotencyKey, t.PreAssembly)
	})
	t.cancelAssembleTimeoutSchedule = t.clock.ScheduleInterval(ctx, t.requestTimeout, func() {
		t.emit(&RequestTimeoutIntervalEvent{
			BaseEvent: BaseEvent{
				TransactionID: t.ID,
			},
		})
	})
	return t.pendingAssembleRequest.Nudge(ctx)
}

func (t *Transaction) nudgeAssembleRequest(ctx context.Context) error {
	if t.pendingAssembleRequest == nil {
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "nudgeAssembleRequest called with no pending request")
	}
	return t.pendingAssembleRequest.Nudge(ctx)
}

func (t *Transaction) assembleTimeoutExceeded(ctx context.Context) bool {
	if t.pendingAssembleRequest == nil {
		//strange situation to be in if we get to the point of this being nil, should immediately leave the state where we ever ask this question
		// however we go here, the answer to the question is "false" because there is no pending request to timeout but log this as it is a strange situation
		// and might be an indicator of another issue
		log.L(ctx).Infof("assembleTimeoutExceeded called on transaction %s with no pending assemble request", t.ID)
		return false
	}
	return t.clock.HasExpired(t.pendingAssembleRequest.FirstRequestTime(), t.assembleTimeout)

}

func (t *Transaction) isNotAssembled() bool {
	//test against the list of states that we consider to be past the point of assemble as there is more chance of us noticing
	// a failing test if we add new states in the future and forget to update this list

	return t.GetState() != State_Endorsement_Gathering &&
		t.GetState() != State_Confirming_Dispatch &&
		t.GetState() != State_Ready_For_Dispatch &&
		t.GetState() != State_Dispatched &&
		t.GetState() != State_Submitted &&
		t.GetState() != State_Confirmed
}

func (t *Transaction) notifyDependentsOfAssembled(ctx context.Context) error {
	//this function is called when the transaction is successfully assembled
	// and we have a duty to inform all the transactions that are ordered behind us
	if t.nextTransaction != nil {
		err := t.nextTransaction.HandleEvent(ctx, &DependencyAssembledEvent{
			BaseEvent: BaseEvent{
				TransactionID: t.nextTransaction.ID,
			},
			DependencyID: t.ID,
		})
		if err != nil {
			log.L(ctx).Errorf("Error notifying next transaction %s of assembly of transaction %s: %s", t.nextTransaction.ID, t.ID, err)
			return err
		}
	}

	for _, dependentId := range t.dependents {
		dependent := t.grapher.TransactionByID(ctx, dependentId)
		if dependent == nil {
			msg := fmt.Sprintf("notifyDependentsOfReadiness: Dependent transaction %s not found in memory", dependentId)
			log.L(ctx).Error(msg)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
		err := dependent.HandleEvent(ctx, &DependencyAssembledEvent{
			BaseEvent: BaseEvent{
				TransactionID: t.nextTransaction.ID,
			},
			DependencyID: t.ID,
		})
		if err != nil {
			log.L(ctx).Errorf("Error notifying dependent transaction %s of assembly of transaction %s: %s", dependent.ID, t.ID, err)
			return err
		}
	}
	return nil
}

func (t *Transaction) notifyDependentsOfRevert(ctx context.Context) error {
	//this function is called when the transaction enters the reverted state on a revert response from assemble
	// NOTE: at this point, we have not been assembled and therefore are not the minter of any state the only transactions that could possibly be dependent on us are those in the pool from the same sender

	for _, dependentID := range append(t.dependents, t.preAssembleDependents...) {
		dependentTxn := t.grapher.TransactionByID(ctx, dependentID)
		if dependentTxn != nil {
			err := dependentTxn.HandleEvent(ctx, &DependencyRevertedEvent{
				BaseEvent: BaseEvent{
					TransactionID: dependentID,
				},
				DependencyID: t.ID,
			})
			if err != nil {
				log.L(ctx).Errorf("Error notifying dependent transaction %s of revert of transaction %s: %s", dependentID, t.ID, err)
				return err
			}
		} else {
			//TODO can we Assume that the dependent is no longer in memory and doesn't need to know about this event?  Point to (write) the architecture doc that explains why this is safe

			msg := fmt.Sprintf("notifyDependentsOfRevert: Dependent transaction %s not found in memory", dependentID)
			log.L(ctx).Error(msg)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}

	}

	return nil
}

func (t *Transaction) calculatePostAssembleDependencies(ctx context.Context) error {
	//Dependencies can arise because  we have been assembled to spend states that were produced by other transactions
	// or because there are other transactions from the same sender that have not been dispatched yet or because the user has declared explicit dependencies
	// this function calculates the dependencies relating to states and sets up the reverse association
	// it is assumed that the other dependencies have already been set up when the transaction was first received by the coordinator TODO correct this comment line with more accurate description of when we expect the static dependencies to have been calculated.  Or make it more vague.
	if t.PostAssembly == nil {
		msg := fmt.Sprintf("Cannot calculate dependencies for transaction %s without a PostAssembly", t.ID)
		log.L(ctx).Error(msg)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
	}

	found := make(map[uuid.UUID]bool)
	t.dependencies = make([]uuid.UUID, 0, len(t.PostAssembly.InputStates)+len(t.PostAssembly.ReadStates))
	for _, state := range append(t.PostAssembly.InputStates, t.PostAssembly.ReadStates...) {
		dependency, err := t.grapher.LookupMinter(ctx, state.ID)
		if err != nil {
			errMsg := fmt.Sprintf("Error looking up dependency for state %s: %s", state.ID, err)
			log.L(ctx).Error(errMsg)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, errMsg)
		}
		if dependency == nil {
			log.L(ctx).Infof("No minter found for state %s", state.ID)
			//assume the state was produced by a confirmed transaction
			//TODO should we validate this by checking the domain context? If not, explain why this is safe in the architecture doc
			continue
		}
		if found[dependency.ID] {
			continue
		}
		found[dependency.ID] = true
		t.dependencies = append(t.dependencies, dependency.ID)
		//also set up the reverse association
		dependency.dependents = append(dependency.dependents, t.ID)
	}
	return nil
}

func (t *Transaction) writeLockAndDistributeStates(ctx context.Context) error {
	return t.engineIntegration.WriteLockAndDistributeStatesForTransaction(ctx, t.PrivateTransaction)
}

func (t *Transaction) incrementAssembleErrors(ctx context.Context) error {
	t.errorCount++
	return nil
}

func validator_MatchesPendingAssembleRequest(ctx context.Context, txn *Transaction, event common.Event) (bool, error) {
	switch event := event.(type) {
	case *AssembleSuccessEvent:
		return txn.pendingAssembleRequest != nil && txn.pendingAssembleRequest.IdempotencyKey() == event.RequestID, nil
	case *AssembleRevertResponseEvent:
		return txn.pendingAssembleRequest != nil && txn.pendingAssembleRequest.IdempotencyKey() == event.RequestID, nil
	}
	return false, nil
}

func action_SendAssembleRequest(ctx context.Context, txn *Transaction) error {
	return txn.sendAssembleRequest(ctx)
}

func action_NudgeAssembleRequest(ctx context.Context, txn *Transaction) error {
	return txn.nudgeAssembleRequest(ctx)
}

func action_NotifyDependentsOfAssembled(ctx context.Context, txn *Transaction) error {
	return txn.notifyDependentsOfAssembled(ctx)
}

func action_NotifyDependentsOfRevert(ctx context.Context, txn *Transaction) error {
	return txn.notifyDependentsOfRevert(ctx)
}

func action_IncrementAssembleErrors(ctx context.Context, txn *Transaction) error {
	return txn.incrementAssembleErrors(ctx)
}

func guard_AssembleTimeoutExceeded(ctx context.Context, txn *Transaction) bool {
	return txn.assembleTimeoutExceeded(ctx)
}
