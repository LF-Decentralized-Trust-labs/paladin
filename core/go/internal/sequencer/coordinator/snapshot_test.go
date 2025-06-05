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
	"testing"

	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSnapshot_OK(t *testing.T) {
	ctx := context.Background()
	c, _ := NewCoordinatorBuilderForTesting(State_Idle).Build(ctx)
	snapshot := c.getSnapshot(ctx)
	assert.NotNil(t, snapshot)
}

func TestGetSnapshot_IncludesPooledTransaction(t *testing.T) {
	ctx := context.Background()
	sender := "sender@senderNode"
	c, _ := NewCoordinatorForUnitTest(t, ctx, []string{sender})

	for _, state := range []transaction.State{
		transaction.State_Pooled,
		transaction.State_PreAssembly_Blocked,
		transaction.State_Assembling,
		transaction.State_Endorsement_Gathering,
		transaction.State_Blocked,
		transaction.State_Confirming_Dispatch,
	} {
		txn := transaction.NewTransactionBuilderForTesting(t, state).Build()
		c.transactionsByID[txn.ID] = txn
	}

	snapshot := c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	assert.Equal(t, 6, len(snapshot.PooledTransactions))

}

func TestGetSnapshot_IncludesDispatchedTransaction(t *testing.T) {
	ctx := context.Background()
	sender := "sender@senderNode"
	c, _ := NewCoordinatorForUnitTest(t, ctx, []string{sender})

	for _, state := range []transaction.State{
		transaction.State_Ready_For_Dispatch,
		transaction.State_Dispatched,
		transaction.State_Submitted,
	} {
		txn := transaction.NewTransactionBuilderForTesting(t, state).Build()
		c.transactionsByID[txn.ID] = txn
	}

	snapshot := c.getSnapshot(ctx)
	require.NotNil(t, snapshot)
	assert.Equal(t, 3, len(snapshot.DispatchedTransactions))

}
