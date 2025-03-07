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

	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

type State int

const (
	State_Initial State = iota // Initial state before anything is calculated

)

type EventType = common.EventType

const (
	Event_Received  EventType = iota // Transaction initially received by the sender.
	Event_Confirmed                  // confirmation received from the blockchain of either a successful or reverted transaction

)

type StateMachine struct {
	currentState State
}

// Actions can be specified for transition to a state either as the OnTransitionTo function that will run for all transitions to that state or as the On field in the Transition struct if the action applies
// for a specific transition
type Action func(ctx context.Context, txn *Transaction) error

// TODO should we pass static config ( e.g. timeouts) to the guards instead of storing them in every transaction struct instance?
type Guard func(ctx context.Context, txn *Transaction) bool

type Transition struct {
	To State // State to transition to if the guard condition is met
	If Guard // Condition to evaluate the transaction against to determine if this transition should be taken
	On Action
}

type ActionRule struct {
	Action Action
	If     Guard
}

type EventHandler struct {
	Validator   func(ctx context.Context, txn *Transaction, event common.Event) (bool, error) // function to validate whether the event is valid for the current state of the transaction.  This is optional.  If not defined, the event is always considered valid.
	Actions     []ActionRule                                                                  // list of actions to be taken when this event is received.  These actions are run before any transition specific actions
	Transitions []Transition                                                                  // list of transitions that this event could trigger.  The list is ordered so the first matching transition is the one that will be taken.
}

type StateDefinition struct {
	OnTransitionTo Action                     // function to be invoked when transitioning into this state.  This is invoked after any transition specific actions have been invoked
	Events         map[EventType]EventHandler // rules to define what events apply to this state and what transitions they trigger.  Any events not in this list are ignored while in this state.
}

var stateDefinitionsMap map[State]StateDefinition

func init() {
	// Initialize state definitions in init function to avoid circular dependencies
	stateDefinitionsMap = map[State]StateDefinition{
		State_Initial: {
			Events: map[EventType]EventHandler{
				Event_Received: { //TODO rename this event type because it is the first one we see in this struct and it seems like we are saying this is a definition related to receiving an event (at one level that is correct but it is not what is meant by Event_Received)

				},
			},
		},
	}
}

func (t *Transaction) InitializeStateMachine(initialState State) {
	t.stateMachine = &StateMachine{
		currentState: initialState,
	}
}

func (t *Transaction) HandleEvent(ctx context.Context, event common.Event) error {

	//determine whether this event is valid for the current state
	eventHandler, err := t.evaluateEvent(ctx, event)
	if err != nil || eventHandler == nil {
		return err
	}

	//If we get here, the state machine has defined a rule for handling this event
	//Apply the event to the transaction to update the internal state
	// so that the guards and actions defined in the state machine can reference the new internal state of the coordinator
	err = t.applyEvent(ctx, event)
	if err != nil {
		return err
	}

	err = t.performActions(ctx, *eventHandler)
	if err != nil {
		return err
	}

	//Determine whether this event triggers a state transition
	err = t.evaluateTransitions(ctx, event, *eventHandler)
	return err

}

// Function evaluateEvent evaluates whether the event is relevant given the current state of the transaction
func (t *Transaction) evaluateEvent(ctx context.Context, event common.Event) (*EventHandler, error) {
	sm := t.stateMachine

	//Determine if and how this event applies in the current state and which, if any, transition it triggers
	eventHandlers := stateDefinitionsMap[sm.currentState].Events
	eventHandler, isHandlerDefined := eventHandlers[event.Type()]
	if isHandlerDefined {
		//By default all events in the list are applied unless there is a validator function and it returns false
		if eventHandler.Validator != nil {
			valid, err := eventHandler.Validator(ctx, t, event)
			if err != nil {
				//This is an unexpected error.  If the event is invalid, the validator should return false and not an error
				log.L(ctx).Errorf("Error validating event %v: %v", event.Type(), err)
				return nil, err
			}
			if !valid {
				//This is perfectly normal sometimes an event happens and is no longer relevant to the transaction so we just ignore it and move on
				log.L(ctx).Debugf("Event %v is not valid: %v", event.Type(), valid)
				return nil, nil
			}
		}
		return &eventHandler, nil
	} else {
		// no event handler defined for this event while in this state
		log.L(ctx).Debugf("No event handler defined for Event %v in State %s", event.Type(), sm.currentState.String())
		return nil, nil
	}

}

// Function applyEvent updates the internal state of the Transaction with information from the event
// this happens before the state machine is evaluated for transitions that may be triggered by the event
// so that any guards on the transition rules can take into account the new internal state of the Transaction after this event has been applied
func (t *Transaction) applyEvent(ctx context.Context, event common.Event) error {
	//TODO reconsider moving these back into the state machine definition.
	var err error
	switch event := event.(type) {

	default:
		//other events may trigger actions and/or state transitions but not require any internal state to be updated
		log.L(ctx).Debugf("no internal state to apply for event type %T", event)
	}
	return err
}

func (t *Transaction) performActions(ctx context.Context, eventHandler EventHandler) error {
	for _, rule := range eventHandler.Actions {
		if rule.If == nil || rule.If(ctx, t) {
			err := rule.Action(ctx, t)
			if err != nil {
				//any recoverable errors should have been handled by the action function
				log.L(ctx).Errorf("Error applying action: %v", err)
				return err
			}
		}
	}
	return nil
}

func (t *Transaction) evaluateTransitions(ctx context.Context, event common.Event, eventHandler EventHandler) error {
	sm := t.stateMachine
	for _, rule := range eventHandler.Transitions {
		if rule.If == nil || rule.If(ctx, t) { //if there is no guard defined, or the guard returns true
			log.L(ctx).Infof("Transaction %s transitioning from %s to %s triggered by event %T", t.ID.String(), sm.currentState.String(), rule.To.String(), event)
			sm.currentState = rule.To
			newStateDefinition := stateDefinitionsMap[sm.currentState]
			//run any actions specific to the transition first
			if rule.On != nil {
				err := rule.On(ctx, t)
				if err != nil {
					//any recoverable errors should have been handled by the action function
					log.L(ctx).Errorf("Error transitioning to state %v: %v", sm.currentState, err)
					return err
				}
			}

			// then run any actions for the state entry
			if newStateDefinition.OnTransitionTo != nil {
				err := newStateDefinition.OnTransitionTo(ctx, t)
				if err != nil {
					// any recoverable errors should have been handled by the OnTransitionTo function
					log.L(ctx).Errorf("Error transitioning to state %v: %v", sm.currentState, err)
					return err
				}
			} else {
				log.L(ctx).Debugf("No OnTransitionTo function defined for state %v", sm.currentState)
			}

			break
		}
	}
	return nil
}

func guard_Not(guard Guard) Guard {
	return func(ctx context.Context, txn *Transaction) bool {
		return !guard(ctx, txn)
	}
}

func guard_And(guards ...Guard) Guard {
	return func(ctx context.Context, txn *Transaction) bool {
		for _, guard := range guards {
			if !guard(ctx, txn) {
				return false
			}
		}
		return true
	}
}

func guard_Or(guards ...Guard) Guard {
	return func(ctx context.Context, txn *Transaction) bool {
		for _, guard := range guards {
			if guard(ctx, txn) {
				return true
			}
		}
		return false
	}
}

func (s *State) String() string {
	switch *s {
	case State_Initial:
		return "Initial"
	}
	return fmt.Sprintf("Unknown (%d)", *s)
}
