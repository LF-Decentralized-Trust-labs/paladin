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

package stages

import (
	"context"
	"encoding/json"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type AttestationResult struct {
}

type AttestationStage struct{}

func (as *AttestationStage) Name() string {
	return "attestation"
}

func (as *AttestationStage) GetIncompletePreReqTxIDs(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) *types.TxProcessPreReq {
	txsToCheck := tsg.GetPreReqTransactions(ctx)
	if tsg.GetDispatchAddress(ctx) == "" {
		// no dispatch address selected yet, continue to assign one
		return nil
	}
	preReqsPending := sfs.DependencyChecker().PreReqsMatchCondition(ctx, txsToCheck, func(preReqTx transactionstore.TxStateGetters) (preReqComplete bool) {
		return preReqTx.IsAttestationCompleted(ctx) // TODO: need to do a better check here

	})
	if len(preReqsPending) > 0 {
		return &types.TxProcessPreReq{
			TxIDs: preReqsPending,
		}
	}
	return nil
}

func (as *AttestationStage) ProcessEvents(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService, stageEvents []*types.StageEvent) (unprocessedStageEvents []*types.StageEvent, txUpdates *transactionstore.TransactionUpdate, nextStep types.StageProcessNextStep) {
	unprocessedStageEvents = []*types.StageEvent{}
	nextStep = types.NextStepWait
	for _, se := range stageEvents {
		if string(se.Stage) == as.Name() { // the current stage does not care about events from other stages yet (may need to be for interrupts)
			if se.Data != nil {
				switch v := se.Data.(type) {
				case prototk.AttestationResult: // TODO, we need to check the attestation matches the current version
					if txUpdates == nil {
						txUpdates = &transactionstore.TransactionUpdate{}
					}
					existingAttResults := tsg.GetAttestationResults(ctx)
					existingAttResults = append(existingAttResults, &v)
					attResultBytes, err := json.Marshal(existingAttResults)
					attResultStr := string(attResultBytes)
					if err != nil {
						// stage retry needed
						nextStep = types.NextStepNewStage
						break
					}
					txUpdates.AttestationResults = &attResultStr
					if len(existingAttResults) == len(tsg.GetAttestationPlan(ctx)) {
						nextStep = types.NextStepNewStage
					} else {
						nextStep = types.NextStepWait
					}
				}
			}
			//TODO: panic error, retry when data is nil?

		} else {
			unprocessedStageEvents = append(unprocessedStageEvents, se)
		}
	}
	return
}

func (as *AttestationStage) MatchStage(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) bool {
	return tsg.IsAttestationCompleted(ctx)
}

func (as *AttestationStage) PerformAction(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) (actionOutput interface{}, actionTriggerErr error) {
	if as.GetIncompletePreReqTxIDs(ctx, tsg, sfs) != nil {
		return nil, i18n.NewError(ctx, msgs.MsgTransactionProcessorBlockedOnDependency, tsg.GetTxID(ctx), as.Name())
	}
	attPlan := tsg.GetAttestationPlan(ctx)
	attResults := tsg.GetAttestationResults(ctx)
	for _, ap := range attPlan {
		toBeComplete := true
		for _, ar := range attResults {
			if ar.GetAttestationType().Type() == ap.GetAttestationType().Type() {
				toBeComplete = false
				break
			}
		}
		if toBeComplete {
			// TODO: emit the attestation request in a separate go routine
			sfs.TransportManager().SyncExchange() // this should be async.
		}
	}
	return nil, nil
}
