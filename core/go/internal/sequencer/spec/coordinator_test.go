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

package spec

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

func TestCoordinator_InitializeOK(t *testing.T) {
	ctx := context.Background()

	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Idle).Build(ctx)

	assert.Equal(t, coordinator.State_Idle, c.GetCurrentState(), "current state is %s", c.GetCurrentState())
}

func TestCoordinator_Idle_ToActive_OnTransactionsDelegated(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"

	builder := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Idle)
	builder.OriginatorIdentityPool(originator)
	builder.GetDomainAPI().On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})
	c, _ := builder.Build(ctx)

	assert.Equal(t, coordinator.State_Idle, c.GetCurrentState())

	err := c.ProcessEvent(ctx, &coordinator.TransactionsDelegatedEvent{
		Originator:   originator,
		Transactions: testutil.NewPrivateTransactionBuilderListForTesting(1).Address(builder.GetContractAddress()).BuildSparse(),
	})
	assert.NoError(t, err)

	assert.Equal(t, coordinator.State_Active, c.GetCurrentState(), "current state is %s", c.GetCurrentState())

}

func TestCoordinator_Idle_ToObserving_OnHeartbeatReceived(t *testing.T) {
	ctx := context.Background()
	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Idle).Build(ctx)
	assert.Equal(t, coordinator.State_Idle, c.GetCurrentState())

	err := c.ProcessEvent(ctx, &coordinator.HeartbeatReceivedEvent{})
	assert.NoError(t, err)
	assert.Equal(t, coordinator.State_Observing, c.GetCurrentState(), "current state is %s", c.GetCurrentState())

}

func TestCoordinator_Observing_ToStandby_OnDelegated_IfBehind(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"

	builder := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Observing).
		OriginatorIdentityPool(originator).
		ActiveCoordinatorBlockHeight(200).
		CurrentBlockHeight(194) // default tolerance is 5 so this is behind
	c, _ := builder.Build(ctx)

	err := c.ProcessEvent(ctx, &coordinator.TransactionsDelegatedEvent{
		Originator:   originator,
		Transactions: testutil.NewPrivateTransactionBuilderListForTesting(1).Address(builder.GetContractAddress()).BuildSparse(),
	})
	assert.NoError(t, err)

	assert.Equal(t, coordinator.State_Standby, c.GetCurrentState(), "current state is %s", c.GetCurrentState())
}

func TestCoordinator_Observing_ToElect_OnDelegated_IfNotBehind(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"

	builder := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Observing).
		OriginatorIdentityPool(originator).
		ActiveCoordinatorBlockHeight(200).
		CurrentBlockHeight(195) // default tolerance is 5 so this is not behind
	c, mocks := builder.Build(ctx)

	err := c.ProcessEvent(ctx, &coordinator.TransactionsDelegatedEvent{
		Originator:   originator,
		Transactions: testutil.NewPrivateTransactionBuilderListForTesting(1).Address(builder.GetContractAddress()).BuildSparse(),
	})
	assert.NoError(t, err)

	assert.Equal(t, coordinator.State_Elect, c.GetCurrentState(), "current state is %s", c.GetCurrentState())
	assert.True(t, mocks.SentMessageRecorder.HasSentHandoverRequest(), "expected handover request to be sent")

}

func TestCoordinator_Standby_ToElect_OnNewBlock_IfNotBehind(t *testing.T) {
	ctx := context.Background()
	originator := "sender@senderNode"
	builder := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Standby).
		OriginatorIdentityPool(originator).
		ActiveCoordinatorBlockHeight(200).
		CurrentBlockHeight(194)
	c, _ := builder.Build(ctx)

	err := c.ProcessEvent(ctx, &coordinator.NewBlockEvent{
		BlockHeight: 195, // default tolerance is 5 in the test setup so we are not behind
	})
	assert.NoError(t, err)

	assert.Equal(t, coordinator.State_Elect, c.GetCurrentState(), "current state is %s", c.GetCurrentState())
}

func TestCoordinator_Standby_NoTransition_OnNewBlock_IfStillBehind(t *testing.T) {
	ctx := context.Background()

	builder := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Standby).
		ActiveCoordinatorBlockHeight(200).
		CurrentBlockHeight(193)
	c, mocks := builder.Build(ctx)

	err := c.ProcessEvent(ctx, &coordinator.NewBlockEvent{
		BlockHeight: 194, // default tolerance is 5 in the test setup so this is still behind
	})
	assert.NoError(t, err)

	assert.Equal(t, coordinator.State_Standby, c.GetCurrentState(), "current state is %s", c.GetCurrentState())
	assert.False(t, mocks.SentMessageRecorder.HasSentHandoverRequest(), "handover request not expected to be sent")
}

func TestCoordinator_Elect_ToPrepared_OnHandover(t *testing.T) {
	ctx := context.Background()
	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Elect).Build(ctx)

	err := c.ProcessEvent(ctx, &coordinator.HandoverReceivedEvent{})
	assert.NoError(t, err)

	assert.Equal(t, coordinator.State_Prepared, c.GetCurrentState(), "current state is %s", c.GetCurrentState())
}

func TestCoordinator_Prepared_ToActive_OnTransactionConfirmed_IfFlushCompleted(t *testing.T) {
	ctx := context.Background()
	builder := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Prepared)
	c, _ := builder.Build(ctx)

	domainAPI := builder.GetDomainAPI()
	domainAPI.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
	})

	err := c.ProcessEvent(ctx, &coordinator.TransactionConfirmedEvent{
		From:  builder.GetFlushPointSignerAddress(),
		Nonce: builder.GetFlushPointNonce(),
		Hash:  builder.GetFlushPointHash(),
	})
	assert.NoError(t, err)

	assert.Equal(t, coordinator.State_Active, c.GetCurrentState(), "current state is %s", c.GetCurrentState())

	//TODO should have other test cases where there are multiple flush points across multiple signers ( and across multiple coordinators?)
	//TODO test case where the nonce and signer match but hash does not.  This should still trigger the transition because there will never be another confirmed transaction for that nonce and signer

}

func TestCoordinator_PreparedNoTransition_OnTransactionConfirmed_IfNotFlushCompleted(t *testing.T) {
	ctx := context.Background()

	builder := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Prepared)
	c, _ := builder.Build(ctx)

	otherHash := pldtypes.Bytes32(pldtypes.RandBytes(32))
	otherNonce := builder.GetFlushPointNonce() - 1

	err := c.ProcessEvent(ctx, &coordinator.TransactionConfirmedEvent{
		From:  builder.GetFlushPointSignerAddress(),
		Nonce: otherNonce,
		Hash:  otherHash,
	})
	assert.NoError(t, err)

	assert.Equal(t, coordinator.State_Prepared, c.GetCurrentState(), "current state is %s", c.GetCurrentState())

}

func TestCoordinator_Active_ToIdle_OnTransactionConfirmed_IfNoTransactionsInFlight(t *testing.T) {
	ctx := context.Background()

	soleTransaction := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Active).
		Transactions(soleTransaction).
		Build(ctx)

	err := c.ProcessEvent(ctx, &coordinator.TransactionConfirmedEvent{
		From:  soleTransaction.GetSignerAddress(),
		Nonce: *soleTransaction.GetNonce(),
		Hash:  *soleTransaction.GetLatestSubmissionHash(),
	})
	assert.NoError(t, err)
	assert.Equal(t, coordinator.State_Idle, c.GetCurrentState(), "current state is %s", c.GetCurrentState())

}

func TestCoordinator_ActiveNoTransition_OnTransactionConfirmed_IfNotTransactionsEmpty(t *testing.T) {
	ctx := context.Background()

	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Active).
		Transactions(delegation1, delegation2).
		Build(ctx)

	err := c.ProcessEvent(ctx, &coordinator.TransactionConfirmedEvent{
		From:  delegation1.GetSignerAddress(),
		Nonce: *delegation1.GetNonce(),
		Hash:  *delegation1.GetLatestSubmissionHash(),
	})
	assert.NoError(t, err)

	assert.Equal(t, coordinator.State_Active, c.GetCurrentState(), "current state is %s", c.GetCurrentState())
}

func TestCoordinator_Active_ToFlush_OnHandoverRequest(t *testing.T) {
	ctx := context.Background()

	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Active).
		Transactions(delegation1, delegation2).
		Build(ctx)

	err := c.ProcessEvent(ctx, &coordinator.HandoverRequestEvent{
		Requester: "newCoordinator",
	})
	assert.NoError(t, err)

	assert.Equal(t, coordinator.State_Flush, c.GetCurrentState(), "current state is %s", c.GetCurrentState())

}

func TestCoordinator_Flush_ToClosing_OnTransactionConfirmed_IfFlushComplete(t *testing.T) {
	ctx := context.Background()

	//We have 2 transactions in flight but only one of them has passed the point of no return so we
	// should consider the flush complete when that one is confirmed
	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Confirming_Dispatchable).Build()

	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Flush).
		Transactions(delegation1, delegation2).
		Build(ctx)

	err := c.ProcessEvent(ctx, &coordinator.TransactionConfirmedEvent{
		From:  delegation1.GetSignerAddress(),
		Nonce: *delegation1.GetNonce(),
		Hash:  *delegation1.GetLatestSubmissionHash(),
	})
	assert.NoError(t, err)

	assert.Equal(t, coordinator.State_Closing, c.GetCurrentState(), "current state is %s", c.GetCurrentState())

}

func TestCoordinator_FlushNoTransition_OnTransactionConfirmed_IfNotFlushComplete(t *testing.T) {
	ctx := context.Background()

	//We have 2 transactions in flight and passed the point of no return but only one of them will be confirmed so we should not
	// consider the flush complete

	delegation1 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()
	delegation2 := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Flush).
		Transactions(delegation1, delegation2).
		Build(ctx)

	err := c.ProcessEvent(ctx, &coordinator.TransactionConfirmedEvent{
		From:  delegation1.GetSignerAddress(),
		Nonce: *delegation1.GetNonce(),
		Hash:  *delegation1.GetLatestSubmissionHash(),
	})
	assert.NoError(t, err)

	assert.Equal(t, coordinator.State_Flush, c.GetCurrentState(), "current state is %s", c.GetCurrentState())

}

func TestCoordinator_Closing_ToIdle_OnHeartbeatInterval_IfClosingGracePeriodExpired(t *testing.T) {
	ctx := context.Background()

	d := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Closing).
		HeartbeatsUntilClosingGracePeriodExpires(1).
		Transactions(d).
		Build(ctx)

	err := c.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{})
	assert.NoError(t, err)

	assert.Equal(t, coordinator.State_Idle, c.GetCurrentState(), "current state is %s", c.GetCurrentState())

}

func TestCoordinator_ClosingNoTransition_OnHeartbeatInterval_IfNotClosingGracePeriodExpired(t *testing.T) {
	ctx := context.Background()

	d := transaction.NewTransactionBuilderForTesting(t, transaction.State_Submitted).Build()

	c, _ := coordinator.NewCoordinatorBuilderForTesting(t, coordinator.State_Closing).
		HeartbeatsUntilClosingGracePeriodExpires(2).
		Transactions(d).
		Build(ctx)

	err := c.ProcessEvent(ctx, &common.HeartbeatIntervalEvent{})
	assert.NoError(t, err)

	assert.Equal(t, coordinator.State_Closing, c.GetCurrentState(), "current state is %s", c.GetCurrentState())

}
