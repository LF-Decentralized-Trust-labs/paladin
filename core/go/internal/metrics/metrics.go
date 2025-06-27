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
	"time"

	"github.com/hyperledger/firefly-common/pkg/metric"
	"github.com/prometheus/client_golang/prometheus"
)

const componentName = "paladin"

var metricsSubsystemName = "core"

type metricsManager struct {
	ctx             context.Context
	timeMap         map[string]time.Time
	metricManager   metric.MetricsManager
	metricsRegistry *prometheus.Registry
}

func NewMetricsManager(ctx context.Context) Metrics {
	registry := prometheus.NewRegistry()

	mm := &metricsManager{
		ctx:             ctx,
		timeMap:         make(map[string]time.Time),
		metricsRegistry: registry,
	}

	return mm
}

func (mm *metricsManager) MetricManager() metric.MetricsManager {
	return mm.metricManager
}

func (mm *metricsManager) Registry() *prometheus.Registry {
	return mm.metricsRegistry
}

type Metrics interface {
	MetricManager() metric.MetricsManager
	Registry() *prometheus.Registry
}
