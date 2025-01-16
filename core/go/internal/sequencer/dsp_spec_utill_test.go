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
	"errors"
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
)

//TODO seed the random generator so that tests are deterministic

// The Fixture Builder pattern is a builder for creating test fixtures
// The Builder objects follow the builder pattern, but the goal is to construct a test fixture.  The fields of the Builder object hold the instructions for creating the fixture. The `Build` method constructs the fixture object using those instructions and/or defaults/random values if no instructions are provided.
// The Fixture objects contain a pointer to the actual product code under test (the CUT) as well as values that record the data used to initialize that code at the beginning of the test. The Cut can stimulated and then inspected to assert the desired behavior of the spec. The fixture values can be referenced later in the test e.g. to compare against the final state of the CUT.
//TODO - consider whether we actually separate fixture and builder objects or whether the fixture can be a builder.

// FixtureBuilder functions come in a few flavours
// Basic setters.  They set a scalar or a single object value on the builder that is used to initialize the fixture instead of the random or default value. IN cases where they set an object value, a new type is defined for the test utils ( separate from the type used in the actual code) to pass the values of that object.
// List setters.  Similar to Basic setter but take an array of scalars or objects
// BuilderFunction. these pass a function that takes a FixtureBuilder for a sub object.

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
	nodeName                            string
	activeCoordinator                   string
	isInObservingState                  bool
	isInSenderState                     bool
	coordinatorState                    CoordinatorState
	committee                           []string
	delegatedTransactions               []DelegatedTransaction
	pooledTransactions                  *PooledTransactionListFixtureBuilder
	dispatchedTransactionLists          []*DispatchedTransactionListFixture
	blockHeight                         int64
	coordinatorSelector                 *coordinatorSelectorForTesting
	startingBlockRange                  uint64
	coordinatorRankingForCurrentBlock   []string
	coordinatorRankingForNextBlock      []string
	latestReceivedHeartbeatNotification CoordinatorHeartbeatNotificationBuilder
}

const (
	BlockRange_Current = iota
	BlockRange_Next
)

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

type PooledTransactionListFixtureBuilder struct {
	FixtureBuilder
	length      int
	coordinator string
}

type PrivateTransactionListFixtureBuilder struct {
	FixtureBuilder
	length int
}

type TestFixture struct {
	ctx context.Context
}

type ContractSequencerAgentFixture struct {
	TestFixture
	csa                    *contractSequencerAgent
	outboundMessageMonitor *fakeTransportManager
	pooledTransactions     *PooledTransactionListFixture
	blockHeight            int64
	coordinatorSelector    *coordinatorSelectorForTesting
	startingBlockRange     uint64
}

type PooledTransactionListFixture struct {
	TestFixture
	pooledTransactions []*components.PrivateTransaction
	coordinator        string
}

type PrivateTransactionListFixture struct {
	TestFixture
	privateTransactions []*components.PrivateTransaction
}

func Given(t *testing.T) *FixtureBuilder {
	return &FixtureBuilder{
		t: t,
	}
}

func (f *FixtureBuilder) ContractSequencerAgent() *ContractSequencerAgentFixtureBuilder {
	return &ContractSequencerAgentFixtureBuilder{
		FixtureBuilder:      *f,
		coordinatorSelector: newCoordinatorSelectorForTesting(f.t),
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

func (c *ContractSequencerAgentFixtureBuilder) Committee(committeeMember ...string) *ContractSequencerAgentFixtureBuilder {
	c.committee = committeeMember
	return c
}

func (c *ContractSequencerAgentFixtureBuilder) ActiveCoordinator(activeCoordinator string) *ContractSequencerAgentFixtureBuilder {
	c.activeCoordinator = activeCoordinator
	return c
}

func (c *ContractSequencerAgentFixtureBuilder) CoordinatorState(coordinatorState CoordinatorState) *ContractSequencerAgentFixtureBuilder {
	c.coordinatorState = coordinatorState
	return c
}

func (c *ContractSequencerAgentFixtureBuilder) LatestReceivedHeartbeatNotification(latestReceivedHeartbeatNotificationFn func(builder CoordinatorHeartbeatNotificationBuilder)) *ContractSequencerAgentFixtureBuilder {
	latestReceivedHeartbeatNotificationFn(c.latestReceivedHeartbeatNotification)
	return c
}

func (c *ContractSequencerAgentFixtureBuilder) NodeName(nodeName string) *ContractSequencerAgentFixtureBuilder {
	c.nodeName = nodeName
	return c
}

/*const (
	BlockHeight_LastInRange = -1
)*/

/*
CoordinatorHeartbeatNotificationBuilder
*/
type CoordinatorHeartbeatNotificationBuilder struct {
	from              string
	contractAddress   *tktypes.EthAddress
	flushPoints       []uuid.UUID //TODO may need to replace this with an array of FlushPoint objects ( which would be a struct with a UUID and a signing address)
	coordinatorStatus CoordinatorState
}

func NewCoordinatorHeartbeatNotificationBuilder() *CoordinatorHeartbeatNotificationBuilder {
	return &CoordinatorHeartbeatNotificationBuilder{}
}

func (c *CoordinatorHeartbeatNotificationBuilder) CoordinatorState(coordinatorStatus CoordinatorState) *CoordinatorHeartbeatNotificationBuilder {
	c.coordinatorStatus = coordinatorStatus
	return c
}

func (c *CoordinatorHeartbeatNotificationBuilder) FlushPoint(flushPoints []uuid.UUID) *CoordinatorHeartbeatNotificationBuilder {
	c.flushPoints = flushPoints
	return c
}

//func (c *ContractSequencerAgentFixtureBuilder) RelativeBlockHeight(height int64) *ContractSequencerAgentFixtureBuilder {
//TODO do we ever have a need to
// set the actual block height on the CSA? All of the rules are based on the block range number and the behavior when the range changes or the range is different from some other node sending a particular message
// in fact, other than logging, is there ever a need for the csa to track the actual block height
// rather than simply the range number? c.blockHeight = height
//return c
//}

func (c *ContractSequencerAgentFixtureBuilder) CoordinatorRankingForCurrentBlock(coordinatorNames ...string) *ContractSequencerAgentFixtureBuilder {
	c.coordinatorRankingForCurrentBlock = coordinatorNames

	return c
}

func (c *ContractSequencerAgentFixtureBuilder) CoordinatorRankingForNextBlock(coordinatorNames ...string) *ContractSequencerAgentFixtureBuilder {
	c.coordinatorRankingForNextBlock = coordinatorNames

	return c
}

func (c *coordinatorSelectorForTesting) coordinatorRank(blockRange uint64, rank int, coordinatorID string) {

	if c.coordinatorRanks == nil {
		c.coordinatorRanks = make(map[uint64]map[int]string)
	}
	if c.coordinatorRanks[blockRange] == nil {
		c.coordinatorRanks[blockRange] = make(map[int]string)
	}
	c.coordinatorRanks[blockRange][rank] = coordinatorID
}

func (c *ContractSequencerAgentFixtureBuilder) PooledTransactions(fn func(builder *PooledTransactionListFixtureBuilder)) *ContractSequencerAgentFixtureBuilder {
	c.pooledTransactions = &PooledTransactionListFixtureBuilder{
		FixtureBuilder: c.FixtureBuilder,
	}
	fn(c.pooledTransactions)
	return c
}

func (c *ContractSequencerAgentFixtureBuilder) DispatchedTransactions(transactions ...*DispatchedTransactionListFixture) *ContractSequencerAgentFixtureBuilder {
	// Flatten all transaction arrays into a single array
	for _, txList := range transactions {
		c.dispatchedTransactionLists = append(c.dispatchedTransactionLists, txList)
	}
	return c
}

type DelegatedTransaction struct {
	senderNode       string
	transactionAlias string
	transactionState TransactionState
}

func (c *ContractSequencerAgentFixtureBuilder) DelegatedTransactions(delegatedTransactions []DelegatedTransaction) *ContractSequencerAgentFixtureBuilder {
	c.delegatedTransactions = delegatedTransactions
	return c
}

func (c *ContractSequencerAgentFixtureBuilder) Build() *ContractSequencerAgentFixture {
	ctx := context.Background()

	nodeName := "defaultNodeName"
	if c.nodeName != "" {
		nodeName = c.nodeName
	}

	signer := tktypes.RandAddress()

	outboundMessageMonitor := newFakeTransportManager(c.t)

	contractAddress := tktypes.RandAddress()

	csa := NewContractSequencerAgent(context.Background(), nodeName, outboundMessageMonitor, c.coordinatorSelector, c.committee, contractAddress).(*contractSequencerAgent)
	// Set default configuration
	csa.blockRangeSize = 1000
	c.startingBlockRange = uint64(rand.Intn((int)(math.MaxInt64 / csa.blockRangeSize)))
	csa.currentBlockRange = c.startingBlockRange
	csa.heartbeatFailureThreshold = 5
	csa.activeCoordinator = c.activeCoordinator

	for i, coordinator := range c.coordinatorRankingForCurrentBlock {
		c.coordinatorSelector.coordinatorRank(c.startingBlockRange, i+1, coordinator)
	}

	for i, coordinator := range c.coordinatorRankingForNextBlock {
		c.coordinatorSelector.coordinatorRank(c.startingBlockRange+1, i+1, coordinator)
	}

	if c.isInObservingState {
		csa.activeCoordinator = c.activeCoordinator
	}
	if c.isInSenderState {
		pooledTransactionsFixture := c.pooledTransactions.Build()
		for _, transaction := range pooledTransactionsFixture.pooledTransactions {
			csa.delegationsByTransactionID[transaction.ID.String()] = &Delegation{
				Transaction: transaction,
			}
		}
		//there must be an active coordinator if we are in sender state so set it to a random value if not already provided
		if csa.activeCoordinator == "" {
			csa.activeCoordinator = uuid.New().String()
		}
	}
	if c.coordinatorState != "" {
		csa.coordinatorState = c.coordinatorState
	}

	for _, dt := range c.delegatedTransactions {
		switch dt.transactionState {
		case TransactionState_Assembled:
			//TODO add to graph
			fallthrough
		case TransactionState_Pooled:
			pooledTransaction := &PooledTransaction{}
			pooledTransaction.Transaction = &components.PrivateTransaction{}
			pooledTransaction.Transaction.ID = uuid.New()
			pooledTransaction.Sender = dt.senderNode
			csa.transactionPool.AddTransaction(ctx, pooledTransaction)
		case TransactionState_Dispatched:
			if csa.dispatchedTransactionsByCoordinator[nodeName] == nil {
				csa.dispatchedTransactionsByCoordinator[nodeName] = make([]*DispatchedTransaction, 0)
			}
			dispatchedTransaction := &DispatchedTransaction{}
			dispatchedTransaction.TransactionID = uuid.New()
			dispatchedTransaction.Signer = *signer

			csa.dispatchedTransactionsByCoordinator[nodeName] = append(csa.dispatchedTransactionsByCoordinator[nodeName], dispatchedTransaction)
		case TransactionState_Committed:
		case TransactionState_Rejected:
		case TransactionState_ConfirmedSuccess:
		case TransactionState_ConfirmedReverted:
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
		blockHeight:            c.blockHeight,
		startingBlockRange:     c.startingBlockRange,
		outboundMessageMonitor: outboundMessageMonitor,
		coordinatorSelector:    c.coordinatorSelector,
	}
}

// a  programmable implementation of the  CoordinatorSelector interface
// its behavior is controlled in a very simple way by defining a ranking for each coordinator in each block range
type coordinatorSelectorForTesting struct {
	t                *testing.T
	coordinatorRanks map[uint64]map[int]string
	eliminated       []string
	currentRank      int
	committeeSize    int
}

func newCoordinatorSelectorForTesting(t *testing.T) *coordinatorSelectorForTesting {
	return &coordinatorSelectorForTesting{
		t:                t,
		coordinatorRanks: make(map[uint64]map[int]string),
		currentRank:      1,
	}
}

func (c *coordinatorSelectorForTesting) Initialize(ctx context.Context, committee []CommitteeMember) error {
	c.currentRank = 1
	c.committeeSize = len(committee)
	return nil
}

func (c *coordinatorSelectorForTesting) Reset(ctx context.Context) error {
	c.currentRank = 1
	return nil
}

func (c *coordinatorSelectorForTesting) Eliminate(ctx context.Context, coordinator string) error {
	//move to the next ranked coordinator
	if c.currentRank < c.committeeSize {
		c.currentRank++
	} else {
		c.currentRank = 1
	}
	return nil
}

func (c *coordinatorSelectorForTesting) Select(ctx context.Context, blockRange uint64) (string, error) {
	if c.coordinatorRanks[blockRange] == nil {
		errorMessage := fmt.Sprintf("No coordinator ranks defined for block range %d", blockRange)
		c.t.Log(errorMessage)
		return "", errors.New(errorMessage)
	}

	coordinator := c.coordinatorRanks[blockRange][int(c.currentRank)]

	if coordinator == "" {
		errorMessage := fmt.Sprintf("No coordinator defined for rank %d in block range %d", c.currentRank, blockRange)
		c.t.Log(errorMessage)
		return "", errors.New(errorMessage)
	}

	return coordinator, nil

}

func (f *ContractSequencerAgentFixture) HeartbeatIntervalPassed() {

	f.csa.HandleMissedHeartbeat(f.ctx)
}

func (f *ContractSequencerAgentFixture) ExceedHeartbeatFailureThreshold() {

	for i := 0; i < f.csa.heartbeatFailureThreshold; i++ {
		f.csa.HandleMissedHeartbeat(f.ctx)
	}
}

func (f *ContractSequencerAgentFixture) AlmostExceedHeartbeatFailureThreshold() {

	for i := 0; i < f.csa.heartbeatFailureThreshold-1; i++ {
		f.csa.HandleMissedHeartbeat(f.ctx)
	}
}

func (f *ContractSequencerAgentFixture) MoveToNextBlockRange() {
	currentBlockRange := f.csa.currentBlockRange
	nextBlockRange := currentBlockRange + 1
	newBlockHeight := nextBlockRange * f.csa.blockRangeSize
	f.csa.HandleBlockHeightChange(f.ctx, newBlockHeight)
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

func (f *FixtureBuilder) PooledTransactionList() *PooledTransactionListFixtureBuilder {
	return &PooledTransactionListFixtureBuilder{
		FixtureBuilder: *f,
		coordinator:    uuid.New().String(),
	}
}

func (d *PooledTransactionListFixtureBuilder) Coordinator(coordinator string) *PooledTransactionListFixtureBuilder {
	d.coordinator = coordinator
	return d
}

func (d *PooledTransactionListFixtureBuilder) Length(length int) *PooledTransactionListFixtureBuilder {
	d.length = length
	return d
}

func (d *PooledTransactionListFixtureBuilder) Build() *PooledTransactionListFixture {
	pooledTransactions := make([]*components.PrivateTransaction, d.length)
	for i := 0; i < d.length; i++ {
		transaction := &components.PrivateTransaction{
			ID: uuid.New(),
		}
		pooledTransactions[i] = transaction
	}
	return &PooledTransactionListFixture{
		pooledTransactions: pooledTransactions,
		coordinator:        d.coordinator,
	}
}

func (f *FixtureBuilder) PrivateTransactionList() *PrivateTransactionListFixtureBuilder {
	return &PrivateTransactionListFixtureBuilder{
		FixtureBuilder: *f,
	}
}

func (d *PrivateTransactionListFixtureBuilder) Length(length int) *PrivateTransactionListFixtureBuilder {
	d.length = length
	return d
}

func (d *PrivateTransactionListFixtureBuilder) Build() *PrivateTransactionListFixture {
	privateTransactions := make([]*components.PrivateTransaction, d.length)
	for i := 0; i < d.length; i++ {
		transaction := &components.PrivateTransaction{
			ID: uuid.New(),
		}
		privateTransactions[i] = transaction
	}
	return &PrivateTransactionListFixture{
		privateTransactions: privateTransactions,
	}
}

type ReliableMessageMatcher func(message *components.ReliableMessage) bool
type FireAndForgetMessageMatcher func(message *components.FireAndForgetMessageSend) bool

type OutboundMessageMonitor interface {
	Sends(outboundMessageMatcher FireAndForgetMessageMatcher) bool
	SendsReliable(outboundMessageMatcher ReliableMessageMatcher) bool
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
	expectedPooledTransactions *PooledTransactionListFixture
	expectedTo                 string
}

func DelegationRequestMatcher() *DelegationRequestMatcherBuilder {
	return &DelegationRequestMatcherBuilder{}
}

func (d *DelegationRequestMatcherBuilder) Containing(pooledTransactions *PooledTransactionListFixture) *DelegationRequestMatcherBuilder {
	d.expectedPooledTransactions = pooledTransactions
	return d
}

func (d *DelegationRequestMatcherBuilder) To(expectedTo string) *DelegationRequestMatcherBuilder {
	d.expectedTo = expectedTo
	return d
}

func (d *DelegationRequestMatcherBuilder) Match() FireAndForgetMessageMatcher {
	return func(message *components.FireAndForgetMessageSend) bool {
		if message.MessageType != MessageType_DelegationRequest {
			return false
		}
		actualDelegationRequest, err := ParseDelegationRequest(message.Payload)

		if d.expectedPooledTransactions != nil {
			if err != nil {
				return false
			}
			if len(actualDelegationRequest.Transactions) != len(d.expectedPooledTransactions.pooledTransactions) {
				return false
			}
			actualTransactionIDs := make([]string, len(actualDelegationRequest.Transactions))
			for i, transaction := range actualDelegationRequest.Transactions {
				actualTransactionIDs[i] = transaction.ID.String()
			}
			for _, transaction := range d.expectedPooledTransactions.pooledTransactions {
				expectedTransactionID := transaction.ID.String()
				if !contains(actualTransactionIDs, expectedTransactionID) {
					return false
				}
			}
		}
		if d.expectedTo != "" {
			if message.Node != d.expectedTo {
				return false
			}
		}

		return true
	}
}

type HandoverRequestMatcherBuilder struct {
	expectedTo string
}

func HandoverRequestMatcher() *HandoverRequestMatcherBuilder {
	return &HandoverRequestMatcherBuilder{}
}

func (h *HandoverRequestMatcherBuilder) To(expectedTo string) *HandoverRequestMatcherBuilder {
	h.expectedTo = expectedTo
	return h
}

func (h *HandoverRequestMatcherBuilder) Match() FireAndForgetMessageMatcher {
	return func(message *components.FireAndForgetMessageSend) bool {
		if message.MessageType != MessageType_HandoverRequest {
			return false
		}
		if h.expectedTo != "" {
			if message.Node != h.expectedTo {
				return false
			}
		}
		return true
	}
}

type CoordinatorHeartbeatNotificationMatcherBuilder struct {
	expectedFrom                             string
	expectedContractAddress                  *tktypes.EthAddress
	expectedCoordinatorState                 CoordinatorState
	expectedTo                               string
	expectedPooledTransactionListMatcher     *PooledTransactionListMatcher
	expectedDispatchedTransactionListMatcher *DispatchedTransactionListMatcher
	expectedConfirmedTransactionListMatcher  *ConfirmedTransactionListMatcher
	expectedBlockHeight                      *uint64
}

func CoordinatorHeartbeatNotificationMatcher() *CoordinatorHeartbeatNotificationMatcherBuilder {
	return &CoordinatorHeartbeatNotificationMatcherBuilder{}
}

func (c *CoordinatorHeartbeatNotificationMatcherBuilder) From(expectedFrom string) *CoordinatorHeartbeatNotificationMatcherBuilder {
	c.expectedFrom = expectedFrom
	return c
}

func (c *CoordinatorHeartbeatNotificationMatcherBuilder) To(expectedTo string) *CoordinatorHeartbeatNotificationMatcherBuilder {
	c.expectedTo = expectedTo
	return c
}

func (c *CoordinatorHeartbeatNotificationMatcherBuilder) ContractAddress(expectedContractAddress *tktypes.EthAddress) *CoordinatorHeartbeatNotificationMatcherBuilder {
	c.expectedContractAddress = expectedContractAddress
	return c
}

func (c *CoordinatorHeartbeatNotificationMatcherBuilder) CoordinatorState(expectedCoordinatorState CoordinatorState) *CoordinatorHeartbeatNotificationMatcherBuilder {
	c.expectedCoordinatorState = expectedCoordinatorState
	return c
}

func (c *CoordinatorHeartbeatNotificationMatcherBuilder) BlockHeight(expectedBlockHeight uint64) *CoordinatorHeartbeatNotificationMatcherBuilder {
	c.expectedBlockHeight = &expectedBlockHeight
	return c
}

type PooledTransactionListMatcher struct {
	length *int
}

func (p *PooledTransactionListMatcher) Length(length int) *PooledTransactionListMatcher {
	p.length = &length
	return p
}

type PooledTransactionList []*PooledTransaction

func (p *PooledTransactionListMatcher) Match(actualPooledTransactions PooledTransactionList) bool {
	if p.length != nil {
		if len(actualPooledTransactions) != *p.length {
			return false
		}
	}
	return true
}

func (c *CoordinatorHeartbeatNotificationMatcherBuilder) PooledTransactionList(fn func(matcher *PooledTransactionListMatcher)) *CoordinatorHeartbeatNotificationMatcherBuilder {
	matcher := &PooledTransactionListMatcher{
		length: nil,
	}
	fn(matcher)
	c.expectedPooledTransactionListMatcher = matcher
	return c
}

type DispatchedTransactionListMatcher struct {
	length *int
}

func (p *DispatchedTransactionListMatcher) Length(length int) *DispatchedTransactionListMatcher {
	p.length = &length
	return p
}

type DispatchedTransactionList []*DispatchedTransaction

func (p *DispatchedTransactionListMatcher) Match(actualDispatchedTransactionList DispatchedTransactionList) bool {
	if p.length != nil {
		if len(actualDispatchedTransactionList) != *p.length {
			return false
		}
	}
	return true
}

func (c *CoordinatorHeartbeatNotificationMatcherBuilder) DispatchedTransactionList(fn func(matcher *DispatchedTransactionListMatcher)) *CoordinatorHeartbeatNotificationMatcherBuilder {
	matcher := &DispatchedTransactionListMatcher{
		length: nil,
	}
	fn(matcher)
	c.expectedDispatchedTransactionListMatcher = matcher
	return c
}

type ConfirmedTransactionListMatcher struct {
	length *int
}

func (p *ConfirmedTransactionListMatcher) Length(length int) *ConfirmedTransactionListMatcher {
	p.length = &length
	return p
}

type ConfirmedTransactionList []*ConfirmedTransaction

func (p *ConfirmedTransactionListMatcher) Match(actualConfirmedTransactionList ConfirmedTransactionList) bool {
	if p.length != nil {
		if len(actualConfirmedTransactionList) != *p.length {
			return false
		}
	}
	return true
}

func (c *CoordinatorHeartbeatNotificationMatcherBuilder) ConfirmedTransactionList(fn func(matcher *ConfirmedTransactionListMatcher)) *CoordinatorHeartbeatNotificationMatcherBuilder {
	matcher := &ConfirmedTransactionListMatcher{
		length: nil,
	}
	fn(matcher)
	c.expectedConfirmedTransactionListMatcher = matcher
	return c
}

func (c *CoordinatorHeartbeatNotificationMatcherBuilder) Match() FireAndForgetMessageMatcher {
	return func(message *components.FireAndForgetMessageSend) bool {
		if message.MessageType != MessageType_CoordinatorHeartbeatNotification {
			return false
		}
		actualCoordinatorHeartbeatNotification, err := ParseCoordinatorHeartbeatNotification(message.Payload)
		if err != nil {
			return false
		}

		if c.expectedFrom != "" {
			if actualCoordinatorHeartbeatNotification.From != c.expectedFrom {
				return false
			}
		}
		if c.expectedContractAddress != nil {
			if actualCoordinatorHeartbeatNotification.ContractAddress != c.expectedContractAddress {
				return false
			}
		}
		if c.expectedCoordinatorState != "" {
			if actualCoordinatorHeartbeatNotification.CoordinatorState != c.expectedCoordinatorState {
				return false
			}
		}

		if c.expectedPooledTransactionListMatcher != nil {
			if !c.expectedPooledTransactionListMatcher.Match(actualCoordinatorHeartbeatNotification.PooledTransactions) {
				return false
			}
		}

		if c.expectedDispatchedTransactionListMatcher != nil {
			if !c.expectedDispatchedTransactionListMatcher.Match(actualCoordinatorHeartbeatNotification.DispatchedTransactions) {
				return false
			}
		}

		if c.expectedTo != "" {
			if message.Node != c.expectedTo {
				return false
			}
		}

		if c.expectedBlockHeight != nil {
			if actualCoordinatorHeartbeatNotification.BlockHeight != *c.expectedBlockHeight {
				return false
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
