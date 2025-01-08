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

/*
This file contains utility functions that are used by the sequencer spec tests.
*/

package sequencer

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"gorm.io/gorm"
)

type GivenContractSequencerAgentInObservingStateConditions struct {
	activeCoordinator string
}

type fakeContractSequencerHeartbeatTicker struct {
	heartbeatFailureThreshold int
	csa                       ContractSequencerAgent
	channel                   chan struct{}
}

func (f *fakeContractSequencerHeartbeatTicker) C() <-chan struct{} {
	return f.channel
}

func (f *fakeContractSequencerHeartbeatTicker) ExceedHeartbeatFailureThreshold(ctx context.Context) {
	for i := 0; i <= f.heartbeatFailureThreshold; i++ {
		f.channel <- struct{}{}
	}
}

type ReliableMessageMatcher func(message *components.ReliableMessage) bool
type FireAndForgetMessageMatcher func(message *components.FireAndForgetMessageSend) bool

type OutboundMessageMonitor interface {
	Sends(outboundMessageMatcher FireAndForgetMessageMatcher) bool
	SendsReliable(outboundMessageMatcher ReliableMessageMatcher) bool
}

type fakeTransportManager struct {
	sentFireAndForgetMessages []*components.FireAndForgetMessageSend
	sentReliableMessages      []*components.ReliableMessage
}

func (f *fakeTransportManager) Send(ctx context.Context, send *components.FireAndForgetMessageSend) error {
	f.sentFireAndForgetMessages = append(f.sentFireAndForgetMessages, send)
	return nil
}

func (f *fakeTransportManager) SendReliable(ctx context.Context, dbTX *gorm.DB, msg ...*components.ReliableMessage) (preCommit func(), err error) {
	f.sentReliableMessages = append(f.sentReliableMessages, msg...)
	return func() {}, nil
}

func (f *fakeTransportManager) Sends(outboundMessageMatcher FireAndForgetMessageMatcher) bool {
	// Implement the method as needed for your tests
	for _, message := range f.sentFireAndForgetMessages {
		if outboundMessageMatcher(message) {
			return true
		}
	}
	return false
}

func (f *fakeTransportManager) SendsReliable(outboundMessageMatcher ReliableMessageMatcher) bool {
	// Implement the method as needed for your tests
	for _, message := range f.sentReliableMessages {
		if outboundMessageMatcher(message) {
			return true
		}
	}
	return false
}

// TODO rewrite this as a builder pattern e.g Given().ContractSequencerAgent().InObservingState().WithActiveCoordinator("coordinator1").Build()
func GivenContractSequencerAgentInObservingState(t *testing.T, ctx context.Context, conditions *GivenContractSequencerAgentInObservingStateConditions) (ContractSequencerAgent, *fakeContractSequencerHeartbeatTicker, OutboundMessageMonitor) {
	csa := NewContractSequencerAgent().(*contractSequencerAgent)
	if conditions != nil {
		csa.activeCoordinator = conditions.activeCoordinator
	}
	fakeContractSequencerHeartbeatTicker := &fakeContractSequencerHeartbeatTicker{
		csa:     csa,
		channel: make(chan struct{}),
	}

	outboundMessageMonitor := &fakeTransportManager{}

	csa.Start(ctx, fakeContractSequencerHeartbeatTicker, outboundMessageMonitor, func() {})
	return csa, fakeContractSequencerHeartbeatTicker, outboundMessageMonitor
}

func GivenContractSequencerAgentInSenderState(t *testing.T, ctx context.Context, conditions *GivenContractSequencerAgentInObservingStateConditions) (ContractSequencerAgent, *fakeContractSequencerHeartbeatTicker, OutboundMessageMonitor) {
	csa := NewContractSequencerAgent().(*contractSequencerAgent)

	if conditions != nil && conditions.activeCoordinator != "" {
		csa.activeCoordinator = conditions.activeCoordinator
	} else {
		//create a random coordinator name
		randomCoordinator := uuid.New().String()
		csa.activeCoordinator = randomCoordinator
	}

	fakeContractSequencerHeartbeatTicker := &fakeContractSequencerHeartbeatTicker{
		csa:     csa,
		channel: make(chan struct{}),
	}
	outboundMessageMonitor := &fakeTransportManager{}
	//create a random transaction
	transaction := &components.PrivateTransaction{}
	transaction.ID = uuid.New()
	csa.delegationsByTransactionID[transaction.ID.String()] = &delegation{
		transaction: transaction,
	}
	csa.Start(ctx, fakeContractSequencerHeartbeatTicker, outboundMessageMonitor, func() {})
	return csa, fakeContractSequencerHeartbeatTicker, outboundMessageMonitor
}

func ActiveCoordinatorBecomesEmpty(t *testing.T, ctx context.Context, csa ContractSequencerAgent) bool {
	// Wait for the state change notifier to be triggered
	select {
	case <-csa.(*contractSequencerAgent).stateChangeNotifier:
	case <-ctx.Done():
		t.Log("Timed out waiting for state change notifier")
	case <-time.After(2 * time.Second):
		t.Log("Timed out waiting for state change notifier")
	}
	return csa.ActiveCoordinator() == ""
}

func ActiveCoordinatorBecomesEqualTo(t *testing.T, ctx context.Context, csa ContractSequencerAgent, activeCoordinator string) bool {
	// Wait for the state change notifier to be triggered
	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-csa.(*contractSequencerAgent).stateChangeNotifier:
			if csa.ActiveCoordinator() == activeCoordinator {
				return true
			}
		case <-ctx.Done():
			t.Log("Timed out waiting for state change notifier")
			return csa.ActiveCoordinator() == activeCoordinator
		case <-timeout:
			return csa.ActiveCoordinator() == activeCoordinator
		}
	}
}

func DelegationRequestMatcher(message *components.FireAndForgetMessageSend) bool {
	return message.MessageType == "DelegationRequest"
}
