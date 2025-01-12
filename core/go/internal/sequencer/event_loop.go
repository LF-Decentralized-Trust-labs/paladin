/*
 * Copyright © 2025 Kaleido, Inc.
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

package sequencer

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/msgs"
)

type EventLoop interface {
	Start(ctx context.Context, heartbeatTicker ContractSequencerHeartbeatTicker, transportManager TransportManager, onStop func()) error
}

type eventLoop struct {
	contractSequencerAgent ContractSequencerAgent
	transportManager       TransportManager
}

func NewEventLoop(csa *contractSequencerAgent, transportManager TransportManager) EventLoop {
	return &eventLoop{
		contractSequencerAgent: csa,
		transportManager:       transportManager,
	}
}

func (el *eventLoop) Start(ctx context.Context, heartbeatTicker ContractSequencerHeartbeatTicker, transportManager TransportManager, onStop func()) error {
	if transportManager == nil {
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "transport manager is nil")
	}
	if heartbeatTicker == nil {
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "heartbeatTicker is nil")
	}
	el.transportManager = transportManager
	go func() {
		for {
			select {
			case <-heartbeatTicker.C():
				err := el.contractSequencerAgent.HandleMissedHeartbeat(ctx)
				if err != nil {
					//TODO
				}
			}
		}
	}()
	return nil
}
