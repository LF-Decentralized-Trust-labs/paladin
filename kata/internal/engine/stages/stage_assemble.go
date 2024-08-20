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
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
)

type AssembleResult struct {
}

type AssembleStage struct{}

func (as *AssembleStage) Name() string {
	return "dispatch"
}

func (as *AssembleStage) GetIncompletePreReqTxIDs(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) *types.TxProcessPreReq {
	sequencePreReqsToCheck := tsg.GetSequencePreReqTransactions(ctx)                    // only get the pre-req in the sequence
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

func (as *AssembleStage) MatchStage(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) bool {
	assembleRound := sfs.Sequencer().GetLatestAssembleRoundForTx(ctx, tsg.GetTxID(ctx)) // TODO: deal with a tx in multiple sequences
	return tsg.GetAssembledRound(ctx) != assembleRound
}

func (as *AssembleStage) PerformAction(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) (actionOutput interface{}, actionErr error) {
	if as.GetIncompletePreReqTxIDs(ctx, tsg, sfs) != nil {
		return nil, i18n.NewError(ctx, msgs.MsgTransactionProcessorBlockedOnDependency, tsg.GetTxID(ctx), as.Name())
	}

	sfs.StateStore().RunInDomainContext(tsg.GetDomainID(ctx), func(ctx context.Context, dsi statestore.DomainStateInterface) error {
		return nil
	})

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

	return nil, i18n.NewError(ctx, msgs.MsgTransactionProcessorActionFailed, tsg.GetTxID(ctx), as.Name())
}
