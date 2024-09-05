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
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type TransportMessage struct {
	MessageID     uuid.UUID
	CorrelationID *uuid.UUID
	Destination   tktypes.PrivateIdentityLocator
	ReplyTo       tktypes.PrivateIdentityLocator
	MessageType   string
	Payload       []byte
}

type TransportManagerToTransport interface {
	plugintk.TransportAPI
	Initialized()
}

type TransportManager interface {
	ManagerLifecycle
	ConfiguredTransports() map[string]*PluginConfig
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
	// e.g. at-most-once delivery semantics
	Send(ctx context.Context, message *TransportMessage) error
}
