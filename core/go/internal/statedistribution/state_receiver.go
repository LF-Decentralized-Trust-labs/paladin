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
	"google.golang.org/protobuf/proto"
)

func (sd *stateDistributer) AcknowledgeState(ctx context.Context, stateDistributionID string) {
	log.L(ctx).Debugf("stateDistributer:AcknowledgeState %s ", stateDistributionID)
	sd.acknowledgedChan <- stateDistributionID
}

func (sd *stateDistributer) sendStateAcknowledgement(ctx context.Context, domainName string, contractAddress string, stateId string, receivingParty string, distributingNode string, distributionID string) error {
	log.L(ctx).Debugf("stateDistributer:sendStateAcknowledgement %s %s %s %s %s %s", domainName, contractAddress, stateId, receivingParty, distributingNode, distributionID)
	stateAcknowledgedEvent := &pb.StateAcknowledgedEvent{
		DomainName:      domainName,
		ContractAddress: contractAddress,
		StateId:         stateId,
		Party:           receivingParty,
		DistributionId:  distributionID,
	}
	stateAcknowledgedEventBytes, err := proto.Marshal(stateAcknowledgedEvent)
	if err != nil {
		log.L(ctx).Errorf("Error marshalling state acknowledgment event: %s", err)
		return err
	}

	err = sd.transportManager.Send(ctx, &components.TransportMessage{
		MessageType: "StateAcknowledgedEvent",
		Payload:     stateAcknowledgedEventBytes,
		Node:        distributingNode,
		Component:   STATE_DISTRIBUTER_DESTINATION,
		ReplyTo:     sd.nodeID,
	})
	if err != nil {
		log.L(ctx).Errorf("Error sending state produced event: %s", err)
		return err
	}

	return nil
}
