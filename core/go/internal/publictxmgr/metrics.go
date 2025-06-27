/*
 * Copyright © 2025 Kaleido, Inc.
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

	"github.com/kaleido-io/paladin/common/go/pkg/log"
)

type PublicTxManagerMetricsManager interface {
	InitMetrics(ctx context.Context)
	RecordOperationMetrics(ctx context.Context, operationName string, operationResult string, durationInSeconds float64)
	RecordStageChangeMetrics(ctx context.Context, stage string, durationInSeconds float64)
	RecordInFlightTxQueueMetrics(ctx context.Context, usedCountPerStage map[string]int, freeCount int)
	RecordCompletedTransactionCountMetrics(ctx context.Context, processStatus string)
}

type publicTxEngineMetrics struct {
}

func (thm *publicTxEngineMetrics) InitMetrics(ctx context.Context) {
	log.L(ctx).Tracef("Init metrics")
	// TODO
}

func (thm *publicTxEngineMetrics) RecordOperationMetrics(ctx context.Context, operationName string, operationResult string, durationInSeconds float64) {
	log.L(ctx).Tracef("RecordOperationMetrics")
	// TODO
}

func (thm *publicTxEngineMetrics) RecordStageChangeMetrics(ctx context.Context, stage string, durationInSeconds float64) {
	log.L(ctx).Tracef("RecordStageChangeMetrics")
	// TODO
}

func (thm *publicTxEngineMetrics) RecordInFlightOrchestratorPoolMetrics(ctx context.Context, usedCountPerState map[string]int, freeCount int) {
	log.L(ctx).Tracef("RecordInFlightEnginePoolMetrics")
	// TODO
}

func (thm *publicTxEngineMetrics) RecordInFlightTxQueueMetrics(ctx context.Context, usedCountPerStage map[string]int, freeCount int) {
	log.L(ctx).Tracef("RecordInFlightTxQueueMetrics")
	// TODO
}

func (thm *publicTxEngineMetrics) RecordCompletedTransactionCountMetrics(ctx context.Context, processStatus string) {
	log.L(ctx).Tracef("RecordCompletedTransactionCountMetrics")
	// TODO
}
