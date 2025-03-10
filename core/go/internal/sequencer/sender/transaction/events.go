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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type Event interface {
	common.Event
	GetTransactionID() uuid.UUID

	// Function ApplyToTransaction updates the internal state of the Transaction with information from the event
	// this happens before the state machine is evaluated for transitions that may be triggered by the event
	// so that any guards on the transition rules can take into account the new internal state of the Transaction after this event has been applied
	ApplyToTransaction(ctx context.Context, txn *Transaction) error
}

type event struct {
	TransactionID uuid.UUID
}

// Default implementation is a no-op
func (e *event) ApplyToTransaction(ctx context.Context, _ *Transaction) error {
	log.L(ctx).Debugf("nothing to apply for event type %T", e)
	return nil
}

func (e *event) GetTransactionID() uuid.UUID {
	return e.TransactionID
}

type ConfirmedSuccessEvent struct {
	event
}

func (_ *ConfirmedSuccessEvent) Type() EventType {
	return Event_ConfirmedSuccess
}

type ConfirmedRevertedEvent struct {
	event
}

func (_ *ConfirmedRevertedEvent) Type() EventType {
	return Event_ConfirmedReverted
}

type CreatedEvent struct {
	event
	PrivateTransaction *components.PrivateTransaction
}

func (_ *CreatedEvent) Type() EventType {
	return Event_Created
}

type DelegatedEvent struct {
	event
	Coordinator string
}

func (_ *DelegatedEvent) Type() EventType {
	return Event_Delegated
}

func (event *DelegatedEvent) ApplyToTransaction(_ context.Context, txn *Transaction) error {
	txn.currentDelegate = event.Coordinator
	return nil
}

type AssembleRequestReceivedEvent struct {
	event
	RequestID               uuid.UUID
	Coordinator             string
	CoordinatorsBlockHeight int64
	StateLocksJSON          []byte
}

func (_ *AssembleRequestReceivedEvent) Type() EventType {
	return Event_AssembleRequestReceived
}

func (event *AssembleRequestReceivedEvent) ApplyToTransaction(_ context.Context, txn *Transaction) error {
	txn.currentDelegate = event.Coordinator

	txn.latestAssembleRequest = &assembleRequestFromCoordinator{
		coordinatorsBlockHeight: event.CoordinatorsBlockHeight,
		stateLocksJSON:          event.StateLocksJSON,
		requestID:               event.RequestID,
	}

	return nil
}

type AssembleAndSignSuccessEvent struct {
	event
	PostAssembly *components.TransactionPostAssembly
	RequestID    uuid.UUID
}

func (_ *AssembleAndSignSuccessEvent) Type() EventType {
	return Event_AssembleAndSignSuccess
}

func (event *AssembleAndSignSuccessEvent) ApplyToTransaction(_ context.Context, txn *Transaction) error {
	txn.PostAssembly = event.PostAssembly
	txn.latestFulfilledAssembleRequestID = event.RequestID
	return nil
}

type AssembleRevertEvent struct {
	event
	PostAssembly *components.TransactionPostAssembly
	RequestID    uuid.UUID
}

func (_ *AssembleRevertEvent) Type() EventType {
	return Event_AssembleRevert
}

func (event *AssembleRevertEvent) ApplyToTransaction(_ context.Context, txn *Transaction) error {
	txn.PostAssembly = event.PostAssembly
	txn.latestFulfilledAssembleRequestID = event.RequestID
	return nil
}

type AssembleParkEvent struct {
	event
	PostAssembly *components.TransactionPostAssembly
	RequestID    uuid.UUID
}

func (_ *AssembleParkEvent) Type() EventType {
	return Event_AssemblePark
}

func (event *AssembleParkEvent) ApplyToTransaction(_ context.Context, txn *Transaction) error {
	txn.PostAssembly = event.PostAssembly
	txn.latestFulfilledAssembleRequestID = event.RequestID
	return nil
}

type AssembleErrorEvent struct {
	event
}

func (_ *AssembleErrorEvent) Type() EventType {
	return Event_AssembleError
}

type DispatchConfirmationRequestReceivedEvent struct {
	event
	RequestID        uuid.UUID
	Coordinator      string
	PostAssemblyHash *tktypes.Bytes32
}

func (_ *DispatchConfirmationRequestReceivedEvent) Type() EventType {
	return Event_DispatchConfirmationRequestReceived
}

type CoordinatorChangedEvent struct {
	event
	Coordinator string
}

func (_ *CoordinatorChangedEvent) Type() EventType {
	return Event_CoordinatorChanged
}

func (event *CoordinatorChangedEvent) ApplyToTransaction(_ context.Context, txn *Transaction) error {
	txn.currentDelegate = event.Coordinator
	return nil
}

type DispatchHeartbeatReceivedEvent struct {
	event
}

func (_ *DispatchHeartbeatReceivedEvent) Type() EventType {
	return Event_DispatchHeartbeatReceived
}

type ResumedEvent struct {
	event
}

func (_ *ResumedEvent) Type() EventType {
	return Event_Resumed
}
