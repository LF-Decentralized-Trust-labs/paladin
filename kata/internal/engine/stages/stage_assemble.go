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
	"github.com/kaleido-io/paladin/kata/pkg/proto/sequence"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type AssembleStage struct {
	sequencer types.Sequencer
}

func NewAssembleStage(sequencer types.Sequencer) *AssembleStage {
	return &AssembleStage{
		sequencer: sequencer,
	}
}
func (as *AssembleStage) Name() string {
	return "assemble"
}

func (as *AssembleStage) GetIncompletePreReqTxIDs(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) *types.TxProcessPreReq {
	//TODO for now we don't have any pre-reqs for assemble stage

	return nil
}

type assembleComplete struct {
}

func (as *AssembleStage) ProcessEvents(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService, stageEvents []*types.StageEvent) (unprocessedStageEvents []*types.StageEvent, txUpdates *transactionstore.TransactionUpdate, nextStep types.StageProcessNextStep) {
	tx := tsg.HACKGetPrivateTx()

	unprocessedStageEvents = []*types.StageEvent{}
	if len(stageEvents) > 0 {
		for _, event := range stageEvents {

			switch event.Data.(type) {
			case assembleComplete:
				err := as.sequencer.OnTransactionAssembled(ctx, &sequence.TransactionAssembledEvent{
					TransactionId: tx.ID.String(),
					NodeId:        "todo",
					InputStateID:  []string{},
					OutputStateID: []string{},
				})
				if err != nil {
					log.L(ctx).Errorf("OnTransactionAssembled failed: %s", err)
					panic("todo")
				}
				return nil, nil, types.NextStepNewStage
			default:
				unprocessedStageEvents = append(unprocessedStageEvents, event)
			}
		}
		return stageEvents, nil, types.NextStepNewStage
	}
	return nil, nil, types.NextStepWait
}

func (as *AssembleStage) MatchStage(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) bool {
	tx := tsg.HACKGetPrivateTx()

	// if we have a private transaction but do not have a post assemble payload, we are in the assemble stage
	return tx != nil && tx.PostAssembly == nil

}

func (as *AssembleStage) PerformAction(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) (actionOutput interface{}, actionTriggerErr error) {
	// temporary hack.  components.PrivateTx should be passed in as a parameter
	tx := tsg.HACKGetPrivateTx()
	//TODO assembly must be single threaded ( at least single thread per domain contract)
	// can we assume that we are already on a single thread or do we need to delegate to a single thread here
	if as.GetIncompletePreReqTxIDs(ctx, tsg, sfs) != nil {
		return nil, i18n.NewError(ctx, msgs.MsgTransactionProcessorBlockedOnDependency, tsg.GetTxID(ctx), as.Name())
	}

	err := sfs.DomainAPI().AssembleTransaction(ctx, tx)
	if err != nil {
		log.L(ctx).Errorf("AssembleTransaction failed: %s", err)
		return nil, i18n.WrapError(ctx, err, msgs.MsgEngineAssembleError)
	}
	switch tx.PostAssembly.AssemblyResult {
	case prototk.AssembleTransactionResponse_OK:
		return assembleComplete{}, nil

	default:
		log.L(ctx).Errorf("assemble result was %s", tx.PostAssembly.AssemblyResult)
		return nil, i18n.NewError(ctx, msgs.MsgEngineAssembleError, tx.PostAssembly.AssemblyResult)

	}
}
