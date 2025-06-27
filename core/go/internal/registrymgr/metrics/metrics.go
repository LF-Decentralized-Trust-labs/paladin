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

type RegistryManagerMetrics interface {
	IncRegistries()
}

var METRICS_SUBSYSTEM = "registry_manager"

type registryManagerMetrics struct {
	registries prometheus.Counter
}

func InitMetrics(ctx context.Context, registry *prometheus.Registry) *registryManagerMetrics {
	metrics := &registryManagerMetrics{}

	metrics.registries = prometheus.NewCounter(prometheus.CounterOpts{Name: "registries_total",
		Help: "Registries created", Subsystem: METRICS_SUBSYSTEM})

	registry.MustRegister(metrics.registries)
	return metrics
}

func (dtm *registryManagerMetrics) IncRegistries() {
	dtm.registries.Inc()
}
