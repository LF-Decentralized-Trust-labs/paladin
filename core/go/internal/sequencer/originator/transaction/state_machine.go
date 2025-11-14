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
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

type State int

const (
	State_Initial State = iota // Initial state before anything is calculated
	State_Pending              // Intent for the transaction has been created in the database and has been assigned a unique ID but is not currently known to be being processed by a coordinator
	//TODO currently Pending doesn't really make sense as a state because it is an instantaneous state.  It is the state of the transaction when it is first created and then immediately transitions to Delegated.
	// States only really make sense when the transaction is waiting for something to happen.  We could remove this state and just have the transaction start in the Delegated state
	// However, there may be a need for the originator to only delegate a subset of the transactions ( e.g. maybe there is an absolute ordering requirement and the only way to achieve that is by holding back until the dependency is confirmed)
	// It is also slightly complicated by the fact that the delegation request is sent by an action of the originator state machine because it sends the delegation request for multiple transactions at once.
	// Need a decision point on whether that is done by a) transaction emitting and event that triggers the originator to send the delegation request or b) the transaction state machine action makes a syncronous call to the originator to include that transaction in a new delegation request.
	// NOTE: initially there was a thought that we needed a pending state in case there is no current active coordinator so we can't go straight to delegated.  However, the current model is that we don't actually wait for any response from the coordinator.  We simply send the delegation request and assume that it is delegated at that point.
	// We only resend the request if we don't see the heartbeat.
	// Might need to rethink this and allow for some ack and shorter retry interval to tolerate less reliable networks,
	// Need to make a decision and document it in a README
	State_Delegated             // the transaction has been sent to the current active coordinator
	State_Assembling            // the coordinator has sent an assemble request that we have not replied to yet
	State_Endorsement_Gathering //we have responded to an assemble request and are waiting the coordinator to gather endorsements and send us a dispatch confirmation request
	State_Signing               // we have assembled the transaction and are waiting for the signing module to sign it before we respond to the coordinator with the signed assembled transaction
	State_Prepared              // we know that the coordinator has got as far as preparing a public transaction and we have sent a positive response to a coordinator's dispatch confirmation request but have not yet received a heartbeat that notifies us that the coordinator has dispatched the transaction to a public transaction manager for submission
	State_Dispatched            // the active coordinator that this transaction was delegated to has dispatched the transaction to a public transaction manager for submission
	State_Sequenced             // the transaction has been assigned a nonce by the public transaction manager
	State_Submitted             // the transaction has been submitted to the blockchain
	State_Confirmed             // the public transaction has been confirmed by the blockchain as successful
	State_Reverted              // upon attempting to assemble the transaction, the domain code has determined that the intent is not valid and the transaction is finalized as reverted
	State_Parked                // upon attempting to assemble the transaction, the domain code has determined that the transaction is not ready to be assembled and it is parked for later processing.  All remaining transactions for the current originator can continue - unless they have an explicit dependency on this transaction

)

type EventType = common.EventType

const (
	Event_Created                    EventType = iota // Transaction initially received by the originator or has been loaded from the database after a restart / swap-in
	Event_ConfirmedSuccess                            // confirmation received from the blockchain of base ledge transaction successful completion
	Event_ConfirmedReverted                           // confirmation received from the blockchain of base ledge transaction failure
	Event_Delegated                                   // transaction has been delegated to a coordinator
	Event_AssembleRequestReceived                     // coordinator has requested that we assemble the transaction
	Event_AssembleAndSignSuccess                      // we have successfully assembled the transaction and signing module has signed the assembled transaction
	Event_AssembleRevert                              // we have failed to assemble the transaction
	Event_AssemblePark                                // we have parked the transaction
	Event_AssembleError                               // an unexpected error occurred while trying to assemble the transaction
	Event_Dispatched                                  // coordinator has dispatched the transaction to a public transaction manager
	Event_PreDispatchRequestReceived                  // coordinator has requested confirmation that the transaction is OK to be dispatched
	Event_Resumed                                     // Received an RPC call to resume a parked transaction
	Event_NonceAssigned                               // the public transaction manager has assigned a nonce to the transaction
	Event_Submitted                                   // the transaction has been submitted to the blockchain
	Event_CoordinatorChanged                          // the coordinator has changed
)

type StateMachine struct {
	currentState    State
	lastStateChange time.Time
	latestEvent     string
}

// Actions can be specified for transition to a state either as the OnTransitionTo function that will run for all transitions to that state or as the On field in the Transition struct if the action applies
// for a specific transition
type Action func(ctx context.Context, txn *Transaction) error

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
				Event_Created: {
					Transitions: []Transition{
						{
							To: State_Pending,
						},
					},
				},
			},
		},
		State_Pending: {
			Events: map[EventType]EventHandler{
				Event_Delegated: {
					Transitions: []Transition{
						{
							To: State_Delegated,
						},
					},
				},
			},
		},
		State_Delegated: {
			Events: map[EventType]EventHandler{
				Event_AssembleRequestReceived: {
					Validator: validator_AssembleRequestMatches,
					Transitions: []Transition{
						{
							To: State_Assembling,
						},
					},
				},
				Event_CoordinatorChanged: {},
			},
		},
		State_Assembling: {
			OnTransitionTo: action_AssembleAndSign,
			Events: map[EventType]EventHandler{
				Event_AssembleAndSignSuccess: {
					Transitions: []Transition{
						{
							To: State_Endorsement_Gathering,
							On: action_SendAssembleSuccessResponse,
						},
					},
				},
				Event_AssembleRevert: {
					Transitions: []Transition{
						{
							To: State_Reverted,
							On: action_SendAssembleRevertResponse,
						},
					},
				},
				Event_AssemblePark: {
					Transitions: []Transition{
						{
							To: State_Parked,
							On: action_SendAssembleParkResponse,
						},
					},
				},
				Event_CoordinatorChanged: {
					//would be very strange to have missed a bunch of heartbeats and switched coordinators if we recently received an assemble request but it is possible so we need to handle it
					Transitions: []Transition{
						{
							To: State_Delegated,
						},
					},
				},
				Event_AssembleRequestReceived: {
					// For some reason we've been asked to assemble again. We must not have moved to endorsement gathering,
					// reverted, or parked. This could be because of a temporary issue preventing assembly (e.g. we couldn't
					// resolve a remote verifier while it was offline). Assuming this is a new request, action it.
					Validator: validator_AssembleRequestMatches,
					Actions: []ActionRule{
						{
							If:     guard_Not(guard_AssembleRequestMatchesPreviousResponse),
							Action: action_AssembleAndSign,
						},
						{
							If:     guard_AssembleRequestMatchesPreviousResponse,
							Action: action_ResendAssembleSuccessResponse,
						},
					},
					// No transition - we're still assembling
				},
			},
		},
		State_Endorsement_Gathering: {
			Events: map[EventType]EventHandler{
				Event_AssembleRequestReceived: {
					Validator: validator_AssembleRequestMatches,
					Actions: []ActionRule{
						{
							//We thought we had got as far as endorsement but it seems like the coordinator had not got the response in time and has resent the assemble request, we simply reply with the same response as before
							If:     guard_AssembleRequestMatchesPreviousResponse,
							Action: action_ResendAssembleSuccessResponse,
						}},
					Transitions: []Transition{{
						//This is different from the previous request. The coordinator must have decided that it was necessary to re-assemble with different available states so we go back to assembling state for a do-over
						If: guard_Not(guard_AssembleRequestMatchesPreviousResponse),
						To: State_Assembling,
					}},
				},
				Event_CoordinatorChanged: {
					Transitions: []Transition{
						{
							To: State_Delegated,
						},
					},
				},
				Event_PreDispatchRequestReceived: {
					Validator: validator_PreDispatchRequestMatchesAssembledDelegation,
					Transitions: []Transition{
						{
							To: State_Prepared,
							On: action_SendPreDispatchResponse,
						},
					},
				},
			},
		},
		State_Prepared: {
			Events: map[EventType]EventHandler{
				Event_Dispatched: {
					//Note: no validator here although this event may or may not match the most recent dispatch confirmation response.
					// It is possible that we timed out  on Prepared state, delegated to another coordinator, got as far as prepared again and now just learning that
					// the original coordinator has dispatched the transaction.
					// We can't do anything to stop that, but it is interesting to apply the information from event to our state machine because we don't know which of
					// the many base ledger transactions will eventually be confirmed and we are actually not too fussy about which one does
					Transitions: []Transition{
						{
							To: State_Dispatched,
						},
					},
				},
				Event_PreDispatchRequestReceived: {
					Validator: validator_PreDispatchRequestMatchesAssembledDelegation,
					// This means that we have already sent a dispatch confirmation response and we get another one.
					// 3 possibilities, 1) the response got lost and the same coordinator is retrying -> compare the request idempotency key and or validator_PreDispatchRequestMatchesAssembledDelegation
					//                  2) There is a coordinator that we previously delegated to, and assembled for, but since assumed had become unavailable and changed to another coordinator, but the first coordinator is somehow limping along and has got as far as endorsing that previously assembled transaction. But we have already chosen our new horse for this transaction so reject.
					//                  3) There is a bug somewhere.  Don't attempt to distinguish between 2 and 3.  Just reject the request and let the coordinator deal with it.
					Actions: []ActionRule{
						{
							Action: action_ResendPreDispatchResponse,
						},
					},
				},
				Event_CoordinatorChanged: {
					Transitions: []Transition{
						{
							To: State_Delegated,
						},
					},
					// this is a particularly interesting case because the coordinator has been changed ( most likely because the previous coordinator has stopped sending heartbeats)
					// just as we are about at the point of no return.  We have already sent a dispatch confirmation response and are waiting for the dispatch heartbeat
					// only option is to go with the new coordinator.  Assuming the old coordinator didn't receive the confirmation response, or has went offline before dispatching the transactions to a public transaction manager
					// worst case scenario, it has already dispatched the transaction and the base ledger double intent protection will cause one of the transactions to fail
				},
			},
		},
		State_Dispatched: {
			//TODO this is modelled as a state that is discrete to sequenced and submitted but it may be more elegant to model those as sub states of dispatch
			// because there is a set of rules that apply to all of them given that it is possible that it all happens so quickly from dispatch -> sequenced -> submitted -> confirmed
			// that we don't have time to see the heartbeat for those intermediate states so all of those states do actually behave like substates
			// the difference between each one is whether we have the signer address, or also the nonce or also the submission hash
			// for now, we simply copy some event handler rules across dispatched , sequenced and submitted
			Events: map[EventType]EventHandler{
				Event_ConfirmedSuccess: {
					Transitions: []Transition{{
						To: State_Confirmed,
					}},
				},
				Event_ConfirmedReverted: {
					Transitions: []Transition{{
						To: State_Delegated, //trust coordinator to retry
					}},
				},
				Event_CoordinatorChanged: {
					// coordinator has changed after we have seen the transaction dispatched.
					// we will either see the dispatched transaction confirmed or reverted by the blockchain but that might not be for a long long time
					// the fact that the coordinator has been changed on us means that we have lost contact with the original coordinator.
					// The original coordinator may or may not have lost contact with the base ledger.
					// If so, it may be days or months before it reconnects and managed to submit the transaction.
					// Rather than waiting in hope, we carry on with the new coordinator.  The double intent protection in the base ledger will ensure that only one of the coordinators manages to get the transaction through
					// and the other one will revert.  We just need to make sure that we don't overreact when we see a revert.
					// We _could_ introduce a new state that we transition here to give some time, after realizing the coordinator has gone AWOL in case the transaction has made it to that coordinator's
					// EVM node which is actively trying to get it into a block and we just don't get heartbeats for that.
					// However, by waiting, we would need to delaying other transactions from being delegated and assembled or risk things happening out of order
					// and the only downside of not waiting is that we plough ahead with a new assembly of things that will never get to the base ledger because the txn at the front will cause a double intent
					// so we need to redo them all - which isn't much worse than waiting and then redoing them all. On the other hand, if we plough ahead, there is a chance that new assembly does get to the base ledger
					// and there would have been no point waiting
					Transitions: []Transition{
						{
							To: State_Delegated,
						},
					},
				},
				Event_NonceAssigned: {
					Transitions: []Transition{
						{
							To: State_Sequenced,
						},
					},
				},
				Event_Submitted: {
					//we can skip past sequenced and go straight to submitted.
					Transitions: []Transition{
						{
							To: State_Submitted,
						},
					},
				},
			},
		},
		State_Sequenced: {
			Events: map[EventType]EventHandler{
				Event_ConfirmedSuccess: {
					Transitions: []Transition{{
						To: State_Confirmed,
					}},
				},
				Event_ConfirmedReverted: {
					Transitions: []Transition{{
						To: State_Delegated, //trust coordinator to retry
					}},
				},
				Event_CoordinatorChanged: {
					Transitions: []Transition{
						{
							To: State_Delegated,
						},
					},
				},
				Event_Submitted: {
					Transitions: []Transition{
						{
							To: State_Submitted,
						},
					},
				},
			},
		},
		State_Submitted: {
			Events: map[EventType]EventHandler{
				Event_Submitted: {}, // continue to handle submitted events in this state in case the submission hash changes
				Event_ConfirmedSuccess: {
					Transitions: []Transition{{
						To: State_Confirmed,
					}},
				},
				Event_ConfirmedReverted: {
					Transitions: []Transition{{
						To: State_Delegated, //trust coordinator to retry
					}},
				},
				Event_CoordinatorChanged: {
					Transitions: []Transition{
						{
							To: State_Delegated,
						},
					},
				},
				// After submission there's a race for us or the coordinator to find out that the base ledger transaction
				// reverted. We need to accomodate the coordinator getting there first and sending a new assemble request
				// before we receive the revert and moved back to delegated.
				Event_AssembleRequestReceived: {
					Validator: validator_AssembleRequestMatches,
					Transitions: []Transition{
						{
							To: State_Assembling,
						},
					},
				},
			},
		},

		State_Parked: {
			Events: map[EventType]EventHandler{
				Event_AssembleRequestReceived: {
					Actions: []ActionRule{
						{
							//it seems like the coordinator had not got the response in time and has resent the assemble request, we simply reply with the same response as before
							If:     guard_AssembleRequestMatchesPreviousResponse,
							Action: action_ResendAssembleParkResponse,
						}},
				},
				Event_Resumed: {
					Transitions: []Transition{{
						To: State_Pending,
					}},
				},
			},
		},
		State_Reverted: {
			Events: map[EventType]EventHandler{
				Event_AssembleRequestReceived: {
					Actions: []ActionRule{
						{
							//it seems like the coordinator had not got the response in time and has resent the assemble request, we simply reply with the same response as before
							If:     guard_AssembleRequestMatchesPreviousResponse,
							Action: action_ResendAssembleRevertResponse,
						}},
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
				log.L(ctx).Errorf("error validating event %s: %v", event.TypeString(), err)
				return nil, err
			}
			if !valid {
				// This is perfectly normal sometimes an event happens and is no longer relevant to the transaction so we just ignore it and move on.
				// We log a warning in case it's not a late-delivered message but something that needs looking in to
				log.L(ctx).Warnf("originator transaction event %s is not valid for current state %s", event.TypeString(), sm.currentState.String())
				return nil, nil
			}
		}
		return &eventHandler, nil
	}
	return nil, nil
}

// Function applyEvent updates the internal state of the Transaction with information from the event
// this happens before the state machine is evaluated for transitions that may be triggered by the event
// so that any guards on the transition rules can take into account the new internal state of the Transaction after this event has been applied
func (t *Transaction) applyEvent(ctx context.Context, event common.Event) error {
	var err error
	switch event := event.(type) {
	case Event:
		err = event.ApplyToTransaction(ctx, t)

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
				log.L(ctx).Errorf("error applying action: %v", err)
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
			// (Odd spacing is intentional to align logs more clearly)
			log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_STATE)).Debugf("orig-tx  | %s   | %s | %T | %s -> %s", t.Address.String()[0:8], t.ID.String()[0:8], event, sm.currentState.String(), rule.To.String())
			t.metrics.ObserveSequencerTXStateChange("Originator_"+rule.To.String(), time.Duration(event.GetEventTime().Sub(sm.lastStateChange).Milliseconds()))
			sm.latestEvent = event.TypeString()
			sm.lastStateChange = time.Now()
			sm.currentState = rule.To
			newStateDefinition := stateDefinitionsMap[sm.currentState]
			//run any actions specific to the transition first
			if rule.On != nil {
				err := rule.On(ctx, t)
				if err != nil {
					//any recoverable errors should have been handled by the action function
					log.L(ctx).Errorf("error transitioning originator transaction to state %v: %v", sm.currentState, err)
					return err
				}
			}

			// then run any actions for the state entry
			if newStateDefinition.OnTransitionTo != nil {
				err := newStateDefinition.OnTransitionTo(ctx, t)
				if err != nil {
					// any recoverable errors should have been handled by the OnTransitionTo function
					log.L(ctx).Errorf("error transitioning originator transaction to state %v: %v", sm.currentState, err)
					return err
				}
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

func (s State) String() string {
	switch s {
	case State_Initial:
		return "State_Initial"
	case State_Pending:
		return "State_Pending"
	case State_Delegated:
		return "State_Delegated"
	case State_Assembling:
		return "State_Assembling"
	case State_Endorsement_Gathering:
		return "State_Endorsement_Gathering"
	case State_Signing:
		return "State_Signing"
	case State_Prepared:
		return "State_Prepared"
	case State_Dispatched:
		return "State_Dispatched"
	case State_Sequenced:
		return "State_Sequenced"
	case State_Submitted:
		return "State_Submitted"
	case State_Confirmed:
		return "State_Confirmed"
	case State_Reverted:
		return "State_Reverted"
	case State_Parked:
		return "State_Parked"
	}
	return "Unknown"
}
