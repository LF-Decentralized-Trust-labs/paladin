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

package engine

import (
	"context"
	"time"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
)

type TxStageProcessor interface {
	ProcessEvents(ctx context.Context, tsg transactionstore.TxStateGetters, stageEvents []*StageEvent) (unprocessedStageEvents []*StageEvent, txUpdates *transactionstore.TransactionUpdate, stageCompleted bool)
	TriggerAction(ctx context.Context, tsg transactionstore.TxStateGetters) (actionOutput interface{}, actionErr error)
}

type TxProcessPreReq struct {
	TxIDs   string
	Timeout *time.Duration
}

type StageController interface {
	CalculateStage(ctx context.Context, tsg transactionstore.TxStateGetters) (TxStageName, *TxProcessPreReq) // output the processing stage a transaction is on based on the transaction state
	ProcessEventsForStage(ctx context.Context, stage string, tsg transactionstore.TxStateGetters, stageEvents []*StageEvent) (unprocessedStageEvents []*StageEvent, txUpdates *transactionstore.TransactionUpdate, stageCompleted bool)
	TriggerActionForStage(ctx context.Context, stage string, tsg transactionstore.TxStateGetters) (actionOutput interface{}, actionErr error)
}

type PaladinStageController struct {
	stageProcessors map[string]TxStageProcessor
}

func (psc *PaladinStageController) CalculateStage(ctx context.Context, tsg transactionstore.TxStateGetters) (TxStageName, *TxProcessPreReq) {
	// TODO: output the processing stage a transaction is on based on the transaction state
	return "test", nil
}

func (psc *PaladinStageController) ProcessEventsForStage(ctx context.Context, stage string, tsg transactionstore.TxStateGetters, stageEvents []*StageEvent) (unprocessedStageEvents []*StageEvent, txUpdates *transactionstore.TransactionUpdate, stageCompleted bool) {
	stageProcessor := psc.stageProcessors[stage]
	if stageProcessor == nil {
		panic(i18n.NewError(ctx, msgs.MsgTransactionProcessorInvalidStage, stage)) // This is a code bug, CalculateStage function should never return stage that doesn't have StageProcessor registered
	}
	return stageProcessor.ProcessEvents(ctx, tsg, stageEvents)
}
func (psc *PaladinStageController) TriggerActionForStage(ctx context.Context, stage string, tsg transactionstore.TxStateGetters) (actionOutput interface{}, actionErr error) {
	stageProcessor := psc.stageProcessors[stage]
	if stageProcessor == nil {
		panic(i18n.NewError(ctx, msgs.MsgTransactionProcessorInvalidStage, stage)) // This is a code bug, CalculateStage function should never return stage that doesn't have StageProcessor registered
	}
	return stageProcessor.TriggerAction(ctx, tsg)
}

func NewStageController(ctx context.Context) StageController {
	return &PaladinStageController{}
}
