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

package coordinator

import (
	"context"

	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

type State int
type EventType = common.EventType

const (
	State_Idle      State = iota //Not acting as a coordinator and not aware of any other active coordinators
	State_Observing              //Not acting as a coordinator but aware of another node acting as a coordinator
	State_Elect                  //Elected to take over from another coordinator and waiting for handover information
	State_Standby                //TODO comments to describe these
	State_Prepared
	State_Active
	State_Flush
	State_Closing
)

const (
	Event_Activated EventType = iota //TODO comments to describe these
	Event_Nominated
	Event_Flushed
	Event_Closed
	Event_TransactionsDelegated
	Event_TransactionConfirmed
	Event_TransactionDispatchConfirmed
	Event_HeartbeatReceived
	Event_NewBlock
	Event_HandoverRequestReceived
	Event_HandoverReceived
	Event_TransactionStateTransition
)

type StateMachine struct {
	currentState     State
	stateDefinitions map[State]StateDefinition
}

type Transition struct {
	To State
	If Guard
}

type OnTransitionTo func(ctx context.Context, c *coordinator, to, from State)

type StateDefinition struct {
	OnTransitionTo OnTransitionTo             // function to be invoked when transitioning into this state
	Transitions    map[EventType][]Transition // rules to define when to exit this state and which state to transition to
}

func (c *coordinator) InitializeStateMachine(initialState State) {
	c.stateMachine = &StateMachine{
		currentState: initialState,
	}
	sm := c.stateMachine

	//Transition rules determine which state to transition to when a particular event is received depending on the current state
	// Transitions can optionally be guarded by a function that returns a boolean.
	// Guard functions are synchronous, pure functions with inputs of the event and current state (including internal fine grained state) of the coordinator after the event has been applied
	// and outputs a boolean
	sm.stateDefinitions = map[State]StateDefinition{
		State_Idle: {
			OnTransitionTo: nil,
			Transitions: map[EventType][]Transition{
				Event_TransactionsDelegated: {
					{
						To: State_Active,
					},
				},
				Event_HeartbeatReceived: {
					{
						To: State_Observing,
					},
				},
			},
		},
		State_Observing: {
			OnTransitionTo: nil,
			Transitions: map[EventType][]Transition{
				Event_TransactionsDelegated: {
					{
						To: State_Standby,
						If: behind,
					},
					{
						To: State_Elect,
						If: notBehind,
					},
				},
			},
		},
		State_Standby: {
			OnTransitionTo: nil,
			Transitions: map[EventType][]Transition{
				Event_NewBlock: {
					{
						To: State_Elect,
						If: notBehind,
					},
				},
			},
		},
		State_Elect: {
			OnTransitionTo: action_SendHandoverRequest,
			Transitions: map[EventType][]Transition{
				Event_HandoverReceived: {
					{
						To: State_Prepared,
					},
				},
			},
		},
		State_Prepared: {
			OnTransitionTo: nil,
			Transitions: map[EventType][]Transition{
				Event_TransactionConfirmed: {
					{
						To: State_Active,
						If: activeCoordinatorFlushComplete,
					},
				},
			},
		},
		State_Active: {
			OnTransitionTo: action_SelectTransaction,
			Transitions: map[EventType][]Transition{
				Event_TransactionConfirmed: {
					{
						To: State_Idle,
						If: noTransactionsInflight,
					},
				},
				Event_HandoverRequestReceived: {
					{
						To: State_Flush,
					},
				},
			},
		},
		State_Flush: {
			OnTransitionTo: nil,
			Transitions: map[EventType][]Transition{
				Event_TransactionConfirmed: {
					{
						To: State_Closing,
						If: flushComplete,
					},
				},
			},
		},
		State_Closing: {
			OnTransitionTo: nil,
			Transitions: map[EventType][]Transition{
				common.Event_HeartbeatInterval: {
					{
						To: State_Idle,
						If: closingGracePeriodExpired,
					},
				},
			},
		},
	}

}

func (c *coordinator) HandleEvent(ctx context.Context, event common.Event) error {

	//Apply the event to the coordinator to update the internal state
	// so that the guards and actions defined in the state machine can reference the new internal state of the coordinator
	err := c.applyEvent(ctx, event)
	if err != nil {
		return err
	}

	//Determine whether this event triggers a state transition
	err = c.evaluateTransitions(ctx, event)
	return err

}

// Function applyEvent updates the internal state of the coordinator with information from the event
// this happens before the state machine is evaluated for transitions that may be triggered by the event
// so that any guards on the transition rules can take into account the new internal state of the coordinator after this event has been applied
func (c *coordinator) applyEvent(ctx context.Context, event common.Event) error {
	var err error
	// First apply the event to the update the internal fine grained state of the coordinator if there is any handler registered for the current state
	switch event := event.(type) {
	case *TransactionsDelegatedEvent:
		//TODO dependant on state?
		err = c.addToDelegatedTransactions(ctx, event.Sender, event.Transactions)

	case *TransactionConfirmedEvent:
		//This may be a confirmation of a transaction that we have have been coordinating or it may be one that another coordinator has been coordinating
		//if the latter, then we may or may not know about it depending on whether we have seen a heartbeat from that coordinator since last time
		// we were loaded into memory
		//TODO - we can't actually guarantee that we have all transactions we dispatched in memory.
		//Even assuming that the public txmgr is in the same process (may not be true forever)  and assuming that we haven't been swapped out ( likely not to be true very soon) there is still a chance that the transaction was submitted to the base ledger, then the process restarted then we get the confirmation.				//When the process starts, we need to make sure that the coordinator is pre loaded with knowledge of all transactions that it has dispatched
		if !c.confirmDispatchedTransaction(ctx, event.From, event.Nonce, event.Hash, event.RevertReason) {
			c.confirmMonitoredTransaction(ctx, event.From, event.Nonce)
		}

	case *transaction.AssembleSuccessEvent:
		c.propagateEventToTransaction(ctx, event)

	case *transaction.AssembleRevertResponseEvent:
		c.propagateEventToTransaction(ctx, event)

	case *TransactionDispatchConfirmedEvent:
		c.propagateEventToTransaction(ctx, event)
	case *NewBlockEvent:
		c.currentBlockHeight = event.BlockHeight
	case *HeartbeatReceivedEvent:
		c.activeCoordinator = event.From
		c.activeCoordinatorBlockHeight = event.BlockHeight
		for _, flushPoint := range event.FlushPoints {
			c.activeCoordinatorsFlushPointsBySignerNonce[flushPoint.GetSignerNonce()] = flushPoint
		}
	case *common.HeartbeatIntervalEvent:
		//TODO send heartbeat message

		c.heartbeatIntervalsSinceStateChange++
		err = c.propagateEventToAllTransactions(ctx, event)

	}

	if err != nil {
		log.L(ctx).Errorf("Error applying event %v: %v", event.Type(), err)
	}
	return err
}

func (c *coordinator) evaluateTransitions(ctx context.Context, event common.Event) error {
	sm := c.stateMachine

	transitions := sm.stateDefinitions[sm.currentState].Transitions
	if transitionRules, ok := transitions[event.Type()]; ok {
		for _, rule := range transitionRules {
			if rule.If == nil || rule.If(ctx, c) { //if there is no guard defined, or the guard returns true
				log.L(context.Background()).Infof("Coordinator for %s transitioning from %s to %s triggered by event %T", c.contractAddress.String(), sm.currentState.String(), rule.To.String(), event)

				fromState := sm.currentState
				sm.currentState = rule.To
				newStateDefinition := sm.stateDefinitions[sm.currentState]
				if newStateDefinition.OnTransitionTo != nil {
					newStateDefinition.OnTransitionTo(ctx, c, rule.To, fromState)
				} else {
					log.L(context.Background()).Debugf("No OnTransitionTo function defined for state %v", sm.currentState)
				}
				c.heartbeatIntervalsSinceStateChange = 0
				break
			}
		}
	} else {
		log.L(ctx).Debugf("No transition for Event %v from State %s", event.Type(), sm.currentState.String())
	}
	return nil
}

func action_SendHandoverRequest(ctx context.Context, c *coordinator, to, from State) {
	c.sendHandoverRequest(ctx)
}

func action_SelectTransaction(ctx context.Context, c *coordinator, to, from State) {
	c.selectNextTransaction(ctx, nil)
}

func (s *State) String() string {
	switch *s {
	case State_Idle:
		return "Idle"
	case State_Observing:
		return "Observing"
	case State_Elect:
		return "Elect"
	case State_Standby:
		return "Standby"
	case State_Prepared:
		return "Prepared"
	case State_Active:
		return "Active"
	case State_Flush:
		return "Flush"
	case State_Closing:
		return "Closing"
	}
	return "Unknown"
}
