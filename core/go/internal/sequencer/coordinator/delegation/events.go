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

package delegation

import (
	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type Event interface {
	common.Event
	GetTransactionID() uuid.UUID
}

type delegationEvent struct {
	TransactionID uuid.UUID
}

func (e *delegationEvent) GetTransactionID() uuid.UUID {
	return e.TransactionID
}

// DelegationSelectedEvent
type SelectedEvent struct {
	delegationEvent
}

func (_ *SelectedEvent) Type() EventType {
	return Event_Selected
}

// AssembleRequestSentEvent
type AssembleRequestSentEvent struct {
	delegationEvent
}

func (_ *AssembleRequestSentEvent) Type() EventType {
	return Event_AssembleRequestSent
}

// AssembledEvent
type AssembledEvent struct {
	delegationEvent
}

func (_ *AssembledEvent) Type() EventType {
	return Event_Assembled
}

// EndorsedEvent
type EndorsedEvent struct {
	delegationEvent
}

func (_ *EndorsedEvent) Type() EventType {
	return Event_Endorsed
}

// DispatchConfirmedEvent
type DispatchConfirmedEvent struct {
	delegationEvent
}

func (_ *DispatchConfirmedEvent) Type() EventType {
	return Event_DispatchConfirmed
}

// CollectedEvent
type CollectedEvent struct {
	delegationEvent
}

func (_ *CollectedEvent) Type() EventType {
	return Event_Collected
}

// NonceAllocatedEvent
type NonceAllocatedEvent struct {
	delegationEvent
}

func (_ *NonceAllocatedEvent) Type() EventType {
	return Event_NonceAllocated
}

// SubmittedEvent
type SubmittedEvent struct {
	delegationEvent
}

func (_ *SubmittedEvent) Type() EventType {
	return Event_Submitted
}

// ConfirmedEvent
type ConfirmedEvent struct {
	delegationEvent
	Nonce        uint64
	Hash         tktypes.Bytes32
	RevertReason tktypes.HexBytes
}

func (_ *ConfirmedEvent) Type() EventType {
	return Event_Confirmed
}
