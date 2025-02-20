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

	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

type inFlightTransactionState struct {
	PublicTxManagerMetricsManager
	BalanceManager
	InMemoryTxStateManager

	orchestratorContext *OrchestratorContext
	versions            []InFlightTransactionStateVersion

	// not used by this struct but passed down into versions
	testOnlyNoEventMode bool
	InFlightStageActionTriggers
	statusUpdater    StatusUpdater
	submissionWriter *submissionWriter
}

func (iftxs *inFlightTransactionState) GetVersions(ctx context.Context) []InFlightTransactionStateVersion {
	return iftxs.versions
}

func (iftxs *inFlightTransactionState) GetVersion(ctx context.Context, id int) InFlightTransactionStateVersion {
	return iftxs.versions[id]
}

func (iftxs *inFlightTransactionState) GetCurrentVersion(ctx context.Context) InFlightTransactionStateVersion {
	return iftxs.versions[len(iftxs.versions)-1]
}

func (iftxs *inFlightTransactionState) NewVersion(ctx context.Context) {
	iftxs.versions[len(iftxs.versions)-1].SetCurrent(ctx, false)
	iftxs.versions[len(iftxs.versions)-1].Cancel(ctx)
	iftxs.versions = append(iftxs.versions, NewInFlightTransactionStateVersion(
		len(iftxs.versions),
		iftxs.PublicTxManagerMetricsManager,
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
	runningStages := false
	for _, version := range iftxs.versions {
		runningStages = runningStages || version.GetRunningStageContext(ctx) != nil
	}
	return iftxs.IsReadyToExit() && !runningStages
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
	return iftxs.GetCurrentVersion(ctx).GetStage(ctx)
}

func NewInFlightTransactionStateManager(thm PublicTxManagerMetricsManager,
	bm BalanceManager,
	ifsat InFlightStageActionTriggers,
	imtxs InMemoryTxStateManager,
	statusUpdater StatusUpdater,
	submissionWriter *submissionWriter,
	noEventMode bool,
) InFlightTransactionStateManager {
	return &inFlightTransactionState{
		PublicTxManagerMetricsManager: thm,
		BalanceManager:                bm,
		versions: []InFlightTransactionStateVersion{
			NewInFlightTransactionStateVersion(0, thm, bm, ifsat, imtxs, statusUpdater, submissionWriter, noEventMode),
		},
		InMemoryTxStateManager:      imtxs,
		InFlightStageActionTriggers: ifsat,
		statusUpdater:               statusUpdater,
		submissionWriter:            submissionWriter,
		testOnlyNoEventMode:         noEventMode,
	}
}
