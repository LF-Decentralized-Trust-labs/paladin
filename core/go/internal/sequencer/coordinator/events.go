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
	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/kaleido-io/paladin/core/internal/sequencer/transport"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type Event interface {
	common.Event
}

type CoordinatorStateEventActivated struct{}

func (_ *CoordinatorStateEventActivated) Type() EventType {
	return Event_Activated
}

type TransactionsDelegatedEvent struct {
	Sender             string // Fully qualified identity locator for the sender
	Transactions       []*components.PrivateTransaction
	SendersBlockHeight uint64
}

func (_ *TransactionsDelegatedEvent) Type() EventType {
	return Event_TransactionsDelegated
}

func (_ *TransactionsDelegatedEvent) TypeString() string {
	return "Event_TransactionsDelegated"
}

type CoordinatorFlushedEvent struct{}

func (_ *CoordinatorFlushedEvent) Type() EventType {
	return Event_Flushed
}

func (_ *CoordinatorFlushedEvent) TypeString() string {
	return "Event_Flushed"
}

type TransactionConfirmedEvent struct {
	From         *tktypes.EthAddress
	Nonce        uint64
	Hash         tktypes.Bytes32
	RevertReason tktypes.HexBytes
}

func (_ *TransactionConfirmedEvent) Type() EventType {
	return Event_TransactionConfirmed
}

func (_ *TransactionConfirmedEvent) TypeString() string {
	return "Event_TransactionConfirmed"
}

type TransactionDispatchConfirmedEvent struct {
	TransactionID uuid.UUID
}

func (_ *TransactionDispatchConfirmedEvent) Type() EventType {
	return Event_TransactionDispatchConfirmed
}

func (_ *TransactionDispatchConfirmedEvent) TypeString() string {
	return "Event_TransactionDispatchConfirmed"
}
func (t *TransactionDispatchConfirmedEvent) GetTransactionID() uuid.UUID {
	return t.TransactionID
}

type HeartbeatReceivedEvent struct {
	transport.CoordinatorHeartbeatNotification
}

func (_ *HeartbeatReceivedEvent) Type() EventType {
	return Event_HeartbeatReceived
}

func (_ *HeartbeatReceivedEvent) TypeString() string {
	return "Event_HeartbeatReceived"
}

type HandoverRequestEvent struct {
	Requester string
}

func (_ *HandoverRequestEvent) Type() EventType {
	return Event_HandoverRequestReceived
}

func (_ *HandoverRequestEvent) TypeString() string {
	return "Event_HandoverRequestReceived"
}

type NewBlockEvent struct {
	BlockHeight uint64
}

func (_ *NewBlockEvent) Type() EventType {
	return Event_NewBlock
}

func (_ *NewBlockEvent) TypeString() string {
	return "Event_NewBlock"
}

type HandoverReceivedEvent struct {
}

func (_ *HandoverReceivedEvent) Type() EventType {
	return Event_HandoverReceived
}

func (_ *HandoverReceivedEvent) TypeString() string {
	return "Event_HandoverReceived"
}

type TransactionStateTransitionEvent struct {
	TransactionID uuid.UUID
	From          transaction.State
	To            transaction.State
}

func (_ *TransactionStateTransitionEvent) Type() EventType {
	return Event_TransactionStateTransition
}

func (_ *TransactionStateTransitionEvent) TypeString() string {
	return "Event_TransactionStateTransition"
}
