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

	"github.com/kaleido-io/paladin/core/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/core/internal/transactionstore"
)

type DispatchAddress string

type TxSubmissionOutput struct {
	ErrorMessage  string
	TransactionID string
}

type DispatchStage struct{}

func (ds *DispatchStage) Name() string {
	return "dispatch"
}

func (ds *DispatchStage) GetIncompletePreReqTxIDs(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) *enginespi.TxProcessPreReq {
	return nil
}

func (ds *DispatchStage) ProcessEvents(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService, stageEvents []*enginespi.StageEvent) (unprocessedStageEvents []*enginespi.StageEvent, txUpdates *transactionstore.TransactionUpdate, nextStep enginespi.StageProcessNextStep) {
	unprocessedStageEvents = []*enginespi.StageEvent{}
	nextStep = enginespi.NextStepWait
	for _, se := range stageEvents {
		if string(se.Stage) != ds.Name() {
			unprocessedStageEvents = append(unprocessedStageEvents, se)
		}
	}
	return
}

func (ds *DispatchStage) MatchStage(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) bool {
	tx := tsg.HACKGetPrivateTx()
	return tx.Signer != ""
}

func (ds *DispatchStage) PerformAction(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) (actionOutput interface{}, actionTriggerErr error) {
	//for now, we just get stuck in the dispatch stage indefinitely
	//there is code coming in another branch to replace the dispatch stage with a number of new stages to nurture the transaction throught to sumbission and confirmation on the base ledger
	return nil, nil
}
