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
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type AssembleStage struct{}

func (as *AssembleStage) Name() string {
	return "assemble"
}

func (as *AssembleStage) GetIncompletePreReqTxIDs(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) *types.TxProcessPreReq {
	sequencePreReqsToCheck := tsg.GetPreReqTransactions(ctx) // only get the pre-req in the sequence
	if sfs.Sequencer() == nil {
		// no sequencer, no need to check pre-reqs
		return nil
	}
	assembleRound := sfs.Sequencer().GetLatestAssembleRoundForTx(ctx, tsg.GetTxID(ctx)) // TODO: doesn't work when a tx is in multiple sequences and rounds need to checked for all sequences
	preReqsPending := sfs.DependencyChecker().PreReqsMatchCondition(ctx, sequencePreReqsToCheck, func(preReqTx transactionstore.TxStateGetters) (preReqComplete bool) {
		return preReqTx.GetAssembledRound(ctx) == assembleRound // only treat pre-req in the sequence as assembled when their assembled record matches the newest assemble round,
	})
	if len(preReqsPending) > 0 {
		return &types.TxProcessPreReq{
			TxIDs: preReqsPending,
		}
	}
	return nil
}

func (as *AssembleStage) ProcessEvents(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService, stageEvents []*types.StageEvent) (unprocessedStageEvents []*types.StageEvent, txUpdates *transactionstore.TransactionUpdate, nextStep types.StageProcessNextStep) {
	unprocessedStageEvents = []*types.StageEvent{}
	nextStep = types.NextStepWait
	for _, se := range stageEvents {
		if string(se.Stage) == as.Name() { // the current stage does not care about events from other stages yet (may need to be for interrupts)
			if se.Data != nil {
				switch v := se.Data.(type) {
				case prototk.AssembleTransactionResponse:
					if v.AssemblyResult == prototk.AssembleTransactionResponse_OK {
						attPlan, err := json.Marshal(v.AttestationPlan)
						attPlanStr := string(attPlan)
						// transaction assembled, store the information into DB
						txUpdates = &transactionstore.TransactionUpdate{
							AssembledRound:  tsg.GetAssembledRound(ctx) + 1, // TODO. this should be in the assemble response
							PayloadJSON:     v.AssembledTransaction.String(),
							AttestationPlan: &attPlanStr,
							AssembleError:   err.Error(),
						}
					} else {
						txUpdates = &transactionstore.TransactionUpdate{
							AssembledRound: tsg.GetAssembledRound(ctx) + 1, // TODO. this should be in the assemble response
							PayloadJSON:    "",                             // wipe the previous assemble response as they are no longer valid
							AssembleError:  *v.RevertReason,
						}
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

func (as *AssembleStage) MatchStage(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) bool {
	// assembleRound := sfs.Sequencer().GetLatestAssembleRoundForTx(ctx, tsg.GetTxID(ctx)) // TODO: deal with a tx in multiple sequences
	// return tsg.GetAssembledRound(ctx) != assembleRound || tsg.GetPayloadJSON(ctx) == ""
	return tsg.GetPayloadJSON(ctx) == ""
}

func (as *AssembleStage) PerformAction(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) (actionOutput interface{}, actionTriggerErr error) {
	if as.GetIncompletePreReqTxIDs(ctx, tsg, sfs) != nil {
		return nil, i18n.NewError(ctx, msgs.MsgTransactionProcessorBlockedOnDependency, tsg.GetTxID(ctx), as.Name())
	}

	var assembleResponse *prototk.AssembleTransactionResponse

	assembleErr := sfs.StateStore().RunInDomainContext(tsg.GetDomainID(ctx), func(ctx context.Context, dsi statestore.DomainStateInterface) error {
		// todo delegate to domain to do state generation
		assembleResponse = &prototk.AssembleTransactionResponse{
			// dummy empty object for now
		}
		return nil
	})

	if assembleErr != nil {
		assembleErrStr := assembleErr.Error()
		return prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			RevertReason:   &assembleErrStr,
		}, nil
	} else if assembleResponse == nil {
		return nil, i18n.NewError(ctx, msgs.MsgTransactionProcessorEmptyAssembledResult, tsg.GetTxID(ctx))
	} else {
		return assembleResponse, nil
	}
}
