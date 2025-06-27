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

type DistributedSequencerMetrics interface {
	IncAssembledTransactions()
	IncDispatchedTransactions()
}

var METRICS_SUBSYSTEM = "distributed_sequencer"

type distributedSequencerMetrics struct {
	assembledTransactions  prometheus.Counter
	dispatchedTransactions prometheus.Counter
}

func InitMetrics(ctx context.Context, registry *prometheus.Registry) *distributedSequencerMetrics {
	metrics := &distributedSequencerMetrics{}

	metrics.assembledTransactions = prometheus.NewCounter(prometheus.CounterOpts{Name: "assembled_txns_total",
		Help: "Distributed sequencer assembled transactions", Subsystem: METRICS_SUBSYSTEM})
	metrics.dispatchedTransactions = prometheus.NewCounter(prometheus.CounterOpts{Name: "dispatched_txns_total",
		Help: "Distributed sequencer dispatched transactions", Subsystem: METRICS_SUBSYSTEM})

	registry.MustRegister(metrics.assembledTransactions)
	registry.MustRegister(metrics.dispatchedTransactions)
	return metrics
}

func (dtm *distributedSequencerMetrics) IncAssembledTransactions() {
	dtm.assembledTransactions.Inc()
}

func (dtm *distributedSequencerMetrics) IncDispatchedTransactions() {
	dtm.dispatchedTransactions.Inc()
}
