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

package coordinator

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

func privateTransactionMatcher(txID ...uuid.UUID) func(*components.PrivateTransaction) bool {
	return func(tx *components.PrivateTransaction) bool {
		for _, id := range txID {
			if tx.ID == id {
				return true
			}
		}
		return false
	}
}

type SentMessageRecorder struct {
	transaction.SentMessageRecorder
	hasSentHandoverRequest bool
	hasSentHeartbeat       bool
}

func NewSentMessageRecorder() *SentMessageRecorder {
	return &SentMessageRecorder{
		SentMessageRecorder: *transaction.NewSentMessageRecorder(),
	}
}

func (r *SentMessageRecorder) Reset(ctx context.Context) {
	r.hasSentHandoverRequest = false
	r.hasSentHeartbeat = false
	r.SentMessageRecorder.Reset(ctx)
}

func (r *SentMessageRecorder) SendHandoverRequest(ctx context.Context, activeCoordinator string, contractAddress *tktypes.EthAddress) {
	r.hasSentHandoverRequest = true
}

func (r *SentMessageRecorder) HasSentHandoverRequest() bool {
	return r.hasSentHandoverRequest
}

func (r *SentMessageRecorder) SendHeartbeat(ctx context.Context, coordinatorSnapshop *common.CoordinatorSnapshot) {
	r.hasSentHeartbeat = true
}

func (r *SentMessageRecorder) HasSentHeartbeat() bool {
	return r.hasSentHeartbeat
}

type CoordinatorBuilderForTesting struct {
	state                                    State
	committeeMembers                         []string
	contractAddress                          *tktypes.EthAddress
	currentBlockHeight                       *uint64
	activeCoordinatorBlockHeight             *uint64
	activeCoordinator                        *string
	flushPointTransactionID                  *uuid.UUID
	flushPointHash                           *tktypes.Bytes32
	flushPointNonce                          *uint64
	flushPointSignerAddress                  *tktypes.EthAddress
	emitFunction                             func(event common.Event)
	transactions                             []*transaction.Transaction
	heartbeatsUntilClosingGracePeriodExpires *int
}

type CoordinatorDependencyMocks struct {
	SentMessageRecorder *SentMessageRecorder
	Clock               *common.FakeClockForTesting
	EngineIntegration   *common.FakeEngineIntegrationForTesting
	emittedEvents       []common.Event
}

func NewCoordinatorBuilderForTesting(state State) *CoordinatorBuilderForTesting {
	return &CoordinatorBuilderForTesting{
		state: state,
	}
}

func (b *CoordinatorBuilderForTesting) CommitteeMembers(committeeMembers ...string) *CoordinatorBuilderForTesting {
	b.committeeMembers = committeeMembers
	return b
}

func (b *CoordinatorBuilderForTesting) ContractAddress(contractAddress *tktypes.EthAddress) *CoordinatorBuilderForTesting {
	b.contractAddress = contractAddress
	return b
}

func (b *CoordinatorBuilderForTesting) GetContractAddress() tktypes.EthAddress {
	return *b.contractAddress
}

func (b *CoordinatorBuilderForTesting) CurrentBlockHeight(currentBlockHeight uint64) *CoordinatorBuilderForTesting {
	b.currentBlockHeight = &currentBlockHeight
	return b
}

func (b *CoordinatorBuilderForTesting) ActiveCoordinatorBlockHeight(activeCoordinatorBlockHeight uint64) *CoordinatorBuilderForTesting {
	b.activeCoordinatorBlockHeight = &activeCoordinatorBlockHeight
	return b
}

func (b *CoordinatorBuilderForTesting) Transactions(transactions ...*transaction.Transaction) *CoordinatorBuilderForTesting {
	b.transactions = transactions
	return b
}

func (b *CoordinatorBuilderForTesting) HeartbeatsUntilClosingGracePeriodExpires(heartbeatsUntilClosingGracePeriodExpires int) *CoordinatorBuilderForTesting {
	b.heartbeatsUntilClosingGracePeriodExpires = &heartbeatsUntilClosingGracePeriodExpires
	return b
}

func (b *CoordinatorBuilderForTesting) GetFlushPointNonce() uint64 {
	return *b.flushPointNonce
}

func (b *CoordinatorBuilderForTesting) GetFlushPointSignerAddress() *tktypes.EthAddress {
	return b.flushPointSignerAddress
}

func (b *CoordinatorBuilderForTesting) GetFlushPointHash() tktypes.Bytes32 {
	return *b.flushPointHash
}

func (b *CoordinatorBuilderForTesting) Build(ctx context.Context) (*coordinator, *CoordinatorDependencyMocks) {

	if b.committeeMembers == nil {
		b.committeeMembers = []string{"member1@node1"}
	}
	if b.contractAddress == nil {
		b.contractAddress = tktypes.RandAddress()
	}
	mocks := &CoordinatorDependencyMocks{
		SentMessageRecorder: NewSentMessageRecorder(),
		Clock:               &common.FakeClockForTesting{},
		EngineIntegration:   &common.FakeEngineIntegrationForTesting{},
	}

	b.emitFunction = func(event common.Event) {
		mocks.emittedEvents = append(mocks.emittedEvents, event)
	}

	coordinator, err := NewCoordinator(
		ctx,
		mocks.SentMessageRecorder,
		b.committeeMembers,
		mocks.Clock,
		b.emitFunction,
		mocks.EngineIntegration,
		mocks.Clock.Duration(1000), // Request timeout
		mocks.Clock.Duration(5000), // Assemble timeout
		100,                        // Block range size
		b.contractAddress,          // Contract address,
		5,                          // Block height tolerance
		5,                          // Closing grace period (measured in number of heartbeat intervals)
	)
	if err != nil {
		panic(err)
	}

	for _, tx := range b.transactions {
		coordinator.transactionsByID[tx.ID] = tx
	}

	coordinator.stateMachine.currentState = b.state
	switch b.state {
	case State_Observing:
		fallthrough
	case State_Standby:
		fallthrough
	case State_Elect:
		if b.currentBlockHeight == nil {
			b.currentBlockHeight = ptrTo(uint64(0))
		}
		if b.activeCoordinatorBlockHeight == nil {
			b.activeCoordinatorBlockHeight = ptrTo(uint64(0))
		}

		if b.activeCoordinator == nil {
			b.activeCoordinator = ptrTo("activeCoordinator")
		}

		coordinator.currentBlockHeight = *b.currentBlockHeight
		coordinator.activeCoordinatorBlockHeight = *b.activeCoordinatorBlockHeight
		coordinator.activeCoordinator = *b.activeCoordinator
	case State_Prepared:
		if b.flushPointTransactionID == nil {
			b.flushPointTransactionID = ptrTo(uuid.New())
		}
		if b.flushPointHash == nil {
			b.flushPointHash = ptrTo(tktypes.Bytes32(tktypes.RandBytes(32)))
		}
		if b.flushPointNonce == nil {
			b.flushPointNonce = ptrTo(uint64(42))
		}
		if b.flushPointSignerAddress == nil {
			b.flushPointSignerAddress = tktypes.RandAddress()
		}

		coordinator.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
			fmt.Sprintf("%s:%d", b.flushPointSignerAddress.String(), *b.flushPointNonce): {
				TransactionID: *b.flushPointTransactionID,
				Hash:          *b.flushPointHash,
				Nonce:         *b.flushPointNonce,
				From:          *b.flushPointSignerAddress,
			},
		}
	case State_Closing:
		if b.heartbeatsUntilClosingGracePeriodExpires == nil {
			b.heartbeatsUntilClosingGracePeriodExpires = ptrTo(5)
		}
		coordinator.heartbeatIntervalsSinceStateChange = 5 - *b.heartbeatsUntilClosingGracePeriodExpires

	}

	return coordinator, mocks
}
