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

package originator

import (
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

type Event interface {
	common.Event
}

type HeartbeatIntervalEvent struct {
	common.BaseEvent
}

func (*HeartbeatIntervalEvent) Type() EventType {
	return Event_HeartbeatInterval
}

func (*HeartbeatIntervalEvent) TypeString() string {
	return "Event_HeartbeatInterval"
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

type TransactionCreatedEvent struct {
	common.BaseEvent
	Transaction *components.PrivateTransaction
}

func (*TransactionCreatedEvent) Type() EventType {
	return Event_TransactionCreated
}

func (*TransactionCreatedEvent) TypeString() string {
	return "Event_TransactionCreated"
}

type TransactionConfirmedEvent struct {
	common.BaseEvent
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
