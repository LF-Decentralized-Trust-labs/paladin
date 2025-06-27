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

package metrics

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
)

type PublicTransactionManagerMetrics interface {
	IncSubmittedTransactions()
	IncCompletedTransactions()
}

var METRICS_SUBSYSTEM = "public_transaction_manager"

type publicTransactionManagerMetrics struct {
	submittedTransactions prometheus.Counter
	completedTransactions prometheus.Counter
}

func InitMetrics(ctx context.Context, registry *prometheus.Registry) *publicTransactionManagerMetrics {
	metrics := &publicTransactionManagerMetrics{}

	metrics.submittedTransactions = prometheus.NewCounter(prometheus.CounterOpts{Name: "submitted_txns_total",
		Help: "Public transaction manager submitted transactions", Subsystem: METRICS_SUBSYSTEM})
	metrics.completedTransactions = prometheus.NewCounter(prometheus.CounterOpts{Name: "completed_txns_total",
		Help: "Public transaction manager completed transactions", Subsystem: METRICS_SUBSYSTEM})

	registry.MustRegister(metrics.submittedTransactions)
	registry.MustRegister(metrics.completedTransactions)
	return metrics
}

func (dtm *publicTransactionManagerMetrics) IncSubmittedTransactions() {
	dtm.submittedTransactions.Inc()
}

func (dtm *publicTransactionManagerMetrics) IncCompletedTransactions() {
	dtm.completedTransactions.Inc()
}
