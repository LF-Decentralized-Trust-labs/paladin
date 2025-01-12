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
	"context"
	"testing"

	"github.com/kaleido-io/paladin/core/internal/components"
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

	ctx := context.Background()
	// GIVEN a ContractSequencerAgent is in Observing state
	//  `AND` `ActiveCoordinator` is not empty

	fixture := Given(t).
		ContractSequencerAgent().
		InObservingState().
		ActiveCoordinator(testCoordinatorNode).
		Build()

	//`WHEN` the sender does not receive any `CoordinatorHeartbeatNotification` message for a period of `CoordinatorHeartbeatFailureThreshold`
	fixture.ExceedHeartbeatFailureThreshold(ctx)

	//`THEN` the `ActiveCoordinator` is empty
	assert.True(t, ActiveCoordinatorBecomesEmpty(t, ctx, fixture.csa))

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

	//`GIVEN` a`ContractSequencerAgent` is in `Sending` state
	fixture := Given(t).
		ContractSequencerAgent().
		InSenderState().
		DelegatedTransactions(
			func(builder *DelegatedTransactionListFixtureBuilder) {
				builder.
					Length(2)
			}).
		ActiveCoordinator(testCoordinatorNode1).
		Build()

	//`WHEN` a `CoordinatorHeartbeatNotification` heartbeat message is received from any node other than the current `ActiveCoordinator`
	err := fixture.csa.HandleCoordinatorHeartbeatNotification(fixture.ctx, &CoordinatorHeartbeatNotification{
		From: testCoordinatorNode2,
	})
	require.NoError(t, err)

	//`THEN` the `ActiveCoordinator` is equal to the sender of the given `CoordinatorHeartbeatNotification`
	assert.True(t, ActiveCoordinatorBecomesEqualTo(t, fixture.ctx, fixture.csa, testCoordinatorNode2))

	//		`AND` a `DelegationRequest` message for all transactions in `DELEGATED` state is sent to the new `ActiveCoordinator`
	assert.True(
		t,
		fixture.outboundMessageMonitor.Sends(
			FireAndForgetMessageMatcher(DelegationRequestMatcher().
				Containing(fixture.delegatedTransactions).Match(),
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
		InSenderState().
		ActiveCoordinator("testCoordinatorNode1").
		DelegatedTransactions(
			func(builder *DelegatedTransactionListFixtureBuilder) {
				builder.
					Length(2).
					Coordinator("testCoordinatorNode1")
			}).
		Build()

	//Exercise
	fixture.ExceedHeartbeatFailureThreshold(fixture.ctx)

	//Verify
	assert.True(
		t,
		fixture.outboundMessageMonitor.Sends(
			DelegationRequestMatcher().
				Containing(
					fixture.delegatedTransactions,
				).
				Match(),
		),
		"Expected delegation request not sent.",
	)
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
