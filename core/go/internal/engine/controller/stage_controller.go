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

package controller

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/transactionstore"
)

type TxStageProcessor interface {
	ProcessEvents(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService, stageEvents []*enginespi.StageEvent) (unprocessedStageEvents []*enginespi.StageEvent, txUpdates *transactionstore.TransactionUpdate, nextStep enginespi.StageProcessNextStep)
	PerformAction(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) (actionOutput interface{}, actionTriggerErr error)
	GetIncompletePreReqTxIDs(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) *enginespi.TxProcessPreReq
	MatchStage(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) bool
	Name() string
}

type StageController interface {
	CalculateStage(ctx context.Context, tsg transactionstore.TxStateGetters) string // output the processing stage a transaction is on based on the transaction state
	ProcessEventsForStage(ctx context.Context, stage string, tsg transactionstore.TxStateGetters, stageEvents []*enginespi.StageEvent) (unprocessedStageEvents []*enginespi.StageEvent, txUpdates *transactionstore.TransactionUpdate, nextStep enginespi.StageProcessNextStep)
	PerformActionForStage(ctx context.Context, stage string, tsg transactionstore.TxStateGetters) (actionOutput interface{}, actionTriggerErr error)
	GetAllStages() []string
}

type PaladinStageController struct {
	stageFoundationService enginespi.StageFoundationService
	stageProcessors        map[string]TxStageProcessor
	stageNames             []string
}

func (psc *PaladinStageController) CalculateStage(ctx context.Context, tsg transactionstore.TxStateGetters) string {
	calculatedStage := ""
	for _, stage := range psc.stageNames {
		stageProcessor := psc.stageProcessors[stage]
		if stageProcessor.MatchStage(ctx, tsg, psc.stageFoundationService) {
			calculatedStage = stage
			break // we break as soon as we found a matching state, if a transaction could match more stages, we'll have random behavior, but we don't waste cycles to figure that out
		}
	}
	if calculatedStage == "" {
		panic(i18n.NewError(ctx, msgs.MsgTransactionProcessorUndeterminedStage, tsg.GetTxID(ctx)))
	}

	return calculatedStage
}

func (psc *PaladinStageController) ProcessEventsForStage(ctx context.Context, stage string, tsg transactionstore.TxStateGetters, stageEvents []*enginespi.StageEvent) (unprocessedStageEvents []*enginespi.StageEvent, txUpdates *transactionstore.TransactionUpdate, nextStep enginespi.StageProcessNextStep) {
	stageProcessor := psc.stageProcessors[stage]
	if stageProcessor == nil {
		panic(i18n.NewError(ctx, msgs.MsgTransactionProcessorInvalidStage, stage)) // This is a code bug, CalculateStage function should never return stage that doesn't have StageProcessor registered
	}
	return stageProcessor.ProcessEvents(ctx, tsg, psc.stageFoundationService, stageEvents)
}
func (psc *PaladinStageController) PerformActionForStage(ctx context.Context, stage string, tsg transactionstore.TxStateGetters) (actionOutput interface{}, actionTriggerErr error) {
	stageProcessor := psc.stageProcessors[stage]
	if stageProcessor == nil {
		panic(i18n.NewError(ctx, msgs.MsgTransactionProcessorInvalidStage, stage)) // This is a code bug, CalculateStage function should never return stage that doesn't have StageProcessor registered
	}
	txProcessPreReq := stageProcessor.GetIncompletePreReqTxIDs(ctx, tsg, psc.stageFoundationService)
	if txProcessPreReq != nil {
		psc.stageFoundationService.DependencyChecker().RegisterPreReqTrigger(ctx, tsg.GetTxID(ctx), txProcessPreReq)
		return
	}
	actionOutput, actionTriggerErr = stageProcessor.PerformAction(ctx, tsg, psc.stageFoundationService)
	return
}

func (psc *PaladinStageController) GetAllStages() []string {
	if psc.stageNames == nil {
		psc.stageNames = make([]string, 0, len(psc.stageProcessors))
		for key := range psc.stageProcessors {
			psc.stageNames = append(psc.stageNames, key)
		}
	}
	return psc.stageNames
}

func NewPaladinStageController(ctx context.Context, stageFoundationService enginespi.StageFoundationService, tsps []TxStageProcessor) StageController {
	stageProcessorsMap := map[string]TxStageProcessor{}
	stageNames := []string{}
	for _, tsp := range tsps {
		stageNames = append(stageNames, tsp.Name())
		stageProcessorsMap[tsp.Name()] = tsp
	}
	return &PaladinStageController{
		stageProcessors:        stageProcessorsMap,
		stageFoundationService: stageFoundationService,
		stageNames:             stageNames,
	}
}
