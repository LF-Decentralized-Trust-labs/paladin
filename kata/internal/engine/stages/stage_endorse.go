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

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
)

type EndorsementResult struct {
}

type EndorsementStage struct{}

func (es *EndorsementStage) Name() string {
	return "dispatch"
}

func (es *EndorsementStage) GetIncompletePreReqTxIDs(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) *types.TxProcessPreReq {
	txsToCheck := tsg.GetPreReqTransactions(ctx)
	if tsg.GetDispatchAddress(ctx) == "" {
		// no dispatch address selected yet, continue to assign one
		return nil
	}
	preReqsPending := sfs.DependencyChecker().PreReqsMatchCondition(ctx, txsToCheck, func(preReqTx transactionstore.TxStateGetters) (preReqComplete bool) {
		if preReqTx.GetDispatchAddress(ctx) != tsg.GetDispatchAddress(ctx) { // the pre-req tx is managed by a different address - no matter which node
			if preReqTx.GetConfirmedTxHash(ctx) != "" {
				return true
			}
		} else {
			// managed by the same address so we can chain them using nonce
			if preReqTx.GetDispatchTxID(ctx) != "" { // if the pre-req transaction already has a tracking tx ID, it means it has a nonce, so it's no longer a pre-req
				return true
			}
		}
		return false
	})
	if len(preReqsPending) > 0 {
		return &types.TxProcessPreReq{
			TxIDs: preReqsPending,
		}
	}
	return nil
}

func (es *EndorsementStage) ProcessEvents(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService, stageEvents []*types.StageEvent) (unprocessedStageEvents []*types.StageEvent, txUpdates *transactionstore.TransactionUpdate, nextStep types.StageProcessNextStep) {
	unprocessedStageEvents = []*types.StageEvent{}
	nextStep = types.NextStepWait
	for _, se := range stageEvents {
		if string(se.Stage) == es.Name() { // the current stage does not care about events from other stages yet (may need to be for interrupts)
			if se.Data != nil {
				switch v := se.Data.(type) {
				case DispatchAddress:
					if txUpdates == nil {
						txUpdates = &transactionstore.TransactionUpdate{}
					}
					txUpdates.DispatchAddress = confutil.P(string(v))
					nextStep = types.NextStepNewAction
				case TxSubmissionOutput:
					submissionOutput := v
					if submissionOutput.ErrorMessage != "" {
						// handle submission error
					} else {
						if txUpdates == nil {
							txUpdates = &transactionstore.TransactionUpdate{}
						}
						txUpdates.DispatchTxID = &submissionOutput.TransactionID
						nextStep = types.NextStepNewStage
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

func (es *EndorsementStage) MatchStage(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) bool {
	if tsg.GetDispatchTxPayload(ctx) != "" && sfs.IdentityResolver().IsCurrentNode(tsg.GetDispatchNode(ctx)) && tsg.GetDispatchTxID(ctx) == "" {
		// NOTE: we will use transaction payload hash as idempotency key, so it's retry safe
		return true
	}
	return false
}

func (es *EndorsementStage) PerformAction(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) (actionOutput interface{}, actionTriggerErr error) {
	if es.GetIncompletePreReqTxIDs(ctx, tsg, sfs) != nil {
		return nil, i18n.NewError(ctx, msgs.MsgTransactionProcessorBlockedOnDependency, tsg.GetTxID(ctx), es.Name())
	}

	if tsg.GetDispatchAddress(ctx) == "" {
		preReqAddresses := sfs.DependencyChecker().GetPreReqDispatchAddresses(ctx, tsg.GetPreReqTransactions(ctx))
		address := sfs.IdentityResolver().GetDispatchAddress(preReqAddresses)
		if address != "" {
			return DispatchAddress(address), nil
		}
	} else {
		// TODO: submit tx
		return TxSubmissionOutput{}, nil
	}

	return nil, i18n.NewError(ctx, msgs.MsgTransactionProcessorActionFailed, tsg.GetTxID(ctx), es.Name())
}
