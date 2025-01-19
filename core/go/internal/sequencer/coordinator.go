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

package sequencer

import (
	"context"

	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
)

type CoordinatorState string

const (
	CoordinatorState_Idle     CoordinatorState = "CoordinatorState_Idle"
	CoordinatorState_Standby  CoordinatorState = "CoordinatorState_Standby"
	CoordinatorState_Elect    CoordinatorState = "CoordinatorState_Elect"
	CoordinatorState_Prepared CoordinatorState = "CoordinatorState_Prepared"
	CoordinatorState_Active   CoordinatorState = "CoordinatorState_Active"
	CoordinatorState_Flush    CoordinatorState = "CoordinatorState_Flush"
	CoordinatorState_Closing  CoordinatorState = "CoordinatorState_Closing"
)

type coordinator struct {
	state                              CoordinatorState
	dispatchedTransactions             *DispatchedTransactionIndex
	confirmedTransactions              *ConfirmedTransactionIndex
	assembledTransactions              Graph
	pooledTransactions                 TransactionPool
	heartbeatIntervalsSinceStateChange int
}

func NewCoordinator(ctx context.Context) *coordinator {
	return &coordinator{
		state:                              CoordinatorState_Idle,
		dispatchedTransactions:             NewDispatchedTransactionIndex(ctx),
		confirmedTransactions:              NewConfirmedTransactionIndex(ctx),
		pooledTransactions:                 NewTransactionPool(ctx),
		assembledTransactions:              NewGraph(ctx),
		heartbeatIntervalsSinceStateChange: 0,
	}
}

func (c *coordinator) handleTransactionConfirmed(ctx context.Context, transaction *blockindexer.IndexedTransactionNotify) {

	dispatchedTransaction := c.dispatchedTransactions.findBySignerNonce(transaction.From, transaction.Nonce)
	if dispatchedTransaction == nil {
		return
	}

	if dispatchedTransaction.LatestSubmissionHash == ptrTo(transaction.Hash) {
		// This is the transaction that we are looking for
	} else {
		//TODO - what does this mean.  We have missed a submission?  Or is it possible that an earlier submission has managed to get confirmed?
		//either way, I this must be the transaction that we are looking for because we can't re-use a nonce
	}

	c.dispatchedTransactions.remove(dispatchedTransaction)
	c.confirmedTransactions.add(&ConfirmedTransaction{
		TransactionID: dispatchedTransaction.TransactionID,
		Hash:          transaction.Hash,
		RevertReason:  transaction.RevertReason,
	})

	if c.state == CoordinatorState_Flush && c.dispatchedTransactions.empty() {
		c.state = CoordinatorState_Closing
		c.heartbeatIntervalsSinceStateChange = 0
	}

}

func ptrTo[T any](v T) *T {
	return &v
}
