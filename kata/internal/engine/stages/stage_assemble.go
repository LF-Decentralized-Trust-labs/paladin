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
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
)

type AssembleStage struct{}

func (as *AssembleStage) Name() string {
	return "assemble"
}

func (as *AssembleStage) GetIncompletePreReqTxIDs(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) *types.TxProcessPreReq {
	//TODO for now we don't have any pre-reqs for assemble stage

	return nil
}

func (as *AssembleStage) ProcessEvents(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService, stageEvents []*types.StageEvent) (unprocessedStageEvents []*types.StageEvent, txUpdates *transactionstore.TransactionUpdate, nextStep types.StageProcessNextStep) {
	if len(stageEvents) > 0 {
		log.L(ctx).Errorf("Assemble stage does not expect any events, but got %d", len(stageEvents))
		return stageEvents, nil, types.NextStepWait
	}
	return nil, nil, types.NextStepWait
}

func (as *AssembleStage) MatchStage(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) bool {

	// if we have a private transaction but do not have a post assemble payload, we are in the assemble stage
	return tsg.HACKGetPrivateTx() != nil && tsg.HACKGetPrivateTx().PostAssembly == nil

}

func (as *AssembleStage) PerformAction(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) (actionOutput interface{}, actionTriggerErr error) {
	//TODO assembly must be single threaded ( at least single thread per domain contract)
	// can we assume that we are already on a single thread or do we need to delegate to a single thread here
	if as.GetIncompletePreReqTxIDs(ctx, tsg, sfs) != nil {
		return nil, i18n.NewError(ctx, msgs.MsgTransactionProcessorBlockedOnDependency, tsg.GetTxID(ctx), as.Name())
	}

	err := sfs.DomainAPI().AssembleTransaction(ctx, tsg.HACKGetPrivateTx())
	if err != nil {
		log.L(ctx).Errorf("AssembleTransaction failed: %s", err)
		return nil, i18n.WrapError(ctx, err, msgs.MsgEngineAssembleError)
	}
	return nil, nil
}
