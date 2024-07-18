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

package stage

import (
	"context"

	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
)

type StageProcessNextStep int

const (
	NextStepWait StageProcessNextStep = iota
	NextStepNewStage
	NextStepNewAction
)

type StageEvent struct {
	ID          string           `json:"id"` // TODO: not sure how useful it is to have this ID as the process of event should be idempotent?
	Stage       string           `json:"stage"`
	TxID        string           `json:"transactionId"`
	PreReqTxIDs *TxProcessPreReq `json:"preReq,omitempty"`
	Data        interface{}      `json:"data"` // schema decided by each stage
}

type TxProcessPreReq struct {
	TxIDs []string `json:"transactionIds,omitempty"`
}

// defines the methods for checking whether a transaction's dependents matches a specific criteria
type DependencyChecker interface {
	PreReqsMatchCondition(ctx context.Context, preReqTxIDs []string, conditionFunc func(tsg transactionstore.TxStateGetters) (preReqComplete bool)) (filteredPreReqTxIDs []string)
	GetPreReqDispatchAddresses(ctx context.Context, preReqTxIDs []string) (dispatchAddresses []string)
	RegisterPreReqTrigger(ctx context.Context, txID string, txPreReq *TxProcessPreReq)
}
