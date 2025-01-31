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
	State_Pooled             State = iota //waiting in the pool to be assembled
	State_Assembling                      // an assemble request has been sent but we are waiting for the response
	State_Assembled                       // assembled and waiting for endorsement
	State_Endorsed                        // endorsed and waiting for dispatch confirmation
	State_Ready_For_Dispatch              // dispatch confirmation received and waiting to be collected by the dispatcher thread.Going into this state is the point of no return
	State_Dispatched                      // collected by the dispatcher thread but not yet
	State_Submitted                       // at least one submission has been made to the blockchain
	State_Confirmed                       // "recently" confirmed on the base ledger.  NOTE: confirmed transactions are not held in memory for ever so getting a list of confirmed transactions will only return those confirmed recently
)

type EventType = common.EventType

const (
	Event_Selected                 EventType = iota // selected from the pool as the next transaction to be assembled
	Event_AssembleRequestSent                       // assemble request sent to the assembler
	Event_Assembled                                 // assemble response received from the sender
	Event_Endorsed                                  // endorsement received from one endorser
	Event_AttestationPlanFulfilled                  // endorsed by all required endorsers
	Event_DispatchConfirmed                         // dispatch confirmation received from the sender
	Event_Collected                                 // collected by the dispatcher thread
	Event_NonceAllocated                            // nonce allocated by the dispatcher thread
	Event_Submitted                                 // submission made to the blockchain.  Each time this event is received, the submission hash is updated
	Event_Confirmed                                 // confirmation received from the blockchain of either a successful or reverted transaction

)

// List of functions, a subset of which can be registered with the state machine to handle events for each state
type EventHandlers struct {
	OnSelected            func(ctx context.Context, event *SelectedEvent) error
	OnAssembleRequestSent func(ctx context.Context, event *AssembleRequestSentEvent) error
	OnAssembled           func(ctx context.Context, event *AssembledEvent) error
	OnEndorsed            func(ctx context.Context, event *EndorsedEvent) error
	OnDispatchConfirmed   func(ctx context.Context, event *DispatchConfirmedEvent) error
	OnCollected           func(ctx context.Context, event *CollectedEvent) error
	OnNonceAllocated      func(ctx context.Context, event *NonceAllocatedEvent) error
	OnSubmitted           func(ctx context.Context, event *SubmittedEvent) error
	OnConfirmed           func(ctx context.Context, event *ConfirmedEvent) error
}

type StateMachine struct {
	currentState State
	transitions  map[State]map[EventType]State
	handlers     map[State]EventHandlers
	transaction  *Transaction
}

func (d *Transaction) InitializeStateMachine(initialState State) {
	d.stateMachine = &StateMachine{
		transaction:  d,
		currentState: initialState,
		transitions:  make(map[State]map[EventType]State),
	}

	//Transitions
	d.stateMachine.transitions = map[State]map[EventType]State{
		State_Pooled: {
			Event_Selected: State_Assembling,
		},
		State_Assembling: {
			Event_Assembled: State_Assembled,
		},
		State_Assembled: {
			Event_AttestationPlanFulfilled: State_Endorsed,
		},
		State_Endorsed: {
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

	//State initializers are functions that are called when the state machine enters a new state

	//Event handlers are functions that are called when an event is received while in a particular state
	d.stateMachine.handlers = map[State]EventHandlers{
		State_Pooled: {
			OnSelected: func(ctx context.Context, event *SelectedEvent) error {
				return d.sendAssembleRequest(ctx)
			},
		},
		State_Assembled: {
			OnEndorsed: func(ctx context.Context, event *EndorsedEvent) error {
				return d.sendEndorsementRequests(ctx)
			},
		},
	}

}

func (d *Transaction) HandleEvent(ctx context.Context, event Event) {
	sm := d.stateMachine

	if newState, ok := sm.transitions[sm.currentState][event.Type()]; ok {
		sm.currentState = newState
	} else {
		log.L(ctx).Debugf("No transition for Event %v from State %s", event.Type(), sm.currentState.String())
	}

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
	case *AssembledEvent:
		if handler := handlers.OnAssembled; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring AssembledEvent while in State %s", sm.currentState.String())
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
}

func (s *State) String() string {
	switch *s {
	case State_Pooled:
		return "Pooled"
	case State_Assembling:
		return "Assembling"
	case State_Assembled:
		return "Assembled"
	case State_Endorsed:
		return "Endorsed"
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
