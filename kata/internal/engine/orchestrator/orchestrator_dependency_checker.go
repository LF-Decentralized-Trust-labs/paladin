/*
 * Copyright © 2024 Kaleido, Inc.
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

package orchestrator

import (
	"context"

	"github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
)

// dependency checker functions are used to efficiently figure out the states of pre-req transactions based on their tx IDs

func (oc *Orchestrator) PreReqsMatchCondition(ctx context.Context, preReqTxIDs []string, conditionFunc func(tsg transactionstore.TxStateGetters) (preReqComplete bool)) (filteredPreReqTxIDs []string) {
	// TODO
	return preReqTxIDs
}
func (oc *Orchestrator) GetPreReqDispatchAddresses(ctx context.Context, preReqTxIDs []string) (dispatchAddresses []string) {
	// TODO
	return nil
}
func (oc *Orchestrator) RegisterPreReqTrigger(ctx context.Context, txID string, txPreReq *types.TxProcessPreReq) {
	// TODO
}
