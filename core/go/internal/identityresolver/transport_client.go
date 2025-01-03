// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package identityresolver

import (
	"context"

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

func (ir *identityResolver) HandlePaladinMsg(ctx context.Context, message *components.TransportMessage) {
	//TODO this need to become an ultra low latency, non blocking, handover to the event loop thread.
	// need some thought on how to handle errors, retries, buffering, swapping idle sequencers in and out of memory etc...

	//Send the event to the sequencer for the contract and any transaction manager for the signing key
	messagePayload := message.Payload
	replyToDestination := message.ReplyTo

	switch message.MessageType {

	case "ResolveVerifierRequest":
		go ir.handleResolveVerifierRequest(ctx, messagePayload, replyToDestination, &message.MessageID)
	case "ResolveVerifierResponse":
		go ir.handleResolveVerifierReply(ctx, messagePayload, message.CorrelationID.String())
	case "ResolveVerifierError":
		go ir.handleResolveVerifierError(ctx, messagePayload, message.CorrelationID.String())
	default:
		log.L(ctx).Errorf("Unknown message type: %s", message.MessageType)
	}
}
