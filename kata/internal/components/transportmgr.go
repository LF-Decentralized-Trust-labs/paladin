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
)

// TransportTarget splits out the four parts of the routing required
type TransportTarget struct {
	Node string
	// Component string // TODO: Need to discuss with Hosie
	// Identity string // TODO: Need to discuss with Hosie
}

type TransportMessage struct {
	MessageID     uuid.UUID
	CorrelationID *uuid.UUID
	Destination   TransportTarget
	ReplyTo       TransportTarget
	Payload       []byte
}

type TransportMessageInput struct {
	Destination     TransportTarget
	ReplyToIdentity string
	CorrelationID   *uuid.UUID
	Payload         []byte
}

type TransportManagerToTransport interface {
	plugintk.TransportAPI
	Initialized()
}

type TransportManager interface {
	ManagerLifecycle
	ConfiguredTransports() map[string]*PluginConfig
	TransportRegistered(name string, id uuid.UUID, toTransport TransportManagerToTransport) (fromTransport plugintk.TransportCallbacks, err error)
	Send(ctx context.Context, message *TransportMessageInput) error
}
