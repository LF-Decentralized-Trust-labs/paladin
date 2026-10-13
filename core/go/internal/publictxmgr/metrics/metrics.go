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

	RecordOperationMetrics(ctx context.Context, operationName string, operationResult string, durationInSeconds float64)
	RecordStageChangeMetrics(ctx context.Context, stage string, durationInSeconds float64)
	RecordInFlightTxQueueMetrics(ctx context.Context, usedCountPerStage map[string]int, freeCount int)
	RecordCompletedTransactionCountMetrics(ctx context.Context, processStatus string)
	RecordInFlightOrchestratorPoolMetrics(ctx context.Context, usedCountPerState map[string]int, freeCount int)
}

var METRICS_SUBSYSTEM = "public_transaction_manager"

// durationBuckets are second-based buckets covering sub-millisecond to multi-minute ranges
// typical for signing, gas estimation, and submission operations.
var durationBuckets = []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60}

type publicTransactionManagerMetrics struct {
	dbSubmittedTransactions   prometheus.Counter
	completedTransactions     prometheus.Counter
	operationDuration         *prometheus.HistogramVec
	stageDuration             *prometheus.HistogramVec
	inflightTxsByStage        *prometheus.GaugeVec
	inflightTxsFree           prometheus.Gauge
	completedTxsByStatus      *prometheus.CounterVec
	inflightOrchestratorState *prometheus.GaugeVec
	inflightOrchestratorFree  prometheus.Gauge
}

func InitMetrics(ctx context.Context, registry *prometheus.Registry) *publicTransactionManagerMetrics {
	metrics := &publicTransactionManagerMetrics{}

	metrics.dbSubmittedTransactions = prometheus.NewCounter(prometheus.CounterOpts{Name: "db_submitted_txns_total",
		Help: "Public transaction manager transactions submitted to the DB", Subsystem: METRICS_SUBSYSTEM})
	metrics.completedTransactions = prometheus.NewCounter(prometheus.CounterOpts{Name: "completed_txns_total",
		Help: "Public transaction manager completed transactions", Subsystem: METRICS_SUBSYSTEM})
	metrics.operationDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "operation_duration_seconds",
		Help: "Duration of public transaction manager operations in seconds", Subsystem: METRICS_SUBSYSTEM, Buckets: durationBuckets},
		[]string{"operation", "result"})
	metrics.stageDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "stage_duration_seconds",
		Help: "Duration of each in-flight transaction stage in seconds", Subsystem: METRICS_SUBSYSTEM, Buckets: durationBuckets},
		[]string{"stage"})
	metrics.inflightTxsByStage = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "inflight_txns_by_stage",
		Help: "Number of in-flight transactions per stage", Subsystem: METRICS_SUBSYSTEM},
		[]string{"stage"})
	metrics.inflightTxsFree = prometheus.NewGauge(prometheus.GaugeOpts{Name: "inflight_txns_free",
		Help: "Number of free slots in the in-flight transaction queue", Subsystem: METRICS_SUBSYSTEM})
	metrics.completedTxsByStatus = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "completed_txns_by_status_total",
		Help: "Public transaction manager completed transactions by status", Subsystem: METRICS_SUBSYSTEM},
		[]string{"status"})
	metrics.inflightOrchestratorState = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "inflight_orchestrators_by_state",
		Help: "Number of in-flight orchestrators per state", Subsystem: METRICS_SUBSYSTEM},
		[]string{"state"})
	metrics.inflightOrchestratorFree = prometheus.NewGauge(prometheus.GaugeOpts{Name: "inflight_orchestrators_free",
		Help: "Number of free slots in the orchestrator pool", Subsystem: METRICS_SUBSYSTEM})

	registry.MustRegister(metrics.dbSubmittedTransactions)
	registry.MustRegister(metrics.completedTransactions)
	registry.MustRegister(metrics.operationDuration)
	registry.MustRegister(metrics.stageDuration)
	registry.MustRegister(metrics.inflightTxsByStage)
	registry.MustRegister(metrics.inflightTxsFree)
	registry.MustRegister(metrics.completedTxsByStatus)
	registry.MustRegister(metrics.inflightOrchestratorState)
	registry.MustRegister(metrics.inflightOrchestratorFree)

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
	ptm.operationDuration.With(prometheus.Labels{"operation": operationName, "result": operationResult}).Observe(durationInSeconds)
}

func (ptm *publicTransactionManagerMetrics) RecordStageChangeMetrics(ctx context.Context, stage string, durationInSeconds float64) {
	ptm.stageDuration.With(prometheus.Labels{"stage": stage}).Observe(durationInSeconds)
}

func (ptm *publicTransactionManagerMetrics) RecordInFlightTxQueueMetrics(ctx context.Context, usedCountPerStage map[string]int, freeCount int) {
	for stage, count := range usedCountPerStage {
		ptm.inflightTxsByStage.With(prometheus.Labels{"stage": stage}).Set(float64(count))
	}
	ptm.inflightTxsFree.Set(float64(freeCount))
}

func (ptm *publicTransactionManagerMetrics) RecordCompletedTransactionCountMetrics(ctx context.Context, processStatus string) {
	ptm.completedTxsByStatus.With(prometheus.Labels{"status": processStatus}).Inc()
}

func (ptm *publicTransactionManagerMetrics) RecordInFlightOrchestratorPoolMetrics(ctx context.Context, usedCountPerState map[string]int, freeCount int) {
	for state, count := range usedCountPerState {
		ptm.inflightOrchestratorState.With(prometheus.Labels{"state": state}).Set(float64(count))
	}
	ptm.inflightOrchestratorFree.Set(float64(freeCount))
}
