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

package sender

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/sender/transaction"
	"github.com/google/uuid"
)

type State int
type EventType = common.EventType

const (
	State_Idle      State = iota //Not acting as a sender and not aware of any active coordinators
	State_Observing              //Not acting as a sender but aware of a node (which may be the same node) acting as a coordinator
	State_Sending                //Has some transactions that have been sent to a coordinator but not yet confirmed TODO should this be named State_Monitoring or State_Delegated or even State_Sent.  Sending sounds like it is in the process of sending the request message.
)

const (
	Event_HeartbeatInterval                EventType = iota + 300 // the heartbeat interval has passed since the last time a heartbeat was received or the last time this event was received
	Event_HeartbeatReceived                                       // a heartbeat message was received from the current active coordinator
	Event_TransactionCreated                                      // a new transaction has been created and is ready to be sent to the coordinator TODO maybe name something like Intent created?
	Event_TransactionConfirmed                                    // a transaction, that was send by this sender, has been confirmed on the base ledger
	Event_NewBlock                                                // a new block has been mined on the base ledger
	Event_Base_Ledger_Transaction_Reverted                        // A transaction has moved from the dispatched to pending state because it was reverted on the base ledger
)

type StateMachine struct {
	currentState State
}

// Actions can be specified for transition to a state either as the OnTransitionTo function that will run for all transitions to that state or as the On field in the Transition struct if the action applies
// for a specific transition
type Action func(ctx context.Context, s *sender) error
type ActionRule struct {
	Action Action
	If     Guard
}

type Guard func(ctx context.Context, s *sender) bool

type Transition struct {
	To State // State to transition to if the guard condition is met
	If Guard // Condition to evaluate the transaction against to determine if this transition should be taken
	On Action
}

type EventHandler struct {
	Validator   func(ctx context.Context, s *sender, event common.Event) (bool, error) // function to validate whether the event is valid for the current state of the sender.  This is optional.  If not defined, the event is always considered valid.
	Actions     []ActionRule                                                           // list of actions to be taken when this event is received.  These actions are run before any transition specific actions
	Transitions []Transition                                                           // list of transitions that this event could trigger.  The list is ordered so the first matching transition is the one that will be taken.
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
				Event_HeartbeatReceived: {
					Transitions: []Transition{{
						To: State_Observing,
					}},
				},
				Event_TransactionCreated: {
					Transitions: []Transition{{
						To: State_Sending,
						On: action_SendDelegationRequest,
					}},
				},
			},
		},
		State_Observing: {
			Events: map[EventType]EventHandler{
				Event_HeartbeatInterval: {
					Transitions: []Transition{{
						To: State_Idle,
						If: guard_HeartbeatThresholdExceeded,
					}},
				},
				Event_TransactionCreated: {
					Transitions: []Transition{{
						To: State_Sending,
						On: action_SendDelegationRequest,
					}},
				},
				Event_NewBlock:          {},
				Event_HeartbeatReceived: {},
			},
		},
		State_Sending: {
			Events: map[EventType]EventHandler{
				Event_TransactionConfirmed: {
					Transitions: []Transition{{
						To: State_Observing,
						If: guard_Not(guard_HasUnconfirmedTransactions),
					}},
				},
				Event_TransactionCreated: {
					Actions: []ActionRule{{
						Action: action_SendDelegationRequest,
					}},
				},
				Event_NewBlock: {},
				Event_HeartbeatReceived: {
					Actions: []ActionRule{{
						If:     guard_HasDroppedTransactions,
						Action: action_SendDroppedTXDelegationRequest,
					}},
				},
				Event_Base_Ledger_Transaction_Reverted: {
					Actions: []ActionRule{{
						Action: action_SendDelegationRequest, //TODO Is this redundant?, coordinator should retry this unless it has dropped the transaction and we already handle the dropped case
					}},
				},
			},
		},
	}
}

func (s *sender) InitializeStateMachine(initialState State) {
	s.stateMachine = &StateMachine{
		currentState: initialState,
	}
}

// Process a state machine event immediately. Should only be called on the sequencer loop, or in tests to avoid timing conditions
func (s *sender) ProcessEvent(ctx context.Context, event common.Event) error {
	log.L(ctx).Infof("Distributed sender handling new event (contract address %s, node name %s)", s.contractAddress, s.nodeName)

	if transactionEvent, ok := event.(transaction.Event); ok {
		log.L(ctx).Infof("Propagating transaction event to transaction: %s", transactionEvent.TypeString())
		return s.propagateEventToTransaction(ctx, transactionEvent)
	}

	//determine whether this event is valid for the current state
	log.L(ctx).Infof("Evaluating event: %s", event.TypeString())
	eventHandler, err := s.evaluateEvent(ctx, event)
	if err != nil || eventHandler == nil {
		return err
	}

	//If we get here, the state machine has defined a rule for handling this event
	//Apply the event to the coordinator to update the internal state
	// so that the guards and actions defined in the state machine can reference the new internal state of the coordinator
	log.L(ctx).Infof("Applying event: %s", event.TypeString())
	err = s.applyEvent(ctx, event)
	if err != nil {
		return err
	}

	err = s.performActions(ctx, *eventHandler)
	if err != nil {
		return err
	}

	//Determine whether this event triggers a state transition
	err = s.evaluateTransitions(ctx, event, *eventHandler)
	return err
}

// Queue a state machine event for the sequencer loop to process. Should be called by most Paladin components to ensure memory integrity of
// sequencer state machine and transactions.
func (s *sender) QueueEvent(ctx context.Context, event common.Event) error {
	log.L(ctx).Infof("Pushing sender event onto event queue: %s", event.TypeString())
	s.senderEvents <- event
	log.L(ctx).Infof("Pushed sender event onto event queue: %s", event.TypeString())
	return nil
}

func (s *sender) SetActiveCoordinator(ctx context.Context, coordinator string) error {
	if coordinator == "" {
		return i18n.NewError(ctx, msgs.MsgDistSeqInternalError, "Cannot set active coordinator to an empty string")
	}
	if s.activeCoordinatorNode != "" {
		return i18n.NewError(ctx, msgs.MsgDistSeqInternalError, "SetActiveCoordinator should only be used when an active coordinator has not yet been set")
	}
	s.activeCoordinatorNode = coordinator
	log.L(ctx).Infof("[Sequencer] initial active coordinator set to %s", s.activeCoordinatorNode)
	return nil
}

func (s *sender) GetTxStatus(ctx context.Context, txID uuid.UUID) (status components.PrivateTxStatus, err error) {
	if txn, ok := s.transactionsByID[txID]; !ok {
		endorsements := txn.GetEndorsementStatus(ctx)
		return components.PrivateTxStatus{
			TxID:         txID.String(),
			Status:       txn.GetCurrentState().String(),
			LatestEvent:  txn.GetLatestEvent(),
			Endorsements: endorsements,
			// MRW TODO - latest error, failure message etc.
		}, nil
	}
	return components.PrivateTxStatus{
		TxID:   txID.String(),
		Status: "unknown",
	}, nil
}

// Function evaluateEvent evaluates whether the event is relevant given the current state of the coordinator
func (s *sender) evaluateEvent(ctx context.Context, event common.Event) (*EventHandler, error) {
	sm := s.stateMachine

	//Determine if and how this event applies in the current state and which, if any, transition it triggers
	eventHandlers := stateDefinitionsMap[sm.currentState].Events
	eventHandler, isHandlerDefined := eventHandlers[event.Type()]
	if isHandlerDefined {
		//By default all events in the list are applied unless there is a validator function and it returns false
		if eventHandler.Validator != nil {
			valid, err := eventHandler.Validator(ctx, s, event)
			if err != nil {
				//This is an unexpected error.  If the event is invalid, the validator should return false and not an error
				log.L(ctx).Errorf("[Sequencer] error validating event %s: %v", event.TypeString(), err)
				return nil, err
			}
			if !valid {
				//This is perfectly normal sometimes an event happens and is no longer relevant to the coordinator so we just ignore it and move on
				log.L(ctx).Debugf("[Sequencer] sender event %s is not valid for current state %s", event.TypeString(), sm.currentState.String())
				return nil, nil
			}
		}
		return &eventHandler, nil
	} else {
		// no event handler defined for this event while in this state
		log.L(ctx).Debugf("[Sequencer] no sender event handler defined for Event %s in State %s", event.TypeString(), sm.currentState.String())
		return nil, nil
	}
}

// Function applyEvent updates the internal state of the coordinator with information from the event
// this happens before the state machine is evaluated for transitions that may be triggered by the event
// so that any guards on the transition rules can take into account the new internal state of the coordinator after this event has been applied
func (s *sender) applyEvent(ctx context.Context, event common.Event) error {
	var err error
	// First apply the event to the update the internal fine grained state of the coordinator if there is any handler registered for the current state
	switch event := event.(type) {
	case *HeartbeatReceivedEvent:
		err = s.applyHeartbeatReceived(ctx, event)
	case *TransactionCreatedEvent:
		err = s.createTransaction(ctx, event.Transaction)
	case *TransactionConfirmedEvent:
		err = s.confirmTransaction(ctx, event.From, event.Nonce, event.Hash, event.RevertReason)
	default:
		log.L(ctx).Debugf("[Sequencer] no action defined for event %T", event)

	}
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] error applying event %v: %v", event.Type(), err)
	}
	return err
}

func (s *sender) performActions(ctx context.Context, eventHandler EventHandler) error {
	for _, rule := range eventHandler.Actions {
		if rule.If == nil || rule.If(ctx, s) {
			err := rule.Action(ctx, s)
			if err != nil {
				//any recoverable errors should have been handled by the action function
				log.L(ctx).Errorf("[Sequencer] error applying action: %v", err)
				return err
			}
		}
	}
	return nil
}

func (s *sender) evaluateTransitions(ctx context.Context, event common.Event, eventHandler EventHandler) error {
	sm := s.stateMachine

	for _, rule := range eventHandler.Transitions {
		if rule.If == nil || rule.If(ctx, s) { //if there is no guard defined, or the guard returns true
			log.L(log.WithComponent(ctx, common.SUBCOMP_STATE)).Debugf("sdr      | %s | %T | %s -> %s", s.contractAddress.String()[0:8], event, sm.currentState.String(), rule.To.String())

			sm.currentState = rule.To
			newStateDefinition := stateDefinitionsMap[sm.currentState]
			//run any actions specific to the transition first
			if rule.On != nil {
				err := rule.On(ctx, s)
				if err != nil {
					//any recoverable errors should have been handled by the action function
					log.L(ctx).Errorf("[Sequencer] error transitioning sender to state %v: %v", sm.currentState, err)
					return err
				}
			}

			// then run any actions for the state entry
			if newStateDefinition.OnTransitionTo != nil {
				err := newStateDefinition.OnTransitionTo(ctx, s)
				if err != nil {
					// any recoverable errors should have been handled by the OnTransitionTo function
					log.L(ctx).Errorf("[Sequencer] error transitioning sender to state %v: %v", sm.currentState, err)
					return err
				}
			} else {
				log.L(ctx).Debugf("[Sequencer] no OnTransitionTo function defined for state %v", sm.currentState)
			}

			break
		}
	}
	return nil

}

func guard_Not(guard Guard) Guard {
	return func(ctx context.Context, s *sender) bool {
		return !guard(ctx, s)
	}
}

func (s State) String() string {
	switch s {
	case State_Idle:
		return "Idle"
	case State_Observing:
		return "Observing"
	case State_Sending:
		return "Sending"
	}
	return "Unknown"
}
