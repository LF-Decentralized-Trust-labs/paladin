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
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

type Event interface {
	common.Event
}

type CoordinatorStateEventActivated struct{}

func (*CoordinatorStateEventActivated) Type() EventType {
	return Event_Activated
}

type TransactionsDelegatedEvent struct {
	common.BaseEvent
	Originator             string // Fully qualified identity locator for the originator
	Transactions           []*components.PrivateTransaction
	OriginatorsBlockHeight uint64
}

func (*TransactionsDelegatedEvent) Type() EventType {
	return Event_TransactionsDelegated
}

func (*TransactionsDelegatedEvent) TypeString() string {
	return "Event_TransactionsDelegated"
}

type CoordinatorFlushedEvent struct{}

func (*CoordinatorFlushedEvent) Type() EventType {
	return Event_Flushed
}

func (*CoordinatorFlushedEvent) TypeString() string {
	return "Event_Flushed"
}

type TransactionConfirmedEvent struct {
	common.BaseEvent
	TxID         uuid.UUID
	From         *pldtypes.EthAddress
	Nonce        uint64
	Hash         pldtypes.Bytes32
	RevertReason pldtypes.HexBytes
}

func (*TransactionConfirmedEvent) Type() EventType {
	return Event_TransactionConfirmed
}

func (*TransactionConfirmedEvent) TypeString() string {
	return "Event_TransactionConfirmed"
}

type TransactionDispatchConfirmedEvent struct {
	common.BaseEvent
	TransactionID uuid.UUID
}

func (*TransactionDispatchConfirmedEvent) Type() EventType {
	return Event_TransactionDispatchConfirmed
}

func (*TransactionDispatchConfirmedEvent) TypeString() string {
	return "Event_TransactionDispatchConfirmed"
}
func (t *TransactionDispatchConfirmedEvent) GetTransactionID() uuid.UUID {
	return t.TransactionID
}

type EndorsementRequestedEvent struct {
	common.BaseEvent
	From string
}

func (*EndorsementRequestedEvent) Type() EventType {
	return Event_EndorsementRequested
}

func (*EndorsementRequestedEvent) TypeString() string {
	return "Event_EndorsementRequested"
}

type HeartbeatReceivedEvent struct {
	common.BaseEvent
	transport.CoordinatorHeartbeatNotification
}

func (*HeartbeatReceivedEvent) Type() EventType {
	return Event_HeartbeatReceived
}

func (*HeartbeatReceivedEvent) TypeString() string {
	return "Event_HeartbeatReceived"
}

type HandoverRequestEvent struct {
	common.BaseEvent
	Requester string
}

func (*HandoverRequestEvent) Type() EventType {
	return Event_HandoverRequestReceived
}

func (*HandoverRequestEvent) TypeString() string {
	return "Event_HandoverRequestReceived"
}

type NewBlockEvent struct {
	common.BaseEvent
	BlockHeight uint64
}

func (*NewBlockEvent) Type() EventType {
	return Event_NewBlock
}

func (*NewBlockEvent) TypeString() string {
	return "Event_NewBlock"
}

type HandoverReceivedEvent struct {
	common.BaseEvent
}

func (*HandoverReceivedEvent) Type() EventType {
	return Event_HandoverReceived
}

func (*HandoverReceivedEvent) TypeString() string {
	return "Event_HandoverReceived"
}

type TransactionStateTransitionEvent struct {
	TransactionID uuid.UUID
	From          transaction.State
	To            transaction.State
}

func (*TransactionStateTransitionEvent) Type() EventType {
	return Event_TransactionStateTransition
}

func (*TransactionStateTransitionEvent) TypeString() string {
	return "Event_TransactionStateTransition"
}
