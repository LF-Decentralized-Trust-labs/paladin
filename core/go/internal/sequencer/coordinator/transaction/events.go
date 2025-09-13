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
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

type Event interface {
	common.Event
	GetTransactionID() uuid.UUID
}

type BaseCoordinatorEvent struct {
	common.BaseEvent
	TransactionID uuid.UUID
}

func (e *BaseCoordinatorEvent) GetTransactionID() uuid.UUID {
	return e.TransactionID
}

// TransactionReceivedEvent is "emitted" when the coordinator receives a transaction.
// Feels slightly artificial to model this as an event because it happens every time we create a transaction object
// but rather than bury the logic in NewTransaction func, modeling this event allows us to define the initial state transition rules in the same declarative stateDefinitions structure as all other state transitions
type ReceivedEvent struct {
	BaseCoordinatorEvent
}

func (_ *ReceivedEvent) Type() EventType {
	return Event_Received
}

func (_ *ReceivedEvent) TypeString() string {
	return "Event_Received"
}

// TransactionSelectedEvent
type SelectedEvent struct {
	BaseCoordinatorEvent
}

func (_ *SelectedEvent) Type() EventType {
	return Event_Selected
}

func (_ *SelectedEvent) TypeString() string {
	return "Event_Selected"
}

// AssembleRequestSentEvent
type AssembleRequestSentEvent struct {
	BaseCoordinatorEvent
}

func (_ *AssembleRequestSentEvent) Type() EventType {
	return Event_AssembleRequestSent
}

func (_ *AssembleRequestSentEvent) TypeString() string {
	return "Event_AssembleRequestSent"
}

// AssembleSuccessEvent
type AssembleSuccessEvent struct {
	BaseCoordinatorEvent
	PostAssembly *components.TransactionPostAssembly
	PreAssembly  *components.TransactionPreAssembly
	RequestID    uuid.UUID
}

func (_ *AssembleSuccessEvent) Type() EventType {
	return Event_Assemble_Success
}

func (_ *AssembleSuccessEvent) TypeString() string {
	return "Event_Assemble_Success"
}

// AssembleRevertResponseEvent
type AssembleRevertResponseEvent struct {
	BaseCoordinatorEvent
	PostAssembly *components.TransactionPostAssembly
	RequestID    uuid.UUID
}

func (_ *AssembleRevertResponseEvent) Type() EventType {
	return Event_Assemble_Revert_Response
}

func (_ *AssembleRevertResponseEvent) TypeString() string {
	return "Event_Assemble_Revert_Response"
}

// EndorsedEvent
type EndorsedEvent struct {
	BaseCoordinatorEvent
	Endorsement *prototk.AttestationResult
	RequestID   uuid.UUID
}

func (_ *EndorsedEvent) Type() EventType {
	return Event_Endorsed
}

func (_ *EndorsedEvent) TypeString() string {
	return "Event_Endorsed"
}

// EndorsedRejectedEvent
type EndorsedRejectedEvent struct {
	BaseCoordinatorEvent
	RevertReason           string
	Party                  string
	AttestationRequestName string
	RequestID              uuid.UUID
}

func (_ *EndorsedRejectedEvent) Type() EventType {
	return Event_EndorsedRejected
}

func (_ *EndorsedRejectedEvent) TypeString() string {
	return "Event_EndorsedRejected"
}

// DispatchConfirmedEvent
type DispatchConfirmedEvent struct {
	BaseCoordinatorEvent
	RequestID uuid.UUID
}

func (_ *DispatchConfirmedEvent) Type() EventType {
	return Event_DispatchConfirmed
}

func (_ *DispatchConfirmedEvent) TypeString() string {
	return "Event_DispatchConfirmed"
}

// DispatchConfirmationRejectedEvent
type DispatchConfirmationRejectedEvent struct {
	BaseCoordinatorEvent
}

func (_ *DispatchConfirmationRejectedEvent) Type() EventType {
	return Event_DispatchConfirmationRejected
}

func (_ *DispatchConfirmationRejectedEvent) TypeString() string {
	return "Event_DispatchConfirmationRejected"
}

// CollectedEvent
// Collected by the dispatcher thread and dispatched to a public transaction manager for a given signer address
type CollectedEvent struct {
	BaseCoordinatorEvent
	SignerAddress pldtypes.EthAddress
}

func (_ *CollectedEvent) Type() EventType {
	return Event_Collected
}

func (_ *CollectedEvent) TypeString() string {
	return "Event_Collected"
}

// NonceAllocatedEvent
type NonceAllocatedEvent struct {
	BaseCoordinatorEvent
	Nonce uint64
}

func (_ *NonceAllocatedEvent) Type() EventType {
	return Event_NonceAllocated
}

func (_ *NonceAllocatedEvent) TypeString() string {
	return "Event_NonceAllocated"
}

// SubmittedEvent
type SubmittedEvent struct {
	BaseCoordinatorEvent
	SubmissionHash pldtypes.Bytes32
}

func (_ *SubmittedEvent) Type() EventType {
	return Event_Submitted
}

func (_ *SubmittedEvent) TypeString() string {
	return "Event_Submitted"
}

// ConfirmedEvent
type ConfirmedEvent struct {
	BaseCoordinatorEvent
	Nonce        uint64
	Hash         pldtypes.Bytes32
	RevertReason pldtypes.HexBytes
}

func (_ *ConfirmedEvent) Type() EventType {
	return Event_Confirmed
}

func (_ *ConfirmedEvent) TypeString() string {
	return "Event_Confirmed"
}

type DependencyAssembledEvent struct {
	BaseCoordinatorEvent
	DependencyID uuid.UUID
}

func (_ *DependencyAssembledEvent) Type() EventType {
	return Event_DependencyAssembled
}

func (_ *DependencyAssembledEvent) TypeString() string {
	return "Event_DependencyAssembled"
}

type DependencyRevertedEvent struct {
	BaseCoordinatorEvent
	DependencyID uuid.UUID
}

func (_ *DependencyRevertedEvent) Type() EventType {
	return Event_DependencyReverted
}

func (_ *DependencyRevertedEvent) TypeString() string {
	return "Event_DependencyReverted"
}

type DependencyReadyEvent struct {
	BaseCoordinatorEvent
	DependencyID uuid.UUID
}

func (_ *DependencyReadyEvent) Type() EventType {
	return Event_DependencyReady
}

func (_ *DependencyReadyEvent) TypeString() string {
	return "Event_DependencyReady"
}

type RequestTimeoutIntervalEvent struct {
	BaseCoordinatorEvent
}

func (_ *RequestTimeoutIntervalEvent) Type() EventType {
	return Event_RequestTimeoutInterval
}

func (_ *RequestTimeoutIntervalEvent) TypeString() string {
	return "Event_RequestTimeoutInterval"
}

// events emitted by the transaction state machine whenever a state transition occurs
type StateTransitionEvent struct {
	BaseCoordinatorEvent
	FromState State
	ToState   State
}

func (_ *StateTransitionEvent) Type() EventType {
	return Event_StateTransition
}

func (_ *StateTransitionEvent) TypeString() string {
	return "Event_StateTransition"
}

// events emitted by the transaction state machine whenever a state transition occurs
type HeartbeatIntervalEvent struct {
	BaseCoordinatorEvent
}

func (_ *HeartbeatIntervalEvent) Type() EventType {
	return Event_HeartbeatInterval
}

func (_ *HeartbeatIntervalEvent) TypeString() string {
	return "Event_HeartbeatInterval"
}
