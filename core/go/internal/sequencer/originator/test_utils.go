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

package originator

import (
	"context"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
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

func (r *SentMessageRecorder) SendDelegationRequest(ctx context.Context, coordinator string, transactions []*components.PrivateTransaction, originatorsBlockHeight uint64) error {
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

type OriginatorBuilderForTesting struct {
	state            State
	nodeName         *string
	committeeMembers []string
	contractAddress  *pldtypes.EthAddress
	transactions     []*transaction.Transaction
	metrics          metrics.DistributedSequencerMetrics
}

type OriginatorDependencyMocks struct {
	SentMessageRecorder *SentMessageRecorder
	Clock               *common.FakeClockForTesting
	EngineIntegration   *common.FakeEngineIntegrationForTesting
}

func NewOriginatorBuilderForTesting(state State) *OriginatorBuilderForTesting {
	return &OriginatorBuilderForTesting{
		state:   state,
		metrics: metrics.InitMetrics(context.Background(), prometheus.NewRegistry()),
	}
}

func (b *OriginatorBuilderForTesting) ContractAddress(contractAddress *pldtypes.EthAddress) *OriginatorBuilderForTesting {
	b.contractAddress = contractAddress
	return b
}

func (b *OriginatorBuilderForTesting) NodeName(nodeName string) *OriginatorBuilderForTesting {
	b.nodeName = &nodeName
	return b
}

func (b *OriginatorBuilderForTesting) CommitteeMembers(committeeMembers ...string) *OriginatorBuilderForTesting {
	b.committeeMembers = committeeMembers
	return b
}

func (b *OriginatorBuilderForTesting) Transactions(transactions ...*transaction.Transaction) *OriginatorBuilderForTesting {
	b.transactions = transactions
	return b
}

func (b *OriginatorBuilderForTesting) GetContractAddress() pldtypes.EthAddress {
	return *b.contractAddress
}

func (b *OriginatorBuilderForTesting) GetCoordinatorHeartbeatThresholdMs() int {
	return TestDefault_HeartbeatThreshold * TestDefault_HeartbeatIntervalMs
}

func (b *OriginatorBuilderForTesting) Build(ctx context.Context) (*originator, *OriginatorDependencyMocks) {

	if b.nodeName == nil {
		b.nodeName = ptrTo("member1@node1")
	}

	if b.committeeMembers == nil {
		b.committeeMembers = []string{*b.nodeName}
	}

	if b.contractAddress == nil {
		b.contractAddress = pldtypes.RandAddress()
	}
	mocks := &OriginatorDependencyMocks{
		SentMessageRecorder: NewSentMessageRecorder(),
		Clock:               &common.FakeClockForTesting{},
		EngineIntegration:   &common.FakeEngineIntegrationForTesting{},
	}

	var originator *originator

	var err error
	originator, err = NewOriginator(
		ctx,
		*b.nodeName,
		mocks.SentMessageRecorder,
		mocks.Clock,
		mocks.EngineIntegration,
		100, // Block range size
		b.contractAddress,
		TestDefault_HeartbeatIntervalMs,
		TestDefault_HeartbeatThreshold,
		b.metrics,
	)

	for _, tx := range b.transactions {
		originator.transactionsByID[tx.ID] = tx
		originator.transactionsOrdered = append(originator.transactionsOrdered, &tx.ID)
		switch tx.GetCurrentState() {
		case transaction.State_Submitted:
			originator.submittedTransactionsByHash[*tx.GetLatestSubmissionHash()] = &tx.ID
		}
	}

	if err != nil {
		panic(err)
	}

	originator.stateMachine.currentState = b.state
	switch b.state {
	// Any state specific setup can be done here
	}

	err = originator.SetActiveCoordinator(ctx, "coordinator")
	if err != nil {
		return nil, nil
	}

	return originator, mocks
}
