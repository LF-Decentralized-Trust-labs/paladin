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

//TODO how do we test this?  If a CSA object exists, it is by definition in memory and not idle
// need to add  ContractSequencerAgentManager to manage the lifecycle of the CSA and test this rul
//`GIVEN` a`ContractSequencerAgent` is in `Idle` state (i.e. not loaded into memory)
//`WHEN` a `CoordinatorHeartbeatNotification` heartbeat message is received from any node
//`THEN` the `ContractSequencerAgent` is in `Observing` state
// `AND` `ActiveCoordinator` is equal to the sender of the given `CoordinatorHeartbeatNotification`

// TODO better names for these tests.  Rule1 is not very descriptive and the whole GIVEN/WHEN/THEN thing is not very concise
// unless we specifically number the rules in the spec.
func TestRule1(t *testing.T) {
	ctx := context.Background()
	// GIVEN a contract is in Observing state
	csa, _, _ := GivenContractSequencerAgentInObservingState(t, ctx, nil)

	// WHEN a CoordinatorHeartbeatNotification heartbeat message is received from any node
	testCoordinatorNode := "testCoordinatorNode"
	err := csa.HandleCoordinatorHeartbeatNotification(ctx, &CoordinatorHeartbeatNotification{
		From: testCoordinatorNode,
	})
	require.NoError(t, err)

	// THEN the ActiveCoordinator is equal to the sender of the given CoordinatorHeartbeatNotification
	assert.Equal(t, testCoordinatorNode, csa.ActiveCoordinator())
}

func TestRule2(t *testing.T) {
	/*
		   	`GIVEN` a`ContractSequencerAgent` is in `Observing` state
				`AND` `ActiveCoordinator` is not empty
		   	`WHEN` the sender does not receive any `CoordinatorHeartbeatNotification` message for a period of `CoordinatorHeartbeatFailureThreshold`
		   	`THEN` the `ActiveCoordinator` is empty
	*/

	testCoordinatorNode := "testCoordinatorNode"

	ctx := context.Background()
	// GIVEN a ContractSequencerAgent is in Observing state
	csa, heartbeatTicker, _ := GivenContractSequencerAgentInObservingState(t, ctx, &GivenContractSequencerAgentInObservingStateConditions{
		activeCoordinator: testCoordinatorNode,
	})

	//`WHEN` the sender does not receive any `CoordinatorHeartbeatNotification` message for a period of `CoordinatorHeartbeatFailureThreshold`
	heartbeatTicker.ExceedHeartbeatFailureThreshold(ctx)

	//`THEN` the `ActiveCoordinator` is empty
	assert.True(t, ActiveCoordinatorBecomesEmpty(t, ctx, csa))

}

func TestRule3(t *testing.T) {
	/*
		`GIVEN` a`ContractSequencerAgent` is in `Sending` state
		`WHEN` a `CoordinatorHeartbeatNotification` heartbeat message is received from any node other than the current `ActiveCoordinator`
		`THEN` the `ActiveCoordinator` is equal to the sender of the given `CoordinatorHeartbeatNotification`
			`AND` a `DelegationRequest` message for all transactions in `DELEGATED` state is sent to the new `ActiveCoordinator`
	*/
	ctx := context.Background()
	testCoordinatorNode1 := "testCoordinatorNode1"
	testCoordinatorNode2 := "testCoordinatorNode2"

	//`GIVEN` a`ContractSequencerAgent` is in `Sending` state
	csa, _, outboundMessageMonitor := GivenContractSequencerAgentInSenderState(t, ctx, &GivenContractSequencerAgentInObservingStateConditions{
		activeCoordinator: testCoordinatorNode1,
	})

	//`WHEN` a `CoordinatorHeartbeatNotification` heartbeat message is received from any node other than the current `ActiveCoordinator`
	err := csa.HandleCoordinatorHeartbeatNotification(ctx, &CoordinatorHeartbeatNotification{
		From: testCoordinatorNode2,
	})
	require.NoError(t, err)

	//`THEN` the `ActiveCoordinator` is equal to the sender of the given `CoordinatorHeartbeatNotification`
	assert.True(t, ActiveCoordinatorBecomesEqualTo(t, ctx, csa, testCoordinatorNode2))

	//		`AND` a `DelegationRequest` message for all transactions in `DELEGATED` state is sent to the new `ActiveCoordinator`
	assert.True(t, outboundMessageMonitor.Sends(DelegationRequestMatcher))

}

func TestRule10000(t *testing.T) {
	ctx := context.Background()

	//`WHEN` a user calls `ptx_SendTransaction`
	//`THEN` a new transaction is created in the `Pending` state
	//	`AND` the transaction id is returned to the user
	//	`AND` the ContractSequencerAgent` is in `Sender` state
	//	`AND` the transaction is in `Pending` state

	csa, _, _ := GivenContractSequencerAgentInObservingState(t, ctx, nil)

	err := csa.HandleTransaction(ctx, &components.PrivateTransaction{})
	require.NoError(t, err)

}
