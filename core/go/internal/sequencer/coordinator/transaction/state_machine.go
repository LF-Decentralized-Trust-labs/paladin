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

	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

type State int

const (
	State_Pooled                State = iota //waiting in the pool to be assembled
	State_Assembling                         // an assemble request has been sent but we are waiting for the response
	State_Endorsement_Gathering              // assembled and waiting for endorsement
	State_Confirming_Dispatch                // endorsed and waiting for dispatch confirmation
	State_Ready_For_Dispatch                 // dispatch confirmation received and waiting to be collected by the dispatcher thread.Going into this state is the point of no return
	State_Dispatched                         // collected by the dispatcher thread but not yet
	State_Submitted                          // at least one submission has been made to the blockchain
	State_Confirmed                          // "recently" confirmed on the base ledger.  NOTE: confirmed transactions are not held in memory for ever so getting a list of confirmed transactions will only return those confirmed recently
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
	Event_Selected            EventType = iota // selected from the pool as the next transaction to be assembled
	Event_AssembleRequestSent                  // assemble request sent to the assembler
	Event_Assemble_Success                     // assemble response received from the sender
	Event_Assemble_Revert                      // assemble response received from the sender with a revert reason
	Event_Endorsed                             // endorsement received from one endorser
	Event_EndorsedRejected                     // endorsement received from one endorser with a revert reason
	Event_DispatchConfirmed                    // dispatch confirmation received from the sender
	Event_Collected                            // collected by the dispatcher thread
	Event_NonceAllocated                       // nonce allocated by the dispatcher thread
	Event_Submitted                            // submission made to the blockchain.  Each time this event is received, the submission hash is updated
	Event_Confirmed                            // confirmation received from the blockchain of either a successful or reverted transaction

)

// TODO should we ever need to pass the event into the transition function?  The event should have been applied before the transition.
func (s *State) OnTransitionTo(ctx context.Context, txn *Transaction, from State, event Event) error {
	log.L(context.Background()).Debugf("Transitioning from %v to %v triggered by event %v", from, s, event)
	switch *s {
	case State_Assembling:
		return txn.sendAssembleRequest(ctx)
	case State_Endorsement_Gathering:
		return txn.sendEndorsementRequests(ctx)
	case State_Confirming_Dispatch:
		return txn.sendDispatchConfirmationRequest(ctx)
	}
	log.L(ctx).Infof("No transition function defined for state %v", s)
	return nil
}

type StateMachine struct {
	currentState State
	transitions  map[State]map[EventType][]Transition
	handlers     map[State]EventHandlers
	//transaction  *Transaction
}

// List of functions, a subset of which can be registered with the state machine to handle events for each state
type EventHandlers struct {
	OnSelected            func(ctx context.Context, event *SelectedEvent) error
	OnAssembleRequestSent func(ctx context.Context, event *AssembleRequestSentEvent) error
	OnAssembleSuccess     func(ctx context.Context, event *AssembleSuccessEvent) error
	OnAssembleRevert      func(ctx context.Context, event *AssembleRevertEvent) error
	OnEndorsed            func(ctx context.Context, event *EndorsedEvent) error
	OnDispatchConfirmed   func(ctx context.Context, event *DispatchConfirmedEvent) error
	OnCollected           func(ctx context.Context, event *CollectedEvent) error
	OnNonceAllocated      func(ctx context.Context, event *NonceAllocatedEvent) error
	OnSubmitted           func(ctx context.Context, event *SubmittedEvent) error
	OnConfirmed           func(ctx context.Context, event *ConfirmedEvent) error
}

type Transition struct {
	To State
	If Guard
}

func (d *Transaction) InitializeStateMachine(initialState State) {
	d.stateMachine = &StateMachine{
		//transaction:  d,
		currentState: initialState,
	}

	//Transitions
	d.stateMachine.transitions = map[State]map[EventType][]Transition{
		State_Pooled: {
			Event_Selected: {{
				To: State_Assembling,
			}},
		},
		State_Assembling: {
			Event_Assemble_Success: {{
				To: State_Endorsement_Gathering,
			}},
		},
		State_Endorsement_Gathering: {
			Event_Endorsed: {{
				To: State_Confirming_Dispatch,
				If: guard_AttestationPlanFulfilled,
			}},
		},
	}

	/*



			State_Confirming_Dispatch: {
				Event_DispatchConfirmed: State_Ready_For_Dispatch,
			},
			State_Ready_For_Dispatch: {
				Event_Collected: State_Dispatched,
			},
			State_Dispatched: {
				Event_Submitted: State_Submitted,
			},
			State_Submitted: {
				Event_Confirmed: State_Confirmed,
			},
		}
	*/

	//Event handlers are functions that are called when an event is received while in a particular state,
	//even if the event does not cause a state transition, it may still have an effect on the internal fine grained state of the coordinator
	//or its subordinate state machines (transactions)
	d.stateMachine.handlers = make(map[State]EventHandlers)

	//Some events are handled the same regardless of the state
	for _, state := range allStates {
		handler := d.stateMachine.handlers[state]
		handler.OnAssembleSuccess = func(ctx context.Context, event *AssembleSuccessEvent) error {
			d.PostAssembly = event.postAssembly
			return nil
		}

		handler.OnEndorsed = func(ctx context.Context, event *EndorsedEvent) error {
			d.applyEndorsement(ctx, event.Endorsement, event.RequestID)
			return nil
		}

		d.stateMachine.handlers[state] = handler
	}

}

func (t *Transaction) HandleEvent(ctx context.Context, event Event) {
	sm := t.stateMachine

	handlers := sm.handlers[sm.currentState]
	switch event := event.(type) {
	case *SelectedEvent:
		if handler := handlers.OnSelected; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring SelectedEvent while in State %s", sm.currentState.String())
		}
	case *AssembleRequestSentEvent:
		if handler := handlers.OnAssembleRequestSent; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring AssembleRequestSentEvent while in State %s", sm.currentState.String())
		}
	case *AssembleSuccessEvent:
		if handler := handlers.OnAssembleSuccess; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring AssembleSuccessEvent while in State %s", sm.currentState.String())
		}
	case *AssembleRevertEvent:
		if handler := handlers.OnAssembleRevert; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring AssembleRevertEvent while in State %s", sm.currentState.String())
		}
	case *EndorsedEvent:
		if handler := handlers.OnEndorsed; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring EndorsedEvent while in State %s", sm.currentState.String())
		}
	case *DispatchConfirmedEvent:
		if handler := handlers.OnDispatchConfirmed; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring DispatchConfirmedEvent while in State %s", sm.currentState.String())
		}
	case *CollectedEvent:
		if handler := handlers.OnCollected; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring CollectedEvent while in State %s", sm.currentState.String())
		}
	case *NonceAllocatedEvent:
		if handler := handlers.OnNonceAllocated; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring NonceAllocatedEvent while in State %s", sm.currentState.String())
		}
	case *SubmittedEvent:
		if handler := handlers.OnSubmitted; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring SubmittedEvent while in State %s", sm.currentState.String())
		}
	case *ConfirmedEvent:
		if handler := handlers.OnConfirmed; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring ConfirmedEvent while in State %s", sm.currentState.String())
		}

	}

	//Determine whether this event triggers a state transition
	if transitionRules, ok := sm.transitions[sm.currentState][event.Type()]; ok {
		for _, rule := range transitionRules {
			if rule.If == nil || rule.If(ctx, t) { //if there is no guard defined, or the guard returns true
				fromState := sm.currentState
				sm.currentState = rule.To
				err := sm.currentState.OnTransitionTo(ctx, t, fromState, event)
				if err != nil {
					//TODO what should we do if the transition fails?  Abandon the transaction? Retry the transition?
					//Any recoverable error should be handled by the transition function so only unrecoverable errors should be seen here
					log.L(ctx).Errorf("Error transitioning from %v to %v triggered by event %v: %v", fromState, sm.currentState, event, err)
				}
				t.heartbeatIntervalsSinceStateChange = 0
				break
			}
		}

	} else {
		log.L(ctx).Debugf("No transition for Event %v from State %s", event.Type(), sm.currentState.String())
	}
}

func (s *State) String() string {
	switch *s {
	case State_Pooled:
		return "Pooled"
	case State_Assembling:
		return "Assembling"
	case State_Endorsement_Gathering:
		return "State_Endorsement_Gathering"
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
