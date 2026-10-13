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
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.IncCompletedTransactions()
	metrics.IncCompletedTransactions()
	metrics.IncCompletedTransactions()
	metrics.IncCompletedTransactions()
	metrics.IncCompletedTransactionsByN(5)
	metrics.IncDBSubmittedTransactions()
	metrics.IncDBSubmittedTransactions()
	metrics.IncDBSubmittedTransactionsByN(5)

	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	byName := make(map[string]float64)
	for _, mf := range metricFamilies {
		if len(mf.GetMetric()) > 0 && mf.GetMetric()[0].GetCounter() != nil {
			byName[mf.GetName()] = mf.GetMetric()[0].GetCounter().GetValue()
		}
	}
	assert.Equal(t, float64(9), byName["public_transaction_manager_completed_txns_total"])
	assert.Equal(t, float64(7), byName["public_transaction_manager_db_submitted_txns_total"])
}

func TestRecordOperationMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)

	metrics.RecordOperationMetrics(context.Background(), "sign", "success", 0.05)
	metrics.RecordOperationMetrics(context.Background(), "sign", "fail", 0.01)
	metrics.RecordOperationMetrics(context.Background(), "send", "success", 0.1)

	mfs, err := registry.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "public_transaction_manager_operation_duration_seconds" {
			// three distinct label combinations: sign/success, sign/fail, send/success
			assert.Len(t, mf.GetMetric(), 3)
			return
		}
	}
	t.Fatal("operation_duration_seconds metric not found")
}

func TestRecordStageChangeMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)

	metrics.RecordStageChangeMetrics(context.Background(), "sign", 0.02)
	metrics.RecordStageChangeMetrics(context.Background(), "submit", 0.15)
	metrics.RecordStageChangeMetrics(context.Background(), "sign", 0.03)

	mfs, err := registry.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "public_transaction_manager_stage_duration_seconds" {
			// two distinct stage label values: "sign" and "submit"
			assert.Len(t, mf.GetMetric(), 2)
			for _, m := range mf.GetMetric() {
				if m.GetLabel()[0].GetValue() == "sign" {
					assert.Equal(t, uint64(2), m.GetHistogram().GetSampleCount())
				}
			}
			return
		}
	}
	t.Fatal("stage_duration_seconds metric not found")
}

func TestRecordInFlightTxQueueMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)

	stageCounts := map[string]int{
		"sign":   3,
		"submit": 5,
	}
	metrics.RecordInFlightTxQueueMetrics(context.Background(), stageCounts, 12)

	mfs, err := registry.Gather()
	require.NoError(t, err)

	foundByStage := false
	foundFree := false
	for _, mf := range mfs {
		switch mf.GetName() {
		case "public_transaction_manager_inflight_txns_by_stage":
			assert.Len(t, mf.GetMetric(), 2)
			foundByStage = true
		case "public_transaction_manager_inflight_txns_free":
			assert.Equal(t, float64(12), mf.GetMetric()[0].GetGauge().GetValue())
			foundFree = true
		}
	}
	assert.True(t, foundByStage, "inflight_txns_by_stage metric not found")
	assert.True(t, foundFree, "inflight_txns_free metric not found")
}

func TestRecordCompletedTransactionCountMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)

	metrics.RecordCompletedTransactionCountMetrics(context.Background(), "success")
	metrics.RecordCompletedTransactionCountMetrics(context.Background(), "success")
	metrics.RecordCompletedTransactionCountMetrics(context.Background(), "fail")

	mfs, err := registry.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "public_transaction_manager_completed_txns_by_status_total" {
			assert.Len(t, mf.GetMetric(), 2)
			for _, m := range mf.GetMetric() {
				if m.GetLabel()[0].GetValue() == "success" {
					assert.Equal(t, float64(2), m.GetCounter().GetValue())
				} else {
					assert.Equal(t, float64(1), m.GetCounter().GetValue())
				}
			}
			return
		}
	}
	t.Fatal("completed_txns_by_status_total metric not found")
}

func TestRecordInFlightOrchestratorPoolMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)

	stateCounts := map[string]int{
		"running": 4,
		"waiting": 2,
		"idle":    1,
	}
	metrics.RecordInFlightOrchestratorPoolMetrics(context.Background(), stateCounts, 3)

	mfs, err := registry.Gather()
	require.NoError(t, err)

	foundByState := false
	foundFree := false
	for _, mf := range mfs {
		switch mf.GetName() {
		case "public_transaction_manager_inflight_orchestrators_by_state":
			assert.Len(t, mf.GetMetric(), 3)
			foundByState = true
		case "public_transaction_manager_inflight_orchestrators_free":
			assert.Equal(t, float64(3), mf.GetMetric()[0].GetGauge().GetValue())
			foundFree = true
		}
	}
	assert.True(t, foundByState, "inflight_orchestrators_by_state metric not found")
	assert.True(t, foundFree, "inflight_orchestrators_free metric not found")
}
