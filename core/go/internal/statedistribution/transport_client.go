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

package statedistribution

import (
	"context"

	"github.com/kaleido-io/paladin/core/internal/components"
	pb "github.com/kaleido-io/paladin/core/pkg/proto/engine"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"google.golang.org/protobuf/proto"
)

const STATE_DISTRIBUTER_DESTINATION = "state-distributer"

func (sd *stateDistributer) Destination() string {
	return STATE_DISTRIBUTER_DESTINATION
}

func (sd *stateDistributer) ReceiveTransportMessage(ctx context.Context, message *components.TransportMessage) {
	log.L(ctx).Debugf("stateDistributer:ReceiveTransportMessage")
	messagePayload := message.Payload

	switch message.MessageType {
	case "StateProducedEvent":
		distributingNode := message.ReplyTo
		go sd.handleStateProducedEvent(ctx, messagePayload, distributingNode)
	case "StateAcknowledgedEvent":
		go sd.handleStateAcknowledgedEvent(ctx, message.Payload)
	default:
		log.L(ctx).Errorf("Unknown message type: %s", message.MessageType)
	}
}

func (sd *stateDistributer) handleStateProducedEvent(ctx context.Context, messagePayload []byte, distributingNode string) {
	log.L(ctx).Debugf("stateDistributer:handleStateProducedEvent")
	stateProducedEvent := &pb.StateProducedEvent{}
	err := proto.Unmarshal(messagePayload, stateProducedEvent)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal StateProducedEvent: %s", err)
		return
	}

	err = sd.receivedStateWriter.QueueAndWait(ctx, stateProducedEvent.DomainName, *tktypes.MustEthAddress(stateProducedEvent.ContractAddress), tktypes.MustParseBytes32(stateProducedEvent.SchemaId), tktypes.RawJSON(stateProducedEvent.StateDataJson))
	if err != nil {
		log.L(ctx).Errorf("Error writing state: %s", err)
		//don't send the acknowledgement, with a bit of luck, the sender will retry and we will get it next time
		return
	}

	// No error means either this is the first time we have received this state or we already have it an onConflict ignore means we idempotently accept it
	// If the latter, then the sender probably didn't get our previous acknowledgement so either way, we send an acknowledgement

	err = sd.sendStateAcknowledgement(
		ctx,
		stateProducedEvent.DomainName,
		stateProducedEvent.ContractAddress,
		stateProducedEvent.StateId,
		stateProducedEvent.Party,
		distributingNode,
		stateProducedEvent.DistributionId)
	if err != nil {
		log.L(ctx).Errorf("Error sending state acknowledgement: %s", err)
		//not much more we can do here.  The sender will inevitably retry and we will hopefully send the ack next time
	}
}

func (sd *stateDistributer) handleStateAcknowledgedEvent(ctx context.Context, messagePayload []byte) {
	log.L(ctx).Debugf("stateDistributer:handleStateAcknowledgedEvent")
	stateAcknowledgedEvent := &pb.StateAcknowledgedEvent{}
	err := proto.Unmarshal(messagePayload, stateAcknowledgedEvent)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal StateAcknowledgedEvent: %s", err)
		return
	}
	sd.acknowledgementWriter.Queue(ctx, stateAcknowledgedEvent.DistributionId)
	// no need to wait for the flush to complete, we can just stop the in memory loop from retrying
	// worst case scenario, we crash before this is written to the DB, we do some redundant retries after a restart
	// but waiting for the flush here is not going to prevent that
	sd.acknowledgedChan <- stateAcknowledgedEvent.DistributionId

}
