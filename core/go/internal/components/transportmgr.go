/*
 * Copyright © 2024 Kaleido, Inc.
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

package components

import (
	"context"

	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
)

type FireAndForgetMessageSend struct {
	Node          string
	Component     prototk.PaladinMsg_Component
	MessageID     *uuid.UUID // optionally supplied by caller for request/reply correlation
	CorrelationID *uuid.UUID
	MessageType   string
	Payload       []byte
}

type ReceivedMessage struct {
	FromNode      string
	MessageID     uuid.UUID
	CorrelationID *uuid.UUID
	MessageType   string
	Payload       []byte
}

type TransportManagerToTransport interface {
	plugintk.TransportAPI
	Initialized()
}

// TransportClient is the interface for a component that can receive messages from the transport manager
type TransportClient interface {
	// This function is used by the transport manager to deliver messages to the engine.
	//
	// The implementation of this function:
	// - MUST thread safe
	// - SHOULD NOT perform any processing within the function call itself beyond routing
	//
	// There is no ack to the messages. They are at-most-once delivery. So there is no error return.
	// Use it or lose it.
	//
	// The design assumption of the transport manager is that the engine is entirely responsible
	// for determining what thread-of-control to dispatch any given message to.
	// This is because the determination of that is not dependent on who it came from,
	// but rather what its purpose is.
	//
	// Most likely processing pattern is:
	// - Pick a suitable go channel for a thread-of-control that could process the message (existing or new)
	// - Push the message into that go channel
	// - Handle the situation where the go channel is full (mark a miss for that routine to go back and handle when it gets free)
	//
	// The TransportMessage wrapper for the payload contains some fields designed to help
	// an engine perform this routing to the correct channel. These can be enhanced as required, but that
	// does require change to each plugin to propagate that extra field.
	//
	// There is very limited ordering performed by the transport manager itself.
	// It delivers messages to this function:
	// - in whatever order they are received from the transport plugin(s), which is dependent on the _sender_ usually
	// - with whatever concurrency is performed by the transport plugin(s), which is commonly one per remote node, but that's not assured
	HandlePaladinMsg(ctx context.Context, msg *ReceivedMessage)
}

type TransportManager interface {
	ManagerLifecycle
	ConfiguredTransports() map[string]*pldconf.PluginConfig
	TransportRegistered(name string, id uuid.UUID, toTransport TransportManagerToTransport) (fromTransport plugintk.TransportCallbacks, err error)
	LocalNodeName() string

	// Send a message - performs a cache-optimized registry lookup of the transport to use for the node,
	// then synchronously calls the transport to *accept* the message for sending.
	// The caller should assume this could involve I/O and hence might block the calling routine.
	// However, how much actual I/O is performed in-line with the function call is transport plugin dependent.
	//
	// The transport tries to feedback failure when it is immediate, but the transport has no guarantees
	// on delivery, and the target failing to process the message should be considered a possible
	// situation to recover from (although not critical path).
	//
	// at-most-once delivery semantics
	Send(ctx context.Context, send *FireAndForgetMessageSend) error

	// Sends a message with at-least-once delivery semantics
	//
	// Each reliable message type has special building code in the transport manager, which assembles the full
	// message by combining the metadata in the ReliableMessage with data looked up from other components.
	// This avoids the performance and storage cost of writing the big data (states, receipts) multiple times.
	//
	// The message is persisted to the DB in the supplied transaction, then sent on the wire with indefinite retry
	// including over node restart, until an ack is returned from the remote node.
	SendReliable(ctx context.Context, dbTX persistence.DBTX, msg ...*pldapi.ReliableMessage) (err error)

	QueryReliableMessages(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.ReliableMessage, error)
	QueryReliableMessageAcks(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.ReliableMessageAck, error)
}
