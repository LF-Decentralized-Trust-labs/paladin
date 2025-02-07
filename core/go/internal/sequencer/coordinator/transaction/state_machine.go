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
	"sync"

	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

type State int

const (
	State_Pooled State = iota // waiting in the pool to be assembled
	//TODO State_PreAssembly_Blocked is this the best name?  Should probably rename State_Blocked to State_Endorsement_Gathering_Blocked
	State_PreAssembly_Blocked   // has not been assembled yet and cannot be assembled because a dependency never got assembled successfully - i.e. it was either Parked or Reverted is also blocked
	State_Assembling            // an assemble request has been sent but we are waiting for the response
	State_Reverted              // the transaction has been reverted by the assembler/sender
	State_Endorsement_Gathering // assembled and waiting for endorsement
	State_Blocked               // is fully endorsed but cannot proceed due to dependencies not being ready for dispatch
	State_Confirming_Dispatch   // endorsed and waiting for dispatch confirmation
	State_Ready_For_Dispatch    // dispatch confirmation received and waiting to be collected by the dispatcher thread.Going into this state is the point of no return
	State_Dispatched            // collected by the dispatcher thread but not yet
	State_Submitted             // at least one submission has been made to the blockchain
	State_Confirmed             // "recently" confirmed on the base ledger.  NOTE: confirmed transactions are not held in memory for ever so getting a list of confirmed transactions will only return those confirmed recently
)

var allStates = []State{
	State_Pooled,
	State_Assembling,
	State_Endorsement_Gathering,
	State_Confirming_Dispatch,
	State_Ready_For_Dispatch,
	State_Dispatched,
	State_Submitted,
	State_Confirmed,
}

type EventType = common.EventType

const (
	Event_Selected                     EventType = iota // selected from the pool as the next transaction to be assembled
	Event_AssembleRequestSent                           // assemble request sent to the assembler
	Event_Assemble_Success                              // assemble response received from the sender
	Event_Assemble_Revert_Response                      // assemble response received from the sender with a revert reason
	Event_Endorsed                                      // endorsement received from one endorser
	Event_EndorsedRejected                              // endorsement received from one endorser with a revert reason
	Event_DependencyReady                               // another transaction, for which this transaction has a dependency on, has become ready for dispatch
	Event_DependencyReverted                            // another transaction, for which this transaction has a dependency on, has been reverted
	Event_DispatchConfirmed                             // dispatch confirmation received from the sender
	Event_DispatchConfirmationRejected                  // dispatch confirmation response received from the sender with a rejection
	Event_Collected                                     // collected by the dispatcher thread
	Event_NonceAllocated                                // nonce allocated by the dispatcher thread
	Event_Submitted                                     // submission made to the blockchain.  Each time this event is received, the submission hash is updated
	Event_Confirmed                                     // confirmation received from the blockchain of either a successful or reverted transaction
	Event_HeartbeatInterval                             // heartbeat interval has passed since the last state change or since the last heartbeat interval

)

type StateMachine struct {
	currentState State
}

type Transition struct {
	To State
	If Guard
}

type OnTransitionTo func(ctx context.Context, txn *Transaction, to, from State) error

type StateDefinition struct {
	OnTransitionTo OnTransitionTo             // function to be invoked when transitioning into this state
	Transitions    map[EventType][]Transition // rules to define when to exit this state and which state to transition to
}

// Initialize state definitions in a function to avoid circular dependencies
var stateDefinitionsMap map[State]StateDefinition
var stateDefinitionsLock sync.Mutex

func stateDefinitions() map[State]StateDefinition {
	if stateDefinitionsMap != nil {
		return stateDefinitionsMap
	}
	stateDefinitionsLock.Lock()
	defer stateDefinitionsLock.Unlock()
	if stateDefinitionsMap != nil {
		return stateDefinitionsMap
	}
	stateDefinitionsMap = map[State]StateDefinition{
		//TODO rules for transition from initial state to either pooled (selectable) or blocked or whatever, depending on the state of dependency transactions
		State_Pooled: {
			OnTransitionTo: action_initializeDependencies,
			Transitions: map[EventType][]Transition{
				Event_Selected: {{
					To: State_Assembling,
				}},
				Event_DependencyReverted: {{
					To: State_PreAssembly_Blocked,
				}},
			},
		},
		State_Assembling: {
			OnTransitionTo: action_SendAssembleRequest,
			Transitions: map[EventType][]Transition{
				Event_Assemble_Success: {{
					To: State_Endorsement_Gathering,
				}},
				Event_HeartbeatInterval: {{
					To: State_Pooled,
					If: guard_AssembleTimeoutExceeded,
				}},
				Event_Assemble_Revert_Response: {{
					To: State_Reverted,
				}},
			},
		},
		State_Endorsement_Gathering: {
			OnTransitionTo: action_SendEndorsementRequests,
			Transitions: map[EventType][]Transition{
				Event_Endorsed: {
					{
						To: State_Confirming_Dispatch,
						If: guard_And(guard_AttestationPlanFulfilled, guard_NoDependenciesNotReady),
					},
					{
						To: State_Blocked,
						If: guard_And(guard_AttestationPlanFulfilled, guard_HasDependenciesNotReady),
					},
				},
			},
		},
		State_Blocked: {
			OnTransitionTo: nil,
			Transitions: map[EventType][]Transition{
				Event_DependencyReady: {{
					To: State_Confirming_Dispatch,
					If: guard_And(guard_AttestationPlanFulfilled, guard_NoDependenciesNotReady),
				}},
			},
		},
		State_Confirming_Dispatch: {
			OnTransitionTo: action_SendDispatchConfirmationRequest,
			Transitions: map[EventType][]Transition{
				Event_DispatchConfirmed: {{
					To: State_Ready_For_Dispatch,
				}},
			},
		},
		State_Ready_For_Dispatch: {
			OnTransitionTo: action_NotifyDependentsOfReadiness, //TODO also at this point we should notify the dispatch thread to come and collect this transaction
			Transitions: map[EventType][]Transition{
				Event_Collected: {{
					To: State_Dispatched,
				}},
			},
		},
		State_Dispatched: {
			OnTransitionTo: nil,
			Transitions: map[EventType][]Transition{
				Event_Submitted: {{
					To: State_Submitted,
				}},
			},
		},
		State_Submitted: {
			OnTransitionTo: nil,
			Transitions: map[EventType][]Transition{
				Event_Confirmed: {{
					To: State_Confirmed,
				}},
			},
		},
		State_Reverted: {
			OnTransitionTo: action_NotifyDependentsOfRevert,
		},
		//TODO add transition to final state and call cleanup
	}
	return stateDefinitionsMap
}

func (t *Transaction) InitializeStateMachine(initialState State) {
	t.stateMachine = &StateMachine{
		currentState: initialState,
	}
}

// TODO break this out to 2 explicit steps a) applyEvent[InCurrentState] and b) evaluateTransition[ToNewState]
func (t *Transaction) HandleEvent(ctx context.Context, event Event) {
	sm := t.stateMachine

	switch event := event.(type) {
	case *SelectedEvent:
		//TODO
	case *AssembleRequestSentEvent:
		//TODO
	case *AssembleSuccessEvent:
		t.applyPostAssembly(ctx, event.postAssembly)
	case *AssembleRevertResponseEvent:
		t.applyPostAssembly(ctx, event.postAssembly)
	case *EndorsedEvent:
		t.applyEndorsement(ctx, event.Endorsement, event.RequestID)
	case *DispatchConfirmedEvent:
		t.applyDispatchConfirmation(ctx, event.RequestID)
	case *DispatchConfirmationRejectedEvent:
		//TODO
	case *CollectedEvent:
		//TODO
	case *NonceAllocatedEvent:
		//TODO
	case *SubmittedEvent:
		//TODO
	case *ConfirmedEvent:
		//TODO
	}

	//Determine whether this event triggers a state transition
	transitions := stateDefinitions()[sm.currentState].Transitions
	if transitionRules, ok := transitions[event.Type()]; ok {
		for _, rule := range transitionRules {
			if rule.If == nil || rule.If(ctx, t) { //if there is no guard defined, or the guard returns true
				log.L(ctx).Infof("Transaction %s transitioning from %v to %v triggered by event %v", t.ID.String(), sm.currentState, rule.To, event.Type())
				fromState := sm.currentState
				sm.currentState = rule.To
				newStateDefinition := stateDefinitions()[sm.currentState]
				if newStateDefinition.OnTransitionTo != nil {
					err := newStateDefinition.OnTransitionTo(ctx, t, rule.To, fromState)
					if err != nil {
						//TODO any recoverable errors should have been handled by the OnTransitionTo function so this is a panic or at least, abort the coordinator for this contract
						log.L(ctx).Errorf("Error transitioning to state %v: %v", sm.currentState, err)
						break
					}
				} else {
					log.L(ctx).Debugf("No OnTransitionTo function defined for state %v", sm.currentState)
				}
				t.heartbeatIntervalsSinceStateChange = 0
				break
			}
		}

	} else {
		log.L(ctx).Debugf("No transition for Event %v from State %s", event.Type(), sm.currentState.String())
	}
}

func action_SendAssembleRequest(ctx context.Context, txn *Transaction, to, from State) error {
	return txn.sendAssembleRequest(ctx)
}

func action_SendEndorsementRequests(ctx context.Context, txn *Transaction, to, from State) error {
	return txn.sendEndorsementRequests(ctx)
}

func action_SendDispatchConfirmationRequest(ctx context.Context, txn *Transaction, to, from State) error {
	return txn.sendDispatchConfirmationRequest(ctx)
}

func action_NotifyDependentsOfReadiness(ctx context.Context, txn *Transaction, to, from State) error {
	return txn.notifyDependentsOfReadiness(ctx)
}

func action_NotifyDependentsOfRevert(ctx context.Context, txn *Transaction, to, from State) error {
	return txn.notifyDependentsOfRevert(ctx)
}

func action_initializeDependencies(ctx context.Context, txn *Transaction, to, from State) error {
	return txn.initializeDependencies(ctx)
}

func (s *State) String() string {
	switch *s {
	case State_Pooled:
		return "Pooled"
	case State_Assembling:
		return "Assembling"
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
	}
	return "Unknown"
}
