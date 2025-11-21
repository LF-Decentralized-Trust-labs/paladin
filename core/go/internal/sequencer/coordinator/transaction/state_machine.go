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
	State_Initial               State = iota // Initial state before anything is calculated
	State_Pooled                             // waiting in the pool to be assembled - TODO should rename to "Selectable" or "Selectable_Pooled".  Related to potential rename of `State_PreAssembly_Blocked`
	State_PreAssembly_Blocked                // has not been assembled yet and cannot be assembled because a dependency never got assembled successfully - i.e. it was either Parked or Reverted is also blocked
	State_Assembling                         // an assemble request has been sent but we are waiting for the response
	State_Reverted                           // the transaction has been reverted by the assembler/sender
	State_Endorsement_Gathering              // assembled and waiting for endorsement
	State_Blocked                            // is fully endorsed but cannot proceed due to dependencies not being ready for dispatch
	State_Confirming_Dispatch                // endorsed and waiting for dispatch confirmation
	State_Ready_For_Dispatch                 // dispatch confirmation received and waiting to be collected by the dispatcher thread.Going into this state is the point of no return
	State_Dispatched                         // collected by the dispatcher thread but not yet
	State_Submitted                          // at least one submission has been made to the blockchain
	State_Confirmed                          // "recently" confirmed on the base ledger.  NOTE: confirmed transactions are not held in memory for ever so getting a list of confirmed transactions will only return those confirmed recently
	State_Final                              // final state for the transaction. Transactions are removed from memory as soon as they enter this state
)

type EventType = common.EventType

const (
	Event_Received                     EventType = iota + common.Event_HeartbeatInterval + 1 // Transaction initially received by the coordinator.  Might seem redundant explicitly modeling this as an event rather than putting this logic into the constructor, but it is useful to make the initial state transition rules explicit in the state machine definitions
	Event_Selected                                                                           // selected from the pool as the next transaction to be assembled
	Event_AssembleRequestSent                                                                // assemble request sent to the assembler
	Event_Assemble_Success                                                                   // assemble response received from the sender
	Event_Assemble_Revert_Response                                                           // assemble response received from the sender with a revert reason
	Event_Endorsed                                                                           // endorsement received from one endorser
	Event_EndorsedRejected                                                                   // endorsement received from one endorser with a revert reason
	Event_DependencyReady                                                                    // another transaction, for which this transaction has a dependency on, has become ready for dispatch
	Event_DependencyAssembled                                                                // another transaction, for which this transaction has a dependency on, has been assembled
	Event_DependencyReverted                                                                 // another transaction, for which this transaction has a dependency on, has been reverted
	Event_DispatchConfirmed                                                                  // dispatch confirmation received from the sender
	Event_DispatchConfirmationRejected                                                       // dispatch confirmation response received from the sender with a rejection
	Event_Collected                                                                          // collected by the dispatcher thread
	Event_NonceAllocated                                                                     // nonce allocated by the dispatcher thread
	Event_Submitted                                                                          // submission made to the blockchain.  Each time this event is received, the submission hash is updated
	Event_Confirmed                                                                          // confirmation received from the blockchain of either a successful or reverted transaction
	Event_RequestTimeoutInterval                                                             // event emitted by the state machine on a regular period while we have pending requests
	Event_StateTransition                                                                    // event emitted by the state machine when a state transition occurs.  TODO should this be a separate enum?
	Event_AssembleTimeout                                                                    // the assemble timeout period has passed since we sent the first assemble request
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
					Transitions: []Transition{
						{
							To: State_Pooled,
							If: guard_And(guard_Not(guard_HasUnassembledDependencies), guard_Not(guard_HasUnknownDependencies)),
						},
						{
							To: State_PreAssembly_Blocked,
							If: guard_Or(guard_HasUnassembledDependencies, guard_HasUnknownDependencies),
						},
					},
				},
			},
		},
		State_PreAssembly_Blocked: {
			Events: map[EventType]EventHandler{
				Event_DependencyAssembled: {
					Transitions: []Transition{{
						To: State_Pooled,
						If: guard_Not(guard_HasUnassembledDependencies),
					}},
				},
			},
		},
		State_Pooled: {
			OnTransitionTo: action_initializeDependencies,
			Events: map[EventType]EventHandler{
				Event_Selected: {
					Transitions: []Transition{
						{
							To: State_Assembling,
						}},
				},
				Event_DependencyReverted: {
					Transitions: []Transition{{
						To: State_PreAssembly_Blocked,
					}},
				},
			},
		},
		State_Assembling: {
			OnTransitionTo: action_SendAssembleRequest,
			Events: map[EventType]EventHandler{
				Event_Assemble_Success: {
					Validator: validator_MatchesPendingAssembleRequest,
					Transitions: []Transition{
						{
							To: State_Endorsement_Gathering,
							On: action_NotifyDependentsOfAssembled,
						}},
				},
				Event_RequestTimeoutInterval: {
					Actions: []ActionRule{{
						Action: action_NudgeAssembleRequest,
						If:     guard_Not(guard_AssembleTimeoutExceeded),
					}},
					Transitions: []Transition{{
						To: State_Pooled,
						If: guard_AssembleTimeoutExceeded,
						On: action_IncrementAssembleErrors,
					}},
				},
				Event_Assemble_Revert_Response: {
					Validator: validator_MatchesPendingAssembleRequest,
					Transitions: []Transition{{
						To: State_Reverted,
					}},
				},
			},
		},
		State_Endorsement_Gathering: {
			OnTransitionTo: action_SendEndorsementRequests,
			Events: map[EventType]EventHandler{
				Event_Endorsed: {
					Transitions: []Transition{
						{
							To: State_Confirming_Dispatch,
							If: guard_And(guard_AttestationPlanFulfilled, guard_Not(guard_HasDependenciesNotReady)),
						},
						{
							To: State_Blocked,
							If: guard_And(guard_AttestationPlanFulfilled, guard_HasDependenciesNotReady),
						},
					},
				},
				Event_EndorsedRejected: {
					Transitions: []Transition{
						{
							To: State_Pooled,
							On: action_IncrementAssembleErrors,
						},
					},
				},
				Event_RequestTimeoutInterval: {
					Actions: []ActionRule{{
						Action: action_NudgeEndorsementRequests,
					}},
				},
			},
		},
		State_Blocked: {
			Events: map[EventType]EventHandler{
				Event_DependencyReady: {
					Transitions: []Transition{{
						To: State_Confirming_Dispatch,
						If: guard_And(guard_AttestationPlanFulfilled, guard_Not(guard_HasDependenciesNotReady)),
					}},
				},
			},
		},
		State_Confirming_Dispatch: {
			OnTransitionTo: action_SendDispatchConfirmationRequest,
			Events: map[EventType]EventHandler{
				Event_DispatchConfirmed: {
					Validator: validator_MatchesPendingDispatchConfirmationRequest,
					Transitions: []Transition{
						{
							To: State_Ready_For_Dispatch,
						}},
				},
				Event_RequestTimeoutInterval: {
					Actions: []ActionRule{{
						Action: action_NudgeDispatchConfirmationRequest,
					}},
				},
			},
		},
		State_Ready_For_Dispatch: {
			OnTransitionTo: action_NotifyDependentsOfReadiness, //TODO also at this point we should notify the dispatch thread to come and collect this transaction
			Events: map[EventType]EventHandler{
				Event_Collected: {
					Transitions: []Transition{
						{
							To: State_Dispatched,
						}},
				},
			},
		},
		State_Dispatched: {
			Events: map[EventType]EventHandler{
				Event_Submitted: {
					Transitions: []Transition{
						{
							To: State_Submitted,
						}},
				},
				Event_NonceAllocated: {},
			},
		},
		State_Submitted: {
			Events: map[EventType]EventHandler{
				Event_Confirmed: {
					Transitions: []Transition{
						{
							If: guard_Not(guard_HasRevertReason),
							To: State_Confirmed,
						},
						{
							If: guard_HasRevertReason,
							To: State_Pooled,
						},
					},
				},
			},
		},
		State_Reverted: {
			OnTransitionTo: action_NotifyDependentsOfRevert,
			Events: map[EventType]EventHandler{
				common.Event_HeartbeatInterval: {
					Transitions: []Transition{
						{
							If: guard_HasGracePeriodPassedSinceStateChange,
							To: State_Final,
						}},
				},
			},
		},
		State_Confirmed: {
			Events: map[EventType]EventHandler{
				common.Event_HeartbeatInterval: {
					Transitions: []Transition{
						{
							If: guard_HasGracePeriodPassedSinceStateChange,
							To: State_Final,
						}},
				},
			},
		},
		State_Final: {
			OnTransitionTo: action_Cleanup,
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
				log.L(ctx).Errorf("Error validating event %s: %v", event.TypeString(), err)
				return nil, err
			}
			if !valid {
				//This is perfectly normal sometimes an event happens and is no longer relevant to the transaction so we just ignore it and move on
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

// Function applyEvent updates the internal state of the Transaction with information from the event
// this happens before the state machine is evaluated for transitions that may be triggered by the event
// so that any guards on the transition rules can take into account the new internal state of the Transaction after this event has been applied
func (t *Transaction) applyEvent(ctx context.Context, event common.Event) error {
	var err error
	switch event := event.(type) {
	case *AssembleSuccessEvent:
		err = t.applyPostAssembly(ctx, event.PostAssembly)
		if err == nil {
			err = t.writeLockAndDistributeStates(ctx)
		}
	case *AssembleRevertResponseEvent:
		err = t.applyPostAssembly(ctx, event.PostAssembly)
	case *EndorsedEvent:
		err = t.applyEndorsement(ctx, event.Endorsement, event.RequestID)
	case *EndorsedRejectedEvent:
		err = t.applyEndorsementRejection(ctx, event.RevertReason, event.Party, event.AttestationRequestName)
	case *DispatchConfirmedEvent:
		err = t.applyDispatchConfirmation(ctx, event.RequestID)
	case *CollectedEvent:
		t.signerAddress = &event.SignerAddress
	case *NonceAllocatedEvent:
		t.nonce = &event.Nonce
	case *SubmittedEvent:
		t.latestSubmissionHash = &event.SubmissionHash
	case *ConfirmedEvent:
		t.revertReason = event.RevertReason
	case *common.HeartbeatIntervalEvent:
		t.heartbeatIntervalsSinceStateChange++
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
			previousState := sm.currentState
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

			// if there is a state change notification function, run it
			if t.notifyOfTransition != nil {
				t.notifyOfTransition(ctx, t, sm.currentState, previousState)

			}
			t.heartbeatIntervalsSinceStateChange = 0
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

func (s State) String() string {
	switch s {
	case State_Initial:
		return "Initial"
	case State_Pooled:
		return "Pooled"
	case State_PreAssembly_Blocked:
		return "PreAssembly_Blocked"
	case State_Assembling:
		return "Assembling"
	case State_Reverted:
		return "Reverted"
	case State_Endorsement_Gathering:
		return "State_Endorsement_Gathering"
	case State_Blocked:
		return "Blocked"
	case State_Confirming_Dispatch:
		return "State_Confirming_Dispatch"
	case State_Ready_For_Dispatch:
		return "Ready_For_Dispatch"
	case State_Dispatched:
		return "Dispatched"
	case State_Submitted:
		return "Submitted"
	case State_Confirmed:
		return "Confirmed"
	case State_Final:
		return "Final"
	}
	return fmt.Sprintf("Unknown (%d)", s)
}
