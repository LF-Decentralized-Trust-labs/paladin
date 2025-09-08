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

package publictxmgr

import (
	"context"
	"math/big"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/publictxmgr/metrics"
)

type inFlightTransactionState struct {
	metrics.PublicTransactionManagerMetrics
	BalanceManager
	InMemoryTxStateManager

	orchestratorContext *OrchestratorContext
	generations         []InFlightTransactionStateGeneration

	// not used by this struct but passed down into versions
	testOnlyNoEventMode bool
	InFlightStageActionTriggers
	statusUpdater    StatusUpdater
	submissionWriter *submissionWriter
}

func (iftxs *inFlightTransactionState) GetGenerations(ctx context.Context) []InFlightTransactionStateGeneration {
	return iftxs.generations
}

func (iftxs *inFlightTransactionState) GetGeneration(ctx context.Context, id int) InFlightTransactionStateGeneration {
	return iftxs.generations[id]
}

func (iftxs *inFlightTransactionState) GetCurrentGeneration(ctx context.Context) InFlightTransactionStateGeneration {
	return iftxs.generations[len(iftxs.generations)-1]
}

func (iftxs *inFlightTransactionState) GetPreviousGenerations(ctx context.Context) []InFlightTransactionStateGeneration {
	if len(iftxs.generations) < 2 {
		return []InFlightTransactionStateGeneration{}
	}
	return iftxs.generations[:len(iftxs.generations)-1]
}

func (iftxs *inFlightTransactionState) NewGeneration(ctx context.Context) {
	iftxs.generations[len(iftxs.generations)-1].SetCurrent(ctx, false)
	iftxs.generations[len(iftxs.generations)-1].Cancel(ctx)
	iftxs.generations = append(iftxs.generations, NewInFlightTransactionStateGeneration(
		iftxs.PublicTransactionManagerMetrics,
		iftxs.BalanceManager,
		iftxs.InFlightStageActionTriggers,
		iftxs.InMemoryTxStateManager,
		iftxs.statusUpdater,
		iftxs.submissionWriter,
		iftxs.testOnlyNoEventMode,
	))
}

// I think the answer is to look at all the stage outputs and if they're gone then it can be removed
func (iftxs *inFlightTransactionState) CanBeRemoved(ctx context.Context) bool {
	return iftxs.IsReadyToExit()
}

func (iftxs *inFlightTransactionState) CanSubmit(ctx context.Context, cost *big.Int) bool {
	log.L(ctx).Tracef("ProcessInFlightTransaction transaction entry, transaction orchestrator context: %+v, cost: %s", iftxs.orchestratorContext, cost.String())
	if iftxs.orchestratorContext.AvailableToSpend == nil {
		log.L(ctx).Tracef("ProcessInFlightTransaction transaction can be submitted for zero gas price chain, orchestrator context: %+v", iftxs.orchestratorContext)
		return true
	}
	if cost != nil {
		return iftxs.orchestratorContext.AvailableToSpend.Cmp(cost) != -1 && !iftxs.orchestratorContext.PreviousNonceCostUnknown
	}
	log.L(ctx).Debugf("ProcessInFlightTransaction cannot submit transaction, transaction orchestrator context: %+v, cost: %s", iftxs.orchestratorContext, cost.String())
	return false
}

func (iftxs *inFlightTransactionState) SetOrchestratorContext(ctx context.Context, tec *OrchestratorContext) {
	iftxs.orchestratorContext = tec
}

func (iftxs *inFlightTransactionState) GetStage(ctx context.Context) InFlightTxStage {
	return iftxs.GetCurrentGeneration(ctx).GetStage(ctx)
}

func NewInFlightTransactionStateManager(thm metrics.PublicTransactionManagerMetrics,
	bm BalanceManager,
	ifsat InFlightStageActionTriggers,
	imtxs InMemoryTxStateManager,
	statusUpdater StatusUpdater,
	submissionWriter *submissionWriter,
	noEventMode bool,
) InFlightTransactionStateManager {
	return &inFlightTransactionState{
		PublicTransactionManagerMetrics: thm,
		BalanceManager:                  bm,
		generations: []InFlightTransactionStateGeneration{
			NewInFlightTransactionStateGeneration(thm, bm, ifsat, imtxs, statusUpdater, submissionWriter, noEventMode),
		},
		InMemoryTxStateManager:      imtxs,
		InFlightStageActionTriggers: ifsat,
		statusUpdater:               statusUpdater,
		submissionWriter:            submissionWriter,
		testOnlyNoEventMode:         noEventMode,
	}
}
