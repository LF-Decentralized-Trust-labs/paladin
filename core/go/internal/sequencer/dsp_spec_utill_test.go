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
	"math/rand"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
)

// The Fixture Builder pattern is a builder for creating test fixtures
// The Builder objects follow the builder pattern, but the goal is to construct a test fixture.  The fields of the Builder object hold the instructions for creating the fixture. The `Build` method constructs the fixture object using those instructions and/or defaults/random values if no instructions are provided.
// The Fixture objects contain a pointer to the actual product code under test (the CUT) as well as values that record the data used to initialize that code at the beginning of the test. The Cut can stimulated and then inspected to assert the desired behavior of the spec. The fixture values can be referenced later in the test e.g. to compare against the final state of the CUT.
//TODO - consider whether we actually separate fixture and builder objects or whether the fixture can be a builder.

type GivenContractSequencerAgentInObservingStateConditions struct {
	activeCoordinator string
}

func contains[T comparable](slice []T, item T) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

type FixtureBuilder struct {
	t *testing.T
}

type ContractSequencerAgentFixtureBuilder struct {
	FixtureBuilder
	activeCoordinator          string
	isInObservingState         bool
	isInSenderState            bool
	delegatedTransactions      *DelegatedTransactionListFixtureBuilder
	dispatchedTransactionLists []*DispatchedTransactionListFixture
}

type DispatchedTransactionListFixture struct {
	TestFixture
	dispatchedTransactions []*DispatchedTransaction
	coordinator            string
}

type DispatchedTransactionListFixtureBuilder struct {
	FixtureBuilder
	length      int
	coordinator string
}

type DelegatedTransactionListFixtureBuilder struct {
	FixtureBuilder
	length      int
	coordinator string
}

type TestFixture struct {
	ctx context.Context
}

type ContractSequencerAgentFixture struct {
	TestFixture
	csa                    *contractSequencerAgent
	outboundMessageMonitor *fakeTransportManager
	delegatedTransactions  *DelegatedTransactionListFixture
}

type DelegatedTransactionListFixture struct {
	TestFixture
	delegatedTransactions []*components.PrivateTransaction
	coordinator           string
}

func Given(t *testing.T) *FixtureBuilder {
	return &FixtureBuilder{
		t: t,
	}
}

func (f *FixtureBuilder) ContractSequencerAgent() *ContractSequencerAgentFixtureBuilder {
	return &ContractSequencerAgentFixtureBuilder{
		FixtureBuilder: *f,
	}
}

func (c *ContractSequencerAgentFixtureBuilder) InObservingState() *ContractSequencerAgentFixtureBuilder {
	c.isInObservingState = true
	return c
}

func (c *ContractSequencerAgentFixtureBuilder) InSenderState() *ContractSequencerAgentFixtureBuilder {
	c.isInSenderState = true
	return c
}

func (c *ContractSequencerAgentFixtureBuilder) ActiveCoordinator(activeCoordinator string) *ContractSequencerAgentFixtureBuilder {
	c.activeCoordinator = activeCoordinator
	return c
}

func (c *ContractSequencerAgentFixtureBuilder) DelegatedTransactions(fn func(builder *DelegatedTransactionListFixtureBuilder)) *ContractSequencerAgentFixtureBuilder {
	c.delegatedTransactions = &DelegatedTransactionListFixtureBuilder{
		FixtureBuilder: c.FixtureBuilder,
	}
	fn(c.delegatedTransactions)
	return c
}

func (c *ContractSequencerAgentFixtureBuilder) DispatchedTransactions(transactions ...*DispatchedTransactionListFixture) *ContractSequencerAgentFixtureBuilder {
	// Flatten all transaction arrays into a single array
	for _, txList := range transactions {
		c.dispatchedTransactionLists = append(c.dispatchedTransactionLists, txList)
	}
	return c
}

func (c *ContractSequencerAgentFixtureBuilder) Build() *ContractSequencerAgentFixture {
	ctx := context.Background()

	outboundMessageMonitor := newFakeTransportManager(c.t)

	csa := NewContractSequencerAgent(outboundMessageMonitor).(*contractSequencerAgent)
	if c.isInObservingState {
		csa.activeCoordinator = c.activeCoordinator
	}
	if c.isInSenderState {
		delegatedTransactionsFixture := c.delegatedTransactions.Build()
		for _, transaction := range delegatedTransactionsFixture.delegatedTransactions {
			csa.delegationsByTransactionID[transaction.ID.String()] = &delegation{
				transaction: transaction,
			}
		}
		//there must be an active coordinator if we are in sender state so set it to a random value if not already provided
		if csa.activeCoordinator == "" {
			csa.activeCoordinator = uuid.New().String()
		}
	}

	for _, c := range c.dispatchedTransactionLists {
		if csa.dispatchedTransactionsByCoordinator == nil {
			csa.dispatchedTransactionsByCoordinator = make(map[string][]*DispatchedTransaction)
		}
		csa.dispatchedTransactionsByCoordinator[c.coordinator] = c.dispatchedTransactions
	}

	return &ContractSequencerAgentFixture{
		TestFixture: TestFixture{
			ctx: ctx,
		},
		csa:                    csa,
		outboundMessageMonitor: outboundMessageMonitor,
	}
}

func (f *ContractSequencerAgentFixture) ExceedHeartbeatFailureThreshold(ctx context.Context) {
	for i := 0; i <= f.csa.heartbeatFailureThreshold; i++ {
		f.csa.HandleMissedHeartbeat(ctx)
	}
}

func (f *FixtureBuilder) DispatchedTransactionList() *DispatchedTransactionListFixtureBuilder {
	return &DispatchedTransactionListFixtureBuilder{
		FixtureBuilder: *f,
		coordinator:    uuid.New().String(),
	}
}

func (d *DispatchedTransactionListFixtureBuilder) Coordinator(coordinator string) *DispatchedTransactionListFixtureBuilder {
	d.coordinator = coordinator
	return d
}

func (d *DispatchedTransactionListFixtureBuilder) Length(length int) *DispatchedTransactionListFixtureBuilder {
	d.length = length
	return d
}

func (d *DispatchedTransactionListFixtureBuilder) Build() *DispatchedTransactionListFixture {

	dispatchedTransactions := make([]*DispatchedTransaction, d.length)
	for i := 0; i < d.length; i++ {
		dispatchedTransactions[i] = &DispatchedTransaction{
			TransactionID:        uuid.New(),
			Signer:               *tktypes.RandAddress(),
			LatestSubmissionHash: tktypes.RandBytes(32),
			Nonce:                rand.Uint64(),
		}
	}
	return &DispatchedTransactionListFixture{
		dispatchedTransactions: dispatchedTransactions,
		coordinator:            d.coordinator,
	}
}

func (f *FixtureBuilder) DelegatedTransactionList() *DelegatedTransactionListFixtureBuilder {
	return &DelegatedTransactionListFixtureBuilder{
		FixtureBuilder: *f,
		coordinator:    uuid.New().String(),
	}
}

func (d *DelegatedTransactionListFixtureBuilder) Coordinator(coordinator string) *DelegatedTransactionListFixtureBuilder {
	d.coordinator = coordinator
	return d
}

func (d *DelegatedTransactionListFixtureBuilder) Length(length int) *DelegatedTransactionListFixtureBuilder {
	d.length = length
	return d
}

func (d *DelegatedTransactionListFixtureBuilder) Build() *DelegatedTransactionListFixture {
	delegatedTransactions := make([]*components.PrivateTransaction, d.length)
	for i := 0; i < d.length; i++ {
		transaction := &components.PrivateTransaction{
			ID: uuid.New(),
		}
		delegatedTransactions[i] = transaction
	}
	return &DelegatedTransactionListFixture{
		delegatedTransactions: delegatedTransactions,
		coordinator:           d.coordinator,
	}
}

type ReliableMessageMatcher func(message *components.ReliableMessage) bool
type FireAndForgetMessageMatcher func(message *components.FireAndForgetMessageSend) bool

type OutboundMessageMonitor interface {
	Sends(outboundMessageMatcher FireAndForgetMessageMatcher) bool
	SendsReliable(outboundMessageMatcher ReliableMessageMatcher) bool
}

func GivenContractSequencerAgentInSenderState(t *testing.T, ctx context.Context, conditions *GivenContractSequencerAgentInObservingStateConditions) (ContractSequencerAgent, *fakeContractSequencerHeartbeatTicker, OutboundMessageMonitor) {
	outboundMessageMonitor := newFakeTransportManager(t)
	csa := NewContractSequencerAgent(outboundMessageMonitor).(*contractSequencerAgent)

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

	//create a random transaction
	transaction := &components.PrivateTransaction{}
	transaction.ID = uuid.New()
	csa.delegationsByTransactionID[transaction.ID.String()] = &delegation{
		transaction: transaction,
	}
	return csa, fakeContractSequencerHeartbeatTicker, outboundMessageMonitor
}

func ActiveCoordinatorBecomesEmpty(t *testing.T, ctx context.Context, csa ContractSequencerAgent) bool {
	//TODO this is worded as if it is a "eventually..." type of assertion but at the moment the CSA is not doing any complex threading stuff so this us a plain old assertion
	return csa.ActiveCoordinator() == ""
}

func ActiveCoordinatorBecomesEqualTo(t *testing.T, ctx context.Context, csa ContractSequencerAgent, activeCoordinator string) bool {
	//TODO this is worded as if it is a "eventually..." type of assertion but at the moment the CSA is not doing any complex threading stuff so this us a plain old assertion

	return csa.ActiveCoordinator() == activeCoordinator
}

type DelegationRequestMatcherBuilder struct {
	expectedDelegatedTransactions *DelegatedTransactionListFixture
}

func DelegationRequestMatcher() *DelegationRequestMatcherBuilder {
	return &DelegationRequestMatcherBuilder{}
}

func (d *DelegationRequestMatcherBuilder) Containing(delegatedTransactions *DelegatedTransactionListFixture) *DelegationRequestMatcherBuilder {
	d.expectedDelegatedTransactions = delegatedTransactions

	return d
}

func (d *DelegationRequestMatcherBuilder) Match() FireAndForgetMessageMatcher {
	return func(message *components.FireAndForgetMessageSend) bool {
		if message.MessageType != "DelegationRequest" {
			return false
		}
		if d.expectedDelegatedTransactions != nil {
			actualDelegationRequest, err := ParseDelegationRequest(message.Payload)
			if err != nil {
				return false
			}
			if len(actualDelegationRequest.Transactions) != len(d.expectedDelegatedTransactions.delegatedTransactions) {
				return false
			}
			actualTransactionIDs := make([]string, len(actualDelegationRequest.Transactions))
			for i, transaction := range actualDelegationRequest.Transactions {
				actualTransactionIDs[i] = transaction.ID.String()
			}
			for _, transaction := range d.expectedDelegatedTransactions.delegatedTransactions {
				expectedTransactionID := transaction.ID.String()
				if !contains(actualTransactionIDs, expectedTransactionID) {
					return false
				}
			}
		}
		return true
	}
}

// RandomDispatchedTransaction creates DispatchedTransaction with random values
func RandomDispatchedTransaction() *DispatchedTransaction {
	return &DispatchedTransaction{
		TransactionID:        uuid.New(),
		Signer:               *tktypes.RandAddress(),
		LatestSubmissionHash: tktypes.RandBytes(32),
	}
}

// Tests for the above utilities
func TestFixtureBuilder(t *testing.T) {
	fixture := Given(t).
		ContractSequencerAgent().
		InObservingState().
		ActiveCoordinator("coordinator1").
		Build()

	assert.NotNil(t, fixture.csa)
	assert.Equal(t, "coordinator1", fixture.csa.ActiveCoordinator())

}
