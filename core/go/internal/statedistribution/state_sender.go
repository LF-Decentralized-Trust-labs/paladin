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
	"time"

	"github.com/kaleido-io/paladin/core/internal/components"
	pb "github.com/kaleido-io/paladin/core/pkg/proto/sequence"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"google.golang.org/protobuf/proto"
)

func (sd *stateDistributer) DistributeStates(ctx context.Context, stateDistributions []*StateDistribution) {
	log.L(ctx).Debugf("stateDistributer:DistributeStates %d state distributions", len(stateDistributions))
	for _, stateDistribution := range stateDistributions {
		sd.inputChan <- stateDistribution
	}
}

func (sd *stateDistributer) sendState(ctx context.Context, stateDistribution *StateDistribution) {
	log.L(ctx).Debugf("stateDistributer:sendState %s %s %s %s %s %s", stateDistribution.Domain, stateDistribution.ContractAddress, stateDistribution.SchemaID, stateDistribution.StateID, stateDistribution.IdentityLocator, stateDistribution.ID)

	stateProducedEvent := &pb.StateProducedEvent{
		DomainName:      stateDistribution.Domain,
		ContractAddress: stateDistribution.ContractAddress,
		SchemaId:        stateDistribution.SchemaID,
		StateId:         stateDistribution.StateID,
		StateDataJson:   stateDistribution.StateDataJson,
		Party:           stateDistribution.IdentityLocator,
		DistributionId:  stateDistribution.ID,
	}
	stateProducedEventBytes, err := proto.Marshal(stateProducedEvent)
	if err != nil {
		log.L(ctx).Errorf("Error marshalling delegate transaction message: %s", err)
		return
	}

	targetNode, err := tktypes.PrivateIdentityLocator(stateDistribution.IdentityLocator).Node(ctx, false)
	if err != nil {
		log.L(ctx).Errorf("Error getting node for party %s", stateDistribution.IdentityLocator)
		return
	}

	err = sd.transportManager.Send(ctx, &components.TransportMessage{
		MessageType: "StateProducedEvent",
		Payload:     stateProducedEventBytes,
		Node:        targetNode,
		Component:   STATE_DISTRIBUTER_DESTINATION,
		ReplyTo:     sd.nodeID,
	})
	if err != nil {
		log.L(ctx).Errorf("Error sending state produced event: %s", err)
		return
	}

	go func() {
		time.Sleep(RETRY_TIMEOUT)
		sd.retryChan <- stateDistribution.ID
	}()

}
