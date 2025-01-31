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
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
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

// AssembledEvent
type AssembledEvent struct {
	event
}

func (_ *AssembledEvent) Type() EventType {
	return Event_Assembled
}

// EndorsedEvent
type EndorsedEvent struct {
	event
}

func (_ *EndorsedEvent) Type() EventType {
	return Event_Endorsed
}

// DispatchConfirmedEvent
type DispatchConfirmedEvent struct {
	event
}

func (_ *DispatchConfirmedEvent) Type() EventType {
	return Event_DispatchConfirmed
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
