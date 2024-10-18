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

package privatetxnmgr

import (
	"context"

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

// If we had lots of these we would probably want to centralise the assignment of the constants to avoid duplication
// but currently there is only 2 ( the other being IDENTITIY_RESOLVER_DESTINATION )
const PRIVATE_TX_MANAGER_DESTINATION = "private-tx-manager"

func (p *privateTxManager) Destination() string {
	return PRIVATE_TX_MANAGER_DESTINATION
}

func (p *privateTxManager) ReceiveTransportMessage(ctx context.Context, message *components.TransportMessage) {
	//TODO this need to become an ultra low latency, non blocking, handover to the event loop thread.
	// need some thought on how to handle errors, retries, buffering, swapping idle orchestrators in and out of memory etc...

	//Send the event to the orchestrator for the contract and any transaction manager for the signing key
	messagePayload := message.Payload
	replyToDestination := message.ReplyTo

	switch message.MessageType {
	case "EndorsementRequest":
		go p.handleEndorsementRequest(ctx, messagePayload, replyToDestination)
	case "EndorsementResponse":
		go p.handleEndorsementResponse(ctx, messagePayload)
	case "DelegationRequest":
		go p.handleDelegationRequest(ctx, messagePayload)
	default:
		log.L(ctx).Errorf("Unknown message type: %s", message.MessageType)
	}

}
