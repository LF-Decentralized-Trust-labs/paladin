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
	State_Idle      State = iota // Not acting as a coordinator and not aware of any other active coordinators
	State_Observing              // Not acting as a coordinator but aware of another node acting as a coordinator
	State_Elect                  // Elected to take over from another coordinator and waiting for handover information
	State_Standby                // Going to be coordinator on the next block range but local indexer is not at that block yet.
	State_Prepared               // Have received the handover response but haven't seen the flush point confirmed
	State_Active                 // Have seen the flush point or have reason to believe the old coordinator has become unavailable and am now assembling transactions based on available knowledge of the state of the base ledger and submitting transactions to the base ledger.
	State_Flush                  // Stopped assembling and dispatching transactions but continue to submit transactions that are already dispatched
	State_Closing                // Have flushed and am continuing to sent closing status for `x` heartbeats.
)

const (
	Event_Activated EventType = iota + common.Event_HeartbeatInterval + 1 //
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
	currentState State
}

// Actions can be specified for transition to a state either as the OnTransitionTo function that will run for all transitions to that state or as the On field in the Transition struct if the action applies
// for a specific transition
type Action func(ctx context.Context, c *coordinator) error
type ActionRule struct {
	Action Action
	If     Guard
}

type Transition struct {
	To State // State to transition to if the guard condition is met
	If Guard // Condition to evaluate the transaction against to determine if this transition should be taken
	On Action
}

type EventHandler struct {
	Validator   func(ctx context.Context, c *coordinator, event common.Event) (bool, error) // function to validate whether the event is valid for the current state of the coordinator.  This is optional.  If not defined, the event is always considered valid.
	Actions     []ActionRule                                                                // list of actions to be taken when this event is received.  These actions are run before any transition specific actions
	Transitions []Transition                                                                // list of transitions that this event could trigger.  The list is ordered so the first matching transition is the one that will be taken.
}

type StateDefinition struct {
	OnTransitionTo Action                     // function to be invoked when transitioning into this state.  This is invoked after any transition specific actions have been invoked
	Events         map[EventType]EventHandler // rules to define what events apply to this state and what transitions they trigger.  Any events not in this list are ignored while in this state.
}

var stateDefinitionsMap map[State]StateDefinition

func init() {
	// Initialize state definitions in init function to avoid circular dependencies
	stateDefinitionsMap = map[State]StateDefinition{
		State_Idle: {
			Events: map[EventType]EventHandler{

				Event_TransactionsDelegated: {
					Transitions: []Transition{{
						To: State_Active,
					}},
				},
				Event_HeartbeatReceived: {
					Transitions: []Transition{{
						To: State_Observing,
					}},
				},
			},
		},
		State_Observing: {
			Events: map[EventType]EventHandler{
				common.Event_HeartbeatInterval: {},
				Event_TransactionsDelegated: {
					Transitions: []Transition{
						{
							To: State_Standby,
							If: guard_Behind,
						},
						{
							To: State_Elect,
							If: guard_Not(guard_Behind),
						},
					},
				},
			},
		},
		State_Standby: {
			Events: map[EventType]EventHandler{
				common.Event_HeartbeatInterval: {},
				Event_TransactionsDelegated:    {},
				Event_NewBlock: {
					Transitions: []Transition{{
						To: State_Elect,
						If: guard_Not(guard_Behind),
					}},
				},
			},
		},
		State_Elect: {
			OnTransitionTo: action_SendHandoverRequest,
			Events: map[EventType]EventHandler{
				common.Event_HeartbeatInterval: {},
				Event_TransactionsDelegated:    {},
				Event_HandoverReceived: {
					Transitions: []Transition{{
						To: State_Prepared,
					}},
				},
			},
		},
		State_Prepared: {
			Events: map[EventType]EventHandler{
				common.Event_HeartbeatInterval: {},
				Event_TransactionsDelegated:    {},
				Event_TransactionConfirmed: {
					Transitions: []Transition{{
						To: State_Active,
						If: guard_ActiveCoordinatorFlushComplete,
					}},
				},
			},
		},
		State_Active: {
			OnTransitionTo: action_SelectTransaction,
			Events: map[EventType]EventHandler{
				common.Event_HeartbeatInterval: {
					Actions: []ActionRule{{
						Action: action_SendHeartbeat,
					}},
				},
				Event_TransactionsDelegated: {
					Actions: []ActionRule{{
						Action: action_SelectTransaction,
						If:     guard_Not(guard_HasTransactionAssembling),
					}},
				},
				Event_TransactionConfirmed: {
					Transitions: []Transition{{
						To: State_Idle,
						If: guard_Not(guard_HasTransactionsInflight),
					}},
				},
				Event_HandoverRequestReceived: {
					Transitions: []Transition{{
						To: State_Flush,
					}},
				},
			},
		},
		State_Flush: {
			//TODO should we move to active if we get delegated transactions while in flush?
			Events: map[EventType]EventHandler{
				common.Event_HeartbeatInterval: {},
				Event_TransactionConfirmed: {
					Transitions: []Transition{{
						To: State_Closing,
						If: guard_FlushComplete,
					}},
				},
			},
		},
		State_Closing: {
			//TODO should we move to active if we get delegated transactions while in closing?
			Events: map[EventType]EventHandler{
				common.Event_HeartbeatInterval: {
					Transitions: []Transition{{
						To: State_Idle,
						If: guard_ClosingGracePeriodExpired,
					}},
				},
			},
		},
	}
}

func (c *coordinator) InitializeStateMachine(initialState State) {
	c.stateMachine = &StateMachine{
		currentState: initialState,
	}
}

func (c *coordinator) HandleEvent(ctx context.Context, event common.Event) error {

	if transactionEvent, ok := event.(transaction.Event); ok {
		return c.propagateEventToTransaction(ctx, transactionEvent)
	}

	//determine whether this event is valid for the current state
	eventHandler, err := c.evaluateEvent(ctx, event)
	if err != nil || eventHandler == nil {
		return err
	}

	//If we get here, the state machine has defined a rule for handling this event
	//Apply the event to the coordinator to update the internal state
	// so that the guards and actions defined in the state machine can reference the new internal state of the coordinator
	err = c.applyEvent(ctx, event)
	if err != nil {
		return err
	}

	err = c.performActions(ctx, *eventHandler)
	if err != nil {
		return err
	}

	//Determine whether this event triggers a state transition
	err = c.evaluateTransitions(ctx, event, *eventHandler)
	return err

}

// Function evaluateEvent evaluates whether the event is relevant given the current state of the coordinator
func (c *coordinator) evaluateEvent(ctx context.Context, event common.Event) (*EventHandler, error) {
	sm := c.stateMachine

	//Determine if and how this event applies in the current state and which, if any, transition it triggers
	eventHandlers := stateDefinitionsMap[sm.currentState].Events
	eventHandler, isHandlerDefined := eventHandlers[event.Type()]
	if isHandlerDefined {
		//By default all events in the list are applied unless there is a validator function and it returns false
		if eventHandler.Validator != nil {
			valid, err := eventHandler.Validator(ctx, c, event)
			if err != nil {
				//This is an unexpected error.  If the event is invalid, the validator should return false and not an error
				log.L(ctx).Errorf("Error validating event %s: %v", event.TypeString(), err)
				return nil, err
			}
			if !valid {
				//This is perfectly normal sometimes an event happens and is no longer relevant to the coordinator so we just ignore it and move on
				log.L(ctx).Debugf("Event %s is not valid: %t", event.TypeString(), valid)
				return nil, nil
			}
		}
		return &eventHandler, nil
	} else {
		// no event handler defined for this event while in this state
		log.L(ctx).Debugf("No event handler defined for Event %s in State %s", event.TypeString(), sm.currentState.String())
		return nil, nil
	}
}

// Function applyEvent updates the internal state of the coordinator with information from the event
// this happens before the state machine is evaluated for transitions that may be triggered by the event
// so that any guards on the transition rules can take into account the new internal state of the coordinator after this event has been applied
func (c *coordinator) applyEvent(ctx context.Context, event common.Event) error {
	var err error
	// First apply the event to the update the internal fine grained state of the coordinator if there is any handler registered for the current state
	switch event := event.(type) {
	case *TransactionsDelegatedEvent:
		err = c.addToDelegatedTransactions(ctx, event.Sender, event.Transactions)
	case *TransactionConfirmedEvent:
		//This may be a confirmation of a transaction that we have have been coordinating or it may be one that another coordinator has been coordinating
		//if the latter, then we may or may not know about it depending on whether we have seen a heartbeat from that coordinator since last time
		// we were loaded into memory
		//TODO - we can't actually guarantee that we have all transactions we dispatched in memory.
		//Even assuming that the public txmgr is in the same process (may not be true forever)  and assuming that we haven't been swapped out ( likely not to be true very soon) there is still a chance that the transaction was submitted to the base ledger, then the process restarted then we get the confirmation.				//When the process starts, we need to make sure that the coordinator is pre loaded with knowledge of all transactions that it has dispatched
		isDispatchedTransaction, err := c.confirmDispatchedTransaction(ctx, event.From, event.Nonce, event.Hash, event.RevertReason)
		if err != nil {
			log.L(ctx).Errorf("Error confirming transaction From: %s , Nonce: %d, Hash: %v: %v", event.From, event.Nonce, event.Hash, err)
			return err
		}
		if !isDispatchedTransaction {
			c.confirmMonitoredTransaction(ctx, event.From, event.Nonce)
		}
	case *transaction.AssembleSuccessEvent:
		err = c.propagateEventToTransaction(ctx, event)
	case *transaction.AssembleRevertResponseEvent:
		err = c.propagateEventToTransaction(ctx, event)
	case *TransactionDispatchConfirmedEvent:
		err = c.propagateEventToTransaction(ctx, event)
	case *NewBlockEvent:
		c.currentBlockHeight = event.BlockHeight
	case *HeartbeatReceivedEvent:
		c.activeCoordinator = event.From
		c.activeCoordinatorBlockHeight = event.BlockHeight
		for _, flushPoint := range event.FlushPoints {
			c.activeCoordinatorsFlushPointsBySignerNonce[flushPoint.GetSignerNonce()] = flushPoint
		}
	case *common.HeartbeatIntervalEvent:
		c.heartbeatIntervalsSinceStateChange++
		//TODO is this the right place to do this vs more generically in the handleEvent function?
		err = c.propagateEventToAllTransactions(ctx, event)
	}
	if err != nil {
		log.L(ctx).Errorf("Error applying event %v: %v", event.Type(), err)
	}
	return err
}

func (c *coordinator) performActions(ctx context.Context, eventHandler EventHandler) error {
	for _, rule := range eventHandler.Actions {
		if rule.If == nil || rule.If(ctx, c) {
			err := rule.Action(ctx, c)
			if err != nil {
				//any recoverable errors should have been handled by the action function
				log.L(ctx).Errorf("Error applying action: %v", err)
				return err
			}
		}
	}
	return nil
}

func (c *coordinator) evaluateTransitions(ctx context.Context, event common.Event, eventHandler EventHandler) error {
	sm := c.stateMachine

	for _, rule := range eventHandler.Transitions {
		if rule.If == nil || rule.If(ctx, c) { //if there is no guard defined, or the guard returns true
			log.L(ctx).Infof("Coordinator for address %s transitioning from %s to %s triggered by event %T", c.contractAddress.String(), sm.currentState.String(), rule.To.String(), event)
			sm.currentState = rule.To
			newStateDefinition := stateDefinitionsMap[sm.currentState]
			//run any actions specific to the transition first
			if rule.On != nil {
				err := rule.On(ctx, c)
				if err != nil {
					//any recoverable errors should have been handled by the action function
					log.L(ctx).Errorf("Error transitioning to state %v: %v", sm.currentState, err)
					return err
				}
			}

			// then run any actions for the state entry
			if newStateDefinition.OnTransitionTo != nil {
				err := newStateDefinition.OnTransitionTo(ctx, c)
				if err != nil {
					// any recoverable errors should have been handled by the OnTransitionTo function
					log.L(ctx).Errorf("Error transitioning to state %v: %v", sm.currentState, err)
					return err
				}
			} else {
				log.L(ctx).Debugf("No OnTransitionTo function defined for state %v", sm.currentState)
			}

			c.heartbeatIntervalsSinceStateChange = 0
			break
		}
	}
	return nil

}

func action_SendHandoverRequest(ctx context.Context, c *coordinator) error {
	c.sendHandoverRequest(ctx)
	return nil
}

func action_SelectTransaction(ctx context.Context, c *coordinator) error {
	return c.selectNextTransaction(ctx, nil)
}

func (s State) String() string {
	switch s {
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
