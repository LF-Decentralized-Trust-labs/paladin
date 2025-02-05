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
	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type Event interface {
	common.Event
	GetTransactionID() uuid.UUID
}

type event struct {
	TransactionID uuid.UUID
}

func (e *event) GetTransactionID() uuid.UUID {
	return e.TransactionID
}

// TransactionSelectedEvent
type SelectedEvent struct {
	event
}

func (_ *SelectedEvent) Type() EventType {
	return Event_Selected
}

// AssembleRequestSentEvent
type AssembleRequestSentEvent struct {
	event
}

func (_ *AssembleRequestSentEvent) Type() EventType {
	return Event_AssembleRequestSent
}

// AssembleSuccessEvent
type AssembleSuccessEvent struct {
	event
	postAssembly *components.TransactionPostAssembly
}

func (_ *AssembleSuccessEvent) Type() EventType {
	return Event_Assemble_Success
}

// AssembleRevertEvent
type AssembleRevertEvent struct {
	event
	postAssembly *components.TransactionPostAssembly
}

func (_ *AssembleRevertEvent) Type() EventType {
	return Event_Assemble_Revert
}

// EndorsedEvent
type EndorsedEvent struct {
	event
	Endorsement *prototk.AttestationResult
	RequestID   uuid.UUID
}

func (_ *EndorsedEvent) Type() EventType {
	return Event_Endorsed
}

// EndorsedRejectedEvent
type EndorsedRejectedEvent struct {
	event
	RevertReason           string
	Party                  string
	AttestationRequestName string
	RequestID              uuid.UUID
}

func (_ *EndorsedRejectedEvent) Type() EventType {
	return Event_EndorsedRejected
}

// DispatchConfirmedEvent
type DispatchConfirmedEvent struct {
	event
	RequestID uuid.UUID
}

func (_ *DispatchConfirmedEvent) Type() EventType {
	return Event_DispatchConfirmed
}

// DispatchConfirmationRejectedEvent
type DispatchConfirmationRejectedEvent struct {
	event
}

func (_ *DispatchConfirmationRejectedEvent) Type() EventType {
	return Event_DispatchConfirmationRejected
}

// CollectedEvent
type CollectedEvent struct {
	event
}

func (_ *CollectedEvent) Type() EventType {
	return Event_Collected
}

// NonceAllocatedEvent
type NonceAllocatedEvent struct {
	event
}

func (_ *NonceAllocatedEvent) Type() EventType {
	return Event_NonceAllocated
}

// SubmittedEvent
type SubmittedEvent struct {
	event
}

func (_ *SubmittedEvent) Type() EventType {
	return Event_Submitted
}

// ConfirmedEvent
type ConfirmedEvent struct {
	event
	Nonce        uint64
	Hash         tktypes.Bytes32
	RevertReason tktypes.HexBytes
}

func (_ *ConfirmedEvent) Type() EventType {
	return Event_Confirmed
}

type DependencyReadyEvent struct {
	event
	DependencyID uuid.UUID
}

func (_ *DependencyReadyEvent) Type() EventType {
	return Event_DependencyReady
}
