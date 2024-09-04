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
	"sync"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
	"github.com/kaleido-io/paladin/kata/pkg/proto/sequence"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type AssembleStage struct {
	sequencer enginespi.Sequencer
	nodeID    string
	lock      sync.Mutex
}

func NewAssembleStage(sequencer enginespi.Sequencer, nodeID string) *AssembleStage {
	return &AssembleStage{
		sequencer: sequencer,
		nodeID:    nodeID,
	}
}
func (as *AssembleStage) Name() string {
	return "assemble"
}

func (as *AssembleStage) GetIncompletePreReqTxIDs(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) *enginespi.TxProcessPreReq {
	//TODO for now we don't have any pre-reqs for assemble stage

	return nil
}

type assembleComplete struct {
}

func stateIDs(states []*components.FullState) []string {
	stateIDs := make([]string, 0, len(states))
	for _, state := range states {
		stateIDs = append(stateIDs, state.ID.String())
	}
	return stateIDs
}

func (as *AssembleStage) ProcessEvents(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService, stageEvents []*enginespi.StageEvent) (unprocessedStageEvents []*enginespi.StageEvent, txUpdates *transactionstore.TransactionUpdate, nextStep enginespi.StageProcessNextStep) {
	tx := tsg.HACKGetPrivateTx()

	unprocessedStageEvents = []*enginespi.StageEvent{}
	if len(stageEvents) > 0 {
		for _, event := range stageEvents {

			switch event.Data.(type) {
			case assembleComplete:

				err := as.sequencer.HandleTransactionAssembledEvent(ctx, &sequence.TransactionAssembledEvent{
					TransactionId: tx.ID.String(),
					NodeId:        as.nodeID,
					InputStateId:  stateIDs(tx.PostAssembly.InputStates),
					OutputStateId: stateIDs(tx.PostAssembly.OutputStates),
				})
				if err != nil {
					log.L(ctx).Errorf("HandleTransactionAssembledEvent failed: %s", err)
					panic("todo")
				}
				return nil, nil, enginespi.NextStepNewStage
			default:
				unprocessedStageEvents = append(unprocessedStageEvents, event)
			}
		}
		return stageEvents, nil, enginespi.NextStepNewStage
	}
	return nil, nil, enginespi.NextStepWait
}

func (as *AssembleStage) MatchStage(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) bool {
	tx := tsg.HACKGetPrivateTx()

	// if we have a private transaction but do not have a post assemble payload, we are in the assemble stage
	return tx != nil && tx.PostAssembly == nil

}

func (as *AssembleStage) PerformAction(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) (actionOutput interface{}, actionTriggerErr error) {
	// temporary hack.  components.PrivateTx should be passed in as a parameter
	tx := tsg.HACKGetPrivateTx()
	log.L(ctx).Debugf("AssembleStage.PerformAction tx: %s", tx.ID.String())

	if as.GetIncompletePreReqTxIDs(ctx, tsg, sfs) != nil {
		return nil, i18n.NewError(ctx, msgs.MsgTransactionProcessorBlockedOnDependency, tsg.GetTxID(ctx), as.Name())
	}

	//assembly must be single threaded ( at least single thread per domain contract)
	as.lock.Lock()
	defer as.lock.Unlock()
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
