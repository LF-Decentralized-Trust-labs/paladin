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

type TransactionManagerMetrics interface {
	IncRpc(method string)
}

var METRICS_SUBSYSTEM = "transaction_manager"

type transactionManagerMetrics struct {
	rpc *prometheus.CounterVec
}

func InitMetrics(ctx context.Context, registry *prometheus.Registry) *transactionManagerMetrics {
	metrics := &transactionManagerMetrics{}

	labels := []string{"method"}
	metrics.rpc = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "rpc_total",
		Help: "Transaction manager RPC calls", Subsystem: METRICS_SUBSYSTEM}, labels)

	registry.MustRegister(metrics.rpc)
	return metrics
}

func (dtm *transactionManagerMetrics) IncRpc(method string) {
	labels := prometheus.Labels{"method": method}
	dtm.rpc.With(labels).Inc()
}
