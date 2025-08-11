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

package metrics

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
)

type PublicTransactionManagerMetrics interface {
	IncDBSubmittedTransactions()
	IncDBSubmittedTransactionsByN(numberOfTransactions uint64)
	IncCompletedTransactions()
	IncCompletedTransactionsByN(numberOfTransactions uint64)

	// TODO - TX manager currently expects these, currently they need implementing
	RecordOperationMetrics(ctx context.Context, operationName string, operationResult string, durationInSeconds float64)
	RecordStageChangeMetrics(ctx context.Context, stage string, durationInSeconds float64)
	RecordInFlightTxQueueMetrics(ctx context.Context, usedCountPerStage map[string]int, freeCount int)
	RecordCompletedTransactionCountMetrics(ctx context.Context, processStatus string)
	RecordInFlightOrchestratorPoolMetrics(ctx context.Context, usedCountPerState map[string]int, freeCount int)
}

var METRICS_SUBSYSTEM = "public_transaction_manager"

type publicTransactionManagerMetrics struct {
	dbSubmittedTransactions prometheus.Counter
	completedTransactions   prometheus.Counter
}

func InitMetrics(ctx context.Context, registry *prometheus.Registry) *publicTransactionManagerMetrics {
	metrics := &publicTransactionManagerMetrics{}

	metrics.dbSubmittedTransactions = prometheus.NewCounter(prometheus.CounterOpts{Name: "db_submitted_txns_total",
		Help: "Public transaction manager transactions submitted to the DB", Subsystem: METRICS_SUBSYSTEM})
	metrics.completedTransactions = prometheus.NewCounter(prometheus.CounterOpts{Name: "completed_txns_total",
		Help: "Public transaction manager completed transactions", Subsystem: METRICS_SUBSYSTEM})

	registry.MustRegister(metrics.dbSubmittedTransactions)
	registry.MustRegister(metrics.completedTransactions)
	return metrics
}

func (ptm *publicTransactionManagerMetrics) IncDBSubmittedTransactions() {
	ptm.dbSubmittedTransactions.Inc()
}

func (ptm *publicTransactionManagerMetrics) IncDBSubmittedTransactionsByN(numberOfTransactions uint64) {
	ptm.dbSubmittedTransactions.Add(float64(numberOfTransactions))
}

func (ptm *publicTransactionManagerMetrics) IncCompletedTransactions() {
	ptm.completedTransactions.Inc()
}

func (ptm *publicTransactionManagerMetrics) IncCompletedTransactionsByN(numberOfTransactions uint64) {
	ptm.completedTransactions.Add(float64(numberOfTransactions))
}

func (ptm *publicTransactionManagerMetrics) RecordOperationMetrics(ctx context.Context, operationName string, operationResult string, durationInSeconds float64) {
	// TODO
}

func (ptm *publicTransactionManagerMetrics) RecordStageChangeMetrics(ctx context.Context, stage string, durationInSeconds float64) {
	// TODO
}

func (ptm *publicTransactionManagerMetrics) RecordInFlightTxQueueMetrics(ctx context.Context, usedCountPerStage map[string]int, freeCount int) {
	// TODO
}

func (ptm *publicTransactionManagerMetrics) RecordCompletedTransactionCountMetrics(ctx context.Context, processStatus string) {
	// TODO
}

func (ptm *publicTransactionManagerMetrics) RecordInFlightOrchestratorPoolMetrics(ctx context.Context, usedCountPerState map[string]int, freeCount int) {
	// TODO
}
