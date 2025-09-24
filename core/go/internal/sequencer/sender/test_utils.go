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

package sender

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/metrics"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/sender/transaction"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	uuid "github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	TestDefault_HeartbeatThreshold  int = 5
	TestDefault_HeartbeatIntervalMs int = 100
)

type SentMessageRecorder struct {
	transaction.SentMessageRecorder
	hasSentDelegationRequest bool
	delegatedTransactions    []*components.PrivateTransaction
}

func NewSentMessageRecorder() *SentMessageRecorder {
	return &SentMessageRecorder{}
}

func (r *SentMessageRecorder) SendDelegationRequest(ctx context.Context, coordinator string, transactions []*components.PrivateTransaction, sendersBlockHeight uint64) error {
	r.delegatedTransactions = transactions
	r.hasSentDelegationRequest = true
	return nil
}

func (r *SentMessageRecorder) HasSentDelegationRequest() bool {
	return r.hasSentDelegationRequest
}

func (r *SentMessageRecorder) HasDelegatedTransaction(txid uuid.UUID) bool {
	for _, tx := range r.delegatedTransactions {
		if tx.ID == txid {
			return true
		}
	}
	return false
}

func (r *SentMessageRecorder) Reset(ctx context.Context) {
	r.SentMessageRecorder.Reset(ctx)
	r.hasSentDelegationRequest = false
	r.delegatedTransactions = nil
}

type SenderBuilderForTesting struct {
	state            State
	nodeName         *string
	committeeMembers []string
	contractAddress  *pldtypes.EthAddress
	emitFunction     func(event common.Event)
	transactions     []*transaction.Transaction
	metrics          metrics.DistributedSequencerMetrics
}

type SenderDependencyMocks struct {
	SentMessageRecorder *SentMessageRecorder
	Clock               *common.FakeClockForTesting
	EngineIntegration   *common.FakeEngineIntegrationForTesting
	emittedEvents       []common.Event
}

func NewSenderBuilderForTesting(state State) *SenderBuilderForTesting {
	return &SenderBuilderForTesting{
		state:   state,
		metrics: metrics.InitMetrics(context.Background(), prometheus.NewRegistry()),
	}
}

func (b *SenderBuilderForTesting) ContractAddress(contractAddress *pldtypes.EthAddress) *SenderBuilderForTesting {
	b.contractAddress = contractAddress
	return b
}

func (b *SenderBuilderForTesting) NodeName(nodeName string) *SenderBuilderForTesting {
	b.nodeName = &nodeName
	return b
}

func (b *SenderBuilderForTesting) CommitteeMembers(committeeMembers ...string) *SenderBuilderForTesting {
	b.committeeMembers = committeeMembers
	return b
}

func (b *SenderBuilderForTesting) Transactions(transactions ...*transaction.Transaction) *SenderBuilderForTesting {
	b.transactions = transactions
	return b
}

func (b *SenderBuilderForTesting) GetContractAddress() pldtypes.EthAddress {
	return *b.contractAddress
}

func (b *SenderBuilderForTesting) GetCoordinatorHeartbeatThresholdMs() int {
	return TestDefault_HeartbeatThreshold * TestDefault_HeartbeatIntervalMs
}

func (b *SenderBuilderForTesting) Build(ctx context.Context) (*sender, *SenderDependencyMocks) {

	if b.nodeName == nil {
		b.nodeName = ptrTo("member1@node1")
	}

	if b.committeeMembers == nil {
		b.committeeMembers = []string{*b.nodeName}
	}

	if b.contractAddress == nil {
		b.contractAddress = pldtypes.RandAddress()
	}
	mocks := &SenderDependencyMocks{
		SentMessageRecorder: NewSentMessageRecorder(),
		Clock:               &common.FakeClockForTesting{},
		EngineIntegration:   &common.FakeEngineIntegrationForTesting{},
	}

	var sender *sender

	b.emitFunction = func(event common.Event) {
		mocks.emittedEvents = append(mocks.emittedEvents, event)
		_ = sender.ProcessEvent(ctx, event)
	}
	var err error
	sender, err = NewSender(
		ctx,
		*b.nodeName,
		mocks.SentMessageRecorder,
		mocks.Clock,
		b.emitFunction,
		mocks.EngineIntegration,
		100, // Block range size
		b.contractAddress,
		TestDefault_HeartbeatIntervalMs,
		TestDefault_HeartbeatThreshold,
		b.metrics,
	)

	for _, tx := range b.transactions {
		sender.transactionsByID[tx.ID] = tx
		sender.transactionsOrdered = append(sender.transactionsOrdered, &tx.ID)
		switch tx.GetCurrentState() {
		case transaction.State_Submitted:
			sender.submittedTransactionsByHash[*tx.GetLatestSubmissionHash()] = &tx.ID
		}
	}

	if err != nil {
		panic(err)
	}

	sender.stateMachine.currentState = b.state
	switch b.state {
	// Any state specific setup can be done here
	}

	return sender, mocks
}
