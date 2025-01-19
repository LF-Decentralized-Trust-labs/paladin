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
This file contains all test that assert spec compliance.  There is one test for each `Rule` defined in the spec.
*/

package sequencer

import (
	"testing"

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRule1_1(t *testing.T) {
	// TODO how do we test this?  If a CSA object exists, it is by definition in memory and not idle
	// need to add  ContractSequencerAgentManager to manage the lifecycle of the CSA and test this rul
	// `GIVEN` a`ContractSequencerAgent` is in `Idle` state (i.e. not loaded into memory)
	// `WHEN` a `CoordinatorHeartbeatNotification` heartbeat message is received from any node
	// `THEN` the `ContractSequencerAgent` is in `Observing` state
	// `AND` `ActiveCoordinator` is equal to the sender of the given `CoordinatorHeartbeatNotification`
	t.Skip("Not implemented")
}

func TestRule1_2(t *testing.T) {
	// GIVEN a contract is in Observing state
	fixture := Given(t).
		ContractSequencerAgent().
		InObservingState().
		Build()

	// WHEN a CoordinatorHeartbeatNotification heartbeat message is received from any node
	testCoordinatorNode := "testCoordinatorNode"
	err := fixture.csa.HandleCoordinatorHeartbeatNotification(fixture.ctx, &CoordinatorHeartbeatNotification{
		From: testCoordinatorNode,
	})
	require.NoError(t, err)

	// THEN the ActiveCoordinator is equal to the sender of the given CoordinatorHeartbeatNotification
	assert.Equal(t, testCoordinatorNode, fixture.csa.ActiveCoordinator())
}

func TestRule1_3(t *testing.T) {
	/*
		   	`GIVEN` a`ContractSequencerAgent` is in `Observing` state
				`AND` `ActiveCoordinator` is not empty
		   	`WHEN` the sender does not receive any `CoordinatorHeartbeatNotification` message for a period of `CoordinatorHeartbeatFailureThreshold`
		   	`THEN` the `ActiveCoordinator` is empty
	*/

	testCoordinatorNode := "testCoordinatorNode"

	fixture := Given(t).
		ContractSequencerAgent().
		InObservingState().
		ActiveCoordinator(testCoordinatorNode).
		Build()

	fixture.ExceedHeartbeatFailureThreshold()

	assert.True(t, ActiveCoordinatorBecomesEmpty(t, fixture.ctx, fixture.csa))

}

func TestRule1_4(t *testing.T) {
	/*
		`GIVEN` a`ContractSequencerAgent` is in `Sending` state
		`WHEN` a `CoordinatorHeartbeatNotification` heartbeat message is received from any node other than the current `ActiveCoordinator`
		`THEN` the `ActiveCoordinator` is equal to the sender of the given `CoordinatorHeartbeatNotification`
			`AND` a `DelegationRequest` message for all transactions in `DELEGATED` state is sent to the new `ActiveCoordinator`
	*/
	testCoordinatorNode1 := "testCoordinatorNode1"
	testCoordinatorNode2 := "testCoordinatorNode2"

	fixture := Given(t).
		ContractSequencerAgent().
		InSenderState().
		PooledTransactions(
			func(builder *PooledTransactionListFixtureBuilder) {
				builder.
					Length(2)
			}).
		ActiveCoordinator(testCoordinatorNode1).
		Build()

	err := fixture.csa.HandleCoordinatorHeartbeatNotification(fixture.ctx, &CoordinatorHeartbeatNotification{
		From: testCoordinatorNode2,
	})
	require.NoError(t, err)

	assert.True(t, ActiveCoordinatorBecomesEqualTo(t, fixture.ctx, fixture.csa, testCoordinatorNode2))
	assert.True(
		t,
		fixture.outboundMessageMonitor.Sends(
			FireAndForgetMessageMatcher(DelegationRequestMatcher().
				Containing(fixture.pooledTransactions).Match(),
			),
		),
	)
}

func TestRule2_1(t *testing.T) {
	/*
	   `GIVEN` a`ContractSequencerAgent` is in `Observing` state
	   `WHEN` a heartbeat message is received that contains any transactions which have a signing address and nonce number
	   `THEN` those transactions replace the list of `DispatchedTransactions` associated with that `Coordinator/Submitter`
	*/

	fixture := Given(t).
		ContractSequencerAgent().
		InObservingState().
		Build()

	dispatchedTransactions := Given(t).DispatchedTransactionList().Length(3).Build().dispatchedTransactions

	err := fixture.csa.HandleCoordinatorHeartbeatNotification(fixture.ctx, &CoordinatorHeartbeatNotification{
		From:                   fixture.csa.ActiveCoordinator(),
		DispatchedTransactions: dispatchedTransactions,
	})
	require.NoError(t, err)

	assert.Len(t, fixture.csa.DispatchedTransactions(fixture.ctx), len(dispatchedTransactions))
	assert.ElementsMatch(t, dispatchedTransactions, fixture.csa.DispatchedTransactions(fixture.ctx))
}

func TestRule2_2(t *testing.T) {
	/*
		`GIVEN` a`ContractSequencerAgent` is in `Observing` state and is aware of some transactions that have been dispatched by previous active coordinators
		`WHEN` a heartbeat message is received that contains any transactions which have a signing address and nonce number
		`THEN` those transactions replace the list of `DispatchedTransactions` associated with that `Coordinator/Submitter`
	*/

	transactionsDispatchedByPreviousCoordinator := Given(t).DispatchedTransactionList().Coordinator("testCoordinatorNode1").Length(3).Build()

	fixture := Given(t).
		ContractSequencerAgent().
		InObservingState().
		DispatchedTransactions(transactionsDispatchedByPreviousCoordinator).
		Build()

	transactionsDispatchedByNewCoordinator := Given(t).DispatchedTransactionList().Coordinator("testCoordinatorNode2").Length(2).Build()

	err := fixture.csa.HandleCoordinatorHeartbeatNotification(fixture.ctx, &CoordinatorHeartbeatNotification{
		From:                   "testCoordinatorNode2",
		DispatchedTransactions: transactionsDispatchedByNewCoordinator.dispatchedTransactions,
	})
	require.NoError(t, err)

	assert.Len(t, fixture.csa.DispatchedTransactions(fixture.ctx), 5)
	assert.Subset(t, fixture.csa.DispatchedTransactions(fixture.ctx), transactionsDispatchedByNewCoordinator.dispatchedTransactions)
	assert.Subset(t, fixture.csa.DispatchedTransactions(fixture.ctx), transactionsDispatchedByPreviousCoordinator.dispatchedTransactions)
}

func TestRule2_3(t *testing.T) {
	/*
		`GIVEN` a`ContractSequencerAgent` is in `Observing` state and is aware of some transactions that have been dispatched by multiple coordinators
		`WHEN` a heartbeat message is received that contains any transactions which have a signing address and nonce number
		`THEN` those transactions replace the list of `DispatchedTransactions` associated with that `Coordinator/Submitter`
	*/

	transactionsDispatchedByCoordinator1 := Given(t).DispatchedTransactionList().Coordinator("testCoordinatorNode1").Length(2).Build()

	transactionsDispatchedByCoordinator2 := Given(t).DispatchedTransactionList().Coordinator("testCoordinatorNode2").Length(2).Build()

	fixture := Given(t).
		ContractSequencerAgent().
		InObservingState().
		DispatchedTransactions(transactionsDispatchedByCoordinator1, transactionsDispatchedByCoordinator2).
		Build()

	newSnapshotOfTransactionsDispatchedByCoordinator2 := Given(t).DispatchedTransactionList().Coordinator("testCoordinatorNode2").Length(3).Build()

	err := fixture.csa.HandleCoordinatorHeartbeatNotification(fixture.ctx, &CoordinatorHeartbeatNotification{
		From:                   "testCoordinatorNode2",
		DispatchedTransactions: newSnapshotOfTransactionsDispatchedByCoordinator2.dispatchedTransactions,
	})
	require.NoError(t, err)

	assert.Len(t, fixture.csa.DispatchedTransactions(fixture.ctx), len(transactionsDispatchedByCoordinator1.dispatchedTransactions)+len(newSnapshotOfTransactionsDispatchedByCoordinator2.dispatchedTransactions))
	assert.Subset(t, fixture.csa.DispatchedTransactions(fixture.ctx), transactionsDispatchedByCoordinator1.dispatchedTransactions)
	assert.Subset(t, fixture.csa.DispatchedTransactions(fixture.ctx), newSnapshotOfTransactionsDispatchedByCoordinator2.dispatchedTransactions)
}

func TestRule3_1(t *testing.T) {
	/*
		`GIVEN` a`ContractSequencerAgent` is in `Sender` state
			`AND` there are some transactions in `Delegated` state
			`AND` the `ActiveCoordinator` is the `n`th most preferred coordinator for the current block range
		`WHEN` the sender does not receive any `CoordinatorHeartbeatNotification` message from the `ActiveCoordinator` for a period of `CoordinatorHeartbeatFailureThreshold`
		`THEN` the `ActiveCoordinator` is the `n+1`th most preferred coordinator for the current block range
			`AND` a `DelegationRequest` message for all transactions in `DELEGATED` state is sent to the new `ActiveCoordinator`
	*/

	//Setup
	fixture := Given(t).
		ContractSequencerAgent().
		Committee("testCoordinatorNode1", "testCoordinatorNode2", "testCoordinatorNode3").
		InSenderState().
		ActiveCoordinator("testCoordinatorNode1").
		CoordinatorRankingForCurrentBlock("testCoordinatorNode1", "testCoordinatorNode2", "testCoordinatorNode3").
		PooledTransactions(
			func(builder *PooledTransactionListFixtureBuilder) {
				builder.
					Length(2).
					Coordinator("testCoordinatorNode1")
			}).
		Build()

	//Exercise
	fixture.ExceedHeartbeatFailureThreshold()

	//Verify
	assert.Equal(t, "testCoordinatorNode2", fixture.csa.ActiveCoordinator())
	assert.True(
		t,
		fixture.outboundMessageMonitor.Sends(
			DelegationRequestMatcher().
				Containing(
					fixture.pooledTransactions,
				).
				To("testCoordinatorNode2").
				Match(),
		),
		"Expected delegation request not sent.",
	)
}

func TestRule3_2(t *testing.T) {
	/*
		   `GIVEN` a`ContractSequencerAgent` is in `Sender` state
		   		`AND` there are some transactions in `Delegated` state
		   		`AND` the `ActiveCoordinator` is the `n`th most preferred coordinator for the current block range
		   `WHEN` the sender does not receive any `CoordinatorHeartbeatNotification` message from the `ActiveCoordinator` for a period less than `CoordinatorHeartbeatFailureThreshold`
		   `THEN` the `ActiveCoordinator` is unchanged
			   	`AND` a `DelegationRequest` message is not sent to any other coordinator
	*/

	//Setup
	fixture := Given(t).
		ContractSequencerAgent().
		Committee("testCoordinatorNode1", "testCoordinatorNode2", "testCoordinatorNode3").
		InSenderState().
		ActiveCoordinator("testCoordinatorNode1").
		CoordinatorRankingForCurrentBlock("testCoordinatorNode1", "testCoordinatorNode2", "testCoordinatorNode3").
		PooledTransactions(
			func(builder *PooledTransactionListFixtureBuilder) {
				builder.
					Length(2).
					Coordinator("testCoordinatorNode1")
			}).
		Build()

	//Exercise
	fixture.AlmostExceedHeartbeatFailureThreshold()

	//Verify
	assert.Equal(t, "testCoordinatorNode1", fixture.csa.ActiveCoordinator())
	assert.False(
		t,
		fixture.outboundMessageMonitor.Sends(
			DelegationRequestMatcher().
				Match(),
		),
		"Expected delegation request not sent.",
	)
}
func TestRule4_1(t *testing.T) {
	/*
		   `GIVEN` a`ContractSequencerAgent` is in `Sender` state
			   	`AND` there are some transactions in `Delegated` state
			   	`AND` the active coordinator is block range `n`
		   `WHEN` a new block height is detected which moves `this` block height to a range higher than `n`
		   `THEN` `ActiveCoordinator` is the highest ranking coordinator for the current block range
		   		`AND` sender sends a `DelegationRequest` to containing all transactions currently in `Delegated` state to `ActiveCoordinator
	*/

	//Setup
	fixture := Given(t).
		ContractSequencerAgent().
		InSenderState().
		ActiveCoordinator("testCoordinatorNode1").
		//RelativeBlockHeight(BlockHeight_LastInRange).
		CoordinatorRankingForCurrentBlock("testCoordinatorNode1", "testCoordinatorNode2", "testCoordinatorNode3").
		CoordinatorRankingForNextBlock("testCoordinatorNode2", "testCoordinatorNode3", "testCoordinatorNode1").
		PooledTransactions(
			func(builder *PooledTransactionListFixtureBuilder) {
				builder.
					Length(2).
					Coordinator("testCoordinatorNode1")
			}).
		Build()

	//Exercise
	fixture.MoveToNextBlockRange()

	//Verify
	assert.Equal(t, "testCoordinatorNode2", fixture.csa.ActiveCoordinator())
	assert.True(
		t,
		fixture.outboundMessageMonitor.Sends(
			DelegationRequestMatcher().
				Containing(
					fixture.pooledTransactions,
				).
				To("testCoordinatorNode2").
				Match(),
		),
	)
}

func TestRule4_2(t *testing.T) {
	/*
	   	`GIVEN` a ContractSequencerAgent` is not in `Coordinator` state
	 		`AND` `ActiveCoordinator` is empty
	   	`WHEN` a `DelegationRequest` is received
	   	`THEN` the `ContractSequencerAgent` is in `Coordinator.Active` state
	*/

	//Setup
	fixture := Given(t).
		ContractSequencerAgent().
		Build()

	//Exercise
	privateTransactionList := Given(t).PrivateTransactionList().Length(2).Build()

	fixture.csa.HandleDelegationRequest(fixture.ctx, &DelegationRequest{
		Sender:          "testSenderNode1",
		ContractAddress: tktypes.RandAddress(),
		Transactions:    privateTransactionList.privateTransactions,
	})

	//Verify
	assert.True(t, fixture.csa.IsCoordinator())
	assert.Equal(t, CoordinatorState_Active, fixture.csa.CoordinatorState())

}

func TestRule4_3(t *testing.T) {
	/*
		`GIVEN` a ContractSequencerAgent` is not in `Coordinator` state
			`AND` `ActiveCoordinator` is not empty
			`AND`  `FlushPoint` is empty in the latest `CoordinatorHeartbeatNotification` from the `ActiveCoordinator`
		`WHEN` a `DelegationRequest` is received
		`THEN` the `ContractSequencerAgent` is in `Coordinator.Elect` state
			`AND` a `HandoverRequest` is sent to the `ActiveCoordinator`
	*/

	//Setup
	fixture := Given(t).
		ContractSequencerAgent().
		ActiveCoordinator("otherCoordinatorNode").
		LatestReceivedHeartbeatNotification(
			func(builder CoordinatorHeartbeatNotificationBuilder) {
				builder.CoordinatorState(CoordinatorState_Active)
			},
		).
		Build()

	//Exercise
	privateTransactionList := Given(t).PrivateTransactionList().Length(2).Build()

	fixture.csa.HandleDelegationRequest(fixture.ctx, &DelegationRequest{
		Sender:          "testSenderNode1",
		ContractAddress: tktypes.RandAddress(),
		Transactions:    privateTransactionList.privateTransactions,
	})

	//Verify
	assert.True(t, fixture.csa.IsCoordinator())
	assert.Equal(t, CoordinatorState_Elect, fixture.csa.CoordinatorState())
	assert.True(
		t,
		fixture.outboundMessageMonitor.Sends(
			HandoverRequestMatcher().
				To("otherCoordinatorNode").
				Match(),
		),
	)
}

func TestRule4_4(t *testing.T) {
	/*
		`GIVEN` a`ContractSequencerAgent` is in `Coordinator.Flush` state
			`AND` some transactions have been delegated by multiple, but not all, senders
			`AND` some of those delegated transactions have been dispatched
			`AND` some of the dispatched transactions have been confirmed on the blockchain as success
			`AND` some of the dispatched transactions have been confirmed on the blockchain as reverted
		`WHEN` `HeartbeatInterval` passes
		`THEN` a `CoordinatorHeartbeatNotification` message is broadcast to every node in the group reporting the `Flushpoints`, current block height, all dispatched unconfirmed transactions, and zero non dispatched transactions, all transaction dispatched by this coordinator that have been confirmed ( success or revert) on the block chain since entering flush state
	*/

	//Setup
	fixture := Given(t).
		ContractSequencerAgent().
		NodeName("nodeA").
		Committee("nodeA", "nodeB", "nodeC", "nodeD").
		CoordinatorState(CoordinatorState_Flush).
		DelegatedTransactions(
			[]*TransactionFixture{
				NewTransactionFixture("txB1").Sender("nodeB").State(TransactionState_ConfirmedSuccess),
				NewTransactionFixture("txB2").Sender("nodeB").State(TransactionState_ConfirmedReverted),
				NewTransactionFixture("txB3").Sender("nodeB").State(TransactionState_Dispatched),
				NewTransactionFixture("txB4").Sender("nodeB").State(TransactionState_Assembled),
				NewTransactionFixture("txB5").Sender("nodeB").State(TransactionState_Pooled),
				NewTransactionFixture("txC1").Sender("nodeC").State(TransactionState_ConfirmedSuccess),
				NewTransactionFixture("txC2").Sender("nodeC").State(TransactionState_ConfirmedReverted),
				NewTransactionFixture("txC3").Sender("nodeC").State(TransactionState_Dispatched),
			},
		).
		Build()

	//Exercise
	fixture.HeartbeatIntervalPassed()

	//Verify
	assert.True(
		t,
		fixture.outboundMessageMonitor.Sends(
			CoordinatorHeartbeatNotificationMatcher(t).
				To("nodeB").
				CoordinatorState(CoordinatorState_Flush).
				PooledTransactionList(
					func(matcher *PooledTransactionListMatcher) {
						matcher.
							Length(2)
					}).
				DispatchedTransactionList(
					func(matcher *DispatchedTransactionListMatcher) {
						matcher.
							Length(2)
					}).
				ConfirmedTransactionList(
					func(matcher *ConfirmedTransactionListMatcher) {
						matcher.
							Length(4)
					}).
				BlockHeight(fixture.csa.BlockHeight()).
				Match(),
		),
	)

	assert.True(
		t,
		fixture.outboundMessageMonitor.Sends(
			CoordinatorHeartbeatNotificationMatcher(t).
				To("nodeC").
				CoordinatorState(CoordinatorState_Flush).
				PooledTransactionList(
					func(matcher *PooledTransactionListMatcher) {
						matcher.
							Length(0)
					}).
				DispatchedTransactionList(
					func(matcher *DispatchedTransactionListMatcher) {
						matcher.
							Length(2)
					}).
				ConfirmedTransactionList(
					func(matcher *ConfirmedTransactionListMatcher) {
						matcher.
							Length(4)
					}).
				BlockHeight(fixture.csa.BlockHeight()).
				Match(),
		),
	)

	assert.True(
		t,
		fixture.outboundMessageMonitor.Sends(
			CoordinatorHeartbeatNotificationMatcher(t).
				To("nodeD").
				CoordinatorState(CoordinatorState_Flush).
				PooledTransactionList(
					func(matcher *PooledTransactionListMatcher) {
						matcher.
							Length(0)
					}).
				DispatchedTransactionList(
					func(matcher *DispatchedTransactionListMatcher) {
						matcher.
							Length(2)
					}).
				ConfirmedTransactionList(
					func(matcher *ConfirmedTransactionListMatcher) {
						matcher.
							Length(4)
					}).
				BlockHeight(fixture.csa.BlockHeight()).
				Match(),
		),
	)
}

func TestRule4_5(t *testing.T) {
	/*
		`GIVEN` a`ContractSequencerAgent` is in `Coordinator.Flush` state
		  `AND` a number of transactions have been dispatched and submitted but not confirmed
		`WHEN` a block receipt is received that confirms (success or revert) the final transaction (flushpoint) dispatched by this coordinator
		`THEN` a ContractSequencerAgent` is in `Coordinator.Closing` state
	*/

	//Setup
	fixture := Given(t).
		ContractSequencerAgent().
		NodeName("nodeA").
		Committee("nodeA", "nodeB", "nodeC", "nodeD").
		CoordinatorState(CoordinatorState_Flush).
		DelegatedTransactions(
			[]*TransactionFixture{
				NewTransactionFixture("txB1").Sender("nodeB").State(TransactionState_Submitted),
			}).
		Build()

	//Exercise
	fixture.ConfirmTransaction("txB1")

	//Verify
	assert.Equal(t, CoordinatorState_Closing, fixture.csa.CoordinatorState())

}

func TestRule4_6(t *testing.T) {
	/*
		`GIVEN`  a ContractSequencerAgent` is in `Coordinator.Closing` state
		`WHEN` `HeartbeatInterval` passes
		`THEN` a `CoordinatorHeartbeatNotification` message is broadcast to every node in the group reporting the `ClosingStatus`  which includes a list of all transactions that were confirmed ( as success or revert) on the block chain since entering flush state
	*/
	//Setup
	fixture := Given(t).
		ContractSequencerAgent().
		NodeName("nodeA").
		Committee("nodeA", "nodeB", "nodeC", "nodeD").
		CoordinatorState(CoordinatorState_Closing).
		DelegatedTransactions(
			[]*TransactionFixture{
				NewTransactionFixture("txA1").Sender("nodeA").State(TransactionState_Submitted),
				NewTransactionFixture("txB1").Sender("nodeB").State(TransactionState_Submitted),
				NewTransactionFixture("txC1").Sender("nodeC").State(TransactionState_Submitted),
				NewTransactionFixture("txD1").Sender("nodeD").State(TransactionState_Submitted),
			}).
		Build()

	//Exercise
	fixture.HeartbeatIntervalPassed()

	//Verify
	assert.True(
		t,
		fixture.outboundMessageMonitor.Sends(
			CoordinatorHeartbeatNotificationMatcher(t).
				To("nodeB").
				CoordinatorState(CoordinatorState_Closing).
				ConfirmedTransactionList(
					func(matcher *ConfirmedTransactionListMatcher) {
						matcher.
							Length(4)
					}).
				BlockHeight(fixture.csa.BlockHeight()).
				Match(),
		),
	)
}

func TestRule4_7(t *testing.T) {
	/*
	   `GIVEN`  a ContractSequencerAgent` is in `Coordinator.Closing` state
	   `WHEN` `CoordinatorHeartbeatFailureThreshold` passes since entering `Coordinator.Closing` state
	   `THEN` the `ContractSequencerAgent` is in `Coordinator.Idle` state
	*/

	//Setup
	fixture := Given(t).
		ContractSequencerAgent().
		NodeName("nodeA").
		Committee("nodeA", "nodeB", "nodeC", "nodeD").
		CoordinatorState(CoordinatorState_Closing).
		HeartbeatIntervalsSinceStateChange(5).
		DelegatedTransactions(
			[]*TransactionFixture{
				NewTransactionFixture("txB1").Sender("nodeB").State(TransactionState_Submitted),
			}).
		Build()

	//Exercise
	fixture.HeartbeatIntervalPassed()

	//Verify
	assert.Equal(t, CoordinatorState_Idle, fixture.csa.CoordinatorState())

	// Extra verify that no further heartbeat is sent after the state change
	fixture.outboundMessageMonitor.Clear()
	fixture.HeartbeatIntervalPassed()
	assert.False(
		t,
		fixture.outboundMessageMonitor.Sends(
			CoordinatorHeartbeatNotificationMatcher(t).
				Match(),
		),
	)
}

func TestRule4_8(t *testing.T) {
	/*
		`GIVEN` a`ContractSequencerAgent` is in `Coordinator.Flush` state
		`WHEN` a `DispatchConfirmation` is received for a prepared transaction
		`THEN` the transaction is not dispatched
	*/

	//Setup
	fixture := Given(t).
		ContractSequencerAgent().
		NodeName("nodeA").
		Committee("nodeA", "nodeB", "nodeC", "nodeD").
		CoordinatorState(CoordinatorState_Flush).
		DelegatedTransactions(
			[]*TransactionFixture{
				NewTransactionFixture("txB1").Sender("nodeB").State(TransactionState_ConfirmingForDispatch),
			}).
		Build()

	//Exercise
	fixture.HandleDispatchConfirmationResponse("txB1")

	//Verify
	assert.False(
		t,
		fixture.outboundMessageMonitor.Sends(
			DispatchConfirmationResponseMatcher(t).
				Match(),
		),
	)
}

func TestRule4_9(t *testing.T) {
	/*
	   `GIVEN` a`ContractSequencerAgent` is in `Coordinator.Flush` state
	   `WHEN` a `EndorsementResponse` is received for an `Assembled` transaction
	   `THEN` the `EndorsementResponse` is ignored
	   	`AND` there is no `DispatchConfirmationRequest`

	   NOTE: this is a difficult rule to express because the memory state of the Coordinator does not necessarily contain the Assembled transaction. It is valid for the coordinator to delete all non dispatched transactions from memory when it goes into `Flush` state.
	*/

	//Setup
	fixture := Given(t).
		ContractSequencerAgent().
		NodeName("nodeA").
		Committee("nodeA", "nodeB", "nodeC").
		CoordinatorState(CoordinatorState_Flush).
		DelegatedTransactions(
			[]*TransactionFixture{
				NewTransactionFixture("txB1").
					Sender("nodeB").
					State(TransactionState_Assembled).
					RequiredEndorsements("nodeA", "nodeB", "nodeC").
					Endorsements("nodeA", "nodeB"),
			}).
		Build()

	//Exercise
	fixture.HandleDispatchConfirmationResponse("txB1")

	//Verify
	assert.False(
		t,
		fixture.outboundMessageMonitor.Sends(
			DispatchConfirmationResponseMatcher(t).
				Match(),
		),
	)
}

func TestRule5_9(t *testing.T) {
	/*
		`GIVEN` a `ContractSequencerAgent` is in `Coordinator.Active` state
			`AND` a transaction is in `Graph` state
			`AND` all bar one endorsements have been received
			`AND` the transaction has no dependencies
		`WHEN` the final `EndorsementResponse` is received
		`THEN` the `ContractSequencerAgent` sends a `DispatchConfirmationRequest`
			`AND` the transaction is in `ConfirmingForDispatch` state
	*/

	//Setup
	fixture := Given(t).
		ContractSequencerAgent().
		NodeName("nodeA").
		Committee("nodeA", "nodeB", "nodeC").
		CoordinatorState(CoordinatorState_Active).
		DelegatedTransactions(
			[]*TransactionFixture{
				NewTransactionFixture("txB1").
					Sender("nodeB").
					State(TransactionState_Assembled).
					RequiredEndorsements("nodeA", "nodeB", "nodeC").
					Endorsements("nodeA", "nodeB"),
			}).
		Build()

	//Exercise
	fixture.HandleDispatchConfirmationResponse("txB1")

	//Verify
	assert.True(
		t,
		fixture.outboundMessageMonitor.Sends(
			DispatchConfirmationResponseMatcher(t).
				Match(),
		),
	)

	//assert.Equal(t, TransactionState_ConfirmingForDispatch, fixture.TransactionState("txB1"))
}

func TestRule10000(t *testing.T) {

	//`WHEN` a user calls `ptx_SendTransaction`
	//`THEN` a new transaction is created in the `Pending` state
	//	`AND` the transaction id is returned to the user
	//	`AND` the ContractSequencerAgent` is in `Sender` state
	//	`AND` the transaction is in `Pending` state

	fixture := Given(t).
		ContractSequencerAgent().
		InObservingState().
		Build()

	err := fixture.csa.HandleTransaction(fixture.ctx, &components.PrivateTransaction{})
	require.NoError(t, err)

}
