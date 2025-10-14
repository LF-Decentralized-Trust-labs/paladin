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
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type DistributedSequencerMetrics interface {
	IncAcceptedTransactions()
	IncAssembledTransactions()
	IncEndorsedTransactions()
	IncDispatchedTransactions()
	IncConfirmedTransactions()
	IncRevertedTransactions()
	ObserveSequencerTXStateChange(state string, duration time.Duration)
	SetActiveCoordinators(numberOfActiveCoordinators int)
	SetActiveSequencers(numberOfActiveSequencers int)
	IncCoordinatingTransactions()
	DecCoordinatingTransactions()
}

var METRICS_SUBSYSTEM = "distributed_sequencer"

type distributedSequencerMetrics struct {
	acceptedTransactions     prometheus.Counter
	assembledTransactions    prometheus.Counter
	endorsedTransactions     prometheus.Counter
	dispatchedTransactions   prometheus.Counter
	confirmedTransactions    prometheus.Counter
	revertedTransactions     prometheus.Counter
	sequencerStage           *prometheus.HistogramVec
	activeCoordinators       prometheus.Gauge
	activeSequencers         prometheus.Gauge
	coordinatingTransactions prometheus.Gauge
}

func InitMetrics(ctx context.Context, registry *prometheus.Registry) *distributedSequencerMetrics {
	metrics := &distributedSequencerMetrics{}

	metrics.acceptedTransactions = prometheus.NewCounter(prometheus.CounterOpts{Name: "accepted_txns_total",
		Help: "Distributed sequencer accepted transactions", Subsystem: METRICS_SUBSYSTEM})
	metrics.assembledTransactions = prometheus.NewCounter(prometheus.CounterOpts{Name: "assembled_txns_total",
		Help: "Distributed sequencer assembled transactions", Subsystem: METRICS_SUBSYSTEM})
	metrics.endorsedTransactions = prometheus.NewCounter(prometheus.CounterOpts{Name: "endorsed_txns_total",
		Help: "Distributed sequencer endorsed transactions", Subsystem: METRICS_SUBSYSTEM})
	metrics.dispatchedTransactions = prometheus.NewCounter(prometheus.CounterOpts{Name: "dispatched_txns_total",
		Help: "Distributed sequencer dispatched transactions", Subsystem: METRICS_SUBSYSTEM})
	metrics.confirmedTransactions = prometheus.NewCounter(prometheus.CounterOpts{Name: "confirmed_txns_total",
		Help: "Distributed sequencer confirmed transactions", Subsystem: METRICS_SUBSYSTEM})
	metrics.revertedTransactions = prometheus.NewCounter(prometheus.CounterOpts{Name: "reverted_txns_total",
		Help: "Distributed sequencer reverted transactions", Subsystem: METRICS_SUBSYSTEM})
	metrics.sequencerStage = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "sequencer_stage",
		Help: "Distributed sequencer stage", Subsystem: METRICS_SUBSYSTEM, Buckets: []float64{5, 10, 20, 40, 80, 200, 400, 800, 1600}}, []string{"stage"})
	metrics.activeCoordinators = prometheus.NewGauge(prometheus.GaugeOpts{Name: "active_coordinators",
		Help: "Distributed sequencer active coordinators", Subsystem: METRICS_SUBSYSTEM})
	metrics.activeSequencers = prometheus.NewGauge(prometheus.GaugeOpts{Name: "active_sequencers",
		Help: "Distributed sequencer active sequencers", Subsystem: METRICS_SUBSYSTEM})
	metrics.coordinatingTransactions = prometheus.NewGauge(prometheus.GaugeOpts{Name: "coordinating_txns",
		Help: "Distributed sequencer coordinating transactions", Subsystem: METRICS_SUBSYSTEM})
	registry.MustRegister(metrics.acceptedTransactions)
	registry.MustRegister(metrics.assembledTransactions)
	registry.MustRegister(metrics.endorsedTransactions)
	registry.MustRegister(metrics.dispatchedTransactions)
	registry.MustRegister(metrics.confirmedTransactions)
	registry.MustRegister(metrics.sequencerStage)
	registry.MustRegister(metrics.activeCoordinators)
	registry.MustRegister(metrics.activeSequencers)
	registry.MustRegister(metrics.coordinatingTransactions)
	return metrics
}

func (dtm *distributedSequencerMetrics) IncAcceptedTransactions() {
	dtm.acceptedTransactions.Inc()
}

func (dtm *distributedSequencerMetrics) IncAssembledTransactions() {
	dtm.assembledTransactions.Inc()
}

func (dtm *distributedSequencerMetrics) IncEndorsedTransactions() {
	dtm.endorsedTransactions.Inc()
}

func (dtm *distributedSequencerMetrics) IncDispatchedTransactions() {
	dtm.dispatchedTransactions.Inc()
}

func (dtm *distributedSequencerMetrics) IncConfirmedTransactions() {
	dtm.confirmedTransactions.Inc()
}

func (dtm *distributedSequencerMetrics) IncRevertedTransactions() {
	dtm.revertedTransactions.Inc()
}

func (dtm *distributedSequencerMetrics) ObserveSequencerTXStateChange(state string, duration time.Duration) {
	dtm.sequencerStage.WithLabelValues(state).Observe(float64(duration.Milliseconds()))
}

func (dtm *distributedSequencerMetrics) SetActiveCoordinators(numberOfActiveCoordinators int) {
	dtm.activeCoordinators.Set(float64(numberOfActiveCoordinators))
}

func (dtm *distributedSequencerMetrics) SetActiveSequencers(numberOfActiveSequencers int) {
	dtm.activeSequencers.Set(float64(numberOfActiveSequencers))
}

func (dtm *distributedSequencerMetrics) IncCoordinatingTransactions() {
	dtm.coordinatingTransactions.Inc()
}
func (dtm *distributedSequencerMetrics) DecCoordinatingTransactions() {
	dtm.coordinatingTransactions.Dec()
}
