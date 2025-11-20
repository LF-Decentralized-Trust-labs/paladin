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
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
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

func (*ReceivedEvent) Type() EventType {
	return Event_Received
}

func (*ReceivedEvent) TypeString() string {
	return "Event_Received"
}

// TransactionSelectedEvent
type SelectedEvent struct {
	BaseCoordinatorEvent
}

func (*SelectedEvent) Type() EventType {
	return Event_Selected
}

func (*SelectedEvent) TypeString() string {
	return "Event_Selected"
}

// AssembleRequestSentEvent
type AssembleRequestSentEvent struct {
	BaseCoordinatorEvent
}

func (*AssembleRequestSentEvent) Type() EventType {
	return Event_AssembleRequestSent
}

func (*AssembleRequestSentEvent) TypeString() string {
	return "Event_AssembleRequestSent"
}

// AssembleSuccessEvent
type AssembleSuccessEvent struct {
	BaseCoordinatorEvent
	PostAssembly *components.TransactionPostAssembly
	PreAssembly  *components.TransactionPreAssembly
	RequestID    uuid.UUID
}

func (*AssembleSuccessEvent) Type() EventType {
	return Event_Assemble_Success
}

func (*AssembleSuccessEvent) TypeString() string {
	return "Event_Assemble_Success"
}

// AssembleRevertResponseEvent
type AssembleRevertResponseEvent struct {
	BaseCoordinatorEvent
	PostAssembly *components.TransactionPostAssembly
	RequestID    uuid.UUID
}

func (*AssembleRevertResponseEvent) Type() EventType {
	return Event_Assemble_Revert_Response
}

func (*AssembleRevertResponseEvent) TypeString() string {
	return "Event_Assemble_Revert_Response"
}

// EndorsedEvent
type EndorsedEvent struct {
	BaseCoordinatorEvent
	Endorsement *prototk.AttestationResult
	RequestID   uuid.UUID
}

func (*EndorsedEvent) Type() EventType {
	return Event_Endorsed
}

func (*EndorsedEvent) TypeString() string {
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

func (*EndorsedRejectedEvent) Type() EventType {
	return Event_EndorsedRejected
}

func (*EndorsedRejectedEvent) TypeString() string {
	return "Event_EndorsedRejected"
}

// DispatchRequestApprovedEvent
type DispatchRequestApprovedEvent struct {
	BaseCoordinatorEvent
	RequestID uuid.UUID
}

func (*DispatchRequestApprovedEvent) Type() EventType {
	return Event_DispatchRequestApproved
}

func (*DispatchRequestApprovedEvent) TypeString() string {
	return "Event_DispatchRequestApproved"
}

// DispatchRequestRejectedEvent
type DispatchRequestRejectedEvent struct {
	BaseCoordinatorEvent
}

func (*DispatchRequestRejectedEvent) Type() EventType {
	return Event_DispatchRequestRejected
}

func (*DispatchRequestRejectedEvent) TypeString() string {
	return "Event_DispatchRequestRejected"
}

// CollectedEvent
// Collected by the public transaction manager after being dispatched
type CollectedEvent struct {
	BaseCoordinatorEvent
	SignerAddress pldtypes.EthAddress
}

func (*CollectedEvent) Type() EventType {
	return Event_Collected
}

func (*CollectedEvent) TypeString() string {
	return "Event_Collected"
}

// DispatchedEvent
// Collected by the dispatcher thread and dispatched to the public transaction manager
type DispatchedEvent struct {
	BaseCoordinatorEvent
}

func (*DispatchedEvent) Type() EventType {
	return Event_Dispatched
}

func (*DispatchedEvent) TypeString() string {
	return "Event_Dispatched"
}

// NonceAllocatedEvent
type NonceAllocatedEvent struct {
	BaseCoordinatorEvent
	Nonce uint64
}

func (*NonceAllocatedEvent) Type() EventType {
	return Event_NonceAllocated
}

func (*NonceAllocatedEvent) TypeString() string {
	return "Event_NonceAllocated"
}

// SubmittedEvent
type SubmittedEvent struct {
	BaseCoordinatorEvent
	SubmissionHash pldtypes.Bytes32
}

func (*SubmittedEvent) Type() EventType {
	return Event_Submitted
}

func (*SubmittedEvent) TypeString() string {
	return "Event_Submitted"
}

// ConfirmedEvent
type ConfirmedEvent struct {
	BaseCoordinatorEvent
	Nonce        uint64
	Hash         pldtypes.Bytes32
	RevertReason pldtypes.HexBytes
}

func (*ConfirmedEvent) Type() EventType {
	return Event_Confirmed
}

func (*ConfirmedEvent) TypeString() string {
	return "Event_Confirmed"
}

type DependencyAssembledEvent struct {
	BaseCoordinatorEvent
	DependencyID uuid.UUID
}

func (*DependencyAssembledEvent) Type() EventType {
	return Event_DependencyAssembled
}

func (*DependencyAssembledEvent) TypeString() string {
	return "Event_DependencyAssembled"
}

type DependencyRevertedEvent struct {
	BaseCoordinatorEvent
	DependencyID uuid.UUID
}

func (*DependencyRevertedEvent) Type() EventType {
	return Event_DependencyReverted
}

func (*DependencyRevertedEvent) TypeString() string {
	return "Event_DependencyReverted"
}

type DependencyReadyEvent struct {
	BaseCoordinatorEvent
	DependencyID uuid.UUID
}

func (*DependencyReadyEvent) Type() EventType {
	return Event_DependencyReady
}

func (*DependencyReadyEvent) TypeString() string {
	return "Event_DependencyReady"
}

type RequestTimeoutIntervalEvent struct {
	BaseCoordinatorEvent
}

func (*RequestTimeoutIntervalEvent) Type() EventType {
	return Event_RequestTimeoutInterval
}

func (*RequestTimeoutIntervalEvent) TypeString() string {
	return "Event_RequestTimeoutInterval"
}

// events emitted by the transaction state machine whenever a state transition occurs
type StateTransitionEvent struct {
	BaseCoordinatorEvent
	FromState State
	ToState   State
}

func (*StateTransitionEvent) Type() EventType {
	return Event_StateTransition
}

func (*StateTransitionEvent) TypeString() string {
	return "Event_StateTransition"
}

// events emitted by the transaction state machine whenever a state transition occurs
type HeartbeatIntervalEvent struct {
	BaseCoordinatorEvent
}

func (*HeartbeatIntervalEvent) Type() EventType {
	return Event_HeartbeatInterval
}

func (*HeartbeatIntervalEvent) TypeString() string {
	return "Event_HeartbeatInterval"
}
