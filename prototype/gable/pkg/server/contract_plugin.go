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
package server

import (
	"context"
	"io"

	"github.com/aidarkhanov/nanoid"
	"github.com/hyperledger/firefly-common/pkg/log"

	"github.com/kaleido-io/paladin/gable/pkg/proto"
)

type ContractPlugin interface {
	Listen()
}

type contractPlugin struct {
	eventHandlerDone chan struct{}
	eventStream      proto.PaladinContractPluginService_RegisterServer
	contractId       string
}

func NewContractPlugin(contractEventStream proto.PaladinContractPluginService_RegisterServer) ContractPlugin {
	return &contractPlugin{
		eventStream:      contractEventStream,
		eventHandlerDone: make(chan struct{}),
	}
}

func (cp *contractPlugin) eventHandler(ctx context.Context) {
	defer close(cp.eventHandlerDone)
	for {
		event, err := cp.eventStream.Recv()
		if err == io.EOF {
			log.L(ctx).Info("EOF - exiting")
			return
		}

		if event.CorrelationId == "" {
			// Always just send back an ack for now
			log.L(ctx).Infof("Received event %s [%s]", event, event.Type)
			if err := cp.eventStream.Send(&proto.ContractPluginEvent{
				ContractPluginId: cp.contractId,
				Type:             "ack",
				Arguments:        []string{},
				Id:               nanoid.New(),
				CorrelationId:    event.Id,
			}); err != nil {
				log.L(ctx).Error("Error sending - closing channel", err)
				return
			}
		} else {
			log.L(ctx).Infof("Received reply %s to event %s [%s]", event, event.CorrelationId, event.Type)
		}
	}
}

func (cp *contractPlugin) Listen() {

	cp.eventHandler(log.WithLogField(cp.eventStream.Context(), "contractId", cp.contractId))

}
