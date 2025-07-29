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

package sender

import (
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/transport"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type Event interface {
	common.Event
}

type HeartbeatIntervalEvent struct {
}

func (_ *HeartbeatIntervalEvent) Type() EventType {
	return Event_HeartbeatInterval
}

func (_ *HeartbeatIntervalEvent) TypeString() string {
	return "Event_HeartbeatInterval"
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

type TransactionCreatedEvent struct {
	Transaction *components.PrivateTransaction
}

func (_ *TransactionCreatedEvent) Type() EventType {
	return Event_TransactionCreated
}

func (_ *TransactionCreatedEvent) TypeString() string {
	return "Event_TransactionCreated"
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
