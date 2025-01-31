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
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	mock "github.com/stretchr/testify/mock"
	"gotest.tools/assert"
)

func TestStateMachineInitializeOK(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorForUnitTest(t, ctx)

	assert.Equal(t, State_Idle, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())
}

func TestStateMachineIdleToActiveOnTransactionsDelegated(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorForUnitTest(t, ctx)
	assert.Equal(t, State_Idle, c.stateMachine.currentState)

	c.HandleEvent(ctx, &TransactionsDelegatedEvent{
		Sender:       "sender",
		Transactions: newPrivateTransactionsForTesting(1),
	})

	assert.Equal(t, State_Active, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())

}

func TestStateMachineIdleToObservingOnHeartbeatReceived(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorForUnitTest(t, ctx)
	assert.Equal(t, State_Idle, c.stateMachine.currentState)

	c.HandleEvent(ctx, &HeartbeatReceivedEvent{})

	assert.Equal(t, State_Observing, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())

}

func TestStateMachineObservingToStandbyOnDelegatedIfBehind(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorForUnitTest(t, ctx)
	c.stateMachine.currentState = State_Observing
	c.currentBlockHeight = 194 // default tolerance is 5 in the test setup so this is behind
	c.activeCoordinatorBlockHeight = 200

	c.HandleEvent(ctx, &TransactionsDelegatedEvent{
		Sender:       "sender",
		Transactions: newPrivateTransactionsForTesting(1),
	})

	assert.Equal(t, State_Standby, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())
}

func TestStateMachineObservingToElectOnDelegatedIfNotBehind(t *testing.T) {
	ctx := context.Background()
	c, mocks := NewCoordinatorForUnitTest(t, ctx)
	mocks.messageSender.On("SendHandoverRequest", mock.Anything, "activeCoordinator", c.contractAddress).Return()
	c.stateMachine.currentState = State_Observing
	c.activeCoordinator = "activeCoordinator"
	c.currentBlockHeight = 195 // default tolerance is 5 in the test setup so we are not behind
	c.activeCoordinatorBlockHeight = 200

	c.HandleEvent(ctx, &TransactionsDelegatedEvent{
		Sender:       "sender",
		Transactions: newPrivateTransactionsForTesting(1),
	})

	assert.Equal(t, State_Elect, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())
}

func TestStateMachineStandbyToElectOnNewBlockIfNotBehind(t *testing.T) {
	ctx := context.Background()
	c, mocks := NewCoordinatorForUnitTest(t, ctx)
	mocks.messageSender.On("SendHandoverRequest", mock.Anything, "activeCoordinator", c.contractAddress).Return()
	c.stateMachine.currentState = State_Standby
	c.activeCoordinator = "activeCoordinator"
	c.currentBlockHeight = 194
	c.activeCoordinatorBlockHeight = 200

	c.HandleEvent(ctx, &NewBlockEvent{
		BlockHeight: 195, // default tolerance is 5 in the test setup so we are not behind
	})

	assert.Equal(t, State_Elect, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())
}

func TestStateMachineStandbyNotToElectOnNewBlockIfStillBehind(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorForUnitTest(t, ctx)
	c.stateMachine.currentState = State_Standby
	c.currentBlockHeight = 193
	c.activeCoordinatorBlockHeight = 200

	c.HandleEvent(ctx, &NewBlockEvent{
		BlockHeight: 194, // default tolerance is 5 in the test setup so this is still behind
	})

	assert.Equal(t, State_Standby, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())
}

func TestStateMachineElectToPreparedOnHandover(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorForUnitTest(t, ctx)
	c.stateMachine.currentState = State_Elect

	c.HandleEvent(ctx, &HandoverReceivedEvent{})

	assert.Equal(t, State_Prepared, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())
}

func TestStateMachinePreparedToActiveOnTransactionConfirmedIfFlushCompleted(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorForUnitTest(t, ctx)
	c.stateMachine.currentState = State_Prepared
	c.activeCoordinator = "activeCoordinator"
	flushPointTransactionID := uuid.New()
	flushPointHash := tktypes.Bytes32(tktypes.RandBytes(32))
	flushPointNonce := uint64(42)
	flushPointSignerAddress := tktypes.RandAddress()
	c.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
		fmt.Sprintf("%s:%d", flushPointSignerAddress.String(), flushPointNonce): {
			TransactionID: flushPointTransactionID,
			Hash:          flushPointHash,
			Nonce:         flushPointNonce,
			From:          *flushPointSignerAddress,
		},
	}

	c.HandleEvent(ctx, &TransactionConfirmedEvent{
		From:  flushPointSignerAddress,
		Nonce: flushPointNonce,
		Hash:  flushPointHash,
	})

	assert.Equal(t, State_Active, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())

	//TODO should have other test cases where there are multiple flush points across multiple signers ( and across multiple coordinators?)
	//TODO test case where the nonce and signer match but hash does not.  This should still trigger the transition because there will never be another confirmed transaction for that nonce and signer

}

func TestStateMachinePreparedNoTransitionOnTransactionConfirmedIfNotFlushCompleted(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorForUnitTest(t, ctx)
	c.stateMachine.currentState = State_Prepared
	c.activeCoordinator = "activeCoordinator"
	flushPointTransactionID := uuid.New()
	flushPointHash := tktypes.Bytes32(tktypes.RandBytes(32))
	flushPointNonce := uint64(42)
	flushPointSignerAddress := tktypes.RandAddress()

	otherHash := tktypes.Bytes32(tktypes.RandBytes(32))
	otherNonce := uint64(41)

	c.activeCoordinatorsFlushPointsBySignerNonce = map[string]*common.FlushPoint{
		fmt.Sprintf("%s:%d", flushPointSignerAddress.String(), flushPointNonce): {
			TransactionID: flushPointTransactionID,
			Hash:          flushPointHash,
			Nonce:         flushPointNonce,
			From:          *flushPointSignerAddress,
		},
	}

	c.HandleEvent(ctx, &TransactionConfirmedEvent{
		From:  flushPointSignerAddress,
		Nonce: otherNonce,
		Hash:  otherHash,
	})

	assert.Equal(t, State_Prepared, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())

	//TODO should have other test cases where there are multiple flush points across multiple signers ( and across multiple coordinators?)

}

func TestStateMachineActiveToIdleOnTransactionConfirmedIfNoTransactionsInFlight(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorForUnitTest(t, ctx)
	c.stateMachine.currentState = State_Active

	soleTransaction := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		soleTransaction.ID: soleTransaction,
	}

	c.HandleEvent(ctx, &TransactionConfirmedEvent{
		From:  soleTransaction.GetSignerAddress(),
		Nonce: *soleTransaction.GetNonce(),
		Hash:  *soleTransaction.GetLatestSubmissionHash(),
	})

	assert.Equal(t, State_Idle, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())

}

func TestStateMachineActiveNoTransitionOnTransactionConfirmedIfNotTransactionsEmpty(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorForUnitTest(t, ctx)
	c.stateMachine.currentState = State_Active

	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		delegation1.ID: delegation1,
		delegation2.ID: delegation2,
	}

	c.HandleEvent(ctx, &TransactionConfirmedEvent{
		From:  delegation1.GetSignerAddress(),
		Nonce: *delegation1.GetNonce(),
		Hash:  *delegation1.GetLatestSubmissionHash(),
	})

	assert.Equal(t, State_Active, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())
}

func TestStateMachineActiveToFlushOnHandoverRequest(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorForUnitTest(t, ctx)
	c.stateMachine.currentState = State_Active

	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		delegation1.ID: delegation1,
		delegation2.ID: delegation2,
	}

	c.HandleEvent(ctx, &HandoverRequestEvent{
		Requester: "newCoordinator",
	})

	assert.Equal(t, State_Flush, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())

}

func TestStateMachineFlushToClosingOnTransactionConfirmedIfFlushComplete(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorForUnitTest(t, ctx)
	c.stateMachine.currentState = State_Flush

	//We have 2 transactions in flight but only of them has passed the point of no return so we
	// should consider the flush complete when that one is confirmed
	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Endorsed).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		delegation1.ID: delegation1,
		delegation2.ID: delegation2,
	}

	c.HandleEvent(ctx, &TransactionConfirmedEvent{
		From:  delegation1.GetSignerAddress(),
		Nonce: *delegation1.GetNonce(),
		Hash:  *delegation1.GetLatestSubmissionHash(),
	})

	assert.Equal(t, State_Closing, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())

}

func TestStateMachineFlushNoTransitionOnTransactionConfirmedIfNotFlushComplete(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorForUnitTest(t, ctx)
	c.stateMachine.currentState = State_Flush

	//We have 2 transactions in flight and passed the point of no return but only one of them will be confirmed so we should not
	// consider the flush complete

	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		delegation1.ID: delegation1,
		delegation2.ID: delegation2,
	}

	c.HandleEvent(ctx, &TransactionConfirmedEvent{
		From:  delegation1.GetSignerAddress(),
		Nonce: *delegation1.GetNonce(),
		Hash:  *delegation1.GetLatestSubmissionHash(),
	})

	assert.Equal(t, State_Flush, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())

}

func TestStateMachineClosingToIdleOnHeartbeatIntervalIfClosingGracePeriodExpired(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorForUnitTest(t, ctx)
	c.stateMachine.currentState = State_Closing

	d := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		d.ID: d,
	}
	//one heartbeat interval away from the grace period expiring
	c.heartbeatIntervalsSinceStateChange = 4

	c.HandleEvent(ctx, &HeartbeatIntervalEvent{})

	assert.Equal(t, State_Idle, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())

}

func TestStateMachineClosingNoTransitionOnHeartbeatIntervalIfNotClosingGracePeriodExpired(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorForUnitTest(t, ctx)
	c.stateMachine.currentState = State_Closing

	d := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	c.transactionsByID = map[uuid.UUID]*transaction.Transaction{
		d.ID: d,
	}

	//two heartbeat intervals away from the grace period expiring
	c.heartbeatIntervalsSinceStateChange = 3

	c.HandleEvent(ctx, &HeartbeatIntervalEvent{})

	assert.Equal(t, State_Closing, c.stateMachine.currentState, "current state is %s", c.stateMachine.currentState.String())

}

func newPrivateTransactionsForTesting(num int) []*components.PrivateTransaction {
	txs := make([]*components.PrivateTransaction, num)
	for i := 0; i < num; i++ {
		txs[i] = &components.PrivateTransaction{}
	}
	return txs
}

func newPrivateTransactionForTesting() *components.PrivateTransaction {
	return &components.PrivateTransaction{
		ID: uuid.New(),
	}
}

type coordinatorDependencyMocks struct {
	messageSender *MockMessageSender
}

func NewCoordinatorForUnitTest(t *testing.T, ctx context.Context) (*coordinator, *coordinatorDependencyMocks) {

	mocks := &coordinatorDependencyMocks{
		messageSender: NewMockMessageSender(t),
	}

	coordinator := NewCoordinator(ctx, mocks.messageSender, 100, tktypes.RandAddress(), 5, 5)
	return coordinator, mocks
}
