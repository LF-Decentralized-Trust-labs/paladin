/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License dps distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package stages

import (
	"context"

	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
)

type DispatchPrepStage struct {
}

func (dps *DispatchPrepStage) Name() string {
	return "dispatch_prep"
}

func (dps *DispatchPrepStage) GetIncompletePreReqTxIDs(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) *enginespi.TxProcessPreReq {
	return nil
}

func (dps *DispatchPrepStage) ProcessEvents(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService, stageEvents []*enginespi.StageEvent) (unprocessedStageEvents []*enginespi.StageEvent, txUpdates *transactionstore.TransactionUpdate, nextStep enginespi.StageProcessNextStep) {
	unprocessedStageEvents = []*enginespi.StageEvent{}
	nextStep = enginespi.NextStepWait
	for _, se := range stageEvents {
		if string(se.Stage) == dps.Name() { // the current stage does not care about events from other stages yet (may need to be for interrupts)
			if se.Data != nil {
				switch v := se.Data.(type) {
				case *components.EthTransaction:
					txUpdates = &transactionstore.TransactionUpdate{
						PreparedTransaction: v,
					}
					nextStep = enginespi.NextStepNewStage
				}
			}
		} else {
			unprocessedStageEvents = append(unprocessedStageEvents, se)
		}
	}
	return
}

func (dps *DispatchPrepStage) MatchStage(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) bool {
	return tsg.HACKGetPrivateTx().PreAssembly != nil && tsg.HACKGetPrivateTx().PostAssembly != nil && tsg.HACKGetPrivateTx().PreparedTransaction == nil
}

func (dps *DispatchPrepStage) PerformAction(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) (actionOutput interface{}, actionTriggerErr error) {
	prepError := sfs.DomainAPI().PrepareTransaction(ctx, tsg.HACKGetPrivateTx())

	if prepError != nil {
		return nil, prepError
	} else {
		return tsg.HACKGetPrivateTx().PreparedTransaction, nil
	}
}
