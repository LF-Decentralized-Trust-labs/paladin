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

	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator/transaction"
)

type Guard func(ctx context.Context, c *coordinator) bool

func behind(ctx context.Context, c *coordinator) bool {
	//Return true if the current block height that our indexer has reached is behind the current coordinator
	// there is a configured tolerance so if we are within this tolerance we are not considered behind
	return c.currentBlockHeight < c.activeCoordinatorBlockHeight-c.blockHeightTolerance
}

func notBehind(ctx context.Context, c *coordinator) bool {
	return !behind(ctx, c)
}

func activeCoordinatorFlushComplete(ctx context.Context, c *coordinator) bool {
	for _, flushPoint := range c.activeCoordinatorsFlushPointsBySignerNonce {
		if !flushPoint.Confirmed {
			return false
		}
	}
	return true
}

// Function flushComplete returns true if there are no transactions past the point of no return that haven't been confirmed yet
func flushComplete(ctx context.Context, c *coordinator) bool {
	return len(
		c.getTransactionsInStates(ctx, []transaction.State{
			transaction.State_Ready_For_Dispatch,
			transaction.State_Dispatched,
			transaction.State_Submitted,
		}),
	) == 0
}

// Function noTransactionsInflight returns true if all transactions that have been delegated to this coordinator have been confirmed
func noTransactionsInflight(ctx context.Context, c *coordinator) bool {
	return len(
		c.getTransactionsNotInStates(ctx, []transaction.State{
			transaction.State_Confirmed,
		}),
	) == 0
}

func closingGracePeriodExpired(ctx context.Context, c *coordinator) bool {
	return c.heartbeatIntervalsSinceStateChange >= c.closingGracePeriod
}
