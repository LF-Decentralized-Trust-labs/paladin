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

package preparedtxdistribution

import (
	"context"
	"encoding/json"

	"github.com/kaleido-io/paladin/core/internal/components"
	pb "github.com/kaleido-io/paladin/core/pkg/proto/engine"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"google.golang.org/protobuf/proto"
)

func (sd *preparedTransactionDistributer) HandlePaladinMsg(ctx context.Context, message *components.TransportMessage) {
	log.L(ctx).Debugf("preparedTransactionDistributer:HandlePaladinMsg")
	messagePayload := message.Payload

	switch message.MessageType {
	case "PreparedTransactionMessage":
		distributingNode := message.ReplyTo
		go sd.handlePreparedTransactionMessage(ctx, messagePayload, distributingNode)
	case "PreparedTransactionAcknowledgedMessage":
		go sd.handlePreparedTransactionAcknowledgedMessage(ctx, message.Payload)
	default:
		log.L(ctx).Errorf("Unknown message type: %s", message.MessageType)
	}
}

func (sd *preparedTransactionDistributer) handlePreparedTransactionMessage(ctx context.Context, messagePayload []byte, distributingNode string) {
	log.L(ctx).Debugf("preparedTransactionDistributer:handlePreparedTransactionMessage")
	preparedTransactionMessage := &pb.PreparedTransactionMessage{}
	err := proto.Unmarshal(messagePayload, preparedTransactionMessage)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal PreparedTransactionMessage: %s", err)
		return
	}

	receivedTransaction := new(components.PrepareTransactionWithRefs)
	err = json.Unmarshal(preparedTransactionMessage.PreparedTransactionJson, receivedTransaction)
	if err != nil {
		log.L(ctx).Errorf("Error unmarshalling prepared transaction json: %s", err)
	}

	err = sd.receivedPreparedTransactionWriter.QueueAndWait(
		ctx,
		preparedTransactionMessage.DomainName,
		*tktypes.MustEthAddress(preparedTransactionMessage.ContractAddress),
		receivedTransaction,
	)
	if err != nil {
		log.L(ctx).Errorf("Error writing prepared transaction: %s", err)
		//don't send the acknowledgement, with a bit of luck, the sender will retry and we will get it next time
		return
	}

	// No error means either this is the first time we have received this prepared transaction or we already have it an onConflict ignore means we idempotently accept it
	// If the latter, then the sender probably didn't get our previous acknowledgement so either way, we send an acknowledgement

	err = sd.sendPreparedTransactionAcknowledgement(
		ctx,
		preparedTransactionMessage.DomainName,
		preparedTransactionMessage.ContractAddress,
		preparedTransactionMessage.PreparedTxnId,
		preparedTransactionMessage.Party,
		distributingNode,
		preparedTransactionMessage.DistributionId)
	if err != nil {
		log.L(ctx).Errorf("Error sending prepared transaction acknowledgement: %s", err)
		//not much more we can do here.  The sender will inevitably retry and we will hopefully send the ack next time
	}
}

func (sd *preparedTransactionDistributer) handlePreparedTransactionAcknowledgedMessage(ctx context.Context, messagePayload []byte) {
	log.L(ctx).Debugf("preparedTransactionDistributer:handlePreparedTransactionAcknowledgedMessage")
	preparedTransactionAcknowledgedMessage := &pb.PreparedTransactionAcknowledgedMessage{}
	err := proto.Unmarshal(messagePayload, preparedTransactionAcknowledgedMessage)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal PreparedTransactionAcknowledgedMessage: %s", err)
		return
	}
	sd.acknowledgementWriter.Queue(ctx, preparedTransactionAcknowledgedMessage.DistributionId)
	// no need to wait for the flush to complete, we can just stop the in memory loop from retrying
	// worst case scenario, we crash before this is written to the DB, we do some redundant retries after a restart
	// but waiting for the flush here is not going to prevent that
	sd.acknowledgedChan <- preparedTransactionAcknowledgedMessage.DistributionId

}
