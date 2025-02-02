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
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

type State int
type EventType = common.EventType

const (
	State_Idle      State = iota //Not acting as a coordinator and not aware of any other active coordinators
	State_Observing              //Not acting as a coordinator but aware of another node acting as a coordinator
	State_Elect                  //Elected to take over from another coordinator and waiting for handover information
	State_Standby                //
	State_Prepared
	State_Active
	State_Flush
	State_Closing
)

var allStates = []State{State_Idle, State_Observing, State_Elect, State_Standby, State_Prepared, State_Active, State_Flush, State_Closing}

const (
	Event_Activated EventType = iota
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
	Event_HeartbeatInterval
)

func (s *State) OnTransitionTo(ctx context.Context, c *coordinator, from State, event Event) {
	log.L(context.Background()).Debugf("Transitioning from %v to %v triggered by event %v", from, s, event)
	switch *s {
	case State_Observing:
		//TODO - start observing the active coordinator
	case State_Elect:
		c.sendHandoverRequest(ctx)
	case State_Prepared:
	}

}

type StateMachine struct {
	currentState State
	transitions  map[State]map[EventType][]Transition
	handlers     map[State]EventHandlers
	//coordinator  coordinator
}

type EventHandlers struct {
	OnTransactionsDelegated        func(ctx context.Context, event *TransactionsDelegatedEvent) error
	OnTransactionConfirmed         func(ctx context.Context, event *TransactionConfirmedEvent) error
	OnTransactionDispatchConfirmed func(ctx context.Context, event *TransactionDispatchConfirmedEvent) error
	OnNewBlock                     func(ctx context.Context, event *NewBlockEvent) error
	OnHeartbeatReceived            func(ctx context.Context, event *HeartbeatReceivedEvent) error
	OnHeartbeatInterval            func(ctx context.Context, event *HeartbeatIntervalEvent) error
}

type Transition struct {
	To State
	If Guard
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
	sm.transitions = map[State]map[EventType][]Transition{
		State_Idle: {
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
		State_Observing: {
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
		State_Standby: {
			Event_NewBlock: {
				{
					To: State_Elect,
					If: notBehind,
				},
			},
		},
		State_Elect: {
			Event_HandoverReceived: {
				{
					To: State_Prepared,
				},
			},
		},
		State_Prepared: {
			Event_TransactionConfirmed: {
				{
					To: State_Active,
					If: activeCoordinatorFlushComplete,
				},
			},
		},
		State_Active: {
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
		State_Flush: {
			Event_TransactionConfirmed: {
				{
					To: State_Closing,
					If: flushComplete,
				},
			},
		},
		State_Closing: {
			Event_HeartbeatInterval: {
				{
					To: State_Idle,
					If: closingGracePeriodExpired,
				},
			},
		},
	}

	//Event handlers are functions that are called when an event is received while in a particular state,
	//even if the event does not cause a state transition, it may still have an effect on the internal fine grained state of the coordinator
	//or its subordinate state machines (transactions)
	sm.handlers = map[State]EventHandlers{
		State_Active: {
			OnTransactionsDelegated: func(ctx context.Context, delegatedEvent *TransactionsDelegatedEvent) error {
				c.addToDelegatedTransactions(ctx, delegatedEvent.Sender, delegatedEvent.Transactions)
				return nil
			},
			OnTransactionDispatchConfirmed: func(ctx context.Context, event *TransactionDispatchConfirmedEvent) error {
				c.propagateEventToTransaction(ctx, event)
				return nil
			},
		},
		State_Elect: {
			OnTransactionsDelegated: func(ctx context.Context, delegatedEvent *TransactionsDelegatedEvent) error {
				c.addToDelegatedTransactions(ctx, delegatedEvent.Sender, delegatedEvent.Transactions)
				return nil
			},
		},
	}
	//Some events are handled the same regardless of the state
	for _, state := range allStates {
		handler := sm.handlers[state]
		handler.OnNewBlock = func(ctx context.Context, event *NewBlockEvent) error {
			c.currentBlockHeight = event.BlockHeight
			return nil
		}
		handler.OnHeartbeatReceived = func(ctx context.Context, event *HeartbeatReceivedEvent) error {
			c.activeCoordinator = event.From
			c.activeCoordinatorBlockHeight = event.BlockHeight
			for _, flushPoint := range event.FlushPoints {
				c.activeCoordinatorsFlushPointsBySignerNonce[flushPoint.GetSignerNonce()] = flushPoint
			}
			return nil
		}
		handler.OnTransactionConfirmed = func(ctx context.Context, event *TransactionConfirmedEvent) error {
			//This may be a confirmation of a transaction that we have have been coordinating or it may be one that another coordinator has been coordinating
			//if the latter, then we may or may not know about it depending on whether we have seen a heartbeat from that coordinator since last time
			// we were loaded into memory
			//TODO - we can't actually guarantee that we have all transactions we dispatched in memory.
			//Even assuming that the public txmgr is in the same process (may not be true forever)  and assuming that we haven't been swapped out ( likely not to be true very soon) there is still a chance that the transaction was submitted to the base ledger, then the process restarted then we get the confirmation.				//When the process starts, we need to make sure that the coordinator is pre loaded with knowledge of all transactions that it has dispatched
			if !c.confirmDispatchedTransaction(ctx, event.From, event.Nonce, event.Hash, event.RevertReason) {
				c.confirmMonitoredTransaction(ctx, event.From, event.Nonce)
			}
			return nil
		}
		handler.OnHeartbeatInterval = func(ctx context.Context, event *HeartbeatIntervalEvent) error {
			c.heartbeatIntervalsSinceStateChange++
			return nil
		}
		sm.handlers[state] = handler
	}

}

func (c *coordinator) HandleEvent(ctx context.Context, event Event) {

	sm := c.stateMachine

	// First apply the event to the update the internal fine grained state of the coordinator if there is any handler registered for the current state
	switch event := event.(type) {
	case *TransactionsDelegatedEvent:
		if handler := sm.handlers[sm.currentState].OnTransactionsDelegated; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring TransactionsDelegatedEvent while in State %s", sm.currentState.String())
		}
	case *TransactionConfirmedEvent:
		if handler := sm.handlers[sm.currentState].OnTransactionConfirmed; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring TransactionConfirmedEvent while in State %s", sm.currentState.String())
		}
	case *TransactionDispatchConfirmedEvent:
		if handler := sm.handlers[sm.currentState].OnTransactionDispatchConfirmed; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring TransactionDispatchConfirmedEvent while in State %s", sm.currentState.String())
		}
	case *NewBlockEvent:
		if handler := sm.handlers[sm.currentState].OnNewBlock; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring NewBlockEvent while in State %s", sm.currentState.String())
		}
	case *HeartbeatReceivedEvent:
		if handler := sm.handlers[sm.currentState].OnHeartbeatReceived; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring HeartbeatReceivedEvent while in State %s", sm.currentState.String())
		}
	case *HeartbeatIntervalEvent:
		if handler := sm.handlers[sm.currentState].OnHeartbeatInterval; handler != nil {
			handler(ctx, event)
		} else {
			log.L(ctx).Debugf("Ignoring HeartbeatIntervalEvent while in State %s", sm.currentState.String())
		}
	}

	//Determine whether this event triggers a state transition
	if transitionRules, ok := sm.transitions[sm.currentState][event.Type()]; ok {
		for _, rule := range transitionRules {
			if rule.If == nil || rule.If(ctx, c) { //if there is no guard defined, or the guard returns true
				fromState := sm.currentState
				sm.currentState = rule.To
				sm.currentState.OnTransitionTo(ctx, c, fromState, event)
				c.heartbeatIntervalsSinceStateChange = 0
				break
			}
		}

	} else {
		log.L(ctx).Debugf("No transition for Event %v from State %s", event.Type(), sm.currentState.String())
	}

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
